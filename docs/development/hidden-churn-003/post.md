# HIDDEN-CHURN-003 — Sprint Post

## Summary

Closed the must-fix residual from HIDDEN-CHURN-002. The hidden (L1) layer now uses **incremental clustering** as the default consolidation step: only orphan L0 nodes are assigned to existing patterns (or clustered into new ones), and existing patterns are never destroyed or re-clustered. **Live Tier-3: 100% identity survival** (vs CHURN-002's 75%) and steady-state cycle wall-time **~10s** (vs ~360s full re-cluster).

## What shipped

- **`IncrementalHiddenNodes`** (default consolidation hidden step, `HIDDEN_INCREMENTAL_ENABLED=true`): fetch orphan L0 nodes → `assignOrphansToPatterns` (Go-side `nearestPatternByCentroid`, cosine ≥ `HIDDEN_INCREMENTAL_ASSIGN_SIM_THRESHOLD` 0.80 → batch `GENERALIZES` edges with CUIDv2 edge ids + cosine weights + incremental-mean centroid update) → `clusterNewBaseNodes` for the unassigned remainder (create-only CUIDv2; reclassification skipped as a churn source). **No existing pattern is destroyed or re-clustered.**
- `fetchOrphanBaseNodes` (refactored `fetchAllBaseNodes` → shared `fetchBaseNodes(orphanOnly)`).
- Explicit full re-cluster retained: `mdemg concepts recluster --space-id <id>` → consolidate `full_recluster` field → `hidden.WithFullRecluster(ctx)` override of the incremental dispatch.
- Pure helpers extracted for unit-testing: `nearestPatternByCentroid`, `incrementalMean`.

## Live Tier-3 evidence (mdemg-dev)

| | CHURN-002 (full re-cluster) | CHURN-003 (incremental) |
|---|---|---|
| Identity survival / cycle | ~75% (3093/4144) | **100% (4484/4484)** |
| Patterns destroyed / cycle | ~1,100 | **0** (only added: 4484→4758) |
| Steady-state cycle wall-time | ~360s | **~10s** (82 orphans) |

Cycle 1 cleared a one-time 3,094-orphan backlog (716 assigned, 266 new patterns, 61s); cycle 2 was steady-state (0 assigned to existing, 82 orphans → 8 new patterns, 10s). Every original pattern node_id survived both cycles.

## Trade-off + follow-up

Incremental-only clustering lets pattern count grow slowly (no periodic re-cluster on the auto path). The explicit `mdemg concepts recluster` command is the quality-maintenance escape hatch; split/merge maintenance (merge near-duplicate patterns, split oversized) is a possible future refinement if drift is observed. `HIDDEN_INCREMENTAL_ENABLED=false` reverts to the CHURN-002 full path.

## Testing

- **Tier 1** (`hidden_identity_test.go`): `hiddenIncrementalAssignThreshold` default/override; `nearestPatternByCentroid` (match / below-threshold / empty); `incrementalMean` (weighted mean + length-mismatch guard).
- **Tier 2** (`tests/integration/hidden_test.go::TestHiddenIncrementalAssignment`): form patterns → ingest more → incremental re-consolidate → assert all original node_ids survive + count never drops (no deletes).
- **Tier 3** (this doc): real binary, 2 incremental cycles on mdemg-dev, 100% survival + ~36× wall-time reduction.

## Documents Accessed
- `internal/hidden/service.go` (`fetchBaseNodes`, `IncrementalHiddenNodes`, `clusterNewBaseNodes`), `hidden_identity.go` (`assignOrphansToPatterns`, `nearestPatternByCentroid`, `incrementalMean`), `step_hidden.go`, `clustering.go`, `theme_identity.go` (`assignNoiseToThemes`)
- `internal/api/handlers.go` (`handleConsolidate` full-recluster override), `internal/models/models.go`, `internal/cli/concepts.go`, `internal/config/config.go`
- Live mdemg-dev (pattern counts/ids, cycle wall-time, `~/.mdemg/logs/server.log`)
