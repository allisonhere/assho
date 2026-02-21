package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// parseSSHConfig reads an SSH config file and extracts Host blocks into []Host.
// It skips wildcard patterns (e.g. Host *, Host 192.168.*) and Match blocks.
func parseSSHConfig(path string) ([]Host, error) {
	path = expandPath(path)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open ssh config: %w", err)
	}
	defer f.Close()

	type hostBlock struct {
		aliases  []string
		hostname string
		user     string
		port     string
		identity string
	}

	var blocks []hostBlock
	var current *hostBlock
	inMatch := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split into keyword and argument(s).
		keyword, args := splitDirective(line)
		keyword = strings.ToLower(keyword)

		if keyword == "match" {
			// End any current host block, ignore match blocks.
			if current != nil {
				blocks = append(blocks, *current)
				current = nil
			}
			inMatch = true
			continue
		}

		if keyword == "host" {
			// End previous block.
			if current != nil {
				blocks = append(blocks, *current)
				current = nil
			}
			inMatch = false

			aliases := strings.Fields(args)
			// Filter out wildcard aliases.
			var clean []string
			for _, a := range aliases {
				if !isWildcard(a) {
					clean = append(clean, a)
				}
			}
			if len(clean) == 0 {
				continue // All aliases were wildcards; skip this block.
			}
			current = &hostBlock{aliases: clean}
			continue
		}

		if inMatch || current == nil {
			continue
		}

		switch keyword {
		case "hostname":
			current.hostname = args
		case "user":
			current.user = args
		case "port":
			current.port = args
		case "identityfile":
			current.identity = args
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading ssh config: %w", err)
	}
	// Flush last block.
	if current != nil {
		blocks = append(blocks, *current)
	}

	// Convert blocks to Host entries — one per alias.
	var hosts []Host
	for _, b := range blocks {
		for _, alias := range b.aliases {
			h := Host{
				ID:           newHostID(),
				Alias:        alias,
				Hostname:     b.hostname,
				User:         b.user,
				Port:         b.port,
				IdentityFile: b.identity,
			}
			// Default hostname to alias if not set.
			if h.Hostname == "" {
				h.Hostname = alias
			}
			if h.Port == "" {
				h.Port = "22"
			}
			hosts = append(hosts, h)
		}
	}
	return hosts, nil
}

// importSSHConfig parses ~/.ssh/config and returns only hosts whose alias
// doesn't already exist in existing (case-insensitive comparison).
func importSSHConfig(existing []Host) (imported []Host, skipped int, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, 0, fmt.Errorf("cannot determine home directory: %w", err)
	}
	configPath := filepath.Join(home, ".ssh", "config")

	parsed, err := parseSSHConfig(configPath)
	if err != nil {
		return nil, 0, err
	}

	// Build lookup set of existing aliases.
	existingAliases := make(map[string]bool, len(existing))
	for _, h := range existing {
		existingAliases[strings.ToLower(strings.TrimSpace(h.Alias))] = true
	}

	for _, h := range parsed {
		key := strings.ToLower(strings.TrimSpace(h.Alias))
		if existingAliases[key] {
			skipped++
			continue
		}
		existingAliases[key] = true // prevent dupes within the import itself
		imported = append(imported, h)
	}
	return imported, skipped, nil
}

// exportSSHConfig appends all non-container, non-duplicate hosts to
// ~/.ssh/config. Returns (exported, skipped, error).
func exportSSHConfig(hosts []Host) (exported int, skipped int, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, 0, fmt.Errorf("cannot determine home directory: %w", err)
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return 0, 0, fmt.Errorf("cannot create .ssh directory: %w", err)
	}
	configPath := filepath.Join(sshDir, "config")

	// Build set of aliases already present in the SSH config.
	existingContent, _ := os.ReadFile(configPath)
	existingAliases := map[string]bool{}
	for _, line := range strings.Split(string(existingContent), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "host ") {
			continue
		}
		for _, alias := range strings.Fields(line[5:]) {
			if !isWildcard(alias) {
				existingAliases[strings.ToLower(alias)] = true
			}
		}
	}

	var entries []string
	for _, h := range hosts {
		if h.IsContainer || h.Alias == "" || h.Hostname == "" {
			continue
		}
		key := strings.ToLower(h.Alias)
		if existingAliases[key] {
			skipped++
			continue
		}
		existingAliases[key] = true // prevent dupes within this export

		var lines []string
		lines = append(lines, "Host "+h.Alias)
		lines = append(lines, "    HostName "+h.Hostname)
		if h.User != "" {
			lines = append(lines, "    User "+h.User)
		}
		if h.Port != "" && h.Port != "22" {
			lines = append(lines, "    Port "+h.Port)
		}
		if h.IdentityFile != "" {
			lines = append(lines, "    IdentityFile "+h.IdentityFile)
		}
		if h.ProxyJump != "" {
			lines = append(lines, "    ProxyJump "+h.ProxyJump)
		}
		entries = append(entries, strings.Join(lines, "\n"))
		exported++
	}

	if exported == 0 {
		return 0, skipped, nil
	}

	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return 0, skipped, fmt.Errorf("cannot open SSH config: %w", err)
	}
	defer f.Close()

	// Ensure there is a blank line before our block if the file has content.
	if len(existingContent) > 0 {
		sep := "\n"
		if !strings.HasSuffix(string(existingContent), "\n") {
			sep = "\n\n"
		}
		fmt.Fprint(f, sep)
	}
	_, err = fmt.Fprint(f, strings.Join(entries, "\n\n")+"\n")
	return exported, skipped, err
}

// splitDirective splits an SSH config line into keyword and the rest.
func splitDirective(line string) (keyword, args string) {
	// SSH config allows = or whitespace as separator.
	if idx := strings.IndexByte(line, '='); idx != -1 {
		keyword = strings.TrimSpace(line[:idx])
		args = strings.TrimSpace(line[idx+1:])
		return
	}
	parts := strings.SplitN(line, " ", 2)
	keyword = parts[0]
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return
}

// isWildcard returns true if the alias contains glob characters.
func isWildcard(alias string) bool {
	return strings.ContainsAny(alias, "*?")
}
