"""GRPO loss unit tests — numerics ≤ 1e-6 delta against hand-computed fixtures."""
from __future__ import annotations

import math

import numpy as np
import pytest

from neural.training.rl.grpo_loss import (
    _MAX_LOG_RATIO,
    GRPOLossResult,
    compute_grpo_loss,
)

TOL = 1e-6


def test_hand_computed_fixture_matches():
    """N=2 fixture. All scalars pre-computed by hand; see module top-comment.

    Inputs:
      logprobs_new = [-1.0, -2.0]
      logprobs_old = [-1.5, -1.8]
      advantages   = [ 0.5, -0.3]
      ref_logprobs = [-1.2, -2.1]
      clip_ratio=0.2, kl_coef=0.01, entropy_coef=0.0

    Derivation:
      log_ratio      = [ 0.5, -0.2]
      ratio          = [exp(0.5), exp(-0.2)] = [1.6487212707..., 0.8187307531...]
      clipped        = [1.2, 0.8187307531...]   (first clamped to 1+ε=1.2)
      surr_unclipped = [0.8243606..., -0.2456192...]
      surr_clipped   = [0.6,        -0.2456192...]
      min            = [0.6,        -0.2456192...]
      policy_loss    = -(0.6 + -0.24561923) / 2 = -0.17719039
      kl_loss        = mean([0.2, 0.1]) = 0.15
      entropy        = -mean([-1, -2]) = 1.5
      total          = -0.17719039 + 0.01 * 0.15 - 0 * 1.5 = -0.17569039
    """
    result = compute_grpo_loss(
        logprobs_new=np.array([-1.0, -2.0]),
        logprobs_old=np.array([-1.5, -1.8]),
        advantages=np.array([0.5, -0.3]),
        ref_logprobs=np.array([-1.2, -2.1]),
        clip_ratio=0.2,
        kl_coef=0.01,
        entropy_coef=0.0,
    )

    # Hand-computed ground truth.
    expected_ratio_0 = math.exp(0.5)
    expected_ratio_1 = math.exp(-0.2)
    expected_mean_ratio = (expected_ratio_0 + expected_ratio_1) / 2

    expected_surr_unclipped_0 = expected_ratio_0 * 0.5
    expected_surr_clipped_0 = 1.2 * 0.5
    expected_surr_1 = expected_ratio_1 * (-0.3)  # not clipped
    expected_policy_loss = -(
        min(expected_surr_unclipped_0, expected_surr_clipped_0) + expected_surr_1
    ) / 2

    expected_kl = (0.2 + 0.1) / 2
    expected_entropy = -((-1.0 + -2.0) / 2)
    expected_total = expected_policy_loss + 0.01 * expected_kl - 0.0 * expected_entropy

    assert abs(result.policy_loss - expected_policy_loss) < TOL
    assert abs(result.kl_loss - expected_kl) < TOL
    assert abs(result.entropy - expected_entropy) < TOL
    assert abs(result.total - expected_total) < TOL
    assert abs(result.mean_ratio - expected_mean_ratio) < TOL
    assert abs(result.frac_clipped - 0.5) < TOL  # 1 of 2 clipped
    assert abs(result.mean_advantage - 0.1) < TOL
    assert abs(result.std_advantage - 0.4) < TOL


def test_entropy_coef_nonzero():
    """Entropy term subtracted scaled by coef."""
    r = compute_grpo_loss(
        logprobs_new=np.array([-1.0, -2.0]),
        logprobs_old=np.array([-1.0, -2.0]),  # ratio = 1 everywhere
        advantages=np.array([0.0, 0.0]),
        ref_logprobs=np.array([-1.0, -2.0]),
        clip_ratio=0.2,
        kl_coef=0.0,
        entropy_coef=0.1,
    )
    # policy_loss=0, kl=0, entropy=1.5, total = 0 - 0.1*1.5 = -0.15
    assert abs(r.entropy - 1.5) < TOL
    assert abs(r.total - (-0.15)) < TOL


def test_identical_policies_zero_policy_loss_with_zero_adv():
    """ratio==1 everywhere + zero advantages → policy_loss=0."""
    n = 4
    lp = np.array([-1.0, -2.0, -0.5, -1.5])
    r = compute_grpo_loss(
        logprobs_new=lp,
        logprobs_old=lp,
        advantages=np.zeros(n),
        ref_logprobs=lp,
        clip_ratio=0.2,
        kl_coef=0.01,
    )
    assert abs(r.policy_loss) < TOL
    assert abs(r.kl_loss) < TOL
    assert abs(r.mean_ratio - 1.0) < TOL
    assert abs(r.frac_clipped) < TOL


def test_log_ratio_clamp_prevents_overflow():
    """Log-ratio > _MAX_LOG_RATIO clamps to exp(±20) without overflow."""
    r = compute_grpo_loss(
        logprobs_new=np.array([100.0]),
        logprobs_old=np.array([-100.0]),  # log_ratio = 200, clamps to 20
        advantages=np.array([1.0]),
        ref_logprobs=np.array([0.0]),
        clip_ratio=0.2,
        kl_coef=0.0,
    )
    # ratio = exp(20) after clamp; clipped to 1.2. min(exp(20), 1.2) = 1.2. loss = -1.2
    assert abs(r.mean_ratio - math.exp(_MAX_LOG_RATIO)) < 1.0  # huge, but finite
    assert math.isfinite(r.total)
    assert math.isfinite(r.policy_loss)


def test_shape_mismatch_raises():
    with pytest.raises(ValueError, match="shape mismatch"):
        compute_grpo_loss(
            logprobs_new=np.array([-1.0, -2.0]),
            logprobs_old=np.array([-1.0]),
            advantages=np.array([0.5, -0.3]),
            ref_logprobs=np.array([-1.0, -2.0]),
        )


def test_empty_batch_raises():
    empty = np.array([])
    with pytest.raises(ValueError, match="empty batch"):
        compute_grpo_loss(
            logprobs_new=empty,
            logprobs_old=empty,
            advantages=empty,
            ref_logprobs=empty,
        )


def test_invalid_clip_ratio_raises():
    args = dict(
        logprobs_new=np.array([-1.0]),
        logprobs_old=np.array([-1.0]),
        advantages=np.array([0.0]),
        ref_logprobs=np.array([-1.0]),
    )
    with pytest.raises(ValueError, match="clip_ratio"):
        compute_grpo_loss(**args, clip_ratio=0.0)
    with pytest.raises(ValueError, match="clip_ratio"):
        compute_grpo_loss(**args, clip_ratio=1.0)
    with pytest.raises(ValueError, match="clip_ratio"):
        compute_grpo_loss(**args, clip_ratio=-0.1)


def test_result_to_dict():
    r = GRPOLossResult(
        total=1.0, policy_loss=0.5, kl_loss=0.1, entropy=2.0,
        mean_ratio=1.05, frac_clipped=0.25, mean_advantage=0.3, std_advantage=0.4,
    )
    d = r.to_dict()
    assert d["total"] == 1.0 and d["policy_loss"] == 0.5
    assert set(d) == {
        "total", "policy_loss", "kl_loss", "entropy",
        "mean_ratio", "frac_clipped", "mean_advantage", "std_advantage",
    }
