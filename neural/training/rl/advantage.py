"""Advantage estimator with three zero-stddev policies.

The GRPO advantage for sample i of task t at training step s is:

    A_{i,t,s} = (r_{i,t,s} - μ_t) / σ_t_effective

where:
    r      — scalar reward for sample i
    μ_t    — mean reward for task t (historical, from benchmark_results)
    σ_t_effective — normalization denominator, chosen by policy:

        intra_batch_only:  σ_batch(t) if σ_historical(t) == 0 else σ_historical(t)
        widen:              max(σ_historical, μ_t × widen_factor)
        drop:               skip the task entirely (sample discarded from loss)

After normalization, advantages are clipped to `advantage_clip` to prevent
a single extreme reward from dominating the gradient.

All three policies are configurable via `configs/rl_phase11.yaml` §advantage.
Default is `intra_batch_only` because 4 of 16 Phase 10 tasks have
σ_historical = 0 (structured-output tasks with deterministic scoring) and
temperature sampling in the RL rollout still produces real intra-batch variance.
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

import numpy as np


ZeroStddevPolicy = Literal["intra_batch_only", "widen", "drop"]


@dataclass(frozen=True)
class AdvantageEstimate:
    """Per-call advantage output + diagnostics."""

    advantages: np.ndarray          # shape (N_kept,) — may be < input N under `drop`
    keep_mask: np.ndarray           # shape (N,) bool — False entries dropped
    effective_stddev: dict[str, float]  # task_id → σ used for normalization
    mean_advantage: float
    std_advantage: float
    n_dropped: int


def estimate_advantage(
    rewards: np.ndarray,
    task_ids: list[str],
    stddev_cache: dict[str, float],
    *,
    policy: ZeroStddevPolicy = "intra_batch_only",
    widen_factor: float = 0.05,
    min_stddev: float = 1e-4,
    advantage_clip: tuple[float, float] = (-5.0, 5.0),
    mean_cache: dict[str, float] | None = None,
) -> AdvantageEstimate:
    """Compute per-sample advantages.

    Parameters
    ----------
    rewards        : (N,) scalar reward per sample.
    task_ids       : length-N list mapping each sample to its task.
    stddev_cache   : task_id → historical stddev (may be 0.0). Missing keys
                     trigger the same zero-stddev policy as an explicit 0.
    policy         : how to handle zero-stddev tasks (see module docstring).
    widen_factor   : used only when policy == "widen".
    min_stddev     : floor applied after policy resolution — prevents
                     division-by-zero when even the batch collapses.
    advantage_clip : (low, high) range for final advantage clipping.
    mean_cache     : optional task_id → historical mean. Falls back to the
                     batch mean per task when absent.

    Returns
    -------
    AdvantageEstimate with normalized advantages, keep mask, and diagnostics.
    """
    n = len(rewards)
    if len(task_ids) != n:
        raise ValueError(
            f"task_ids length {len(task_ids)} != rewards length {n}"
        )
    if policy not in ("intra_batch_only", "widen", "drop"):
        raise ValueError(f"unknown zero_stddev_policy: {policy}")
    if advantage_clip[0] >= advantage_clip[1]:
        raise ValueError(f"invalid advantage_clip range: {advantage_clip}")

    rewards = np.asarray(rewards, dtype=np.float64)
    keep = np.ones(n, dtype=bool)
    effective_stddev: dict[str, float] = {}

    # Group sample indices by task for per-task stats.
    by_task: dict[str, list[int]] = {}
    for i, t in enumerate(task_ids):
        by_task.setdefault(t, []).append(i)

    # Compute batch-level mean + stddev per task (used by intra_batch_only
    # and as fallback mean).
    batch_mean: dict[str, float] = {}
    batch_stddev: dict[str, float] = {}
    for t, idxs in by_task.items():
        batch_mean[t] = float(np.mean(rewards[idxs]))
        batch_stddev[t] = float(np.std(rewards[idxs]))

    advantages_full = np.zeros(n, dtype=np.float64)

    for t, idxs in by_task.items():
        hist_sd = stddev_cache.get(t, 0.0) or 0.0
        hist_mean = (mean_cache or {}).get(t)
        mu = hist_mean if hist_mean is not None else batch_mean[t]

        # Resolve σ based on policy + historical data.
        if hist_sd > 0.0:
            sigma = hist_sd
        else:
            # Zero-stddev branch — apply configured policy.
            if policy == "intra_batch_only":
                sigma = batch_stddev[t]
            elif policy == "widen":
                sigma = max(abs(mu) * widen_factor, 0.0)
            elif policy == "drop":
                # Mark these samples for exclusion; skip computation.
                for i in idxs:
                    keep[i] = False
                effective_stddev[t] = 0.0
                continue
            else:  # pragma: no cover — guarded above
                raise AssertionError(f"unreachable policy: {policy}")

        # Apply minimum stddev floor.
        sigma = max(sigma, min_stddev)
        effective_stddev[t] = sigma

        # Normalize and clip.
        for i in idxs:
            a = (rewards[i] - mu) / sigma
            advantages_full[i] = a

    # Clip all kept advantages to config range.
    advantages_full = np.clip(advantages_full, advantage_clip[0], advantage_clip[1])

    advantages_kept = advantages_full[keep]
    n_dropped = int((~keep).sum())

    mean_adv = float(np.mean(advantages_kept)) if advantages_kept.size else 0.0
    std_adv = float(np.std(advantages_kept)) if advantages_kept.size else 0.0

    return AdvantageEstimate(
        advantages=advantages_kept,
        keep_mask=keep,
        effective_stddev=effective_stddev,
        mean_advantage=mean_adv,
        std_advantage=std_adv,
        n_dropped=n_dropped,
    )
