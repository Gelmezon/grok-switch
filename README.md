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

## Why I built grok-switch

I run Grok Build on Linux servers and use relay providers instead of the official authentication. Switching between them sounds simple—change the URL, swap the key—but it kept not working, and I kept spending time debugging TOML to find out why.

The root cause is that Grok Build `0.2.106` needs more than a global `[endpoints].api_url`. Every model section needs its own `base_url` and `api_backend`. If those are missing or wrong, the relay's `/v1/models` and `/v1/chat/completions` both return 200, but Grok itself still says `Authentication required`. That combination is annoying to debug because everything looks fine until it isn't.

Relay model lists also change over time—models get added, renamed, or dropped. I was maintaining a handwritten list and it kept drifting. Direct edits to `~/.grok/config.toml` made things worse: easy to clobber unrelated settings, easy to leave the file broken if the write gets interrupted.

So I automated it. `grok-switch` stores each relay as a profile, fetches the live model list when you switch, and writes the full per-model configuration instead of having you do it by hand. It backs up before every change and rolls back if something goes wrong.

## Problems it solves

| Real-world problem | How grok-switch handles it |
|---|---|
| The relay URL was changed, but Grok still asks for `/login` | It generates the current per-model `base_url`, `api_backend = "chat_completions"`, and authentication fields instead of relying only on the legacy global endpoint. |
| A relay's model inventory keeps changing | Adding, editing, or switching a profile fetches `/v1/models`, deduplicates the result, and generates every model entry. An existing default is preserved while available. |
| URLs, keys, and models from multiple relays get mixed up | Each provider is stored as an independent profile that can be inspected, tested, and selected by name. |
| “The API responds” does not mean “Grok can chat” | `test` checks authentication, the Models endpoint, and Chat Completions separately instead of treating one successful HTTP response as proof. |
| Manual TOML edits can corrupt or overwrite other settings | It updates only relay-related fields, preserves unknown settings, writes atomically, parses the result, and creates a backup before switching. |
| Official Grok authentication is needed again | `official` removes fields managed by grok-switch while preserving unrelated Grok settings. |
| The server has no desktop environment | It ships as one Linux binary with a full-screen TUI, regular CLI output, pipe support, and CI-friendly operation. |
| API keys can leak into shell history or `ps` | There is no `--api-key` argument. Hidden prompts, standard input, environment variables, and restrictive file permissions are supported. |
| Telemetry, indexing, or the harness should not upload a codebase | “Disable codebase upload” is enabled by default and writes Grok's privacy settings with the active profile; users can explicitly opt out. |

## What happens during a switch

When you run `grok-switch use relay-a`, the tool:

1. Loads the profile's Base URL and API key.
2. Requests `/v1/models` from the relay to synchronize the real model inventory.
3. Preserves the current default if it is still available; otherwise it selects an available model by preference.
4. Generates independent Grok routing, authentication, and reasoning settings for every model.
5. Backs up `~/.grok/config.toml`, atomically updates the managed fields, and parses the result again for verification.
6. Marks the profile as active. Grok Build then talks directly to the configured relay.

`grok-switch` is **not a relay or network proxy**. It does not forward, store, or inspect normal Grok conversation traffic. It manages local configuration and contacts a relay only when a profile is added, edited, selected, or tested. It also cannot make an expired key or an incompatible API work: it currently targets OpenAI Chat Completions-compatible services that expose `/v1/models` and `/v1/chat/completions`.

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

## Everyday usage

For day-to-day use, the following commands manage multiple **Grok Build relay profiles** and safely switch `~/.grok/config.toml` between those profiles and the official authentication configuration.

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

**Note**: When adding, editing, or switching a profile, grok-switch requests the relay's `/v1/models` endpoint, generates configuration for every returned model, and selects a default automatically. An existing default is preserved while it remains available.

### Common CLI commands

```bash
# Interactive add
grok-switch add relay-a

# Non-interactive add (CI)
GROK_SWITCH_API_KEY='sk-xxx' grok-switch add relay-a \
  --base-url https://relay.example.com/v1

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
| `test <name-or-id>` | Test authentication, the Models endpoint, and Chat Completions separately |
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

- API keys **never** appear in arguments, logs, or shell history (use `GROK_SWITCH_API_KEY` env var or interactive prompts).
- Directories: `0700`; files: `0600`; process: `umask(0077)`.
- Atomic writes + automatic backups + rollback.
- `status` shows `official` / `managed-relay` / `unmanaged-or-unknown`.
- `test` validates auth + Models + Chat Completions separately.

## Disable codebase upload

Every profile has a **Disable codebase upload** toggle that is enabled by default. In the TUI add/edit screen, press `Tab` to focus the privacy setting and `Space` to toggle it. The next `use` operation writes the following fields to `~/.grok/config.toml` while protection is enabled:

```toml
[features]
telemetry = false
codebase_indexing = false

[telemetry]
trace_upload = false

[harness]
disable_codebase_upload = true
```

For non-interactive CLI use:

```bash
# Default: deny uploads
grok-switch add relay-a --base-url https://relay.example.com/v1 \
  --codebase-upload deny

# Change an existing profile, then select it again to apply the config
grok-switch edit relay-a --codebase-upload allow
grok-switch use relay-a
```

`deny` writes the four fields above. `allow` removes only those four grok-switch-managed fields and restores Grok's own default behavior; it does not actively set telemetry or uploads to `true`. The privacy fields remain when switching to `official` because they are not part of relay routing.

## Self-updates

TUI auto-checks latest release (caches 24h). Press `U` to force check → auto-update dialog.

```bash
grok-switch update          # Check + update (interactive)
grok-switch update --yes   # Non-interactive update
grok-switch update rollback # Restore previous version
```

Previous binary kept as `grok-switch.previous`. Auto mode: `GROK_SWITCH_UPDATE_MODE=auto`.

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | Success |
| 1 | Runtime error |
| 2 | Usage error |
| 3 | `status`: unmanaged/unknown or profile mismatch |
| 4 | Profile not found |
| 5 | Lock timeout |
| 6 | Config parsing failed |
| 7 | Backup/restore failed |

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
