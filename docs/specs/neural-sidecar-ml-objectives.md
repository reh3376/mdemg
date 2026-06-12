# Neural Sidecar ML Objectives

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Component**: Tier prediction model (`/protocol/predict-tier`)
**Training pipeline**: `neural/neural_sidecar/train_protocol.py`
**Base model**: `cross-encoder/ms-marco-MiniLM-L-6-v2`
**Data source**: J17 protocol JSONL from `protocol_data_collector.go`

---

## Primary Objective

Maximize **comprehension-per-token weighted by severity**.

The tier prediction model selects the encoding tier (T1/T2/T3) that achieves the highest comprehension score per token consumed, weighted by the constraint's severity level:

| Severity | Weight | Meaning |
|----------|--------|---------|
| `must` / `must_not` (!) | 3 | Critical constraints. Miscomprehension causes policy violations. |
| `should` / `should_not` (?) | 2 | Important constraints. Miscomprehension causes suboptimal behavior. |
| `info` (~) | 1 | Informational. Miscomprehension has low impact. |

**Objective function** (per constraint):

```
score(tier) = severity_weight * comprehension(tier) / token_count(tier)
```

The model predicts the tier that maximizes this score given the constraint text, agent context, and trust score.

---

## Hard Constraint

**Must-level items (`!`) ALWAYS use T3 when NLI comprehension < 0.9.**

This is a post-prediction override, not a model objective. Regardless of what the model predicts, the Go caller enforces:

```
if severity == "must" && nli_comprehension(tier) < 0.9:
    tier = T3  # full natural language
```

Rationale: A must-level constraint misunderstood at T1 or T2 causes a policy violation. The token cost of T3 (~80 tokens) is negligible compared to the cost of a violation. This override is implemented in the tier selection logic (`internal/jiminy/encoder.go`), not in the model itself, so the model can learn freely without safety constraints distorting the loss surface.

---

## Training Target

**Input**: `{constraint_text, agent_context, trust_score}`
**Output**: `{optimal_tier, expected_comprehension}`

The training pipeline (`train_protocol.py`) derives optimal tiers from historical data:

1. Group protocol JSONL records by `constraint_code`
2. For each constraint, compute average efficiency per tier: `avg_comprehension / max(avg_token_count, 1)`
3. The tier with highest efficiency is the optimal tier for that constraint
4. Generate training pairs: `(constraint_text, "trust={trust_score} {agent_action}")` with score 1.0 for optimal tier, 0.2 for suboptimal tiers

The cross-encoder regresses a single score; tier boundaries at 0.7 (T1/T2) and 0.4 (T2/T3) map the regression output to discrete tiers.

---

## Evaluation Metric

**Agreement rate**: percentage of predictions where the ML-selected tier matches the best-outcome tier from historical data.

```
agreement_rate = count(ml_tier == optimal_tier) / count(all_predictions)
```

The optimal tier for evaluation is derived from the same efficiency formula used in training, computed over a held-out validation set (default 10% split).

Secondary metric: **Spearman rank correlation** between the model's regression scores and the training labels. Reported as `spearman_correlation` in `training_metadata.json` after each training run.

---

## Loss Function

**Weighted cross-entropy with severity weighting.**

The base loss is cross-entropy between predicted tier probabilities and the one-hot optimal tier label. Each sample is weighted by the constraint's severity:

```
L = -sum_i [ w_i * sum_k [ y_ik * log(p_ik) ] ]

where:
  w_i = severity_weight(sample_i)   # 3 for must, 2 for should, 1 for info
  y_ik = 1 if tier k is optimal for sample i, else 0
  p_ik = predicted probability of tier k for sample i
```

In the current cross-encoder regression implementation, the severity weighting is applied via the training pair score: optimal tier samples get score `1.0 * severity_weight/3`, suboptimal tier samples get `0.2 * severity_weight/3`. This ensures the model pays more attention to getting must-level constraints right.

---

## Negative Examples

Negative examples are cases where the ML-predicted tier led to lower comprehension than the rule-based tier selector would have chosen. These are identified by comparing:

1. **ML outcome**: The actual comprehension score when the ML tier was used
2. **Rule-based counterfactual**: The historical average comprehension for the rule-based tier on the same constraint

A sample qualifies as a negative example when:

```
comprehension(ml_tier) < comprehension(rule_tier) - epsilon
```

where `epsilon = 0.05` (avoids noise from measurement variance).

Negative examples serve two purposes:

1. **Training signal**: They are included in the training set with score 0.0 (penalizing the ML-selected tier) to teach the model to avoid these predictions.

2. **Evaluation diagnostic**: A rising count of negative examples across training iterations indicates the model is diverging from the rule-based baseline rather than improving on it. If negative examples exceed 15% of the validation set, the training run is flagged for review.

---

## Minimum Data Requirements

| Threshold | Value | Rationale |
|-----------|-------|-----------|
| Minimum samples to train | 500 | Below this, the model overfits to a small constraint vocabulary |
| Minimum constraints represented | 10 | Need diversity across severity levels and code types |
| Minimum sessions represented | 5 | Trust score variance requires multi-session data |
| Validation split | 10% | Held out before training, used for Spearman evaluation |

The training pipeline (`train_protocol.py`) enforces the 500-sample minimum and skips training with a diagnostic message if insufficient data is available.

---

## Model Versioning

Models are saved as versioned directories under `.mdemg/neural/models/protocol-tier/`:

```
protocol-tier/
  v1/
    model.safetensors
    training_metadata.json
  v2/
    ...
  current -> v2/   (symlink, updated by training pipeline)
```

The `current` symlink is updated atomically after each successful training run. The sidecar loads from whatever path `NEURAL_TIER_MODEL` points to (typically `current`).

Rollback: point `NEURAL_TIER_MODEL` to a previous version directory or set it to empty to disable ML predictions entirely.
