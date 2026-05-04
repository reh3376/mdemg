# Sprint POST-FT-LORA-PHASE13.2 — Per-Category Column-Weight Tuning

> **STUB — scoped 2026-05-04** as a follow-up to Phase 13.1 (column-voting default-on with embedding-heavy 0.50/0.20/0.15/0.15). Phase 13.1's 120q full A/B passed with mean +0.023 but had **2 boundary regressions in `business_logic_constraints`** that the eps fix shipped in Phase 14.1 likely now eliminates. This sprint re-runs Phase 13.1's full 120q A/B with the eps comparator to confirm the boundary regressions were artifacts. If they were artifacts, Phase 13.2 is closed as documentation. If real, this sprint extends the `business_logic_constraints`-specific weight tune.

## Context

Phase 13.1 verdict (commit `6ed411e`):
- 120q full: mean **+0.023**, 30 improvements, 2 boundary regressions in `business_logic_constraints`
- Both regressions had `delta=-0.10` exactly (display-rounded; raw float was -0.10000000…ε which tripped the strict `<` check)

Phase 14.1 Epic 2 shipped `eps=1e-6` tolerance in `uvts_ab_compare.py:121`. Re-running Phase 13.1's 120q comparison with the new comparator should produce 0 regressions (per Phase 14.1's verified pattern: same-input quick A/B went from 7 false-positives at MIN=10/p95 to 0).

**Phase chain.** Phase 13 → Phase 13.1 (default-on) → **Phase 13.2 (this — re-verify)** → possible 13.2.1 (real per-category weight tuning if regressions are real).

## Hypothesis

Phase 13.1's 2 boundary regressions are floating-point artifacts, not real quality losses. Re-running with the eps fix produces verdict=PASS with 0 regressions. Phase 13.2 closes as a documentation update + retroactive memory note.

If real, scope Phase 13.2.1 with `RETRIEVAL_COLUMN_WEIGHT_*` per-category overrides (mirrors Phase 14.1's per-category dispatch but on the column-weight side instead of the gate side).

## Scope

**Path A (likely — eps artifacts):**

| # | Deliverable | Path |
|---|---|---|
| 1 | Re-run Phase 13.1 120q A/B with eps comparator | `scripts/phase13_1_ablation_runner.py` (existing) |
| 2 | Update Phase 13.1 post + CHANGELOG to reflect 0-regression outcome | `docs/development/post-ft-lora/phase_13_1_post.md` |
| 3 | CMS observation noting the boundary cases were artifacts | runtime |

**Path B (less likely — real regressions):**

| # | Deliverable | Path |
|---|---|---|
| 1 | Read q-IDs of the 2 business_logic_constraints regressions, inspect failure mode | `phase_13_2_forensic.md` |
| 2 | Per-category column-weight override config (mirror Phase 14.1's gate dispatch) | `internal/config/config.go` + `internal/retrieval/scoring_rrf.go` |
| 3 | Ablation sweep: per-category weight presets | `scripts/phase13_2_runner.py` |
| 4 | A/B + conditional default flip if passes | standard pattern |

## Pre-gate

Phase 14.1 commit landed (eps fix is in `main`); Phase 13.1 baseline grades preserved at `/tmp/phase13_1_full/`.

## Estimate

- Path A: ~0.5 dev-days (just a re-run + doc update)
- Path B: ~2 dev-days (real ablation work)

## Budget

- Path A: ~$10 OpenAI (one 120q full)
- Path B: ~$15 OpenAI (3-preset quick + 1 full)

## Acceptance

If Path A 120q produces 0 regressions: Phase 13.2 closes as artifacts confirmed.
If Path A still produces regressions: switch to Path B, scope 13.2.1.

## Documents Accessed (during planning)

- `docs/development/post-ft-lora/phase_13_1_post.md` (boundary regression detail)
- `docs/tests/uvts/runners/uvts_ab_compare.py` (eps fix at line 121)
- `scripts/phase13_1_ablation_runner.py` (re-run target)
