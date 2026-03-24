# Synergy Optimization (Claude Code ↔ MDEMG)

**Phase**: Synergy | **Status**: Complete | **Date**: 2026-03-24

## Overview

Reduces token overhead between Claude Code's persistent state (`.md` files) and MDEMG's CMS by ~60%, migrating displaced knowledge into the CMS graph and adding automated monitoring to prevent re-bloat.

## Problem

Claude Code loads CLAUDE.md (~3.5K tokens) on every turn and MEMORY.md (~3.5K tokens) at session start. With 14 auto-memory files (~36K bytes), total static overhead was ~7-8.5K tokens/session, with 30-40% content overlap with CMS.

## Solution

### Content Triage (Epic 1)

- **CLAUDE.md**: Trimmed from 348 → 124 lines (~65% reduction, ~2K tokens/turn saved)
- **MEMORY.md**: Trimmed from 220 → 40 lines (~82% reduction, ~2.5K tokens/session saved)
- **Auto-memory files**: Reduced from 14 → 3 files (~79% reduction)
- Archival content migrated to CMS via `scripts/synergy-migrate.sh`
- Dev safety: files moved to `~/mdemg/temp/` (not deleted)

### Migration Script

`scripts/synergy-migrate.sh` handles one-time content migration:
- Jiminy health gate (non-negotiable — refuses to run if Jiminy unhealthy)
- Ingests files to CMS via `/v1/conversation/observe` with `synergy-migration` tags
- Moves files to `~/mdemg/temp/{obsolete,migrated,archived}/`
- Writes persistent flag: `.mdemg/synergy-migrated.json`
- Supports `--dry-run`, `--force`, `--space-id`

### Automated Infrastructure (Epic 2)

**Memory Overflow Interceptor** (`.claude/hooks/post-tool-observe.py`):
- Detects MEMORY.md writes exceeding `SYNERGY_MEMORY_LINE_THRESHOLD` (default: 120)
- Checks Jiminy health before ingesting (skips if unhealthy — content stays in MEMORY.md as safety buffer)
- Auto-classifies overflow content and POSTs to CMS with `auto-overflow` tag
- Overflow observations enter CMS as volatile → naturally decay if unused

**Hook Enhancements**:
- `session-start.sh`: Synergy fingerprint (line counts, Jiminy health, migration status) → CMS
- `prompt-context.sh`: Token count footer `[synergy-meta: recall_tokens=N, guidance_tokens=M]`
- `pre-compact.sh`: Jiminy health check before compaction (warns if unhealthy + migrated)

### CLI Commands (Epic 2)

```bash
mdemg synergy status [--json] [--space-id]    # Display synergy health metrics
mdemg synergy migrate [--dry-run] [--space-id] # Run/re-run CMS migration
mdemg synergy check [--auto] [--space-id]      # Cron-compatible health check
```

### RSIC Monitoring (Epic 3)

**New Assessment Dimension**: `SynergyHealth` added as 7th RSIC dimension (10% weight).

Scoring factors:
- Jiminy health (unhealthy → 0.0 score)
- CLAUDE.md lines vs target
- MEMORY.md lines vs target
- Overflow event rate (24h)
- Overlap score

**New Reflection Patterns**:
- `#17 synergy_jiminy_unhealthy` (Critical) — Jiminy down + migration applied
- `#18 memory_file_bloat` (Medium) — overflow rate exceeds threshold
- `#19 synergy_overlap_drift` (Medium) — overlap score > 0.4

### API Endpoint

`GET /v1/synergy/status?space_id=<id>` — returns synergy health metrics.

## Configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `SYNERGY_MEMORY_LINE_THRESHOLD` | 120 | Overflow trigger threshold |
| `SYNERGY_MEMORY_AUTO_INGEST` | true | Auto-ingestion master switch |
| `SYNERGY_CLAUDE_MD_PATH` | auto-detect | Path to CLAUDE.md |
| `SYNERGY_MEMORY_MD_PATH` | auto-detect | Path to MEMORY.md |
| `SYNERGY_ASSESSMENT_ENABLED` | true | RSIC synergy dimension switch |
| `SYNERGY_TARGET_CLAUDE_LINES` | 150 | Target CLAUDE.md line count |
| `SYNERGY_TARGET_MEMORY_LINES` | 120 | Target MEMORY.md line count |
| `SYNERGY_OVERLAP_SAMPLE_SIZE` | 5 | Lines sampled for overlap check |
| `SYNERGY_OVERLAP_THRESHOLD` | 0.85 | Similarity threshold |
| `SYNERGY_OVERFLOW_ALERT_THRESHOLD` | 5 | Events/24h before RSIC alert |
| `SYNERGY_MAX_HOOK_TOKENS` | 500 | Max per-prompt hook tokens |
| `SYNERGY_CRON_INTERVAL` | 4h | Health check cron interval |
| `SYNERGY_CRON_ENABLED` | true | Cron health checks switch |

## Key Files

| File | Purpose |
|------|---------|
| `internal/api/handlers_synergy.go` | `/v1/synergy/status` endpoint |
| `internal/cli/synergy.go` | `mdemg synergy` CLI commands |
| `internal/ape/self_assess.go` | `scoreSynergy()` RSIC dimension |
| `internal/ape/self_reflect.go` | Patterns #17-19 |
| `internal/ape/types_rsic.go` | `SynergyHealth` report fields |
| `internal/config/config.go` | 13 `SYNERGY_*` config vars |
| `scripts/synergy-migrate.sh` | One-time CMS migration |
| `.claude/hooks/post-tool-observe.py` | Overflow interceptor |
| `.claude/hooks/session-start.sh` | Synergy fingerprint |
| `docs/api/api-spec/uats/specs/synergy_status.uats.json` | Contract test |

## Tests

- 3 handler tests (`internal/api/handlers_synergy_test.go`)
- 6 RSIC assess tests (`internal/ape/self_assess_synergy_test.go`)
- 8 CLI tests (`internal/cli/synergy_test.go`)
- 1 UATS spec (9 assertions, all passing)

## Token Savings

| Source | Before | After | Savings |
|--------|--------|-------|---------|
| CLAUDE.md (per turn) | ~3.5K tokens | ~1.5K tokens | ~2K/turn |
| MEMORY.md (session start) | ~3.5K tokens | ~0.5K tokens | ~3K/session |
| Auto-memory (selective) | ~36K bytes | ~4K bytes | ~90% reduction |

Over a 50-turn session: ~100K tokens saved on CLAUDE.md alone.

### Recovery Buffer (Store-and-Forward)

When Jiminy is down, observations that would normally flow to `mdemg-dev` are buffered in a two-tier store-and-forward system to prevent data loss:

**Tier 1 (CMS space)**: Observations written to `synergy-buffer` space via `/v1/conversation/observe`. Works when the MDEMG server is up but Jiminy is down. Observations get embedded at write time.

**Tier 2 (Local JSONL)**: When the MDEMG server is also unreachable, observations are appended to `.mdemg/synergy-recovery-buffer.jsonl` with FIFO eviction (max 50 entries).

**Auto-flush**: On session start, if Jiminy is healthy, buffered entries are automatically promoted: JSONL → synergy-buffer → mdemg-dev.

**CLI commands**:
```bash
mdemg synergy buffer-status [--json]        # Show pending buffer entries
mdemg synergy flush-buffer [--force] [--dry-run]  # Manual flush
```

**RSIC Pattern #20** (`synergy_recovery_buffer_pending`): Fires when buffer has pending entries. Medium severity at ≤20 entries, High above 20.

| Variable | Default | Purpose |
|----------|---------|---------|
| `SYNERGY_RECOVERY_BUFFER_SPACE` | `synergy-buffer` | CMS space for buffered observations |
| `SYNERGY_RECOVERY_BUFFER_PATH` | `.mdemg/synergy-recovery-buffer.jsonl` | Local JSONL fallback |
| `SYNERGY_RECOVERY_BUFFER_MAX_ENTRIES` | `50` | Max JSONL entries (FIFO eviction) |
| `SYNERGY_RECOVERY_AUTO_FLUSH` | `true` | Auto-flush on Jiminy recovery |

## Critical Prerequisite

**Jiminy must be healthy** before any .md pruning. If Jiminy is down after migration, CMS cannot surface the knowledge that was moved out of .md files — catastrophic forgetting risk. Pattern #17 is the loudest signal in the system for this scenario.
