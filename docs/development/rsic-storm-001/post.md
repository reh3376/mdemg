# RSIC-STORM-001 — Sprint Post

Closed: 2026-06-11 · Branch: `reh3376_dev01` · Off-roadmap, earned by live
evidence (storm re-confirmed during SUPERVISOR-002 and
BACKUP-RESTORE-VERIFY-001 drills).

## Shipped

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Plan with corrected incident attribution | `815b909` |
| A | Atomic trigger admission (reserve-on-allow) | `33e373d` |
| B+C | Attributable archival + cooler cap + unified tombstone predicate (shared Cypher const) | `c9edf33` |
| D | 952 non-error cooler-tombstone victims recovered + graduated | (live op, audit-trailed) |
| E | Tier 3: storm-rate, rollback drill, cap re-fire, recovery, LLM health | `verification.md` |
| F | Feature doc + CHANGELOG + CLAUDE.md + post | (docs commit) |

## Headline numbers (live)

- Micro cycles: ~47 per 4-minute window → **1**; rejections now real
  (97 cooldown + 35 overlap in the same window).
- LLM: **0 errors** since the fix (intent_translate had failed all day).
- Rollback: `RestoredCount` 0 → **3/3** in the live drill.
- Cooler: cap bound at exactly **500** with loud warning (incident sweep
  was 5,397 uncapped); all 500 were error-debris.
- Recovery: **952** observations back in the live graph, graduated,
  one-query reversible via `recovered_reason`.

## Corrected incident record

The 2026-06-11 "Significant Node Count Drop" was the **Context Cooler**
draining its pre-DH-004 volatile backlog via the session-start hook's
graduate call — not RSIC. The mis-attribution itself was caused by the
two defects this sprint fixed (bare `is_archived` tombstones + the
`archive_reason`/`archived_reason` naming split). RSIC's own tombstones
totaled 54 nodes (its RSIC-VALIDATE-001 constraint works).

## Follow-ups (disclosed, not started)

- **RETRIEVE-CTX-001**: `retrieval.intent_translate` ctx-cancellation
  class on the recall path (hook needs its response; detaching has a
  different tradeoff than FEEDBACK-CTX). Storm removal may have already
  reduced it to noise — measure first.
- RSIC LLM-reflector truncated-JSON failures: re-measure under post-storm
  load before any fix.
- The remaining error-debris backlog drains at cooler cap rate
  (~500/graduate-run) — no action needed.

## Documents Accessed

See `sprint_plan_rsic_storm_001.md` §11 + `verification.md`.
