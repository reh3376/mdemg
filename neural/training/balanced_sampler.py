#!/usr/bin/env python3
"""balanced_sampler.py — Sprint FT-LORA-DATA Epic 3.

Per-tier pre-processing sampler (Appendix A Path A). NOT a mlx_lm patch —
rows are upsampled / downsampled before being handed to mlx_lm.lora.

Per-task behavior:
  * count < target  → upsample via deterministic duplication. If
    target/count > duplication_ceiling, abort with task name + factor.
  * count > target  → random.sample(rows, target) with seeded RNG.
  * count == target → pass-through.

TIER_TASKS mapping is the single source of truth for which task → which
family/tier. Anchor-prompt-derived (confirmed via
training_data/routing_profiles/anchor_prompts.jsonl, 16 tasks across 3
families as of 2026-04-22).

Usage:
    python -m neural.training.balanced_sampler \\
        --tier tier1 \\
        --input-real training_data/curated/sft_interactions_v2/recurated.jsonl \\
        --input-synth-dir training_data/curated/sft_synth_v1/ \\
        --per-task-floor 200 --ape-reflect-target 500 \\
        --duplication-ceiling 5 --seed 42 \\
        --out training_data/curated/balanced/tier1.jsonl
"""

from __future__ import annotations

import argparse
import hashlib
import json
import random
import sys
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any

# ---------------------------------------------------------------------------
# Task → family mapping (derived from anchor_prompts.jsonl, 2026-04-22).
# This is the single source of truth for Sprint FT-LORA-DATA. A task's
# family is fixed by Sprint D's activation profiling.
# ---------------------------------------------------------------------------

TIER_TASKS: dict[str, list[str]] = {
    "tier1": [
        # reasoning_think (T, 7)
        "ape.reflect", "consulting.synthesis", "hidden.summarize",
        "jiminy.synthesize", "metalearn.generalize", "retrieval.rerank_nli",
        "summarize.generate",
        # classify_notink (C, 6)
        "consulting.classify", "hidden.reclassify", "jiminy.codegen",
        "jiminy.evaluate", "retrieval.intent_translate", "retrieval.query_classify",
        # structured_notink (J, 3)
        "hidden.name_emergence", "jiminy.evaluate_llm", "retrieval.rerank_cross",
    ],
    "T": [
        "ape.reflect", "consulting.synthesis", "hidden.summarize",
        "jiminy.synthesize", "metalearn.generalize", "retrieval.rerank_nli",
        "summarize.generate",
    ],
    "C": [
        "consulting.classify", "hidden.reclassify", "jiminy.codegen",
        "jiminy.evaluate", "retrieval.intent_translate", "retrieval.query_classify",
    ],
    "J": [
        "hidden.name_emergence", "jiminy.evaluate_llm", "retrieval.rerank_cross",
    ],
}


class DuplicationCeilingExceeded(RuntimeError):
    """Raised when upsampling a task would require > duplication_ceiling."""


def build_per_task_target(
    tier: str,
    per_task_floor: int,
    ape_reflect_target: int,
) -> dict[str, int]:
    """Compute the target row-count for every task in a given tier.

    ape.reflect gets a special target (500 by default, diversity audit D-X5).
    All other tasks get per_task_floor. Tasks outside the tier are omitted.
    """
    if tier not in TIER_TASKS:
        raise ValueError(
            f"Unknown tier '{tier}'. Valid: {sorted(TIER_TASKS.keys())}",
        )
    targets: dict[str, int] = {}
    for task in TIER_TASKS[tier]:
        if task == "ape.reflect":
            targets[task] = ape_reflect_target
        else:
            targets[task] = per_task_floor
    return targets


def _task_of(row: dict[str, Any]) -> str | None:
    """Extract task name from either meta sidecar or top-level field."""
    meta = row.get("meta")
    if isinstance(meta, dict) and meta.get("task_name"):
        return meta["task_name"]
    return row.get("task_name")


def balanced_sample(
    rows: list[dict[str, Any]],
    per_task_target: dict[str, int],
    seed: int = 42,
    duplication_ceiling: int = 5,
) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Balance `rows` to `per_task_target` counts.

    Returns (balanced_rows, metadata). Metadata includes per-task
    duplication_factors, input_counts, output_counts, and the effective seed.

    Raises DuplicationCeilingExceeded when any task would require
    target/count > duplication_ceiling.
    """
    rng = random.Random(seed)

    # Bucket by task
    buckets: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for row in rows:
        task = _task_of(row)
        if task is None:
            continue
        if task in per_task_target:
            buckets[task].append(row)

    input_counts = {t: len(buckets.get(t, [])) for t in per_task_target}
    duplication_factors: dict[str, float] = {}
    output: list[dict[str, Any]] = []
    upsampled_rows_per_task: dict[str, int] = {}

    for task, target in per_task_target.items():
        available = buckets.get(task, [])
        count = len(available)

        if count == 0:
            # Absent task — leave a zero entry; caller must have supplied
            # synthesis rows already merged into `rows`.
            duplication_factors[task] = 0.0
            continue

        if count >= target:
            # Downsample or pass-through.
            sampled = rng.sample(available, target) if count > target else list(available)
            duplication_factors[task] = 1.0
            output.extend(sampled)
            continue

        # count < target: upsample via integer duplication with seeded shuffle
        # of remainder. Abort if factor exceeds ceiling.
        factor = target / count
        if factor > duplication_ceiling:
            raise DuplicationCeilingExceeded(
                f"task '{task}' needs factor {factor:.2f}× "
                f"({count} rows → {target} target) "
                f"but ceiling is {duplication_ceiling}×. "
                "Raise --duplication-ceiling or lower --per-task-floor for this task."
            )
        duplication_factors[task] = round(factor, 3)
        whole = target // count
        remainder = target - (whole * count)
        for _ in range(whole):
            # Deterministic duplication: keep original order each pass.
            output.extend(available)
        if remainder:
            # Seeded sample of remainder rows — distinct rows across duplicates.
            output.extend(rng.sample(available, remainder))
        upsampled_rows_per_task[task] = target - count

    metadata = {
        "input_counts": input_counts,
        "output_counts": dict(Counter(_task_of(r) for r in output)),
        "duplication_factors": duplication_factors,
        "upsampled_rows_per_task": upsampled_rows_per_task,
        "seed": seed,
        "duplication_ceiling": duplication_ceiling,
        "per_task_target": dict(per_task_target),
    }
    return output, metadata


# ---------------------------------------------------------------------------
# I/O helpers
# ---------------------------------------------------------------------------

def load_jsonl(path: Path) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            rows.append(json.loads(line))
    return rows


def load_synth_dir(dir_path: Path) -> list[dict[str, Any]]:
    """Load all *.jsonl files under dir_path (Epic 2 output)."""
    rows: list[dict[str, Any]] = []
    if not dir_path.exists():
        return rows
    for p in sorted(dir_path.glob("*.jsonl")):
        rows.extend(load_jsonl(p))
    return rows


def sha256_of_rows(rows: list[dict[str, Any]]) -> str:
    """Deterministic SHA256 over row sequence (json-dumps with sorted keys)."""
    h = hashlib.sha256()
    for r in rows:
        h.update(json.dumps(r, sort_keys=True).encode("utf-8"))
        h.update(b"\n")
    return h.hexdigest()


def write_jsonl(path: Path, rows: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w") as f:
        for r in rows:
            f.write(json.dumps(r) + "\n")


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(
        description="Sprint FT-LORA-DATA Epic 3 — per-tier balanced sampler.",
    )
    p.add_argument("--tier", required=True, choices=sorted(TIER_TASKS.keys()))
    p.add_argument("--input-real", required=True, type=Path,
                   help="Epic 1 output: recurated.jsonl (real traffic).")
    p.add_argument("--input-synth-dir", required=True, type=Path,
                   help="Epic 2 output dir: sft_synth_v1/ (4 synthesis files).")
    p.add_argument("--per-task-floor", type=int, default=200)
    p.add_argument("--ape-reflect-target", type=int, default=500)
    p.add_argument("--duplication-ceiling", type=int, default=5)
    p.add_argument("--seed", type=int, default=42)
    p.add_argument("--out", required=True, type=Path)
    p.add_argument("--report", type=Path, default=None,
                   help="Optional JSON report path.")
    args = p.parse_args(argv)

    if not args.input_real.exists():
        sys.stderr.write(f"ERROR: --input-real not found: {args.input_real}\n")
        return 2

    real_rows = load_jsonl(args.input_real)
    synth_rows = load_synth_dir(args.input_synth_dir)

    per_task_target = build_per_task_target(
        args.tier, args.per_task_floor, args.ape_reflect_target,
    )

    try:
        balanced, meta = balanced_sample(
            real_rows + synth_rows,
            per_task_target=per_task_target,
            seed=args.seed,
            duplication_ceiling=args.duplication_ceiling,
        )
    except DuplicationCeilingExceeded as e:
        sys.stderr.write(f"ERROR: duplication ceiling breached: {e}\n")
        return 3

    write_jsonl(args.out, balanced)

    meta["tier"] = args.tier
    meta["input_real_rows"] = len(real_rows)
    meta["input_synth_rows"] = len(synth_rows)
    meta["output_total_rows"] = len(balanced)
    meta["output_sha256"] = sha256_of_rows(balanced)
    meta["output_path"] = str(args.out)

    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(meta, indent=2, sort_keys=True))

    print(f"Tier:              {args.tier}")
    print(f"Input real rows:   {meta['input_real_rows']}")
    print(f"Input synth rows:  {meta['input_synth_rows']}")
    print(f"Output rows:       {meta['output_total_rows']}")
    print(f"Output SHA256:     {meta['output_sha256']}")
    print("Per-task (input → target → output | factor):")
    for task in per_task_target:
        src = meta["input_counts"].get(task, 0)
        tgt = per_task_target[task]
        out = meta["output_counts"].get(task, 0)
        f = meta["duplication_factors"].get(task, 0.0)
        print(f"  {task}: {src} → {tgt} → {out}  (×{f})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
