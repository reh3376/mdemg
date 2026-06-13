"""Phase 10 benchmark runner.

For each ULTS spec in docs/tests/ults/specs/, finds matching rows in the
golden validation holdout (matched by system_prompt_hash), invokes the model
under test N times, computes deterministic rewards via
neural.training.reward_functions.compute_reward(), and (optionally) calls
the LLM judge for subjective metrics. Aggregates per-task mean/stddev via
RunAggregator and writes a JSON report shaped to match the V0012 TSDB schema.

Design invariants:
  - Judge uses its own fixed sampling (see llm_judge.py) — never inherits
    task-under-test sampling kwargs.
  - MLX target is configured (default 127.0.0.1:8101). Runner never spawns
    or kills the MLX server.
  - CUIDv2 run_id (MEMORY).

Usage:
    python -m neural.benchmarks.run_benchmark \
        --config configs/benchmark_phase10.yaml \
        --out training_data/eval/benchmark_qwen3_14b_v1_baseline.json
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import time
from collections.abc import Callable
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

try:
    import yaml
except ImportError:  # pragma: no cover
    print("PyYAML required", file=sys.stderr)
    sys.exit(2)

from neural.benchmarks._ids import new_run_id
from neural.benchmarks.llm_judge import JudgeError, judge
from neural.benchmarks.sampling_policy import (
    SamplingPolicyError,
    group_weight,
    resolve_sampling,
)
from neural.benchmarks.variance import RunAggregator, RunSample
from neural.training.reward_functions import compute_reward


# ── MLX transport ────────────────────────────────────────────────────────────


class MLXError(RuntimeError):
    pass


def _mlx_chat(
    base_url: str,
    model: str,
    messages: list[dict[str, str]],
    sampling_kwargs: dict[str, Any],
    *,
    max_tokens: int,
    timeout_s: float,
    _http_post: Callable[..., bytes] | None = None,
) -> tuple[str, str | None, dict[str, Any]]:
    """POST to mlx_lm.server /chat/completions.

    Returns ``(content, finish_reason, usage)``:
      * ``finish_reason``: ``"stop"`` (natural end) / ``"length"``
        (hit ``max_tokens`` — TRUNCATION signal) / ``None`` (server
        didn't emit one). Callers must surface ``"length"`` in the
        per-row record so the operator can detect truncation-silent
        reward degradation.
      * ``usage``: OpenAI-style ``{prompt_tokens, completion_tokens,
        total_tokens}`` dict (may be partial / empty — best-effort).

    ``_http_post`` is a test hook with signature
    ``(url, headers, body, timeout) -> bytes``.
    """
    body: dict[str, Any] = {
        "model": model,
        "messages": messages,
        "max_tokens": max_tokens,
    }
    body.update(sampling_kwargs)

    url = f"{base_url.rstrip('/')}/chat/completions"
    headers = {"Content-Type": "application/json"}
    payload = json.dumps(body).encode("utf-8")

    if _http_post is not None:
        raw = _http_post(url, headers, payload, timeout_s)
    else:
        try:
            req = Request(url, data=payload, headers=headers, method="POST")
            with urlopen(req, timeout=timeout_s) as resp:  # noqa: S310
                raw = resp.read()
        except HTTPError as e:
            raise MLXError(f"MLX HTTP {e.code}: {e.reason}") from e
        except (URLError, TimeoutError) as e:
            raise MLXError(f"MLX network/timeout: {e}") from e

    try:
        data = json.loads(raw.decode("utf-8"))
        choice0 = data["choices"][0]
        content = choice0.get("message", {}).get("content") or ""
        finish_reason = choice0.get("finish_reason")
        usage = data.get("usage") or {}
        return content, finish_reason, usage
    except (json.JSONDecodeError, KeyError, IndexError) as e:
        raise MLXError(f"MLX malformed response: {e}") from e


# ── Spec & golden row handling ───────────────────────────────────────────────


@dataclass(frozen=True)
class Spec:
    path: Path
    task_name: str
    sampling_group: str
    reward_functions: list[str]
    quality_metrics: list[dict[str, Any]]
    system_prompt_hash: str | None
    dynamic_prompt: bool
    output_schema: dict[str, Any] | None
    performance_max_tokens: int
    performance_latency_ms: int
    raw: dict[str, Any]

    @classmethod
    def load(cls, path: Path) -> "Spec":
        raw = json.loads(path.read_text())
        perf = raw.get("performance") or {}
        return cls(
            path=path,
            task_name=raw.get("task", {}).get("name", path.stem),
            sampling_group=raw["sampling_group"],
            reward_functions=list(raw.get("reward_functions") or []),
            quality_metrics=list(raw.get("quality_metrics") or []),
            system_prompt_hash=(raw.get("prompt") or {}).get("system_prompt_hash"),
            dynamic_prompt=bool((raw.get("prompt") or {}).get("dynamic_prompt", False)),
            output_schema=raw.get("output_schema"),
            performance_max_tokens=int(perf.get("max_tokens", 3000)),
            performance_latency_ms=int(perf.get("latency_budget_ms", 15000)),
            raw=raw,
        )


def _sha256_text(s: str) -> str:
    return hashlib.sha256(s.encode("utf-8")).hexdigest()


def _count_by(rows: list[dict[str, Any]], key: str) -> dict[str, int]:
    """Count occurrences of ``row[key]`` across rows (None → 'unknown')."""
    counts: dict[str, int] = {}
    for r in rows:
        v = r.get(key)
        k = "unknown" if v is None else str(v)
        counts[k] = counts.get(k, 0) + 1
    return counts


def load_golden_rows(golden_path: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    with golden_path.open() as fh:
        for line in fh:
            line = line.strip()
            if line:
                rows.append(json.loads(line))
    return rows


def match_rows_for_spec(
    spec: Spec, rows: list[dict[str, Any]]
) -> list[dict[str, Any]]:
    """Select golden rows belonging to this spec.

    Primary match: row meta.task_name == spec.task_name (robust, survives
    prompt template edits). Fallback: system-message sha256 == spec hash,
    for rows without a meta.task_name field. Returns [] when neither signal
    is available — zero-matched specs are surfaced in the final report.
    """
    matched: list[dict[str, Any]] = []
    for row in rows:
        meta = row.get("meta") or {}
        row_task = meta.get("task_name")
        if row_task is not None:
            # Row has an authoritative task label — trust it; skip hash fallback
            # so specs that share a system prompt (e.g., jiminy.evaluate vs
            # jiminy.evaluate_llm) don't cross-match.
            if row_task == spec.task_name:
                matched.append(row)
            continue
        # Only for rows without meta.task_name, fall back to system-prompt hash
        if spec.system_prompt_hash and not spec.dynamic_prompt:
            msgs = row.get("messages") or []
            sys_msgs = [m for m in msgs if m.get("role") == "system"]
            if sys_msgs and _sha256_text(
                sys_msgs[0].get("content", "")
            ) == spec.system_prompt_hash:
                matched.append(row)
    return matched


def _extract_reward_kwargs(spec: Spec, row: dict[str, Any]) -> dict[str, Any]:
    kwargs: dict[str, Any] = {}
    if spec.output_schema:
        kwargs["schema"] = spec.output_schema
    # target/expected from the golden row's assistant message, when present
    msgs = row.get("messages") or []
    for m in msgs:
        if m.get("role") == "assistant":
            kwargs["target"] = m.get("content", "")
            kwargs["expected"] = m.get("content", "")
            break
    return kwargs


def _human_task_description(spec: Spec) -> str:
    t = spec.raw.get("task") or {}
    desc = t.get("description") or ""
    name = t.get("name") or spec.path.stem
    return f"{name}: {desc}".strip()


# ── Runner ───────────────────────────────────────────────────────────────────


@dataclass
class RunnerOptions:
    model_path: str
    mlx_base_url: str
    n_runs: int
    golden_path: Path
    specs_dir: Path
    task_filter: list[str] | None            # only run these task.name values
    enable_judge: bool
    judge_metrics: list[str]
    config: dict[str, Any]
    mlx_model_name: str
    persist_tsdb: bool                       # wiring deferred to Epic 3
    # Benchmark-wide HTTP timeout override (seconds). Spec.performance_latency_ms
    # is a production SLO, not a benchmark budget; benchmarking a 14B 4-bit
    # model on Metal needs more wall-clock per call. ``None`` → use spec budget
    # (backward-compatible). Set via --mlx-timeout-s at the CLI.
    mlx_timeout_s: float | None = None
    # Phase 11.5e row sweep: 0 = use ALL matched rows, K>0 = cap at K rows.
    # Default 0 (all rows) corrects the prior single-prompt-per-spec MVP. The
    # 11.5c clean baselines were measured with rows[0]-only behavior — those
    # reports remain on disk; new reports must be regenerated for direct
    # comparison.
    rows_per_spec: int = 0
    # test hooks
    mlx_http_post: Callable[..., bytes] | None = None
    judge_http_post: Callable[..., bytes] | None = None


def run(opts: RunnerOptions) -> dict[str, Any]:
    """Execute the benchmark and return a JSON-serializable report."""
    started_at = datetime.now(timezone.utc).isoformat()
    run_id = new_run_id()

    # Collect specs
    spec_paths = sorted(opts.specs_dir.glob("*.ults.json"))
    specs = [Spec.load(p) for p in spec_paths]
    if opts.task_filter:
        specs = [s for s in specs if s.task_name in opts.task_filter]

    golden_rows = load_golden_rows(opts.golden_path)

    aggregator = RunAggregator()
    benchmark_results_rows: list[dict[str, Any]] = []

    per_spec_info: dict[str, dict[str, Any]] = {}

    for spec in specs:
        rows = match_rows_for_spec(spec, golden_rows)
        matched_count = len(rows)
        per_spec_info[spec.task_name] = {
            "sampling_group": spec.sampling_group,
            "reward_functions": spec.reward_functions,
            "matched_golden_rows": matched_count,
        }
        if matched_count == 0:
            continue

        try:
            sampling_kwargs = resolve_sampling(spec.raw, opts.config)
        except SamplingPolicyError as e:
            per_spec_info[spec.task_name]["error"] = f"sampling_policy: {e}"
            continue

        # Phase 11.5e: row sweep — iterate all matched rows by default. The
        # prior behavior (rows[0] for N repeats) reduced any 20-row task to a
        # single-prompt benchmark with N stochastic samples — variance signal
        # only, not generalization signal. opts.rows_per_spec caps at K rows
        # (0 = all). Total per-task calls = min(K, len(rows)) × n_runs.
        rows_to_use = rows if opts.rows_per_spec <= 0 else rows[: opts.rows_per_spec]

        for row_idx, row in enumerate(rows_to_use):
            messages = row.get("messages") or []
            # Strip any assistant turn — we want the model to generate, not see, the target
            invoke_messages = [m for m in messages if m.get("role") != "assistant"]
            reward_kwargs = _extract_reward_kwargs(spec, row)

            for run_idx in range(opts.n_runs):
                t0 = time.time()
                try:
                    chat_out = _mlx_chat(
                        base_url=opts.mlx_base_url,
                        model=opts.mlx_model_name,
                        messages=invoke_messages,
                        sampling_kwargs=sampling_kwargs,
                        max_tokens=spec.performance_max_tokens,
                        timeout_s=(
                            opts.mlx_timeout_s
                            if opts.mlx_timeout_s is not None
                            else spec.performance_latency_ms / 1000.0
                        ),
                        _http_post=opts.mlx_http_post,
                    )
                except MLXError as e:
                    # Don't overwrite earlier successes on the same spec;
                    # accumulate per-row errors so the operator can spot a
                    # bad-prompt-row vs a model-wide failure.
                    errs = per_spec_info[spec.task_name].setdefault("row_errors", [])
                    errs.append({"row_idx": row_idx, "run_idx": run_idx, "error": f"mlx: {e}"})
                    continue

                # Back-compat: accept either the new (str, finish_reason, usage)
                # tuple or the legacy bare-string return (test hooks).
                if isinstance(chat_out, tuple):
                    response, finish_reason, usage = chat_out
                else:
                    response, finish_reason, usage = chat_out, None, {}

                latency_ms = int((time.time() - t0) * 1000)

                # Deterministic rewards
                reward_vector = compute_reward(
                    response, spec.reward_functions, **reward_kwargs
                )

                # Subjective judge (optional)
                judge_scores: dict[str, float] = {}
                judge_meta: dict[str, Any] = {}
                if opts.enable_judge:
                    task_desc = _human_task_description(spec)
                    for metric in opts.judge_metrics:
                        try:
                            jr = judge(
                                task_description=task_desc,
                                task_response=response,
                                metric=metric,
                                run_idx=run_idx,
                                config=opts.config,
                                _http_post=opts.judge_http_post,
                            )
                            judge_scores[metric] = jr.score
                            judge_meta[metric] = {
                                "model": jr.model,
                                "seed": jr.seed,
                                "prompt_template_sha": jr.prompt_template_sha,
                                "latency_ms": jr.latency_ms,
                            }
                        except JudgeError as e:
                            judge_meta[metric] = {"error": str(e)}

                # Aggregator key: encode row+run as a stable integer so per-task
                # variance reflects per-prompt diversity + per-prompt sampling.
                aggregator.add(RunSample(
                    task_id=spec.task_name,
                    run_idx=row_idx * opts.n_runs + run_idx,
                    reward_vector=reward_vector,
                    judge_scores=judge_scores,
                ))

                # Row shaped for V0012 TSDB benchmark_results.
                # ``finish_reason="length"`` marks a truncation — the operator
                # must be able to spot this without re-reading response_text.
                benchmark_results_rows.append({
                    "run_id": run_id,
                    "task_id": spec.task_name,
                    "row_idx": row_idx,
                    "run_idx": run_idx,
                    "sampling_group": spec.sampling_group,
                    "response_text": response,
                    "reward_vector": reward_vector,
                    "judge_scores": judge_scores,
                    "judge_meta": judge_meta,
                    "latency_ms": latency_ms,
                    "finish_reason": finish_reason,
                    "completion_tokens": usage.get("completion_tokens"),
                    "prompt_tokens": usage.get("prompt_tokens"),
                    "truncated": finish_reason == "length",
                })

    # Aggregate per task
    per_task_aggregate = [a.to_dict() for a in aggregator.aggregate_all()]

    # Group-weighted aggregate score
    # For each present group, compute mean of per-task overall_means, then weight
    group_means: dict[str, list[float]] = {"T": [], "C": [], "J": []}
    for spec in specs:
        info = per_spec_info.get(spec.task_name, {})
        if info.get("matched_golden_rows", 0) == 0 or "error" in info:
            continue
        try:
            t_agg = aggregator.aggregate(spec.task_name)
        except KeyError:
            continue
        group_means.setdefault(spec.sampling_group, []).append(t_agg.overall_mean)

    aggregate_weighted = 0.0
    weight_used = 0.0
    per_group_summary: dict[str, dict[str, Any]] = {}
    for group, vals in group_means.items():
        if not vals:
            per_group_summary[group] = {"n_tasks": 0}
            continue
        g_mean = sum(vals) / len(vals)
        w = group_weight(group, opts.config)
        aggregate_weighted += w * g_mean
        weight_used += w
        per_group_summary[group] = {
            "n_tasks": len(vals),
            "group_mean": g_mean,
            "weight": w,
        }

    # Normalize if some groups missing (so aggregate stays in [0,1])
    aggregate_weighted_normalized = (
        aggregate_weighted / weight_used if weight_used > 0 else 0.0
    )

    completed_at = datetime.now(timezone.utc).isoformat()

    report = {
        "run_id": run_id,
        "started_at": started_at,
        "completed_at": completed_at,
        "model_path": opts.model_path,
        "mlx_base_url": opts.mlx_base_url,
        "config_sha": _sha256_text(json.dumps(opts.config, sort_keys=True)),
        "n_runs_per_task": opts.n_runs,
        "specs_total": len(specs),
        "specs_with_matched_rows": sum(
            1 for v in per_spec_info.values() if v.get("matched_golden_rows", 0) > 0
        ),
        "specs_with_errors": sum(1 for v in per_spec_info.values() if "error" in v),
        "specs_with_zero_successful_calls": _count_zero_call_specs(specs, per_spec_info, aggregator),
        "per_spec": per_spec_info,
        "per_task_aggregate": per_task_aggregate,
        "per_group_summary": per_group_summary,
        "aggregate_weighted_score_raw": aggregate_weighted,
        "aggregate_weighted_score": aggregate_weighted_normalized,
        "benchmark_results_rows": benchmark_results_rows,
        "completion_summary": {
            "total_rows": len(benchmark_results_rows),
            "finish_reason_counts": _count_by(
                benchmark_results_rows, "finish_reason"
            ),
            "truncated_rows": [
                {"task_id": r["task_id"], "run_idx": r["run_idx"],
                 "completion_tokens": r.get("completion_tokens")}
                for r in benchmark_results_rows if r.get("truncated")
            ],
        },
    }

    # EVAL-INTEGRITY-001: hard-fail determination. A run that made zero
    # successful calls (or where no task contributed a score) previously
    # reported aggregate 0.0 — indistinguishable from a genuine low score.
    # Mark it as an error so the gate/CLI rejects it instead of treating a
    # broken run as a passed/failed model judgment.
    degenerate_reasons: list[str] = []
    if weight_used == 0:
        degenerate_reasons.append("no task contributed a score (weight_used==0)")
    if report["specs_with_zero_successful_calls"] > 0:
        degenerate_reasons.append(
            f"{report['specs_with_zero_successful_calls']} task(s) had matched rows but zero successful calls"
        )
    report["status"] = "error" if degenerate_reasons else "ok"
    report["degenerate_reasons"] = degenerate_reasons

    # Stagnation comparison against prior benchmark_runs, if history JSON provided
    # (for MVP, this compares against --history-dir)
    return report


# ── Stagnation ───────────────────────────────────────────────────────────────


def check_stagnation(
    this_aggregate: float,
    prior_aggregates: list[float],
    per_task_now: dict[str, float],
    per_task_prior: dict[str, dict[str, float]],
    config: dict[str, Any],
) -> dict[str, Any]:
    """Return stagnation flag + reasoning.

    Blocks only if len(prior_aggregates) >= config.stagnation.min_history - 1.
    """
    st = config["stagnation"]
    min_history = int(st["min_history"])
    agg_thresh = float(st["aggregate_delta_threshold"])
    task_thresh = float(st["per_task_regression_threshold"])

    history = [*prior_aggregates, this_aggregate]
    if len(history) < min_history:
        return {"flag": False, "reason": "insufficient_history", "history_len": len(history)}

    # rule 1: last N deltas all below threshold
    recent = history[-min_history:]
    deltas = [abs(recent[i + 1] - recent[i]) for i in range(len(recent) - 1)]
    all_below = all(d < agg_thresh for d in deltas)

    # rule 2: any task regressed > task_thresh vs most recent prior
    regressed: list[str] = []
    for tid, now_score in per_task_now.items():
        prior_scores_for_task = [
            per_task_prior[hid].get(tid)
            for hid in per_task_prior
            if tid in per_task_prior[hid]
        ]
        if not prior_scores_for_task:
            continue
        latest_prior = prior_scores_for_task[-1]
        if latest_prior is None:
            continue
        if latest_prior - now_score > task_thresh:
            regressed.append(tid)

    flag = bool(all_below or regressed)
    return {
        "flag": flag,
        "history_len": len(history),
        "recent_deltas": deltas,
        "all_deltas_below_threshold": all_below,
        "regressed_tasks": regressed,
        "threshold_aggregate": agg_thresh,
        "threshold_per_task": task_thresh,
    }


# ── CLI ──────────────────────────────────────────────────────────────────────


def _parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--config", type=Path,
                    default=Path("configs/benchmark_phase10.yaml"))
    ap.add_argument("--model", type=str, default=None,
                    help="override model_under_test.path")
    ap.add_argument("--mlx-base-url", type=str, default=None,
                    help="override http://<host>:<port>/v1 for mlx_lm.server")
    ap.add_argument("--mlx-model-name", type=str, default="local-mlx",
                    help="model name forwarded in /chat/completions body")
    ap.add_argument("--golden", type=Path, default=None,
                    help="override golden_holdout.out_path")
    ap.add_argument("--specs-dir", type=Path, default=None,
                    help="override ults.specs_dir")
    ap.add_argument("--out", type=Path, required=True,
                    help="output JSON report path")
    ap.add_argument("--task-filter", type=str, default="",
                    help="comma-separated task.name values to restrict the run")
    ap.add_argument("--n-runs", type=int, default=None,
                    help="override n_runs")
    ap.add_argument("--enable-judge", action="store_true",
                    help="also call the LLM judge (requires OPENAI_API_KEY)")
    ap.add_argument("--persist-tsdb", action="store_true",
                    help="(Epic 3 TBD) also write rows to TSDB V0012 tables")
    ap.add_argument("--mlx-timeout-s", type=float, default=None,
                    help="Benchmark-wide per-call timeout (seconds). "
                         "Overrides each spec's performance.latency_budget_ms "
                         "(which is the production SLO, not a benchmark budget). "
                         "Recommended for 14B models on Metal: 180.")
    ap.add_argument("--rows-per-spec", type=int, default=0,
                    help="Cap number of golden rows tested per spec. "
                         "0 (default) = ALL matched rows (Phase 11.5e correct behavior). "
                         "1 = legacy single-row × n_runs (the 11.5c MVP). "
                         "K>1 = first K rows × n_runs.")
    return ap.parse_args(argv)


def _count_zero_call_specs(specs, per_spec_info, aggregator) -> int:
    """Count specs that matched golden rows but produced zero successful calls
    (aggregate n==0) — the silent zero-call-scored-as-0.0 class (EVAL-INTEGRITY-001)."""
    n = 0
    for spec in specs:
        info = per_spec_info.get(spec.task_name, {})
        if info.get("matched_golden_rows", 0) == 0 or "error" in info:
            continue
        try:
            if aggregator.aggregate(spec.task_name).n == 0:
                n += 1
        except KeyError:
            n += 1
    return n


def main(argv: list[str] | None = None) -> int:
    ns = _parse_args(argv)
    if not ns.config.exists():
        print(f"config not found: {ns.config}", file=sys.stderr)
        return 2

    config = yaml.safe_load(ns.config.read_text())

    mut = config["model_under_test"]
    base_url = ns.mlx_base_url or f"http://{mut['mlx_host']}:{mut['mlx_port']}/v1"

    opts = RunnerOptions(
        model_path=ns.model or mut["path"],
        mlx_base_url=base_url,
        mlx_model_name=ns.mlx_model_name,
        n_runs=ns.n_runs if ns.n_runs is not None else int(config["n_runs"]),
        golden_path=ns.golden or Path(config["golden_holdout"]["out_path"]),
        specs_dir=ns.specs_dir or Path(config["ults"]["specs_dir"]),
        task_filter=[x for x in ns.task_filter.split(",") if x] or None,
        enable_judge=bool(ns.enable_judge),
        judge_metrics=list(config["judge"]["metrics"]),
        config=config,
        persist_tsdb=bool(ns.persist_tsdb),
        mlx_timeout_s=ns.mlx_timeout_s,
        rows_per_spec=int(ns.rows_per_spec),
    )
    if opts.enable_judge and not os.environ.get("OPENAI_API_KEY"):
        print("--enable-judge requested but OPENAI_API_KEY unset", file=sys.stderr)
        return 2

    report = run(opts)
    ns.out.parent.mkdir(parents=True, exist_ok=True)
    ns.out.write_text(json.dumps(report, indent=2, sort_keys=True))

    if opts.persist_tsdb:
        from neural.benchmarks.persist import write_sql_sidecar
        sql_path = ns.out.with_suffix(".sql")
        n = write_sql_sidecar(report, sql_path)
        print(f"tsdb sidecar written: {sql_path} ({n} statements)")
        print(f"  apply with: psql \"$TSDB_URL\" -f {sql_path}")

    print(f"aggregate_weighted_score = {report['aggregate_weighted_score']:.4f}")
    print(f"specs_with_matched_rows  = {report['specs_with_matched_rows']}/{report['specs_total']}")
    print(f"report written: {ns.out}")
    if report.get("status") == "error":
        print(f"BENCHMARK DEGENERATE — not a valid model judgment: {'; '.join(report['degenerate_reasons'])}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
