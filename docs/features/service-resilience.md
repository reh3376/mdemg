---
created: 2026-04-02
updated: 2026-05-04
version: v0.6.0
author: reh3376
status: active
phase: "SVC-RES + 11.6.x additions"
---

# Service Resilience & Ingest Pipeline Hardening

## Summary

**Feature**: Service Resilience
**Summary**: Resilience mechanisms for data loss prevention across Docker and native deployment topologies, including supervision models and ingest pipeline hardening.


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

## Server-Native Alert Evaluation (SNA-001)

The server evaluates 13 TSDB-query alert rules natively, eliminating the dependency on Grafana for alerting. Rules are defined in `internal/alert/rules.go` and evaluated by the `Evaluator` in `internal/alert/evaluator.go`.

### Alert Delivery Chain (Server-Native)

```
MDEMG Server
├── Health Prober (4 targets)       → alert dispatcher → user
├── CB State Changes                → alert dispatcher → user
├── LLM Consecutive Failures        → alert dispatcher → user
├── TSDB Writer Overflow            → alert dispatcher → user
├── RSIC Self-Reflect (29 patterns) → alert dispatcher → user
└── Alert Evaluator (13 rules)      → alert dispatcher → user
      ├── Periodic TSDB queries (30s default)
      ├── ForDuration state tracking (prevents flapping)
      └── Graceful degradation (log + skip on TSDB unavailable)

Grafana remains for dashboards only. Grafana alert rules are supplementary.
```

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `ALERT_EVALUATOR_ENABLED` | `true` | Enable server-native rule evaluation |
| `ALERT_EVALUATOR_INTERVAL_SEC` | `30` | Base evaluation tick interval |

## Circuit Breaker Admin Endpoints (DH-004)

Each LLM task is wrapped in a registered circuit breaker (`openai-constraint-classify`, `jiminy-synthesis`, etc.). A breaker opens after `failure_threshold` consecutive failures and refuses calls for `timeout_sec` before entering half-open probe.

Two admin endpoints let operators inspect and reset breakers manually (gated by `AUTH_API_KEYS`):

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/admin/breakers` | GET | List all breakers with state, consecutive failures, last failure time |
| `/v1/admin/breakers/reset` | POST | Force a named breaker to `StateClosed` (`{"name":"<breaker>"}`) |

Use these when a breaker has tripped on a transient incident and auto-recovery via half-open probe hasn't happened yet. Complementary behavior:
- `LLM_RETRY_DEADLINE_ENABLED=true` (default) retries once on `context.DeadlineExceeded` when budget permits, preventing a single slow OpenAI call from tripping the breaker.
- `CONSULTING_CLASSIFY_TIMEOUT_MS` default is 30000 (was 15000), giving `gpt-5.4-mini` enough headroom under load.

See `docs/user/api-reference.md#admin-circuit-breakers` for request/response details.

## Goroutine Supervisor (SNA-001)

Background goroutines (health prober, alert evaluator) are monitored by a supervisor (`internal/supervisor/`) that provides:

- **Panic recovery** — `defer recover()` on every supervised goroutine
- **Auto-restart** — exponential backoff (5s base, doubles each retry, max 3 restarts)
- **Alerts** — warning on restart, critical on permanent failure (max retries exceeded)
- **Graceful shutdown** — context cancellation stops all supervised workers

## Phase 11.6.x Additions (2026-05-04 backfill)

Phase 11.6.x ("Operational hygiene", `phase_11_6_x_post.md`) added several resilience knobs that were not captured here when the Phase 11.6.x sprint shipped. Backfilled now per the per-feature-doc-required rule.

### RSIC Concurrency Limit (Epic 1)

Concurrent RSIC reflection cycles (`ape.reflect` + `consulting.classify` + `jiminy.evaluate*`) could overlap when multiple sessions hit mdemg simultaneously, causing one slow LLM call to compound on top of another and triggering the watchdog. Phase 11.6.x added a semaphore at the RSIC entry point.

| Env Var | Default | Description |
|---|---|---|
| `RSIC_MAX_CONCURRENT_CYCLES` | `2` | Maximum parallel RSIC cycles. Excess attempts queue (with timeout) |
| `RSIC_CYCLE_QUEUE_TIMEOUT_MS` | `15000` | Time-out for cycles waiting in queue before returning a transient error |

Implementation: `internal/rsic/coordinator.go` — `sync.WaitGroup` + `chan struct{}` semaphore.

### Prompt Cache Configuration (Epic 4)

llama-server's `--prompt-cache` flag retains the prompt-prefix KV state across calls, dramatically reducing time-to-first-token for repeated prompt patterns (RSIC reflection prompts share a long prefix). Phase 11.6.x configured the cache size and aging.

| Env Var | Default | Description |
|---|---|---|
| `LLM_PROMPT_CACHE_PATH` | `/tmp/mdemg-prompt-cache` | Disk path for the persisted KV cache |
| `LLM_PROMPT_CACHE_SIZE_MB` | `512` | Maximum cache size; LRU eviction beyond |

Operators verifying the cache is hot can `tail -f $LLM_PROMPT_CACHE_PATH` for new entries during steady-state load.

### ConflictTracker Production Wiring (Epic 5 / Workstream C #1)

When two pieces of guidance produce contradictory signals (e.g. one says "follow this constraint", another says "violate it for performance"), the ConflictTracker records the pair in TSDB V0015 (`guidance_conflicts` hypertable). Phase 11.6.x wired this in production for two consumers:

1. **UVTS A/B failures** — Phase 12 Epic 6 wires UVTS gate-fails into ConflictTracker so retrieval-quality conflicts surface alongside other guidance conflicts in Grafana
2. **Jiminy escalation events** — when the same constraint re-fires after being marked `surfaced`, the conflict is recorded for J17 protocol-stability analysis

| Env Var | Default | Description |
|---|---|---|
| `CONFLICT_TRACKER_ENABLED` | `true` | Master toggle; false suppresses V0015 writes |
| `CONFLICT_TRACKER_DEDUP_WINDOW_SEC` | `300` | Identical conflicts within window dedup to one row |

### Jiminy Defaults

Phase 11.6.x flipped two Jiminy defaults that the original phase missed:

| Env Var | Pre-11.6.x | Post-11.6.x | Rationale |
|---|---|---|---|
| `JIMINY_OUTCOME_LLM_ENABLED` | `false` | `true` | Tier-2 outcome classifier was effective enough to default-on |
| `JIMINY_FOLLOW_RATE_PERSIST` | `false` | `true` | T1 comprehension gate (`J17_T1_COMPREHENSION_GATE`) needs persisted history |

## Documents Accessed

- `internal/cli/service.go`, `service_darwin.go`, `service_linux.go`, `service_stub.go` — service management CLI
- `internal/cli/hooks.go` — hook registration and settings merge
- `internal/cli/ingest_claude_md.go` — buffer/flush logic
- `internal/cli/data.go` — audit subcommand
- `internal/rsic/coordinator.go` — Phase 11.6.x semaphore
- `internal/conversation/conflict_tracker.go` — Phase 11.6.x ConflictTracker production wiring
- `.claude/hooks/session-start.sh` — auto-start, error logging, TSDB check
- `.claude/hooks/prompt-context.sh` — visible warning
- `.claude/hooks/post-tool-observe.py` — prune-guard, protected overflow, error logging
- `packaging/launchd/*.plist` — LaunchAgent supervision templates
- `docs/development/ft-lora/phase_11_6_x_post.md` — origin sprint for the additions above
