"""X8 — Capture gpt-5.4-mini distillation dataset for Phase 11.5 Branch B.

Walks all matched golden rows for the 2 distillation tasks, samples
gpt-5.4-mini at C-group temp=0.7, computes deterministic reward, and
emits raw_responses.jsonl + a manifest. Post-processing into mlx_lm.lora
chat-format train/valid splits is a separate step (post_process_distill.py).

Design:
- Reuses neural/benchmarks/run_benchmark helpers (Spec, match_rows_for_spec,
  load_golden_rows, _extract_reward_kwargs, _mlx_chat) — single source of truth
  for prompt construction, schema validation, and reward computation.
- Reuses scripts/x7_gpt54mini_benchmark.openai_http_post — single source of truth
  for OpenAI HTTP transport (Authorization, kwarg stripping, max_tokens rename).
- Per (spec, row): N_SAMPLES_PER_ROW gpt-mini calls at temp=0.7, each captured.
- All raw responses saved (no in-flight reward filtering); post-processor
  applies the configured threshold so we can re-tune without re-paying OpenAI.

Cost estimate (defaults):
- 2 tasks × 5 rows × 10 samples = 100 calls
- ~500 input + ~50 output tokens/call ≈ $0.025 for gpt-5.4-mini
"""
from __future__ import annotations

import json
import os
import sys
import time
from dataclasses import asdict
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import yaml  # noqa: E402

from neural.benchmarks.run_benchmark import (  # noqa: E402
    Spec,
    _extract_reward_kwargs,
    _mlx_chat,
    load_golden_rows,
    match_rows_for_spec,
)
from neural.benchmarks.sampling_policy import resolve_sampling  # noqa: E402
from neural.training.reward_functions import compute_reward  # noqa: E402

# Reuse X7's OpenAI HTTP transport
sys.path.insert(0, str(Path(__file__).resolve().parent))
from x7_gpt54mini_benchmark import openai_http_post  # noqa: E402

# ---- defaults (CLI/env overridable) ----
DISTILL_TASKS_DEFAULT = ["retrieval.query_classify", "guardrail.evaluate"]
N_SAMPLES_PER_ROW_DEFAULT = 10
OUT_DIR_DEFAULT = Path("training_data/distill/phase11_5")
OPENAI_BASE_URL = "https://api.openai.com/v1"
OPENAI_MODEL = "gpt-5.4-mini"


def _now_ms() -> int:
    return int(time.time() * 1000)


def _cuid() -> str:
    """CUIDv2 for run_id (MEMORY: never UUID)."""
    try:
        from cuid2 import Cuid  # type: ignore[import-not-found]
        return Cuid(length=24).generate()
    except ImportError:
        # Fallback per memory_cuidv2_required.md — blake2b on time_ns
        import hashlib
        return hashlib.blake2b(str(time.time_ns()).encode(), digest_size=12).hexdigest()


def main() -> int:
    config_path = Path("configs/benchmark_phase10.yaml")
    config = yaml.safe_load(config_path.read_text())

    distill_tasks = os.environ.get("X8_TASKS", ",".join(DISTILL_TASKS_DEFAULT)).split(",")
    n_samples = int(os.environ.get("X8_N_SAMPLES", N_SAMPLES_PER_ROW_DEFAULT))
    out_dir = Path(os.environ.get("X8_OUT_DIR", str(OUT_DIR_DEFAULT)))
    out_dir.mkdir(parents=True, exist_ok=True)

    raw_path = out_dir / "raw_responses.jsonl"
    manifest_path = out_dir / "raw_manifest.json"

    # Discover specs
    specs_dir = Path(config["ults"]["specs_dir"])
    spec_paths = sorted(specs_dir.glob("*.ults.json"))
    specs_all = [Spec.load(p) for p in spec_paths]
    specs = [s for s in specs_all if s.task_name in distill_tasks]
    if not specs:
        print(f"ERROR: no specs matched {distill_tasks}", file=sys.stderr)
        return 1
    if len(specs) != len(distill_tasks):
        missing = set(distill_tasks) - {s.task_name for s in specs}
        print(f"WARNING: missing specs for {missing}", file=sys.stderr)

    golden = load_golden_rows(Path(config["golden_holdout"]["out_path"]))

    run_id = _cuid()
    started_at_ms = _now_ms()
    print(f"X8 run_id: {run_id}")
    print(f"  tasks: {[s.task_name for s in specs]}")
    print(f"  n_samples_per_row: {n_samples}")
    print(f"  out: {raw_path}")
    print()

    raw_lines: list[str] = []
    per_task_stats: dict[str, dict[str, Any]] = {}
    total_calls = 0

    for spec in specs:
        rows = match_rows_for_spec(spec, golden)
        if not rows:
            print(f"WARN: {spec.task_name} has no matched golden rows — skipping")
            continue

        sampling_kwargs = resolve_sampling(spec.raw, config)
        # Override temp ONLY if the C-group default isn't already 0.7;
        # config gives us C-group temp=0.7 already, so this is a passthrough.
        # We intentionally do NOT override — distillation should sample
        # with the same recipe production uses.
        per_task_stats[spec.task_name] = {
            "sampling_group": spec.sampling_group,
            "matched_rows": len(rows),
            "n_samples_per_row": n_samples,
            "calls_attempted": 0,
            "calls_succeeded": 0,
            "rewards": [],
            "errors": [],
        }

        for row_idx, row in enumerate(rows):
            messages = row.get("messages") or []
            invoke_messages = [m for m in messages if m.get("role") != "assistant"]
            reward_kwargs = _extract_reward_kwargs(spec, row)

            for sample_idx in range(n_samples):
                t0 = time.time()
                per_task_stats[spec.task_name]["calls_attempted"] += 1
                total_calls += 1
                try:
                    chat_out = _mlx_chat(
                        base_url=OPENAI_BASE_URL,
                        model=OPENAI_MODEL,
                        messages=invoke_messages,
                        sampling_kwargs=sampling_kwargs,
                        max_tokens=spec.performance_max_tokens,
                        timeout_s=60.0,
                        _http_post=openai_http_post,
                    )
                except Exception as e:  # noqa: BLE001
                    err = str(e)[:300]
                    per_task_stats[spec.task_name]["errors"].append({
                        "row_idx": row_idx, "sample_idx": sample_idx, "error": err,
                    })
                    print(f"  [ERR] {spec.task_name} row={row_idx} s={sample_idx}: {err[:120]}")
                    continue

                if isinstance(chat_out, tuple):
                    response, finish_reason, usage = chat_out
                else:
                    response, finish_reason, usage = chat_out, None, {}

                latency_ms = int((time.time() - t0) * 1000)

                reward_vector = compute_reward(
                    response, spec.reward_functions, **reward_kwargs
                )
                reward_score = (
                    sum(reward_vector.values()) / len(reward_vector)
                    if reward_vector else 0.0
                )

                line = {
                    "run_id": run_id,
                    "task_id": spec.task_name,
                    "sampling_group": spec.sampling_group,
                    "row_idx": row_idx,
                    "sample_idx": sample_idx,
                    "messages_in": invoke_messages,
                    "response_text": response,
                    "reward_vector": reward_vector,
                    "reward_score": reward_score,
                    "finish_reason": finish_reason,
                    "completion_tokens": usage.get("completion_tokens"),
                    "prompt_tokens": usage.get("prompt_tokens"),
                    "latency_ms": latency_ms,
                    "sampling_kwargs": sampling_kwargs,
                    "model": OPENAI_MODEL,
                }
                raw_lines.append(json.dumps(line))
                per_task_stats[spec.task_name]["calls_succeeded"] += 1
                per_task_stats[spec.task_name]["rewards"].append(reward_score)

                if total_calls % 10 == 0:
                    print(f"  ... {total_calls} calls, last reward={reward_score:.3f}")

    raw_path.write_text("\n".join(raw_lines) + "\n")

    # Manifest
    completed_at_ms = _now_ms()
    elapsed_s = (completed_at_ms - started_at_ms) / 1000.0

    def _stats(rs: list[float]) -> dict[str, Any]:
        if not rs:
            return {"n": 0}
        return {
            "n": len(rs),
            "mean": sum(rs) / len(rs),
            "min": min(rs),
            "max": max(rs),
            "ge_0.8": sum(1 for r in rs if r >= 0.8),
            "ge_0.7": sum(1 for r in rs if r >= 0.7),
            "ge_0.5": sum(1 for r in rs if r >= 0.5),
        }

    manifest = {
        "run_id": run_id,
        "started_at_ms": started_at_ms,
        "completed_at_ms": completed_at_ms,
        "elapsed_s": elapsed_s,
        "model": OPENAI_MODEL,
        "openai_base_url": OPENAI_BASE_URL,
        "tasks": distill_tasks,
        "n_samples_per_row": n_samples,
        "total_calls": total_calls,
        "per_task": {
            task: {
                "sampling_group": stats["sampling_group"],
                "matched_rows": stats["matched_rows"],
                "calls_attempted": stats["calls_attempted"],
                "calls_succeeded": stats["calls_succeeded"],
                "reward_stats": _stats(stats["rewards"]),
                "n_errors": len(stats["errors"]),
                "errors_sample": stats["errors"][:3],
            }
            for task, stats in per_task_stats.items()
        },
        "raw_path": str(raw_path),
    }
    manifest_path.write_text(json.dumps(manifest, indent=2))

    print()
    print(f"X8 complete: {total_calls} calls in {elapsed_s:.1f}s")
    print(f"  raw: {raw_path}")
    print(f"  manifest: {manifest_path}")
    print()
    for task, stats in per_task_stats.items():
        rs = _stats(stats["rewards"])
        print(f"  {task}: n={rs['n']} mean={rs.get('mean',0):.3f} >=0.8: {rs.get('ge_0.8',0)}/{rs['n']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
