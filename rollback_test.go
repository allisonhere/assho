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
