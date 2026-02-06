package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/filepicker"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
}

// FilterValue implements list.Item
func (h Host) FilterValue() string { return h.Alias + " " + h.Hostname } 
func (h Host) Title() string       { return h.Alias } 
func (h Host) Description() string {
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

type testConnectionMsg struct {
	err error
}

func initialModel() model {
	hosts := loadHosts()
	var items []list.Item
	for _, h := range hosts {
		items = append(items, h)
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Asshi Sessions"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	// Form inputs
	inputs := make([]textinput.Model, 5)
	labels := []string{"Alias", "Hostname", "User", "Port (22)", "Identity File (Optional)"}
	for i := range inputs {
		t := textinput.New()
		t.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
		t.Prompt = fmt.Sprintf("% -25s", labels[i]+":")
		t.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
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
		// Construct SSH command to test connection (batch mode, no pwd)
		args := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=no"}
		if h.Port != "" {
			args = append(args, "-p", h.Port)
		}
		if h.IdentityFile != "" {
			args = append(args, "-i", h.IdentityFile)
		}
		target := h.Hostname
		if h.User != "" {
			target = h.User + "@" + h.Hostname
		}
		args = append(args, target, "exit")

		cmd := exec.Command("ssh", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return testConnectionMsg{err: fmt.Errorf("%v: %s", err, string(output))}
		}
		return testConnectionMsg{err: nil}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case testConnectionMsg:
		if msg.err == nil {
			m.testStatus = "✓ Connection Successful"
			m.testResult = true
		} else {
			m.testStatus = fmt.Sprintf("✗ Connection Failed: %v", msg.err)
			// Truncate if too long
			if len(m.testStatus) > 100 {
				m.testStatus = m.testStatus[:97] + "..."
			}
			m.testResult = false
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
			case "enter":
				// Connect
				if i, ok := m.list.SelectedItem().(Host); ok {
					m.sshToRun = &i
					return m, tea.Quit
				}
			case "e":
				if i, ok := m.list.SelectedItem().(Host); ok {
					m.state = stateForm
					m.selectedHost = &i
					m.populateForm(i)
					return m, nil
				}
			case "d":
				// Delete
				if index := m.list.Index(); index >= 0 && len(m.list.Items()) > 0 {
					m.list.RemoveItem(index)
					m.save()
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
}

func (m *model) saveFromForm() {
	newHost := Host{
		Alias:        m.inputs[0].Value(),
		Hostname:     m.inputs[1].Value(),
		User:         m.inputs[2].Value(),
		Port:         m.inputs[3].Value(),
		IdentityFile: m.inputs[4].Value(),
	}

	if m.selectedHost != nil {
		// Update existing (find by matching fields or just index if we tracked it)
		// For simplicity in this v1, we just replace the list item if we can find it
		// or re-load.
		// A better way for bubbles list: update the item in place.
		// Since we don't have stable IDs, let's just remove the old one (if exact match)
		// actually, easiest: remove old, append new, re-sort/list.
		// Optimization: We can just swap it in the list items.
	}
	
	// Simply reloading from list state + appending/updating
	// For MVP, just append to list and save to disk
	items := m.list.Items()
	
	// If editing, we need to remove the old one first.
	// Since we don't have a unique ID, we will just use the pointer comparison from selectedHost
	if m.selectedHost != nil {
		for i, item := range items {
			if h, ok := item.(Host); ok && h == *m.selectedHost {
				items = append(items[:i], items[i+1:]...)
				break
			}
		}
	}

	items = append(items, newHost)
	m.list.SetItems(items)
	m.save()
}

func (m *model) save() {
	var hosts []Host
	for _, item := range m.list.Items() {
		if h, ok := item.(Host); ok {
			hosts = append(hosts, h)
		}
	}
	saveHosts(hosts)
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
		
		binary, lookErr := exec.LookPath("ssh")
		if lookErr != nil {
			panic(lookErr)
		}

		env := os.Environ()
		execErr := syscall.Exec(binary, append([]string{"ssh"}, args...), env)
		if execErr != nil {
			panic(execErr)
		}
	}
}
