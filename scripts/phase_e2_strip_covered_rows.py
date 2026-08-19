#!/usr/bin/env python3
"""
PHASE-E2-CORPUS-CURATION-001 — strip PROVEN_COVERAGE rows from the v2
FT corpus using PHASE-E1's rows_to_strip.jsonl.

Produces:
  - <out-dir>/train.jsonl (SUBSTRATE_MISS + AUDIT_ERROR rows preserved verbatim)
  - <out-dir>/manifest.json (source SHA + output SHA + strip provenance)

Contracts:
  - v2 files are READ-ONLY (never mutated); pre + post SHA256 checked.
  - Output is deterministic: same v2 + same strip-list → byte-identical output.
  - Every kept row's messages + meta preserved verbatim (no re-normalization).
"""

import argparse
import hashlib
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path


def sha256_of_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            h.update(chunk)
    return h.hexdigest()


def load_strip_set(path: Path) -> set[int]:
    """Read rows_to_strip.jsonl, extract every row_idx into a set."""
    out: set[int] = set()
    with path.open() as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            obj = json.loads(line)
            idx = obj.get("row_idx")
            if isinstance(idx, int) and idx >= 0:
                out.add(idx)
    return out


def strip_corpus(source_path: Path, strip_ids: set[int], out_path: Path) -> tuple[int, int, int]:
    """Read source line-by-line; write rows whose row_idx is NOT in strip_ids.
    Returns (input_count, kept_count, stripped_count).
    """
    out_path.parent.mkdir(parents=True, exist_ok=True)
    input_count = 0
    kept_count = 0
    with source_path.open("rb") as src, out_path.open("wb") as dst:
        for row_idx, raw in enumerate(src):
            input_count += 1
            if row_idx in strip_ids:
                continue
            # Preserve the raw line byte-for-byte (no re-encode)
            dst.write(raw)
            kept_count += 1
    return input_count, kept_count, input_count - kept_count


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--source-corpus", default="training_data/sft/claude_code_knowledge_v2/train.jsonl")
    p.add_argument("--strip-list", default="docs/development/phase-e1-corpus-audit-001/rows_to_strip.jsonl")
    p.add_argument("--out-dir", default="training_data/sft/claude_code_knowledge_v3_stripped")
    p.add_argument("--dry-run", action="store_true", help="Report counts + SHAs, do not write output")
    args = p.parse_args()

    source_path = Path(args.source_corpus)
    strip_list_path = Path(args.strip_list)
    out_dir = Path(args.out_dir)
    out_train_path = out_dir / "train.jsonl"
    out_manifest_path = out_dir / "manifest.json"

    if not source_path.exists():
        print(f"[e2] ERROR: source corpus not found: {source_path}", file=sys.stderr)
        return 2
    if not strip_list_path.exists():
        print(f"[e2] ERROR: strip-list not found: {strip_list_path}", file=sys.stderr)
        return 2

    source_sha_before = sha256_of_file(source_path)
    strip_sha = sha256_of_file(strip_list_path)
    strip_ids = load_strip_set(strip_list_path)

    print(f"[e2] source: {source_path}")
    print(f"[e2] source sha256: {source_sha_before}")
    print(f"[e2] strip-list: {strip_list_path}  strip_count={len(strip_ids)}  strip_sha={strip_sha}")
    print(f"[e2] out-dir:  {out_dir}")

    if args.dry_run:
        # Count without writing
        input_count = 0
        stripped = 0
        with source_path.open("rb") as f:
            for row_idx, _ in enumerate(f):
                input_count += 1
                if row_idx in strip_ids:
                    stripped += 1
        kept = input_count - stripped
        print(f"[e2] dry-run: input={input_count} would_strip={stripped} would_keep={kept}")
        return 0

    input_count, kept_count, stripped_count = strip_corpus(source_path, strip_ids, out_train_path)
    print(f"[e2] wrote {out_train_path}  input={input_count} kept={kept_count} stripped={stripped_count}")

    # Verify source unchanged
    source_sha_after = sha256_of_file(source_path)
    if source_sha_before != source_sha_after:
        print(f"[e2] ERROR: source corpus SHA changed during run — refusing to write manifest.", file=sys.stderr)
        print(f"[e2]   before: {source_sha_before}", file=sys.stderr)
        print(f"[e2]   after:  {source_sha_after}", file=sys.stderr)
        return 3

    # Manifest
    out_train_sha = sha256_of_file(out_train_path)
    manifest = {
        "sprint": "PHASE-E2-CORPUS-CURATION-001",
        "epic": "1-2 (strip execution + manifest generation)",
        "family_name": "claude_code_knowledge_v3_stripped",
        "base_dataset_ver": "claude_docs_v2_stripped_via_e1_audit",
        "meta_placement": "embedded",
        "row_counts": {
            "train": kept_count,
            "total": kept_count,
        },
        "file_sha256": {
            "train.jsonl": out_train_sha,
        },
        "per_task_counts": {
            "claude.code_knowledge": {
                "total": kept_count,
                "train": kept_count,
            },
        },
        "source_v2_row_count": input_count,
        "stripped_row_count": stripped_count,
        "source_v2_sha256": source_sha_before,
        "strip_provenance": {
            "sprint_id": "PHASE-E1-CORPUS-AUDIT-001",
            "strip_list_sha256": strip_sha,
            "audit_date": "2026-08-19",
            "audit_threshold": 0.30,
            "audit_method": "asymmetric 3-gram overlap (answer→retrieved content)",
            "audit_substrate": "mdemg-dev",
            "audit_endpoint": "/v1/memory/retrieve include_content=true",
        },
        "preserves": [
            "training_data/sft/claude_code_knowledge/  (v1, 2141 rows, superseded)",
            "training_data/sft/claude_code_knowledge_v2/  (v2, 2848 rows, source of strip)",
        ],
        "trained_against_model_sha": None,
        "base_model_name": "mlx-community/Qwen3-14B-4bit",
        "generated_at_utc": datetime.now(tz=timezone.utc).replace(microsecond=0).isoformat(),
    }
    with out_manifest_path.open("w") as f:
        json.dump(manifest, f, indent=2)
        f.write("\n")
    print(f"[e2] wrote {out_manifest_path}")

    # Print summary
    print()
    print("=== Phase E2 strip summary ===")
    print(f"  input rows (v2/train.jsonl): {input_count}")
    print(f"  PROVEN_COVERAGE stripped:    {stripped_count}")
    print(f"  SUBSTRATE_MISS kept:         {kept_count}  → {out_train_path.name}")
    print(f"  output SHA256:               {out_train_sha}")
    print(f"  source SHA256 verified unchanged.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
