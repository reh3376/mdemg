# Jiminy Lever D — concept-bias fetch (L2+ substrate surfacing)

**Sprint**: LEVER-D-CONCEPT-BIAS-001 (JIMINY-SUBSTRATE-NATIVE-001 Phase C)
**Shipped**: 2026-08-18
**Default**: OFF (opt-in via `.env`)

## Why

Live substrate on mdemg-dev has **~9,732 non-archived L2+ `concept` / `emergent_concept` nodes** (L2: 229, L3: 4,666, L4: 1,847, L5: 2,990). These are what the graph has been *building* over time via hidden-layer consolidation — the substrate-native abstractions MDEMG was designed to accumulate.

Before this sprint, Jiminy's Lever C (`fetchActionableCandidates`) was role-filtered to `constraint`/`correction` only. L2+ concepts partially surfaced via `RetrieveForJiminy` but were classified as low-priority `learning` items, downweighted in the guidance-type-weight step.

Phase C's arc thesis: **the substrate topology has more to say than Lever C surfaces**. Lever D exposes L2+ concepts as first-class guidance items so operators see MDEMG's emergent abstractions ("this rule is a case of the X emergent concept the graph has been building") when relevant.

## Choices

### Mirror Lever C's shape, don't invent

`fetchConceptCandidates` is a structural mirror of `fetchActionableCandidates`: same role-filtered cosine-over-partition Cypher, same fail-open contract, same RRF-SCALE-001-safe gate on the vector-index `sim` (stable [0,1], never on RRF Score), same JIMINY-ARCHIVED-CODE-FILTER-001 archive gate. The role filter is `role_type IN ['concept','emergent_concept']` + `layer >= min_layer` (default 2) instead of `role_type IN ['constraint','correction']`.

### Priority "medium", not "high"

Lever D items are tagged `Priority = "medium"` (below Lever C actionables' "high"). This ensures actionables still win Lever A's actionable quota; concepts join the pool as complementary context, not competitor for the actionable slots.

### Distinct guidance type `GuidanceConcept`

The type was already declared (`internal/jiminy/types.go:25`) for retrieval-classified L2+ items. This sprint gives it a first-class producer path. Downstream synthesis + classifier prompts already handle `concept` type as an abstraction category.

### Higher-recall sim_floor default (0.55 vs Lever C's 0.60)

L2+ emergent concepts are inherently more abstract than L1 constraints; their embeddings are averages of member embeddings, so the effective cosine peak is lower even for a good match. Starting at 0.55 provides higher recall to observe the noise floor; operators tune via `.env` if the noise is high.

### Alternative considered but not chosen: ancestor-linkage

Instead of adding new items, walk each Lever C actionable's parent concepts via `ABSTRACTS_TO`/`GENERALIZES` and attach them as breadcrumbs. Presented to operator as alternative; not chosen for this sprint. Reservable as `LEVER-D-ANCESTOR-LINKAGE-001` follow-up if desired.

## How it works

1. `fetchConceptCandidates(ctx, spaceID, embedding, topK, simFloor, minLayer)` runs one Cypher over the role+layer-filtered partition:
   ```cypher
   MATCH (c:MemoryNode {space_id: $spaceId})
   WHERE c.role_type IN ['concept', 'emergent_concept']
     AND c.layer >= $minLayer
     AND NOT coalesce(c.is_archived, false)
     AND c.embedding IS NOT NULL
   WITH c, vector.similarity.cosine(c.embedding, $embedding) AS sim
   WHERE sim >= $simFloor
   RETURN c.node_id, c.name, c.summary, c.layer, sim
   ORDER BY sim DESC LIMIT $topK
   ```
2. Returns `[]GuidanceItem` with `Type=GuidanceConcept`, `Priority="medium"`, `Confidence=sim` (clamped to `maxConfidence`).
3. In `Guide()`, after Lever C's actionable merge, Lever D concepts merge into the same pool with **dedup by node_id**. Debug field `debug.leverd_concept_merged = N`.
4. Downstream (scope-gate, dedup, filter-by-confidence, sort) is unchanged.

### Fail-open contract

- nil driver / empty embedding / topK ≤ 0 → `nil` (Guide() continues without concept merge).
- Cypher execution error → `nil` + debug log; caller proceeds without concept enrichment.

## How to use

### Enabling

```bash
# in /Users/reh3376/mdemg/.env
JIMINY_LEVER_D_ENABLED=true

# optional tuning
JIMINY_LEVER_D_TOPK=3           # concepts merged per request (default 3)
JIMINY_LEVER_D_SIM_FLOOR=0.55   # cosine floor [0,1] (default 0.55 — start higher-recall to observe)
JIMINY_LEVER_D_MIN_LAYER=2      # minimum node.layer (default 2)
```

Restart: `launchctl kickstart -k gui/501/com.mdemg.server`. Boot log confirms:

```
INFO msg="jiminy: lever d concept bias" enabled=true topk=3 sim_floor=0.55 min_layer=2
```

### Observability

Per-call debug field:

```json
{ "debug": { "leverd_concept_merged": 2 } }
```

= number of concept items merged into the guidance pool (post-dedup).

### Rollback

`JIMINY_LEVER_D_ENABLED=false`, restart. Byte-identical to pre-Lever-D behavior.

## Live-smoke evidence (2026-08-18 mdemg-dev)

Query: "how does the memory graph hierarchy work with emergent concepts" with context "designing a substrate-native retrieval architecture".

**BASELINE (Lever D off)** — 2 items:
- correction `gfob1d9udsphaf` (query-mdemg-cms-file-paths)
- constraint `rtyx9qcql5os1j` (OPERATOR CORRECTION)

**CANDIDATE (Lever D on, default knobs)** — 4 items (+2 NEW):
- correction + constraint (unchanged)
- **concept `ea0c61d9-…` (EmergentConcept-L3-post-cms-1111)** — NEW, sim 0.7070
- **concept `ecc4bf56-…` (EmergentConcept-L4-memory-space-393)** — NEW, sim 0.7069

Mechanism verified: two substrate-native L3+ emergent concepts surfaced that Lever C could not have selected (their `role_type` is outside its filter). Dedup + priority tagging preserved; existing surfacing unchanged.

## Follow-ups (deferred)

- **Passive A/B** — enable `JIMINY_LEVER_D_ENABLED=true` for a 168h window; measure whether concept surfacing correlates with operator follow rate (or is noise). Data-decide flip.
- **LEVER-D-ANCESTOR-LINKAGE-001** — the alternative shape not chosen for this sprint: attach each Lever C actionable's parent concepts as breadcrumbs rather than adding new items.
- **B1 activation-enrichment for Lever D** — extend `activationEnrichLeverC` to also spread from concept seeds, or add a parallel `activationEnrichLeverD`.
- **B2 effectiveness signal for concepts** — concepts don't accumulate GUIDANCE_OUTCOME today (Lever B's outcome tracker is scoped to constraint role); would need a per-concept outcome sink.
- **Higher `MIN_LAYER` tuning** — start with L2+ (default); consider L3+ or L4+ if noise from L2 concept nodes is high.

## References

- Sprint plan: [`docs/development/lever-d-concept-bias-001/sprint_plan.md`](../development/lever-d-concept-bias-001/sprint_plan.md)
- Sprint post: [`docs/development/lever-d-concept-bias-001/sprint_post.md`](../development/lever-d-concept-bias-001/sprint_post.md)
- Producer: `internal/jiminy/service.go::fetchConceptCandidates`
- Merge site: `internal/jiminy/service.go::Guide()` after Lever C merge
- Reference pattern: `internal/jiminy/service.go::fetchActionableCandidates` (Lever C)
- Related: LEVER-C-TIGHTEN-001/002, ACTIVATION-DRIVEN-DISCOVERY-001 (Phase B1), EFFECTIVENESS-BLEND-001 (Phase B2)
