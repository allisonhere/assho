package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// --- Data Models ---

type Host struct {
	ID           string `json:"id"`
	Alias        string `json:"alias"`
	Hostname     string `json:"hostname"`
	User         string `json:"user"`
	Port         string `json:"port"`
	IdentityFile string `json:"identity_file,omitempty"`
	Password     string `json:"password,omitempty"`
	PasswordRef  string `json:"password_ref,omitempty"`
	GroupID      string `json:"group_id,omitempty"`

	// Docker Support
	Containers  []Host `json:"containers,omitempty"` // Nested hosts (containers)
	IsContainer bool   `json:"is_container,omitempty"`
	Expanded    bool   `json:"-"` // UI State
	ParentID    string `json:"-"` // Reference to parent (SSH host)
	ListIndent  int    `json:"-"` // UI indent level for tree rendering
}

type Group struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Expanded bool   `json:"expanded,omitempty"`
}

type groupItem struct {
	Group
}

func (g groupItem) FilterValue() string { return g.Name }
func (g groupItem) Title() string       { return g.Name }
func (g groupItem) Description() string { return "group" }

// FilterValue implements list.Item
func (h Host) FilterValue() string { return h.Alias + " " + h.Hostname }
func (h Host) Title() string {
	if h.IsContainer {
		return "  🐳 " + h.Alias
	}
	prefix := "▶ "
	if h.Expanded {
		prefix = "▼ "
	}
	return prefix + h.Alias
}
func (h Host) Description() string {
	if h.IsContainer {
		return fmt.Sprintf("Container: %s", h.Hostname)
	}
	desc := fmt.Sprintf("%s@%s", h.User, h.Hostname)
	if h.Port != "" && h.Port != "22" {
		desc += fmt.Sprintf(":%s", h.Port)
	}
	return desc
}

// --- Config Management ---

func getConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "hosts.json"
	}
	return filepath.Join(home, ".config", "assho", "hosts.json")
}

func getLegacyConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "hosts.json"
	}
	return filepath.Join(home, ".config", "asshi", "hosts.json")
}

func shouldPersistPassword() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ASSHO_STORE_PASSWORD")))
	if value == "" {
		// Backward compatibility with old env name.
		value = strings.ToLower(strings.TrimSpace(os.Getenv("ASSHI_STORE_PASSWORD")))
	}
	if value == "" {
		return true
	}
	return value != "0" && value != "false" && value != "no"
}

func allowInsecureTest() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ASSHO_INSECURE_TEST")))
	if value == "" {
		// Backward compatibility with old env name.
		value = strings.ToLower(strings.TrimSpace(os.Getenv("ASSHI_INSECURE_TEST")))
	}
	return value == "1" || value == "true" || value == "yes"
}

const (
	configVersion     = 3
	secretServiceName = "assho"
	maxHistoryEntries = 50
)

type HistoryEntry struct {
	HostID    string `json:"host_id"`
	Alias     string `json:"alias"`
	Timestamp int64  `json:"timestamp"`
}

func recordHistory(hostID, alias string, history []HistoryEntry) []HistoryEntry {
	entry := HistoryEntry{
		HostID:    hostID,
		Alias:     alias,
		Timestamp: time.Now().Unix(),
	}
	// Deduplicate by host ID (remove old entry for same host).
	filtered := []HistoryEntry{entry}
	for _, h := range history {
		if h.HostID != hostID {
			filtered = append(filtered, h)
		}
	}
	if len(filtered) > maxHistoryEntries {
		filtered = filtered[:maxHistoryEntries]
	}
	return filtered
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func newHostID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func expandPath(path string) string {
	if path == "" {
		return path
	}
	path = os.ExpandEnv(path)
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			if strings.HasPrefix(path, "~/") {
				return filepath.Join(home, path[2:])
			}
		}
	}
	return path
}

// --- Keychain ---

func storePasswordSecret(ref, password string) error {
	if ref == "" || password == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch runtime.GOOS {
	case "darwin":
		cmd := exec.CommandContext(ctx, "security", "add-generic-password", "-U", "-a", ref, "-s", secretServiceName, "-w", password)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("security store failed: %v (%s)", err, strings.TrimSpace(string(output)))
		}
		return nil
	case "linux":
		if !commandExists("secret-tool") {
			return fmt.Errorf("secret-tool not installed")
		}
		cmd := exec.CommandContext(ctx, "secret-tool", "store", "--label=assho password", "service", secretServiceName, "account", ref)
		cmd.Stdin = strings.NewReader(password)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("secret-tool store failed: %v (%s)", err, strings.TrimSpace(string(output)))
		}
		return nil
	default:
		return fmt.Errorf("keychain backend unsupported on %s", runtime.GOOS)
	}
}

func lookupPasswordSecret(ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch runtime.GOOS {
	case "darwin":
		cmd := exec.CommandContext(ctx, "security", "find-generic-password", "-a", ref, "-s", secretServiceName, "-w")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(output)), nil
	case "linux":
		if !commandExists("secret-tool") {
			return "", fmt.Errorf("secret-tool not installed")
		}
		cmd := exec.CommandContext(ctx, "secret-tool", "lookup", "service", secretServiceName, "account", ref)
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(output)), nil
	default:
		return "", fmt.Errorf("keychain backend unsupported on %s", runtime.GOOS)
	}
}

// --- Host/Group Helpers ---

func sanitizeHostsForSave(hosts []Host) []Host {
	sanitized := make([]Host, len(hosts))
	for i, h := range hosts {
		sanitized[i] = h
		if !shouldPersistPassword() {
			sanitized[i].Password = ""
			sanitized[i].PasswordRef = ""
		} else if sanitized[i].Password != "" {
			// Prefer keychain storage; fall back to plaintext if unavailable.
			if err := storePasswordSecret(sanitized[i].ID, sanitized[i].Password); err == nil {
				sanitized[i].PasswordRef = sanitized[i].ID
				sanitized[i].Password = ""
			}
		}
		if len(h.Containers) > 0 {
			sanitized[i].Containers = sanitizeHostsForSave(h.Containers)
		}
	}
	return sanitized
}

func ensureHostIDs(hosts []Host) ([]Host, bool) {
	changed := false
	for i := range hosts {
		if hosts[i].ID == "" {
			hosts[i].ID = newHostID()
			changed = true
		}
		if len(hosts[i].Containers) > 0 {
			var childChanged bool
			hosts[i].Containers, childChanged = ensureHostIDs(hosts[i].Containers)
			if childChanged {
				changed = true
			}
		}
	}
	return hosts, changed
}

func ensureGroupIDs(groups []Group) ([]Group, bool) {
	changed := false
	for i := range groups {
		if groups[i].ID == "" {
			groups[i].ID = newHostID()
			changed = true
		}
	}
	return groups, changed
}

func hydrateHostPasswords(hosts []Host) []Host {
	for i := range hosts {
		if hosts[i].Password == "" && hosts[i].PasswordRef != "" {
			if secret, err := lookupPasswordSecret(hosts[i].PasswordRef); err == nil {
				hosts[i].Password = secret
			}
		}
		if len(hosts[i].Containers) > 0 {
			hosts[i].Containers = hydrateHostPasswords(hosts[i].Containers)
		}
	}
	return hosts
}

// --- Config I/O ---

type configFile struct {
	Version int            `json:"version"`
	Groups  []Group        `json:"groups,omitempty"`
	Hosts   []Host         `json:"hosts,omitempty"`
	History []HistoryEntry `json:"history,omitempty"`
}

func loadConfig() ([]Group, []Host, []HistoryEntry, error) {
	path := getConfigPath()
	loadedFromLegacy := false
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			legacyPath := getLegacyConfigPath()
			if legacyPath != path {
				if legacyFile, legacyErr := os.Open(legacyPath); legacyErr == nil {
					f = legacyFile
					loadedFromLegacy = true
				} else if !os.IsNotExist(legacyErr) {
					return []Group{}, []Host{}, nil, legacyErr
				}
			}
			if f == nil {
				// Return default/example data if no config exists
				return []Group{}, []Host{
					{ID: newHostID(), Alias: "Localhost", Hostname: "127.0.0.1", User: "root", Port: "22"},
				}, nil, nil
			}
		} else {
			return []Group{}, []Host{}, nil, err
		}
	}
	defer f.Close()

	bytes, readErr := io.ReadAll(f)
	if readErr != nil {
		return []Group{}, []Host{}, nil, readErr
	}

	var cfg configFile
	if err := json.Unmarshal(bytes, &cfg); err == nil {
		if cfg.Version > 0 || len(cfg.Hosts) > 0 || len(cfg.Groups) > 0 {
			if cfg.Version == 0 {
				cfg.Version = 1
			}
			if loadedFromLegacy {
				if err := saveConfig(cfg.Groups, cfg.Hosts, cfg.History); err != nil {
					return cfg.Groups, hydrateHostPasswords(cfg.Hosts), cfg.History, fmt.Errorf("migrated legacy config but failed to persist new path: %w", err)
				}
			}
			return cfg.Groups, hydrateHostPasswords(cfg.Hosts), cfg.History, nil
		}
	}

	// Backward compatibility with old hosts-only format.
	var hosts []Host
	if err := json.Unmarshal(bytes, &hosts); err == nil {
		if loadedFromLegacy {
			if err := saveConfig([]Group{}, hosts, nil); err != nil {
				return []Group{}, hydrateHostPasswords(hosts), nil, fmt.Errorf("migrated legacy hosts but failed to persist new path: %w", err)
			}
		}
		return []Group{}, hydrateHostPasswords(hosts), nil, nil
	}
	return []Group{}, []Host{}, nil, fmt.Errorf("invalid config format")
}

func saveConfig(groups []Group, hosts []Host, history []HistoryEntry) error {
	path := getConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sanitizedHosts := sanitizeHostsForSave(hosts)
	cfg := configFile{
		Version: configVersion,
		Groups:  groups,
		Hosts:   sanitizedHosts,
		History: history,
	}
	bytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if _, err := f.Write(bytes); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
