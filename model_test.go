package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
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
		inputs:    newFormInputs(),
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
		if got := m.inputs[c.field].Value(); got != c.want {
			t.Errorf("field %s (index %d): got %q, want %q", c.name, c.field, got, c.want)
		}
	}
}

func TestPopulateFormMissingGroup(t *testing.T) {
	m := model{
		rawGroups: []Group{{ID: "g1", Name: "prod"}},
		inputs:    newFormInputs(),
	}
	h := Host{
		Alias:    "orphan",
		Hostname: "1.2.3.4",
		GroupID:  "deleted-group-id",
	}
	m.populateForm(h)

	// Deleted group → falls back to (none).
	if got := m.inputs[fieldGroup].Value(); got != "(none)" {
		t.Errorf("expected (none) for missing group, got %q", got)
	}
	if m.groupCustom {
		t.Error("expected groupCustom=false when group is simply missing")
	}
}

func TestPopulateFormNoGroup(t *testing.T) {
	m := model{
		rawGroups: []Group{{ID: "g1", Name: "prod"}},
		inputs:    newFormInputs(),
	}
	h := Host{Alias: "standalone", Hostname: "5.6.7.8"}
	m.populateForm(h)

	if got := m.inputs[fieldGroup].Value(); got != "(none)" {
		t.Errorf("expected (none) for ungrouped host, got %q", got)
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
