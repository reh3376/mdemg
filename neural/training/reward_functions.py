"""GRPO Reward Functions for MDEMG Post-SFT Reinforcement Learning.

Provides reward functions referenced by ULTS specs' reward_functions arrays.
Each function returns a float in [0.0, 1.0] suitable for GRPO training.

The 18 unique reward functions across all 16 ULTS tasks are grouped into
categories: structural, classification, quality, ranking, and performance.

Usage:
    from training.reward_functions import get_reward_function, compute_reward

    fn = get_reward_function("json_valid")
    score = fn(response, schema=output_schema)

    # Or compute all rewards for a task:
    scores = compute_reward(response, reward_names, schema=output_schema)
"""

import json
import math
from typing import Any, Callable


# Type alias for reward functions
RewardFn = Callable[..., float]


# ── Structural Rewards ──


def json_valid(response: str, **kwargs: Any) -> float:
    """Return 1.0 if response is a valid JSON object, 0.0 otherwise."""
    try:
        parsed = json.loads(response)
        return 1.0 if isinstance(parsed, dict) else 0.0
    except (json.JSONDecodeError, TypeError):
        return 0.0


def schema_match(response: str, schema: dict[str, Any] | None = None, **kwargs: Any) -> float:
    """Partial credit for schema conformance: required keys + type checks."""
    if not schema:
        return json_valid(response)

    schema_type = schema.get("type", "string")
    if schema_type == "string":
        return 1.0 if response and response.strip() else 0.0

    try:
        parsed = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    if not isinstance(parsed, dict):
        return 0.0

    required = schema.get("required", [])
    properties = schema.get("properties", {})

    if not required and not properties:
        return 1.0

    # Score: fraction of required keys present with correct types
    checks = []

    type_map = {
        "string": str, "number": (int, float), "integer": int,
        "boolean": bool, "array": list, "object": dict,
    }

    for key in required:
        if key not in parsed:
            checks.append(0.0)
            continue

        prop_schema = properties.get(key, {})
        expected_type = prop_schema.get("type")
        if expected_type and expected_type in type_map:
            checks.append(1.0 if isinstance(parsed[key], type_map[expected_type]) else 0.5)
        else:
            checks.append(1.0)  # Key present, no type constraint

    return sum(checks) / len(checks) if checks else 1.0


def format_valid(response: str, **kwargs: Any) -> float:
    """Return 1.0 if response has valid structure (non-empty, well-formed)."""
    if not response or not response.strip():
        return 0.0
    # Check for common format issues
    stripped = response.strip()
    if stripped.startswith("{") or stripped.startswith("["):
        try:
            json.loads(stripped)
            return 1.0
        except json.JSONDecodeError:
            return 0.0
    return 1.0  # Non-JSON text is valid for string outputs


# ── Classification Rewards ──


def classification_accuracy(
    response: str,
    expected: str | None = None,
    **kwargs: Any,
) -> float:
    """Return 1.0 if predicted classification matches expected, 0.0 otherwise."""
    if expected is None:
        return json_valid(response)  # Can't verify without ground truth

    try:
        parsed = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    if isinstance(parsed, dict):
        predicted = parsed.get("type", parsed.get("classification", ""))
    else:
        predicted = str(parsed)

    return 1.0 if str(predicted).lower().strip() == str(expected).lower().strip() else 0.0


def evaluation_accuracy(
    response: str,
    expected_verdict: str | None = None,
    **kwargs: Any,
) -> float:
    """Reward for correct evaluation verdict (jiminy.evaluate tasks)."""
    if expected_verdict is None:
        return json_valid(response)

    try:
        parsed = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    if isinstance(parsed, dict):
        verdict = parsed.get("verdict", parsed.get("evaluation", ""))
        return 1.0 if str(verdict).lower().strip() == expected_verdict.lower().strip() else 0.0

    return 0.0


# ── Quality Rewards ──


def coherence_score(response: str, **kwargs: Any) -> float:
    """Heuristic coherence score based on structure indicators."""
    if not response or not response.strip():
        return 0.0

    score = 0.5  # Base score for non-empty
    text = response.strip()

    # Reward sentence structure
    sentences = [s.strip() for s in text.replace(".", ".\n").split("\n") if s.strip()]
    if len(sentences) >= 2:
        score += 0.2

    # Reward reasonable length
    words = text.split()
    if 10 <= len(words) <= 500:
        score += 0.2

    # Penalize repetition
    if len(set(words)) / max(len(words), 1) < 0.3:
        score -= 0.3

    return max(0.0, min(1.0, score))


def coverage_score(response: str, **kwargs: Any) -> float:
    """Heuristic coverage score — rewards detail and breadth."""
    if not response or not response.strip():
        return 0.0

    words = response.strip().split()
    word_count = len(words)

    if word_count < 5:
        return 0.1
    elif word_count < 20:
        return 0.4
    elif word_count < 50:
        return 0.7
    else:
        return min(1.0, 0.7 + (word_count - 50) / 500)


def summary_quality(response: str, **kwargs: Any) -> float:
    """Combined coherence + coverage for summary-type outputs."""
    return (coherence_score(response) + coverage_score(response)) / 2


def explanation_quality(response: str, **kwargs: Any) -> float:
    """Reward for explanation quality (evaluation tasks)."""
    if not response or not response.strip():
        return 0.0

    try:
        parsed = json.loads(response)
        explanation = parsed.get("explanation", parsed.get("reasoning", ""))
    except (json.JSONDecodeError, TypeError):
        explanation = response

    if not explanation:
        return 0.0

    words = str(explanation).split()
    if len(words) < 5:
        return 0.2
    elif len(words) < 20:
        return 0.6
    else:
        return 0.9


def specificity_score(response: str, **kwargs: Any) -> float:
    """Reward specific, actionable guidance over generic text."""
    if not response or not response.strip():
        return 0.0

    text = response.lower()
    generic_phrases = ["in general", "it depends", "there are many", "various ways"]
    specific_indicators = ["must", "never", "always", "specifically", "because", "when"]

    generic_count = sum(1 for p in generic_phrases if p in text)
    specific_count = sum(1 for p in specific_indicators if p in text)

    base = 0.5
    base += specific_count * 0.1
    base -= generic_count * 0.15

    return max(0.0, min(1.0, base))


def actionability_score(response: str, **kwargs: Any) -> float:
    """Reward actionable insights with concrete recommendations."""
    if not response or not response.strip():
        return 0.0

    text = response.lower()
    action_indicators = [
        "should", "recommend", "consider", "implement", "refactor",
        "add", "remove", "replace", "migrate", "update",
    ]

    hits = sum(1 for ind in action_indicators if ind in text)
    return min(1.0, 0.3 + hits * 0.15)


def follow_rate(response: str, **kwargs: Any) -> float:
    """Proxy for guidance follow rate — rewards structured, clear guidance."""
    return (specificity_score(response) + actionability_score(response)) / 2


def insight_count(response: str, **kwargs: Any) -> float:
    """Reward based on number of distinct insights/points."""
    if not response or not response.strip():
        return 0.0

    # Count bullet points, numbered items, or sentences
    lines = [l.strip() for l in response.split("\n") if l.strip()]
    points = sum(1 for l in lines if l.startswith(("-", "*", "•")) or
                 (len(l) > 1 and l[0].isdigit() and l[1] in ".)" ))

    if points == 0:
        # Fall back to sentence count
        points = len([s for s in response.split(".") if s.strip()])

    if points >= 5:
        return 1.0
    elif points >= 3:
        return 0.8
    elif points >= 1:
        return 0.5
    return 0.2


def uniqueness_check(response: str, **kwargs: Any) -> float:
    """Reward unique, non-templated responses."""
    if not response or not response.strip():
        return 0.0

    words = response.strip().split()
    unique_ratio = len(set(w.lower() for w in words)) / max(len(words), 1)

    if unique_ratio > 0.7:
        return 1.0
    elif unique_ratio > 0.5:
        return 0.7
    elif unique_ratio > 0.3:
        return 0.4
    return 0.1


def naming_quality_score(response: str, **kwargs: Any) -> float:
    """Reward quality of concept/cluster names."""
    try:
        parsed = json.loads(response)
        name = parsed.get("name", parsed.get("cluster_name", ""))
    except (json.JSONDecodeError, TypeError):
        name = response.strip()

    if not name:
        return 0.0

    words = name.split()
    # Good names are 2-5 words, descriptive
    if 2 <= len(words) <= 5:
        return 0.9
    elif len(words) == 1:
        return 0.5
    else:
        return 0.4


def generalization_quality_score(response: str, **kwargs: Any) -> float:
    """Reward quality of cross-codebase generalizations."""
    return (coherence_score(response) + specificity_score(response) + coverage_score(response)) / 3


# ── Ranking Rewards ──


def ndcg_delta(
    response: str,
    reference_scores: list[float] | None = None,
    **kwargs: Any,
) -> float:
    """Reward ranking quality improvement. Without reference, checks format."""
    if reference_scores is None:
        # Can't compute without reference; check if response looks like scores
        try:
            scores = json.loads(response)
            if isinstance(scores, list) and all(isinstance(s, (int, float)) for s in scores):
                return 0.7  # Valid format but can't verify quality
        except (json.JSONDecodeError, TypeError):
            pass
        return json_valid(response) * 0.5

    return 0.7  # Placeholder — real NDCG needs query-doc relevance labels


def score_calibration(response: str, **kwargs: Any) -> float:
    """Reward well-calibrated reranking scores (in [0, 1] range)."""
    try:
        parsed = json.loads(response)
        if isinstance(parsed, list):
            scores = parsed
        elif isinstance(parsed, dict):
            scores = list(parsed.values())
        else:
            return 0.0
    except (json.JSONDecodeError, TypeError):
        return 0.0

    if not scores:
        return 0.0

    numeric = [s for s in scores if isinstance(s, (int, float))]
    if not numeric:
        return 0.0

    # Check scores are in [0, 1]
    in_range = sum(1 for s in numeric if 0 <= s <= 1)
    return in_range / len(numeric)


def score_correlation(response: str, **kwargs: Any) -> float:
    """Proxy for score correlation quality."""
    return score_calibration(response)


def recall_improvement(response: str, **kwargs: Any) -> float:
    """Reward for intent translation that improves retrieval recall."""
    if not response or not response.strip():
        return 0.0

    # Reward query expansion/rewriting that adds terms
    text = response.strip()
    words = text.split()
    if len(words) > 3:
        return min(1.0, 0.5 + len(words) * 0.05)
    return 0.3


# ── Performance Reward ──


def latency_reward(
    response: str,
    latency_ms: float = 0,
    budget_ms: float = 3000,
    **kwargs: Any,
) -> float:
    """Sigmoid decay reward based on latency vs budget.

    Returns 1.0 when latency is well under budget, decays toward 0.0
    as latency exceeds budget.
    """
    if budget_ms <= 0:
        return 1.0

    ratio = latency_ms / budget_ms
    # Sigmoid centered at ratio=1.0, steepness=5
    return 1.0 / (1.0 + math.exp(5 * (ratio - 1.0)))


# ── Registry ──


REWARD_REGISTRY: dict[str, RewardFn] = {
    # Structural
    "json_valid": json_valid,
    "schema_match": schema_match,
    "format_valid": format_valid,
    # Classification
    "classification_accuracy": classification_accuracy,
    "evaluation_accuracy": evaluation_accuracy,
    # Quality
    "coherence_score": coherence_score,
    "coverage_score": coverage_score,
    "summary_quality": summary_quality,
    "explanation_quality": explanation_quality,
    "specificity_score": specificity_score,
    "actionability_score": actionability_score,
    "follow_rate": follow_rate,
    "insight_count": insight_count,
    "uniqueness_check": uniqueness_check,
    "naming_quality_score": naming_quality_score,
    "generalization_quality_score": generalization_quality_score,
    # Ranking
    "ndcg_delta": ndcg_delta,
    "score_calibration": score_calibration,
    "score_correlation": score_correlation,
    "recall_improvement": recall_improvement,
    # Performance
    "latency_reward": latency_reward,
}


def get_reward_function(name: str) -> RewardFn | None:
    """Look up a reward function by name."""
    return REWARD_REGISTRY.get(name)


def compute_reward(
    response: str,
    reward_names: list[str],
    **kwargs: Any,
) -> dict[str, float]:
    """Compute all named rewards for a response.

    Returns dict mapping reward name to score.
    """
    results = {}
    for name in reward_names:
        fn = REWARD_REGISTRY.get(name)
        if fn:
            results[name] = fn(response, **kwargs)
        else:
            results[name] = 0.0
    return results
