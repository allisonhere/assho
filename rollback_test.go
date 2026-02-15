package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func makeSaveFailingHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	configDirPath := filepath.Join(home, ".config", "assho")
	if err := os.MkdirAll(filepath.Dir(configDirPath), 0o755); err != nil {
		t.Fatalf("failed creating parent config dir: %v", err)
	}
	// Create a file where saveConfig expects a directory, forcing MkdirAll to fail.
	if err := os.WriteFile(configDirPath, []byte("block"), 0o644); err != nil {
		t.Fatalf("failed creating blocking file: %v", err)
	}
	return home
}

func newTestListModel(groups []Group, hosts []Host) list.Model {
	l := list.New(flattenHosts(groups, hosts), hostDelegate{}, 80, 24)
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	return l
}

func newTestHistoryListModel() list.Model {
	l := list.New([]list.Item{}, hostDelegate{}, 80, 24)
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	return l
}

func TestSaveFromFormRollsBackOnSaveError(t *testing.T) {
	makeSaveFailingHome(t)

	originalGroups := []Group{{ID: "g1", Name: "prod", Expanded: true}}
	originalHosts := []Host{{ID: "h1", Alias: "web", Hostname: "10.0.0.1", User: "root", Port: "22", GroupID: "g1"}}

	m := model{
		rawGroups:   originalGroups,
		rawHosts:    originalHosts,
		inputs:      newFormInputs(),
		historyList: newTestHistoryListModel(),
	}
	m.list = newTestListModel(m.rawGroups, m.rawHosts)
	m.buildGroupOptions("")

	m.inputs[0].SetValue("api")
	m.inputs[1].SetValue("10.0.0.2")
	m.inputs[2].SetValue("root")
	m.inputs[3].SetValue("22")
	m.inputs[4].SetValue("")
	m.inputs[5].SetValue("")
	m.inputs[6].SetValue("staging")
	m.groupCustom = true

	if err := m.saveFromForm(); err == nil {
		t.Fatal("expected saveFromForm to fail")
	}

	if len(m.rawGroups) != 1 || m.rawGroups[0].Name != "prod" {
		t.Fatalf("groups should be rolled back, got %+v", m.rawGroups)
	}
	if len(m.rawHosts) != 1 || m.rawHosts[0].Alias != "web" {
		t.Fatalf("hosts should be rolled back, got %+v", m.rawHosts)
	}
}

func TestDeleteGroupByIDRollsBackOnSaveError(t *testing.T) {
	makeSaveFailingHome(t)

	m := model{
		rawGroups:   []Group{{ID: "g1", Name: "prod", Expanded: true}},
		rawHosts:    []Host{{ID: "h1", Alias: "web", GroupID: "g1"}},
		historyList: newTestHistoryListModel(),
	}
	m.list = newTestListModel(m.rawGroups, m.rawHosts)

	if err := m.deleteGroupByID("g1"); err == nil {
		t.Fatal("expected deleteGroupByID to fail")
	}

	if len(m.rawGroups) != 1 || m.rawGroups[0].ID != "g1" {
		t.Fatalf("group deletion should be rolled back, got %+v", m.rawGroups)
	}
	if len(m.rawHosts) != 1 || m.rawHosts[0].GroupID != "g1" {
		t.Fatalf("host group assignment should be rolled back, got %+v", m.rawHosts)
	}
}

func TestMoveItemRollsBackOnSaveError(t *testing.T) {
	makeSaveFailingHome(t)

	hosts := []Host{
		{ID: "h1", Alias: "first", Hostname: "10.0.0.1", User: "root", Port: "22"},
		{ID: "h2", Alias: "second", Hostname: "10.0.0.2", User: "root", Port: "22"},
	}
	m := model{
		rawHosts:    hosts,
		list:        newTestListModel(nil, hosts),
		historyList: newTestHistoryListModel(),
	}
	// Select first item and try to move it down.
	m.list.Select(0)
	msg := m.moveItem(+1)
	if msg == "" {
		t.Fatal("expected moveItem to return error message on save failure")
	}
	if !strings.Contains(msg, "Failed to reorder") {
		t.Fatalf("unexpected error message: %s", msg)
	}
	// Verify rollback: order should be unchanged.
	if m.rawHosts[0].ID != "h1" || m.rawHosts[1].ID != "h2" {
		t.Fatalf("hosts should be rolled back, got %+v", m.rawHosts)
	}
}

func TestMoveItemSwapsUngroupedHosts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	hosts := []Host{
		{ID: "h1", Alias: "first", Hostname: "10.0.0.1", User: "root", Port: "22"},
		{ID: "h2", Alias: "second", Hostname: "10.0.0.2", User: "root", Port: "22"},
		{ID: "h3", Alias: "third", Hostname: "10.0.0.3", User: "root", Port: "22"},
	}
	m := model{
		rawHosts:    hosts,
		list:        newTestListModel(nil, hosts),
		historyList: newTestHistoryListModel(),
	}

	// Select first host and move down.
	m.list.Select(0)
	msg := m.moveItem(+1)
	if msg != "" {
		t.Fatalf("unexpected error: %s", msg)
	}
	if m.rawHosts[0].ID != "h2" || m.rawHosts[1].ID != "h1" || m.rawHosts[2].ID != "h3" {
		t.Fatalf("expected h2,h1,h3 after move down, got %s,%s,%s",
			m.rawHosts[0].ID, m.rawHosts[1].ID, m.rawHosts[2].ID)
	}

	// Move h1 (now at index 1) back up.
	m.list.Select(1) // h1 is now at flat list index 1
	msg = m.moveItem(-1)
	if msg != "" {
		t.Fatalf("unexpected error: %s", msg)
	}
	if m.rawHosts[0].ID != "h1" || m.rawHosts[1].ID != "h2" {
		t.Fatalf("expected h1,h2 after move up, got %s,%s",
			m.rawHosts[0].ID, m.rawHosts[1].ID)
	}
}

func TestMoveItemSwapsGroups(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	groups := []Group{
		{ID: "g1", Name: "alpha", Expanded: true},
		{ID: "g2", Name: "beta", Expanded: true},
	}
	hosts := []Host{
		{ID: "h1", Alias: "a-host", GroupID: "g1"},
		{ID: "h2", Alias: "b-host", GroupID: "g2"},
	}
	m := model{
		rawGroups:   groups,
		rawHosts:    hosts,
		list:        newTestListModel(groups, hosts),
		historyList: newTestHistoryListModel(),
	}

	// Find the second group in the flat list and select it.
	for i, it := range m.list.Items() {
		if g, ok := it.(groupItem); ok && g.ID == "g2" {
			m.list.Select(i)
			break
		}
	}
	msg := m.moveItem(-1)
	if msg != "" {
		t.Fatalf("unexpected error: %s", msg)
	}
	if m.rawGroups[0].ID != "g2" || m.rawGroups[1].ID != "g1" {
		t.Fatalf("expected g2,g1 after move up, got %s,%s",
			m.rawGroups[0].ID, m.rawGroups[1].ID)
	}
}

func TestMoveItemRespectsGroupBoundary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	groups := []Group{{ID: "g1", Name: "prod", Expanded: true}}
	hosts := []Host{
		{ID: "h1", Alias: "ungrouped", Hostname: "10.0.0.1"},
		{ID: "h2", Alias: "grouped", Hostname: "10.0.0.2", GroupID: "g1"},
	}
	m := model{
		rawGroups:   groups,
		rawHosts:    hosts,
		list:        newTestListModel(groups, hosts),
		historyList: newTestHistoryListModel(),
	}

	// Select the ungrouped host (first in flat list) and try to move down.
	// There's no other ungrouped host below, so it should be a no-op.
	m.list.Select(0)
	msg := m.moveItem(+1)
	if msg != "" {
		t.Fatalf("unexpected error: %s", msg)
	}
	// Order should be unchanged.
	if m.rawHosts[0].ID != "h1" || m.rawHosts[1].ID != "h2" {
		t.Fatalf("hosts should not have changed, got %s,%s",
			m.rawHosts[0].ID, m.rawHosts[1].ID)
	}
}

func TestUpdateEnterRollsBackHistoryOnSaveError(t *testing.T) {
	makeSaveFailingHome(t)

	host := Host{ID: "h1", Alias: "web", Hostname: "10.0.0.1", User: "root", Port: "22"}
	m := model{
		state:       stateList,
		rawHosts:    []Host{host},
		list:        newTestListModel(nil, []Host{host}),
		historyList: newTestHistoryListModel(),
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(model)

	if got.sshToRun != nil {
		t.Fatal("ssh launch should not proceed when history save fails")
	}
	if len(got.history) != 0 {
		t.Fatalf("history should be rolled back, got %+v", got.history)
	}
	if !got.statusIsError || !strings.Contains(got.statusMessage, "Failed to save history") {
		t.Fatalf("expected visible history save error, got status=%q", got.statusMessage)
	}
}
