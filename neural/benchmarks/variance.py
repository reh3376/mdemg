"""Per-task reward variance aggregator.

Phase 11 GRPO reads `stddev` as the advantage-normalization denominator:
    advantage = (reward - mean) / max(stddev, eps)

Accepts a stream of (task_id, run_idx, reward_vector, judge_scores) tuples,
emits per-task mean/stddev/min/max over the N repeat runs configured in
`configs/benchmark_phase10.yaml :: n_runs`.

`reward_vector` is the dict returned by `reward_functions.compute_reward()`;
`judge_scores` is the dict of LLM-judge metric-name → score (0-1). Both are
aggregated: per-metric mean/stddev plus an overall scalar reward defined as
the unweighted mean of all numeric entries in reward_vector for that run.
"""
from __future__ import annotations

import math
import statistics
from dataclasses import dataclass, field
from typing import Any


@dataclass
class RunSample:
    """One invocation of a task."""
    task_id: str
    run_idx: int
    reward_vector: dict[str, float]
    judge_scores: dict[str, float] = field(default_factory=dict)


@dataclass
class TaskAggregate:
    """Aggregate across N runs of a single task."""
    task_id: str
    n: int
    overall_mean: float
    overall_stddev: float
    overall_min: float
    overall_max: float
    # per-metric mean/stddev, union of reward keys and judge keys
    per_metric_mean: dict[str, float]
    per_metric_stddev: dict[str, float]

    def to_dict(self) -> dict[str, Any]:
        return {
            "task_id": self.task_id,
            "n": self.n,
            "overall_mean": self.overall_mean,
            "overall_stddev": self.overall_stddev,
            "overall_min": self.overall_min,
            "overall_max": self.overall_max,
            "per_metric_mean": dict(self.per_metric_mean),
            "per_metric_stddev": dict(self.per_metric_stddev),
        }


class RunAggregator:
    """Accumulate RunSample instances, emit TaskAggregate on demand."""

    def __init__(self) -> None:
        self._samples: dict[str, list[RunSample]] = {}

    def add(self, sample: RunSample) -> None:
        self._samples.setdefault(sample.task_id, []).append(sample)

    def task_ids(self) -> list[str]:
        return sorted(self._samples)

    @staticmethod
    def _overall_scalar(sample: RunSample) -> float | None:
        """Scalar reward for a single run.

        Defined as the unweighted mean of numeric reward_vector values.
        Judge scores are tracked separately and do NOT feed the scalar
        reward (the judge is subjective-only; variance bound must come from
        deterministic reward functions for GRPO stability).

        Returns ``None`` when every reward_vector entry is non-numeric or
        NaN — caller must exclude these from aggregate stats, not coerce to 0.
        """
        vals = [float(v) for v in sample.reward_vector.values()
                if isinstance(v, (int, float)) and not math.isnan(float(v))]
        if not vals:
            return None
        return sum(vals) / len(vals)

    def aggregate(self, task_id: str) -> TaskAggregate:
        samples = self._samples.get(task_id)
        if not samples:
            raise KeyError(f"no samples for task_id={task_id!r}")

        overalls = [o for o in (self._overall_scalar(s) for s in samples)
                    if o is not None]
        n = len(overalls)
        if n == 0:
            # All runs had no numeric reward — degenerate but non-fatal.
            return TaskAggregate(
                task_id=task_id, n=0,
                overall_mean=0.0, overall_stddev=0.0,
                overall_min=0.0, overall_max=0.0,
                per_metric_mean={}, per_metric_stddev={},
            )

        overall_stddev = statistics.pstdev(overalls) if n > 1 else 0.0

        # union of keys across runs (per-run missing keys → dropped from that run's
        # contribution but not blocking)
        all_keys: set[str] = set()
        for s in samples:
            all_keys.update(s.reward_vector.keys())
            all_keys.update(s.judge_scores.keys())

        per_metric_mean: dict[str, float] = {}
        per_metric_stddev: dict[str, float] = {}
        for key in sorted(all_keys):
            series: list[float] = []
            for s in samples:
                v = s.reward_vector.get(key)
                if v is None:
                    v = s.judge_scores.get(key)
                if v is None:
                    continue
                try:
                    fv = float(v)
                except (TypeError, ValueError):
                    continue
                if math.isnan(fv):
                    continue
                series.append(fv)
            if not series:
                continue
            per_metric_mean[key] = sum(series) / len(series)
            per_metric_stddev[key] = statistics.pstdev(series) if len(series) > 1 else 0.0

        return TaskAggregate(
            task_id=task_id,
            n=n,
            overall_mean=sum(overalls) / n,
            overall_stddev=overall_stddev,
            overall_min=min(overalls),
            overall_max=max(overalls),
            per_metric_mean=per_metric_mean,
            per_metric_stddev=per_metric_stddev,
        )

    def aggregate_all(self) -> list[TaskAggregate]:
        return [self.aggregate(tid) for tid in self.task_ids()]
