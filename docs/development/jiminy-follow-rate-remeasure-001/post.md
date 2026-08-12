> ⚠️ **TRAJECTORY ANNOTATION (added 2026-08-12 by FRAMING-HYGIENE-SWEEP-001):** the framing in this doc calls the current follow-rate "honest steady state" / "not urgent" / "expected". That framing was **superseded** by the operator directive of 2026-08-11 ("If the main LLM is only following 10-13% of Jiminy's guidance this entire project is a complete failure") — the arc that owns the ≥80% target is [`docs/development/jiminy-ceiling-break-2/`](../jiminy-ceiling-break-2/README.md). Sprint history preserved below for context; do NOT act on the "not urgent" / "by design" conclusions in the body — those conclusions are wrong per the current architectural directive.

---

# JIMINY-FOLLOW-RATE-REMEASURE-001 — Sprint Post

**Date:** 2026-08-08
**Branch:** `reh3376_dev01`
**Type:** Passive measurement / audit (zero code change)

## Summary

Verified the CLAUDE.md-recorded ≈13% honest steady-state prediction for Jiminy follow rate, 9 days after the last member of the ceiling-remediation arc shipped (JIMINY-CORPUS-002 / JIMINY-TIER1-BYPASS-001 / CORRECTION-CODE-GEN-001, all 2026-07-30).

**Measurement: 11.62% follow rate** (188 followed / 1618 actionable outcomes, 7d window on mdemg-dev).

Baseline pre-arc was 11.44% (54/472). Delta: **+0.18 percentage points across 3.4× traffic volume — effectively flat.**

## What we learned

1. The CEILING investigation's **~35-50% predicted lift did not materialize** — missed by ≥25pp. Its own downstream sprint (JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001) had already recorded this in its A/B verdict (12.97% actual vs 18-25% predicted band).
2. The CLAUDE.md **recalibrated ≈13% pin is correct** — actual 11.62% is within 1.4pp / ~1σ noise of it. No CLAUDE.md update needed.
3. **Tier1 bypass is fully deployed** — zero `tier1` rows in the classifier_source breakdown for actionable follow/ignore decisions.
4. **Correction and constraint follow rates are at parity** (11.86% vs 11.52%) — CORRECTION-CODE-GEN-001 delivered attribution, not compliance lift. Attribution ≠ compliance.
5. **Follow rate is stable** — no drift across the 7d window (11.48% → 11.79%).

## Why the predicted lift failed

Per the CLAUDE.md pin from JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001:

> "88% of NA routing landed on advisory guidance outside the actionable denominator (64/534 imperative); followed count scales with surfacing volume (48→24 as volume −48%)."

The context-mismatch credit clause IS routing lots of verdicts to `not_applicable` — but predominantly on advisory guidance which was never in the actionable denominator in the first place. On imperative constraints and corrections (which ARE in the denominator), NA routing is rare. So the shrink-the-denominator mechanism the CEILING §6 recommendation modeled didn't move the actionable rate.

## Where the remaining ceiling lives

**Corpus quality is now the binding constraint.** Every classifier lever the arc could pull has been pulled; the follow rate stabilized at ~12% because a meaningful fraction of the constraint corpus doesn't produce follows for durable reasons (ambiguous imperatives, over-broad rules, rules that genuinely don't apply as often as their embedding-similarity-based surfacing suggests they do).

**Two candidate next sprints** disclosed (neither urgent):

- **HITL-CURATION-003** — corpus-quality lever via operator-graded rule tombstones. Direct impact class. Named in Q4 roadmap §3 #4.
- **JIMINY-CLASSIFIER-CONTEXT-002** — prompt tightening to unblock NA-routing on imperatives (currently under-fires there per the COMPLIANCE-CREDIT verdict). ~1-2d. Small-impact speculative.

## Arch rule pinned (NEW)

⚠️ **Attribution shipments (assigning codes / IDs / labels to graph nodes retroactively) are NOT quality shipments** — they enable correct measurement + downstream analysis, but they don't move follow-rate / compliance metrics. When predicting sprint impact on a downstream metric, distinguish: does this sprint change the SIGNAL the metric measures, or does it change what USERS do? Only the second class moves compliance rates.

This is a corollary to the DASHBOARD-TRUTH class of findings: measurement fidelity ≠ substrate quality. CORRECTION-CODE-GEN-001 was correct + necessary + didn't lift follow rate; nobody should have expected it to.

## Cross-references

- Baseline: `docs/development/jiminy-ceiling-investigation-001/post.md` §2, §6
- Recalibration: CLAUDE.md search "Recalibrated steady-state expectation ≈13%" (from JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001)
- Verdict full: `docs/development/jiminy-follow-rate-remeasure-001/verdict.md`

## Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q4.md` §3, §5
- `docs/development/jiminy-ceiling-investigation-001/post.md`, `sprint_plan.md`
- `CHANGELOG.md` (arc member entries + JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001)
- `internal/tsdb/migrations/011_constraint_outcomes.sql`
- Live queries against `mdemg-dev` TSDB `constraint_outcomes` hypertable
