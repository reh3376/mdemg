"""Asymmetric Quantization Predicate for MoE-Sieve Tier-2 Deployment
(Sprint FT-LORA-E Epic 3).

Builds an `mlx_lm.convert` predicate that keeps attention + shared-expert +
router weights in BF16 while pushing only the routed experts to MXFP4_MOE.

This matches memo 07 v3.1 §5 asymmetric-quant decision: the routed experts
carry the bulk of model parameters (~28 GB of the 35 B total) and tolerate
aggressive quantization; attention + shared expert + router carry the
reasoning pathway and stay at BF16 for quality.

The predicate signature — `(path: str, module: nn.Module) -> Union[bool, dict]`
— matches `mlx_lm.utils.quantize_model` (utils.py:778) and
`mlx_lm.convert.convert` (convert.py:96-97).

Return values:
    False           — skip quantization (keep BF16 / model native dtype).
    dict(…)         — quantize with these params.
    True            — quantize with default params (unused here).

CLI entry point allows a dry-run classification summary without writing
any converted model — Phase 5 runbook owns the real conversion.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from collections import Counter
from pathlib import Path
from typing import Any, Callable, Iterator


# mxfp4 params as exposed by mlx_lm.utils.quantize_model defaults_for_mode
# (utils.py:800-804): group_size=32, bits=4 when mode="mxfp4".
MXFP4_SPEC: dict[str, Any] = {"group_size": 32, "bits": 4, "mode": "mxfp4"}

# Classification categories (for dry-run summary and unit tests).
CAT_ATTENTION = "attention"
CAT_SHARED_EXPERT = "shared_expert"
CAT_ROUTER = "router"
CAT_ROUTED_EXPERTS = "routed_experts"
CAT_OTHER = "other"
ALL_CATEGORIES = (
    CAT_ATTENTION,
    CAT_SHARED_EXPERT,
    CAT_ROUTER,
    CAT_ROUTED_EXPERTS,
    CAT_OTHER,
)


def classify_path(path: str) -> str:
    """Classify a module path into one of the 5 Sprint E asymmetric categories.

    Order of checks matters — `mlp.experts.{E}.down_proj` must be classified
    as ROUTED_EXPERTS before any MLP-ish pattern would claim it.
    """
    # Routed experts must come first — they live under mlp.experts.{idx}.
    if "mlp.experts." in path:
        return CAT_ROUTED_EXPERTS
    # Shared expert: mlp.shared_expert.{gate,up,down}_proj.
    if "mlp.shared_expert." in path:
        return CAT_SHARED_EXPERT
    # Router: mlp.gate (single linear; NOT mlp.gate_proj which would be
    # part of shared/routed above). Match only the exact "mlp.gate" suffix or
    # "mlp.gate." (compound children).
    if path.endswith(".mlp.gate") or ".mlp.gate." in path:
        return CAT_ROUTER
    # Attention: self_attn.*.
    if ".self_attn." in path:
        return CAT_ATTENTION
    return CAT_OTHER


def build_asymmetric_predicate(
    shared_bits: str = "bf16",
    routed_spec: dict[str, Any] | None = None,
    attn_bits: str = "bf16",
    router_bits: str = "bf16",
) -> Callable[[str, Any], bool | dict[str, Any]]:
    """Return a quant_predicate compatible with mlx_lm.convert.

    The three `*_bits="bf16"` params signal "keep native, do not quantize".
    Currently only `"bf16"` is supported for the BF16 categories — the
    predicate returns `False` to skip quantization (model stays in its
    loaded dtype, typically BF16). Any other value raises; we don't silently
    reinterpret an unexpected spec.

    Args:
        shared_bits:   quant spec for mlp.shared_expert.*   (default bf16 = skip).
        routed_spec:   dict for mlp.experts.*               (default mxfp4).
        attn_bits:     quant spec for self_attn.*           (default bf16).
        router_bits:   quant spec for mlp.gate              (default bf16).

    Returns:
        Callable accepted by mlx_lm.utils.quantize_model's quant_predicate kwarg.
    """
    routed_spec = dict(routed_spec) if routed_spec is not None else dict(MXFP4_SPEC)

    def _as_decision(spec: str | dict[str, Any]) -> bool | dict[str, Any]:
        if spec == "bf16":
            return False
        if isinstance(spec, dict):
            return dict(spec)
        raise ValueError(
            f"Unsupported quant spec: {spec!r}. Expected 'bf16' or a "
            f"dict like {{'group_size': 32, 'bits': 4, 'mode': 'mxfp4'}}."
        )

    attn_decision = _as_decision(attn_bits)
    shared_decision = _as_decision(shared_bits)
    router_decision = _as_decision(router_bits)
    routed_decision = _as_decision(routed_spec)

    def predicate(path: str, module: Any) -> bool | dict[str, Any]:
        cat = classify_path(path)
        if cat == CAT_ROUTED_EXPERTS:
            return routed_decision if not isinstance(routed_decision, bool) else routed_decision
        if cat == CAT_SHARED_EXPERT:
            return shared_decision
        if cat == CAT_ROUTER:
            return router_decision
        if cat == CAT_ATTENTION:
            return attn_decision
        # CAT_OTHER — keep BF16. Embeddings, norms, lm_head, etc.
        return False

    return predicate


# ───────────────────────── CLI ─────────────────────────


def _classify_model_paths(model_path: str) -> Counter:
    """Iterate a loaded MLX model's named parameters and classify each path.

    Loads the model lazily via mlx_lm.load; used by the dry-run.
    """
    try:
        from mlx_lm import load as mlx_load  # deferred import
    except ImportError:
        print("ERROR: mlx-lm not installed in active venv", file=sys.stderr)
        raise SystemExit(1)

    print(f"[INFO] Loading model from {model_path} for dry-run classification…")
    model, _tokenizer = mlx_load(model_path)

    counts: Counter[str] = Counter()
    for path, _param in _iter_named_leaves(model):
        counts[classify_path(path)] += 1
    return counts


def _iter_named_leaves(model: Any) -> Iterator[tuple[str, Any]]:
    """Yield (path, module) for leaf modules with quantizable weights.

    Uses model.named_modules() and filters to Linear-like leaves
    (those with a `weight` attribute and no submodules).
    """
    # mlx nn.Module doesn't offer a stable `named_modules`; it DOES offer
    # `named_parameters` which yields (path, param) for tensor leaves. That's
    # what quantize_model iterates, so we match it.
    for path, _param in model.named_parameters():
        # Strip the terminal parameter name (".weight", ".bias") so the path
        # matches the module-level path that mlx_lm.quantize_model's
        # wrapped_predicate receives.
        if path.endswith(".weight") or path.endswith(".bias"):
            yield path.rsplit(".", 1)[0], _param


def _cmd_dry_run(args: argparse.Namespace) -> int:
    """Classify every module path in the model and print summary counts."""
    counts = _classify_model_paths(args.input_model)
    total = sum(counts.values())
    print()
    print("═══ Asymmetric-Quant Classification (dry-run) ═══")
    print(f"  input model:  {args.input_model}")
    print(f"  total paths:  {total}")
    for cat in ALL_CATEGORIES:
        n = counts.get(cat, 0)
        decision = _describe_decision_for_category(cat, args)
        print(f"  {cat:20s} {n:6d}   → {decision}")
    if args.output_json:
        Path(args.output_json).write_text(
            json.dumps({"total": total, "counts": dict(counts)}, indent=2)
        )
        print(f"\nSummary written to {args.output_json}")
    return 0


def _describe_decision_for_category(cat: str, args: argparse.Namespace) -> str:
    if cat == CAT_ROUTED_EXPERTS:
        return f"quantize {args.routed_spec}"
    if cat == CAT_SHARED_EXPERT:
        return f"{args.shared_bits} (skip)"
    if cat == CAT_ATTENTION:
        return f"{args.attn_bits} (skip)"
    if cat == CAT_ROUTER:
        return f"{args.router_bits} (skip)"
    return "bf16 (skip — uncategorized)"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Asymmetric quant helper for MoE-Sieve Tier 2 deployment.",
    )
    parser.add_argument("--input-model", required=True, help="Path to loaded MLX model.")
    parser.add_argument(
        "--output-model", default=None,
        help="Path to write quantized model (ignored with --dry-run).",
    )
    parser.add_argument(
        "--shared-bits",
        default=os.environ.get("ASYMMETRIC_QUANT_SHARED", "bf16"),
        help="Quant spec for mlp.shared_expert.* (default from "
             "ASYMMETRIC_QUANT_SHARED or 'bf16').",
    )
    parser.add_argument(
        "--routed-spec",
        default=os.environ.get("ASYMMETRIC_QUANT_ROUTED", "mxfp4_moe"),
        help="Quant spec for mlp.experts.* (default from "
             "ASYMMETRIC_QUANT_ROUTED or 'mxfp4_moe'). Use 'mxfp4_moe' for "
             "{group_size:32,bits:4,mode:mxfp4}.",
    )
    parser.add_argument(
        "--attn-bits",
        default=os.environ.get("ASYMMETRIC_QUANT_ATTN", "bf16"),
        help="Quant spec for self_attn.* (default from "
             "ASYMMETRIC_QUANT_ATTN or 'bf16').",
    )
    parser.add_argument(
        "--router-bits", default="bf16",
        help="Quant spec for mlp.gate (router). (default 'bf16').",
    )
    parser.add_argument(
        "--dry-run", action="store_true",
        help="Classify module paths and print summary; do not write converted model.",
    )
    parser.add_argument(
        "--output-json", default=None,
        help="(dry-run) Write classification counts to this JSON file.",
    )
    args = parser.parse_args(argv)

    # Normalize mxfp4_moe alias → dict
    if args.routed_spec == "mxfp4_moe" or args.routed_spec == "mxfp4":
        args.routed_spec = MXFP4_SPEC
    elif isinstance(args.routed_spec, str):
        # Accept JSON-encoded dict too for flexibility
        try:
            args.routed_spec = json.loads(args.routed_spec)
        except json.JSONDecodeError:
            print(f"ERROR: cannot parse --routed-spec={args.routed_spec!r}",
                  file=sys.stderr)
            return 2

    if args.dry_run:
        return _cmd_dry_run(args)

    if not args.output_model:
        print("ERROR: --output-model required unless --dry-run.", file=sys.stderr)
        return 2

    # Full conversion is deferred to Phase 5 runbook per Sprint E scope.
    print(
        "Non-dry-run conversion is out of scope for Sprint E. "
        "Use --dry-run to classify paths, then Phase 5 owns real conversion.",
        file=sys.stderr,
    )
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
