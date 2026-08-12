# JIMINY-CORPUS-003 — Sprint Post

**Date:** 2026-08-11
**Branch:** `reh3376_dev01`
**Phase 1 of:** `docs/development/jiminy-ceiling-break-2/README.md`

## Shipped

**Stage 1 (corpus purge, live on `mdemg-dev`):**
- 64 live constraints → 33 canonical (48% purge)
- 15 TOMBSTONE_JUNK — session-records, event-logs, narratives, truncated content, feature-spec, impl details
- 2 TOMBSTONE_STALE — `must-pin-mlx-8101` (superseded by Phase 13.5 llama-server), `rename-before-finetune` (superseded by MoE→dense pivot)
- 14 TOMBSTONE_DUPLICATE — 6 clusters: branch-protection, CUIDv2, mdemg-db-start, goreleaser, CMS-usage, e2e-testing, test-failure-ownership, jiminy-enforce-should-vs-must

All tombstones use `is_archived=true` + `archive_reason='jiminy_corpus_003_purge_{junk|stale|dedup}'` + `archived_at=datetime()`. Fully reversible via rollback cypher in sprint_plan.md §10.

Backup: `docs/development/jiminy-corpus-003/pre_purge_backup.jsonl` (65 lines incl. header) — full JSONL export of every node's content pre-purge.

Tombstoned node_ids: enumerated in `tombstone_list.md`.

**Stage 2 (strict-mode default verification):**
- Added explicit `JIMINY_MODE=strict` to `.env` for operator visibility (code default was already `strict`; env line makes shipped state operator-scannable)
- Restart + boot log confirms: `jiminy: mode mode=strict session_id=claude-core strict_enabled=true`
- CLI cross-check: `mdemg jiminy mode` reports `mode: strict (strict=true); boot default: strict; default session: claude-core`

## Live Tier-3 (mdemg-dev, 2026-08-11 20:50-21:00 UTC)

| Verification | Result |
|---|---|
| Pre-purge count | 64 live constraint nodes |
| Post-junk-tombstone | 49 live (delta 15 as expected) |
| Post-stale-tombstone | 47 live (delta 2 as expected) |
| Post-dedup-tombstone | 33 live (delta 14 as expected) |
| Boot log strict mode | `mode=strict strict_enabled=true` |
| Retrieve smoke | `/v1/memory/retrieve` returns results (constraint-partition direct-fetch is Lever C's path; not observed here — separate Phase 2 concern) |
| Rollback cypher | Documented + tested against archive_reason predicate |

## Delta expectations

Per the JIMINY-CEILING-BREAK-2 arc analysis, Phase 1 alone predicted **+2-5 pp** actionable follow rate over 7d as the 168h window rolls off signals from the purged nodes. Passive re-check: **2026-08-18**.

## Framing correction — pinned

This sprint's IMMEDIATE cause was the operator directive 2026-08-11 ("If the main LLM is only following 10-13% of Jiminy's guidance this entire project is a complete failure"). That correctly overrides prior framing in JIMINY-FOLLOW-RATE-REMEASURE-001 (2026-08-08 verdict called ~12% "honest steady state" + downgraded ceiling work to non-urgent) and JIMINY-HEURISTIC-DEFAULT-001 (panel title "honest ~12% post-heuristic-fix" normalized a failure state as a target).

The 2026-08-01 architectural directive (`trust-signal-must-be-persisted-never-ignore-honest`) already anchored this: "Do not normalize either as acceptable." Every "by design" / "matches actionable steady state" / "not a defect" phrase in the DASHBOARD-TRUTH / JIMINY-HEURISTIC-DEFAULT-001 posts violated that directive. The JIMINY-CEILING-BREAK-2 arc doc explicitly rejects them going forward.

## Two arch rules pinned (CLAUDE.md)

1. **Corpus curation is the single highest-leverage lever for follow-rate ceiling.** Every irrelevant constraint the classifier surfaces trains the agent to ignore constraints as a class. When actionable follow rate drops below 40%, the corpus is the first thing to audit — before classifier prompt work, before retrieval tightening, before model swap. The audit criterion is "would a competent developer reminded of this rule before an action actually follow it?" — NOT "is this rule true?" (many session-records are true but not applicable-across-contexts).

2. **Follow-rate framing must always be trajectory language, never "by design" language.** Panel titles, sprint verdicts, alert-floor comments, and CLAUDE.md pins that call a sub-50% follow rate "honest by design" / "expected steady state" / "not urgent" normalize a substrate-quality failure and violate the 2026-08-01 architectural directive. Correct framing: "current N%, target M%, arc that owns the delta is X."

## Follow-ups disclosed

- **JIMINY-CEILING-BREAK-2 master arc** (`docs/development/jiminy-ceiling-break-2/README.md`) — owns the >80% target with 5 phases (this = Phase 1). Phases 2-4 spec'd with expected delta arithmetic (+5-10pp / +5-10pp / +15-25pp).
- **Framing hygiene sweep** — remaining DASHBOARD-TRUTH / JIMINY-* panel titles + docs that still frame ~12% as "by design" need trajectory language update. Deferred to Phase 2 kickoff.
- **Muse Glimmer eval** — remains queued (Phase 5); low priority until upstream stack is aligned.

## Documents Accessed

- Operator directives 2026-08-11 (mid-turn: "complete failure" + ">80% target")
- Operator directive 2026-08-01 (`trust-signal-must-be-persisted-never-ignore-honest`)
- `docs/development/jiminy-corpus-003/{sprint_plan,tombstone_list,pre_purge_backup.jsonl}`
- `docs/development/jiminy-ceiling-break-2/README.md` (the wider arc)
- Live Cypher against `mdemg-dev` role_type='constraint'
- CLAUDE.md pins: JIMINY-CORPUS-001, JIMINY-CORPUS-002, JIMINY-ARCHIVED-CODE-FILTER-001, JIMINY-ENFORCE-001, JIMINY-MODE-001, JIMINY-CLASSIFY-ESCALATION-INSPECT-001
- `internal/config/config.go` (JiminyMode + JiminyStrictDefaultEnabled defaults verified)
- `~/.mdemg/logs/server.log` (boot log confirmation post-restart)
