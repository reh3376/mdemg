# Sprint POST-FT-LORA-PHASE14.2 — Note 05 Sparse Fingerprints (Adaptive Catalog)

> **STUB — scoped 2026-05-04** as the second follow-up to Phase 14 narrow close. Phase 14 Epic 0 forensic found that the spec's static 64-bit/64-bit/64-bit/64-bit catalog allocation across (symbols, paths, roles, reserved) wastes 128 bits on `whk-wms` (which has 0 distinct symbols + 0 distinct roles). This sprint redesigns the catalog Builder to allocate bits adaptively per-space based on observed feature density, then ships the full Note 05 surface (V0028+V0029 schema, fingerprint computation, context column, backfill CLI).

## Context

Phase 14 Epic 0 audit (`phase_14_score_distribution_analysis.md` §4) found `whk-wms` MemoryNode property surface:
- 0 distinct `symbol` properties (the field doesn't exist on this space)
- 0 distinct `role` properties (not populated)
- 8360 distinct `path` properties (rich)
- 5 `role_type` values + 5 `layer` values (categorical, low-cardinality but always informative)
- `tags` array (not yet enumerated)

Spec's static 64/64/64/64 split would silently produce poor fingerprints on this space. The adaptive design measures density at refresh and allocates bits proportionally.

## Hypothesis

Per-space adaptive catalog with floors:
- Reserve 32 bits for `role_type × layer` combinations (always informative, low cardinality)
- Distribute remaining 224 bits across (symbols, paths, top-N tags) proportional to `log(distinct_count)` with floor 16 bits per kind that has ≥10 distinct values
- For `whk-wms`: ~192 bits paths + 32 bits role_type/layer + 32 bits tags
- For `mdemg-dev`-style code spaces (when `symbol` populated): 64+ symbols + paths + role_type/layer + tags

This produces meaningful fingerprints across heterogeneous spaces without operator tuning.

## Scope

| # | Deliverable | Path |
|---|---|---|
| 1 | Catalog package | `internal/hidden/context_catalog.go` — `Catalog` struct, adaptive `Builder` with density measurement, lookups (`SymbolBit`, `PathBit`, `RoleBit`, `RoleTypeBit`, `LayerBit`, `TagBit`), `LoadActive` / `LoadVersion` |
| 2 | Catalog density audit | `internal/hidden/catalog_audit.go` — Cypher walk over MemoryNodes per kind, top-N tag enumeration, distinct counting |
| 3 | Neo4j V0028 | Adds `MemoryNode.context_fingerprint_active` (uint16 list), `_version` (int) properties + index on `_version` for version-mismatch fallback |
| 4 | Neo4j V0029 | New `ContextCatalog` node label; per (space_id, version); contains `bits[]` (`{position, kind, ref, token}`) |
| 5 | TSDB V0020 | `context_catalog_versions` hypertable for historical snapshots; allows Phase 14.2.1 retune |
| 6 | Fingerprint computation | `internal/conversation/fingerprint.go` (~150 LOC) wired into `service.observe`. Empty fingerprint when catalog cold (graceful degradation) |
| 7 | Catalog refresh job | Weekly hook in `internal/ape/cycle.go` macro cycle; runs density audit + builds new catalog version + retains previous versions |
| 8 | Context column | `internal/retrieval/column_context.go` — `Column` interface impl. Score = Jaccard between observation fingerprint and query fingerprint. **Decision fork at execution**: 5th RRF column vs scoring term. Default: 5th column (RRF is production path post-13.1) |
| 9 | Strict-mode + threshold | `?strict_context=true` + `RETRIEVAL_CTX_STRICT_THRESHOLD` (default 0.25 Jaccard) |
| 10 | Query-side fingerprint | Optional `context_fingerprint` request body field; otherwise derived from session co-activation history |
| 11 | Historical backfill CLI | `mdemg migrate context-fingerprint --space <id>` (batched 1000/batch, resumable) |
| 12 | Combined A/B with Phase 14.1 gate | UVTS quick + full × 3 presets (fingerprint_only, gate+fingerprint, baseline) |
| 13 | Conditional default flips per Epic 7 matrix | |
| 14 | Docs | sprint plan + post + new `docs/features/context-fingerprinting.md` + standard 4-doc updates |
| 15 | Tier 1+2+3 testing | Live polysemy demo on a non-production space (`mdemg-dev`): seed 3 ErrorHandler-style observations in distinct contexts, retrieve with each context, verify ranking shifts |

## Estimate

~7 dev-days. Larger than Phase 14.1 because of the schema + catalog package + backfill + combined A/B.

## Budget

~$25 OpenAI for combined A/B sweep (3 presets × quick + 1 full).

## Pre-gate

- Phase 14.1 has shipped (gate either flag-off or default-on per its A/B). If gate ships default-on, Phase 14.2 must run combined-A/B against the gate-on production state.
- Catalog density audit produces a written record of `whk-wms` (and mdemg-dev) feature distribution before Builder is implemented.

## Acceptance

Per Epic 7 matrix in original Phase 14 plan:
- If `fingerprint_only` passes A/B → flip `RetrievalCtxColumnEnabled=true`, ship default-on
- If `gate + fingerprint` combined passes → flip both defaults
- If neither passes → ship flag-off, scope Phase 14.2.1

Live polysemy demo must show observable ranking shift on the seeded fixture for the sprint to be considered functionally complete (even if A/B verdict is fail — proves the mechanism works regardless of A/B noise).

## Documents Accessed (during planning)

- `docs/development/post-ft-lora/sprint_plan_phase_14_sparse_fingerprints_and_gate.md` (frozen plan, narrowed)
- `docs/development/post-ft-lora/phase_14_score_distribution_analysis.md` (Epic 0 forensic — bit-allocation rationale)
- `docs/development/post-ft-lora/phase_14_post.md`
- `docs/research/mdemg_sprint_ideas/05-context-specific-node-activations.md`
- `internal/hidden/types.go` — observation struct (V0028 extension target)
- `internal/conversation/service.go` — observe pipeline (fingerprint hook)
- `internal/ape/cycle.go` — macro-cycle scheduler (refresh hook)
- Phase 14 conditional V0028+V0029+V0020 sketches
