# LEVER-D-CONCEPT-BIAS-001 — Sprint Plan

**Master arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase C

## 1. Header & Metadata

- **Sprint ID**: `LEVER-D-CONCEPT-BIAS-001`
- **Arc**: JIMINY-SUBSTRATE-NATIVE-001 (Phase C)
- **Author**: reh3376 / claude
- **Date**: 2026-08-18
- **Branch**: `reh3376_dev01`
- **Estimated wall-clock**: ~2-3 hours
- **Sprint format**: v1.0 (12-section)

## 2. Problem Statement

Live substrate on mdemg-dev has **~9,732 L2+ `concept` / `emergent_concept` nodes** (L2: 229, L3: 4,666, L4: 1,847, L5: 2,990) that are largely invisible to Jiminy's Lever C (which is role-filtered to `constraint`/`correction`). L2+ concepts partially surface via retrieval (`RetrieveForJiminy`) but are classified as low-priority `learning` items, downweighted in the guidance-type-weight step.

Phase C's design intent (JIMINY-SUBSTRATE-NATIVE-001 arc README) is to "expose L2+ emergent concepts as guidance items directly" — bypassing the low-priority classification path so operators see substrate-native abstractions ("this rule is a case of the X emergent concept the graph has been building") when relevant to the current query.

This sprint ships **Lever D — Concept-Bias**, a mirror of Lever C's `fetchActionableCandidates` but scoped to concepts at layer≥2.

## 3. Scope & Constraints

### In scope
1. **New Cypher fetch** `fetchConceptCandidates(ctx, spaceID, embedding, topK, simFloor, minLayer)` — role-filtered cosine over `role_type IN ['concept','emergent_concept'] AND layer >= $minLayer`. Returns `[]GuidanceItem` typed as `GuidanceConcept`.
2. **Merge into pool in `Guide()`** at the same site as Lever C's actionable merge — after retrieval, before scope-gate. Dedup by node_id vs already-present items. Debug field `debug.leverd_concept_merged`.
3. **4 new config knobs**: `JIMINY_LEVER_D_ENABLED` (false), `_TOPK` (3), `_SIM_FLOOR` (0.55), `_MIN_LAYER` (2). All default OFF in code AND `.env`.
4. **Boot log**: `jiminy: lever d concept bias enabled=... topk=... sim_floor=... min_layer=...`.
5. **Pin tests**: default-off byte-identical (function early-returns; no merge); with flag on, merges concepts into pool; dedup works.

### Out of scope
- **Ancestor-linkage (walk parent concepts on shipped constraints)** — offered as alternative in operator recon; not chosen. Reservable for Phase C follow-up.
- **Concept-effectiveness weighting** — B2 shipped effectiveness for constraint role; extending to concept role is separate additive follow-up.
- **B1 activation reranking on concept items** — Lever C's `activationEnrichLeverC` operates on Lever C actionables only; extending to Lever D concepts is a separate sprint if needed.

### Hard invariants
- **Default OFF in code AND `.env`** (behavior-changing per HEBB-ETA-001 rule).
- **RRF-SCALE-001-safe**: gates on cosine sim (stable [0,1]), NEVER on RRF Score.
- **JIMINY-ARCHIVED-CODE-FILTER-001 contract**: `NOT coalesce(c.is_archived, false)` in the Cypher WHERE.
- **CACHE-KEY-002 forcing function**: the flag lives on Jiminy path, not `RetrieveRequest` — no CacheKey change required (mirrors Lever C's non-impact on the retrieval cache namespace).
- **Fail-open**: nil driver / empty embedding / query error → return nil (no merge); Guide() continues without concept enrichment.

## 4. Dependencies

**Upstream (must be shipped)**:
- ✅ `GuidanceConcept` type exists (`internal/jiminy/types.go:25`)
- ✅ `fetchActionableCandidates` (Lever C, reference pattern)
- ✅ Substrate has L2+ concept nodes (verified: 9,732 non-archived on mdemg-dev)
- ✅ Vector-index cosine on MemoryNode.embedding (shipped)

**Downstream (this sprint unblocks)**:
- Concept-Lever activation reranking (would combine with B1)
- Concept-effectiveness weighting (would combine with B2)
- Ancestor-linkage (independent alternative)

## 5. Implementation Plan

### Epic 1: `fetchConceptCandidates` (~45min)
- New function in `internal/jiminy/service.go`, mirrors `fetchActionableCandidates` structure.
- Cypher:
  ```
  MATCH (c:MemoryNode {space_id: $spaceId})
  WHERE c.role_type IN ['concept', 'emergent_concept']
    AND c.layer >= $minLayer
    AND NOT coalesce(c.is_archived, false)
    AND c.embedding IS NOT NULL
  WITH c, vector.similarity.cosine(c.embedding, $embedding) AS sim
  WHERE sim >= $simFloor
  RETURN c.node_id, coalesce(c.name,''), coalesce(c.summary,''), c.layer, sim
  ORDER BY sim DESC LIMIT $topK
  ```
- Return `[]GuidanceItem` with `Type = GuidanceConcept`, `Priority = "medium"` (below "high" actionables), `Confidence = sim` (clamped).
- Fail-open: driver nil / empty embedding / query error → nil.

### Epic 2: config + boot log (~15min)
- 4 new config fields in `internal/config/config.go` alongside `JiminyLeverC*` block.
- Boot log line in `internal/api/server.go` `handleJiminyStartup`, adjacent to Lever C block.

### Epic 3: `Guide()` merge (~20min)
- After Lever C merge (existing block at `service.go:1214`), add Lever D merge block.
- Dedup: skip concepts whose node_id is already in the pool.
- Debug field `debug.leverd_concept_merged = added`.

### Epic 4: tests (~30min)
- New file `internal/jiminy/lever_d_concept_test.go`:
  - `TestFetchConceptCandidates_DriverNilIsSafe` — returns nil, no panic.
  - `TestFetchConceptCandidates_EmptyEmbeddingIsSafe` — returns nil.
  - `TestFetchConceptCandidates_TopKZeroIsSafe` — returns nil.
- Contract-level tests for the new type + merge (Cypher execution requires live Neo4j; keep those under integration).

### Epic 5: live Tier-3 verification (~15min)
- Build, kickstart, boot log confirms `enabled=false` default.
- Flip `.env` `JIMINY_LEVER_D_ENABLED=true` + kickstart.
- `/v1/jiminy/guide` on a real query, verify:
  - `debug.leverd_concept_merged > 0`
  - Response contains items with `type=concept`
  - Restore `.env` post-smoke.

### Epic 6: docs (~15min)
- New feature doc `docs/features/jiminy-lever-d-concept-bias.md`.
- Sprint post `docs/development/lever-d-concept-bias-001/sprint_post.md`.
- CLAUDE.md architecture note.
- CHANGELOG entry.

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit
- 3 fail-safe tests in `lever_d_concept_test.go` (nil driver / empty embedding / topK≤0).
- Full test suite green (`go test ./internal/jiminy/... ./internal/config/... ./internal/api/...`).

### Tier 2 — Integration
- Compile clean; no interface changes required.
- Existing UATS suite green (no schema/handler changes).

### Tier 3 — Live end-to-end (mdemg-dev)
- Boot log confirms default OFF.
- `.env` flip + boot log confirms ON.
- Live `/v1/jiminy/guide` on real query; concept items appear; `debug.leverd_concept_merged > 0`.
- `.env` restored.

## 7. Commit Strategy

- 1 primary commit for the sprint.
- Any fix-commit for live-smoke-discovered surprise.

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./internal/jiminy/ ./internal/config/ ./internal/api/` clean
- [ ] `go test ./internal/jiminy/... ./internal/config/... ./internal/api/...` green
- [ ] Boot log confirms `jiminy: lever d concept bias enabled=false ...`
- [ ] Live: candidate produces `type=concept` items with `leverd_concept_merged > 0`
- [ ] `.env` restored post-smoke
- [ ] Sprint plan in `docs/development/lever-d-concept-bias-001/`
- [ ] Feature doc present
- [ ] CLAUDE.md note + CHANGELOG entry
- [ ] PR sprint-summary comment

## 9. Documentation Update

### Files created
- `docs/development/lever-d-concept-bias-001/{sprint_plan,sprint_post}.md`
- `docs/features/jiminy-lever-d-concept-bias.md`
- `internal/jiminy/lever_d_concept_test.go`

### Files modified
- `internal/jiminy/service.go` — new `fetchConceptCandidates` + `Guide()` merge
- `internal/config/config.go` — 4 new fields
- `internal/api/server.go` — boot log
- `CLAUDE.md`, `CHANGELOG.md`

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| L2+ concepts flood the guidance pool, drowning actionables | Medium | Medium | Default OFF; TopK=3 default; distinct `medium` priority tag (below actionables); Lever A's actionable quota still applies |
| Query with sim_floor=0.55 surfaces stale/noisy concepts | Medium | Low | Same sim_floor pattern as Lever C (LEVER-C-TIGHTEN-001 tuned to 0.60 there — we start higher-recall at 0.55 to observe the noise floor; operator tunes via `.env`) |
| Concept content is embedding-normalized-summary rather than actionable rule text — may confuse operators | Low | Low | Distinct `type=concept` tag flags it as abstraction, not actionable; content preview shows the summary directly |
| Dedup collision if a constraint IS classified as concept type by retrieval — but constraint role_type != concept, so this can't happen | N/A | N/A | Role filter guarantees disjoint sets |

## 11. Rollback Procedures

- Zero substrate mutation; pure read.
- `JIMINY_LEVER_D_ENABLED=false` → byte-identical to pre-sprint.
- Code rollback: revert commit; no schema changes.

## 12. Documents Accessed

- `internal/jiminy/service.go` (`fetchActionableCandidates` 3372+ — reference pattern; `Guide()` merge 1214+; `activationEnrichLeverC` 3450+)
- `internal/jiminy/types.go` (GuidanceConcept type 25)
- `internal/config/config.go` (JiminyLeverC* block 361-368 — pattern; JiminyLeverCActivation/Effectiveness init)
- `internal/api/server.go` (boot log for lever c 1194-1215 — pattern)
- Live cypher-shell queries on mdemg-dev (L0-L5 role/layer distribution; ~9,732 L2+ concept nodes verified)
- `CLAUDE.md` (JIMINY-CORPUS-001, LEVER-C-TIGHTEN-001, ACTIVATION-DRIVEN-DISCOVERY-001, EFFECTIVENESS-BLEND-001, JIMINY-SUBSTRATE-NATIVE-001 arc README)
- `docs/features/jiminy-lever-c-activation.md`

---
