# Jiminy follow-rate decline 2026-08-10 — INVESTIGATION

**Investigator:** Claude Opus 4.7
**Date:** 2026-08-10
**Trigger:** operator observation "Jiminy guidance follow rate on consistent decline. High last week near 30% → currently only 15.3%. Deep dive + suggest a fix. Consider LLM-model-swap as one angle."

## Headline

**Not a substrate quality decline. Measurement-bias correction.**

The `mdemg_jiminy_follow_rate` gauge is being deflated as the `heuristic` classifier fallback fires less often. The heuristic's default verdict is `partial_compliance` (0.5 credit); the LLM classifier's actual signal is ~1% partial_compliance. When LLM stability improves → heuristic mix shrinks → gauge deflates. **The gauge was previously inflated by an unreliable-LLM proxy, not by higher substrate quality.**

## The signal the operator saw

Reproduced the exact gauge SQL (`DatasetBuilder.GuidanceEffectiveness`, `internal/tsdb/dataset_builder.go:282`) against the last 14 days on `mdemg-dev`:

| Day    | Rolling 168h follow-rate |
|--------|-----------------------|
| 07-27  | 22.04%                |
| 07-28  | 23.20%                |
| 07-29  | 15.22%                |
| 07-30  | 13.54%                |
| ...    | (dip 13-15%)          |
| 08-04  | 22.53%                |
| 08-05  | 22.37%                |
| 08-06  | **24.39%** (peak)     |
| 08-07  | 21.58%                |
| 08-08  | 21.85%                |
| 08-09  | 19.56%                |
| 08-10  | **16.42%** (current)  |

Operator's "30%" was slightly off; "~23-24% high → 16% current" is the honest read. Direction + magnitude of the decline are real.

## Root cause — classifier mix shift

The `mdemg_jiminy_follow_rate` gauge weights outcomes:
- `followed` → 1.0
- `partial_compliance` → 0.5
- `ignored` / `contradicted` / anything else → 0.0

Per-day per-classifier-source `partial_compliance` rate:

| Day    | heuristic verdicts | heur % partial_comp | llm verdicts | llm % partial_comp |
|--------|------------------|--------------------|--------------|--------------------|
| 08-01  | 238              | **100%**           | 367          | 1.1%               |
| 08-02  | 206              | **100%**           | 178          | 0%                 |
| 08-03  | 130              | **100%**           | 440          | 0.9%               |
| 08-04  | 51               | **100%**           | 246          | 0%                 |
| 08-05  | 18               | **100%**           | 300          | 0.7%               |
| 08-06  | 33               | **100%**           | 762          | 0.3%               |
| 08-07  | **0**            | —                  | 87           | 0%                 |
| 08-08  | **0**            | —                  | 100          | 1.0%               |
| 08-09  | 6                | 100%               | 85           | 1.2%               |
| 08-10  | 34               | **97.1%**          | 386          | 0.3%               |

Every `partial_compliance` verdict comes from the heuristic classifier. **The LLM classifier virtually never assigns `partial_compliance`.**

## Recent 3d vs prior 3d outcome delta

| guidance_type | outcome           | prior 3d | recent 3d | Δ    |
|--------------|-------------------|---------:|----------:|-----:|
| pattern      | partial_compliance| 317      | 17        | **−300** |
| learning     | partial_compliance| 140      | 20        | **−120** |
| constraint   | partial_compliance| 87       | 4         | **−83**  |
| pattern      | ignored           | 293      | 182       | −111 |
| constraint   | ignored           | 346      | 179       | −167 |
| pattern      | followed          | 59       | 23        | −36  |
| constraint   | followed          | 48       | 22        | −26  |
| correction   | followed          | 16       | 14        | −2   |

`partial_compliance` collapsed **593 → 42** (−551, −93%) — dominant driver of the gauge decline. `followed` counts also dropped proportional to total volume; the actionable follow rate over 3d windows remained stable (11-12%).

## Why the heuristic fires

`internal/jiminy/outcome_classifier.go:359`:

```go
// Heuristic fallback: no LLM available or LLM returned unknown.
if hasNegation {
    return ClassificationResult{Outcome: OutcomeContradicted, Confidence: similarity, Source: "heuristic"}
}
if similarity >= oc.highThreshold {
    return ClassificationResult{Outcome: OutcomeFollowed, Confidence: similarity, Source: "heuristic"}
}
return ClassificationResult{Outcome: OutcomePartialCompliance, Confidence: similarity, Source: "heuristic"}
```

The default-partial-compliance branch fires when:
1. `oc.llm == nil` (LLM disabled) — not the case on mdemg-dev
2. LLM returned `OutcomeUnknown` (parse error, timeout, refusal, circuit-breaker open)

On 07-29 through 08-06, LLM instability drove heuristic mix to 10-51% of daily volume — inflating the gauge by ~10 percentage points. When LLM stabilized (LLM-HEALTH-CANCELLATION-ALERT-001 shipped 2026-07-21 + LLM-HEALTH-INVESTIGATION-001's `caller_canceled` tagging classification fixes, both compounding), heuristic drops → gauge deflates.

## Sanity check — actionable follow rate

The actionable-only follow rate (`Actionable Compliance Rate` panel, DASHBOARD-TRUTH-002 + JIMINY-FOLLOW-RATE-REMEASURE-001) is **STABLE at ~11-12%** across the same window:

| Day (7d rolling) | actionable_pct | raw_pct |
|-----------------|----------------|---------|
| 08-01           | 10.08%         | 9.78%   |
| 08-04           | 11.76%         | 9.98%   |
| 08-06           | 11.60%         | 10.40%  |
| 08-08           | 11.48%         | 10.36%  |
| 08-10           | **12.61%**     | 11.61%  |

Actionable follow rate is actually **UP** slightly (10.08 → 12.61 in 9 days). The measurement that matters (constraint + correction compliance) shows the substrate healthy.

## What is not the cause

- **Not** a corpus quality regression: JIMINY-ARCHIVED-CODE-FILTER-001 (2026-08-10) tightened the corpus; effects would move actionable rate too.
- **Not** JIMINY-TIER1-BYPASS-001 (2026-07-30): the tier1 bypass routes more traffic to the LLM; if anything it should reduce the gauge inflation by shrinking tier1 volume (which is what happened).
- **Not** LLM model degradation: LLM verdict distribution is stable (~1% partial_compliance across the whole 14-day window).
- **Not** guidance-surfacing volume: total outcomes went up not down (per-day 300 → 500+ during the peak window).

## The gauge is broken as a substrate-quality signal

The `mdemg_jiminy_follow_rate` gauge's design flaw:

**When the LLM classifier is HEALTHIER, the gauge goes DOWN.** That's the opposite of what a substrate-quality signal should do. A follow-rate metric should be indifferent to classifier availability; here it's inversely coupled.

The heuristic fallback's design was pre-JIMINY-TIER1-BYPASS-001: it was supposed to be RARE (fires only when LLM unavailable or unparseable). Its default of `partial_compliance` was a "give benefit of doubt when we don't know" heuristic that made sense when the fallback was rare. As LLM reliability + tier1-bypass increased the LLM's share of the mix, the heuristic's inflation is now the DOMINANT source of gauge movement.

## Two-lever fix

### Lever A (immediate, minimum-code) — change heuristic default from `partial_compliance` to `ignored`

Single-line edit at `outcome_classifier.go:359`:

```go
- return ClassificationResult{Outcome: OutcomePartialCompliance, Confidence: similarity, Source: "heuristic"}
+ return ClassificationResult{Outcome: OutcomeIgnored,           Confidence: similarity, Source: "heuristic"}
```

**Rationale:** when we don't know the outcome, defaulting to a HALF-CREDIT verdict (partial_compliance = 0.5) inflates the follow rate. Defaulting to `ignored` (zero credit) is CONSERVATIVE — it treats unknown-outcome cases as "assume the agent didn't follow" and lets classifier improvements EARN credit.

**Effect:** the gauge will drop from ~16% to ~11% overnight (matches the honest actionable-follow rate). Alert floor `JIMINY_FOLLOW_RATE_ALERT_FLOOR` (currently 0.15) will need lowering to 0.05 (mirror `GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR` from JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 which already sits below the ~13% actionable steady state).

**Historical rows:** DO NOT retroactively rewrite. Old rows are the honest reflection of the classifier's actual verdicts at those times. Windowed metrics naturally age them out; the shift will be visible as a step-change on the panel, which is exactly the right operator experience — a truthful annotation.

### Lever B (structural) — separate `heuristic_unparseable` from `heuristic_similarity_only` source labels

Today `classifier_source='heuristic'` collapses two very different failure modes:
1. LLM returned `OutcomeUnknown` (parse fail, refusal, timeout) — we truly don't know
2. Similarity fell below tier1-bypass threshold (below `naThreshold`) — matched to `not_applicable` short-circuit; the bypass code sees this too

Splitting the labels lets alert rules key on "unparseable" separately from "under threshold", and lets dashboards show them separately. `HeuristicShareRule` (CLASSIFIER-CONSISTENCY-001) already exists — extending it with a specific `heuristic_unparseable` component would be a stronger LLM-health signal than the current omnibus counter.

### Lever C (defer — the model-swap angle you mentioned)

Evaluate `Qwen3-14B → newer model` post-beta-testing. A stronger classifier could:
- Reduce parse-fail / refusal rate (fewer OutcomeUnknown, fewer heuristic fallbacks)
- Increase partial_compliance detection rate (the current LLM classifier's ~1% partial_compliance rate is suspiciously low — a stronger classifier might actually find that ~5-10% of verdicts are genuinely partial)

**But Lever C alone does NOT fix the gauge bias.** Even a perfect LLM classifier would leave the heuristic default at `partial_compliance` — any residual heuristic firing (network blip, one bad response) still inflates. Lever A is the durable fix; Lever C is a quality lift on top.

## Recommendation

Ship **Lever A + alert floor recalibration + panel description update** as sprint `JIMINY-HEURISTIC-DEFAULT-001`. Lever B follows as an observability lift once we see the post-Lever-A steady state on the honest signal. Lever C stays on the post-beta-release roadmap.

Passive re-measurement of both gauges 7 days after Lever A ships. If Lever B fires enough to be actionable, sprint it.

## Documents Accessed

- `internal/tsdb/dataset_builder.go` (GuidanceEffectiveness — the gauge query)
- `internal/jiminy/outcome_classifier.go` (heuristic default → partial_compliance)
- `internal/jiminy/stats.go` (Neo4j fallback + inflated-rate comment)
- `internal/ape/self_assess.go` (applyHonestFollowRate override)
- `internal/config/config.go` (RSICGuidanceEffectivenessWindowHours, JiminyFollowRateAlertFloor)
- Live SQL: 20d classifier-source × outcome distribution, 14d gauge reproduction, 3d-vs-3d outcome delta
- CLAUDE.md pins: JIMINY-FOLLOW-RATE-REMEASURE-001, JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001, JIMINY-CLASSIFIER-CONTEXT-001, LLM-HEALTH-CANCELLATION-ALERT-001, CLASSIFIER-CONSISTENCY-001, JIMINY-ARCHIVED-CODE-FILTER-001
