"""LoRA Fine-Tuning Script for MDEMG.

Thin wrapper around mlx-lm-lora that validates the dataset manifest
before training, enforces anti-collapse constraints, and logs metrics.

Usage:
    python -m training.train_ft \\
        --dataset curated/v1/ \\
        --base-model mlx-community/Qwen3-30B-A3B-4bit \\
        --adapter-path adapters/v1/ \\
        --epochs 3 --batch-size 4 --lora-rank 16

Requirements:
    pip install mlx-lm>=0.20.0  (or: pip install -e ".[lora]")
"""

import argparse
import json
import os
import sys
import time
from pathlib import Path
from typing import Any


# Anti-collapse: minimum exogenous data ratio
MIN_EXOGENOUS_RATIO = 0.4


def load_manifest(dataset_dir: str) -> dict[str, Any]:
    """Load and return the dataset manifest.json."""
    manifest_path = os.path.join(dataset_dir, "manifest.json")
    if not os.path.exists(manifest_path):
        raise FileNotFoundError(f"manifest.json not found in {dataset_dir}")
    with open(manifest_path) as f:
        return json.load(f)


def validate_manifest(manifest: dict[str, Any]) -> list[str]:
    """Validate manifest against training prerequisites.

    Returns list of error strings. Empty list = valid.
    """
    errors = []

    # Check splits exist
    splits = manifest.get("splits", {})
    train_split = splits.get("train", {})
    if not train_split:
        errors.append("manifest missing splits.train")
    elif train_split.get("rows", 0) == 0:
        errors.append("train split has 0 rows")

    # Check quality gates
    gates = manifest.get("quality_gates", {})

    exogenous_ratio = gates.get("exogenous_ratio", 0)
    exogenous_met = gates.get("exogenous_ratio_met", False)
    if not exogenous_met:
        errors.append(
            f"exogenous ratio {exogenous_ratio:.4f} below target "
            f"(manifest gate failed)"
        )
    if exogenous_ratio < MIN_EXOGENOUS_RATIO:
        errors.append(
            f"exogenous ratio {exogenous_ratio:.4f} below anti-collapse "
            f"minimum {MIN_EXOGENOUS_RATIO}"
        )

    if not gates.get("no_train_test_overlap", True):
        errors.append("train/test split has overlap — data contamination risk")

    return errors


def resolve_lora_rank(
    dataset_dir: str,
    cli_rank: int | None,
    ults_dir: str | None,
) -> int:
    """Resolve LoRA rank: CLI override > ULTS spec max > default 16."""
    if cli_rank is not None:
        return cli_rank

    if ults_dir and os.path.isdir(ults_dir):
        max_rank = 0
        for spec_file in Path(ults_dir).glob("*.ults.json"):
            try:
                with open(spec_file) as f:
                    spec = json.load(f)
                rank = spec.get("training_config", {}).get("rank", 0)
                max_rank = max(max_rank, rank)
            except (json.JSONDecodeError, KeyError):
                continue
        if max_rank > 0:
            return max_rank

    return 16


def build_train_config(
    dataset_dir: str,
    base_model: str,
    adapter_path: str,
    epochs: int,
    batch_size: int,
    learning_rate: float,
    lora_rank: int,
    lora_layers: int,
    max_seq_length: int,
    train_rows: int = 0,
) -> dict[str, Any]:
    """Build the training configuration dict for mlx-lm."""
    train_path = os.path.join(dataset_dir, "train.jsonl")
    test_path = os.path.join(dataset_dir, "test.jsonl")

    row_count = train_rows if train_rows > 0 else _count_lines(train_path)
    config = {
        "model": base_model,
        "data": train_path,
        "adapter_path": adapter_path,
        "train": True,
        "iters": epochs * row_count // batch_size,
        "batch_size": batch_size,
        "learning_rate": learning_rate,
        "lora_layers": lora_layers,
        "lora_rank": lora_rank,
        "max_seq_length": max_seq_length,
    }

    if os.path.exists(test_path):
        config["test"] = True
        config["test_batches"] = 10
        config["val_data"] = test_path

    return config


def _count_lines(path: str) -> int:
    """Count lines in a file."""
    if not os.path.exists(path):
        return 0
    with open(path) as f:
        return sum(1 for _ in f)


def write_training_log(log_path: str, entry: dict[str, Any]) -> None:
    """Append a training log entry as JSONL."""
    with open(log_path, "a") as f:
        f.write(json.dumps(entry) + "\n")


def run_train(
    dataset_dir: str,
    base_model: str,
    adapter_path: str,
    epochs: int = 3,
    batch_size: int = 4,
    learning_rate: float = 1e-5,
    lora_rank: int | None = None,
    lora_layers: int = 16,
    max_seq_length: int = 2048,
    ults_dir: str | None = None,
    dry_run: bool = False,
) -> dict[str, Any]:
    """Run LoRA fine-tuning.

    Returns a summary dict with training metadata.
    """
    # Load and validate manifest
    manifest = load_manifest(dataset_dir)
    errors = validate_manifest(manifest)
    if errors:
        print("Manifest validation failed:", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        raise SystemExit(1)

    # Resolve LoRA rank
    resolved_rank = resolve_lora_rank(dataset_dir, lora_rank, ults_dir)

    # Use manifest row count (authoritative) instead of re-counting file lines
    train_rows = manifest.get("splits", {}).get("train", {}).get("rows", 0)

    # Build config
    config = build_train_config(
        dataset_dir=dataset_dir,
        base_model=base_model,
        adapter_path=adapter_path,
        epochs=epochs,
        batch_size=batch_size,
        learning_rate=learning_rate,
        lora_rank=resolved_rank,
        lora_layers=lora_layers,
        max_seq_length=max_seq_length,
        train_rows=train_rows,
    )
    summary = {
        "dataset_id": manifest.get("dataset_id"),
        "dataset_version": manifest.get("version"),
        "base_model": base_model,
        "adapter_path": adapter_path,
        "lora_rank": resolved_rank,
        "lora_layers": lora_layers,
        "epochs": epochs,
        "batch_size": batch_size,
        "learning_rate": learning_rate,
        "train_rows": train_rows,
        "iters": config["iters"],
        "exogenous_ratio": manifest.get("quality_gates", {}).get("exogenous_ratio", 0),
    }

    if dry_run:
        summary["status"] = "dry_run"
        return summary

    # Create adapter output directory
    os.makedirs(adapter_path, exist_ok=True)

    # Write config for reproducibility
    config_path = os.path.join(adapter_path, "train_config.json")
    with open(config_path, "w") as f:
        json.dump(config, f, indent=2)

    # Import mlx-lm (deferred to avoid import error when not installed)
    try:
        from mlx_lm import lora as mlx_lora  # noqa: F401
    except ImportError:
        print(
            "ERROR: mlx-lm not installed. Install with: pip install 'mlx-lm>=0.20.0'",
            file=sys.stderr,
        )
        raise SystemExit(1)

    # Set up training log
    log_path = os.path.join(adapter_path, "training_log.jsonl")
    start_time = time.time()

    write_training_log(log_path, {
        "event": "train_start",
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "config": config,
    })

    # Run training via mlx-lm CLI (subprocess for clean isolation)
    import subprocess

    cmd = [
        sys.executable, "-m", "mlx_lm.lora",
        "--model", base_model,
        "--data", dataset_dir,
        "--adapter-path", adapter_path,
        "--train",
        "--iters", str(config["iters"]),
        "--batch-size", str(batch_size),
        "--learning-rate", str(learning_rate),
        "--lora-layers", str(lora_layers),
        "--lora-rank", str(resolved_rank),
    ]

    if config.get("test"):
        cmd.extend(["--test", "--test-batches", str(config["test_batches"])])

    result = subprocess.run(cmd, capture_output=False)

    elapsed = time.time() - start_time
    summary["elapsed_seconds"] = round(elapsed, 1)
    summary["status"] = "success" if result.returncode == 0 else "failed"
    summary["exit_code"] = result.returncode

    write_training_log(log_path, {
        "event": "train_end",
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "status": summary["status"],
        "elapsed_seconds": summary["elapsed_seconds"],
    })

    if result.returncode != 0:
        raise SystemExit(result.returncode)

    return summary


def main():
    parser = argparse.ArgumentParser(
        description="LoRA Fine-Tuning for MDEMG",
    )
    parser.add_argument(
        "--dataset", required=True,
        help="Dataset directory containing manifest.json and train.jsonl",
    )
    parser.add_argument(
        "--base-model", required=True,
        help="Base model (HuggingFace ID or local path)",
    )
    parser.add_argument(
        "--adapter-path", required=True,
        help="Output directory for LoRA adapter weights",
    )
    parser.add_argument("--epochs", type=int, default=3, help="Training epochs (default: 3)")
    parser.add_argument("--batch-size", type=int, default=4, help="Batch size (default: 4)")
    parser.add_argument(
        "--learning-rate", type=float, default=1e-5,
        help="Learning rate (default: 1e-5)",
    )
    parser.add_argument(
        "--lora-rank", type=int, default=None,
        help="LoRA rank (default: max from ULTS specs, or 16)",
    )
    parser.add_argument("--lora-layers", type=int, default=16, help="Number of LoRA layers (default: 16)")
    parser.add_argument(
        "--max-seq-length", type=int, default=2048,
        help="Maximum sequence length (default: 2048)",
    )
    parser.add_argument(
        "--ults-dir", default=None,
        help="ULTS specs directory for auto-resolving LoRA rank",
    )
    parser.add_argument("--dry-run", action="store_true", help="Validate manifest and show config without training")
    parser.add_argument("--report", help="Write JSON summary to file")
    args = parser.parse_args()

    summary = run_train(
        dataset_dir=args.dataset,
        base_model=args.base_model,
        adapter_path=args.adapter_path,
        epochs=args.epochs,
        batch_size=args.batch_size,
        learning_rate=args.learning_rate,
        lora_rank=args.lora_rank,
        lora_layers=args.lora_layers,
        max_seq_length=args.max_seq_length,
        ults_dir=args.ults_dir,
        dry_run=args.dry_run,
    )

    # Print summary
    print()
    print("═══ Training Summary ═══")
    for k, v in summary.items():
        print(f"  {k}: {v}")

    if args.report:
        with open(args.report, "w") as f:
            json.dump(summary, f, indent=2)
        print(f"\nReport written to {args.report}")


if __name__ == "__main__":
    main()
