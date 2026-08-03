# ENFORCE-004-FOLLOWUP — Sprint Post

**Date:** 2026-08-03 | **Branch:** `reh3376_dev01`
**Parent:** JIMINY-ENFORCE arc — deferred follow-up for ENFORCE-004
**Trigger:** JIMINY-ENFORCE-004/-005 shipped enforcement outcomes to `constraint_outcomes` + alert rules. The disclosed follow-up was "elevate from alert-driven to RSIC-actionable" by adding self_reflect patterns that recommend concrete constraint-management actions.

## Verdict

**Shipped.** `DatasetProvider.EnforcementOutcomes` returns per-constraint-code counts of the three enforcement outcome types. `SelfAssessmentReport.EnforcementOutcomes` populated in `Assess()`. Two new self_reflect patterns emit recommendations. **Live-verified**: `/v1/self-improve/assess` now returns the field populated with the missed_violation row from ENFORCE-005's live drill.

## What shipped

### E1 — TSDB dataset method (`internal/tsdb/dataset_builder.go`)
- `DatasetProvider.EnforcementOutcomes(ctx, spaceID, window) -> map[code]EnforcementOutcomeCounts`
- Query: `GROUP BY constraint_code` with `COUNT(*) FILTER (WHERE outcome_type = 'X')` for each of the three enforcement outcome types
- Filters `constraint_code != ''` — empty codes have no actionable target
- New `EnforcementOutcomeCounts` struct (three int fields)

### E2 — SelfAssessmentReport field (`internal/ape/types_rsic.go`)
`EnforcementOutcomes map[string]tsdb.EnforcementOutcomeCounts` — populated in `Assess()` via the new dataset method. Silent skip on query error (patterns simply won't fire this cycle; alerts still cover the operator-visible layer).

### E3 — Two new self_reflect patterns (`internal/ape/self_reflect.go`)
Both fire per-code when the corresponding count crosses the config threshold (uses `BlockedFalsePositiveAlertThreshold` and `MissedViolationAlertThreshold` from ENFORCE-004/005 — one config source-of-truth):

- **`enforcement_false_positive_high`** (MEDIUM severity, `RecommendedAction: archive_ineffective_constraints`) — classifier keeps flagging something the operator keeps overriding. Deprecate/reword.
- **`enforcement_missed_violation_high`** (MEDIUM severity, `RecommendedAction: adjust_guidance_confidence`) — classifier isn't catching what operator corrects. Widen match surface or lower escalation gate.

Both actions are pre-existing `AllowedLLMActions` entries — no action-registry expansion needed (deterministic-source patterns following RSIC-LLM-ALERT-GUARD-001).

### E4 — Mock provider extension (`internal/ape/self_reflect_data_test.go`)
`mockDatasetProvider` gains `enforcementOutcomes` field + `EnforcementOutcomes()` method. All 8+ pre-existing tests that set the mock provider still compile + pass.

### E5 — Pin tests (4 new)
- `TestReflect_EnforcementFalsePositiveHigh_Fires` — verifies pattern fires at ≥threshold, includes action + value, silent on below-threshold codes
- `TestReflect_EnforcementMissedViolationHigh_Fires` — same shape for the missed-violation pattern
- `TestReflect_EnforcementOutcomes_NoDataNoFire` — nil `EnforcementOutcomes` → no fires
- `TestReflect_EnforcementOutcomes_ZeroThresholdDisables` — config threshold ≤0 falls back to safe default 3 (reflector never silently disables; alert rule respects ≤0 as disable)

All pass.

## Live Tier-3 (mdemg-dev, 2026-08-03)

```bash
# Rebuild + restart + trigger self-improve assess
$ curl -s -X POST http://localhost:9999/v1/self-improve/assess \
    -H "Content-Type: application/json" \
    -d '{"space_id":"mdemg-dev"}' | jq .data.enforcement_outcomes
{
  "no-direct-main-commits-must-master-cms-usage-track": {
    "blocked_true_positive": 0,
    "blocked_false_positive": 0,
    "missed_violation": 1
  }
}
```

RSIC now sees the enforcement outcomes populated in the report. The `missed_violation` row came from ENFORCE-005's live drill (correction observation → matched constraint → row landed). Count is 1 (below default threshold 3) so no pattern fires yet — expected behavior. Once real corrections accumulate against a specific constraint, the pattern will fire and RSIC's action-execution layer picks up `adjust_guidance_confidence` for that code.

## Rules pinned

⚠️ **Reflector-side threshold defaults MUST fall back safely, even when the paired alert-rule config treats ≤0 as disable.** Different failure modes: an alert rule with threshold=0 disabled is operator-visible-off; a reflector pattern with threshold=0 disabled is operator-invisible-off (RSIC just quietly never emits). The reflector pattern falls back to a safe default (3) when the config value is unset, so operators who haven't tuned the knob still get the signal. Alert rules can be ≤0=disabled because their absence is visible.

⚠️ **New self_reflect patterns SHOULD reuse existing `AllowedLLMActions` entries when the semantic fits, not mint new actions.** Actions have downstream execution + calibration paths that need per-action wiring. `archive_ineffective_constraints` covers the "deprecate/reword" semantic for false_positive; `adjust_guidance_confidence` covers the "widen match / lower gate" semantic for missed_violation. Minting new actions like `deprecate_constraint` or `strengthen_constraint` would add real work with no capability gain over the existing shipped actions.

## Not shipped (disclosed)

- **RSIC action-execution layer for enforcement patterns** — the reflector emits the insight; whether/how the RSIC executor acts on it (auto-archive constraint on threshold breach vs operator-approved via HITL) is the next design decision. Alert-visible signal for now.
- **Cross-space enforcement pattern** — the current query is per-space; a global "constraint X is overridden in every space" signal would be more powerful but requires wider query surface.
- **Historical trend over enforcement outcomes** — MetricTrend-shaped signal so RSIC can detect a constraint's override-rate ACCELERATING (not just being high).

## Rollback

Single-commit revert. The persisted `enforcement_outcomes` field values will simply stop being read; the constraint_outcomes rows themselves persist and continue feeding the alert rules from ENFORCE-004/005.

## Documents Accessed

- JIMINY-ENFORCE-004/-005 posts (arc context)
- `internal/tsdb/dataset_builder.go` (new EnforcementOutcomes method + type)
- `internal/ape/types_rsic.go` (new SelfAssessmentReport field)
- `internal/ape/self_assess.go` (populate)
- `internal/ape/self_reflect.go` (two new patterns)
- `internal/ape/self_reflect_data_test.go` (mock provider extension + 4 tests)
- `internal/ape/llm_reflector.go` (verified AllowedLLMActions shape — reused existing actions)
- Live `/v1/self-improve/assess` (mdemg-dev)
