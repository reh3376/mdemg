# Consolidation Performance

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

### The `dynamic_edges` circuit-breaker

The instrumentation showed `post_clustering` was ~29 min — entirely the
`dynamic_edges` step. `CreateDynamicEdges` finds pairs of upper-layer concepts to
link via a **Cartesian product** over all L≥minLayer nodes:

```cypher
MATCH (a:MemoryNode {space_id: $spaceId}), (b:MemoryNode {space_id: $spaceId})
WHERE a.layer >= $minLayer AND b.layer >= $minLayer AND a.node_id < b.node_id ...
```

At 8,705 L3+ nodes that is ~75.8M pairs materialized before the `LIMIT 50` — for a
total yield of a few hundred edges. `CreateDynamicEdges` now counts L≥minLayer
nodes first (via the existing `memorynode_layer_idx`) and **skips the cross-join
loudly** when the count exceeds `DYNAMIC_EDGES_MAX_NODES`:

```
WARN dynamic_edges: SKIPPED — upper-layer node count exceeds the O(n²) cross-join
     ceiling; awaiting the vector-index rewrite  count=8705 ceiling=2000 min_layer=3
```

Small graphs (below the ceiling) keep full behavior. Large graphs skip until the
Sprint-B vector-index rewrite (`db.index.vector.queryNodes`, top-K per node →
O(n·logn)) lets the guard lift.

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

- **Operators:** nothing to do — the guard is on by default. Tunables:
  - `DYNAMIC_EDGES_MAX_NODES` (default 2000) — the L≥minLayer ceiling above which
    `dynamic_edges` skips. `0` disables the guard (restores the O(n²) behavior).
  - `CONSOLIDATE_TIMEOUT_MS` (default 60 min) — server-side cycle deadline for the
    manual path.
- **Watch the breakdown:** the `mdemg_consolidation_phase_duration_seconds` gauge
  shows where a cycle spends its time; `step:<name>` rows break down composite
  phases.

## What's next (Sprint B)

- **`dynamic_edges` vector-index rewrite** — restore the edges at O(n·logn) so the
  guard can lift on large graphs.
- **Incremental ForwardPass/BackwardPass** — `forward_initial` + `backward` (~27s)
  full-scan all L1 nodes every cycle; the `last_forward_pass`/`last_backward_pass`
  properties + indexes already exist to gate them to changed patterns only.
- **`summaries_llm`** (120s, manual-path only) — bound / make async.

## Reference

- Sprint: `docs/development/consolidate-perf-001/`
- Code: `internal/hidden/service.go` (`RunConsolidation`, `CreateDynamicEdges`,
  `countNodesAtOrAboveLayer`), `internal/hidden/pipeline.go`,
  `internal/api/handlers.go`, `internal/metrics/collectors.go`
