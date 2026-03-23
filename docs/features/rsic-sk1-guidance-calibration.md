# RSIC-SK1: Jiminy Guidance Self-Calibration

## Overview

RSIC-SK1 closes three feedback loop gaps in the Jiminy guidance system, creating a self-improving guidance pipeline where chronically ignored guidance decays automatically and consistently followed guidance strengthens.

### The Three Gaps

1. **Dispatcher gap**: Reflection pattern #9 emits `review_guidance_effectiveness` but the dispatcher had no executor — it hit `default: error` at `task_dispatch.go`.
2. **Confidence scope gap**: ConfidenceUpdater only fired for `GuidanceConstraint` types. Corrections, patterns, decisions with source nodes got no confidence feedback.
3. **SignalLearner isolation**: Created at `server.go` but never wired to Jiminy — only tracked RSIC-internal signals, not guidance signals.

## Architecture

```
Guide() ──→ Emit items ──→ SignalLearner.RecordEmission()
                │
                ▼
         Agent acts on guidance
                │
                ▼
RecordOutcome() ──→ Classify outcome (followed/ignored/contradicted)
                │              │
                ▼              ▼
    ConfidenceUpdater      SignalLearner.RecordResponse()
    (all guidance types)   (followed/partial only)
                │
                ▼
    RSIC Reflect ──→ Pattern #9 (health < 0.5): review_guidance_effectiveness
                     Pattern #15 (health < 0.7): adjust_guidance_confidence
                │
                ▼
    RSIC Dispatch ──→ review_guidance_effectiveness (diagnostic)
                      adjust_guidance_confidence (boost/decay/archive)
                      archive_ineffective_constraints (cleanup)
                │
                ▼
         Guide() improved (higher-confidence items surface, low-confidence decay)
```

## New RSIC Actions

### `review_guidance_effectiveness` (diagnostic)
Queries per-constraint effectiveness and categorizes items as high (>= 0.7), low (< 0.7), or insufficient (< 3 surfaces). Returns counts for monitoring. Triggered by reflection pattern #9 (guidance health < 0.5).

### `adjust_guidance_confidence` (corrective)
- **Boosts** items with effectiveness >= 0.7 (records `followed` outcome)
- **Decays** items with effectiveness < 0.1 AND >= 5 surfaces (records `ignored` outcome)
- **Archives** stale constraints below the archive threshold
- Triggered by reflection pattern #15 (guidance health < 0.7)

### `archive_ineffective_constraints` (cleanup)
Delegates to `ConfidenceUpdater.ArchiveStaleConstraints()`. Archives all constraint nodes in the space whose confidence has fallen below the configured threshold.

## Reflection Patterns

| Pattern | ID | Threshold | Action | Purpose |
|---------|----|-----------|---------|---------|
| #9 | `low_guidance_follow_rate` | health < 0.5 | `review_guidance_effectiveness` | Diagnostic — surfaces when guidance is mostly ignored |
| #15 | `guidance_confidence_drift` | health < 0.7 | `adjust_guidance_confidence` | Corrective — triggers earlier so confidence drift is addressed before critical |

Pattern #15 has a higher threshold (0.7 vs 0.5) to catch drift before it becomes critical. Both can trigger simultaneously when health is below 0.5.

## SignalLearner Integration

The Hebbian signal learner now tracks guidance signals alongside RSIC-internal signals:

- **Emissions** recorded in `Guide()` for every surfaced guidance item after filtering
- **Responses** recorded in `RecordOutcome()` for `followed` and `partial_compliance` outcomes
- Signal codes: `guidance:<constraint_code>` if available, else `guidance:<type>`
- Independent of persistence — works even when Neo4j persistence is disabled

## ConfidenceUpdater Extension

### All guidance types now tracked
The `item.Type == GuidanceConstraint` guard was removed. All guidance types (constraints, corrections, patterns, decisions, etc.) with source nodes now receive confidence updates.

### Archive guard for non-constraints
The Cypher archive logic now checks `n.constraint_type IS NOT NULL` before setting status to `archived`. This prevents non-constraint nodes (corrections, learnings) from being archived due to low confidence, since archiving is only appropriate for constraints.

## Configuration Reference

| Variable | Default | Purpose |
|----------|---------|---------|
| `RSIC_GUIDANCE_CALIBRATION_ENABLED` | `true` | Master switch for RSIC-SK1 actions |
| `RSIC_GUIDANCE_MIN_SURFACES` | `3` | Min surfaces before effectiveness is meaningful |
| `RSIC_GUIDANCE_BOOST_THRESHOLD` | `0.7` | Effectiveness rate above which confidence is boosted |
| `RSIC_GUIDANCE_DECAY_THRESHOLD` | `0.1` | Effectiveness rate below which confidence is decayed |
| `RSIC_GUIDANCE_DECAY_MIN_SURFACES` | `5` | Min surfaces before decay applies |

## Optional Features (Not Implemented)

Each optional feature has explicit acceptance criteria. Do NOT implement unless the criteria are met through measured production data.

- **Opt-A: Suppress Chronically Ignored Items** — Requires any constraint with follow rate < 10% after 10+ surfaces
- **Opt-B: Auto-tune `JIMINY_MIN_CONFIDENCE`** — Requires GuidanceHealth stays below 0.6 after 100+ RSIC cycles
- **Opt-C: Persist SignalLearner State to Neo4j** — Requires server restarts cause GuidanceHealth to drop > 0.1
- **Opt-D: Per-Source-Type Effectiveness Analysis** — Requires SourceDiversity consistently < 0.5

## Test Results

| Test | File | Status |
|------|------|--------|
| `TestExecuteReviewGuidanceEffectiveness` | `internal/ape/task_dispatch_guidance_test.go` | PASS |
| `TestExecuteAdjustGuidanceConfidence` | `internal/ape/task_dispatch_guidance_test.go` | PASS |
| `TestExecuteArchiveIneffectiveConstraints` | `internal/ape/task_dispatch_guidance_test.go` | PASS |
| `TestExecuteAdjustGuidanceConfidence_GetError` | `internal/ape/task_dispatch_guidance_test.go` | PASS |
| `TestReflect_GuidanceConfidenceDrift` | `internal/ape/self_reflect_test.go` | PASS |
| `TestReflect_LowGuidanceFollowRate` | `internal/ape/self_reflect_test.go` | PASS |
| `TestBuildTaskSpec_AdjustGuidanceConfidence` | `internal/ape/task_spec_guidance_test.go` | PASS |
| `TestGuide_SignalLearnerEmission` | `internal/jiminy/guidance_calibration_test.go` | PASS |
| `TestRecordOutcome_SignalLearnerResponse` | `internal/jiminy/guidance_calibration_test.go` | PASS |
| `TestGuidanceSignalCode` | `internal/jiminy/guidance_calibration_test.go` | PASS |

All 10 tests passing. Full `go test ./internal/ape/...` and `go test ./internal/jiminy/...` suites pass. `golangci-lint`: 0 issues.

## Documents Accessed

- `internal/ape/task_dispatch.go` — Added field, setter, 3 executor switch cases, 3 executor methods
- `internal/ape/task_spec.go` — Added `adjust_guidance_confidence` spec case + description
- `internal/ape/self_reflect.go` — Added reflection pattern #15
- `internal/ape/types_rsic.go` — Added `GuidanceCalibrationProvider` interface + `GuidanceEffectivenessItem` type
- `internal/jiminy/service.go` — Extended `RecordOutcome` confidence to all types, added signal emissions/responses, added `UpdateNodeConfidence`/`ArchiveStaleConstraints`/`SetSignalLearner`/`guidanceSignalCode`
- `internal/jiminy/types.go` — Added `SignalLearnerProvider` interface
- `internal/jiminy/confidence_updater.go` — Added `n.constraint_type IS NOT NULL` archive guard
- `internal/api/rsic_adapters.go` — Added `rsicGuidanceCalibrationAdapter`
- `internal/api/server.go` — Wired `SetGuidanceCalibrator` and `SetSignalLearner`
- `internal/config/config.go` — Added 5 `RSIC_GUIDANCE_*` config fields, parsing, struct assignment
- `.env.example` — Added 5 `RSIC_GUIDANCE_*` variables
- `docs/api/api-spec/uats/specs/rsic_guidance_calibration.uats.json` — New UATS spec
