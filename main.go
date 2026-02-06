package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/filepicker"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/crypto/ssh"
)

// --- Styles ---
var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)
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
	Alias        string `json:"alias"`
	Hostname     string `json:"hostname"`
	User         string `json:"user"`
	Port         string `json:"port"`
	IdentityFile string `json:"identity_file,omitempty"`
	Password     string `json:"password,omitempty"`
	
	// Docker Support
	Containers []Host `json:"containers,omitempty"` // Nested hosts (containers)
	IsContainer bool   `json:"is_container,omitempty"`
	Expanded    bool   `json:"-"` // UI State
	ParentHost  *Host  `json:"-"` // Reference to parent (SSH host)
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

func loadHosts() []Host {
	path := getConfigPath()
	f, err := os.Open(path)
	if err != nil {
		// Return default/example data if no config exists
		return []Host{
			{Alias: "Localhost", Hostname: "127.0.0.1", User: "root", Port: "22"},
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
	bytes, _ := json.MarshalIndent(hosts, "", "  ")
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
	list          list.Model
	rawHosts      []Host // Source of truth for tree structure
	inputs        []textinput.Model
	filepicker    filepicker.Model
	focusIndex    int
	state         state
	selectedHost  *Host // For editing
	err           error
	quitting      bool
	sshToRun      *Host // If set, will exec ssh on quit
	testStatus    string // Status message for connection test
	testResult    bool   // true = success, false = failure
}

type scanDockerMsg struct {
	hostIndex int
	containers []Host
	err error
}

type testConnectionMsg struct {
	err error
}

func scanDockerContainers(h Host, index int) tea.Cmd {
	return func() tea.Msg {
		// Run ssh command to get docker containers
		// docker ps --format "{{.ID}}|{{.Names}}|{{.Image}}"
		cmdStr := `docker ps --format "{{.ID}}|{{.Names}}|{{.Image}}"`
		
		args := []string{h.Hostname}
		if h.User != "" {
			args = append([]string{"-l", h.User}, args...)
		}
		if h.Port != "" {
			args = append([]string{"-p", h.Port}, args...)
		}
		if h.IdentityFile != "" {
			args = append([]string{"-i", h.IdentityFile}, args...)
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

		cmd := exec.Command(finalCmd, sshArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return scanDockerMsg{hostIndex: index, err: fmt.Errorf("scan failed: %v", err)}
		}

		var containers []Host
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" { continue }
			parts := strings.Split(line, "|")
			if len(parts) >= 2 {
				id := parts[0]
				name := parts[1]
				// image := parts[2]
				containers = append(containers, Host{
					Alias: name,
					Hostname: id, // Use ID as "hostname" for exec
					User: "root", // Default to root inside container
					IsContainer: true,
					ParentHost: &h, // We need to store parent connection details?
					// Actually, we can't store pointer to struct in JSON easily
					// We will handle parent relation at runtime or flatten config
				})
			}
		}
		return scanDockerMsg{hostIndex: index, containers: containers}
	}
}

// Helper to flatten the tree for list view
func flattenHosts(hosts []Host) []list.Item {
	var items []list.Item
	for _, h := range hosts {
		// Create a copy to avoid modifying original state if needed
		// but here we want to reference the expanded state
		items = append(items, h)
		if h.Expanded {
			for _, c := range h.Containers {
				// We need to pass parent info to container so it knows how to connect
				c.ParentHost = &h 
				items = append(items, c)
			}
		}
	}
	return items
}

func initialModel() model {
	hosts := loadHosts()
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
			// Fallback to current user? Or strict?
			// Let's assume strict for test
		}

		config := &ssh.ClientConfig{
			User:            user,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For testing only
			Timeout:         5 * time.Second,
		}

		var auths []ssh.AuthMethod

		// 1. Password
		if h.Password != "" {
			auths = append(auths, ssh.Password(h.Password))
		}

		// 2. Identity File
		if h.IdentityFile != "" {
			key, err := os.ReadFile(h.IdentityFile)
			if err == nil {
				signer, err := ssh.ParsePrivateKey(key)
				if err == nil {
					auths = append(auths, ssh.PublicKeys(signer))
				}
			}
		}

		// 3. Agent (Always try if available)
		if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
			/*
			// Commented out to avoid extra dependencies for now, 
			// but could use "golang.org/x/crypto/ssh/agent"
			*/
		}

		// If no auth provided, just check TCP connectivity
		if len(auths) == 0 {
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(h.Hostname, port), 2*time.Second)
			if err != nil {
				return testConnectionMsg{err: fmt.Errorf("unreachable: %v", err)}
			}
			conn.Close()
			return testConnectionMsg{err: fmt.Errorf("reachable, but no auth provided")}
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
								if h.Alias == i.Alias && h.Hostname == i.Hostname {
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
						if h.Alias == i.Alias && h.Hostname == i.Hostname {
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
							if h.Alias == i.Alias && h.Hostname == i.Hostname {
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
					idx := -1
					for k, h := range m.rawHosts {
						if h.Alias == i.Alias && h.Hostname == i.Hostname {
							idx = k
							break
						}
					}
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
							if h.Alias == i.Alias && h.Hostname == i.Hostname {
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
		// Update inputs
		for i := range m.inputs {
			m.inputs[i], cmd = m.inputs[i].Update(msg)
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

	s += "\n" + statusMessageStyle("Press Enter on last field to save • Ctrl+t to test • Ctrl+f to pick key • Esc to cancel")

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
			if h.Alias == m.selectedHost.Alias && h.Hostname == m.selectedHost.Hostname {
				// Preserve containers/expanded state
				newHost.Containers = h.Containers
				newHost.Expanded = h.Expanded
				m.rawHosts[i] = newHost
				break
			}
		}
	} else {
		m.rawHosts = append(m.rawHosts, newHost)
	}

	m.list.SetItems(flattenHosts(m.rawHosts))
	m.save()
}

func (m *model) save() {
	// We save rawHosts directly
	saveHosts(m.rawHosts)
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
		args := []string{h.Hostname}
		if h.User != "" {
			args = append([]string{"-l", h.User}, args...)
		}
		if h.Port != "" {
			args = append([]string{"-p", h.Port}, args...)
		}
		if h.IdentityFile != "" {
			args = append([]string{"-i", h.IdentityFile}, args...)
		}

		fmt.Printf("Connecting to %s...\n", h.Alias)
		
		binary := "ssh"
		// Check for sshpass if password is provided (and not already inside a container which implies parent)
		// Logic:
		// If normal host: ssh [args]
		// If container: ssh [parent-args] -t "docker exec -it [id] /bin/sh"
		
		var finalArgs []string
		
		if h.IsContainer && h.ParentHost != nil {
			// Connect to parent, then run docker exec
			parent := h.ParentHost
			
			// Build parent SSH args
			sshArgs := []string{"-t"} // Force TTY
			if parent.User != "" {
				sshArgs = append(sshArgs, "-l", parent.User)
			}
			if parent.Port != "" {
				sshArgs = append(sshArgs, "-p", parent.Port)
			}
			if parent.IdentityFile != "" {
				sshArgs = append(sshArgs, "-i", parent.IdentityFile)
			}
			sshArgs = append(sshArgs, parent.Hostname)
			
			// Append the docker command
			// docker exec -it [containerID] /bin/sh
			// TODO: Make shell configurable? Default to /bin/sh or /bin/bash
			dockerCmd := fmt.Sprintf("docker exec -it %s /bin/sh", h.Hostname)
			sshArgs = append(sshArgs, dockerCmd)
			
			finalArgs = sshArgs
			
			// Password handling for parent
			if parent.Password != "" {
				sshpassPath, err := exec.LookPath("sshpass")
				if err == nil {
					binary = sshpassPath
					// Prepend ssh and password args
					newArgs := []string{"-p", parent.Password, "ssh"}
					finalArgs = append(newArgs, finalArgs...)
				} else {
					fmt.Println("Warning: Parent password saved but 'sshpass' not found.")
					// Fallback to ssh
					finalArgs = append([]string{"ssh"}, finalArgs...)
				}
			} else {
				// No password, just ssh
				finalArgs = append([]string{"ssh"}, finalArgs...)
			}
			
		} else {
			// Normal SSH
			finalArgs = args
			// Password handling
			if h.Password != "" {
				sshpassPath, err := exec.LookPath("sshpass")
				if err == nil {
					binary = sshpassPath
					newArgs := []string{"-p", h.Password, "ssh"}
					finalArgs = append(newArgs, finalArgs...)
				} else {
					fmt.Println("Warning: Password saved but 'sshpass' not found.")
					finalArgs = append([]string{"ssh"}, finalArgs...)
				}
			} else {
				finalArgs = append([]string{"ssh"}, finalArgs...)
			}
		}

		// Execute
		// finalArgs contains the full command line parts excluding the binary itself if it was sshpass
		// Wait, syscall.Exec expects argv[0] to be the binary name
		
		// Let's simplify the construction above to just be arguments to the binary
		// If binary is sshpass: args = [-p, pass, ssh, ...]
		// If binary is ssh: args = [...]
		
		finalBinaryPath, lookErr := exec.LookPath(binary)
		if lookErr != nil {
			finalBinaryPath = binary
		}

		env := os.Environ()
		// syscall.Exec(binaryPath, argv, env)
		// argv[0] should be the binary name
		argv := append([]string{binary}, finalArgs[1:]...) // Remove the leading "ssh" or "sshpass" we added artificially in logic above?
		
		// Actually, let's rewrite the logic slightly to be cleaner:
		// We constructed finalArgs assuming it *starts* with the arguments.
		// In the previous block:
		// newArgs := []string{"-p", h.Password, "ssh"} -> "ssh" is an argument to sshpass
		// finalArgs = append(newArgs, args...) -> [-p, pass, ssh, hostname...]
		
		// So argv should be [binary, finalArgs...]
		// But wait, my logic above added "ssh" to finalArgs in the 'else' blocks too.
		// "finalArgs = append([]string{"ssh"}, finalArgs...)"
		
		// So finalArgs is the FULL command list including the command itself.
		// We need to separate binary and args.
		
		// Let's just trust finalArgs construction:
		// [0] is what we conceptually run.
		// But for syscall.Exec, argv[0] is the name.
		
		// Logic:
		// If binary == sshpass:
		// realArgs = [-p, pass, ssh, host...]
		
		// If binary == ssh:
		// realArgs = [host...] (Original args)
		
		realArgs := finalArgs
		if binary == "sshpass" {
			realArgs = finalArgs
		} else {
			// binary is "ssh"
			// Check if first arg is "ssh" (added by our logic) and strip it
			if len(finalArgs) > 0 && finalArgs[0] == "ssh" {
				realArgs = finalArgs[1:]
			}
		}

		// argv[0] must be the binary name
		argv = append([]string{binary}, realArgs...)

		if err := syscall.Exec(finalBinaryPath, argv, env); err != nil {
			panic(err)
		}
	}
}
