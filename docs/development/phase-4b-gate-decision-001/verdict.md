# PHASE-4B-GATE-DECISION-001 — Verdict

**Task**: #159
**Investigated**: 2026-09-11
**Decision**: **DEFER retrain — the gate criterion is technically met but the metric is not yet actionable, AND cross-class analysis suggests the ceiling isn't classifier-quality-bound.**

## TL;DR

- **Honest classifier follow rate (30-day window)**: **17.74%** (n=234)
- **Gate rule from design spec**: retrain if classifier < 70%
- **Gate math says**: retrain justified (17.74% << 70%)
- **BUT the cross-class band is NARROW** (15.28% → 26.47% across all 4 classes) — the follow-rate ceiling is NOT dominated by classifier quality. Retrain would move classifier at best; the aggregate would barely budge.
- **AND organic signal density is low** — only 234 classifier-class outcomes over 30 days = ~8/day. Retrain eval would need 30-60 days of live data to prove any lift over noise.
- **Recommendation**: DEFER Phase 4b large retrain. Ship Path 2 observer #1 first (2-4h, cheap) to grow the process-class denominator + expose more real signal.

## D1 — Row split pre vs post-#158

```
pre_metric_partition (before 2026-09-10) | 7239
post_metric_partition (since 2026-09-10) | 0
```

**Zero organic outcomes** have been recorded since the Path 3 metric partition shipped. The 0.252 gauge live-verified in the #158 sprint post was over 7239 pre-#158 rows all defaulting to `verifiability_class='classifier'` at the column DEFAULT — NOT honest node-classified data. Path 3 shipped the **infrastructure**; the honest reading requires either post-#158 organic data (which hasn't happened) or a retroactive CASE-based classification of historical rows (what this sprint does).

## D6 — Honest per-class rates (168h)

Only 3 rows in the last 168h — sample too small for gate decision.

```
classifier | 3 | 33.33%
```

## D7 — Honest per-class rates (720h / 30 days) — THE ACTIONABLE SIGNAL

Applied via SQL CASE JOIN using the exact 33-code seed from `jiminy_constraint_class.go::jiminySeedClassMap`:

| Honest class | Total | Follow % | Signal-per-day |
|---|---|---|---|
| **classifier** | **234** | **17.74%** | ~8/day |
| process | 50 | 22.00% | ~1.7/day |
| hybrid | 17 | 26.47% | ~0.6/day |
| human | 757 | 16.25% | ~25/day |
| unmapped | 288 | 15.28% | ~10/day |

**Cross-class band: 15.28% → 26.47%.** Narrow. The ceiling is class-independent within noise.

## Analysis

### Gate math (literal reading of the design spec)

- Threshold: 70% classifier follow rate
- Measured: 17.74%
- 17.74% << 70% → **RETRAIN JUSTIFIED BY LITERAL GATE RULE**

### But the gate rule was designed for a different scenario

The design spec (JIMINY-METRIC-DENOMINATOR-DESIGN-001 Phase E) assumed:
1. Classifier class would show a QUALITY GAP relative to process/human classes (which would be handled by Path 2 + HITL)
2. Retrain would specifically address that gap

**Reality (D7)**: classifier class is NOT worse than process or hybrid. The follow-rate ceiling is a SYSTEM property (surfacing precision, corpus quality, action-context mismatch, rule verifiability), not a CLASSIFIER quality problem.

### What retrain COULD improve

An LLM retrain could improve:
- Classifier's ability to grade partial_compliance correctly (currently near-zero rate post-#158 stricter routing per JIMINY-CEILING-INVESTIGATION-002)
- Classifier's precision on borderline "did the agent follow this rule?" cases

An LLM retrain CANNOT improve:
- Whether the RIGHT rules get surfaced by retrieval (Lever C precision — that's substrate)
- Whether the agent actually follows the rules (that's behavior, not classification)
- Human-class rules the classifier structurally can't verify

### Signal density is critically low

- 234 classifier-class rows / 30 days = ~8/day organic
- Statistical detection of a +5pp lift on n=234 needs p<0.05 → requires ~200 more rows (i.e., another month at current rate)
- Retrain compute (30-100h) → return on investment measured in months, not days

### The cross-class band is the honest signal

The follow-rate ceiling gap between classes is <11pp (15.28% → 26.47%). If retrain could move the classifier class UP by even +20pp (optimistic), the classifier class would go 17.74% → ~37.74% — but the other classes stay at 15-26%, so the AGGREGATE (weighted by row count) would move maybe +5pp. That's not what we set out to do.

## Verdict: DEFER retrain

**Recommended sequencing** (in order of value × cost):

1. **Ship Path 2 first observer** (#160 JIMINY-PROCESS-OBSERVER-01, `lint-before-commit`, 2-4h). The process class currently has 50 rows / 22% follow. Ship real process-observation events + a matcher. Watch the process gauge move against a real signal.

2. **Wait 30-90 days** for organic classifier-class row count to grow (currently ~8/day). Re-measure. If the class shows a genuine QUALITY REGRESSION (e.g. drops below 10% while other classes stay at 15-26%), retrain becomes clearly justified. If it stays in the 15-25% band with others, retrain is wasted compute.

3. **Consider corpus growth** instead of retrain — the small n=234 over 30 days is a signal that either (a) the classifier-class rules aren't being surfaced often enough, or (b) the corpus is too narrow. Growing classifier-class rules to N=15+ (from current 9) is cheaper + moves the metric denominator.

4. **Reserve retrain for a substrate-cleaner state** — after Path 2 gives real process signal + the corpus stabilizes.

## Alternative small-scope retrain (if operator wants to spend some compute)

If the operator is committed to spending compute on retrain-adjacent work:

- **Skip full Phase 4b retrain** (30-100h)
- **Ship a TARGETED small-LoRA on the 9 classifier-class rule set only** — 4-8h wall-clock, ~2h compute, fresh benchmark eval on classifier-class-specific data
- **Compare pre/post classifier follow rate** on the 234-row sample
- **Decision criterion**: if targeted small-LoRA moves classifier class by >+10pp on same-runtime benchmark, THEN scope full Phase 4b. Otherwise, retrain is confirmed not the right lever.

## Follow-ups filed

1. **Retire the gate rule's 70% threshold** — the design spec's threshold was an educated guess before data existed. The honest data suggests the ceiling isn't classifier-quality-bound; the threshold should be re-derived (or replaced with a "class-differential trigger": retrain only if classifier drops >10pp below other classes)
2. **Add "signal density" to future gate decisions** — <10 rows/day organic on a class = the class isn't ready to gate compute-heavy work
3. **Consider a shipped small-LoRA sandbox** for cheap experiments — a `mdemg adapter benchmark` pipeline (task #139 shipped) already supports this shape
4. **Do post-#158 organic re-measure at T+30 days (2026-10-10)** — by then enough post-Path-3 outcomes should exist to test whether the honest-classification data flow actually settles the gate

## Documents Accessed

- Live TSDB `mdemg_metrics.constraint_outcomes` — D1 (row split), D6 + D7 (per-class rates via CASE JOIN)
- `internal/cli/jiminy_constraint_class.go::jiminySeedClassMap` — 33-code static seed used as CASE map
- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` — Phase E gate rule
- `docs/development/jiminy-metric-partition-001/sprint_post.md` — Path 3 ship state
- `docs/development/jiminy-ceiling-investigation-002/verdict.md` — root cause taxonomy
- Operator directive: "proceed with #159" (2026-09-11)
