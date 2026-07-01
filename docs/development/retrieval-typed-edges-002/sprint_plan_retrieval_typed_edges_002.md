# Sprint Plan — RETRIEVAL-TYPED-EDGES-002 (Phase 2): grow the semantic edges + re-measure

## 1. Header & Metadata
- **Sprint ID**: RETRIEVAL-TYPED-EDGES-002
- **Sprint line**: `docs/development/retrieval-typed-edges-002/`
- **Date opened**: 2026-07-01
- **Target version**: v0.12.0 (minor — retrieval + edge-creation)
- **Estimated effort**: ~2–3 dev-days
- **Risk level**: Medium (an algorithmic edge-creation rewrite + a retrieval-quality
  change; both measured behind the flag before any default-on)

## 2. Problem Statement
Phase 1 (RETRIEVAL-TYPED-EDGES-001) wired the RRF graph column to spread
activation through typed semantic edges, but the UVTS A/B returned **+0.0000**.
The measured cause: the semantic edges barely exist — their only producer,
`dynamic_edges`, creates ~none (0 ANALOGOUS_TO on a fresh corpus; 258 on
mdemg-dev across 355k edges). A Phase-2 recon also found the graph column
**already expands candidates** (it emits the full activation map — spread-to
nodes, not just seeds; `column_graph.go:104+`), so candidate expansion is not the
missing piece. The missing piece is a **quantity of semantic edges that connect
to relevant nodes**. Secondarily, RRF may suppress single-column typed-edge
candidates (weight 0.15, no cross-column consensus) — measured only if growth
alone stays flat. This sprint grows the edges and re-measures, per the directive.

## 3. Scope & Constraints
**In scope:**
1. **`dynamic_edges` vector-index rewrite** — replace the O(n²) Cartesian
   cross-join (`MATCH (a),(b)`) with per-node top-K via the `memNodeEmbedding`
   vector index (O(n·logn)), so ANALOGOUS_TO/BRIDGES/etc. form in quantity and
   keep forming; relax the CONSOLIDATE-PERF-001 circuit-breaker for the bounded
   new path; preserve the existing edge-type inference (`inferEdgeType`).
2. **Re-populate + re-measure** — run consolidation to build the grown edge set;
   re-run the Phase-1 UVTS A/B (flag off vs on).
3. **Conditional fusion/weight investigation** — only if growth + the existing
   expansion stay flat: diagnose whether typed-edge candidates are RRF-suppressed,
   test a config-gated graph-column weight / consensus tweak, re-A/B.

**Out of scope:** a new dedicated typed-edge column (only if Epic 3 proves it);
changing the RRF algorithm beyond a measured knob; incremental-ForwardPass.

**Constraints:** no hardcoded values (K, sim threshold, degree cap, oversample →
config); RRF-SCALE-001 safe (gate on cosine/typed-edge presence, never raw
`RetrieveResult.Score`); measure-first — every retrieval change behind the Phase-1
flag, A/B before default-on; the edge-creation rewrite preserves the inferred
edge type; sequential epics; live Tier-3 + UVTS A/B required; dev branch → auto-PR.

## 4. Dependencies
- `internal/hidden/service.go` — `CreateDynamicEdges`, `inferEdgeType`,
  `L5SourceMinLayer`, `countNodesAtOrAboveLayer`, `DynamicEdgesMaxNodes` guard.
- The `memNodeEmbedding` Neo4j vector index.
- `internal/retrieval/{column_graph.go, activation.go, scoring_rrf.go}` — the
  Phase-1 flag + `EDGE_ATTENTION_*` weights + the candidate-emitting graph column.
- The UVTS harness + the loaded `lnl-demo-whk` corpus (from Phase 1).

## 5. Implementation Plan (sequential epics + gates)
- **Epic 0** — this plan committed.
- **Epic 1 — vector-index rewrite of `CreateDynamicEdges`.** For each L≥minLayer
  node `a`: `CALL db.index.vector.queryNodes('memNodeEmbedding', K×oversample,
  a.embedding)` → filter to L≥minLayer ∧ not-already-connected ∧ degree-cap ∧
  `sim ≥ DYNAMIC_EDGE_SIM_THRESHOLD` → create the inferred typed edge (CUIDv2 id).
  New config: `DYNAMIC_EDGE_TOPK`, `DYNAMIC_EDGE_SIM_THRESHOLD`,
  `DYNAMIC_EDGE_OVERSAMPLE`. Relax the circuit-breaker (the new path is
  per-node-bounded, not O(n²)). *Gate*: on lnl-demo-whk it creates a meaningful
  semantic-edge population (≫ current ~0), bounded (degree cap), completing fast
  (not the 29-min cross-join). Tier-1: the top-K query + filters + type inference.
- **Epic 2 — re-populate + A/B.** Run consolidation on lnl-demo-whk (and mdemg-dev)
  to build the edges; verify the ANALOGOUS_TO/BRIDGES count grew (record before→after).
  Re-run the UVTS **full** A/B (flag off vs on). *Gate*: a committed verdict — lift
  (Note-02 gate) → flip default-on; flat → Epic 3.
- **Epic 3 — conditional fusion/weight investigation.** Only if Epic 2 is flat:
  diagnose RRF single-column suppression of typed-edge-expanded candidates; test a
  config-gated graph-column weight bump *or* a consensus-reinforcement adjustment;
  re-A/B. *Gate*: measured lift, or a documented "typed-edge retrieval needs a
  dedicated column" finding (→ Phase 3) — never a silent flat.
- **Epic 4 — Documentation** (final): `typed-semantic-edges.md` + `consolidation-performance.md`
  (dynamic_edges now O(n·logn)) updates, CLAUDE.md note, CHANGELOG, the A/B verdict(s).

## 6. Testing Plan (3 tiers)
- **Tier 1:** the per-node top-K edge-creation query + the layer/degree/threshold
  filters; edge-type inference preserved; config resolution (K/threshold/oversample).
- **Tier 2:** run the rewrite on a seeded graph → a bounded, correctly-typed edge
  set; degree cap honored; no O(n²) blowup.
- **Tier 3 (live):** consolidation creates the grown edges on the real corpus
  (before→after counts); the UVTS **full** A/B off vs on is the headline verdict;
  confirm no consolidation-CPU regression (the rewrite stays fast).

## 7. Commit Strategy
One commit per epic on `reh3376_dev01`; final promotes CHANGELOG → v0.12.0.
Push → auto-PR.

## 8. Verification Checklist
- [ ] `CreateDynamicEdges` uses the vector index (no `MATCH (a),(b)` cross-join)
- [ ] semantic-edge population grows measurably + stays bounded (degree cap)
- [ ] rewrite is O(n·logn) fast (no consolidation-CPU regression)
- [ ] edge-type inference preserved; CUIDv2 ids
- [ ] UVTS full A/B verdict committed; Note-02 gate applied; flip decision recorded
- [ ] RRF-SCALE-001 safe (no raw-score gating)
- [ ] no hardcoded values (K/threshold/oversample/degree-cap config)
- [ ] `golangci-lint` + build + full `go test ./...` green
- [ ] `typed-semantic-edges.md` + CLAUDE.md + CHANGELOG + verdict updated

## 9. Documentation Update — Epic 4 above.

## 10. Risks & Mitigations
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Edge flood (per-node top-K creates too many) | Medium | Medium | Degree cap + `sim` threshold + Tier-2 bound assertion; tune K |
| Activation saturation from many semantic edges | Medium | Medium | Attention weights + the A/B per-question regression gate; flag default-off until measured |
| Vector-index top-K misses L≥minLayer neighbors (sparse in global top-K) | Medium | Medium | Oversample + layer-filter; if still sparse, a role/layer-scoped cosine over the small L≥minLayer partition (Lever-C pattern) |
| Growth alone still flat (RRF suppresses typed-edge candidates) | Medium | Medium | Epic 3 measures it explicitly → a real fix or a documented Phase-3 dedicated-column finding |
| RRF-SCALE-001 regression (raw-score gate) | Low | High | Gate only on cosine / typed-edge presence; checklist + review |

## 11. Documents Accessed
- `internal/hidden/service.go` (`CreateDynamicEdges` @ 3071, `inferEdgeType` @ 2796)
- `internal/retrieval/column_graph.go` (candidate-emitting graph column @ 104+)
- `internal/retrieval/activation.go`, `scoring_rrf.go`
- `internal/config/config.go`
- `docs/development/retrieval-typed-edges-001/` (the Phase-1 A/B verdict + finding)
- `docs/tests/uvts/` harness

## 12. Rollback Procedures
The `dynamic_edges` rewrite is config-gated; the retrieval change stays behind the
Phase-1 flag (`RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED`, default-off) until the A/B
passes. Revert the commit(s). No schema/migration (edges are additive; a bad edge
set is cleared by `graph repair` / re-consolidation).
