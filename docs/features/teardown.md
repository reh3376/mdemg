# Feature: Instance Teardown

**Command:** `mdemg teardown`
**Phase:** S16 (Sidecar)
**Status:** Complete

## Overview

The `mdemg teardown` command completely removes all traces of an MDEMG instance from a project directory and (optionally) from the entire system. It provides a single, unified cleanup utility that covers all 17 categories of artifacts MDEMG creates.

## Usage

```bash
# Preview what would be removed (safe, no changes)
mdemg teardown --dry-run

# Interactive teardown (prompts for confirmation)
mdemg teardown

# Non-interactive teardown
mdemg teardown --yes

# Export CMS/RSIC/Jiminy data before teardown
mdemg teardown --export --yes

# Preserve Neo4j volume (graph data)
mdemg teardown --keep-data --yes

# Full system removal (binary, plugins, systemd, man pages)
mdemg teardown --full --yes

# Machine-readable output
mdemg teardown --format json --yes
```

## Scopes

### Instance (default)

Removes all project-level MDEMG artifacts:
- Stops MDEMG server (graceful SIGTERM → force SIGKILL)
- Stops and removes Docker container
- Removes Docker volume (unless `--keep-data`)
- Deletes Neo4j space data (batch delete)
- Removes keyring secrets (neo4j-password, openai-api-key, jwt-secret, linear-webhook)
- Uninstalls git hooks (post-commit) and Claude Code hooks
- Cleans MCP configs (.mcp.json, .claude/mcp.json, .cursor/mcp.json, .vscode/mcp.json)
- Backs up and removes `.mdemg/` directory
- Removes `.mdemgignore`
- Deregisters from sidebar/menubar instance registries
- Cleans MDEMG entries from `.claude/settings.local.json`

### Full (`--full`)

Everything in Instance scope, plus:
- Removes system binary (or advises `brew uninstall mdemg` for Homebrew installs)
- Removes systemd units (Linux: `mdemg@.service`, `mdemg-rsic@.*`)
- Removes man pages (`/usr/local/share/man/man1/mdemg*.1`)
- Removes shell completions (bash, zsh, fish)
- Removes plugins directory (`/usr/local/share/mdemg/`)

## Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview what would be removed without executing |
| `--format` | Output format: `text` (default) or `json` |
| `-y, --yes` | Skip confirmation prompt |
| `--force` | Allow deletion of protected spaces (`mdemg-dev`, `mdemg-global`) |
| `--full` | Include system-level cleanup |
| `--export` | Export CMS data before teardown |
| `--keep-data` | Preserve Neo4j volume |
| `--space-id` | Space ID for Neo4j data cleanup (default: auto-detect from config or directory name) |

## Safety Guarantees

1. **Backup**: `.mdemg/` is renamed to `.mdemg-backup-<timestamp>/` before removal
2. **Protected spaces**: `mdemg-dev` and `mdemg-global` are skipped unless `--force` is used
3. **Dry-run**: `--dry-run` previews all actions without executing any
4. **Confirmation**: Interactive prompt required unless `--yes` is passed; full scope requires typing "teardown"
5. **Non-destructive to .env**: `.env` is preserved (may contain non-MDEMG variables)
6. **Selective MCP cleanup**: Only removes `mdemg` entry from MCP configs; preserves other servers
7. **Soft failures**: Each phase logs warnings and continues on error (no single failure blocks the entire teardown)

## Cross-Platform Support

| Artifact | macOS | Linux | Windows |
|----------|-------|-------|---------|
| Server stop | SIGTERM/SIGKILL | SIGTERM/SIGKILL | TerminateProcess |
| Docker cleanup | Yes | Yes | Yes |
| Neo4j space data | Yes | Yes | Yes |
| Keyring secrets | Keychain | secret-tool | Credential Manager |
| Git hooks | Yes | Yes | Yes |
| Claude hooks | .sh | .sh | .ps1 |
| MCP configs | Yes | Yes | Yes |
| Sidebar registry | ~/Library/App Support/com.reh3376.mdemg-menubar/ | ~/.config/mdemg-sidebar/ | N/A |
| System binary | brew uninstall / manual | /usr/local/bin/ | N/A |
| Systemd units | N/A | /etc/systemd/system/ | N/A |

## Companion App Integration

Teardown is accessible from the native companion apps on macOS and Linux:

| Platform | App | Integration |
|----------|-----|-------------|
| **macOS** | mdemg-menubar (Swift) | Teardown button in Status tab + context menu on instances. Calls `mdemg teardown --yes --format json`, parses report, deregisters instance. |
| **Linux** | mdemg-linux-sidebar (Tauri) | "Teardown Instance" section in Config tab with dry-run preview and confirmation dialog. Calls CLI via Rust command bridge. |
| **Windows** | Install-MDEMG.ps1 | `-Uninstall` flag attempts `mdemg teardown --yes` before manual cleanup. |

## Execution Phases

| Phase | Description | Scope |
|-------|-------------|-------|
| 0 | Pre-export (optional) | Instance |
| 1 | Stop MDEMG server | Instance |
| 2 | Remove Docker container | Instance |
| 3 | Remove Docker volume | Instance |
| 4 | Delete Neo4j space data | Instance |
| 5 | Remove keyring secrets | Instance |
| 6 | Uninstall hooks | Instance |
| 7 | Remove IDE configs (MCP) | Instance |
| 8 | Backup and remove .mdemg/ | Instance |
| 9 | Remove .mdemgignore | Instance |
| 10 | Deregister from sidebar apps | Instance |
| 11 | Clean Claude settings | Instance |
| 12 | Remove system binary | Full only |
| 13 | Remove system artifacts | Full only |

## Key Files

- `internal/cli/teardown.go` — Core command implementation (14 phases)
- `internal/cli/sidebar_registry.go` — Sidebar/menubar instance registry utilities
- `internal/cli/mcp_cleanup.go` — MCP config and Claude settings cleanup
- `internal/cli/teardown_test.go` — Unit tests
- `internal/cli/root.go` — Command registration
- `packaging/mdemg-menubar/MdemgMenuBar/Services/CLIExecutor.swift` — macOS CLI teardown methods
- `packaging/mdemg-menubar/MdemgMenuBar/Services/PollingManager.swift` — macOS teardown state/actions
- `packaging/mdemg-menubar/MdemgMenuBar/Views/StatusView.swift` — macOS teardown button in OverviewTab
- `packaging/mdemg-menubar/MdemgMenuBar/Views/InstanceManagerView.swift` — macOS teardown context menu
- `packaging/mdemg-linux-sidebar/src-tauri/src/commands.rs` — Linux teardown Rust commands
- `packaging/mdemg-linux-sidebar/src/tabs/config.js` — Linux teardown UI in config tab
- `packaging/mdemg-windows/Install-MDEMG.ps1` — Windows installer teardown integration

## Documents Accessed

- `internal/cli/sidecar_uninstall.go` — Pattern reference (phased teardown, dry-run, backup)
- `internal/cli/daemon.go` — Process stop/kill logic
- `internal/cli/hooks.go` — Hook uninstall functions
- `internal/cli/docker.go` — Docker container/volume management
- `internal/cli/space.go` — Space deletion, export patterns
- `internal/cli/init.go` — Menubar registration (registerWithMenubar)
- `internal/secrets/keyring.go` — Keyring API
- `internal/sidecar/types.go` — Report structures
- `internal/sidecar/report.go` — Report helpers
- `internal/transfer/exporter.go` — Export API
- `internal/api/handlers.go` — Protected space check
- `docs/development/API_REFERENCE.md` — CLI reference
- `AGENT_HANDOFF.md` — Phase artifact index
