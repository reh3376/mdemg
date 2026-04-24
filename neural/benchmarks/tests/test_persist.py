"""Unit tests for persist.render_* — asserts V0012 column shape."""
from __future__ import annotations

from pathlib import Path

from neural.benchmarks.persist import (
    render_benchmark_results_inserts,
    render_benchmark_runs_insert,
    write_sql_sidecar,
)


V012_RUNS_COLS = {
    "run_id", "started_at", "completed_at", "model_path",
    "base_sha", "adapter_manifest_sha", "dataset_sha",
    "config_sha", "n_runs_per_task", "specs_total",
    "specs_with_matched_rows", "aggregate_weighted_score",
    "stagnation_flag", "stagnation_reason", "report_path",
}
V012_RESULTS_COLS = {
    "recorded_at", "run_id", "task_id", "run_idx",
    "sampling_group", "response_text", "reward_vector",
    "judge_scores", "judge_meta", "latency_ms",
    "adapter_sha", "base_sha", "dataset_sha",
}


def _sample_report() -> dict:
    return {
        "run_id": "cuid2-abc123",
        "started_at": "2026-04-23T12:00:00+00:00",
        "completed_at": "2026-04-23T12:30:00+00:00",
        "model_path": ".local-models/qwen3-14b-mdemg-v1",
        "base_sha": "abc",
        "adapter_manifest_sha": "def",
        "dataset_sha": "ghi",
        "config_sha": "jkl",
        "n_runs_per_task": 5,
        "specs_total": 3,
        "specs_with_matched_rows": 3,
        "aggregate_weighted_score": 0.8123,
        "stagnation": {"flag": False, "reason": "insufficient_history"},
        "benchmark_results_rows": [
            {
                "run_id": "cuid2-abc123",
                "task_id": "task_t",
                "run_idx": 0,
                "sampling_group": "T",
                "response_text": '{"ok": true}',
                "reward_vector": {"json_valid": 1.0},
                "judge_scores": {"coherence": 0.9},
                "judge_meta": {"coherence": {"model": "gpt-5.4-mini", "seed": 0}},
                "latency_ms": 123,
            },
            {
                "run_id": "cuid2-abc123",
                "task_id": "task_c",
                "run_idx": 0,
                "sampling_group": "C",
                "response_text": "it's fine",  # contains single quote — must be escaped
                "reward_vector": {"json_valid": 0.0},
                "judge_scores": {},
                "judge_meta": {},
                "latency_ms": 90,
            },
        ],
    }


def test_runs_insert_has_v012_columns() -> None:
    stmt = render_benchmark_runs_insert(_sample_report())
    # Column names listed in the INSERT header
    col_block = stmt.split("(", 1)[1].split(")", 1)[0]
    cols_present = {c.strip() for c in col_block.split(",")}
    assert cols_present == V012_RUNS_COLS


def test_results_insert_has_v012_columns() -> None:
    stmts = render_benchmark_results_inserts(_sample_report())
    assert len(stmts) == 2
    for stmt in stmts:
        col_block = stmt.split("(", 1)[1].split(")", 1)[0]
        cols_present = {c.strip() for c in col_block.split(",")}
        assert cols_present == V012_RESULTS_COLS


def test_results_insert_escapes_single_quotes() -> None:
    stmts = render_benchmark_results_inserts(_sample_report())
    # Second row's response_text is "it's fine"
    row_c = stmts[1]
    assert "'it''s fine'" in row_c


def test_results_insert_casts_jsonb() -> None:
    stmts = render_benchmark_results_inserts(_sample_report())
    for stmt in stmts:
        assert "::jsonb" in stmt  # reward_vector, judge_scores, judge_meta cast


def test_runs_insert_null_for_missing_field() -> None:
    # Report without optional SHAs — should emit NULL, not error
    minimal = {
        "run_id": "r",
        "started_at": "t0",
        "completed_at": "t1",
        "model_path": "m",
        "config_sha": "c",
        "n_runs_per_task": 1,
        "specs_total": 1,
        "specs_with_matched_rows": 1,
        "aggregate_weighted_score": 0.5,
        "benchmark_results_rows": [],
    }
    stmt = render_benchmark_runs_insert(minimal)
    # adapter_manifest_sha, base_sha, dataset_sha, stagnation_reason should be NULL
    assert "NULL" in stmt


def test_write_sql_sidecar_wraps_in_transaction(tmp_path: Path) -> None:
    out = tmp_path / "bench.sql"
    n = write_sql_sidecar(_sample_report(), out)
    txt = out.read_text()
    assert txt.startswith("-- Phase 10 benchmark persistence")
    assert "\nBEGIN;\n" in txt
    assert txt.rstrip().endswith("COMMIT;")
    # 1 run row + 2 result rows = 3 statements
    assert n == 3


def test_stagnation_flag_defaults_false() -> None:
    r = _sample_report()
    r.pop("stagnation", None)
    stmt = render_benchmark_runs_insert(r)
    assert "FALSE" in stmt
