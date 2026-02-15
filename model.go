package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Main Model ---

type state int

const (
	stateList state = iota
	stateForm
	stateFilePicker
	stateGroupPrompt
	stateHistory
)

type model struct {
	list            list.Model
	rawGroups       []Group
	rawHosts        []Host // Source of truth for tree structure
	inputs          []textinput.Model
	groupInput      textinput.Model
	filepicker      filepicker.Model
	spinner         spinner.Model
	focusIndex      int
	state           state
	selectedHost    *Host // For editing
	err             error
	quitting        bool
	sshToRun        *Host  // If set, will exec ssh on quit
	testStatus      string // Status message for connection test
	testResult      bool   // true = success, false = failure
	scanning        bool   // true while Docker scan in progress
	testing         bool   // true while connection test in progress
	width           int    // terminal width
	height          int    // terminal height
	formError       string // inline form validation/action error
	keyPickFocus    bool   // true when [Pick] button on key field is focused
	groupOptions    []string
	groupIndex      int
	groupCustom     bool
	groupAction     string // create|rename
	groupTarget     string // group id for rename
	deleteFocus     bool   // true when Delete Host button is focused in edit form
	deleteArmed     bool   // true when delete confirmation is armed
	listDeleteArmed bool
	listDeleteID    string
	listDeleteType  string // host|group
	listDeleteLabel string
	statusMessage   string
	statusIsError   bool
	history         []HistoryEntry
	historyList     list.Model
	aboutOpen       bool
	aboutFrame      int
	headerFrame     int
}

type modelSnapshot struct {
	rawGroups []Group
	rawHosts  []Host
	history   []HistoryEntry
}

func cloneGroups(groups []Group) []Group {
	if len(groups) == 0 {
		return nil
	}
	cloned := make([]Group, len(groups))
	copy(cloned, groups)
	return cloned
}

func cloneHosts(hosts []Host) []Host {
	if len(hosts) == 0 {
		return nil
	}
	cloned := make([]Host, len(hosts))
	for i := range hosts {
		cloned[i] = hosts[i]
		if len(hosts[i].Containers) > 0 {
			cloned[i].Containers = cloneHosts(hosts[i].Containers)
		}
	}
	return cloned
}

func cloneHistory(history []HistoryEntry) []HistoryEntry {
	if len(history) == 0 {
		return nil
	}
	cloned := make([]HistoryEntry, len(history))
	copy(cloned, history)
	return cloned
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
		t.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorDimText)
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
	groups, hosts, history, loadErr := loadConfig()
	var hostsUpdated bool
	hosts, hostsUpdated = ensureHostIDs(hosts)
	var groupsUpdated bool
	groups, groupsUpdated = ensureGroupIDs(groups)
	if hostsUpdated || groupsUpdated {
		if err := saveConfig(groups, hosts, history); err != nil {
			if loadErr != nil {
				loadErr = errors.Join(loadErr, err)
			} else {
				loadErr = err
			}
		}
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
	groupInput := textinput.New()
	groupInput.Prompt = "  Group Name  "
	groupInput.Placeholder = "e.g. prod"
	groupInput.PromptStyle = lipgloss.NewStyle().Foreground(colorHighlight).Bold(true)
	groupInput.TextStyle = lipgloss.NewStyle().Foreground(colorText)
	groupInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorSubtle)
	groupInput.Cursor.Style = lipgloss.NewStyle().Foreground(colorSecondary)

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

	hl := list.New([]list.Item{}, delegate, 0, 0)
	hl.Title = ""
	hl.SetShowStatusBar(false)
	hl.SetFilteringEnabled(false)
	hl.SetShowTitle(false)
	hl.SetShowHelp(false)

	return model{
		list:        l,
		rawGroups:   groups,
		rawHosts:    hosts,
		inputs:      inputs,
		groupInput:  groupInput,
		filepicker:  fp,
		spinner:     sp,
		state:       stateList,
		err:         loadErr,
		history:     history,
		historyList: hl,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, headerTick())
}

// --- Finder Helpers ---

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

// --- Model Methods ---

func (m *model) focusInputs() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	m.deleteFocus = false
	m.deleteArmed = false
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
	m.deleteFocus = false
	m.deleteArmed = false
	for i := range m.inputs {
		m.inputs[i].Reset()
		m.inputs[i].Blur()
	}
	// New host defaults.
	m.inputs[3].SetValue("22")
	m.inputs[3].CursorEnd()
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
	snapshot := m.snapshot()

	alias := strings.TrimSpace(m.inputs[0].Value())
	if alias == "" {
		return fmt.Errorf("alias is required")
	}
	for i := range m.rawHosts {
		if strings.EqualFold(strings.TrimSpace(m.rawHosts[i].Alias), alias) {
			if m.selectedHost == nil || m.rawHosts[i].ID != m.selectedHost.ID {
				return fmt.Errorf("alias already exists: %s", alias)
			}
		}
	}

	newHost := Host{
		ID:           "",
		Alias:        alias,
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
	if err := m.save(); err != nil {
		m.restoreSnapshot(snapshot)
		return fmt.Errorf("failed to save changes: %w", err)
	}
	return nil
}

func (m *model) save() error {
	return saveConfig(m.rawGroups, m.rawHosts, m.history)
}

func (m *model) rebuildHistoryList() {
	hostByID := make(map[string]*Host, len(m.rawHosts))
	for i := range m.rawHosts {
		hostByID[m.rawHosts[i].ID] = &m.rawHosts[i]
		for j := range m.rawHosts[i].Containers {
			hostByID[m.rawHosts[i].Containers[j].ID] = &m.rawHosts[i].Containers[j]
		}
	}

	var items []list.Item
	seen := map[string]bool{}
	for _, entry := range m.history {
		if seen[entry.HostID] {
			continue
		}
		seen[entry.HostID] = true
		if h, ok := hostByID[entry.HostID]; ok {
			items = append(items, *h)
		} else {
			// Host was deleted — show cached alias as a placeholder.
			items = append(items, Host{
				ID:    entry.HostID,
				Alias: entry.Alias + " (deleted)",
			})
		}
		if len(items) >= 5 {
			break
		}
	}
	m.historyList.SetItems(items)
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

func (m *model) deleteGroupByID(groupID string) error {
	snapshot := m.snapshot()

	for idx := range m.rawGroups {
		if m.rawGroups[idx].ID == groupID {
			m.rawGroups = append(m.rawGroups[:idx], m.rawGroups[idx+1:]...)
			break
		}
	}
	for i := range m.rawHosts {
		if m.rawHosts[i].GroupID == groupID {
			m.rawHosts[i].GroupID = ""
		}
	}
	m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
	if err := m.save(); err != nil {
		m.restoreSnapshot(snapshot)
		return err
	}
	return nil
}

func (m *model) openGroupPrompt(action, targetID, initialName string) {
	m.state = stateGroupPrompt
	m.groupAction = action
	m.groupTarget = targetID
	m.groupInput.Reset()
	m.groupInput.SetValue(initialName)
	m.groupInput.CursorEnd()
	m.groupInput.Focus()
}

// moveItem reorders the selected item in the list by swapping it with its
// neighbor in the given direction (-1 = up, +1 = down). Groups swap with
// adjacent groups; hosts swap with the adjacent host in the same group.
// Returns a non-empty status message on error or no-op.
func (m *model) moveItem(direction int) string {
	sel := m.list.SelectedItem()
	if sel == nil {
		return ""
	}

	switch item := sel.(type) {
	case groupItem:
		idx := findGroupIndexByID(m.rawGroups, item.ID)
		if idx == -1 {
			return ""
		}
		newIdx := idx + direction
		if newIdx < 0 || newIdx >= len(m.rawGroups) {
			return ""
		}
		snapshot := m.snapshot()
		m.rawGroups[idx], m.rawGroups[newIdx] = m.rawGroups[newIdx], m.rawGroups[idx]
		m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
		if err := m.save(); err != nil {
			m.restoreSnapshot(snapshot)
			return fmt.Sprintf("Failed to reorder: %v", err)
		}
		// Reselect the moved item.
		m.reselectItem(item.ID, true)
		return ""

	case Host:
		if item.IsContainer {
			return ""
		}
		idx := findHostIndexByID(m.rawHosts, item.ID)
		if idx == -1 {
			return ""
		}
		groupID := m.rawHosts[idx].GroupID

		// Find the neighbor in the same group.
		neighborIdx := -1
		if direction < 0 {
			for i := idx - 1; i >= 0; i-- {
				if m.rawHosts[i].GroupID == groupID {
					neighborIdx = i
					break
				}
			}
		} else {
			for i := idx + 1; i < len(m.rawHosts); i++ {
				if m.rawHosts[i].GroupID == groupID {
					neighborIdx = i
					break
				}
			}
		}
		if neighborIdx == -1 {
			return ""
		}

		snapshot := m.snapshot()
		m.rawHosts[idx], m.rawHosts[neighborIdx] = m.rawHosts[neighborIdx], m.rawHosts[idx]
		m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
		if err := m.save(); err != nil {
			m.restoreSnapshot(snapshot)
			return fmt.Sprintf("Failed to reorder: %v", err)
		}
		// Reselect the moved item.
		m.reselectItem(item.ID, false)
		return ""
	}
	return ""
}

// reselectItem finds an item by ID in the flat list and selects it.
func (m *model) reselectItem(id string, isGroup bool) {
	for i, it := range m.list.Items() {
		if isGroup {
			if g, ok := it.(groupItem); ok && g.ID == id {
				m.list.Select(i)
				return
			}
		} else {
			if h, ok := it.(Host); ok && h.ID == id {
				m.list.Select(i)
				return
			}
		}
	}
}

func (m *model) clearListDeleteConfirm() {
	m.listDeleteArmed = false
	m.listDeleteID = ""
	m.listDeleteType = ""
	m.listDeleteLabel = ""
}

func (m *model) snapshot() modelSnapshot {
	return modelSnapshot{
		rawGroups: cloneGroups(m.rawGroups),
		rawHosts:  cloneHosts(m.rawHosts),
		history:   cloneHistory(m.history),
	}
}

func (m *model) restoreSnapshot(snapshot modelSnapshot) {
	m.rawGroups = snapshot.rawGroups
	m.rawHosts = snapshot.rawHosts
	m.history = snapshot.history
	m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
	m.rebuildHistoryList()
}
