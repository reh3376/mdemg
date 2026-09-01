#!/usr/bin/env python3
"""MDEMG-USAGE-LORA-001 Epic 4-supplemental — measure mdemg.usage capability.

The shipped `run_benchmark` reads holdout from `training_data/eval/valid_clean.jsonl`
which does NOT include mdemg.usage rows (mdemg_usage_v1 was carved AFTER
valid_clean was frozen). This targeted eval runs the frozen adapter against
`training_data/sft/mdemg_usage_v1/benchmark_holdout.jsonl` (121 rows held
out from the mdemg_usage_v1 corpus per Epic 1's 80/10/10 split) and
computes 3 factuality proxy metrics:

    - exact substring recall: fraction of assistant-answer sentences that
      appear verbatim in the model output (case-insensitive, whitespace-normalized)
    - keyword-set jaccard: overlap of top-K nouns/verbs between answer + output
    - length-ratio quality gate: penalise outputs that are <30% or >300% of
      the expected answer length (proxy for hallucination + truncation)

Aggregate = weighted mean of the three, per-row.

Endpoint: --mlx-base-url (default http://127.0.0.1:8103/v1 — bench-serve)
Model:    --mlx-model-name (default .local-models/qwen3-14b-4bit-base)

Output: `training_data/eval/mdemg_usage_lora_001_iter7200_mdemg_usage.json`
"""
from __future__ import annotations

import argparse
import json
import re
import statistics
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib import request as urlreq
from urllib.error import URLError


def call_llm(base_url: str, model: str, system: str, user: str,
             max_tokens: int, timeout_s: int) -> tuple[str, float, str]:
    """Return (response_text, latency_s, err)."""
    payload = {
        "model": model,
        "messages": [
            {"role": "system", "content": system},
            {"role": "user", "content": user},
        ],
        "max_tokens": max_tokens,
        "temperature": 0.0,
    }
    data = json.dumps(payload).encode("utf-8")
    req = urlreq.Request(
        base_url.rstrip("/") + "/chat/completions",
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    t0 = time.time()
    try:
        with urlreq.urlopen(req, timeout=timeout_s) as resp:
            body = json.loads(resp.read().decode("utf-8"))
        latency = time.time() - t0
        return body["choices"][0]["message"]["content"], latency, ""
    except URLError as e:
        return "", time.time() - t0, f"url:{e}"
    except Exception as e:
        return "", time.time() - t0, f"err:{type(e).__name__}:{e}"


_SENT_RE = re.compile(r"[.!?]\s+|[.!?]$|\n\n")


def sentences(s: str) -> list[str]:
    parts = [p.strip() for p in _SENT_RE.split(s) if p.strip()]
    return parts


_TOKEN_RE = re.compile(r"[A-Za-z0-9_]+")


def tokens(s: str) -> set[str]:
    return {t.lower() for t in _TOKEN_RE.findall(s) if len(t) > 3}


def normalize(s: str) -> str:
    return re.sub(r"\s+", " ", s.lower()).strip()


def score_row(expected: str, actual: str) -> dict:
    """3 metrics + aggregate; all in [0,1]."""
    # 1. Exact substring recall — sentences from expected present in actual
    exp_sents = sentences(expected)
    norm_actual = normalize(actual)
    if exp_sents:
        hit = sum(1 for s in exp_sents if normalize(s) in norm_actual)
        substr_recall = hit / len(exp_sents)
    else:
        substr_recall = 0.0

    # 2. Token jaccard
    exp_tok = tokens(expected)
    act_tok = tokens(actual)
    if exp_tok:
        j = len(exp_tok & act_tok) / len(exp_tok | act_tok) if (exp_tok | act_tok) else 0.0
    else:
        j = 0.0

    # 3. Length-ratio gate — 1.0 if 30% ≤ len_ratio ≤ 300%; degrades linearly outside
    len_e = len(expected)
    len_a = len(actual)
    if len_e == 0:
        length_gate = 0.0
    else:
        ratio = len_a / len_e
        if 0.30 <= ratio <= 3.00:
            length_gate = 1.0
        elif ratio < 0.30:
            length_gate = max(0.0, ratio / 0.30)
        else:  # > 3.00
            length_gate = max(0.0, 3.00 / ratio)

    # Aggregate: substr_recall gets highest weight (factuality > lexical > length)
    agg = 0.5 * substr_recall + 0.3 * j + 0.2 * length_gate

    return {
        "substr_recall": round(substr_recall, 4),
        "token_jaccard": round(j, 4),
        "length_gate": round(length_gate, 4),
        "aggregate": round(agg, 4),
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.strip().splitlines()[0])
    ap.add_argument("--holdout", type=Path,
                    default=Path("training_data/sft/mdemg_usage_v1/benchmark_holdout.jsonl"))
    ap.add_argument("--out", type=Path,
                    default=Path("training_data/eval/mdemg_usage_lora_001_iter7200_mdemg_usage.json"))
    ap.add_argument("--mlx-base-url", default="http://127.0.0.1:8103/v1")
    ap.add_argument("--mlx-model-name", default=".local-models/qwen3-14b-4bit-base")
    ap.add_argument("--max-tokens", type=int, default=1024)
    ap.add_argument("--timeout-s", type=int, default=180)
    ap.add_argument("--limit", type=int, default=0, help="cap rows (0=all)")
    args = ap.parse_args()

    if not args.holdout.exists():
        print(f"ERROR: {args.holdout} not found", file=sys.stderr)
        return 2

    rows = [json.loads(line) for line in args.holdout.open() if line.strip()]
    if args.limit > 0:
        rows = rows[:args.limit]

    print(f"Evaluating {len(rows)} rows against {args.mlx_base_url} (model={args.mlx_model_name})")
    print(f"Adapter under test: whatever bench-serve on that URL is serving")
    print()

    results: list[dict] = []
    started = datetime.now(timezone.utc)
    for i, r in enumerate(rows):
        msgs = r["messages"]
        system = msgs[0]["content"]
        user = msgs[1]["content"]
        expected = msgs[2]["content"]
        actual, latency, err = call_llm(
            args.mlx_base_url, args.mlx_model_name, system, user,
            args.max_tokens, args.timeout_s,
        )
        if err:
            print(f"  row {i+1}/{len(rows)} ERR ({latency:.1f}s): {err}", file=sys.stderr)
            results.append({
                "row_id": r["meta"].get("row_id"),
                "source_path": r["meta"].get("source_path"),
                "surface": r["meta"].get("source_surface"),
                "latency_s": round(latency, 2),
                "err": err,
                "expected_len": len(expected),
                "actual_len": 0,
                "metrics": {"substr_recall": 0.0, "token_jaccard": 0.0, "length_gate": 0.0, "aggregate": 0.0},
            })
            continue
        m = score_row(expected, actual)
        results.append({
            "row_id": r["meta"].get("row_id"),
            "source_path": r["meta"].get("source_path"),
            "surface": r["meta"].get("source_surface"),
            "latency_s": round(latency, 2),
            "expected_len": len(expected),
            "actual_len": len(actual),
            "metrics": m,
            "actual_preview": actual[:200],
            "expected_preview": expected[:200],
        })
        print(f"  row {i+1}/{len(rows)} ({latency:5.1f}s) agg={m['aggregate']:.3f} "
              f"(substr={m['substr_recall']:.2f}, jaccard={m['token_jaccard']:.2f}, "
              f"len={m['length_gate']:.2f}) surface={r['meta'].get('source_surface')}",
              file=sys.stderr)
    ended = datetime.now(timezone.utc)

    # Aggregate
    aggs = [r["metrics"]["aggregate"] for r in results]
    substrs = [r["metrics"]["substr_recall"] for r in results]
    jaccs = [r["metrics"]["token_jaccard"] for r in results]
    lens = [r["metrics"]["length_gate"] for r in results]
    err_count = sum(1 for r in results if r.get("err"))

    summary = {
        "sprint": "MDEMG-USAGE-LORA-001",
        "epic": "4-supplemental (mdemg.usage capability probe)",
        "adapter_under_test": "adapters/mdemg_usage_lora_001 (iter 7200 frozen)",
        "adapter_sha256": "de2675b58800fc0362db26785941806ecb99a514e93e1c4f4a1db11ffc81e8c6",
        "endpoint": args.mlx_base_url,
        "base_model": args.mlx_model_name,
        "holdout": str(args.holdout),
        "row_count": len(results),
        "err_count": err_count,
        "aggregate": {
            "mean": round(statistics.mean(aggs), 4) if aggs else 0.0,
            "median": round(statistics.median(aggs), 4) if aggs else 0.0,
            "stddev": round(statistics.stdev(aggs), 4) if len(aggs) > 1 else 0.0,
            "min": round(min(aggs), 4) if aggs else 0.0,
            "max": round(max(aggs), 4) if aggs else 0.0,
        },
        "per_metric_mean": {
            "substr_recall": round(statistics.mean(substrs), 4) if substrs else 0.0,
            "token_jaccard": round(statistics.mean(jaccs), 4) if jaccs else 0.0,
            "length_gate": round(statistics.mean(lens), 4) if lens else 0.0,
        },
        "per_surface": {},
        "started_at_utc": started.isoformat(),
        "ended_at_utc": ended.isoformat(),
        "wall_clock_min": round((ended - started).total_seconds() / 60, 1),
        "results": results,
    }

    # Per-surface breakdown
    by_surf: dict[str, list[float]] = {}
    for r in results:
        s = r.get("surface") or "unknown"
        by_surf.setdefault(s, []).append(r["metrics"]["aggregate"])
    for s, vals in by_surf.items():
        summary["per_surface"][s] = {
            "n": len(vals),
            "mean": round(statistics.mean(vals), 4),
            "min": round(min(vals), 4),
            "max": round(max(vals), 4),
        }

    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(summary, indent=2))
    print(f"\nWrote: {args.out}")
    print(f"Aggregate mean: {summary['aggregate']['mean']:.4f}  (median {summary['aggregate']['median']:.4f})")
    print(f"Wall: {summary['wall_clock_min']:.1f} min | errs: {err_count}/{len(results)}")
    print(f"Per surface:")
    for s, v in summary["per_surface"].items():
        print(f"  {s:<12} n={v['n']:>3}  mean={v['mean']:.3f}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
