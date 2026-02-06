# Asshi (A Simple SSH Interface)

Asshi is a terminal-based SSH session manager built with Go and Bubble Tea. It allows you to save, manage, and quickly connect to your SSH hosts without remembering IP addresses or flags.

## Installation

```bash
go install github.com/allie/asshi@latest
```
(Or build from source)
```bash
go build -o asshi main.go
sudo mv asshi /usr/local/bin/
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