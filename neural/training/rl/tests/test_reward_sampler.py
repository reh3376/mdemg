"""Reward sampler unit tests — scalarize, index build, 3 sampling strategies."""
from __future__ import annotations

import statistics
from collections import Counter

import pytest

from neural.training.rl.reward_sampler import (
    RewardSample,
    RewardSampler,
    scalarize_reward,
)

TOL = 1e-9


# ── scalarize_reward ────────────────────────────────────────────────────────

def test_scalarize_empty_vector_returns_zero():
    assert scalarize_reward({}) == 0.0
    assert scalarize_reward(None) == 0.0  # type: ignore[arg-type]


def test_scalarize_simple_mean():
    assert abs(scalarize_reward({"a": 1.0, "b": 3.0}) - 2.0) < TOL


def test_scalarize_skips_bools_and_strings():
    # bools are subclasses of int — must be excluded explicitly.
    rv = {"a": 1.0, "b": 3.0, "c": True, "d": "skip_me", "e": None}
    assert abs(scalarize_reward(rv) - 2.0) < TOL


def test_scalarize_all_non_numeric_returns_zero():
    assert scalarize_reward({"a": "s", "b": None, "c": True}) == 0.0


# ── RewardSampler construction + indices ────────────────────────────────────

def _mk_row(task, idx, group, rewards, resp="resp"):
    return {
        "task_id": task,
        "run_idx": idx,
        "sampling_group": group,
        "response_text": resp,
        "reward_vector": rewards,
    }


def test_empty_rows_raises():
    with pytest.raises(ValueError, match=">=?1 row|≥1 row"):
        RewardSampler([])


def test_mean_and_stddev_cache_matches_pstdev():
    # Task t0: scalars = [mean({a:1,b:3})=2, mean({a:2,b:4})=3] → mean=2.5, pstdev=0.5
    rows = [
        _mk_row("t0", 0, "T", {"a": 1.0, "b": 3.0}),
        _mk_row("t0", 1, "T", {"a": 2.0, "b": 4.0}),
        _mk_row("t1", 0, "C", {"a": 5.0}),  # single → pstdev=0 by our convention
    ]
    s = RewardSampler(rows)
    assert abs(s.mean_cache["t0"] - 2.5) < TOL
    assert abs(s.stddev_cache["t0"] - statistics.pstdev([2.0, 3.0])) < TOL
    assert abs(s.mean_cache["t1"] - 5.0) < TOL
    assert s.stddev_cache["t1"] == 0.0
    assert s.task_ids == ["t0", "t1"]


def test_zero_stddev_tasks_listing():
    rows = [
        _mk_row("deterministic", 0, "C", {"x": 1.0}),
        _mk_row("deterministic", 1, "C", {"x": 1.0}),  # same reward
        _mk_row("variable", 0, "T", {"x": 1.0}),
        _mk_row("variable", 1, "T", {"x": 2.0}),
    ]
    s = RewardSampler(rows)
    assert s.zero_stddev_tasks() == ["deterministic"]


def test_row_count():
    rows = [_mk_row("t0", i, "T", {"a": float(i)}) for i in range(5)]
    assert RewardSampler(rows).row_count == 5


# ── sampling strategies ─────────────────────────────────────────────────────

def _make_sampler(strategy, seed=42):
    rows = []
    # 3 groups, uneven sizes: T=6, C=3, J=1 → total 10
    for i in range(6):
        rows.append(_mk_row(f"t_T_{i % 2}", i, "T", {"x": float(i)}))
    for i in range(3):
        rows.append(_mk_row(f"t_C_{i % 2}", i, "C", {"x": float(i)}))
    rows.append(_mk_row("t_J_0", 0, "J", {"x": 0.5}))
    return RewardSampler(rows, strategy=strategy, rng_seed=seed)


def test_sample_batch_returns_correct_count_random():
    s = _make_sampler("random")
    batch = s.sample_batch(8)
    assert len(batch) == 8
    assert all(isinstance(x, RewardSample) for x in batch)


def test_sample_batch_weighted_by_inverse_count():
    s = _make_sampler("weighted_by_inverse_count")
    batch = s.sample_batch(20)
    assert len(batch) == 20


def test_stratified_by_group_all_groups_covered():
    """stratified_by_group guarantees ≥1 from each group (when n ≥ n_groups)."""
    s = _make_sampler("stratified_by_group")
    batch = s.sample_batch(10)
    groups = Counter(sample.sampling_group for sample in batch)
    assert set(groups) == {"T", "C", "J"}
    assert sum(groups.values()) == 10


def test_stratified_total_matches_n_small_batch():
    """Small batch still sums to n (remainder adjustment logic)."""
    s = _make_sampler("stratified_by_group")
    for n in (3, 4, 5, 7, 9):
        batch = s.sample_batch(n)
        assert len(batch) == n, f"batch size {len(batch)} != requested {n}"


def test_negative_batch_size_raises():
    s = _make_sampler("random")
    with pytest.raises(ValueError, match="positive"):
        s.sample_batch(0)
    with pytest.raises(ValueError, match="positive"):
        s.sample_batch(-5)


def test_sampled_rewards_match_source_vector():
    """scalar_reward on a sample == scalarize_reward(its reward_vector)."""
    rows = [_mk_row("t0", 0, "T", {"a": 2.0, "b": 4.0})]
    s = RewardSampler(rows)
    sample = s.sample_batch(1)[0]
    assert abs(sample.scalar_reward - 3.0) < TOL
    assert sample.reward_vector == {"a": 2.0, "b": 4.0}
    assert sample.task_id == "t0"
    assert sample.sampling_group == "T"


def test_rng_seed_reproducibility():
    rows = [_mk_row("t0", i, "T", {"x": float(i)}) for i in range(10)]
    a = RewardSampler(rows, strategy="random", rng_seed=123).sample_batch(5)
    b = RewardSampler(rows, strategy="random", rng_seed=123).sample_batch(5)
    assert [x.run_idx for x in a] == [x.run_idx for x in b]


def test_to_diag_shape():
    rows = [_mk_row("t0", 0, "T", {"x": 1.0})]
    sample = RewardSampler(rows).sample_batch(1)[0]
    diag = sample.to_diag()
    assert set(diag) == {"task_id", "run_idx", "sampling_group", "scalar_reward"}


def test_stratified_by_task_equalizes_within_group():
    """stratified_by_task: draws are uniform across TASKS in a group,
    independent of how many rows each task has. Validates the fix for the
    Phase 11 retrieval.query_classify regression — small-row-count tasks
    were under-represented under stratified_by_group."""
    import collections
    # C-group: task A has 10 rows, task B has 5 rows. Under
    # stratified_by_group, A is drawn 2x as often as B. Under
    # stratified_by_task, both should be drawn ~equally.
    rows = []
    for i in range(10):
        rows.append(_mk_row("c.big", i, "C", {"acc": 0.5}))
    for i in range(5):
        rows.append(_mk_row("c.small", i, "C", {"acc": 0.5}))
    s = RewardSampler(rows, strategy="stratified_by_task", rng_seed=0)

    # Draw a large batch so the law-of-large-numbers gives a stable ratio.
    batch = s.sample_batch(2000)
    counts = collections.Counter(x.task_id for x in batch)
    big = counts["c.big"]
    small = counts["c.small"]
    # Expect roughly equal: ratio should be near 1.0 ± 0.1
    ratio = big / small
    assert 0.85 < ratio < 1.15, f"big/small={ratio:.3f} (counts: big={big} small={small})"


def test_stratified_by_group_undersamples_low_row_tasks():
    """Confirms the bias that motivates stratified_by_task. Under
    stratified_by_group, the 10-row task should be drawn ~2x as often as
    the 5-row task (linear with row count)."""
    import collections
    rows = []
    for i in range(10):
        rows.append(_mk_row("c.big", i, "C", {"acc": 0.5}))
    for i in range(5):
        rows.append(_mk_row("c.small", i, "C", {"acc": 0.5}))
    s = RewardSampler(rows, strategy="stratified_by_group", rng_seed=0)
    batch = s.sample_batch(2000)
    counts = collections.Counter(x.task_id for x in batch)
    ratio = counts["c.big"] / counts["c.small"]
    # Should be close to 10/5 = 2.0 (the bias we're fixing)
    assert 1.7 < ratio < 2.3, f"expected ~2.0; got {ratio:.3f}"


def test_stratified_by_task_total_matches_n():
    rows = [_mk_row(f"t{i//3}.x", i, "T", {"x": 1.0}) for i in range(15)]
    rows += [_mk_row(f"c{i//2}.x", i, "C", {"x": 1.0}) for i in range(10)]
    s = RewardSampler(rows, strategy="stratified_by_task", rng_seed=42)
    for n in (1, 8, 32, 100):
        batch = s.sample_batch(n)
        assert len(batch) == n
