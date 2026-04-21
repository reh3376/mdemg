#!/usr/bin/env python3
"""
openai_ft_upload_and_launch.py — Upload JSONL + launch fine-tuning job.

Reads the artifacts produced by `training.openai_ft_adapter` and:

    1. Re-runs the pre-upload check (scripts/openai_ft_check.py).
    2. Enforces a hard cost cap (--max-cost-usd, default 50.00). Aborts
       BEFORE any network I/O if the manifest's cost estimate exceeds the cap.
    3. Confirms with the user (interactive) unless --yes is passed.
    4. Uploads combined_train.jsonl and combined_val.jsonl via
       openai.files.create(purpose="fine-tune").
    5. Launches a fine-tuning job via openai.fine_tuning.jobs.create().
    6. Appends provenance + job metadata to run_notes.md in the artifact dir.

Requires:
    - $OPENAI_API_KEY in the environment
    - Python package `openai>=1.50`

Usage:
    python scripts/openai_ft_upload_and_launch.py \\
        --dir training_data/openai_ft/20260420 \\
        --model gpt-4.1-mini-2025-04-14 \\
        --max-cost-usd 50.00 \\
        [--suffix mdemg-ftoai001] \\
        [--yes]
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from datetime import datetime, timezone


def _load_manifest(directory: str) -> dict:
    path = os.path.join(directory, "manifest.json")
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def _run_pre_check(directory: str) -> int:
    """Shell out to openai_ft_check.py for a single source of truth."""
    script = os.path.join(os.path.dirname(os.path.abspath(__file__)), "openai_ft_check.py")
    result = subprocess.run(
        [sys.executable, script, "--dir", directory],
        check=False,
    )
    return result.returncode


def _confirm(prompt: str) -> bool:
    try:
        reply = input(prompt).strip().lower()
    except EOFError:
        return False
    return reply in {"y", "yes"}


def _append_run_notes(
    directory: str,
    *,
    model: str,
    train_file_id: str,
    val_file_id: str,
    job_id: str,
    cost_estimate_usd: float,
    suffix: str | None,
) -> None:
    path = os.path.join(directory, "run_notes.md")
    entry = f"""\
## Fine-tuning job launched — {datetime.now(timezone.utc).isoformat()}

| Field | Value |
|---|---|
| job_id | `{job_id}` |
| base_model | `{model}` |
| suffix | `{suffix or "(none)"}` |
| train_file_id | `{train_file_id}` |
| val_file_id | `{val_file_id}` |
| cost_estimate_usd | {cost_estimate_usd:.2f} |
| start_time | {datetime.now(timezone.utc).isoformat()} |
| end_time | _pending_ |
| n_epochs_actual | _pending_ |
| final_train_loss | _pending_ |
| final_val_loss | _pending_ |
| total_cost_usd | _pending_ |
| error_if_any | _none_ |

Monitor with:

    openai fine_tuning.jobs.retrieve {job_id}

"""
    mode = "a" if os.path.exists(path) else "w"
    with open(path, mode, encoding="utf-8") as f:
        f.write(entry)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dir", required=True, help="openai_ft artifact directory")
    parser.add_argument("--model", default="gpt-4.1-mini-2025-04-14")
    parser.add_argument(
        "--max-cost-usd",
        type=float,
        default=50.00,
        help="Abort before upload if manifest cost_estimate_usd exceeds this.",
    )
    parser.add_argument(
        "--suffix",
        default=None,
        help="Optional OpenAI job suffix (appears in fine_tuned_model name).",
    )
    parser.add_argument("--yes", action="store_true", help="Skip interactive confirmation.")
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print what would happen and exit 0 without touching OpenAI.",
    )
    args = parser.parse_args(argv)

    # 1. Pre-upload check
    rc = _run_pre_check(args.dir)
    if rc != 0:
        print("ABORT: pre-upload check failed.", file=sys.stderr)
        return rc

    # 2. Cost cap — BEFORE network I/O
    manifest = _load_manifest(args.dir)
    cost = float(manifest.get("totals", {}).get("cost_estimate_usd", 0.0))
    if cost > args.max_cost_usd:
        print(
            f"ABORT: manifest cost_estimate_usd ${cost:.2f} exceeds --max-cost-usd ${args.max_cost_usd:.2f}. "
            f"Raise the cap or shrink the dataset. No network calls made.",
            file=sys.stderr,
        )
        return 2

    # 3. Confirmation
    train_rows = manifest.get("splits", {}).get("train", {}).get("rows", 0)
    val_rows = manifest.get("splits", {}).get("val", {}).get("rows", 0)
    print(
        f"About to upload and launch fine-tuning job:\n"
        f"  model            = {args.model}\n"
        f"  suffix           = {args.suffix or '(none)'}\n"
        f"  train rows       = {train_rows}\n"
        f"  val rows         = {val_rows}\n"
        f"  cost estimate    = ${cost:.2f} (cap ${args.max_cost_usd:.2f})\n"
        f"  artifact dir     = {args.dir}\n"
    )

    if args.dry_run:
        print("DRY-RUN: exiting without uploading.")
        return 0

    if not args.yes and not _confirm("Proceed? [y/N]: "):
        print("ABORT: user declined confirmation.", file=sys.stderr)
        return 3

    # 4. Upload + launch
    if not os.environ.get("OPENAI_API_KEY"):
        print("ABORT: OPENAI_API_KEY not set.", file=sys.stderr)
        return 4

    try:
        from openai import OpenAI  # local import — heavy, optional for --dry-run
    except ImportError:
        print(
            "ABORT: `openai` package not installed. Run `pip install 'openai>=1.50'`.",
            file=sys.stderr,
        )
        return 5

    client = OpenAI()

    train_path = os.path.join(args.dir, "combined_train.jsonl")
    val_path = os.path.join(args.dir, "combined_val.jsonl")

    print(f"Uploading {train_path} …")
    with open(train_path, "rb") as f:
        train_file = client.files.create(file=f, purpose="fine-tune")
    print(f"  train file id: {train_file.id}")

    print(f"Uploading {val_path} …")
    with open(val_path, "rb") as f:
        val_file = client.files.create(file=f, purpose="fine-tune")
    print(f"  val file id:   {val_file.id}")

    job_kwargs: dict = {
        "training_file": train_file.id,
        "validation_file": val_file.id,
        "model": args.model,
    }
    if args.suffix:
        job_kwargs["suffix"] = args.suffix

    print(f"Launching fine-tuning job (model={args.model}) …")
    job = client.fine_tuning.jobs.create(**job_kwargs)
    print(f"  job id: {job.id}")
    print(f"  status: {job.status}")

    _append_run_notes(
        args.dir,
        model=args.model,
        train_file_id=train_file.id,
        val_file_id=val_file.id,
        job_id=job.id,
        cost_estimate_usd=cost,
        suffix=args.suffix,
    )
    print(f"run_notes.md updated at {os.path.join(args.dir, 'run_notes.md')}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
