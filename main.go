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
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// --- Theme ---

var (
	// Core palette
	colorPrimary   = lipgloss.Color("#7C3AED") // Vibrant purple
	colorSecondary = lipgloss.Color("#06B6D4") // Cyan
	colorAccent    = lipgloss.Color("#F59E0B") // Amber
	colorSuccess   = lipgloss.Color("#10B981") // Emerald
	colorDanger    = lipgloss.Color("#EF4444") // Red
	colorMuted     = lipgloss.Color("#6B7280") // Gray
	colorSubtle    = lipgloss.Color("#374151") // Dark gray
	colorText      = lipgloss.Color("#F9FAFB") // Near white
	colorDimText   = lipgloss.Color("#9CA3AF") // Dim text
	colorBorder    = lipgloss.Color("#4B5563") // Border gray
	colorHighlight = lipgloss.Color("#A78BFA") // Light purple

	// App chrome
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorPrimary).
			Bold(true).
			Padding(0, 1)

	// Header
	headerStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	headerAccentStyle = lipgloss.NewStyle().
				Foreground(colorSecondary).
				Bold(true)

	headerDimStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	// List item styles
	itemNormalTitle = lipgloss.NewStyle().
			Foreground(colorText).
			PaddingLeft(2)

	itemNormalDesc = lipgloss.NewStyle().
			Foreground(colorDimText).
			PaddingLeft(2)

	itemSelectedTitle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true).
				PaddingLeft(1).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(colorPrimary)

	itemSelectedDesc = lipgloss.NewStyle().
				Foreground(colorHighlight).
				PaddingLeft(1).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(colorPrimary)

	// Form styles
	formBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			Width(60)

	formTitleStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorPrimary).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1)

	formHintStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	formDividerStyle = lipgloss.NewStyle().
				Foreground(colorSubtle)

	// Status bar
	helpBarStyle = lipgloss.NewStyle().
			Foreground(colorDimText).
			Background(lipgloss.Color("#1F2937")).
			Padding(0, 1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(colorDimText)

	helpSepStyle = lipgloss.NewStyle().
			Foreground(colorSubtle)

	// Badge styles
	badgeStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorPrimary).
			Padding(0, 1).
			Bold(true)

	containerBadgeStyle = lipgloss.NewStyle().
				Foreground(colorText).
				Background(lipgloss.Color("#0891B2")).
				Padding(0, 1).
				Bold(true)

	// Test result styles
	testSuccessStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true)

	testFailStyle = lipgloss.NewStyle().
			Foreground(colorDanger).
			Bold(true)

	testPendingStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	// FilePicker Styles
	fpDirStyle      = lipgloss.NewStyle().Foreground(colorSecondary)
	fpFileStyle     = lipgloss.NewStyle().Foreground(colorText)
	fpSelectedStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)

	// File picker box
	fpBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSecondary).
			Padding(1, 2)

	// Spinner style
	spinnerStyle = lipgloss.NewStyle().Foreground(colorSecondary)
)

// --- ASCII Art Header ---

func renderHeader(hostCount int, containerCount int) string {
	lines := []string{
		`                     .__    .__`,
		`_____    ______ _____|  |__ |__|`,
		`\__  \  /  ___//  ___/  |  \|  |`,
		` / __ \_\___ \ \___ \|   Y  \  |`,
		`(____  /____  >____  >___|  /__|`,
		`     \/     \/     \/     \/    `,
	}

	colors := []lipgloss.Color{
		colorPrimary,   // purple
		colorPrimary,   // purple
		colorHighlight, // light purple
		colorHighlight, // light purple
		colorSecondary, // cyan
		colorSecondary, // cyan
	}

	var logo strings.Builder
	for i, line := range lines {
		style := lipgloss.NewStyle().Foreground(colors[i]).Bold(true)
		logo.WriteString("  " + style.Render(line) + "\n")
	}

	stats := headerDimStyle.Render(fmt.Sprintf("  %d hosts", hostCount))
	if containerCount > 0 {
		stats += headerDimStyle.Render(fmt.Sprintf(" · %d containers", containerCount))
	}

	return logo.String() + stats + "\n"
}

// --- Help Bar ---

func helpEntry(key, desc string) string {
	return helpKeyStyle.Render(key) + " " + helpDescStyle.Render(desc)
}

func renderListHelp() string {
	entries := []string{
		helpEntry("n", "new"),
		helpEntry("e", "edit"),
		helpEntry("d", "delete"),
		helpEntry("enter", "connect"),
		helpEntry("/", "filter"),
		helpEntry("space", "expand"),
		helpEntry("ctrl+d", "scan"),
		helpEntry("q", "quit"),
	}
	sep := helpSepStyle.Render(" | ")
	return helpBarStyle.Render(strings.Join(entries, sep))
}

func renderFormHelp() string {
	entries := []string{
		helpEntry("tab", "next"),
		helpEntry("enter", "save"),
		helpEntry("ctrl+t", "test"),
		helpEntry("enter on pick", "file picker"),
		helpEntry("arrows on group", "select"),
		helpEntry("esc", "cancel"),
	}
	sep := helpSepStyle.Render(" | ")
	return helpBarStyle.Render(strings.Join(entries, sep))
}

func renderFilePickerHelp() string {
	entries := []string{
		helpEntry("arrows", "navigate"),
		helpEntry("enter", "select"),
		helpEntry("esc", "cancel"),
	}
	sep := helpSepStyle.Render(" | ")
	return helpBarStyle.Render(strings.Join(entries, sep))
}

// --- Custom List Delegate ---

type hostDelegate struct{}

func (d hostDelegate) Height() int                             { return 2 }
func (d hostDelegate) Spacing() int                            { return 1 }
func (d hostDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d hostDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	isSelected := index == m.Index()

	if g, ok := listItem.(groupItem); ok {
		icon := " ▶ "
		if g.Expanded {
			icon = " ▼ "
		}
		title := "📁 " + g.Name
		desc := "  group"
		if isSelected {
			fmt.Fprintf(w, "%s", itemSelectedTitle.Render(strings.TrimLeft(icon+title, " ")))
			fmt.Fprintf(w, "\n%s", itemSelectedDesc.Render(strings.TrimLeft(desc, " ")))
		} else {
			fmt.Fprintf(w, "%s", itemNormalTitle.Render(strings.TrimLeft(icon+title, " ")))
			fmt.Fprintf(w, "\n%s", itemNormalDesc.Render(strings.TrimLeft(desc, " ")))
		}
		return
	}

	h, ok := listItem.(Host)
	if !ok {
		return
	}

	// Build the icon and title
	var icon, title, desc string
	indent := strings.Repeat("  ", h.ListIndent)

	if h.IsContainer {
		icon = "📦 "
		title = h.Alias
		desc = fmt.Sprintf("container %s", h.Hostname)
	} else {
		if h.Expanded {
			icon = "▼ "
		} else {
			icon = "▶ "
		}

		// Auth indicator
		authIcon := "🌐 " // globe - no specific auth
		if h.IdentityFile != "" {
			authIcon = "🔑 " // key
		} else if h.Password != "" {
			authIcon = "🔒 " // lock
		}

		title = authIcon + h.Alias

		desc = ""
		connStr := fmt.Sprintf("%s@%s", h.User, h.Hostname)
		if h.Port != "" && h.Port != "22" {
			connStr += fmt.Sprintf(":%s", h.Port)
		}
		desc = connStr

		if len(h.Containers) > 0 {
			desc += fmt.Sprintf(" [%d containers]", len(h.Containers))
		}
	}

	if isSelected {
		fmt.Fprintf(w, "%s", itemSelectedTitle.Render(indent+icon+title))
		fmt.Fprintf(w, "\n%s", itemSelectedDesc.Render(indent+"  "+desc))
	} else {
		fmt.Fprintf(w, "%s", itemNormalTitle.Render(indent+icon+title))
		fmt.Fprintf(w, "\n%s", itemNormalDesc.Render(indent+"  "+desc))
	}
}

// statusMessageStyle kept as a func for backwards compat with filepicker hint
func statusMessageStyle(s string) string {
	return formHintStyle.Render(s)
}

// --- Data Models ---

type Host struct {
	ID           string `json:"id"`
	Alias        string `json:"alias"`
	Hostname     string `json:"hostname"`
	User         string `json:"user"`
	Port         string `json:"port"`
	IdentityFile string `json:"identity_file,omitempty"`
	Password     string `json:"password,omitempty"`
	GroupID      string `json:"group_id,omitempty"`

	// Docker Support
	Containers  []Host `json:"containers,omitempty"` // Nested hosts (containers)
	IsContainer bool   `json:"is_container,omitempty"`
	Expanded    bool   `json:"-"` // UI State
	ParentID    string `json:"-"` // Reference to parent (SSH host)
	ListIndent  int    `json:"-"` // UI indent level for tree rendering
}

type Group struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Expanded bool   `json:"expanded,omitempty"`
}

type groupItem struct {
	Group
}

func (g groupItem) FilterValue() string { return g.Name }
func (g groupItem) Title() string       { return g.Name }
func (g groupItem) Description() string { return "group" }

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
	if value == "" {
		return true
	}
	return value != "0" && value != "false" && value != "no"
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

func ensureGroupIDs(groups []Group) ([]Group, bool) {
	changed := false
	for i := range groups {
		if groups[i].ID == "" {
			groups[i].ID = newHostID()
			changed = true
		}
	}
	return groups, changed
}

type configFile struct {
	Groups []Group `json:"groups,omitempty"`
	Hosts  []Host  `json:"hosts,omitempty"`
}

func loadConfig() ([]Group, []Host) {
	path := getConfigPath()
	f, err := os.Open(path)
	if err != nil {
		// Return default/example data if no config exists
		return []Group{}, []Host{
			{ID: newHostID(), Alias: "Localhost", Hostname: "127.0.0.1", User: "root", Port: "22"},
		}
	}
	defer f.Close()

	bytes, _ := io.ReadAll(f)

	var cfg configFile
	if err := json.Unmarshal(bytes, &cfg); err == nil && (len(cfg.Hosts) > 0 || len(cfg.Groups) > 0) {
		return cfg.Groups, cfg.Hosts
	}

	// Backward compatibility with old hosts-only format.
	var hosts []Host
	_ = json.Unmarshal(bytes, &hosts)
	return []Group{}, hosts
}

func saveConfig(groups []Group, hosts []Host) error {
	path := getConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sanitizedHosts := sanitizeHostsForSave(hosts)
	cfg := configFile{
		Groups: groups,
		Hosts:  sanitizedHosts,
	}
	bytes, _ := json.MarshalIndent(cfg, "", "  ")
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
	rawGroups    []Group
	rawHosts     []Host // Source of truth for tree structure
	inputs       []textinput.Model
	filepicker   filepicker.Model
	spinner      spinner.Model
	focusIndex   int
	state        state
	selectedHost *Host // For editing
	err          error
	quitting     bool
	sshToRun     *Host  // If set, will exec ssh on quit
	testStatus   string // Status message for connection test
	testResult   bool   // true = success, false = failure
	scanning     bool   // true while Docker scan in progress
	testing      bool   // true while connection test in progress
	width        int    // terminal width
	height       int    // terminal height
	formError    string // inline form validation/action error
	keyPickFocus bool   // true when [Pick] button on key field is focused
	groupOptions []string
	groupIndex   int
	groupCustom  bool
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
func flattenHosts(groups []Group, hosts []Host) []list.Item {
	var items []list.Item

	// Ungrouped hosts first.
	for i := range hosts {
		if hosts[i].GroupID != "" {
			continue
		}
		h := hosts[i]
		h.ListIndent = 0
		items = append(items, h)
		if h.Expanded {
			for j := range h.Containers {
				c := h.Containers[j]
				c.ParentID = h.ID
				c.ListIndent = 1
				items = append(items, c)
			}
		}
	}

	// Then grouped hosts under each group row.
	for i := range groups {
		g := groups[i]
		items = append(items, groupItem{Group: g})
		if !g.Expanded {
			continue
		}
		for j := range hosts {
			if hosts[j].GroupID != g.ID {
				continue
			}
			h := hosts[j]
			h.ListIndent = 1
			items = append(items, h)
			if h.Expanded {
				for k := range h.Containers {
					c := h.Containers[k]
					c.ParentID = h.ID
					c.ListIndent = 2
					items = append(items, c)
				}
			}
		}
	}
	return items
}

func countContainers(hosts []Host) int {
	count := 0
	for _, h := range hosts {
		count += len(h.Containers)
	}
	return count
}

func newFormInputs() []textinput.Model {
	inputs := make([]textinput.Model, 7)
	labels := []string{"Alias", "Hostname", "User", "Port", "Key File", "Password", "Group"}
	placeholders := []string{"my-server", "192.168.1.100", "root", "22", "optional key path", "", "optional group name"}
	for i := range inputs {
		t := textinput.New()
		t.Cursor.Style = lipgloss.NewStyle().Foreground(colorSecondary)
		t.Prompt = fmt.Sprintf("  %-12s", labels[i])
		t.PromptStyle = lipgloss.NewStyle().Foreground(colorHighlight).Bold(true)
		t.TextStyle = lipgloss.NewStyle().Foreground(colorText)
		t.Placeholder = placeholders[i]
		t.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorSubtle)
		if i == 5 {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
			t.Placeholder = "••••••••"
		}
		inputs[i] = t
	}
	return inputs
}

func initialModel() model {
	groups, hosts := loadConfig()
	var hostsUpdated bool
	hosts, hostsUpdated = ensureHostIDs(hosts)
	var groupsUpdated bool
	groups, groupsUpdated = ensureGroupIDs(groups)
	if hostsUpdated || groupsUpdated {
		_ = saveConfig(groups, hosts)
	}
	items := flattenHosts(groups, hosts)

	delegate := hostDelegate{}
	l := list.New(items, delegate, 0, 0)
	l.Title = ""
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.Styles.Title = titleStyle

	inputs := newFormInputs()

	fp := filepicker.New()
	fp.AllowedTypes = []string{} // All files
	fp.CurrentDirectory, _ = os.UserHomeDir()
	fp.ShowHidden = true
	fp.Styles.Directory = fpDirStyle
	fp.Styles.File = fpFileStyle
	fp.Styles.Selected = fpSelectedStyle

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinnerStyle

	return model{
		list:       l,
		rawGroups:  groups,
		rawHosts:   hosts,
		inputs:     inputs,
		filepicker: fp,
		spinner:    sp,
		state:      stateList,
	}
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
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

func findGroupIndexByID(groups []Group, id string) int {
	for i := range groups {
		if groups[i].ID == id {
			return i
		}
	}
	return -1
}

func findGroupByName(groups []Group, name string) int {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return -1
	}
	for i := range groups {
		if strings.ToLower(strings.TrimSpace(groups[i].Name)) == target {
			return i
		}
	}
	return -1
}

func (m *model) buildGroupOptions(selectedName string) {
	m.groupOptions = []string{"(none)"}
	for i := range m.rawGroups {
		m.groupOptions = append(m.groupOptions, m.rawGroups[i].Name)
	}
	m.groupOptions = append(m.groupOptions, "+ New group...")
	m.groupIndex = 0
	m.groupCustom = false

	target := strings.TrimSpace(selectedName)
	if target == "" {
		m.inputs[6].SetValue("(none)")
		return
	}
	for i, opt := range m.groupOptions {
		if strings.EqualFold(opt, target) {
			m.groupIndex = i
			m.inputs[6].SetValue(opt)
			return
		}
	}
	// Unknown group name: switch to custom mode with the provided name.
	m.groupCustom = true
	m.groupIndex = len(m.groupOptions) - 1
	m.inputs[6].SetValue(target)
}

func (m *model) applyGroupSelectionToInput() {
	if m.groupCustom {
		return
	}
	if len(m.groupOptions) == 0 {
		m.inputs[6].SetValue("(none)")
		return
	}
	if m.groupIndex < 0 {
		m.groupIndex = 0
	}
	if m.groupIndex >= len(m.groupOptions) {
		m.groupIndex = len(m.groupOptions) - 1
	}
	m.inputs[6].SetValue(m.groupOptions[m.groupIndex])
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
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case testConnectionMsg:
		m.testStatus, m.testResult = formatTestStatus(msg.err)
		m.testing = false
		return m, nil
	case scanDockerMsg:
		m.scanning = false
		if msg.err != nil {
			// Handle error
		} else {
			if msg.hostIndex >= 0 && msg.hostIndex < len(m.rawHosts) {
				m.rawHosts[msg.hostIndex].Containers = msg.containers
				m.rawHosts[msg.hostIndex].Expanded = true
				m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 12) // Room for header + help bar
		m.filepicker.Height = msg.Height - 8
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
				m.inputs = newFormInputs()
				m.resetForm()
				m.buildGroupOptions("")
				m.formError = ""
				m.keyPickFocus = false
				return m, m.focusInputs()
			case "enter", "space":
				switch i := m.list.SelectedItem().(type) {
				case groupItem:
					for idx := range m.rawGroups {
						if m.rawGroups[idx].ID == i.ID {
							m.rawGroups[idx].Expanded = !m.rawGroups[idx].Expanded
							m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
							return m, nil
						}
					}
				case Host:
					if i.IsContainer {
						m.sshToRun = &i
						return m, tea.Quit
					}
					if msg.String() == "space" {
						for idx, h := range m.rawHosts {
							if h.ID == i.ID {
								m.rawHosts[idx].Expanded = !m.rawHosts[idx].Expanded
								m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
								return m, nil
							}
						}
					}
					if msg.String() == "enter" {
						m.sshToRun = &i
						return m, tea.Quit
					}
				}
			case "right":
				if g, ok := m.list.SelectedItem().(groupItem); ok {
					for idx := range m.rawGroups {
						if m.rawGroups[idx].ID == g.ID {
							if !m.rawGroups[idx].Expanded {
								m.rawGroups[idx].Expanded = true
								m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
							}
							return m, nil
						}
					}
				}
				if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
					for idx, h := range m.rawHosts {
						if h.ID == i.ID {
							if !h.Expanded {
								m.rawHosts[idx].Expanded = true

								// Auto-scan if empty
								if len(h.Containers) == 0 {
									m.scanning = true
									m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
									return m, scanDockerContainers(m.rawHosts[idx], idx)
								}

								m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
							}
							return m, nil
						}
					}
				}
			case "left":
				if g, ok := m.list.SelectedItem().(groupItem); ok {
					for idx := range m.rawGroups {
						if m.rawGroups[idx].ID == g.ID {
							if m.rawGroups[idx].Expanded {
								m.rawGroups[idx].Expanded = false
								m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
							}
							return m, nil
						}
					}
				}
				if i, ok := m.list.SelectedItem().(Host); ok {
					if !i.IsContainer {
						// Collapse if expanded
						for idx, h := range m.rawHosts {
							if h.ID == i.ID {
								if h.Expanded {
									m.rawHosts[idx].Expanded = false
									m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
								}
								return m, nil
							}
						}
					}
				}
			case "ctrl+d":
				// Scan Docker containers for selected host
				if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
					idx := findHostIndexByID(m.rawHosts, i.ID)
					if idx != -1 {
						m.scanning = true
						return m, scanDockerContainers(m.rawHosts[idx], idx)
					}
				}
			case "e":
				if i, ok := m.list.SelectedItem().(Host); ok && !i.IsContainer {
					m.state = stateForm
					m.selectedHost = &i
					m.inputs = newFormInputs()
					m.populateForm(i)
					m.formError = ""
					m.keyPickFocus = false
					return m, m.focusInputs()
				}
			case "d":
				// Delete
				if index := m.list.Index(); index >= 0 && len(m.list.Items()) > 0 {
					if g, ok := m.list.SelectedItem().(groupItem); ok {
						for idx := range m.rawGroups {
							if m.rawGroups[idx].ID == g.ID {
								m.rawGroups = append(m.rawGroups[:idx], m.rawGroups[idx+1:]...)
								break
							}
						}
						for i := range m.rawHosts {
							if m.rawHosts[i].GroupID == g.ID {
								m.rawHosts[i].GroupID = ""
							}
						}
						m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
						m.save()
						return m, nil
					}
					if i, ok := m.list.SelectedItem().(Host); ok {
						for idx, h := range m.rawHosts {
							if h.ID == i.ID {
								m.rawHosts = append(m.rawHosts[:idx], m.rawHosts[idx+1:]...)
								break
							}
						}
						m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
						m.save()
					}
				}
			}
		} else if m.state == stateFilePicker {
			switch msg.String() {
			case "esc", "q":
				m.state = stateForm
				m.keyPickFocus = false
				return m, m.focusInputs()
			}
		} else if m.state == stateForm {
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "ctrl+t":
				h := Host{
					Hostname:     m.inputs[1].Value(),
					User:         m.inputs[2].Value(),
					Port:         m.inputs[3].Value(),
					IdentityFile: m.inputs[4].Value(),
					Password:     m.inputs[5].Value(),
				}
				m.testStatus = ""
				m.testing = true
				return m, testConnection(h)
			case "esc":
				m.state = stateList
				m.testStatus = ""
				m.formError = ""
				m.keyPickFocus = false
				return m, nil
			case "tab", "down":
				if m.focusIndex == 6 && !m.groupCustom {
					if len(m.groupOptions) > 0 {
						m.groupIndex = (m.groupIndex + 1) % len(m.groupOptions)
						m.applyGroupSelectionToInput()
					}
					return m, nil
				}
				if m.focusIndex == 4 && !m.keyPickFocus {
					m.keyPickFocus = true
					return m, nil
				}
				if m.focusIndex == 4 && m.keyPickFocus {
					m.keyPickFocus = false
					m.focusIndex = 5
					return m, m.focusInputs()
				}
				m.focusIndex++
				if m.focusIndex >= len(m.inputs) {
					m.focusIndex = 0
				}
				m.keyPickFocus = false
				return m, m.focusInputs()
			case "shift+tab", "up":
				if m.focusIndex == 6 && !m.groupCustom {
					if len(m.groupOptions) > 0 {
						m.groupIndex--
						if m.groupIndex < 0 {
							m.groupIndex = len(m.groupOptions) - 1
						}
						m.applyGroupSelectionToInput()
					}
					return m, nil
				}
				if m.focusIndex == 5 {
					m.focusIndex = 4
					m.keyPickFocus = true
					return m, nil
				}
				if m.focusIndex == 4 && m.keyPickFocus {
					m.keyPickFocus = false
					return m, m.focusInputs()
				}
				m.focusIndex--
				if m.focusIndex < 0 {
					m.focusIndex = len(m.inputs) - 1
				}
				m.keyPickFocus = false
				return m, m.focusInputs()
			case "enter":
				if m.focusIndex == 4 && m.keyPickFocus {
					m.state = stateFilePicker
					m.keyPickFocus = false
					return m, m.filepicker.Init()
				}
				if m.focusIndex == 6 && !m.groupCustom {
					if len(m.groupOptions) > 0 && m.groupOptions[m.groupIndex] == "+ New group..." {
						m.groupCustom = true
						m.inputs[6].SetValue("")
						m.inputs[6].Placeholder = "new group name"
						return m, m.focusInputs()
					}
				}
				if m.focusIndex == len(m.inputs)-1 {
					if err := m.saveFromForm(); err != nil {
						m.formError = err.Error()
						return m, nil
					}
					m.formError = ""
					m.keyPickFocus = false
					m.state = stateList
					return m, nil
				}
				m.focusIndex++
				m.formError = ""
				m.keyPickFocus = false
				return m, m.focusInputs()
			case "left":
				if m.focusIndex == 6 && !m.groupCustom {
					if len(m.groupOptions) > 0 {
						m.groupIndex--
						if m.groupIndex < 0 {
							m.groupIndex = len(m.groupOptions) - 1
						}
						m.applyGroupSelectionToInput()
					}
					return m, nil
				}
			case "right":
				if m.focusIndex == 6 && !m.groupCustom {
					if len(m.groupOptions) > 0 {
						m.groupIndex = (m.groupIndex + 1) % len(m.groupOptions)
						m.applyGroupSelectionToInput()
					}
					return m, nil
				}
			default:
				if m.focusIndex == 4 && m.keyPickFocus {
					return m, nil
				}
				if m.focusIndex == 6 && !m.groupCustom {
					return m, nil
				}
				// Forward all other keys (typing, backspace, delete, etc.) to focused input
				if m.focusIndex >= 0 && m.focusIndex < len(m.inputs) {
					m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
					return m, cmd
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
			m.inputs[4].CursorEnd()
			m.state = stateForm
			m.keyPickFocus = false
			return m, m.focusInputs()
		} else if didSelect, _ := m.filepicker.DidSelectDisabledFile(msg); didSelect {
			m.state = stateForm
			m.keyPickFocus = false
			return m, m.focusInputs()
		}
	} else if m.state == stateForm {
		// Handle non-key messages (cursor blink, etc.) for focused input
		if m.focusIndex >= 0 && m.focusIndex < len(m.inputs) {
			if m.focusIndex == 4 && m.keyPickFocus {
				return m, tea.Batch(cmds...)
			}
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
		header := renderHeader(len(m.rawHosts), countContainers(m.rawHosts))

		var scanStatus string
		if m.scanning {
			scanStatus = "\n " + m.spinner.View() + " " +
				lipgloss.NewStyle().Foreground(colorSecondary).Render("Scanning containers...") + "\n"
		}

		content := header + m.list.View() + scanStatus
		help := "\n" + renderListHelp()
		return appStyle.Render(content + help)
	}
	if m.state == stateFilePicker {
		title := formTitleStyle.Render("📂 Select Identity File")
		content := fpBoxStyle.Render(m.filepicker.View())
		help := "\n" + renderFilePickerHelp()
		return appStyle.Render(title + "\n\n" + content + help)
	}
	// Form View
	var formTitle string
	if m.selectedHost == nil {
		formTitle = formTitleStyle.Render("✨ New Session")
	} else {
		formTitle = formTitleStyle.Render("✏️  Edit Session")
	}

	divider := formDividerStyle.Render(strings.Repeat("─", 40))

	// Build form content
	var formContent strings.Builder
	formContent.WriteString(formTitle + "\n\n")

	// Connection section
	formContent.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Bold(true).Render("  CONNECTION") + "\n")
	formContent.WriteString(divider + "\n")
	for i := 0; i < 4; i++ {
		formContent.WriteString(m.inputs[i].View() + "\n")
	}

	formContent.WriteString("\n")
	// Auth section
	formContent.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Bold(true).Render("  AUTHENTICATION") + "\n")
	formContent.WriteString(divider + "\n")
	pickStyle := lipgloss.NewStyle().
		Foreground(colorText).
		Background(colorSecondary).
		Bold(true).
		Padding(0, 1)
	if m.focusIndex == 4 && m.keyPickFocus {
		pickStyle = pickStyle.Background(colorPrimary)
	}
	formContent.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, m.inputs[4].View(), "  ", pickStyle.Render("Pick")) + "\n")
	formContent.WriteString(m.inputs[5].View() + "\n")

	formContent.WriteString("\n")
	formContent.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Bold(true).Render("  ORGANIZATION") + "\n")
	formContent.WriteString(divider + "\n")
	if m.groupCustom {
		formContent.WriteString(m.inputs[6].View() + "\n")
	} else {
		groupLabelStyle := lipgloss.NewStyle().Foreground(colorMuted)
		groupValueStyle := lipgloss.NewStyle().Foreground(colorDimText)
		if m.focusIndex == 6 {
			groupLabelStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
			groupValueStyle = lipgloss.NewStyle().Foreground(colorText)
		}
		groupValue := "(none)"
		if len(m.groupOptions) > 0 {
			groupValue = m.groupOptions[m.groupIndex]
		}
		formContent.WriteString(groupLabelStyle.Render("  Group       ") + groupValueStyle.Render("◀ "+groupValue+" ▶") + "\n")
	}

	// Test status
	if m.testing {
		formContent.WriteString("\n " + m.spinner.View() + " " +
			testPendingStyle.Render("Testing connection..."))
	} else if m.testStatus != "" {
		if m.testResult {
			formContent.WriteString("\n  " + testSuccessStyle.Render("✔ "+m.testStatus))
		} else {
			formContent.WriteString("\n  " + testFailStyle.Render("✘ "+m.testStatus))
		}
	}
	if m.formError != "" {
		formContent.WriteString("\n  " + testFailStyle.Render("✘ "+m.formError))
	}

	form := formBoxStyle.Render(formContent.String())
	help := "\n" + renderFormHelp()

	return appStyle.Render(form + help)
}

// --- Helpers ---

func (m *model) focusInputs() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := 0; i < len(m.inputs); i++ {
		if i == m.focusIndex {
			m.keyPickFocus = false
			cmds[i] = m.inputs[i].Focus()
			// Put cursor at end so editing existing values behaves naturally.
			m.inputs[i].CursorEnd()
			m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
			m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(colorText)
		} else {
			m.inputs[i].Blur()
			m.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(colorMuted)
			m.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(colorText)
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) resetForm() {
	m.focusIndex = 0
	m.formError = ""
	m.keyPickFocus = false
	for i := range m.inputs {
		m.inputs[i].Reset()
		m.inputs[i].Blur()
	}
	m.inputs[0].Focus()
}

func (m *model) populateForm(h Host) {
	m.resetForm()
	m.inputs[0].SetValue(h.Alias)
	m.inputs[0].CursorEnd()
	m.inputs[1].SetValue(h.Hostname)
	m.inputs[1].CursorEnd()
	m.inputs[2].SetValue(h.User)
	m.inputs[2].CursorEnd()
	m.inputs[3].SetValue(h.Port)
	m.inputs[3].CursorEnd()
	m.inputs[4].SetValue(h.IdentityFile)
	m.inputs[4].CursorEnd()
	m.inputs[5].SetValue(h.Password)
	m.inputs[5].CursorEnd()
	groupName := ""
	if h.GroupID != "" {
		if idx := findGroupIndexByID(m.rawGroups, h.GroupID); idx != -1 {
			groupName = m.rawGroups[idx].Name
		}
	}
	m.buildGroupOptions(groupName)
	m.inputs[6].CursorEnd()
}

func (m *model) saveFromForm() error {
	newHost := Host{
		ID:           "",
		Alias:        m.inputs[0].Value(),
		Hostname:     m.inputs[1].Value(),
		User:         m.inputs[2].Value(),
		Port:         m.inputs[3].Value(),
		IdentityFile: m.inputs[4].Value(),
		Password:     m.inputs[5].Value(),
	}
	groupName := strings.TrimSpace(m.inputs[6].Value())
	if !m.groupCustom {
		if len(m.groupOptions) > 0 {
			selected := m.groupOptions[m.groupIndex]
			if selected == "(none)" {
				groupName = ""
			} else if selected == "+ New group..." {
				return fmt.Errorf("new group selected but name not provided")
			} else {
				groupName = selected
			}
		} else {
			groupName = ""
		}
	}

	if groupName == "" {
		newHost.GroupID = ""
	} else {
		groupIdx := findGroupByName(m.rawGroups, groupName)
		if groupIdx == -1 {
			m.rawGroups = append(m.rawGroups, Group{
				ID:       newHostID(),
				Name:     groupName,
				Expanded: true,
			})
			groupIdx = len(m.rawGroups) - 1
		}
		newHost.GroupID = m.rawGroups[groupIdx].ID
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

	m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
	m.save()
	return nil
}

func (m *model) save() {
	saveConfig(m.rawGroups, m.rawHosts)
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

		connectStyle := lipgloss.NewStyle().Foreground(colorSecondary).Bold(true)
		hostStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
		fmt.Printf("\n %s %s\n\n", connectStyle.Render("→ Connecting to"), hostStyle.Render(h.Alias))

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
