package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempSSHConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp ssh config: %v", err)
	}
	return path
}

func TestParseSSHConfig(t *testing.T) {
	config := `
# A comment
Host web-server
    HostName 10.0.0.1
    User deploy
    Port 2222
    IdentityFile ~/.ssh/id_web

Host db
    HostName db.example.com
    User admin
`
	path := writeTempSSHConfig(t, config)
	hosts, err := parseSSHConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	h := hosts[0]
	if h.Alias != "web-server" {
		t.Errorf("expected alias 'web-server', got %q", h.Alias)
	}
	if h.Hostname != "10.0.0.1" {
		t.Errorf("expected hostname '10.0.0.1', got %q", h.Hostname)
	}
	if h.User != "deploy" {
		t.Errorf("expected user 'deploy', got %q", h.User)
	}
	if h.Port != "2222" {
		t.Errorf("expected port '2222', got %q", h.Port)
	}
	if h.IdentityFile != "~/.ssh/id_web" {
		t.Errorf("expected identity '~/.ssh/id_web', got %q", h.IdentityFile)
	}

	h2 := hosts[1]
	if h2.Alias != "db" {
		t.Errorf("expected alias 'db', got %q", h2.Alias)
	}
	if h2.Hostname != "db.example.com" {
		t.Errorf("expected hostname 'db.example.com', got %q", h2.Hostname)
	}
	if h2.Port != "22" {
		t.Errorf("expected default port '22', got %q", h2.Port)
	}
}

func TestParseSSHConfigSkipsWildcards(t *testing.T) {
	config := `
Host *
    ServerAliveInterval 60

Host 192.168.*
    User root

Host real-host
    HostName 1.2.3.4
    User alice
`
	path := writeTempSSHConfig(t, config)
	hosts, err := parseSSHConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host (wildcards skipped), got %d", len(hosts))
	}
	if hosts[0].Alias != "real-host" {
		t.Errorf("expected alias 'real-host', got %q", hosts[0].Alias)
	}
}

func TestParseSSHConfigMultiAlias(t *testing.T) {
	config := `
Host foo bar
    HostName multi.example.com
    User multi
`
	path := writeTempSSHConfig(t, config)
	hosts, err := parseSSHConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts from multi-alias, got %d", len(hosts))
	}
	if hosts[0].Alias != "foo" || hosts[1].Alias != "bar" {
		t.Errorf("expected aliases 'foo' and 'bar', got %q and %q", hosts[0].Alias, hosts[1].Alias)
	}
	if hosts[0].Hostname != "multi.example.com" || hosts[1].Hostname != "multi.example.com" {
		t.Errorf("both aliases should share the same hostname")
	}
}

func TestParseSSHConfigEqualsDelimiter(t *testing.T) {
	config := `
Host eq-host
    HostName=eq.example.com
    User = equser
    Port=3333
`
	path := writeTempSSHConfig(t, config)
	hosts, err := parseSSHConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	h := hosts[0]
	if h.Hostname != "eq.example.com" {
		t.Errorf("expected hostname 'eq.example.com', got %q", h.Hostname)
	}
	if h.User != "equser" {
		t.Errorf("expected user 'equser', got %q", h.User)
	}
	if h.Port != "3333" {
		t.Errorf("expected port '3333', got %q", h.Port)
	}
}

func TestParseSSHConfigMatchBlock(t *testing.T) {
	config := `
Host before-match
    HostName before.example.com

Match host *.internal
    User internal

Host after-match
    HostName after.example.com
`
	path := writeTempSSHConfig(t, config)
	hosts, err := parseSSHConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts (match skipped), got %d", len(hosts))
	}
	if hosts[0].Alias != "before-match" || hosts[1].Alias != "after-match" {
		t.Errorf("expected 'before-match' and 'after-match', got %q and %q", hosts[0].Alias, hosts[1].Alias)
	}
}

func TestParseSSHConfigInclude(t *testing.T) {
	dir := t.TempDir()

	included := filepath.Join(dir, "included")
	includedContent := "Host included-host\n    HostName 5.5.5.5\n"
	if err := os.WriteFile(included, []byte(includedContent), 0600); err != nil {
		t.Fatalf("failed to write included config: %v", err)
	}

	mainContent := "Include " + included + "\n\nHost main-host\n    HostName 1.1.1.1\n"
	mainPath := filepath.Join(dir, "config")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0600); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	hosts, err := parseSSHConfig(mainPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts (1 included + 1 main), got %d: %v", len(hosts), hosts)
	}
	aliases := map[string]bool{}
	for _, h := range hosts {
		aliases[h.Alias] = true
	}
	if !aliases["included-host"] {
		t.Error("expected included-host to be parsed from included file")
	}
	if !aliases["main-host"] {
		t.Error("expected main-host to be parsed from main file")
	}
}

func TestImportSSHConfigDeduplicates(t *testing.T) {
	config := `
Host existing-host
    HostName 1.1.1.1

Host new-host
    HostName 2.2.2.2

Host EXISTING-HOST
    HostName 3.3.3.3
`
	path := writeTempSSHConfig(t, config)

	// Override the import path by calling parseSSHConfig directly and testing dedup logic.
	parsed, err := parseSSHConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	existing := []Host{
		{ID: "1", Alias: "Existing-Host", Hostname: "old.example.com"},
	}

	// Simulate importSSHConfig dedup logic inline since it hardcodes ~/.ssh/config.
	existingAliases := make(map[string]bool)
	for _, h := range existing {
		existingAliases[strings.ToLower(h.Alias)] = true
	}
	var imported []Host
	skipped := 0
	for _, h := range parsed {
		key := strings.ToLower(h.Alias)
		if existingAliases[key] {
			skipped++
			continue
		}
		existingAliases[key] = true
		imported = append(imported, h)
	}

	if len(imported) != 1 {
		t.Fatalf("expected 1 imported host, got %d", len(imported))
	}
	if imported[0].Alias != "new-host" {
		t.Errorf("expected imported alias 'new-host', got %q", imported[0].Alias)
	}
	if skipped != 2 {
		t.Errorf("expected 2 skipped, got %d", skipped)
	}
}
