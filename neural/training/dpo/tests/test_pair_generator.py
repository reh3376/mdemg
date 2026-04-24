"""DPO pair generator unit tests."""
from __future__ import annotations

import json
from pathlib import Path

import pytest

from neural.training.dpo.pair_generator import (
    DpoPair,
    PairGeneratorConfig,
    _prompt_hash,
    _scalarize,
    build_manifest,
    generate_pairs,
    write_outputs,
)


def _row(task, idx, group, rewards, resp=None):
    return {
        "task_id": task,
        "run_idx": idx,
        "sampling_group": group,
        "response_text": resp if resp is not None else f"resp_{task}_{idx}",
        "reward_vector": rewards,
    }


# ── scalarize ───────────────────────────────────────────────────────────────


def test_scalarize_mean_default():
    cfg = PairGeneratorConfig()
    assert abs(_scalarize({"a": 1.0, "b": 3.0}, "t", cfg) - 2.0) < 1e-9


def test_scalarize_weighted():
    cfg = PairGeneratorConfig(
        reward_aggregation="weighted",
        reward_weights={"t": {"a": 0.75, "b": 0.25}},
    )
    # 0.75 * 1.0 + 0.25 * 3.0 = 0.75 + 0.75 = 1.5
    assert abs(_scalarize({"a": 1.0, "b": 3.0}, "t", cfg) - 1.5) < 1e-9


def test_scalarize_weighted_falls_back_to_mean_when_task_missing():
    cfg = PairGeneratorConfig(
        reward_aggregation="weighted",
        reward_weights={"other_task": {"a": 1.0}},
    )
    # task "t" not in weights → mean path
    assert abs(_scalarize({"a": 1.0, "b": 3.0}, "t", cfg) - 2.0) < 1e-9


# ── prompt hashing ──────────────────────────────────────────────────────────


def test_prompt_hash_default_groups_by_task():
    cfg = PairGeneratorConfig()
    r1 = _row("t0", 0, "T", {"x": 0.5}, resp="A")
    r2 = _row("t0", 1, "T", {"x": 0.6}, resp="B")  # same task, different resp
    r3 = _row("t1", 0, "T", {"x": 0.7}, resp="A")
    assert _prompt_hash(r1, cfg) == _prompt_hash(r2, cfg)
    assert _prompt_hash(r1, cfg) != _prompt_hash(r3, cfg)


def test_prompt_hash_task_first_tokens_distinguishes_by_response():
    cfg = PairGeneratorConfig(prompt_hash_strategy="task_first_tokens",
                               task_first_tokens_n=16)
    r1 = _row("t0", 0, "T", {"x": 0.5}, resp="A" * 100)
    r2 = _row("t0", 1, "T", {"x": 0.6}, resp="B" * 100)
    assert _prompt_hash(r1, cfg) != _prompt_hash(r2, cfg)


# ── pair generation ─────────────────────────────────────────────────────────


def test_generate_pairs_basic_delta_filter():
    """Pairs below threshold excluded."""
    rows = [
        _row("t0", 0, "T", {"x": 0.9}),
        _row("t0", 1, "T", {"x": 0.5}),  # delta 0.4 > 0.15 → pair
        _row("t0", 2, "T", {"x": 0.8}),  # delta 0.1 (vs 0.9) < 0.15 → skip against 0.9
    ]
    # Sorted desc: [0.9, 0.8, 0.5]. Against 0.9: 0.8 skipped, 0.5 kept.
    # Against 0.8: 0.5 kept (delta 0.3).
    pairs = generate_pairs(rows, PairGeneratorConfig(reward_delta_threshold=0.15))
    # Expect pairs: (0.9, 0.5) and (0.8, 0.5)
    assert len(pairs) == 2
    deltas = sorted(p.reward_delta for p in pairs)
    assert abs(deltas[0] - 0.3) < 1e-9
    assert abs(deltas[1] - 0.4) < 1e-9


def test_generate_pairs_respects_max_per_bucket():
    """max_pairs_per_bucket caps output per bucket."""
    rows = [_row("t0", i, "T", {"x": 1.0 - i * 0.2}) for i in range(5)]
    # 5 items, 2-combinations satisfying delta≥0.3: many. Cap should be 2.
    pairs = generate_pairs(
        rows, PairGeneratorConfig(reward_delta_threshold=0.3, max_pairs_per_bucket=2)
    )
    assert len(pairs) == 2


def test_generate_pairs_excludes_zero_delta_pairs():
    rows = [
        _row("t0", 0, "T", {"x": 0.5}),
        _row("t0", 1, "T", {"x": 0.5}),  # identical
    ]
    pairs = generate_pairs(rows, PairGeneratorConfig())
    assert pairs == []


def test_generate_pairs_multiple_tasks_independent():
    rows = [
        _row("t0", 0, "T", {"x": 0.9}),
        _row("t0", 1, "T", {"x": 0.5}),  # delta 0.4
        _row("t1", 0, "C", {"x": 0.8}),
        _row("t1", 1, "C", {"x": 0.6}),  # delta 0.2
    ]
    pairs = generate_pairs(rows, PairGeneratorConfig(reward_delta_threshold=0.15))
    task_counts = {}
    for p in pairs:
        task_counts[p.task_id] = task_counts.get(p.task_id, 0) + 1
    assert task_counts == {"t0": 1, "t1": 1}


def test_generate_pairs_sorts_chosen_higher():
    rows = [
        _row("t0", 1, "T", {"x": 0.2}),   # purposely reversed input order
        _row("t0", 0, "T", {"x": 0.9}),
    ]
    pairs = generate_pairs(rows, PairGeneratorConfig(reward_delta_threshold=0.15))
    assert len(pairs) == 1
    assert pairs[0].chosen_reward > pairs[0].rejected_reward
    assert pairs[0].chosen_run_idx == 0  # the 0.9 row


def test_generate_pairs_skips_single_row_buckets():
    rows = [_row("t_solo", 0, "T", {"x": 0.5})]
    assert generate_pairs(rows, PairGeneratorConfig()) == []


def test_pair_id_stable_across_calls():
    rows = [
        _row("t0", 0, "T", {"x": 0.9}),
        _row("t0", 1, "T", {"x": 0.5}),
    ]
    p1 = generate_pairs(rows, PairGeneratorConfig())[0]
    p2 = generate_pairs(rows, PairGeneratorConfig())[0]
    assert p1.pair_id == p2.pair_id


# ── manifest ────────────────────────────────────────────────────────────────


def test_manifest_records_under_floor_tasks():
    pairs = [
        DpoPair(pair_id="p", task_id="short", sampling_group="T", prompt_hash="h",
                chosen="a", rejected="b",
                chosen_reward=0.9, rejected_reward=0.5, reward_delta=0.4,
                chosen_run_idx=0, rejected_run_idx=1),
    ]
    cfg = PairGeneratorConfig(min_pairs_per_task=5)
    manifest = build_manifest(pairs, cfg, source_run_ids=["run1"])
    assert manifest["tasks_under_floor"] == {"short": 1}
    assert manifest["total_pairs"] == 1
    assert manifest["source_run_ids"] == ["run1"]


def test_manifest_histogram_bucketing():
    pairs = [
        DpoPair(pair_id=f"p{i}", task_id="t", sampling_group="T", prompt_hash="h",
                chosen="a", rejected="b",
                chosen_reward=0.5 + i * 0.05, rejected_reward=0.0,
                reward_delta=0.5 + i * 0.05,
                chosen_run_idx=i, rejected_run_idx=i + 1)
        for i in range(5)
    ]
    manifest = build_manifest(pairs, PairGeneratorConfig(reward_delta_threshold=0.5),
                               source_run_ids=[])
    hist = manifest["reward_delta_histogram"]
    assert sum(hist["counts"]) == 5
    assert hist["lo"] == 0.5
    assert abs(hist["hi"] - 0.7) < 1e-9


def test_manifest_empty_pairs():
    manifest = build_manifest([], PairGeneratorConfig(), source_run_ids=["r1"])
    assert manifest["total_pairs"] == 0
    assert manifest["per_task_coverage"] == {}
    assert manifest["tasks_under_floor"] == {}


# ── write_outputs integration ───────────────────────────────────────────────


def test_write_outputs_round_trip(tmp_path: Path):
    rows = [
        _row("t0", 0, "T", {"a": 1.0, "b": 0.8}),   # scalar = 0.9
        _row("t0", 1, "T", {"a": 0.5, "b": 0.5}),   # scalar = 0.5  delta 0.4
    ]
    pairs = generate_pairs(rows, PairGeneratorConfig())
    manifest = build_manifest(pairs, PairGeneratorConfig(), source_run_ids=["run_x"])
    pairs_path = tmp_path / "pairs.jsonl"
    manifest_path = tmp_path / "manifest.json"
    write_outputs(pairs, manifest, pairs_path, manifest_path)

    # Pairs JSONL is well-formed.
    lines = pairs_path.read_text().strip().splitlines()
    assert len(lines) == 1
    obj = json.loads(lines[0])
    assert obj["task_id"] == "t0"
    assert abs(obj["reward_delta"] - 0.4) < 1e-9

    # Manifest contains a SHA of the pairs file.
    m = json.loads(manifest_path.read_text())
    assert "pairs_jsonl_sha256" in m
    assert len(m["pairs_jsonl_sha256"]) == 64
