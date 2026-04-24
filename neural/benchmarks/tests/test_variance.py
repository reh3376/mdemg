"""Unit tests for variance.RunAggregator."""
from __future__ import annotations

import math

import pytest

from neural.benchmarks.variance import RunAggregator, RunSample


def test_single_sample_stddev_zero() -> None:
    agg = RunAggregator()
    agg.add(RunSample("task_a", 0, {"r1": 0.8, "r2": 0.6}))
    a = agg.aggregate("task_a")
    assert a.n == 1
    assert a.overall_stddev == 0.0
    assert a.overall_mean == pytest.approx((0.8 + 0.6) / 2)


def test_known_variance() -> None:
    """Overall scalar = mean of reward_vector per run.

    Runs: (1.0, 0.0), (1.0, 0.0), (0.5, 0.5), (0.0, 1.0), (0.0, 1.0)
    overall per run = 0.5, 0.5, 0.5, 0.5, 0.5 — stddev should be exactly 0.
    """
    agg = RunAggregator()
    payloads = [(1.0, 0.0), (1.0, 0.0), (0.5, 0.5), (0.0, 1.0), (0.0, 1.0)]
    for i, (a, b) in enumerate(payloads):
        agg.add(RunSample("t", i, {"r1": a, "r2": b}))
    got = agg.aggregate("t")
    assert got.n == 5
    assert got.overall_mean == pytest.approx(0.5)
    assert got.overall_stddev == pytest.approx(0.0)


def test_nonzero_variance() -> None:
    agg = RunAggregator()
    for i, v in enumerate([0.0, 0.0, 1.0, 1.0]):
        agg.add(RunSample("t", i, {"r": v}))
    got = agg.aggregate("t")
    # pstdev([0,0,1,1]) = 0.5
    assert got.overall_stddev == pytest.approx(0.5)
    assert got.overall_min == 0.0
    assert got.overall_max == 1.0


def test_per_metric_mean_and_stddev() -> None:
    agg = RunAggregator()
    agg.add(RunSample("t", 0, {"a": 0.2, "b": 0.5}))
    agg.add(RunSample("t", 1, {"a": 0.4, "b": 0.5}))
    got = agg.aggregate("t")
    assert got.per_metric_mean["a"] == pytest.approx(0.3)
    assert got.per_metric_mean["b"] == pytest.approx(0.5)
    # pstdev([0.2, 0.4]) = 0.1
    assert got.per_metric_stddev["a"] == pytest.approx(0.1)
    assert got.per_metric_stddev["b"] == pytest.approx(0.0)


def test_judge_scores_enter_per_metric_not_overall() -> None:
    """Judge scores populate per_metric_* but do NOT feed overall scalar
    (GRPO advantage norm must come from deterministic rewards only)."""
    agg = RunAggregator()
    agg.add(RunSample(
        "t", 0,
        reward_vector={"r": 1.0},
        judge_scores={"coherence": 0.0},
    ))
    got = agg.aggregate("t")
    assert got.overall_mean == pytest.approx(1.0)  # judge did NOT drag down
    assert "coherence" in got.per_metric_mean
    assert got.per_metric_mean["coherence"] == pytest.approx(0.0)


def test_nan_excluded() -> None:
    agg = RunAggregator()
    agg.add(RunSample("t", 0, {"r": 1.0}))
    agg.add(RunSample("t", 1, {"r": float("nan")}))
    got = agg.aggregate("t")
    # NaN is filtered from both overall and per-metric
    assert not math.isnan(got.overall_mean)
    assert got.overall_mean == pytest.approx(1.0)


def test_missing_task_raises() -> None:
    with pytest.raises(KeyError):
        RunAggregator().aggregate("nope")


def test_aggregate_all_sorted() -> None:
    agg = RunAggregator()
    agg.add(RunSample("b", 0, {"r": 0.5}))
    agg.add(RunSample("a", 0, {"r": 0.5}))
    tids = [a.task_id for a in agg.aggregate_all()]
    assert tids == ["a", "b"]


def test_to_dict_roundtrip() -> None:
    agg = RunAggregator()
    agg.add(RunSample("t", 0, {"r": 0.7}))
    d = agg.aggregate("t").to_dict()
    assert d["task_id"] == "t"
    assert d["n"] == 1
    assert d["overall_mean"] == pytest.approx(0.7)
    assert d["per_metric_mean"]["r"] == pytest.approx(0.7)
