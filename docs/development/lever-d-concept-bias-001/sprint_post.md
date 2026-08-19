# LEVER-D-CONCEPT-BIAS-001 — Sprint Post

**Arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase C
**Shipped**: 2026-08-18
**Ship state**: code + tests + docs shipped default-OFF; passive A/B deferred to operator

## What shipped

1. **New producer** `fetchConceptCandidates(ctx, spaceID, embedding, topK, simFloor, minLayer)` in `internal/jiminy/service.go` — role+layer-filtered cosine over `role_type IN ['concept','emergent_concept'] AND layer >= minLayer`. Mirrors `fetchActionableCandidates` (Lever C) shape. Returns `[]GuidanceItem{Type: GuidanceConcept, Priority: "medium"}`.
2. **Merge in `Guide()`** immediately after Lever C's actionable merge — dedup by node_id. Debug field `debug.leverd_concept_merged`.
3. **4 new config knobs** (all default OFF/safe): `JIMINY_LEVER_D_ENABLED` (false), `_TOPK` (3), `_SIM_FLOOR` (0.55), `_MIN_LAYER` (2).
4. **Boot log line** `jiminy: lever d concept bias enabled=... topk=... sim_floor=... min_layer=...` (always emitted).
5. **3 pin tests** (fail-open contract): `TestFetchConceptCandidates_DriverNilIsSafe`, `_EmptyEmbeddingIsSafe`, `_TopKZeroIsSafe`.
6. **Docs**: sprint plan + this post + `docs/features/jiminy-lever-d-concept-bias.md`.

## Recon findings (verified live on mdemg-dev before writing code)

Applied `must-validate-all-claims-before-commit`.

| Claim | Verification | Verdict |
|-------|--------------|---------|
| L2+ substrate has meaningful volume | live cypher: 9,732 non-archived concept/emergent_concept nodes | ✅ confirmed |
| L2+ concepts are invisible to Lever C | code inspection: `fetchActionableCandidates` filters role_type IN ['constraint','correction'] | ✅ confirmed |
| `GuidanceConcept` type already exists | `internal/jiminy/types.go:25` | ✅ confirmed |
| Concept nodes have populated `.name` / `.summary` / `.embedding` | live cypher: name populated on all sampled concepts | ✅ confirmed |
| L2+ concepts are structurally distinct from L0-L1 | layer distribution: L2=229, L3=4666, L4=1847, L5=2990 | ✅ confirmed |

## Live Tier-3 evidence

Query: **"how does the memory graph hierarchy work with emergent concepts"** with context `"designing a substrate-native retrieval architecture"`.

**BASELINE (Lever D off)** — 2 items:
```
[0] correction   gfob1d9udsphaf conf=0.6677  query-mdemg-cms-file-paths
[1] constraint   rtyx9qcql5os1j conf=0.6535  OPERATOR CORRECTION ...
```

**CANDIDATE (Lever D on, default topK=3, sim_floor=0.55, min_layer=2)** — 4 items (+2 NEW):
```
[0] correction   gfob1d9udsphaf conf=0.6681  query-mdemg-cms-file-paths     (unchanged)
[1] constraint   rtyx9qcql5os1j conf=0.6531  OPERATOR CORRECTION ...        (unchanged)
[2] concept      ea0c61d9-...   conf=0.7070  EmergentConcept-L3-post-cms    ← NEW L3 concept
[3] concept      ecc4bf56-...   conf=0.7069  EmergentConcept-L4-memory      ← NEW L4 concept
```

**Mechanism verified end-to-end**: two substrate-native L3+ emergent concepts surfaced that Lever C could not have selected (their `role_type` is outside its filter). Dedup + priority tagging preserved; existing actionables unchanged.

## Decisions

| Decision | Rationale |
|----------|-----------|
| Ship Lever D over ancestor-linkage | Operator-selected. Adds substrate items visible in the output; ancestor-linkage would only decorate existing items. |
| Priority "medium" (below actionables' "high") | Concepts are complementary context, not competitors for the actionable quota. |
| sim_floor default 0.55 (vs Lever C's 0.60) | Emergent concept embeddings are averaged over members → lower cosine peak even for good matches; higher-recall default lets operators observe noise floor. |
| `min_layer` = 2 (L2+) | Aligns with arc thesis ("L2+ emergent concepts") — L1 concepts are usually member-level, not yet abstracted; excluding them favors the higher-order signal. |
| Default OFF in code AND `.env` | Behavior-changing per HEBB-ETA-001 rule. |
| Mirror Lever C shape exactly | Existing surfacing behavior known-good; new producer inherits all the safety contracts (fail-open, archive gate, RRF-SCALE-001-safe). |

## Follow-ups (disclosed, deferred)

1. **[Passive] LEVER-D-AB-001** — enable in `.env` for a 168h window; measure follow-rate delta on concept-typed items vs baseline (no concept surfacing). Data-decide flip. NOT urgent — JIMINY-CEILING-BREAK-2 T+168h re-check on 2026-08-19 owns the primary substrate-quality signal.
2. **[Small]** URL override `?leverd=true|false` for per-request A/B without `.env` flip.
3. **LEVER-D-ANCESTOR-LINKAGE-001** — the alternative Phase C shape not chosen for this sprint: attach parent concepts to shipped actionables as breadcrumbs rather than adding new items.
4. **B1 activation-enrichment for Lever D concepts** — currently `activationEnrichLeverC` is scoped to actionable seeds; extending to concept seeds could apply the substrate-topological rerank to L2+ surfacings too.
5. **B2 effectiveness signal for concepts** — concepts don't accumulate GUIDANCE_OUTCOME today (Lever B's outcome tracker is scoped to constraint role); would require a per-concept outcome sink.
6. **Concept dedup vs abstraction hierarchy** — a concept surfaced as a candidate may be an ancestor of a shipped constraint (via ABSTRACTS_TO). Surfacing both would double-report. Follow-up if operator observes redundancy in real traffic.

## Arch rules pinned

- **When adding a new substrate-native surfacing lever**, mirror the shipped lever's structure exactly: same Cypher shape (role-filtered + layer-filtered + archive-filtered + cosine ≥ floor), same fail-open contract (nil driver / empty embedding / topK ≤ 0 → nil), same RRF-SCALE-001-safe cosine gate (never on RRF Score), same JIMINY-ARCHIVED-CODE-FILTER-001 archive filter. Diverge only on the role filter + priority tag + guidance type.
- **New guidance surfacing levers ship default OFF in code AND `.env`** (HEBB-ETA-001 rule, reaffirmed by Phase B1/B2/C).
- **Distinct debug field per lever** (`leverc_actionable_merged`, `leverd_concept_merged`) — enables operator + tests to distinguish which lever contributed to a given surfacing without ambiguity.

## Documents Accessed

- `internal/jiminy/service.go` (`fetchActionableCandidates` 3372+ — reference; `Guide()` merge 1214+ — insertion site; `activationEnrichLeverC` for structural comparison)
- `internal/jiminy/types.go` (`GuidanceConcept` type 25)
- `internal/config/config.go` (`JiminyLeverC*` block — pattern for new fields; init pattern)
- `internal/api/server.go` (jiminy boot log 1206-1235)
- `internal/jiminy/lever_d_concept_test.go` (3 fail-open tests)
- Live cypher-shell queries on mdemg-dev (L0-L5 layer/role distribution — 9,732 non-archived L2+ concepts)
- Live `/v1/jiminy/guide` smoke (baseline vs candidate on real query)
- `docs/features/jiminy-lever-c-activation.md` (feature doc pattern)
- `CLAUDE.md` (JIMINY-CORPUS-001, LEVER-C-TIGHTEN-001/002, ACTIVATION-DRIVEN-DISCOVERY-001, EFFECTIVENESS-BLEND-001, JIMINY-SUBSTRATE-NATIVE-001 arc)
- `docs/development/lever-d-concept-bias-001/sprint_plan.md`
