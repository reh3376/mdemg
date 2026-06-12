# Phase 97: Process Lifecycle + Secret Management

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Status:** Complete
**Branch:** `mdemg-dev01`
**Depends on:** Phase 95 (Database + Embedding + Migrations)
**Effort:** M

---

## Overview

Phase 97 addresses two friction points in the MDEMG developer workflow:

1. **No daemon mode** (Gap 10 — HIGH): `mdemg serve` runs foreground-only, requiring a dedicated terminal. Developers need `mdemg start/stop/restart/status` lifecycle management like `mdemg db start/stop/status`.

2. **No secret management** (Gap 14 — MEDIUM): API keys and passwords live in plaintext `.env` files. No system keychain integration.

Both gaps are from `docs/specs/phase92-gap-analysis.md`.

---

## Implementation

### Secret Management (`internal/secrets/keyring.go`)

Thin wrapper around `go-keyring` for cross-platform keychain access:

- **macOS**: Keychain
- **Linux**: secret-tool (D-Bus Secret Service)
- **Windows**: Credential Manager

**API:**

- `Set(key, value) error` — store secret
- `Get(key) (string, error)` — retrieve (returns `ErrNotFound` if missing)
- `Delete(key) error` — remove secret
- `ResolveSecrets()` — iterate `KnownSecrets`, set env vars if not already set

**Known Secrets (auto-mapped to env vars):**

| Key | Env Var |
|-----|---------|
| `neo4j-password` | `NEO4J_PASS` |
| `openai-api-key` | `OPENAI_API_KEY` |
| `jwt-secret` | `AUTH_JWT_SECRET` |
| `linear-webhook` | `LINEAR_WEBHOOK_SECRET` |

Arbitrary keys are also accepted but will not be auto-resolved.

### Secret CLI Commands (`internal/cli/secrets.go`)

Three subcommands under `mdemg config`:

| Command | Description |
|---------|-------------|
| `mdemg config set-secret <key> [value]` | Store secret (prompts for hidden input if value omitted) |
| `mdemg config get-secret <key>` | Retrieve and print secret (exit 1 if not found) |
| `mdemg config list-secrets` | List known secrets with keychain status (never prints values) |

### Config Priority Update (`internal/cli/config_loader.go`)

`secrets.ResolveSecrets()` is called between YAML config and `.env` loading:

```
defaults → yaml → keychain → .env → env vars → flags
```

Keychain is opportunistic — if unavailable, falls back silently.

### Process Lifecycle (`internal/cli/daemon.go`)

**PID file utilities:**

- `pidFilePath()` → `.mdemg/mdemg.pid`
- `logFilePath()` → `.mdemg/logs/mdemg.log`
- `writePID()` — atomic write (tmp + rename)
- `readPID()`, `removePID()`, `isProcessAlive()`, `processUptime()`

**Commands:**

| Command | Description |
|---------|-------------|
| `mdemg start` | Start server as background daemon |
| `mdemg stop` | Stop server (SIGTERM, 30s timeout, SIGKILL fallback) |
| `mdemg restart` | Stop then start |
| `mdemg status` | Show server/DB status, health, uptime |

**`mdemg start` behavior:**

1. Check if already running (PID file + process alive check)
2. Auto-start Neo4j container if stopped (unless `--no-db`)
3. Resolve binary path, build `serve` args
4. Open log file (truncated on each start)
5. Load `.env` (godotenv) then `.mdemg/config.yaml` (YAML skip-if-set) into parent env — child inherits via `os.Environ()`
6. `exec.Command` with `SysProcAttr{Setsid: true}` for process detachment
6. Write PID file, wait 2s for early crash detection
7. Poll `.mdemg.port` file for port discovery (up to 10s)

**`mdemg stop` behavior:**

1. Read PID file, check process alive
2. Send SIGTERM, poll for exit (30s)
3. SIGKILL if still alive after timeout
4. Remove PID file
5. Print reminder about `mdemg db stop`

**`mdemg status` output:**

```
MDEMG Status
============
  Server:    running (pid=12345)
  Port:      9999
  Uptime:    2h 15m
  Log:       .mdemg/logs/mdemg.log
  Health:    ok
  Neo4j:     running (mdemg-neo4j-dev)
```

### Ignore Patterns (`internal/config/yaml_config.go`)

Added `.env` and `.env.*` to default `.mdemgignore` patterns to prevent accidental ingestion of secret files.

---

## Files Created

| File | Lines | Description |
|------|-------|-------------|
| `internal/secrets/keyring.go` | ~80 | Keychain wrapper |
| `internal/cli/secrets.go` | ~130 | `config set-secret/get-secret/list-secrets` commands |
| `internal/cli/daemon.go` | ~310 | `start/stop/restart/status` commands + PID utilities |

## Files Modified

| File | Changes |
|------|---------|
| `go.mod` / `go.sum` | Added `github.com/zalando/go-keyring`, `golang.org/x/term` |
| `internal/cli/root.go` | Registered `start`, `stop`, `restart`, `status` commands |
| `internal/cli/config_cmd.go` | Registered `set-secret`, `get-secret`, `list-secrets` under `config` |
| `internal/cli/config_loader.go` | Added `secrets.ResolveSecrets()` call, updated priority comment |
| `internal/config/yaml_config.go` | Added `.env`/`.env.*` to `GenerateIgnoreFile()` defaults |

---

## Key Design Decisions

1. **Daemon = detached child process** via `os/exec` + `SysProcAttr{Setsid: true}`. No supervisor/watchdog — sufficient for a developer tool.
2. **`mdemg stop` = server only**. Does NOT stop Neo4j. Prints reminder.
3. **`mdemg start` auto-starts Neo4j** if Docker available and container exists but stopped. `--no-db` to skip.
4. **PID + log files are project-local** (`.mdemg/mdemg.pid`, `.mdemg/logs/mdemg.log`).
5. **Log file truncated on each `start`**. Sufficient for dev tool use case.
6. **Keychain is opportunistic**. If unavailable, falls back silently to `.env`/env vars.
7. **Config priority**: defaults → yaml → keychain → .env → env vars → flags.

---

## Documents Accessed

- `docs/specs/phase92-gap-analysis.md` — Gap 10 (Process Lifecycle), Gap 14 (Security)
- `AGENT_HANDOFF.md` — Phase 97 entry
- `internal/cli/serve.go` — signal handling, port file, MCP subprocess
- `internal/cli/docker.go` — `DockerAvailable()`, `InspectContainer()`, `WaitForPort()`
- `internal/cli/db.go` — `db start/stop/status` pattern
- `internal/cli/config_loader.go` — layered config loading
- `internal/cli/config_cmd.go` — config subcommand registration
- `internal/config/yaml_config.go` — `GenerateIgnoreFile()` default patterns
- `internal/cli/root.go` — command registration
- `docs/features/unified-cli.md` — current CLI docs
- `CHANGELOG.md` — changelog format
- `README.md` — quickstart section
