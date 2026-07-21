---
created: 2026-03-23
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: RSIC-SK1
---

# RSIC-SK1: Jiminy Guidance Self-Calibration

## Summary

**Feature**: RSIC-SK1 Guidance Self-Calibration
**Summary**: Closes three feedback loop gaps in the Jiminy guidance system, creating a self-improving guidance pipeline where chronically ignored guidance decays automatically and consistently followed guidance strengthens.

## Vision & Goals

A guidance system that cannot learn from its own effectiveness is static. RSIC-SK1 closes the feedback loop: guidance items that are consistently followed get boosted confidence (surfaced more), items that are chronically ignored get decayed (surfaced less), and items below the archive threshold are removed. This makes Jiminy a genuine learning system — it gets better at guidance over time based on measured outcomes.

## Current State

### Architecture

```
Guide() --> Emit items --> SignalLearner.RecordEmission()
                |
                v
         Agent acts on guidance
                |
                v
RecordOutcome() --> Classify outcome (followed/ignored/contradicted)
                |              |
                v              v
    ConfidenceUpdater      SignalLearner.RecordResponse()
    (all guidance types)   (followed/partial only)
                |
                v
    RSIC Reflect --> Pattern #9 (health < 0.5): review_guidance_effectiveness
                     Pattern #15 (health < 0.7): adjust_guidance_confidence
                |
                v
    RSIC Dispatch --> review_guidance_effectiveness (diagnostic)
                      adjust_guidance_confidence (boost/decay/archive)
                      archive_ineffective_constraints (cleanup)
                |
                v
         Guide() improved (higher-confidence items surface, low-confidence decay)
```

**Three Gaps Closed:**

1. **Dispatcher gap**: Reflection pattern #9 emits `review_guidance_effectiveness` but dispatcher had no executor — added switch case and executor method
2. **Confidence scope gap**: ConfidenceUpdater only fired for `GuidanceConstraint` types — removed type guard, all guidance types now receive confidence updates
3. **SignalLearner isolation**: Created at `server.go` but never wired to Jiminy — now wired via `SetSignalLearner()`, tracks guidance signals alongside RSIC-internal signals

### Workflow

**New RSIC Actions:**

- **`review_guidance_effectiveness`** (diagnostic): Queries per-constraint effectiveness, categorizes as high (>=0.7), low (<0.7), or insufficient (<3 surfaces). Triggered by pattern #9.
- **`adjust_guidance_confidence`** (corrective): Boosts items with effectiveness >=0.7, decays items with effectiveness <0.1 AND >=5 surfaces, archives stale constraints. Triggered by pattern #15.
- **`archive_ineffective_constraints`** (cleanup): Archives all constraint nodes below confidence threshold.

**Reflection Patterns:**

| Pattern | ID | Threshold | Action |
|---------|----|-----------|--------|
| #9 | `low_guidance_follow_rate` | health < 0.5 | `review_guidance_effectiveness` |
| #15 | `guidance_confidence_drift` | health < 0.7 | `adjust_guidance_confidence` |

**SignalLearner Integration:**

- Emissions recorded in `Guide()` for every surfaced item after filtering
- Responses recorded in `RecordOutcome()` for `followed` and `partial_compliance` outcomes
- Signal codes: `guidance:<constraint_code>` if available, else `guidance:<type>`

**ConfidenceUpdater Extension:**

- All guidance types now tracked (type guard removed)
- Archive guard: Cypher checks `n.constraint_type IS NOT NULL` before archiving — non-constraint nodes (corrections, learnings) not archived

### Configuration

See Configuration Reference table below.

## Notes

### Known Limitations

- Effectiveness requires minimum 3 surfaces before meaningful

### Risks & Gaps

- Opt-C (persist SignalLearner to Neo4j) — shipped: state persists to Neo4j (V0024) with a 30s background flush loop

### Future Improvements

**Optional features (require measured production data):**

- **Opt-A**: Suppress chronically ignored items (follow rate <10% after 10+ surfaces)
- **Opt-B**: Auto-tune `JIMINY_MIN_CONFIDENCE` (health stays <0.6 after 100+ cycles)
- **Opt-C**: Persist SignalLearner state to Neo4j (shipped — see Risks & Gaps above)
- **Opt-D**: Per-source-type effectiveness analysis (SourceDiversity <0.5)

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| GET | `/v1/self-improve/signals` | Signal effectiveness tracking | N/A |
| POST | `/v1/self-improve/assess` | Triggers RSIC assessment (includes guidance health) | `specs/rsic_assess.uats.json` |

## CLI Commands

None — RSIC-SK1 operates automatically via RSIC cycle and Jiminy feedback.

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `RSIC_GUIDANCE_CALIBRATION_ENABLED` | `true` | Master switch for RSIC-SK1 actions |
| `RSIC_GUIDANCE_MIN_SURFACES` | `3` | Min surfaces before effectiveness is meaningful |
| `RSIC_GUIDANCE_BOOST_THRESHOLD` | `0.7` | Effectiveness rate above which confidence is boosted |
| `RSIC_GUIDANCE_DECAY_THRESHOLD` | `0.1` | Effectiveness rate below which confidence is decayed |
| `RSIC_GUIDANCE_DECAY_MIN_SURFACES` | `5` | Min surfaces before decay applies |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| RSIC Engine | Requires — dispatches review/adjust/archive actions |
| Jiminy Inner Voice | Requires — guidance items emitted and outcomes recorded |
| SignalLearner | Requires — Hebbian signal effectiveness tracking |
| ConfidenceUpdater | Requires — per-item confidence boost/decay |
| Constraint Nodes | Feeds into — low-confidence constraints archived |

## Related Files

- `internal/ape/task_dispatch.go` - 3 executor methods for guidance actions
- `internal/ape/task_spec.go` - `adjust_guidance_confidence` spec case
- `internal/ape/self_reflect.go` - Reflection pattern #15
- `internal/ape/types_rsic.go` - `GuidanceCalibrationProvider` interface
- `internal/jiminy/service.go` - Extended RecordOutcome, SetSignalLearner
- `internal/jiminy/confidence_updater.go` - Archive guard for non-constraints
- `internal/api/rsic_adapters.go` - `rsicGuidanceCalibrationAdapter`
- `internal/api/server.go` - Wiring: SetGuidanceCalibrator, SetSignalLearner
- `internal/config/config.go` - 5 `RSIC_GUIDANCE_*` config fields
