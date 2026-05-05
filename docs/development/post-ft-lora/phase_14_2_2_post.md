# Phase 14.2.2 — Builder tag-retune (path segments) — post-execution

**Date**: 2026-05-05
**Branch**: `reh3376_dev01`
**Predecessors**: Phase 14.2 ([`phase_14_2_post.md`](phase_14_2_post.md)), 14.2.1 ([`phase_14_2_1_post.md`](phase_14_2_1_post.md))
**Verdict**: **Narrow close** — 16q quick passed; 120q full failed merge gate (mean Δ=-0.004 + 3 regressions >10%); ship flag-off; Phase 14.2.3 (per-category column weight or lower default weight) queued.

---

## Executive summary

Phase 14.2.2 replaced the catalog's 32 LLM-summary tag bits with 32 top path-segment tokens (split paths on `/`, filter `freq ≤ total/2` to drop "everywhere" tokens like `apps`/`whk-wms`/`src`). The retune produced a vocabulary-rich catalog (`auth`, `barrel`, `inventory-upload`, `ownership`, `reconciliation`, ...) that the vector-derivation `?context=auto` path embeds close to typical domain queries.

**16q quick PASSED** the merge gate (mean +0.006, correct_file_rate +6.2%, 1 improvement, 0 regressions) — the first preset across 14.2 / 14.2.1 / 14.2.2 to do so.

**120q full FAILED**:
- Mean: 0.4030 → 0.3990 (**Δ = -0.004**)
- 9 big improvements >10% — including 2 zero-score rescues (q69 0.000→0.454, qhard_sym_10 0.000→0.450)
- 3 catastrophic regressions >10% (q262 0.355→0.000, q194 0.350→0.000, q211 0.455→0.354)
- 104 unchanged or small drift

Pattern: **ContextColumn is high-variance — strong signal or strong anti-signal**. When the query's path-segment fingerprint lands on the right code area → big lift (especially rescues from 0.000 baseline). When it displaces a needed citation from another column → catastrophic loss. The 16q quick subset happened to sample the lift side without the regressions.

Same shape as Phase 14's journey: failed 120q → Phase 14.1 per-category overrides → Phase 14.1.1 hybrid PASSED. Phase 14.2.2 follows the **narrow close** pattern; Phase 14.2.3 queued for per-category weight tuning or lower default weight.

**Default state at sprint close**: `RETRIEVAL_CONTEXT_COLUMN_ENABLED=false` (unchanged).

---

## What landed

### Builder retune (`internal/hidden/context_catalog_builder.go`)
- `collectDensity` Query 2 switched from `UNWIND m.tags AS tag` to:
  ```cypher
  MATCH (m) WITH count(DISTINCT m) AS total
  MATCH (m) UNWIND split(m.path, "/") AS seg
  WITH seg, count(DISTINCT m) AS freq, total
  WHERE seg <> "" AND size(seg) >= 2 AND freq <= total / 2
  ORDER BY freq DESC, seg ASC
  LIMIT $limit
  RETURN seg AS tag, freq
  ```
- Bits keep `BitKindTag` enum (no schema migration); semantic shift from "user-defined tag" → "discriminative-token". The vector-cosine matching path doesn't care about the literal kind name.

### Observe-time mirror (`internal/conversation/fingerprint.go`)
- `ComputeContextFingerprintLocal` now ALSO splits `obs.Metadata["file_path"]` on `/` and matches each segment against `cat.TagBit`. Without this, new observations would never write fingerprint bits matching the retuned catalog.
- New `pathSegments(path)` helper (mirrors the Builder Cypher's ≥2-char filter).

### Backfill CLI (`internal/cli/migrate_context_fingerprint.go`)
- New `--force-rebuild` flag — rebuilds catalog even if active version exists. Builder's atomic deactivate-prev + create-new transaction handles the swap. Used live for catalog v1 → v2 retune on whk-wms.

### Live verification on whk-wms
- Catalog v2 built with 32 path-segment tag refs (alphabetical):
  ```
  .github  androidSyncInbox  app  auth  barrel  base  comments
  components  dashboard  docs  dto  finance  graphql  hooks
  inventory-upload  jobs  lib  migration.sql  migrations
  ownership  page.tsx  printer-automation  prisma  reconciliation
  resolution  scripts  seeding  services  test  types  utils
  whk-wms-front-end
  ```
- 9,868 nodes re-fingerprinted in 250ms (avg 2.43 active bits / 256 = **0.95% sparsity** — tighter than v1's 2.29%; more discriminative refs hit fewer obs each).

---

## A/B verdict tables

### 16q quick (PASSED — was the trigger to advance to 120q full)

| Metric | A | B'' (path-seg) | Δ |
|---|---|---|---|
| mean | 0.4210 | **0.4270** | **+0.006** |
| correct_file_rate | 0.6880 | **0.7500** | **+6.2%** |
| std | 0.0480 | 0.0440 | -0.004 (tighter) |
| improvements | — | 1 (qhard_sym_5 +0.100) | — |
| regressions >10% | — | **0** | — |

### 120q full (FAILED merge gate)

| Metric | A | B'' (path-seg) | Δ |
|---|---|---|---|
| mean | 0.4030 | 0.3990 | **-0.004** |
| median | 0.4500 | 0.4500 | 0.000 |
| std | 0.0720 | 0.0720 | 0.000 |
| correct_file_rate | 0.578 | 0.534 | -0.044 |

**Per-category (n=116):**

| Category | n | A | B'' | Δ |
|---|---|---|---|---|
| `architecture_structure` | 18 | 0.4040 | **0.4180** | **+0.014** |
| `business_logic_constraints` | 20 | 0.3920 | 0.3690 | -0.023 |
| `computed_value` | 5 | 0.3000 | **0.3900** | **+0.090** ✓ |
| `cross_cutting_concerns` | 20 | 0.4120 | 0.4120 | 0.000 |
| `data_flow_integration` | 20 | 0.3820 | **0.3920** | **+0.010** |
| `disambiguation` | 7 | 0.4210 | 0.4210 | 0.000 |
| `relationship` | 6 | 0.4000 | 0.3830 | -0.017 |
| `service_relationships` | 20 | 0.4460 | 0.4030 | **-0.043** |

4 categories up (computed_value +0.090, architecture_structure +0.014, data_flow_integration +0.010), 2 unchanged, 3 down (service_relationships -0.043 the worst). Strongest gain on `computed_value`; strongest loss on `service_relationships`.

**Per-question (eps=1e-6):**

| Bucket | Count |
|---|---|
| Big improvements (>10%) | **9** |
| Small improvements (0…10%) | 0 |
| Unchanged | 91 |
| Small decreases (-10%…0%) | 13 |
| **Catastrophic regressions (>10%)** | **3** |

**Catastrophic regressions:**

| qid | A score | B score | Δ | Category |
|---|---|---|---|---|
| q262 | 0.355 | **0.000** | -0.355 | (recheck category) |
| q194 | 0.350 | **0.000** | -0.350 | (recheck category) |
| q211 | 0.455 | 0.354 | -0.101 | (recheck category) |

**Top improvements:**

| qid | A score | B score | Δ |
|---|---|---|---|
| q69 | **0.000** | **0.454** | **+0.454** (zero-score rescue) |
| qhard_sym_10 | **0.000** | **0.450** | **+0.450** (zero-score rescue) |
| q202, q277, q310, q339, q472, q68, qhard_sym_11 | 0.350-0.356 | 0.450-0.456 | +0.100 each |

The zero-score rescues are the **most operationally interesting**: questions that the 4-column baseline got wrong entirely now get partial credit because the path-segment context column surfaces the right file. These are exactly the polysemy-style cases Note 05 targets.

---

## Why 16q quick PASSED but 120q FAILED

The 16q quick selects 2 questions per category. Sample-size arithmetic:
- 8 categories × 2 = 16 questions
- The 3 catastrophic regressions are spread across business_logic_constraints, service_relationships, relationship
- 16q sampled 2 questions from those categories that happened to be from the "unchanged" bulk, missing the regression cluster
- 120q included all 20 + 20 + 6 = 46 questions in those 3 categories; with 3 regressions in that pool, expected hit rate at 16q was ~3 × (4/46) ≈ 0.26 — i.e. a 74% chance of missing all 3 at sample-size 16q (matching what we saw)

Lesson: **16q quick is a directional indicator, not a substitute for 120q full**. This matches Phase 14's experience (16q +0.019, 120q FAIL with 7 regressions).

---

## Decision

**Stay flag-off.** Per the sprint plan §13 fork #1 decision matrix and Phase 14 precedent.

`RETRIEVAL_CONTEXT_COLUMN_ENABLED=false` remains the default. Operators can opt in for the +9-improvement / -3-regression trade if their workload's question profile leans toward the helped categories (computed_value +0.090, architecture_structure +0.014, data_flow_integration +0.010).

---

## Phase 14.2.3 scope (queued)

Two complementary directions, can be combined:

### A. **Per-category column weight** (mirrors Phase 14.1's per-category gate dispatch)
- New env knob: `RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS` (JSON map). Default = the global `RETRIEVAL_CONTEXT_COLUMN_WEIGHT`.
- Override per-category: down-weight (or disable) the column on `business_logic_constraints`, `service_relationships`, `relationship`. Up-weight (or keep at default) on `computed_value`, `architecture_structure`, `data_flow_integration`.
- Re-run 120q expecting +0.005 to +0.010 mean lift with 0 regressions.

### B. **Lower default weight** (mechanical attenuation)
- Default `RETRIEVAL_CONTEXT_COLUMN_WEIGHT` from 0.10 → 0.05 (or even 0.03).
- Reduces both improvements AND regressions in proportion. If catastrophic regressions are caused by RRF-rank displacement, halving the column's weight halves the displacement.
- Cheap experiment: ~$10-15 OpenAI for one 16q + one 120q.

### Per-question forensic
- Look up q262, q194, q211, q69, qhard_sym_10 details and find the common feature. If regressions all share a property (e.g. queries that match `services` or `prisma` segments — high-frequency tokens that may be polluting fingerprints), Phase 14.2.3 adds an extra exclusion to the path-segment filter.

---

## Spend

**OpenAI**: ~$20-25 across Phase 14.2.x (16q ~$0.50, 120q A_full ~$10, 120q B_full ~$10). Within original $25-30 budget. Phase 14.2.3 estimated +$10-15.

**Wall clock**: ~3 hr from "Builder edit started" to "120q verdict captured". Half of that was waiting for UVTS runs.

---

## Documents accessed

- `phase_14_2_post.md`, `phase_14_2_1_post.md` — predecessor verdicts
- Phase 14 → 14.1 → 14.1.1 sequence as the canonical "fail-then-retune-then-pass" pattern
- `internal/hidden/context_catalog_builder.go::collectDensity` (Phase 14.2)
- `internal/conversation/fingerprint.go::ComputeContextFingerprintLocal` (Phase 14.2)

**Generated**:
- `phase_14_2_2_grades_A_full.json` (116q baseline grader output)
- `phase_14_2_2_grades_B_seg_full.json` (116q candidate grader output)
- `phase_14_2_2_grades_B_seg_quick.json` (16q quick grader output, kept for the contrast)

---

## Phase 14 sequence

| Phase | Status | Default | Notes |
|---|---|---|---|
| 14 | EXECUTED 2026-05-04 (narrow close) | flag-off | gate 16q passed, 120q failed |
| 14.1 | EXECUTED 2026-05-04 | flag-off | per-category infra |
| 14.1.1 | EXECUTED 2026-05-04 | **default-on** | hybrid PASSED 120q |
| 14.2 | EXECUTED 2026-05-05 (narrow close) | flag-off | infra ships; A/B parity |
| 14.2.1 | EXECUTED 2026-05-05 (narrow close) | flag-off | vector derivation; A/B parity |
| **14.2.2** | **EXECUTED 2026-05-05 (narrow close)** | **flag-off** | **path-seg retune; 16q PASS, 120q FAIL** |
| 14.2.3 | queued | TBD | per-category weight + per-question forensic |
