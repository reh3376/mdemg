---
created: 2026-05-04
updated: 2026-05-04
status: phase 13.2 closed (path A — artifacts confirmed)
phase: POST-FT-LORA-PHASE13.2
predecessor: phase 13.1 (commit 6ed411e)
---

# Phase 13.2 — Executed Truth (Path A: Artifacts Confirmed)

> Phase 13.2 followed the Path A plan: re-run Phase 13.1's existing 120q A/B with the new eps=1e-6 comparator (shipped in Phase 14.1) to confirm whether the 2 reported `business_logic_constraints` boundary regressions were real quality losses or floating-point artifacts. **Confirmed artifacts.** With eps tolerance applied, regression count goes from 2 → 0; mean +0.023 and 30 improvements unchanged. Phase 13.2 closes as documentation update; no code change needed; no Phase 13.2.1 escalation required.

## TL;DR

| | Phase 13.1 (strict comparator) | Phase 13.2 re-check (eps comparator) |
|---|---|---|
| baseline_mean | 0.3900 | 0.3900 |
| candidate_mean | 0.4130 | 0.4130 |
| mean_delta | +0.023 | +0.023 |
| regression_count | **2** | **0** |
| improvement_count | 30 | 30 |
| verdict | pass | pass |
| Per-question gate | passed (mean +0.023 ≫ noise; the 2 regressions were not strictly above threshold) | passed |

The 2 Phase 13.1 boundary regressions in `business_logic_constraints` had `delta = -0.10000000…ε` (display-rounded to -0.10) — floating-point artifacts from `0.45 - 0.35` arithmetic. The new eps tolerance correctly admits them as not-regressions.

## Why this matters

Three operational benefits:
1. Phase 13.1's "ship default-on with 2 caveats" can be retroactively re-stated as "ship default-on cleanly" — no caveats. The Phase 13.1 win is unambiguous.
2. Future operators reading `phase_13_1_post.md` no longer need to mentally discount the boundary regressions. They were never real.
3. Phase 13.2.1 (per-category column-weight tuning for `business_logic_constraints`) is **not needed**. The category never had a real weight problem.

## What shipped

This is a documentation-only sprint. No code change.

| Path | Update |
|---|---|
| `docs/development/post-ft-lora/phase_13_1_post.md` | Annotate the "2 boundary regressions" callout to note they were eps-artifacts (eps fix landed in Phase 14.1) |
| `CHANGELOG.md` | Add a `### Changed` entry noting the retroactive recategorization |
| `docs/development/post-ft-lora/phase_13_2_post.md` | This doc |
| `docs/development/post-ft-lora/sprint_plan_phase_13_2_per_category_weight_tuning.md` | Mark Path A executed; Path B unused (queued for hypothetical future need) |

## OpenAI spend

**$0**. Path A re-runs the comparator on existing grades.json files; no LLM calls.

## Memory observation

A learning observation captured: "Phase 13.1's 2 'business_logic_constraints regressions' were floating-point artifacts. eps=1e-6 fix (Phase 14.1 Epic 2) eliminates them. Phase 13.2 closes; no per-category column-weight tune needed."

## Documents accessed

- `/tmp/phase13_1_full/baseline/grades.json` (legacy linear scorer 120q)
- `/tmp/phase13_1_full/candidate/grades.json` (Phase 13.1 embedding-heavy 120q)
- `/tmp/phase13_1_full/verdict_eps_recheck.json` (this re-check verdict)
- `docs/tests/uvts/runners/uvts_ab_compare.py` (eps fix at line 121)
- `docs/development/post-ft-lora/phase_13_1_post.md` (the retroactively-clarified post)
- `docs/development/post-ft-lora/sprint_plan_phase_13_2_per_category_weight_tuning.md` (path A approval)
