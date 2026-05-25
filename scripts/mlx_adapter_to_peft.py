#!/usr/bin/env python3
"""Sprint MODEL-DIST-002 Epic 1 — MLX LoRA adapter → PEFT format.

Converts the MLX-format LoRA adapter at `adapters/tier1/adapters.safetensors`
into a PEFT-format directory (`adapter_config.json` + `adapter_model.safetensors`)
consumable by llama.cpp's `convert_lora_to_gguf.py`.

Key transformation (per MODEL-DIST-001 epic_2_forensic.md analysis):

  MLX                                         PEFT
  ─────────────────────────────────────────  ──────────────────────────────────────────────
  model.layers.<N>.<module>.lora_a            base_model.model.model.layers.<N>.<module>.lora_A.default.weight
  shape (input_features, rank)                shape (rank, input_features)                — transpose

  model.layers.<N>.<module>.lora_b            base_model.model.model.layers.<N>.<module>.lora_B.default.weight
  shape (rank, output_features)               shape (output_features, rank)               — transpose

Usage:
    python3 scripts/mlx_adapter_to_peft.py \\
        --mlx-dir adapters/tier1 \\
        --output .local-models/mdemg-llm-v1-adapter-peft \\
        --base-model mlx-community/Qwen3-14B-4bit

Reads `<mlx-dir>/adapter_config.json` + `<mlx-dir>/adapters.safetensors`.
Writes `<output>/adapter_config.json` + `<output>/adapter_model.safetensors`.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path


# Regex for MLX key parsing.
# MLX format: "model.layers.<N>.<module_path>.lora_a" or ".lora_b"
# We need to preserve the layer index and module path verbatim.
MLX_KEY_RE = re.compile(r"^model\.layers\.(\d+)\.(.+)\.(lora_a|lora_b)$")


def mlx_to_peft_key(mlx_key: str) -> str:
    """Translate one MLX key to its PEFT equivalent.

    Raises ValueError if the key doesn't match the expected MLX shape.
    """
    m = MLX_KEY_RE.match(mlx_key)
    if not m:
        raise ValueError(f"unexpected MLX key shape: {mlx_key!r}")
    layer_idx, module_path, lora_dir = m.group(1), m.group(2), m.group(3)
    # MLX uses lowercase "a"/"b"; PEFT uses uppercase "A"/"B" with "default" adapter name.
    peft_dir = "lora_A" if lora_dir == "lora_a" else "lora_B"
    # NOTE: convert_lora_to_gguf.py from llama.cpp b9000 expects the
    # single-adapter PEFT layout (`.lora_A.weight`), not the multi-adapter
    # layout (`.lora_A.default.weight`). Both are valid PEFT forms; we emit
    # the simpler form for compatibility with the converter.
    return f"base_model.model.model.layers.{layer_idx}.{module_path}.{peft_dir}.weight"


def transpose_lora_tensor(tensor, lora_dir: str):
    """Transpose MLX lora tensor to PEFT convention.

    MLX:
        lora_a: shape (input_features, rank)
        lora_b: shape (rank, output_features)
    PEFT:
        lora_A: shape (rank, input_features)   — transpose of MLX lora_a
        lora_B: shape (output_features, rank)  — transpose of MLX lora_b

    Both directions are 2D transposes; we just need to ensure we're swapping
    the right axes. torch.Tensor.T (or .transpose(0, 1)) does the right thing
    for both since the source tensors are 2D.
    """
    if tensor.ndim != 2:
        raise ValueError(f"lora_{lora_dir.lower()} expected 2D tensor; got shape {tensor.shape}")
    return tensor.T.contiguous()


def build_peft_adapter_config(mlx_cfg: dict, base_model: str) -> dict:
    """Translate MLX-format adapter_config.json into PEFT schema.

    MLX config contains training metadata (iters, batch_size, learning_rate, ...)
    that PEFT doesn't care about. PEFT cares about: peft_type, task_type,
    base_model_name_or_path, r, lora_alpha, target_modules, fan_in_fan_out,
    init_lora_weights, modules_to_save, bias, lora_dropout.
    """
    lora_params = mlx_cfg.get("lora_parameters", {})
    rank = lora_params.get("rank", 32)
    alpha = lora_params.get("alpha", 64)
    target_modules_raw = lora_params.get("keys", [])

    # MLX's "keys" list uses bare module names like "self_attn.q_proj"; PEFT's
    # target_modules can be either bare names or fully qualified. Keep the
    # leaf module name only — that matches how PEFT matches modules during
    # adapter application.
    target_modules = []
    for k in target_modules_raw:
        # "self_attn.q_proj" -> "q_proj"; "mlp.down_proj" -> "down_proj"
        target_modules.append(k.rsplit(".", 1)[-1] if "." in k else k)
    target_modules = sorted(set(target_modules))

    return {
        "auto_mapping": None,
        "base_model_name_or_path": base_model,
        "bias": "none",
        "fan_in_fan_out": False,
        "inference_mode": True,
        "init_lora_weights": True,
        "layers_pattern": None,
        "layers_to_transform": None,
        "loftq_config": {},
        "lora_alpha": alpha,
        "lora_dropout": lora_params.get("dropout", 0.0),
        "megatron_config": None,
        "megatron_core": "megatron.core",
        "modules_to_save": None,
        "peft_type": "LORA",
        "r": rank,
        "rank_pattern": {},
        "revision": None,
        "target_modules": target_modules,
        "task_type": "CAUSAL_LM",
        "use_dora": False,
        "use_rslora": False,
    }


def convert(mlx_dir: Path, output_dir: Path, base_model: str) -> dict:
    """Run the full MLX → PEFT conversion. Returns a summary dict."""
    # Lazy imports — only required when actually running (tests don't need them).
    from safetensors import safe_open
    from safetensors.torch import save_file

    mlx_cfg_path = mlx_dir / "adapter_config.json"
    mlx_weights_path = mlx_dir / "adapters.safetensors"

    if not mlx_cfg_path.exists():
        raise FileNotFoundError(f"MLX adapter config not found: {mlx_cfg_path}")
    if not mlx_weights_path.exists():
        raise FileNotFoundError(f"MLX adapter weights not found: {mlx_weights_path}")

    mlx_cfg = json.loads(mlx_cfg_path.read_text())
    output_dir.mkdir(parents=True, exist_ok=True)

    # Write PEFT adapter_config.json.
    peft_cfg = build_peft_adapter_config(mlx_cfg, base_model)
    (output_dir / "adapter_config.json").write_text(json.dumps(peft_cfg, indent=2))

    # Load + transform MLX tensors, save as PEFT.
    peft_tensors = {}
    tensor_count_in = 0
    tensor_count_out = 0
    with safe_open(str(mlx_weights_path), framework="pt") as f:
        for mlx_key in f.keys():
            tensor_count_in += 1
            peft_key = mlx_to_peft_key(mlx_key)
            tensor = f.get_tensor(mlx_key)
            lora_dir = "a" if mlx_key.endswith("lora_a") else "b"
            transposed = transpose_lora_tensor(tensor, lora_dir)
            peft_tensors[peft_key] = transposed
            tensor_count_out += 1

    save_file(peft_tensors, str(output_dir / "adapter_model.safetensors"))

    return {
        "input_tensor_count": tensor_count_in,
        "output_tensor_count": tensor_count_out,
        "rank": peft_cfg["r"],
        "alpha": peft_cfg["lora_alpha"],
        "target_modules": peft_cfg["target_modules"],
        "base_model": base_model,
        "output_dir": str(output_dir),
    }


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--mlx-dir", type=Path, required=True,
                   help="Directory containing MLX adapter_config.json + adapters.safetensors")
    p.add_argument("--output", type=Path, required=True,
                   help="Output directory for PEFT adapter_config.json + adapter_model.safetensors")
    p.add_argument("--base-model", type=str, default="mlx-community/Qwen3-14B-4bit",
                   help="base_model_name_or_path to write into PEFT adapter_config.json")
    args = p.parse_args()

    try:
        summary = convert(args.mlx_dir, args.output, args.base_model)
    except Exception as e:
        print(f"FATAL: {e}", file=sys.stderr)
        return 1
    print(json.dumps(summary, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
