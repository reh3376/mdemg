"""Integration test: run_benchmark.run() against mocked MLX + mocked judge.

Covers Epic 2 gate:
  - Runner produces V0012-shaped rows (one per (task, run_idx)).
  - Per-task aggregate math matches RunAggregator output.
  - aggregate_weighted_score equals Σ(w_g * mean(per-task overall_mean))/Σ(w_g used)
    within 1e-6.
  - Stagnation check behaves as specified.
"""
from __future__ import annotations

import copy
import hashlib
import json
from pathlib import Path
from typing import Any

import pytest

from neural.benchmarks.run_benchmark import (
    RunnerOptions,
    check_stagnation,
    run,
)


# Mirrors configs/benchmark_phase10.yaml but small (n_runs=2, 3-task subset).
CONFIG: dict[str, Any] = {
    "version": "1.0.0",
    "n_runs": 2,
    "stagnation": {
        "min_history": 3,
        "aggregate_delta_threshold": 0.005,
        "per_task_regression_threshold": 0.02,
    },
    "sampling_groups": {
        "T": {"weight": 0.50, "temperature": 0.6, "top_p": 0.95, "top_k": 20},
        "C": {"weight": 0.35, "temperature": 0.7, "top_p": 0.8, "top_k": 20},
        "J": {
            "weight": 0.15,
            "temperature": 0.0,
            "top_p": 1.0,
            "top_k": -1,
            "presence_penalty": 1.5,
        },
    },
    "model_under_test": {
        "path": ".local-models/test-model",
        "mlx_host": "127.0.0.1",
        "mlx_port": 8101,
    },
    "judge": {
        "model": "gpt-5.4-mini",
        "temperature": 0.0,
        "seed_source": "run_idx",
        "max_tokens": 4000,
        "latency_budget_ms": 30000,
        "metrics": ["coherence", "depth", "relevance", "naturalness"],
        "prompt_dir": "neural/benchmarks/judge_prompts",
    },
    "ults": {
        "specs_dir": "docs/tests/ults/specs",
        "required_spec_fields": [
            "sampling_group", "reward_functions", "quality_metrics", "task.name",
        ],
        "allowed_sampling_groups": ["T", "C", "J"],
    },
    "golden_holdout": {
        "out_path": "training_data/eval/valid_golden.jsonl",
        "fraction": 0.15,
        "seed": 0,
    },
}


def _sha256(s: str) -> str:
    return hashlib.sha256(s.encode("utf-8")).hexdigest()


def _write_spec(dir_: Path, task_name: str, group: str, system_text: str) -> Path:
    spec = {
        "task": {"name": task_name, "description": f"test spec for {task_name}"},
        "sampling_group": group,
        "reward_functions": ["json_valid"],
        "quality_metrics": [
            {"name": "coherence", "threshold": 0.5},
        ],
        "prompt": {
            "system_prompt_hash": _sha256(system_text),
        },
        "output_schema": {"type": "object"},
        "performance": {
            "max_tokens": 3000,
            "latency_budget_ms": 15000,
        },
    }
    path = dir_ / f"{task_name}.ults.json"
    path.write_text(json.dumps(spec, indent=2))
    return path


def _write_golden(
    path: Path,
    rows: list[tuple[str, str, str]],  # (system, user, assistant)
) -> None:
    with path.open("w") as fh:
        for system, user, assistant in rows:
            row = {
                "messages": [
                    {"role": "system", "content": system},
                    {"role": "user", "content": user},
                    {"role": "assistant", "content": assistant},
                ],
            }
            fh.write(json.dumps(row) + "\n")


def _make_mlx_mock(content: str):
    """Return a test hook for MLX /chat/completions that always emits ``content``."""
    calls: list[dict] = []

    def _post(url, headers, body, timeout):
        calls.append({
            "url": url,
            "body": json.loads(body.decode("utf-8")),
            "timeout": timeout,
        })
        payload = {
            "choices": [{"message": {"content": content}}],
            "usage": {"prompt_tokens": 50, "completion_tokens": 20},
        }
        return json.dumps(payload).encode("utf-8")

    _post.calls = calls  # type: ignore[attr-defined]
    return _post


def _make_judge_mock(score: float):
    def _post(url, headers, body, timeout):
        payload = {
            "choices": [
                {"message": {"content": json.dumps({
                    "score": score, "rationale": "mocked",
                })}}
            ],
            "usage": {"prompt_tokens": 100, "completion_tokens": 50},
        }
        return json.dumps(payload).encode("utf-8")
    return _post


# ── Fixtures ─────────────────────────────────────────────────────────────────


@pytest.fixture
def bench_env(tmp_path: Path) -> dict[str, Any]:
    specs_dir = tmp_path / "specs"
    specs_dir.mkdir()
    golden_path = tmp_path / "valid_golden.jsonl"

    # Three tasks — one per group — each with a distinct system prompt so
    # match_rows_for_spec can sha256-match deterministically.
    sys_t = "You are the T-group task system. Think, then answer."
    sys_c = "You are the C-group classifier. Label only."
    sys_j = "You are the J-group structured extractor."

    _write_spec(specs_dir, "task_t", "T", sys_t)
    _write_spec(specs_dir, "task_c", "C", sys_c)
    _write_spec(specs_dir, "task_j", "J", sys_j)

    _write_golden(golden_path, [
        (sys_t, "t user msg", '{"result": "t_target"}'),
        (sys_c, "c user msg", '{"label": "A"}'),
        (sys_j, "j user msg", '{"extracted": "value"}'),
    ])

    return {
        "specs_dir": specs_dir,
        "golden_path": golden_path,
    }


# ── Tests ────────────────────────────────────────────────────────────────────


def test_runner_produces_vrows_shape(bench_env) -> None:
    mlx_post = _make_mlx_mock('{"ok": true}')
    opts = RunnerOptions(
        model_path=".local-models/test-model",
        mlx_base_url="http://127.0.0.1:8101/v1",
        mlx_model_name="test",
        n_runs=2,
        golden_path=bench_env["golden_path"],
        specs_dir=bench_env["specs_dir"],
        task_filter=None,
        enable_judge=False,
        judge_metrics=CONFIG["judge"]["metrics"],
        config=copy.deepcopy(CONFIG),
        persist_tsdb=False,
        mlx_http_post=mlx_post,
    )
    report = run(opts)

    # 3 specs × 2 runs = 6 rows
    rows = report["benchmark_results_rows"]
    assert len(rows) == 6

    required_keys = {
        "run_id", "task_id", "run_idx", "sampling_group",
        "response_text", "reward_vector", "judge_scores",
        "judge_meta", "latency_ms",
    }
    for r in rows:
        assert required_keys <= set(r.keys()), f"missing keys: {required_keys - set(r.keys())}"
        assert r["response_text"] == '{"ok": true}'
        # json_valid on '{"ok": true}' == 1.0
        assert r["reward_vector"]["json_valid"] == pytest.approx(1.0)
        # Judge disabled
        assert r["judge_scores"] == {}
        assert r["judge_meta"] == {}

    task_ids = {r["task_id"] for r in rows}
    assert task_ids == {"task_t", "task_c", "task_j"}


def test_runner_aggregate_weighted_math(bench_env) -> None:
    """aggregate_weighted_score matches (Σ w·group_mean)/Σ w within 1e-6."""
    mlx_post = _make_mlx_mock('{"ok": true}')
    opts = RunnerOptions(
        model_path=".local-models/test-model",
        mlx_base_url="http://127.0.0.1:8101/v1",
        mlx_model_name="test",
        n_runs=2,
        golden_path=bench_env["golden_path"],
        specs_dir=bench_env["specs_dir"],
        task_filter=None,
        enable_judge=False,
        judge_metrics=CONFIG["judge"]["metrics"],
        config=copy.deepcopy(CONFIG),
        persist_tsdb=False,
        mlx_http_post=mlx_post,
    )
    report = run(opts)

    # All responses valid JSON → every task overall_mean = 1.0
    # Group means: T=1.0, C=1.0, J=1.0; weights: 0.5/0.35/0.15 → sum weighted = 1.0,
    # weight_used = 1.0, aggregate_weighted_score_normalized = 1.0.
    assert report["aggregate_weighted_score"] == pytest.approx(1.0, abs=1e-6)
    assert report["aggregate_weighted_score_raw"] == pytest.approx(1.0, abs=1e-6)

    # Cross-check per-group summary
    summary = report["per_group_summary"]
    for g in ("T", "C", "J"):
        assert summary[g]["n_tasks"] == 1
        assert summary[g]["group_mean"] == pytest.approx(1.0)

    # Manual re-derivation
    total = 0.0
    weight_used = 0.0
    for g in ("T", "C", "J"):
        w = CONFIG["sampling_groups"][g]["weight"]
        total += w * summary[g]["group_mean"]
        weight_used += w
    assert report["aggregate_weighted_score"] == pytest.approx(
        total / weight_used, abs=1e-6
    )


def test_runner_mixed_responses_per_task_stats(bench_env) -> None:
    """Task T gets a broken JSON response → json_valid == 0 for T; C/J still 1."""
    # We'll return two distinct contents for different runs by side-effecting a counter.
    calls: list[dict] = []
    responses_iter = iter([
        # task_c run 0, task_c run 1
        '{"label": "A"}', '{"label": "A"}',
        # task_j run 0, task_j run 1
        '{"extracted": "x"}', '{"extracted": "y"}',
        # task_t run 0, task_t run 1
        "not-json", "not-json either",
    ])
    # The runner iterates sorted(specs_dir.glob("*.ults.json")) so glob order
    # is alphabetical → task_c, task_j, task_t. Responses above follow that.

    def mlx_post(url, headers, body, timeout):
        calls.append({"body": json.loads(body.decode("utf-8"))})
        content = next(responses_iter)
        return json.dumps({
            "choices": [{"message": {"content": content}}],
        }).encode("utf-8")

    opts = RunnerOptions(
        model_path=".local-models/test-model",
        mlx_base_url="http://127.0.0.1:8101/v1",
        mlx_model_name="test",
        n_runs=2,
        golden_path=bench_env["golden_path"],
        specs_dir=bench_env["specs_dir"],
        task_filter=None,
        enable_judge=False,
        judge_metrics=CONFIG["judge"]["metrics"],
        config=copy.deepcopy(CONFIG),
        persist_tsdb=False,
        mlx_http_post=mlx_post,
    )
    report = run(opts)
    per_task = {a["task_id"]: a for a in report["per_task_aggregate"]}
    assert per_task["task_c"]["overall_mean"] == pytest.approx(1.0)
    assert per_task["task_j"]["overall_mean"] == pytest.approx(1.0)
    assert per_task["task_t"]["overall_mean"] == pytest.approx(0.0)

    # Group means: T=0, C=1, J=1 → weighted_raw = 0.5*0 + 0.35*1 + 0.15*1 = 0.5
    # normalized = 0.5 / 1.0 = 0.5
    assert report["aggregate_weighted_score"] == pytest.approx(0.5, abs=1e-6)


def test_runner_judge_enabled_populates_judge_scores(bench_env) -> None:
    mlx_post = _make_mlx_mock('{"ok": true}')
    judge_post = _make_judge_mock(0.77)

    opts = RunnerOptions(
        model_path=".local-models/test-model",
        mlx_base_url="http://127.0.0.1:8101/v1",
        mlx_model_name="test",
        n_runs=1,
        golden_path=bench_env["golden_path"],
        specs_dir=bench_env["specs_dir"],
        task_filter=["task_t"],  # single task — cheaper
        enable_judge=True,
        judge_metrics=["coherence", "depth"],
        config=copy.deepcopy(CONFIG),
        persist_tsdb=False,
        mlx_http_post=mlx_post,
        judge_http_post=judge_post,
    )
    # Judge requires api_key in env OR passed explicitly; we stub by setting env
    # (the mock consumes body + returns fixed payload without auth checks).
    import os
    os.environ.setdefault("OPENAI_API_KEY", "sk-test-mock")

    report = run(opts)
    rows = report["benchmark_results_rows"]
    assert len(rows) == 1
    r = rows[0]
    assert set(r["judge_scores"].keys()) == {"coherence", "depth"}
    assert r["judge_scores"]["coherence"] == pytest.approx(0.77)
    assert r["judge_scores"]["depth"] == pytest.approx(0.77)
    # Meta records model + seed + template sha
    for metric in ("coherence", "depth"):
        meta = r["judge_meta"][metric]
        assert meta["model"] == "gpt-5.4-mini"
        assert meta["seed"] == 0
        assert isinstance(meta["prompt_template_sha"], str)


def test_runner_reports_unmatched_spec_when_hash_absent(tmp_path: Path) -> None:
    specs_dir = tmp_path / "specs"
    specs_dir.mkdir()
    golden_path = tmp_path / "g.jsonl"

    _write_spec(specs_dir, "task_t", "T", "sys text no golden row")
    # Golden row with DIFFERENT system → hash mismatch → 0 matches
    _write_golden(golden_path, [
        ("UNRELATED", "u", "a"),
    ])

    mlx_post = _make_mlx_mock('{"ok": true}')
    opts = RunnerOptions(
        model_path=".local-models/test-model",
        mlx_base_url="http://127.0.0.1:8101/v1",
        mlx_model_name="test",
        n_runs=2,
        golden_path=golden_path,
        specs_dir=specs_dir,
        task_filter=None,
        enable_judge=False,
        judge_metrics=CONFIG["judge"]["metrics"],
        config=copy.deepcopy(CONFIG),
        persist_tsdb=False,
        mlx_http_post=mlx_post,
    )
    report = run(opts)
    assert report["specs_total"] == 1
    assert report["specs_with_matched_rows"] == 0
    assert report["benchmark_results_rows"] == []
    info = report["per_spec"]["task_t"]
    assert info["matched_golden_rows"] == 0


def test_runner_respects_task_filter(bench_env) -> None:
    mlx_post = _make_mlx_mock('{"ok": true}')
    opts = RunnerOptions(
        model_path=".local-models/test-model",
        mlx_base_url="http://127.0.0.1:8101/v1",
        mlx_model_name="test",
        n_runs=1,
        golden_path=bench_env["golden_path"],
        specs_dir=bench_env["specs_dir"],
        task_filter=["task_j"],
        enable_judge=False,
        judge_metrics=CONFIG["judge"]["metrics"],
        config=copy.deepcopy(CONFIG),
        persist_tsdb=False,
        mlx_http_post=mlx_post,
    )
    report = run(opts)
    assert report["specs_total"] == 1
    task_ids = {r["task_id"] for r in report["benchmark_results_rows"]}
    assert task_ids == {"task_j"}


def test_runner_mlx_body_carries_group_sampling(bench_env) -> None:
    """T-group body must carry temperature=0.6, top_p=0.95, top_k=20 — no presence_penalty."""
    mlx_post = _make_mlx_mock('{"ok": true}')
    opts = RunnerOptions(
        model_path=".local-models/test-model",
        mlx_base_url="http://127.0.0.1:8101/v1",
        mlx_model_name="test",
        n_runs=1,
        golden_path=bench_env["golden_path"],
        specs_dir=bench_env["specs_dir"],
        task_filter=["task_t"],
        enable_judge=False,
        judge_metrics=CONFIG["judge"]["metrics"],
        config=copy.deepcopy(CONFIG),
        persist_tsdb=False,
        mlx_http_post=mlx_post,
    )
    run(opts)
    body = mlx_post.calls[0]["body"]  # type: ignore[attr-defined]
    assert body["temperature"] == 0.6
    assert body["top_p"] == 0.95
    assert body["top_k"] == 20
    assert "presence_penalty" not in body


def test_runner_j_group_body_has_presence_penalty(bench_env) -> None:
    mlx_post = _make_mlx_mock('{"ok": true}')
    opts = RunnerOptions(
        model_path=".local-models/test-model",
        mlx_base_url="http://127.0.0.1:8101/v1",
        mlx_model_name="test",
        n_runs=1,
        golden_path=bench_env["golden_path"],
        specs_dir=bench_env["specs_dir"],
        task_filter=["task_j"],
        enable_judge=False,
        judge_metrics=CONFIG["judge"]["metrics"],
        config=copy.deepcopy(CONFIG),
        persist_tsdb=False,
        mlx_http_post=mlx_post,
    )
    run(opts)
    body = mlx_post.calls[0]["body"]  # type: ignore[attr-defined]
    assert body["temperature"] == 0.0
    assert body["presence_penalty"] == 1.5
    # top_k=-1 ("unrestricted") must be dropped from body — MLX rejects -1
    assert "top_k" not in body


def test_runner_strips_assistant_turn(bench_env) -> None:
    """Invoke messages must NOT include the assistant target — the model generates."""
    mlx_post = _make_mlx_mock('{"ok": true}')
    opts = RunnerOptions(
        model_path=".local-models/test-model",
        mlx_base_url="http://127.0.0.1:8101/v1",
        mlx_model_name="test",
        n_runs=1,
        golden_path=bench_env["golden_path"],
        specs_dir=bench_env["specs_dir"],
        task_filter=["task_c"],
        enable_judge=False,
        judge_metrics=CONFIG["judge"]["metrics"],
        config=copy.deepcopy(CONFIG),
        persist_tsdb=False,
        mlx_http_post=mlx_post,
    )
    run(opts)
    body = mlx_post.calls[0]["body"]  # type: ignore[attr-defined]
    roles = [m["role"] for m in body["messages"]]
    assert "assistant" not in roles
    assert "system" in roles and "user" in roles


# ── Stagnation logic ─────────────────────────────────────────────────────────


def test_stagnation_insufficient_history() -> None:
    out = check_stagnation(
        this_aggregate=0.9,
        prior_aggregates=[0.85],
        per_task_now={"a": 0.9},
        per_task_prior={"h1": {"a": 0.85}},
        config=CONFIG,
    )
    assert out["flag"] is False
    assert out["reason"] == "insufficient_history"


def test_stagnation_flat_deltas_triggers() -> None:
    out = check_stagnation(
        this_aggregate=0.8505,
        prior_aggregates=[0.850, 0.8503],
        per_task_now={"a": 0.85},
        per_task_prior={"h1": {"a": 0.85}, "h2": {"a": 0.85}},
        config=CONFIG,
    )
    assert out["flag"] is True
    assert out["all_deltas_below_threshold"] is True


def test_stagnation_per_task_regression_triggers() -> None:
    out = check_stagnation(
        this_aggregate=0.9,
        prior_aggregates=[0.5, 0.7],
        per_task_now={"a": 0.50},  # dropped 0.30 from prior 0.80
        per_task_prior={"h1": {"a": 0.80}, "h2": {"a": 0.80}},
        config=CONFIG,
    )
    assert out["flag"] is True
    assert "a" in out["regressed_tasks"]


def test_stagnation_growth_not_flagged() -> None:
    out = check_stagnation(
        this_aggregate=0.90,
        prior_aggregates=[0.75, 0.82],
        per_task_now={"a": 0.90},
        per_task_prior={"h1": {"a": 0.75}, "h2": {"a": 0.82}},
        config=CONFIG,
    )
    assert out["flag"] is False
    assert out["all_deltas_below_threshold"] is False
    assert out["regressed_tasks"] == []
