package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var version = "dev"

func findHostByAlias(hosts []Host, alias string) *Host {
	lower := strings.ToLower(alias)
	for i := range hosts {
		if strings.ToLower(hosts[i].Alias) == lower {
			return &hosts[i]
		}
		for j := range hosts[i].Containers {
			if strings.ToLower(hosts[i].Containers[j].Alias) == lower {
				return &hosts[i].Containers[j]
			}
		}
	}
	return nil
}

func cliList() {
	_, hosts, _, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%-20s %-30s %-6s %-16s %s\n", "ALIAS", "HOST", "PORT", "USER", "NOTES")
	fmt.Println(strings.Repeat("-", 80))
	for _, h := range hosts {
		if h.IsContainer {
			continue
		}
		port := h.Port
		if port == "" {
			port = "22"
		}
		notes := h.Notes
		if len(notes) > 30 {
			notes = notes[:29] + "…"
		}
		fmt.Printf("%-20s %-30s %-6s %-16s %s\n", h.Alias, h.Hostname, port, h.User, notes)
	}
}

func cliConnect(alias string) {
	_, hosts, _, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}
	h := findHostByAlias(hosts, alias)
	if h == nil {
		fmt.Fprintf(os.Stderr, "host not found: %s\n", alias)
		os.Exit(1)
	}
	sshArgs := buildSSHArgs(*h, false, "")
	binary, args, extraEnv, ok := buildSSHCommand(h.Password, sshArgs)
	if h.Password != "" && !ok {
		fmt.Fprintln(os.Stderr, "warning: password set but sshpass not found")
	}
	finalBinaryPath, lookErr := exec.LookPath(binary)
	if lookErr != nil {
		finalBinaryPath = binary
	}
	env := append(os.Environ(), extraEnv...)
	argv := append([]string{binary}, args...)
	if err := syscall.Exec(finalBinaryPath, argv, env); err != nil {
		fmt.Fprintf(os.Stderr, "failed to exec SSH: %v\n", err)
		os.Exit(1)
	}
}

func cliTest(alias string) {
	_, hosts, _, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}
	h := findHostByAlias(hosts, alias)
	if h == nil {
		fmt.Fprintf(os.Stderr, "host not found: %s\n", alias)
		os.Exit(1)
	}
	cmd := testConnection(*h)
	msg := cmd().(testConnectionMsg)
	status, success := formatTestStatus(msg.err)
	if success {
		fmt.Println("✔ " + status)
		os.Exit(0)
	} else {
		fmt.Fprintln(os.Stderr, "✘ "+status)
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Println("assho " + version)
			return
		case "list":
			cliList()
			return
		case "connect":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: assho connect <alias>")
				os.Exit(1)
			}
			cliConnect(os.Args[2])
			return
		case "test":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: assho test <alias>")
				os.Exit(1)
			}
			cliTest(os.Args[2])
			return
		}
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}

	// Exec SSH after TUI cleanup
	if finalModel, ok := m.(model); ok && finalModel.sshToRun != nil {
		h := finalModel.sshToRun

		connectStyle := lipgloss.NewStyle().Foreground(colorSecondary).Bold(true)
		hostStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
		fmt.Printf("\n %s %s\n\n", connectStyle.Render("→ Connecting to"), hostStyle.Render(h.Alias))

		var sshArgs []string
		var password string
		if h.IsContainer {
			if h.ParentID == "" {
				fmt.Println("Error: container missing parent host reference.")
				return
			}
			parentIdx := findHostIndexByID(finalModel.rawHosts, h.ParentID)
			if parentIdx == -1 {
				fmt.Println("Error: parent host not found for container.")
				return
			}
			parent := finalModel.rawHosts[parentIdx]
			dockerCmd := fmt.Sprintf("docker exec -it %s sh -c 'command -v bash >/dev/null 2>&1 && exec bash || exec sh'", h.Hostname)
			sshArgs = buildSSHArgs(parent, true, dockerCmd)
			password = parent.Password
		} else {
			sshArgs = buildSSHArgs(*h, false, "")
			password = h.Password
		}

		binary, args, extraEnv, ok := buildSSHCommand(password, sshArgs)
		if password != "" && !ok {
			fmt.Println("Warning: Password provided but 'sshpass' not found.")
		}

		finalBinaryPath, lookErr := exec.LookPath(binary)
		if lookErr != nil {
			finalBinaryPath = binary
		}

		env := append(os.Environ(), extraEnv...)
		argv := append([]string{binary}, args...)

		if err := syscall.Exec(finalBinaryPath, argv, env); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to exec SSH: %v\n", err)
			os.Exit(1)
		}
	}
}
