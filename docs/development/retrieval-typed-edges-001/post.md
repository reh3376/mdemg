# RETRIEVAL-TYPED-EDGES-001 (Sprint B) — Post

**Status: Phase 1 SHIPPED default-off; the A/B shows Phase 1 alone is a no-op → Phase 2 (grow the edges) is the real lever.** · 2026-07-01 · branch `reh3376_dev01`

Operator directive: *"we most certainly want typed semantic edges
(ANALOGOUS_TO/BRIDGES/etc.) to influence retrieval."* This sprint built the
scorer mechanism (Phase 1) and **measured** whether it moves retrieval quality.
It doesn't — and the measurement precisely explains why, which is the deliverable.

## Shipped (Epics 0–2)

1. **Config-driven weights (Epic 1).** The 7 typed semantic-edge attention
   weights (ANALOGOUS_TO 0.55, BRIDGES 0.60, COMPOSES_WITH 0.50, CONTRASTS_WITH
   0.40, INFLUENCES 0.45, DEFINES_SYMBOL 0.70, THEME_OF 0.65) were hardcoded in
   `ComputeEdgeAttention` — now `EDGE_ATTENTION_*` (defaults = prior literals).
2. **RRF graph column wired to typed edges (Epic 2).** Behind
   `RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED` (default-off), `column_graph.go` spreads
   via `SpreadingActivationWithAttention` (all typed edges, config-weighted)
   instead of the basic `SpreadingActivation` (CO_ACTIVATED_WITH only). The flag +
   the 7 weights join `scorerVersion()` (cache namespace). The 8 new keys are
   exposed in the `/ui/` config tab.

## The A/B (Epic 3) — clean, definitive, negative

Ingested the `whk-wms` codebase (8,973 L0 nodes, 35,146 symbols) into the
`lnl-demo-whk` space and ran the UVTS quick A/B (flag off vs on):

| | mean | median | correct_file_rate | regressions | improvements |
|---|---|---|---|---|---|
| OFF | 0.4080 | 0.45 | 0.562 | — | — |
| ON | 0.4080 | 0.45 | 0.562 | 0 | 0 |
| **Δ** | **+0.0000** | — | — | — | — |

**Byte-identical across all 16 questions / 8 categories.** Verified the flag
*actually took effect* (not a wiring bug): `retrieval_audit` shows both
`tge=off` and `tge=on` scorer versions (cache-isolated → real recompute).

### Why zero — the actual finding

Two compounding reasons, both measured:

1. **The semantic edges the directive targets barely exist.** Their only producer
   is `dynamic_edges`, which creates almost none (gated by L3+ count and the
   CONSOLIDATE-PERF-001 circuit-breaker). `lnl-demo-whk`: **0 ANALOGOUS_TO, 0
   BRIDGES, 1 INFLUENCES** (vs 44,607 ASSOCIATED_WITH + 8,749 GENERALIZES
   structural). `mdemg-dev`: only 258 ANALOGOUS_TO / 14 BRIDGES / 36 INFLUENCES
   across 355k edges. There is nothing for the scorer to spread through.
2. **The graph column's spreading doesn't reach the final ranking.** Even when
   flag-on spreads through the 44k+ *structural* edges, the RRF top-5 is
   unchanged — the graph column (weight 0.15) is dominated by embedding (0.50) +
   BM25 (0.20), and its activation changes don't surface new candidates (it
   re-ranks the same vector-recall seeds).

## Conclusion & the real path (Phase 2)

**Phase 1 (scorer wiring) is necessary but not sufficient.** Shipping it
default-off is correct — measure-first prevented flipping a no-op default-on.
Achieving the directive requires **Phase 2**, now precisely scoped by this A/B:

1. **Grow the semantic edges** — the `dynamic_edges` O(n·logn) vector-index
   rewrite (deferred from CONSOLIDATE-PERF-001) so ANALOGOUS_TO/BRIDGES form in
   quantity and keep forming as the graph grows.
2. **Make the typed-edge signal reach the ranking** — the graph column re-ranks
   existing candidates; typed edges need to *expand the candidate pool* (surface
   semantically-connected nodes that vector recall misses), or a dedicated
   typed-edge column with enough weight. Re-run this A/B after (1)+(2).

## Testing
- **Tier 1:** `TestComputeEdgeAttention_SemanticWeightsConfigDriven`,
  `TestScorerVersion_FlipsOnTypedEdges`.
- **Tier 3 (live):** the UVTS quick A/B above on the real server + a freshly
  ingested corpus; flag effect confirmed via `retrieval_audit` scorer versions.
  Verdict: `ab_verdict_quick.json`.

## Documents Accessed
- `internal/retrieval/activation.go`, `column_graph.go`, `service.go`
- `internal/config/config.go`, `yaml_config.go`
- `internal/api/ui/tabs/config.js`
- `docs/tests/uvts/` harness; `mdemg ingest`
