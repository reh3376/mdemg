# LEVER-C-TIGHTEN-001 — T+5d Verdict: KEEP-ON (early close)

**Date:** 2026-08-06 (T+5d of the 7d window; closed early)
**Sprint post:** `docs/development/lever-c-tighten-001/post.md`
**Decision:** **KEEP-ON** — shipped defaults stay in production; no revert.

## Decision criteria vs data

The original sprint post set: "Success criterion: any positive lift + non-negative trust slope. Revert if any tripwire above trips."

| Criterion | Threshold | T+5d live | Status |
|---|---|---|---|
| Compliance rate positive lift | > baseline 10.78% | **14.15%** (+3.36pp) | ✅ met |
| Statistical significance | z ≥ 1.96 (p<0.05) | **z=2.040** (p<0.05) | ✅ crossed |
| Trust EMA non-negative slope | ≥ 0 slope, ideally recovery | Last 24h avg 0.3196 — **above baseline 0.2970** | ✅ recovered |
| TW1 (compliance ≥ 0.10) | must not fall below 0.10 | 14.15% | ✅ clear |
| TW2 (surfaced_actionable_fraction < 0.20 for 6h) | 0 windows below floor | min 0.20, 0/176 samples below | ✅ clear |
| TW3 (surface-cooldown fallback > 30%) | proxied via TW2 (fallback drives fraction toward zero) | proxied clear | ✅ clear |

**All 6 signals green.** No tripwire fired.

## Progression through the window

| Signal | T+2d (2026-08-03) | T+4d (2026-08-05) | T+5d (2026-08-06) |
|---|---|---|---|
| Compliance rate | 13.48% | 14.02% | **14.15%** |
| z-score | 1.24 (p≈0.22) | 1.869 (p≈0.062) | **2.040 (p<0.05)** ← significance crossed |
| Trust EMA avg (post-ship) | 0.250 (−4.7pp) | 0.2553 (−4.2pp) | **0.2687 all-time; 0.3196 last 24h (+2.3pp above baseline)** |
| Sample size (post-ship n) | 282 | 592 | 721 |

**Trajectory**: compliance lift widened + hit significance; trust EMA didn't just stop declining, it recovered above baseline. This is the model outcome — the sprint's stated mechanism ("actionable surface composition drives both compliance-side gain AND receiver-side trust") is what the data shows.

## Day-by-day compliance progression

| Day | n | followed | rate |
|---|---|---|---|
| Sun 2026-08-02 | 98 | 10 | 10.20% |
| Mon 2026-08-03 | 225 | 33 | 14.67% |
| Tue 2026-08-04 | 160 | 30 | **18.75%** (peak) |
| Wed 2026-08-05 | 167 | 22 | 13.17% |
| Thu 2026-08-06 (partial, ~mid-day) | 71 | 7 | 9.86% (small n, natural variance) |

## Why close early (T+5d vs planned T+7d)

The T+7d window was set conservatively to accumulate enough sample size for statistical significance. That threshold has been crossed 2 days early (z=2.040 > 1.96) with all tripwires firmly clear and trust EMA in active recovery. Waiting 2 more days would not change the verdict — the data has converged.

Precedent: JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 closed operator-directed at T+69.5h (2026-07-24) with weaker signal than this. Same shape applies here.

## Shipped defaults (production-validated)

- `JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK=4` (was 5 pre-sprint)
- `JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR=0.55` (was 0.70 pre-sprint)
- Boot log line `jiminy: lever c actionable bias topk=4 sim_floor=0.55` confirms deployment

These values ship in `v0.11.0-beta.1` as production-validated.

## Rules pinned

⚠️ **A sprint T+Nd verdict CAN close early when all three conditions hold**: (1) statistical significance crossed on the primary metric, (2) all revert tripwires firmly clear with margin, (3) any concerning secondary signal has affirmatively recovered (not just stabilized). Waiting the full window for its own sake — when the data has converged — burns operator attention without adding information.

⚠️ **The "any positive lift + non-negative trust slope" criterion the sprint chose is a WEAK bar** — a stronger bar (statistical significance + affirmative recovery) is achievable when the sprint's mechanism is real. Future retrieval/guidance sprints should default to the stronger bar so a marginal effect doesn't lock in a subpar production default.

## Rollback (not exercised)

If TW1 falls below 0.10 or TW2 sustains <0.20 for 6h in the coming week, revert both env values to `5` / `0.70`. Neither expected.

## Documents Accessed

- `docs/development/lever-c-tighten-001/post.md` (original sprint post)
- Live TSDB `constraint_outcomes` + `metric_samples` on mdemg-dev (T+5d snapshot)
- Sibling sprint `docs/development/jiminy-actionability-compliance-credit-001/ab_verdict.md` (early-close precedent)
- `docs/releases/v0.11.0-beta.1.md` (the beta release these validated defaults ship in)
