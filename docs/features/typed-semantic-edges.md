# Typed Semantic Edges in Retrieval

## Why

MDEMG forms typed semantic edges between concepts — `ANALOGOUS_TO`, `BRIDGES`,
`COMPOSES_WITH`, `CONTRASTS_WITH`, `INFLUENCES`, `DEFINES_SYMBOL`, `THEME_OF` —
that encode cross-concept relationships. The goal is for these to influence what
retrieval surfaces (connection-layer quality). RETRIEVAL-TYPED-EDGES-001 (Phase 1)
built the scorer mechanism to spread activation through them.

## How it works

The RRF graph column (`internal/retrieval/column_graph.go`) seeds from vector
recall, fetches the seed set's outgoing edges, and spreads activation. By default
it uses **basic `SpreadingActivation`**, which propagates only through
`CO_ACTIVATED_WITH` (learned co-activation) — the typed semantic edges are fetched
but ignored (this filter exists to prevent activation saturation from dense
structural connectivity).

With `RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED=true`, the column instead uses
**`SpreadingActivationWithAttention`**, which weights every edge type via
`ComputeEdgeAttention`. The 7 semantic weights are config-driven:

| Env var | Default |
|---|---|
| `EDGE_ATTENTION_ANALOGOUS_TO` | 0.55 |
| `EDGE_ATTENTION_BRIDGES` | 0.60 |
| `EDGE_ATTENTION_COMPOSES_WITH` | 0.50 |
| `EDGE_ATTENTION_CONTRASTS_WITH` | 0.40 |
| `EDGE_ATTENTION_INFLUENCES` | 0.45 |
| `EDGE_ATTENTION_DEFINES_SYMBOL` | 0.70 |
| `EDGE_ATTENTION_THEME_OF` | 0.65 |

The flag + weights are folded into `scorerVersion()`, so flipping the flag or
tuning a weight invalidates the retrieval cache. All 8 keys appear in the `/ui/`
config tab.

## Status: default-off (the A/B says Phase 1 alone is a no-op)

The UVTS A/B (flag off vs on, on a freshly-ingested corpus) found **zero
difference** — because:

1. **The semantic edges barely exist.** Their only producer, `dynamic_edges`,
   creates almost none (it's an O(n²) cross-join, gated by a node-count
   circuit-breaker; see CONSOLIDATE-PERF-001). A fresh corpus has ~0
   ANALOGOUS_TO/BRIDGES; mdemg-dev has only ~300 across 355k edges.
2. **The graph column's spreading doesn't reach the final ranking** — at weight
   0.15 it's dominated by embedding (0.50) + BM25 (0.20), and it re-ranks the
   existing vector-recall seeds without surfacing new candidates.

## What's needed to fulfil the goal (Phase 2)

1. **Grow the semantic edges** — rewrite `dynamic_edges` to O(n·logn) via the
   Neo4j vector index (`db.index.vector.queryNodes`, top-K per node) so
   ANALOGOUS_TO/BRIDGES form in quantity and the circuit-breaker can lift.
2. **Make typed edges expand the candidate pool** — surface semantically-connected
   nodes that vector recall misses (not just re-rank existing candidates), or add
   a dedicated typed-edge column with enough weight. Then re-run the A/B.

## How to use

- **Operators:** default-off; no action. To experiment: set
  `RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED=true` (or toggle in the `/ui/` config tab)
  and tune the `EDGE_ATTENTION_*` weights. Re-verify with the UVTS A/B before
  relying on it — Phase 2 is the prerequisite for a real effect.

## Reference
- Sprint: `docs/development/retrieval-typed-edges-001/` (`ab_verdict_quick.json`)
- Code: `internal/retrieval/{activation.go,column_graph.go,service.go}`
