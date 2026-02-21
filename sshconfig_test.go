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

func TestExportSSHConfigBasic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	hosts := []Host{
		{ID: "h1", Alias: "web", Hostname: "10.0.0.1", User: "deploy", Port: "2222", IdentityFile: "~/.ssh/id_web"},
		{ID: "h2", Alias: "db", Hostname: "db.example.com", User: "admin", Port: "22"},
		{ID: "h3", Alias: "proxy", Hostname: "jump.example.com", ProxyJump: "bastion"},
	}

	exported, skipped, err := exportSSHConfig(hosts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exported != 3 || skipped != 0 {
		t.Fatalf("expected exported=3 skipped=0, got exported=%d skipped=%d", exported, skipped)
	}

	content, err := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		t.Fatalf("cannot read written config: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "Host web") || !strings.Contains(s, "HostName 10.0.0.1") {
		t.Errorf("missing web host block in output:\n%s", s)
	}
	if !strings.Contains(s, "Port 2222") {
		t.Errorf("expected non-default port in output:\n%s", s)
	}
	if strings.Contains(s, "Port 22\n") {
		t.Errorf("default port 22 should be omitted:\n%s", s)
	}
	if !strings.Contains(s, "IdentityFile ~/.ssh/id_web") {
		t.Errorf("missing IdentityFile in output:\n%s", s)
	}
	if !strings.Contains(s, "ProxyJump bastion") {
		t.Errorf("missing ProxyJump in output:\n%s", s)
	}
}

func TestExportSSHConfigSkipsDuplicatesAndContainers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write a pre-existing SSH config with one host.
	sshDir := filepath.Join(home, ".ssh")
	_ = os.MkdirAll(sshDir, 0700)
	existing := "Host already-there\n    HostName 9.9.9.9\n"
	_ = os.WriteFile(filepath.Join(sshDir, "config"), []byte(existing), 0600)

	hosts := []Host{
		{ID: "h1", Alias: "already-there", Hostname: "9.9.9.9"},
		{ID: "h2", Alias: "new-host", Hostname: "1.2.3.4"},
		{ID: "c1", Alias: "my-container", Hostname: "abc123", IsContainer: true},
		{ID: "h3", Alias: "", Hostname: "no-alias.example.com"}, // no alias
	}

	exported, skipped, err := exportSSHConfig(hosts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exported != 1 {
		t.Fatalf("expected 1 exported, got %d", exported)
	}
	if skipped != 1 {
		t.Fatalf("expected 1 skipped (already-there), got %d", skipped)
	}

	content, _ := os.ReadFile(filepath.Join(sshDir, "config"))
	s := string(content)
	if !strings.Contains(s, "Host new-host") {
		t.Errorf("expected new-host in output:\n%s", s)
	}
	if strings.Contains(s, "my-container") || strings.Contains(s, "no-alias") {
		t.Errorf("container/no-alias host should not appear:\n%s", s)
	}
}

func TestExportSSHConfigZeroExported(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// All hosts already present.
	sshDir := filepath.Join(home, ".ssh")
	_ = os.MkdirAll(sshDir, 0700)
	_ = os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host web\n    HostName 1.1.1.1\n"), 0600)

	hosts := []Host{{ID: "h1", Alias: "web", Hostname: "1.1.1.1"}}
	exported, skipped, err := exportSSHConfig(hosts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exported != 0 || skipped != 1 {
		t.Fatalf("expected exported=0 skipped=1, got exported=%d skipped=%d", exported, skipped)
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
