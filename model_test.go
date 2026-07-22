package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// --- flattenAll ---

func TestFlattenAllIncludesCollapsedGroupsAndContainers(t *testing.T) {
	groups := []Group{
		{ID: "g1", Name: "prod", Expanded: false}, // collapsed group
	}
	hosts := []Host{
		{
			ID:         "h1",
			Alias:      "ungrouped",
			Expanded:   false, // unexpanded — containers hidden in normal view
			Containers: []Host{{ID: "c1", Alias: "ctr"}},
		},
		{
			ID:         "h2",
			Alias:      "grouped",
			GroupID:    "g1",
			Expanded:   false,
			Containers: []Host{{ID: "c2", Alias: "ctr2"}},
		},
	}

	items := flattenAll(groups, hosts)
	// Expected: h1, c1, g1(groupItem), h2, c2
	if len(items) != 5 {
		t.Fatalf("expected 5 items (ungrouped+container, groupItem, grouped+container), got %d", len(items))
	}

	// Verify flattenHosts would return fewer items (respects collapse state).
	visible := flattenHosts(groups, hosts)
	// h1 (not expanded, so c1 hidden) + g1 (collapsed, so h2 hidden) = 2 items
	if len(visible) != 2 {
		t.Fatalf("flattenHosts with collapsed group should return 2 items, got %d", len(visible))
	}
}

func TestFlattenAllNoGroups(t *testing.T) {
	hosts := []Host{
		{ID: "h1", Alias: "a", Expanded: false, Containers: []Host{{ID: "c1", Alias: "ctr"}}},
		{ID: "h2", Alias: "b"},
	}
	items := flattenAll(nil, hosts)
	// h1, c1, h2 = 3
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestFlattenAllPreservesIndent(t *testing.T) {
	groups := []Group{{ID: "g1", Name: "prod", Expanded: false}}
	hosts := []Host{
		{ID: "h1", Alias: "web", GroupID: "g1", Containers: []Host{{ID: "c1", Alias: "ctr"}}},
	}
	items := flattenAll(groups, hosts)
	// groupItem, web (indent=1), ctr (indent=2)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if h, ok := items[1].(Host); !ok || h.ListIndent != 1 {
		t.Fatalf("grouped host should have indent 1, got %+v", items[1])
	}
	if c, ok := items[2].(Host); !ok || c.ListIndent != 2 {
		t.Fatalf("container of grouped host should have indent 2, got %+v", items[2])
	}
}

// --- populateForm ---

func TestPopulateFormAllFields(t *testing.T) {
	m := model{
		rawGroups: []Group{{ID: "g1", Name: "prod"}},
		form:      formState{inputs: newFormInputs()},
	}
	h := Host{
		Alias:        "web",
		Hostname:     "10.0.0.1",
		User:         "alice",
		Port:         "2222",
		ProxyJump:    "bastion.example.com",
		IdentityFile: "~/.ssh/id_rsa",
		Password:     "s3cr3t",
		GroupID:      "g1",
	}
	m.populateForm(h)

	cases := []struct {
		field int
		want  string
		name  string
	}{
		{fieldAlias, "web", "Alias"},
		{fieldHostname, "10.0.0.1", "Hostname"},
		{fieldUser, "alice", "User"},
		{fieldPort, "2222", "Port"},
		{fieldProxyJump, "bastion.example.com", "ProxyJump"},
		{fieldKeyFile, "~/.ssh/id_rsa", "KeyFile"},
		{fieldPassword, "s3cr3t", "Password"},
		{fieldGroup, "prod", "Group"},
	}
	for _, c := range cases {
		if got := m.form.inputs[c.field].Value(); got != c.want {
			t.Errorf("field %s (index %d): got %q, want %q", c.name, c.field, got, c.want)
		}
	}
}

func TestPopulateFormMissingGroup(t *testing.T) {
	m := model{
		rawGroups: []Group{{ID: "g1", Name: "prod"}},
		form:      formState{inputs: newFormInputs()},
	}
	h := Host{
		Alias:    "orphan",
		Hostname: "1.2.3.4",
		GroupID:  "deleted-group-id",
	}
	m.populateForm(h)

	// Deleted group → falls back to (none).
	if got := m.form.inputs[fieldGroup].Value(); got != "(none)" {
		t.Errorf("expected (none) for missing group, got %q", got)
	}
	if m.form.groupCustom {
		t.Error("expected groupCustom=false when group is simply missing")
	}
}

func TestPopulateFormNoGroup(t *testing.T) {
	m := model{
		rawGroups: []Group{{ID: "g1", Name: "prod"}},
		form:      formState{inputs: newFormInputs()},
	}
	h := Host{Alias: "standalone", Hostname: "5.6.7.8"}
	m.populateForm(h)

	if got := m.form.inputs[fieldGroup].Value(); got != "(none)" {
		t.Errorf("expected (none) for ungrouped host, got %q", got)
	}
}

func TestRenderFormViewWideShowsReorganizedSections(t *testing.T) {
	m := model{
		width:  120,
		height: 36,
		form:   newFormState(newFormInputs()),
	}
	out := m.renderFormView()

	for _, want := range []string{"Endpoint", "Authentication", "Routing", "Details", "Current field"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected form view to contain section %q", want)
		}
	}

	if strings.Index(out, "Routing") < strings.Index(out, "Authentication") {
		t.Error("expected Authentication section to appear before Routing")
	}
	if strings.Index(out, "Details") < strings.Index(out, "Routing") {
		t.Error("expected Details section to appear after Routing")
	}
}

func TestRenderFormUsesModalOnlyWhenTerminalIsRoomy(t *testing.T) {
	m := model{width: 120, height: 36, form: newFormState(newFormInputs())}
	modal := m.renderFormView()
	if !strings.Contains(modal, "╭") || !strings.Contains(modal, "╯") {
		t.Fatal("expected a bordered modal on a roomy terminal")
	}
	backdrop := fitViewToBounds(dimBase(m.renderListView()), 120, 36)
	if strings.Split(modal, "\n")[0] != strings.Split(backdrop, "\n")[0] {
		t.Fatal("expected the dimmed dashboard to remain visible around the modal")
	}

	for _, size := range []struct{ width, height int }{{99, 36}, {120, 27}} {
		out := model{width: size.width, height: size.height, form: newFormState(newFormInputs())}.renderFormView()
		if strings.Contains(out, "╭") {
			t.Fatalf("%dx%d should use the full-terminal workspace", size.width, size.height)
		}
	}
}

func TestOverlayCenterPreservesBackdropOnBothSides(t *testing.T) {
	got := overlayCenter("abcdefghij", "XX", 10, 1)
	if got != "abcdXXghij" {
		t.Fatalf("expected modal to replace only its centered cells, got %q", got)
	}
}

func TestRenderFormViewScrollsFocusedNotesIntoView(t *testing.T) {
	m := model{
		width:  90,
		height: 18,
		form:   newFormState(newFormInputs()),
	}
	m.form.inputs[fieldNotes].SetValue("prod DB")
	m.form.focus = controlNotes

	out := m.renderFormView()
	if !strings.Contains(out, "Notes") || !strings.Contains(out, "prod DB") {
		t.Fatal("expected focused Notes control and its value to be visible")
	}
}

func TestRenderFormViewFitsTerminal(t *testing.T) {
	cases := []struct {
		name          string
		width, height int
	}{
		{name: "minimum", width: 36, height: 12},
		{name: "compact", width: 52, height: 16},
		{name: "standard", width: 80, height: 24},
		{name: "wide", width: 120, height: 36},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := model{width: tc.width, height: tc.height, form: newFormState(newFormInputs())}
			m.form.focus = controlNotes
			m.form.inputs[fieldNotes].SetValue("visible note")
			out := m.renderFormView()
			lines := strings.Split(out, "\n")
			if len(lines) > tc.height {
				t.Fatalf("rendered %d lines in a %d-row terminal", len(lines), tc.height)
			}
			for i, line := range lines {
				if got := ansi.StringWidth(line); got > tc.width {
					t.Fatalf("line %d is %d columns in a %d-column terminal", i, got, tc.width)
				}
			}
			if !strings.Contains(out, "visible note") {
				t.Fatal("focused Notes value should remain visible")
			}
		})
	}
}

func TestRenderFormKeepsEveryFocusedControlVisible(t *testing.T) {
	sizes := []struct{ width, height int }{{36, 12}, {52, 16}, {80, 24}, {120, 36}}
	for _, size := range sizes {
		for control := controlAlias; control <= controlDelete; control++ {
			m := model{
				width:  size.width,
				height: size.height,
				form:   newFormState(newFormInputs()),
			}
			host := Host{ID: "h1", Alias: "edge", Hostname: "10.0.0.8"}
			m.form.selectedHost = &host
			m.form.focus = control
			m.buildGroupOptions("")
			out := ansi.Strip(m.renderFormView())
			want := formControlLabel(control)
			if control == controlKeyPicker {
				want = formControlLabel(controlKeyFile)
			}
			if !strings.Contains(out, want) {
				t.Fatalf("%dx%d: focused control %q was not visible", size.width, size.height, want)
			}
		}
	}
}

func TestRenderFormTooSmallNoticeFits(t *testing.T) {
	m := model{width: 30, height: 8, form: newFormState(newFormInputs())}
	out := m.renderFormView()
	if !strings.Contains(out, "Terminal too small") {
		t.Fatal("expected resize notice")
	}
	lines := strings.Split(out, "\n")
	if len(lines) > 8 {
		t.Fatalf("rendered %d lines in an 8-row terminal", len(lines))
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got > 30 {
			t.Fatalf("line %d is %d columns in a 30-column terminal", i, got)
		}
	}
}

func TestFormResizePreservesValuesAndFocus(t *testing.T) {
	m := model{
		state:       stateForm,
		form:        newFormState(newFormInputs()),
		list:        newTestListModel(nil, nil),
		historyList: newTestHistoryListModel(),
	}
	m.form.focus = controlNotes
	m.form.inputs[fieldNotes].SetValue("keep this")

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	got := result.(model)
	result, _ = got.Update(tea.WindowSizeMsg{Width: 36, Height: 12})
	got = result.(model)
	if got.form.focus != controlNotes || got.form.inputs[fieldNotes].Value() != "keep this" {
		t.Fatal("resize should preserve form focus and values")
	}
	if !strings.Contains(got.renderFormView(), "keep this") {
		t.Fatal("focused value should remain visible after resizing")
	}
}

func TestFormCtrlSSavesFromAnyField(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ASSHO_STORE_PASSWORD", "0")
	m := model{
		state:       stateForm,
		form:        newFormState(newFormInputs()),
		list:        newTestListModel(nil, nil),
		historyList: newTestHistoryListModel(),
	}
	m.form.inputs[fieldAlias].SetValue("edge")
	m.form.inputs[fieldHostname].SetValue("10.0.0.8")
	m.form.focus = controlUser
	m.buildGroupOptions("")

	result, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyCtrlS})
	got := result.(model)
	if got.state != stateList {
		t.Fatal("Ctrl+S should save and return to the host list")
	}
	if len(got.rawHosts) != 1 || got.rawHosts[0].Alias != "edge" {
		t.Fatalf("expected saved host, got %+v", got.rawHosts)
	}
}

func TestFormCtrlSFocusesInvalidField(t *testing.T) {
	m := model{state: stateForm, form: newFormState(newFormInputs())}
	m.form.focus = controlNotes
	m.form.inputs[fieldAlias].SetValue("edge")

	result, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyCtrlS})
	got := result.(model)
	if got.state != stateForm {
		t.Fatal("invalid form should remain open")
	}
	if got.form.focus != controlHostname {
		t.Fatalf("expected invalid Hostname to receive focus, got %v", got.form.focus)
	}
	if !strings.Contains(got.form.formError, "hostname is required") {
		t.Fatalf("expected hostname validation error, got %q", got.form.formError)
	}
}

func TestFormEnterOnNotesAdvancesWithoutSaving(t *testing.T) {
	m := model{state: stateForm, form: newFormState(newFormInputs())}
	m.form.focus = controlNotes
	m.form.inputs[fieldAlias].SetValue("edge")
	m.form.inputs[fieldHostname].SetValue("10.0.0.8")

	result, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	got := result.(model)
	if got.state != stateForm || len(got.rawHosts) != 0 {
		t.Fatal("Enter on Notes must not save the form")
	}
	if got.form.focus != controlAlias {
		t.Fatalf("expected focus to wrap to Alias, got %v", got.form.focus)
	}
}

func TestFormAgentForwardingToggle(t *testing.T) {
	m := model{state: stateForm, form: newFormState(newFormInputs())}
	m.form.focus = controlForwardAgent

	result, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	got := result.(model)
	if !forwardAgentEnabled(got.form.inputs[fieldForwardAgent].Value()) {
		t.Fatal("Enter should enable agent forwarding")
	}
	result, _ = got.updateForm(tea.KeyMsg{Type: tea.KeySpace})
	got = result.(model)
	if forwardAgentEnabled(got.form.inputs[fieldForwardAgent].Value()) {
		t.Fatal("Space should disable agent forwarding")
	}
}

func TestFormTextInputsReceiveSpacesAndCursorKeys(t *testing.T) {
	m := model{state: stateForm, form: newFormState(newFormInputs())}
	m.form.focus = controlNotes
	m.form.inputs[fieldNotes].SetValue("prod")
	m.form.inputs[fieldNotes].CursorEnd()
	m.form.inputs[fieldNotes].Focus()

	result, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got := result.(model)
	result, _ = got.updateForm(tea.KeyMsg{Type: tea.KeyLeft})
	got = result.(model)
	result, _ = got.updateForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D', 'B'}})
	got = result.(model)
	if value := got.form.inputs[fieldNotes].Value(); value != "prodDB " {
		t.Fatalf("expected spaces and cursor movement to reach Notes, got %q", value)
	}
}

func TestFormNavigationIncludesKeyPicker(t *testing.T) {
	m := model{state: stateForm, form: newFormState(newFormInputs())}
	m.form.focus = controlKeyFile

	result, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyTab})
	got := result.(model)
	if got.form.focus != controlKeyPicker {
		t.Fatalf("expected key picker after key path, got %v", got.form.focus)
	}
	result, _ = got.updateForm(tea.KeyMsg{Type: tea.KeyTab})
	got = result.(model)
	if got.form.focus != controlPassword {
		t.Fatalf("expected password after key picker, got %v", got.form.focus)
	}
}

func TestFormDeleteStillRequiresTwoConfirmations(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ASSHO_STORE_PASSWORD", "0")
	host := Host{ID: "h1", Alias: "edge", Hostname: "10.0.0.8"}
	m := model{
		state:       stateForm,
		rawHosts:    []Host{host},
		form:        newFormState(newFormInputs()),
		list:        newTestListModel(nil, []Host{host}),
		historyList: newTestHistoryListModel(),
	}
	m.form.selectedHost = &host
	m.form.focus = controlDelete

	result, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	got := result.(model)
	if !got.form.deleteArmed || len(got.rawHosts) != 1 {
		t.Fatal("first Enter should arm deletion without deleting")
	}
	result, _ = got.updateForm(tea.KeyMsg{Type: tea.KeyEnter})
	got = result.(model)
	if got.state != stateList || len(got.rawHosts) != 0 {
		t.Fatal("second Enter should delete the host and return to the list")
	}
}

// --- rebuildHistoryList pruning ---

func TestRebuildHistoryListPrunesDeletedHosts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	existing := Host{ID: "h1", Alias: "web", Hostname: "10.0.0.1", User: "root", Port: "22"}
	m := model{
		rawHosts: []Host{existing},
		history: []HistoryEntry{
			{HostID: "h1", Alias: "web", Timestamp: 3},
			{HostID: "h-gone", Alias: "old", Timestamp: 2},
			{HostID: "h-gone2", Alias: "older", Timestamp: 1},
		},
		historyList: newTestHistoryListModel(),
	}

	m.rebuildHistoryList()

	if len(m.history) != 1 {
		t.Fatalf("expected 1 history entry after pruning deleted hosts, got %d: %v", len(m.history), m.history)
	}
	if m.history[0].HostID != "h1" {
		t.Fatalf("expected surviving entry to be h1, got %q", m.history[0].HostID)
	}

	items := m.historyList.Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 item in history list, got %d", len(items))
	}
}

func TestRebuildHistoryListDeduplicates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	h1 := Host{ID: "h1", Alias: "web", Hostname: "10.0.0.1", User: "root", Port: "22"}
	h2 := Host{ID: "h2", Alias: "db", Hostname: "10.0.0.2", User: "root", Port: "22"}
	m := model{
		rawHosts: []Host{h1, h2},
		history: []HistoryEntry{
			{HostID: "h1", Alias: "web", Timestamp: 4},
			{HostID: "h2", Alias: "db", Timestamp: 3},
			{HostID: "h1", Alias: "web", Timestamp: 2}, // duplicate h1
		},
		historyList: newTestHistoryListModel(),
	}

	m.rebuildHistoryList()

	// All 3 entries kept in history (dedup is only for UI display).
	if len(m.history) != 3 {
		t.Fatalf("expected m.history untouched (no deletions), got %d entries", len(m.history))
	}
	// But UI list shows only 2 unique hosts.
	items := m.historyList.Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 unique hosts in history list, got %d", len(items))
	}
}

func TestRebuildHistoryListNoPruneWhenAllExist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	h1 := Host{ID: "h1", Alias: "web", Hostname: "10.0.0.1", User: "root", Port: "22"}
	original := []HistoryEntry{
		{HostID: "h1", Alias: "web", Timestamp: 1},
	}
	m := model{
		rawHosts:    []Host{h1},
		history:     original,
		historyList: newTestHistoryListModel(),
	}

	m.rebuildHistoryList()

	// No pruning should occur; history length unchanged.
	if len(m.history) != 1 {
		t.Fatalf("expected history unchanged, got %d entries", len(m.history))
	}
}

// Verify that the history list items satisfy list.Item interface.
var _ list.Item = Host{}
var _ list.Item = groupItem{}
