---
created: 2026-03-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: synergy
---

# Synergy Optimization

## Summary

**Feature**: Synergy Optimization (Claude Code <-> MDEMG)
**Summary**: Reduces token overhead ~60% between Claude Code and MDEMG by trimming context files, migrating content to CMS, adding a synergy health API and CLI, and integrating synergy as an RSIC health dimension.

## Vision & Goals

Claude Code's context window is finite. Every token spent on static memory files is a token not available for reasoning. Synergy optimization ensures MDEMG's Claude Code integration is token-efficient: context files are minimal pointers, real content lives in CMS, and the synergy health score tracks whether the integration is drifting back toward bloat.

## Current State

### Architecture

**Token Reduction:**
- `CLAUDE.md` trimmed 348 -> 124 lines
- `MEMORY.md` trimmed 220 -> 40 lines
- Auto-memory files reduced 14 -> 3
- 8 files ingested to CMS, 2 obsolete moved to archive

**SynergyHealth RSIC Dimension** — 7th RSIC dimension, weight `RSIC_HEALTH_WEIGHT_SYNERGY` (default 0.05 since DH-005). `scoreSynergy()` scorer evaluates file counts, line counts, CMS node totals, overlap ratio.

**RSIC Reflection Patterns #17-19:**

| # | Pattern | Severity |
|---|---------|----------|
| 17 | `synergy_jiminy_unhealthy` | Critical |
| 18 | `memory_file_bloat` | Medium |
| 19 | `synergy_overlap_drift` | Medium |

**Confidence Score Normalization** — Discovered unbounded retrieval scores leaking into `GuidanceItem.Confidence`. New `internal/mathutil/` package with Clamp, Sigmoid, NormalizeScore. MaxConfidence cap at 0.95 (Bayesian ceiling).

### Workflow

**Hook Enhancements:**
- `post-tool-observe.py` — Memory overflow interceptor with Jiminy gate
- `session-start.sh` — Synergy fingerprint written to CMS
- `prompt-context.sh` — Token count footer appended
- `pre-compact.sh` — Jiminy health check before compaction

**Migration:** `scripts/synergy-migrate.sh` (Jiminy health gate, persistent flag, dev safety)

### Configuration

13 `SYNERGY_*` config vars in `internal/config/config.go` and `.env.example`.

## Notes

### Known Limitations

- Migration script must be run manually for existing installations
- Synergy health weight is config-driven (`RSIC_HEALTH_WEIGHT_SYNERGY`, default 0.05 per DH-005 — LOW reliability, file-size proxy)

### Risks & Gaps

None identified.

### Future Improvements

- Automatic synergy migration during `mdemg upgrade`
- Configurable RSIC dimension weights

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| GET | `/v1/synergy/status?space_id=<id>` | Synergy health metrics (file counts, line counts, CMS totals, overlap ratio) | `specs/synergy_status.uats.json` |

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg synergy status` | Display synergy health metrics |
| `mdemg synergy migrate` | Migrate content from files to CMS |
| `mdemg synergy check [--auto]` | Check and optionally fix synergy issues |

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `SYNERGY_*` (13 vars) | Various | Synergy optimization configuration (see `internal/config/config.go`) |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| RSIC Engine | Feeds into — SynergyHealth is RSIC dimension #7 |
| Jiminy | Requires — health gate for memory overflow |
| CMS | Requires — content migrated from files to CMS observations |
| Claude Code Hooks | Enhances — fingerprinting, token counting, overflow detection |

## Related Files

- `internal/api/handlers_synergy.go` - Synergy status handler
- `internal/cli/synergy.go` - CLI subcommands (~600 lines)
- `internal/ape/self_assess.go` - `scoreSynergy()` RSIC scorer
- `internal/ape/self_reflect.go` - Reflection patterns #17-19
- `internal/mathutil/normalize.go` - Clamp, Sigmoid, NormalizeScore
- `internal/jiminy/retrieval_source.go` - Sigmoid normalization
- `scripts/synergy-migrate.sh` - Migration script
- `docs/features/synergy-optimization.md` - Full optimization specification
