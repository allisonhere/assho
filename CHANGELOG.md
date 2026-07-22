# Changelog

## v0.2.0 — 2026-07-22

Assho now includes a complete SSH key-management workflow and a responsive host editor.

### Added

- A responsive add/edit experience that uses a centered modal on larger terminals and a scrolling full-screen workspace on compact terminals.
- Public-key installation from an existing host with `Ctrl+K`, using the configured identity, SSH defaults, or a selected `.pub` file.
- Staged fleet key rotation with `K`, including Ed25519 key generation, strict replacement-key verification, per-host backups, exact old-key removal, and resumable journals.
- Reviewed server host-key enrollment for every TUI SSH action. OpenSSH displays the SHA256 fingerprint and writes approved entries to `known_hosts`; changed keys remain hard failures.

### Fixed

- Existing-key selection no longer falls through to key generation when `ssh-add` is unavailable or cancelled.
- Systems without an SSH agent can use an unencrypted selected identity directly; verification still completes before any destructive rotation step.
- Fleet rotation rejects `.pub` files when a private identity is required and refuses to rotate a host to its already configured key.
- Declined host trust and failed replacement verification preserve existing access and cannot trigger an unsafe cleanup step.

### Operational notes

- Fleet rotation manages the standard remote `~/.ssh/authorized_keys` path. Hosts using an alternate `AuthorizedKeysFile` or `AuthorizedKeysCommand` require manual cleanup.
- Rotation journals are stored under `~/.config/assho/rotation-runs/` with private permissions and never contain passwords or private-key contents.
