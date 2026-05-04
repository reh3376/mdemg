# Sprint POST-FT-LORA-PHASE14.1 — Adaptive Per-Category Sparse Gate

> **STUB — scoped 2026-05-04** as a follow-up to Phase 14 narrow close. Phase 14 shipped Note 06 sparse gate flag-off after 120q full A/B failed per-question on `architecture_structure` (3 of 7 boundary regressions). This sprint retunes the gate with category-aware MIN_ACTIVE / MAX_ACTIVE / percentile and re-runs the A/B with the goal of flipping default-on.

## Context

Phase 14 Epic 2 verdict: 16q quick PASSED at MIN=10/p95 (mean +0.019, 0 regressions, 3 improvements). 120q full FAILED per-question (mean parity 0.413=0.413, 7 boundary regressions, 7 offsetting improvements).

Per-category breakdown of the 120q full result (from `phase_14_post.md`):

| Category | n | Δ | Regressions | Pattern |
|---|---|---|---|---|
| **architecture_structure** | 20 | **−0.015** | **3** | Right citation lives at rank 11–20 — gate cuts it |
| business_logic_constraints | 20 | +0.010 | 1 | Net win |
| relationship | 6 | +0.017 | 0 | Net win |
| data_flow_integration | 20 | −0.005 | 2 | Cancels |
| cross_cutting_concerns | 20 | 0 | 1 | Cancels |
| (others) | varies | 0 | 0 | Unchanged |
| **total** | **120** | **0.000** | **7** | — |

## Hypothesis

The gate's failure mode is **structurally category-specific**: queries that need rank 11–20 citations belong predominantly to `architecture_structure` (struct + file lookups deep in the graph). A category-aware MIN_ACTIVE — keep top-3 for diffuse categories, top-20 for `architecture_structure` — should preserve the 50%-prompt-bloat-reduction win on most categories while eliminating the per-question regressions on the affected one.

## Scope

| # | Deliverable | Path |
|---|---|---|
| 1 | Per-category override config | `internal/config/config.go` — new `SPARSE_GATE_CATEGORY_OVERRIDES` JSON env (e.g. `{"architecture_structure": {"min_active": 20, "percentile": 0.92}}`) |
| 2 | Category resolution | `internal/retrieval/gate.go` — new `SparseGateOpts.CategoryOverrides` field + apply override when query category known. Categorization comes from the spec's per-question `category` field via `SparseQueryHints.Category` carried on the request |
| 3 | Hint plumbing | `internal/api/handlers.go` + `internal/models/models.go` — `category` request field, populated from spec by uvts_runner |
| 4 | `eps` tolerance in A/B comparator | `docs/tests/uvts/runners/uvts_ab_compare.py` — fix the floating-point boundary issue that produced 7 false-positive regressions in Phase 14 Epic 2 |
| 5 | Phase 14 Epic 2 sweep re-run with adaptive overrides | `scripts/phase14_1_adaptive_runner.py` |
| 6 | Conditional default flip if A/B passes | `RetrievalSparseEnabled` `false → true` in same commit |
| 7 | Tier 1 unit tests + Tier 3 live A/B | Standard |
| 8 | Docs | sprint plan (this) + post-doc + update `sparse-retrieval.md` |

## Estimate

~3 dev-days. Gate code is already in place; this sprint adds category dispatch + retune + re-A/B.

## Budget

~$15 OpenAI for one full 120q A/B + a few quick sweeps. Same scale as Phase 14 Epic 2.

## Pre-gate

Phase 14 commit landed; V0019 has accumulated production rows for retune analysis.

## Acceptance

A/B verdict on adaptive overrides:
- 16q quick PASS (mean ≥ A, no per-question regression > 10%)
- 120q full PASS (same criterion, with `eps` tolerance applied to comparator)
- `architecture_structure` per-category mean ≥ baseline within noise

If passes → flip `SPARSE_RETRIEVAL_ENABLED=true` default, ship default-on.
If fails → ship category-overrides flag-off-but-wired, scope Phase 14.1.1 (queued).

## Documents Accessed (during planning)

- `docs/development/post-ft-lora/phase_14_post.md` (Epic 2 verdict tables)
- `docs/development/post-ft-lora/phase_14_score_distribution_analysis.md` (Epic 0 forensic)
- `docs/features/sparse-retrieval.md`
- `internal/retrieval/gate.go`, `gate_test.go`
- `docs/tests/uvts/runners/uvts_ab_compare.py`
- `/tmp/phase14_epic2_full/sparse-p95-min10/verdict.json` (the regression set)
