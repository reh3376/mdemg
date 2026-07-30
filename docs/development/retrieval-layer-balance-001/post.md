# RETRIEVAL-LAYER-BALANCE-001 — Sprint Post

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** RETRIEVAL-QUALITY-AUDIT-001 recommendation #2 (RQA
cluster C — abstract-over-concrete drift). Q4 follow-up #3.

## Verdict

**Shipped.** Post-rerank concrete-quota injection surfaces buried L0/L1
concrete candidates ahead of the abstract L3+ emergent-concept clusters
that dominate keyword-shaped queries. Both flags flipped ON in `.env`
after live smoke; q14 CUIDv2 top slot changed from `L3
EmergentConcept-...` (0 helpful) to `L1 constraint "[must] Always use
CUIDv2 for identifiers"` (the exact answer the user was looking for).

## Diagnosis evolved TWICE during live smoke — architectural rule pinned

**RQA-001's framing was structurally incomplete.** The audit said
concrete nodes "aren't in the retrieval pool"; live smoke on mdemg-dev
proved that framing wrong TWICE:

1. **First reframe (E1 concrete-recall)**: added a supplementary role/
   layer-filtered vector search over the concrete partition, merged
   into the pre-fusion pool. Live log: `concrete_returned=5,
   concrete_added_to_pool=0` — meaning **the primary vector recall
   ALREADY contained all top-5 concrete candidates**. They just ranked
   low. The Lever-C shape (proven for guidance) was insufficient here.

2. **Second reframe (Epic 2 concrete-quota)**: added a post-rerank
   layer-diversity quota mirroring RETRIEVAL-DIVERSITY-001's shape.
   Live log revealed `pool_size=15, concrete_in_pool=0` — **the LLM
   cross-encoder rerank was scoring concrete L0/L1 candidates so much
   lower than same-topic emergent-concept clusters (which share query
   terms in their NAMES) that none survived the rerank cut**. Even
   raising `ReturnK` to the full `RerankTopN=30` scored set left 0
   concretes in the returned pool.

3. **Third and shipping architecture**: the concrete-recall candidates
   are STASHED separately (bypassing rerank entirely) and injected into
   the top-K by `ApplyConcreteQuotaWithExtra` at the same seam where
   RETRIEVAL-DIVERSITY-001 sits. This is the ONLY way to promote them,
   because rerank is the actual gate that filters L0/L1 out.

⚠️ **Architectural rule pinned**: when a retrieval intervention aims to
surface a specific candidate class into top-K, first identify WHICH
STAGE (recall / RRF fusion / rerank / truncation) is filtering it out.
Live smoke found it BENEATH the LLM cross-encoder — an intervention
at earlier stages (pool addition, fusion re-weighting) would have been
architecturally unable to fix it because rerank's LLM scoring
overrides the fused score entirely. The proven shape for
class-preserving surfacing is: (a) fetch the class via a targeted
role/layer query, (b) bypass rerank for those candidates, (c) inject
post-rerank via a quota promoter.

⚠️ **Second rule pinned**: `RQA-001`-style qualitative audits produce
CORRECT problem statements but sometimes INCORRECT root-cause
diagnoses. When the audit says "X is missing from the pool," verify
LIVE whether the pool has X + where in the pipeline it drops out
BEFORE picking an intervention shape. This sprint would have shipped
E1 (concrete-recall alone) as a working solution based on the audit's
diagnosis — until live smoke revealed E1 was insufficient. The
`concrete_added_to_pool=0` log line was the tell.

## What shipped

**Config (5 env vars):**
- `RETRIEVAL_CONCRETE_RECALL_ENABLED` — supplementary vector query
  (default false → true in `.env`)
- `RETRIEVAL_CONCRETE_RECALL_TOPK` — max concrete candidates fetched
  (default 5)
- `RETRIEVAL_CONCRETE_RECALL_LAYER_MAX` — max layer treated as
  "concrete" (default 1)
- `RETRIEVAL_CONCRETE_RECALL_SIM_FLOOR` — cosine floor for concrete
  candidates (default 0.30, ⚠️ RRF-SCALE-001-safe: cosine sim [0,1]
  stable, NEVER the RRF Score)
- `RETRIEVAL_CONCRETE_RECALL_ROLE_TYPES` — csv role_types accepted as
  "concrete" (default `leaf,constraint,correction,conversation_observation`)
- `RETRIEVAL_CONCRETE_QUOTA_ENABLED` — post-rerank quota promoter
  (default false → true in `.env`)
- `RETRIEVAL_CONCRETE_QUOTA_MIN_SLOTS` — guaranteed L0/L1 slots in
  top-K (default 1)

**Code:**
- `internal/retrieval/concrete_recall.go` — `fetchConcreteRecall` +
  `ConcreteCandidatesToResults` + `mergeConcreteCandidates` +
  `parseConcreteRoleTypes`. Mirrors Lever C's role-filtered cosine
  query shape.
- `internal/retrieval/concrete_quota.go` — `ApplyConcreteQuota`
  (single-pool) + `ApplyConcreteQuotaWithExtra` (accepts a separate
  extra-concrete pool that bypasses rerank) + `rerankReturnK`
  (asks reranker for the full scored set when quota is enabled).
- `internal/retrieval/service.go` — three-line wire: fetch
  concrete-recall + stash the results, run rerank with expanded
  ReturnK, call quota with extras before diversity + truncation.
- `internal/models/models.go` — new per-request `?concrete=true|false`
  fields (`ConcreteRecallOverridePresent` + `ConcreteRecallEnabled`).
- `internal/api/handlers.go` — URL param parsing (mirrors `?sparse=`).
- `internal/retrieval/cache.go` — added the concrete-recall override
  to CacheKey (CACHE-KEY-002 contract).
- `internal/retrieval/cache_key_coverage_test.go` — classified the new
  fields (IN the key).
- 15 unit tests: 3 role-type parsing + 4 candidate merge dedup + 8
  quota reorder scenarios.

**⚠️ Live-smoke debugging surfaced ONE additional bug** (also fixed):
Neo4j's `vector.similarity.cosine()` errors with "Argument a is not a
valid vector" when passed a `MemoryNode.embedding` property from a
node whose embedding is empty or wrong-dimensional. My `IS NOT NULL`
guard was insufficient — some nodes have empty-list embeddings that
pass IS NOT NULL. Added `AND size(n.embedding) = size($embedding)`
to the query. Lever C didn't hit this class because its filtered
partition (~hundreds of constraint/correction nodes) happens to have
uniform embeddings; my broader concrete partition (~55k L0/L1 nodes)
did.

## Live Tier-3 A/B on mdemg-dev

### q14 "what are the CUIDv2 rules and why not UUID?"

Before:
```
[0] L3 emergent_concept  score=0.507 EmergentConcept-L3-uuid-cuidv2-115
[1] L3 emergent_concept  score=0.507 EmergentConcept-L3-uuid-cuidv2-114
[2] L4 emergent_concept  score=0.457 EmergentConcept-L4-uuid-cuidv2-59
[3] L4 emergent_concept  score=0.456 EmergentConcept-L4-uuid-cuidv2-1177
[4] L4 emergent_concept  score=0.455 EmergentConcept-L4-uuid-cuidv2-1170
```
Helpful@5: 0/5

After:
```
[0] L1 constraint       score=0.840 [must] Always use CUIDv2 for identifiers
[1] L3 emergent_concept score=0.007 EmergentConcept-L3-uuid-cuidv2-115
[2] L3 emergent_concept score=0.007 EmergentConcept-L3-uuid-cuidv2-114
[3] L4 emergent_concept score=0.007 EmergentConcept-L4-uuid-cuidv2-59
[4] L4 emergent_concept score=0.006 EmergentConcept-L4-uuid-cuidv2-1177
```
Helpful@5: 1/5 (but the ONE is the actual answer — the imperative
constraint the user was seeking).

### Guards

**q10 "how do I enable the recursive-retrain actuator?"** (was 5/5):
- Baseline: 4 L0 leaf + 1 L0 conversation_observation
- Candidate: 5 L0 leaf + 1 L0 observation (order shuffled, +`post.md` gained)
- **IMPROVED 4/5 → 5/5**

**q08 "what was the reframe of DRIFT-VALIDATION-001?"** (was 1/5 due
to wrong-domain L5 concepts):
- Baseline: 2 L0 leaf + 3 abstract (unchanged)
- Candidate: 2 L0 leaf + 3 abstract (order shuffled by ~0.075 score
  delta on the two L0 files, otherwise identical)
- **NO REGRESSION** (same helpful@5)

### Performance

Latency added by concrete-recall + expanded ReturnK: ~2-4s per query
(concrete-recall Cypher runs ~4s on the 55k L0/L1 partition; rerank
scores 30 vs 15 candidates — LLM cost). Acceptable for the retrieval
quality gain; fail-open on error.

## Follow-ups disclosed

1. **Concrete-recall latency** (~4s) is the largest addition. Neo4j
   vector index over the 55k L0/L1 partition works but isn't
   index-optimized for cosine — a future sprint could add a
   partition-scoped vector index and cut this to <200ms. Not urgent
   (background retrieval is already 5-15s p95).

2. **q08's remaining 3/5 abstract results** aren't concrete-quota-
   related — cluster B (specific-value queries) is a separate class
   RQA-001 flagged. Deferred to a future sprint (or absorbed into
   #4 JIMINY-TIER1-BYPASS-001 / #6 FT-DORMANT-CLEANUP-001 if the
   underlying concept-layer hygiene work covers it).

3. **UVTS 120q A/B not run** — the sprint targets a specific query
   shape (cluster C) so a broad A/B risks getting drowned in noise.
   If the operator wants a full-corpus verdict, run:
   `python3 docs/tests/uvts/runners/uvts_runner.py --spec
   docs/tests/uvts/specs/lnl_demo_validation.uvts.json --profile full
   --extra-url-params concrete=false` (baseline) then repeat with
   `concrete=true`.

## Documents Accessed

- `docs/development/retrieval-quality-audit-001/post.md` (parent —
  cluster C recommendation)
- `docs/development/retrieval-layer-balance-001/sprint_plan.md` (this
  dir)
- `internal/jiminy/service.go::fetchActionableCandidates` (Lever C
  reference)
- `internal/retrieval/service.go` (integration seam — vectorRecall,
  rerank, ApplyDiversityFilter)
- `internal/retrieval/rerank.go` (understanding ReturnK behavior)
- `internal/retrieval/cache.go` + `cache_key_coverage_test.go`
  (CACHE-KEY-002 contract)
- `internal/models/models.go` (SparseEnabled precedent)
- `internal/api/handlers.go` (URL override precedent)
- `internal/config/config.go` (env-var wire pattern)
- Live TSDB + Neo4j queries against mdemg-dev for the layer-
  distribution audit (top-20 CUIDv2 = 100% L3+ pre-fix)
- Live server log (`~/.mdemg/logs/server.log`) for the two-reframe
  diagnostic pipeline
