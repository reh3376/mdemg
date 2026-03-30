# Service Resilience & Ingest Pipeline Hardening

This document covers the resilience mechanisms that prevent data loss when MDEMG services are unavailable.

## Service Topology

### Docker Deployment (Primary)

In Docker mode, all services run as containers with `restart: unless-stopped`. Docker handles process supervision, restart on crash, and startup ordering via `depends_on` with health checks. No LaunchAgent/systemd configuration is needed.

| Service | Container | Supervision | Purpose |
|---------|-----------|-------------|---------|
| MDEMG Server | `mdemg` | `restart: unless-stopped` | HTTP API, CMS, RSIC, Jiminy, J17 |
| Neo4j | `neo4j` | `restart: unless-stopped` | Graph database |
| TimescaleDB | `timescaledb` | `restart: unless-stopped` | Time-series metrics |
| Neural Sidecar | `neural-sidecar` | `restart: unless-stopped` | Reranking, NLI |
| Grafana | `grafana` | `restart: unless-stopped` | Observability dashboards |

### Native Deployment (Dev-Only)

For native development, MDEMG runs as three supervised processes:

| Service | Binary | Supervision | Purpose |
|---------|--------|-------------|---------|
| MDEMG Server | `mdemg serve` | KeepAlive (auto-restart on crash) | HTTP API, CMS, RSIC, Jiminy, J17 |
| Neural Sidecar | `python3 -m uvicorn` | KeepAlive (auto-restart on crash) | Embedding inference, cross-encoder re-ranking |
| Ingest Timer | `mdemg ingest-claude-md` | StartInterval (every 30 min) | Periodic re-ingestion of CLAUDE.md files |

Without supervision, a server crash or reboot silently disables all 5 Claude Code hooks — session-start warns once, prompt-context exits silently, post-tool-observe fires into the void.

## Process Supervision

### macOS (launchd)

Three LaunchAgent plists in `packaging/launchd/`:

- `com.mdemg.server.plist` — `KeepAlive: {SuccessfulExit: false}`, `ThrottleInterval: 30` (prevents crash-loop rapid restarts)
- `com.mdemg.neural-sidecar.plist` — same KeepAlive pattern for the Python sidecar
- `com.mdemg.ingest-claude-md.plist` — `StartInterval: 1800` (timer-based, every 30 min)

Install and manage with:

```bash
mdemg service install              # Install + start all services
mdemg service status               # Show running/stopped state
mdemg service restart              # Restart all services
mdemg service logs -f              # Follow log output
mdemg service uninstall            # Stop + remove all services
```

Uses modern `launchctl bootstrap`/`bootout` API (not deprecated `load`/`unload`).

### Linux (systemd)

Wraps existing systemd units from `packaging/mdemg_linux/systemd/`:

- `mdemg.service` — server (templated per user)
- `mdemg-rsic.service` — RSIC self-improvement cycle
- `mdemg-rsic.timer` — periodic RSIC trigger

### `mdemg service` vs `mdemg start`

| Feature | `mdemg start` | `mdemg service install` |
|---------|---------------|------------------------|
| Persistence | PID file, dies on reboot | OS-level, survives reboot |
| Crash recovery | None | Auto-restart (30s throttle) |
| Scope | Server only | Server + sidecar + ingest timer |
| Control | `mdemg stop/restart` | `mdemg service status/restart/logs` |

Use `mdemg start` for development. Use `mdemg service install` for persistent deployments.

## Hook Auto-Recovery

### Auto-Start (session-start.sh)

When the server is down at session start, the hook attempts to start it automatically:

1. Detect server down via `/healthz` check (2s timeout)
2. Run `./bin/mdemg start --auto-migrate` if binary exists
3. Poll 5 times at 2s intervals (10s total, within 15s hook timeout)
4. On success: proceed normally with CMS resume
5. On failure: display degraded-mode warning, continue without memory

### Visible Warnings

- **prompt-context.sh**: Prints "CMS unavailable — no memory context for this prompt" instead of silently exiting
- **session-start.sh**: Shows full disconnected-mode banner with investigation checklist

### Error Logging

All ingest operations now log to `~/.mdemg/logs/ingest-claude-md.log` instead of `/dev/null`:

- `session-start.sh` fire-and-forget ingest → logs errors
- `post-tool-observe.py` Popen ingest calls → logs errors
- LaunchAgent ingest timer → logs to `~/.mdemg/logs/ingest-claude-md.log`

### TimescaleDB Health Check

Session-start checks `pg_isready` on `$TSDB_PORT` (default 5433) and warns if TimescaleDB is down — training data collection depends on it.

## Ingest Pipeline Hardening

### Local JSONL Buffer

When the server is unreachable during `mdemg ingest-claude-md`, entries are buffered locally:

**Buffer location**: `.mdemg/ingest-buffer.jsonl` (configurable via `INGEST_BUFFER_PATH`)

**Buffer format** (one JSON object per line):
```json
{"path":"CLAUDE.md","content":"...","content_hash":"sha256:abc...","tags":["claude-md","config"],"buffered_at":"2026-03-30T12:00:00Z","space_id":"mdemg-dev","file_size":1234,"line_count":50}
```

**Behavior**:
- On server down: buffer to JSONL, FIFO eviction at `INGEST_BUFFER_MAX_ENTRIES` (default 100)
- On server up: flush pending buffer entries before processing new files
- Partial flush failures are preserved — only successfully ingested entries are removed

### Prune-Guard Detection

Before ingesting a CLAUDE.md file, `post-tool-observe.py` checks for content shrinkage:

1. Read current line count from disk
2. Query `/v1/memory/node/meta` for stored line count
3. If file shrank by >10 lines: record a `[prune-guard]` observation with old hash before ingesting

This preserves context about what was lost when Claude Code self-prunes `.md` files.

### Protected Overflow

When MEMORY.md grows too large and overflow detection triggers:

- **Before**: `POST /v1/conversation/observe` — volatile observation with Context Cooler decay (10%/day)
- **After**: `POST /v1/memory/ingest` — stable leaf node with no decay

Overflow content is now permanently preserved as ingested memory nodes.

## Auditing

```bash
mdemg data audit                   # Compare disk vs CMS state
mdemg data audit --space-id mdemg-dev
```

Reports for each tracked file:
- **current** — disk matches CMS, ingested within 24h
- **STALE** — ingested more than 24h ago
- **SHRANK** — disk has significantly fewer lines than CMS record
- **DELETED** — file exists in CMS but not on disk
- **NOT INGESTED** — file exists on disk but not in CMS

Also shows: pending ingest buffer entries, server/Neo4j/sidecar health.

## Hook Template Sync

Active hooks in `.claude/hooks/` are the source of truth. Templates in `internal/cli/hook_templates/` are parameterized copies. `claudeHookFiles()` registers all 5 hooks with correct events, timeouts, and matchers. See `docs/features/ide-repo-integration.md` for the full registration table.

## Documents Accessed

- `internal/cli/service.go`, `service_darwin.go`, `service_linux.go`, `service_stub.go` — service management CLI
- `internal/cli/hooks.go` — hook registration and settings merge
- `internal/cli/ingest_claude_md.go` — buffer/flush logic
- `internal/cli/data.go` — audit subcommand
- `.claude/hooks/session-start.sh` — auto-start, error logging, TSDB check
- `.claude/hooks/prompt-context.sh` — visible warning
- `.claude/hooks/post-tool-observe.py` — prune-guard, protected overflow, error logging
- `packaging/launchd/*.plist` — LaunchAgent supervision templates
