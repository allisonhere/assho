package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)


func TestSaveConfigWritesVersion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")

	groups := []Group{{ID: "g1", Name: "prod", Expanded: true}}
	hosts := []Host{{ID: "h1", Alias: "srv", Hostname: "srv", User: "root", Port: "22", GroupID: "g1", Password: "secret"}}
	if err := saveConfig(groups, hosts, nil); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	cfgPath := filepath.Join(tmp, ".config", "assho", "hosts.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Version != configVersion {
		t.Fatalf("expected version %d, got %d", configVersion, cfg.Version)
	}
	if len(cfg.Hosts) != 1 || cfg.Hosts[0].Password != "" {
		t.Fatalf("expected persisted hosts with scrubbed password, got %+v", cfg.Hosts)
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected permissions 0600, got %04o", info.Mode().Perm())
	}
}


func TestFlattenHostsIndentation(t *testing.T) {
	groups := []Group{{ID: "g1", Name: "prod", Expanded: true}}
	hosts := []Host{
		{ID: "h0", Alias: "ungrouped", Hostname: "u", User: "root", Port: "22"},
		{ID: "h1", Alias: "grouped", Hostname: "g", User: "root", Port: "22", GroupID: "g1", Expanded: true, Containers: []Host{{ID: "c1", Alias: "ctr", IsContainer: true}}},
	}
	items := flattenHosts(groups, hosts)
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	ungrouped, ok := items[0].(Host)
	if !ok || ungrouped.ListIndent != 0 {
		t.Fatalf("expected ungrouped host indent 0, got %#v", items[0])
	}
	_, ok = items[1].(groupItem)
	if !ok {
		t.Fatalf("expected group row at index 1, got %#v", items[1])
	}
	grouped, ok := items[2].(Host)
	if !ok || grouped.ListIndent != 1 {
		t.Fatalf("expected grouped host indent 1, got %#v", items[2])
	}
	container, ok := items[3].(Host)
	if !ok || container.ListIndent != 2 {
		t.Fatalf("expected container indent 2, got %#v", items[3])
	}
}

func TestSaveFromFormRejectsDuplicateAlias(t *testing.T) {
	m := model{
		rawHosts: []Host{{ID: "h1", Alias: "web"}},
		inputs:   newFormInputs(),
	}
	m.inputs[0].SetValue("web")
	m.inputs[1].SetValue("10.0.0.1")
	m.inputs[2].SetValue("root")
	m.inputs[3].SetValue("22")
	m.inputs[4].SetValue("")
	m.inputs[5].SetValue("")
	m.buildGroupOptions("")

	if err := m.saveFromForm(); err == nil {
		t.Fatal("expected duplicate alias error, got nil")
	}
}
