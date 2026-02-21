<p align="center">
  <img src="screenshot.png" alt="Assho — Another SSH Organizer" width="700">
</p>

<h1 align="center">Assho</h1>

<p align="center">
  <strong>A</strong>nother <strong>SSH</strong> <strong>O</strong>rganizer<br>
  <em>A fast, beautiful TUI for managing and connecting to your SSH hosts.</em>
</p>

<p align="center">
  <a href="https://github.com/allisonhere/assho/releases"><img src="https://img.shields.io/github/v/release/allisonhere/assho?style=flat-square&color=7C3AED" alt="Release"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/license-MIT-06B6D4?style=flat-square" alt="License"></a>
  <a href="https://github.com/allisonhere/assho"><img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS-F59E0B?style=flat-square" alt="Platform"></a>
</p>

---

Stop typing `ssh root@192.168.1.47 -p 2222 -i ~/.ssh/id_rsa` from memory. Assho gives you a searchable, organized dashboard for all your servers — with one-key connect.

## Features

- **Instant connect** — select a host and hit Enter. SSH hands off immediately; the TUI exits cleanly.
- **Connection history** — press `h` to see your recently connected hosts and reconnect instantly.
- **ProxyJump support** — specify a bastion/jump host per server; it's passed straight to SSH's `-J` flag.
- **Docker container access** — expand any host to discover and shell into its running containers.
- **Host groups** — organize servers into collapsible, reorderable groups (prod, staging, homelab, etc.).
- **Duplicate host** — clone any host with `c` and tweak the copy, great for similar servers.
- **SSH config import/export** — pull hosts in from `~/.ssh/config` with `i`, push them back out with `Ctrl+E`.
- **Fuzzy search** — type `/` and filter across all hosts and groups by alias or hostname.
- **Connection testing** — verify connectivity before saving with `Ctrl+T`.
- **Identity file picker** — browse and select SSH keys with a built-in file picker.
- **Secure password storage** — passwords stored in your OS keychain (macOS Keychain / Linux `secret-tool`), never in plaintext.
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

> Make sure `~/go/bin` is in your `PATH`.

### From Source

```bash
git clone https://github.com/allisonhere/assho.git
cd assho
sudo make install
```

## Usage

```bash
assho            # launch the TUI
assho --version  # print version
```

### Keybindings

#### Dashboard

| Key | Action |
|---|---|
| `Enter` | Connect to selected host |
| `n` | New host |
| `e` | Edit selected host |
| `c` | Duplicate selected host |
| `d` | Delete (press twice to confirm) |
| `Space` | Expand/collapse host containers |
| `→` | Expand host or group (auto-scans Docker if empty) |
| `←` | Collapse host or group |
| `Ctrl+D` | Force-scan Docker containers |
| `/` | Filter / search |
| `h` | Recent connection history |
| `i` | Import hosts from `~/.ssh/config` |
| `Ctrl+E` | Export hosts to `~/.ssh/config` |
| `Shift+↑` / `Shift+↓` | Reorder hosts / groups |
| `g` | Create group |
| `r` | Rename selected group |
| `d` / `x` | Delete group (press twice to confirm) |
| `a` | About |
| `q` | Quit |

#### History

| Key | Action |
|---|---|
| `Enter` | Connect |
| `e` | Edit host |
| `h` / `Esc` / `q` | Back to dashboard |

#### Add / Edit Form

| Key | Action |
|---|---|
| `Tab` / `↓` | Next field |
| `Shift+Tab` / `↑` | Previous field |
| `Enter` | Advance to next field, or save on the last field |
| `Enter` | Open file picker (when Key File field is focused) |
| `←` / `→` | Cycle group selection |
| `Ctrl+T` | Test connection |
| `Esc` | Cancel |

### Form Fields

| Field | Description |
|---|---|
| Alias | Friendly name shown in the list |
| Hostname | IP address or hostname |
| User | SSH username |
| Port | SSH port (default: 22) |
| ProxyJump | Jump host in `[user@]host[:port]` format, passed to SSH's `-J` |
| Key File | Path to identity file; use the `Pick` button to browse |
| Password | Stored in your OS keychain, not in the config file |
| Group | Assign to an existing group or create a new one |

## Configuration

Sessions are stored in `~/.config/assho/hosts.json` (mode `0600`).

### Environment Variables

| Variable | Description |
|---|---|
| `ASSHO_STORE_PASSWORD` | Set to `0` or `false` to disable password persistence |
| `ASSHO_INSECURE_TEST` | Set to `1` to skip host key verification during connection tests |

## Built With

- [Go](https://go.dev)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — styling

## License

[MIT](LICENSE) — made by [Allison](https://github.com/allisonhere)
