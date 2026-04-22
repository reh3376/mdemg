"""Unit tests for early_stop.py (Sprint FT-LORA-E Epic 4)."""

from __future__ import annotations

import pytest

from training.early_stop import (
    EarlyStopMonitor,
    RL_VAL_REWARD_RE,
    SFT_VAL_LOSS_RE,
)


# ────────────── regex pinned to mlx_lm 0.31.2 trainer.py:302-307 ──────────────


def test_sft_regex_matches_trainer_format():
    m = SFT_VAL_LOSS_RE.search("Iter 100: Val loss 0.531, Val took 4.217s")
    assert m and m.group(1) == "100" and m.group(2) == "0.531"


def test_sft_regex_allows_leading_noise():
    m = SFT_VAL_LOSS_RE.search("[mlx_lm] Iter 200: Val loss 1.234 foo")
    assert m and m.group(2) == "1.234"


def test_sft_regex_rejects_train_loss_lines():
    """Ensure we don't accidentally match training-loss lines."""
    assert SFT_VAL_LOSS_RE.search("Iter 1: Train loss 0.888") is None


def test_rl_regex_handles_negative_reward():
    m = RL_VAL_REWARD_RE.search("Iter 50: Val reward -0.42")
    assert m and m.group(2) == "-0.42"


# ────────────── SFT fire semantics ──────────────


def test_sft_first_value_is_baseline_no_fire():
    mon = EarlyStopMonitor(mode="sft", ratio=1.05, patience=2)
    assert mon.on_line("Iter 100: Val loss 0.500") is False
    assert mon.best == 0.500
    assert not mon.fired


def test_sft_improving_trajectory_no_fire():
    mon = EarlyStopMonitor(mode="sft", ratio=1.05, patience=2)
    for line in [
        "Iter 100: Val loss 0.600",
        "Iter 200: Val loss 0.550",
        "Iter 300: Val loss 0.500",
        "Iter 400: Val loss 0.480",
    ]:
        assert mon.on_line(line) is False
    assert mon.best == 0.480
    assert mon.consecutive_bad == 0


def test_sft_fires_on_patience_2_consecutive_bad():
    """val_loss > best × 1.05 for 2 consecutive evals → fire."""
    mon = EarlyStopMonitor(mode="sft", ratio=1.05, patience=2)
    assert mon.on_line("Iter 100: Val loss 0.500") is False  # baseline
    # 0.500 × 1.05 = 0.525 — need > 0.525 to be bad.
    assert mon.on_line("Iter 200: Val loss 0.600") is False  # bad #1
    assert mon.consecutive_bad == 1
    assert mon.on_line("Iter 300: Val loss 0.700") is True   # bad #2 → fire
    assert mon.fired
    assert "SFT early-stop fired" in mon.fired_reason
    assert "iter 300" in mon.fired_reason


def test_sft_no_fire_on_patience_1_only():
    """One bad eval alone is not enough."""
    mon = EarlyStopMonitor(mode="sft", ratio=1.05, patience=2)
    mon.on_line("Iter 100: Val loss 0.500")
    assert mon.on_line("Iter 200: Val loss 0.800") is False
    assert mon.consecutive_bad == 1 and not mon.fired


def test_sft_improvement_resets_consecutive():
    mon = EarlyStopMonitor(mode="sft", ratio=1.05, patience=2)
    mon.on_line("Iter 100: Val loss 0.500")
    mon.on_line("Iter 200: Val loss 0.600")   # bad #1
    assert mon.consecutive_bad == 1
    mon.on_line("Iter 300: Val loss 0.450")   # improvement — new best, reset
    assert mon.consecutive_bad == 0
    assert mon.best == 0.450


def test_non_matching_lines_ignored():
    mon = EarlyStopMonitor(mode="sft", ratio=1.05, patience=2)
    for line in [
        "Starting training…",
        "Iter 10: Train loss 1.234",
        "some random stderr line",
        "",
    ]:
        assert mon.on_line(line) is False
    assert mon.best is None


def test_sft_fire_is_idempotent():
    mon = EarlyStopMonitor(mode="sft", ratio=1.05, patience=2)
    mon.on_line("Iter 100: Val loss 0.500")
    mon.on_line("Iter 200: Val loss 0.600")
    mon.on_line("Iter 300: Val loss 0.700")
    assert mon.fired
    # Subsequent lines still return True, but don't mutate state.
    snapshot = mon.sidecar_record()
    mon.on_line("Iter 400: Val loss 0.800")
    assert mon.sidecar_record() == snapshot


# ────────────── RL path (wired but unvalidated) ──────────────


def test_rl_fires_on_reward_degradation():
    """val_reward < best × 0.95 for 2 consecutive evals → fire."""
    mon = EarlyStopMonitor(mode="rl", ratio=0.95, patience=2)
    mon.on_line("Iter 100: Val reward 1.000")  # baseline
    # 1.000 × 0.95 = 0.95 — need < 0.95 to be bad.
    mon.on_line("Iter 200: Val reward 0.800")  # bad #1
    assert mon.consecutive_bad == 1
    assert mon.on_line("Iter 300: Val reward 0.700") is True  # bad #2 → fire
    assert mon.fired
    assert "RL early-stop fired" in mon.fired_reason


def test_rl_no_fire_on_non_matching_sft_line():
    """RL monitor must NOT match SFT val_loss lines."""
    mon = EarlyStopMonitor(mode="rl", ratio=0.95, patience=2)
    assert mon.on_line("Iter 100: Val loss 0.500") is False
    assert mon.best is None


# ────────────── validation ──────────────


def test_invalid_mode_rejected():
    with pytest.raises(ValueError, match="mode must be"):
        EarlyStopMonitor(mode="foo")  # type: ignore[arg-type]


def test_sft_ratio_must_exceed_one():
    with pytest.raises(ValueError, match="SFT ratio must be > 1.0"):
        EarlyStopMonitor(mode="sft", ratio=0.95)


def test_rl_ratio_must_be_fraction():
    with pytest.raises(ValueError, match="RL ratio must be in"):
        EarlyStopMonitor(mode="rl", ratio=1.05)


def test_patience_at_least_one():
    with pytest.raises(ValueError, match="patience must be"):
        EarlyStopMonitor(mode="sft", patience=0)


def test_sidecar_record_shape():
    mon = EarlyStopMonitor(mode="sft", ratio=1.05, patience=2)
    mon.on_line("Iter 100: Val loss 0.500")
    mon.on_line("Iter 200: Val loss 0.600")
    rec = mon.sidecar_record()
    assert rec["mode"] == "sft"
    assert rec["ratio"] == 1.05
    assert rec["patience"] == 2
    assert rec["best_value"] == 0.500
    assert rec["best_iter"] == 100
    assert rec["fired"] is False
    assert rec["final_consecutive_bad"] == 1
    assert len(rec["history"]) == 2
