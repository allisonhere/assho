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
- **Connection history** — press `h` to see your recently connected hosts and reconnect instantly. Last-connected time is shown inline on each host.
- **ProxyJump support** — specify a bastion/jump host per server; it's passed straight to SSH's `-J` flag.
- **Port forwarding** — configure a local tunnel per host (e.g. `5432:localhost:5432`); passed to SSH's `-L` flag automatically.
- **Docker container access** — expand any host to discover and shell into its running containers. Container lists auto-refresh every 30 seconds; press `Ctrl+D` to force an immediate re-scan.
- **Host groups** — organize servers into collapsible, reorderable groups (prod, staging, homelab, etc.).
- **Pinned hosts** — pin frequently used hosts with `p`; they float to the top of the list under a ★ Pinned header.
- **Notes** — attach a free-text note to any host (shown truncated in the list).
- **Duplicate host** — clone any host with `c` and tweak the copy, great for similar servers.
- **SSH config import** — pull hosts in from `~/.ssh/config` with `i`.
- **Non-interactive CLI** — connect, test, list, or export hosts without launching the TUI (see [CLI Usage](#cli-usage)).
- **SSH config export** — print all hosts as `~/.ssh/config` stanzas with `assho export`, so other tools (VS Code Remote, rsync, scp) can see them.
- **Fuzzy search** — type `/` and filter across all hosts and groups by alias or hostname.
- **Connection testing** — verify connectivity before saving with `Ctrl+T`.
- **Identity file picker** — browse and select SSH keys with a built-in file picker.
- **Public-key installation** — from an existing host form, press `Ctrl+K` to install its configured public key, an agent/default identity, or a separately browsed `.pub` file. Private keys never leave your machine.
- **Staged fleet key rotation** — press `K` on the dashboard to rotate selected hosts sequentially. Assho verifies the replacement key before updating local config or removing the old remote key, keeps remote backups, and journals incomplete runs for safe resume.
- **Reviewed host trust** — unknown SSH servers pause the current action and open a fingerprint review flow. OpenSSH records approved keys in `~/.ssh/known_hosts`; changed or revoked server keys are never replaced automatically.
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

`sudo make install` also installs the man page to `/usr/local/share/man/man1/`. View it with `man assho`.

## Usage

```bash
assho                    # launch the TUI
assho --help             # print usage
assho --version          # print version
```

### CLI Usage

```bash
assho list                    # print all hosts as a table
assho connect <alias>         # connect directly, no TUI
assho test <alias>            # test connectivity, exits 0/1
assho export                  # print hosts as SSH config stanzas
assho completion bash         # print bash completion script
assho completion zsh          # print zsh completion script
assho completion fish         # print fish completion script
assho help                    # print usage
```

### Shell Completions

Enable tab-completion for `assho connect` and `assho test`:

**bash** — add to `~/.bashrc`:
```bash
eval "$(assho completion bash)"
```

**zsh** — add to `~/.zshrc`:
```zsh
eval "$(assho completion zsh)"
```

**fish** — run once:
```fish
assho completion fish > ~/.config/fish/completions/assho.fish
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
| `p` | Pin / unpin host |
| `Space` | Expand/collapse host containers |
| `→` | Expand host or group (auto-scans Docker if empty) |
| `←` | Collapse host or group |
| `Ctrl+D` | Force re-scan Docker containers immediately |
| `/` | Filter / search |
| `h` | Recent connection history |
| `i` | Import hosts from `~/.ssh/config` |
| `K` | Open staged fleet key rotation |
| `Shift+↑` / `Shift+↓` | Reorder hosts / groups |
| `g` | Create group |
| `r` | Rename selected group |
| `d` / `x` | Delete group (press twice to confirm) |
| `a` | About |
| `?` | Keybinding help |
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
| `Enter` | Advance from text fields or activate the focused picker, toggle, selector, or delete action |
| `Ctrl+S` | Save from anywhere in the form |
| `Space` / `Enter` | Toggle agent forwarding when that control is focused |
| `Enter` | Open the file picker when `Browse` is focused |
| `←` / `→` | Cycle group selection |
| `Ctrl+T` | Test the connection and show its status |
| `Ctrl+K` | Install public-key access for the host being edited |
| `?` | Keybinding help |
| `Esc` | Cancel |

The form responds to both terminal width and height. Terminals at least 100 columns by 28 rows open a centered modal over the dimmed dashboard, with a two-column form and contextual rail. Medium and compact terminals switch to an inset or full-screen scrolling workspace automatically. The focused control is always kept in view. Terminals smaller than 36 columns by 12 rows show a resize notice instead of overflowing.

#### Key Rotation

Fleet rotation is intended for small-to-medium sets of saved SSH hosts. Select hosts, choose an existing private key or generate a new Ed25519 key, and confirm. For each host Assho preflights access, installs the public key, authenticates using only the replacement key, updates the saved `IdentityFile`, backs up `~/.ssh/authorized_keys`, removes the exact old key, and verifies once more. A host failure stops destructive steps for that host but does not stop later hosts.

Rotation journals are stored with mode `0600` under `~/.config/assho/rotation-runs/`; the directory uses mode `0700`. Journals contain paths, fingerprints, stages, and errors—never passwords or private-key contents. Assho manages the standard `~/.ssh/authorized_keys` file only. Hosts using `AuthorizedKeysCommand` or a nonstandard `AuthorizedKeysFile` need manual cleanup. Large fleets should use SSH certificates or configuration management instead.

The workflow requires OpenSSH tools (`ssh`, `ssh-copy-id`, and `ssh-keygen`). When an SSH agent is active, Assho uses `ssh-add` once so passphrase-protected identities can be reused across the run; without an agent, it uses the selected identity directly and the strict verification step safely rejects an unusable key before anything destructive happens. `sshpass` remains optional and is used through its environment interface when a saved password is available; otherwise the terminal presents the normal interactive SSH prompt.

#### Server host-key trust

Before any TUI SSH action, Assho checks the server against the standard user and system known-hosts files. An unknown server opens a confirmation overlay, then hands control to OpenSSH so it can display the SHA256 fingerprint and write an approved entry itself. Compare that fingerprint with the server console or another trusted source. Assho never silently accepts or replaces a changed host key; a mismatch remains a hard failure.

### Form Fields

#### Endpoint

| Field | Description |
|---|---|
| Alias | Friendly name shown in the list |
| Hostname | IP address or hostname |
| User | SSH username |
| Port | SSH port (default: 22) |

#### Authentication

| Field | Description |
|---|---|
| Key File | Path to identity file; use the `Browse` control to select a file |
| Password | Stored in your OS keychain, not in the config file |
| Fwd. Agent | Toggle SSH agent forwarding (`-A`) with `Space` or `Enter` |

#### Routing

| Field | Description |
|---|---|
| ProxyJump | Jump host in `[user@]host[:port]` format, passed to SSH's `-J` |
| LocalFwd | Port tunnel in `local:host:remote` format, passed to SSH's `-L` |

#### Details

| Field | Description |
|---|---|
| Group | Assign to an existing group or create a new one |
| Notes | Free-text note shown in the host list |

## Configuration

Sessions are stored in `~/.config/assho/hosts.json` (mode `0600`).

### Environment Variables

| Variable | Description |
|---|---|
| `ASSHO_STORE_PASSWORD` | Set to `0` or `false` to disable password persistence |
| `ASSHO_INSECURE_TEST` | Development only: set to `1` to bypass host-key verification for connection tests |

## Built With

- [Go](https://go.dev)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — styling

## License

[MIT](LICENSE) — made by [Allison](https://github.com/allisonhere)
