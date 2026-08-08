# HITL-CURATION-003 — Sprint Plan

## 1. Header & Metadata

- **Sprint:** HITL-CURATION-003
- **Date:** 2026-08-08
- **Branch:** `reh3376_dev01`
- **Author:** Roger Henley (via Claude Opus 4.7)
- **Parent context:** Q4 roadmap §3 #4 (`direct corpus lever`). Direct successor to HITL-CURATION-002 (autograder substrate), HITL-AUTO-DISMISS-001 (`NonReinforcingApplier` interface), AUTOGRADE-SCHEDULE-001 (scheduled loop). Named as the corpus-quality lever by JIMINY-FOLLOW-RATE-REMEASURE-001 (2026-08-08).
- **Effort:** 1 day (7 sequential epics, 3 code + 1 config + 1 tests + 1 live smoke + 1 docs).

## 2. Problem Statement

The follow-rate ceiling is corpus-bound. JIMINY-FOLLOW-RATE-REMEASURE-001 verified the substrate is stable at ~11.6% follow rate (vs the ceiling investigation's 35-50% prediction). Every classifier-side lever has been pulled. The remaining lever is HITL curation of the guidance corpus itself — but the shipped autograde-scheduler covers only `contradicted_drafts`, not `guidance`.

Two blockers prevented naive extension to guidance:

1. **`GuidanceSink` doesn't implement `NonReinforcingApplier`**. The handler at `handlers_review.go:314-332` silently no-ops the sink dispatch when `reinforce=false` (scheduler default) AND the sink doesn't implement the interface. So today, adding `guidance` to `REVIEW_AUTOGRADE_SCHEDULE_DATASETS` would write `review_grades` rows and do nothing else.

2. **Handler always records the grade row.** Even after Epic 1's gating (make ApplyNonReinforcing refuse reinforceable verdicts), the handler at `handlers_review.go:341` unconditionally called `reviewWriter.Record(...)` after the sink dispatch. Combined with `guidanceDataset.FetchCandidates`' current-rubric-version filter, this would silently drain the operator queue of every autograded row regardless of whether the sink handled it — the exact class the Plan-agent validation caught.

## 3. Scope & Constraints

- **In-scope:** Extend `GuidanceSink` with `ApplyNonReinforcing` gated to dim==2; add per-dataset autograde prompt hint; sample-strategy switch for starvation-free scheduled backfill; handler skips `Record` when sink refuses (Epic 1's queue-drain guard); pin tests; live Tier-3; docs + arch rule.
- **Out-of-scope (deliberate):** Scheduling `llm:*` datasets (~3200 LLM calls/day, deferred to separate sprint after guidance cost/value is measured); changing `REVIEW_AUTOGRADE_SCHEDULE_DATASETS` code default (behavior-changing-on-upgrade class per HEBB-ETA-001 rule); adding a metric for `hitl_autograde_noop_total{dataset="guidance"}`.
- **Invariants preserved:** HITL-CURATION-002 substrate-mutation invariant (auto-grades never mutate substrate); AUTOGRADE-SCHEDULE-001 `--force`-forbidden invariant; NonReinforcingApplier contract from HITL-AUTO-DISMISS-001.

## 4. Dependencies

- HITL-REVIEW-001 (platform), HITL-CURATION-002 (autograder), HITL-AUTO-DISMISS-001 (NonReinforcingApplier interface), AUTOGRADE-SCHEDULE-001 (scheduled loop). All shipped.
- No schema changes.
- `guidance_training_rows` populated on mdemg-dev (verified: 1618 actionable rows / 7d).
- `constraint_outcomes` LLM autograder wired (verified via HITL-CURATION-002 dry-runs).

## 5. Implementation Plan (sequential — per CLAUDE.md `sequential-epics` rule)

**Epic 1** — Add `ApplyNonReinforcing` to `GuidanceSink`. Return `handled=true` ONLY when `correctedOutcome(g) != ""` fails (dim==2 unclear); dims 0/1/3/4 return `handled=false`. Verb `guidance:autograde:noop:unclear`.

**Epic 2** — Add `AutogradePromptHint()` on `guidanceDataset`. Teach outcome_type / guidance_type / outcome_label_correctness semantics.

**Epic 3** — Add `SampleStrategy` to `CandidateQuery`; `guidanceDataset.FetchCandidates` switches ORDER BY. CLI `--sample-strategy` flag; scheduler subprocess passes `oldest-ungraded`. Handler `sample_strategy` query param passthrough.

**Epic 4** — Config comment update on `ReviewAutogradeScheduleDatasets` (naming guidance as opt-in). Keep code default `contradicted_drafts`.

**Epic 5** — Pin tests: `TestGuidanceSink_ApplyNonReinforcing_OnlyUnclearIsHandled`, `_ZeroSubstrateMutation`, `_Apply_UnchangedByHITLCuration003`, `_Reverse_AutogradeNoop_IsNoop`, `TestGuidanceDataset_AutogradePromptHint_NonemptyAndCoversTypology`, `TestAutogradeScheduler_RunOne_AlwaysOldestUngraded`.

**Epic 6** — Live Tier-3 smoke on mdemg-dev.

**Epic 7** — Docs: feature doc, CLAUDE.md pin, CHANGELOG, sprint_plan + post.

⚠️ **Discovered mid-execution (Epic 6):** The handler at `handlers_review.go:341` unconditionally recorded the grade row after sink dispatch. Even with Epic 1's gating, this would drain the operator queue. Fixed inline as part of Epic 1's implementation surface: handler skips `reviewWriter.Record` when `reinforce=false` AND `handled=false`. Response payload gains `grade_recorded: bool` field so the autograder can distinguish "written to corpus" from "skipped as reinforceable."

## 6. Testing Plan

**Tier 1 — Unit tests (colocated):**
- `internal/review/sink_guidance_test.go` (+4 tests): ApplyNonReinforcing gating semantics + zero-mutation invariant + regression pin on Apply + Reverse-noop-safe.
- `internal/api/guidance_dataset_test.go` (new file, +1 test): AutogradePromptHint typology coverage.
- `internal/review/schedule_test.go` (+1 test): scheduler subprocess always passes `--sample-strategy=oldest-ungraded`.
- Existing `TestAutogradeScheduler_RunOne_BuildsCorrectArgs` extended to include new flag in `want`.

**Tier 2 — Integration:** Deferred — the 409/force flow is exercised in Tier 3.

**Tier 3 — Live Tier-3 smoke against mdemg-dev.** Executed 2026-08-08; results in Epic 6 of the post. Verified: sample-strategy ordering; dry-run autograde with hint spliced; real autograde on dim=4 rows produces zero grade rows (handler correctly skips Record); rows stay operator-actionable; synthetic dim=2 grade lands with verb `guidance:autograde:noop:unclear` and correctly filters from queue; cleanup smoke row removed.

## 7. Commit Strategy

Single commit landing all 7 epics: `feat(hitl-curation-003): guidance dataset joins auto-grade schedule (invariant-preserving, starvation-free)`.

## 8. Verification Checklist

- [x] `go build ./...` clean
- [x] `golangci-lint run ./internal/{review,api,cli,config}/...` = 0 issues
- [x] `go test ./internal/review/... ./internal/api/... -count=1` all pass
- [x] Live smoke on mdemg-dev: sample-strategy, dry-run, real autograde with sink-refuse, dim=2 sink-handle, queue-visibility preservation, zero substrate mutation, cleanup
- [x] Handler queue-drain guard verified live (0 grade rows landed for 2 dim=4 autograde POSTs)
- [x] `docs/features/hitl-auto-curation.md` extended
- [x] `CLAUDE.md` pin added for the new NonReinforcingApplier arch rule
- [x] `CHANGELOG.md` Unreleased entry

## 9. Documentation Update

Files updated (Epic 7):
- `docs/features/hitl-auto-curation.md` — new subsection "Extending to the guidance dataset"
- `CLAUDE.md` — new arch rule pin
- `CHANGELOG.md` — Unreleased entry
- `docs/development/hitl-curation-003/sprint_plan.md` — this file
- `docs/development/hitl-curation-003/post.md` — sprint post with live results

## 10. Risks & Mitigations

**R1:** Autograded rows for reinforceable-verdict guidance still don't land in the corpus (no gold row). Reduces corpus growth vs the naive-drain design.
*Mitigation:* This is the correct trade — reinforceable verdicts SHOULD stay operator-actionable. Corpus growth for those rows happens when the operator confirms them (whichever way). Non-reinforceable (dim==2) rows DO land in the corpus via ApplyNonReinforcing.

**R2:** The handler's `grade_recorded=false` response might confuse autograder callers that expect a grade to always land.
*Mitigation:* Field is additive; existing callers read `reinforcement_applied` which is unchanged. Documented in feature doc.

**R3:** `oldest-ungraded` strategy pulls very old rows (2026-06-24) that may reference expired/removed source_node_ids, causing sink lookups to fail.
*Mitigation:* GuidanceSink.Apply already tolerates missing node_id (see sink_guidance.go:61-62). Zero-mutation path in Epic 1 doesn't touch nodes at all.

## 11. Rollback Procedures

- Revert the commit; no schema changes; no persistent state changes on mdemg-dev (Epic 6 smoke cleaned up its own row).
- If needed, disable ApplyNonReinforcing pathway by reverting `GuidanceSink`'s method — falls back to the pre-sprint silent-no-op handler behavior (which was also invariant-preserving, just without the intended corpus lever).
- No `.env` changes shipped.

## 12. Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q4.md` §3 #4, §5
- `docs/development/jiminy-follow-rate-remeasure-001/verdict.md`, `post.md`
- `docs/development/hitl-curation-002/` (arc predecessor)
- `docs/development/hitl-auto-dismiss-001/` (NonReinforcingApplier introduction)
- `docs/development/autograde-schedule-001/`
- `internal/review/sink.go`, `dataset.go`, `sink_guidance.go`, `schedule.go`, `autograder.go`
- `internal/api/guidance_dataset.go`, `contradicted_drafts_dataset.go`, `handlers_review.go`, `llm_dataset.go`
- `internal/cli/review.go`
- `internal/config/config.go`
- Plan file: `/Users/reh3376/.claude/plans/shimmying-jingling-hamming.md`
- Live queries + curl output against mdemg-dev during Epic 6
