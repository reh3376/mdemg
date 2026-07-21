# JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 — E1 Baseline Stats

**Date:** 2026-07-21
**Space:** `mdemg-dev`
**Window:** last 7 days

## Actionable outcome distribution (`constraint_outcomes`)

```
guidance_type | outcome_type       |  n
---------------|--------------------|-----
constraint    | contradicted       |   3
constraint    | followed           |  31
constraint    | ignored            | 311
constraint    | partial_compliance |  10
correction    | contradicted       |   1
correction    | followed           |  15
correction    | ignored            |  79
correction    | partial_compliance |   1
```

**Actionable follow rate baseline: 10.20% (46/451).**
- Constraint follow rate: 31/355 = 8.73%
- Correction follow rate: 15/96 = 15.63%

## LLM classifier verdict distribution (`llm_interactions.jiminy.evaluate_llm`)

```
total_classifications:  701
followed:                88 (12.6%)
ignored:                503 (71.8%)
not_applicable:          83 (11.8%)  ← already gated OUT of constraint_outcomes
contradicted:             8 ( 1.1%)
partial_compliance:       ~19 (2.7%)
```

**Note the 83 `not_applicable` verdicts already flowing at the classifier level** — those are correctly filtered out by `service.go:1730,1762` before reaching `constraint_outcomes`. The 503 `ignored` verdicts are what feed the inflated actionable denominator.

## Predicted post-fix impact

If Fix 2 (non-violation-credit for must_not) routes ~50% of current `ignored` verdicts on constraints to `not_applicable`:

- ~250 rows moved out of `constraint_outcomes` denominator
- Actionable follow rate: 46 / (451 - 250) = 46 / 201 = **~22.9%**

This matches the JIMINY-ACTIONABILITY-INVERSION-001 fix_spec.md predicted lift (10% → ~20%). The exact multiplier depends on how selectively the LLM applies the new rule; A/B measurement over 3-day window will pin the real value.

## Cross-check: 30d window (broader baseline)

```
guidance_type | followed | total | rate
--------------|----------|-------|------
constraint    |      165 |  1648 |  10.0%
correction    |       12 |    59 |  20.3%
Actionable    |      177 |  1707 |  10.4%
```

Consistent — ~10% actionable follow rate is the stable steady-state baseline.

## Success criteria for the operator's 3-day A/B

- **Actionable follow rate lifts to 18-25%** (predicted 22.9%, band accounts for uncertainty)
- **`not_applicable` emissions rise ~50%** (from ~85/wk → ~130/wk baseline scale)
- **Total actionable-outcome volume in `constraint_outcomes` drops ~40%** (fewer rows land because more filter out)
- **No follow-rate spike to unrealistic-good (>50%)** — that would signal over-crediting

If the observed lift is <5pp or the "unrealistic-good" tripwire fires, revert flag.
