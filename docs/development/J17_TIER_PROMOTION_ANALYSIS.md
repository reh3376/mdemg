# J17 Tier Promotion Analysis

**Date:** 2026-04-06
**Branch:** `reh3376_dev01`
**Base:** v0.7.1

## Summary

Tested the full J17 trust promotion chain (T3 → T2 → T1) end-to-end. Trust accrued from 0.22 to 0.76 across 15 feedback cycles after fixing three bugs in the trust accrual pipeline. T1 encoding was correctly assigned to items but J8 synthesis overrides the compact format, preventing the 5.2x token compression from reaching the agent.

## Bugs Fixed

### 1. Trust Relevance Threshold Too High

**File:** `internal/jiminy/service.go:1341`
**Before:** `trustRelevanceThreshold = 0.5`
**After:** `trustRelevanceThreshold = 0.20`

The threshold filtered out `partial_compliance` outcomes (similarity 0.20-0.50) before they reached the trust scorer. With 38% of outcomes being partial_compliance, this effectively halved trust growth rate. Aligning with the classifier's `JIMINY_OUTCOME_SIMILARITY_LOW` (0.20) allows all classified outcomes to affect trust.

### 2. OutcomePartialCompliance Missing from Trust Aggregate

**File:** `internal/jiminy/service.go:1347-1368`

The trust aggregate switch only counted `followed`, `ignored`, and `contradicted`. `OutcomePartialCompliance` was silently dropped — items that passed the relevance threshold but were classified as partial compliance never contributed to the aggregate outcome. Even if ALL outcomes in a feedback call were partial_compliance, the aggregate would be `OutcomeUnknown` (no trust update).

Fixed by adding `partialCount` to the switch and routing it to `OutcomePartialCompliance` when no follows dominate and no ignores exist.

### 3. WarmStore Upward-Crossing Invalidation

**File:** `internal/jiminy/service.go:1376-1382`

The WarmStore invalidation logic only fired on downward tier crossings (T1→T2, T2→T3). Upward crossings (T3→T2, T2→T1) did not invalidate the cache, meaning stale lower-tier guidance persisted until natural expiry. Added checks for both promotion and demotion crossings.

## Trust Progression

```
Cycle  F  P  I  Trust   Tier  Notes
-----  -  -  -  ------  ----  -----
 0     -  -  -  0.2200  T3    Baseline (stale since 2026-03-31)
 1     5  1  4  0.2700  T3    First feedback in 6 days
 2     3  6  1  0.3200  T3    +0.05 (follows > ignores)
 3     5  2  0  0.3700  T2    *** T2 CROSSED ***
 4     6  0  3  0.4200  T2
 5     4  1  5  0.4000  T2    -0.02 (context mismatch)
 6     7  0  3  0.4500  T2
 7    10  0  0  0.5000  T2    Perfect cycle
 8     9  0  1  0.5500  T2
 9     1  0  4  0.5300  T2    -0.02 (context mismatch)
10     8  0  2  0.5800  T2
11     ?  ?  ?  0.5600  T2    -0.02
12    10  0  0  0.6100  T2    Perfect — comprehensive action summary
13    10  0  0  0.6600  T2    Perfect
14    10  0  0  0.7100  T2
15     9  0  0  0.7600  T1    *** T1 CROSSED ***
```

**Key observations:**
- 15 cycles to reach T1, 3 cycles to reach T2
- 3 negative cycles (trust decreased) due to action summaries not matching surfaced guidance
- Comprehensive action summaries that explicitly reference surfaced constraint behaviors achieve 10/10 followed
- Context-dependent guidance selection means different contexts surface different items, affecting match rates

## Final Metrics Comparison

| Metric | Baseline | Final |
|--------|----------|-------|
| Trust Score | 0.22 | 0.76 |
| Tier | T3 | T1 |
| Feedback Count | 12 | 27 |
| Tier Distribution (T1/T2/T3) | 0/0/0 | 2.9%/74.4%/22.7% |
| Compression Ratio | 0 | 1.70x |
| T2 Comprehension | 0 | 77.8% |
| Code Coverage | 100% | 82.4% |

## Issue Found: J8 Synthesis Overrides T1 Encoding

**Severity:** P1

When `JIMINY_SYNTHESIS_ENABLED=true` (default), the J8 LLM synthesizer generates a full natural language narrative that replaces the tier-encoded `prompt_augmentation` at `service.go:889`. The T1 compact encoding (`TYPE:SEV|code|[annotations]`) is computed by `encoder.Encode()` but immediately overwritten.

**Impact:** The primary benefit of T1 — ~5.2x token compression — never reaches the agent. All guidance is delivered as full natural language regardless of tier, defeating the purpose of trust-based tier promotion.

**Recommendation:** Make synthesis tier-aware:
- At T1 trust: skip synthesis, use compact encoding directly
- At T2 trust: use synthesis but with a telegraphic style prompt
- At T3 trust: full synthesis (current behavior)

Alternatively, pass the tier-encoded augmentation to the synthesizer as structured context, letting the LLM maintain the compact format while adding coherence.

## Trust Accrual Rate

**Post-fix effective rate:** With the comprehensive action summary pattern (explicitly referencing constraint behaviors), trust increased by +0.05 per cycle (100% follow rate). At ~10 seconds per feedback call (hook cooldown), reaching T1 from initial trust (0.65) requires:

- `(0.75 - 0.65) / 0.05 = 2 cycles` (best case, ~20 seconds)
- From 0.22 baseline: `(0.75 - 0.22) / 0.05 = 10.6 → 11 cycles` (best case)
- With 3 negative cycles: 15 cycles actual (37% overhead from context mismatches)

## Data Location

```
investigation-data/j17-tier-test/
├── baseline/
│   ├── protocol-metrics.json
│   └── trust-status.json
├── t2_035_050/
│   ├── protocol-metrics.json
│   └── trust-status.json
├── t2_050_075/
│   └── trust-status.json
├── t1_above_075/
│   ├── guidance-response.json
│   ├── protocol-metrics.json
│   └── trust-status.json
├── trust-progression.log
└── final-comparison.json
```

## Documents Accessed

- `internal/jiminy/service.go:1330-1392` — Trust aggregate, WarmStore invalidation
- `internal/jiminy/trust.go:112-149` — TrustScorer.RecordOutcome()
- `internal/jiminy/encoder.go:123-145` — selectTier()
- `internal/jiminy/service.go:852-892` — Encode + J8 synthesis override
- `/Users/reh3376/Downloads/J17_TIER_PROMOTION_TEST_PLAN.md` — User's test plan
