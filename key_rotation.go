package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type filePickerPurpose int

const (
	pickerIdentity filePickerPurpose = iota
	pickerInstallPublic
	pickerRotationPrivate
)

type keyInstallPhase int

const (
	keyInstallChoose keyInstallPhase = iota
	keyInstallConfirm
	keyInstallRunning
	keyInstallDone
)

type keyInstallState struct {
	phase       keyInstallPhase
	choice      int
	host        Host
	publicKey   string
	fingerprint string
	errorText   string
}

type keyInstallFinishedMsg struct{ err error }

type rotationPhase int

const (
	rotationSelectHosts rotationPhase = iota
	rotationChooseKey
	rotationGenerateKey
	rotationConfirm
	rotationRunning
	rotationSummary
)

type rotationStage string

const (
	stagePreflight rotationStage = "preflight"
	stageInstall   rotationStage = "install"
	stageVerify    rotationStage = "verify"
	stageUpdate    rotationStage = "update-local"
	stageRemove    rotationStage = "remove-old"
	stageFinal     rotationStage = "final-verify"
)

type rotationHostStatus string

const (
	rotationPending         rotationHostStatus = "pending"
	rotationWorking         rotationHostStatus = "working"
	rotationComplete        rotationHostStatus = "complete"
	rotationFailed          rotationHostStatus = "failed"
	rotationCleanupRequired rotationHostStatus = "cleanup-required"
)

type rotationHostResult struct {
	HostID         string             `json:"host_id"`
	Alias          string             `json:"alias"`
	Target         string             `json:"target"`
	Status         rotationHostStatus `json:"status"`
	Stage          rotationStage      `json:"stage"`
	Error          string             `json:"error,omitempty"`
	OldFingerprint string             `json:"old_fingerprint,omitempty"`
	OldIdentity    string             `json:"old_identity,omitempty"`
	BackupPath     string             `json:"backup_path,omitempty"`
	NewPreexisting bool               `json:"new_key_preexisting,omitempty"`
}

type rotationRun struct {
	ID             string               `json:"id"`
	CreatedAt      time.Time            `json:"created_at"`
	CompletedAt    *time.Time           `json:"completed_at,omitempty"`
	NewIdentity    string               `json:"new_identity"`
	NewPublicKey   string               `json:"new_public_key"`
	NewFingerprint string               `json:"new_fingerprint"`
	Current        int                  `json:"current"`
	Hosts          []rotationHostResult `json:"hosts"`
	Complete       bool                 `json:"complete"`
}

type rotationState struct {
	phase       rotationPhase
	cursor      int
	selected    map[string]bool
	keyChoice   int
	pathInput   textinput.Model
	run         *rotationRun
	errorText   string
	resumeRun   *rotationRun
	agentLoaded bool
}

type rotationStepMsg struct {
	hostIndex      int
	stage          rotationStage
	preexisting    bool
	oldFingerprint string
	backupPath     string
	err            error
	rollbackTried  bool
}

type rotationKeyReadyMsg struct {
	privateKey string
	loaded     bool
	err        error
}

func hostFromForm(m model) Host {
	return Host{
		ID:           m.form.selectedHost.ID,
		Alias:        strings.TrimSpace(m.form.inputs[fieldAlias].Value()),
		Hostname:     strings.TrimSpace(m.form.inputs[fieldHostname].Value()),
		User:         strings.TrimSpace(m.form.inputs[fieldUser].Value()),
		Port:         strings.TrimSpace(m.form.inputs[fieldPort].Value()),
		IdentityFile: strings.TrimSpace(m.form.inputs[fieldKeyFile].Value()),
		Password:     m.form.inputs[fieldPassword].Value(),
		ProxyJump:    strings.TrimSpace(m.form.inputs[fieldProxyJump].Value()),
	}
}

func (m model) openKeyInstall() (tea.Model, tea.Cmd) {
	host := hostFromForm(m)
	m.keyInstall = keyInstallState{phase: keyInstallChoose, host: host}
	m.state = stateKeyInstall
	return m, nil
}

func (m model) updateKeyInstall(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		if m.keyInstall.phase == keyInstallDone {
			m.state = stateForm
			return m, m.focusInputs()
		}
		if m.keyInstall.phase == keyInstallConfirm {
			m.keyInstall.phase = keyInstallChoose
			m.keyInstall.errorText = ""
			return m, nil
		}
		if m.keyInstall.phase != keyInstallRunning {
			m.state = stateForm
			return m, m.focusInputs()
		}
	case "up", "k":
		if m.keyInstall.phase == keyInstallChoose {
			m.keyInstall.choice = (m.keyInstall.choice + 2) % 3
		}
	case "down", "j":
		if m.keyInstall.phase == keyInstallChoose {
			m.keyInstall.choice = (m.keyInstall.choice + 1) % 3
		}
	case "enter":
		switch m.keyInstall.phase {
		case keyInstallChoose:
			m.keyInstall.errorText = ""
			switch m.keyInstall.choice {
			case 0:
				path, err := publicKeyForIdentity(m.keyInstall.host.IdentityFile)
				if err != nil {
					m.keyInstall.errorText = err.Error()
					return m, nil
				}
				m.keyInstall.publicKey = path
				m.keyInstall.fingerprint, _ = publicKeyFingerprint(path)
				m.keyInstall.phase = keyInstallConfirm
			case 1:
				m.keyInstall.publicKey = ""
				m.keyInstall.fingerprint = "ssh-agent/default identities"
				m.keyInstall.phase = keyInstallConfirm
			case 2:
				m.pickerUse = pickerInstallPublic
				m.filepicker.AllowedTypes = []string{".pub"}
				m.state = stateFilePicker
				return m, m.filepicker.Init()
			}
		case keyInstallConfirm:
			return m, checkHostTrustCmd(pendingSSHAction{
				kind:      sshActionInstallKey,
				host:      m.keyInstall.host,
				trustHost: m.keyInstall.host,
				publicKey: m.keyInstall.publicKey,
			})
		case keyInstallDone:
			m.state = stateForm
			return m, m.focusInputs()
		}
	}
	return m, nil
}

func (m model) finishKeyInstall(msg keyInstallFinishedMsg) (tea.Model, tea.Cmd) {
	m.keyInstall.phase = keyInstallDone
	if msg.err != nil {
		m.keyInstall.errorText = "Key installation failed: " + msg.err.Error()
	} else {
		m.keyInstall.errorText = ""
	}
	return m, nil
}

func (m *model) returnFromFilePicker(selected bool, path string) {
	switch m.pickerUse {
	case pickerInstallPublic:
		m.state = stateKeyInstall
		if selected {
			if _, _, err := readPublicKey(path); err != nil {
				m.keyInstall.errorText = err.Error()
				return
			}
			m.keyInstall.publicKey = path
			m.keyInstall.fingerprint, _ = publicKeyFingerprint(path)
			m.keyInstall.phase = keyInstallConfirm
		}
	case pickerRotationPrivate:
		m.state = stateRotation
		if selected {
			if err := validateRotationPrivateKey(path); err != nil {
				m.rotation.phase = rotationChooseKey
				m.rotation.errorText = err.Error()
				break
			}
			m.rotation.errorText = ""
			m.rotation.phase = rotationConfirm
			m.rotation.run = &rotationRun{NewIdentity: path}
		}
	default:
		m.state = stateForm
		m.form.focus = controlKeyFile
		if selected {
			m.form.inputs[fieldKeyFile].SetValue(path)
			m.form.inputs[fieldKeyFile].CursorEnd()
		}
	}
	m.pickerUse = pickerIdentity
}

func publicKeyForIdentity(identity string) (string, error) {
	identity = expandPath(strings.TrimSpace(identity))
	if identity == "" {
		return "", errors.New("this host has no configured identity; choose Browse or ssh-agent/default")
	}
	if strings.HasSuffix(identity, ".pub") {
		if _, _, err := readPublicKey(identity); err != nil {
			return "", err
		}
		return identity, nil
	}
	path := identity + ".pub"
	if _, _, err := readPublicKey(path); err != nil {
		return "", fmt.Errorf("matching public key %s is unavailable: %w", path, err)
	}
	return path, nil
}

func validateRotationPrivateKey(path string) error {
	path = expandPath(strings.TrimSpace(path))
	if path == "" {
		return errors.New("select an SSH private key")
	}
	if strings.HasSuffix(strings.ToLower(path), ".pub") {
		return errors.New("select the private key, not its .pub file")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("selected private key is unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("selected private key is not a regular file")
	}
	if _, err := publicKeyForIdentity(path); err != nil {
		return err
	}
	return nil
}

func readPublicKey(path string) (string, string, error) {
	data, err := os.ReadFile(expandPath(path))
	if err != nil {
		return "", "", err
	}
	fields := strings.Fields(string(data))
	for i := 0; i+1 < len(fields); i++ {
		if isPublicKeyType(fields[i]) {
			return fields[i], fields[i+1], nil
		}
	}
	return "", "", fmt.Errorf("%s is not an OpenSSH public key", path)
}

func isPublicKeyType(s string) bool {
	return strings.HasPrefix(s, "ssh-") || strings.HasPrefix(s, "ecdsa-") || strings.HasPrefix(s, "sk-")
}

func publicKeyFingerprint(path string) (string, error) {
	out, err := exec.Command("ssh-keygen", "-lf", expandPath(path)).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fingerprint failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func buildCopyIDCommand(host Host, publicKey string) (*exec.Cmd, error) {
	if !commandExists("ssh-copy-id") {
		return nil, errors.New("ssh-copy-id is required")
	}
	if strings.TrimSpace(host.Hostname) == "" {
		return nil, errors.New("hostname is required")
	}
	args := []string{}
	if publicKey != "" {
		if _, _, err := readPublicKey(publicKey); err != nil {
			return nil, err
		}
		args = append(args, "-i", expandPath(publicKey))
	}
	if host.Port != "" && host.Port != "22" {
		args = append(args, "-p", host.Port)
	}
	if host.ProxyJump != "" {
		args = append(args, "-o", "ProxyJump="+host.ProxyJump)
	}
	args = append(args, "-o", "StrictHostKeyChecking=yes")
	args = append(args, sshTarget(host))
	if host.Password != "" && commandExists("sshpass") {
		cmd := exec.Command("sshpass", append([]string{"-e", "ssh-copy-id"}, args...)...)
		cmd.Env = append(os.Environ(), "SSHPASS="+host.Password)
		return cmd, nil
	}
	return exec.Command("ssh-copy-id", args...), nil
}

func sshTarget(host Host) string {
	if host.User == "" {
		return host.Hostname
	}
	return host.User + "@" + host.Hostname
}

func (m model) openRotation() (tea.Model, tea.Cmd) {
	selected := make(map[string]bool)
	incomplete, _ := newestIncompleteRotation()
	input := textinput.New()
	input.Prompt = "Private key path  "
	input.SetValue(defaultRotationKeyPath())
	input.CursorEnd()
	m.rotation = rotationState{phase: rotationSelectHosts, selected: selected, pathInput: input, resumeRun: incomplete}
	m.state = stateRotation
	return m, nil
}

func defaultRotationKeyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "id_ed25519_assho_"+time.Now().Format("20060102"))
}

func selectableHosts(hosts []Host) []Host {
	out := make([]Host, 0, len(hosts))
	for _, host := range hosts {
		if !host.IsContainer && host.Hostname != "" {
			out = append(out, host)
		}
	}
	return out
}

func (m model) updateRotation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	hosts := selectableHosts(m.rawHosts)
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		if m.rotation.phase == rotationRunning {
			return m, nil
		}
		if m.rotation.phase == rotationSelectHosts || m.rotation.phase == rotationSummary {
			m.state = stateList
			return m, nil
		}
		m.rotation.phase = rotationSelectHosts
		m.rotation.errorText = ""
		return m, nil
	case "q":
		if m.rotation.phase == rotationGenerateKey {
			break
		}
		if m.rotation.phase == rotationRunning {
			return m, nil
		}
		if m.rotation.phase == rotationSelectHosts || m.rotation.phase == rotationSummary {
			m.state = stateList
			return m, nil
		}
		m.rotation.phase = rotationSelectHosts
		m.rotation.errorText = ""
		return m, nil
	case "up", "k":
		if m.rotation.phase == rotationSelectHosts && len(hosts) > 0 {
			m.rotation.cursor = (m.rotation.cursor + len(hosts) - 1) % len(hosts)
		} else if m.rotation.phase == rotationChooseKey {
			m.rotation.keyChoice = (m.rotation.keyChoice + 1) % 2
		}
	case "down", "j":
		if m.rotation.phase == rotationSelectHosts && len(hosts) > 0 {
			m.rotation.cursor = (m.rotation.cursor + 1) % len(hosts)
		} else if m.rotation.phase == rotationChooseKey {
			m.rotation.keyChoice = (m.rotation.keyChoice + 1) % 2
		}
	case " ":
		if m.rotation.phase == rotationSelectHosts && len(hosts) > 0 {
			id := hosts[m.rotation.cursor].ID
			m.rotation.selected[id] = !m.rotation.selected[id]
		}
	case "a":
		if m.rotation.phase == rotationSelectHosts {
			all := len(m.rotation.selected) != len(hosts)
			m.rotation.selected = make(map[string]bool)
			if all {
				for _, host := range hosts {
					m.rotation.selected[host.ID] = true
				}
			}
		}
	case "r":
		if m.rotation.phase == rotationSelectHosts && m.rotation.resumeRun != nil {
			m.rotation.run = m.rotation.resumeRun
			m.rotation.phase = rotationRunning
			return m.startOrResumeRotation()
		}
	case "enter":
		switch m.rotation.phase {
		case rotationSelectHosts:
			if countSelected(m.rotation.selected) == 0 {
				m.rotation.errorText = "Select at least one host."
				return m, nil
			}
			m.rotation.phase = rotationChooseKey
			m.rotation.errorText = ""
		case rotationChooseKey:
			if m.rotation.keyChoice == 0 {
				m.pickerUse = pickerRotationPrivate
				m.filepicker.AllowedTypes = []string{}
				m.state = stateFilePicker
				return m, m.filepicker.Init()
			}
			m.rotation.phase = rotationGenerateKey
			return m, m.rotation.pathInput.Focus()
		case rotationGenerateKey:
			path := expandPath(strings.TrimSpace(m.rotation.pathInput.Value()))
			if path == "" {
				m.rotation.errorText = "Choose a private key path."
				return m, nil
			}
			if _, err := os.Stat(path); err == nil {
				m.rotation.errorText = "That key already exists; choose Browse or another path."
				return m, nil
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				m.rotation.errorText = err.Error()
				return m, nil
			}
			cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-a", "64", "-f", path, "-C", "assho rotation "+time.Now().Format("2006-01-02"))
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return rotationKeyReadyMsg{privateKey: path, loaded: false, err: err}
			})
		case rotationConfirm:
			if m.rotation.run == nil {
				return m, nil
			}
			private := expandPath(m.rotation.run.NewIdentity)
			public, err := publicKeyForIdentity(private)
			if err != nil {
				m.rotation.errorText = err.Error()
				return m, nil
			}
			fingerprint, err := publicKeyFingerprint(public)
			if err != nil {
				m.rotation.errorText = err.Error()
				return m, nil
			}
			if !m.rotation.agentLoaded {
				if sshAgentAvailable() {
					if !commandExists("ssh-add") {
						m.rotation.errorText = "An SSH agent is active, but ssh-add is unavailable"
						return m, nil
					}
					m.rotation.errorText = "Loading replacement key into ssh-agent… press Enter again after it succeeds."
					cmd := exec.Command("ssh-add", private)
					return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
						return rotationKeyReadyMsg{privateKey: private, loaded: true, err: err}
					})
				}
				// An agent is a convenience for passphrase-protected identities, not
				// a requirement for ordinary private keys. The strict per-host
				// verification below will safely reject an unusable identity before
				// the saved config or remote old key is touched.
				m.rotation.agentLoaded = true
				m.rotation.errorText = ""
			}
			results := make([]rotationHostResult, 0, countSelected(m.rotation.selected))
			for _, host := range hosts {
				if m.rotation.selected[host.ID] {
					results = append(results, rotationHostResult{HostID: host.ID, Alias: host.Alias, Target: sshTarget(host), Status: rotationPending, Stage: stagePreflight})
					results[len(results)-1].OldIdentity = host.IdentityFile
				}
			}
			m.rotation.run = &rotationRun{ID: time.Now().UTC().Format("20060102T150405Z") + "-" + newHostID()[:6], CreatedAt: time.Now().UTC(), NewIdentity: private, NewPublicKey: public, NewFingerprint: fingerprint, Hosts: results}
			if err := saveRotationRun(m.rotation.run); err != nil {
				m.rotation.errorText = err.Error()
				return m, nil
			}
			m.rotation.phase = rotationRunning
			return m.startOrResumeRotation()
		case rotationSummary:
			m.state = stateList
		}
	}
	if m.rotation.phase == rotationGenerateKey {
		var cmd tea.Cmd
		m.rotation.pathInput, cmd = m.rotation.pathInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func countSelected(selected map[string]bool) int {
	n := 0
	for _, value := range selected {
		if value {
			n++
		}
	}
	return n
}

func sshAgentAvailable() bool {
	socket := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
	if socket == "" {
		return false
	}
	info, err := os.Stat(socket)
	return err == nil && info.Mode()&os.ModeSocket != 0
}

func (m model) finishRotationKey(msg rotationKeyReadyMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.rotation.agentLoaded = false
		if msg.loaded {
			// This was an ssh-add attempt for an existing or freshly generated
			// identity. Keep the user on the confirmation screen with the same
			// key instead of incorrectly switching to key generation.
			m.rotation.errorText = "Could not load the selected key into ssh-agent: " + msg.err.Error()
			m.rotation.phase = rotationConfirm
		} else {
			m.rotation.errorText = "Key generation failed: " + msg.err.Error()
			m.rotation.phase = rotationGenerateKey
		}
		return m, nil
	}
	m.rotation.run = &rotationRun{NewIdentity: msg.privateKey}
	m.rotation.agentLoaded = msg.loaded
	if msg.loaded {
		m.rotation.errorText = "Replacement key loaded. Press Enter to start the staged rotation."
	} else {
		m.rotation.errorText = "Key generated. Press Enter to load it into ssh-agent."
	}
	m.rotation.phase = rotationConfirm
	return m, nil
}

func (m model) startOrResumeRotation() (tea.Model, tea.Cmd) {
	run := m.rotation.run
	if run == nil || len(run.Hosts) == 0 {
		m.rotation.errorText = "Rotation journal has no hosts."
		m.rotation.phase = rotationSummary
		return m, nil
	}
	for run.Current < len(run.Hosts) && (run.Hosts[run.Current].Status == rotationComplete || run.Hosts[run.Current].Status == rotationFailed || run.Hosts[run.Current].Status == rotationCleanupRequired) {
		run.Current++
	}
	if run.Current >= len(run.Hosts) {
		return m.finishRotationRun()
	}
	run.Hosts[run.Current].Status = rotationWorking
	if run.Hosts[run.Current].Stage == "" {
		run.Hosts[run.Current].Stage = stagePreflight
	}
	_ = saveRotationRun(run)
	return m, m.rotationCommand(run.Current, run.Hosts[run.Current].Stage)
}

func (m model) rotationCommand(index int, stage rotationStage) tea.Cmd {
	run := m.rotation.run
	if run == nil || index < 0 || index >= len(run.Hosts) {
		return nil
	}
	hostIndex := findHostIndexByID(m.rawHosts, run.Hosts[index].HostID)
	if hostIndex < 0 {
		return func() tea.Msg {
			return rotationStepMsg{hostIndex: index, stage: stage, err: errors.New("host no longer exists in config")}
		}
	}
	host := m.rawHosts[hostIndex]
	if stage == stageRemove {
		host.IdentityFile = run.Hosts[index].OldIdentity
	}
	if stage != stageUpdate {
		return checkHostTrustCmd(pendingSSHAction{
			kind:          sshActionRotation,
			host:          host,
			trustHost:     host,
			rotationIndex: index,
			rotationStage: stage,
		})
	}
	return m.rotationCommandTrusted(index, stage)
}

func (m model) rotationCommandTrusted(index int, stage rotationStage) tea.Cmd {
	run := m.rotation.run
	if run == nil || index < 0 || index >= len(run.Hosts) {
		return nil
	}
	hostIndex := findHostIndexByID(m.rawHosts, run.Hosts[index].HostID)
	if hostIndex < 0 {
		return func() tea.Msg {
			return rotationStepMsg{hostIndex: index, stage: stage, err: errors.New("host no longer exists in config")}
		}
	}
	host := m.rawHosts[hostIndex]
	if stage == stageRemove {
		host.IdentityFile = run.Hosts[index].OldIdentity
	}
	switch stage {
	case stageInstall:
		cmd, err := buildCopyIDCommand(host, run.NewPublicKey)
		if err != nil {
			return func() tea.Msg { return rotationStepMsg{hostIndex: index, stage: stage, err: err} }
		}
		return tea.ExecProcess(cmd, func(err error) tea.Msg { return rotationStepMsg{hostIndex: index, stage: stage, err: err} })
	default:
		return func() tea.Msg { return executeRotationStage(host, run, index, stage) }
	}
}

func executeRotationStage(host Host, run *rotationRun, index int, stage rotationStage) rotationStepMsg {
	msg := rotationStepMsg{hostIndex: index, stage: stage}
	switch stage {
	case stagePreflight:
		newType, newBlob, err := readPublicKey(run.NewPublicKey)
		if err != nil {
			msg.err = err
			return msg
		}
		if host.IdentityFile != "" {
			oldPublic, err := publicKeyForIdentity(host.IdentityFile)
			if err != nil {
				msg.err = fmt.Errorf("old public key unavailable: %w", err)
				return msg
			}
			oldType, oldBlob, err := readPublicKey(oldPublic)
			if err != nil {
				msg.err = err
				return msg
			}
			if oldType == newType && oldBlob == newBlob {
				msg.err = errors.New("replacement key is the same as the configured key")
				return msg
			}
			msg.oldFingerprint, _ = publicKeyFingerprint(oldPublic)
		}
		present, _, err := remoteKeyAction(host, newType, newBlob, "check", run.ID)
		if err != nil {
			msg.err = fmt.Errorf("preflight authentication failed: %w", err)
			return msg
		}
		msg.preexisting = present
	case stageVerify, stageFinal:
		msg.err = verifyWithIdentity(host, run.NewIdentity)
		if msg.err != nil && stage == stageVerify && !run.Hosts[index].NewPreexisting {
			newType, newBlob, parseErr := readPublicKey(run.NewPublicKey)
			if parseErr == nil {
				_, _, _ = remoteKeyAction(host, newType, newBlob, "remove", run.ID+"-rollback")
			}
		}
	case stageRemove:
		if host.IdentityFile == "" {
			return msg
		}
		oldPublic, err := publicKeyForIdentity(host.IdentityFile)
		if err != nil {
			msg.err = err
			return msg
		}
		keyType, blob, err := readPublicKey(oldPublic)
		if err != nil {
			msg.err = err
			return msg
		}
		_, msg.backupPath, msg.err = remoteKeyAction(host, keyType, blob, "remove", run.ID)
	}
	return msg
}

func (m model) finishRotationStep(msg rotationStepMsg) (tea.Model, tea.Cmd) {
	run := m.rotation.run
	if run == nil || msg.hostIndex != run.Current || msg.hostIndex >= len(run.Hosts) {
		return m, nil
	}
	result := &run.Hosts[msg.hostIndex]
	if msg.err != nil {
		if msg.stage == stageInstall && !result.NewPreexisting && !msg.rollbackTried {
			hostIndex := findHostIndexByID(m.rawHosts, result.HostID)
			if hostIndex >= 0 {
				host := m.rawHosts[hostIndex]
				originalErr := msg.err
				return m, func() tea.Msg {
					keyType, blob, parseErr := readPublicKey(run.NewPublicKey)
					if parseErr == nil {
						_, _, _ = remoteKeyAction(host, keyType, blob, "remove", run.ID+"-rollback")
					}
					return rotationStepMsg{hostIndex: msg.hostIndex, stage: msg.stage, err: originalErr, rollbackTried: true}
				}
			}
		}
		result.Error = msg.err.Error()
		if msg.stage == stageRemove || msg.stage == stageFinal {
			result.Status = rotationCleanupRequired
		} else {
			result.Status = rotationFailed
		}
		run.Current++
		_ = saveRotationRun(run)
		return m.startOrResumeRotation()
	}
	switch msg.stage {
	case stagePreflight:
		result.NewPreexisting = msg.preexisting
		result.OldFingerprint = msg.oldFingerprint
		result.Stage = stageInstall
	case stageInstall:
		result.Stage = stageVerify
	case stageVerify:
		result.Stage = stageUpdate
	case stageUpdate:
		hostIndex := findHostIndexByID(m.rawHosts, result.HostID)
		if hostIndex < 0 {
			result.Status = rotationFailed
			result.Error = "host no longer exists in config"
			run.Current++
			_ = saveRotationRun(run)
			return m.startOrResumeRotation()
		}
		oldIdentity := m.rawHosts[hostIndex].IdentityFile
		m.rawHosts[hostIndex].IdentityFile = run.NewIdentity
		if err := m.save(); err != nil {
			m.rawHosts[hostIndex].IdentityFile = oldIdentity
			configErr := fmt.Errorf("local config update failed: %w", err)
			if !result.NewPreexisting {
				host := m.rawHosts[hostIndex]
				return m, func() tea.Msg {
					keyType, blob, parseErr := readPublicKey(run.NewPublicKey)
					if parseErr == nil {
						_, _, _ = remoteKeyAction(host, keyType, blob, "remove", run.ID+"-rollback")
					}
					return rotationStepMsg{hostIndex: msg.hostIndex, stage: stageUpdate, err: configErr, rollbackTried: true}
				}
			}
			return m.finishRotationStep(rotationStepMsg{hostIndex: msg.hostIndex, stage: stageUpdate, err: configErr, rollbackTried: true})
		}
		result.Stage = stageRemove
	case stageRemove:
		result.BackupPath = msg.backupPath
		result.Stage = stageFinal
	case stageFinal:
		result.Status = rotationComplete
		result.Stage = stageFinal
		run.Current++
	}
	_ = saveRotationRun(run)
	if result.Stage == stageUpdate {
		return m.finishRotationStep(rotationStepMsg{hostIndex: msg.hostIndex, stage: stageUpdate})
	}
	if msg.stage == stageFinal {
		return m.startOrResumeRotation()
	}
	return m, m.rotationCommand(msg.hostIndex, result.Stage)
}

func (m model) finishRotationRun() (tea.Model, tea.Cmd) {
	now := time.Now().UTC()
	m.rotation.run.Complete = true
	m.rotation.run.CompletedAt = &now
	_ = saveRotationRun(m.rotation.run)
	_ = pruneRotationRuns(50)
	m.rotation.phase = rotationSummary
	m.list.SetItems(flattenHosts(m.rawGroups, m.rawHosts))
	return m, nil
}

func verifyWithIdentity(host Host, identity string) error {
	args := sshArgs(host, identity, true)
	args = append(args, sshTarget(host), "true")
	cmd := exec.Command("ssh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("new-key verification failed: %s", cleanCommandError(out, err))
	}
	return nil
}

func sshArgs(host Host, identity string, strictIdentity bool) []string {
	args := []string{"-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=yes"}
	if strictIdentity {
		args = append(args, "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes")
	}
	if host.Port != "" && host.Port != "22" {
		args = append(args, "-p", host.Port)
	}
	if host.ProxyJump != "" {
		args = append(args, "-J", host.ProxyJump)
	}
	if identity != "" {
		args = append(args, "-i", expandPath(identity))
	}
	return args
}

func remoteKeyAction(host Host, keyType, blob, action, runID string) (bool, string, error) {
	if !isPublicKeyType(keyType) || strings.ContainsAny(blob, "'\" ") {
		return false, "", errors.New("invalid public key tokens")
	}
	if action != "check" && action != "remove" {
		return false, "", errors.New("invalid remote key action")
	}
	script := `set -eu
auth="$HOME/.ssh/authorized_keys"
if [ ! -f "$auth" ]; then printf 'MISSING\n'; exit 0; fi
key_type='` + keyType + `'
key_blob='` + blob + `'
if [ '` + action + `' = check ]; then
  awk -v t="$key_type" -v b="$key_blob" '{for(i=1;i<NF;i++) if($i==t && $(i+1)==b) found=1} END{print found ? "PRESENT" : "MISSING"}' "$auth"
  exit 0
fi
umask 077
backup="$auth.assho-` + safeRunID(runID) + `.bak"
cp -p "$auth" "$backup"
tmp=$(mktemp "$HOME/.ssh/.authorized_keys.assho.XXXXXX")
trap 'rm -f "$tmp"' EXIT HUP INT TERM
awk -v t="$key_type" -v b="$key_blob" '{drop=0; for(i=1;i<NF;i++) if($i==t && $(i+1)==b) drop=1; if(!drop) print}' "$auth" > "$tmp"
chmod 600 "$tmp"
mv "$tmp" "$auth"
trap - EXIT HUP INT TERM
printf 'BACKUP %s\n' "$backup"
`
	args := sshArgs(host, host.IdentityFile, false)
	args = append(args, sshTarget(host), "sh", "-s")
	var cmd *exec.Cmd
	if host.Password != "" && commandExists("sshpass") {
		cmd = exec.Command("sshpass", append([]string{"-e", "ssh"}, args...)...)
		cmd.Env = append(os.Environ(), "SSHPASS="+host.Password)
	} else {
		cmd = exec.Command("ssh", args...)
	}
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, "", fmt.Errorf("remote authorized_keys operation failed: %s", cleanCommandError(out, err))
	}
	line := strings.TrimSpace(string(out))
	if strings.Contains(line, "PRESENT") {
		return true, "", nil
	}
	if idx := strings.LastIndex(line, "BACKUP "); idx >= 0 {
		return false, strings.TrimSpace(line[idx+7:]), nil
	}
	return false, "", nil
}

func cleanCommandError(out []byte, err error) string {
	message := strings.TrimSpace(string(out))
	if message == "" {
		return err.Error()
	}
	return message
}

func safeRunID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "run"
	}
	return b.String()
}

func rotationDirectory() string { return filepath.Join(filepath.Dir(getConfigPath()), "rotation-runs") }

func saveRotationRun(run *rotationRun) error {
	if run == nil || run.ID == "" {
		return errors.New("rotation run has no ID")
	}
	dir := rotationDirectory()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, safeRunID(run.ID)+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadRotationRuns() ([]rotationRun, error) {
	entries, err := os.ReadDir(rotationDirectory())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	runs := make([]rotationRun, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(rotationDirectory(), entry.Name()))
		if err != nil {
			continue
		}
		var run rotationRun
		if json.Unmarshal(data, &run) == nil {
			runs = append(runs, run)
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) })
	return runs, nil
}

func newestIncompleteRotation() (*rotationRun, error) {
	runs, err := loadRotationRuns()
	if err != nil {
		return nil, err
	}
	for i := range runs {
		if !runs[i].Complete {
			return &runs[i], nil
		}
	}
	return nil, nil
}

func pruneRotationRuns(keepComplete int) error {
	runs, err := loadRotationRuns()
	if err != nil {
		return err
	}
	complete := 0
	for _, run := range runs {
		if !run.Complete {
			continue
		}
		complete++
		if complete > keepComplete {
			if err := os.Remove(filepath.Join(rotationDirectory(), safeRunID(run.ID)+".json")); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func (m model) renderKeyInstallView() string {
	width, height := normalizedSize(m.width, m.height)
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorText).Render("INSTALL PUBLIC KEY") + "\n")
	b.WriteString(formHintStyle.Render(m.keyInstall.host.Alias+" · "+sshTarget(m.keyInstall.host)) + "\n\n")
	switch m.keyInstall.phase {
	case keyInstallChoose:
		b.WriteString("Choose the public key source:\n\n")
		options := []string{"Configured identity (.pub)", "ssh-agent / default identities", "Browse for a .pub file"}
		for i, option := range options {
			b.WriteString(selectionLine(i == m.keyInstall.choice, option) + "\n")
		}
	case keyInstallConfirm:
		b.WriteString(formSectionStyle.Render("Ready to install") + "\n\n")
		b.WriteString("Source  " + displayPath(m.keyInstall.publicKey, "ssh-agent/default") + "\n")
		b.WriteString("Key     " + m.keyInstall.fingerprint + "\n\n")
		b.WriteString(formHintStyle.Render("The remote host may prompt for its current password. No private key is uploaded.") + "\n")
	case keyInstallRunning:
		b.WriteString(m.spinner.View() + " Installing key; complete the SSH prompt if one appears…\n")
	case keyInstallDone:
		if m.keyInstall.errorText == "" {
			b.WriteString(testSuccessStyle.Render("✔ Public-key access installed."))
		} else {
			b.WriteString(testFailStyle.Render("✘ " + m.keyInstall.errorText))
		}
	}
	if m.keyInstall.errorText != "" && m.keyInstall.phase != keyInstallDone {
		b.WriteString("\n" + testFailStyle.Render("✘ "+m.keyInstall.errorText))
	}
	b.WriteString("\n\n" + helpEntry("enter", "continue") + "  " + helpEntry("esc", "back"))
	return centeredWorkspace(b.String(), width, height)
}

func (m model) renderRotationView() string {
	width, height := normalizedSize(m.width, m.height)
	inner := max(min(width-6, 100), 30)
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorText).Render("FLEET KEY ROTATION") + "\n")
	b.WriteString(formHintStyle.Render("Staged replacement · old access remains until the new key verifies") + "\n\n")
	hosts := selectableHosts(m.rawHosts)
	switch m.rotation.phase {
	case rotationSelectHosts:
		if m.rotation.resumeRun != nil {
			b.WriteString(testPendingStyle.Render("Incomplete run available — press r to resume.") + "\n\n")
		}
		for i, host := range hosts {
			mark := "[ ]"
			if m.rotation.selected[host.ID] {
				mark = "[x]"
			}
			b.WriteString(selectionLine(i == m.rotation.cursor, mark+" "+host.Alias+"  "+sshTarget(host)) + "\n")
		}
		b.WriteString("\n" + helpEntry("space", "toggle") + "  " + helpEntry("a", "all") + "  " + helpEntry("enter", "continue"))
	case rotationChooseKey:
		b.WriteString("Choose the replacement identity:\n\n")
		b.WriteString(selectionLine(m.rotation.keyChoice == 0, "Use an existing private key (not .pub)") + "\n")
		b.WriteString(selectionLine(m.rotation.keyChoice == 1, "Generate a new Ed25519 key") + "\n")
	case rotationGenerateKey:
		b.WriteString("Generate an Ed25519 replacement key:\n\n" + m.rotation.pathInput.View() + "\n")
	case rotationConfirm:
		path := ""
		if m.rotation.run != nil {
			path = m.rotation.run.NewIdentity
		}
		b.WriteString(formSectionStyle.Render("Confirm staged rotation") + "\n\n")
		b.WriteString(fmt.Sprintf("Hosts       %d\nPrivate key %s\n\n", countSelected(m.rotation.selected), path))
		b.WriteString(formHintStyle.Render("For every host: preflight → install → verify → update config → backup and remove old key → verify again."))
	case rotationRunning:
		if m.rotation.run != nil {
			b.WriteString(fmt.Sprintf("Run %s · %d/%d\n\n", m.rotation.run.ID, min(m.rotation.run.Current+1, len(m.rotation.run.Hosts)), len(m.rotation.run.Hosts)))
			for i, result := range m.rotation.run.Hosts {
				icon := "○"
				if result.Status == rotationWorking {
					icon = "◉"
				}
				if result.Status == rotationComplete {
					icon = "✔"
				}
				if result.Status == rotationFailed || result.Status == rotationCleanupRequired {
					icon = "✘"
				}
				line := fmt.Sprintf("%s %-20s %-16s %s", icon, result.Alias, result.Stage, result.Status)
				if i == m.rotation.run.Current {
					line = lipgloss.NewStyle().Foreground(colorPrimary).Render(line)
				}
				b.WriteString(line + "\n")
			}
			b.WriteString("\n" + formHintStyle.Render("Rotation is journaled. Exit is disabled while a host step is running."))
		}
	case rotationSummary:
		complete, failed, cleanup := 0, 0, 0
		if m.rotation.run != nil {
			for _, result := range m.rotation.run.Hosts {
				switch result.Status {
				case rotationComplete:
					complete++
				case rotationCleanupRequired:
					cleanup++
				default:
					failed++
				}
			}
			b.WriteString(fmt.Sprintf("Complete %d   Failed %d   Cleanup required %d\n\n", complete, failed, cleanup))
			for _, result := range m.rotation.run.Hosts {
				line := fmt.Sprintf("%-20s %s", result.Alias, result.Status)
				if result.Error != "" {
					line += " — " + result.Error
				}
				b.WriteString(ansi.Truncate(line, inner, "…") + "\n")
			}
		}
		b.WriteString("\n" + helpEntry("enter", "dashboard"))
	}
	if m.rotation.errorText != "" {
		b.WriteString("\n\n" + testFailStyle.Render("✘ "+m.rotation.errorText))
	}
	if m.rotation.phase != rotationRunning && m.rotation.phase != rotationSummary {
		b.WriteString("\n\n" + helpEntry("esc", "back"))
	}
	return centeredWorkspace(b.String(), width, height)
}

func normalizedSize(width, height int) (int, int) {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}
func displayPath(path, fallback string) string {
	if path == "" {
		return fallback
	}
	return path
}
func selectionLine(selected bool, label string) string {
	if selected {
		return lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("› " + label)
	}
	return "  " + label
}
func centeredWorkspace(content string, width, height int) string {
	panelWidth := max(min(width-6, 100), 28)
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorPrimary).Padding(1, 2).Width(panelWidth).Render(content)
	return fitViewToBounds(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel), width, height)
}
