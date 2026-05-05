# Sprint POST-FT-LORA-PHASE14.2 — Note 05 Sparse Fingerprints (Two-Phase + Adaptive Catalog)

## Context

The Phase 14 sequence (14 → 14.1 → 14.1.1) closed with the Note 06 sparse activation gate **default-on**: `MIN_ACTIVE=15` global + `data_flow_integration` MIN=20 override, 120q full A/B mean +0.003, 0 regressions, 10 improvements. The retrieval pipeline now has its first gate, but it operates only on existing per-call score distributions — there's no signal to discriminate the *same* node observed in *different* contexts. Phase 14.2 ships Note 05 (Context-Specific Node Activations) — sparse fingerprints on observations + a 5th `ContextColumn` for the RRF aggregator + a per-space adaptive Catalog that allocates bit budget by feature density.

**The polysemy problem this solves.** MDEMG today has **one MemoryNode per symbol with one embedding**. When the same symbol appears in unrelated contexts (e.g. `ErrorHandler` in `auth/auth.go` vs `payments/payments.go`), retrieval can't discriminate. Existing workarounds (space scoping, role filtering) require the caller to know to specify them. The HTM solution: keep one node per symbol (graph stays stable) but tag each observation with a sparse fingerprint of "what else was active when it was captured." Retrieval ranks observations by fingerprint similarity to the query's current context.

**Two findings from Phase 1 exploration that shape this plan.**

1. **Co-active nodes are NOT available at observe time.** `CoactivateSession` runs *after* `createObservationNode()`, querying Neo4j retroactively for session siblings. Note 05's original spec (`docs/research/mdemg_sprint_ideas/05-context-specific-node-activations.md` lines 105–132) assumed co-activity at observe time — that's wrong. Operator-approved resolution: **two-phase fingerprint** — observe-time fingerprint uses local features (path, role_type, layer, top-N tags); a weekly post-hoc refresh re-computes fingerprints with actual co-activation edges via the existing `internal/ape/cycle.go::CycleOrchestrator.RunCycle` macro-cycle hook.

2. **Catalog bit allocation must be adaptive per space.** Phase 14 Epic 0 forensic queried `whk-wms` MemoryNodes: 0 distinct `symbol` values, 0 distinct `role` values, 8360 paths, 5 `role_type` values, 5 `layer` values. Note 05's static 64/64/64/64 spec wastes 128 bits on this space. Operator-approved resolution: per-space Catalog Builder measures density at refresh time and allocates bits proportionally to `log(distinct_count)` with a 32-bit `role_type × layer` floor.

**Why the timing is right.** Phase 14.1.1 stabilized the gate (default-on) with the per-category override mechanism + V0019 telemetry + comparator eps fix. The infrastructure that Phase 14.2 needs — `Column` interface, RRF aggregator, `Service.scorerVersion()` cache namespace, gate operating on aggregator output — is mature and warm. Note 05 was always queued for "after Note 06 stabilizes."

**Sprint chain.** Phase 14 → 14.1 → 14.1.1 (gate default-on) → **Phase 14.2 (this)** → optional 14.3 / Note 07-09 capstones.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | POST-FT-LORA-PHASE14.2 |
| Title | Note 05 — Sparse Fingerprints (Two-Phase + Adaptive Catalog) |
| Date | 2026-05-05 (plan) |
| Branch | `reh3376_dev01` |
| Predecessor | Phase 14.1.1 (commit `028bfdc`, sparse gate default-on) |
| Successor | Phase 14.3 / Note 07 (TBD) |
| Type | Code-large + research (~1500 LOC production + ~600 LOC tests; 2 Neo4j migrations + 1 TSDB migration + new package + 5th RRF column + observe-time hook + macro-cycle hook + new CLI + 12 new env knobs) |
| Risk | MEDIUM (additive: new column + new property; gate path unchanged; flag-off-by-default rollout matches Phase 14.1.1 → default-on conditional pattern) |
| Budget | $25–30 OpenAI (Epic 6 A/B sweep: 3 quick presets + 1-2 full 120q runs) |
| Effort estimate | ~13 dev-days (~3 weeks): Note 05 design (10.5 days) + Phase 14 Epic 0 adaptive Builder redesign (+2 days) + Phase 1 exploration two-phase shape (+0.5 days) |
| New TSDB migrations | V0020 `context_catalog_versions` (historical catalog snapshots for Phase 14.2.1 retune) |
| New Neo4j migrations | V0028 (`MemoryNode.context_fingerprint_active`/`_version` properties + index on `_version`) + V0029 (`ContextCatalog` node label + indexes) |
| Post-sprint artifacts | `internal/hidden/context_catalog.go` (new ~250 LOC), `internal/conversation/fingerprint.go` (new ~150 LOC), `internal/retrieval/column_context.go` (new ~120 LOC), `internal/cli/migrate_context_fingerprint.go` (new ~150 LOC), 12 new env-var knobs in `config.go`, post-doc + plan-frozen + new feature doc + standard 4-doc updates |

## 2. Problem Statement

### Problem A — polysemy without graph duplication

MDEMG today has one `MemoryNode` per symbol with one embedding. When the same symbol appears in unrelated contexts, retrieval can't tell them apart. The HTM solution: keep one node per symbol (graph stays stable) but each *observation* of that symbol carries a sparse fingerprint of what else was active when it was captured. Retrieval ranks observations by fingerprint similarity to the query's current context.

### Problem B — `whk-wms` lacks the spec's assumed feature surface

Note 05's static 64-symbols / 64-paths / 64-roles / 64-reserved bit allocation assumes a code-rich space. `whk-wms` (a conversation-history space) has 0 distinct symbols + 0 distinct roles. Static allocation wastes 128 bits on this space. Phase 14.2 ships an adaptive Catalog Builder that measures per-space feature density at refresh time and allocates bits proportionally with floors.

### Problem C — co-activity isn't available at observe time

Note 05's `computeContextFingerprint(ctx, obs, coActiveNodes)` signature assumed `coActiveNodes` is known at observe time. Phase 1 exploration found `CoactivateSession` runs *after* node creation, querying Neo4j retroactively. The two-phase resolution: observe-time fingerprint uses local features only (path, role_type, layer, top-N tags); a weekly post-hoc refresh re-computes fingerprints with actual co-activation edges via the existing macro-cycle hook. Cold-start works (every observation has *some* fingerprint immediately); mature data gets HTM-faithful fingerprints after the first weekly refresh tick.

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | Neo4j V0028 migration | `migrations/V0028__context_fingerprints.cypher` (root `migrations/` per project convention) — adds `context_fingerprint_active []uint16` + `context_fingerprint_version int` properties on `MemoryNode`, index on `_version` |
| 2 | Neo4j V0029 migration | `migrations/V0029__context_catalog.cypher` — `ContextCatalog` node label, indexes on `(space_id, version)` and `(space_id, is_active)` |
| 3 | TSDB V0020 migration | `internal/tsdb/migrations/020_context_catalog_versions.sql` — historical catalog snapshots hypertable for Phase 14.2.1 retune (similar shape to V0019) |
| 4 | Catalog package | `internal/hidden/context_catalog.go` (new ~250 LOC) — `Catalog` struct, adaptive `Builder` with per-space density measurement + bit allocation, `LoadActive(ctx, spaceID)` + `LoadVersion(ctx, spaceID, version)` lookups, `SymbolBit/PathBit/RoleBit/RoleTypeBit/LayerBit/TagBit` accessors |
| 5 | Two-phase fingerprint computation | `internal/conversation/fingerprint.go` (new ~150 LOC). Observe-time uses observation-local features only. Wired into `Service.Observe` (`internal/conversation/service.go:219`) — sets `ContextFingerprintActive` + `ContextFingerprintVersion` on the `Observation` struct before `createObservationNode()` Cypher (line ~537) |
| 6 | Post-hoc refresh in CycleOrchestrator | New stage 6 in `internal/ape/cycle.go::CycleOrchestrator.RunCycle` (after Validate). Recomputes fingerprints for observations whose `version < latest_catalog_version`, using actual co-activation edges from Neo4j. Gated on `cfg.ContextFingerprintRefreshEnabled` (default true) |
| 7 | ContextColumn (5th RRF column) | `internal/retrieval/column_context.go` (new ~120 LOC) implements `Column` interface from `internal/retrieval/column.go:20-31`. Score = Jaccard similarity between observation fingerprint and query fingerprint. Wired into `internal/retrieval/scoring_rrf.go:45-49` array as the 5th column |
| 8 | Cache-namespace bump | `internal/retrieval/service.go::scorerVersion()` extended: `v1-rrf4` → `v1-rrf5`; format string adds `\|ctx=%t\|ctx_w=%.3f`. Per Phase 13.1 + 14.1 precedent — config-flip auto-flips namespace |
| 9 | Query-side fingerprint | `internal/models/models.go::RetrieveRequest` adds optional `QueryContextFingerprint []uint16` (request-body field) + `?context=auto\|...` URL param. When unset, helper derives from session's recent co-activations; when set, used verbatim |
| 10 | Strict mode | `?strict_context=true` URL param on retrieve + `RETRIEVAL_CTX_STRICT_THRESHOLD` env (default 0.25 Jaccard) — observations below threshold dropped pre-aggregation |
| 11 | Backfill CLI | `internal/cli/migrate_context_fingerprint.go` (new ~150 LOC) — `mdemg migrate context-fingerprint --space <id> [--dry-run] [--batch-size 500]`. Streaming Neo4j cursor pattern from `internal/cli/graph_repair.go`. Resumable (skips already-fingerprinted observations of current catalog version) |
| 12 | 12 new config knobs | `internal/config/config.go` — see "Configuration Reference" in §8 |
| 13 | 5+ Prometheus metrics | `internal/metrics/collectors.go` — `mdemg_context_fingerprint_size`, `_catalog_version`, `_similarity_score`, `_observations_without_fingerprint`, `_refresh_duration_seconds` |
| 14 | Per-Note A/B + combined A/B | UVTS quick (16q) × 3 presets (fingerprint_only, gate+fingerprint, baseline=current Phase 14.1.1 production) → full 120q on winner |
| 15 | Conditional default flips | `RetrievalContextColumnEnabled` flag flipped per A/B verdict matrix (single commit at sprint close) |
| 16 | Sprint plan + post + new feature doc | `sprint_plan_phase_14_2_*.md` + `phase_14_2_post.md` + `phase_14_2_forensic.md` (Epic 0 output) + new `docs/features/context-fingerprinting.md` |

**Out of scope (deferred):**

- Per-question rank-of-correct-citation telemetry on V0019 — would close Phase 14.1.1's open data gap but doubles V0019 schema; defer to Phase 14.x.1 if needed
- HTM sequence memory (Note 08) — depends on fingerprints but is its own sprint
- Active inference unification (Note 09)
- Cross-version fingerprint comparison via re-fingerprinting on catalog refresh — version-mismatch fallback (no-context match) is sufficient for v1 per Note 05 §4.3
- Automatic catalog refresh on first observation in a new space (cold start) — manual `mdemg migrate context-fingerprint --space <id>` is sufficient for v1
- Cross-space A/B (different `space_id` between baseline and candidate) — within-space only

**Constraints (hard, per MEMORY):**

- **Sequential epics** — schema before catalog package; catalog before fingerprint computation; computation before column wiring; column before A/B; A/B before default flip; docs always last
- **No hardcoded values** — every threshold/weight/percentile/clamp/budget/refresh-interval lives in env with `Validate()` bounds checks
- **Plan-options pattern** — 5 decision forks documented in §13; recommendations baked in; disclose at PR
- **Single batched commit at sprint close**
- **Sprint summary on PR comment** (per Phase 14 / 14.1 / 14.1.1 precedent)
- **CUIDv2 for any new IDs** (catalog versions land with `version_id` CUIDv2)
- **`max_tokens ≥ 3000`, `latency_budget_ms ≥ 15000`** — no LLM call sites added in this sprint
- **Live-testing required (Tier 3)** — A/B against real `whk-wms` + real grader; live polysemy demo on a non-production space
- **A/B merge gate** — same as Phase 14.1.1: B mean ≥ A AND no per-question regression > 10% (with `eps=1e-6` tolerance from Phase 14.1)
- **Default flip conditional** on full 120q passing (per Phase 13.1 / 14.1.1 precedent)

## 4. Dependencies

**Consumed (code, pre-existing — reuse, do not duplicate):**
- `internal/retrieval/column.go:20-31` — `Column` interface (the contract the new ContextColumn implements)
- `internal/retrieval/scoring_rrf.go:28-103` — `Service.ScoreAndRankRRF`; column-array wire-in at lines 45-49
- `internal/retrieval/consensus.go::Aggregate` — RRF aggregator (handles N columns generically; no changes needed)
- `internal/retrieval/service.go::scorerVersion()` (lines 68-84) — cache-namespace key (extend)
- `internal/retrieval/gate.go` — sparse activation gate (unchanged; operates on aggregator output)
- `internal/conversation/service.go::Service.Observe` (line 219) — observe-time fingerprint hook site
- `internal/conversation/types.go::Observation` (lines 52-89) — struct extension target
- `internal/ape/cycle.go::CycleOrchestrator.RunCycle` (line 124) — macro-cycle hook for weekly refresh
- `internal/cli/graph_repair.go` (lines 13-242) — streaming Neo4j cursor pattern for backfill CLI
- `internal/config/config.go::FromEnv` — env-loading pattern (mirror Phase 14.1 / 14.1.1 patterns)
- `migrations/V0024__signal_state.cypher` (and similar) — Cypher migration file pattern (`V<num>__<desc>.cypher`, `IF NOT EXISTS` constraints, idempotent)
- `internal/tsdb/migrations/019_sparse_gate_metrics.sql` — TSDB migration template (Phase 14.1.1)
- `docs/tests/uvts/runners/uvts_runner.py` + `uvts_ab_compare.py` (with eps=1e-6 fix from 14.1) — A/B harness
- `scripts/phase14_1_adaptive_runner.py` — extensible to per-context-column ablation

**Consumed (data):**
- Neo4j `whk-wms` space (8360 paths, 0 symbols, 0 roles, 5 role_types, 5 layers — primary A/B target)
- Phase 14.1.1 production baseline grades at `/tmp/phase13_1_full/candidate/grades.json` (current 120q production = gate-on, MIN=15+data_flow_integration override)
- Existing CO_ACTIVATED_WITH edges in Neo4j (input for post-hoc fingerprint refresh)
- TSDB V0017 `retrieval_audit` rows (cross-reference for Phase 14.2.1 if needed)

**Consumed (compute):**
- mdemg HTTP API at `localhost:9999`
- llama-server at port 8102 (Phase 13.5 substrate, stable)
- TimescaleDB at `localhost:5433`
- Neo4j at `localhost:7687`
- OpenAI API for UVTS grader (`gpt-5.4-mini`)

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch clean post-#368 merge; mdemg + llama-server healthy; TSDB schema 19; sparse gate default-on (Phase 14.1.1 production state).

### Epic 0 — Preflight + Multi-Space Catalog Density Audit

> Phase 14 Epic 0 audited only `whk-wms`. This epic extends to ≥2 spaces (whk-wms + mdemg-dev) so the adaptive Builder design has cross-space data, not just single-space evidence.

1. Cypher density audit per space: distinct-counts of `path`, `symbol`, `role`, `role_type`, `layer`, top-32 `tags` for `whk-wms`, `mdemg-dev`, and any other ≥1000-node space.
2. Tag-array enumeration: top-32 tag values per space (Note 05's reserved-64 partition was meant for tags but was never enumerated).
3. Cross-space density comparison → adaptive bit-allocation algorithm:
   - Reserve 32 bits for `role_type × layer` (always informative, low cardinality, works on all spaces)
   - Distribute remaining 224 bits across (symbols, paths, top-N tags) proportional to `log(1 + distinct_count)` with floor 16 bits per kind that has ≥10 distinct values
   - Per-space allocation persisted in `ContextCatalog.bits[]`
4. Identify any per-space property gaps (e.g. spaces with 0 tags) — design fallbacks
5. Audit current Neo4j MemoryNode property surface for naming conflicts with `context_fingerprint_active` / `_version` (V0028 must MERGE without breaking existing constraints)

**Output:** `docs/development/post-ft-lora/phase_14_2_forensic.md` with:
- Per-space density table (path, symbol, role, role_type, layer, tags) for ≥2 spaces
- Recommended Builder algorithm pseudocode + per-space allocation example
- Property-conflict audit (V0028/V0029 ready to ship without breaking constraints)

**Gate:** forensic doc committed; Builder algorithm grounded in cross-space data; no naming conflicts identified.

### Epic 1 — Schema (Neo4j V0028 + V0029, TSDB V0020) + Catalog Package Skeleton

1. **Neo4j V0028** — `migrations/V0028__context_fingerprints.cypher`:
   - Add `context_fingerprint_active []uint16` + `context_fingerprint_version int` properties on MemoryNode (no constraint; default NULL for existing nodes)
   - `CREATE INDEX context_fingerprint_idx IF NOT EXISTS FOR (m:MemoryNode) ON (m.context_fingerprint_version)` — supports version-mismatch fallback queries
   - `MERGE (migration:Migration {version: 28})` + `SchemaMeta` version bump to 28 (per project convention)
2. **Neo4j V0029** — `migrations/V0029__context_catalog.cypher`:
   - `ContextCatalog` node label per `(space_id, version)`
   - Indexes: `(space_id, version)`, `(space_id, is_active)` for fast active-version lookup
   - `bits[]` array of `{position, kind, ref, token}` objects
3. **TSDB V0020** — `internal/tsdb/migrations/020_context_catalog_versions.sql`:
   - Hypertable `context_catalog_versions` with columns `(version_id CUIDv2, recorded_at, space_id, version, total_bits, allocation_json, top_symbols TEXT[], top_paths TEXT[], top_tags TEXT[])`
   - 7-day chunks (matches V0017 / V0019)
   - `TSDB_REQUIRED_SCHEMA_VERSION` 19 → 20 in `config.go`
4. **Catalog package skeleton** — `internal/hidden/context_catalog.go`:
   - `Catalog` struct + `Builder` constructor signature (no body yet; Epic 2 implements)
   - `SymbolBit/PathBit/RoleBit/RoleTypeBit/LayerBit/TagBit` lookup methods (returning `(bit uint16, ok bool)`)
   - `LoadActive(ctx, spaceID)` + `LoadVersion(ctx, spaceID, version)` lookup signatures
5. Tier 1 unit tests on `Catalog` lookups (deterministic given fixture)

**Gate:** migrations apply forward (and reverse for Cypher per project convention); catalog skeleton + tests green; lint clean; `TSDB_REQUIRED_SCHEMA_VERSION=20` enforced.

### Epic 2 — Adaptive Builder + Refresh Job

1. **Implement `Builder.BuildForSpace(ctx, spaceID)`** in `internal/hidden/context_catalog.go`:
   - Query Neo4j for distinct counts per kind (paths, symbols, roles, role_type, layer values, top-N tags)
   - Allocate 256 bits per Epic 0 algorithm: 32-bit role_type×layer floor + log-proportional remainder across rich kinds
   - Assign each distinct value to a bit position deterministically (top-N most frequent in each kind)
   - Persist new `ContextCatalog` node in Neo4j (with `is_active=true`, mark previous active version `is_active=false`)
   - Persist version snapshot to TSDB V0020
2. **Builder determinism**: bit assignments must be stable across re-runs given the same input (sorted top-N selection + lexicographic tie-break). Builder idempotent.
3. **Catalog refresh hook** in `internal/ape/cycle.go::CycleOrchestrator.RunCycle`:
   - New stage 6 (post-Validate, pre-return)
   - Gated on `cfg.ContextFingerprintRefreshEnabled` (default true)
   - Calls `Builder.BuildForSpace(ctx, spaceID)` + then triggers post-hoc refresh of observations whose `version < new_catalog.version` (batched via internal helper, separate from the user-facing CLI in Epic 5)
   - Time-bounded: respect `cfg.ContextFingerprintRefreshTimeoutMs` (default 60000) — partial refresh acceptable; remaining observations picked up next cycle
4. **Cold-start handling**: if no `ContextCatalog` exists for a space, observations get empty fingerprints (graceful degradation per Note 05 §2.4); first refresh tick produces v1 catalog
5. Tier 1 + Tier 2 tests: deterministic Builder, idempotent re-run, version transition (v1 → v2 marks v1 `is_active=false` atomically), partial-refresh time-budget respected

**Gate:** Builder ships + tests green; `RunCycle` stage 6 fires correctly under test fixture; `ContextCatalog` node MERGE is idempotent; cold-start path exercised.

### Epic 3 — Observe-Time Fingerprint Computation

1. **Extend `Observation` struct** in `internal/conversation/types.go` (lines 52-89):
   - Add `ContextFingerprintActive []uint16` + `ContextFingerprintVersion int` fields after `OrgFlaggedBy`
2. **Implement `computeContextFingerprintLocal`** in new `internal/conversation/fingerprint.go`:
   - Inputs: `obs *Observation`, `catalog *Catalog`
   - Bits set:
     - Path bit (if `obs.Metadata["file_path"]` matches a catalog path entry)
     - Role bit (from `obs.Metadata["role"]` if present)
     - RoleType bit (from `obs.RoleType`)
     - Layer bit (from `obs.Layer`)
     - Top-N tag bits (intersection of `obs.Tags` with catalog tag entries)
   - Symbol bit deferred to post-hoc refresh (no co-activation at observe time)
   - Returns sorted `[]uint16` for stable storage
3. **Wire into `Service.Observe`** (`internal/conversation/service.go:219`):
   - After observation struct is constructed, before `createObservationNode()` Cypher (line ~537)
   - Load active catalog for `obs.SpaceID` via `Catalog.LoadActive(ctx, spaceID)`
   - If catalog cold (nil): set empty fingerprint + `version=0`
   - If hot: compute via `computeContextFingerprintLocal`; set fields on `Observation`
4. **Update `createObservationNode()` Cypher** (`internal/conversation/service.go` lines ~537-567) to set new fields
5. **Implement post-hoc refinement** (`internal/conversation/fingerprint.go`):
   - `RefineWithCoactivations(ctx, obsID, catalog)` — used by Epic 2's CycleOrchestrator hook
   - Queries Neo4j for `MATCH (obs)-[:CO_ACTIVATED_WITH]-(other) RETURN other`
   - Adds symbol bits for each co-activated MemoryNode that has a catalog symbol bit
   - Updates observation `ContextFingerprintActive` + bumps `_version` to current catalog
6. Tier 1 + Tier 2 tests: deterministic fingerprint computation, empty-catalog fallback, Cypher integration test, post-hoc refinement on a seeded fixture

**Gate:** observe-time fingerprint sets correctly on new observations; post-hoc refinement adds symbol bits when CO_ACTIVATED_WITH edges exist; tests green; lint clean.

### Epic 4 — Context-Aware Retrieval (5th RRF Column + Strict Mode)

1. **Implement `ContextColumn`** in new `internal/retrieval/column_context.go`:
   - Implements `Column` interface from `column.go:20-31`
   - `Run(ctx, q ColumnQuery)` ranks candidates by Jaccard similarity between observation fingerprint and `q.QueryContextFingerprint`
   - Score = `|intersection| / |union|`, bounded `[0, 1]`
   - When query fingerprint empty or candidate fingerprint empty → score 0 (graceful degradation; column contributes 0 to RRF sum but doesn't block)
   - Cypher fetch path: candidate fingerprints already loaded from Neo4j during upstream candidate gathering (Phase 13.1 path); ContextColumn reads from in-memory candidates (no extra I/O)
2. **Wire into RRF aggregator** at `internal/retrieval/scoring_rrf.go:45-49`:
   - Add 5th column to array, gated on `cfg.RetrievalColumnContextEnabled`
   - Update header comment from "4 columns" to "5 columns"
3. **Cache-namespace bump** in `internal/retrieval/service.go::scorerVersion()` (lines 68-84):
   - Bump `v1-rrf4` → `v1-rrf5`
   - Add `|ctx=%t|ctx_w=%.3f` to format string
   - Include `RetrievalContextStrictThreshold` if non-default (so strict-mode flips the namespace too)
4. **Extend `ColumnQuery`** in `internal/retrieval/column.go` (line 35):
   - Add `QueryContextFingerprint []uint16` field
5. **Extend `RetrieveRequest`** in `internal/models/models.go`:
   - Add `QueryContextFingerprint []uint16 \`json:"query_context_fingerprint,omitempty"\``
   - Add `StrictContextMode bool \`json:"strict_context,omitempty"\``
6. **URL params** in `internal/api/handlers.go::handleRetrieve`:
   - `?strict_context=true` populates `req.StrictContextMode`
   - `?context=auto` triggers a server-side helper to derive query fingerprint from session's recent co-activations (Note 05 §3.1 default behavior)
7. **Strict-mode pre-aggregation filter** in `internal/retrieval/service.go::Retrieve`:
   - When `req.StrictContextMode=true` AND `req.QueryContextFingerprint` non-empty, filter candidates by Jaccard ≥ `cfg.RetrievalContextStrictThreshold` (default 0.25) BEFORE the RRF aggregator runs
8. **Per-candidate explanation surface** when `req.JiminyEnabled`: include matched-bits enumeration in `debug.context_match[node_id] = [bit_positions]`
9. Tier 1 + Tier 2 tests: ContextColumn ranking math, strict-mode filter math, cache namespace flip on enable/disable

**Gate:** ContextColumn fires correctly in 5-column RRF; cache namespace flips when context-enabled toggles; strict-mode filter works on a polysemy fixture; tests green; lint clean.

### Epic 5 — Backfill CLI + Opt-In Rollout

1. **Implement `mdemg migrate context-fingerprint --space <id>`** in new `internal/cli/migrate_context_fingerprint.go`:
   - Streaming Neo4j cursor pattern from `internal/cli/graph_repair.go` (no checkpoint table; idempotent via SET-overwrite)
   - Batches of 500 observations per transaction (configurable via `--batch-size`)
   - `--dry-run` flag (default true; matches `graph_repair.go` safety pattern)
   - Per observation: load active catalog → compute observe-time-style fingerprint (no co-activations) → SET fingerprint + version → emit progress to stdout every 100 obs
   - Idempotent: skips observations whose `version >= current_catalog_version`
2. **Document rollout flow** in new `docs/features/context-fingerprinting.md`:
   - New observations get fingerprints automatically (Epic 3 wiring)
   - Historical observations require operator-run `mdemg migrate context-fingerprint --space <id>` per space (opt-in)
   - Refresh cadence: weekly via CycleOrchestrator (Epic 2)
3. **Verify on `mdemg-dev` (non-production space)**:
   - Backfill 100 observations → check fingerprint distribution + `version` field populated
   - Re-run → idempotent (no churn)
4. Tier 2 integration test: seeded space + backfill CLI → assert all observations have fingerprints + version

**Gate:** backfill CLI works on `mdemg-dev`; idempotent re-run produces no churn; doc complete; lint clean.

### Epic 6 — A/B Verification (Note 05 alone + Combined with Phase 14.1.1 Gate)

1. **Extend `scripts/phase14_1_adaptive_runner.py`** to support context-column presets (or new `scripts/phase14_2_runner.py` if extension is awkward)
2. **3 quick-profile (16q) presets** vs Phase 14.1.1 production baseline (gate-on, MIN=15+data_flow override, no context column):
   - **`fingerprint_only`**: ContextColumn enabled (weight 0.10) + sparse gate enabled (current 14.1.1 config). Tests Note 05 lift on top of stable gate.
   - **`gate+fingerprint_combined`**: same as fingerprint_only but with the 14.1.1 gate's `data_flow_integration` override expanded to consider context column (no extra knob — same gate config; tests interaction)
   - **`fingerprint_strict`**: ContextColumn enabled + `?strict_context=true` enforced for queries with rich session co-activation context
3. **Acceptance**: at least one preset passes (B mean ≥ A AND no per-question regression > 10% with `eps=1e-6` tolerance from Phase 14.1)
4. **Full 120q on quick winner**: Same criterion. Per-category breakdown captures whether Note 05 lifts polysemy-prone categories (`architecture_structure`, `data_flow_integration`)
5. **Live polysemy demo (Tier 3)**:
   - Seed 3 observations of `ErrorHandler` in distinct contexts on `mdemg-dev` (auth/, payments/, inventory/)
   - Retrieve with each context; verify ranking shifts toward the matching-context observation
   - Document the verdict with screenshots / debug output in `phase_14_2_post.md`
6. **Decision matrix** for default flips:
   - If `fingerprint_only` passes 120q → flip `RetrievalContextColumnEnabled=true` default
   - If `fingerprint_strict` passes 120q only → ship strict-mode opt-in, default-off
   - If neither passes → ship flag-off, scope Phase 14.2.1 with weight ablation

**Gate:** A/B verdict captured (per-preset); live polysemy demo passes; default flips applied per matrix; Tier 3 live test documented.

### Epic 7 — Documentation (Final Epic — Never Cut)

- `docs/development/post-ft-lora/sprint_plan_phase_14_2_note_05_sparse_fingerprints.md` — frozen plan
- `docs/development/post-ft-lora/phase_14_2_post.md` — executed truth: A/B verdicts, polysemy demo evidence, OpenAI spend actual, fork outcomes, default flip rationale
- `docs/development/post-ft-lora/phase_14_2_forensic.md` — Epic 0 multi-space density tables
- `docs/features/context-fingerprinting.md` (NEW per `feedback_per_feature_docs_required.md`) — Why / Choices / How it works / How to use
- `SPRINT_ROADMAP_POST_FT_LORA.md` — Phase 14.2 EXECUTED with commit SHA; Note 07/08/09 status
- `AGENT_HANDOFF.md` top entry
- `CHANGELOG.md` `[Unreleased] ### Added`
- `CLAUDE.md` — new "Context Fingerprinting (Phase 14.2)" Architecture-Notes subsection covering: feature-flag enable/disable, the adaptive Catalog Builder, two-phase fingerprinting (observe-time + post-hoc refresh), ContextColumn weight, strict-mode threshold, refresh-cycle hook, backfill CLI

**Gate:** all docs landed; cross-refs valid; conditional default flips applied.

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit) — `go test -race`:**
- `internal/hidden/context_catalog_test.go` — Builder determinism (same input → same bit assignments), idempotent re-run, lookups (Symbol/Path/Role/RoleType/Layer/Tag)
- `internal/conversation/fingerprint_test.go` — observe-time computation deterministic, empty-catalog fallback, post-hoc refinement
- `internal/retrieval/column_context_test.go` — Jaccard ranking math, version-mismatch fallback (returns 0), empty-fingerprint fallback
- `internal/retrieval/service_test.go` — `scorerVersion()` flips on context-column toggle
- `internal/config/config_phase14_2_test.go` — defaults + env override + Validate cross-field for the 12 new knobs

**Tier 2 (Integration) — `go test -tags=integration`:**
- `tests/integration/context_fingerprint_e2e_test.go` — observe with metadata → retrieve with context fingerprint → verify ranking shift on a polysemy fixture
- `tests/integration/catalog_refresh_test.go` — schedule + verify new ContextCatalog version created; old versions retained with `is_active=false`
- `tests/integration/v0028_v0029_migration_test.go` — Cypher forward + reverse on dev DB; pre+post node-count audit
- `tests/integration/v0020_migration_test.go` — TSDB forward + reverse on test DB
- `tests/integration/backfill_cli_test.go` — seeded space + CLI run → all observations have fingerprints; idempotent re-run

**Tier 3 (Live E2E) — MANDATORY per CLAUDE.md `d10c1a5`:**
- **Epic 6 A/B verification** — UVTS quick × 3 presets + full 120q on winner; real mdemg + real grader; verdict captured in `uvts_runs`
- **Live polysemy demo** — seed 3 ErrorHandler observations on `mdemg-dev` in distinct contexts; retrieve with each context; verify ranking shifts (the canonical "this works" demonstration for Note 05)
- **Live backfill** — `mdemg migrate context-fingerprint --space mdemg-dev` produces fingerprints; check via Cypher `MATCH (m:MemoryNode {space_id:'mdemg-dev'}) WHERE m.context_fingerprint_version IS NOT NULL RETURN count(m)`
- **Live refresh tick** — manually trigger `RunCycle` on `mdemg-dev`; verify new ContextCatalog version + observations re-fingerprinted
- **Long-running soak (24 hr, low-volume)**: verify catalog refresh fires weekly; no migration deadlocks; Prometheus metrics flowing

**State restoration (MEMORY):** all changes additive or feature-flagged. Rollback options:
1. `git revert <commit>` — removes catalog, fingerprint, ContextColumn, CLI, docs
2. **Runtime emergency disable**: `RETRIEVAL_CONTEXT_COLUMN_ENABLED=false` + `mdemg restart`. Reverts to Phase 14.1.1 4-column RRF behavior; cache becomes cold (different scorer_version namespace)
3. **TSDB rollback**: `mdemg tsdb migrate --target V0019` drops `context_catalog_versions`; preserves V0017+V0018+V0019 untouched
4. **Neo4j rollback** (manual): drop `ContextCatalog` nodes + remove `context_fingerprint_active` / `_version` properties from MemoryNode (per project Cypher migration convention — manual SET NULL)
5. **Per-feature suppress** (no rebuild needed): `RETRIEVAL_CONTEXT_COLUMN_WEIGHT=0` zeros the column's RRF contribution while keeping fingerprints written for future re-enable

**Gate:** all 3 tiers green; A/B verdict captured (pass / fail); polysemy demo shows expected ranking shift; backfill + refresh live-verified.

## 7. Commit Strategy

Single batched commit at sprint close (per MEMORY rule):

- Title: `feat(retrieval): Sprint POST-FT-LORA-PHASE14.2 — Note 05 sparse fingerprints (two-phase, adaptive catalog) + 5th RRF column`
- Body: scope summary, A/B verdict (per-preset numbers), live polysemy demo result, V0028+V0029+V0020 row counts, decision-fork outcomes, OpenAI spend actual, conditional default-flip note
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`

Push to `reh3376_dev01` → auto-PR opens → **sprint summary comment posted to PR per MEMORY rule (not gated on CI)**.

## 8. Verification Checklist

- [ ] Epic 0: forensic doc shipped; multi-space density audit committed; per-space allocation algorithm documented
- [ ] Epic 1: V0028 + V0029 + V0020 migrations apply forward; catalog package skeleton + tests green; `TSDB_REQUIRED_SCHEMA_VERSION=20`
- [ ] Epic 2: Builder ships deterministic + idempotent; `RunCycle` stage 6 fires; cold-start path exercised; tests green
- [ ] Epic 3: observe-time fingerprint sets on new observations; post-hoc refinement adds symbol bits; tests green
- [ ] Epic 4: ContextColumn fires in 5-column RRF; cache namespace flips on toggle; strict mode works on polysemy fixture
- [ ] Epic 5: backfill CLI works on `mdemg-dev`; idempotent re-run; doc complete
- [ ] Epic 6: A/B verdict captured; live polysemy demo passes; conditional default flip applied per matrix
- [ ] Epic 7: sprint plan + post + forensic + new feature doc + ROADMAP + AGENT_HANDOFF + CHANGELOG + CLAUDE.md
- [ ] Commit pushed; auto-PR updated; sprint summary on PR
- [ ] OpenAI spend logged (target $25–30, cap $100)
- [ ] All 5 decision-fork choices disclosed in commit body + PR comment
- [ ] `golangci-lint run ./...` clean
- [ ] CI green on the auto-PR
- [ ] Memory observation captured (CMS) at sprint close

### Configuration Reference (12 new knobs)

| Env Var | Default | Description |
|---|---|---|
| `RETRIEVAL_CONTEXT_COLUMN_ENABLED` | `false` initially; flipped per Epic 6 verdict | Master toggle for the 5th RRF column |
| `RETRIEVAL_CONTEXT_COLUMN_WEIGHT` | `0.10` | RRF weight on the context column |
| `RETRIEVAL_CONTEXT_STRICT_THRESHOLD` | `0.25` | Jaccard threshold for `?strict_context=true` mode |
| `CONTEXT_FINGERPRINT_ENABLED` | `true` | Master toggle for fingerprint computation at observe time |
| `CONTEXT_FINGERPRINT_BIT_BUDGET` | `256` | Total bit count per fingerprint (Note 05 spec default) |
| `CONTEXT_FINGERPRINT_REFRESH_ENABLED` | `true` | Master toggle for the post-hoc refresh stage in CycleOrchestrator |
| `CONTEXT_FINGERPRINT_REFRESH_INTERVAL_HOURS` | `168` | Catalog refresh cadence (weekly per spec) |
| `CONTEXT_FINGERPRINT_REFRESH_TIMEOUT_MS` | `60000` | Per-cycle time budget for post-hoc refresh batch |
| `CONTEXT_CATALOG_TOP_N_PATHS` | `192` | Top-N path bits in the catalog |
| `CONTEXT_CATALOG_TOP_N_TAGS` | `32` | Top-N tag bits in the catalog |
| `CONTEXT_CATALOG_FLOOR_BITS_PER_KIND` | `16` | Minimum bits allocated to any kind with ≥10 distinct values |
| `CONTEXT_CATALOG_ROLE_TYPE_LAYER_BITS` | `32` | Reserved bits for `role_type × layer` combinations |

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 7. Per `feedback_per_feature_docs_required.md`: Phase 14.2 ships a new user/operator-visible feature (context fingerprinting) → MUST add `docs/features/context-fingerprinting.md` with Why / Choices / How it works / How to use.

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | A/B regression on quick profile (Note 05 underperforms baseline) | Medium | Equal-weight starting point (`weight=0.10`); observation-local fingerprint is the cold-start state — won't actively hurt; expect post-hoc refinement to lift mature data over time | Iterate weight / drop column / ship flag-off; scope Phase 14.2.1 with adjusted weight |
| 2 | Adaptive Builder produces unstable bit assignments across refreshes | Medium | Builder is deterministic given same input (sorted top-N + lexicographic tie-break); top-N selection requires distinct counts to be relatively stable week-over-week (whk-wms is) | Pin catalog version in tests; if production unstable, fall back to fixed-allocation per-space override env |
| 3 | Backfill on large space (mdemg-dev: 78k observations) takes hours | High | Batched (500), resumable (idempotent SET); operator runs in maintenance window | Skip backfill; new observations only; post-hoc refresh catches up over weeks |
| 4 | Observe-time fingerprint adds latency to `Service.Observe` | Low | Catalog lookup is in-memory after first load; compute is `O(catalog_size)` ≈ 256 bit checks; <1ms | Cache catalog per process |
| 5 | Cross-version fingerprint comparison ambiguous | Low | Versioned fingerprints; cross-version → no-context fallback (Note 05 §4.3) | Re-fingerprinting in post-hoc refresh handles forward migration |
| 6 | V0028 ALTER on production-sized graph blocks | Low | Property addition only; Neo4j ALTER is fast (no constraint added) | Test on `mdemg-dev` first |
| 7 | Cache pollution between context-on and context-off A/B presets | Low | `Service.scorerVersion()` extended with `ctx=%t\|ctx_w=%.3f`; flag-flip = different namespace = automatic cold cache | Manual cache flush via admin endpoint |
| 8 | Live polysemy demo seeds don't survive catalog refresh | Low | Seeds use `mdemg-dev` (test space); refresh cadence weekly so demo seeds persist for the sprint window | Reseed if needed; document seed Cypher |
| 9 | Combined gate+fingerprint A/B unexpected interaction | Medium | Epic 6 specifically tests combined preset against individual + baseline; flip both only if combined passes | Ship the one that passes individually; defer combo to Phase 14.2.1 |
| 10 | Tag inventory empty on whk-wms breaks adaptive Builder | Medium | Epic 0 audit catches; Builder allocates 0 bits to empty kinds (no crash); fall back to path+role_type+layer split | Document gap; Phase 14.2.1 backfills tag normalization |
| 11 | Phase 14.1.1 sparse gate interferes with ContextColumn (gate cuts ContextColumn votes) | Low | Gate operates on aggregated ranks, not per-column. ContextColumn votes are already integrated by aggregator before gate fires | If issue surfaces in A/B, scope per-column gate exemption in Phase 14.2.1 |
| 12 | golangci-lint regression on the new package | Low | Run lint per epic before gate; Phase 14.1.1 already established lint-clean baseline | Fix in-sprint |

## 11. Documents Accessed (during planning)

**Read during planning:**
- `docs/research/mdemg_sprint_ideas/05-context-specific-node-activations.md` — Note 05 design draft (380 lines; full read-through)
- `docs/development/post-ft-lora/sprint_plan_phase_14_2_note_05_sparse_fingerprints.md` — Phase 14.2 stub plan (77 lines)
- `docs/development/post-ft-lora/phase_14_score_distribution_analysis.md` §4 — Phase 14 Epic 0 forensic on whk-wms feature density (0 symbols, 0 roles, 8360 paths, etc.)
- `docs/development/post-ft-lora/phase_14_post.md` — Phase 14 narrow close + 14.2 deferral context
- `docs/development/post-ft-lora/phase_14_1_1_post.md` — Phase 14.1.1 default-on baseline (the production state Phase 14.2 A/B compares against)
- `internal/retrieval/column.go` (lines 20-31, 35) — Column interface + ColumnQuery shape (extended in Epic 4)
- `internal/retrieval/scoring_rrf.go` (lines 28-103, 45-49) — RRF aggregator + 4-column array wire-in (5th column added in Epic 4)
- `internal/retrieval/consensus.go` — RRF aggregator (no changes; handles N columns generically)
- `internal/retrieval/service.go::scorerVersion()` (lines 68-84) — cache namespace key (extended in Epic 4)
- `internal/retrieval/gate.go` — sparse gate (unchanged; operates on aggregator output)
- `internal/conversation/service.go::Service.Observe` (line 219) — observe-time fingerprint hook site
- `internal/conversation/types.go` (lines 52-89) — Observation struct
- `internal/ape/cycle.go::CycleOrchestrator.RunCycle` (line 124) — macro-cycle stage 6 hook
- `internal/cli/graph_repair.go` (lines 13-242) — backfill CLI pattern model
- `migrations/V0024__signal_state.cypher` — Cypher migration template
- `internal/tsdb/migrations/019_sparse_gate_metrics.sql` — TSDB migration template
- Memory: `feedback_sprint_plan_format.md`, `feedback_no_hardcoded_values.md`, `feedback_sequential_epics.md`, `feedback_plan_options_pattern.md`, `feedback_live_testing_required.md`, `feedback_sprint_summary_on_pr.md`, `feedback_per_feature_docs_required.md`, `feedback_data_decides_not_operator.md`, `feedback_cuidv2_required.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`

**External:** none required.

## 12. Rollback

All changes additive or feature-flagged. Specific rollback steps:

1. **`git revert <final commit SHA>`** — removes catalog package, fingerprint computation, ContextColumn, backfill CLI, V0028+V0029+V0020 migrations, doc files, config knobs.
2. **Runtime emergency disable** (no rebuild needed):
   - `RETRIEVAL_CONTEXT_COLUMN_ENABLED=false` + `mdemg restart` → reverts to Phase 14.1.1 4-column RRF; cache becomes cold (different scorer_version namespace) — first 5 minutes of post-disable retrieves are uncached, then warm
   - `CONTEXT_FINGERPRINT_ENABLED=false` → stops new observations from getting fingerprints; existing fingerprints stay on disk (no harm; just dormant)
   - `CONTEXT_FINGERPRINT_REFRESH_ENABLED=false` → halts post-hoc refresh in CycleOrchestrator
3. **TSDB rollback**: `mdemg tsdb migrate --target V0019` drops `context_catalog_versions` table only. V0017+V0018+V0019 + earlier preserved.
4. **Neo4j rollback** (manual per project convention; no built-in reverse migration framework):
   - `MATCH (m:MemoryNode) REMOVE m.context_fingerprint_active, m.context_fingerprint_version` (drops the properties)
   - `MATCH (c:ContextCatalog) DETACH DELETE c` (drops catalog nodes)
   - `DROP INDEX context_fingerprint_idx IF EXISTS` (cleanup)
5. **Per-feature suppress** (no rebuild, no restart needed if config hot-reload): `RETRIEVAL_CONTEXT_COLUMN_WEIGHT=0` zeros the column's RRF contribution while keeping fingerprints written for future re-enable.

Phase 11+ artifacts untouched. mdemg-llm-v1 model untouched. Phase 14.1.1 sparse gate continues to work (it operates on the aggregator output regardless of column count).

---

## 13. Plan-Options (decision forks — pick at execution, disclose in PR)

Per MEMORY `feedback_plan_options_pattern.md`. Forks below are decided at Epic 0 close (data-cited where possible) or empirically by A/B (Epic 6).

| # | Fork | Recommendation (provisional) | Alternative(s) | Decision basis |
|---|---|---|---|---|
| 1 | **Fingerprint timing** | **Two-phase: observe-time + post-hoc refresh** (operator-approved) | (a) Observation-local only; (b) Async post-hoc only; (c) Synchronous co-activity at observe time | Operator-approved 2026-05-05. Cold-start works (every obs has *some* fingerprint immediately); mature data gets HTM-faithful fingerprints after weekly refresh tick. |
| 2 | **Bit assignment policy** | **Adaptive per-space** (Builder counts density at refresh, allocates by `log(distinct)` with floors) | static 64/64/64/64 (Note 05 spec) | Phase 14 Epic 0 forensic: whk-wms has 0 symbols + 0 roles → static split wastes 128 bits. Adaptive design grounded in cross-space data per Epic 0. |
| 3 | **Catalog refresh cadence** | **Weekly (168 hr)** per spec | Daily (24 hr) — too noisy for top-N stability; monthly — too stale for new co-activations | Spec default + bit-stability evidence; aligns with macro-cycle scheduler. |
| 4 | **ContextColumn default weight** | **0.10** (Note 05 spec default) | 0.05 (gentler), 0.15 (stronger) | Spec default; Epic 6 A/B can ablate if needed in Phase 14.2.1. Equal-weight prior is honest; don't pre-tune without data. |
| 5 | **Strict-mode default Jaccard threshold** | **0.25** per spec | Higher (0.4) for tighter context match; lower (0.1) for permissive | Spec default. Strict-mode is opt-in (default behavior is soft weighting); threshold only matters when explicitly invoked. |

If Epic 0 forensic data definitively contradicts a "spec default" pick (e.g. cross-space density tables suggest different bit budget), the data wins.

---

## Acceptance bar (top-level)

A successful Phase 14.2 ships when:
1. **Epic 0 forensic** identifies cross-space-validated adaptive Builder algorithm
2. **Epic 1+2** schema + Builder + refresh hook all functional + tested
3. **Epic 3** observe-time fingerprint sets on new observations + post-hoc refinement adds symbol bits when CO_ACTIVATED_WITH edges exist
4. **Epic 4** ContextColumn fires correctly in 5-column RRF; cache namespace flips
5. **Epic 5** backfill CLI works idempotently on a non-production space
6. **Epic 6** at least one A/B preset passes 120q full (mean ≥ A + 0 regressions per-question with eps tolerance)
7. **Live polysemy demo** shows expected ranking shift on the seeded ErrorHandler fixture
8. **Documentation complete** — sprint plan + post + forensic + new feature doc + standard 4-doc footprint
9. **Default flip** applied per Epic 6 verdict matrix

A "failed" Phase 14.2 (no preset reaches A/B pass) ships value: schema + adaptive Builder + fingerprint computation + backfill + Phase 14.2.1 scoped (mirrors Phase 14 → 14.1 → 14.1.1 split). Mirrors the precedent established by the Phase 14.x sequence.

---

## Critical files to modify (concrete paths)

**New files:**
- `migrations/V0028__context_fingerprints.cypher`
- `migrations/V0029__context_catalog.cypher`
- `internal/tsdb/migrations/020_context_catalog_versions.sql`
- `internal/hidden/context_catalog.go` + `_test.go`
- `internal/conversation/fingerprint.go` + `_test.go`
- `internal/retrieval/column_context.go` + `_test.go`
- `internal/cli/migrate_context_fingerprint.go`
- `docs/features/context-fingerprinting.md`
- `docs/development/post-ft-lora/sprint_plan_phase_14_2_*.md` (overwrite stub)
- `docs/development/post-ft-lora/phase_14_2_post.md`
- `docs/development/post-ft-lora/phase_14_2_forensic.md`
- `tests/integration/context_fingerprint_e2e_test.go`
- `tests/integration/catalog_refresh_test.go`
- `tests/integration/v0028_v0029_migration_test.go`
- `tests/integration/v0020_migration_test.go`
- `tests/integration/backfill_cli_test.go`

**Existing files to modify:**
- `internal/conversation/types.go` (extend Observation struct)
- `internal/conversation/service.go` (Service.Observe + createObservationNode Cypher)
- `internal/ape/cycle.go` (CycleOrchestrator.RunCycle stage 6)
- `internal/retrieval/column.go` (extend ColumnQuery)
- `internal/retrieval/scoring_rrf.go` (5th column wire-in + comment)
- `internal/retrieval/service.go` (scorerVersion + strict-mode pre-aggregation filter)
- `internal/api/handlers.go` (URL params for ?strict_context= and ?context=auto)
- `internal/models/models.go` (RetrieveRequest extension)
- `internal/config/config.go` (12 new knobs + Validate cross-field checks; bump TSDB_REQUIRED_SCHEMA_VERSION 19→20)
- `internal/metrics/collectors.go` (5 new histograms/gauges)
- `scripts/phase14_1_adaptive_runner.py` (extend to context-column presets) OR new `scripts/phase14_2_runner.py`
- `AGENT_HANDOFF.md`, `CHANGELOG.md`, `CLAUDE.md`, `SPRINT_ROADMAP_POST_FT_LORA.md`
- `docs/features/sparse-retrieval.md` (cross-ref Phase 14.2 ContextColumn)

## Verification (end-to-end, after Epic 6)

1. **Schema verified live**:
   - `mdemg tsdb status` shows `schema_version=20`
   - Cypher: `CALL db.indexes() YIELD name WHERE name='context_fingerprint_idx' RETURN *` returns the index
   - Cypher: `MATCH (c:ContextCatalog {space_id:'whk-wms', is_active:true}) RETURN c.version, size(c.bits)` returns `(1, 256)` after first refresh
2. **Observe pipeline verified**:
   - POST `/v1/conversation/observe` with metadata → check Cypher `MATCH (m:MemoryNode) WHERE m.obs_id = $id RETURN m.context_fingerprint_active, m.context_fingerprint_version`
3. **Retrieve pipeline verified**:
   - POST `/v1/memory/retrieve` with `query_context_fingerprint` → response `debug.context_match[node_id]` populated
   - `?strict_context=true` filters out low-Jaccard candidates
4. **Backfill CLI verified**:
   - `mdemg migrate context-fingerprint --space mdemg-dev --dry-run=false --batch-size 500`
   - Verify with Cypher `MATCH (m:MemoryNode {space_id:'mdemg-dev'}) WHERE m.context_fingerprint_version > 0 RETURN count(m)`
5. **Post-hoc refresh verified**:
   - Trigger `RunCycle` manually on `mdemg-dev`
   - Check that observations whose `version < new_catalog.version` got refreshed
6. **A/B verdict verified**: `/tmp/phase14_2_full/<winner>/verdict.json` shows `verdict: pass` with mean Δ ≥ 0 and 0 per-question regressions
7. **Live polysemy demo verified**: 3 ErrorHandler observations seeded; retrieves with distinct contexts produce ranking shifts captured in `phase_14_2_post.md`
8. **Prometheus metrics verified**: `/v1/metrics/snapshot` shows `mdemg_context_fingerprint_size`, `_catalog_version`, `_similarity_score`, `_observations_without_fingerprint`, `_refresh_duration_seconds` all populated post-cycle
9. **CMS observation captured** at sprint close noting: "Phase 14.2 Note 05 sparse fingerprints + ContextColumn shipped [default state]; polysemy demo on mdemg-dev verified ranking shifts; Phase 14.x sequence (14, 14.1, 14.1.1, 14.2) complete."
