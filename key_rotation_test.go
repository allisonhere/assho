package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const testPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestOnlyKeyMaterial assho-test\n"

func writeTestKey(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(testPublicKey), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeExecutable(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestReadPublicKeyAndIdentityPair(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(private, []byte("private-placeholder"), 0600); err != nil {
		t.Fatal(err)
	}
	public := writeTestKey(t, dir, "id_ed25519.pub")

	got, err := publicKeyForIdentity(private)
	if err != nil {
		t.Fatal(err)
	}
	if got != public {
		t.Fatalf("got %q, want %q", got, public)
	}
	keyType, blob, err := readPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	if keyType != "ssh-ed25519" || !strings.HasPrefix(blob, "AAAAC3") {
		t.Fatalf("unexpected parsed key: %q %q", keyType, blob)
	}
}

func TestBuildCopyIDCommandKeepsPasswordOutOfArguments(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, dir, "ssh-copy-id")
	writeExecutable(t, dir, "sshpass")
	t.Setenv("PATH", dir)
	public := writeTestKey(t, dir, "rotation.pub")
	host := Host{Hostname: "server.example", User: "deploy", Port: "2222", ProxyJump: "jump.example", Password: "do-not-log"}

	cmd, err := buildCopyIDCommand(host, public)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{"sshpass", "-e", "ssh-copy-id", "-i", public, "-p", "2222", "ProxyJump=jump.example", "deploy@server.example"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("command %q is missing %q", joined, want)
		}
	}
	if strings.Contains(joined, host.Password) {
		t.Fatalf("password leaked into command arguments: %q", joined)
	}
	foundPasswordEnv := false
	for _, value := range cmd.Env {
		if value == "SSHPASS="+host.Password {
			foundPasswordEnv = true
		}
	}
	if !foundPasswordEnv {
		t.Fatal("SSHPASS environment was not set")
	}
}

func TestRotationJournalPermissionsAndResume(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	run := &rotationRun{
		ID:          "run-safe",
		CreatedAt:   time.Now().UTC(),
		NewIdentity: "/keys/private-path-only",
		Hosts:       []rotationHostResult{{HostID: "one", Alias: "prod", Status: rotationWorking, Stage: stageVerify}},
	}
	if err := saveRotationRun(run); err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(rotationDirectory())
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0700 {
		t.Fatalf("journal directory mode = %o, want 700", dirInfo.Mode().Perm())
	}
	path := filepath.Join(rotationDirectory(), "run-safe.json")
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0600 {
		t.Fatalf("journal mode = %o, want 600", fileInfo.Mode().Perm())
	}
	loaded, err := newestIncompleteRotation()
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.ID != run.ID || loaded.Hosts[0].Stage != stageVerify {
		t.Fatalf("unexpected resumed run: %#v", loaded)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "password") || strings.Contains(string(data), "PRIVATE KEY") {
		t.Fatalf("journal contains secret-like material: %s", data)
	}
}

func TestVerifiedReplacementUpdatesConfigBeforeRemoval(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := initialModel()
	m.rawHosts = []Host{{ID: "host-1", Alias: "prod", Hostname: "prod.example", User: "root", IdentityFile: "/keys/old"}}
	m.rotation = rotationState{phase: rotationRunning, run: &rotationRun{
		ID:           "run-update",
		CreatedAt:    time.Now().UTC(),
		NewIdentity:  "/keys/new",
		NewPublicKey: "/keys/new.pub",
		Hosts: []rotationHostResult{{
			HostID: "host-1", Alias: "prod", Status: rotationWorking, Stage: stageVerify, OldIdentity: "/keys/old",
		}},
	}}

	updatedModel, cmd := m.finishRotationStep(rotationStepMsg{hostIndex: 0, stage: stageVerify})
	updated := updatedModel.(model)
	if updated.rawHosts[0].IdentityFile != "/keys/new" {
		t.Fatalf("identity = %q, want replacement", updated.rawHosts[0].IdentityFile)
	}
	if updated.rotation.run.Hosts[0].Stage != stageRemove {
		t.Fatalf("stage = %q, want %q", updated.rotation.run.Hosts[0].Stage, stageRemove)
	}
	if updated.rotation.run.Hosts[0].OldIdentity != "/keys/old" {
		t.Fatal("old identity was not retained for safe remote cleanup")
	}
	if cmd == nil {
		t.Fatal("expected remote removal command after config update")
	}
}

func TestVerificationFailureNeverChangesConfiguredIdentity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := initialModel()
	m.rawHosts = []Host{{ID: "host-1", Alias: "prod", Hostname: "prod.example", IdentityFile: "/keys/old"}}
	m.rotation = rotationState{phase: rotationRunning, run: &rotationRun{
		ID: "run-fail", CreatedAt: time.Now().UTC(), NewIdentity: "/keys/new",
		Hosts: []rotationHostResult{{HostID: "host-1", Alias: "prod", Status: rotationWorking, Stage: stageVerify}},
	}}

	updatedModel, _ := m.finishRotationStep(rotationStepMsg{hostIndex: 0, stage: stageVerify, err: os.ErrPermission})
	updated := updatedModel.(model)
	if updated.rawHosts[0].IdentityFile != "/keys/old" {
		t.Fatalf("failed verification changed identity to %q", updated.rawHosts[0].IdentityFile)
	}
	if updated.rotation.run.Hosts[0].Status != rotationFailed {
		t.Fatalf("status = %q, want failed", updated.rotation.run.Hosts[0].Status)
	}
}

func TestRemovalFailureIsCleanupRequired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := initialModel()
	m.rawHosts = []Host{{ID: "host-1", Alias: "prod", Hostname: "prod.example", IdentityFile: "/keys/new"}}
	m.rotation = rotationState{phase: rotationRunning, run: &rotationRun{
		ID: "run-cleanup", CreatedAt: time.Now().UTC(), NewIdentity: "/keys/new",
		Hosts: []rotationHostResult{{HostID: "host-1", Alias: "prod", Status: rotationWorking, Stage: stageRemove, OldIdentity: "/keys/old"}},
	}}

	updatedModel, _ := m.finishRotationStep(rotationStepMsg{hostIndex: 0, stage: stageRemove, err: os.ErrPermission})
	updated := updatedModel.(model)
	if updated.rotation.run.Hosts[0].Status != rotationCleanupRequired {
		t.Fatalf("status = %q, want cleanup-required", updated.rotation.run.Hosts[0].Status)
	}
}

func TestRotationViewsStayWithinTerminal(t *testing.T) {
	m := model{
		width: 48, height: 16,
		rawHosts: []Host{{ID: "one", Alias: "a-very-long-production-host-name", Hostname: "prod.example", User: "deploy"}},
		rotation: rotationState{phase: rotationSelectHosts, selected: map[string]bool{"one": true}},
	}
	out := m.renderRotationView()
	lines := strings.Split(out, "\n")
	if len(lines) > m.height {
		t.Fatalf("view has %d lines, terminal height is %d", len(lines), m.height)
	}
	for i, line := range lines {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("line %d width = %d, terminal width = %d", i, ansi.StringWidth(line), m.width)
		}
	}
}

func TestInstallPickerDoesNotOverwriteIdentity(t *testing.T) {
	dir := t.TempDir()
	public := writeTestKey(t, dir, "other.pub")
	m := initialModel()
	m.state = stateFilePicker
	m.pickerUse = pickerInstallPublic
	m.form.inputs[fieldKeyFile].SetValue("/keys/configured")
	m.keyInstall = keyInstallState{phase: keyInstallChoose}
	m.returnFromFilePicker(true, public)
	if m.form.inputs[fieldKeyFile].Value() != "/keys/configured" {
		t.Fatal("install-only public key picker overwrote the configured private identity")
	}
	if m.state != stateKeyInstall || m.keyInstall.phase != keyInstallConfirm {
		t.Fatalf("unexpected picker return state: %v / %v", m.state, m.keyInstall.phase)
	}
}

func TestExistingKeyLoadFailureStaysOnExistingKey(t *testing.T) {
	m := model{rotation: rotationState{
		phase: rotationConfirm,
		run:   &rotationRun{NewIdentity: "/keys/current"},
	}}

	updatedModel, _ := m.finishRotationKey(rotationKeyReadyMsg{
		privateKey: "/keys/current",
		loaded:     true,
		err:        os.ErrPermission,
	})
	updated := updatedModel.(model)
	if updated.rotation.phase != rotationConfirm {
		t.Fatalf("phase = %v, want existing-key confirmation", updated.rotation.phase)
	}
	if updated.rotation.run == nil || updated.rotation.run.NewIdentity != "/keys/current" {
		t.Fatal("selected existing key was discarded")
	}
	if strings.Contains(strings.ToLower(updated.rotation.errorText), "generation") {
		t.Fatalf("existing-key error incorrectly mentions generation: %q", updated.rotation.errorText)
	}
}

func TestGenerationFailureReturnsToGenerateScreen(t *testing.T) {
	m := model{rotation: rotationState{phase: rotationGenerateKey}}
	updatedModel, _ := m.finishRotationKey(rotationKeyReadyMsg{
		privateKey: "/keys/new",
		loaded:     false,
		err:        os.ErrPermission,
	})
	updated := updatedModel.(model)
	if updated.rotation.phase != rotationGenerateKey {
		t.Fatalf("phase = %v, want generation", updated.rotation.phase)
	}
}

func TestMissingSSHAgentUsesIdentityDirectly(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	if sshAgentAvailable() {
		t.Fatal("empty SSH_AUTH_SOCK should not report an available agent")
	}
}

func TestRotationRejectsPublicKeyAsPrivateIdentity(t *testing.T) {
	public := writeTestKey(t, t.TempDir(), "id_ed25519.pub")
	err := validateRotationPrivateKey(public)
	if err == nil || !strings.Contains(err.Error(), "not its .pub") {
		t.Fatalf("expected a clear public-key rejection, got %v", err)
	}
}

var _ tea.Msg = rotationStepMsg{}
