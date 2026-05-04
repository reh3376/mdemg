---
created: 2026-05-04
updated: 2026-05-04
status: phase 14.1 executed
phase: POST-FT-LORA-PHASE14.1
predecessor: phase 14 narrow close (commit e17a2b5)
successor: phase 14.1.1 (queued — complexity-based override design)
---

# Phase 14.1 — Executed Truth

> Phase 14.1 shipped per-category override infrastructure flag-off after 120q full A/B failed. The category abstraction is the wrong cut-line for the underlying failure mode (rank-cut for multi-required-file questions). Phase 14.1.1 queued to retry with a complexity / required-files-count abstraction. Phase 14's eps-tolerance fix in `uvts_ab_compare.py` ships and is verified working (regression count dropped from Phase 14's 7 false-positives to 2 real regressions).

## TL;DR

| Goal | Outcome | Reason |
|---|---|---|
| Ship per-category override config + dispatch + tests | **Done** | `SPARSE_GATE_CATEGORY_OVERRIDES` env var + `Category` request field + URL param + UVTS runner injection + 4 new unit tests |
| Ship comparator `eps=1e-6` fix | **Done** | `uvts_ab_compare.py:121` — verified by quick sweep (Phase 14's 7 boundary regressions reduced to 0 in equivalent presets) |
| Pass 16q quick A/B with override | **Done** | 2 of 3 presets passed (arch-only and arch-data-flow tied at mean parity, 0 regressions, 1 improvement each) |
| Pass 120q full A/B and flip default-on | **Failed** | arch-only: mean -0.009, 2 catastrophic regressions (q119 service_relationships -0.45, q333 data_flow_integration -0.35). Both have 3 required_files; the rank-cut at MIN=10 drops cited files |
| Conditional default flip | **NOT applied** | Per Phase 14.1 §10 risk #1: ship flag-off; Phase 14.1.1 scoped |

## What shipped

### 1. Per-category override config

| Path | Notes |
|---|---|
| `internal/config/config.go` | New `SparseGateOverride` struct (pointer fields = "fall back to global"); `SparseGateCategoryOverrides map[string]SparseGateOverride` field; JSON env parser + Validate() bounds checks |
| `internal/retrieval/gate.go` | New `SparseGateCategoryOpts` (cycle-safe mirror); `SparseGateOpts.CategoryOverrides` + `Category`; `resolveCategoryOpts()` helper; `translateCategoryOverrides()` config→gate translator |
| `internal/retrieval/gate_test.go` | 4 new tests: AppliesMatchingCategory, PointerNilFallsBack, EmptyCategoryUsesGlobals, NilMapUsesGlobals, AllFieldsSet |

### 2. Hint plumbing (request → gate)

| Path | Notes |
|---|---|
| `internal/models/models.go` | New `Category string` field on `RetrieveRequest` |
| `internal/api/handlers.go` | `?category=...` URL param; JSON body field takes precedence |
| `internal/retrieval/service.go` | Passes `req.Category` + translated override map into `SparseGateOpts` |
| `docs/tests/uvts/runners/uvts_runner.py` | Injects spec per-question `category` field into retrieve body |

### 3. Comparator `eps=1e-6` fix

| Path | Notes |
|---|---|
| `docs/tests/uvts/runners/uvts_ab_compare.py:121` | `delta < -regression_threshold` → `delta < -(regression_threshold + eps)`. Eliminates floating-point boundary false-positives (Phase 14 had 7) |

### 4. Adaptive ablation runner

| Path | Notes |
|---|---|
| `scripts/phase14_1_adaptive_runner.py` | 4 presets (baseline + 3 override variants conservative→aggressive); same env-mutate→restart→uvts→ab_compare lifecycle as Phase 14 Epic 2 runner |

## A/B verdicts

### 16q quick sweep (Epic 3)

| Preset | n | mean | regressions | improvements | Δmean | verdict |
|---|---|---|---|---|---|---|
| baseline-sparse-off | 16 | 0.4083 | — | — | — | A |
| **adaptive-arch-only** | 16 | 0.4083 | **0** | 1 | **0.000** | **PASS** |
| adaptive-arch-data-flow | 16 | 0.4083 | 0 | 1 | 0.000 | PASS (tied with arch-only) |
| adaptive-all-affected | 16 | 0.4021 | 0 | 1 | -0.006 | fail (mean) |

The eps fix worked: where Phase 14's Epic 2 reported 7 boundary regressions at MIN=10/p95, Phase 14.1's quick sweep at the same MIN=10/p95 now reports 0 regressions across all 3 candidate presets.

Per §13 fork #1 (conservative wins ties), `adaptive-arch-only` advanced to 120q full.

### 120q full sweep (Epic 3)

| Preset | n | mean | regressions ≥0.10 | improvements ≥0.10 | Δmean | verdict |
|---|---|---|---|---|---|---|
| baseline (Phase 13.1 prod) | 120 | 0.4128 | — | — | — | A |
| **adaptive-arch-only** | 120 | 0.4040 | **2** | **6** | **-0.009** | **fail** |

The 2 regressions are catastrophic (not boundary):

| q_id | category | required_files | delta | scoring tiers |
|---|---|---|---|---|
| 119 | service_relationships | 3 | **-0.45** | baseline tier=minimal/matched_files=True/cit=0.1/final=0.45 → candidate tier=none/matched_files=False/cit=0.0/final=0.0 |
| 333 | data_flow_integration | 3 | **-0.35** | baseline tier=minimal/cit=0.0/final=0.35 → candidate tier=none/final=0.0 |

### Per-category 120q breakdown

| Category | n | Baseline | Candidate | Δ | Note |
|---|---|---|---|---|---|
| architecture_structure | 20 | 0.4410 | 0.4310 | -0.010 | Override target — slight improvement vs Phase 14's -0.015 but still net negative |
| business_logic_constraints | 20 | 0.3868 | 0.3968 | +0.010 | Net win |
| cross_cutting_concerns | 20 | 0.4116 | 0.4166 | +0.005 | Mild positive |
| **data_flow_integration** | 20 | 0.3967 | 0.3692 | **-0.028** | NEW worst (q333 -0.35 dragged the average) |
| **service_relationships** | 20 | 0.4359 | 0.4084 | **-0.028** | NEW worst (q119 -0.45) — Phase 14 had ZERO regressions here |
| relationship | 6 | 0.4167 | 0.4000 | -0.017 | Small-n; not material |
| computed_value | 6 | 0.3667 | 0.3667 | 0.000 | Unchanged |
| disambiguation | 8 | 0.4250 | 0.4250 | 0.000 | Unchanged |
| **weighted total** | **120** | **0.4128** | **0.4040** | **-0.009** | fail |

## Diagnosis: category is the wrong abstraction

Phase 14.1's hypothesis was "category-specific overrides fix the rank-11–20 citation-loss." The 120q full data falsifies it:

1. **The override target (`architecture_structure`) showed mild improvement** (-0.015 → -0.010) but didn't go positive
2. **Two NEW catastrophic regressions appeared in different categories** (`service_relationships`, `data_flow_integration`) where Phase 14 had no regressions
3. **Both NEW regressions have 3 required_files** — vs Phase 14's regressions which were mostly 2 required_files

The per-category abstraction misses the actual signal: **questions whose right answer requires N files are harder to gate when N≥3**. The candidate ranking spreads N relevant files across rank 1–N+M; cutting at rank 10 risks losing one of the N when M is small. q119 (3 files in `service_relationships`) failed because the 3 service files spread across rank 6–14 in the original ranking; gate at MIN=10 cut one.

**Phase 14.1.1 scope** (queued):
- New override signal: `required_files` count or pre-computed complexity hint
- Implementation: spec-time complexity tag → `?complexity=multi-file-3+` URL param → gate uses MIN=20 for complexity tags above a threshold
- A/B vs Phase 13.1 baseline expected to clean up both q119 and q333

## OpenAI spend (actual)

| Run | Cost estimate |
|---|---|
| 16q quick × 4 presets | ~$2.40 |
| 120q full × 1 (arch-only) | ~$10 |
| **Total** | **~$12.40** |

Well under sprint $15-25 budget.

## Decision-fork outcomes (sprint plan §13)

| # | Fork | Provisional | Outcome |
|---|---|---|---|
| 1 | Override scope | per-category | Per-category SHIPPED but failed 120q. Phase 14.1.1 will try complexity-based |
| 2 | Override env shape | JSON env | Confirmed; lint clean; works end-to-end |
| 3 | architecture_structure MIN value | 20 | Confirmed by quick. 120q showed mild lift on target category but lost ground elsewhere |
| 4 | Default flip on quick alone | NO — 120q required | Held; quick passed but 120q failed → no flip |

## Documents accessed

- `docs/development/post-ft-lora/phase_14_1_forensic.md` (Epic 0 output)
- `docs/development/post-ft-lora/sprint_plan_phase_14_1_*.md` (frozen plan)
- `/tmp/phase14_epic2_full/sparse-p95-min10/verdict.json` (Phase 14 7-regression list)
- `/tmp/phase14_1_quick/*/verdict.json` (Epic 3 quick sweep)
- `/tmp/phase14_1_full/adaptive-arch-only/verdict.json` (Epic 3 full A/B)
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` (per-question complexity + required_files)
- `internal/retrieval/gate.go`, `gate_test.go`
- `internal/config/config.go`
- `internal/api/handlers.go`
- `internal/models/models.go`
- `docs/tests/uvts/runners/uvts_ab_compare.py:121` (eps fix)
- `docs/tests/uvts/runners/uvts_runner.py` (category injection)
- `scripts/phase14_1_adaptive_runner.py` (new ablation runner)
