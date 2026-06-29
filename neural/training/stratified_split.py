#!/usr/bin/env python3
"""stratified_split.py — Sprint FT-LORA-DATA Epic 4.

Per-task stratified 90/10 train/valid split + manifest emission.

Invariants:
  * Each task partitioned independently with seeded shuffle.
  * When a task has ≥ 20 rows, valid gets min 2 rows (floor).
  * When a task has < 20 rows, split is count-wise with same 90/10 ratio
    (1 valid if rounds up, else 0 — but the balanced sampler guarantees
    ≥ 200 per task, so this branch is defensive).
  * Manifest includes file SHA256s, per-task counts, provenance, and
    Sprint-plan-pinned SHAs.

Usage:
    python -m neural.training.stratified_split \\
        --input training_data/curated/balanced/tier1.jsonl \\
        --out-dir training_data/sft/tier1 \\
        --tier tier1 --family-name tier1 \\
        --base-dataset-ver v2026-04-22 \\
        --seed 42
"""

from __future__ import annotations

import argparse
import hashlib
import json
import random
import subprocess
import sys
from collections import defaultdict
from pathlib import Path
from typing import Any

# Reuse sibling modules
_NEURAL_ROOT = Path(__file__).resolve().parents[1]
if str(_NEURAL_ROOT) not in sys.path:
    sys.path.insert(0, str(_NEURAL_ROOT))

from training.balanced_sampler import TIER_TASKS, _task_of  # noqa: E402
from training.recurate import SPRINT_PLAN_RAW_SHA_PIN  # noqa: E402

# Sprint C model config pin (docs/development/ft-lora/sprint_plan_ft_lora_data.md §1)
SPRINT_C_MODEL_CONFIG_SHA = (
    "a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5"
)

# Plan §5 Epic 4 — 90/10 split ratio (train:valid)
TRAIN_RATIO = 0.9
VALID_FLOOR_WHEN_ABUNDANT = 2  # min valid rows when count ≥ 20


def file_sha256(path: Path, chunk_size: int = 1 << 20) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        while chunk := f.read(chunk_size):
            h.update(chunk)
    return h.hexdigest()


def split_task(rows: list[dict[str, Any]], seed: int,
               ratio: float = TRAIN_RATIO,
               floor: int = VALID_FLOOR_WHEN_ABUNDANT) -> tuple[list, list]:
    """Shuffle then split one task's rows.

    * count ≥ 20: valid = max(floor, round(count * (1 - ratio)))
    * count < 20:  valid = max(1, round(count * (1 - ratio))) if count ≥ 2 else 0

    Returns (train_rows, valid_rows).
    """
    rng = random.Random(seed)
    rows = list(rows)
    rng.shuffle(rows)

    n = len(rows)
    if n == 0:
        return [], []

    if n >= 20:
        valid_n = max(floor, round(n * (1.0 - ratio)))
    elif n >= 2:
        valid_n = max(1, round(n * (1.0 - ratio)))
    else:  # single row
        valid_n = 0

    train_n = n - valid_n
    return rows[:train_n], rows[train_n:]


def stratified_split(rows: list[dict[str, Any]],
                     seed: int = 42,
                     ratio: float = TRAIN_RATIO,
                     ) -> tuple[list, list, dict[str, dict[str, int]]]:
    """Stratify rows by meta.task_name, split each 90/10, concatenate.

    Returns (train_rows, valid_rows, per_task_counts).
    per_task_counts[task] = {"train": N, "valid": M, "total": N+M}
    """
    buckets: dict[str, list[dict]] = defaultdict(list)
    for r in rows:
        t = _task_of(r)
        if t is None:
            continue
        buckets[t].append(r)

    train: list[dict] = []
    valid: list[dict] = []
    per_task: dict[str, dict[str, int]] = {}
    # Stable task ordering for deterministic output.
    for task in sorted(buckets.keys()):
        task_rows = buckets[task]
        # Each task gets its own sub-seed so adding/removing tasks doesn't
        # reshuffle other tasks.
        sub_seed = hash((seed, task)) & 0x7FFFFFFF
        tr, va = split_task(task_rows, seed=sub_seed, ratio=ratio)
        train.extend(tr)
        valid.extend(va)
        per_task[task] = {
            "train": len(tr), "valid": len(va), "total": len(tr) + len(va),
        }
    return train, valid, per_task


def build_source_composition(rows: list[dict[str, Any]]) -> dict[str, int]:
    """Tally rows by provenance source tag."""
    tally: dict[str, int] = defaultdict(int)
    for r in rows:
        meta = r.get("meta", {})
        src = meta.get("source", "unknown")
        tally[src] += 1
    return dict(tally)


def count_weak_signal_rows(rows: list[dict[str, Any]]) -> int:
    return sum(1 for r in rows if r.get("meta", {}).get("weak_signal"))


def load_ults_versions(ults_dir: Path, tasks: list[str]) -> dict[str, str]:
    """Return {task_name: "<ults_version>-task-<task_version>"} for traceability."""
    versions: dict[str, str] = {}
    if not ults_dir.exists():
        return versions
    for task in tasks:
        # file naming: "consulting.synthesis" → "consulting_synthesis.ults.json"
        fname = task.replace(".", "_") + ".ults.json"
        p = ults_dir / fname
        if not p.exists():
            continue
        try:
            spec = json.loads(p.read_text())
            uv = spec.get("ults_version", "?")
            tv = spec.get("task", {}).get("version", "?")
            versions[task] = f"{uv}-task-{tv}"
        except (json.JSONDecodeError, OSError):
            continue
    return versions


def git_sha(project_root: Path) -> str:
    try:
        out = subprocess.check_output(
            ["git", "-C", str(project_root), "rev-parse", "HEAD"],
            stderr=subprocess.DEVNULL,
        )
        return out.decode().strip()
    except (subprocess.CalledProcessError, FileNotFoundError):
        return "unknown"


def write_jsonl(path: Path, rows: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w") as f:
        for r in rows:
            f.write(json.dumps(r) + "\n")


def build_manifest(
    *,
    tier: str,
    family_name: str,
    out_dir: Path,
    train_rows: list[dict[str, Any]],
    valid_rows: list[dict[str, Any]],
    per_task: dict[str, dict[str, int]],
    seed: int,
    base_dataset_ver: str,
    duplication_factors: dict[str, float] | None,
    generator_sha: str,
    ults_versions: dict[str, str],
    synthesis_version: str,
    ape_reflect_target: int,
    meta_placement: str,
) -> dict[str, Any]:
    all_rows = train_rows + valid_rows
    train_path = out_dir / "train.jsonl"
    valid_path = out_dir / "valid.jsonl"
    return {
        "sprint": "FT-LORA-DATA",
        "generator_sha": generator_sha,
        "tier": tier,
        "family_name": family_name,
        "row_counts": {
            "train": len(train_rows),
            "valid": len(valid_rows),
            "total": len(all_rows),
        },
        "per_task_counts": per_task,
        "source_composition": build_source_composition(all_rows),
        "weak_signal_rows": count_weak_signal_rows(all_rows),
        "ape_reflect_target": ape_reflect_target,
        "duplication_factors": duplication_factors or {},
        "seed": seed,
        "file_sha256": {
            "train.jsonl": file_sha256(train_path),
            "valid.jsonl": file_sha256(valid_path),
        },
        "base_dataset_ver": base_dataset_ver,
        "trained_against_model_sha": SPRINT_C_MODEL_CONFIG_SHA,
        "raw_dataset_sha_pin": SPRINT_PLAN_RAW_SHA_PIN,
        "ults_spec_versions": ults_versions,
        "synthesis_version": synthesis_version,
        "meta_placement": meta_placement,
        "synthesis_non_determinism_note": (
            "Teacher temp=0.7; Epic 2 outputs are non-reproducible bit-identical. "
            "Determinism applies from balanced_sampler onward given fixed Epic 2 outputs."
        ),
    }


def strip_meta_sidecar(rows: list[dict[str, Any]]) -> tuple[list[dict], list[dict]]:
    """Split `meta` sidecar off each row (for meta_placement='sidecar' fallback).

    Returns (rows_without_meta, meta_entries_in_parallel_order).
    """
    stripped: list[dict] = []
    meta_entries: list[dict] = []
    for r in rows:
        meta = r.get("meta", {})
        row_copy = {k: v for k, v in r.items() if k != "meta"}
        stripped.append(row_copy)
        meta_entries.append(meta)
    return stripped, meta_entries


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(
        description="Sprint FT-LORA-DATA Epic 4 — stratified 90/10 split + manifest.",
    )
    p.add_argument("--input", required=True, type=Path,
                   help="Epic 3 output: balanced JSONL.")
    p.add_argument("--out-dir", required=True, type=Path,
                   help="Target directory (train.jsonl + valid.jsonl land here).")
    p.add_argument("--tier", required=True, choices=sorted(TIER_TASKS.keys()))
    p.add_argument("--family-name", required=True,
                   help="Name recorded in manifest (e.g., 'tier1', 'reasoning_think').")
    p.add_argument("--seed", type=int, default=42)
    p.add_argument("--train-ratio", type=float, default=TRAIN_RATIO)
    p.add_argument("--base-dataset-ver", required=True,
                   help="Matches --dataset-ver from recurate.py.")
    p.add_argument("--synthesis-version", default="v1-unknown",
                   help="Stamped in manifest. Typically v1-{git_sha_short}.")
    p.add_argument("--ape-reflect-target", type=int, default=500)
    p.add_argument("--sampler-report", type=Path, default=None,
                   help="Optional: balanced_sampler.py --report JSON to carry through.")
    p.add_argument("--ults-dir", type=Path,
                   default=_NEURAL_ROOT.parent / "docs/tests/ults/specs")
    p.add_argument("--meta-placement", choices=("embedded", "sidecar"),
                   default="embedded",
                   help="Embedded: meta field on each row. Sidecar: meta stripped "
                        "to sibling meta.jsonl (activate when mlx_lm rejects meta).")
    p.add_argument("--project-root", type=Path,
                   default=_NEURAL_ROOT.parent)
    args = p.parse_args(argv)

    if not args.input.exists():
        sys.stderr.write(f"ERROR: --input not found: {args.input}\n")
        return 2

    with open(args.input) as f:
        rows = [json.loads(line) for line in f if line.strip()]

    train_rows, valid_rows, per_task = stratified_split(
        rows, seed=args.seed, ratio=args.train_ratio,
    )

    args.out_dir.mkdir(parents=True, exist_ok=True)

    # Meta-placement: embedded (default) keeps rows intact; sidecar strips
    # meta into a parallel-order meta.jsonl for mlx_lm compatibility.
    if args.meta_placement == "sidecar":
        train_stripped, train_meta = strip_meta_sidecar(train_rows)
        valid_stripped, valid_meta = strip_meta_sidecar(valid_rows)
        write_jsonl(args.out_dir / "train.jsonl", train_stripped)
        write_jsonl(args.out_dir / "valid.jsonl", valid_stripped)
        write_jsonl(args.out_dir / "meta.jsonl", train_meta + valid_meta)
    else:
        write_jsonl(args.out_dir / "train.jsonl", train_rows)
        write_jsonl(args.out_dir / "valid.jsonl", valid_rows)

    # Carry duplication_factors through from sampler report if provided.
    duplication_factors: dict[str, float] = {}
    if args.sampler_report and args.sampler_report.exists():
        report = json.loads(args.sampler_report.read_text())
        duplication_factors = report.get("duplication_factors", {})

    ults_versions = load_ults_versions(args.ults_dir, TIER_TASKS[args.tier])

    manifest = build_manifest(
        tier=args.tier,
        family_name=args.family_name,
        out_dir=args.out_dir,
        train_rows=train_rows,
        valid_rows=valid_rows,
        per_task=per_task,
        seed=args.seed,
        base_dataset_ver=args.base_dataset_ver,
        duplication_factors=duplication_factors,
        generator_sha=git_sha(args.project_root),
        ults_versions=ults_versions,
        synthesis_version=args.synthesis_version,
        ape_reflect_target=args.ape_reflect_target,
        meta_placement=args.meta_placement,
    )

    manifest_path = args.out_dir / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True))

    print(f"Tier:         {args.tier}")
    print(f"Family:       {args.family_name}")
    print(f"Out dir:      {args.out_dir}")
    print(f"Train rows:   {len(train_rows)}")
    print(f"Valid rows:   {len(valid_rows)}")
    print(f"Train SHA:    {manifest['file_sha256']['train.jsonl']}")
    print(f"Valid SHA:    {manifest['file_sha256']['valid.jsonl']}")
    print(f"Meta placement: {args.meta_placement}")
    print("Per-task (train / valid / total):")
    for task, counts in sorted(per_task.items()):
        print(f"  {task}: {counts['train']} / {counts['valid']} / {counts['total']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
