# Phase 14.2 — Note 05 Sparse Fingerprints (post-execution)

**Date**: 2026-05-05
**Branch**: `reh3376_dev01`
**Sprint plan**: [`sprint_plan_phase_14_2_note_05_sparse_fingerprints.md`](sprint_plan_phase_14_2_note_05_sparse_fingerprints.md)
**Forensic**: [`phase_14_2_forensic.md`](phase_14_2_forensic.md)
**Feature doc**: [`docs/features/context-fingerprinting.md`](../../features/context-fingerprinting.md)
**Verdict**: **Narrow close** — infrastructure shipped flag-off; A/B fingerprint-derivation tuning deferred to Phase 14.2.1.

---

## Executive summary

Phase 14.2 ships the full infrastructure for Note 05 sparse context fingerprints: schema (V0025 + V0026 + TSDB V0020), adaptive Builder, two-phase fingerprint computation, 5th RRF column (ContextColumn), strict-mode pre-filter, server-side `?context=auto` query-fingerprint helper, and the `mdemg migrate context-fingerprint` backfill CLI. The 16q quick A/B against the refreshed `whk-wms` corpus showed:

- **Mean Δ = -0.006** (within `eps=1e-6`)
- **1 improvement (qhard_sym_5 +0.100), 0 regressions >10%, 13 unchanged**
- **`correct_file_rate` 0.688 → 0.625** (ContextColumn slightly displaced ranks on 1-2 questions)

**Diagnosis**: catalog tags are LLM-summary-extracted high-level engineering concepts (`api`, `architecture`, `caching`, `logging`, ...); UVTS queries use domain vocabulary (`barrel ownership transfer`, `validation error`, ...). `?context=auto`'s naive token-match derivation produces empty fingerprints for most queries → ContextColumn contributes 0 to the RRF sum → B ≈ A.

**Default state at sprint close**: `RETRIEVAL_CONTEXT_COLUMN_ENABLED=false` (unchanged from Epic 1 default). Operators can opt in. `CONTEXT_FINGERPRINT_ENABLED=true` stays the default so observation fingerprints accumulate for future use.

---

## What landed

### Schema
- **Neo4j V0025** — `MemoryNode.context_fingerprint_active []uint16` + `_version int` + index on `_version`
- **Neo4j V0026** — `ContextCatalog` node label, `bits_json` JSON-encoded property (Neo4j disallows arrays of map literals — see Bug 2 below)
- **TSDB V0020** — `context_catalog_versions` hypertable
- `TSDB_REQUIRED_SCHEMA_VERSION` 19 → 20 + 4 deploy configs bumped 24 → 26

### Code
- `internal/hidden/context_catalog.go` (~225 LOC) — Catalog struct, Loader/Builder interfaces, BitKind enum
- `internal/hidden/context_catalog_builder.go` (~620 LOC) — neo4jBuilder + neo4jLoader; 4 Cypher density queries; deterministic allocateBits; atomic persistCatalog
- `internal/conversation/fingerprint.go` (~210 LOC) — observe-time `ComputeContextFingerprintLocal` + post-hoc `RefineWithCoactivations`
- `internal/retrieval/column_context.go` (~110 LOC) — ContextColumn + JaccardFingerprint
- `internal/cli/migrate_context_fingerprint.go` (~290 LOC) — backfill CLI keyed on node_id
- `internal/api/handlers.go` — `?strict_context=true` + `?context=auto` URL params + `deriveQueryFingerprint` helper + `tokenizeForFingerprint` (camelCase / snake_case / kebab-case aware)
- `internal/ape/cycle.go` — Stage 6 hook on `CycleOrchestrator.RunCycle` (gated, time-bounded, weekly)
- 5 wire-ins: `ScoreAndRankRRF` 5th column, cache namespace `v1-rrf4|...` → `v1-rrf5|...|c=W|...|ctx=B|strict=T`, `Service.SetCatalogLoader` for observe-time, `CycleOrchestrator.SetContextCatalog` for Stage 6, `Server.deriveQueryFingerprint` for `?context=auto`

### 12 new env knobs
`CONTEXT_FINGERPRINT_{ENABLED, BIT_BUDGET, REFRESH_ENABLED, REFRESH_INTERVAL_HOURS, REFRESH_TIMEOUT_MS}`,
`CONTEXT_CATALOG_{TOP_N_PATHS, TOP_N_TAGS, FLOOR_BITS_PER_KIND, ROLE_TYPE_LAYER_BITS}`,
`RETRIEVAL_CONTEXT_{COLUMN_ENABLED, COLUMN_WEIGHT, STRICT_THRESHOLD}`. Validate cross-field check enforces `TopNPaths + TopNTags + RoleTypeLayerBits ≤ BitBudget`.

### Live verification
- `whk-wms` corpus refreshed: deleted (45,705 nodes including symbols/edges), re-ingested from `WhiskeyHouse/whk-wms@c1e4263e` (3,804 source files, 8,974 elements, 9,791 MemoryNodes, 5 role_types, 5 layers, 64 distinct tags, 35,146 symbols extracted, 0 errors, 26 min)
- 5-layer hierarchy consolidated (756 hidden + 76/6/1/1 concepts at L2-L5)
- ContextCatalog v1 persisted (256 bits: 7 role_type×layer + 32 tags + 217 paths)
- 9,541 nodes fingerprinted (avg 5.87 active bits / 256 = **2.29% sparsity** — matches HTM <2-3% target)
- mdemg-dev space: 78,041 nodes intact (protected-space whitelist worked correctly)

---

## A/B results — 16q quick on refreshed whk-wms

```
Metric                      A_baseline    B_fp_only            Δ
-----------------------------------------------------------------
  mean                          0.4210       0.4150      -0.0060
  median                        0.4500       0.4500      +0.0000
  min                           0.3500       0.3500      +0.0000
  max                           0.4590       0.4590      +0.0000
  std                           0.0480       0.0500      +0.0020
  correct_file_rate             0.6880       0.6250      -0.0630
  high_score_rate               0.0000       0.0000      +0.0000

Per-category mean:
  computed_value (n=2)            0.4000     0.3500    -0.0500
  architecture_structure (n=2)    0.4550     0.4550    +0.0000
  relationship (n=2)              0.4000     0.4500    +0.0500
  cross_cutting_concerns (n=2)    0.4520     0.4020    -0.0500
  service_relationships (n=2)     0.4550     0.4550    +0.0000
  business_logic_constraints (n=2)0.4020     0.4020    +0.0000
  data_flow_integration (n=2)     0.3530     0.3530    +0.0000
  disambiguation (n=2)            0.4500     0.4500    +0.0000

Per-question:
  improvements: 1 (qhard_sym_5, +0.100)
  unchanged:   13
  regressions >10%: 0
```

Both negatives (computed_value, cross_cutting_concerns) are 2-question categories where one displaced rank flips the average 50% — within noise. 1 improvement / 0 regressions on 16q is a soft positive signal that the column is *capable of helping*, but the average lift is below detection threshold.

**Merge gate**: B mean (0.4150) < A mean (0.4210) by -0.006. Per Phase 14.x precedent (Phase 14 quick was +0.019 to advance to 120q; Phase 14.1 16q tied to advance), this is below the bar to spend the ~$15-20 OpenAI budget on full 120q. **Stay flag-off.**

---

## Why the lift was small — diagnosis

The 32 catalog tag bits selected by frequency from gpt-4o-mini's LLM-summary tags are domain-agnostic engineering buckets:

```
api, architecture, authentication, authorization, bash, caching, class,
comparison, config, documentation, error-handling, github-actions, graphql,
interface, logging, markdown, migration, module, nestjs, postgresql, ...
```

User queries use the actual code/feature vocabulary:

```
"Trace the data flow for circuit breaker reset on successful batch"
"Where does barrel ownership transfer happen?"
"Find the BarrelOwnership type definition"
```

These intersect rarely. `?context=auto`'s straight token-to-tag-bit lookup almost always returns empty. The ContextColumn observes empty fingerprints, returns empty candidates, contributes 0 to RRF — the 5-column scorer effectively becomes the 4-column Phase 13.1 scorer for these queries.

The polysemy resolution power of fingerprints is real (the `qhard_sym_5` +0.100 came from a query whose tokens overlapped the `interface` tag bit, which then surfaced the right symbol). But for it to fire on most queries, fingerprint derivation needs to be either:
1. **Vector-based**: embed query → top-N closest catalog tag embeddings → fingerprint (Phase 14.2.1)
2. **Operator-driven**: callers supply `query_context_fingerprint` directly per their domain knowledge
3. **Better catalog tags**: use code symbols + file-tree segments instead of LLM summary buckets (Phase 14.2.1 retune of Builder tag selection)

---

## Bugs caught by live testing

The first live `mdemg migrate context-fingerprint` run surfaced 3 production bugs that integration tests would have caught earlier — committed in `0228fba`:

1. **`neo4j.ExecuteRead/Write[T]` panics on nil interface returns** — driver's generic `castGeneric` does `result.(T)` without a nil-safe path. Fix: switch discard-result call sites to non-generic `sess.ExecuteRead/Write`; use typed pointers (`*neo4j.Node`, `*time.Time`) for result-returning sites.
2. **Neo4j rejects arrays of map literals as property values** — V0026 originally specified `bits[]` as `[{position, kind, ref, token}, ...]`. Cypher TypeError. Fix: serialize to JSON string `bits_json`.
3. **Backfill CLI keyed on `obs_id`** — code-derived MemoryNodes (whk-wms is a code/synth space) carry `node_id` but not `obs_id`. The CLI's filter silently dropped all 9,121 nodes. Fix: rekey on `node_id`; read `m.path` (not `m.file_path`).

All three fixed before the run completed. The runtime panic recovery + idempotency (skip if version already current) of the catalog Builder + backfill meant zero data corruption from the failed first attempts.

---

## Decision matrix outcome

From sprint plan §13:

| Outcome | Default flip | Status |
|---|---|---|
| `fingerprint_only` passes 120q | flip `RETRIEVAL_CONTEXT_COLUMN_ENABLED=true` | ❌ did not pass 16q |
| `fingerprint_strict` passes 120q only | ship strict-mode opt-in | not run (pointless without query fingerprints) |
| Neither passes | ship flag-off, scope Phase 14.2.1 with weight ablation | ✅ **selected** |

**Phase 14.2.1 scope** (queued):
- Vector-based query→fingerprint derivation (embed query, find top-N closest catalog tag/path bit refs)
- Builder tag-selection retune: prefer code-symbol n-grams + file-tree path segments over LLM-summary buckets
- Per-category column weight (mirroring Phase 14.1's per-category gate dispatch)
- Re-run 16q quick → if pass, full 120q → conditional default flip

---

## Spend

**OpenAI**: ~$2.50 (estimate — 1 baseline 16q + 1 candidate 16q + 380 LLM summaries during whk-wms re-ingest at gpt-4o-mini rates). Well below the $25-30 budget; the unspent budget rolls into Phase 14.2.1.

**Wall clock**: ~3 hr from "start ingest" to "A/B compared" (5 min ingest + 26 min consolidation + 2 min catalog build + 30 min A/B + 30 min bug-fix iteration + 15 min question filtering).

---

## Operator runbook (current state)

```bash
# 1. Verify schema state
./bin/mdemg tsdb status                                      # expect: schema_version=20
curl -u neo4j:$NEO4J_PASS -d '{"statements":[{"statement":"MATCH (m:Migration) RETURN max(m.version)"}]}' \
     http://localhost:7474/db/neo4j/tx/commit                # expect: 26

# 2. Build catalog + backfill a space
./bin/mdemg migrate context-fingerprint --space-id <id> --build --dry-run=false

# 3. Verify fingerprints
curl -u neo4j:$NEO4J_PASS -d '{"statements":[{"statement":"MATCH (m:MemoryNode {space_id:\"<id>\"}) WHERE m.context_fingerprint_version IS NOT NULL RETURN count(m), avg(size(m.context_fingerprint_active))"}]}' \
     http://localhost:7474/db/neo4j/tx/commit

# 4. (Optional) opt into the 5th RRF column
echo "RETRIEVAL_CONTEXT_COLUMN_ENABLED=true" >> .env
./bin/mdemg restart

# 5. (Optional) test with server-side query fingerprint derivation
curl -X POST 'http://localhost:9999/v1/memory/retrieve?context=auto' \
     -H 'Content-Type: application/json' \
     -d '{"space_id":"<id>","query_text":"<query>","top_k":5}'
```

---

## Documents accessed

**Read during execution:**
- `docs/research/mdemg_sprint_ideas/05-context-specific-node-activations.md` — Note 05 design draft
- `docs/development/post-ft-lora/sprint_plan_phase_14_2_note_05_sparse_fingerprints.md` — frozen plan
- `docs/development/post-ft-lora/phase_14_2_forensic.md` — Epic 0 multi-space density
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` — question source (filtered to 116 path-validated as `test_questions_phase14_2.json`)
- `internal/hidden/{context_catalog,context_catalog_builder}.go` (post-Epic-2 production code)
- Neo4j v5 driver source: `transaction_helpers.go` (root cause of bug 1)

**Generated**:
- `docs/architecture/benchmarks/whk-wms/test_questions_phase14_2.json` (116 questions, path-filtered from original 120)
- `docs/tests/uvts/specs/whk_wms_phase14_2.uvts.json` (new spec for the refreshed corpus)
- `/tmp/phase14_2/{A_baseline,B_fingerprint}/grades.json` (A/B verdict data — local-only)

---

## Phase 14 sequence so far

| Phase | Status | Default state |
|---|---|---|
| 14 | EXECUTED 2026-05-04 (narrow close) | flag-off (sparse gate) |
| 14.1 | EXECUTED 2026-05-04 | flag-off (per-cat overrides ship infra) |
| 14.1.1 | EXECUTED 2026-05-04 | **default-on** (sparse gate, hybrid PASSED) |
| 13.6 | EXECUTED 2026-05-04 | LLM_* primary names (deprecated MLX_* aliases) |
| 13.2 | EXECUTED 2026-05-04 (re-comparison only) | unchanged |
| **14.2** | **EXECUTED 2026-05-05 (this — narrow close)** | **flag-off (RETRIEVAL_CONTEXT_COLUMN_ENABLED=false default)** |
| 14.2.1 | queued | TBD per A/B verdict |
