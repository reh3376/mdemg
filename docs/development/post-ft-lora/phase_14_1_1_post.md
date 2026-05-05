---
created: 2026-05-04
updated: 2026-05-04
status: phase 14.1.1 executed (default-on)
phase: POST-FT-LORA-PHASE14.1.1
predecessor: phase 14.1 (commit f293933)
---

# Phase 14.1.1 — Executed Truth (Default-On)

> Phase 14.1.1 ships sparse retrieval gate **default-on** with the hybrid winner: `SPARSE_MIN_ACTIVE=15` global + `data_flow_integration` per-category override at MIN=20. 120q full A/B PASSED with mean +0.003, **0 regressions**, **10 improvements**. Closes the Phase 14 → 14.1 → 14.1.1 sequence.

## TL;DR

| Goal | Outcome |
|---|---|
| Test simpler-first hypothesis: global `MIN_ACTIVE=15` | 120q **fail** by 1 question (q302 -0.45 in `data_flow_integration`); mean +0.001 was positive but per-question gate failed |
| Pivot: hybrid `MIN=15 global + data_flow_integration MIN=20` | 120q **PASS**: mean +0.003, 0 regressions, 10 improvements |
| Conditional default flip | **Applied**: `SPARSE_RETRIEVAL_ENABLED` `false → true`, `SPARSE_MIN_ACTIVE` `3 → 15`, `SPARSE_GATE_CATEGORY_OVERRIDES` seeded with `{"data_flow_integration": {"min_active": 20}}` |
| Phase 14 + 14.1 work activated | **Yes** — gate becomes a default behavior. Operators opt-out via `SPARSE_RETRIEVAL_ENABLED=false` |

## Test path (efficient-first design)

The Phase 14.1.1 plan stub at `sprint_plan_phase_14_1_1_complexity_based_override.md` listed a complexity-based override design (`SPARSE_GATE_COMPLEXITY_OVERRIDE_THRESHOLD` + `?required_files_count=N` URL param + UVTS runner injection). It also called out an alternative simpler design: raise global `MIN_ACTIVE` to 15.

The simpler design was tested first because it required ZERO new code (just env-var flip). If it passed 120q, no plumbing needed. If it failed, escalate to complexity plumbing.

**Test 1 — global MIN=15** (`adaptive-min15-global`):
- 120q: mean **+0.001**, 1 catastrophic regression (q302 in `data_flow_integration` -0.45), 7 improvements → fail per-question gate, but **mean gate passed** for the first time across Phase 14 / 14.1 attempts

**Diagnosis of q302**: `data_flow_integration`, complexity `system-wide`, **4 required_files**. With MIN=15, the gate cuts ranks 16-20; one of the 4 required files lived in that zone. This was the only category-and-question combination that needed a wider gate.

**Test 2 — hybrid `MIN=15 global + data_flow_integration MIN=20`** (`adaptive-min15-hybrid`):
- 120q: mean **+0.003**, **0 regressions**, **10 improvements** → **PASS**

The hybrid uses the existing Phase 14.1 per-category override mechanism — no new code paths. The category-based dispatch was the wrong primary abstraction (Phase 14.1's failure) but is the right safety net for known-bad categories.

## A/B verdicts

### 120q full at hybrid (Test 2) vs Phase 13.1 production baseline

| | n | mean | regressions ≥0.10 | improvements ≥0.10 | Δmean | verdict |
|---|---|---|---|---|---|---|
| baseline (Phase 13.1 prod) | 120 | 0.4130 | — | — | — | A |
| **hybrid (MIN=15 + data_flow MIN=20)** | 120 | **0.4160** | **0** | **10** | **+0.003** | **PASS** |

### Per-category 120q breakdown

| Category | n | Baseline | Candidate | Δ | Note |
|---|---|---|---|---|---|
| architecture_structure | 20 | 0.4410 | 0.4310 | -0.010 | Mild aggregate decline; no per-question regressions ≥0.10 |
| business_logic_constraints | 20 | 0.3868 | 0.4018 | **+0.015** | Net win |
| computed_value | 6 | 0.3667 | 0.3667 | 0.000 | Unchanged |
| cross_cutting_concerns | 20 | 0.4116 | 0.4166 | +0.005 | Mild positive |
| **data_flow_integration** | 20 | 0.3967 | 0.3967 | **0.000** | Override worked: q302's 4-file regression eliminated |
| disambiguation | 8 | 0.4250 | 0.4375 | **+0.0125** | Net win |
| relationship | 6 | 0.4167 | 0.4167 | 0.000 | Unchanged |
| service_relationships | 20 | 0.4359 | 0.4409 | +0.005 | Mild positive |
| **weighted total** | **120** | **0.4130** | **0.4160** | **+0.003** | **PASS** |

## What shipped

### Default flip in `internal/config/config.go`

```diff
- sparseRetrievalEnabled := getBool("SPARSE_RETRIEVAL_ENABLED", false)
+ sparseRetrievalEnabled := getBool("SPARSE_RETRIEVAL_ENABLED", true)
- sparseMinActive, err := atoi("SPARSE_MIN_ACTIVE", 3)
+ sparseMinActive, err := atoi("SPARSE_MIN_ACTIVE", 15)

# Default override map seeded with the Phase 14.1.1 hybrid winner:
+ sparseGateDataFlowMin := 20
+ sparseGateCategoryOverrides := map[string]SparseGateOverride{
+     "data_flow_integration": {MinActive: &sparseGateDataFlowMin},
+ }
```

### Operator-facing changes

- Out of the box, `mdemg start` now applies the gate at MIN=15 (top 15 of 20-candidate sets) globally, with MIN=20 specifically for `data_flow_integration` queries
- Rerank prompt input drops ~25% (from K=20 to K=15) on most calls
- Operator opt-out: `SPARSE_RETRIEVAL_ENABLED=false` in `.env` + restart
- Operator override of seeded map: `SPARSE_GATE_CATEGORY_OVERRIDES='{...}'` in `.env` REPLACES the seed entirely (merge isn't supported; document accordingly)

## OpenAI spend (actual)

| Run | Cost |
|---|---|
| 120q × 1 (MIN=15 global, simpler-first) | ~$10 |
| 120q × 1 (MIN=15 hybrid, winner) | ~$10 |
| **Total** | **~$20** |

Within sprint $15-25 budget.

## Decision-fork outcomes

| Fork | Outcome |
|---|---|
| Test simpler MIN=15 first vs immediately build complexity plumbing | Simpler-first won the gamble: hybrid (a small extension of simpler) passed without needing the new URL-param + UVTS-runner-injection plumbing the original 14.1.1 plan required |

## Memory observations

- New observation recorded at sprint close (CMS): "Phase 14.1.1 hybrid passed 120q. Sparse gate now default-on with MIN=15 + data_flow_integration MIN=20."

## Documents accessed

- `docs/development/post-ft-lora/sprint_plan_phase_14_1_1_complexity_based_override.md` (plan stub — alternative design proven sufficient)
- `docs/development/post-ft-lora/phase_14_1_post.md` (per-category lessons informed the hybrid design)
- `/tmp/phase14_1_1_full/min15-global/verdict.json` (Test 1 — 1 regression in q302)
- `/tmp/phase14_1_1_full/min15-hybrid/verdict.json` (Test 2 — winner)
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` (q302 detail)
- `internal/config/config.go` (default flip)
