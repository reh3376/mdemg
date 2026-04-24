"""GRPO loss — clipped surrogate + KL penalty.

Implements the core Group Relative Policy Optimization loss used by the Phase 11
trainer. Stays framework-agnostic: consumes NumPy arrays of logprobs + rewards
and returns a scalar loss + per-component breakdown. The MLX trainer wraps this
via a thin mlx.array → np.ndarray adapter on each step.

Formula:
    ratio = exp(logπ_new(a|s) - logπ_old(a|s))
    clipped = clip(ratio, 1 - ε, 1 + ε)
    policy_loss = -mean(min(ratio * A, clipped * A))
    kl_loss     = mean(KL(π_new ‖ π_ref))              [forward KL, log-space]
    entropy     = -mean(logπ_new)                      [bonus, configurable]
    total       = policy_loss + kl_coef * kl_loss - entropy_coef * entropy

Numerical stability:
    - ratios clamped to exp(±MAX_LOG_RATIO) = exp(±20) before mean;
      prevents overflow when π_new/π_old diverge mid-training.
    - KL computed in log-space: log(π_new/π_ref) × π_new via exp,
      with the same clamp for stability.

Unit-tested against hand-computed scalars in tests/test_grpo_loss.py.
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import numpy as np


# Clamp log(ratio) to ±20 — exp(20) ≈ 4.85e8, well inside float64 range,
# and any larger ratio is almost certainly a bug (e.g., stale reference model).
_MAX_LOG_RATIO = 20.0


@dataclass(frozen=True)
class GRPOLossResult:
    """Per-step loss breakdown. All values are scalars; `total` is what the
    optimizer consumes."""

    total: float
    policy_loss: float
    kl_loss: float
    entropy: float
    # Diagnostics (not optimized against).
    mean_ratio: float
    frac_clipped: float
    mean_advantage: float
    std_advantage: float

    def to_dict(self) -> dict[str, float]:
        return {
            "total": float(self.total),
            "policy_loss": float(self.policy_loss),
            "kl_loss": float(self.kl_loss),
            "entropy": float(self.entropy),
            "mean_ratio": float(self.mean_ratio),
            "frac_clipped": float(self.frac_clipped),
            "mean_advantage": float(self.mean_advantage),
            "std_advantage": float(self.std_advantage),
        }


def compute_grpo_loss(
    logprobs_new: np.ndarray,
    logprobs_old: np.ndarray,
    advantages: np.ndarray,
    ref_logprobs: np.ndarray,
    *,
    clip_ratio: float = 0.2,
    kl_coef: float = 0.01,
    entropy_coef: float = 0.0,
) -> GRPOLossResult:
    """Compute scalar GRPO loss + component breakdown.

    Parameters
    ----------
    logprobs_new : (N,) array of sum-of-token logprobs under the current policy.
    logprobs_old : (N,) array of sum-of-token logprobs under the policy at
                   rollout time (≈ logprobs_new at step 0).
    advantages   : (N,) array of normalized advantages (already reduced by
                   `advantage.estimate_advantage`).
    ref_logprobs : (N,) array of sum-of-token logprobs under the frozen
                   reference policy (Phase 5 SFT model).
    clip_ratio   : ε in the clipped surrogate. 0.2 is the PPO / GRPO default.
    kl_coef      : coefficient on KL(π_new ‖ π_ref). Larger values stay closer
                   to the reference.
    entropy_coef : coefficient on the entropy bonus (negated in loss, so higher
                   coef → more exploration). 0.0 by default per Phase 11 config.

    Returns
    -------
    GRPOLossResult with the scalar `total` + diagnostic fields.
    """
    if not (logprobs_new.shape == logprobs_old.shape == advantages.shape == ref_logprobs.shape):
        raise ValueError(
            f"shape mismatch: new={logprobs_new.shape} old={logprobs_old.shape} "
            f"adv={advantages.shape} ref={ref_logprobs.shape}"
        )
    if logprobs_new.size == 0:
        raise ValueError("empty batch; cannot compute loss")
    if not 0.0 < clip_ratio < 1.0:
        raise ValueError(f"clip_ratio must be in (0, 1); got {clip_ratio}")

    # ── ratio = exp(log_new - log_old) with numerical clamp ─────────────────
    log_ratio = logprobs_new - logprobs_old
    log_ratio = np.clip(log_ratio, -_MAX_LOG_RATIO, _MAX_LOG_RATIO)
    ratio = np.exp(log_ratio)

    # ── clipped surrogate ───────────────────────────────────────────────────
    clipped_ratio = np.clip(ratio, 1.0 - clip_ratio, 1.0 + clip_ratio)
    surr_unclipped = ratio * advantages
    surr_clipped = clipped_ratio * advantages
    # GRPO takes the MIN of the two — the pessimistic objective (PPO-style).
    policy_loss = -np.mean(np.minimum(surr_unclipped, surr_clipped))
    frac_clipped = float(np.mean(ratio != clipped_ratio))

    # ── KL(π_new ‖ π_ref), log-space for stability ──────────────────────────
    # Forward KL: E_{π_new}[log(π_new / π_ref)]. With sum-of-token logprobs and
    # one sample per (s, a), the Monte Carlo estimator is just (log_new - log_ref).
    log_diff = logprobs_new - ref_logprobs
    log_diff = np.clip(log_diff, -_MAX_LOG_RATIO, _MAX_LOG_RATIO)
    kl_loss = float(np.mean(log_diff))

    # ── entropy bonus (optional) ────────────────────────────────────────────
    # Estimator: H(π_new) ≈ -E[logπ_new]. Exact entropy requires full vocab
    # logits; this Monte Carlo estimator is what PPO implementations use.
    entropy = float(-np.mean(logprobs_new))

    total = float(policy_loss) + kl_coef * kl_loss - entropy_coef * entropy

    return GRPOLossResult(
        total=total,
        policy_loss=float(policy_loss),
        kl_loss=kl_loss,
        entropy=entropy,
        mean_ratio=float(np.mean(ratio)),
        frac_clipped=frac_clipped,
        mean_advantage=float(np.mean(advantages)),
        std_advantage=float(np.std(advantages)),
    )


def _as_np(x: Any) -> np.ndarray:
    """Convenience — coerce lists/tuples/mlx arrays to np.ndarray. Kept as a
    hook so the MLX trainer can swap in a zero-copy path later.
    """
    if isinstance(x, np.ndarray):
        return x
    return np.asarray(x, dtype=np.float64)
