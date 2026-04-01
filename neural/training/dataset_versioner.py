#!/usr/bin/env python3
"""
dataset_versioner.py — Dataset Assembly & Versioning

Assembles filtered + converted training data into versioned datasets with
temporal train/test/val splits. Supports multi-source merging and deduplication.

Pipeline:
    1. Load all filtered JSONL from --input-dir (supports multiple sources)
    2. Merge — combine all records, tag with source
    3. Deduplicate — SHA-256(system_prompt + user_prompt), keep first by time
    4. Temporal sort — sort by time
    5. Temporal split — train/test/val by time (NEVER random)
    6. Task balance check — warn if any task has < min_per_task records
    7. Exogenous ratio check — verify alpha >= threshold
    8. Format convert — run format_converter on each split
    9. Write train.jsonl, test.jsonl, val.jsonl + manifest.json

Usage:
    python -m training.dataset_versioner \\
        --input-dir /tmp/filtered/ \\
        --output-dir /tmp/curated/v1/ \\
        --version v1 \\
        --train-ratio 0.8 --test-ratio 0.1 --val-ratio 0.1 \\
        --min-per-task 10 --exogenous-ratio 0.4 --raft-ratio 0.8
"""

import argparse
import hashlib
import json
import os
import sys
import time
from pathlib import Path
from typing import Any

from training.format_converter import convert_record, validate_mlx_format


def _generate_dataset_id() -> str:
    """Generate a dataset identifier.

    Uses timestamp + hash for deterministic, collision-resistant IDs.
    CUIDv2 is preferred but may not be installed.
    """
    try:
        from cuid2 import cuid  # type: ignore
        return cuid()
    except ImportError:
        pass

    # Fallback: timestamp-based ID (collision-resistant for single-machine use)
    ts = str(time.time_ns())
    h = hashlib.sha256(ts.encode()).hexdigest()[:12]
    return f"ds_{h}"


def _compute_sha256(path: str) -> str:
    """Compute SHA-256 hash of a file."""
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            h.update(chunk)
    return h.hexdigest()


def load_records(input_dirs: list[str]) -> list[dict[str, Any]]:
    """Load all JSONL records from input directories."""
    records = []
    for d in input_dirs:
        p = Path(d)
        jsonl_files = sorted(p.glob("*.jsonl"))
        for jf in jsonl_files:
            source = str(jf)
            with open(jf) as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        record = json.loads(line)
                        record["_source"] = source
                        records.append(record)
                    except json.JSONDecodeError:
                        continue
    return records


def deduplicate(
    records: list[dict[str, Any]],
    dedup_key: str = "prompt",
) -> tuple[list[dict[str, Any]], int]:
    """Deduplicate records, keeping first by time.

    dedup_key modes:
        prompt: SHA-256(system_prompt + user_prompt) — SFT training
        prompt-response: SHA-256(system_prompt + user_prompt + response) — DPO/diversity
        trace: SHA-256(trace_id) — no dedup (debugging only)
    """
    # Sort by time first so "first" is deterministic
    records.sort(key=lambda r: r.get("time", ""))

    seen: set[str] = set()
    unique = []
    duplicates = 0

    for rec in records:
        sp = rec.get("system_prompt", "") or ""
        up = rec.get("user_prompt", "") or ""
        if dedup_key == "prompt-response":
            resp = rec.get("response", "") or ""
            dedup_input = sp + up + resp
        elif dedup_key == "trace":
            dedup_input = rec.get("trace_id", "") or ""
        else:
            dedup_input = sp + up
        h = hashlib.sha256(dedup_input.encode()).hexdigest()
        if h in seen:
            duplicates += 1
            continue
        seen.add(h)
        unique.append(rec)

    return unique, duplicates


def temporal_split(
    records: list[dict[str, Any]],
    train_ratio: float,
    test_ratio: float,
    val_ratio: float,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]], list[dict[str, Any]]]:
    """Split records temporally. Records MUST be sorted by time.

    Train = first train_ratio%, test = next test_ratio%, val = last val_ratio%.
    """
    n = len(records)
    if n == 0:
        return [], [], []

    # Handle edge case: too few records
    if n < 3:
        return records, [], []

    train_end = int(n * train_ratio)
    test_end = train_end + int(n * test_ratio)

    # Ensure at least 1 record in each non-empty split
    if train_end == 0:
        train_end = 1
    if test_end <= train_end:
        test_end = min(train_end + 1, n)

    train = records[:train_end]
    test = records[train_end:test_end]
    val = records[test_end:]

    return train, test, val


def check_temporal_integrity(
    train: list[dict[str, Any]],
    test: list[dict[str, Any]],
    val: list[dict[str, Any]],
) -> bool:
    """Verify no temporal leakage: max(train.time) < min(test.time)."""
    if not train or not test:
        return True

    max_train = max(r.get("time", "") for r in train)
    min_test = min(r.get("time", "") for r in test)

    return max_train <= min_test


def compute_task_distribution(records: list[dict[str, Any]]) -> dict[str, int]:
    """Count records per task_name."""
    dist: dict[str, int] = {}
    for r in records:
        task = r.get("task_name", "unknown") or "unknown"
        dist[task] = dist.get(task, 0) + 1
    return dist


def write_split(records: list[dict[str, Any]], path: str, raft_ratio: float) -> dict[str, Any]:
    """Convert records to MLX format and write to JSONL. Returns split metadata."""
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)

    rows = 0
    with open(path, "w") as f:
        for rec in records:
            converted = convert_record(rec, raft_ratio)
            errors = validate_mlx_format(converted)
            if errors:
                continue
            f.write(json.dumps(converted) + "\n")
            rows += 1

    sha = _compute_sha256(path)

    # Temporal range
    times = [r.get("time", "") for r in records if r.get("time")]
    time_from = min(times) if times else ""
    time_to = max(times) if times else ""

    return {
        "rows": rows,
        "sha256": sha,
        "temporal_range": {"from": time_from, "to": time_to},
        "task_distribution": compute_task_distribution(records),
    }


def run_versioner(
    input_dirs: list[str],
    output_dir: str,
    version: str,
    train_ratio: float = 0.8,
    test_ratio: float = 0.1,
    val_ratio: float = 0.1,
    min_per_task: int = 10,
    exogenous_ratio: float = 0.4,
    raft_ratio: float = 0.8,
    dedup_key: str = "prompt",
    first_cycle: bool = False,
) -> dict[str, Any]:
    """Run the dataset versioning pipeline. Returns the dataset manifest."""
    os.makedirs(output_dir, exist_ok=True)

    # Step 1: Load
    all_records = load_records(input_dirs)
    total_loaded = len(all_records)

    if total_loaded == 0:
        raise ValueError("No records found in input directories")

    # Step 2-3: Deduplicate
    unique_records, duplicates_removed = deduplicate(all_records, dedup_key=dedup_key)

    # Step 4: Temporal sort (already done in deduplicate, but re-sort for safety)
    unique_records.sort(key=lambda r: r.get("time", ""))

    # Step 5: Temporal split
    train, test, val = temporal_split(unique_records, train_ratio, test_ratio, val_ratio)

    # Step 5b: Temporal integrity check
    temporal_ok = check_temporal_integrity(train, test, val)
    if not temporal_ok:
        raise ValueError("TEMPORAL LEAK: max(train.time) >= min(test.time)")

    # Step 6: Task balance check
    train_dist = compute_task_distribution(train)
    min_per_task_met = all(c >= min_per_task for c in train_dist.values()) if train_dist else False

    warnings = []
    for task, count in train_dist.items():
        if count < min_per_task:
            warnings.append(f"Task '{task}' has only {count} train records (min: {min_per_task})")

    # Step 7: Exogenous ratio
    total_for_ratio = len(unique_records)
    if first_cycle:
        # First training cycle: all data is exogenous by definition
        actual_exogenous = 1.0
        print("INFO: --first-cycle: all data treated as exogenous (no prior training)")
    else:
        exogenous_count = sum(1 for r in unique_records if not r.get("used_for_train", False))
        actual_exogenous = exogenous_count / total_for_ratio if total_for_ratio > 0 else 1.0

    # Step 8-9: Format convert + write splits
    train_meta = write_split(train, os.path.join(output_dir, "train.jsonl"), raft_ratio)
    test_meta = write_split(test, os.path.join(output_dir, "test.jsonl"), raft_ratio)
    val_meta = write_split(val, os.path.join(output_dir, "val.jsonl"), raft_ratio)

    # Check no overlap
    train_times = {r.get("time") for r in train}
    test_times = {r.get("time") for r in test}
    val_times = {r.get("time") for r in val}
    no_overlap = not (train_times & test_times) and not (train_times & val_times) and not (test_times & val_times)

    # Build manifest
    dataset_id = _generate_dataset_id()
    sources = list({r.get("_source", "unknown") for r in all_records})

    manifest = {
        "dataset_id": dataset_id,
        "version": version,
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "sources": sources,
        "total_loaded": total_loaded,
        "deduplication": {
            "total_before": total_loaded,
            "duplicates_removed": duplicates_removed,
            "total_after": len(unique_records),
        },
        "splits": {
            "train": train_meta,
            "test": test_meta,
            "val": val_meta,
        },
        "quality_gates": {
            "temporal_split": temporal_ok,
            "no_train_test_overlap": no_overlap,
            "exogenous_ratio": round(actual_exogenous, 4),
            "exogenous_ratio_met": actual_exogenous >= exogenous_ratio,
            "min_per_task_met": min_per_task_met,
        },
        "config": {
            "train_ratio": train_ratio,
            "test_ratio": test_ratio,
            "val_ratio": val_ratio,
            "min_per_task": min_per_task,
            "exogenous_ratio_target": exogenous_ratio,
            "raft_ratio": raft_ratio,
        },
        "warnings": warnings,
    }

    # Write manifest
    manifest_path = os.path.join(output_dir, "manifest.json")
    with open(manifest_path, "w") as f:
        json.dump(manifest, f, indent=2)

    return manifest


def main():
    parser = argparse.ArgumentParser(description="Dataset Versioner")
    parser.add_argument("--input-dir", nargs="+", required=True, help="Input directories with filtered JSONL")
    parser.add_argument("--output-dir", required=True, help="Output directory for versioned dataset")
    parser.add_argument("--version", required=True, help="Dataset version label (e.g., v1)")
    parser.add_argument("--train-ratio", type=float, default=0.8, help="Train split ratio (default: 0.8)")
    parser.add_argument("--test-ratio", type=float, default=0.1, help="Test split ratio (default: 0.1)")
    parser.add_argument("--val-ratio", type=float, default=0.1, help="Val split ratio (default: 0.1)")
    parser.add_argument("--min-per-task", type=int, default=10, help="Minimum records per task (default: 10)")
    parser.add_argument("--exogenous-ratio", type=float, default=0.4, help="Minimum exogenous ratio (default: 0.4)")
    parser.add_argument("--raft-ratio", type=float, default=0.8, help="RAFT context inclusion ratio (default: 0.8)")
    parser.add_argument(
        "--dedup-key",
        choices=["prompt", "prompt-response", "trace"],
        default="prompt",
        help="Dedup hash mode: prompt (SFT), prompt-response (DPO), trace (debug). Default: prompt",
    )
    parser.add_argument("--first-cycle", action="store_true",
                        help="First training cycle: skip exogenous ratio check (all data is exogenous)")
    args = parser.parse_args()

    manifest = run_versioner(
        input_dirs=args.input_dir,
        output_dir=args.output_dir,
        version=args.version,
        train_ratio=args.train_ratio,
        test_ratio=args.test_ratio,
        val_ratio=args.val_ratio,
        min_per_task=args.min_per_task,
        exogenous_ratio=args.exogenous_ratio,
        raft_ratio=args.raft_ratio,
        dedup_key=args.dedup_key,
        first_cycle=args.first_cycle,
    )

    print(f"Dataset {manifest['version']} created: {args.output_dir}")
    print(f"  ID:    {manifest['dataset_id']}")
    print(f"  Train: {manifest['splits']['train']['rows']} rows")
    print(f"  Test:  {manifest['splits']['test']['rows']} rows")
    print(f"  Val:   {manifest['splits']['val']['rows']} rows")
    print(f"  Dedup: {manifest['deduplication']['duplicates_removed']} removed")
    print(f"  Temporal split: {'OK' if manifest['quality_gates']['temporal_split'] else 'FAILED'}")
    print(f"  No overlap:     {'OK' if manifest['quality_gates']['no_train_test_overlap'] else 'FAILED'}")
    print(f"  Exogenous:      {manifest['quality_gates']['exogenous_ratio']:.2f}")

    if manifest["warnings"]:
        print("\nWarnings:")
        for w in manifest["warnings"]:
            print(f"  - {w}")


if __name__ == "__main__":
    main()
