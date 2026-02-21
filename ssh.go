package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type scanDockerMsg struct {
	hostIndex  int
	containers []Host
	err        error
}

type testConnectionMsg struct {
	err error
}

func testConnection(h Host) tea.Cmd {
	return func() tea.Msg {
		if h.Hostname == "" {
			return testConnectionMsg{err: fmt.Errorf("hostname required")}
		}
		port := h.Port
		if port == "" {
			port = "22"
		}
		user := h.User
		if user == "" {
			user = os.Getenv("USER")
			if user == "" {
				return testConnectionMsg{err: fmt.Errorf("user required")}
			}
		}

		args := []string{
			"-o", "ConnectTimeout=5",
			"-o", "NumberOfPasswordPrompts=1",
			"-o", "PreferredAuthentications=publickey,password,keyboard-interactive",
		}
		if allowInsecureTest() {
			args = append(args, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null")
		} else {
			args = append(args, "-o", "StrictHostKeyChecking=yes")
		}
		if user != "" {
			args = append(args, "-l", user)
		}
		if port != "" {
			args = append(args, "-p", port)
		}
		if h.IdentityFile != "" {
			args = append(args, "-i", expandPath(h.IdentityFile))
		}
		if h.ProxyJump != "" {
			args = append(args, "-J", h.ProxyJump)
		}
		args = append(args, h.Hostname, "exit")

		binary := "ssh"
		cmdArgs := args
		// Prefer key-based auth when an identity file is configured.
		// Only force sshpass when password is set and no key file is provided.
		if h.Password != "" && strings.TrimSpace(h.IdentityFile) == "" {
			sshpassPath, err := exec.LookPath("sshpass")
			if err != nil {
				return testConnectionMsg{err: fmt.Errorf("password provided but sshpass not installed")}
			}
			binary = sshpassPath
			cmdArgs = append([]string{"-p", h.Password, "ssh"}, args...)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, cmdArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return testConnectionMsg{err: fmt.Errorf("connection test timed out")}
			}
			out := strings.TrimSpace(string(output))
			if out == "" {
				out = err.Error()
			}
			return testConnectionMsg{err: fmt.Errorf("%s", out)}
		}
		return testConnectionMsg{err: nil}
	}
}

func scanDockerContainers(h Host, index int) tea.Cmd {
	return func() tea.Msg {
		// Run ssh command to get docker containers
		// docker ps --format "{{.ID}}|{{.Names}}|{{.Image}}"
		cmdStr := `docker ps --format "{{.ID}}|{{.Names}}|{{.Image}}"`

		args := []string{
			"-o", "BatchMode=yes",
			"-o", "ConnectTimeout=5",
		}
		args = append(args, h.Hostname)
		if h.User != "" {
			args = append([]string{"-l", h.User}, args...)
		}
		if h.Port != "" {
			args = append([]string{"-p", h.Port}, args...)
		}
		if h.IdentityFile != "" {
			args = append([]string{"-i", expandPath(h.IdentityFile)}, args...)
		}
		// If password exists, use sshpass?
		// For simplicity, we assume key-based or agent for scanning to avoid hanging
		// Or we can use the same sshpass logic if available

		finalCmd := "ssh"
		sshArgs := append(args, cmdStr)

		if h.Password != "" {
			sshpassPath, err := exec.LookPath("sshpass")
			if err == nil {
				finalBinary := sshpassPath
				newArgs := []string{"-p", h.Password, "ssh"}
				sshArgs = append(newArgs, sshArgs...)
				finalCmd = finalBinary
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, finalCmd, sshArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return scanDockerMsg{hostIndex: index, err: fmt.Errorf("scan timed out")}
			}
			return scanDockerMsg{hostIndex: index, err: fmt.Errorf("scan failed: %v", err)}
		}

		var containers []Host
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) >= 2 {
				id := parts[0]
				name := parts[1]
				// image := parts[2]
				containers = append(containers, Host{
					ID:          newHostID(),
					Alias:       name,
					Hostname:    id,     // Use ID as "hostname" for exec
					User:        "root", // Default to root inside container
					IsContainer: true,
					ParentID:    h.ID,
				})
			}
		}
		return scanDockerMsg{hostIndex: index, containers: containers}
	}
}

func buildSSHArgs(h Host, forceTTY bool, remoteCmd string) []string {
	args := []string{}
	if forceTTY {
		args = append(args, "-t")
	}
	if h.User != "" {
		args = append(args, "-l", h.User)
	}
	if h.Port != "" {
		args = append(args, "-p", h.Port)
	}
	if h.IdentityFile != "" {
		args = append(args, "-i", expandPath(h.IdentityFile))
	}
	if h.ProxyJump != "" {
		args = append(args, "-J", h.ProxyJump)
	}
	args = append(args, h.Hostname)
	if remoteCmd != "" {
		args = append(args, remoteCmd)
	}
	return args
}

func buildSSHCommand(password string, sshArgs []string) (string, []string, bool) {
	if password == "" {
		return "ssh", sshArgs, true
	}
	sshpassPath, err := exec.LookPath("sshpass")
	if err != nil {
		return "ssh", sshArgs, false
	}
	return sshpassPath, append([]string{"-p", password, "ssh"}, sshArgs...), true
}

func formatTestStatus(err error) (string, bool) {
	if err == nil {
		return "Connection successful", true
	}
	msg := err.Error()
	if strings.Contains(msg, "REMOTE HOST IDENTIFICATION HAS CHANGED") {
		return "Host key mismatch in ~/.ssh/known_hosts. Refusing to connect.", false
	}
	if strings.Contains(msg, "REVOKED HOST KEY") {
		return "Host key is revoked in ~/.ssh/known_hosts.", false
	}
	if strings.Contains(msg, "Host key verification failed") ||
		strings.Contains(msg, "authenticity of host") ||
		strings.Contains(msg, "No RSA host key is known") {
		return "Host key is unknown. Run `ssh <host>` once or set ASSHO_INSECURE_TEST=1 to bypass for testing.", false
	}
	return msg, false
}
