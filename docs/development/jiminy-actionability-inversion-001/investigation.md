# JIMINY-ACTIONABILITY-INVERSION-001 — Investigation

**Date:** 2026-07-21
**Method:** direct SQL against `constraint_outcomes` + `llm_interactions` + code-path audit on `mdemg-dev`.

## E1 — Inversion reproduced (stable across 24h/7d/30d)

```
window | guidance_type |  n   | followed | follow_pct
--------|---------------|------|----------|------------
30d    | constraint    | 1648 |      165 |       10.0
30d    | correction    |   59 |       12 |       20.3
30d    | learning      |  596 |       71 |       11.9
30d    | pattern       |  582 |      124 |       21.3
30d    | concept       |   78 |        9 |       11.5
30d    | risk          |   31 |        6 |       19.4

Actionable combined (constraint+correction, 30d):  177/1707 = 10.4%
Advisory combined  (pattern+learning+concept+risk): 210/1287 = 16.3%
```

Inversion **stable and reproducible** — advisory types are followed at ~1.6× the rate of actionable across all window sizes. Not measurement noise.

## E2 — H1: Classifier calibration asymmetry (PARTIALLY CONFIRMED)

Restricting to LLM-classified rows only (n≥10, 30d):

```
guidance_type | classifier | n    | follow%
constraint    | llm        | 1500 | 10.6
constraint    | tier1      |  133 |  3.0
pattern       | llm        |  537 | 21.2
pattern       | tier1      |   32 | 31.3
```

**Inversion holds within a single classifier.** LLM-only: pattern 21.2% ≫ constraint 10.6% (2× ratio). Tier1-only: pattern 31.3% ≫ constraint 3.0% (10× ratio). Tier1 (embedding-similarity short-circuit) is *more* biased against constraints, but LLM shows the same pattern → not a tier1 artifact.

## E3 — H2: 4-band restoration bias (PARTIALLY CONFIRMED, minor)

```
guidance_type | avg_sim | sub_low (<0.20) | sub_low_pct
constraint    |  0.788  |             129 |         7.8
pattern       |  0.810  |              22 |         3.8
learning      |  0.793  |              22 |         3.7
correction    |  0.753  |               7 |        11.3
```

Constraint has ~2× more sub-0.20 rows than pattern/learning. These fall into the `[0.10, 0.20)` band that JIMINY-CORPUS-001 E4 restored to `ignored` (vs the sub-0.10 band which stays `not_applicable`).

⚠️ **Important side-finding**: `not_applicable_pct = 0.0` across ALL types! `constraint_outcomes` never contains `not_applicable` rows. Root cause is deliberate: `internal/jiminy/service.go:1730,1762` gates `not_applicable` OUT of both the Neo4j `PersistGuidanceOutcome` and the TSDB `outcomeWriter` — "topically unrelated items should not create edges or decay confidence." **So the follow-rate denominator ALREADY excludes sub-0.10 `not_applicable` rows**; the constraint 7.8% sub-0.20 band is genuine "borderline-relevant but classified as ignored".

Even excluding that 7.8% band, constraint follow rate is (10.6%/(100-7.8%)) ≈ 11.5% — still ~2× below pattern 21.2%. H2 is a small contributor, not the driver.

## E4 — H3: Lever C surface bias (STRONGLY CONFIRMED — the dominant driver)

```
guidance_type | unique_ids | total_events | events_per_id
constraint    |         73 |         1656 |          22.7
learning      |         57 |          596 |          10.5
pattern       |         59 |          586 |           9.9
correction    |          8 |           62 |           7.8
```

**Constraints surfaced 22.7× per unique ID vs pattern 9.9× — 2.3× more per ID.**

Lever C (constraint-partition biasing, JIMINY-CORPUS-001 E5, `JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED`) actively over-samples the `role_type='constraint'` partition into every retrieval. It surfaces the same ~73 constraints repeatedly, including in contexts where they don't apply. Contexts where they don't apply → the classifier correctly says `ignored` → the denominator inflates.

Advisory types have no equivalent biasing lever; they surface only via base retrieval where they tend to be more contextually relevant to the specific action.

## E5 — H4: Duplication after purge (SAME AS H3)

H4 (duplication) is the same phenomenon as H3 (Lever C over-surfacing). The 22.7 events/ID for constraints IS the duplication signal. Post-purge (140→61 nodes), Lever C surfaces the surviving 61 constraints even more concentratedly. Session cooldown (JIMINY-CORPUS-001 E3) helps but doesn't fully offset Lever C's aggressive top-K.

## E6 — H5: Prompt bias (REFUTED)

`internal/jiminy/outcome_classifier.go:20-49` — the classifier system prompt is **symmetric across guidance types**. It gives one classification rule set applied uniformly. There is no advisory-vs-actionable prompt-level bias.

**However, the prompt does say:**
> "Constraint type (must/must_not = strict compliance expected; should/should_not = flexible)"

This is intentional and CORRECT. Constraint-type items typically use imperative must/must_not language; pattern/learning/risk items typically use flexible should/should_not framing. The LLM correctly applies stricter criteria to strict guidance → more `ignored` verdicts on constraints for actions that "don't violate but also don't apply". This asymmetry is by design, not a bug.

## Root cause verdict

**The actionability inversion is REAL but arises from a correct interaction of three intentional design elements:**

1. **Constraint semantics**: constraints use specific, imperative language ("NEVER commit directly to main"). An action that doesn't touch main isn't credited as "following" — it's judged `ignored` because the LLM correctly reads it as "the agent didn't demonstrate applying the constraint."
2. **Lever C over-surfacing** (`JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED`): actively boosts constraint items into retrieval → 2.3× more surface events per ID than advisory types → denominator inflates with contexts where constraints don't apply.
3. **LLM classifier's correct strict-vs-flexible calibration** (from the system prompt): must-type constraints get judged strictly; should-type advisory gets judged flexibly. Correct semantics, but produces asymmetric rates.

**This is not a bug in any single component. It is emergent behavior.**

## What this means for the dashboard

The **Should-Follow Rate** panel (JIMINY-CORPUS-001 § "should-follow gap") assumes "actionable is what SHOULD be followed" → therefore actionable follow rate ≥ raw follow rate. That premise is broken by the above:
- Actionable rate is genuinely LOWER because of over-surfacing + strict evaluation.
- Advisory rate is genuinely HIGHER because of correct flexible evaluation and less over-surfacing.
- The dashboard's ">90% goal" applied to should-follow is directionally wrong. Real actionable follow rate under current architecture will always be ~half of advisory.

## Fix spec (deferred to a separate sprint)

See `fix_spec.md`. Summary:

1. **Rename/reframe the panel** — "Actionable Compliance Rate" with description explaining the expected floor is lower than raw follow rate under current architecture (single-panel wording change).
2. **Non-violation credit for must_not constraints** — extend the classifier prompt with an explicit rule: "for must_not-type constraints, an action that doesn't violate the constraint AND doesn't have obvious opportunity to violate it should be `not_applicable`, not `ignored`." This routes those rows out of the outcome_writer entirely (per the current OutcomeNotApplicable gate). Would raise constraint follow rate substantially without changing substrate.
3. **Reduce Lever C top-K** — trade some retrieval coverage for less over-surfacing. Requires A/B against retrieval quality. Higher risk.

## Sample size caveats

- Correction (n=59) is too small for stable inference — treated as noise-adjacent.
- Constraint (n=1648-1656) is the dominant actionable class and drives the inversion.
- 30d window is the honest post-fix baseline (excludes pre-2026-06-11 heuristic era).
