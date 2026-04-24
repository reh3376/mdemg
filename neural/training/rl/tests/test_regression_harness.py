"""Regression harness tests — Tier 2 integration with mocked Phase 10 runner.

Verifies gate 5a/5b pass/fail logic, report emission, and dispatch to the
injectable benchmark runner.
"""
from __future__ import annotations

import json
from pathlib import Path

import pytest

from neural.training.rl.regression import (
    DualGateReport,
    RegressionConfig,
    evaluate_gate_5a,
    evaluate_gate_5b,
    run_dual_regression,
)


def _rep(aggregate, tasks=None):
    return {
        "aggregate_weighted_score": aggregate,
        "per_task_aggregate": {
            t: {"weighted_score": s} for t, s in (tasks or {}).items()
        },
    }


# ── gate 5a ─────────────────────────────────────────────────────────────────


def test_gate_5a_pass_meeting_aggregate_and_no_task_regression():
    cfg = RegressionConfig(phase5_baseline_aggregate=0.8338,
                            aggregate_target_multiplier=1.02,
                            per_task_max_regression=0.02)
    sandbox = _rep(0.86, tasks={"t1": 0.90, "t2": 0.82})
    baseline = _rep(0.8338, tasks={"t1": 0.88, "t2": 0.80})
    g = evaluate_gate_5a(sandbox, baseline, cfg)
    assert g.passed
    assert g.failures == []
    assert abs(g.aggregate_target - 0.8338 * 1.02) < 1e-9


def test_gate_5a_fails_when_aggregate_below_target():
    cfg = RegressionConfig(phase5_baseline_aggregate=0.8338,
                            aggregate_target_multiplier=1.02,
                            per_task_max_regression=0.02)
    sandbox = _rep(0.84, tasks={"t1": 0.88, "t2": 0.80})  # below 0.8505
    baseline = _rep(0.8338, tasks={"t1": 0.88, "t2": 0.80})
    g = evaluate_gate_5a(sandbox, baseline, cfg)
    assert not g.passed
    assert any("aggregate" in f for f in g.failures)


def test_gate_5a_fails_on_per_task_regression():
    cfg = RegressionConfig(phase5_baseline_aggregate=0.8338,
                            aggregate_target_multiplier=1.00,  # relaxed aggregate
                            per_task_max_regression=0.02)
    sandbox = _rep(0.85, tasks={"t1": 0.85, "t2": 0.70})  # t2 regressed 10pp
    baseline = _rep(0.8338, tasks={"t1": 0.88, "t2": 0.80})
    g = evaluate_gate_5a(sandbox, baseline, cfg)
    assert not g.passed
    assert any("t2" in f and "regressed" in f for f in g.failures)


def test_gate_5a_flags_missing_task_in_sandbox():
    cfg = RegressionConfig()
    sandbox = _rep(0.99, tasks={"t1": 0.99})  # t2 missing
    baseline = _rep(0.80, tasks={"t1": 0.80, "t2": 0.80})
    g = evaluate_gate_5a(sandbox, baseline, cfg)
    assert not g.passed
    assert any("t2" in f and "missing" in f for f in g.failures)


def test_gate_5a_reports_per_task_regressions_dict():
    cfg = RegressionConfig(aggregate_target_multiplier=1.00,
                            per_task_max_regression=0.10)  # lenient
    sandbox = _rep(0.85, tasks={"t1": 0.90, "t2": 0.75})
    baseline = _rep(0.83, tasks={"t1": 0.88, "t2": 0.80})
    g = evaluate_gate_5a(sandbox, baseline, cfg)
    # t1: baseline 0.88 - sandbox 0.90 = -0.02 (improvement)
    # t2: baseline 0.80 - sandbox 0.75 = +0.05 (regression under cap)
    assert abs(g.per_task_regressions["t1"] - (-0.02)) < 1e-9
    assert abs(g.per_task_regressions["t2"] - 0.05) < 1e-9
    assert g.passed  # regression under cap 0.10


# ── gate 5b ─────────────────────────────────────────────────────────────────


def test_gate_5b_pass_when_delta_small():
    cfg = RegressionConfig(fresh_merge_max_delta=0.005)
    g = evaluate_gate_5b(_rep(0.8600), _rep(0.8620), cfg)
    assert g.passed


def test_gate_5b_fails_when_delta_exceeds_cap():
    cfg = RegressionConfig(fresh_merge_max_delta=0.005)
    g = evaluate_gate_5b(_rep(0.8600), _rep(0.8700), cfg)  # delta 0.01
    assert not g.passed
    assert any("adapter-merge corruption" in f for f in g.failures)


def test_gate_5b_symmetric():
    """|sandbox - fresh| is the same regardless of order."""
    cfg = RegressionConfig(fresh_merge_max_delta=0.005)
    g1 = evaluate_gate_5b(_rep(0.86), _rep(0.87), cfg)
    g2 = evaluate_gate_5b(_rep(0.87), _rep(0.86), cfg)
    assert g1.passed == g2.passed
    assert abs(g1.aggregate_actual - g2.aggregate_actual) < 1e-9


# ── orchestrator with mocked runner ─────────────────────────────────────────


def _mk_baseline(tmp_path: Path, aggregate=0.8338, tasks=None) -> Path:
    path = tmp_path / "baseline.json"
    path.write_text(json.dumps(_rep(aggregate, tasks)))
    return path


def test_run_dual_regression_passes_both_gates(tmp_path: Path):
    cfg = RegressionConfig(phase5_baseline_aggregate=0.8338,
                            aggregate_target_multiplier=1.02,
                            per_task_max_regression=0.02,
                            fresh_merge_max_delta=0.005)
    baseline_path = _mk_baseline(tmp_path, aggregate=0.8338,
                                  tasks={"t1": 0.88, "t2": 0.80})

    calls = []
    def runner(cfg_path, adapter):
        calls.append((cfg_path, adapter))
        # Both return the same high score ⇒ 5a passes + 5b delta=0 passes.
        return _rep(0.86, tasks={"t1": 0.89, "t2": 0.82})

    report_path = tmp_path / "report.json"
    dual = run_dual_regression(
        cfg=cfg,
        sandbox_adapter_path="/s/path",
        fresh_adapter_path="/f/path",
        phase10_config_path="configs/benchmark_phase10.yaml",
        baseline_report_path=baseline_path,
        run_benchmark=runner,
        report_path=report_path,
    )
    assert dual.overall_verdict == "pass"
    assert dual.gate_5a.passed and dual.gate_5b.passed
    assert len(calls) == 2
    assert calls[0][1] == "/s/path" and calls[1][1] == "/f/path"
    # Report on disk.
    loaded = json.loads(report_path.read_text())
    assert loaded["overall_verdict"] == "pass"


def test_run_dual_regression_fails_on_5a_only(tmp_path: Path):
    cfg = RegressionConfig(phase5_baseline_aggregate=0.8338,
                            aggregate_target_multiplier=1.02)
    baseline_path = _mk_baseline(tmp_path, aggregate=0.8338,
                                  tasks={"t1": 0.88})

    def runner(cfg_path, adapter):
        return _rep(0.80, tasks={"t1": 0.80})  # below target

    dual = run_dual_regression(
        cfg=cfg,
        sandbox_adapter_path="/s", fresh_adapter_path="/f",
        phase10_config_path="c.yaml",
        baseline_report_path=baseline_path,
        run_benchmark=runner,
    )
    assert dual.overall_verdict == "fail"
    assert not dual.gate_5a.passed
    assert dual.gate_5b.passed  # sandbox == fresh → delta 0


def test_run_dual_regression_fails_on_5b_only(tmp_path: Path):
    cfg = RegressionConfig(phase5_baseline_aggregate=0.80,
                            aggregate_target_multiplier=1.00,
                            per_task_max_regression=1.0,  # permissive
                            fresh_merge_max_delta=0.005)
    baseline_path = _mk_baseline(tmp_path, aggregate=0.80,
                                  tasks={"t1": 0.80})

    call_count = {"i": 0}
    def runner(cfg_path, adapter):
        call_count["i"] += 1
        # First call (sandbox): 0.85; second (fresh): 0.87 → delta 0.02 > 0.005
        return _rep(0.85 if call_count["i"] == 1 else 0.87, tasks={"t1": 0.85})

    dual = run_dual_regression(
        cfg=cfg,
        sandbox_adapter_path="/s", fresh_adapter_path="/f",
        phase10_config_path="c.yaml",
        baseline_report_path=baseline_path,
        run_benchmark=runner,
    )
    assert dual.overall_verdict == "fail"
    assert dual.gate_5a.passed
    assert not dual.gate_5b.passed


def test_run_dual_regression_writes_full_report(tmp_path: Path):
    baseline_path = _mk_baseline(tmp_path, aggregate=0.80, tasks={"t1": 0.80})
    def runner(cfg_path, adapter):
        return _rep(0.85, tasks={"t1": 0.85})
    report_path = tmp_path / "r.json"
    run_dual_regression(
        cfg=RegressionConfig(phase5_baseline_aggregate=0.80,
                              aggregate_target_multiplier=1.00,
                              per_task_max_regression=1.0,
                              fresh_merge_max_delta=1.0),
        sandbox_adapter_path="/s", fresh_adapter_path="/f",
        phase10_config_path="c.yaml",
        baseline_report_path=baseline_path,
        run_benchmark=runner,
        report_path=report_path,
    )
    loaded = json.loads(report_path.read_text())
    # Full structure persisted.
    assert set(loaded) >= {"overall_verdict", "gate_5a", "gate_5b",
                            "adapter_paths", "baseline_paths",
                            "sandbox_report", "fresh_report"}
    assert loaded["adapter_paths"]["sandbox"] == "/s"
    assert loaded["adapter_paths"]["fresh"] == "/f"
