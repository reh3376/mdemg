"""Sampling-group → MLX-kwargs resolver.

ULTS specs declare `sampling_group ∈ {T, C, J}`. The benchmark config
(`configs/benchmark_phase10.yaml :: sampling_groups`) maps each group to the
canonical recipe from Sprint C Gate 2. This resolver returns the MLX-compatible
kwargs dict for an `mlx_lm.server` /chat/completions call.

Key invariant: the J-group `presence_penalty=1.5` is **mandatory** and must not
be dropped by any downstream layer. The LLM judge has its own fixed sampling
kwargs and never calls into this module.
"""
from __future__ import annotations

from typing import Any

ALLOWED_GROUPS = ("T", "C", "J")


class SamplingPolicyError(ValueError):
    """Raised for any spec that can't be resolved."""


def resolve_sampling(
    ults_spec: dict[str, Any], config: dict[str, Any]
) -> dict[str, Any]:
    """Return MLX /chat/completions kwargs for the given ULTS spec.

    ``config`` is the top-level benchmark config (parsed from
    ``configs/benchmark_phase10.yaml``); we read ``config['sampling_groups']``.

    Returned dict always includes: ``temperature``, ``top_p``, ``top_k``;
    for J-group also ``presence_penalty``.
    """
    group = ults_spec.get("sampling_group")
    if group not in ALLOWED_GROUPS:
        raise SamplingPolicyError(
            f"sampling_group={group!r} must be one of {ALLOWED_GROUPS}"
        )

    groups_cfg = config.get("sampling_groups") or {}
    recipe = groups_cfg.get(group)
    if not recipe:
        raise SamplingPolicyError(
            f"config.sampling_groups.{group} missing — cannot resolve sampling"
        )

    kwargs: dict[str, Any] = {
        "temperature": float(recipe["temperature"]),
        "top_p": float(recipe["top_p"]),
    }
    # top_k=-1 means "unrestricted" per ULTS; MLX server does not accept -1.
    # Drop the key entirely when unrestricted so MLX applies its own default.
    top_k_val = int(recipe["top_k"])
    if top_k_val >= 0:
        kwargs["top_k"] = top_k_val
    # presence_penalty is mandatory for J-group per ULTS contract.
    if "presence_penalty" in recipe:
        kwargs["presence_penalty"] = float(recipe["presence_penalty"])
    elif group == "J":
        raise SamplingPolicyError(
            "J-group recipe missing presence_penalty — mandatory per ULTS contract"
        )
    return kwargs


def group_weight(group: str, config: dict[str, Any]) -> float:
    """Return the aggregate-score weight for ``group``."""
    if group not in ALLOWED_GROUPS:
        raise SamplingPolicyError(f"group {group!r} not in {ALLOWED_GROUPS}")
    recipe = (config.get("sampling_groups") or {}).get(group)
    if not recipe or "weight" not in recipe:
        raise SamplingPolicyError(
            f"config.sampling_groups.{group}.weight missing"
        )
    return float(recipe["weight"])
