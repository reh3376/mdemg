"""Fuse LoRA Adapter + Quantize for Production Deployment.

Merges a trained LoRA adapter into the base model weights, quantizes
to 4-bit for inference, and optionally verifies the fused model with
a single test prompt.

Usage:
    # Post-Phase-5 pivot (2026-04-22): dense Qwen3-14B-4bit replaces abandoned
    # Qwen3.6-35B-A3B MoE. Merged MDEMG model is 4-bit preserved (no re-quant
    # needed — base is already 4-bit, LoRA folds without re-quantization).
    # Use `python -m mlx_lm fuse` directly for the dense path; this module
    # remains for future symmetric re-quant needs.
    python -m training.quantize_deploy \
        --base-model mlx-community/Qwen3-14B-4bit \
        --adapter-path adapters/tier1/ \
        --output-path .local-models/qwen3-14b-mdemg-v1/ \
        --quantize 4bit

    # With verification:
    python -m training.quantize_deploy \
        --base-model mlx-community/Qwen3-14B-4bit \
        --adapter-path adapters/tier1/ \
        --output-path .local-models/qwen3-14b-mdemg-v1/ \
        --quantize 4bit \
        --verify

Requirements:
    pip install -e ".[lora]"
"""

import argparse
import json
import os
import subprocess
import sys
import time
from typing import Any


QUANTIZE_BITS = {
    "4bit": 4,
    "8bit": 8,
}

VERIFY_PROMPT = {
    "system": "You are a query classifier. Respond with a JSON object containing a 'types' array.",
    "user": "How does the authentication middleware work?",
}


def fuse_adapter(
    base_model: str,
    adapter_path: str,
    output_path: str,
) -> tuple[bool, str]:
    """Fuse LoRA adapter weights into base model.

    Returns (success, message).
    """
    cmd = [
        sys.executable, "-m", "mlx_lm.fuse",
        "--model", base_model,
        "--adapter-path", adapter_path,
        "--save-path", output_path,
    ]

    print("Fusing adapter into base model...")
    print(f"  Base model: {base_model}")
    print(f"  Adapter: {adapter_path}")
    print(f"  Output: {output_path}")

    result = subprocess.run(cmd, capture_output=True, text=True)

    if result.returncode != 0:
        return False, f"fuse failed (exit {result.returncode}): {result.stderr.strip()}"

    return True, "fuse complete"


def quantize_model(
    model_path: str,
    output_path: str,
    bits: int,
) -> tuple[bool, str]:
    """Quantize a model to the specified bit width.

    Returns (success, message).
    """
    cmd = [
        sys.executable, "-m", "mlx_lm.convert",
        "--hf-path", model_path,
        "--mlx-path", output_path,
        "-q",
        "--q-bits", str(bits),
    ]

    print(f"Quantizing to {bits}-bit...")
    result = subprocess.run(cmd, capture_output=True, text=True)

    if result.returncode != 0:
        return False, f"quantize failed (exit {result.returncode}): {result.stderr.strip()}"

    return True, f"quantized to {bits}-bit"


def verify_model(output_path: str) -> tuple[bool, str]:
    """Run a single inference to verify the fused model works.

    Returns (success, message).
    """
    cmd = [
        sys.executable, "-m", "mlx_lm.generate",
        "--model", output_path,
        "--prompt", f"<|im_start|>system\n{VERIFY_PROMPT['system']}<|im_end|>\n"
                    f"<|im_start|>user\n{VERIFY_PROMPT['user']}<|im_end|>\n"
                    f"<|im_start|>assistant\n",
        "--max-tokens", "100",
    ]

    print("Verifying fused model with test prompt...")
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=120)

    if result.returncode != 0:
        return False, f"verify failed (exit {result.returncode}): {result.stderr.strip()}"

    output = result.stdout.strip()
    if not output:
        return False, "verify failed: empty output"

    print(f"  Response preview: {output[:200]}")
    return True, "verification passed"


def run_quantize_deploy(
    base_model: str,
    adapter_path: str,
    output_path: str,
    quantize: str | None = None,
    verify: bool = False,
    dry_run: bool = False,
) -> dict[str, Any]:
    """Run the full fuse + quantize + verify pipeline.

    Returns a summary dict.
    """
    # Validate inputs
    if not os.path.isdir(adapter_path):
        print(f"ERROR: adapter path not found: {adapter_path}", file=sys.stderr)
        raise SystemExit(1)

    adapter_config = os.path.join(adapter_path, "adapter_config.json")
    if not os.path.exists(adapter_config) and not os.path.exists(
        os.path.join(adapter_path, "adapters.safetensors")
    ):
        print(f"WARNING: no adapter weights found in {adapter_path}", file=sys.stderr)

    summary: dict[str, Any] = {
        "base_model": base_model,
        "adapter_path": adapter_path,
        "output_path": output_path,
        "quantize": quantize,
        "steps": [],
    }

    if dry_run:
        summary["status"] = "dry_run"
        summary["steps"] = ["fuse", "quantize", "verify"] if verify else ["fuse", "quantize"]
        return summary

    start = time.time()
    os.makedirs(output_path, exist_ok=True)

    # Step 1: Fuse
    if quantize:
        # Fuse to temp path, then quantize to final output
        fuse_path = output_path + "-fused"
    else:
        fuse_path = output_path

    ok, msg = fuse_adapter(base_model, adapter_path, fuse_path)
    summary["steps"].append({"step": "fuse", "success": ok, "message": msg})
    if not ok:
        summary["status"] = "failed"
        summary["failed_step"] = "fuse"
        return summary

    # Step 2: Quantize (optional)
    if quantize:
        bits = QUANTIZE_BITS.get(quantize)
        if not bits:
            print(f"ERROR: unknown quantize level '{quantize}'. Use: {list(QUANTIZE_BITS.keys())}",
                  file=sys.stderr)
            raise SystemExit(1)

        ok, msg = quantize_model(fuse_path, output_path, bits)
        summary["steps"].append({"step": "quantize", "success": ok, "message": msg})
        if not ok:
            summary["status"] = "failed"
            summary["failed_step"] = "quantize"
            return summary

    # Step 3: Verify (optional)
    final_path = output_path
    if verify:
        ok, msg = verify_model(final_path)
        summary["steps"].append({"step": "verify", "success": ok, "message": msg})
        if not ok:
            summary["status"] = "failed"
            summary["failed_step"] = "verify"
            return summary

    elapsed = time.time() - start
    summary["elapsed_seconds"] = round(elapsed, 1)
    summary["status"] = "success"

    # Check output size
    total_size = 0
    for root, _, files in os.walk(output_path):
        for f in files:
            total_size += os.path.getsize(os.path.join(root, f))
    summary["output_size_gb"] = round(total_size / (1024 ** 3), 2)

    return summary


def main():
    parser = argparse.ArgumentParser(
        description="Fuse LoRA adapter + quantize for deployment",
    )
    parser.add_argument(
        "--base-model", required=True,
        help="Base model (HuggingFace ID or local path)",
    )
    parser.add_argument(
        "--adapter-path", required=True,
        help="Path to trained LoRA adapter directory",
    )
    parser.add_argument(
        "--output-path", required=True,
        help="Output path for fused/quantized model",
    )
    parser.add_argument(
        "--quantize", choices=list(QUANTIZE_BITS.keys()), default=None,
        help="Quantization level (default: no quantization)",
    )
    parser.add_argument(
        "--verify", action="store_true",
        help="Run a test inference after fusing",
    )
    parser.add_argument("--dry-run", action="store_true", help="Show steps without executing")
    parser.add_argument("--report", help="Write JSON summary to file")
    args = parser.parse_args()

    summary = run_quantize_deploy(
        base_model=args.base_model,
        adapter_path=args.adapter_path,
        output_path=args.output_path,
        quantize=args.quantize,
        verify=args.verify,
        dry_run=args.dry_run,
    )

    # Print summary
    print()
    print("═══ Deploy Summary ═══")
    for k, v in summary.items():
        if k != "steps":
            print(f"  {k}: {v}")
    for step in summary.get("steps", []):
        status = "OK" if step.get("success") else "FAIL"
        print(f"  [{status}] {step['step']}: {step.get('message', '')}")

    if args.report:
        with open(args.report, "w") as f:
            json.dump(summary, f, indent=2)
        print(f"\nReport written to {args.report}")

    if summary.get("status") == "failed":
        raise SystemExit(1)


if __name__ == "__main__":
    main()
