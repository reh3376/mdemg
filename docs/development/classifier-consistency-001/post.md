# CLASSIFIER-CONSISTENCY-001 — Sprint Post

**Date:** 2026-07-28 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive Finding 2.

## Verdict

**Shipped in reduced scope after investigation reframe.** One alert rule +
config + serve wire + tests + docs, single commit. The deep-dive's 25%
"chronic" symptom was a single 2026-07-21 burst; the sprint's residual
value is durable observability against future recurrences.

## What the investigation surfaced

1. **BOTH `constraint_outcomes` and `guidance_training_rows` writers use
   the SAME `cr.Source`** from ONE `Service.processOutcome()` classifier
   call. Fresh writes to both tables are byte-identical on the
   classifier_source field.

2. **The 25% vs 0.03% split comes ENTIRELY from GUIDANCE-AUDIT-001's
   asymmetric retroactive relabeling** — the audit fires on training_rows
   only, because the retrain pipeline consumes training_rows and
   constraint_outcomes is telemetry-only.

3. **The 25% headline was ONE burst**: 2026-07-21 had 451 heuristic rows
   (35.5% of 1,272 rows that day). The 6 days on either side averaged 0-1%.
   Root cause: 8 sprints shipped simultaneously → LLM saturation →
   classifier fallback path exercised. The fix for the class shipped the
   same day (LLM-HEALTH-CANCELLATION-ALERT-001).

The deep-dive was **right about the class** of failure (heuristic fallback
under saturation) and **wrong about the tempo** (transient burst, not
chronic).

## What shipped

Single commit under `CLASSIFIER-CONSISTENCY-001` slug:

- `alert.HeuristicShareRule(threshold, lookbackHours)` factory
- 2 config knobs: `HEURISTIC_SHARE_THRESHOLD` (0.05), `HEURISTIC_SHARE_LOOKBACK_HOURS` (24)
- Serve wire alongside HITL-CURATION-002's `HITLCurationStalledRule`
- 2 targeted unit tests (defaults + custom params + SQL invariants)
- Sweep-test auto-coverage (`allRules()` walker updated)
- CHANGELOG entry + CLAUDE.md architectural note
- This post + sprint plan

## What was intentionally NOT shipped

- **Extending GUIDANCE-AUDIT to `constraint_outcomes`** — the operator
  raised this option; my analysis rated it marginal-value:
  (i) baseline heuristic share is 0-1% (any bias correction is
  below-noise-floor), (ii) constraint_outcomes is telemetry-only (no
  substrate-mutating reader — retrain uses training_rows), (iii)
  retroactive relabeling can't prevent future bursts (only real-time
  alerts do that). Deferrable-until-triggered — this alert IS the trigger
  signal.

## Testing

- **Tier 1 (unit):** `TestHeuristicShareRule_Defaults` +
  `TestHeuristicShareRule_CustomParams` — pin service, severity,
  threshold default, operator, COALESCE + NULLIF idle-safe SQL contract,
  ForDuration flap guard. Both green.
- **Tier 2 (contract):** `TestAllRules_NoLimitOneAntiPattern` +
  `TestAllRules_DistinctServicePerSeverity` auto-cover via `allRules()`
  walker update. `go build ./...`, `golangci-lint 0 issues`,
  `go test ./...` full green.
- **Tier 3 (live):** kickstart on new binary; rule count 29 → 30 (new
  rule loaded); direct SQL against mdemg-dev returns single non-NULL row
  `heuristic_share=0.00623` (0.62% actual, matches 24h breakdown
  2/321 rows); threshold=0.05, gt → CLEAR (correct — steady state well
  below burst threshold).

## Rules pinned

1. **Burst-vs-chronic distinction**: a deep-dive can be right about the
   CLASS of failure while wrong about the TEMPO. Before shipping a
   "chronic issue" fix, check the per-day trend, not just the
   aggregate-window slice. The 25% number here was 100% dominated by
   ONE day; the whole framing shifted.
2. **`constraint_outcomes` is TELEMETRY-only.** Consumers are dashboards
   and alert rules; the retrain pipeline consumes `guidance_training_rows`.
   When adding a downstream reader that requires classifier_source
   accuracy on outcomes, extending GUIDANCE-AUDIT becomes worthwhile —
   until then, the alert is sufficient.

## Documents Accessed

- `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md` (parent
  trigger, Finding 2)
- `docs/development/classifier-consistency-001/sprint_plan.md` (this dir)
- `internal/jiminy/{service,outcome_classifier}.go` (traced `cr.Source`
  propagation through both writers)
- `internal/tsdb/{constraint_outcomes_writer,guidance_training_rows_writer}.go`
  (confirmed both writers use the same field)
- `internal/api/guidance_audit.go` (traced asymmetric relabeling — the
  actual cause of the training_rows vs outcomes divergence)
- `internal/alert/rules.go` (ORPHAN-ALERT / NODE-DROP-CALIBRATION /
  HITL-CURATION-002-E4 patterns being mirrored)
- Live TSDB per-day trend query against `constraint_outcomes` (10 days)
- Post-restart log inspection for rule-count verification
