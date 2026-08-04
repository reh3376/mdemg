# HITL-AUTO-DISMISS-001 — Sprint Post

**Date:** 2026-08-04 | **Branch:** `reh3376_dev01`
**Trigger:** Triage of MEDIUM `hitl-curation: MDEMG HITL Curation Stalled` alert (fired 2026-08-04T13:53:02Z, 9 pending drafts / 0 operator grades in 7d). Autograder confidently labeled 7 of 9 (4 dim=0 dismiss + 3 dim=4 approve) but wrote grade rows only — the draft `status` stayed `pending` because the sink was gated behind `reinforce=true`, treating dismissal (invariant-safe: no substrate mutation) the same as approval (invariant-blocked: mutates via `conversation.Correct`).

**Sibling ancestor:** HITL-CURATION-002 (the platform + autograder + invariant). This sprint plugs the specific design gap it left open.

## Verdict

Shipped. New `review.NonReinforcingApplier` optional interface lets sinks opt in to draining non-substrate-mutating verdicts under `reinforce=false`. `contradictedDraftsSink` implements it: dim=0 (dismiss) → `MarkDismissed` runs, returns `handled=true`; dim=4 (approve) → returns `handled=false` (Correct sink still requires operator grade). Handler wires the fall-through. Autograder CLI gains `--force` to backfill after sink-logic changes (was silently swallowing 409 idempotency errors).

**Live-verified**: 9 pending → 5 pending, alert stall_signal 9 → 5 (below the `gt 5` threshold; will clear on next 1h eval). No `reinforcement_applied=true` rows written — invariant intact.

## What shipped

### `internal/review/sink.go` — the opt-in interface
```go
type NonReinforcingApplier interface {
    ApplyNonReinforcing(ctx, g Grade) (detail ReinforcementDetail, handled bool, err error)
}
```
- Sinks that DON'T need this (guidance sink, LLM NoopSinks) don't implement it — no interface bloat.
- Contract: `ApplyNonReinforcing` is contractually restricted to operations that touch ONLY the dataset's own row/status, NEVER the cognitive substrate (Neo4j nodes, trust scores, edges, embeddings).
- Return `handled=false` for verdicts the caller must skip (approve/defer/unclear); `handled=true` for verdicts the sink drained.

### `internal/api/contradicted_drafts_dataset.go` — the sink implementation
`contradictedDraftsSink.ApplyNonReinforcing`:
- dim=0 (dismiss) → `writer.MarkDismissed(g.ItemID)`, returns `handled=true`, verb `correction_draft:dismiss:auto` (`:auto` suffix marks autograder-authored writes for auditability)
- dim=4 (approve), dim=2 (defer), unclear → returns `handled=false` (no mutation)
- nil writer → clean error, no panic

### `internal/api/handlers_review.go` — the dispatch
```go
if reinforce {
    detail, err := d.Sink().Apply(...)   // existing path — substrate mutation
} else if nra, ok := d.Sink().(review.NonReinforcingApplier); ok {
    detail, handled, err := nra.ApplyNonReinforcing(...)   // NEW — invariant-safe drain
    if handled { detailJSON = ... }
}
```
- `applied` (in `review_grades.reinforcement_applied`) stays `false` for the non-reinforcing path — the flag semantically means "did the substrate get mutated," which auto-dismiss never does. Detail JSON is populated so the audit trail is complete.

### `internal/cli/review.go` — `--force` flag
Autograder's `postAutoGrade` silently swallowed 409 idempotency responses ("item already graded at rubric_version"). Fine for organic backfill, but blocked backfill after a SINK-LOGIC change (the exact case this sprint creates). New `--force` flag passes `force:true` to `/v1/review/grade`, letting a re-issue drive the newly-non-reinforcing sink path.

## Tests

6 new pin tests in `internal/api/hitl_auto_dismiss_test.go`:
- `TestContradictedSinkImplementsNonReinforcingApplier` — interface-contract pin; a refactor that removes this capability MUST break this test
- `TestApplyNonReinforcing_DismissVerdictHandlesAndUpdatesStatus` — dim=0 → MarkDismissed + handled=true + `:auto` suffix + NO MarkApproved
- `TestApplyNonReinforcing_ApproveVerdictRefusedForInvariant` — dim=4 → handled=false + NO writer method fires (the invariant guardrail)
- `TestApplyNonReinforcing_DeferVerdictReturnsFalse` — dim=2 → handled=false, no mutation
- `TestApplyNonReinforcing_NilWriterErrors` — clean error, no panic
- `TestApplyNonReinforcing_WriterErrorPropagates` — writer error → handled=false + err surfaces

`go test ./internal/api/... ./internal/review/... ./internal/cli/...` clean; lint 0 issues.

## Live Tier-3 (mdemg-dev)

Real drain of the 9-draft queue that fired the alert:

**Pre-sprint** (post-rebuild, pre-`--force`):
- Autograder run POSTs → server 409s every item (idempotency) → 0 status changes → alert unchanged.

**Post-sprint** (with `--force`):
```
autograde summary: 7 confident-auto | 2 borderline (pending) | 0 errors

draft status: approved=2, dismissed=4, pending=5   (was: approved=2, pending=9)

review_grades (last 2 min):
  4 rows: dim=0, verb=correction_draft:dismiss:auto, reinforcement_applied=false  ← the drain
  2 rows: dim=4, reinforcement_detail=NULL,          reinforcement_applied=false  ← invariant guardrail
                    (approve verdicts correctly refused by ApplyNonReinforcing)

alert stall_signal: 9 → 5   (threshold: gt 5 → 5 is NOT > 5 → alert clears)
```

The 5 remaining pending drafts are: 3 real rules awaiting operator approval (MDEMG-dev space rule, no-direct-main-commit constraint, DATAPRUNE-AUDIT rule) + 2 borderline items the autograder couldn't confidently label (LOW conf). These are legitimately for operator attention — the alert would re-fire only if the operator lets them accumulate past the threshold.

## Rules pinned

⚠️ **Sinks with mixed reinforcing/non-reinforcing verdicts MUST implement `review.NonReinforcingApplier`** to let the autograder drain the non-mutating side. The "reinforce=false = no sink at all" pattern shipped in HITL-REVIEW-001 was correct as a starting invariant but leaves noise-drain capability on the table; extending via this interface preserves the invariant (approve/mutation stays operator-only) while unlocking the non-mutating drain path. When adding a new dataset whose sink has verdicts that touch only the dataset's own row, implement `NonReinforcingApplier` — do not extend `Apply` with a runtime `reinforce` flag (Apply's semantics are "substrate mutation," full stop).

⚠️ **The `:auto` suffix on `ReinforcementDetail.Verb` is the auditability contract for autograder-authored writes**, mirroring the `grader_id LIKE 'auto:%'` pin from HITL-CURATION-002. Anyone reading `reinforcement_detail` JSON can distinguish operator-driven applies from autograder-driven ones without re-joining `review_grades.grader_id`.

⚠️ **Autograder CLI `--force` is REQUIRED to backfill after any sink-logic change**, not just for operator-authored re-grades. The idempotency 409 was silently swallowed pre-sprint (correct default for organic re-runs; wrong default for backfill). When a sink's behavior for a given verdict changes, backfill = `mdemg review autograde --dataset X --force`.

## Not shipped (intentional)

- **Extending the pattern to `guidance` sink**: guidance sink verdicts (correctness dimensions) ALL trigger substrate mutation via `trust.Adjust`/`AdjustNodeConfidenceDirect` — no verdict is invariant-safe under `reinforce=false`. Adding `NonReinforcingApplier` there would be a no-op. Correctly skipped.
- **Autograder cadence scheduler**: this sprint drains what's queued, but doesn't schedule the autograder to run periodically. If the operator wants stall-alerts to self-heal for future accumulation, that's a separate sprint (`AUTOGRADE-SCHEDULE-001`, ~1h) — add a scheduled loop that runs `--force`-less autograde nightly under supervisor.
- **Reversing an auto-dismiss**: not adding a special path. `contradictedDraftsSink.Reverse` unconditionally calls `ResetToPending`, which correctly undoes both operator-approve (L0 obs stays; operator uses `mdemg concepts tombstone` for full undo) AND auto-dismiss (nothing to undo beyond the status flip).

## Follow-ups disclosed

- **AUTOGRADE-SCHEDULE-001** — scheduled autograde loop (default off; opt-in for automated stall prevention). Not urgent — operator can run manually.
- **JIMINY-CONTRADICTED-BRIDGE-QUALITY-001** — Sprint C in the immediate queue. Attacks the noise SOURCE (contradicted-bridge classifier writing Bash errors + Phase-N notes as drafts). Reduces future queue accumulation, complementary to this sprint's drain.

## Rollback

Single-commit revert. Auto-dismissed drafts stay dismissed (idempotent). The `NonReinforcingApplier` interface is optional — removing it leaves the platform in HITL-CURATION-002 shape (dismisses don't drain on auto-grade; operator drains manually).

## Documents Accessed

- `internal/review/sink.go` (interface added), `internal/review/dataset.go` (ReviewItem shape)
- `internal/api/contradicted_drafts_dataset.go:158` (existing sink)
- `internal/api/handlers_review.go:265` (grade dispatch, existing reinforce branch)
- `internal/cli/review.go:170` (autograde CLI + postAutoGrade)
- `internal/alert/rules.go:1229` (HITLCurationStalledRule contract)
- HITL-CURATION-002 + HITL-REVIEW-001 posts (parent sprints)
- Live `contradicted_correction_drafts` + `review_grades` state on mdemg-dev
