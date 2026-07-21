# JIMINY-ACTIONABILITY-INVERSION-001 — Fix Spec

**Date:** 2026-07-21
**Confidence:** HIGH — root cause verified live; each proposed fix has clear expected effect.

## What we found (recap)

The Should-Follow Rate is *lower* than the raw Follow Rate because:
1. Lever C over-surfaces constraints (22.7 events per ID vs 9.9 for advisory types) into contexts where they don't apply.
2. The LLM classifier correctly applies stricter criteria to must/must_not constraints — an action that doesn't violate but also doesn't demonstrate applying the rule is `ignored`, not `followed`.
3. The `constraint_outcomes` table filters out `not_applicable`, but keeps `ignored` — so borderline-relevant surfacings count against the actionable denominator.

This is an emergent property of correct design, not a defect. But the Should-Follow panel's premise ("actionable is what SHOULD be followed → rate should be ≥ raw") is broken.

## Recommendation: 3 fixes, ranked by leverage-to-risk

### Fix 1 — Panel wording / metric reframe (HIGHEST LEVERAGE, ZERO RISK)

Rename **Should-Follow Follow Rate** → **Actionable Compliance Rate**. Update description:

> Actionable compliance rate = fraction of surfaced constraint+correction items whose corresponding action shows evidence of applying the guidance. Under current architecture (Lever C constraint-partition biasing surfaces constraints widely, including in non-applicable contexts) this rate is EXPECTED to be lower than the raw Follow Rate. A raw rate of 15% with an actionable rate of 10% is normal, NOT a defect. The absolute goal is not >90% — it's stable-and-rising trend (measured over ≥14d windows).

Also update the alert rule `guidance_should_follow_rate_low` to a lower threshold (recommend 0.05) OR remove it in favor of a trend-based rule.

**Effort:** 1-2 hours. **Risk:** none. **Expected effect:** operator no longer misreads the panel as a defect signal.

### Fix 2 — Non-violation credit for must_not (HIGH LEVERAGE, LOW RISK)

Extend `classifySystemPrompt` in `internal/jiminy/outcome_classifier.go:20-49` with an explicit rule:

> **Non-violation credit for must_not**: for must_not-type constraints (e.g. "never commit directly to main"), if the action neither violated the constraint nor plausibly had opportunity to violate it (e.g. the action didn't touch the mechanism the constraint applies to), classify as `not_applicable`, NOT `ignored`. Only use `ignored` when the action clearly could and should have applied the constraint.

Expected effect: many current `ignored` rows on constraints like "never commit to main" (when the action is unrelated to git) will re-route to `not_applicable`, which is filtered out of `constraint_outcomes` (per `service.go:1762` gate). Constraint follow-rate denominator shrinks proportionally. Follow rate rises to a level that reflects genuine relevance.

**Predicted impact** (rough): if ~50% of current `ignored` constraint rows are unrelated-context (a conservative estimate given the 22.7 events-per-id density), the constraint follow rate would rise from 10.0% to ~20% — matching advisory. The inversion CLOSES.

**Effort:** small (~1 dev-day). Risk factors:
- Existing golden tests may break — need to add explicit "non-violation for must_not" test cases.
- LLM prompt hash pinned in ULTS spec (`retrieval_intent_translate.ults.json` or `jiminy_evaluate*.ults.json`); update the pinned hash in the same PR (this is the recurring "prompt drift" class caught by ULTS-CI-001).
- Real live A/B before default-flip: compare follow-rate distributions pre/post-prompt-change over ≥7d window.

**Recommended: SHIP as a follow-up sprint.**

### Fix 3 — Reduce Lever C top-K (MEDIUM LEVERAGE, MEDIUM RISK)

Tune `JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK` (currently biases N constraints into every retrieval) down by ~40% to reduce over-surfacing. Reduces the events-per-ID density from 22.7 toward advisory's ~10.

Risk: reduces constraint retrieval coverage. Some legitimate constraint surfacings may be missed. Requires a full UVTS A/B before flipping — mirror the NEURAL-RERANK-QUALITY-AB-001 discipline.

**NOT RECOMMENDED for the near term** — Fix 2 achieves the same denominator reduction without cutting coverage.

## Recommended sprint

**JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001** (small, ~1.5 dev-days):
- E1 sprint plan
- E2 update `classifySystemPrompt` with non-violation credit rule
- E3 Tier-1 tests covering the new rule
- E4 update ULTS spec's `system_prompt_hash` for the classifier task
- E5 live Tier-3 A/B: 3-day window pre/post enable, compare constraint follow-rate + not_applicable rate (which lives in server logs, not TSDB, but is inferrable via `llm_interactions.response`)
- E6 default-flip if A/B shows constraint follow-rate rises ≥5 percentage points AND no drop in genuine constraint compliance detection
- E7 update DASHBOARD-TRUTH-002-shipped "Should-Follow" panel with the corrected framing (Fix 1)
- E8 canonical docs

## Panel-side fix ONLY (if the above sprint is deferred)

If the operator decides Fix 2 isn't worth the effort, ship just Fix 1 as **DASHBOARD-TRUTH-003** or a small doc-only sprint. Zero risk, closes the operator-scannable interpretation gap.

## Confidence + open questions

Confidence in the root cause: **HIGH**. Evidence is stable across windows, split by classifier, replicated within-LLM. Multiple independent signals converge.

Open questions:
- Does Lever C over-surfacing actually cost quality anywhere, or is it purely a measurement-side inflation? (Not investigated here — would need UVTS A/B.)
- Is there a similar inversion on tier1 classification that's larger than the LLM inversion (10× vs 2×)? Yes, per E2 data. Suggests tier1's embedding-similarity heuristic is systematically biased against imperative-text guidance. Separate sprint if we care.

## What NOT to do

- **Do not** flip Lever C off. It's the mechanism that closes the retrieval-role gap that was itself a longstanding JIMINY-CORPUS-001 issue. Turning it off would regress guidance quality even if it "fixed" the panel.
- **Do not** re-enable `not_applicable` writes to `constraint_outcomes`. The current gate is correct — topically unrelated items shouldn't decay constraint confidence. Emit them elsewhere if needed for observability.
- **Do not** treat this investigation's conclusion as "the panel is wrong so ignore it." The panel signal is honest data; only the *interpretation* was wrong. Actionable compliance rate is a REAL metric worth tracking — just at a lower expected floor than the raw follow rate.
