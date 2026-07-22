# CONSOLIDATE-PERF-002 — Sprint Post

**Date:** 2026-07-22 | **Branch:** `reh3376_dev01`
**Parent:** CONSOLIDATE-PERF-001's named Sprint B + the TYPED-EDGES-002
retrieval-contention capacity note.

## Results (live, mdemg-dev)

| Phase | legacy 7d avg (max) | incremental (5 live cycles) | factor |
|---|---|---|---|
| forward_initial | 17.0s (26.0) | 0.18–0.33s | ~60× |
| backward | 15.8s (25.2) | 0.23–0.38s | ~55× |
| step:dynamic_edges | 23.7s (34.0) | 0.45–0.56s | ~50× |
| post_clustering | 29.2s (91.8) | 2.48s | ~12× |

≈55s → ≈1s per cycle on the targeted phases, × ~14 cycles/day ≈ **12+
minutes/day of heavy Neo4j load removed** — directly attacking the
consolidation-vs-retrieval contention TYPED-EDGES-002 documented.

## What shipped

- `HIDDEN_INCREMENTAL_PASSES_ENABLED` (code default false; `.env` true
  after smoke): forward L1/L2+ and backward passes collect pending ids
  (gates: `last_*_pass` NULL bootstrap; member `b.updated_at`; membership
  `r.created_at` on GENERALIZES/ABSTRACTS_TO — catches CHURN-003
  re-assignments of old nodes; cascade advancement), then process
  id-batches. ⚠️ **Never combine a gate with SKIP/LIMIT pagination** —
  stamping nodes mid-pagination mutates the filtered set and SKIP jumps
  over still-pending nodes (pinned in `TestIncrementalByIDQueriesShape`).
- **One source of truth for the math**: aggregation bodies extracted to
  shared consts used by BOTH paths; `TestLegacyPassCypherComposition` pins
  the composed legacy strings byte-for-byte against pre-refactor goldens
  (flag-off is provably unchanged).
- `DYNAMIC_EDGE_INCREMENTAL_LOOKBACK_HOURS` (default 6; 0 = full sweep):
  dynamic-edge seeds restricted to recently created/updated nodes when
  incremental is on. Edges MERGE, so an over-wide window only wastes work.

## The live-caught defect (own fix-commit)

Cycle 1: forward dropped to 0.28s but **backward stayed at 16.2s** — the
cascade clause `nc.last_forward_pass > h.last_backward_pass` selected EVERY
L1, because the theme/emergent forward writers
(`forwardPassConversationThemes` / `forwardPassEmergentConcepts`) stamp
`last_forward_pass` unconditionally on their node classes every cycle.
A stamp is not a change. Fix: the incremental forward post-stamps
**`last_forward_change`** on only the nodes it actually re-aggregated
(cheap query over the pending-id set), and the backward cascade keys on
that. Cycle 2: backward 0.28s.

## Propagation verification (surgical)

- A fresh novel observation stayed orphan (below CHURN-003's 0.80
  assignment threshold — expected, not this sprint's concern), so the
  binding test touched ONE existing member's `updated_at`:
  next cycle, exactly its L1 re-aggregated (`last_forward_pass` +
  `last_forward_change` + `last_backward_pass` all fresh) and its concept
  cascaded (`c.last_forward_pass` fresh) — while phases stayed at
  0.18/0.23/0.46s. The gates recompute precisely the changed ancestry.
- State note: that member's `updated_at` now reads the touch time (true —
  it was touched); the probe observation remains as an ordinary note.

## Caveats (documented in code + feature doc)

Weight-only drift (decay touching edge weights, no timestamps) and
theme-writer-only concept changes are invisible to the gates; both
self-correct when any member changes, and `concepts recluster` /
`full_recluster` remains the explicit full path. Rollback:
`HIDDEN_INCREMENTAL_PASSES_ENABLED=false` → byte-identical legacy sweeps
(golden-pinned).

## Verification checklist

- [x] Golden composition + shape + gate pins green; `go test ./internal/hidden/ ./internal/config/`
- [x] build + lint clean
- [x] Live: 5 cycles — idle ≈1s total on targeted phases; change-cycle
      re-aggregates exactly the touched chain
- [x] Backward-cascade defect fixed + re-measured live
- [x] Docs: feature doc §Sprint-B, CHANGELOG, CLAUDE.md note, this post
- [x] Env-var drift checker clean

## Documents Accessed

`internal/hidden/service.go` (ForwardPass/BackwardPass/CreateDynamicEdges +
theme/emergent forward writers), `internal/hidden/incremental_passes.go`
(new), live `metric_samples` phase table (7d baseline + 5 sprint cycles),
live Neo4j timestamp-coverage + chain-freshness queries,
CONSOLIDATE-PERF-001 / HIDDEN-CHURN-003 / RETRIEVAL-TYPED-EDGES-002 notes.
