package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var version = "dev"

const cliHelp = `assho — Another SSH Organizer

USAGE
  assho                         launch the TUI
  assho <command> [args]        run a CLI command

COMMANDS
  connect <alias>               connect directly to a host, no TUI
  test <alias>                  test SSH connectivity; exits 0 on success
  list                          print all hosts as a table
  completion <bash|zsh|fish>    print shell completion script

OPTIONS
  --version, -v                 print version and exit
  --help, -h                    show this help

SHELL COMPLETIONS
  bash    eval "$(assho completion bash)"
  zsh     eval "$(assho completion zsh)"
  fish    assho completion fish > ~/.config/fish/completions/assho.fish
`

type resolvedAliasTarget struct {
	host   Host
	parent *Host
}

func findHostByAlias(hosts []Host, alias string) *Host {
	target, err := resolveAliasForCLITest(hosts, alias)
	if err != nil {
		return nil
	}
	return &target.host
}

func resolveAliasForCLITest(hosts []Host, alias string) (*resolvedAliasTarget, error) {
	lower := strings.ToLower(strings.TrimSpace(alias))
	if lower == "" {
		return nil, fmt.Errorf("host not found: %s", alias)
	}

	var hostMatches []Host
	var containerMatches []resolvedAliasTarget
	for i := range hosts {
		if strings.ToLower(hosts[i].Alias) == lower {
			hostMatches = append(hostMatches, hosts[i])
		}
		for j := range hosts[i].Containers {
			if strings.ToLower(hosts[i].Containers[j].Alias) == lower {
				parent := hosts[i]
				containerMatches = append(containerMatches, resolvedAliasTarget{
					host:   hosts[i].Containers[j],
					parent: &parent,
				})
			}
		}
	}

	switch {
	case len(hostMatches) == 1:
		return &resolvedAliasTarget{host: hostMatches[0]}, nil
	case len(hostMatches) > 1:
		return nil, fmt.Errorf("alias %q is ambiguous across multiple hosts", alias)
	case len(containerMatches) == 1:
		return &containerMatches[0], nil
	case len(containerMatches) > 1:
		return nil, fmt.Errorf("alias %q is ambiguous across multiple containers", alias)
	default:
		return nil, fmt.Errorf("host not found: %s", alias)
	}
}

func fprintCLIList(w io.Writer, hosts []Host) {
	fmt.Fprintf(w, "%-20s %-30s %-6s %-16s %s\n", "ALIAS", "HOST", "PORT", "USER", "NOTES")
	fmt.Fprintln(w, strings.Repeat("-", 80))
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
		fmt.Fprintf(w, "%-20s %-30s %-6s %-16s %s\n", h.Alias, h.Hostname, port, h.User, notes)
	}
}

func cliList() {
	_, hosts, _, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}
	fprintCLIList(os.Stdout, hosts)
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
	target, err := resolveAliasForCLITest(hosts, alias)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var testErr error
	if target.host.IsContainer {
		if target.parent == nil {
			testErr = fmt.Errorf("container %q is missing its parent host reference", target.host.Alias)
		} else {
			testErr = runSSHTest(*target.parent, fmt.Sprintf("docker exec %s sh -c 'exit'", target.host.Hostname))
		}
	} else {
		testErr = runSSHTest(target.host, "exit")
	}
	status, success := formatTestStatus(testErr)
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
		case "--help", "-h", "help":
			fmt.Print(cliHelp)
			return
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
		case "_aliases":
			_, hosts, _, err := loadConfig()
			if err != nil {
				os.Exit(1)
			}
			fprintAliases(os.Stdout, hosts)
			return
		case "completion":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: assho completion [bash|zsh|fish]")
				os.Exit(1)
			}
			switch os.Args[2] {
			case "bash":
				fmt.Println(bashCompletion)
			case "zsh":
				fmt.Println(zshCompletion)
			case "fish":
				fmt.Println(fishCompletion)
			default:
				fmt.Fprintf(os.Stderr, "unknown shell %q; supported: bash, zsh, fish\n", os.Args[2])
				os.Exit(1)
			}
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
