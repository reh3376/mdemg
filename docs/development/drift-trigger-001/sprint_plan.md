# DRIFT-TRIGGER-001 — Sprint Plan

**Date:** 2026-07-28 | **Branch:** `reh3376_dev01`
**Original candidate name:** DRIFT-VALIDATION-001 (Q4 frontier deep-dive #3)
**Reframe:** the investigation surfaced that the drift-signal chain
DOESN'T end at the actuator — `ft_production_drift` is architecturally
observational, and there's no code path from it to cycle-open. This sprint
CLOSES that gap. Renamed to DRIFT-TRIGGER-001 because it's capability
work, not validation.

## 1. Header & Metadata

Wire `ft_production_drift` into the RSIC actuator: add a new self-reflect
pattern that reads the drift signal from a new `DatasetProvider` method
and emits a `trigger_training_pipeline` insight — the same insight
`training_data_ready` (pattern 29) emits. Flows through the shipped
actuator gate (`FT_LOOP_ENABLED` default false, freshness gate, interval
gate, single-flight — all preserved). Effort ~1 day. Risk low: the
insight is a diagnostic when the actuator is off; safety envelope is the
shipped Gate.

## 2. Problem Statement

`ft_production_drift` fires as an alert (dispatched via the hook channel)
when the latest benchmark aggregate falls below the active model's score
by margin (default 0.05). **The alert is purely observational.** There
is NO code path from the alert to `Gate.EvaluateTrigger`, and no other
mechanism translates drift into a training-cycle trigger.

The only path to cycle-open is `training_data_ready` (self_reflect.go
pattern 29): fires when `TrainingReadiness.Tasks.readyCount > 0` — i.e.
"enough new training data has accumulated." That's the DATA-availability
signal. The MODEL-quality signal has no automation.

Investigation trail:
- `alert.FtProductionDriftRule` fires → `alertDispatcher.Dispatch` →
  `~/.mdemg/alerts/current.json` + optional macOS notification
- Nothing else consumes the alert programmatically
- `Gate.EvaluateTrigger` is only called by `task_dispatch.go:1169` in
  response to the `trigger_training_pipeline` insight
- The insight is emitted by exactly ONE place: pattern 29
  (`training_data_ready`)

**Consequence:** on a hypothetical substrate where retrain is warranted
because the model has degraded (drift high) but training data hasn't
accumulated much (readyCount 0), the actuator will NEVER open a cycle.
Drift is diagnosed but not treated.

**This sprint closes the loop:** drift becomes a valid trigger reason,
subject to the same shipped gates that guard readiness-triggered cycles.

## 3. Scope & Constraints

**In scope (4 sequential epics):**

- **E1** — `DatasetProvider.ProductionDrift(ctx)` method + implementation
  in `dataset_builder.go`; mirrors the exact SQL of
  `alert.FtProductionDriftRule` (single source of truth). Returns
  `ProductionDriftSummary{Delta, ActiveScore, LatestBenchScore, LatestBenchAt}`.
- **E2** — `SelfAssessmentReport.ProductionDrift` field + wire population
  in `self_assess.go` (same pattern as EmbeddingDataset / TrainingReadiness).
- **E3** — New self-reflect pattern (31) `production_drift_detected`
  emits `trigger_training_pipeline` insight when
  `ProductionDrift.Delta > FT_DRIFT_MARGIN` (same margin the alert uses;
  single source of truth via config). Description text names the delta
  + active + bench scores so the emitted insight is self-explanatory.
- **E4** — Docs (feature note, CHANGELOG, CLAUDE.md pin) + live drill.

**Out of scope:**

- **Changing the shipped Gate behavior** — the interval / freshness /
  single-flight / enabled gates all stay as-is. This sprint changes ONE
  thing: adds a new REASON the trigger insight can fire. Downstream gate
  behavior is unchanged, so the safety envelope is identical.
- **Changing the drift alert or its dispatch** — the alert still fires
  to the operator. This sprint adds an ADDITIONAL consequence for the
  same underlying condition, not a replacement.
- **Removing the `training_data_ready` pattern** — both patterns coexist;
  either can emit the trigger insight; `deduplicateInsights` (already
  shipped) prevents dual-fire when both conditions hold.

## 4. Dependencies

- **FT-RECURSIVE-002** (shipped): the Gate + ledger + actuator
- **FT-RECURSIVE-004** (shipped): `ft_production_drift` alert + margin config
- **RSIC self-reflect pattern shape** (many patterns exist; 29 is the
  direct precedent for `trigger_training_pipeline`)
- `FT_LOOP_ENABLED` config flag (already exists, default false)

## 5. Implementation Plan (sequential epics + gates)

**E1 — `DatasetProvider.ProductionDrift`** (internal/tsdb)
- Add `ProductionDriftSummary` struct: `Delta float64, ActiveScore float64,
  LatestBenchScore float64, LatestBenchAt time.Time, HasActive bool,
  HasBench bool`.
- Add `ProductionDrift(ctx) (*ProductionDriftSummary, error)` to the
  `DatasetProvider` interface + implementation.
- SQL mirrors `alert.FtProductionDriftRule` verbatim + returns the
  intermediate values (not just `GREATEST(0, delta)`) so the reflector
  can log meaningful descriptions.
- **Gate:** unit test — the summary matches the alert-rule math on
  representative fixtures.

**E2 — Wire report population** (internal/ape/self_assess.go)
- Add `ProductionDrift *tsdb.ProductionDriftSummary` field to
  `SelfAssessmentReport` in `types_rsic.go`.
- In `self_assess.go`, fetch alongside the other datasets (same warn-on-
  error pattern).
- Test-mock: `mockDatasetProvider` in `self_reflect_data_test.go` gains
  a `ProductionDrift` method.
- **Gate:** build clean, existing tests pass.

**E3 — Reflect pattern** (internal/ape/self_reflect.go)
- Add pattern 31 after `training_data_ready`:
  ```go
  if report.ProductionDrift != nil && report.ProductionDrift.HasActive &&
     report.ProductionDrift.HasBench &&
     report.ProductionDrift.Delta > r.cfg.FtDriftMargin {
    insights = append(insights, ReflectionInsight{
      PatternID:         "production_drift_detected",
      Severity:          SeverityMedium,
      Description:       fmt.Sprintf("Production drift %.4f (active %.4f, latest bench %.4f) exceeds margin %.4f",
        report.ProductionDrift.Delta, report.ProductionDrift.ActiveScore,
        report.ProductionDrift.LatestBenchScore, r.cfg.FtDriftMargin),
      RecommendedAction: "trigger_training_pipeline",
      Metric:            "ft_production_drift",
      Value:             report.ProductionDrift.Delta,
      Threshold:         r.cfg.FtDriftMargin,
    })
  }
  ```
- ⚠️ Reads `r.cfg.FtDriftMargin` — the SAME config the alert rule uses
  (single source of truth for the threshold).
- **Gate:** unit test — pattern fires when delta > margin AND has active
  AND has bench; does NOT fire when any condition fails; the insight's
  RecommendedAction is `trigger_training_pipeline` (verifies the wire
  reaches the actuator via task_dispatch's existing case handler).

**E4 — Live Tier-3 drill + docs**
- **Drill (mdemg-dev, with actuator OFF for safety)**:
  1. Seed a synthetic score-regressed benchmark row (aggregate 0.4;
     current active 0.8655; expected drift 0.4655 >> 0.05 margin).
  2. Trigger an RSIC self-assess cycle; verify pattern 31 emits the
     insight (check via the RSIC insights output or log).
  3. Because `FT_LOOP_ENABLED=false` (default): task_dispatch will
     process the insight but Gate.EvaluateTrigger will return
     `suppress_disabled` — no cycle opens (this is the correct behavior).
     Verify no cycle row appears in `ft_training_cycles`.
  4. Clean up the seeded benchmark row.
- **Optional stretch drill** (only if operator authorizes): with
  `FT_LOOP_ENABLED=true` temporarily, seed drift, verify a cycle opens,
  IMMEDIATELY set `FT_LOOP_ENABLED=false` + delete the cycle row before
  the controller picks it up. Higher risk, not required for the sprint
  gate — the flag-off drill proves the wire; the flag-on drill proves
  the shipped gate connects.
- **Docs**:
  - New `docs/features/drift-triggered-retrain.md` (short — 1 page)
  - CHANGELOG entry
  - CLAUDE.md pin: "drift is now a valid trigger reason for
    trigger_training_pipeline, subject to the same actuator gates as
    training_data_ready"

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** `TestProductionDrift_Summary` (math matches the
  alert rule), `TestReflectPattern_ProductionDriftDetected` (pattern fires
  under delta > margin; doesn't fire when has_active=false, has_bench=false,
  or delta <= margin). Update `TestReflect_TrainingReadinessMissing` and
  siblings to include the ProductionDrift-nil case.
- **Tier 2 (contract):** `go build ./...`, `golangci-lint 0 issues`,
  `go test ./... -count=1` full green. `mockDatasetProvider` gains the
  new method (all sites updated).
- **Tier 3 (live):** the drill in E4 above.

## 7. Commit Strategy

Sequential single-epic commits: E1 → E2 → E3 → E4, per operator's
"no parallelization" rule. Each commit stands on its own.

## 8. Verification Checklist

- [ ] `DatasetProvider.ProductionDrift` implemented + unit-tested
- [ ] `SelfAssessmentReport.ProductionDrift` populated
- [ ] `mockDatasetProvider` updated (all test callers)
- [ ] Pattern 31 fires under correct conditions (unit-tested)
- [ ] Pattern 31 uses `r.cfg.FtDriftMargin` (single source of truth)
- [ ] Full `go test ./...`, `go build`, `golangci-lint` all green
- [ ] Live drill: synthetic drift → pattern fires → gate suppresses
      (actuator off); benchmark row cleaned up
- [ ] Feature doc + CHANGELOG + CLAUDE.md pin
- [ ] No serving-symlink touch throughout
- [ ] No real training cycle opens during drill

## 9. Rollback Procedures

- All four epics are additive (new dataset method, new report field, new
  reflect pattern, docs). Revert commit fully removes.
- The shipped `training_data_ready` pattern continues to trigger cycles
  as before.
- Config flag `FT_LOOP_ENABLED` remains the master gate; unchanged.
- No schema change, no substrate mutation.

## 10. Risks & Mitigations

- **Risk:** dual-fire — pattern 29 AND pattern 31 both emit
  `trigger_training_pipeline` on the same cycle, causing duplicated
  cycle opens.
  - **Mitigation:** `deduplicateInsights` (already shipped in
    task_dispatch) collapses identical `recommended_action` values.
    Verified by reading the shipped dedup path. Additionally, Gate's
    single-flight guard (`hasOpenCycle` check) prevents a duplicate
    cycle even if dedup fails.
- **Risk:** the drift pattern fires TOO OFTEN — on every RSIC cycle where
  drift is above margin, so once drift condition holds, actuator opens
  a cycle every RSIC interval.
  - **Mitigation:** Gate's interval guard (`RetrainInterval` default is
    hours-scale). Even continuous pattern firing → at most ONE cycle per
    interval. Also: Gate's `hasOpenCycle` guard blocks a second cycle
    while the first is running.
- **Risk:** drift condition is transient / measurement noise → spurious
  triggers.
  - **Mitigation:** margin default 0.05 = 5-point aggregate drop, which
    is real signal not noise (bench-to-bench variance is much smaller —
    live 24d range is 0.0033). Operator can tighten via config.
- **Risk:** the drill accidentally triggers a real training run.
  - **Mitigation:** actuator OFF by default (`FT_LOOP_ENABLED=false`),
    kept off through the drill. Optional stretch drill (flag-on) is
    NOT required for the sprint gate; if attempted, the abort path is
    to flag-off + delete the cycle row before controller picks it up.

## 11. Documents Accessed

Filled in commit messages + post.md.

## 12. Documentation Update

Final epic — never cut (Sprint Plan Format v1.0). Covered by E4.
