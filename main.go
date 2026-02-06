package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// --- Styles ---
var (
	appStyle   = lipgloss.NewStyle().Padding(1, 2)
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)
	statusMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A0A0A0")).
				Render

	// FilePicker Styles
	fpDirStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	fpFileStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	fpSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
)

// --- Data Models ---

type Host struct {
	ID           string `json:"id"`
	Alias        string `json:"alias"`
	Hostname     string `json:"hostname"`
	User         string `json:"user"`
	Port         string `json:"port"`
	IdentityFile string `json:"identity_file,omitempty"`
	Password     string `json:"password,omitempty"`

	// Docker Support
	Containers  []Host `json:"containers,omitempty"` // Nested hosts (containers)
	IsContainer bool   `json:"is_container,omitempty"`
	Expanded    bool   `json:"-"` // UI State
	ParentID    string `json:"-"` // Reference to parent (SSH host)
}

// FilterValue implements list.Item
func (h Host) FilterValue() string { return h.Alias + " " + h.Hostname }
func (h Host) Title() string {
	if h.IsContainer {
		return "  🐳 " + h.Alias
	}
	prefix := "▶ "
	if h.Expanded {
		prefix = "▼ "
	}
	return prefix + h.Alias
}
func (h Host) Description() string {
	if h.IsContainer {
		return fmt.Sprintf("Container: %s", h.Hostname)
	}
	desc := fmt.Sprintf("%s@%s", h.User, h.Hostname)
	if h.Port != "" && h.Port != "22" {
		desc += fmt.Sprintf(":%s", h.Port)
	}
	return desc
}

// --- Config Management ---

func getConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "hosts.json"
	}
	return filepath.Join(home, ".config", "asshi", "hosts.json")
}

func shouldPersistPassword() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ASSHI_STORE_PASSWORD")))
	return value == "1" || value == "true" || value == "yes"
}

func allowInsecureTest() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ASSHI_INSECURE_TEST")))
	return value == "1" || value == "true" || value == "yes"
}

func sanitizeHostsForSave(hosts []Host) []Host {
	sanitized := make([]Host, len(hosts))
	for i, h := range hosts {
		sanitized[i] = h
		if !shouldPersistPassword() {
			sanitized[i].Password = ""
		}
		if len(h.Containers) > 0 {
			sanitized[i].Containers = sanitizeHostsForSave(h.Containers)
		}
	}
	return sanitized
}

func newHostID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func ensureHostIDs(hosts []Host) ([]Host, bool) {
	changed := false
	for i := range hosts {
		if hosts[i].ID == "" {
			hosts[i].ID = newHostID()
			changed = true
		}
		if len(hosts[i].Containers) > 0 {
			var childChanged bool
			hosts[i].Containers, childChanged = ensureHostIDs(hosts[i].Containers)
			if childChanged {
				changed = true
			}
		}
	}
	return hosts, changed
}

func loadHosts() []Host {
	path := getConfigPath()
	f, err := os.Open(path)
	if err != nil {
		// Return default/example data if no config exists
		return []Host{
			{ID: newHostID(), Alias: "Localhost", Hostname: "127.0.0.1", User: "root", Port: "22"},
		}
	}
	defer f.Close()

	var hosts []Host
	bytes, _ := io.ReadAll(f)
	json.Unmarshal(bytes, &hosts)
	return hosts
}

func saveHosts(hosts []Host) error {
	path := getConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sanitized := sanitizeHostsForSave(hosts)
	bytes, _ := json.MarshalIndent(sanitized, "", "  ")
	_, err = f.Write(bytes)
	return err
}

// --- Main Model ---

type state int

const (
	stateList state = iota
	stateForm
	stateFilePicker
)

type model struct {
	list         list.Model
	rawHosts     []Host // Source of truth for tree structure
	inputs       []textinput.Model
	filepicker   filepicker.Model
	focusIndex   int
	state        state
	selectedHost *Host // For editing
	err          error
	quitting     bool
	sshToRun     *Host  // If set, will exec ssh on quit
	testStatus   string // Status message for connection test
	testResult   bool   // true = success, false = failure
}

type scanDockerMsg struct {
	hostIndex  int
	containers []Host
	err        error
}

type testConnectionMsg struct {
	err error
}

func scanDockerContainers(h Host, index int) tea.Cmd {
	return func() tea.Msg {
		// Run ssh command to get docker containers
		// docker ps --format "{{.ID}}|{{.Names}}|{{.Image}}"
		cmdStr := `docker ps --format "{{.ID}}|{{.Names}}|{{.Image}}"`

		args := []string{
			"-o", "BatchMode=yes",
			"-o", "ConnectTimeout=5",
		}
		args = append(args, h.Hostname)
		if h.User != "" {
			args = append([]string{"-l", h.User}, args...)
		}
		if h.Port != "" {
			args = append([]string{"-p", h.Port}, args...)
		}
		if h.IdentityFile != "" {
			args = append([]string{"-i", expandPath(h.IdentityFile)}, args...)
		}
		// If password exists, use sshpass?
		// For simplicity, we assume key-based or agent for scanning to avoid hanging
		// Or we can use the same sshpass logic if available

		finalCmd := "ssh"
		sshArgs := append(args, cmdStr)

		if h.Password != "" {
			sshpassPath, err := exec.LookPath("sshpass")
			if err == nil {
				finalBinary := sshpassPath
				newArgs := []string{"-p", h.Password, "ssh"}
				sshArgs = append(newArgs, sshArgs...)
				finalCmd = finalBinary
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, finalCmd, sshArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return scanDockerMsg{hostIndex: index, err: fmt.Errorf("scan timed out")}
			}
			return scanDockerMsg{hostIndex: index, err: fmt.Errorf("scan failed: %v", err)}
		}

		var containers []Host
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) >= 2 {
				id := parts[0]
				name := parts[1]
				// image := parts[2]
				containers = append(containers, Host{
					ID:          newHostID(),
					Alias:       name,
					Hostname:    id,     // Use ID as "hostname" for exec
					User:        "root", // Default to root inside container
					IsContainer: true,
					ParentID:    h.ID,
				})
			}
		}
		return scanDockerMsg{hostIndex: index, containers: containers}
	}
}

// Helper to flatten the tree for list view
func flattenHosts(hosts []Host) []list.Item {
	var items []list.Item
	for i := range hosts {
		h := hosts[i]
		// Create a copy to avoid modifying original state if needed
		// but here we want to reference the expanded state
		items = append(items, h)
		if h.Expanded {
			for j := range h.Containers {
				c := h.Containers[j]
				// Ensure container knows its parent for connection resolution
				c.ParentID = h.ID
				items = append(items, c)
			}
		}
	}
	return items
}

func initialModel() model {
	hosts := loadHosts()
	var updated bool
	hosts, updated = ensureHostIDs(hosts)
	if updated {
		_ = saveHosts(hosts)
	}
	items := flattenHosts(hosts)

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Asshi Sessions"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	// Form inputs
	inputs := make([]textinput.Model, 6)
	labels := []string{"Alias", "Hostname", "User", "Port (22)", "Identity File (Optional)", "Password (Optional)"}
	for i := range inputs {
		t := textinput.New()
		t.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
		t.Prompt = fmt.Sprintf("% -25s", labels[i]+":")
		t.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
		if i == 5 {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}
		inputs[i] = t
	}

	fp := filepicker.New()
	fp.AllowedTypes = []string{} // All files
	fp.CurrentDirectory, _ = os.UserHomeDir()
	fp.ShowHidden = true
	fp.Styles.Directory = fpDirStyle
	fp.Styles.File = fpFileStyle
	fp.Styles.Selected = fpSelectedStyle

	return model{
		list:       l,
		rawHosts:   hosts,
		inputs:     inputs,
		filepicker: fp,
		state:      stateList,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func hostKeyCallback() ssh.HostKeyCallback {
	if allowInsecureTest() {
		return ssh.InsecureIgnoreHostKey()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ssh.InsecureIgnoreHostKey()
	}
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	cb, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return ssh.InsecureIgnoreHostKey()
	}
	return cb
}

func findHostIndexByID(hosts []Host, id string) int {
	for i := range hosts {
		if hosts[i].ID == id {
			return i
		}
	}
	return -1
}

func expandPath(path string) string {
	if path == "" {
		return path
	}
	path = os.ExpandEnv(path)
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			if strings.HasPrefix(path, "~/") {
				return filepath.Join(home, path[2:])
			}
		}
	}
	return path
}

func formatTestStatus(err error) (string, bool) {
	if err == nil {
		return "Connection successful", true
	}
	var keyErr *knownhosts.KeyError
	if errors.As(err, &keyErr) {
		if len(keyErr.Want) == 0 {
			return "Host key is unknown. Run `ssh <host>` once or set ASSHI_INSECURE_TEST=1 to bypass for testing.", false
		}
		return "Host key mismatch in ~/.ssh/known_hosts. Refusing to connect.", false
	}
	var revokedErr *knownhosts.RevokedError
	if errors.As(err, &revokedErr) {
		return "Host key is revoked in ~/.ssh/known_hosts.", false
	}
	return err.Error(), false
}

func testConnection(h Host) tea.Cmd {
	return func() tea.Msg {
		if h.Hostname == "" {
			return testConnectionMsg{err: fmt.Errorf("hostname required")}
		}
		port := h.Port
		if port == "" {
			port = "22"
		}
		user := h.User
		if user == "" {
			user = os.Getenv("USER")
			if user == "" {
				return testConnectionMsg{err: fmt.Errorf("user required")}
			}
		}

		config := &ssh.ClientConfig{
			User:            user,
			HostKeyCallback: hostKeyCallback(),
			Timeout:         5 * time.Second,
		}

		var auths []ssh.AuthMethod
		var authIssues []string

		// 1. Password
		if h.Password != "" {
			auths = append(auths, ssh.Password(h.Password))
		} else {
			authIssues = append(authIssues, "password not set")
		}

		// 2. Identity File
		if h.IdentityFile != "" {
			keyPath := expandPath(h.IdentityFile)
			key, err := os.ReadFile(keyPath)
			if err != nil {
				authIssues = append(authIssues, fmt.Sprintf("identity file read failed: %s", err))
			} else {
				signer, err := ssh.ParsePrivateKey(key)
				if err != nil {
					var passErr *ssh.PassphraseMissingError
					if errors.As(err, &passErr) {
						authIssues = append(authIssues, "identity file is encrypted; use ssh-agent")
					} else {
						authIssues = append(authIssues, fmt.Sprintf("identity file parse failed: %s", err))
					}
				} else {
					auths = append(auths, ssh.PublicKeys(signer))
				}
			}
		} else {
			authIssues = append(authIssues, "identity file not set")
		}

		// 3. SSH Agent (if available)
		var agentConn net.Conn
		if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
			conn, err := net.Dial("unix", socket)
			if err == nil {
				agentConn = conn
				ag := agent.NewClient(conn)
				if signers, err := ag.Signers(); err == nil {
					if len(signers) > 0 {
						auths = append(auths, ssh.PublicKeys(signers...))
					} else {
						authIssues = append(authIssues, "ssh-agent has no keys loaded")
					}
				} else {
					authIssues = append(authIssues, fmt.Sprintf("ssh-agent error: %s", err))
				}
			} else {
				authIssues = append(authIssues, fmt.Sprintf("ssh-agent socket error: %s", err))
			}
		} else {
			authIssues = append(authIssues, "SSH_AUTH_SOCK not set")
		}
		if agentConn != nil {
			defer agentConn.Close()
		}

		// If no auth provided, just check TCP connectivity
		if len(auths) == 0 {
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(h.Hostname, port), 2*time.Second)
			if err != nil {
				return testConnectionMsg{err: fmt.Errorf("unreachable: %v", err)}
			}
			conn.Close()
			msg := "reachable, but no auth provided"
			if len(authIssues) > 0 {
				msg = msg + " (" + strings.Join(authIssues, "; ") + ")"
			}
			return testConnectionMsg{err: fmt.Errorf("%s", msg)}
		}

		config.Auth = auths
		client, err := ssh.Dial("tcp", net.JoinHostPort(h.Hostname, port), config)
		if err != nil {
			return testConnectionMsg{err: err}
		}
		defer client.Close()

		// Optional: Run a dummy command?
		// session, err := client.NewSession()
		// if err == nil { defer session.Close(); session.Run("exit") }

		return testConnectionMsg{err: nil}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case testConnectionMsg:
		m.testStatus, m.testResult = formatTestStatus(msg.err)
		return m, nil
	case scanDockerMsg:
		if msg.err != nil {
			// Handle error
		} else {
			if msg.hostIndex >= 0 && msg.hostIndex < len(m.rawHosts) {
				m.rawHosts[msg.hostIndex].Containers = msg.containers
				m.rawHosts[msg.hostIndex].Expanded = true
				m.list.SetItems(flattenHosts(m.rawHosts))
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 2)
		m.filepicker.Height = msg.Height - 5
		return m, nil

	case tea.KeyMsg:
		if m.state == stateList {
			if m.list.FilterState() == list.Filtering {
				break // Let the list handle input if filtering
			}
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "q":
				if m.list.FilterState() != list.Filtering {
					m.quitting = true
					return m, tea.Quit
				}
			case "n":
				m.state = stateForm
				m.selectedHost = nil // New host
				m.resetForm()
				return m, nil
			case "enter", "space":
				// Toggle expand/collapse if it has containers
				// Or connect
				if i, ok := m.list.SelectedItem().(Host); ok {
					if i.IsContainer {
						m.sshToRun = &i
						return m, tea.Quit
					} else {
						// For regular host, toggle if space
						if msg.String() == "space" {
							for idx, h := range m.rawHosts {
								if h.ID == i.ID {
									m.rawHosts[idx].Expanded = !m.rawHosts[idx].Expanded
									m.list.SetItems(flattenHosts(m.rawHosts))
									return m, nil
								}
							}
						}

						// If enter, connect
						if msg.String() == "enter" {
							m.sshToRun = &i
							return m, tea.Quit
						}
					}
				}
			case "right":
				if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
					for idx, h := range m.rawHosts {
						if h.ID == i.ID {
							if !h.Expanded {
								m.rawHosts[idx].Expanded = true

								// Auto-scan if empty
								if len(h.Containers) == 0 {
									// Return batch command: Update list AND scan
									// We need to update list immediately to show expanded state (even if empty)
									m.list.SetItems(flattenHosts(m.rawHosts))
									return m, scanDockerContainers(m.rawHosts[idx], idx)
								}

								m.list.SetItems(flattenHosts(m.rawHosts))
							}
							return m, nil
						}
					}
				}
			case "left":
				if i, ok := m.list.SelectedItem().(Host); ok {
					if !i.IsContainer {
						// Collapse if expanded
						for idx, h := range m.rawHosts {
							if h.ID == i.ID {
								if h.Expanded {
									m.rawHosts[idx].Expanded = false
									m.list.SetItems(flattenHosts(m.rawHosts))
								}
								return m, nil
							}
						}
					}
				}
			case "ctrl+d":
				// Scan Docker containers for selected host
				if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
					// Find index in rawHosts
					idx := findHostIndexByID(m.rawHosts, i.ID)
					if idx != -1 {
						return m, scanDockerContainers(m.rawHosts[idx], idx)
					}
				}
			case "e":
				if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
					m.state = stateForm
					m.selectedHost = &i
					m.populateForm(i)
					return m, nil
				}
			case "d":
				// Delete
				if index := m.list.Index(); index >= 0 && len(m.list.Items()) > 0 {
					// Need to remove from rawHosts
					if i, ok := m.list.SelectedItem().(Host); ok {
						for idx, h := range m.rawHosts {
							if h.ID == i.ID {
								m.rawHosts = append(m.rawHosts[:idx], m.rawHosts[idx+1:]...)
								break
							}
						}
						m.list.SetItems(flattenHosts(m.rawHosts))
						m.save()
					}
				}
			}
		} else if m.state == stateFilePicker {
			switch msg.String() {
			case "esc", "q":
				m.state = stateForm
				return m, nil
			}
		} else if m.state == stateForm {
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "ctrl+t":
				// Test connection from form values
				h := Host{
					Hostname:     m.inputs[1].Value(),
					User:         m.inputs[2].Value(),
					Port:         m.inputs[3].Value(),
					IdentityFile: m.inputs[4].Value(),
					Password:     m.inputs[5].Value(),
				}
				m.testStatus = "Testing connection..."
				m.testResult = false // Reset color to neutral/fail until success
				return m, testConnection(h)
			case "ctrl+f":
				if m.focusIndex == 4 { // Only on Identity File field
					m.state = stateFilePicker
					// Reset filepicker size/path if needed or keep last state
					// m.filepicker.CurrentDirectory, _ = os.UserHomeDir()
					return m, m.filepicker.Init()
				}
			case "esc":
				m.state = stateList
				m.testStatus = "" // Clear status on exit
				return m, nil
			case "tab", "down":
				m.focusIndex++
				if m.focusIndex >= len(m.inputs) {
					m.focusIndex = 0
				}
				cmds = append(cmds, m.focusInputs())
			case "shift+tab", "up":
				m.focusIndex--
				if m.focusIndex < 0 {
					m.focusIndex = len(m.inputs) - 1
				}
				cmds = append(cmds, m.focusInputs())
			case "enter":
				if m.focusIndex == len(m.inputs)-1 {
					m.saveFromForm()
					m.state = stateList
				} else {
					m.focusIndex++
					cmds = append(cmds, m.focusInputs())
				}
			}
		}
	}

	if m.state == stateList {
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.state == stateFilePicker {
		m.filepicker, cmd = m.filepicker.Update(msg)
		cmds = append(cmds, cmd)

		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			m.inputs[4].SetValue(path)
			m.state = stateForm
		} else if didSelect, _ := m.filepicker.DidSelectDisabledFile(msg); didSelect {
			m.state = stateForm
		}

	} else {
		// Update only the focused input to avoid extra work and lag
		if m.focusIndex >= 0 && m.focusIndex < len(m.inputs) {
			m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	if m.state == stateList {
		return appStyle.Render(m.list.View())
	}
	if m.state == stateFilePicker {
		return appStyle.Render(titleStyle.Render("Select Identity File") + "\n\n" + m.filepicker.View() + "\n" + statusMessageStyle("Esc to cancel • Enter to select"))
	}

	// Form View
	s := "\n"
	if m.selectedHost == nil {
		s += titleStyle.Render("New Session") + "\n\n"
	} else {
		s += titleStyle.Render("Edit Session") + "\n\n"
	}

	for i := range m.inputs {
		s += m.inputs[i].View() + "\n"
	}

	s += "\n" + statusMessageStyle("Press Enter on last field to save • Ctrl+T to test • Ctrl+F to pick key • Esc to cancel")

	if m.testStatus != "" {
		color := lipgloss.Color("#FF0000") // Red
		if m.testResult {
			color = lipgloss.Color("#00FF00") // Green
		}
		statusStyle := lipgloss.NewStyle().Foreground(color).Bold(true)
		s += "\n\n" + statusStyle.Render(m.testStatus)
	}
	return appStyle.Render(s)
}

// --- Helpers ---

func (m *model) focusInputs() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := 0; i < len(m.inputs); i++ {
		if i == m.focusIndex {
			cmds[i] = m.inputs[i].Focus()
			m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
			m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
		} else {
			m.inputs[i].Blur()
			m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) resetForm() {
	m.focusIndex = 0
	for i := range m.inputs {
		m.inputs[i].Reset()
		m.inputs[i].Blur()
	}
	m.inputs[0].Focus()
}

func (m *model) populateForm(h Host) {
	m.resetForm()
	m.inputs[0].SetValue(h.Alias)
	m.inputs[1].SetValue(h.Hostname)
	m.inputs[2].SetValue(h.User)
	m.inputs[3].SetValue(h.Port)
	m.inputs[4].SetValue(h.IdentityFile)
	m.inputs[5].SetValue(h.Password)
}

func (m *model) saveFromForm() {
	newHost := Host{
		ID:           "",
		Alias:        m.inputs[0].Value(),
		Hostname:     m.inputs[1].Value(),
		User:         m.inputs[2].Value(),
		Port:         m.inputs[3].Value(),
		IdentityFile: m.inputs[4].Value(),
		Password:     m.inputs[5].Value(),
	}

	if m.selectedHost != nil {
		// Update existing
		for i, h := range m.rawHosts {
			if h.ID == m.selectedHost.ID {
				// Preserve containers/expanded state
				newHost.ID = h.ID
				newHost.Containers = h.Containers
				newHost.Expanded = h.Expanded
				m.rawHosts[i] = newHost
				break
			}
		}
	} else {
		newHost.ID = newHostID()
		m.rawHosts = append(m.rawHosts, newHost)
	}

	m.list.SetItems(flattenHosts(m.rawHosts))
	m.save()
}

func (m *model) save() {
	// We save rawHosts directly
	saveHosts(m.rawHosts)
}

func buildSSHArgs(h Host, forceTTY bool, remoteCmd string) []string {
	args := []string{}
	if forceTTY {
		args = append(args, "-t")
	}
	if h.User != "" {
		args = append(args, "-l", h.User)
	}
	if h.Port != "" {
		args = append(args, "-p", h.Port)
	}
	if h.IdentityFile != "" {
		args = append(args, "-i", expandPath(h.IdentityFile))
	}
	args = append(args, h.Hostname)
	if remoteCmd != "" {
		args = append(args, remoteCmd)
	}
	return args
}

func buildSSHCommand(password string, sshArgs []string) (string, []string, bool) {
	if password == "" {
		return "ssh", sshArgs, true
	}
	sshpassPath, err := exec.LookPath("sshpass")
	if err != nil {
		return "ssh", sshArgs, false
	}
	return sshpassPath, append([]string{"-p", password, "ssh"}, sshArgs...), true
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}

	// Exec SSH after TUI cleanup
	if finalModel, ok := m.(model); ok && finalModel.sshToRun != nil {
		h := finalModel.sshToRun

		fmt.Printf("Connecting to %s...\n", h.Alias)

		var sshArgs []string
		var password string
		if h.IsContainer {
			if h.ParentID == "" {
				fmt.Println("Error: container missing parent host reference.")
				return
			}
			parentIdx := findHostIndexByID(finalModel.rawHosts, h.ParentID)
			if parentIdx == -1 {
				fmt.Println("Error: parent host not found for container.")
				return
			}
			parent := finalModel.rawHosts[parentIdx]
			dockerCmd := fmt.Sprintf("docker exec -it %s /bin/sh", h.Hostname)
			sshArgs = buildSSHArgs(parent, true, dockerCmd)
			password = parent.Password
		} else {
			sshArgs = buildSSHArgs(*h, false, "")
			password = h.Password
		}

		binary, args, ok := buildSSHCommand(password, sshArgs)
		if password != "" && !ok {
			fmt.Println("Warning: Password provided but 'sshpass' not found.")
		}

		finalBinaryPath, lookErr := exec.LookPath(binary)
		if lookErr != nil {
			finalBinaryPath = binary
		}

		env := os.Environ()
		argv := append([]string{binary}, args...)

		if err := syscall.Exec(finalBinaryPath, argv, env); err != nil {
			panic(err)
		}
	}
}
