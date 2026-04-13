package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type statusClearMsg struct{ version int }

func statusClearCmd(version int) tea.Cmd {
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg {
		return statusClearMsg{version: version}
	})
}

// --- Main Model ---

type state int

const (
	stateList state = iota
	stateForm
	stateFilePicker
	stateGroupPrompt
	stateHistory
)

// Form field indices (must match newFormInputs order).
const (
	fieldAlias        = 0
	fieldHostname     = 1
	fieldUser         = 2
	fieldPort         = 3
	fieldProxyJump    = 4
	fieldLocalForward = 5
	fieldKeyFile      = 6
	fieldNotes        = 7
	fieldPassword     = 8
	fieldForwardAgent = 9
	fieldGroup        = 10
	fieldCount        = 11
)

type model struct {
	list        list.Model
	rawGroups   []Group
	rawHosts    []Host // Source of truth for tree structure
	form        formState
	groupPrompt groupPromptState
	filepicker  filepicker.Model
	spinner     spinner.Model
	state       state
	err         error
	quitting    bool
	sshToRun    *Host // If set, will exec ssh on quit
	scanning    bool  // true while Docker scan in progress
	width       int   // terminal width
	height      int   // terminal height
	listDelete  listDeleteState
	status      statusState
	history     []HistoryEntry
	historyList list.Model
	about       aboutState
	headerFrame int
}

type formState struct {
	inputs       []textinput.Model
	focusIndex   int
	selectedHost *Host  // For editing
	formError    string // inline form validation/action error
	keyPickFocus bool   // true when [Pick] button on key field is focused
	deleteFocus  bool   // true when Delete Host button is focused
	deleteArmed  bool   // true when delete confirmation is armed
	testStatus   string // Status message for connection test
	testResult   bool   // true = success, false = failure
	testing      bool   // true while connection test in progress
	groupOptions []string
	groupIndex   int
	groupCustom  bool
}

type groupPromptState struct {
	input  textinput.Model
	action string // create|rename
	target string // group id for rename
}

type aboutState struct {
	open  bool
	frame int
}

type statusState struct {
	message string
	isError bool
	version int
}

type listDeleteState struct {
	armed bool
	id    string
	kind  string // host|group
	label string
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

	// Pinned hosts first under a synthetic group header.
	var pinnedIdx []int
	for i := range hosts {
		if hosts[i].Pinned && !hosts[i].IsContainer {
			pinnedIdx = append(pinnedIdx, i)
		}
	}
	if len(pinnedIdx) > 0 {
		items = append(items, groupItem{
			Group:     Group{ID: "__pinned__", Name: "★ Pinned", Expanded: true},
			HostCount: len(pinnedIdx),
		})
		for _, i := range pinnedIdx {
			h := hosts[i]
			h.ListIndent = 1
			items = append(items, h)
			if h.Expanded {
				for j := range h.Containers {
					c := h.Containers[j]
					c.ParentID = h.ID
					c.ListIndent = 2
					items = append(items, c)
				}
			}
		}
	}

	// Ungrouped hosts.
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
		hostCount := 0
		for j := range hosts {
			if hosts[j].GroupID == g.ID {
				hostCount++
			}
		}
		items = append(items, groupItem{Group: g, HostCount: hostCount})
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

// flattenAll is like flattenHosts but includes every host and container
// regardless of group/host expansion state. Used to populate the list before
// filter mode so that hosts inside collapsed groups are searchable.
func flattenAll(groups []Group, hosts []Host) []list.Item {
	var items []list.Item

	// Pinned section first.
	var pinnedIdx []int
	for i := range hosts {
		if hosts[i].Pinned && !hosts[i].IsContainer {
			pinnedIdx = append(pinnedIdx, i)
		}
	}
	if len(pinnedIdx) > 0 {
		items = append(items, groupItem{
			Group:     Group{ID: "__pinned__", Name: "★ Pinned", Expanded: true},
			HostCount: len(pinnedIdx),
		})
		for _, i := range pinnedIdx {
			h := hosts[i]
			h.ListIndent = 1
			items = append(items, h)
			for j := range h.Containers {
				c := h.Containers[j]
				c.ParentID = h.ID
				c.ListIndent = 2
				items = append(items, c)
			}
		}
	}

	for i := range hosts {
		if hosts[i].GroupID != "" {
			continue
		}
		h := hosts[i]
		h.ListIndent = 0
		items = append(items, h)
		for j := range h.Containers {
			c := h.Containers[j]
			c.ParentID = h.ID
			c.ListIndent = 1
			items = append(items, c)
		}
	}
	for i := range groups {
		g := groups[i]
		hostCount := 0
		for j := range hosts {
			if hosts[j].GroupID == g.ID {
				hostCount++
			}
		}
		items = append(items, groupItem{Group: g, HostCount: hostCount})
		for j := range hosts {
			if hosts[j].GroupID != g.ID {
				continue
			}
			h := hosts[j]
			h.ListIndent = 1
			items = append(items, h)
			for k := range h.Containers {
				c := h.Containers[k]
				c.ParentID = h.ID
				c.ListIndent = 2
				items = append(items, c)
			}
		}
	}
	return items
}

// buildLastConnected returns a map of hostID → most-recent connection timestamp
// built from history (which is ordered newest-first).
func buildLastConnected(history []HistoryEntry) map[string]int64 {
	m := make(map[string]int64, len(history))
	for _, e := range history {
		if _, exists := m[e.HostID]; !exists {
			m[e.HostID] = e.Timestamp
		}
	}
	return m
}

func countContainers(hosts []Host) int {
	count := 0
	for _, h := range hosts {
		count += len(h.Containers)
	}
	return count
}

func newFormInputs() []textinput.Model {
	inputs := make([]textinput.Model, fieldCount)
	labels := []string{"Alias", "Hostname", "User", "Port", "ProxyJump", "LocalFwd", "Key File", "Notes", "Password", "Fwd. Agent", "Group"}
	placeholders := []string{"my-server", "192.168.1.100", "root", "22", "user@bastion:port", "5432:localhost:5432", "optional key path", "optional note", "", "yes to enable (-A)", "optional group name"}
	for i := range inputs {
		t := textinput.New()
		t.Cursor.Style = lipgloss.NewStyle().Foreground(colorSecondary)
		t.Prompt = fmt.Sprintf("  %-12s", labels[i])
		t.PromptStyle = lipgloss.NewStyle().Foreground(colorHighlight).Bold(true)
		t.TextStyle = lipgloss.NewStyle().Foreground(colorText)
		t.Placeholder = placeholders[i]
		t.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorDimText)
		if i == fieldPassword {
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

	// Separate keychain lookup warnings (non-fatal, shown as a timed status
	// message) from real config errors (shown as a permanent banner).
	var keychainWarning string
	if loadErr != nil && strings.HasPrefix(loadErr.Error(), "keychain lookup failed:") {
		keychainWarning = loadErr.Error()
		loadErr = nil
	}
	items := flattenHosts(groups, hosts)

	delegate := hostDelegate{lastConnected: buildLastConnected(history)}
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

	m := model{
		list:        l,
		rawGroups:   groups,
		rawHosts:    hosts,
		form:        formState{inputs: inputs},
		groupPrompt: groupPromptState{input: groupInput},
		filepicker:  fp,
		spinner:     sp,
		state:       stateList,
		err:         loadErr,
		history:     history,
		historyList: hl,
	}
	if keychainWarning != "" {
		m.status.message = keychainWarning
		m.status.isError = true
		m.status.version++
	}
	return m
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick, headerTick(), dockerRefreshTick()}
	if m.status.message != "" {
		cmds = append(cmds, statusClearCmd(m.status.version))
	}
	return tea.Batch(cmds...)
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
	cmds := make([]tea.Cmd, len(m.form.inputs))
	m.form.deleteFocus = false
	m.form.deleteArmed = false
	for i := 0; i < len(m.form.inputs); i++ {
		if i == m.form.focusIndex {
			m.form.keyPickFocus = false
			cmds[i] = m.form.inputs[i].Focus()
			// Put cursor at end so editing existing values behaves naturally.
			m.form.inputs[i].CursorEnd()
			m.form.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
			m.form.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(colorText)
		} else {
			m.form.inputs[i].Blur()
			m.form.inputs[i].PromptStyle = lipgloss.NewStyle().Foreground(colorMuted)
			m.form.inputs[i].TextStyle = lipgloss.NewStyle().Foreground(colorText)
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) resetForm() {
	m.form.focusIndex = 0
	m.form.formError = ""
	m.form.keyPickFocus = false
	m.form.deleteFocus = false
	m.form.deleteArmed = false
	for i := range m.form.inputs {
		m.form.inputs[i].Reset()
		m.form.inputs[i].Blur()
	}
	// New host defaults.
	m.form.inputs[fieldPort].SetValue("22")
	m.form.inputs[fieldPort].CursorEnd()
	m.form.inputs[fieldAlias].Focus()
}

func (m *model) populateForm(h Host) {
	m.resetForm()
	m.form.inputs[fieldAlias].SetValue(h.Alias)
	m.form.inputs[fieldAlias].CursorEnd()
	m.form.inputs[fieldHostname].SetValue(h.Hostname)
	m.form.inputs[fieldHostname].CursorEnd()
	m.form.inputs[fieldUser].SetValue(h.User)
	m.form.inputs[fieldUser].CursorEnd()
	m.form.inputs[fieldPort].SetValue(h.Port)
	m.form.inputs[fieldPort].CursorEnd()
	m.form.inputs[fieldProxyJump].SetValue(h.ProxyJump)
	m.form.inputs[fieldProxyJump].CursorEnd()
	m.form.inputs[fieldLocalForward].SetValue(h.LocalForward)
	m.form.inputs[fieldLocalForward].CursorEnd()
	m.form.inputs[fieldKeyFile].SetValue(h.IdentityFile)
	m.form.inputs[fieldKeyFile].CursorEnd()
	m.form.inputs[fieldNotes].SetValue(h.Notes)
	m.form.inputs[fieldNotes].CursorEnd()
	m.form.inputs[fieldPassword].SetValue(h.Password)
	m.form.inputs[fieldPassword].CursorEnd()
	if h.ForwardAgent {
		m.form.inputs[fieldForwardAgent].SetValue("yes")
	} else {
		m.form.inputs[fieldForwardAgent].SetValue("")
	}
	m.form.inputs[fieldForwardAgent].CursorEnd()
	groupName := ""
	if h.GroupID != "" {
		if idx := findGroupIndexByID(m.rawGroups, h.GroupID); idx != -1 {
			groupName = m.rawGroups[idx].Name
		}
	}
	m.buildGroupOptions(groupName)
	m.form.inputs[fieldGroup].CursorEnd()
}

func (m *model) saveFromForm() error {
	snapshot := m.snapshot()

	alias := strings.TrimSpace(m.form.inputs[fieldAlias].Value())
	if alias == "" {
		return fmt.Errorf("alias is required")
	}
	hostname := strings.TrimSpace(m.form.inputs[fieldHostname].Value())
	if hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if portStr := strings.TrimSpace(m.form.inputs[fieldPort].Value()); portStr != "" {
		n, err := strconv.Atoi(portStr)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("port must be a number between 1 and 65535")
		}
	}
	for i := range m.rawHosts {
		if strings.EqualFold(strings.TrimSpace(m.rawHosts[i].Alias), alias) {
			if m.form.selectedHost == nil || m.rawHosts[i].ID != m.form.selectedHost.ID {
				return fmt.Errorf("alias already exists: %s", alias)
			}
		}
	}

	fwdAgent := strings.ToLower(strings.TrimSpace(m.form.inputs[fieldForwardAgent].Value()))
	newHost := Host{
		ID:           "",
		Alias:        alias,
		Hostname:     hostname,
		User:         m.form.inputs[fieldUser].Value(),
		Port:         m.form.inputs[fieldPort].Value(),
		ProxyJump:    m.form.inputs[fieldProxyJump].Value(),
		LocalForward: m.form.inputs[fieldLocalForward].Value(),
		IdentityFile: m.form.inputs[fieldKeyFile].Value(),
		Notes:        m.form.inputs[fieldNotes].Value(),
		Password:     m.form.inputs[fieldPassword].Value(),
		ForwardAgent: fwdAgent == "yes" || fwdAgent == "1" || fwdAgent == "true",
	}
	groupName := strings.TrimSpace(m.form.inputs[fieldGroup].Value())
	if !m.form.groupCustom {
		if len(m.form.groupOptions) > 0 {
			selected := m.form.groupOptions[m.form.groupIndex]
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
				ID:       newGroupID(),
				Name:     groupName,
				Expanded: true,
			})
			groupIdx = len(m.rawGroups) - 1
		}
		newHost.GroupID = m.rawGroups[groupIdx].ID
	}

	if m.form.selectedHost != nil {
		// Update existing
		for i, h := range m.rawHosts {
			if h.ID == m.form.selectedHost.ID {
				// Preserve containers/expanded/pinned state
				newHost.ID = h.ID
				newHost.Containers = h.Containers
				newHost.Expanded = h.Expanded
				newHost.Pinned = h.Pinned
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

func (m *model) refreshDelegate() {
	m.list.SetDelegate(hostDelegate{lastConnected: buildLastConnected(m.history)})
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
	var pruned bool
	var kept []HistoryEntry
	for _, entry := range m.history {
		h, exists := hostByID[entry.HostID]
		if !exists {
			// Host was deleted — drop it from stored history.
			pruned = true
			continue
		}
		kept = append(kept, entry)
		if seen[entry.HostID] {
			continue
		}
		seen[entry.HostID] = true
		items = append(items, *h)
	}
	if pruned {
		m.history = kept
		_ = m.save()
	}
	m.historyList.SetItems(items)
	m.refreshDelegate()
}

func (m *model) buildGroupOptions(selectedName string) {
	m.form.groupOptions = []string{"(none)"}
	for i := range m.rawGroups {
		m.form.groupOptions = append(m.form.groupOptions, m.rawGroups[i].Name)
	}
	m.form.groupOptions = append(m.form.groupOptions, "+ New group...")
	m.form.groupIndex = 0
	m.form.groupCustom = false

	target := strings.TrimSpace(selectedName)
	if target == "" {
		m.form.inputs[fieldGroup].SetValue("(none)")
		return
	}
	for i, opt := range m.form.groupOptions {
		if strings.EqualFold(opt, target) {
			m.form.groupIndex = i
			m.form.inputs[fieldGroup].SetValue(opt)
			return
		}
	}
	// Unknown group name: switch to custom mode with the provided name.
	m.form.groupCustom = true
	m.form.groupIndex = len(m.form.groupOptions) - 1
	m.form.inputs[fieldGroup].SetValue(target)
}

func (m *model) applyGroupSelectionToInput() {
	if m.form.groupCustom {
		return
	}
	if len(m.form.groupOptions) == 0 {
		m.form.inputs[fieldGroup].SetValue("(none)")
		return
	}
	if m.form.groupIndex < 0 {
		m.form.groupIndex = 0
	}
	if m.form.groupIndex >= len(m.form.groupOptions) {
		m.form.groupIndex = len(m.form.groupOptions) - 1
	}
	m.form.inputs[fieldGroup].SetValue(m.form.groupOptions[m.form.groupIndex])
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
	m.groupPrompt.action = action
	m.groupPrompt.target = targetID
	m.form.formError = ""
	m.groupPrompt.input.Reset()
	m.groupPrompt.input.SetValue(initialName)
	m.groupPrompt.input.CursorEnd()
	m.groupPrompt.input.Focus()
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
	m.listDelete = listDeleteState{}
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
