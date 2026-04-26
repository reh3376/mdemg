"""Reward sampler — reads benchmark_results rows for the GRPO step loop.

Provides the (prompt, response, reward, sampling_group, task_id) tuples that
the trainer batches up, plus cached per-task (mean, stddev) used by the
advantage estimator.

Key design points:
    - All reward math runs in-Python over the reward_vector JSONB. The schema
      stores per-metric rewards; GRPO consumes a scalar per sample, and the
      scalarization is a simple mean of the reward_vector entries (excluding
      judge_scores, which live in a separate column and are regression-only
      per Phase 11 plan).
    - Historical stddev is computed per task at sampler init time, not per
      step — pulling reward_vector for 80+ rows each step adds up fast.
    - Strategies for per-step sampling:
        random                    — uniform over matching rows
        weighted_by_inverse_count — upweight rare tasks
        stratified_by_group       — equal count from each T/C/J group
        stratified_by_task        — equal count per group, then equal weight
                                    per *task* within group (corrects for
                                    tasks with few rows being under-drawn
                                    under stratified_by_group)
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Literal

try:
    import psycopg
except ImportError:  # pragma: no cover
    psycopg = None  # type: ignore[assignment]


SamplingStrategy = Literal[
    "random",
    "weighted_by_inverse_count",
    "stratified_by_group",
    "stratified_by_task",
]


@dataclass(frozen=True)
class RewardSample:
    """One sample in the GRPO batch."""

    task_id: str
    run_idx: int
    sampling_group: str
    # The rendered prompt isn't in benchmark_results (it's derived from the
    # ULTS spec at benchmark time). The trainer re-renders via spec + row
    # context; the row carries the response + reward for reference.
    response_text: str
    reward_vector: dict[str, float]
    scalar_reward: float

    def to_diag(self) -> dict[str, Any]:
        return {
            "task_id": self.task_id,
            "run_idx": self.run_idx,
            "sampling_group": self.sampling_group,
            "scalar_reward": self.scalar_reward,
        }


def scalarize_reward(reward_vector: dict[str, Any]) -> float:
    """Convert the reward_vector JSONB dict → scalar.

    Mean of all numeric entries. Non-numeric values (strings, None) are
    skipped. An empty reward_vector returns 0.0 — this is almost always a
    Phase 10 error row and will be filtered by the preflight in practice.
    """
    if not reward_vector:
        return 0.0
    numeric = [
        float(v) for v in reward_vector.values()
        if isinstance(v, (int, float)) and not isinstance(v, bool)
    ]
    if not numeric:
        return 0.0
    return sum(numeric) / len(numeric)


def _tsdb_dsn(tsdb_cfg: dict[str, Any]) -> str:
    """Build DSN honoring TSDB_* env vars (same pattern as preflight.py)."""
    import os
    host = os.environ.get("TSDB_HOST", tsdb_cfg.get("host", "localhost"))
    port = os.environ.get("TSDB_PORT", str(tsdb_cfg.get("port", 5433)))
    user = os.environ.get("TSDB_USER", tsdb_cfg.get("user", "mdemg"))
    pwd = os.environ.get("TSDB_PASSWORD", tsdb_cfg.get("password", "mdemg_metrics"))
    db = os.environ.get("TSDB_DATABASE", tsdb_cfg.get("database", "mdemg_metrics"))
    return f"host={host} port={port} user={user} password={pwd} dbname={db}"


class RewardSampler:
    """Stateful sampler holding the row cache + per-task stats.

    Initialized once per training run; `sample_batch()` draws fresh batches
    without re-querying the DB every step.
    """

    def __init__(
        self,
        rows: list[dict[str, Any]],
        *,
        strategy: SamplingStrategy = "stratified_by_group",
        rng_seed: int = 0,
    ) -> None:
        """Build a sampler from an in-memory row list.

        Use `from_tsdb(...)` for the production path; the in-memory
        constructor is how unit tests inject fixtures.
        """
        if not rows:
            raise ValueError("reward sampler requires ≥1 row")
        self._rows = rows
        self._strategy = strategy

        import random
        self._rng = random.Random(rng_seed)

        # Per-task stats + group index for stratified sampling.
        self._by_task: dict[str, list[int]] = {}
        self._by_group: dict[str, list[int]] = {}
        self._mean_cache: dict[str, float] = {}
        self._stddev_cache: dict[str, float] = {}
        self._rebuild_indices()

    @classmethod
    def from_tsdb(
        cls,
        tsdb_cfg: dict[str, Any],
        *,
        run_ids: list[str] | None = None,
        strategy: SamplingStrategy = "stratified_by_group",
        rng_seed: int = 0,
    ) -> "RewardSampler":
        """Build by querying benchmark_results.

        If `run_ids` is empty or None, pulls the most recent completed
        benchmark_runs row (matches the config default).
        """
        if psycopg is None:
            raise RuntimeError(
                "psycopg not available; install 'psycopg[binary]' in the venv"
            )
        with psycopg.connect(_tsdb_dsn(tsdb_cfg), connect_timeout=5) as conn:
            with conn.cursor() as cur:
                if not run_ids:
                    cur.execute(
                        "SELECT run_id FROM benchmark_runs "
                        "WHERE completed_at IS NOT NULL "
                        "ORDER BY started_at DESC LIMIT 1"
                    )
                    r = cur.fetchone()
                    if r is None:
                        raise RuntimeError(
                            "no completed benchmark_runs; run Phase 10 first"
                        )
                    run_ids = [r[0]]
                placeholders = ",".join(["%s"] * len(run_ids))
                cur.execute(
                    f"SELECT task_id, run_idx, sampling_group, response_text, "
                    f"reward_vector "
                    f"FROM benchmark_results WHERE run_id IN ({placeholders})",
                    run_ids,
                )
                rows = [
                    {
                        "task_id": r[0],
                        "run_idx": r[1],
                        "sampling_group": r[2],
                        "response_text": r[3],
                        "reward_vector": r[4] or {},
                    }
                    for r in cur.fetchall()
                ]
        return cls(rows, strategy=strategy, rng_seed=rng_seed)

    # ── public accessors ────────────────────────────────────────────────────

    @property
    def task_ids(self) -> list[str]:
        """Unique task_ids in the cache, in stable lexicographic order."""
        return sorted(self._by_task.keys())

    @property
    def mean_cache(self) -> dict[str, float]:
        return dict(self._mean_cache)

    @property
    def stddev_cache(self) -> dict[str, float]:
        return dict(self._stddev_cache)

    @property
    def row_count(self) -> int:
        return len(self._rows)

    def zero_stddev_tasks(self) -> list[str]:
        """Task ids where σ_historical == 0. These trigger the configured
        zero-stddev policy in the advantage estimator."""
        return sorted(t for t, sd in self._stddev_cache.items() if sd == 0.0)

    # ── sampling ────────────────────────────────────────────────────────────

    def sample_batch(self, n: int) -> list[RewardSample]:
        """Draw `n` samples according to the configured strategy."""
        if n <= 0:
            raise ValueError(f"batch size must be positive; got {n}")

        if self._strategy == "random":
            picked = self._rng.choices(range(len(self._rows)), k=n)
        elif self._strategy == "weighted_by_inverse_count":
            weights = [
                1.0 / len(self._by_task[row["task_id"]])
                for row in self._rows
            ]
            picked = self._rng.choices(
                range(len(self._rows)), weights=weights, k=n
            )
        elif self._strategy == "stratified_by_group":
            # Distribute n across the T/C/J groups proportionally to group
            # size, at minimum 1 per group. Remainder goes to groups by the
            # largest-fraction rule.
            groups = list(self._by_group.keys())
            counts = {g: max(1, int(round(n * len(self._by_group[g]) / len(self._rows))))
                      for g in groups}
            # Adjust to match n exactly.
            total = sum(counts.values())
            while total != n:
                if total < n:
                    # Add to largest group first.
                    g = max(groups, key=lambda g: len(self._by_group[g]))
                    counts[g] += 1
                    total += 1
                else:
                    # Remove from smallest group (but keep ≥1 if possible).
                    g = min(groups, key=lambda g: counts[g] if counts[g] > 1 else 10**9)
                    counts[g] -= 1
                    total -= 1
            picked = []
            for g in groups:
                picked.extend(self._rng.choices(self._by_group[g], k=counts[g]))
        elif self._strategy == "stratified_by_task":
            # Two-level uniform draw: distribute n across groups by group
            # size (same as stratified_by_group), then within each group
            # pick a task uniformly across distinct task_ids and a row
            # uniformly within that task. This makes every TASK in a group
            # have equal expected draws, so a 5-row task is sampled at the
            # same rate as a 10-row task — the under-representation that
            # surfaced as the retrieval.query_classify regression at
            # kl_coef=0.05 (-20pp). See Phase 11 doc for analysis.
            groups = list(self._by_group.keys())
            counts = {g: max(1, int(round(n * len(self._by_group[g]) / len(self._rows))))
                      for g in groups}
            total = sum(counts.values())
            while total != n:
                if total < n:
                    g = max(groups, key=lambda g: len(self._by_group[g]))
                    counts[g] += 1; total += 1
                else:
                    g = min(groups, key=lambda g: counts[g] if counts[g] > 1 else 10**9)
                    counts[g] -= 1; total -= 1
            # Pre-compute per-group task lists for fast within-group draw.
            tasks_by_group: dict[str, list[str]] = {}
            for g in groups:
                tasks_by_group[g] = sorted({self._rows[i]["task_id"] for i in self._by_group[g]})
            picked = []
            for g in groups:
                tlist = tasks_by_group[g]
                for _ in range(counts[g]):
                    chosen_task = self._rng.choice(tlist)
                    picked.append(self._rng.choice(self._by_task[chosen_task]))
        else:  # pragma: no cover — validated at init
            raise ValueError(f"unknown strategy: {self._strategy}")

        return [self._build_sample(i) for i in picked]

    # ── internals ───────────────────────────────────────────────────────────

    def _rebuild_indices(self) -> None:
        """Populate by-task / by-group indices and per-task mean+stddev."""
        import statistics as stats

        self._by_task.clear()
        self._by_group.clear()
        for idx, row in enumerate(self._rows):
            self._by_task.setdefault(row["task_id"], []).append(idx)
            self._by_group.setdefault(row["sampling_group"], []).append(idx)

        for task, idxs in self._by_task.items():
            scalars = [
                scalarize_reward(self._rows[i]["reward_vector"]) for i in idxs
            ]
            self._mean_cache[task] = (
                sum(scalars) / len(scalars) if scalars else 0.0
            )
            # Population stddev (matches psycopg STDDEV_POP used in preflight).
            self._stddev_cache[task] = (
                stats.pstdev(scalars) if len(scalars) > 1 else 0.0
            )

    def _build_sample(self, idx: int) -> RewardSample:
        row = self._rows[idx]
        rv = row.get("reward_vector") or {}
        return RewardSample(
            task_id=row["task_id"],
            run_idx=row["run_idx"],
            sampling_group=row["sampling_group"],
            response_text=row.get("response_text") or "",
            reward_vector=dict(rv),
            scalar_reward=scalarize_reward(rv),
        )
