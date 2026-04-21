#!/usr/bin/env python3
"""
openai_ft_baseline_eval.py — Evaluate a (base or fine-tuned) OpenAI model
against a held-out test set for side-by-side comparison.

Reads ``test.jsonl`` from the curated dataset, samples N records (default 300)
proportionally by task, strips each record's assistant message (the ground
truth), sends system+user prompts through ``--model``, captures the response,
and computes:

    1. Cosine similarity between response and ground-truth, via
       ``text-embedding-3-small`` (OpenAI embeddings).
    2. JSON-parseability pass rate (many mdemg tasks emit JSON; parse-fail
       is a clear regression signal).

Outputs ``results.jsonl`` (per-record) and ``summary.json`` (aggregates +
per-task breakdown) in ``--output-dir``.

Task attribution is recovered by joining each ``test.jsonl`` record against
the curated ``filtered.jsonl`` on
``sha256(system_prompt + user_prompt)`` — the same key quality_filter uses
for dedup. This is how we get proportional-by-task sampling and per-task
metrics when the versioned MLX records no longer carry ``task_name``.

Usage — base model (run BEFORE fine-tuning):
    python scripts/openai_ft_baseline_eval.py \\
        --test-jsonl training_data/curated/sft_interactions/versioned/test.jsonl \\
        --filtered-jsonl training_data/curated/sft_interactions/filtered.jsonl \\
        --model gpt-4.1-mini-2025-04-14 \\
        --sample-size 300 \\
        --output-dir training_data/openai_ft/20260420/eval/baseline \\
        --max-cost-usd 10

Usage — fine-tuned model (run AFTER fine-tuning completes, same seed):
    python scripts/openai_ft_baseline_eval.py \\
        --test-jsonl training_data/curated/sft_interactions/versioned/test.jsonl \\
        --filtered-jsonl training_data/curated/sft_interactions/filtered.jsonl \\
        --model ft:gpt-4.1-mini-2025-04-14:ORG::JOB_ID \\
        --sample-size 300 \\
        --sample-seed 42 \\
        --output-dir training_data/openai_ft/20260420/eval/ft \\
        --max-cost-usd 10
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import random
import sys
import time
from collections import defaultdict
from datetime import datetime, timezone

# Rough pricing (confirm at https://openai.com/api/pricing on run day).
# Used only to PRE-ABORT if the estimated spend exceeds --max-cost-usd.
# Not authoritative — actual spend is what the OpenAI bill says.
_PRICING_INPUT_PER_1M: dict[str, float] = {
    # Base models
    "gpt-4.1-mini": 0.40,
    "gpt-4.1-mini-2025-04-14": 0.40,
    "gpt-4o-mini": 0.15,
    "gpt-4o-mini-2024-07-18": 0.15,
    # Conservative overestimate for gpt-5.4-mini — tune once billing confirms.
    "gpt-5.4-mini": 0.80,
}
_PRICING_OUTPUT_PER_1M: dict[str, float] = {
    "gpt-4.1-mini": 1.60,
    "gpt-4.1-mini-2025-04-14": 1.60,
    "gpt-4o-mini": 0.60,
    "gpt-4o-mini-2024-07-18": 0.60,
    "gpt-5.4-mini": 3.20,
}
# Fine-tuned variants: use the base model's pricing key after the "ft:" prefix
# is stripped; OpenAI bills fine-tuned inference at ~3× the base rate.
_FT_MULTIPLIER = 3.0

_EMBED_MODEL = "text-embedding-3-small"
_EMBED_PRICE_PER_1M = 0.02


def _pricing_for_model(model: str) -> tuple[float, float]:
    """Return (input_per_1m, output_per_1m) for a model name."""
    is_ft = model.startswith("ft:")
    key = model.split(":")[1] if is_ft else model
    # Try exact then stripped-suffix matches.
    for m in (key, key.rsplit("-", 1)[0] if "-" in key else key):
        if m in _PRICING_INPUT_PER_1M:
            inp = _PRICING_INPUT_PER_1M[m]
            out = _PRICING_OUTPUT_PER_1M[m]
            if is_ft:
                inp *= _FT_MULTIPLIER
                out *= _FT_MULTIPLIER
            return inp, out
    # Fallback: assume gpt-4.1-mini pricing (safer over-estimate than zero).
    return 0.40, 1.60


# ---------------------------------------------------------------------------
# Dedup-key lookup — recover task_name for test records.
# ---------------------------------------------------------------------------


def _dedup_key(system_prompt: str, user_prompt: str) -> str:
    h = hashlib.sha256()
    h.update((system_prompt or "").encode("utf-8"))
    h.update(b"\x00")
    h.update((user_prompt or "").encode("utf-8"))
    return h.hexdigest()


def load_task_lookup(filtered_path: str) -> dict[str, str]:
    """Map sha256(system_prompt+user_prompt) → task_name, from filtered.jsonl."""
    lookup: dict[str, str] = {}
    with open(filtered_path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue
            sp = rec.get("system_prompt", "")
            up = rec.get("user_prompt", "")
            task = rec.get("task_name") or ""
            if sp or up:
                lookup[_dedup_key(sp, up)] = task
    return lookup


# ---------------------------------------------------------------------------
# Sampling — proportional by task.
# ---------------------------------------------------------------------------


def sample_proportional(
    records: list[dict],
    task_by_key: dict[str, str],
    n: int,
    seed: int,
) -> list[dict]:
    """Stratified proportional sample. Each record gets a task label (possibly
    empty string if unrecoverable). Draw from each bucket in proportion to its
    share of the population, rounding deterministically.
    """
    rng = random.Random(seed)

    # Attach task labels + bucket.
    buckets: dict[str, list[int]] = defaultdict(list)
    for i, rec in enumerate(records):
        msgs = rec.get("messages") or []
        sp = ""
        up = ""
        for m in msgs:
            if m.get("role") == "system":
                sp = m.get("content", "")
            elif m.get("role") == "user":
                up = m.get("content", "")
        task = task_by_key.get(_dedup_key(sp, up), "") or "__unattributed__"
        buckets[task].append(i)

    total = sum(len(v) for v in buckets.values())
    if total == 0 or n >= total:
        return list(records)

    # Allocate per-bucket quotas (floor), then distribute remainder by largest
    # fractional part for determinism.
    quotas_float = {
        t: (len(idxs) * n) / total for t, idxs in buckets.items()
    }
    quotas = {t: int(math.floor(q)) for t, q in quotas_float.items()}
    assigned = sum(quotas.values())
    remainder = n - assigned
    # Rank buckets by fractional leftover, stable.
    leftovers = sorted(
        buckets.keys(),
        key=lambda t: (-(quotas_float[t] - quotas[t]), t),
    )
    for t in leftovers[:remainder]:
        quotas[t] += 1

    chosen: list[int] = []
    for t, q in quotas.items():
        if q <= 0 or not buckets[t]:
            continue
        chosen.extend(rng.sample(buckets[t], min(q, len(buckets[t]))))
    chosen.sort()
    return [records[i] for i in chosen]


# ---------------------------------------------------------------------------
# Per-record evaluation.
# ---------------------------------------------------------------------------


def extract_prompts(record: dict) -> tuple[list[dict], str]:
    """Split a chat record into (messages_to_send, ground_truth_assistant)."""
    msgs = record.get("messages") or []
    to_send = []
    gt = ""
    for m in msgs:
        if m.get("role") == "assistant":
            gt = m.get("content", "") or gt
            # Stop at first assistant — we only replay through that point.
            break
        to_send.append({"role": m["role"], "content": m.get("content", "")})
    return to_send, gt


def try_parse_json(text: str) -> bool:
    """Lenient JSON-parseability check.

    Strips ```json ...``` fences and leading/trailing whitespace. Many tasks
    emit JSON inside a code fence; this is a low bar but catches obvious
    regressions where fine-tuning teaches the model to prepend chatter.
    """
    s = text.strip()
    if s.startswith("```"):
        # Drop optional language tag + fence.
        first_nl = s.find("\n")
        if first_nl >= 0:
            s = s[first_nl + 1 :]
        if s.endswith("```"):
            s = s[:-3]
        s = s.strip()
    try:
        json.loads(s)
        return True
    except (json.JSONDecodeError, ValueError):
        return False


def cosine(a: list[float], b: list[float]) -> float:
    dot = sum(x * y for x, y in zip(a, b))
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(y * y for y in b))
    if na == 0 or nb == 0:
        return 0.0
    return dot / (na * nb)


# ---------------------------------------------------------------------------
# A6 — none-type hallucination detection.
# Some UAITS tasks legitimately emit {"type":"none","summary":""} when there's
# nothing to report. If ground-truth is none-typed and the model responds with
# anything else, that's a hallucination (the model fabricated content).
# ---------------------------------------------------------------------------


def _is_none_type_payload(text: str) -> bool | None:
    """Return True if text parses to {"type":"none","summary":""},
    False if it parses to something else, None if it does not parse."""
    if not text:
        return None
    s = text.strip()
    if s.startswith("```"):
        first_nl = s.find("\n")
        if first_nl >= 0:
            s = s[first_nl + 1 :]
        if s.endswith("```"):
            s = s[:-3]
        s = s.strip()
    try:
        payload = json.loads(s)
    except (json.JSONDecodeError, ValueError):
        return None
    if not isinstance(payload, dict):
        return False
    return payload.get("type") == "none" and payload.get("summary", "") == ""


def hallucination_indicator(ground_truth: str, response: str) -> bool:
    """Flag records where ground-truth is none-typed but response is not.

    Returns False (not applicable) when ground_truth is not none-typed or
    when either side fails to parse — we only mark this when we can tell
    unambiguously that the model fabricated content against an explicit
    "nothing to report" ground truth.
    """
    gt_none = _is_none_type_payload(ground_truth)
    if gt_none is not True:
        return False
    resp_none = _is_none_type_payload(response)
    # If response is also none-typed → not hallucinating.
    # If response parses to something else → hallucinating.
    # If response fails to parse → still treat as hallucination (fabricated
    # non-conforming text where silence was expected).
    return resp_none is not True


def _percentile(xs: list[float], p: float) -> float | None:
    """Linear-interpolation percentile. Returns None for empty input."""
    if not xs:
        return None
    data = sorted(xs)
    if len(data) == 1:
        return data[0]
    k = (len(data) - 1) * (p / 100.0)
    lo = int(math.floor(k))
    hi = int(math.ceil(k))
    if lo == hi:
        return data[lo]
    return data[lo] + (data[hi] - data[lo]) * (k - lo)


def call_with_retry(
    client,
    *,
    model: str,
    messages: list[dict],
    max_completion_tokens: int,
    max_retries: int = 2,
    backoff_base_seconds: float = 2.0,
):
    """Wrap chat.completions.create with explicit retry counting.

    Returns (response, retry_count). Retries on any Exception the SDK raises
    (network, rate-limit, 5xx) with exponential backoff. Retry count is the
    number of retries performed (0 if the first attempt succeeded).
    """
    for attempt in range(max_retries + 1):
        try:
            resp = client.chat.completions.create(
                model=model,
                messages=messages,
                max_completion_tokens=max_completion_tokens,
            )
            return resp, attempt
        except Exception:  # noqa: BLE001 — SDK can raise many types
            if attempt >= max_retries:
                raise
            time.sleep(backoff_base_seconds ** (attempt + 1))
    # Unreachable — loop either returns or raises.
    raise RuntimeError("call_with_retry: exhausted retries without exception")


# ---------------------------------------------------------------------------
# Main.
# ---------------------------------------------------------------------------


def run(
    test_jsonl: str,
    filtered_jsonl: str,
    output_dir: str,
    model: str,
    sample_size: int,
    sample_seed: int,
    max_cost_usd: float,
    max_output_tokens: int,
    yes: bool,
    dry_run: bool,
) -> int:
    os.makedirs(output_dir, exist_ok=True)

    # 1. Load data.
    with open(test_jsonl, encoding="utf-8") as f:
        test_records = [json.loads(line) for line in f if line.strip()]
    print(f"loaded {len(test_records)} test records from {test_jsonl}", file=sys.stderr)

    task_by_key = load_task_lookup(filtered_jsonl)
    print(f"built task lookup with {len(task_by_key)} entries", file=sys.stderr)

    # 2. Sample.
    chosen = sample_proportional(test_records, task_by_key, sample_size, sample_seed)
    print(f"sampled {len(chosen)} records", file=sys.stderr)

    # 3. Cost estimate before network I/O.
    # Rough: avg input ~3K tokens, avg output ~1K tokens per record. Use
    # actual token counts from the records for input side.
    #
    # We approximate with len(content)/4 as a quick tiktoken-free fallback
    # if tiktoken isn't handy — NOT authoritative, just a sanity cap.
    total_input_chars = 0
    for rec in chosen:
        for m in rec.get("messages", []):
            if m.get("role") == "assistant":
                break
            total_input_chars += len(m.get("content", ""))
    est_input_tokens = total_input_chars / 4  # rough
    est_output_tokens = len(chosen) * max_output_tokens / 2  # half the cap, on average
    inp_per_1m, out_per_1m = _pricing_for_model(model)
    est_embed_tokens = 2 * len(chosen) * (
        (sum(len(m.get("content", "")) for rec in chosen for m in rec.get("messages", [])) / max(1, len(chosen))) / 4
    )
    est_cost = (
        (est_input_tokens / 1_000_000) * inp_per_1m
        + (est_output_tokens / 1_000_000) * out_per_1m
        + (est_embed_tokens / 1_000_000) * _EMBED_PRICE_PER_1M
    )
    est_cost = round(est_cost, 4)
    print(
        f"estimated cost: ~${est_cost:.2f} (cap ${max_cost_usd:.2f}, "
        f"pricing inp=${inp_per_1m}/1M out=${out_per_1m}/1M)",
        file=sys.stderr,
    )
    if est_cost > max_cost_usd:
        print(
            f"ABORT: estimated cost ${est_cost:.2f} exceeds --max-cost-usd ${max_cost_usd:.2f}",
            file=sys.stderr,
        )
        return 2

    if dry_run:
        # Still write the sample manifest so we can see which records would run.
        manifest = {
            "dry_run": True,
            "model": model,
            "sample_size": len(chosen),
            "sample_seed": sample_seed,
            "estimated_cost_usd": est_cost,
            "generated_at_utc": datetime.now(timezone.utc).isoformat(),
        }
        with open(os.path.join(output_dir, "summary.json"), "w", encoding="utf-8") as f:
            json.dump(manifest, f, indent=2)
        print("DRY-RUN: wrote summary.json with estimate; no network calls made.")
        return 0

    if not yes:
        reply = input(f"Proceed with ~${est_cost:.2f} spend? [y/N]: ").strip().lower()
        if reply not in {"y", "yes"}:
            print("ABORT: user declined.", file=sys.stderr)
            return 3

    # 4. Run.
    if not os.environ.get("OPENAI_API_KEY"):
        print("ABORT: OPENAI_API_KEY not set.", file=sys.stderr)
        return 4
    try:
        from openai import OpenAI
    except ImportError:
        print("ABORT: pip install 'openai>=1.50' required.", file=sys.stderr)
        return 5

    client = OpenAI()
    results_path = os.path.join(output_dir, "results.jsonl")
    summary_path = os.path.join(output_dir, "summary.json")

    per_task_cos: dict[str, list[float]] = defaultdict(list)
    per_task_parse: dict[str, list[bool]] = defaultdict(list)
    parse_pass = 0
    cos_values: list[float] = []
    # FT-OAI-002 A1–A7 aggregate accumulators
    latency_ms_values: list[int] = []
    retry_counts: list[int] = []
    truncation_flags: list[bool] = []
    hallucination_flags_where_applicable: list[bool] = []  # only none-typed GT records
    errors: list[dict] = []
    started = time.time()

    with open(results_path, "w", encoding="utf-8") as out_f:
        for i, rec in enumerate(chosen, 1):
            messages, gt = extract_prompts(rec)
            # Attribute task for breakdown.
            sp = next((m["content"] for m in messages if m["role"] == "system"), "")
            up = next((m["content"] for m in messages if m["role"] == "user"), "")
            task = task_by_key.get(_dedup_key(sp, up), "") or "__unattributed__"

            # A7: count input chars before any SDK call — always available.
            input_chars = sum(len(m.get("content", "") or "") for m in messages)

            # A1: wall-clock inference latency (chat completion only; not embed).
            t0 = time.perf_counter()
            # Inference with explicit retry counting (A2).
            try:
                resp, retry_count = call_with_retry(
                    client,
                    model=model,
                    messages=messages,
                    max_completion_tokens=max_output_tokens,
                )
                latency_ms = int((time.perf_counter() - t0) * 1000)
                choice = resp.choices[0]
                response_text = choice.message.content or ""
                # G1b: capture finish_reason so we can distinguish "length"
                # (truncation) from "stop" (natural end) per record.
                finish_reason = getattr(choice, "finish_reason", None)
                # G1c: capture per-record token usage. Usage may be absent on
                # error-paths; tolerate missing attributes.
                usage = getattr(resp, "usage", None)
                prompt_tokens = getattr(usage, "prompt_tokens", None) if usage else None
                completion_tokens = getattr(usage, "completion_tokens", None) if usage else None
                # A5: OpenAI request-id for incident correlation. The v1.50 SDK
                # exposes it as `_request_id` on the parsed response object.
                request_id = getattr(resp, "_request_id", None) or getattr(
                    resp, "request_id", None
                )
            except Exception as e:
                latency_ms = int((time.perf_counter() - t0) * 1000)
                errors.append({"i": i, "task": task, "error": str(e)[:300]})
                out_f.write(json.dumps({
                    "i": i, "task": task, "error": str(e)[:300],
                    "latency_ms": latency_ms,
                }) + "\n")
                continue

            # Embed both sides.
            embedding_model_version: str | None = None
            try:
                emb_resp = client.embeddings.create(
                    model=_EMBED_MODEL,
                    input=[gt or " ", response_text or " "],
                )
                gt_emb = emb_resp.data[0].embedding
                resp_emb = emb_resp.data[1].embedding
                sim = cosine(gt_emb, resp_emb)
                # A4: whatever the embedding service actually served. May differ
                # from _EMBED_MODEL (OpenAI sometimes pins a date suffix).
                embedding_model_version = getattr(emb_resp, "model", None) or _EMBED_MODEL
            except Exception as e:
                errors.append({"i": i, "task": task, "error": f"embed: {e!s}"[:300]})
                sim = float("nan")

            parses = try_parse_json(response_text)
            if parses:
                parse_pass += 1
            if not math.isnan(sim):
                cos_values.append(sim)
                per_task_cos[task].append(sim)
            per_task_parse[task].append(parses)

            # A3: truncation_flag — derived convenience on top of finish_reason.
            truncation_flag = finish_reason == "length"
            # A6: hallucination_indicator — fabrication against none-typed GT.
            hallucination_flag = hallucination_indicator(gt, response_text)
            # A7: output chars.
            output_chars = len(response_text)

            # Accumulate aggregates.
            latency_ms_values.append(latency_ms)
            retry_counts.append(retry_count)
            truncation_flags.append(truncation_flag)
            # Only records with none-typed GT contribute to the hallucination
            # rate denominator — otherwise the metric is meaningless.
            if _is_none_type_payload(gt) is True:
                hallucination_flags_where_applicable.append(hallucination_flag)

            out_f.write(json.dumps({
                "i": i,
                "task": task,
                "cosine": sim,
                # G1a: canonical name `parse_ok` (was `parses_as_json` pre-FT-OAI-002).
                # Emit both until all downstream tooling is migrated.
                "parse_ok": parses,
                "parses_as_json": parses,
                # G1b: finish_reason lets us flag "length" as truncation without
                # rerunning the eval.
                "finish_reason": finish_reason,
                # G1c: per-record token counts for cost / length distribution analysis.
                "prompt_tokens": prompt_tokens,
                "completion_tokens": completion_tokens,
                # FT-OAI-002 A1–A7 per-record metric fields.
                "latency_ms": latency_ms,                         # A1
                "retry_count": retry_count,                       # A2
                "truncation_flag": truncation_flag,               # A3
                "embedding_model_version": embedding_model_version,  # A4
                "request_id": request_id,                         # A5
                "hallucination_indicator": hallucination_flag,    # A6
                "input_chars": input_chars,                       # A7
                "output_chars": output_chars,                     # A7
                "ground_truth": gt,
                "response": response_text,
                "n_messages_in": len(messages),
            }, ensure_ascii=False) + "\n")

            if i % 25 == 0:
                elapsed = time.time() - started
                mean_cos = sum(cos_values) / max(1, len(cos_values))
                print(
                    f"  {i}/{len(chosen)} · mean_cos={mean_cos:.3f} · "
                    f"parse_pass={parse_pass}/{i} · elapsed={elapsed:.0f}s",
                    file=sys.stderr,
                )

    # 5. Summary.
    def _mean(xs): return sum(xs) / len(xs) if xs else None
    # FT-OAI-002 aggregate roll-ups for A1–A6.
    truncation_rate = (
        sum(1 for t in truncation_flags if t) / len(truncation_flags)
        if truncation_flags else None
    )
    hallucination_rate_on_none_gt = (
        sum(1 for h in hallucination_flags_where_applicable if h)
        / len(hallucination_flags_where_applicable)
        if hallucination_flags_where_applicable else None
    )
    latency_stats = {
        "mean_ms": _mean(latency_ms_values),
        "p50_ms": _percentile([float(x) for x in latency_ms_values], 50),
        "p95_ms": _percentile([float(x) for x in latency_ms_values], 95),
        "p99_ms": _percentile([float(x) for x in latency_ms_values], 99),
    } if latency_ms_values else None
    summary = {
        "generated_at_utc": datetime.now(timezone.utc).isoformat(),
        "model": model,
        "sample_size_requested": sample_size,
        "sample_size_actual": len(chosen),
        "sample_seed": sample_seed,
        "mean_cosine": _mean(cos_values),
        "parse_pass_rate": parse_pass / max(1, len(chosen)),
        "n_errors": len(errors),
        # FT-OAI-002 Epic 2: new aggregate fields.
        "latency_stats": latency_stats,
        "truncation_rate": truncation_rate,
        "mean_retries": _mean(retry_counts) if retry_counts else None,
        "hallucination_rate_on_none_gt": hallucination_rate_on_none_gt,
        "n_none_gt_records": len(hallucination_flags_where_applicable),
        "per_task": {
            t: {
                "n": len(per_task_cos[t]) or len(per_task_parse[t]),
                "mean_cosine": _mean(per_task_cos[t]),
                "parse_pass_rate": (
                    sum(per_task_parse[t]) / max(1, len(per_task_parse[t]))
                ),
            }
            for t in sorted(set(list(per_task_cos) + list(per_task_parse)))
        },
        "estimated_cost_usd": est_cost,
        "elapsed_seconds": round(time.time() - started, 1),
        "errors_preview": errors[:5],
    }
    with open(summary_path, "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=2)

    print(json.dumps({k: summary[k] for k in ("model", "sample_size_actual", "mean_cosine", "parse_pass_rate", "n_errors", "estimated_cost_usd", "elapsed_seconds")}, indent=2))
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--test-jsonl", required=True)
    parser.add_argument("--filtered-jsonl", required=True,
                        help="Curated filtered.jsonl — used to recover task_name by dedup key.")
    parser.add_argument("--output-dir", required=True)
    parser.add_argument("--model", default="gpt-4.1-mini-2025-04-14")
    parser.add_argument("--sample-size", type=int, default=300)
    parser.add_argument("--sample-seed", type=int, default=42,
                        help="Same seed → same sample. Use same value for baseline and FT runs.")
    parser.add_argument("--max-cost-usd", type=float, default=10.00)
    parser.add_argument("--max-output-tokens", type=int, default=1024)
    parser.add_argument("--yes", action="store_true")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    return run(
        test_jsonl=args.test_jsonl,
        filtered_jsonl=args.filtered_jsonl,
        output_dir=args.output_dir,
        model=args.model,
        sample_size=args.sample_size,
        sample_seed=args.sample_seed,
        max_cost_usd=args.max_cost_usd,
        max_output_tokens=args.max_output_tokens,
        yes=args.yes,
        dry_run=args.dry_run,
    )


if __name__ == "__main__":
    sys.exit(main())
