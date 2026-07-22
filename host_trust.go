package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sshActionKind int

const (
	sshActionConnect sshActionKind = iota
	sshActionTest
	sshActionScan
	sshActionInstallKey
	sshActionRotation
)

type pendingSSHAction struct {
	kind          sshActionKind
	host          Host
	trustHost     Host
	hostIndex     int
	background    bool
	publicKey     string
	rotationIndex int
	rotationStage rotationStage
}

type hostTrustState struct {
	open       bool
	busy       bool
	current    Host
	actions    []pendingSSHAction
	suppressed map[string]bool
	errorText  string
}

type hostTrustCheckMsg struct {
	action pendingSSHAction
	known  bool
	err    error
}

type hostTrustFinishedMsg struct {
	token string
	err   error
}

type hostTrustActionFailedMsg struct{ err error }

func checkHostTrustCmd(action pendingSSHAction) tea.Cmd {
	return func() tea.Msg {
		known, err := hostKeyKnown(action.trustHost)
		return hostTrustCheckMsg{action: action, known: known, err: err}
	}
}

func knownHostToken(host Host) string {
	port := strings.TrimSpace(host.Port)
	if port == "" || port == "22" {
		return strings.TrimSpace(host.Hostname)
	}
	return "[" + strings.TrimSpace(host.Hostname) + "]:" + port
}

func hostKeyKnown(host Host) (bool, error) {
	token := knownHostToken(host)
	if token == "" {
		return false, errors.New("hostname is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	files := []string{
		filepath.Join(home, ".ssh", "known_hosts"),
		filepath.Join(home, ".ssh", "known_hosts2"),
		"/etc/ssh/ssh_known_hosts",
		"/etc/ssh/ssh_known_hosts2",
	}
	var checked bool
	for _, path := range files {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false, err
		}
		checked = true
		cmd := exec.Command("ssh-keygen", "-F", token, "-f", path)
		if err := cmd.Run(); err == nil {
			return true, nil
		} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			return false, fmt.Errorf("known-host lookup failed: %w", err)
		}
	}
	if !checked {
		return false, nil
	}
	return false, nil
}

func buildHostTrustCommand(host Host) (*exec.Cmd, error) {
	if strings.TrimSpace(host.Hostname) == "" {
		return nil, errors.New("hostname is required")
	}
	if !commandExists("ssh") {
		return nil, errors.New("ssh is required")
	}
	args := []string{
		"-o", "StrictHostKeyChecking=ask",
		"-o", "BatchMode=no",
		"-o", "ConnectTimeout=10",
		"-o", "NumberOfPasswordPrompts=0",
		"-o", "PreferredAuthentications=none",
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "PubkeyAuthentication=no",
	}
	if host.Port != "" && host.Port != "22" {
		if port, err := strconv.Atoi(host.Port); err != nil || port < 1 || port > 65535 {
			return nil, errors.New("port must be between 1 and 65535")
		}
		args = append(args, "-p", host.Port)
	}
	if host.ProxyJump != "" {
		args = append(args, "-J", host.ProxyJump)
	}
	args = append(args, sshTarget(host), "true")
	return exec.Command("ssh", args...), nil
}

func (m model) handleHostTrustCheck(msg hostTrustCheckMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.failPendingSSHAction(msg.action, msg.err)
	}
	if msg.known {
		return m.resumePendingSSHAction(msg.action)
	}
	token := knownHostToken(msg.action.trustHost)
	if msg.action.background && m.hostTrust.suppressed[token] {
		return m, nil
	}
	if m.hostTrust.suppressed == nil {
		m.hostTrust.suppressed = make(map[string]bool)
	}
	if !msg.action.background {
		delete(m.hostTrust.suppressed, token)
	}
	if !containsPendingSSHAction(m.hostTrust.actions, msg.action) {
		m.hostTrust.actions = append(m.hostTrust.actions, msg.action)
	}
	if !m.hostTrust.open {
		m.hostTrust.open = true
		m.hostTrust.current = msg.action.trustHost
		m.hostTrust.errorText = ""
	}
	return m, nil
}

func containsPendingSSHAction(actions []pendingSSHAction, candidate pendingSSHAction) bool {
	key := pendingSSHActionKey(candidate)
	for _, action := range actions {
		if pendingSSHActionKey(action) == key {
			return true
		}
	}
	return false
}

func pendingSSHActionKey(action pendingSSHAction) string {
	return fmt.Sprintf("%d|%s|%s|%d|%t|%s|%d|%s", action.kind, action.host.ID, knownHostToken(action.trustHost), action.hostIndex, action.background, action.publicKey, action.rotationIndex, action.rotationStage)
}

func (m model) updateHostTrust(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.hostTrust.busy {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "enter", "y":
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			m.hostTrust.errorText = homeErr.Error()
			return m, nil
		}
		if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
			m.hostTrust.errorText = fmt.Sprintf("could not prepare ~/.ssh: %v", err)
			return m, nil
		}
		cmd, err := buildHostTrustCommand(m.hostTrust.current)
		if err != nil {
			m.hostTrust.errorText = err.Error()
			return m, nil
		}
		m.hostTrust.busy = true
		m.hostTrust.errorText = ""
		token := knownHostToken(m.hostTrust.current)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return hostTrustFinishedMsg{token: token, err: err}
		})
	case "esc", "q", "n":
		return m.declineCurrentHostTrust()
	}
	return m, nil
}

func (m model) finishHostTrust(msg hostTrustFinishedMsg) (tea.Model, tea.Cmd) {
	m.hostTrust.busy = false
	known, lookupErr := hostKeyKnown(m.hostTrust.current)
	if lookupErr == nil && known {
		return m.completeCurrentHostTrust(msg.token)
	}
	reason := errors.New("host key was not trusted")
	if lookupErr != nil {
		reason = lookupErr
	} else if msg.err != nil {
		reason = fmt.Errorf("host key was not trusted: %w", msg.err)
	}
	return m.cancelCurrentHostTrust(msg.token, reason)
}

func (m model) completeCurrentHostTrust(token string) (tea.Model, tea.Cmd) {
	matched, remaining := partitionTrustActions(m.hostTrust.actions, token)
	m.hostTrust.actions = remaining
	m.hostTrust.open = false
	m.hostTrust.errorText = ""
	var cmds []tea.Cmd
	for _, action := range matched {
		var cmd tea.Cmd
		m, cmd = m.resumePendingSSHActionModel(action)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	m, nextCmd := m.advanceHostTrustQueue()
	if nextCmd != nil {
		cmds = append(cmds, nextCmd)
	}
	return m, tea.Batch(cmds...)
}

func (m model) declineCurrentHostTrust() (tea.Model, tea.Cmd) {
	token := knownHostToken(m.hostTrust.current)
	return m.cancelCurrentHostTrust(token, errors.New("host key enrollment was declined"))
}

func (m model) cancelCurrentHostTrust(token string, reason error) (tea.Model, tea.Cmd) {
	matched, remaining := partitionTrustActions(m.hostTrust.actions, token)
	m.hostTrust.actions = remaining
	m.hostTrust.open = false
	m.hostTrust.busy = false
	m.hostTrust.errorText = ""
	if m.hostTrust.suppressed == nil {
		m.hostTrust.suppressed = make(map[string]bool)
	}
	for _, action := range matched {
		if action.background {
			m.hostTrust.suppressed[token] = true
		}
	}
	var cmds []tea.Cmd
	for _, action := range matched {
		var cmd tea.Cmd
		m, cmd = m.failPendingSSHActionModel(action, reason)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	m, nextCmd := m.advanceHostTrustQueue()
	if nextCmd != nil {
		cmds = append(cmds, nextCmd)
	}
	return m, tea.Batch(cmds...)
}

func partitionTrustActions(actions []pendingSSHAction, token string) ([]pendingSSHAction, []pendingSSHAction) {
	var matched, remaining []pendingSSHAction
	for _, action := range actions {
		if knownHostToken(action.trustHost) == token {
			matched = append(matched, action)
		} else {
			remaining = append(remaining, action)
		}
	}
	return matched, remaining
}

func (m model) advanceHostTrustQueue() (model, tea.Cmd) {
	var cmds []tea.Cmd
	for len(m.hostTrust.actions) > 0 {
		action := m.hostTrust.actions[0]
		known, err := hostKeyKnown(action.trustHost)
		if err != nil {
			m.hostTrust.actions = m.hostTrust.actions[1:]
			var cmd tea.Cmd
			m, cmd = m.failPendingSSHActionModel(action, err)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			continue
		}
		if known {
			token := knownHostToken(action.trustHost)
			matched, remaining := partitionTrustActions(m.hostTrust.actions, token)
			m.hostTrust.actions = remaining
			for _, item := range matched {
				var cmd tea.Cmd
				m, cmd = m.resumePendingSSHActionModel(item)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			continue
		}
		m.hostTrust.open = true
		m.hostTrust.current = action.trustHost
		return m, tea.Batch(cmds...)
	}
	m.hostTrust = hostTrustState{suppressed: m.hostTrust.suppressed}
	return m, tea.Batch(cmds...)
}

func (m model) resumePendingSSHAction(action pendingSSHAction) (tea.Model, tea.Cmd) {
	return m.resumePendingSSHActionModel(action)
}

func (m model) resumePendingSSHActionModel(action pendingSSHAction) (model, tea.Cmd) {
	switch action.kind {
	case sshActionConnect:
		updated, cmd := m.connectToHostTrusted(action.host)
		return updated.(model), cmd
	case sshActionTest:
		return m, testConnectionTrusted(action.host)
	case sshActionScan:
		return m, scanDockerContainersTrusted(action.host, action.hostIndex, action.background)
	case sshActionInstallKey:
		cmd, err := buildCopyIDCommand(action.host, action.publicKey)
		if err != nil {
			return m, func() tea.Msg { return keyInstallFinishedMsg{err: err} }
		}
		m.keyInstall.phase = keyInstallRunning
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return keyInstallFinishedMsg{err: err} })
	case sshActionRotation:
		return m, m.rotationCommandTrusted(action.rotationIndex, action.rotationStage)
	default:
		return m, nil
	}
}

func (m model) failPendingSSHAction(action pendingSSHAction, err error) (tea.Model, tea.Cmd) {
	return m.failPendingSSHActionModel(action, err)
}

func (m model) failPendingSSHActionModel(action pendingSSHAction, err error) (model, tea.Cmd) {
	switch action.kind {
	case sshActionConnect:
		return m, func() tea.Msg { return hostTrustActionFailedMsg{err: err} }
	case sshActionTest:
		return m, func() tea.Msg { return testConnectionMsg{err: err} }
	case sshActionScan:
		if action.background {
			return m, nil
		}
		return m, func() tea.Msg {
			return scanDockerMsg{hostIndex: action.hostIndex, background: action.background, err: err}
		}
	case sshActionInstallKey:
		return m, func() tea.Msg { return keyInstallFinishedMsg{err: err} }
	case sshActionRotation:
		return m, func() tea.Msg {
			return rotationStepMsg{hostIndex: action.rotationIndex, stage: action.rotationStage, err: err, rollbackTried: true}
		}
	default:
		return m, nil
	}
}

func (m model) renderHostTrustOverlay(base string) string {
	width, height := normalizedSize(m.width, m.height)
	host := m.hostTrust.current
	port := host.Port
	if port == "" {
		port = "22"
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(colorText).Bold(true).Render("Unknown SSH server") + "\n")
	b.WriteString(formHintStyle.Render("Review the server fingerprint before trusting it.") + "\n\n")
	b.WriteString(formSectionStyle.Render("Target") + "\n")
	b.WriteString("Host  " + host.Hostname + "\n")
	b.WriteString("Port  " + port + "\n")
	if host.ProxyJump != "" {
		b.WriteString("Via   " + host.ProxyJump + "\n")
	}
	b.WriteString("\n")
	b.WriteString("OpenSSH will show the SHA256 fingerprint in the terminal. Compare it with the server console, then answer yes to trust it.\n\n")
	b.WriteString(formHintStyle.Render("Changed or revoked host keys are never replaced automatically.") + "\n\n")
	if m.hostTrust.busy {
		b.WriteString(m.spinner.View() + " Waiting for OpenSSH confirmation…")
	} else {
		b.WriteString(helpEntry("enter", "review fingerprint") + "  " + helpEntry("esc", "cancel"))
	}
	if m.hostTrust.errorText != "" {
		b.WriteString("\n\n" + testFailStyle.Render("✘ "+m.hostTrust.errorText))
	}
	modalWidth := min(68, max(width-6, 30))
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Width(modalWidth).
		Render(b.String())
	backdrop := fitViewToBounds(dimBase(base), width, height)
	return fitViewToBounds(overlayCenter(backdrop, modal, width, height), width, height)
}
