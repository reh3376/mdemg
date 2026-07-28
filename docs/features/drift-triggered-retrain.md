# Drift-Triggered Retrain

**Sprint:** DRIFT-TRIGGER-001 (2026-07-28) — wires `ft_production_drift`
into the RSIC actuator so model degradation can auto-open a retrain cycle
(subject to the shipped Gate).

## Why

Before this sprint, `ft_production_drift` was purely observational — an
alert fired to the operator when the active model's score fell below the
latest benchmark aggregate by more than the drift margin, but nothing
translated that signal into a training-cycle trigger. The only path to
cycle-open was `training_data_ready` (RSIC pattern 29), which is a
DATA-availability signal, not a MODEL-quality signal.

Consequence: on a substrate where retrain is warranted because the model
has degraded (drift high) but training data hasn't accumulated much
(readyCount 0), the actuator would NEVER open a cycle. Drift was
diagnosed but not treated.

## What shipped

A new self-reflect pattern (pattern 31, `production_drift_detected`) that
emits the SAME `trigger_training_pipeline` recommended action as pattern
29, but keyed on the drift signal instead of data-readiness.

The pattern flows through the shipped actuator Gate — all existing guards
stay in place:
- `FT_LOOP_ENABLED` (default false) — master gate
- Single-flight (`hasOpenCycle` check) — no duplicate cycles
- Interval (`RetrainInterval`) — throttles rapid re-triggers
- Freshness (`MinFreshFraction`) — no retrain on stale-only data

## How it works

Three additive pieces:

### 1. `DatasetProvider.ProductionDrift(ctx)` (`internal/tsdb/dataset_builder.go`)

Reads the drift signal directly from TSDB:
- Active score: `MAX(overall_score) FROM ft_model_versions WHERE status='active'`
- Latest bench: `MAX(aggregate_weighted_score) FROM benchmark_runs
  WHERE completed_at = MAX(completed_at)`
- `Delta = max(0, active - latest_bench)` — floors at 0 (bench better than
  active reads as no drift)

The `HasActive`/`HasBench` flags distinguish "no data" from "genuine zero
delta." The alert rule floors to 0 in both cases; the reflect pattern
gates on both flags being true so it never fires spuriously on empty state.

**Math mirrors `alert.FtProductionDriftRule` verbatim** — single source of
truth for the threshold semantics.

### 2. `SelfAssessmentReport.ProductionDrift` (`internal/ape/types_rsic.go`)

New nullable field on the RSIC assessment report. Populated in
`self_assess.go` via `datasetProvider.ProductionDrift(ctx)` alongside the
other TSDB-backed datasets. Same warn-on-error pattern; nil when the
provider returns nil or errors.

### 3. Reflect pattern 31 (`internal/ape/self_reflect.go`)

```
if report.ProductionDrift != nil && HasActive && HasBench &&
   Delta > r.cfg.FtDriftMargin {
    → emit insight (recommended_action: trigger_training_pipeline)
}
```

Same `FT_DRIFT_MARGIN` config the alert rule uses (single source of truth
so the two signals always agree on when drift is happening).

## Safety invariants

- **Actuator OFF by default** (`FT_LOOP_ENABLED=false`) — the pattern
  emits the insight, but Gate.EvaluateTrigger returns `suppress_disabled`
  and no cycle opens. Only the operator's explicit opt-in enables the
  path.
- **Shared threshold** — the alert firing threshold and the trigger
  threshold are the same config knob. When the operator opts in, the
  alert (which they see in the hook channel) and the trigger (which the
  actuator handles) fire on identical conditions. No hidden thresholds.
- **Dual-fire is harmless** — if pattern 29 (`training_data_ready`) and
  pattern 31 both emit `trigger_training_pipeline` on the same cycle,
  `deduplicateInsights` collapses to one, and Gate's single-flight guard
  blocks a second cycle regardless.

## How to enable

```bash
# In .env:
FT_LOOP_ENABLED=true

# Optionally tighten/loosen the threshold (drift OR alert):
FT_DRIFT_MARGIN=0.10        # default 0.05 = 5% score drop
```

Restart the server. Drift ≥ margin at the next RSIC cycle (default ~5 min)
now opens a training cycle via the shipped Gate + controller pipeline. The
tripwire (`FT_LOOP_TRIPWIRE_ENABLED`), auto-rollback, and issue filer all
continue to operate as before — this sprint only adds a new REASON the
trigger fires, not new downstream behavior.

## Known limitations

- **The insight metric name is `ft_production_drift`** — matches the alert
  rule ID so consumers can correlate. If the alert rule ID is ever
  renamed, update both together.
- **The reflect pattern doesn't consider the AGE of the latest benchmark.**
  If a benchmark is very stale and the active model has been re-scored
  meanwhile, the drift computation may be misleading. The
  `ft_benchmark_stale` alert covers this from the other side.

## Sprint reference

Plan: `docs/development/drift-trigger-001/sprint_plan.md`
Post: `docs/development/drift-trigger-001/post.md`
