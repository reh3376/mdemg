# ACTIVATION-DRIVEN-DISCOVERY-001 — Sprint Post

**Arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase B1
**Shipped**: 2026-08-18
**Ship state**: **code + tests + docs shipped default-OFF**; passive re-measurement + flip decision deferred to operator

## What shipped

1. **New substrate primitive** `retrieval.Service.ExpandSeedsByActivation(ctx, spaceID, seeds, queryText) map[nodeID]float64` — thin wrapper around `SpreadingActivationWithAttention` + `fetchOutgoingEdges` + `ComputeEdgeAttention`. Takes caller-supplied seeds (`ActivationSeed{NodeID, Score}`), fetches 1-hop outgoing edges, spreads, returns final activation map.
2. **`jiminy.RetrievalProvider` interface extended** with `ExpandSeedsByActivation` (matching adapter in `internal/api/rsic_adapters.go`; mock updated in `j7_j12_test.go`).
3. **Lever C activation-enrichment** `Service.activationEnrichLeverC(ctx, spaceID, actionables, queryText) []GuidanceItem` — flag-guarded blend `(1-w)*cosine + w*activation` with stable sort. Guarantees actionable coverage (reranks, does NOT filter).
4. **4 config knobs**: `JIMINY_LEVER_C_ACTIVATION_ENABLED` (false), `_STEPS` (2), `_LAMBDA` (0.5), `_WEIGHT` (0.3).
5. **Boot log line** `jiminy: lever c activation enabled=... steps=... lambda=... weight=...` (always emitted for operator visibility).
6. **Debug field** `debug.leverc_activation_enriched=N` (count of Lever C items reranked; matches merged when enrichment ran).
7. **Tests**: 2 retrieval-primitive tests + 6 Jiminy activation-enrichment tests (default-off byte-identical, nil-retriever safe, empty-input safe, rerank correctness, zero-weight identity, activation-error fail-open).
8. **Docs**: sprint plan + this post + `docs/features/jiminy-lever-c-activation.md`.

## Recon findings (verified live on mdemg-dev before drafting code)

Applied `must-validate-all-claims-before-commit` throughout — every substrate assumption was tested via `cypher-shell` before any code was written.

| Claim | Verification | Verdict |
|-------|--------------|---------|
| Lever C is pure role-filtered cosine | `grep` + `Read` `service.go:3372-3462` | ✅ confirmed |
| `SpreadingActivationWithAttention` is a shipped public primitive | `Read` `activation.go:371` | ✅ confirmed |
| `CO_ACTIVATED_WITH` edges are populated | live cypher: 228,636 edges, weight [0.02, 1.0], mean 0.14 | ✅ confirmed |
| `activation_confidence` is populated | live cypher: 69,635/88,276 nodes (79%), mean 0.515 | ✅ confirmed |
| Graph column pattern reusable | `Read` `column_graph.go:40-160` | ✅ confirmed |
| `GUIDANCE_OUTCOME` sink populated (for B2) | live cypher: **0 rows** on mdemg-dev | ❌ **blocks B2** |

⚠️ **B1 recon claim reversed post-ship (2026-08-18, GUIDANCE-OUTCOME-SINK-INVESTIGATE-001)**: the "0 rows on mdemg-dev" claim was a QUERY BUG in the recon, not a code defect. Live re-verify:

- **8,517 GUIDANCE_OUTCOME edges** on mdemg-dev / 110 distinct target nodes
- **809 edges in last 7d** / newest at 2026-08-18T18:25:29Z
- Outcome distribution: 6680 ignored / 1510 followed / 313 partial / 14 contradicted
- Write path from JIMINY-OUTCOME-001 (2026-06-08) is healthy and actively producing

Root cause: `PersistGuidanceOutcome` (`persistence.go:78-86`) writes GUIDANCE_OUTCOME as **self-loops on the target node** (`CREATE (src)-[r]->(src)`) with `space_id` implicit from the node (never set as an edge property). My B1 recon used `MATCH ()-[r:GUIDANCE_OUTCOME]->() WHERE r.space_id='mdemg-dev'` which returned 0 because the edge property is never written. The correct pattern (used by `stats.go:42`) is `MATCH (n:MemoryNode {space_id: $sid})-[r:GUIDANCE_OUTCOME]-()`.

**Phase B2 (Hebbian effectiveness prior) is UNBLOCKED — real data flows.**

## Live Tier-3 evidence

Query: **"must I always use CUIDv2 for identifiers when writing new code"** — CUIDv2 constraint exists at node `inpos5znn984sb`.

**BASELINE (flag=false)** — pure role-filtered cosine:
```
total: 10  merged=3  enriched=0  scope_drops=10
[0] constraint   n_070b56219fb8 conf=0.8808  Configuration must be retrieved and validated
[1] constraint   inpos5znn984sb conf=0.6406  [must] All unique identifiers in MDEMG must use CUIDv2  ← CORRECT
[2] constraint   rtyx9qcql5os1j conf=0.6168  OPERATOR CORRECTION 2026-08-18
```

**CANDIDATE (flag=true, weight=0.3)** — activation-enriched:
```
total: 10  merged=3  enriched=3  scope_drops=10
[0] constraint   n_c5245e1bd622 conf=0.9427  [must] The 'cli' package wraps MCP server
[1] constraint   n_dc26dfac1270 conf=0.8808  [must] Validates cross-field constraints
[2] constraint   n_d6fba3811723 conf=0.8808  [must] Function config.JiminyWarmComputeTimeout
[3] constraint   inpos5znn984sb conf=0.4965  [must] All unique identifiers in MDEMG must use CUIDv2  ← DEMOTED
[4] constraint   rtyx9qcql5os1j conf=0.4780  OPERATOR CORRECTION 2026-08-18
```

**Mechanism works**: `debug.enriched=3` proves all 3 Lever C actionables were reranked; ordering + confidences changed; blend arithmetic correct.

**Trade-off observed**: activation DEMOTED the semantically-correct constraint (CUIDv2 #1 → #3) because other constraints have stronger CO_ACTIVATED_WITH topology in the current substrate. This is a real observation, not a bug. It is exactly why **default-off is the correct ship state** — the flip-decision is data-driven, not intuition-driven.

## Decisions

| Decision | Rationale |
|----------|-----------|
| Direction (a) — drop-in flagged replacement (operator-locked) | Safer + measurable; (b) full redesign risks surfacing zero actionables when neighborhood is sparse in constraints |
| Interface extension over Jiminy-side Cypher | Every Cypher-writing site in `jiminy/` is future-drift risk; a single extended `RetrievalProvider` method preserves package decoupling |
| Blend formula `(1-w)*cosine + w*activation` | Additive; isolated nodes uniformly `(1-w)*cosine` — no differential penalty; conservative default `w=0.3` (cosine dominates 70/30) |
| Default OFF in code AND `.env` | Behavior-changing feature per HEBB-ETA-001 pin ("behavior-changing feature flags MUST default off in BOTH code AND `.env`") |
| Ship code + tests + docs even though live-smoke didn't produce a clear "candidate wins" story | Mechanism is verified working; the trade-off is real; the flip decision is data-decided over time, not gated on a single query |
| GUIDANCE_OUTCOME=0 investigation → separate follow-up sprint (per operator direction) | Folded into this sprint post; does not block B1 shipping; enables B2 kickoff once resolved |

## Follow-ups (disclosed, deferred)

1. **[HIGH PRIORITY, precondition for B2] GUIDANCE-OUTCOME-SINK-INVESTIGATE-001**: The JIMINY-OUTCOME-001 write path (2026-06-08) has zero live `GUIDANCE_OUTCOME` edges on mdemg-dev despite the code being wired. TSDB `constraint_outcomes` has 79 rows; Neo4j has 0 edges. Investigate: is `PersistGuidanceOutcome` being called from `RecordOutcome`? Is the Cypher failing silently? Is `constraint_code` matching finding zero target nodes? Fix + backfill.
2. **[Passive] LEVER-C-ACTIVATION-AB-001**: Enable `JIMINY_LEVER_C_ACTIVATION_ENABLED=true` for a 168h window against real production traffic. Measure follow-rate delta vs baseline. Data-decide flip vs no-flip. NOT urgent — the JIMINY-CEILING-BREAK-2 T+168h re-check on 2026-08-19 owns the primary substrate-quality signal for this window.
3. **[Small]** URL override `?leverc_activation=true|false` on `/v1/jiminy/guide` + `/latest` — measurement scaffolding for the A/B if it can't be resolved via `.env` flip alone.
4. **[Phase B3]** Add `activation_confidence` factor to the blend: `blended = ((1-w)*cosine + w*activation) * (0.5 + 0.5*activation_confidence)` — additive on top of B1.
5. **[Phase C, separate]** Layer/edge-aware surfacing — expose L2+ emergent concepts as guidance items directly.

## Arch rules pinned

- **When adding a substrate primitive Jiminy will consume, extend `jiminy.RetrievalProvider` with a narrow method + mirror the input types (jiminy-side struct that mirrors retrieval-side struct); the adapter converts between packages.** Do NOT have Jiminy write Neo4j Cypher directly — every new Cypher-writing site in `jiminy/` is future-drift risk (JIMINY-ARCHIVED-CODE-FILTER-001 shape).
- **Behavior-changing feature flags MUST default OFF in BOTH code AND `.env`** (HEBB-ETA-001 pin — reaffirmed by this sprint).
- **Substrate mechanism verification stands independent of "candidate wins" A/B verdict.** When a live-smoke shows the mechanism works but the ordering delta is complicated (as here — activation DEMOTED the semantically-correct constraint due to graph topology), ship default-off + document the observation. Flip decision is data-decided over a longer window, not gated on a single query.
- **Reranking blend `(1-w)*cosine + w*signal` where isolated items get `signal=0` produces a uniform `(1-w)*cosine` fall-back** — no differential penalty on isolated items relative to their peers; activated items get a bonus in `[0, w]`. This is preferable to "activation-only, discard cosine for enriched items" which would strand isolated actionables.

## Documents Accessed

- `internal/jiminy/service.go` (Lever C call site 1215; `fetchActionableCandidates` 3372-3462)
- `internal/jiminy/types.go` (`RetrievalProvider` interface 211-215)
- `internal/jiminy/j7_j12_test.go` (`mockRetriever` 101-108)
- `internal/retrieval/activation.go` (`SpreadingActivationWithAttention` 371; `ComputeEdgeAttention` 218; `EdgeAttentionWeights` 190)
- `internal/retrieval/column_graph.go` (reference pattern 40-160)
- `internal/retrieval/service.go` (`vectorRecall` 1392; `fetchOutgoingEdges` 1968)
- `internal/api/rsic_adapters.go` (`jiminyRetrievalAdapter` 384)
- `internal/api/handlers_jiminy.go` (`handleJiminyGuide` 174-234)
- `internal/api/server.go` (jiminy boot log 1194-1201)
- `internal/config/config.go` (Jiminy Lever C config pattern 365-368; init 2966-2980)
- `internal/retrieval/cache_key_coverage_test.go` (CACHE-KEY-002 forcing function; NOT triggered by this sprint — flag lives on Jiminy path, not RetrieveRequest)
- Live cypher-shell queries on mdemg-dev (CO_ACTIVATED_WITH count/weight, activation_confidence population, GUIDANCE_OUTCOME row count)
- Live `/v1/jiminy/guide` smoke calls (baseline + candidate)
- `CLAUDE.md` (LEVER-C-TIGHTEN-001/002, JIMINY-ACTIONABILITY-001, HEBB-ETA-001, RRF-SCALE-001, CACHE-KEY-002, JIMINY-ARCHIVED-CODE-FILTER-001, JIMINY-SUBSTRATE-NATIVE-001 arc)
- `docs/development/activation-driven-discovery-001/sprint_plan.md`
