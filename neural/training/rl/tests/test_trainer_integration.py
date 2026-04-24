"""Trainer integration test — Epic 2 gate.

Fixture: 3 tasks × N=2 rows × 20 steps with mocked MLX. Verifies:
  - step loop runs max_steps when no early-stop
  - every step writes a row to the SQL sidecar
  - run_start + run_complete bookends are emitted
  - early-stop fires when val_reward monotonically degrades past threshold
  - checkpoint callback invoked on completion
  - drop-policy path works end-to-end when intra-batch variance is zero
"""
from __future__ import annotations

from pathlib import Path

import numpy as np
import pytest

from neural.training.rl.reward_sampler import RewardSampler
from neural.training.rl.trainer import (
    GRPOTrainer,
    GRPOTrainerConfig,
    RolloutResult,
    SqlSidecarPersistence,
)


def _rows_fixture():
    """3 tasks (T/C/J) × 2 rows each = 6 rows, variable rewards."""
    rows = []
    # task_T (T group, variable rewards → nonzero stddev)
    rows.append({"task_id": "task_T", "run_idx": 0, "sampling_group": "T",
                 "response_text": "r", "reward_vector": {"x": 0.5}})
    rows.append({"task_id": "task_T", "run_idx": 1, "sampling_group": "T",
                 "response_text": "r", "reward_vector": {"x": 0.8}})
    # task_C (C group, variable)
    rows.append({"task_id": "task_C", "run_idx": 0, "sampling_group": "C",
                 "response_text": "r", "reward_vector": {"x": 0.7}})
    rows.append({"task_id": "task_C", "run_idx": 1, "sampling_group": "C",
                 "response_text": "r", "reward_vector": {"x": 0.9}})
    # task_J (J group, deterministic → zero historical stddev)
    rows.append({"task_id": "task_J", "run_idx": 0, "sampling_group": "J",
                 "response_text": "r", "reward_vector": {"x": 1.0}})
    rows.append({"task_id": "task_J", "run_idx": 1, "sampling_group": "J",
                 "response_text": "r", "reward_vector": {"x": 1.0}})
    return rows


def _base_cfg(tmp_path: Path, **overrides):
    defaults = dict(
        max_steps=20,
        batch_size=6,
        eval_interval_steps=5,
        clip_ratio=0.2,
        kl_coef=0.01,
        entropy_coef=0.0,
        zero_stddev_policy="intra_batch_only",
        widen_factor=0.05,
        min_stddev=1e-4,
        advantage_clip=(-5.0, 5.0),
        early_stop_threshold=0.95,
        early_stop_patience=2,
        base_model_path="/tmp/base",
        base_model_sha="abc123",
        adapter_manifest_sha=None,
        output_path=str(tmp_path / "out_adapter"),
        config_sha="cfg_sha",
        phase10_baseline_run_id="phase10_run",
        sidecar_path=tmp_path / "rl_run.sql",
    )
    defaults.update(overrides)
    return GRPOTrainerConfig(**defaults)


def _mk_rollout_fn(noise_seed=0):
    """Deterministic mock rollout: reward from sample, logprobs with small noise."""
    rng = np.random.default_rng(noise_seed)

    def rollout_fn(batch):
        out = []
        for s in batch:
            # Small per-sample perturbation so intra-batch variance is nonzero.
            noise = rng.normal(0, 0.01)
            out.append(RolloutResult(
                task_id=s.task_id,
                response_text="mock_response",
                scalar_reward=s.scalar_reward + noise,
                logprob_new=-1.0 + noise,
                logprob_old=-1.0,
                logprob_ref=-1.1,
            ))
        return out
    return rollout_fn


# ── tests ───────────────────────────────────────────────────────────────────


def test_full_step_loop_runs_max_steps_without_early_stop(tmp_path):
    """20 steps, eval returns a monotonically improving reward → no early-stop."""
    sampler = RewardSampler(_rows_fixture(), strategy="stratified_by_group", rng_seed=0)
    cfg = _base_cfg(tmp_path)

    eval_scores = iter([0.5, 0.55, 0.6, 0.65])  # step 5, 10, 15, 20
    optimizer_calls = []
    checkpoint_calls = []

    trainer = GRPOTrainer(
        config=cfg,
        sampler=sampler,
        rollout_fn=_mk_rollout_fn(),
        optimizer_step_fn=lambda loss_v: optimizer_calls.append(loss_v),
        eval_fn=lambda step: next(eval_scores),
        checkpoint_fn=lambda path, step: checkpoint_calls.append((path, step)),
    )
    result = trainer.train()

    assert result.total_steps == 20
    assert not result.early_stopped
    assert result.early_stop_reason is None
    assert len(result.step_history) == 20
    assert len(result.eval_history) == 4  # evaluated at 5/10/15/20
    assert len(optimizer_calls) == 20
    assert len(checkpoint_calls) == 1  # one checkpoint on completion
    assert checkpoint_calls[0][1] == 20


def test_sidecar_file_written_with_run_and_step_inserts(tmp_path):
    sampler = RewardSampler(_rows_fixture(), strategy="stratified_by_group", rng_seed=0)
    cfg = _base_cfg(tmp_path, max_steps=5, eval_interval_steps=10)  # no eval fires
    trainer = GRPOTrainer(
        config=cfg,
        sampler=sampler,
        rollout_fn=_mk_rollout_fn(),
        optimizer_step_fn=lambda v: None,
        eval_fn=lambda step: 0.5,
        checkpoint_fn=None,
    )
    trainer.train()

    sidecar_txt = cfg.sidecar_path.read_text()
    assert "INSERT INTO rl_training_runs" in sidecar_txt
    assert "UPDATE rl_training_runs" in sidecar_txt
    # 5 step inserts
    assert sidecar_txt.count("INSERT INTO rl_training_steps") == 5


def test_early_stop_fires_on_degrading_val_reward(tmp_path):
    """RL early-stop: val_reward < best × 0.95 for 2 consecutive evals."""
    sampler = RewardSampler(_rows_fixture(), strategy="stratified_by_group", rng_seed=0)
    cfg = _base_cfg(tmp_path, max_steps=20, eval_interval_steps=2)

    # Evals at steps 2, 4, 6, 8, 10, ...
    # Establish best at step 2 = 1.0, then degrade to 0.9 (1.0 × 0.95 = 0.95, 0.9 < 0.95 → bad)
    # two consecutive bad evals trigger → fires at step 6.
    eval_scores = iter([1.0, 0.9, 0.9, 0.9, 0.9, 0.9, 0.9, 0.9, 0.9, 0.9])

    trainer = GRPOTrainer(
        config=cfg,
        sampler=sampler,
        rollout_fn=_mk_rollout_fn(),
        optimizer_step_fn=lambda v: None,
        eval_fn=lambda step: next(eval_scores),
        checkpoint_fn=None,
    )
    result = trainer.train()

    assert result.early_stopped
    assert result.early_stop_reason is not None
    assert "early-stop fired" in result.early_stop_reason.lower()
    # Fired at step 6 (2 consecutive bad evals after the first good one).
    assert result.total_steps == 6


def test_drop_policy_handles_zero_batch(tmp_path):
    """policy=drop + deterministic task means that task's samples are masked.

    When batch contains ONLY dropped tasks, n_samples=0 → diagnostic row but
    no optimizer step. Loop still progresses.
    """
    # Only task_J rows — deterministic, zero historical stddev.
    deterministic_rows = [
        {"task_id": "task_J", "run_idx": 0, "sampling_group": "J",
         "response_text": "r", "reward_vector": {"x": 1.0}},
        {"task_id": "task_J", "run_idx": 1, "sampling_group": "J",
         "response_text": "r", "reward_vector": {"x": 1.0}},
    ]
    sampler = RewardSampler(deterministic_rows, strategy="random", rng_seed=0)
    cfg = _base_cfg(tmp_path, max_steps=3, batch_size=2, eval_interval_steps=10,
                     zero_stddev_policy="drop")

    optimizer_calls = []
    trainer = GRPOTrainer(
        config=cfg,
        sampler=sampler,
        rollout_fn=_mk_rollout_fn(),
        optimizer_step_fn=lambda v: optimizer_calls.append(v),
        eval_fn=lambda step: 0.5,
        checkpoint_fn=None,
    )
    result = trainer.train()

    assert result.total_steps == 3
    assert len(optimizer_calls) == 0  # no step taken (all dropped)
    # Every diagnostic row shows n_samples=0, n_dropped=batch_size
    for step in result.step_history:
        assert step["n_samples"] == 0
        assert step["n_dropped"] == 2


def test_rollout_length_mismatch_raises(tmp_path):
    sampler = RewardSampler(_rows_fixture(), strategy="stratified_by_group", rng_seed=0)
    cfg = _base_cfg(tmp_path, max_steps=1, eval_interval_steps=10)

    def bad_rollout(batch):
        return [RolloutResult("t", "r", 0.0, 0.0, 0.0, 0.0)]  # len 1 vs batch 6

    trainer = GRPOTrainer(
        config=cfg,
        sampler=sampler,
        rollout_fn=bad_rollout,
        optimizer_step_fn=lambda v: None,
        eval_fn=lambda step: 0.5,
    )
    with pytest.raises(RuntimeError, match="rollout_fn returned"):
        trainer.train()


def test_step_diagnostics_populated(tmp_path):
    """Per-step diag must include all keys expected by the V0013 schema."""
    sampler = RewardSampler(_rows_fixture(), strategy="stratified_by_group", rng_seed=0)
    cfg = _base_cfg(tmp_path, max_steps=2, eval_interval_steps=10)

    trainer = GRPOTrainer(
        config=cfg,
        sampler=sampler,
        rollout_fn=_mk_rollout_fn(),
        optimizer_step_fn=lambda v: None,
        eval_fn=lambda step: 0.5,
    )
    result = trainer.train()

    required_keys = {
        "step_idx", "policy_loss", "kl_loss", "total_loss",
        "advantage_mean", "advantage_std", "reward_batch_mean",
        "mean_ratio", "frac_clipped", "n_samples", "n_dropped",
    }
    for step in result.step_history:
        assert required_keys.issubset(step.keys())
        assert step["n_samples"] == 6  # batch_size


def test_run_id_from_external_source_respected(tmp_path):
    sampler = RewardSampler(_rows_fixture(), strategy="random", rng_seed=0)
    cfg = _base_cfg(tmp_path, max_steps=1, eval_interval_steps=10)

    trainer = GRPOTrainer(
        config=cfg,
        sampler=sampler,
        rollout_fn=_mk_rollout_fn(),
        optimizer_step_fn=lambda v: None,
        eval_fn=lambda step: 0.5,
        run_id="externally_supplied_id",
    )
    result = trainer.train()
    assert result.run_id == "externally_supplied_id"
    sidecar_txt = cfg.sidecar_path.read_text()
    assert "externally_supplied_id" in sidecar_txt


def test_persistence_sidecar_sql_injection_safe(tmp_path):
    """Single-quote in string fields must be escaped."""
    persist = SqlSidecarPersistence(tmp_path / "t.sql")
    persist.write_run_start({
        "run_id": "id'; DROP TABLE runs;--",
        "base_model_path": "/p",
        "base_model_sha": "s",
        "adapter_manifest_sha": None,
        "output_path": "/o",
        "config_sha": "c",
        "started_at": "2026-04-24T00:00:00Z",
        "phase10_baseline_run_id": None,
    })
    persist.flush()
    txt = (tmp_path / "t.sql").read_text()
    # Injection string must be escaped as doubled quotes, not raw.
    assert "DROP TABLE runs" in txt  # still present as a string
    assert "'id''; DROP TABLE runs;--'" in txt  # but properly quoted
