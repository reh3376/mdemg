# RETRIEVAL-TYPED-EDGES-002 (Phase 2) — Post

**Status: SHIPPED — typed semantic edges grown + flipped default-on; amplification found to be a dead end. Plus two substrate-robustness fixes surfaced during the A/B.** · 2026-07-03 · branch `reh3376_dev01`

Phase 1 (RETRIEVAL-TYPED-EDGES-001) built the scorer mechanism but the A/B was
+0.0000 because the semantic edges barely existed. Phase 2 grew them and
re-measured — cleanly, on a quiet system.

## Shipped

1. **Epic 1 — `dynamic_edges` vector-index rewrite.** Replaced the O(n²)
   Cartesian cross-join in `CreateDynamicEdges` with a per-node top-K query via
   the `memNodeEmbedding` vector index (O(n·logn)). Two changes made the edges
   actually grow: (a) the vector index instead of `MATCH (a),(b)`, and (b)
   `DYNAMIC_EDGE_MIN_LAYER` default 3→1 (a fresh corpus has ~no L3+ concepts;
   semantic edges must reach the abundant L1/L2 layers). The degree cap now
   counts only the dynamic semantic-edge types (not the dense structural
   membership edges). Removed the now-obsolete CONSOLIDATE-PERF-001 circuit-breaker
   (`DYNAMIC_EDGES_MAX_NODES` + `countNodesAtOrAboveLayer`). Fixed a CUIDv2
   violation (`randomUUID()`→Go-minted). New config:
   `DYNAMIC_EDGE_{MIN_LAYER,TOPK,SIM_THRESHOLD,OVERSAMPLE}`. **Live: 4,415 typed
   semantic edges created on lnl-demo-whk (up from ~1) in 11.5s** (vs the old
   29-min cross-join).

2. **Epic 2 — re-populate + the clean A/B.** Consolidated to build the edges;
   re-ran the UVTS full 120q A/B on a **quiet system** (watchdog consolidation
   disabled, server warmed):

   | Config | mean | correct_file | Δ vs baseline |
   |---|---|---|---|
   | OFF (baseline) | 0.4120 | 0.608 | — |
   | ON, graph w=0.15 | **0.4130** | **0.617** | **+0.0010** (PASS: 0 regressions, 5 improvements) |

   Typed edges now influence retrieval — small, real, harmless (+0.001 mean,
   +0.009 correct-file), **no latency cost** (flag-on 4.2s = flag-off). ⚠️ The
   earlier "16q +0.007" was small-sample noise (the project rule: decide on the
   full corpus). ⚠️ The earlier "flag-on 8.65s / 2.7× slower" was a Neo4j-load
   confound (the A/B had run during whk-wms consolidations), corrected here.

3. **Epic 3 — amplification (a dead end, measured).** Raising the graph column
   weight 0.15→0.25 to amplify the typed-edge signal **regressed** (mean
   0.4130→0.4090, correct_file 0.617→0.583; computed_value −0.016,
   data_flow −0.010) — the Phase-13 finding reconfirmed: amplifying the graph
   column crowds out embedding/BM25, and the typed edges don't compensate. So the
   weight lever fails; the graph column is too weak a signal to amplify.

4. **Decision — flipped default-on.** `RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED`
   default false→true (operator decision): fulfils the directive (typed edges now
   influence retrieval), zero downside (0 regressions, no latency cost), and keeps
   the semantic edges wired in as they grow. Best config is the natural graph
   weight 0.15 (amplification reverted).

## Substrate-robustness fixes (surfaced by the A/B's load-fragility)

The A/B's early unreliability traced to a live retrieval incident, deeply
root-caused:

- **Cache poisoning fix** — `queryCache.Put` cached empty/canceled responses, so
  a transient-0 (from load) was served as a fast 8ms "0 results" for the whole
  TTL. Now guarded (`len(resp.Results) > 0 && ctx.Err() == nil`).
- **Retrieve deadline fix** — `handleRetrieve` used the raw request ctx, bounded
  only by `HTTPWriteTimeout`=600s, so under Neo4j load the column queries hung for
  tens of seconds to minutes. Added `RETRIEVE_TIMEOUT_MS` (20s) so retrieval
  fails-fast (and, per the cache guard, doesn't poison) instead of hanging.
- **Root cause (capacity, not fixed here):** retrieval and consolidation contend
  for Neo4j — retrieval degrades (0/slow) *during* a consolidation window (proven:
  identical queries return results at neo4j CPU 1, fail at CPU 127). The fixes
  make it degrade *gracefully* (self-healing); reducing the contention itself is
  a follow-up capacity item.

## Testing
- **Tier 1:** `TestDynamicEdgeVectorDefaults`; build + full `go test ./...` green.
- **Tier 3 (live):** 4,415 edges created; the clean 120q A/B (verdict
  `ab_verdict_full.json`); the amplification A/B; the two robustness fixes
  live-verified.

## What's next (optional)
- A **dedicated additive typed-edge column** (6th RRF column) — the one untried
  amplification lever (adds the typed-edge signal without weakening
  embedding/BM25). Uncertain ROI given the weak signal; a research item.
- **Retrieval-vs-consolidation Neo4j contention** — the capacity follow-up.

## Documents Accessed
- `internal/hidden/service.go` (`CreateDynamicEdges`), `internal/retrieval/*`
- `internal/config/config.go`, `yaml_config.go`, `internal/api/handlers.go`
- `docs/tests/uvts/` harness; the Phase-1 sprint line
