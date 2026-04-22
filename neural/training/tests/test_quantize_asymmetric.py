"""Unit tests for quantize_asymmetric.py (Sprint FT-LORA-E Epic 3)."""

from __future__ import annotations

import pytest

from training.quantize_asymmetric import (
    CAT_ATTENTION,
    CAT_OTHER,
    CAT_ROUTED_EXPERTS,
    CAT_ROUTER,
    CAT_SHARED_EXPERT,
    MXFP4_SPEC,
    build_asymmetric_predicate,
    classify_path,
)


# ────────────── classify_path — 8-path matrix ──────────────


@pytest.mark.parametrize("path,expected", [
    # Attention (×2 — q/k_proj and v/o_proj)
    ("language_model.model.layers.0.self_attn.q_proj", CAT_ATTENTION),
    ("language_model.model.layers.39.self_attn.o_proj", CAT_ATTENTION),
    # Shared expert (×2 — gate/up/down)
    ("language_model.model.layers.5.mlp.shared_expert.gate_proj", CAT_SHARED_EXPERT),
    ("language_model.model.layers.5.mlp.shared_expert.down_proj", CAT_SHARED_EXPERT),
    # Router (×1)
    ("language_model.model.layers.3.mlp.gate", CAT_ROUTER),
    # Routed experts (×2 — different experts and projs)
    ("language_model.model.layers.10.mlp.experts.153.down_proj", CAT_ROUTED_EXPERTS),
    ("language_model.model.layers.0.mlp.experts.0.up_proj", CAT_ROUTED_EXPERTS),
    # Other (×1 — embeddings, norms, lm_head)
    ("language_model.model.embed_tokens", CAT_OTHER),
])
def test_classify_path_8_matrix(path: str, expected: str):
    assert classify_path(path) == expected


def test_classify_lm_head_is_other():
    assert classify_path("lm_head") == CAT_OTHER


def test_classify_layernorm_is_other():
    assert classify_path("language_model.model.layers.5.post_attention_layernorm") == CAT_OTHER


def test_classify_routed_expert_wins_over_other_patterns():
    """experts.N must be classified ROUTED_EXPERTS even if name ends in .gate_proj."""
    # Although "mlp.gate_proj" is NOT a route path per se, confirm the routed
    # expert path is matched first even when an expert projection is named "gate_proj".
    p = "language_model.model.layers.0.mlp.experts.1.gate_proj"
    assert classify_path(p) == CAT_ROUTED_EXPERTS


# ────────────── build_asymmetric_predicate ──────────────


def test_predicate_defaults_preserve_quality_path():
    pred = build_asymmetric_predicate()
    # Routed expert → mxfp4 dict
    decision = pred("language_model.model.layers.0.mlp.experts.1.down_proj", None)
    assert decision == MXFP4_SPEC
    # Attention, shared, router → False (skip quant)
    for p in [
        "language_model.model.layers.0.self_attn.q_proj",
        "language_model.model.layers.0.mlp.shared_expert.up_proj",
        "language_model.model.layers.0.mlp.gate",
        "lm_head",
    ]:
        assert pred(p, None) is False


def test_predicate_rejects_unknown_spec():
    with pytest.raises(ValueError, match="Unsupported quant spec"):
        build_asymmetric_predicate(shared_bits="q4_0")


def test_predicate_routed_spec_can_be_overridden():
    custom = {"group_size": 64, "bits": 2, "mode": "mxfp4"}
    pred = build_asymmetric_predicate(routed_spec=custom)
    decision = pred("language_model.model.layers.0.mlp.experts.1.down_proj", None)
    assert decision == custom


def test_mxfp4_spec_shape():
    """Pinned against mlx_lm.utils.quantize_model defaults_for_mode."""
    assert MXFP4_SPEC == {"group_size": 32, "bits": 4, "mode": "mxfp4"}
