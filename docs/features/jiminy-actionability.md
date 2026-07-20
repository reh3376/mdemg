# Jiminy Actionability — Surface Bias + Directive Synthesis (JIMINY-ACTIONABILITY-001)

## Why

Diagnostics (JIMINY-RELEVANCE-001) showed guidance is ignored not because it is *off-topic* but because it is *not actionable*: the abstraction class (`pattern`/`learning`/`concept`) is ~90% of surfaced/outcome guidance and is ignored 54–66%, while the actionable class (`constraint`/`correction`) is ~10% and ignored only 27–35%. This sprint is the near-term surfacing lever — bias what `Guide()` *surfaces*, and how it is *phrased*, toward the actionable.

## Choices

Two independent, default-off, config-driven levers:

- **Lever A — surface-composition reweighting.** A per-type sort weight (`JIMINY_SURFACE_ACTIONABLE_WEIGHT`), a min-actionable quota (`JIMINY_SURFACE_MIN_ACTIONABLE` / `_FRACTION`) reserving surfaced slots before the cut to `max_items`, and an abstraction cap (`JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION`) dropping the abstraction tail first. All default to no-ops, so at defaults the surfaced set is byte-identical to the prior plain truncation; actionable items are never dropped to satisfy the cap.
- **Lever B — directive synthesis.** When `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=true`, the synthesis system prompt is augmented to render abstraction-type evidence as **imperative, task-specific directives** ("Do X", "Before Y, do Z") instead of advisory prose, bounded by `JIMINY_DIRECTIVE_SYNTHESIS_MAX_PROMPT_TOKENS`. Reuses the existing `jiminy.synthesize` call — no new LLM call on the hot path.

A surfaced-composition gauge (`mdemg_jiminy_surfaced_actionable_fraction` / `_abstraction_fraction`) measures the surfaced side directly.

## How it works

- `internal/jiminy/service.go`: `isActionableType` partitions types; `guidanceTypeWeight` applies Lever A's sort multiplier; `applyActionableComposition` enforces the quota + abstraction cap on the already-sorted list before truncation (default-preserving fast path when no quota + no cap). `Guide()` emits the surfaced-composition gauge from the final `filtered` set.
- `internal/jiminy/synthesizer.go` + `guidance_prompt.go`: in directive mode the system prompt gets `directiveSynthesisInstruction`, and `boundDirectivePrompt` keeps the user prompt within the token budget.

## How to use

| Env var | Default | Meaning |
|---|---|---|
| `JIMINY_SURFACE_ACTIONABLE_WEIGHT` | 1.0 (no-op) | Sort-key multiplier for actionable types |
| `JIMINY_SURFACE_MIN_ACTIONABLE` | 0 | Absolute min actionable items reserved |
| `JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION` | 0.0 | Min actionable as a fraction of `max_items` |
| `JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION` | 1.0 (no cap) | Max abstraction items as a fraction of `max_items` |
| `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED` | false | Render abstractions as imperative directives |
| `JIMINY_DIRECTIVE_SYNTHESIS_MAX_PROMPT_TOKENS` | 3500 | Directive-mode prompt token bound |

## Live A/B result (Epic 4)

Levers off vs on, same 6 contexts against mdemg-dev (`docs/development/jiminy-actionability-001/epic4_live_ab.md`):

- **Lever B works** — directive synthesis produces imperative narratives ("it is imperative... You MUST clean up... You MUST NOT delete..."). This is the higher-impact lever: it makes guidance actionable in *phrasing* regardless of *type*.
- **Lever A is mechanically correct but modest** — surfaced actionable fraction 6.7% → 10.5%. It pulls actionables up *where they exist in the candidate pool* but cannot manufacture them; for most contexts retrieval surfaces no actionable candidates.
- **Finding:** the binding constraint is **upstream retrieval candidate composition**, not the surfacing cut.

## Lever C — constraint-inclusion (Epic 5; the lever that works)

Lever C addresses the Epic-4 finding directly: when `JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED=true`, `Guide()` runs a targeted query (`fetchActionableCandidates`) for the top-K `constraint`/`correction` nodes by **embedding cosine similarity** to the context (`vector.similarity.cosine` over the role-filtered set — *not* the RRF score; RRF-SCALE-001-safe) and merges them (dedup by node_id) into the candidate pool. The merged nodes are already correctly typed (the query filters role_type). The disclosed "adapter drops role_type" classification gap is **closed by JIMINY-ROLETYPE-ADAPTER-001** (2026-07-17); Lever C's role-filtered fetch and the general retrieval path now BOTH yield correctly-typed items.

**Config:** `JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED` (false), `JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK` (5), `JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR` (0.30 cosine — raise for tighter relevance).

**Live A/B (controlled, `ab_results.md`):** surfaced **actionable fraction 11.1% → 47.7%** (4.3×), abstraction **88.9% → 52.3%** — **clears the ≤60%-abstraction milestone** Lever A couldn't move. Surfaced constraints are query-relevant. ⚠️ A live-smoke fix was needed mid-sprint: the initial index-scan query (top-50 then role-filter) returned 0 because actionables are ~0.1% of nodes; the role-filtered cosine query fixed it.

## Shipping
All three levers ship **default-off**. **Operator recommendation: enable Lever C** (the actionable-composition mover) and Lever B (imperative phrasing). Lever A's quota/cap then shapes the now-actionable-rich pool. See `docs/development/jiminy-actionability-001/`.

## Follow-up — corpus cleanup + repetition control (JIMINY-CORPUS-001, 2026-07-03)
Enabling Lever C exposed that the `role_type='constraint'` partition it surfaces from was ~half junk and over-repeated. JIMINY-CORPUS-001 addressed the corpus itself:
- **Promotion gate** (`internal/hidden/constraint_gate.go`) stops junk observations (build/test status, bash errors, PR/sprint/phase-completion notes, doc dumps) from becoming constraint nodes — provenance obs_type deny-set + content patterns, config-driven, default-on.
- **Purge:** 140→61 live constraint nodes (tombstone-only, reversible), removing ~58% of the constraint surfacing noise.
- **Repetition control** (`internal/jiminy/surface_cooldown.go`): per-session cooldown on repeatedly-ignored nodes + an effectiveness-prior soft re-rank (both default-on, RRF-SCALE-001-safe).
- **Relevance gate** (`internal/jiminy/outcome_classifier.go`): a precise 4-band classifier so unrelated-domain surfacings are `not_applicable`, near-LOW real ignores are `ignored`.
- **Lever B enabled** (`JIMINY_DIRECTIVE_SYNTHESIS_ENABLED`) + exposed in compose + the UI config tab.

Follow-rate lift is forward-looking (baseline 0.165; re-measure ~1 week out). See `docs/development/jiminy-corpus-001/`.


## Follow-up — role_type adapter gap closed (JIMINY-ROLETYPE-ADAPTER-001, 2026-07-17)
The disclosed follow-up ("retrieval adapter drops role_type → retrieval-sourced items typed `learning`") is **shipped and live-verified**. Additive `RoleType`/`ObsType` fields flow from Neo4j `role_type`/`obs_type` through `retrieval.Candidate` → `models.RetrieveResult` → `jiminy.RetrievalResult`; `classifyRetrievalItem` prefers `role_type` before the Layer≥2 concept short-circuit and the `ObsType` switch. Live smoke on `mdemg-dev`: the L1 UxTS constraint node surfaced with `role_type='constraint'`, Jiminy `latest` returned **4/5 items typed `constraint`** (all 5 would have been `learning` pre-fix), and `constraint_outcomes` now carries `guidance_type='constraint'` rows with matched `constraint_code`. The BM25 sink also picked up `role_type` at source (BM25 runs its own Cypher, not a virtual view over `cands`), and the reasoning-module rerank preserves ontology labels through the proto boundary via the existing `originalByID` restore hook. See `docs/development/jiminy-roletype-adapter-001/`.


## Follow-up — L1 correction producer (JIMINY-CORRECTION-PRODUCER-001, 2026-07-20)
JIMINY-ROLETYPE-ADAPTER-001 wired retrieval + classifier to carry `role_type='correction'` end-to-end, but the correction slot in `constraint_outcomes` still read zero because **no L1 correction nodes existed anywhere**. `CreateConstraintNodes` had a producer since inception; the correction side had none — 32 L0 `obs_type='correction'` observations sat in `mdemg-dev` unpromoted.

JIMINY-CORRECTION-PRODUCER-001 mints the missing L1 layer:
- **`internal/hidden/correction_nodes.go::CreateCorrectionNodes`** mirrors the constraint producer, keyed by `obs_type='correction'` (semantic label, 1:1 with the obs — no type-grouping); idempotent via the `IMPLEMENTS_CORRECTION` guard.
- **`internal/hidden/correction_gate.go::CorrectionPromotionGate`** — content-length + config-driven regex deny-set (reuses the JIMINY-CORPUS-001 junk-class defaults). No obs_type deny-set (the predicate already gates on `obs_type='correction'`).
- Wired as `correctionStep` in the consolidation pipeline (phase 20) alongside `constraintStep`.
- **Live Tier-3:** 32 L1 correction nodes minted; `/v1/memory/retrieve` returns `role_type='correction'` on relevant queries; Jiminy `latest` surfaces `type='correction'`; `constraint_outcomes.guidance_type='correction'` gains its first ever row for `mdemg-dev` (`followed`).

New config: `CORRECTION_PROMOTION_ENABLED` (default true), `CORRECTION_PROMOTION_MIN_CONTENT_LEN` (default 20), `CORRECTION_PROMOTION_REJECT_PATTERNS` (JSON regex array; defaults reuse the constraint gate's junk-class set). Rollback tombstone-only via `is_archived=true`. See `docs/development/jiminy-correction-producer-001/`.
