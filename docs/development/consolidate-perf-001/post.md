# CONSOLIDATE-PERF-001 (Sprint A) — Post

**Status: SHIPPED.** · 2026-06-30 · branch `reh3376_dev01`

Sprint A of the consolidation-performance track: instrument the consolidation
pipeline, then bank the low-risk wins the measurements justify. The headline:
**the instrumentation found the real bottleneck on the first cycle, and it
wasn't where the static analysis pointed.**

## The measurement inverted the assumption

Consolidation was believed to run ~38 min uniformly. The per-phase
instrumentation (Epic 1) showed a steady-state cycle is **~51s** — the first four
phases (node_creation 10s, forward_initial 14s, concept_clustering 1.5s,
backward 14s) — and then **`post_clustering` ran ~29 min and hit the 30-min
timeout**:

```
phase=post_clustering dur_ms=1760506      (~29 min)
phase=summaries dur_ms=0                   (never ran — timed out)
http POST /v1/memory/consolidate duration_ms=1800002  (30-min timeout)
```

The per-step instrumentation pinned it inside `post_clustering` to **`dynamic_edges`**
(`cluster_summary` is default-off; `emergent_l5` is `LIMIT 20`). `CreateDynamicEdges`'
find-pairs query is a **Cartesian product over all L≥minLayer nodes** —
`MATCH (a),(b) WHERE a.layer>=3 AND b.layer>=3` — which at scale (live: **8,705
L3+ nodes → ~75.8M pairs**) dominated the cycle for a paltry yield (272
ANALOGOUS_TO/BRIDGES edges total, `LIMIT 50`/run). The planned cheap win Epic 2
(`findSimilarConcept`) measured at 1.6s — negligible — and was **dropped**.

## Shipped (Epics 1, 3, 4)

1. **Epic 1 — instrumentation.** `mdemg_consolidation_phase_duration_seconds{space_id,phase}`
   (shared `metrics.RecordConsolidationPhase`) around every phase in BOTH
   consolidation paths (service-level `RunConsolidation` = the watchdog driver;
   `handleConsolidate` = manual) + per-pipeline-step timing (`step:<name>`) in
   `RunPhaseRange`/`Run`.
2. **Epic 3 — `dynamic_edges` circuit-breaker.** `CreateDynamicEdges` counts
   L≥minLayer nodes (via the existing `memorynode_layer_idx`) and SKIPS the
   O(n²) cross-join loudly when it exceeds `DYNAMIC_EDGES_MAX_NODES` (default
   2000). Small graphs keep full behavior; large graphs skip until the Sprint-B
   vector-index rewrite. Data-decided replacement for the planned cadence gate
   (cadence wouldn't cut the per-run spike).
3. **Epic 4 — timeout default 30→60 min** (`CONSOLIDATE_TIMEOUT_MS`) so the
   manual path completes instead of aborting mid-cycle.

## Live Tier-3 result (the real binary, restarted, on mdemg-dev)

| Metric | Before | After |
|---|---|---|
| `dynamic_edges` (step) | ~29 min | **34 ms** (skipped, `count=8705 ceiling=2000`) |
| `post_clustering` (phase) | ~1,760,000 ms | **7,674 ms** (~230×) |
| Watchdog cycle (the 2-3/day CPU driver) | ~38 min | **~47 s** (no `summaries_llm`) |
| Manual cycle | 30-min timeout | **167 s** |
| Neo4j windowed CPU | AVG to 471, bursts 1003% | calm (165 post-cycle, < the 500 alert) |
| Identity survival | — | preserved (L1=5130, L3=4634 stable; no churn collapse) |

The 19-alert Neo4j-CPU class (ALERT-TRUTH-001) is eliminated at the source: the
2-3/day full cycles that saturated Neo4j were entirely the `dynamic_edges`
cross-join.

## Sprint-B targets (the now-visible next costs)

- **`summaries_llm` 120s** — `EnhanceSummariesWithLLM` (manual-path only;
  serial local-LLM calls). Bound / make async. Not a CPU concern.
- **forward_initial 14s + backward 13s** — the full-scan ForwardPass/BackwardPass
  (the original #1 hypothesis). The incremental/dirty-tracking rewrite
  (`last_forward_pass`/`last_backward_pass` infra already exists).
- **`dynamic_edges` vector-index rewrite** — restore the edges at O(n·logn) via
  `db.index.vector.queryNodes` (top-K per node) instead of the cross-join, so
  the guard can lift.

## Testing

- **Tier 1:** `TestConsolidationPhaseDuration` (gauge registration/labels).
- **Tier 3 (live):** the table above — deployed, triggered, observed the SKIP
  log + the per-phase/per-step breakdown in TSDB, confirmed identity survival
  and CPU calm.

## Documents Accessed
- `internal/api/handlers.go` (handleConsolidate phases + timeout)
- `internal/hidden/service.go` (`RunConsolidation`, `CreateDynamicEdges`,
  ForwardPass/BackwardPass, `findSimilarConcept`), `pipeline.go`,
  `step_dynamic_edges.go`
- `internal/metrics/collectors.go`
- `internal/config/config.go`
- CLAUDE.md HIDDEN-CHURN-001/002/003 + ALERT-TRUTH-001 notes
