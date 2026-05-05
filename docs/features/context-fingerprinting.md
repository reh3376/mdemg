---
created: 2026-05-04
updated: 2026-05-04
version: v0.7.3
author: reh3376
status: experimental
phase: 14.2
---

# Context Fingerprinting (Note 05 sparse fingerprints)

## Summary

**Feature**: Context Fingerprinting
**Summary**: Per-observation 256-bit sparse vectors that let the retrieval pipeline discriminate the *same* MemoryNode observed in *different* contexts. Adds a 5th column (Jaccard-similarity) to the Phase 13 RRF aggregator and an optional strict-mode pre-filter.

## Vision & Goals

MDEMG today has one MemoryNode per symbol with one embedding. When the same symbol appears in unrelated contexts (e.g. `ErrorHandler` in `auth/auth.go` vs `payments/payments.go`), retrieval can't tell them apart. The HTM solution from Hawkins & Ahmad 2016 is sparse distributed representations: keep one node per symbol (graph stays stable) but tag *each observation* with a sparse fingerprint of "what else was active when it was captured." Retrieval ranks observations by fingerprint similarity to the query's current context.

This sits squarely on the connection-layer of the cognitive substrate (memory-quality + retrieval-quality), aligning with the project purpose: help developers create software that connects in ways that increase the likelihood of better decisions.

## Current State

Phase 14.2 ships:
- A per-space adaptive **ContextCatalog** that allocates 256 bits proportionally to feature density (32 bits → top-N (role_type, layer) tuples; up to 32 bits → top-N tags; remainder → paths). Symbol bits are reserved for code-rich spaces — no production space surveyed in Phase 14.2 Epic 0 has populated `symbol`, so the floor is currently 0.
- **Two-phase fingerprint computation** (operator-approved 2026-05-05):
  - Phase A (observe-time): `Service.Observe` computes the fingerprint from observation-local features (path + role_type × layer + tags). Cold-start works (every observation gets *some* fingerprint immediately; an empty fingerprint when no catalog exists).
  - Phase B (post-hoc): `CycleOrchestrator.RunCycle` stage 6 walks `CO_ACTIVATED_WITH` edges and adds symbol bits, bumping the version field. Mature data gets HTM-faithful fingerprints after the first weekly refresh.
- **5th RRF column** (`ContextColumn`) ranking candidates by Jaccard similarity to a query fingerprint.
- **Strict-context mode** (`?strict_context=true`) drops candidates below `RETRIEVAL_CONTEXT_STRICT_THRESHOLD` (default 0.25 Jaccard) before scoring.
- **Backfill CLI** (`mdemg migrate context-fingerprint --space-id <id>`) for legacy observations that pre-date the feature.

### Architecture

```
                       ┌────────────────────────────────┐
                       │   ContextCatalog (per space)   │
                       │  256 bits: rtl + tags + paths  │
                       │  Neo4j: ContextCatalog node    │
                       │  Versioned, is_active=true     │
                       └───────────────┬────────────────┘
                                       │ adaptive Builder
                                       │ (Cycle stage 6, weekly)
       ┌───────────────────────────────┼───────────────────────────┐
       │                               │                           │
       ▼                               ▼                           ▼
  Service.Observe              MemoryNode props              ContextColumn
  (live observe-time)        - cf_active []uint16          (5th RRF column)
  - path, role_type,         - cf_version int               - Jaccard scoring
    layer, tags                                              - reads cands fp
  - sets cf_active                                           - no extra I/O
    + cf_version
                                       ▲
                                       │ post-hoc refinement
                                       │ via CO_ACTIVATED_WITH
                                       │
                             RefineWithCoactivations
                             (Cycle stage 6 helper)
```

### Workflow

**New observations** (always-on after Phase 14.2 ship):
1. `Service.Observe` builds the `Observation` struct.
2. If `CONTEXT_FINGERPRINT_ENABLED=true` (default) and a CatalogLoader is wired, the loader fetches the active catalog for the space.
3. `ComputeContextFingerprintLocal(obs, cat)` returns the sorted set of bit positions for path + (role_type, layer) + tags.
4. `createObservationNode` Cypher writes `context_fingerprint_active` + `_version` on the new MemoryNode.

**Macro-cycle refresh** (weekly default — `CONTEXT_FINGERPRINT_REFRESH_INTERVAL_HOURS=168`):
1. `CycleOrchestrator.RunCycle` finishes stage 5 (Validate).
2. Stage 6 calls `Loader.Freshness(spaceID)` — if cold-start OR age > interval, build a new catalog.
3. `Builder.BuildForSpace` queries Neo4j density (paths, tags, role_type×layer tuples), allocates 256 bits per the adaptive algorithm, writes a new ContextCatalog node, marks the previous one `is_active=false` atomically.
4. Time-bounded by `CONTEXT_FINGERPRINT_REFRESH_TIMEOUT_MS` (default 60s); partial work is fine — next cycle picks up.

**Retrieval (read path)**:
1. `vectorRecall` Cypher returns `context_fingerprint_active` + `_version` on each Candidate.
2. If the request carries `query_context_fingerprint` AND `strict_context=true`, the strict-mode pre-aggregation filter drops candidates with Jaccard < `RETRIEVAL_CONTEXT_STRICT_THRESHOLD`.
3. `ScoreAndRankRRF` adds the ContextColumn (5th column) when `RETRIEVAL_CONTEXT_COLUMN_ENABLED=true`. The column scores by Jaccard and contributes via the normal RRF fusion at weight `RETRIEVAL_CONTEXT_COLUMN_WEIGHT`.
4. Cache namespace is `v1-rrf5|...|c=W|...|ctx=B|strict=T` — toggling the column or its weight flips the namespace, so A/B sweeps don't share cached results.

### Configuration

See "Configuration Reference" below. Defaults are conservative — `RETRIEVAL_CONTEXT_COLUMN_ENABLED=false` until the Phase 14.2 Epic 6 A/B verdict lands. Fingerprint computation itself defaults on so production data starts accumulating fingerprints immediately.

## Notes

### Known Limitations

- **Symbols not yet allocated**: every production space surveyed in Phase 14.2 Epic 0 (whk-wms, mdemg-dev, linear, ubts-load) has 0 distinct `symbol` values, so the Builder allocates 0 symbol bits. Code-rich spaces will exercise this path once they exist.
- **Cross-version comparison ambiguous**: when a candidate's `_version` differs from the query's catalog `_version`, the bit positions don't necessarily mean the same thing. The Note 05 §4.3 fallback is "no-context match" (Jaccard 0); re-fingerprinting via Stage 6 / backfill normalizes versions.
- **Cold-start**: the very first observation in a brand-new space gets an empty fingerprint until the first Stage 6 tick (or operator-run `mdemg migrate context-fingerprint --build`).

### Risks & Gaps

- **Jiminy + RRF integration**: the Jiminy explainable-retrieval path uses the linear scorer's breakdown-enabled path because RRF doesn't produce a per-component breakdown. ContextColumn integration with Jiminy's explanation surface is queued for a follow-up sprint.
- **Per-space allocation drift**: top-N lists are stable in surveyed production spaces, but a sudden tag-vocabulary shift would invalidate bit positions. Determinism + version bumps make this safe, but operators should monitor catalog refresh frequency.
- **Backfill scale**: large spaces (e.g. mdemg-dev at ~80k observations) take significant wall-clock time to backfill. Run during maintenance windows.

### Future Improvements

- **Symbol bits**: when Note 07 / 08 lands and code-rich spaces accumulate symbol nodes, the Builder will start allocating non-zero symbol bits and the Phase B post-hoc refinement becomes meaningful.
- **Per-category column weight**: pair with the Phase 14.1 per-category gate dispatch (`SPARSE_GATE_CATEGORY_OVERRIDES`) so polysemy-prone categories (architecture_structure, data_flow_integration) can up-weight the ContextColumn.
- **Cross-space fingerprint comparison**: today fingerprints are space-local; Note 09 (Active Inference Unification) may revisit this.

## API Endpoints

| Method | Endpoint | Description | URL Params (Phase 14.2) |
|--------|----------|-------------|-------------------------|
| POST | `/v1/memory/retrieve` | Retrieve with context-aware ranking | `?strict_context=true` (drops low-Jaccard pre-aggregation) |

Request body (Phase 14.2 fields):
- `query_context_fingerprint []uint16` — sorted bit positions of the query's fingerprint. Empty → ContextColumn contributes 0 (graceful degradation).
- `strict_context bool` — when true AND fingerprint non-empty, applies strict-mode pre-filter.

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg migrate context-fingerprint --space-id <id>` | Backfill historical observations (default `--dry-run=true`) |
| `mdemg migrate context-fingerprint --space-id <id> --build` | Cold-start: build catalog before backfill |

Add `--dry-run=false` to actually write. Idempotent: skips observations already at the active catalog version.

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `CONTEXT_FINGERPRINT_ENABLED` | `true` | Master toggle for fingerprint computation at observe time |
| `CONTEXT_FINGERPRINT_BIT_BUDGET` | `256` | Total bit count per fingerprint (Note 05 default) |
| `CONTEXT_FINGERPRINT_REFRESH_ENABLED` | `true` | Master toggle for the post-hoc refresh stage in CycleOrchestrator |
| `CONTEXT_FINGERPRINT_REFRESH_INTERVAL_HOURS` | `168` | Catalog refresh cadence (weekly per Note 05) |
| `CONTEXT_FINGERPRINT_REFRESH_TIMEOUT_MS` | `60000` | Per-cycle time budget for stage-6 refresh batch |
| `CONTEXT_CATALOG_TOP_N_PATHS` | `192` | Top-N path bits in the catalog |
| `CONTEXT_CATALOG_TOP_N_TAGS` | `32` | Top-N tag bits in the catalog |
| `CONTEXT_CATALOG_FLOOR_BITS_PER_KIND` | `16` | Minimum bits allocated to any kind with ≥10 distinct values |
| `CONTEXT_CATALOG_ROLE_TYPE_LAYER_BITS` | `32` | Reserved bits for `role_type × layer` combinations |
| `RETRIEVAL_CONTEXT_COLUMN_ENABLED` | `false` (initial — flipped after Epic 6 verdict) | Master toggle for the 5th RRF column |
| `RETRIEVAL_CONTEXT_COLUMN_WEIGHT` | `0.10` | RRF weight on the context column |
| `RETRIEVAL_CONTEXT_STRICT_THRESHOLD` | `0.25` | Jaccard threshold for `?strict_context=true` mode |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| [Column-Voting Retrieval (Phase 13)](column-voting-retrieval.md) | extends — adds 5th column |
| [Sparse Activation Gate (Phase 14)](sparse-retrieval.md) | parallels — gate sees 5-column aggregator output |
| RSIC CycleOrchestrator | extended — adds stage 6 refresh hook |
| Conversation Service.Observe | extended — sets fingerprint at observe time |

## Related Files

- `internal/hidden/context_catalog.go` — Catalog struct, Loader/Builder interfaces
- `internal/hidden/context_catalog_builder.go` — production Neo4j Builder + Loader
- `internal/conversation/fingerprint.go` — observe-time + post-hoc fingerprint computation
- `internal/retrieval/column_context.go` — 5th RRF column + Jaccard helper
- `internal/cli/migrate_context_fingerprint.go` — backfill CLI
- `migrations/V0025__context_fingerprints.cypher` — Neo4j schema migration
- `migrations/V0026__context_catalog.cypher` — ContextCatalog node label
- `internal/tsdb/migrations/020_context_catalog_versions.sql` — TSDB historical snapshots
- `docs/development/post-ft-lora/sprint_plan_phase_14_2_note_05_sparse_fingerprints.md` — frozen sprint plan
- `docs/development/post-ft-lora/phase_14_2_forensic.md` — Epic 0 multi-space density audit
