"""Unit tests for sampling_policy.resolve_sampling."""
from __future__ import annotations

import json
from pathlib import Path

import pytest

from neural.benchmarks.sampling_policy import (
    ALLOWED_GROUPS,
    SamplingPolicyError,
    group_weight,
    resolve_sampling,
)


# Minimal config fixture mirroring configs/benchmark_phase10.yaml
CONFIG = {
    "sampling_groups": {
        "T": {"weight": 0.50, "temperature": 0.6, "top_p": 0.95, "top_k": 20},
        "C": {"weight": 0.35, "temperature": 0.7, "top_p": 0.8, "top_k": 20},
        "J": {
            "weight": 0.15,
            "temperature": 0.0,
            "top_p": 1.0,
            "top_k": -1,
            "presence_penalty": 1.5,
        },
    }
}


def test_T_group_recipe() -> None:
    spec = {"sampling_group": "T"}
    kw = resolve_sampling(spec, CONFIG)
    assert kw == {"temperature": 0.6, "top_p": 0.95, "top_k": 20}


def test_C_group_recipe() -> None:
    spec = {"sampling_group": "C"}
    kw = resolve_sampling(spec, CONFIG)
    assert kw == {"temperature": 0.7, "top_p": 0.8, "top_k": 20}


def test_J_group_recipe_includes_presence_penalty() -> None:
    spec = {"sampling_group": "J"}
    kw = resolve_sampling(spec, CONFIG)
    assert kw["presence_penalty"] == 1.5
    assert kw["temperature"] == 0.0
    # top_k=-1 in recipe means "unrestricted" — MLX does not accept -1,
    # so the resolver drops the key entirely. MLX then applies its default.
    assert "top_k" not in kw


def test_top_k_negative_is_dropped() -> None:
    """Recipe top_k=-1 (unrestricted) must be omitted from MLX kwargs."""
    spec = {"sampling_group": "J"}
    kw = resolve_sampling(spec, CONFIG)
    assert "top_k" not in kw


def test_top_k_zero_is_kept() -> None:
    """Recipe top_k=0 is a legal explicit value and must pass through."""
    config = {
        "sampling_groups": {
            "T": {"weight": 1.0, "temperature": 0.5, "top_p": 1.0, "top_k": 0},
        }
    }
    kw = resolve_sampling({"sampling_group": "T"}, config)
    assert kw["top_k"] == 0


def test_J_group_missing_presence_penalty_raises() -> None:
    config_bad = {
        "sampling_groups": {
            "J": {"weight": 0.15, "temperature": 0.0, "top_p": 1.0, "top_k": -1}
        }
    }
    with pytest.raises(SamplingPolicyError, match="presence_penalty"):
        resolve_sampling({"sampling_group": "J"}, config_bad)


def test_invalid_group_raises() -> None:
    with pytest.raises(SamplingPolicyError):
        resolve_sampling({"sampling_group": "X"}, CONFIG)


def test_missing_group_raises() -> None:
    with pytest.raises(SamplingPolicyError):
        resolve_sampling({}, CONFIG)


def test_missing_config_raises() -> None:
    with pytest.raises(SamplingPolicyError):
        resolve_sampling({"sampling_group": "T"}, {"sampling_groups": {}})


def test_group_weight() -> None:
    assert group_weight("T", CONFIG) == 0.50
    assert group_weight("C", CONFIG) == 0.35
    assert group_weight("J", CONFIG) == 0.15


def test_all_17_real_specs_resolve() -> None:
    """Preflight-style: every real ULTS spec must resolve without error."""
    specs_dir = Path("docs/tests/ults/specs")
    specs = list(specs_dir.glob("*.ults.json"))
    assert len(specs) == 17, f"expected 17 specs, found {len(specs)}"
    seen_groups: set[str] = set()
    for path in specs:
        spec = json.loads(path.read_text())
        kw = resolve_sampling(spec, CONFIG)
        assert "temperature" in kw
        assert "top_p" in kw
        # top_k is present for T/C (explicit value) and absent for J (sentinel -1)
        if spec["sampling_group"] in ("T", "C"):
            assert "top_k" in kw
        else:
            assert "top_k" not in kw
        seen_groups.add(spec["sampling_group"])
    assert seen_groups == set(ALLOWED_GROUPS)
