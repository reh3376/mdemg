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


def json_valid(
    response: str, schema: dict[str, Any] | None = None, **kwargs: Any
) -> float:
    """Return 1.0 if response is structurally valid JSON against the ULTS schema.

    Schema-aware (``schema.type``), matching
    ``neural.training.evaluate_ft.check_json_valid``:
      * ``"object"`` (default): response must parse to a dict.
      * ``"array"``:            response must parse to a list.
      * ``"string"``:           response must be non-empty and, if
                                ``schema.pattern`` is set, match the regex
                                (no JSON decoding — some specs emit a bare
                                token, e.g. ``jiminy.codegen``).
      * ``"number"``/``"integer"``: response must parse as a numeric primitive.
      * ``"boolean"``:          response must parse as ``true``/``false``.

    Prior behavior (``isinstance(parsed, dict)`` only) silently zero-credited
    every array/string-schema spec (``ape.reflect``, ``hidden.reclassify``,
    ``retrieval.rerank_cross``, ``jiminy.codegen``) when called via the
    Phase 10 registry path. Shadow-mode divergence audit caught it.
    """
    stype = (schema or {}).get("type", "object")
    if stype == "string":
        if not response or not response.strip():
            return 0.0
        pattern = (schema or {}).get("pattern")
        if pattern:
            import re
            return 1.0 if re.match(pattern, response.strip()) else 0.0
        return 1.0
    try:
        parsed = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return 0.0
    if stype == "array":
        return 1.0 if isinstance(parsed, list) else 0.0
    if stype in ("number", "integer"):
        return 1.0 if isinstance(parsed, (int, float)) and not isinstance(parsed, bool) else 0.0
    if stype == "boolean":
        return 1.0 if isinstance(parsed, bool) else 0.0
    # default: object
    return 1.0 if isinstance(parsed, dict) else 0.0


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


def _extract_classification(value: Any) -> str:
    """Return the classification label from a parsed value (dict, list, or str).

    Handles:
      * dict with ``type`` / ``classification`` / ``label`` key → that field
      * dict with ``types``: ["foo"] (list) → first element
      * list → first element as string
      * anything else → ``str(value).strip()``
    """
    if isinstance(value, dict):
        for key in ("type", "classification", "label"):
            if key in value:
                return str(value[key]).strip()
        # Plural form (retrieval.query_classify uses {"types": [...]})
        for key in ("types", "classifications", "labels"):
            v = value.get(key)
            if isinstance(v, list) and v:
                return str(v[0]).strip()
            if isinstance(v, str):
                return v.strip()
        return ""
    if isinstance(value, list) and value:
        return str(value[0]).strip()
    return str(value).strip()


def classification_accuracy(
    response: str,
    expected: str | None = None,
    **kwargs: Any,
) -> float:
    """Return 1.0 if predicted classification matches expected, 0.0 otherwise.

    Both ``response`` and ``expected`` are JSON strings from which a canonical
    classification label is extracted via ``_extract_classification``. Comparing
    parsed label to parsed label — not parsed label to the raw JSON string —
    prevents silent 0-credit when the model produces the exact target JSON.
    """
    if expected is None or expected == "":
        return json_valid(response)  # Can't verify without ground truth

    try:
        parsed_response = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    try:
        parsed_expected = json.loads(expected)
    except (json.JSONDecodeError, TypeError):
        # Caller passed a bare label string rather than JSON — use as-is.
        parsed_expected = expected

    predicted_label = _extract_classification(parsed_response).lower()
    expected_label = _extract_classification(parsed_expected).lower()
    if not expected_label:
        return json_valid(response)
    return 1.0 if predicted_label == expected_label else 0.0


def _extract_verdict(value: Any) -> str:
    """Return the verdict label from a parsed value (dict or scalar)."""
    if isinstance(value, dict):
        for key in ("verdict", "evaluation", "outcome", "decision"):
            if key in value:
                return str(value[key]).strip()
        return ""
    return str(value).strip()


def evaluation_accuracy(
    response: str,
    expected_verdict: str | None = None,
    expected: str | None = None,
    **kwargs: Any,
) -> float:
    """Reward for correct evaluation verdict (jiminy.evaluate / evaluate_llm).

    Accepts ``expected_verdict`` (legacy) OR ``expected`` (the benchmark-runner
    kwarg produced by ``_extract_reward_kwargs`` from the golden row's
    assistant message). When ``expected`` is a JSON string, its verdict is
    parsed out the same way as the response's — so comparing parsed label to
    parsed label, never parsed label to raw JSON.
    """
    target = expected_verdict if expected_verdict is not None else expected
    if target is None or target == "":
        return json_valid(response)

    try:
        parsed_response = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    try:
        parsed_target = json.loads(target)
    except (json.JSONDecodeError, TypeError):
        parsed_target = target

    response_verdict = _extract_verdict(parsed_response).lower()
    target_verdict = _extract_verdict(parsed_target).lower()
    if not target_verdict:
        return json_valid(response)
    return 1.0 if response_verdict == target_verdict else 0.0


# ── Quality Rewards ──


def coherence_score(response: str, **kwargs: Any) -> float:
    """Heuristic coherence score based on structure indicators."""
    if not response or not response.strip():
        return 0.0

    # REWARD-CORRECTNESS-001: length-neutral. A coherent single sentence is
    # not less coherent than two; the old >=2-sentence + 10-word gates
    # capped terse-correct answers at 0.5-0.7. A valid non-repetitive
    # response scores high; only pure repetition (real pathology) is penalized.
    text = response.strip()
    words = text.split()
    if words and len(set(words)) / len(words) < 0.3:
        return 0.4  # degenerate repetition
    return 0.9


def coverage_score(response: str, **kwargs: Any) -> float:
    """Substantiveness score — LENGTH-NEUTRAL (REWARD-CORRECTNESS-001).

    The old length ladder (<20 words→0.4, <50→0.7, then rising) dropped
    spec-correct-but-terse answers below the 0.8 distill gate AND rewarded
    verbosity upward — the corpus-skew mechanism behind the 3 discarded
    retrains. A substantive, valid response now scores high regardless of
    length; only empty/near-empty or pure-repetition content scores low.
    Verbosity is never rewarded past the substantiveness bar.
    """
    if not response or not response.strip():
        return 0.0

    words = response.strip().split()
    word_count = len(words)

    # Pure repetition is genuine pathology (templated/degenerate output).
    if word_count > 0 and len(set(words)) / word_count < 0.3:
        return 0.3
    if word_count < 3:
        return 0.4  # genuinely content-free
    return 0.9  # substantive content — length-neutral, no verbosity bonus


def summary_quality(response: str, **kwargs: Any) -> float:
    """Combined coherence + coverage for summary-type outputs.

    FT-CLASSIFY-002: a classify response of {"type": "none", "summary": ""}
    is CORRECT BY SPEC (the prompt mandates an empty summary when no
    constraint is expressed) — score it 1.0 instead of penalizing the
    empty summary. The old behavior capped every correct-none row at
    ~0.767, which made the reward>=0.8 distill filter silently reject the
    dominant production class (81.9% none) — the actual mechanism behind
    the 11.5d class-skew failure.
    """
    try:
        parsed = json.loads(response)
        if isinstance(parsed, dict) and parsed.get("type") == "none" \
                and not str(parsed.get("summary", "")).strip():
            return 1.0
    except (json.JSONDecodeError, TypeError):
        pass
    return (coherence_score(response) + coverage_score(response)) / 2


def _score_explanation_text(text: str) -> float:
    """Length-neutral (REWARD-CORRECTNESS-001): substantive explanation → 0.9,
    near-empty → 0.5. A correct concise explanation is not worth less than a
    verbose one."""
    words = str(text).split()
    return 0.5 if len(words) < 3 else 0.9


def explanation_quality(response: str, **kwargs: Any) -> float:
    """Reward for explanation quality (evaluation tasks).

    REWARD-CORRECTNESS-002: schema-aware. jiminy.evaluate / jiminy.evaluate_llm
    emit ``{"violations":[{"reasoning":...}], "warnings":[...]}`` — the
    reasoning is NESTED per-violation, not a top-level ``explanation``/
    ``reasoning`` field. The old flat-only lookup scored every such response
    0.0 (even correct ones), dragging the mean below the gate. Now: prefer a
    top-level explanation; else credit nested ``violations[]/warnings[]``
    reasoning; a valid structured verdict with no violations is a correct
    "no issues" answer and is not penalised for having nothing to explain.
    """
    if not response or not response.strip():
        return 0.0

    try:
        parsed = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        # Non-JSON: treat the whole response as the explanation text.
        return _score_explanation_text(response)

    if isinstance(parsed, dict):
        # 1. Top-level explanation / reasoning (flat schema).
        flat = parsed.get("explanation", parsed.get("reasoning", "")) or ""
        if flat:
            return _score_explanation_text(flat)

        # 2. Nested reasoning in violations[] / warnings[] (structured schema).
        nested: list[str] = []
        for key in ("violations", "warnings"):
            items = parsed.get(key)
            if isinstance(items, list):
                for it in items:
                    if isinstance(it, dict):
                        r = it.get("reasoning") or it.get("description") or it.get("explanation")
                        if r:
                            nested.append(str(r))
        if nested:
            avg_words = sum(len(r.split()) for r in nested) / len(nested)
            return 0.9 if avg_words >= 3 else 0.5

        # 3. Valid structured verdict with no violations/warnings is a correct
        #    "no issues" answer — nothing to explain, do not penalise.
        if "violations" in parsed or "warnings" in parsed:
            return 0.9

    # Non-dict JSON (e.g. a bare string) or dict with no recognised fields.
    if isinstance(parsed, str) and parsed.strip():
        return _score_explanation_text(parsed)
    return 0.0


def specificity_score(response: str, **kwargs: Any) -> float:
    """Reward specific guidance over generic text.

    REWARD-CORRECTNESS-002: substantive-floored. The old base of 0.5 dropped
    valid, concise guidance below the gate purely for lacking the ~6 hard-coded
    specific-indicator words — a brittle keyword bag, the jiminy.synthesize
    suppressor. Now a substantive, non-hedging response floors at 0.7;
    specific-indicator words are a bounded BONUS toward 1.0; hedging phrases
    and empty/repetition stay low. Specific-still-beats-generic ordering is
    preserved (the discrimination), but correctness is no longer punished for
    brevity / vocabulary.
    """
    if not response or not response.strip():
        return 0.0

    words = response.split()
    if words and len(set(words)) / len(words) < 0.3:
        return 0.3  # degenerate repetition

    text = response.lower()
    generic_phrases = ["in general", "it depends", "there are many", "various ways"]
    specific_indicators = ["must", "never", "always", "specifically", "because", "when"]
    generic_count = sum(1 for p in generic_phrases if p in text)
    specific_count = sum(1 for p in specific_indicators if p in text)

    base = 0.7  # substantive, valid content floor (was 0.5)
    base += min(specific_count, 3) * 0.1  # bounded bonus
    base -= generic_count * 0.15  # hedging penalty
    return max(0.3, min(1.0, base))


def actionability_score(response: str, **kwargs: Any) -> float:
    """Reward actionable insights with concrete recommendations.

    REWARD-CORRECTNESS-002: substantive-floored. The old 0.3 base needed ~3
    action words to clear a 0.7 bar, dropping valid concise insights. A
    substantive response now floors at 0.7; action-indicator words are a
    bounded BONUS toward 1.0; empty/repetition stay low.
    """
    if not response or not response.strip():
        return 0.0

    words = response.split()
    if words and len(set(words)) / len(words) < 0.3:
        return 0.3  # degenerate repetition

    text = response.lower()
    action_indicators = [
        "should", "recommend", "consider", "implement", "refactor",
        "add", "remove", "replace", "migrate", "update",
    ]
    hits = sum(1 for ind in action_indicators if ind in text)
    return min(1.0, 0.7 + min(hits, 2) * 0.15)  # floor 0.7, bounded bonus


def follow_rate(response: str, **kwargs: Any) -> float:
    """Proxy for guidance follow rate — rewards structured, clear guidance."""
    return (specificity_score(response) + actionability_score(response)) / 2


def insight_count(response: str, **kwargs: Any) -> float:
    """Reward based on number of distinct insights/points."""
    if not response or not response.strip():
        return 0.0

    # Count bullet points, numbered items, or sentences
    lines = [ln.strip() for ln in response.split("\n") if ln.strip()]
    points = sum(1 for ln in lines if ln.startswith(("-", "*", "•")) or
                 (len(ln) > 1 and ln[0].isdigit() and ln[1] in ".)" ))

    if points == 0:
        # Fall back to sentence count
        points = len([s for s in response.split(".") if s.strip()])

    # REWARD-CORRECTNESS-001: do NOT reward insight COUNT upward — that
    # rewarded bullet-spam and dropped a correct single-insight reflection
    # to 0.5 (ape.reflect, the largest target). One genuine insight is a
    # complete correct answer; more is not "better" for inclusion.
    if points >= 1:
        return 0.9
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


# ── Guardrail Rewards (Sprint FT-LORA-PHASE10.5a) ──


def _extract_violation_ids(value: Any) -> set[str]:
    """Extract the set of constraint_node_id strings from a guardrail.evaluate
    response or golden (per ULTS spec ``guardrail_evaluate.ults.json``).

    The spec's output_schema requires ``violations: [{constraint_node_id,
    description, rationale}, ...]``. Returns empty set when the structure
    is missing or malformed — callers distinguish "clean" from "malformed"
    via the surrounding json_valid path.
    """
    if not isinstance(value, dict):
        return set()
    violations = value.get("violations", [])
    if not isinstance(violations, list):
        return set()
    ids: set[str] = set()
    for v in violations:
        if isinstance(v, dict) and "constraint_node_id" in v:
            ids.add(str(v["constraint_node_id"]).strip())
    return ids


def violation_detection_accuracy(
    response: str,
    expected: str | None = None,
    **kwargs: Any,
) -> float:
    """F1 reward for correctly identifying guardrail constraint violations.

    Compares the set of ``constraint_node_id`` values under ``violations``
    in ``response`` against those in ``expected`` (golden). Returns F1 over
    those sets.

    Edge cases:
      * Both sets empty → 1.0 (correctly reported "no violations").
      * Exactly one side empty → 0.0 (missed or fabricated).
      * Invalid JSON → 0.0.
      * ``expected`` missing → fallback to json_valid (consistent with
        ``classification_accuracy`` / ``evaluation_accuracy``).
    """
    if expected is None or expected == "":
        return json_valid(response, schema={"type": "object"})

    try:
        parsed_response = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    try:
        parsed_expected = json.loads(expected)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    pred_ids = _extract_violation_ids(parsed_response)
    exp_ids = _extract_violation_ids(parsed_expected)

    if not exp_ids and not pred_ids:
        return 1.0
    if not exp_ids or not pred_ids:
        return 0.0

    tp = len(pred_ids & exp_ids)
    fp = len(pred_ids - exp_ids)
    fn = len(exp_ids - pred_ids)

    precision = tp / (tp + fp) if (tp + fp) else 0.0
    recall = tp / (tp + fn) if (tp + fn) else 0.0
    if precision + recall == 0:
        return 0.0
    return 2 * precision * recall / (precision + recall)


def false_positive_penalty(
    response: str,
    expected: str | None = None,
    **kwargs: Any,
) -> float:
    """Penalty score for fabricated violations (false positives).

    Returns 1.0 (no penalty) when every predicted ``constraint_node_id``
    is in the golden set; decays linearly to 0.0 as the share of predicted
    violations not present in golden grows. The spec system prompt biases
    toward "Pass" — this reward keeps the model honest about that bias.

    Behavior:
      * No predicted violations → 1.0 (no FPs possible).
      * Invalid JSON → 0.0 (structural failure dominates).
      * ``expected`` missing → fallback to json_valid.

    Score is ``1.0 - fp_count / len(pred_ids)``. Bounded in [0, 1].
    """
    if expected is None or expected == "":
        return json_valid(response, schema={"type": "object"})

    try:
        parsed_response = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    try:
        parsed_expected = json.loads(expected)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    pred_ids = _extract_violation_ids(parsed_response)
    exp_ids = _extract_violation_ids(parsed_expected)

    if not pred_ids:
        return 1.0

    fp = len(pred_ids - exp_ids)
    return max(0.0, 1.0 - fp / len(pred_ids))


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


# ── Documentation-recall rewards (Sprint CLAUDE-DOCS-TRAINING-004) ──


def _tokenize_words(text: str) -> list[str]:
    """Lowercase alphanumeric+underscore tokens; deterministic, no deps."""
    import re
    return re.findall(r"[A-Za-z0-9_]+", (text or "").lower())


def _bigrams(tokens: list[str]) -> set[tuple[str, str]]:
    return {(tokens[i], tokens[i + 1]) for i in range(len(tokens) - 1)}


def _extract_identifiers(text: str) -> set[str]:
    """Extract likely code identifiers from text.

    Recognises (in order): (a) backtick-wrapped tokens, (b) function calls
    `name(` where name has ≥2 chars, (c) PascalCase/CamelCase/mixed with any
    uppercase mid-word (`ClaudeSDKClient`, `queryOptions`), (d) snake_case with
    ≥1 underscore, (e) dotted access `foo.bar`. Strips wrapping punctuation.
    Filters natural-language lowercase words (single lowercase word = not id).
    """
    import re
    if not text:
        return set()
    ids: set[str] = set()
    # (a) backtick-wrapped
    for m in re.findall(r"`([^`\n]+)`", text):
        ids.add(m.strip())
    # (b) function-call form: word followed by ( — trim trailing (
    for m in re.finditer(r"\b([A-Za-z_][A-Za-z0-9_]{1,})\s*\(", text):
        ids.add(m.group(1))
    # (c) uppercase-anywhere-mid-word: any word with ≥2 uppercase letters
    #     or lowercase-first-then-uppercase (queryOptions, PascalCase, ALLCAPS)
    for m in re.finditer(r"\b([A-Za-z_][A-Za-z0-9_]*)\b", text):
        w = m.group(1)
        if sum(1 for c in w if c.isupper()) >= 2:
            ids.add(w)
        elif len(w) >= 3 and w[0].islower() and any(c.isupper() for c in w[1:]):
            ids.add(w)  # camelCase like queryOptions
    # (d) snake_case with ≥1 underscore
    for m in re.finditer(r"\b([A-Za-z_][A-Za-z0-9_]*_[A-Za-z0-9_]+)\b", text):
        ids.add(m.group(1))
    # (e) dotted access foo.bar (word.word, both len≥2)
    for m in re.finditer(r"\b([A-Za-z_][A-Za-z0-9_]{1,}(?:\.[A-Za-z_][A-Za-z0-9_]+)+)\b", text):
        ids.add(m.group(1))
    # normalize (strip trailing/leading whitespace + wrapping punct)
    normalized = {i.strip(" .,;:()[]{}\"'") for i in ids if i.strip()}
    return {i for i in normalized if i and len(i) >= 2}


def factuality_score(
    response: str, expected: str | None = None, target: str | None = None,
    **kwargs: Any,
) -> float:
    """Bigram-F1 overlap between response and reference (expected/target).

    Deterministic, no LLM judge. Range [0.0, 1.0]. When reference is missing
    returns 0.0 (contract: eval requires a golden target). This is a
    lightweight lexical proxy for factual overlap — for the true rubric
    (semantic factuality) an LLM-judge follow-up is disclosed as separate work.
    """
    ref = expected or target or ""
    if not ref or not response:
        return 0.0
    r_bi = _bigrams(_tokenize_words(response))
    e_bi = _bigrams(_tokenize_words(ref))
    if not r_bi or not e_bi:
        return 0.0
    overlap = r_bi & e_bi
    if not overlap:
        return 0.0
    precision = len(overlap) / len(r_bi)
    recall = len(overlap) / len(e_bi)
    return 2 * precision * recall / (precision + recall)


def citation_precision(
    response: str, expected: str | None = None, target: str | None = None,
    **kwargs: Any,
) -> float:
    """Precision of code-identifier citations vs reference.

    Extract identifier-shaped tokens (`Foo`, snake_case, dotted) from BOTH
    response and reference. Precision = |R ∩ E| / |R|. Returns 0.0 when
    response has no identifiers OR reference missing. Rewards models that
    only cite identifiers that actually appear in the reference (i.e. no
    hallucinated API names).
    """
    ref = expected or target or ""
    if not ref or not response:
        return 0.0
    r_ids = _extract_identifiers(response)
    e_ids = _extract_identifiers(ref)
    if not r_ids:
        return 0.0
    if not e_ids:
        return 0.0
    overlap = r_ids & e_ids
    return len(overlap) / len(r_ids)


def concision_ratio(
    response: str, expected: str | None = None, target: str | None = None,
    **kwargs: Any,
) -> float:
    """Ratio min(1.0, ref_len / response_len). Rewards responses ≤ reference length.

    Deterministic by character count. Floors at 0.1 so a truly empty response
    doesn't score 1.0 by virtue of being shorter than the reference. Returns
    0.0 when either side is empty (contract: eval requires both).
    """
    ref = expected or target or ""
    if not ref or not response:
        return 0.0
    ref_len = len(ref)
    resp_len = len(response)
    if resp_len == 0:
        return 0.0
    ratio = ref_len / resp_len
    return max(0.1, min(1.0, ratio))


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
    # Guardrail (Sprint FT-LORA-PHASE10.5a)
    "violation_detection_accuracy": violation_detection_accuracy,
    "false_positive_penalty": false_positive_penalty,
    # Performance
    "latency_reward": latency_reward,
    # Documentation recall (Sprint CLAUDE-DOCS-TRAINING-004)
    "factuality_score": factuality_score,
    "citation_precision": citation_precision,
    "concision_ratio": concision_ratio,
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
