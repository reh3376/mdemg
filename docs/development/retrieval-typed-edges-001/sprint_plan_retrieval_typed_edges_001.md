# Sprint Plan — RETRIEVAL-TYPED-EDGES-001 (Sprint B): typed semantic edges in RRF retrieval

## 1. Header & Metadata
- **Sprint ID**: RETRIEVAL-TYPED-EDGES-001
- **Sprint line**: `docs/development/retrieval-typed-edges-001/`
- **Date opened**: 2026-06-30
- **Target version**: v0.12.0 (minor — retrieval behavior change)
- **Estimated effort**: ~2 dev-days
- **OpenAI spend**: $0
- **Risk level**: Medium — a retrieval-quality change. The existing basic
  `SpreadingActivation` filter (CO_ACTIVATED_WITH only) exists *specifically* to
  prevent activation saturation from dense structural connectivity; routing
  typed edges into spreading must be measured, not assumed.

## 2. Problem Statement
Typed semantic edges (`ANALOGOUS_TO`, `BRIDGES`, `COMPOSES_WITH`,
`CONTRASTS_WITH`, `INFLUENCES`, `DEFINES_SYMBOL`, `THEME_OF`, …) encode exactly
the cross-concept connections MDEMG exists to surface. But the **default RRF
scorer ignores them**: its graph column (`column_graph.go`) runs the basic
`SpreadingActivation`, which propagates only through `CO_ACTIVATED_WITH`
(`activation.go:63`). The edge-attention variant that weights typed edges
(`SpreadingActivationWithAttention`) already exists but runs only on the legacy
Jiminy path (`service.go:724`, reached when `req.JiminyEnabled`). Net: the
substrate forms semantic links that never influence what it retrieves.

Operator directive (2026-06-30): *"we most certainly want typed semantic edges
to influence retrieval … our goal is to improve the functionality and utility of
mdemg."* This is core connection-layer work (memory→retrieval→synthesis), aligned
with MDEMG's purpose.

Two enabling facts found in code:
- **The edges are already fetched.** `fetchOutgoingEdges` loads `type(r) IN
  $allowed` (the configured `AllowedRelationshipTypes`), so the graph column
  already *has* the typed edges — the gap is purely the spreading function.
- **The semantic-edge attention weights are hardcoded** in `activation.go`
  (`AnalogousTo=0.55`, `Bridges=0.60`, …) — a no-hardcoding violation to fix as
  part of this work.

## 3. Scope & Constraints
**In scope:**
1. Make the semantic-edge attention weights config-driven (`EDGE_ATTENTION_*`,
   current values as defaults).
2. Route the RRF graph column through `SpreadingActivationWithAttention` behind
   `RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED` (default-off initially).
3. Verify the typed semantic edges are in `AllowedRelationshipTypes` (fetched);
   add any missing.
4. Saturation guard + weight tuning so dense structural edges
   (`GENERALIZES`/`ABSTRACTS_TO`) don't peg activation.
5. UVTS A/B measurement — the gate for flipping default-on.

**Out of scope (→ dependent Phase 2, the `dynamic_edges` vector-index rewrite to
GROW the edge population):** Phase 1 measures the scorer change against the
existing ~272 semantic edges first; if they help but are too sparse to move the
needle, Phase 2 grows them. Measure before building.

**Constraints:** no hardcoded values; sequential epics, docs last; live Tier-3 +
UVTS A/B required; **RRF-SCALE-001 safe** — gate on scale-invariant signals
(cosine / typed-edge presence), never raw `RetrieveResult.Score`; cache namespace
must include the new flag/weights (scorer-version); dev branch → auto-PR.

## 4. Dependencies
- `internal/retrieval/activation.go` (`SpreadingActivation`,
  `SpreadingActivationWithAttention`, `ComputeEdgeAttention`,
  `EdgeAttentionWeights`).
- `internal/retrieval/column_graph.go` (the RRF graph column).
- `internal/retrieval/service.go` (`fetchOutgoingEdges`,
  `AllowedRelationshipTypes`, `scorerVersion`).
- `internal/config/config.go` (`EdgeAttention*` fields).
- UVTS harness: `make test-uvts-full BASE_URL=…`, `uvts_ab_compare.py`.

## 5. Implementation Plan (sequential epics + gates)
- **Epic 0** — this plan committed.
- **Epic 1 — config-driven weights.** Add `EDGE_ATTENTION_{ANALOGOUS_TO,BRIDGES,
  COMPOSES_WITH,CONTRASTS_WITH,INFLUENCES,DEFINES_SYMBOL,THEME_OF}` (defaults =
  the current hardcoded values); `ComputeEdgeAttention` reads them. *Gate*: unit
  test the weights resolve from config + the defaults match the old literals.
- **Epic 2 — wire attention into the RRF column.** `column_graph.go` calls
  `SpreadingActivationWithAttention` (weights via `ComputeEdgeAttention`) when
  `RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED` (default-off). Confirm the typed semantic
  edges are in `AllowedRelationshipTypes`. Fold the flag into the cache
  namespace (`scorerVersion`). *Gate*: with the flag on, a seeded query's
  activation differs measurably when a typed edge bridges to a node.
- **Epic 3 — UVTS A/B.** Full 120q: flag-off (baseline) vs flag-on, against the
  live graph's existing semantic edges. Apply the Note 02 merge gate (B mean ≥ A
  mean AND no per-question regression > 0.10). Tune the semantic-edge weights;
  watch the saturation failure mode (broad activation flattening). *Gate*: a
  committed verdict file + a decision — flip default-on, iterate weights, or
  escalate to Phase 2 (edges too sparse).
- **Epic 4 — flip + record.** If the A/B passes, default-on; else keep off with
  the measured reason. Record whether Phase 2 (grow edges) is warranted.
- **Epic 5 — Documentation** (final): feature doc, CLAUDE.md note, CHANGELOG, the
  A/B verdict artifact.

## 6. Testing Plan (3 tiers)
- **Tier 1:** config-weight resolution (defaults == old literals); attention
  spreading respects typed weights; a saturation-bound unit (dense structural
  edges don't peg activation to 1.0).
- **Tier 2:** seeded-graph retrieval — a query reaches an analogous concept
  *only* with the flag on; off-path parity (flag-off == current behavior, byte-for-byte).
- **Tier 3 (live UVTS A/B):** the 120q corpus, flag off vs on, on the real
  server + graph; the `uvts_ab_compare.py` verdict is the headline deliverable.

## 7. Commit Strategy
One commit per epic on `reh3376_dev01`; final promotes CHANGELOG → v0.12.0.
Push → auto-PR.

## 8. Verification Checklist
- [ ] semantic-edge attention weights config-driven (defaults == old literals)
- [ ] RRF graph column uses attention spreading behind the flag
- [ ] typed semantic edges confirmed in `AllowedRelationshipTypes`
- [ ] flag folded into the cache scorer-version namespace
- [ ] flag-off path is byte-identical to current behavior (parity test)
- [ ] saturation checked (Tier-1 bound + the A/B failure-mode watch)
- [ ] UVTS A/B verdict committed; Note 02 merge gate applied
- [ ] RRF-SCALE-001 safe (no raw-score gating introduced)
- [ ] `golangci-lint` + build clean
- [ ] CLAUDE.md + CHANGELOG + feature doc + verdict artifact

## 9. Documentation Update — Epic 5 above.

## 10. Risks & Mitigations
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Activation saturation from dense structural edges | Medium | High | Per-type weights + a Tier-1 saturation bound + the A/B per-question regression gate; the flag ships default-off until measured |
| The 272 semantic edges are too sparse to move retrieval | Medium | Medium | Phase 1 measures exactly this; a null result routes to Phase 2 (grow edges via the vector-index rewrite) rather than a blind flip |
| Introducing a raw-score gate (RRF-SCALE-001 regression) | Low | High | Gate only on cosine / typed-edge presence; checklist item + review |
| Cache staleness across the flag flip | Low | Medium | Fold the flag + weights into `scorerVersion` so the namespace changes automatically |

## 11. Documents Accessed
- `internal/retrieval/activation.go`, `column_graph.go`, `service.go`
- `internal/config/config.go` (EdgeAttention* + AllowedRelationshipTypes)
- `docs/tests/uvts/` harness
- CLAUDE.md RRF-SCALE-001 / Column-Voting / sparse-retrieval notes

## 12. Rollback Procedures
The behavior change is entirely behind `RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED`
(default-off until the A/B passes); revert the flip or the commit(s). Config
weights default to the prior literals. No schema/migration.
