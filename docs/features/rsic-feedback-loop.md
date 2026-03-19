# RSIC Feedback Loop

**Phase AR-1** — Post-cycle re-assessment, success criteria evaluation, and auto-rollback.

## Overview

The RSIC (Recursive Self-Improvement Cycle) feedback loop closes the gap between "action taken" and "action effective." Before this feature, RSIC cycles ran actions (prune edges, tombstone stale nodes, trigger consolidation) but had no way to measure whether those actions actually improved the graph's health. The cycle was open-loop.

Now, after every cycle's dispatch phase completes, RSIC runs a second assessment to capture `MetricsAfter`, evaluates per-task success criteria, and can automatically roll back reversible actions that failed to improve the target metrics.

## How It Works

### Post-Cycle Re-Assessment (R1)

After task dispatch completes, `RunCycle()` runs `Assessor.Assess()` a second time:

```
          ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
          │ 1.Assess │───▶│2.Reflect │───▶│ 3. Plan  │───▶│4.Dispatch│
          │ (before) │    │          │    │          │    │          │
          └──────────┘    └──────────┘    └──────────┘    └─────┬────┘
                                                                │
          ┌──────────┐    ┌──────────┐    ┌──────────┐          │
          │6.Rollback│◀───│5.Criteria│◀───│ Assess   │◀─────────┘
          │(if needed│    │  Eval    │    │ (after)  │  ← NEW: R1
          └──────────┘    └──────────┘    └──────────┘
```

The post-assessment populates `CycleOutcome.MetricsAfter` with the same metrics captured in `MetricsBefore`:

| Metric | Description |
|--------|-------------|
| `overall_health` | Composite health score (0.0-1.0) |
| `retrieval_quality` | Vector search quality estimate |
| `memory_health` | Node stability and freshness |
| `edge_health` | Edge weight distribution quality |
| `orphan_ratio` | Fraction of nodes with no edges |
| `correction_rate` | Fraction of correction observations |
| `edge_weight_entropy` | Shannon entropy of edge weights |

If post-assessment fails, the cycle continues without `MetricsAfter` (fail-open).

**Dry-run cycles skip post-assessment** — since no mutations occurred, metrics would be identical.

### Success Criteria Evaluation (R2)

Each `RSICTaskSpec` can include `SuccessCriteria` — a map of metric keys to threshold values. After populating `MetricsAfter`, the calibrator evaluates whether each criterion was met:

```
criterion key: "edges_below_threshold_delta"
operator: inferred from key suffix (_delta = change comparison)
comparison: MetricsAfter[key] - MetricsBefore[key] vs threshold
```

The evaluation produces two new fields on `CycleOutcome`:

| Field | Type | Description |
|-------|------|-------------|
| `criteria_met` | `bool` | True if all criteria were met (or no criteria defined) |
| `criteria_detail` | `map[string]string` | Per-criterion result: `"met"`, `"not_met"`, or `"missing_data"` |

`criteria_met` feeds into calibration — `UpdateCalibration()` now only counts a task as "success" if both the task completed AND criteria were met. This prevents the calibration confidence from inflating due to actions that ran without effect.

### Auto-Rollback (R6)

When `criteria_met` is false, the cycle checks whether any dispatched actions are reversible:

| Reversible Actions | Irreversible Actions |
|-------------------|---------------------|
| `tombstone_stale` | `prune_decayed_edges` |
| `graduate_volatile` | `prune_excess_edges` |
| | `trigger_consolidation` |
| | `refresh_stale_edges` |

For reversible actions, the cycle looks up their snapshot in `SnapshotStore` and calls `Rollback()`, restoring the pre-action state. Irreversible actions are logged as `completed_no_benefit`.

Rollback events emit the `mdemg_rsic_rollbacks_total` Prometheus counter with `{action, reason}` labels.

## Configuration

No new configuration variables required — the feedback loop is always active. Success criteria are defined per-task by the planner.

## Usage

The feedback loop is automatic and transparent. To observe it:

```bash
# Trigger a cycle and inspect metrics_after + criteria
curl -s -X POST http://localhost:9999/v1/self-improve/cycle \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","tier":"micro"}' | jq '{
    metrics_before: .metrics_before,
    metrics_after: .metrics_after,
    criteria_met: .criteria_met,
    criteria_detail: .criteria_detail
  }'

# Check calibration (reflects criteria-aware success tracking)
curl -s http://localhost:9999/v1/self-improve/calibration | jq

# Check rollback snapshots
curl -s http://localhost:9999/v1/self-improve/rollback | jq
```

## Key Files

| File | Description |
|------|-------------|
| `internal/ape/cycle.go` | `RunCycle()` — post-assessment call, auto-rollback logic |
| `internal/ape/calibration.go` | `Validate()` — MetricsAfter population, criteria evaluation; `UpdateCalibration()` — criteria-aware success |
| `internal/ape/types_rsic.go` | `CycleOutcome` — `CriteriaMet`, `CriteriaDetail`, `MetricsAfter` fields |
| `internal/ape/calibration_test.go` | 8 unit tests (post-report, criteria met/not-met/missing, operators, calibration) |
| `tests/integration/autoresearch_test.go` | 3 integration tests (metrics_after, criteria fields, dry-run) |

## Dependencies

- **RSIC core (Phase 60b)** — assess/reflect/plan/dispatch pipeline
- **Snapshot store (Phase 89)** — rollback capability for reversible actions
- **Calibrator (Phase 60b)** — confidence tracking per action type
