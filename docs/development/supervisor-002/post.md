# SUPERVISOR-002 — Sprint Post

Closed: 2026-06-11 · Branch: `reh3376_dev01` · Roadmap: Q3 Phase 2, first
"next in line" member.

## Shipped

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Sprint plan + authoritative loop inventory (27 loops audited) | `c6bb0d0` |
| 1 | Sliding-window restart budget, `Go()` late registration, nil-return semantics, 3 config knobs | `7aa869e` |
| 2 | 12 loops registered under the supervisor (api/ape/backup/tsdb/serve) | `72c80d9` |
| 3 | Rule-health meta-alert (Debug→Warn + per-rule/global verdicts) | `49f16b2`, `3836eed` |
| 4 | RSIC `llm_error_rate_spike` recency gate (`LastErrorAt`) | `0b93e5a` |
| 5 | Tier 3: 2× TSDB-stop drills, recency-gate live evidence, shutdown drain | `verification.md` |
| fix | Drill-caught: streak-relative global-outage discriminator | `28b0db0` |
| fix | Live-caught: feedback outcome processing detached from hook curl lifetime | `f3f50ad` |
| 6 | Feature doc + CHANGELOG + CLAUDE.md + post | (docs commit) |

## Tier 3 highlights (see verification.md)

- `supervisor: started workers=12` (was 3); both graceful-exit paths logged.
- Drill 1 caught a real flaw in my own Epic 3 design (freshness window
  misclassifies outage onset → 2 per-rule leaks); drill 2 after the
  streak-relative fix: **one** `alert-evaluator-degraded`, **zero** leaks.
- Recency gate live: 0 "Jiminy Pipeline Critical" since restart with the
  stale 02:00Z burst still in-window; the *fresh* `jiminy.evaluate_llm`
  failure correctly fired through — and turned out to be a real defect
  (657 ctx-cancellations/24h from the hook's 5s curl), fixed in `f3f50ad`.

## New config

`SUPERVISOR_MAX_RESTARTS` (3), `SUPERVISOR_RESTART_WINDOW_MIN` (60),
`SUPERVISOR_BACKOFF_BASE_SEC` (5), `ALERT_RULE_FAILURE_THRESHOLD` (3),
`RSIC_LLM_ERROR_RECENCY_MIN` (60, 0=off), `JIMINY_FEEDBACK_TIMEOUT_MS`
(60000, 0=unbounded).

## Disclosed scope decisions

- The 9 buffered TSDB event writers + metrics recorder + Jiminy trust
  persistence stay unsupervised here — flush observability is
  TSDB-CONSUME-001's deliverable; supervising without that telemetry is
  motion without observability.
- Insight 25 (latency regression) not recency-gated — 7-day-trend
  comparison lacks the stale-window mechanism.
- RSIC LLM-reflector can still *recommend* `alert_jiminy_critical`; with
  insight 26 gated, the report no longer presents stale spikes, removing
  the trigger observed live.

## Follow-ups

- `jiminy.evaluate_llm` health: post-fix success rate accrues with normal
  hook traffic; the (now-supervised) alert evaluator + LLM
  consecutive-failure alerts will surface any residual failure mode.
- TSDB-CONSUME-001 (writer flush observability) remains next in the
  roadmap's Phase 2 ordering alongside BACKUP-RESTORE-VERIFY-001 /
  UATS-GAP-001 per `ROADMAP_2026Q3.md` §3.

## Documents Accessed

See `sprint_plan_supervisor_002.md` §11 and `verification.md`.
