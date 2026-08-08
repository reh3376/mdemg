# HITL-CURATION-003 — Sprint Post

**Date:** 2026-08-08
**Branch:** `reh3376_dev01`
**Type:** Feature — extends HITL auto-grading to the guidance dataset (invariant-preserving, starvation-free) + closes a discovered handler-side queue-drain bug.

## Summary

Extended the HITL auto-grading substrate to cover the `guidance` dataset (`guidance_training_rows` — the corpus the follow-rate ceiling depends on). Requires opt-in via `REVIEW_AUTOGRADE_SCHEDULE_DATASETS=contradicted_drafts,guidance` — the code default stays `contradicted_drafts` only per HEBB-ETA-001's "behavior-on-upgrade must be opt-in" rule.

Also fixed a discovered handler-side queue-drain bug: `handlers_review.go` was unconditionally recording every grade submission — even when `NonReinforcingApplier` refused (`handled=false`) — which would silently drain the operator queue for zero corpus benefit. Now `Record` fires only when reinforce=true OR sink handled the grade.

## What we shipped

1. **`GuidanceSink.ApplyNonReinforcing`** (`internal/review/sink_guidance.go`) — implements the interface added by HITL-AUTO-DISMISS-001. Returns `handled=true` ONLY when `correctedOutcome(g) == ""` (dim==2 unclear — where `Apply` itself returns `noop:unclear`). Dims 0/1/3/4 (mutation-worthy verdicts) return `handled=false` so the row stays operator-actionable.

2. **`guidanceDataset.AutogradePromptHint()`** (`internal/api/guidance_dataset.go`) — per-dataset typology hint teaching the autograder the guidance dataset's semantics: outcome_type enum, guidance_type strata (actionable vs abstract), how to judge the `outcome_label_correctness` rubric axis.

3. **`SampleStrategy` on `CandidateQuery`** (`internal/review/dataset.go` + `guidanceDataset.FetchCandidates` + `internal/cli/review.go` + `internal/review/schedule.go` + `internal/api/handlers_review.go`) — new `oldest-ungraded` sample strategy prevents low-classifier-confidence tail rows from starving under the scheduled autograder's MinConfidence gate. Scheduler always requests `oldest-ungraded`; interactive CLI defaults to `newest`.

4. **Handler queue-drain guard** (`internal/api/handlers_review.go::handleReviewGrade`) — `reviewWriter.Record` now runs only when reinforce=true OR the NonReinforcingApplier returned `handled=true`. Response payload gains `grade_recorded: bool` field so autograder callers can distinguish "written to corpus" from "skipped as reinforceable / operator-only."

5. **Config comment update** (`internal/config/config.go`) — `ReviewAutogradeScheduleDatasets` field comment names `guidance` as an operator-opt-in dataset.

6. **6 pin tests** (`internal/review/sink_guidance_test.go` + `internal/api/guidance_dataset_test.go` + `internal/review/schedule_test.go`) — gating semantics + zero-mutation invariant + Apply regression + Reverse-noop safety + hint typology coverage + scheduler always-oldest-ungraded.

7. **Docs** (this dir + feature doc + CLAUDE.md + CHANGELOG).

## Live Tier-3 results (mdemg-dev, 2026-08-08 T13:37 UTC)

| Verification | Expected | Actual | Result |
|---|---|---|---|
| `/v1/review/candidates?sample_strategy=oldest-ungraded` | Rows time-ASC | Rows from 2026-06-24 | ✓ |
| `/v1/review/candidates` default | Rows time-DESC | Rows from 2026-08-08 | ✓ |
| Dry-run autograde with hint | 2 confident-auto, ~2.3KB hint | conf=0.90 both, 2325-char hint | ✓ |
| Real autograde on dim=4 rows | Sink refuses, zero grade rows | 0 grade rows in 5min | ✓ |
| Rows STAY visible after refuse | Both still in queue | Both still visible | ✓ |
| Zero substrate mutation | No trust/confidence movement | 0 reinforcement_events | ✓ |
| Synthetic dim=2 grade | Sink handles, row lands, verb=noop:unclear | ✓ verb=`guidance:autograde:noop:unclear`, `grade_recorded=true` | ✓ |
| dim=2 row filtered from queue | Not visible | `NO (correctly filtered)` | ✓ |
| Cleanup smoke row | Removed | `DELETE 1` | ✓ |

## Discovered mid-sprint: handler-side queue-drain

The Plan-agent validation phase caught that Epic 1's ApplyNonReinforcing gating was insufficient by itself — the handler at `handlers_review.go:341` unconditionally called `reviewWriter.Record(...)` after the sink dispatch. So even if the sink returned `handled=false`, a `review_grades` row would land, and `FetchCandidates`' `LEFT JOIN review_grades ... AND r.rubric_version=$2` filter would then hide the row from the operator queue regardless.

The fix (folded into Epic 1's implementation surface): handler skips `Record` when `reinforce=false` AND `handled=false`. Response payload gains `grade_recorded: bool`. Live-verified: 2 real autograde POSTs against dim=4 rows produced 0 grade rows AND kept the rows visible in the operator queue.

⚠️ **New arch rule pinned to CLAUDE.md**: `NonReinforcingApplier` sinks alone can't defend against operator-queue drain — the HANDLER must also skip the grade-record write when the sink refuses. Otherwise the row lands, and `FetchCandidates` filters it out of operator view regardless of the sink's `handled=false` signal. Both layers are required.

## Why the "opt-in via .env" default was the right call

HEBB-ETA-001 established the rule: **behavior-changing defaults ship off in BOTH code AND `.env`** to avoid surprising operators on binary upgrade. Adding `guidance` to `REVIEW_AUTOGRADE_SCHEDULE_DATASETS`' code default would silently expand what runs every 6 hours for operators who've already opted into the scheduler with the current single-dataset mental model. The right shape is: operators explicitly opt in via `.env` after reading the docs.

Follow-up sprint (not this one) can measure cost/value from 1-2 weeks of live guidance-scheduling and, if favorable, flip the code default.

## Attribution vs quality (arch clarification)

JIMINY-FOLLOW-RATE-REMEASURE-001 pinned that **attribution shipments** (CORRECTION-CODE-GEN-001, this sprint's autograded-row corpus growth) enable measurement fidelity but don't move compliance rates. HITL-CURATION-003 is the same class: it grows the corpus for retrain, and reduces operator-triage cost via the dim==2 auto-dismiss path — but does NOT directly lift follow rate. That requires operators to actually GRADE the reinforceable-verdict rows (dim ≤1 or ≥3) so the substrate learns from confirmed corrections.

## Non-goals (explicit exclusions)

- **NOT scheduling `llm:*` datasets.** Defer to a separate sprint after measuring the guidance schedule's real cost/value.
- **NOT changing `REVIEW_AUTOGRADE_SCHEDULE_DATASETS` code default.** Opt-in via `.env` only.
- **NOT adding `hitl_autograde_noop_total` metric.** Nice-to-have; add if operator asks after 1-2 weeks.

## Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q4.md` §3 #4, §5
- `docs/development/jiminy-follow-rate-remeasure-001/verdict.md`, `post.md`
- `docs/development/hitl-curation-002/`, `hitl-auto-dismiss-001/`, `autograde-schedule-001/`
- `internal/review/{sink,dataset,sink_guidance,schedule,autograder}.go`
- `internal/api/{guidance_dataset,contradicted_drafts_dataset,handlers_review,llm_dataset}.go`
- `internal/cli/review.go`
- `internal/config/config.go`
- Live queries + curl output against mdemg-dev (Epic 6)
- Plan file: `/Users/reh3376/.claude/plans/shimmying-jingling-hamming.md`
