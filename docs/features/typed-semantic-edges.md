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

## Status: default-ON (Phase 2 grew the edges; clean A/B passed)

Phase 1's A/B was +0.0000 because the semantic edges barely existed. Phase 2
(RETRIEVAL-TYPED-EDGES-002) grew them via a `dynamic_edges` vector-index rewrite
(O(n·logn) per-node top-K over `memNodeEmbedding`; `DYNAMIC_EDGE_MIN_LAYER`
lowered to 1 so semantic edges reach the abundant L1/L2 concept layers) — **4,415
typed semantic edges on a test corpus, up from ~1**. The clean full-120q A/B on a
quiet, warmed system:

| Config | mean | correct_file |
|---|---|---|
| OFF | 0.4120 | 0.608 |
| **ON (default)** | **0.4130** | **0.617** |

**+0.0010 mean, +0.009 correct-file, 0 regressions, 5 improvements, no latency
cost** (flag-on = flag-off, ~4.2s). Small but real and harmless — so
`RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED` is now **default-on**.

⚠️ **Amplification is a dead end:** raising the graph column weight (0.15→0.25) to
amplify the typed-edge signal *regresses* (crowds out embedding/BM25 — the
Phase-13 finding). Keep the natural graph weight 0.15.

## Dynamic-edge creation (the vector-index rewrite)

`CreateDynamicEdges` now, per L≥`DYNAMIC_EDGE_MIN_LAYER` node, fetches its top
(`DYNAMIC_EDGE_TOPK`×`DYNAMIC_EDGE_OVERSAMPLE`) nearest neighbours from the
`memNodeEmbedding` vector index, keeps those in the same space at the same
min-layer, not-already-connected, under the degree cap, and with cosine
`sim ≥ DYNAMIC_EDGE_SIM_THRESHOLD` — then infers the typed edge (ANALOGOUS_TO /
BRIDGES / etc.). O(n·logn), bounded per-node.

| Env var | Default |
|---|---|
| `DYNAMIC_EDGE_MIN_LAYER` | 1 |
| `DYNAMIC_EDGE_TOPK` | 10 |
| `DYNAMIC_EDGE_SIM_THRESHOLD` | 0.30 |
| `DYNAMIC_EDGE_OVERSAMPLE` | 8 |

## What's next (optional research)

- A **dedicated additive typed-edge column** (6th RRF column) — the one untried
  amplification lever (adds the typed-edge signal without weakening
  embedding/BM25). Uncertain ROI given the weak signal on the test corpus.

## How to use

- **Operators:** default-**on**; no action needed — typed edges influence
  retrieval out of the box. To opt out: `RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED=false`
  (or toggle in the `/ui/` config tab). The `EDGE_ATTENTION_*` weights and the
  `DYNAMIC_EDGE_*` edge-growth knobs are tunable; re-verify any change with the
  UVTS A/B (Note-02 gate). ⚠️ Do **not** raise the graph column weight to amplify
  the signal — that regresses (see above).

## Reference
- Sprint: `docs/development/retrieval-typed-edges-001/` (Phase 1 — scorer) +
  `docs/development/retrieval-typed-edges-002/` (Phase 2 — grow edges, default-on;
  `post.md`, `ab_verdict_full.json`)
- Code: `internal/retrieval/{activation.go,column_graph.go,service.go}`,
  `internal/hidden/service.go` (`CreateDynamicEdges`)
