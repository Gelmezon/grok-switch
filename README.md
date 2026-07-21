**English** | [简体中文](README.zh-CN.md)

# grok-switch

<p align="center">
  <strong>Grok Build Relay Switcher</strong><br>
  Single binary · Zero dependencies · Self-updating · CLI + full-screen TUI · Linux amd64 / arm64
</p>

<p align="center">
  <a href="https://github.com/Gelmezon/grok-switch/releases/latest"><img src="https://img.shields.io/github/v/release/Gelmezon/grok-switch?style=flat-square" alt="release"></a>
  <a href="https://github.com/Gelmezon/grok-switch/releases"><img src="https://img.shields.io/github/downloads/Gelmezon/grok-switch/total?style=flat-square" alt="downloads"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="license"></a>
</p>

---

## One-line installation (recommended)

Run the following command on **Linux**. It detects the system architecture, downloads the latest release, and installs the binary to `/usr/local/bin`.

```bash
curl -fsSL https://raw.githubusercontent.com/Gelmezon/grok-switch/main/scripts/install.sh | sudo bash
```

Verify the installation:

```bash
grok-switch version
grok-switch          # Start the full-screen TUI
```

| Option | Description | Example |
|---|---|---|
| `VERSION` | Install a specific version (default: `latest`) | `VERSION=v0.1.0 sudo -E bash -c 'curl -fsSL ... \| bash'` |
| `INSTALL_DIR` | Installation directory (default: `/usr/local/bin`) | `INSTALL_DIR=$HOME/.local/bin bash scripts/install.sh` |
| `GROK_SWITCH_REPO` | Override the GitHub repository | Default: `Gelmezon/grok-switch` |

Installer source: [`scripts/install.sh`](scripts/install.sh)

The installer downloads `SHA256SUMS` and verifies the binary before writing it to the destination directory.

For self-updates without `sudo`, install to a user-writable directory and ensure that directory is in your `PATH`:

```bash
INSTALL_DIR="$HOME/.local/bin" bash scripts/install.sh
```

---

## Manual release download

Latest build: [Releases](https://github.com/Gelmezon/grok-switch/releases/latest)

| Architecture | File |
|---|---|
| Linux x86_64 | [`grok-switch-linux-amd64`](https://github.com/Gelmezon/grok-switch/releases/latest/download/grok-switch-linux-amd64) |
| Linux arm64 | [`grok-switch-linux-arm64`](https://github.com/Gelmezon/grok-switch/releases/latest/download/grok-switch-linux-arm64) |
| Checksums | [`SHA256SUMS`](https://github.com/Gelmezon/grok-switch/releases/latest/download/SHA256SUMS) |

```bash
# amd64 example
wget https://github.com/Gelmezon/grok-switch/releases/latest/download/grok-switch-linux-amd64
chmod +x grok-switch-linux-amd64
sudo mv grok-switch-linux-amd64 /usr/local/bin/grok-switch

# Optional checksum verification
wget https://github.com/Gelmezon/grok-switch/releases/latest/download/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
```

---

## What is it?

`grok-switch` manages multiple **Grok Build relay profiles** and safely switches `~/.grok/config.toml` between those profiles and the official authentication configuration.

- **One static binary** with no desktop, web, or OAuth dependencies
- Full-screen TUI in a terminal; automatic plain-text output in pipes and CI
- Automatic backup before every switch, with rollback support on failure

```bash
grok-switch add relay-a      # Add a relay provider
grok-switch list             # List providers, including the official default
grok-switch use relay-a      # Switch providers
grok-switch test relay-a     # Test model connectivity
grok-switch status           # Show current status
grok-switch official         # Return to the official configuration
grok-switch backup list      # List backups
```

---

## Quick start

### Full-screen TUI (recommended)

```bash
grok-switch
# or
grok-switch tui
```

| Key | Action |
|---|---|
| `↑↓` / `jk` | Select a profile |
| `Enter` | Switch to the selected profile |
| `a` / `e` / `d` | Add / edit / delete |
| `o` | Return to official authentication |
| `b` | Manage backups |
| `s` | Show current status |
| `U` | Check for and install updates in the TUI |
| `/` | Search |
| `Tab` | Change panel focus |
| `?` | Show help |
| `q` | Quit |

### Common CLI commands

```bash
# Interactive add
grok-switch add relay-a

# Non-interactive add (CI)
GROK_SWITCH_API_KEY='sk-xxx' grok-switch add relay-a \
  --base-url https://relay.example.com/v1 \
  --model grok-4

# Switch / status / official configuration
grok-switch use relay-a
grok-switch status
grok-switch official

# Backups
grok-switch backup list
grok-switch backup restore <filename>
grok-switch backup prune --keep 10

# Application updates
grok-switch update check
grok-switch update
grok-switch update rollback
```

> There is intentionally no `--api-key` option, preventing API keys from appearing in shell history or `ps` output.

---

## Command reference

| Command | Description |
|---|---|
| `grok-switch` / `tui` | Start the full-screen TUI |
| `add` / `edit` / `delete` / `show` / `list` | Manage profiles |
| `use` / `status` / `official` | Switch configurations and inspect status |
| `import-current <name>` | Import the current `config.toml` as a profile |
| `backup list\|restore\|prune` | Manage backups |
| `update check` / `update` / `update rollback` | Check, install, or roll back application updates |
| `version` / `help` / `completion` | Utilities |

**Global flags:** `--no-interactive`, `--json` where supported (for example, `list`).

---

## Environment variables

| Variable | Description | Default |
|---|---|---|
| `GROK_HOME` | Grok configuration directory | `~/.grok` |
| `GROK_CONFIG` | Grok configuration file | `$GROK_HOME/config.toml` |
| `GROK_SWITCH_HOME` | grok-switch data directory | `~/.grok_switch` |
| `GROK_SWITCH_API_KEY` | API key for non-interactive commands | — |
| `GROK_SWITCH_UPDATE_MODE` | Update mode: `notify`, `auto`, or `off` | `notify` |
| `GROK_SWITCH_NO_UPDATE_CHECK` | Disable update checks when the TUI starts | — |
| `NO_COLOR` | Disable colored output | — |
| `GROK_SWITCH_NO_TUI` | Disable the full-screen TUI | — |

---

## Security

- API keys never appear in command-line arguments, logs, or shell history
- Data directories use mode `0700`, sensitive files use `0600`, and the process starts with `umask(0077)`
- Atomic writes, pre-switch backups, and `flock`-based concurrency protection

## Self-updates

The TUI checks the latest stable GitHub release asynchronously at startup and caches the result for 24 hours without blocking the main screen. Press `U` to force a check at any time; when an update is available, the TUI opens a confirmation dialog and completes the installation in place.

```bash
grok-switch update check          # Force an online check
grok-switch update                # Check and update with confirmation
grok-switch update --yes          # Update non-interactively
grok-switch update rollback       # Restore the retained previous version
```

The updater selects the release binary for the current architecture, verifies its SHA-256 digest and embedded version, then performs an atomic replacement in the same directory. The previous binary is retained as `grok-switch.previous`.

Set `GROK_SWITCH_UPDATE_MODE=auto` to enable automatic installation. The target binary must be writable by the current user. A default installation in `/usr/local/bin` will usually require:

```bash
sudo grok-switch update
```

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | Success |
| 1 | Runtime error |
| 2 | Usage error |
| 3 | `status`: on-disk configuration does not match the active profile |
| 4 | Profile not found |
| 5 | Lock timeout |
| 6 | Configuration parsing failed |
| 7 | Backup or restore failed |

---

## Build from source

```bash
git clone https://github.com/Gelmezon/grok-switch.git
cd grok-switch
make build          # dist/grok-switch
make release        # dist/grok-switch-linux-amd64|arm64 + SHA256SUMS
make test && make vet
sudo make install   # → /usr/local/bin/grok-switch
```

**Requirements:** Go 1.24+

Pushing a `v*` tag triggers GitHub Actions to build and publish a release containing the two Linux binaries and `SHA256SUMS`.

---

## License

MIT

**Author:** DavidZhao · **Repository:** [Gelmezon/grok-switch](https://github.com/Gelmezon/grok-switch)
