---
created: 2026-05-04
updated: 2026-05-04
status: phase 14.1 epic 0 output
phase: POST-FT-LORA-PHASE14.1 Epic 0
predecessor: Phase 14 narrow close (commit e17a2b5)
---

# Phase 14.1 Epic 0 — Forensic Re-Confirmation

> Re-confirms Phase 14 Epic 2's per-category regression diagnosis from independent inputs (UVTS spec questions + per-question complexity tags + V0019 telemetry). Validates the override design before Epic 1 implements it.

## TL;DR

The 7 Phase 14 boundary regressions concentrate in **4 categories**, but the regression rate is **above the noise floor in 2 of them** (`architecture_structure` 3/20, `data_flow_integration` 2/20). The other two (`business_logic_constraints` 1/20, `cross_cutting_concerns` 1/20) sit within statistical noise.

**Recommended override map** (3 presets to A/B in Epic 3, ordered conservative → aggressive):

```bash
# Preset A — Conservative (Epic 3 starting point)
SPARSE_GATE_CATEGORY_OVERRIDES='{"architecture_structure":{"min_active":20}}'

# Preset B — Moderate
SPARSE_GATE_CATEGORY_OVERRIDES='{"architecture_structure":{"min_active":20},"data_flow_integration":{"min_active":15}}'

# Preset C — Aggressive (cover all 4 affected categories)
SPARSE_GATE_CATEGORY_OVERRIDES='{"architecture_structure":{"min_active":20},"data_flow_integration":{"min_active":15},"business_logic_constraints":{"min_active":15},"cross_cutting_concerns":{"min_active":15}}'
```

Effective rerank-input reduction across all categories:
- Preset A: ~33% reduction (arch_struct passes through, 7 other cats gate to MIN=10)
- Preset B: ~25% reduction (2 cats permissive, 6 cats gate)
- Preset C: ~17% reduction (4 cats permissive, 4 cats gate)

Phase 14's gate-off baseline ships 20 candidates to rerank (0% reduction). A passing Preset A still gives ~6.7 candidates per call instead of 3 — a meaningful win even before factoring in any quality lift.

## Section 1 — Phase 14 regression categorical breakdown

The 7 regressions from `phase14_epic2_full/sparse-p95-min10/verdict.json`:

| q_id | category | complexity | required_files | delta |
|---|---|---|---|---|
| 66 | architecture_structure | multi-file | 2 | -0.10 |
| 69 | architecture_structure | cross-module | 2 | -0.10 |
| 77 | architecture_structure | system-wide | 2 | -0.10 |
| 211 | business_logic_constraints | multi-file | 3 | -0.10 |
| 308 | data_flow_integration | multi-file | 2 | -0.10 |
| 316 | data_flow_integration | multi-file | 2 | -0.10 |
| 464 | cross_cutting_concerns | cross-module | 2 | -0.10 |

**Rate per category** (out of n questions in 120q profile):

| Category | n | Regressions | Rate | Decision |
|---|---|---|---|---|
| **architecture_structure** | 20 | **3** | **15%** | Override needed |
| **data_flow_integration** | 20 | **2** | **10%** | Override likely needed (above noise) |
| business_logic_constraints | 20 | 1 | 5% | Within noise; override optional |
| cross_cutting_concerns | 20 | 1 | 5% | Within noise; override optional |
| service_relationships | 20 | 0 | 0% | No override needed |
| disambiguation | 8 | 0 | 0% | No override needed |
| computed_value | 6 | 0 | 0% | No override needed |
| relationship | 6 | 0 | 0% | No override needed |

**Pattern**: every regression has `required_files ≥ 2` (multi-file/cross-module/system-wide). But not every multi-file question regresses — `service_relationships` is **75% multi-file / 25% cross-module** with **0 regressions**. Complexity alone doesn't explain the pattern.

## Section 2 — Why some multi-file categories regress and others don't

Spot-check of regressed questions vs unregressed `service_relationships`:

| Sample | Question | Pattern |
|---|---|---|
| **q77** (arch, 3 regressions) | "What is the architecture pattern for handling device sync errors across multiple modules?" | **Pattern-shaped**: needs to find ALL files implementing the pattern |
| **q69** (arch) | "How does the secretsManager module integrate with Azure Key Vault for credential management?" | **Integration-shaped**: needs both module + integration target |
| **q308** (data_flow, 2 regressions) | "How does the GraphQL subscription system propagate comment updates? Describe the PubSub pattern and filtering" | **Pattern-shaped**: PubSub + resolver + filter together |
| **q464** (cross_cutting, 1 regression) | "How does the feature flag context support multi-context targeting in LaunchDarkly?" | **Integration-shaped** |
| service_relationships sample | "Which service does the inventoryUpload service depend on for safety limits?" | **Direct-relationship**: A→B; needs only the connecting file |

The differentiator: regressing questions ask about **architectural patterns or integrations** that span multiple files where each file is roughly equally relevant. The candidate ranking spreads them across rank 1–15+; cutting at rank 10 loses some.

`service_relationships` asks about **direct A→B relationships** where the connecting file ranks 1–3 reliably; rank 11–20 candidates are noise.

## Section 3 — Per-category mean active_count (V0019 production telemetry)

V0019 was wiped during the reverse-migration test on 2026-05-04 (Task #337). Only 4 fresh post-test rows are available — insufficient for category-level distribution analysis. Phase 14.1 Epic 3 will land enough rows for Phase 14.1.1 retune if the override map needs further tuning.

In the absence of V0019 data, Epic 0 falls back on the Phase 14 Epic 2 verdict + UVTS spec analysis (above) as the primary input.

## Section 4 — Override design choices

### Why MIN=20 for architecture_structure (not 15 or 18)

`architecture_structure` had 3 regressions in 20 questions — 15% rate. Complexity is `multi-file (10) / cross-module (9) / system-wide (1)`. The 3 regressing questions (q66, q69, q77) all have 2 required_files; the right files apparently span rank 11–20 in the candidate list with `MIN=10`.

At input K=20, MIN_ACTIVE=20 is effectively no-op for this category — full candidate set passes through to rerank. This is the safest design: it eliminates the regression risk completely while preserving the gate's benefit on the other 7 categories.

Alternative: MIN=15 would cut rank 16–20 only. If the 3 regressing questions had right-files at rank 16–20 specifically, this fixes them. If at rank 11–15, this doesn't. Without per-question rank data we can't be certain. MIN=20 is the conservative choice; MIN=15 could be a future tuning if V0019 collects enough rows to support it.

### Why MIN=15 for data_flow_integration (not 20)

`data_flow_integration` had 2 regressions in 20 questions — 10% rate (lower than arch_struct). Complexity profile is similar but the regression rate suggests not every multi-file question in this category needs the full top-20.

MIN=15 keeps a meaningful gate (5 candidates dropped per call → 25% reduction) while preserving the rank 11–15 zone where most data_flow regressions likely sit.

If Preset B (arch=20 + data_flow=15) still produces data_flow regressions, Preset C bumps it to MIN=20 too.

### Why no override for business_logic_constraints / cross_cutting_concerns

1 regression each, in 20 questions = 5% rate. This sits at or below the floating-point boundary noise floor — Phase 14's eps-tolerance fix in `uvts_ab_compare.py` may already eliminate these as false positives in Epic 3's A/B.

Preset C tests the conservative-extreme case (override all 4) to bound the ablation.

## Section 5 — Acceptance bar for Epic 3

Per sprint plan §5 Epic 3, A/B verdicts must use the new eps-tolerance comparator (Epic 2). With eps=1e-6:

- **16q quick × 3 presets vs baseline-sparse-off**: at least one preset must pass (B mean ≥ A AND no per-question regression > 10% with eps tolerance)
- **120q full on quick winner**: same criterion

If Preset A passes 120q → flip default-on with arch_struct override. Win.
If only Preset B/C pass → flip with broader override map.
If none pass → ship overrides flag-off-but-wired; scope Phase 14.1.1 with per-question rank-of-correct-citation telemetry (would require V0019 schema extension).

## Section 6 — Documents accessed

- `/tmp/phase14_epic2_full/sparse-p95-min10/verdict.json` — Phase 14 regression list (primary input)
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` — 120-question category + complexity + required_files data
- `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` — spec wiring
- TSDB V0019 `sparse_gate_metrics` (insufficient data post-reverse-migration; queued for Phase 14.1.1 retune if needed)
- `docs/development/post-ft-lora/phase_14_post.md` — Phase 14 verdict tables
- `docs/development/post-ft-lora/sprint_plan_phase_14_1_*.md` (this sprint's plan)
