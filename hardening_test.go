package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldPersistPasswordVariants(t *testing.T) {
	t.Setenv("ASSHO_STORE_PASSWORD", "")
	if !shouldPersistPassword() {
		t.Fatal("expected default to persist password")
	}

	t.Setenv("ASSHO_STORE_PASSWORD", "0")
	if shouldPersistPassword() {
		t.Fatal("expected ASSHO_STORE_PASSWORD=0 to disable persistence")
	}

	t.Setenv("ASSHO_STORE_PASSWORD", "false")
	if shouldPersistPassword() {
		t.Fatal("expected ASSHO_STORE_PASSWORD=false to disable persistence")
	}

	t.Setenv("ASSHO_STORE_PASSWORD", "no")
	if shouldPersistPassword() {
		t.Fatal("expected ASSHO_STORE_PASSWORD=no to disable persistence")
	}
}

func TestAllowInsecureTestVariants(t *testing.T) {
	t.Setenv("ASSHO_INSECURE_TEST", "")
	if allowInsecureTest() {
		t.Fatal("expected default secure mode")
	}

	t.Setenv("ASSHO_INSECURE_TEST", "1")
	if !allowInsecureTest() {
		t.Fatal("expected ASSHO_INSECURE_TEST=1 to enable insecure mode")
	}

	t.Setenv("ASSHO_INSECURE_TEST", "yes")
	if !allowInsecureTest() {
		t.Fatal("expected ASSHO_INSECURE_TEST=yes to enable insecure mode")
	}
}

func TestRecordHistoryDedupAndLimit(t *testing.T) {
	history := []HistoryEntry{
		{HostID: "dup", Alias: "old-dup", Timestamp: 1},
	}
	for i := 0; i < maxHistoryEntries+10; i++ {
		history = append(history, HistoryEntry{
			HostID:    fmt.Sprintf("h-%d", i),
			Alias:     "x",
			Timestamp: int64(i + 2),
		})
	}

	got := recordHistory("dup", "new-dup", history)
	if len(got) != maxHistoryEntries {
		t.Fatalf("expected capped history length %d, got %d", maxHistoryEntries, len(got))
	}
	if got[0].HostID != "dup" || got[0].Alias != "new-dup" {
		t.Fatalf("expected newest entry first, got %+v", got[0])
	}
	countDup := 0
	for _, h := range got {
		if h.HostID == "dup" {
			countDup++
		}
	}
	if countDup != 1 {
		t.Fatalf("expected dedup by HostID, got %d duplicate entries", countDup)
	}
}

func TestExpandPathAndCommandExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MY_TEST_SEGMENT", "abc")

	if got := expandPath(""); got != "" {
		t.Fatalf("expected empty path unchanged, got %q", got)
	}

	envPath := expandPath("$MY_TEST_SEGMENT/file")
	if !strings.Contains(envPath, "abc/file") {
		t.Fatalf("expected env expansion, got %q", envPath)
	}

	tildePath := expandPath("~/test-key")
	if !strings.HasPrefix(tildePath, home) {
		t.Fatalf("expected tilde expansion into home, got %q", tildePath)
	}

	if !commandExists("sh") {
		t.Fatal("expected shell binary to exist in PATH")
	}
}

func TestEnsureIDsAndSanitizeHostsForSave(t *testing.T) {
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	hosts := []Host{
		{
			Alias:    "root",
			Hostname: "10.0.0.1",
			Password: "secret",
			Containers: []Host{
				{Alias: "c1", Hostname: "ctr"},
			},
		},
	}
	groups := []Group{{Name: "prod"}}

	hostsWithIDs, hostChanged := ensureHostIDs(hosts)
	if !hostChanged || hostsWithIDs[0].ID == "" || hostsWithIDs[0].Containers[0].ID == "" {
		t.Fatalf("expected IDs to be assigned recursively, got %+v", hostsWithIDs)
	}

	groupsWithIDs, groupChanged := ensureGroupIDs(groups)
	if !groupChanged || groupsWithIDs[0].ID == "" {
		t.Fatalf("expected group IDs to be assigned, got %+v", groupsWithIDs)
	}

	sanitized := sanitizeHostsForSave(hostsWithIDs)
	if sanitized[0].Password != "" || sanitized[0].PasswordRef != "" {
		t.Fatalf("expected password data scrubbed when persistence disabled, got %+v", sanitized[0])
	}
}

func TestBuildSSHHelpersAndFormatStatus(t *testing.T) {
	h := Host{
		Hostname:     "example.com",
		User:         "alice",
		Port:         "2222",
		IdentityFile: "~/id_test",
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	args := buildSSHArgs(h, true, "echo ok")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-t") ||
		!strings.Contains(joined, "-l alice") ||
		!strings.Contains(joined, "-p 2222") ||
		!strings.Contains(joined, "example.com") ||
		!strings.Contains(joined, "echo ok") {
		t.Fatalf("unexpected ssh args: %v", args)
	}
	if !strings.Contains(joined, filepath.Join(home, "id_test")) {
		t.Fatalf("expected expanded identity file path in args: %v", args)
	}

	binary, outArgs, extraEnv, ok := buildSSHCommand("", args)
	if !ok || binary != "ssh" {
		t.Fatalf("expected plain ssh command for empty password, got binary=%q ok=%v", binary, ok)
	}
	if len(outArgs) != len(args) {
		t.Fatalf("expected args passthrough, got %v", outArgs)
	}
	if len(extraEnv) != 0 {
		t.Fatalf("expected no extra env for empty password, got %v", extraEnv)
	}

	msg, success := formatTestStatus(nil)
	if !success || msg != "Connection successful" {
		t.Fatalf("unexpected success status: %q, %v", msg, success)
	}

	errMsg, errSuccess := formatTestStatus(errors.New("boom"))
	if errSuccess || errMsg != "boom" {
		t.Fatalf("unexpected error status: %q, %v", errMsg, errSuccess)
	}
}

func TestLoadConfigReturnsDefaultWhenMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	groups, hosts, history, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 0 || len(history) != 0 {
		t.Fatalf("expected empty groups/history on first run, got groups=%d history=%d", len(groups), len(history))
	}
	if len(hosts) != 1 || hosts[0].Alias != "Localhost" {
		t.Fatalf("expected default localhost seed host, got %+v", hosts)
	}
}

func TestModelFindersAndContainerCount(t *testing.T) {
	hosts := []Host{
		{ID: "h1", Alias: "web", Containers: []Host{{ID: "c1"}, {ID: "c2"}}},
		{ID: "h2", Alias: "db"},
	}
	groups := []Group{
		{ID: "g1", Name: "prod"},
		{ID: "g2", Name: "staging"},
	}

	if idx := findHostIndexByID(hosts, "h2"); idx != 1 {
		t.Fatalf("expected host index 1, got %d", idx)
	}
	if idx := findHostIndexByID(hosts, "missing"); idx != -1 {
		t.Fatalf("expected -1 for missing host, got %d", idx)
	}
	if idx := findGroupIndexByID(groups, "g1"); idx != 0 {
		t.Fatalf("expected group index 0, got %d", idx)
	}
	if idx := findGroupByName(groups, "  STAGING "); idx != 1 {
		t.Fatalf("expected case-insensitive match index 1, got %d", idx)
	}
	if idx := findGroupByName(groups, ""); idx != -1 {
		t.Fatalf("expected -1 for empty group name, got %d", idx)
	}
	if c := countContainers(hosts); c != 2 {
		t.Fatalf("expected 2 containers, got %d", c)
	}
}

func TestGroupSelectionHelpers(t *testing.T) {
	m := model{
		rawGroups: []Group{
			{ID: "g1", Name: "prod"},
			{ID: "g2", Name: "staging"},
		},
		inputs: newFormInputs(),
	}

	m.buildGroupOptions("staging")
	if m.groupCustom {
		t.Fatal("expected known group to be non-custom mode")
	}
	if m.inputs[fieldGroup].Value() != "staging" {
		t.Fatalf("expected selected group value 'staging', got %q", m.inputs[fieldGroup].Value())
	}

	m.groupIndex = -1
	m.applyGroupSelectionToInput()
	if m.inputs[fieldGroup].Value() != "(none)" {
		t.Fatalf("expected clamped group value '(none)', got %q", m.inputs[fieldGroup].Value())
	}

	m.groupCustom = true
	m.inputs[fieldGroup].SetValue("custom")
	m.groupIndex = 2
	m.applyGroupSelectionToInput()
	if m.inputs[fieldGroup].Value() != "custom" {
		t.Fatalf("expected custom value to remain unchanged, got %q", m.inputs[fieldGroup].Value())
	}
}

func TestSnapshotRestoreRoundTrip(t *testing.T) {
	m := model{
		rawGroups: []Group{{ID: "g1", Name: "prod"}},
		rawHosts: []Host{
			{ID: "h1", Alias: "web", Containers: []Host{{ID: "c1", Alias: "ctr"}}},
		},
		history:     []HistoryEntry{{HostID: "h1", Alias: "web", Timestamp: 1}},
		list:        newTestListModel([]Group{{ID: "g1", Name: "prod"}}, []Host{{ID: "h1", Alias: "web"}}),
		historyList: newTestHistoryListModel(),
	}

	s := m.snapshot()

	m.rawGroups[0].Name = "mutated"
	m.rawHosts[0].Alias = "mutated"
	m.rawHosts[0].Containers[0].Alias = "mutated"
	m.history[0].Alias = "mutated"

	m.restoreSnapshot(s)
	if m.rawGroups[0].Name != "prod" || m.rawHosts[0].Alias != "web" {
		t.Fatalf("expected snapshot restore to reset group/host, got groups=%+v hosts=%+v", m.rawGroups, m.rawHosts)
	}
	if m.rawHosts[0].Containers[0].Alias != "ctr" {
		t.Fatalf("expected container alias restore, got %+v", m.rawHosts[0].Containers[0])
	}
	if m.history[0].Alias != "web" {
		t.Fatalf("expected history restore, got %+v", m.history[0])
	}
}

func TestSaveFromFormRequiresHostname(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	m := model{inputs: newFormInputs(), historyList: newTestHistoryListModel()}
	m.list = newTestListModel(nil, nil)
	m.inputs[fieldAlias].SetValue("web")
	// hostname left empty
	m.buildGroupOptions("")
	if err := m.saveFromForm(); err == nil {
		t.Fatal("expected error for empty hostname, got nil")
	}
}

func TestForwardAgentParsedFromForm(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	cases := []struct {
		input string
		want  bool
	}{
		{"yes", true},
		{"YES", true},
		{"1", true},
		{"true", true},
		{"", false},
		{"no", false},
	}
	for _, tc := range cases {
		m := model{inputs: newFormInputs(), historyList: newTestHistoryListModel()}
		m.list = newTestListModel(nil, nil)
		m.inputs[fieldAlias].SetValue("srv")
		m.inputs[fieldHostname].SetValue("10.0.0.1")
		m.inputs[fieldForwardAgent].SetValue(tc.input)
		m.buildGroupOptions("")
		if err := m.saveFromForm(); err != nil {
			t.Fatalf("unexpected error for input %q: %v", tc.input, err)
		}
		if m.rawHosts[len(m.rawHosts)-1].ForwardAgent != tc.want {
			t.Errorf("input %q: got ForwardAgent=%v, want %v", tc.input, !tc.want, tc.want)
		}
		// reset hosts
		m.rawHosts = nil
	}
}

func TestBuildSSHArgsForwardAgent(t *testing.T) {
	h := Host{Hostname: "example.com", ForwardAgent: true}
	args := buildSSHArgs(h, false, "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-A") {
		t.Fatalf("expected -A in ssh args for ForwardAgent=true, got: %v", args)
	}

	h2 := Host{Hostname: "example.com", ForwardAgent: false}
	args2 := buildSSHArgs(h2, false, "")
	joined2 := strings.Join(args2, " ")
	if strings.Contains(joined2, "-A") {
		t.Fatalf("expected no -A in ssh args for ForwardAgent=false, got: %v", args2)
	}
}

func TestBuildSSHCommandUsesEnvVar(t *testing.T) {
	binary, args, extraEnv, ok := buildSSHCommand("s3cr3t", []string{"example.com"})
	if !commandExists("sshpass") {
		t.Skip("sshpass not installed, skipping env var test")
	}
	if !ok {
		t.Fatal("expected ok=true with sshpass installed")
	}
	_ = binary
	if len(args) == 0 || args[0] != "-e" {
		t.Fatalf("expected first sshpass arg to be -e, got: %v", args)
	}
	found := false
	for _, e := range extraEnv {
		if strings.HasPrefix(e, "SSHPASS=") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected SSHPASS= in extra env, got: %v", extraEnv)
	}
}

func TestSaveFromFormPortValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	newModel := func() model {
		m := model{inputs: newFormInputs(), historyList: newTestHistoryListModel()}
		m.list = newTestListModel(nil, nil)
		m.inputs[fieldAlias].SetValue("web")
		m.inputs[fieldHostname].SetValue("10.0.0.1")
		return m
	}

	invalidPorts := []string{"abc", "0", "65536", "-1", "99999"}
	for _, p := range invalidPorts {
		m := newModel()
		m.inputs[fieldPort].SetValue(p)
		if err := m.saveFromForm(); err == nil {
			t.Errorf("expected error for invalid port %q, got nil", p)
		}
	}

	validPorts := []string{"", "22", "1", "65535", "2222"}
	for _, p := range validPorts {
		m := newModel()
		m.inputs[fieldPort].SetValue(p)
		if err := m.saveFromForm(); err != nil {
			t.Errorf("expected no error for valid port %q, got: %v", p, err)
		}
	}
}

func TestNotesAndLocalForwardSavedFromForm(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	m := model{inputs: newFormInputs(), historyList: newTestHistoryListModel()}
	m.list = newTestListModel(nil, nil)
	m.inputs[fieldAlias].SetValue("srv")
	m.inputs[fieldHostname].SetValue("10.0.0.1")
	m.inputs[fieldNotes].SetValue("prod DB, ask Sam")
	m.inputs[fieldLocalForward].SetValue("5432:localhost:5432")
	m.buildGroupOptions("")

	if err := m.saveFromForm(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	saved := m.rawHosts[len(m.rawHosts)-1]
	if saved.Notes != "prod DB, ask Sam" {
		t.Errorf("expected Notes to be saved, got %q", saved.Notes)
	}
	if saved.LocalForward != "5432:localhost:5432" {
		t.Errorf("expected LocalForward to be saved, got %q", saved.LocalForward)
	}
}

func TestBuildSSHArgsLocalForward(t *testing.T) {
	h := Host{
		Hostname:     "example.com",
		LocalForward: "5432:localhost:5432",
	}
	args := buildSSHArgs(h, false, "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-L 5432:localhost:5432") {
		t.Fatalf("expected -L in ssh args for LocalForward, got: %v", args)
	}

	h2 := Host{Hostname: "example.com"}
	args2 := buildSSHArgs(h2, false, "")
	joined2 := strings.Join(args2, " ")
	if strings.Contains(joined2, "-L") {
		t.Fatalf("expected no -L when LocalForward is empty, got: %v", args2)
	}
}

func TestPinnedHostSavedFromForm(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	existing := Host{ID: "h1", Alias: "srv", Hostname: "10.0.0.1", User: "root", Port: "22", Pinned: true}
	m := model{
		rawHosts:    []Host{existing},
		inputs:      newFormInputs(),
		historyList: newTestHistoryListModel(),
	}
	m.list = newTestListModel(nil, m.rawHosts)
	m.selectedHost = &existing
	m.populateForm(existing)
	m.buildGroupOptions("")

	if err := m.saveFromForm(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m.rawHosts[0].Pinned {
		t.Fatal("expected Pinned to be preserved after edit")
	}
}

func TestFlattenHostsPinnedSection(t *testing.T) {
	hosts := []Host{
		{ID: "h1", Alias: "normal", Hostname: "a"},
		{ID: "h2", Alias: "fav", Hostname: "b", Pinned: true},
	}
	items := flattenHosts(nil, hosts)
	// Pinned hosts appear in the pinned section (top) AND in their normal position.
	// Expected: pinned groupItem, h2 (indent=1), h1 (indent=0), h2 (indent=0)
	if len(items) != 4 {
		t.Fatalf("expected 4 items (pinned header + pinned host + 2 ungrouped hosts), got %d", len(items))
	}
	if g, ok := items[0].(groupItem); !ok || g.ID != "__pinned__" {
		t.Fatalf("expected pinned groupItem at index 0, got %T %+v", items[0], items[0])
	}
	if h, ok := items[1].(Host); !ok || h.ID != "h2" || h.ListIndent != 1 {
		t.Fatalf("expected pinned host at index 1 with indent 1, got %+v", items[1])
	}
	// h1 and h2 both appear in the normal ungrouped section.
	if h, ok := items[2].(Host); !ok || h.ID != "h1" || h.ListIndent != 0 {
		t.Fatalf("expected h1 at index 2 with indent 0, got %+v", items[2])
	}
	if h, ok := items[3].(Host); !ok || h.ID != "h2" || h.ListIndent != 0 {
		t.Fatalf("expected h2 at index 3 with indent 0, got %+v", items[3])
	}
}

func TestBuildLastConnected(t *testing.T) {
	history := []HistoryEntry{
		{HostID: "h1", Timestamp: 100},
		{HostID: "h2", Timestamp: 200},
		{HostID: "h1", Timestamp: 50}, // older duplicate — should be ignored
	}
	lc := buildLastConnected(history)
	if lc["h1"] != 100 {
		t.Errorf("expected h1 last connected = 100 (newest first), got %d", lc["h1"])
	}
	if lc["h2"] != 200 {
		t.Errorf("expected h2 last connected = 200, got %d", lc["h2"])
	}
}

func TestFindHostByAlias(t *testing.T) {
	hosts := []Host{
		{ID: "h1", Alias: "web", Hostname: "10.0.0.1"},
		{ID: "h2", Alias: "db", Hostname: "10.0.0.2",
			Containers: []Host{{ID: "c1", Alias: "pg", IsContainer: true}}},
	}

	if h := findHostByAlias(hosts, "web"); h == nil || h.ID != "h1" {
		t.Fatal("expected to find web host")
	}
	if h := findHostByAlias(hosts, "WEB"); h == nil || h.ID != "h1" {
		t.Fatal("expected case-insensitive match for WEB")
	}
	if h := findHostByAlias(hosts, "pg"); h == nil || h.ID != "c1" {
		t.Fatal("expected to find container pg")
	}
	if h := findHostByAlias(hosts, "missing"); h != nil {
		t.Fatal("expected nil for missing alias")
	}
}
