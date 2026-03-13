package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// parseSSHConfig reads an SSH config file and extracts Host blocks into []Host.

// parseSSHConfig reads an SSH config file and extracts Host blocks into []Host.
// It skips wildcard patterns (e.g. Host *, Host 192.168.*) and Match blocks.
// Include directives are followed recursively.
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
	var included []Host // hosts resolved from Include directives
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

		if keyword == "include" {
			// Flush the current host block before following the include.
			if current != nil {
				blocks = append(blocks, *current)
				current = nil
			}
			inMatch = false
			// Resolve path: relative paths are relative to ~/.ssh/.
			pattern := expandPath(args)
			if !filepath.IsAbs(pattern) {
				home, homeErr := os.UserHomeDir()
				if homeErr == nil {
					pattern = filepath.Join(home, ".ssh", pattern)
				}
			}
			matches, _ := filepath.Glob(pattern)
			for _, p := range matches {
				sub, subErr := parseSSHConfig(p)
				if subErr == nil {
					included = append(included, sub...)
				}
			}
			continue
		}

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
	return append(included, hosts...), nil
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
