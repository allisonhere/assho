package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cliTestBinary is the path to the compiled assho binary, built once in TestMain.
var cliTestBinary string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "assho-cli-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	cliTestBinary = filepath.Join(tmp, "assho")
	out, err := exec.Command("go", "build", "-o", cliTestBinary, ".").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build assho binary: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// writeTempConfig saves hosts to a fresh temp HOME and returns that HOME path.
func writeTempConfig(t *testing.T, hosts []Host) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")
	if err := saveConfig(nil, hosts, nil); err != nil {
		t.Fatalf("writeTempConfig: %v", err)
	}
	return home
}

// --- fprintAliases ---

func TestFprintAliasesIncludesHosts(t *testing.T) {
	hosts := []Host{
		{ID: "h1", Alias: "web"},
		{ID: "h2", Alias: "db"},
	}
	var buf bytes.Buffer
	fprintAliases(&buf, hosts)
	out := buf.String()
	for _, alias := range []string{"web", "db"} {
		if !strings.Contains(out, alias) {
			t.Errorf("expected alias %q in output, got: %q", alias, out)
		}
	}
}

func TestFprintAliasesIncludesContainers(t *testing.T) {
	hosts := []Host{
		{ID: "h1", Alias: "docker-host", Containers: []Host{
			{ID: "c1", Alias: "sonarr", IsContainer: true},
		}},
	}
	var buf bytes.Buffer
	fprintAliases(&buf, hosts)
	out := buf.String()
	if !strings.Contains(out, "docker-host") {
		t.Error("expected parent host alias in output")
	}
	if !strings.Contains(out, "sonarr") {
		t.Error("expected container alias in output")
	}
}

func TestFprintAliasesEmptyHosts(t *testing.T) {
	var buf bytes.Buffer
	fprintAliases(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("expected empty output for nil hosts, got: %q", buf.String())
	}
}

// --- completion subprocess tests ---

func TestCLIAliasesOutputsAliases(t *testing.T) {
	home := writeTempConfig(t, []Host{
		{ID: "h1", Alias: "web", Hostname: "10.0.0.1", User: "root"},
		{ID: "h2", Alias: "db", Hostname: "10.0.0.2", User: "root"},
	})
	out, err := runCLI(t, home, "_aliases")
	if err != nil {
		t.Fatalf("_aliases failed: %v\noutput: %s", err, out)
	}
	for _, alias := range []string{"web", "db"} {
		if !strings.Contains(out, alias) {
			t.Errorf("expected alias %q in _aliases output, got: %s", alias, out)
		}
	}
}

func TestCLICompletionBash(t *testing.T) {
	out, err := runCLI(t, t.TempDir(), "completion", "bash")
	if err != nil {
		t.Fatalf("completion bash failed: %v", err)
	}
	if !strings.Contains(out, "complete -F _assho_completions assho") {
		t.Errorf("expected bash completion registration, got:\n%s", out)
	}
}

func TestCLICompletionZsh(t *testing.T) {
	out, err := runCLI(t, t.TempDir(), "completion", "zsh")
	if err != nil {
		t.Fatalf("completion zsh failed: %v", err)
	}
	if !strings.Contains(out, "compdef _assho assho") {
		t.Errorf("expected zsh compdef in output, got:\n%s", out)
	}
}

func TestCLICompletionFish(t *testing.T) {
	out, err := runCLI(t, t.TempDir(), "completion", "fish")
	if err != nil {
		t.Fatalf("completion fish failed: %v", err)
	}
	if !strings.Contains(out, "complete -c assho") {
		t.Errorf("expected fish completion in output, got:\n%s", out)
	}
}

func TestCLICompletionUnknownShell(t *testing.T) {
	out, err := runCLI(t, t.TempDir(), "completion", "powershell")
	if err == nil {
		t.Fatal("expected non-zero exit for unknown shell")
	}
	if !strings.Contains(out, "unknown shell") {
		t.Errorf("expected 'unknown shell' message, got: %q", out)
	}
}

func TestCLICompletionNoArgs(t *testing.T) {
	out, err := runCLI(t, t.TempDir(), "completion")
	if err == nil {
		t.Fatal("expected non-zero exit when completion called with no shell")
	}
	if !strings.Contains(out, "usage:") {
		t.Errorf("expected usage message, got: %q", out)
	}
}

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

// --- fprintCLIList ---

func TestFprintCLIListHeader(t *testing.T) {
	var buf bytes.Buffer
	fprintCLIList(&buf, nil)
	out := buf.String()
	for _, col := range []string{"ALIAS", "HOST", "PORT", "USER", "NOTES"} {
		if !strings.Contains(out, col) {
			t.Errorf("expected header column %q in output", col)
		}
	}
}

func TestFprintCLIListSkipsContainers(t *testing.T) {
	hosts := []Host{
		{ID: "h1", Alias: "web", Hostname: "10.0.0.1", User: "root", Port: "22"},
		{ID: "c1", Alias: "ctr", Hostname: "abc123", IsContainer: true},
	}
	var buf bytes.Buffer
	fprintCLIList(&buf, hosts)
	out := buf.String()
	if strings.Contains(out, "ctr") {
		t.Error("container row should not appear in cliList output")
	}
	if !strings.Contains(out, "web") {
		t.Error("expected host row to appear in cliList output")
	}
}

func TestFprintCLIListDefaultsPortTo22(t *testing.T) {
	hosts := []Host{
		{ID: "h1", Alias: "srv", Hostname: "10.0.0.1", User: "root", Port: ""},
	}
	var buf bytes.Buffer
	fprintCLIList(&buf, hosts)
	if !strings.Contains(buf.String(), "22") {
		t.Error("expected default port 22 when port is empty")
	}
}

func TestFprintCLIListTruncatesNotes(t *testing.T) {
	longNote := strings.Repeat("x", 40)
	hosts := []Host{
		{ID: "h1", Alias: "srv", Hostname: "10.0.0.1", User: "root", Port: "22", Notes: longNote},
	}
	var buf bytes.Buffer
	fprintCLIList(&buf, hosts)
	out := buf.String()
	if strings.Contains(out, longNote) {
		t.Error("expected long note to be truncated")
	}
	if !strings.Contains(out, "…") {
		t.Error("expected truncation ellipsis in output")
	}
}

func TestFprintCLIListMultipleHosts(t *testing.T) {
	hosts := []Host{
		{ID: "h1", Alias: "alpha", Hostname: "10.0.0.1", User: "root", Port: "22"},
		{ID: "h2", Alias: "beta", Hostname: "10.0.0.2", User: "admin", Port: "2222"},
	}
	var buf bytes.Buffer
	fprintCLIList(&buf, hosts)
	out := buf.String()
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Error("expected both hosts in output")
	}
	if !strings.Contains(out, "2222") {
		t.Error("expected non-default port in output")
	}
}

// --- CLI subprocess tests (require binary built in TestMain) ---

func runCLI(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(cliTestBinary, args...)
	cmd.Env = append(os.Environ(), "HOME="+home, "ASSHO_STORE_PASSWORD=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestCLITestNoAlias(t *testing.T) {
	out, err := runCLI(t, t.TempDir(), "test")
	if err == nil {
		t.Fatal("expected non-zero exit for missing alias argument")
	}
	if !strings.Contains(out, "usage:") {
		t.Errorf("expected usage message, got: %q", out)
	}
}

func TestCLITestUnknownAlias(t *testing.T) {
	home := writeTempConfig(t, []Host{
		{ID: "h1", Alias: "web", Hostname: "10.0.0.1", User: "root"},
	})
	out, err := runCLI(t, home, "test", "no-such-host")
	if err == nil {
		t.Fatal("expected non-zero exit for unknown alias")
	}
	if !strings.Contains(out, "host not found") {
		t.Errorf("expected 'host not found', got: %q", out)
	}
}

func TestCLITestAmbiguousAlias(t *testing.T) {
	home := writeTempConfig(t, []Host{
		{ID: "h1", Alias: "web", Hostname: "10.0.0.1", User: "root"},
		{ID: "h2", Alias: "web", Hostname: "10.0.0.2", User: "root"},
	})
	out, err := runCLI(t, home, "test", "web")
	if err == nil {
		t.Fatal("expected non-zero exit for ambiguous alias")
	}
	if !strings.Contains(out, "ambiguous") {
		t.Errorf("expected 'ambiguous' in output, got: %q", out)
	}
}

func TestCLIListOutputFormat(t *testing.T) {
	home := writeTempConfig(t, []Host{
		{ID: "h1", Alias: "prod-web", Hostname: "10.0.0.1", User: "deploy", Port: "2222"},
	})
	out, err := runCLI(t, home, "list")
	if err != nil {
		t.Fatalf("assho list failed: %v\noutput: %q", err, out)
	}
	for _, want := range []string{"ALIAS", "prod-web", "10.0.0.1", "deploy", "2222"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in list output, got:\n%s", want, out)
		}
	}
}

func TestSaveFromFormRejectsDuplicateAlias(t *testing.T) {
	m := model{
		rawHosts: []Host{{ID: "h1", Alias: "web"}},
		form:     formState{inputs: newFormInputs()},
	}
	m.form.inputs[0].SetValue("web")
	m.form.inputs[1].SetValue("10.0.0.1")
	m.form.inputs[2].SetValue("root")
	m.form.inputs[3].SetValue("22")
	m.form.inputs[4].SetValue("")
	m.form.inputs[5].SetValue("")
	m.buildGroupOptions("")

	if err := m.saveFromForm(); err == nil {
		t.Fatal("expected duplicate alias error, got nil")
	}
}
