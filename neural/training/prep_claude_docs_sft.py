#!/usr/bin/env python3
"""CLAUDE-DOCS-TRAINING-001 Epic 5 prep — reshape train.jsonl → SFT dataset dir.

Reads training_data/claude-docs/curated/train.jsonl (Epic 4 leak-safe split)
and produces training_data/sft/claude_code_knowledge/ with:
- train.jsonl — 95% of rows in mlx_lm.lora {messages, meta} format
- valid.jsonl — 5% for early-stop val_loss tracking
- manifest.json — FT-LORA-DATA schema (row_counts, file_sha256,
  raw_dataset_sha_pin, trained_against_model_sha)

Format contract:
- Each row: {"messages": [{system}, {user: prompt}, {assistant: completion}],
             "meta": {task_name, sampling_group, source, source_url, ...}}
- Matches the shape train_ft.py + valid_clean.jsonl expect (verified 2026-08-14).
- Base-model SHA pinned to Phase-5 baseline mlx-community/Qwen3-14B-4bit
  per docs/tests/ubench/specs/mdemg.ubench.json.

Deterministic 95/5 split via SHA-order (no rng seed drift), so reruns produce
byte-identical train/valid files from the same source qa.jsonl.

Usage:
    python3 neural/training/prep_claude_docs_sft.py         # default
    python3 neural/training/prep_claude_docs_sft.py --valid-frac 0.10
    python3 neural/training/prep_claude_docs_sft.py --dry-run

Sprint: docs/development/claude-docs-training-001/
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent.parent
TRAIN_IN = REPO_ROOT / "training_data/claude-docs/curated/train.jsonl"
QA_IN = REPO_ROOT / "training_data/claude-docs/curated/qa.jsonl"
SPLIT_MANIFEST = REPO_ROOT / "training_data/claude-docs/curated/split_manifest.json"
OUT_DIR = REPO_ROOT / "training_data/sft/claude_code_knowledge"

# Same system prompt as the ULTS spec — must match verbatim for the LoRA to
# learn under the same context the eval will use.
SYSTEM_PROMPT = (
    "You are an expert on Claude Code (Anthropic's agentic CLI) and its Agent SDK. "
    "Answer the user's question accurately based on the official Anthropic documentation at code.claude.com. "
    "Be concise, cite specific configuration keys, commands, hook events, or SDK classes by exact name, "
    "and include verbatim code snippets when they clarify the answer. "
    "If a claim would require guessing beyond documented behavior, say so."
)

# Phase-5 baseline base model SHA (per docs/tests/ubench/specs/mdemg.ubench.json).
BASE_MODEL_SHA = "a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5"
BASE_MODEL_NAME = "mlx-community/Qwen3-14B-4bit"


def reshape(row: dict) -> dict:
    return {
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": row["prompt"]},
            {"role": "assistant", "content": row["completion"]},
        ],
        "meta": {
            "task_name": "claude.code_knowledge",
            "sampling_group": "T",
            "source": "code.claude.com",
            "source_url": row["source_url"],
            "source_sha256": row["source_sha256"],
            "source_slug": row["source_slug"],
            "doc_title": row["doc_title"],
            "section_header": row["section_header"],
            "concept_type": row["concept_type"],
            "row_id": row["row_id"],
            "word_count": row["word_count"],
            "extracted_by": "curate_claude_docs.py + prep_claude_docs_sft.py (CLAUDE-DOCS-TRAINING-001)",
        },
    }


def sha256_hex(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--valid-frac", type=float, default=0.05,
                    help="fraction for valid.jsonl (default 0.05)")
    ap.add_argument("--dry-run", action="store_true",
                    help="print stats but do not write outputs")
    args = ap.parse_args()

    if not TRAIN_IN.exists():
        print(f"error: {TRAIN_IN} not found. Run split_claude_docs.py first.", file=sys.stderr)
        return 2

    # Load
    rows: list[dict] = []
    with open(TRAIN_IN, encoding="utf-8") as f:
        for line in f:
            rows.append(json.loads(line))
    print(f"loaded {len(rows)} rows from {TRAIN_IN.relative_to(REPO_ROOT)}")

    # Deterministic split via SHA-order (stable across reruns)
    rows_sorted = sorted(rows, key=lambda r: hashlib.sha256(r["row_id"].encode()).hexdigest())
    n_valid = max(1, int(len(rows_sorted) * args.valid_frac))
    valid_rows = rows_sorted[:n_valid]
    train_rows = rows_sorted[n_valid:]
    print(f"split (SHA-order): {len(train_rows)} train + {len(valid_rows)} valid ({args.valid_frac*100:.1f}%)")

    if args.dry_run:
        print("[dry-run] not writing outputs")
        return 0

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    train_path = OUT_DIR / "train.jsonl"
    valid_path = OUT_DIR / "valid.jsonl"
    manifest_path = OUT_DIR / "manifest.json"

    with open(train_path, "w", encoding="utf-8") as f:
        for r in train_rows:
            f.write(json.dumps(reshape(r), ensure_ascii=False) + "\n")
    with open(valid_path, "w", encoding="utf-8") as f:
        for r in valid_rows:
            f.write(json.dumps(reshape(r), ensure_ascii=False) + "\n")

    train_sha = sha256_hex(train_path.read_bytes())
    valid_sha = sha256_hex(valid_path.read_bytes())

    # raw_dataset_sha_pin: SHA of the source qa.jsonl (the pre-split corpus)
    raw_sha = sha256_hex(QA_IN.read_bytes()) if QA_IN.exists() else ""

    manifest = {
        "sprint": "CLAUDE-DOCS-TRAINING-001",
        "epic": "5 (LoRA training prep — Tier 1)",
        "family_name": "claude_code_knowledge",
        "base_dataset_ver": "claude_docs_v1",
        "generator_sha": "0c3cbe28_sprint_head",
        "meta_placement": "embedded",
        "row_counts": {
            "train": len(train_rows),
            "valid": len(valid_rows),
            "total": len(rows),
        },
        "file_sha256": {
            "train.jsonl": train_sha,
            "valid.jsonl": valid_sha,
        },
        "raw_dataset_sha_pin": raw_sha,
        "trained_against_model_sha": BASE_MODEL_SHA,
        "base_model_name": BASE_MODEL_NAME,
        "per_task_counts": {
            "claude.code_knowledge": {
                "total": len(rows),
                "train": len(train_rows),
                "valid": len(valid_rows),
            }
        },
        "duplication_factors": {"claude.code_knowledge": 1.0},
        "source_split_manifest": str(SPLIT_MANIFEST.relative_to(REPO_ROOT)) if SPLIT_MANIFEST.exists() else None,
        "generated_at_utc": datetime.now(timezone.utc).isoformat(timespec="seconds"),
    }
    manifest_path.write_text(json.dumps(manifest, indent=2), encoding="utf-8")

    print()
    print(f"wrote:")
    print(f"  {train_path.relative_to(REPO_ROOT)}  ({train_path.stat().st_size / 1024:.1f} KB, sha={train_sha[:16]}...)")
    print(f"  {valid_path.relative_to(REPO_ROOT)}  ({valid_path.stat().st_size / 1024:.1f} KB, sha={valid_sha[:16]}...)")
    print(f"  {manifest_path.relative_to(REPO_ROOT)}")
    print()
    print(f"ready to train:")
    print(f"  python3 neural/training/train_ft.py \\")
    print(f"    --tier 1 --mode sft \\")
    print(f"    --base-model {BASE_MODEL_NAME} \\")
    print(f"    --expected-sha256 {BASE_MODEL_SHA} \\")
    print(f"    --dataset {OUT_DIR.relative_to(REPO_ROOT)} \\")
    print(f"    --adapter-path adapters/claude_docs_001/ \\")
    print(f"    --n-epochs 1 --batch-size 4 \\")
    print(f"    --dry-run   # verify config first")

    return 0


if __name__ == "__main__":
    sys.exit(main())
