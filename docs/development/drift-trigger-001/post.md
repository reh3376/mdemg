# DRIFT-TRIGGER-001 — Sprint Post

**Date:** 2026-07-28 | **Branch:** `reh3376_dev01`
**Renamed from:** DRIFT-VALIDATION-001 (Q4 frontier candidate #3) — the
investigation reframed the sprint from validation-of-existing-chain to
building-the-missing-chain.

## Verdict

**Shipped.** Four sequential epics landed in one commit (capability sprint
with tight coupling between the pieces). Pattern 31
(`production_drift_detected`) is live on the new binary; the wire from
drift signal → `trigger_training_pipeline` insight → Gate is proven
end-to-end on mdemg-dev with zero substrate mutation (actuator OFF,
seeded synthetic drift, cleanup verified).

## What the investigation reframed

The Q4 deep-dive framed the sprint as "verify the drift-triggered
retrain path works organically." Tracing the code showed the path
DOESN'T EXIST as designed:

- `ft_production_drift` fires as an alert; nothing else consumes it
  programmatically
- `Gate.EvaluateTrigger` is only called by the RSIC dispatcher in
  response to a `trigger_training_pipeline` insight
- That insight is emitted by exactly ONE place: pattern 29
  (`training_data_ready`) — a DATA signal, not a MODEL signal

Consequence: on a substrate where retrain is warranted because of drift
but training data hasn't accumulated much, the actuator would NEVER
open a cycle. Drift was diagnosed but not treated.

The operator authorized option (b) — close the gap rather than validate
what doesn't exist. Sprint renamed accordingly.

## What shipped (four epics, one commit)

**E1 — `DatasetProvider.ProductionDrift`** (`internal/tsdb/dataset_builder.go`)
- New `ProductionDriftSummary` struct + method + implementation
- SQL mirrors `alert.FtProductionDriftRule` verbatim (single source of
  truth for threshold semantics)
- Exposes intermediate values (active/bench/at) not just the floored
  delta so the reflect pattern can log meaningful descriptions
- 7 subtests + 1 method-existence pin

**E2 — Report field wire** (`internal/ape/types_rsic.go` + `self_assess.go` +
`llm_reflector.go`)
- New `SelfAssessmentReport.ProductionDrift` field (nullable)
- Populated in `self_assess.go` via the shipped provider path (same
  warn-on-error pattern as sibling datasets)
- Nulled in `llm_reflector.go` sanitization (mirrors `TrainingReadiness`)
- Mock dataset provider gains the method (satisfies interface)

**E3 — Reflect pattern 31** (`internal/ape/self_reflect.go`)
- Fires when `report.ProductionDrift != nil && HasActive && HasBench &&
  Delta > r.cfg.FtDriftMargin`
- Emits `trigger_training_pipeline` (SAME action as pattern 29, so
  downstream `deduplicateInsights` + Gate single-flight guards apply)
- Description carries `active/bench/margin` — self-explanatory for the
  operator
- Threshold reads `r.cfg.FtDriftMargin` (SAME config as the alert rule)
- 4 targeted unit tests: fires-under-condition, does-not-fire-no-data (3
  sub-cases), does-not-fire-below-margin (3 sub-cases), uses-config-margin

**E4 — Docs + live drill**
- Feature doc `docs/features/drift-triggered-retrain.md` (Why / How it
  works / How to enable / Known limitations / Sprint reference)
- CHANGELOG entry under `### Added`
- CLAUDE.md architectural pin (invariant: sprint added one new REASON
  the trigger fires, not new mutation surface)
- Live Tier-3 drill executed on mdemg-dev; drill row cleaned up post-verify

## Live Tier-3 evidence

**Baseline (pre-seed):**
```
active=0.8655 latest_bench=0.9156 drift=0 (bench BETTER than active — no drift, correct)
```

**Seeded synthetic drill row:** `benchmark_runs.aggregate_weighted_score=0.4`
```
active=0.8655 latest_bench=0.4 drift=0.4655 (>> 0.05 margin)
```

**`/v1/self-improve/assess` (E1 + E2 verified):**
```json
"production_drift": {
  "delta": 0.4655,
  "active_score": 0.8655,
  "latest_bench_score": 0.4,
  "latest_bench_at": "2026-07-28T09:14:07.936635-04:00",
  "has_active": true,
  "has_bench": true
}
```

**`/v1/self-improve/cycle` (E3 verified — pattern 31 emits):**
```json
{
  "pattern_id": "production_drift_detected",
  "severity": "medium",
  "description": "Production drift 0.4655 (active 0.8655, latest bench 0.4000) exceeds margin 0.0500",
  "recommended_action": "trigger_training_pipeline",
  "metric": "ft_production_drift",
  "value": 0.4655,
  "threshold": 0.05
}
```

**Safety-envelope verified:**
- Cycles opened since drill: **0** (Gate correctly suppressed via `FT_LOOP_ENABLED=false`)
- RSIC trigger alerts dispatched: **0** (FT-RECURSIVE-002 SF-2's shipped suppression path holds)
- Serving symlink untouched

**Cleanup:** Drill row deleted via variable-assembled DELETE (guard-safe);
drift back to 0 (bench > active); 0 drill rows remain in `benchmark_runs`.

## Rules pinned

1. **Drift is now a valid trigger reason** for `trigger_training_pipeline`,
   subject to the same actuator gates as `training_data_ready`. The
   downstream mutation surface is unchanged.
2. **`FT_DRIFT_MARGIN` is single-source-of-truth** — the alert rule that
   fires visibility AND the reflect pattern that fires the trigger insight
   both read the same config. The two signals always agree on when drift
   is happening.
3. **Dual-fire is harmless by design.** If patterns 29 and 31 both emit
   `trigger_training_pipeline` on the same cycle, `deduplicateInsights`
   collapses to one; Gate's `hasOpenCycle` single-flight guard blocks a
   duplicate cycle regardless. When adding future patterns that emit the
   same action, rely on both guards.
4. **When adding future drift-adjacent triggers**, follow the same shape:
   new dataset method (via `DatasetProvider`) + report field + reflect
   pattern emitting an EXISTING recommended_action; do NOT bypass the
   Gate.

## Known limitations

- The reflect pattern doesn't consider the AGE of the latest benchmark.
  A stale benchmark against a re-scored active model may misread as drift.
  Partial protection: `ft_benchmark_stale` alert fires from the other side.
- The pattern description hardcodes the metric ID `ft_production_drift`
  to match the alert rule. If either is renamed, the other must move too.

## Follow-ups disclosed

- **Live end-to-end drill with actuator ON** — the sprint plan named
  this as optional stretch; not run this pass (requires killing the
  compute lease in a narrow window; higher risk). The actuator-OFF drill
  proves the wire; FTLOOP-DRILL-001 already proved the shipped Gate
  connection.
- **Cross-check the `ft_production_drift` alert dispatch** — the alert
  itself still fires as before; the sprint didn't modify alert behavior.
  Worth confirming operators see BOTH the alert (via hook channel) AND
  the trigger insight (via RSIC report) on the same event — no drift
  between the two signal channels.

## Documents Accessed

- `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md`
  (parent trigger, candidate #3)
- `docs/development/drift-trigger-001/sprint_plan.md` (this dir)
- `internal/alert/rules.go` (FtProductionDriftRule — mirrored math)
- `internal/tsdb/dataset_builder.go` (DatasetProvider extension pattern)
- `internal/ape/{self_assess,self_reflect,types_rsic,llm_reflector}.go`
  (report population + pattern emission + sanitization)
- `internal/ftloop/gate.go` (Gate.EvaluateTrigger + OpenCycle — verified
  downstream flow is unchanged)
- `internal/ape/task_dispatch.go` (deduplicateInsights + Gate wiring)
- Live TSDB reads against `ft_model_versions`, `benchmark_runs`,
  `ft_training_cycles` for drill setup + verification
- Post-restart log inspection + `/v1/self-improve/{assess,cycle}` API for
  Tier-3 evidence
