# RETRIEVAL-LAYER-BALANCE-001 — Sprint Plan

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** RETRIEVAL-QUALITY-AUDIT-001 recommendation #2 (RQA
cluster C — abstract-over-concrete drift). Q4 follow-up #3.

## 1. Header & Metadata

Address RQA-001 cluster C by ensuring concrete L0/L1 nodes reach the
RRF candidate pool for queries where they exist in the substrate.
Follows the shipped Lever C pattern (JIMINY-ACTIONABILITY-001 E5) —
supplementary role/layer-filtered vector search over the concrete
partition, merged into the RRF pool BEFORE fusion. Config-gated
default-off; flip after live A/B. ~1-1.5d effort.

## 2. Problem Statement

RQA-001 cluster C: abstract L3+ emergent-concepts crowd out concrete
L0/L1 rules for queries with strong keyword signal (config names,
constants, rule names). Live reproduction on mdemg-dev 2026-07-29:

**q14 "what are the CUIDv2 rules and why not UUID?"** — **top-20
layer distribution: 100% L3/L4/L5, zero L0 or L1**. Substrate has 15
concrete CUIDv2 nodes (5 L0 leaf files including
`feedback_cuidv2_required.md`, 6 L0 conversation observations, 1 L1
correction, 6 L1 constraints incl. "You must never use UUID v4 in
this codebase") — none surface in retrieval.

**Root cause reframed from RQA-001:** the issue is NOT score-ordering
within the RRF pool (Option A can't fix) or column crowding (Option B
can't fix). The concrete nodes never REACH the fused RRF pool at all
— the primary Embedding column's vector search top-K is dominated by
the ~981 L2-L5 emergent-concepts, so the ~15 L0/L1 concrete candidates
get filtered out at recall time.

Structural analog: JIMINY-ACTIONABILITY-001 Lever C solved the
identical shape for guidance surfacing — the `role_type='constraint'`
partition was 0.1% of nodes, never in the global top-50, so a
supplementary role-filtered cosine query fetched them into the
merge pool. This sprint applies the same shape to the concrete-layer
partition for general retrieval.

## 3. Scope & Constraints

**In scope (single-commit sprint):**

- New helper `fetchConcreteCandidates(ctx, spaceID, queryVec, cfg)`
  in `internal/retrieval/` — role-filtered cosine query over
  `MemoryNode WHERE layer <= LAYER_MAX AND role_type IN
  (CONCRETE_ROLE_TYPES)`, `sim >= SIM_FLOOR`, `LIMIT TOPK`
- Wire in `service.go::Retrieve` right where the RRF candidate pool
  is assembled — merge concrete candidates BEFORE the RRF fusion +
  rerank pipeline, dedup by node_id
- Config knobs (env, code default false):
  - `RETRIEVAL_CONCRETE_RECALL_ENABLED` (default false; flipped ON
    in `.env` after live smoke)
  - `RETRIEVAL_CONCRETE_RECALL_TOPK` (default 5)
  - `RETRIEVAL_CONCRETE_RECALL_LAYER_MAX` (default 1)
  - `RETRIEVAL_CONCRETE_RECALL_SIM_FLOOR` (default 0.30 — matches the
    Lever C floor)
  - `RETRIEVAL_CONCRETE_RECALL_ROLE_TYPES` (default
    `leaf,constraint,correction,conversation_observation`; empty =
    all role types under LAYER_MAX)
- ⚠️ **RRF-SCALE-001-safe**: the concrete-recall query gates on the
  `sim` cosine value ([0,1] stable), NEVER on the RRF `Score` (which
  changes when the scorer changes). Concrete candidates enter the RRF
  pool as regular candidates and compete normally on all columns.
- Live smoke on q14 + q08 (guard) + q10 (guard) via `?concrete=true|false`
  URL override

**Out of scope:**

- Keyword-signal classification (RQA-001 Option A) — deferred; if the
  concrete-recall lift makes it unnecessary, drop entirely
- New RRF column (RQA-001 Option B) — deferred; concrete-recall is a
  simpler pre-fusion intervention
- Full UVTS 120q A/B — the audit was 15 queries and the sprint
  targets a specific cluster; run only if the live smoke on the
  RQA-001 diagnostic queries indicates broad enough impact
- Consolidation-time changes to the emergent-concept layer growth
  (that's a separate arc — clustering hygiene, not retrieval)

## 4. Method

**Phase 1 — Recall function + wire**
- Implement `fetchConcreteCandidates` (mirror `fetchActionableCandidates`
  in `internal/jiminy/service.go`)
- Wire in `service.go::Retrieve` at the pool-assembly seam
- Empty-name / empty-embedding results skipped

**Phase 2 — Config + URL override**
- Add 5 env vars to `config.go` with defaults
- Add `?concrete=true|false` URL param to the retrieve handler
  (mirror the shipped `?sparse=` / `?intent=` shape)

**Phase 3 — Live Tier-3 A/B**
- Baseline: `?concrete=false` on q14 (expect current 0/5 helpful L0/L1)
- Candidate: `?concrete=true` on q14 (target: ≥1 L0/L1 concrete result
  in top-5, ideally the `feedback_cuidv2_required.md` L0 file)
- Guard: `?concrete=true` on q10 "how do I enable the recursive-retrain
  actuator?" (was 5/5 helpful — must remain ≥4/5)
- Guard: `?concrete=true` on q08 (was 1/5 due to wrong-domain L5s;
  should still surface the correct DRIFT-VALIDATION reframe)
- Enable flag in `.env` only if q14 improves AND guards hold

**Phase 4 — Docs + commit**
- Post + CHANGELOG + CLAUDE.md pin (rules)

## 5. Testing Plan

- **Tier 1 (unit)**: `fetchConcreteCandidates_test.go` — mock Neo4j
  driver, assert query shape (role/layer filter + sim floor + limit);
  empty pool safe; disabled-flag no-op
- **Tier 2 (integration)**: `service.go` end-to-end with concrete-recall
  ENABLED — assert merged pool contains BOTH pre-existing candidates
  AND concrete candidates, dedup works
- **Tier 3 (live smoke on mdemg-dev)**:
  - q14 baseline layer distribution (all L3+) captured
  - q14 with flag on: ≥1 L0/L1 in top-5
  - q10 with flag on: ≥4/5 helpful maintained
  - q08 with flag on: no regression

## 6. Commit Strategy

Single commit under `RETRIEVAL-LAYER-BALANCE-001`.

## 7. Verification Checklist

- [ ] `fetchConcreteCandidates` implemented with role+layer+sim gate
- [ ] Wired in `service.go` BEFORE RRF fusion (correct architectural
      placement — like Lever C)
- [ ] Dedup by node_id when merging into the pool
- [ ] 5 env vars in config.go + tests
- [ ] `?concrete=true|false` URL override
- [ ] Unit + integration tests green
- [ ] Live A/B on q14 → concrete surfaces
- [ ] Live guards on q10 + q08 → no regression
- [ ] Flag flipped ON in `.env`
- [ ] CHANGELOG + CLAUDE.md pin + post

## 8. Rollback

- Set `RETRIEVAL_CONCRETE_RECALL_ENABLED=false` in `.env` (config
  hot-reload OR restart)
- Long-term: revert the commit

## 9. Risks

- **Risk**: concrete-recall pool crowds out currently-helpful abstract
  results for queries where abstract IS the right answer (q05, q07,
  q10, q12 got 5/5 helpful).
  - **Mitigation**: concrete candidates enter as REGULAR candidates
    and compete on all RRF columns; if their embedding sim is lower
    than the existing abstracts, they'll rank lower after fusion. The
    `SIM_FLOOR=0.30` gate ensures only "sensibly-similar" concretes
    reach the pool.
  - **Guard**: Phase 3 explicitly A/Bs against a 5/5-helpful query
    (q10) — if regression >1 slot, tighten SIM_FLOOR or reduce TOPK
    before ship.

- **Risk**: role_types list drift — a new node role_type gets added
  but isn't in the concrete list, gets no representation.
  - **Mitigation**: `_ROLE_TYPES=""` (empty) accepts any role_type
    under `LAYER_MAX`. Operators can widen without a code change.

- **Risk**: performance — extra vector search per query.
  - **Mitigation**: the concrete partition is small (~O(1000s) vs
    ~O(10000s) for the full pool); vector-index query is O(log n)
    per lookup; the merge is O(pool_size). Bounded and comparable
    to Lever C which shipped without measurable latency lift.

## 10. Documents Accessed

Filled in `post.md`.
