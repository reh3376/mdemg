---
created: 2026-03-30
updated: 2026-04-17
version: v0.5.4
author: reh3376
status: active
phase: "AR-1"
---

# RSIC Feedback Loop

## Summary

**Feature**: RSIC Feedback Loop
**Summary**: Post-cycle re-assessment, success criteria evaluation, and auto-rollback for RSIC actions. Closes the gap between 'action taken' and 'action effective.'


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

Rollback events emit the `mdemg_rsic_rollbacks_total` counter with `{action, reason}` labels. Metrics are exposed via `/v1/metrics/snapshot` and persisted in TimescaleDB.

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

## Protocol Health Null-Tolerance (DH-004)

`ProtocolStats` fields that can be zero because a feature is rarely exercised (not because it's failing) must be null-tolerant in the scoring formula:

| Field | Pattern | Behavior when zero |
|-------|---------|--------------------|
| `CodeCoverage` | Null-tolerant | Returns 1.0 when no constraints defined |
| `TicketRestoreSuccessRate` | Null-tolerant (DH-004) | Returns 1.0 when `TicketRestoreTotal == 0` |
| `ReplayFrequencyPerHour` | Null-tolerant (inherent) | Lower is better — 0 replays yields 0 penalty |

`TicketRestoreTotal` was added to `ProtocolStats` so downstream dashboard consumers can distinguish "no data" (total=0) from "true 100%" (total>0 && ok==total). The change lifts `rsic_health_protocol` for healthy systems with no restore events — previously returning 0.0 indistinguishably from "100% of restores failed," dragging the 15% stability weight.

## Health Weighting & Confidence (DH-005)

The overall-health score is a normalised weighted-confidence sum over 7 sub-dimensions — replacing the prior 4/5/6/7-dimension branch table with a single formula:

```
overall_health = Σ(w_i · c_i · s_i) / Σ(w_i · c_i)
```

where:
- `w_i` = base prior weight (hybrid reliability × user-impact)
- `c_i` = per-dimension data-sufficiency confidence ∈ [0, 1]
- `s_i` = per-dimension score ∈ [0, 1]

Dimensions with `w_i ≤ 0` or `c_i ≤ 0` contribute nothing to either numerator or denominator — they are **automatically excluded, not penalised**. Because the result is a weighted average of values in `[0, 1]`, it always lands in `[0, 1]`, so dashboard thresholds (red `<0.4`, green `≥0.7`) remain meaningful by construction regardless of which dimensions are present.

### Default priors (`DefaultHealthWeights`)

The defaults reflect a hybrid of *reliability* (how trustworthy is this dimension's score?) and *user-impact* (how visible is this dimension in day-to-day operation?). They are **not** a uniform split.

| Dimension | Reliability | Impact | Default weight | Prior weight |
|-----------|-------------|--------|----------------|--------------|
| `RetrievalQuality` | LOW (static `LearningPhase` lookup) | Modest | **0.08** | 0.18 |
| `MemoryHealth` | MODERATE (orphan/correction/consolidation) | HIGH | **0.15** | 0.18 |
| `EdgeHealth` | HIGH (entropy + below-threshold) | MEDIUM | **0.15** | 0.13 |
| `TaskPerformance` | MOD-HIGH (post-DH-004 graduation fix) | HIGH | **0.20** | 0.13 |
| `GuidanceHealth` | MODERATE (follow rate + effectiveness) | HIGH | **0.17** | 0.13 |
| `ProtocolHealth` | HIGH (5-component J17 composite) | MED-HIGH | **0.20** | 0.13 |
| `SynergyHealth` | LOW (CLAUDE.md/MEMORY.md file-size proxy) | LOW | **0.05** | 0.12 |
| **Sum** | | | **1.00** | 1.00 |

The "prior weight" column is the pre-DH-005 near-uniform table — weights were inversely correlated with reliability (the least-reliable dimension carried the most weight). DH-005 inverts that: `Protocol` and `Task` lead, `Retrieval` and `Synergy` trail.

### Per-dimension confidence

Each `score*` function returns `(score, confidence)` where confidence reflects data sufficiency:

| Dimension | Confidence formula | Full-confidence threshold |
|-----------|--------------------|---------------------------|
| `scoreRetrieval` | Map from `LearningPhase` | `warm` or `saturated` ⇒ 1.0 |
| `scoreMemory` | `min(1, TotalNodes / 100)` | 100 nodes |
| `scoreEdge` | `min(1, EdgeCount / 50)`; returns `(1.0, 0)` when `EdgeCount == 0` | 50 edges |
| `scoreTask` | `min(1, (Volatile+Permanent) / 50)`; returns `(0.5, 0)` when 0 | 50 observations |
| `scoreGuidance` | `min(1, TotalGuidanceIssued / 30)`; returns `(score, 0)` when 0 | 30 guidance events |
| `scoreProtocol` | `min(1, TotalEvents / 30)`; returns `(score, 0)` when 0 | 30 J17 events |
| `scoreSynergy` | `1.0` if Jiminy healthy AND files present, else `0.0` | — |

### Operator knobs

Tune weights without a rebuild via env vars:

```bash
RSIC_HEALTH_WEIGHT_RETRIEVAL=0.08
RSIC_HEALTH_WEIGHT_MEMORY=0.15
RSIC_HEALTH_WEIGHT_EDGE=0.15
RSIC_HEALTH_WEIGHT_TASK=0.20
RSIC_HEALTH_WEIGHT_GUIDANCE=0.17
RSIC_HEALTH_WEIGHT_PROTOCOL=0.20
RSIC_HEALTH_WEIGHT_SYNERGY=0.05
```

Rules:
- Values need not sum to 1.0 — the formula normalises.
- `0` is honoured as "disable this dimension entirely."
- Negative values fall back to the default with a warning log.
- All-zero is flagged as a warning by `Config.Validate()` (overall would always be 0).

### Prometheus gauges

Seven new confidence gauges are emitted alongside the score gauges:

```
mdemg_rsic_health_retrieval_confidence{space_id}
mdemg_rsic_health_memory_confidence{space_id}
mdemg_rsic_health_edge_confidence{space_id}
mdemg_rsic_health_task_confidence{space_id}
mdemg_rsic_health_guidance_confidence{space_id}
mdemg_rsic_health_protocol_confidence{space_id}
mdemg_rsic_health_synergy_confidence{space_id}
```

Exposed via `/metrics`, persisted by TSDB writeback, and visualised in the "Dimension Confidence (DH-005)" row of the `mdemg-rsic` Grafana dashboard.

## Context Cooler Graduation Fix (DH-004)

`rsic_health_task` was stuck at 0.019 in `mdemg-dev` because 99.7% of conversation observations stayed volatile forever. Root cause: `CoactivateSession` in `internal/learning/service.go` created `CO_ACTIVATED_WITH` edges between session-observation pairs but never called `UpdateStabilityOnReinforcement`, so `stability_score` never crossed the `COOLER_GRADUATION_THRESHOLD` (0.8).

The DH-004 fix adds `reinforceSessionObservations()` which runs after edge creation and reinforces every observation in the session (`+0.15` stability per coactivation). Forward-only fix — existing volatile data self-heals as ongoing sessions reinforce their constituents. Operators can also backfill via `POST /v1/conversation/graduate`.

## Dependencies

- **RSIC core (Phase 60b)** — assess/reflect/plan/dispatch pipeline
- **Snapshot store (Phase 89)** — rollback capability for reversible actions
- **Calibrator (Phase 60b)** — confidence tracking per action type
