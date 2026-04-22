"""Unit tests for expert_selection.py (Sprint FT-LORA-E Epic 2)."""

from __future__ import annotations

import json
import re
from pathlib import Path

import pytest

from training.expert_selection import (
    DEFAULT_PROJECTIONS,
    ExpertSelection,
    MODULE_PATH_TEMPLATE,
    normalize_family,
)


REPO_ROOT = Path(__file__).resolve().parents[3]
PROFILES_DIR = REPO_ROOT / "training_data" / "routing_profiles"


def _artifact(family_underscore: str) -> Path:
    return PROFILES_DIR / f"profile_routing_{family_underscore}.json"


# ────────────── normalize_family ──────────────


def test_normalize_family_hyphen_to_underscore():
    assert normalize_family("reasoning-think") == "reasoning_think"
    assert normalize_family("classify-notink") == "classify_notink"
    assert normalize_family("structured-notink") == "structured_notink"


def test_normalize_family_underscore_passthrough():
    assert normalize_family("reasoning_think") == "reasoning_think"


# ────────────── load() ──────────────


@pytest.mark.parametrize("family", ["reasoning_think", "classify_notink", "structured_notink"])
def test_load_valid_artifact(family: str):
    """Sprint D artifacts load and expose 40 × 64 expert tensor."""
    sel = ExpertSelection.load(_artifact(family), expected_family=family)
    assert sel.family == family
    assert sel.num_layers == 40
    assert sel.num_experts_per_layer == 256
    assert sel.num_experts_selected_per_layer == 64
    assert len(sel.top_experts_per_layer) == 40
    for layer_list in sel.top_experts_per_layer:
        assert len(layer_list) == 64
        assert all(isinstance(e, int) for e in layer_list)


def test_load_accepts_hyphen_family_name():
    """CLI flag form (hyphen) should normalize and match."""
    sel = ExpertSelection.load(_artifact("reasoning_think"), expected_family="reasoning-think")
    assert sel.family == "reasoning_think"


def test_load_family_mismatch_raises(tmp_path: Path):
    """Loading with the wrong family name fails loudly."""
    with pytest.raises(ValueError, match="Family mismatch"):
        ExpertSelection.load(_artifact("reasoning_think"), expected_family="classify_notink")


def test_load_missing_file_raises(tmp_path: Path):
    with pytest.raises(FileNotFoundError):
        ExpertSelection.load(tmp_path / "does-not-exist.json", expected_family="reasoning_think")


def test_load_bad_schema_missing_field(tmp_path: Path):
    """Missing required fields → ValueError."""
    p = tmp_path / "bad.json"
    p.write_text(json.dumps({
        "family": "reasoning_think",
        "num_layers": 40,
        # missing per_layer etc.
    }))
    with pytest.raises(ValueError, match="missing required fields"):
        ExpertSelection.load(p, expected_family="reasoning_think")


def test_load_layer_count_mismatch(tmp_path: Path):
    p = tmp_path / "bad.json"
    p.write_text(json.dumps({
        "family": "reasoning_think",
        "model_sha256_config": "x",
        "num_layers": 40,
        "num_experts_per_layer": 256,
        "num_experts_selected_per_layer": 64,
        "per_layer": [{"layer": 0, "top_experts": list(range(64))}] * 39,  # 39 not 40
    }))
    with pytest.raises(ValueError, match="Layer count mismatch"):
        ExpertSelection.load(p, expected_family="reasoning_think")


def test_load_top_experts_wrong_length(tmp_path: Path):
    p = tmp_path / "bad.json"
    p.write_text(json.dumps({
        "family": "reasoning_think",
        "model_sha256_config": "x",
        "num_layers": 1,
        "num_experts_per_layer": 256,
        "num_experts_selected_per_layer": 64,
        "per_layer": [{"layer": 0, "top_experts": [1, 2, 3]}],  # only 3
    }))
    with pytest.raises(ValueError, match="top_experts has 3 entries, expected 64"):
        ExpertSelection.load(p, expected_family="reasoning_think")


# ────────────── mlx_lm_keys() ──────────────


@pytest.mark.parametrize("family", ["reasoning_think", "classify_notink", "structured_notink"])
def test_mlx_lm_keys_count(family: str):
    """40 layers × 64 experts × 3 projections = 7,680 keys."""
    sel = ExpertSelection.load(_artifact(family), expected_family=family)
    keys = sel.mlx_lm_keys()
    assert len(keys) == 7680
    assert sel.count() == 7680


def test_mlx_lm_keys_unique():
    sel = ExpertSelection.load(_artifact("reasoning_think"), expected_family="reasoning_think")
    keys = sel.mlx_lm_keys()
    assert len(set(keys)) == len(keys)  # all unique


def test_mlx_lm_keys_format():
    """Key format matches the Qwen3 MoE expert module path."""
    sel = ExpertSelection.load(_artifact("reasoning_think"), expected_family="reasoning_think")
    keys = sel.mlx_lm_keys()
    pat = re.compile(
        r"^language_model\.model\.layers\.\d+\.mlp\.experts\.\d+\.(down|up|gate)_proj$"
    )
    for k in keys:
        assert pat.match(k), f"key {k!r} does not match expected format"


def test_mlx_lm_keys_respects_projections_override():
    sel = ExpertSelection.load(_artifact("reasoning_think"), expected_family="reasoning_think")
    keys = sel.mlx_lm_keys(projections=("down",))
    assert len(keys) == 40 * 64
    assert all(k.endswith(".down_proj") for k in keys)


def test_module_path_template_is_reversible():
    """Formatted template roundtrips via regex so integration test downstream works."""
    s = MODULE_PATH_TEMPLATE.format(layer=7, expert=123, proj="down")
    assert s == "language_model.model.layers.7.mlp.experts.123.down_proj"
    assert DEFAULT_PROJECTIONS == ("down", "up", "gate")
