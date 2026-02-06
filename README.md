# Asshi (A Simple SSH Interface)

Asshi is a terminal-based SSH session manager built with Go and Bubble Tea. It allows you to save, manage, and quickly connect to your SSH hosts without remembering IP addresses or flags.

## Installation

### Option 1: One-Line Install (Recommended)
Installs to `/usr/local/bin` and runs immediately:
```bash
curl -sL https://raw.githubusercontent.com/allisonhere/asshi/main/install.sh | bash
```

### Option 2: Go Install
```bash
go install github.com/allisonhere/asshi@latest
```
*Make sure `~/go/bin` is in your `PATH`.*

### Option 2: Install from Source (System-wide)
To install `asshi` to `/usr/local/bin` so it's available everywhere:

```bash
git clone https://github.com/allisonhere/asshi.git
cd asshi
sudo make install
```

### Option 3: Manual Build (Cross-Platform)
You can build the binary for your specific OS:

```bash
# Linux / macOS
go build -o asshi main.go

# Windows
GOOS=windows GOARCH=amd64 go build -o asshi.exe main.go
```

## Usage

Run the tool:
```bash
asshi
```

### Controls

**Dashboard (List View):**
- `n`: **New** session.
- `e`: **Edit** selected session.
- `d`: **Delete** selected session.
- `Enter`: **Connect** to selected session.
- `/`: **Filter** list (search).
- `q`: **Quit**.

**Form (Add/Edit):**
- `Tab` / `Down`: Next field.
- `Shift+Tab` / `Up`: Previous field.
- `Ctrl+t`: **Test Connection** (tries to connect and exit).
- `Ctrl+f`: **File Picker** (when on Identity File field).
- `Enter`: Save (if on last field) or Next field.
- `Esc`: Cancel.

## Configuration

Sessions are stored in `~/.config/asshi/hosts.json`.
