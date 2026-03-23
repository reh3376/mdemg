# Neural Sidecar Benchmark Protocol

**Epic**: 5 (Protocol Evolution via ML)
**Step**: 5.3
**Prerequisite**: Shadow mode operational (J17 control-loop Gap 6), training pipeline producing versioned models

---

## Overview

This protocol defines how to evaluate the neural sidecar's tier prediction model against the rule-based baseline and determine when the ML model is ready for promotion from shadow mode to active mode. Two evaluation modes are defined: online (compare mode, live traffic) and offline (batch evaluation on historical data).

---

## Online Metrics (Compare Mode)

In compare mode, both the rule-based tier selector and the ML tier predictor run on every constraint. The rule-based result is used for actual encoding; the ML result is logged with prefix `j17-shadow:` for offline comparison. No behavioral effect.

### Agreement Rate

Percentage of constraints where the ML-predicted tier matches the rule-based tier.

```
agreement_rate = count(ml_tier == rule_tier) / count(total_predictions)
```

Measured over a rolling window (default: last 1000 predictions). Logged per-session and aggregated across sessions.

Disaggregated by severity to detect systematic bias:

| Severity | Acceptable Disagreement |
|----------|------------------------|
| must (!) | < 5% (ML must not under-encode critical constraints) |
| should (?) | < 15% |
| info (~) | < 25% |

### Comprehension Delta

Difference in comprehension score between the ML-selected tier and the rule-based tier, measured via NLI or heuristic scoring.

```
comprehension_delta = comprehension(ml_tier) - comprehension(rule_tier)
```

Computed per constraint, averaged over the rolling window. A positive delta means the ML model achieves better comprehension. A negative delta means the ML model degrades comprehension.

Disaggregated by tier transition type to identify failure patterns:

| Transition | Expected Frequency | Risk |
|------------|-------------------|------|
| ML selects T1 where rule selects T2 | Common | Under-encoding: agent may misunderstand |
| ML selects T2 where rule selects T1 | Rare | Over-encoding: wastes tokens but safe |
| ML selects T1 where rule selects T3 | Rare | High risk: skipping two tiers on novel constraint |
| ML selects T3 where rule selects T1 | Very rare | Safe but wasteful |

### Latency Overhead

Additional latency introduced by the ML prediction call, measured as the wall-clock time of the sidecar HTTP round-trip.

```
latency_overhead_ms = sidecar_response_time_ms
```

Measured per prediction from the Go caller side (includes network, serialization, inference). Reported as p50, p95, and p99.

The ML prediction runs in parallel with rule-based tier selection when in shadow mode, so it does not add to the critical path. In active mode, it replaces the rule-based path, so latency becomes critical-path.

---

## Offline Metrics

Batch evaluation on collected protocol JSONL data. Run via `mdemg-neural-evaluate` or as part of the training pipeline validation step.

### Accuracy vs Optimal Tier

For each historical record, compute the optimal tier (highest comprehension-per-token efficiency) and compare to the ML prediction.

```
accuracy = count(ml_tier == optimal_tier) / count(total_records)
```

The optimal tier is derived using the same `derive_optimal_tiers()` function from `train_protocol.py`: group by constraint code, compute average efficiency per tier, select the tier with highest efficiency.

### Severity-Weighted Accuracy

Same as accuracy, but each sample is weighted by its severity (must=3, should=2, info=1):

```
weighted_accuracy = sum(w_i * match_i) / sum(w_i)

where:
  w_i = severity_weight(sample_i)
  match_i = 1 if ml_tier == optimal_tier, else 0
```

This ensures must-level constraint accuracy dominates the metric.

### Token Savings

Estimated token reduction from ML tier selection vs always using T3 (full NL):

```
token_savings = 1 - (sum(tokens_at_ml_tier) / sum(tokens_at_t3))
```

Reported as a percentage. The rule-based baseline achieves approximately 81% savings (19% of T3 token count at T1 compact). The ML model should match or exceed this.

---

## Success Criteria for Promotion

All criteria must be met simultaneously over a minimum evaluation window of 7 days (or 5000 predictions, whichever comes first).

| Metric | Threshold | Rationale |
|--------|-----------|-----------|
| Agreement rate (overall) | >= 85% | ML must broadly agree with the proven rule-based system |
| Agreement rate (must-severity) | >= 95% | Critical constraints cannot tolerate ML-induced tier errors |
| Comprehension delta (mean) | >= 0.0 | ML must not degrade comprehension on average |
| Comprehension delta (must-severity, min) | >= -0.05 | No individual must-level constraint can drop more than 5% |
| p99 latency | < 200ms | Must fit within `J17_SIDECAR_TIMEOUT_MS` default |
| Negative example rate | < 10% | Less than 10% of ML predictions should be worse than rule-based |
| Model confidence (mean) | >= 0.3 | Model should not be guessing at random on most predictions |

### Promotion Decision

Promotion is a manual decision, not automatic. The metrics above are necessary but not sufficient conditions. An operator reviews:

1. Metrics dashboard (agreement, comprehension, latency over time)
2. Failure case analysis (which constraints does ML get wrong, and why)
3. Severity distribution (is ML systematically wrong on one severity class)

If all criteria are met and the failure analysis shows no systematic issues, promotion proceeds by setting `J17_SIDECAR_URL` and removing the shadow-mode flag.

### Rollback

If post-promotion metrics degrade below the success criteria thresholds, rollback is immediate:

1. Set `NEURAL_TIER_MODEL` to empty string (disables ML predictions, sidecar returns `predicted_tier: 0`)
2. Go caller falls back to rule-based tier selection (this is the default behavior when `predicted_tier == 0`)
3. No restart required -- the sidecar's `/protocol/predict-tier` endpoint returns the fallback response instantly

---

## Baseline Comparison Method

The rule-based tier selector (`internal/jiminy/encoder.go`) is the baseline. Its selection logic is deterministic given trust score and code availability:

| Trust Score | Has T1 Code? | Rule-Based Tier |
|-------------|-------------|-----------------|
| > 0.8 | Yes | T1 |
| 0.4 - 0.8 | Yes | T1 |
| < 0.4 | Yes | T2 |
| > 0.8 | No | T2 |
| < 0.8 | No | T3 |

To compute baseline metrics on historical data:

1. Replay each protocol JSONL record through the rule-based selector
2. Record what tier the rule-based system would have chosen
3. Compute comprehension and token count for that tier from historical data
4. Compare against ML predictions on the same records

This gives a paired comparison on identical data, eliminating confounders from different constraint sets or trust distributions.

### A/B Evaluation (Optional)

For higher-confidence promotion decisions, run a split evaluation:

1. Assign 50% of sessions to ML-active, 50% to rule-based (hash on session ID)
2. Measure comprehension and token usage per group over the evaluation window
3. Statistical test: two-sample t-test on comprehension scores, significance level p < 0.05

This is optional because it requires sufficient session volume. Shadow-mode comparison is sufficient for initial promotion.

---

## Metrics Collection

All benchmark metrics are derived from data already collected by the existing infrastructure:

| Data Source | Collector | Storage |
|-------------|-----------|---------|
| ML predictions (shadow mode) | Go caller logs `j17-shadow:` entries | Server logs |
| Rule-based tier selections | `protocol_data_collector.go` | `.mdemg/neural/protocol-data/*.jsonl` |
| Comprehension scores | `NLIComprehensionScorer` or heuristic | Protocol JSONL `comprehension_score` field |
| Latency | Go HTTP client timing | Server logs |
| Protocol metrics snapshot | `GET /v1/jiminy/protocol/metrics` | In-memory (ProtocolMetricsCollector) |

No additional collection infrastructure is needed. Offline evaluation reads the JSONL files; online metrics are computed from server logs.
