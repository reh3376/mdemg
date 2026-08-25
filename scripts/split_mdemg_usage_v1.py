#!/usr/bin/env python3
"""MDEMG-USAGE-LORA-001 Epic 1a — deterministic 80/10/10 split of mdemg_usage_v1.

Splits `training_data/sft/mdemg_usage_v1/train.jsonl` into train / valid /
benchmark_holdout partitions using `sha256(row_id)[:8] mod 10`:
    - buckets 0-7 → train (80%)
    - bucket   8  → valid (10%)
    - bucket   9  → benchmark_holdout (10%)

Deterministic: re-runs produce identical partitions (rows-to-buckets is a pure
function of row_id). The original `train.jsonl` is PRESERVED — the split
outputs are additive files alongside it.

Usage:
    python3 scripts/split_mdemg_usage_v1.py \\
        --in training_data/sft/mdemg_usage_v1/train.jsonl \\
        --out-dir training_data/sft/mdemg_usage_v1/
    python3 scripts/split_mdemg_usage_v1.py --dry-run

Sprint: docs/development/mdemg-usage-lora-001/
"""
from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path


def bucket_for(row_id: str) -> int:
    """Deterministic bucket 0-9 from row_id via SHA-256."""
    if not row_id:
        return 0
    h = hashlib.sha256(row_id.encode("utf-8")).hexdigest()
    return int(h[:8], 16) % 10


def split(in_path: Path, out_dir: Path, dry_run: bool) -> tuple[int, int, int]:
    train_rows: list[str] = []
    valid_rows: list[str] = []
    holdout_rows: list[str] = []
    missing_row_id = 0

    with in_path.open(encoding="utf-8") as fh:
        for line in fh:
            s = line.strip()
            if not s:
                continue
            r = json.loads(s)
            meta = r.get("meta") or {}
            rid = meta.get("row_id") or ""
            if not rid:
                missing_row_id += 1
                # Fall back to full-line hash so we don't silently drop
                rid = hashlib.sha256(s.encode()).hexdigest()
            b = bucket_for(rid)
            if b < 8:
                train_rows.append(line)  # keep original line (byte-verbatim)
            elif b == 8:
                valid_rows.append(line)
            else:
                holdout_rows.append(line)

    tn, vn, hn = len(train_rows), len(valid_rows), len(holdout_rows)
    total = tn + vn + hn
    print(f"Split: train={tn} valid={vn} holdout={hn} total={total}")
    print(
        f"       (target 80/10/10 → {int(total*0.8)}/{int(total*0.1)}/{int(total*0.1)})"
    )
    if missing_row_id:
        print(f"  WARN: {missing_row_id} rows had no meta.row_id; hashed full line for bucketing")

    if not dry_run:
        out_dir.mkdir(parents=True, exist_ok=True)
        (out_dir / "train_split.jsonl").write_text("".join(train_rows), encoding="utf-8")
        (out_dir / "valid_split.jsonl").write_text("".join(valid_rows), encoding="utf-8")
        (out_dir / "benchmark_holdout.jsonl").write_text("".join(holdout_rows), encoding="utf-8")
        for f in ("train_split.jsonl", "valid_split.jsonl", "benchmark_holdout.jsonl"):
            print(f"Wrote: {out_dir / f}")
    return tn, vn, hn


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.strip().splitlines()[0])
    ap.add_argument("--in", dest="in_path", type=Path,
                    default=Path("training_data/sft/mdemg_usage_v1/train.jsonl"))
    ap.add_argument("--out-dir", type=Path,
                    default=Path("training_data/sft/mdemg_usage_v1"))
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    if not args.in_path.exists():
        print(f"ERROR: not found: {args.in_path}", file=sys.stderr)
        return 2

    split(args.in_path, args.out_dir, args.dry_run)
    return 0


if __name__ == "__main__":
    sys.exit(main())
