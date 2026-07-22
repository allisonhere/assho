package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func writeKnownHosts(t *testing.T, home, contents string) string {
	t.Helper()
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHostKeyKnownSupportsDefaultAndCustomPorts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeKnownHosts(t, home,
		"server.example ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestOnlyDefault\n"+
			"[other.example]:2222 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestOnlyCustom\n")

	for _, host := range []Host{
		{Hostname: "server.example", Port: "22"},
		{Hostname: "other.example", Port: "2222"},
	} {
		known, err := hostKeyKnown(host)
		if err != nil {
			t.Fatal(err)
		}
		if !known {
			t.Fatalf("expected %s to be known", knownHostToken(host))
		}
	}
	known, err := hostKeyKnown(Host{Hostname: "new.example", Port: "22"})
	if err != nil {
		t.Fatal(err)
	}
	if known {
		t.Fatal("unexpected known-host match")
	}
}

func TestHostKeyKnownSupportsHashedEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := writeKnownHosts(t, home, "hashed.example ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestOnlyHashed\n")
	if output, err := exec.Command("ssh-keygen", "-H", "-f", path).CombinedOutput(); err != nil {
		t.Fatalf("hash known_hosts: %v (%s)", err, output)
	}
	known, err := hostKeyKnown(Host{Hostname: "hashed.example", Port: "22"})
	if err != nil {
		t.Fatal(err)
	}
	if !known {
		t.Fatal("hashed known-host entry was not recognized")
	}
}

func TestHostTrustCommandUsesReviewedEnrollment(t *testing.T) {
	host := Host{Hostname: "server.example", User: "deploy", Port: "2222", ProxyJump: "jump.example"}
	cmd, err := buildHostTrustCommand(host)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(cmd.Args, " ")
	for _, required := range []string{
		"StrictHostKeyChecking=ask",
		"PreferredAuthentications=none",
		"NumberOfPasswordPrompts=0",
		"-p 2222",
		"-J jump.example",
		"deploy@server.example",
	} {
		if !strings.Contains(args, required) {
			t.Fatalf("enrollment command %q is missing %q", args, required)
		}
	}
	if strings.Contains(args, "StrictHostKeyChecking=no") || strings.Contains(args, "UserKnownHostsFile=/dev/null") {
		t.Fatalf("insecure enrollment option in %q", args)
	}
}

func TestPostEnrollmentCommandsRequireKnownHost(t *testing.T) {
	host := Host{Hostname: "server.example", User: "deploy", Port: "2222", ProxyJump: "jump.example"}
	trusted := strings.Join(buildTrustedSSHArgs(host, false, ""), " ")
	if !strings.Contains(trusted, "StrictHostKeyChecking=yes") {
		t.Fatalf("trusted TUI connection is not strict: %q", trusted)
	}
	cliCompatible := strings.Join(buildSSHArgs(host, false, ""), " ")
	if strings.Contains(cliCompatible, "StrictHostKeyChecking=yes") {
		t.Fatalf("existing CLI argument builder unexpectedly changed: %q", cliCompatible)
	}

	dir := t.TempDir()
	writeExecutable(t, dir, "ssh-copy-id")
	t.Setenv("PATH", dir)
	public := writeTestKey(t, dir, "selected.pub")
	copyCommand, err := buildCopyIDCommand(host, public)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(copyCommand.Args, " "), "StrictHostKeyChecking=yes") {
		t.Fatal("ssh-copy-id could bypass reviewed host trust")
	}
	rotationArgs := strings.Join(sshArgs(host, "/keys/replacement", true), " ")
	if !strings.Contains(rotationArgs, "StrictHostKeyChecking=yes") {
		t.Fatal("rotation SSH command could bypass reviewed host trust")
	}
}

func TestUnknownActionsQueueAndDeduplicate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	host := Host{ID: "one", Alias: "prod", Hostname: "prod.example", Port: "22"}
	action := pendingSSHAction{kind: sshActionScan, host: host, trustHost: host, hostIndex: 0, background: true}
	m := model{}

	updatedModel, _ := m.handleHostTrustCheck(hostTrustCheckMsg{action: action, known: false})
	updated := updatedModel.(model)
	updatedModel, _ = updated.handleHostTrustCheck(hostTrustCheckMsg{action: action, known: false})
	updated = updatedModel.(model)
	if !updated.hostTrust.open || len(updated.hostTrust.actions) != 1 {
		t.Fatalf("expected one open deduplicated request, got open=%v actions=%d", updated.hostTrust.open, len(updated.hostTrust.actions))
	}

	declinedModel, _ := updated.declineCurrentHostTrust()
	declined := declinedModel.(model)
	if declined.hostTrust.open || !declined.hostTrust.suppressed[knownHostToken(host)] {
		t.Fatal("declined background enrollment was not suppressed")
	}
	ignoredModel, cmd := declined.handleHostTrustCheck(hostTrustCheckMsg{action: action, known: false})
	ignored := ignoredModel.(model)
	if ignored.hostTrust.open || cmd != nil {
		t.Fatal("suppressed background action prompted again")
	}
}

func TestForegroundActionOverridesBackgroundSuppression(t *testing.T) {
	host := Host{ID: "one", Hostname: "prod.example", Port: "22"}
	token := knownHostToken(host)
	m := model{hostTrust: hostTrustState{suppressed: map[string]bool{token: true}}}
	action := pendingSSHAction{kind: sshActionTest, host: host, trustHost: host}
	updatedModel, _ := m.handleHostTrustCheck(hostTrustCheckMsg{action: action, known: false})
	updated := updatedModel.(model)
	if !updated.hostTrust.open || updated.hostTrust.suppressed[token] {
		t.Fatal("foreground action did not reopen enrollment")
	}
}

func TestKnownHostConnectResumesOriginalAction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASSHO_STORE_PASSWORD", "0")
	writeKnownHosts(t, home, "prod.example ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestOnlyConnect\n")
	host := Host{ID: "prod", Alias: "prod", Hostname: "prod.example", User: "root", Port: "22"}
	m := initialModel()
	m.rawHosts = []Host{host}
	m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))

	queuedModel, checkCmd := m.connectToHost(host)
	if checkCmd == nil {
		t.Fatal("expected asynchronous host trust check")
	}
	checkedModel, _ := queuedModel.(model).Update(checkCmd())
	checked := checkedModel.(model)
	if checked.sshToRun == nil || checked.sshToRun.ID != host.ID {
		t.Fatal("known host did not resume the original connection")
	}
	if checked.hostTrust.open {
		t.Fatal("known host unexpectedly opened enrollment")
	}
}

func TestAcceptedEnrollmentResumesQueuedAction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	host := Host{ID: "prod", Hostname: "prod.example", Port: "22"}
	action := pendingSSHAction{kind: sshActionTest, host: host, trustHost: host}
	m := model{hostTrust: hostTrustState{open: true, current: host, actions: []pendingSSHAction{action}}}
	writeKnownHosts(t, home, "prod.example ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestOnlyAccepted\n")

	updatedModel, cmd := m.finishHostTrust(hostTrustFinishedMsg{token: knownHostToken(host), err: os.ErrPermission})
	updated := updatedModel.(model)
	if updated.hostTrust.open || len(updated.hostTrust.actions) != 0 {
		t.Fatal("accepted enrollment did not clear the queue")
	}
	if cmd == nil {
		t.Fatal("accepted enrollment did not resume the queued test")
	}
}

func TestDeclinedRotationTrustDoesNotStartRollback(t *testing.T) {
	action := pendingSSHAction{kind: sshActionRotation, rotationIndex: 2, rotationStage: stageInstall}
	m := model{}
	_, cmd := m.failPendingSSHActionModel(action, os.ErrPermission)
	if cmd == nil {
		t.Fatal("expected rotation failure message")
	}
	rawMsg := cmd()
	msg, ok := rawMsg.(rotationStepMsg)
	if !ok {
		t.Fatalf("unexpected message type %T", rawMsg)
	}
	if !msg.rollbackTried {
		t.Fatal("trust rejection could trigger an unnecessary remote rollback")
	}
}

func TestHostTrustOverlayFitsTerminal(t *testing.T) {
	for _, size := range []struct{ width, height int }{{36, 12}, {80, 24}, {120, 36}} {
		m := model{
			width: size.width, height: size.height,
			hostTrust: hostTrustState{open: true, current: Host{Hostname: "a-very-long-proxmox-hostname.example", Port: "2222", ProxyJump: "jump.example"}},
		}
		out := m.renderHostTrustOverlay("dashboard")
		lines := strings.Split(out, "\n")
		if len(lines) > size.height {
			t.Fatalf("%dx%d: got %d lines", size.width, size.height, len(lines))
		}
		for i, line := range lines {
			if ansi.StringWidth(line) > size.width {
				t.Fatalf("%dx%d line %d has width %d", size.width, size.height, i, ansi.StringWidth(line))
			}
		}
	}
}
