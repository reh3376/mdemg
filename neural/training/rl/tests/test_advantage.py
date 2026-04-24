"""Advantage estimator unit tests — zero-stddev policies + clipping + mean fallback."""
from __future__ import annotations

import numpy as np
import pytest

from neural.training.rl.advantage import estimate_advantage

TOL = 1e-6


def test_basic_normalization_with_historical_sigma():
    """Historical σ > 0 → advantages = (r - μ_hist) / σ_hist, clipped."""
    rewards = np.array([1.0, 2.0, 3.0, 4.0])
    task_ids = ["t0", "t0", "t0", "t0"]
    # μ_hist = 2.5, σ_hist = 1.0 → advantages = [-1.5, -0.5, 0.5, 1.5]
    est = estimate_advantage(
        rewards, task_ids,
        stddev_cache={"t0": 1.0},
        mean_cache={"t0": 2.5},
        policy="intra_batch_only",
    )
    np.testing.assert_allclose(est.advantages, [-1.5, -0.5, 0.5, 1.5], atol=TOL)
    assert est.n_dropped == 0
    assert est.keep_mask.all()
    assert abs(est.effective_stddev["t0"] - 1.0) < TOL


def test_intra_batch_only_when_historical_zero():
    """hist_sd == 0 + policy=intra_batch_only → use batch stddev."""
    rewards = np.array([1.0, 2.0, 3.0, 4.0])  # batch σ_pop = sqrt(1.25) ≈ 1.118
    est = estimate_advantage(
        rewards, ["t0"] * 4,
        stddev_cache={"t0": 0.0},
        mean_cache={"t0": 2.5},
        policy="intra_batch_only",
    )
    batch_sigma = float(np.std(rewards))
    expected = (rewards - 2.5) / batch_sigma
    expected = np.clip(expected, -5.0, 5.0)
    np.testing.assert_allclose(est.advantages, expected, atol=TOL)
    assert abs(est.effective_stddev["t0"] - batch_sigma) < TOL


def test_widen_policy_uses_mean_times_factor():
    """policy=widen + hist=0 → σ = max(|μ|×factor, 0) clamped by min_stddev."""
    rewards = np.array([1.0, 1.0, 1.0, 1.0])
    est = estimate_advantage(
        rewards, ["t0"] * 4,
        stddev_cache={"t0": 0.0},
        mean_cache={"t0": 10.0},
        policy="widen",
        widen_factor=0.05,
    )
    # σ = 10 * 0.05 = 0.5; advantages = (1 - 10) / 0.5 = -18, clipped to -5
    assert abs(est.effective_stddev["t0"] - 0.5) < TOL
    np.testing.assert_allclose(est.advantages, [-5.0, -5.0, -5.0, -5.0], atol=TOL)


def test_drop_policy_excludes_zero_stddev_task():
    """policy=drop + hist=0 → samples removed from output, keep_mask False."""
    rewards = np.array([1.0, 2.0, 3.0, 4.0])
    task_ids = ["t0", "t0", "t1", "t1"]
    est = estimate_advantage(
        rewards, task_ids,
        stddev_cache={"t0": 0.0, "t1": 1.0},  # t0 zero, t1 nonzero
        mean_cache={"t0": 1.5, "t1": 3.5},
        policy="drop",
    )
    assert est.n_dropped == 2
    assert list(est.keep_mask) == [False, False, True, True]
    # Kept: (3-3.5)/1 = -0.5 ; (4-3.5)/1 = 0.5
    np.testing.assert_allclose(est.advantages, [-0.5, 0.5], atol=TOL)


def test_missing_mean_cache_falls_back_to_batch_mean():
    """When mean_cache is None or missing a task, uses batch mean."""
    rewards = np.array([2.0, 4.0, 6.0])  # batch mean = 4
    est = estimate_advantage(
        rewards, ["t0"] * 3,
        stddev_cache={"t0": 2.0},
        mean_cache=None,
        policy="intra_batch_only",
    )
    # (r - 4) / 2 = [-1, 0, 1]
    np.testing.assert_allclose(est.advantages, [-1.0, 0.0, 1.0], atol=TOL)


def test_min_stddev_floor_prevents_div_by_zero():
    """When intra-batch σ also collapses, min_stddev floor kicks in."""
    rewards = np.array([5.0, 5.0, 5.0])  # batch σ = 0
    est = estimate_advantage(
        rewards, ["t0"] * 3,
        stddev_cache={"t0": 0.0},
        mean_cache={"t0": 5.0},
        policy="intra_batch_only",
        min_stddev=1e-4,
    )
    assert est.effective_stddev["t0"] >= 1e-4
    # All rewards equal mean → advantages all 0
    np.testing.assert_allclose(est.advantages, [0.0, 0.0, 0.0], atol=TOL)


def test_advantage_clip_bounds():
    """Huge advantages clipped to configured range."""
    rewards = np.array([100.0, -100.0])
    est = estimate_advantage(
        rewards, ["t0", "t0"],
        stddev_cache={"t0": 0.01},
        mean_cache={"t0": 0.0},
        policy="intra_batch_only",
        advantage_clip=(-3.0, 3.0),
    )
    assert est.advantages.max() <= 3.0 + TOL
    assert est.advantages.min() >= -3.0 - TOL


def test_multi_task_independent_normalization():
    """Each task normalized independently."""
    rewards = np.array([1.0, 3.0, 10.0, 20.0])
    task_ids = ["t0", "t0", "t1", "t1"]
    est = estimate_advantage(
        rewards, task_ids,
        stddev_cache={"t0": 1.0, "t1": 5.0},
        mean_cache={"t0": 2.0, "t1": 15.0},
        policy="intra_batch_only",
    )
    # t0: (1-2)/1=-1, (3-2)/1=1; t1: (10-15)/5=-1, (20-15)/5=1
    np.testing.assert_allclose(est.advantages, [-1.0, 1.0, -1.0, 1.0], atol=TOL)


def test_missing_task_in_stddev_cache_triggers_zero_policy():
    """Missing stddev_cache entry behaves same as 0.0."""
    rewards = np.array([1.0, 2.0])
    est = estimate_advantage(
        rewards, ["t_missing", "t_missing"],
        stddev_cache={},  # task not in cache
        mean_cache={"t_missing": 1.5},
        policy="intra_batch_only",
    )
    # Falls back to batch stddev path
    batch_sigma = float(np.std(rewards))
    assert abs(est.effective_stddev["t_missing"] - batch_sigma) < TOL


def test_length_mismatch_raises():
    with pytest.raises(ValueError, match="task_ids length"):
        estimate_advantage(
            np.array([1.0, 2.0, 3.0]),
            ["t0", "t0"],  # len 2 vs rewards len 3
            stddev_cache={"t0": 1.0},
        )


def test_unknown_policy_raises():
    with pytest.raises(ValueError, match="unknown"):
        estimate_advantage(
            np.array([1.0]), ["t0"],
            stddev_cache={"t0": 1.0},
            policy="bogus",  # type: ignore[arg-type]
        )


def test_invalid_clip_range_raises():
    with pytest.raises(ValueError, match="advantage_clip"):
        estimate_advantage(
            np.array([1.0]), ["t0"],
            stddev_cache={"t0": 1.0},
            advantage_clip=(1.0, 1.0),
        )


def test_diagnostics_populated():
    rewards = np.array([1.0, 2.0, 3.0])
    est = estimate_advantage(
        rewards, ["t0"] * 3,
        stddev_cache={"t0": 1.0},
        mean_cache={"t0": 2.0},
    )
    # advantages = [-1, 0, 1]; mean=0, std=sqrt(2/3)
    assert abs(est.mean_advantage) < TOL
    assert abs(est.std_advantage - np.sqrt(2.0 / 3.0)) < TOL
    assert est.n_dropped == 0
