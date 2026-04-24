"""Post-Training Evaluation for MDEMG LoRA Adapters.

Evaluates a fine-tuned model's responses against held-out test data,
scoring each ULTS task on its quality_metrics contract. Produces a
per-task report with weighted scores and pass/fail status.

Usage:
    python -m training.evaluate_ft \
        --test-data curated/v1/test.jsonl \
        --ults-dir docs/tests/ults/specs/ \
        --report eval_report.json

    # With live inference (sends prompts to mlx_lm.server):
    #   Baseline (untuned dense base):
    python -m training.evaluate_ft \
        --test-data curated/v1/test.jsonl \
        --ults-dir docs/tests/ults/specs/ \
        --base-url http://localhost:8101/v1 \
        --model mlx-community/Qwen3-14B-4bit \
        --report eval_report.json

    #   Merged MDEMG fine-tuned (post-Phase-5):
    python -m training.evaluate_ft \
        --test-data curated/v1/test.jsonl \
        --ults-dir docs/tests/ults/specs/ \
        --base-url http://localhost:8101/v1 \
        --model .local-models/qwen3-14b-mdemg-v1 \
        --report eval_report.json

Requirements:
    pip install -e ".[training]"
"""

import argparse
import json
import re
import sys
import time
from pathlib import Path
from typing import Any
from urllib.error import URLError
from urllib.request import Request, urlopen


# ── Schema Validation ──


def check_json_valid(response: str, schema: dict[str, Any] | None = None) -> float:
    """Return 1.0 if response is structurally valid against the ULTS schema.

    Schema-aware semantics (``schema.type``):
      * ``"object"`` (default if unspecified): response must parse to a dict.
      * ``"array"``:                          response must parse to a list.
      * ``"string"``:                         response must be non-empty and,
                                              if ``schema.pattern`` is set,
                                              match the regex. The string is
                                              taken as-is (no JSON decoding)
                                              because specs like jiminy.codegen
                                              instruct the model to emit a bare
                                              token with no quoting.
      * ``"number"``/``"integer"``/``"boolean"``: response must parse as that
                                              primitive.
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


def check_required_keys(response: str, schema: dict[str, Any]) -> float:
    """Return fraction of required keys present in JSON response."""
    required = schema.get("required", [])
    if not required:
        return 1.0

    try:
        parsed = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    if not isinstance(parsed, dict):
        return 0.0

    present = sum(1 for k in required if k in parsed)
    return present / len(required)


def check_non_empty(response: str) -> float:
    """Return 1.0 if response is non-empty, 0.0 otherwise."""
    return 1.0 if response and response.strip() else 0.0


def check_type_conformance(response: str, schema: dict[str, Any]) -> float:
    """Check if property types match schema expectations. Partial credit."""
    try:
        parsed = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    if not isinstance(parsed, dict):
        return 0.0

    properties = schema.get("properties", {})
    if not properties:
        return 1.0

    checks = 0
    passed = 0
    type_map = {
        "string": str,
        "number": (int, float),
        "integer": int,
        "boolean": bool,
        "array": list,
        "object": dict,
    }

    for key, prop_schema in properties.items():
        if key not in parsed:
            continue
        checks += 1
        expected_type = prop_schema.get("type")
        if expected_type and expected_type in type_map:
            if isinstance(parsed[key], type_map[expected_type]):
                passed += 1
        else:
            passed += 1  # No type constraint = pass

    return passed / checks if checks > 0 else 1.0


# ── Heuristic Metric Functions ──

# Filler words that indicate vague, non-specific language
_FILLER_WORDS = frozenset({
    "generally", "typically", "usually", "various", "several", "many",
    "some", "often", "sometimes", "perhaps", "maybe", "might", "could",
    "approximately", "roughly", "basically", "essentially", "somewhat",
})

# Pattern for concrete identifiers (camelCase, snake_case, dotted paths, hex)
_IDENTIFIER_RE = re.compile(
    r"[a-z]+[A-Z]\w*"           # camelCase
    r"|[a-z]\w*_\w+"            # snake_case
    r"|[\w./\\]{2,}\.\w{1,6}"   # file paths / dotted names
    r"|0x[0-9a-fA-F]+"          # hex literals
    r"|\b\d+\.\d+\b"           # decimal numbers
)

# Sentence-ending pattern for checking structure
_SENTENCE_END_RE = re.compile(r"[.!?]\s")


def coherence(response: str, schema: dict[str, Any]) -> float:
    """Score response coherence: sentence structure and content completeness.

    For JSON responses: checks parseability and field population.
    For free-text: checks sentence structure and minimum length.
    Returns 0.0 for empty, 0.0-1.0 for partial, higher for well-formed.
    """
    if not response or not response.strip():
        return 0.0

    text = response.strip()
    score = 0.0

    # Check if schema expects JSON
    if schema.get("type") == "object":
        try:
            parsed = json.loads(text)
            if not isinstance(parsed, dict):
                return 0.1
            # Base score for valid JSON
            score = 0.4
            # Check field population
            properties = schema.get("properties", {})
            if properties:
                populated = sum(
                    1 for k in properties
                    if k in parsed and parsed[k] is not None
                    and str(parsed[k]).strip() not in ("", "null", "N/A")
                )
                score += 0.6 * (populated / len(properties))
            else:
                score += 0.3 if parsed else 0.0
            return min(score, 1.0)
        except (json.JSONDecodeError, TypeError):
            return 0.1  # Expected JSON but got invalid

    # Free-text coherence
    words = text.split()
    word_count = len(words)

    if word_count < 3:
        return 0.1

    # Length component (up to 0.3)
    score += min(word_count / 50.0, 0.3)

    # Sentence structure: has sentence-ending punctuation (up to 0.3)
    sentence_ends = len(_SENTENCE_END_RE.findall(text))
    if text[-1] in ".!?":
        sentence_ends += 1
    if sentence_ends > 0:
        score += min(sentence_ends / 3.0, 0.3)

    # No self-contradiction marker (up to 0.2)
    contradiction_markers = ["however", "but actually", "on the other hand"]
    contradiction_count = sum(1 for m in contradiction_markers if m in text.lower())
    score += 0.2 * max(0, 1.0 - contradiction_count * 0.3)

    # Capitalization (first char uppercase = proper sentence) (up to 0.2)
    if text[0].isupper() or text[0] in '{"[':
        score += 0.2

    return min(score, 1.0)


def coverage(response: str, schema: dict[str, Any]) -> float:
    """Score schema coverage: what fraction of expected content is present.

    For JSON: fraction of properties present with substantive values.
    For free-text: fraction of property-name keywords appearing in response.
    Returns 0.0 for empty, 0.0-1.0 based on coverage fraction.
    """
    if not response or not response.strip():
        return 0.0

    text = response.strip()
    properties = schema.get("properties", {})

    # JSON response with schema properties
    if schema.get("type") == "object" and properties:
        try:
            parsed = json.loads(text)
            if not isinstance(parsed, dict):
                return 0.0
            substantive = 0
            for key, prop in properties.items():
                if key not in parsed:
                    continue
                val = parsed[key]
                prop_type = prop.get("type", "string")
                if prop_type == "string" and isinstance(val, str) and len(val) > 5:
                    substantive += 1
                elif prop_type in ("number", "integer") and isinstance(val, (int, float)) and val != 0:
                    substantive += 1
                elif prop_type == "boolean" and isinstance(val, bool):
                    substantive += 1
                elif prop_type == "array" and isinstance(val, list) and len(val) > 0:
                    substantive += 1
                elif prop_type == "object" and isinstance(val, dict) and len(val) > 0:
                    substantive += 1
            return substantive / len(properties)
        except (json.JSONDecodeError, TypeError):
            return 0.0

    # Free-text: check if response mentions property-name keywords
    if properties:
        lower_text = text.lower()
        mentioned = sum(
            1 for key in properties
            if key.lower().replace("_", " ") in lower_text
            or key.lower() in lower_text
        )
        return mentioned / len(properties)

    # No schema properties — score by response length as a proxy
    words = len(text.split())
    return min(words / 30.0, 1.0)


def specificity(response: str, schema: dict[str, Any]) -> float:
    """Score response specificity: concrete vs vague language.

    Measures density of identifiers, numbers, and paths vs filler words.
    Returns 0.0 for empty, low for vague, high for concrete responses.
    """
    if not response or not response.strip():
        return 0.0

    text = response.strip()
    words = text.lower().split()
    word_count = len(words)

    if word_count < 3:
        return 0.1

    # Count concrete identifiers
    identifiers = _IDENTIFIER_RE.findall(text)
    identifier_density = len(identifiers) / word_count

    # Count filler words
    filler_count = sum(1 for w in words if w.strip(".,;:!?") in _FILLER_WORDS)
    filler_density = filler_count / word_count

    # Score: identifier density contributes positively, filler negatively
    # Identifier component (up to 0.6): even 5% identifier density is good
    id_score = min(identifier_density / 0.05, 1.0) * 0.6

    # Anti-filler component (up to 0.4): lower filler = higher score
    filler_score = max(0, 1.0 - filler_density * 10) * 0.4

    return min(id_score + filler_score, 1.0)


def follow_rate(response: str, schema: dict[str, Any]) -> float:
    """Score instruction-following: schema adherence and format compliance.

    For JSON: checks type constraints and enum adherence.
    For free-text: checks structural compliance markers.
    Returns 0.0 for empty, 0.0-1.0 based on adherence.
    """
    if not response or not response.strip():
        return 0.0

    text = response.strip()

    # JSON schema adherence
    if schema.get("type") == "object":
        try:
            parsed = json.loads(text)
            if not isinstance(parsed, dict):
                return 0.1

            properties = schema.get("properties", {})
            if not properties:
                return 0.8  # Valid JSON, no schema to check against

            checks = 0
            passed = 0

            for key, prop in properties.items():
                if key not in parsed:
                    continue
                checks += 1
                val = parsed[key]

                # Enum check
                if "enum" in prop:
                    passed += 1 if val in prop["enum"] else 0
                    continue

                # Type check
                expected = prop.get("type")
                type_map = {
                    "string": str, "number": (int, float),
                    "integer": int, "boolean": bool,
                    "array": list, "object": dict,
                }
                if expected and expected in type_map:
                    passed += 1 if isinstance(val, type_map[expected]) else 0
                else:
                    passed += 1

            return passed / checks if checks > 0 else 0.8
        except (json.JSONDecodeError, TypeError):
            return 0.0  # Schema expects JSON but response isn't

    # Free-text: basic structural compliance
    score = 0.3  # Base score for non-empty response

    # Reasonable length (not too short, not repetitive)
    words = text.split()
    if len(words) >= 10:
        score += 0.2
    unique_ratio = len(set(w.lower() for w in words)) / len(words) if words else 0
    if unique_ratio >= 0.5:
        score += 0.2

    # Ends with proper punctuation
    if text[-1] in ".!?]})\"'":
        score += 0.15

    # Doesn't start with refusal patterns
    lower = text.lower()
    refusal_starts = ["i cannot", "i can't", "i'm unable", "as an ai"]
    if not any(lower.startswith(r) for r in refusal_starts):
        score += 0.15

    return min(score, 1.0)


# ── Metric Computation ──


# Maps metric names to evaluation functions.
# Each function: (response, schema) -> float in [0.0, 1.0]
METRIC_EVALUATORS: dict[str, Any] = {
    "json_valid": lambda resp, schema: check_json_valid(resp, schema),
    "accuracy": lambda resp, schema: check_required_keys(resp, schema),
    "precision": lambda resp, schema: check_type_conformance(resp, schema),
    "coherence": coherence,
    "coverage": coverage,
    "specificity": specificity,
    "follow_rate": follow_rate,
}


# Maps ULTS quality_metric names → reward_functions.REWARD_REGISTRY names.
# Used by the `--scorer=registry` path (Sprint FT-LORA-PHASE10 Epic 4).
# Unmapped metric names fall back to the heuristic evaluator for registry mode
# so new ULTS metrics don't break the registry path.
REGISTRY_NAME_MAP: dict[str, str] = {
    "json_valid": "json_valid",
    "accuracy": "classification_accuracy",
    "precision": "schema_match",
    "coherence": "coherence_score",
    "coverage": "coverage_score",
    "specificity": "specificity_score",
    "follow_rate": "follow_rate",
}


def _score_via_registry(
    response: str, schema: dict[str, Any], metric_name: str
) -> float | None:
    """Return registry score for metric, or None if unmapped.

    Imports lazily so that evaluate_ft.py does not hard-depend on the
    registry until the operator opts into --scorer={registry,dual}.
    """
    reg_name = REGISTRY_NAME_MAP.get(metric_name)
    if reg_name is None:
        return None
    try:
        from neural.training.reward_functions import compute_reward
    except ImportError:
        return None
    out = compute_reward(response, [reg_name], schema=schema)
    v = out.get(reg_name)
    if v is None:
        return None
    return float(v)


def _heuristic_score(
    response: str, schema: dict[str, Any], metric_name: str
) -> float:
    """Phase 5 heuristic path — bit-identical with the pre-Phase-10 logic."""
    schema_type = schema.get("type", "string")
    evaluator = METRIC_EVALUATORS.get(metric_name)
    if evaluator:
        return evaluator(response, schema)
    if schema_type == "object":
        return check_json_valid(response, schema)
    return check_non_empty(response)


def evaluate_response(
    response: str,
    schema: dict[str, Any],
    quality_metrics: list[dict[str, Any]],
    scorer: str = "heuristic",
) -> dict[str, Any]:
    """Evaluate a single response against ULTS quality_metrics.

    ``scorer`` is one of:
      * ``"heuristic"`` (default) — Phase 5 bit-identical path.
      * ``"registry"``  — score via ``reward_functions.REWARD_REGISTRY``
                          (mapping in ``REGISTRY_NAME_MAP``). Falls back to
                          heuristic for unmapped metrics.
      * ``"dual"``      — runs both; reports heuristic as the authoritative
                          score, attaches registry_score + delta for
                          shadow-parity diagnosis.

    Returns dict with per-metric scores, weighted aggregate, and pass/fail.
    """
    metric_results = {}

    for metric in quality_metrics:
        name = metric["name"]
        weight = metric.get("weight", 0)
        threshold = metric.get("threshold", 0)

        h_score = _heuristic_score(response, schema, name)
        if scorer == "heuristic":
            score = h_score
            shadow: dict[str, Any] = {}
        elif scorer == "registry":
            r = _score_via_registry(response, schema, name)
            score = h_score if r is None else r
            shadow = {"registry_unmapped": r is None} if r is None else {}
        elif scorer == "dual":
            r = _score_via_registry(response, schema, name)
            score = h_score  # heuristic remains authoritative in dual
            shadow = {
                "heuristic_score": h_score,
                "registry_score": r,
                "delta": abs(h_score - r) if r is not None else None,
                "registry_unmapped": r is None,
            }
        else:
            raise ValueError(f"unknown scorer: {scorer!r}")

        metric_results[name] = {
            "score": score,
            "weight": weight,
            "threshold": threshold,
            "met": score >= threshold,
        }
        if shadow:
            metric_results[name]["shadow"] = shadow

    # Weighted aggregate
    total_weight = sum(m.get("weight", 0) for m in quality_metrics)
    if total_weight > 0:
        weighted_score = sum(
            metric_results[m["name"]]["score"] * m.get("weight", 0)
            for m in quality_metrics
        ) / total_weight
    else:
        weighted_score = 0.0

    all_met = all(r["met"] for r in metric_results.values())

    return {
        "metrics": metric_results,
        "weighted_score": round(weighted_score, 4),
        "all_thresholds_met": all_met,
    }


# ── ULTS Spec Loading ──


def load_ults_specs(ults_dir: str) -> dict[str, dict[str, Any]]:
    """Load all ULTS specs keyed by task name."""
    specs = {}
    for spec_path in sorted(Path(ults_dir).glob("*.ults.json")):
        try:
            with open(spec_path) as f:
                spec = json.load(f)
            task_name = spec["task"]["name"]
            specs[task_name] = spec
        except (json.JSONDecodeError, KeyError):
            continue
    return specs


# ── Test Data Loading ──


def load_test_data(test_path: str) -> list[dict[str, Any]]:
    """Load test JSONL records."""
    records = []
    with open(test_path) as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))
    return records


def group_by_task(records: list[dict[str, Any]]) -> dict[str, list[dict[str, Any]]]:
    """Group test records by task_name.

    Supports both top-level ``task_name`` (legacy TSDB export format) and
    ``meta.task_name`` (FT-LORA-DATA curated format).
    """
    groups: dict[str, list[dict[str, Any]]] = {}
    for record in records:
        task = record.get("task_name") or record.get("meta", {}).get("task_name", "unknown")
        groups.setdefault(task, []).append(record)
    return groups


# ── Inference ──


def run_inference(
    base_url: str,
    model: str,
    system_prompt: str,
    user_prompt: str,
    max_tokens: int,
    chat_template_kwargs: dict[str, Any] | None = None,
) -> tuple[str | None, float, str | None]:
    """Send a completion request. Returns (content, latency_ms, error).

    ``chat_template_kwargs`` is forwarded verbatim to the MLX chat completions
    endpoint. For Qwen3.6 / Qwen3-14B this accepts ``{"enable_thinking": false}``
    to disable the ``<think>`` scratchpad on tasks where a JSON-structured
    final answer is expected (prevents scratchpad-pollution + max_tokens
    cutoffs that empty the ``content`` field). Dense Qwen3-14B is the post-
    2026-04-22 baseline; Qwen3.6 MoE is abandoned.
    """
    body = {
        "model": model,
        "messages": [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": user_prompt},
        ],
        "max_tokens": max_tokens,
        "temperature": 0.1,
    }
    if chat_template_kwargs:
        body["chat_template_kwargs"] = chat_template_kwargs

    data = json.dumps(body).encode()
    req = Request(
        f"{base_url}/chat/completions",
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    start = time.monotonic()
    try:
        with urlopen(req, timeout=60) as resp:
            result = json.loads(resp.read())
    except URLError as e:
        return None, 0, str(e)
    except TimeoutError:
        return None, 0, "timeout (60s)"

    elapsed_ms = (time.monotonic() - start) * 1000
    message = result["choices"][0]["message"]
    # MLX 0.31.2 returns ``reasoning`` (think-mode scratchpad) and/or ``content``
    # (final answer). Prefer ``content`` when present; fall back to ``reasoning``
    # if the model was truncated before emitting an answer block.
    content = message.get("content") or message.get("reasoning") or ""
    return content, elapsed_ms, None


# ── Evaluation Engine ──


def extract_response(record: dict[str, Any]) -> str:
    """Extract the assistant response from a test record.

    Supports both raw records (with 'response' field) and
    chat-format records (with 'messages' array).
    """
    # Raw TSDB export format
    if "response" in record:
        return record["response"] or ""

    # Chat format (from format_converter.py)
    messages = record.get("messages", [])
    for msg in reversed(messages):
        if msg.get("role") == "assistant":
            return msg.get("content", "")

    return ""


def extract_prompts(record: dict[str, Any]) -> tuple[str, str]:
    """Extract system and user prompts from a test record."""
    if "system_prompt" in record and "user_prompt" in record:
        return record["system_prompt"] or "", record["user_prompt"] or ""

    messages = record.get("messages", [])
    system = ""
    user = ""
    for msg in messages:
        if msg.get("role") == "system":
            system = msg.get("content", "")
        elif msg.get("role") == "user":
            user = msg.get("content", "")
    return system, user


def evaluate_task(
    task_name: str,
    records: list[dict[str, Any]],
    spec: dict[str, Any],
    base_url: str | None = None,
    model: str | None = None,
    scorer: str = "heuristic",
) -> dict[str, Any]:
    """Evaluate all records for a single ULTS task.

    If base_url is provided, runs live inference instead of using
    stored responses.
    """
    schema = spec.get("output_schema", {})
    quality_metrics = spec.get("quality_metrics", [])
    perf = spec.get("performance", {})
    max_tokens = perf.get("max_tokens", 3000)
    latency_budget = perf.get("latency_budget_ms", 15000)
    chat_template_kwargs = perf.get("chat_template_kwargs") or None

    per_record = []
    latencies = []

    for record in records:
        if base_url and model:
            system_prompt, user_prompt = extract_prompts(record)
            response, latency_ms, error = run_inference(
                base_url, model, system_prompt, user_prompt, max_tokens,
                chat_template_kwargs=chat_template_kwargs,
            )
            if error:
                per_record.append({
                    "error": error,
                    "weighted_score": 0.0,
                    "all_thresholds_met": False,
                })
                continue
            latencies.append(latency_ms)
        else:
            response = extract_response(record)
            latency_ms = record.get("latency_ms", 0)
            if latency_ms:
                latencies.append(latency_ms)

        result = evaluate_response(response, schema, quality_metrics, scorer=scorer)
        per_record.append(result)

    # Aggregate
    n = len(per_record)
    if n == 0:
        return {
            "task": task_name,
            "count": 0,
            "weighted_score": 0.0,
            "all_thresholds_met": False,
            "status": "no_data",
        }

    valid_scores = [r["weighted_score"] for r in per_record if "weighted_score" in r]
    avg_score = sum(valid_scores) / len(valid_scores) if valid_scores else 0.0
    all_met_count = sum(1 for r in per_record if r.get("all_thresholds_met", False))
    threshold_pass_rate = all_met_count / n

    # Per-metric aggregates
    metric_aggregates = {}
    for metric in quality_metrics:
        name = metric["name"]
        threshold = metric.get("threshold", 0)
        scores = [
            r["metrics"][name]["score"]
            for r in per_record
            if "metrics" in r and name in r.get("metrics", {})
        ]
        if scores:
            avg = sum(scores) / len(scores)
            metric_aggregates[name] = {
                "mean_score": round(avg, 4),
                "threshold": threshold,
                "met": avg >= threshold,
                "pass_rate": round(
                    sum(1 for s in scores if s >= threshold) / len(scores), 4
                ),
            }

    # Latency stats
    latency_stats = {}
    if latencies:
        latency_stats = {
            "mean_ms": round(sum(latencies) / len(latencies)),
            "max_ms": round(max(latencies)),
            "budget_ms": latency_budget,
            "within_budget": sum(1 for lat in latencies if lat <= latency_budget) / len(latencies),
        }

    out: dict[str, Any] = {
        "task": task_name,
        "count": n,
        "weighted_score": round(avg_score, 4),
        "threshold_pass_rate": round(threshold_pass_rate, 4),
        "metric_aggregates": metric_aggregates,
        "latency": latency_stats,
        "status": "evaluated",
    }

    # Epic 4 shadow diagnostic: when scorer=dual, compute per-metric mean delta
    # between heuristic and registry paths across records. Operator gate is
    # |delta| < 0.01 per metric (1% per plan §Epic 4).
    if scorer == "dual":
        shadow_agg: dict[str, Any] = {}
        for metric in quality_metrics:
            m = metric["name"]
            deltas: list[float] = []
            unmapped = 0
            for r in per_record:
                shadow = (r.get("metrics", {}).get(m, {}) or {}).get("shadow", {})
                if shadow.get("registry_unmapped"):
                    unmapped += 1
                    continue
                d = shadow.get("delta")
                if d is not None:
                    deltas.append(float(d))
            if deltas:
                shadow_agg[m] = {
                    "n_compared": len(deltas),
                    "n_unmapped": unmapped,
                    "mean_abs_delta": round(sum(deltas) / len(deltas), 6),
                    "max_abs_delta": round(max(deltas), 6),
                    "within_1pct": all(d < 0.01 for d in deltas),
                }
            else:
                shadow_agg[m] = {
                    "n_compared": 0,
                    "n_unmapped": unmapped,
                    "within_1pct": True,  # vacuously (no mapping)
                }
        out["shadow_parity"] = shadow_agg
    return out


def run_evaluation(
    test_path: str,
    ults_dir: str,
    base_url: str | None = None,
    model: str | None = None,
    scorer: str = "heuristic",
) -> dict[str, Any]:
    """Run full evaluation across all tasks.

    Returns a report dict with per-task results and overall summary.
    """
    specs = load_ults_specs(ults_dir)
    if not specs:
        print(f"ERROR: No ULTS specs found in {ults_dir}", file=sys.stderr)
        raise SystemExit(1)

    records = load_test_data(test_path)
    if not records:
        print(f"ERROR: No test records found in {test_path}", file=sys.stderr)
        raise SystemExit(1)

    grouped = group_by_task(records)

    print(f"Evaluating {len(records)} records across {len(grouped)} tasks")
    print(f"ULTS specs loaded: {len(specs)}")
    print(f"{'─' * 70}")
    print(f"{'Task':<35} {'Score':>7} {'Pass%':>7} {'N':>5} {'Status'}")
    print(f"{'─' * 70}")

    task_results = []

    for task_name in sorted(set(list(grouped.keys()) + list(specs.keys()))):
        task_records = grouped.get(task_name, [])
        spec = specs.get(task_name)

        if not spec:
            print(f"{task_name:<35} {'':>7} {'':>7} {len(task_records):>5} no ULTS spec")
            task_results.append({
                "task": task_name,
                "count": len(task_records),
                "status": "no_spec",
            })
            continue

        if not task_records:
            print(f"{task_name:<35} {'':>7} {'':>7} {'0':>5} no test data")
            task_results.append({
                "task": task_name,
                "count": 0,
                "status": "no_data",
            })
            continue

        result = evaluate_task(
            task_name, task_records, spec, base_url, model, scorer=scorer,
        )
        score_str = f"{result['weighted_score']:.4f}"
        pass_str = f"{result.get('threshold_pass_rate', 0):.1%}"
        print(f"{task_name:<35} {score_str:>7} {pass_str:>7} {result['count']:>5} evaluated")
        task_results.append(result)

    print(f"{'─' * 70}")

    # Overall summary
    evaluated = [r for r in task_results if r.get("status") == "evaluated"]
    if evaluated:
        overall_score = sum(r["weighted_score"] for r in evaluated) / len(evaluated)
        tasks_passing = sum(
            1 for r in evaluated if r.get("threshold_pass_rate", 0) >= 0.8
        )
    else:
        overall_score = 0.0
        tasks_passing = 0

    summary = {
        "total_tasks": len(task_results),
        "evaluated_tasks": len(evaluated),
        "overall_weighted_score": round(overall_score, 4),
        "tasks_passing_80pct": tasks_passing,
        "total_records": len(records),
    }

    print(f"Overall: {overall_score:.4f} weighted score, "
          f"{tasks_passing}/{len(evaluated)} tasks >= 80% threshold pass rate")

    report: dict[str, Any] = {
        "task_results": task_results,
        "summary": summary,
        "scorer": scorer,
    }

    # Epic 4: aggregate shadow parity across tasks when scorer=dual.
    # Exit status for the CLI is derived from this block (see main()).
    if scorer == "dual":
        shadow_summary: dict[str, Any] = {
            "divergences_gt_1pct": [],
            "all_metrics_compared": 0,
            "all_metrics_unmapped": 0,
        }
        for r in task_results:
            tname = r.get("task")
            sp = r.get("shadow_parity") or {}
            for metric_name, stats in sp.items():
                if stats.get("n_compared", 0) == 0:
                    shadow_summary["all_metrics_unmapped"] += 1
                    continue
                shadow_summary["all_metrics_compared"] += 1
                if not stats.get("within_1pct", False):
                    shadow_summary["divergences_gt_1pct"].append({
                        "task": tname,
                        "metric": metric_name,
                        "mean_abs_delta": stats.get("mean_abs_delta"),
                        "max_abs_delta": stats.get("max_abs_delta"),
                    })
        report["shadow_summary"] = shadow_summary
    return report


def main():
    parser = argparse.ArgumentParser(
        description="Evaluate LoRA fine-tuned model against ULTS specs",
    )
    parser.add_argument(
        "--test-data", required=True,
        help="Path to test.jsonl (held-out test split)",
    )
    parser.add_argument(
        "--ults-dir",
        default=str(Path(__file__).parent.parent / ".." / "docs" / "tests" / "ults" / "specs"),
        help="Directory containing *.ults.json specs",
    )
    parser.add_argument(
        "--base-url", default=None,
        help="vllm-mlx base URL for live inference (optional)",
    )
    parser.add_argument(
        "--model", default=None,
        help="Model name for live inference (required if --base-url set)",
    )
    parser.add_argument("--report", help="Write JSON report to file")
    parser.add_argument(
        "--scorer",
        choices=["heuristic", "registry", "dual"],
        default="heuristic",
        help=(
            "Evaluation scorer path (Sprint FT-LORA-PHASE10 Epic 4). "
            "'heuristic' = Phase 5 bit-identical default. "
            "'registry' = reward_functions.REWARD_REGISTRY path. "
            "'dual' = shadow-compare both; exit 2 on any |delta| >= 1% per metric."
        ),
    )
    args = parser.parse_args()

    if args.base_url and not args.model:
        parser.error("--model is required when --base-url is set")

    report = run_evaluation(
        test_path=args.test_data,
        ults_dir=args.ults_dir,
        base_url=args.base_url,
        model=args.model,
        scorer=args.scorer,
    )

    if args.report:
        with open(args.report, "w") as f:
            json.dump(report, f, indent=2)
        print(f"\nReport written to {args.report}")

    # Epic 4 shadow gate: in dual mode, any per-metric divergence > 1% is a
    # non-zero exit so the registry path can't silently become a new default
    # until parity is confirmed.
    if args.scorer == "dual":
        divs = (report.get("shadow_summary") or {}).get("divergences_gt_1pct", [])
        if divs:
            print(
                f"\nSHADOW PARITY FAIL: {len(divs)} metric(s) diverge by > 1%:",
                file=sys.stderr,
            )
            for d in divs:
                print(
                    f"  {d['task']}/{d['metric']}: "
                    f"mean|Δ|={d['mean_abs_delta']:.4f} "
                    f"max|Δ|={d['max_abs_delta']:.4f}",
                    file=sys.stderr,
                )
            sys.exit(2)


if __name__ == "__main__":
    main()
