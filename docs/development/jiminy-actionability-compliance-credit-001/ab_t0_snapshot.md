# A/B t0 Snapshot — pre-flip baseline (ab_recipe Step 1)

**Captured:** 2026-07-21T13:33:09Z (immediately before flipping `JIMINY_NONVIOLATION_CREDIT_ENABLED=true` in `.env`)
**Space:** `mdemg-dev` | **Window:** 7 days

## Actionable outcomes (`constraint_outcomes`, constraint+correction)

| metric | value |
|---|---|
| actionable_total | 482 |
| followed | 48 |
| **follow_pct** | **9.96%** |

## LLM classifier verdicts (`llm_interactions.jiminy.evaluate_llm`)

| verdict | count |
|---|---|
| not_applicable | 89 |
| ignored | 549 |
| followed | 96 |
| contradicted | 8 |
| **total** | **781** |

## Re-measure

Run ab_recipe Step 4 on/after **2026-07-24** (T+3d) with a 3-day window. Success band: actionable follow rate 18-25%; NA emissions +~50%; actionable volume -~40%. Tripwires: follow rate >50% (over-crediting) or contradicted -30% → revert.
