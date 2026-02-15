# Assho

**A**nother **SSH** **O**rganizer — a fast, beautiful TUI for managing and connecting to your SSH hosts.

![Assho dashboard screenshot](screenshot.png)

Stop typing `ssh root@192.168.1.47 -p 2222 -i ~/.ssh/id_rsa` from memory. Assho gives you a searchable, organized dashboard for all your servers — with one-key connect.

## Features

- **Instant connect** — select a host and hit Enter. That's it.
- **Connection history** — press `h` to see your 5 most recent connections and reconnect instantly.
- **Docker container access** — expand any host to discover and shell into its running containers (`Ctrl+D` to scan, `Enter` to exec).
- **Host groups** — organize servers into collapsible groups (prod, staging, homelab, etc).
- **SSH config import** — pull in hosts from your existing `~/.ssh/config` with one keypress (`i`).
- **Fuzzy search** — type `/` and filter across all hosts by alias or hostname.
- **Connection testing** — verify connectivity before saving with `Ctrl+T` in the edit form.
- **Identity file picker** — browse and select SSH keys with a built-in file picker.
- **Secure password storage** — passwords are stored in your OS keychain (macOS Keychain / Linux `secret-tool`), never in plaintext.
- **Cross-platform** — Linux (amd64/arm64) and macOS (Intel/Apple Silicon).

## Installation

### One-Line Install (Recommended)
```bash
curl -sL https://raw.githubusercontent.com/allisonhere/assho/main/install.sh | bash
```

### Go Install
```bash
go install github.com/allisonhere/assho@latest
```
*Make sure `~/go/bin` is in your `PATH`.*

### From Source
```bash
git clone https://github.com/allisonhere/assho.git
cd assho
sudo make install
```

## Usage

```bash
assho
```

### Keybindings

**Dashboard:**

| Key | Action |
|-----|--------|
| `Enter` | Connect to selected host |
| `n` | New host |
| `e` | Edit selected host |
| `d` | Delete selected host (press twice to confirm) |
| `/` | Filter / search |
| `h` | Recent connection history |
| `Space` | Expand/collapse host containers |
| `Ctrl+D` | Scan for Docker containers |
| `i` | Import from `~/.ssh/config` |
| `g` | Create group |
| `r` | Rename group |
| `q` | Quit |

**History:**

| Key | Action |
|-----|--------|
| `Enter` | Connect |
| `e` | Edit host |
| `h` / `Esc` | Back to dashboard |

**Add/Edit Form:**

| Key | Action |
|-----|--------|
| `Tab` / `Down` | Next field |
| `Shift+Tab` / `Up` | Previous field |
| `Enter` | Save (on last field) or next field |
| `Ctrl+T` | Test connection |
| `Left` / `Right` | Cycle group selection |
| `Esc` | Cancel |

## Configuration

Sessions are stored in `~/.config/assho/hosts.json`.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ASSHO_STORE_PASSWORD` | Set to `0` or `false` to disable password persistence |
| `ASSHO_INSECURE_TEST` | Set to `1` to skip host key verification during connection tests |
