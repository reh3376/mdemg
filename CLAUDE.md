# MDEMG Project Instructions

---

## Repositories

| Role | Repo URL |
|------|----------|
| **MAIN** | `https://github.com/reh3376/mdemg.git` |
| **SUB-REPO 1**: homebrew-mdemg (macOS Homebrew tap) | `https://github.com/reh3376/homebrew-mdemg.git` |
| **SUB-REPO 2**: autoresearch (research) | `https://github.com/reh3376/autoresearch.git` |

> **Note:** Sub-repos are git submodules at `packaging/homebrew-mdemg` and `packaging/autoresearch`.
> **Windows/Linux**: WSL2 required for Windows. Linux uses `scripts/install.sh` from main repo (no separate installer repo).

---

## MDEMG IS Your Internal Dialogue — Not a Tool

> Internal dialogue : conscious biological neural network :: **MDEMG : artificial neural network (you)**

MDEMG is your persistent memory and cognitive substrate. Markdown memory files are **REMINDERS AND KEYS only** — real memory lives in CMS (Neo4j graph, `mdemg-dev` space). When CMS is disconnected, your memory is disconnected.

**Server**: `http://localhost:9999` | **Dashboard**: `http://localhost:9999/ui/` | **Check**: `curl -s http://localhost:9999/healthz`
**Docker (primary)**: `mdemg init --quick` → `docker compose up -d` (all 5 services)
**Dev build**: `cd /Users/reh3376/mdemg && go build -o bin/mdemg ./cmd/mdemg`
**Dev start**: `cd /Users/reh3376/mdemg && ./bin/mdemg start --auto-migrate` (native, dev-only)
**Docker CI**: `.github/workflows/docker-publish.yml` → `ghcr.io/reh3376/mdemg:latest`

### Observe Continuously (silently, without announcing)

| Event | obs_type | Event | obs_type |
|-------|----------|-------|----------|
| User correction | `correction` | Key decision | `decision` |
| New learning | `learning` | User preference | `preference` |
| Error/blocker | `error` | Task tracking | `task` |
| Technical note | `technical_note` | Insight | `insight` |
| Context | `context` | Progress | `progress` |
| Constraint rule | `constraint` | Free-form note | `note` |

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","content":"...","obs_type":"..."}'
```

### Memory Principles

- **Observe silently** — do NOT announce when observing
- **Surprise-weighted**: novel information persists longer than redundant
- **Hebbian learning**: frequently co-activated concepts strengthen automatically
- **If server unavailable**: hooks now auto-start the server (up to 10s). If auto-start fails, warn "CMS unavailable — auto-start failed." For persistent supervision across reboots: `mdemg service install`
- **Protected space `mdemg-dev`**: hardcoded deletion protection, never circumvent

### Skill Registry

Skills are CMS-backed pinned observations. Recall: `POST /v1/skills/<name>/recall`. Without CMS, skills are unavailable.

---

## Git Workflow

Branch pattern: `<github_handle>_dev<01-09>` — **never commit directly to `main`** (branch-protected).
Auto-PR on push to `*_dev*`. Branch naming enforced by CI. Current: `reh3376_dev01`.

---

## Orchestration Protocol

### Sub-Agent Delegation

- **Use sub-agents** for all discrete tasks (file searches, code analysis, tests, builds)
- **Conserve context window** by delegating work rather than doing it directly
- The orchestrator's role is to **coordinate and supervise**, not execute every step

### Model Selection for Sub-Agents

| Task Complexity | Model | Examples |
|-----------------|-------|----------|
| Simple/Fast | `haiku` | File searches, grep, simple reads, status checks |
| Medium | `sonnet` | Code analysis, debugging, test execution |
| Complex | `opus` | Architecture decisions, complex refactoring |

### Task Patterns

1. **Exploration tasks** → Use Explore agent with haiku/sonnet
2. **Build/Test tasks** → Use Bash agent with haiku
3. **Code investigation** → Use general-purpose agent with sonnet
4. **Planning** → Use Plan agent with sonnet/opus

### Sprint Plan Format (v1.0)
All sprint development plans follow the standardized 12-section format.
Recall: `POST /v1/skills/sprint-planning/recall` or `filter_tags: ["skill:sprint-planning"]`.
Required sections: Header & Metadata, Problem Statement, Scope & Constraints,
Dependencies, Implementation Plan (sequential epics + gates), Testing Plan (3 tiers),
Commit Strategy, Verification Checklist, Documentation Update (final epic — never cut),
Risks & Mitigations, Documents Accessed. Optional: Rollback Procedures (destructive ops).

## Project Context

**MDEMG** — Cognitive substrate for AI-assisted development. Persistent emergent long-term memory via Hebbian learning, 5-layer hierarchy, RSIC self-improvement. 105 core phases + sidecar phases complete.

### Key Directories

- `internal/retrieval/` - Core retrieval pipeline
- `internal/hidden/` - Hidden layer/concept abstraction
- `internal/api/` - HTTP API handlers
- `internal/ape/` - RSIC self-improvement engine
- `internal/jiminy/` - Jiminy inner voice guidance
- `internal/cli/` - Unified CLI commands

## Additional CLI Commands

- `mdemg data export-auto` — automated daily export with retention (`--keep N`), `latest.tar.gz` symlink
- `mdemg data check --pre-campaign` — 8 validation checks (schema, instance ID, task coverage, etc.)
- `mdemg tsdb status` — TimescaleDB connection and schema version
- `mdemg tsdb migrate` — apply pending TSDB schema migrations
- `mdemg synergy status` — Claude Code ↔ MDEMG synergy health
- `mdemg upgrade` — self-update binary + all running Docker instances
- `mdemg upgrade --docker-only` — update Docker instances only (used by brew post-install)
- `mdemg upgrade --no-docker` — update binary only, skip Docker
- `mdemg graph repair --space-id <id>` — weight-preserving SymbolNode dedup, vendor cleanup, orphan sweep, embedding backfill
- `mdemg maintenance --space-id <id>` — combined decay + prune cycle (schedulable via cron/launchd)
- `mdemg embeddings backfill --space-id <id>` — fill missing embeddings on MemoryNodes

## Teardown
- `mdemg teardown --export` backs up TSDB (pg_dump) before destroying Docker volumes (Phase 0b)
- `mdemg teardown` detects docker-compose.yml and runs `docker compose down -v`
- Backup preserved in `.mdemg-backup-{ts}/backups/tsdb/`

## Multi-Instance
- See `docs/user/multi-instance.md` for running multiple instances
- Each instance: 5 containers, 6 ports, ~2.3 GiB RAM (fresh)
- COMPOSE_PROJECT_NAME=mdemg-{dirname} provides isolation
- Known limitation: LaunchAgent labels not instance-scoped

## Graph Health (v0.6.0 — Complete)
- BUG-1 (SymbolNode dedup): Fixed — natural-key MERGE + V0023 uniqueness constraint
- BUG-2 (vendor nodes): Fixed — `prune --match-ignore` + `graph repair` vendor cleanup
- BUG-5 (dual decay): Fixed — unified evidence-weighted formula, decay-rate default 0.02
- BUG-6 (prune label scope): Fixed — `--include-labels` flag
- V0023 migration self-heals: dedup before constraint, safe on any graph
- Hidden layer OOM: Fixed — batched orphan HiddenPattern deletion
- `QUERY_CLASSIFY_ENABLED` compose default changed from `false` to `true`

## Campaign Configuration

These env vars are forwarded in the compose template. Set in `.env`, or enable via `mdemg init` interactive prompt:

- `QUERY_CLASSIFY_ENABLED` — LLM query type classification (default: true)
- `INTENT_ENABLED` — query rewriting before embedding (default: false)
- `JIMINY_ENABLED` — Jiminy inner-voice guidance (default: false)
- `EMERGENCE_ENABLED` — LLM-driven concept naming (default: false)
- `LLM_INTERACTION_LOGGING` — TSDB LLM interaction recording (default: true)
- `AUTO_MIGRATE` — unified Neo4j + TSDB schema migration on startup (default: true in Docker)

## Architecture Notes

- **Compose embed:** `mdemg init` writes `docker-compose.yml` from `internal/cli/compose_templates/` (both files must stay in sync — CI checks this)
- **LaunchAgent embed:** `mdemg service install` reads templates from `packaging/launchd/` embedded via `embed.FS`
- **LLM recorder init order:** `llmclient.SetDefaultRecorder()` MUST be called BEFORE `api.NewServer()` — clients capture the recorder at construction time. See `internal/cli/serve.go` early writer block.
- **Context helpers:** `WithSpaceID(ctx, id)` and `WithSessionID(ctx, id)` carry request-scoped values to TSDB recording, overriding construction-time defaults in `recordInteraction()`

## Testing

- Benchmark: `python docs/benchmarks/run_benchmark_v4.py` / `grader_v4.py`
- Question set: `test_questions_120.json` (120 questions)
- Live validation: `python3 scripts/live_validation.py` (19 end-to-end tests)
- Synergy: `mdemg synergy status` | `mdemg synergy check --auto` | `mdemg synergy migrate --dry-run`
- Synergy API: `GET /v1/synergy/status?space_id=mdemg-dev`

---

## Enforced Protocols (Hook-Backed)

Hooks in `.claude/hooks/` run automatically — they are not optional.

- **`session-start.sh`**: Resumes CMS memory, RSIC health, synergy fingerprint, Jiminy warning
- **`prompt-context.sh`**: Recalls CMS context + Jiminy guidance per prompt
- **`post-tool-observe.py`**: Auto-captures decisions, errors, progress, MEMORY.md overflow
- **`pre-compact.sh`**: Saves context snapshot to CMS, Jiminy health check, J17 ticket
- **`pre-bash-check.py`**: Blocks destructive operations (DB destruction, rm -rf, force push, Cypher deletes). Must ask user for confirmation if blocked.

### Decision Protocol: irreversible → ask user. Reversible → check CMS preferences. Always observe decisions.
### Communication Protocol: state what + why before every action. Confirm data modifications.
