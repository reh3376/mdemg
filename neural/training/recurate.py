#!/usr/bin/env python3
"""recurate.py — Sprint FT-LORA-DATA Epic 1

Provenance-preserving re-curation of the raw llm_interactions.jsonl corpus.

Differences from format_converter.py (existing):
  * Preserves task_name, trace_id, instance_id, space_id, dataset_ver,
    quality, quality_source, source_path under a `meta` sidecar field on
    each output row. These fields are required by the FT-LORA-DATA manifest
    and by Sprint F adapter-regression debugging.
  * Asserts SHA256 of the raw input against a sprint-plan-pinned value
    (raw_dataset_sha_pin). Drift aborts with a field-level message.
  * Applies a configurable quality floor (default 0.6) and records per-task
    drop counts.

Output row schema:
    {
      "messages": [
        {"role": "system", "content": "..."},
        {"role": "user", "content": "..."},
        {"role": "assistant", "content": "..."}
      ],
      "meta": {
        "task_name": "consulting.classify",
        "trace_id": "...",
        "instance_id": "...",
        "space_id": "...",
        "source": "real",
        "dataset_ver": "v2026-04-22",
        "quality": 0.87,
        "quality_source": "heuristic|llm|...",
        "source_path": "..."
      }
    }

Usage:
    python -m neural.training.recurate \\
        --raw-input training_data/raw/extracted/llm_interactions.jsonl \\
        --out training_data/curated/sft_interactions_v2/recurated.jsonl \\
        --dataset-ver v2026-04-22 \\
        --min-quality 0.6

The --expected-raw-sha256 flag defaults to the sprint-plan-pinned value
(Sprint FT-LORA-DATA, §1 Header). Override only to deliberately bypass the
pin.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from collections import Counter
from pathlib import Path
from typing import Any

# Sprint-plan-pinned SHA256 of training_data/raw/extracted/llm_interactions.jsonl
# (computed 2026-04-22 at Epic 1 Step 0; see docs/development/ft-lora/
# sprint_plan_ft_lora_data.md §1 Header).
SPRINT_PLAN_RAW_SHA_PIN = (
    "7caebf75fd59da37221acef887dc822ac9b80d04e19c19b750dd9a4e5eceb988"
)

# Reuse chat-format conversion from the existing module.
# format_converter.convert_record -> {"messages": [...]}
# format_converter.validate_mlx_format -> []  (empty = valid)
# Convention in neural/training/ is ``from training.xxx`` — see
# dataset_versioner.py and paradigm_router.py. When invoked as a script, add
# the neural/ dir to sys.path so the import resolves the same way.
_NEURAL_ROOT = Path(__file__).resolve().parents[1]
if str(_NEURAL_ROOT) not in sys.path:
    sys.path.insert(0, str(_NEURAL_ROOT))

from training.format_converter import convert_record, validate_mlx_format  # noqa: E402


PROVENANCE_FIELDS = (
    "task_name",
    "trace_id",
    "instance_id",
    "space_id",
    "quality",
    "quality_source",
    "source_path",
)


def compute_sha256(path: Path, chunk_size: int = 1 << 20) -> str:
    """Stream-compute SHA256 of a file."""
    h = hashlib.sha256()
    with open(path, "rb") as fin:
        while chunk := fin.read(chunk_size):
            h.update(chunk)
    return h.hexdigest()


def assert_raw_sha(path: Path, expected: str) -> None:
    """Raise SystemExit with a field-level message on SHA mismatch."""
    observed = compute_sha256(path)
    if observed == expected:
        return
    sys.stderr.write(
        "ERROR: raw dataset SHA drift — refusing to start.\n"
        f"  raw-input:  {path}\n"
        f"  expected:   {expected}\n"
        f"  observed:   {observed}\n"
        "This pin is recorded in Sprint FT-LORA-DATA §1 Header.\n"
        "If the raw input was deliberately replaced, re-pin the sprint plan\n"
        "via Epic 1 Step 0 and re-run.  Do NOT override lightly.\n"
    )
    raise SystemExit(2)


def build_meta(record: dict[str, Any], dataset_ver: str) -> dict[str, Any]:
    """Construct the `meta` sidecar for an output row."""
    meta: dict[str, Any] = {"source": "real", "dataset_ver": dataset_ver}
    for field in PROVENANCE_FIELDS:
        meta[field] = record.get(field)
    return meta


def passes_quality(record: dict[str, Any], min_quality: float) -> bool:
    """Quality gate.

    * When ``min_quality <= 0.0``: the gate is disabled — null quality passes.
    * When ``min_quality > 0.0``: null/missing/non-numeric quality drops the row.

    The current corpus (raw + existing filtered) has ``quality = null`` for
    every row (no quality-scoring pass has been run yet). Defaulting the CLI
    to 0.6 and leaving null→drop would zero the output; this split lets the
    operator pass ``--min-quality 0.0`` as a documented "no floor" override
    while preserving strict filtering once quality scoring lands.
    """
    q = record.get("quality")
    if q is None:
        return min_quality <= 0.0
    try:
        return float(q) >= min_quality
    except (TypeError, ValueError):
        return False


def passes_required(record: dict[str, Any]) -> bool:
    """Rows without task_name or trace_id are unusable downstream."""
    return bool(record.get("task_name")) and bool(record.get("trace_id"))


def run_recurate(
    raw_input: Path,
    out: Path,
    dataset_ver: str,
    min_quality: float,
    raft_ratio: float = 0.8,
) -> dict[str, Any]:
    """Run the re-curation pipeline.

    Returns a report dict with counts and per-task drop breakdowns.
    """
    out.parent.mkdir(parents=True, exist_ok=True)

    input_rows = 0
    kept_rows = 0
    dropped_quality = Counter()
    dropped_required = Counter()
    dropped_schema = Counter()
    dropped_json = 0
    kept_per_task: Counter[str] = Counter()

    with open(raw_input) as fin, open(out, "w") as fout:
        for line in fin:
            line = line.strip()
            if not line:
                continue
            input_rows += 1

            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                dropped_json += 1
                continue

            task = record.get("task_name") or "<unknown>"

            if not passes_required(record):
                dropped_required[task] += 1
                continue

            if not passes_quality(record, min_quality):
                dropped_quality[task] += 1
                continue

            converted = convert_record(record, raft_ratio)
            errors = validate_mlx_format(converted)
            if errors:
                dropped_schema[task] += 1
                continue

            converted["meta"] = build_meta(record, dataset_ver)
            fout.write(json.dumps(converted) + "\n")
            kept_rows += 1
            kept_per_task[task] += 1

    return {
        "input_rows": input_rows,
        "kept_rows": kept_rows,
        "dropped_json": dropped_json,
        "dropped_quality_per_task": dict(dropped_quality),
        "dropped_required_per_task": dict(dropped_required),
        "dropped_schema_per_task": dict(dropped_schema),
        "kept_per_task": dict(kept_per_task),
    }


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(
        description="Sprint FT-LORA-DATA Epic 1 — provenance-preserving re-curation.",
    )
    p.add_argument(
        "--raw-input", required=True, type=Path,
        help="Path to raw llm_interactions.jsonl.",
    )
    p.add_argument(
        "--out", required=True, type=Path,
        help="Output recurated.jsonl path.",
    )
    p.add_argument(
        "--expected-raw-sha256",
        default=SPRINT_PLAN_RAW_SHA_PIN,
        help=(
            "SHA256 pin for --raw-input. Defaults to the Sprint FT-LORA-DATA "
            "plan-pinned value. Override only when deliberately bypassing the pin."
        ),
    )
    p.add_argument(
        "--dataset-ver", required=True,
        help="Dataset version string to stamp into meta.dataset_ver "
             "(e.g., v2026-04-22).",
    )
    p.add_argument(
        "--min-quality", type=float, default=0.6,
        help="Quality floor.  Rows with quality < this (or missing) are dropped.",
    )
    p.add_argument(
        "--raft-ratio", type=float, default=0.8,
        help="Deterministic RAFT context inclusion ratio "
             "(same behavior as format_converter.py).",
    )
    args = p.parse_args(argv)

    if not args.raw_input.exists():
        sys.stderr.write(f"ERROR: --raw-input not found: {args.raw_input}\n")
        return 2

    assert_raw_sha(args.raw_input, args.expected_raw_sha256)

    report = run_recurate(
        raw_input=args.raw_input,
        out=args.out,
        dataset_ver=args.dataset_ver,
        min_quality=args.min_quality,
        raft_ratio=args.raft_ratio,
    )

    print(f"Input rows:       {report['input_rows']}")
    print(f"Kept rows:        {report['kept_rows']}")
    print(f"Dropped (json):   {report['dropped_json']}")
    print("Dropped (missing task_name/trace_id) per task:")
    for task, n in sorted(report["dropped_required_per_task"].items()):
        print(f"  {task}: {n}")
    print("Dropped (quality < floor) per task:")
    for task, n in sorted(report["dropped_quality_per_task"].items()):
        print(f"  {task}: {n}")
    print("Dropped (schema) per task:")
    for task, n in sorted(report["dropped_schema_per_task"].items()):
        print(f"  {task}: {n}")
    print("Kept per task:")
    for task, n in sorted(report["kept_per_task"].items(), key=lambda kv: (-kv[1], kv[0])):
        print(f"  {task}: {n}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
