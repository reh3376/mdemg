# Jiminy Lever C — activation-driven reranking

**Sprint**: ACTIVATION-DRIVEN-DISCOVERY-001 (JIMINY-SUBSTRATE-NATIVE-001 Phase B1)
**Shipped**: 2026-08-18
**Default**: OFF (opt-in via `.env`; per-request override via `?leverc_activation=true|false` planned)

## Why

Jiminy's **Lever C** (`fetchActionableCandidates`, `internal/jiminy/service.go`) surfaces actionable constraint/correction nodes by **pure role-filtered vector cosine similarity**. One Neo4j query; no graph traversal; no Hebbian edge-weight signal; no `activation_confidence` weighting. The substrate MDEMG was designed around — a Hebbian-learning multi-layer emergent memory graph — sits with 228,636 CO_ACTIVATED_WITH edges on mdemg-dev that Lever C ignores entirely.

The **JIMINY-SUBSTRATE-NATIVE-001 arc thesis** (operator, 2026-08-18): Jiminy is the internal dialogue; MDEMG is the *infrastructure* that facilitates it. Lever C had been fighting against MDEMG's intended design by reducing guidance surfacing to pure semantic embedding.

Phase B1 exposes the substrate's activation-spreading primitive to Jiminy so Lever C can rerank actionables by what the graph has **learned** about which constraints activate together with which contexts — not just what the text embeds close to.

## Choices

### Direction: (a) drop-in flagged replacement, not full redesign

Operator locked (2026-08-18) direction:
- **(a) drop-in flagged replacement** — keep the current role-filtered cosine seed set (guarantees actionable coverage); rerank the top-K via activation spreading; A/B measurable via URL override; default-off until data justifies flip.
- **(b) full redesign** (start purely from query-embedding → vector-recall → activation) — deferred; risks surfacing zero actionables when the query neighborhood is sparse in constraints.

### Blend formula

```
blended = (1 - w) * item.Confidence + w * activation[node_id]
```

- `w = JIMINY_LEVER_C_ACTIVATION_WEIGHT` (default 0.3 — cosine dominates 70/30, conservative).
- Nodes with no activation score (isolated in the graph) get `activation = 0`, so their blended score is `(1-w) * cosine` — same uniform coefficient across activated + unactivated items; no differential penalty on isolated actionables (**they don't leapfrog activated peers, but they don't get differentially punished either**).

### Reuse the shipped substrate primitive, don't invent a new one

`retrieval.Service.ExpandSeedsByActivation` is a **thin wrapper** around the shipped `SpreadingActivationWithAttention` + `fetchOutgoingEdges` + `ComputeEdgeAttention`. The retrieval graph column (`column_graph.go`) uses exactly the same pipeline; extracting it into a caller-parameterised entry point lets Jiminy (and future non-Jiminy consumers) call it without re-implementing.

### Extend the interface, don't Cypher from Jiminy

`jiminy.RetrievalProvider` gains one method. Alternative: have Jiminy write its own Neo4j query for edge fetch + call the primitive directly. Rejected — every additional Cypher-writing site in `jiminy/` is future drift risk. The `jiminy.ActivationSeed` type mirrors `retrieval.ActivationSeed`; the adapter maps between packages so the two remain decoupled.

### Guarantees actionable coverage

Activation reranks; it does **not** filter. Zero risk of returning fewer actionables than the current pure-cosine. If activation somehow zeroed every score, the seeds themselves are still returned via the blend's `(1-w) * cosine` term.

## How it works

1. Lever C's existing role-filtered cosine query (`fetchActionableCandidates`) runs unchanged, returning the top-K constraint/correction nodes by embedding sim.
2. If `JIMINY_LEVER_C_ACTIVATION_ENABLED=true`, `activationEnrichLeverC` converts those items to `[]ActivationSeed{NodeID, Score=item.Confidence}` and calls `retriever.ExpandSeedsByActivation(spaceID, seeds, queryText)`.
3. The retrieval primitive:
   - Fetches all outgoing edges from the seed set via the existing `fetchOutgoingEdges` (1-hop, batched Cypher).
   - Builds a `QueryContext{QueryText}` and computes per-edge-type attention weights via `ComputeEdgeAttention`.
   - Runs `SpreadingActivationWithAttention(seeds, edges, steps, lambda, attention, hopMin)` — steps + lambda from `JIMINY_LEVER_C_ACTIVATION_STEPS` (2), `_LAMBDA` (0.5).
   - Returns `map[node_id] → activation_score` covering seeds + their newly-activated neighbors.
4. Jiminy blends each item's original cosine confidence with its activation score by the formula above, sorts descending, returns.
5. Downstream (scope-gate, dedup-merge, filter-by-confidence, glossary-code annotation) is unchanged — only the ordering and Confidence of Lever C items changes.

### Fail-open contract

- Nil retriever → identity function.
- Empty actionables → identity function.
- `JIMINY_LEVER_C_ACTIVATION_WEIGHT <= 0` → identity function.
- `retriever.ExpandSeedsByActivation` returns error → identity function + debug log.
- `fetchOutgoingEdges` errors internally → primitive returns `(nil, err)` + WARN log; Jiminy caller sees error and falls back.

## How to use

### Enabling

```bash
# in /Users/reh3376/mdemg/.env
JIMINY_LEVER_C_ACTIVATION_ENABLED=true

# optional tuning
JIMINY_LEVER_C_ACTIVATION_STEPS=2       # propagation steps (default 2)
JIMINY_LEVER_C_ACTIVATION_LAMBDA=0.5    # decay per step [0, 0.9] (default 0.5)
JIMINY_LEVER_C_ACTIVATION_WEIGHT=0.3    # blend weight for activation vs cosine [0, 1] (default 0.3)
```

Restart: `launchctl kickstart -k gui/501/com.mdemg.server`. Boot log confirms:

```
INFO msg="jiminy: lever c activation" enabled=true steps=2 lambda=0.5 weight=0.3
```

### Observability

Boot log line above (always emitted). Per-call debug field:

```json
{ "debug": { "leverc_activation_enriched": 3 } }
```

= number of Lever C actionables that were reranked (equals `leverc_actionable_merged` when no items were dropped).

### Rollback

Set `JIMINY_LEVER_C_ACTIVATION_ENABLED=false`, restart. Byte-identical to pre-B1 behavior.

## Live-smoke observation (2026-08-18 mdemg-dev)

Query: "must I always use CUIDv2 for identifiers when writing new code" (constraint the substrate has as `inpos5znn984sb — All unique identifiers in MDEMG must use CUIDv2`).

- **Baseline (flag=false)**: CUIDv2 constraint ranked **#1** among Lever C actionables (raw cosine 0.6406).
- **Candidate (flag=true, weight 0.3)**: CUIDv2 constraint ranked **#3** (blended 0.4965) — other constraints with stronger CO_ACTIVATED_WITH topology (config-loading, cross-field validation, JiminyWarmComputeTimeout) leapfrogged it.

This is a real substrate observation, not a bug: the CUIDv2 rule has fewer co-activations than other constraints in the current graph state. It's exactly why **default-off is correct** and any default-flip should be data-decided via passive re-measurement against real production traffic (168h+ window, follow-rate delta signal) rather than a single-query intuition test. The mechanism is verified: `debug.enriched=3`, ordering changed, blend arithmetic correct.

## Follow-ups (deferred)

- **Passive A/B measurement** — enable in `.env` for a 168h window; re-measure follow rate vs the baseline window; data-decide whether to flip default. **Not** urgent — the JIMINY-CEILING-BREAK-2 arc T+168h re-check on 2026-08-19 owns the primary substrate-quality signal.
- **Phase B2 — Hebbian effectiveness prior via GUIDANCE_OUTCOME** — **UNBLOCKED**. B1 recon claim of "sink=0 rows" was a query bug corrected 2026-08-18 (see GUIDANCE-OUTCOME-SINK-INVESTIGATE-001 in CLAUDE.md). Live: 8,517 GUIDANCE_OUTCOME edges on mdemg-dev / 809 in last 7d. Correct read pattern is `MATCH (n:MemoryNode {space_id: $sid})-[r:GUIDANCE_OUTCOME]-()` (NOT `WHERE r.space_id=...` — the edge property is not written; space is implicit via the self-loop's node).
- **Phase B3 — precision-confidence weighting** — extend the blend to include `activation_confidence`: `blended = ((1-w) * cosine + w * activation) * confidence_factor`. Additive on top of B1; deferred to keep this sprint compact.
- **URL override** `?leverc_activation=true|false` — mirror `?leverc_topk` / `?reverse_ref` shape. Plumbing is straightforward; deferred as measurement scaffolding until the passive A/B proves insufficient.
- **Phase C — layer/edge-aware surfacing** — expose L2+ emergent concepts as guidance items directly; independent of B1.

## References

- Sprint plan: [`docs/development/activation-driven-discovery-001/sprint_plan.md`](../development/activation-driven-discovery-001/sprint_plan.md)
- Sprint post: [`docs/development/activation-driven-discovery-001/sprint_post.md`](../development/activation-driven-discovery-001/sprint_post.md)
- Substrate primitive: `internal/retrieval/expand_seeds.go`
- Integration: `internal/jiminy/service.go::activationEnrichLeverC`
- Reference pattern: `internal/retrieval/column_graph.go` (`GraphColumn.Run`)
- Related: LEVER-C-TIGHTEN-001/002, JIMINY-ACTIONABILITY-001, HEBB-ETA-001, RETRIEVAL-TYPED-EDGES-001
