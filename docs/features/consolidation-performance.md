# Consolidation Performance


## Sprint B (CONSOLIDATE-PERF-002, 2026-07-22) — incremental forward/backward passes

`HIDDEN_INCREMENTAL_PASSES_ENABLED` (code default false; dev `.env` true
after live smoke): the forward (L1 + L2+) and backward passes gate on
`last_forward_pass`/`last_backward_pass` vs member `updated_at`, membership
edge `created_at`, and cascade advancement — then process
collect-pending-ids in id-batches (never gate+SKIP/LIMIT: stamping mutates
the filtered set mid-pagination). Aggregation math lives in ONE shared
const per pass used by both paths; `TestLegacyPassCypherComposition` pins
the composed legacy strings byte-for-byte. The backward cascade keys on
`last_forward_change` — stamped only by the incremental forward on real
updates — because theme/emergent forward writers stamp `last_forward_pass`
unconditionally every cycle (live-caught: a stamp-based cascade selected
every L1). `DYNAMIC_EDGE_INCREMENTAL_LOOKBACK_HOURS` (6; 0 = full sweep)
bounds dynamic-edge seeds to recently created/updated nodes.

Live (mdemg-dev, vs the 7d legacy averages):

| Phase | legacy avg | incremental |
|---|---|---|
| forward_initial | 17.0s | **0.18–0.33s** |
| backward | 15.8s | **0.23–0.38s** |
| step:dynamic_edges | 23.7s | **0.45–0.56s** |
| post_clustering | 29.2s | 2.48s |

≈55s → ≈1s per cycle on these phases × ~14 cycles/day ≈ **12+ min/day of
heavy Neo4j load removed** (the retrieval-contention driver from
RETRIEVAL-TYPED-EDGES-002). Propagation verified surgically: touching ONE
member's `updated_at` re-aggregated exactly its L1 (forward + change-stamp +
backward) and cascaded its concept, at incremental speed.

Correctness caveats (documented): weight-only drift (decay touching edge
weights) and theme-writer-only concept changes are invisible to the gates —
both self-correct when any member changes; `concepts recluster` /
`full_recluster` remains the explicit full path. Rollback:
`HIDDEN_INCREMENTAL_PASSES_ENABLED=false` restores byte-identical legacy
sweeps.

## Why

Memory consolidation builds the hierarchy (L0 observations → L1 patterns → L2–L5
concepts) and runs message passing. On a large graph (`mdemg-dev`: ~83k nodes) a
full cycle was taking ~38 min and saturating Neo4j CPU 2–3×/day — the real signal
behind the `neo4j-cpu` alert (ALERT-TRUTH-001). CONSOLIDATE-PERF-001 (Sprint A)
instrumented the pipeline and removed the dominant cost.

## How it works

### Per-phase instrumentation

Every consolidation phase emits `mdemg_consolidation_phase_duration_seconds{space_id,phase}`
via the shared `metrics.RecordConsolidationPhase`. It is wired into **both**
consolidation paths:

- `RunConsolidation` (`internal/hidden/service.go`) — the RSIC-watchdog driver
  (the 2–3/day cycles).
- `handleConsolidate` (`internal/api/handlers.go`) — the manual
  `POST /v1/memory/consolidate` path.

Phases: `node_creation`, `forward_initial`, `concept_clustering`
(+ `concept_forward_repeats` sub-signal), `backward`, `post_clustering`,
and `summaries` / `summaries_llm` / `refresh_edges` / `auto_prune`. The pipeline
executor (`RunPhaseRange`/`Run`) additionally emits per-step timing as
`step:<name>` (e.g. `step:dynamic_edges`, `step:emergent_l5`) so a composite
phase can be broken down.

Query the breakdown:

```sql
SELECT labels->>'phase' AS phase, round(value::numeric,1) AS sec
FROM metric_samples
WHERE metric_name='mdemg_consolidation_phase_duration_seconds' AND space_id='mdemg-dev'
ORDER BY value DESC;
```

### The `dynamic_edges` fix (circuit-breaker → vector-index top-K)

The instrumentation showed `post_clustering` was ~29 min — entirely the
`dynamic_edges` step. `CreateDynamicEdges` finds pairs of upper-layer concepts to
link via a **Cartesian product** over all L≥minLayer nodes:

```cypher
MATCH (a:MemoryNode {space_id: $spaceId}), (b:MemoryNode {space_id: $spaceId})
WHERE a.layer >= $minLayer AND b.layer >= $minLayer AND a.node_id < b.node_id ...
```

At 8,705 L3+ nodes that is ~75.8M pairs materialized before the `LIMIT 50` — for a
total yield of a few hundred edges. Sprint A shipped an interim circuit-breaker
(`DYNAMIC_EDGES_MAX_NODES`, default 2000) that skipped the cross-join loudly on
large graphs. **Superseded (RETRIEVAL-TYPED-EDGES-002, 2026-07-03):** the breaker
is removed — `CreateDynamicEdges` now runs a per-node top-K query over the
`memNodeEmbedding` vector index (O(n·logn)), so it runs on graphs of any size.

### Result (live)

| | Before | After |
|---|---|---|
| `dynamic_edges` step | ~29 min | 34 ms (skipped) |
| `post_clustering` phase | ~1,760,000 ms | 7,674 ms (~230×) |
| Watchdog cycle (CPU driver) | ~38 min | ~47 s |
| Manual cycle | 30-min timeout | 167 s |

Identity is preserved (no churn collapse) and Neo4j CPU stays below the alert
threshold.

## How to use

- **Operators:** nothing to do — the vector-index path is on by default. Tunables:
  - `DYNAMIC_EDGE_MIN_LAYER` (default 1), `DYNAMIC_EDGE_TOPK` (default 10),
    `DYNAMIC_EDGE_SIM_THRESHOLD` (default 0.30), `DYNAMIC_EDGE_OVERSAMPLE`
    (default 8) — the vector-index top-K knobs (RETRIEVAL-TYPED-EDGES-002;
    `DYNAMIC_EDGES_MAX_NODES` is removed).
  - `CONSOLIDATE_TIMEOUT_MS` (default 60 min) — server-side cycle deadline for the
    manual path.
- **Watch the breakdown:** the `mdemg_consolidation_phase_duration_seconds` gauge
  shows where a cycle spends its time; `step:<name>` rows break down composite
  phases.

## What's next (Sprint B)

- **`dynamic_edges` vector-index rewrite** — SHIPPED 2026-07-03
  (RETRIEVAL-TYPED-EDGES-002): per-node top-K over the `memNodeEmbedding` vector
  index; the circuit-breaker guard is removed.
- **Incremental ForwardPass/BackwardPass** — `forward_initial` + `backward` (~27s)
  full-scan all L1 nodes every cycle; the `last_forward_pass`/`last_backward_pass`
  properties + indexes already exist to gate them to changed patterns only.
- **`summaries_llm`** (120s, manual-path only) — bound / make async.

## Reference

- Sprint: `docs/development/consolidate-perf-001/`
- Code: `internal/hidden/service.go` (`RunConsolidation`, `CreateDynamicEdges`,
  `countNodesAtOrAboveLayer`), `internal/hidden/pipeline.go`,
  `internal/api/handlers.go`, `internal/metrics/collectors.go`
