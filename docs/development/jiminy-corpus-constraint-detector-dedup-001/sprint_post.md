# JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001 — Sprint Post

**Shipped**: 2026-08-14
**Sprint plan**: `sprint_plan.md`
**Verdict**: Detector-side canonical single-emit + telemetry + config knob + docs SHIPPED. E4 retroactive tombstone was a **no-op** — AUDIT-004 had already cleaned all live twin pairs.

## 1. What shipped

### Code
- `internal/conversation/constraint_detector.go`
  - New `severityPrecedence` map (package-level, pin-tested): `must_not:5 > must:4 > should_not:3 > should:2 > deadline:1`
  - New `dedupEnabled bool` field on `ConstraintDetector` (default `true` via constructor)
  - New `SetDedupEnabled(bool)` setter (called from `NewServiceWithConfig` when config flag differs from default)
  - New `SkippedSuppressed int` field on `DetectedConstraint` (JSON tag `skipped_suppressed,omitempty`), populated on the winning result of a collapse
  - `Detect()` extended with a collapse branch that runs when `dedupEnabled && len(bestByType) > 1`
  - `slog.Debug("constraint detector: multi-emit collapsed to canonical", chosen_type, suppressed_types)` on each collapse
- `internal/conversation/service.go`
  - Wired `SetDedupEnabled(cfg[0].ConstraintDetectorDedupEnabled)` in service constructor
  - `Observe()` reads `SkippedSuppressed` from returned constraints and increments the metric via `metrics.Metrics().ConstraintDetectorMultiEmitSuppressed(req.SpaceID)`
- `internal/config/config.go`
  - New `ConstraintDetectorDedupEnabled bool` struct field
  - New env var `CONSTRAINT_DETECTOR_DEDUP_ENABLED`, default `true`
  - Wired through `FromEnv` + return literal
- `internal/metrics/collectors.go`
  - New `ConstraintDetectorMultiEmitSuppressed func(spaceID string) *Counter` field on the `Collectors` struct
  - Registered `mdemg_constraint_detector_multi_emit_suppressed_total{space_id}` in the constructor

### Tests
- `internal/conversation/constraint_detector_dedup_test.go` — 8 new pin tests + 4 sub-tests:
  1. `TestDetectorDedup_LiveInstance1_MustNotWins` — real content from the audit-004 twin pair
  2. `TestDetectorDedup_LiveInstance2_MustNotWins` — the second twin
  3. `TestDetectorDedup_LegitimatelyMultiSeverity_NotCollapsedWhenDisabled` — regression pin on the disable path
  4. `TestDetectorDedup_LegitimatelyMultiSeverity_CollapsedWhenEnabled_DocumentsTradeOff` — documents the known trade-off
  5. `TestDetectorDedup_SingleSeverity_NoOp` — no metric increment when only one type matches
  6. `TestDetectorDedup_ConfigDefault_TrueSafe` — safe default pin
  7. `TestDetectorDedup_ConfigDisabled_ByteIdenticalToPreSprint` — full disable path parity
  8. `TestDetectorDedup_SeverityPrecedenceOrder` (4 sub-tests) — full precedence ordering
  9. `TestDetectorDedup_RegressionPin_ClauseName` — precedence map exported at package level
  10. `TestDetectorDedup_MetricFieldIsExported` — field access + JSON tag shape

All green.

### Documentation
- `docs/features/constraint-detector.md` (new, first feature doc for this component)
- CLAUDE.md pin
- CHANGELOG entry

### Inventory
- `docs/api/metrics_consumer_inventory.json` — regenerated via `scripts/verify_metrics_consumers.py --generate`; new metric auto-classified `IN_USE_TSDB_ONLY` (no dashboard/alert consumer yet; recorder flushes to `metric_samples`)
- `verify_metrics_consumers.py` returns clean (146 declared / 146 inventoried)

## 2. Live Tier-3 (mdemg-dev)

- Rebuilt + re-signed + kickstarted binary
- POST /v1/conversation/observe with dual-severity content in scratch space `jiminy-corpus-detector-dedup-live-smoke`:
  - Content 1: "You must never invoke schema migrations manually — every DDL change must be a migration file."
    - Result: `detected_constraints=[{constraint_type: "must_not", confidence: 1}]` — SINGLE canonical emit, `must_not` wins over `must` per precedence
  - Content 2: "You should prefer bounded contexts and you should not overload service names."
    - Result: `detected_constraints=[{constraint_type: "should_not", confidence: 0.85}]` — SINGLE canonical emit, `should_not` wins over `should`
- Metric snapshot: `mdemg_constraint_detector_multi_emit_suppressed_total{space_id="jiminy-corpus-detector-dedup-live-smoke"} = 2` — one suppression per collapse
- Cleaned up: `DETACH DELETE` on the 2 smoke-space nodes

## 3. E4 outcome (no-op)

The sprint plan called for retroactive tombstone of 4 live duplicate constraint nodes. Investigation found **0 live dual-mint pairs remaining** — the JIMINY-CORPUS-AUDIT-004 batch on 2026-08-13 had already tombstoned them (e.g., `pwa2lmy6qgu81r10r5xch9nv` was archived with `archive_reason='operator_disposition_2026-08-13_dedup_of_must_not_twin_wrong_semantics'`). E4 is therefore a no-op: the corpus is already clean.

The forward-fix in E1 prevents new duplicates from forming going forward. No `batch_record.md` or `pre_batch_snapshot.json` created — nothing to tombstone.

## 4. Arc-safety

Preserved. Code changes (E1-E3) are forward-only detector behavior on new observe traffic; no substrate mutation; no impact on the JIMINY-CEILING-BREAK-2 T+168h passive re-check on 2026-08-19. `constraint_outcomes` historical rows untouched. The metric flowing to `metric_samples` is additive, not consumed by any existing alert or dashboard.

## 5. Two arch rules pinned to CLAUDE.md

1. **One L0 observation MUST mint ≤1 constraint node.** Enforced UPSTREAM at the detector (this sprint), not at the promoter. Severity precedence is `must_not > must > should_not > should > deadline`. Any new severity bucket added to `initPatterns` MUST get a slot in `severityPrecedence` — missing entry = ineligible-to-win-collapse (falls through to pre-sprint emit-all path).
2. **Genuinely-multi-rule content should be split into separate `observe` calls.** The detector's dedup trade-off (cannot distinguish "one rule with mixed language" from "two rules in one obs") is documented + regression-pinned. Operators authoring rules should submit them one-per-observe.

## 6. Files touched

- `internal/conversation/constraint_detector.go` — clause + field + splice
- `internal/conversation/constraint_detector_dedup_test.go` — 8 new pin tests
- `internal/conversation/service.go` — wired dedup + metric increment
- `internal/config/config.go` — new config knob (3 sites)
- `internal/metrics/collectors.go` — new counter (2 sites)
- `docs/features/constraint-detector.md` — new feature doc
- `docs/api/metrics_consumer_inventory.json` — regenerated
- `docs/development/jiminy-corpus-constraint-detector-dedup-001/sprint_post.md` — this file
- `CHANGELOG.md` — Unreleased entry
- `CLAUDE.md` — arch rules pin

## 7. Documents Accessed

- `internal/conversation/constraint_detector.go` (existing pattern layer + Detect loop)
- `internal/conversation/service.go` (constraint detector wiring in NewServiceWithConfig + Observe call site)
- `internal/hidden/constraint_nodes.go` (L1 promoter — unchanged; consumes fixed detector output)
- `internal/hidden/constraint_gate.go` (JIMINY-CORPUS-001 gate — downstream reference)
- `internal/conversation/constraint_detector_test.go` (existing pin patterns)
- `internal/metrics/collectors.go` (metric struct + registration pattern)
- `internal/config/config.go` (config surface for constraint module)
- `docs/development/jiminy-corpus-audit-004/sprint_post.md` (precedent for the audit that surfaced this class)
- `docs/development/create-correction-dedup-001/sprint_post.md` (sibling defect pattern)
- `scripts/verify_metrics_consumers.py` (metrics inventory forcing function)
- Live Neo4j queries on `mdemg-dev` to confirm 0 live dual-mint pairs remain
- Live TSDB queries + `/v1/metrics/snapshot` to confirm counter increments
