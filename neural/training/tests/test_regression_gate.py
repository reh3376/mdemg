"""Tests for the regression gate."""

import pytest

from training.regression_gate import (
    MAX_REGRESSION_PCT,
    MIN_IMPROVED_TASKS,
    MIN_IMPROVEMENT_PCT,
    MIN_JSON_VALIDITY,
    check_improvements,
    check_json_validity,
    check_overall_score,
    check_regressions,
    index_by_task,
    run_gate,
)


def _task_result(task, score, json_valid_pass_rate=None, status="evaluated"):
    """Build a minimal task result dict."""
    result = {
        "task": task,
        "weighted_score": score,
        "threshold_pass_rate": 1.0,
        "metric_aggregates": {},
        "status": status,
    }
    if json_valid_pass_rate is not None:
        result["metric_aggregates"]["json_valid"] = {
            "mean_score": json_valid_pass_rate,
            "pass_rate": json_valid_pass_rate,
            "threshold": 1.0,
            "met": json_valid_pass_rate >= 1.0,
        }
    return result


def _report(task_results, overall_score=None):
    """Build a minimal eval report."""
    if overall_score is None:
        scores = [r["weighted_score"] for r in task_results if r.get("status") == "evaluated"]
        overall_score = sum(scores) / len(scores) if scores else 0.0
    return {
        "task_results": task_results,
        "summary": {"overall_weighted_score": round(overall_score, 4)},
    }


# ── index_by_task ──


class TestIndexByTask:
    def test_indexes_evaluated(self):
        report = _report([
            _task_result("a", 0.8),
            _task_result("b", 0.9),
        ])
        idx = index_by_task(report)
        assert "a" in idx
        assert "b" in idx

    def test_skips_non_evaluated(self):
        report = _report([
            _task_result("a", 0.8),
            _task_result("b", 0.0, status="no_data"),
        ])
        idx = index_by_task(report)
        assert "a" in idx
        assert "b" not in idx


# ── check_regressions ──


class TestCheckRegressions:
    def test_no_regression(self):
        candidate = {"a": _task_result("a", 0.85)}
        baseline = {"a": _task_result("a", 0.80)}
        assert check_regressions(candidate, baseline) == []

    def test_small_regression_allowed(self):
        # 4% regression is within 5% threshold
        candidate = {"a": _task_result("a", 0.768)}
        baseline = {"a": _task_result("a", 0.80)}
        assert check_regressions(candidate, baseline) == []

    def test_large_regression_caught(self):
        # 10% regression exceeds 5% threshold
        candidate = {"a": _task_result("a", 0.72)}
        baseline = {"a": _task_result("a", 0.80)}
        regs = check_regressions(candidate, baseline)
        assert len(regs) == 1
        assert regs[0]["task"] == "a"
        assert regs[0]["delta"] < 0

    def test_new_task_no_regression(self):
        candidate = {"a": _task_result("a", 0.8), "b": _task_result("b", 0.7)}
        baseline = {"a": _task_result("a", 0.8)}
        assert check_regressions(candidate, baseline) == []

    def test_zero_baseline_skipped(self):
        candidate = {"a": _task_result("a", 0.5)}
        baseline = {"a": _task_result("a", 0.0)}
        assert check_regressions(candidate, baseline) == []


# ── check_improvements ──


class TestCheckImprovements:
    def test_clear_improvement(self):
        candidate = {"a": _task_result("a", 0.90)}
        baseline = {"a": _task_result("a", 0.80)}
        imps = check_improvements(candidate, baseline)
        assert len(imps) == 1
        assert imps[0]["delta_pct"] > 0

    def test_tiny_improvement_not_counted(self):
        # 1% improvement is below 2% threshold
        candidate = {"a": _task_result("a", 0.808)}
        baseline = {"a": _task_result("a", 0.80)}
        assert check_improvements(candidate, baseline) == []

    def test_no_improvement(self):
        candidate = {"a": _task_result("a", 0.80)}
        baseline = {"a": _task_result("a", 0.80)}
        assert check_improvements(candidate, baseline) == []

    def test_new_task_from_zero(self):
        candidate = {"a": _task_result("a", 0.7)}
        baseline = {"a": _task_result("a", 0.0)}
        imps = check_improvements(candidate, baseline)
        assert len(imps) == 1

    def test_new_task_not_in_baseline(self):
        candidate = {"a": _task_result("a", 0.8)}
        baseline = {}
        assert check_improvements(candidate, baseline) == []


# ── check_json_validity ──


class TestCheckJsonValidity:
    def test_high_validity_passes(self):
        candidate = {"a": _task_result("a", 0.9, json_valid_pass_rate=0.98)}
        assert check_json_validity(candidate) == []

    def test_low_validity_fails(self):
        candidate = {"a": _task_result("a", 0.9, json_valid_pass_rate=0.90)}
        failures = check_json_validity(candidate)
        assert len(failures) == 1
        assert failures[0]["task"] == "a"

    def test_string_task_skipped(self):
        # No json_valid metric = not a structured task
        candidate = {"a": _task_result("a", 0.9)}
        assert check_json_validity(candidate) == []

    def test_boundary_at_threshold(self):
        candidate = {"a": _task_result("a", 0.9, json_valid_pass_rate=0.95)}
        assert check_json_validity(candidate) == []

    def test_boundary_below_threshold(self):
        candidate = {"a": _task_result("a", 0.9, json_valid_pass_rate=0.949)}
        failures = check_json_validity(candidate)
        assert len(failures) == 1


# ── check_overall_score ──


class TestCheckOverallScore:
    def test_candidate_higher(self):
        cand = _report([], overall_score=0.85)
        base = _report([], overall_score=0.80)
        result = check_overall_score(cand, base)
        assert result["met"] is True
        assert result["delta"] > 0

    def test_candidate_lower(self):
        cand = _report([], overall_score=0.75)
        base = _report([], overall_score=0.80)
        result = check_overall_score(cand, base)
        assert result["met"] is False

    def test_equal_scores(self):
        cand = _report([], overall_score=0.80)
        base = _report([], overall_score=0.80)
        result = check_overall_score(cand, base)
        assert result["met"] is True


# ── run_gate ──


class TestRunGate:
    def test_pass_all_gates(self):
        baseline = _report([
            _task_result("a", 0.80, json_valid_pass_rate=0.96),
            _task_result("b", 0.75, json_valid_pass_rate=0.98),
            _task_result("c", 0.70),
        ])
        candidate = _report([
            _task_result("a", 0.85, json_valid_pass_rate=0.98),
            _task_result("b", 0.80, json_valid_pass_rate=0.99),
            _task_result("c", 0.70),
        ])

        gate = run_gate(candidate, baseline)
        assert gate["verdict"] == "PASS"
        assert gate["exit_code"] == 0
        assert len(gate["reasons"]) == 0

    def test_fail_on_regression(self):
        baseline = _report([
            _task_result("a", 0.80),
            _task_result("b", 0.90),
        ])
        candidate = _report([
            _task_result("a", 0.85),  # improved
            _task_result("b", 0.70),  # regressed ~22%
        ])

        gate = run_gate(candidate, baseline)
        assert gate["verdict"] == "FAIL"
        assert gate["exit_code"] == 1
        assert len(gate["regressions"]) == 1

    def test_fail_on_json_validity(self):
        baseline = _report([
            _task_result("a", 0.80, json_valid_pass_rate=0.98),
            _task_result("b", 0.75, json_valid_pass_rate=0.97),
        ])
        candidate = _report([
            _task_result("a", 0.85, json_valid_pass_rate=0.99),
            _task_result("b", 0.80, json_valid_pass_rate=0.80),  # below 95%
        ])

        gate = run_gate(candidate, baseline)
        assert gate["verdict"] == "FAIL"
        assert gate["exit_code"] == 1
        assert len(gate["json_validity_failures"]) == 1

    def test_warn_not_enough_improvements(self):
        baseline = _report([
            _task_result("a", 0.80),
            _task_result("b", 0.85),
            _task_result("c", 0.90),
        ])
        # Only 1 task improves (need 2), no regressions
        candidate = _report([
            _task_result("a", 0.85),  # +6.25% — counts
            _task_result("b", 0.85),  # flat
            _task_result("c", 0.90),  # flat
        ])

        gate = run_gate(candidate, baseline)
        assert gate["verdict"] == "WARN"
        assert gate["exit_code"] == 2

    def test_warn_overall_score_lower(self):
        baseline = _report([
            _task_result("a", 0.90),
            _task_result("b", 0.80),
            _task_result("c", 0.70),
        ], overall_score=0.80)
        # Enough improvements, no regressions, but overall drops
        candidate = _report([
            _task_result("a", 0.85),   # -5.5% but within 5% threshold? No, -5.5% > 5%
            _task_result("b", 0.85),   # +6.25%
            _task_result("c", 0.75),   # +7.1%
        ], overall_score=0.78)

        gate = run_gate(candidate, baseline)
        # a regresses 5.5% > 5% threshold -> FAIL
        assert gate["verdict"] == "FAIL"

    def test_constants(self):
        assert MAX_REGRESSION_PCT == 0.05
        assert MIN_IMPROVEMENT_PCT == 0.02
        assert MIN_IMPROVED_TASKS == 2
        assert MIN_JSON_VALIDITY == 0.95
