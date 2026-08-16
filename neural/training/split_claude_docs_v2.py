#!/usr/bin/env python3
"""CLAUDE-DOCS-TRAINING-002 Epic 2 — v2 leak-safe split.

Preserves the V1 GOLDEN VERBATIM (`training_data/eval/claude_code_knowledge_golden.jsonl`)
so adapter_002 can be measured on the same 50-row fixture as adapter_001.

Reads:
- training_data/claude-docs/curated/qa_v2.jsonl (chunked corpus from Epic 1)
- training_data/eval/claude_code_knowledge_golden.jsonl (V1 golden, KEEP AS-IS)
- training_data/claude-docs/curated/split_manifest.json (V1 split provenance)

Produces:
- training_data/claude-docs/curated/train_v2.jsonl (training corpus for adapter_002)
- training_data/claude-docs/curated/split_v2_manifest.json (audit trail)

Leak-safe by construction:
- Read V1 golden rows; extract (source_url, section_index) tuples of the
  ORIGINAL (pre-chunking) rows those golden entries came from.
- Filter qa_v2.jsonl to EXCLUDE any row where (source_url, section_index)
  matches one of those tuples — even if that row is now a chunk of the
  original section, it's still leak by construction.
- Verify inline: 0 (source_url, section_index) overlap AND 0 prompt-string
  overlap between train_v2 and golden.

Same golden = direct A/B comparison of adapter_001 vs adapter_002 possible.

Usage:
    python3 neural/training/split_claude_docs_v2.py         # default
    python3 neural/training/split_claude_docs_v2.py --dry-run

Sprint: docs/development/claude-docs-training-002/
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent.parent
QA_V2_IN = REPO_ROOT / "training_data/claude-docs/curated/qa_v2.jsonl"
GOLDEN_V1 = REPO_ROOT / "training_data/eval/claude_code_knowledge_golden.jsonl"
TRAIN_V2_OUT = REPO_ROOT / "training_data/claude-docs/curated/train_v2.jsonl"
MANIFEST_OUT = REPO_ROOT / "training_data/claude-docs/curated/split_v2_manifest.json"


def sha256_hex(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--dry-run", action="store_true",
                    help="print stats but do not write outputs")
    args = ap.parse_args()

    if not QA_V2_IN.exists():
        print(f"error: {QA_V2_IN} not found. Run chunk_claude_docs.py first.", file=sys.stderr)
        return 2
    if not GOLDEN_V1.exists():
        print(f"error: {GOLDEN_V1} not found (V1 golden missing).", file=sys.stderr)
        return 2

    # Load V1 golden and extract exclusion tuples + prompt strings
    golden_rows: list[dict] = []
    with open(GOLDEN_V1, encoding="utf-8") as f:
        for line in f:
            golden_rows.append(json.loads(line))
    print(f"loaded V1 golden: {len(golden_rows)} rows from {GOLDEN_V1.relative_to(REPO_ROOT)}")

    exclusion_tuples: set[tuple[str, int]] = set()
    golden_prompts: set[str] = set()
    for g in golden_rows:
        # V1 golden has section_index from pre-chunking; that's the tuple we exclude in v2
        exclusion_tuples.add((g["source_url"], g["section_index"]))
        golden_prompts.add(g["prompt"])
    print(f"exclusion tuples (source_url, section_index): {len(exclusion_tuples)}")

    # Load qa_v2 and filter
    total = 0
    excluded = 0
    train_rows: list[dict] = []
    with open(QA_V2_IN, encoding="utf-8") as f:
        for line in f:
            total += 1
            r = json.loads(line)
            key = (r["source_url"], r["section_index"])
            if key in exclusion_tuples:
                excluded += 1
                continue
            train_rows.append(r)
    print(f"qa_v2 loaded: {total} rows; excluded {excluded} (chunks of V1 golden sections); train_v2: {len(train_rows)} rows")

    # LEAK AUDIT #1: no exclusion tuple should appear in train_v2
    train_keys = {(r["source_url"], r["section_index"]) for r in train_rows}
    overlap_tuples = train_keys & exclusion_tuples
    if overlap_tuples:
        print(f"error: LEAK — {len(overlap_tuples)} exclusion tuples still in train_v2", file=sys.stderr)
        return 1
    print("leak audit #1: 0 (source_url, section_index) overlap between train_v2 and V1 golden ✓")

    # LEAK AUDIT #2: no train_v2 prompt should equal a golden prompt string
    train_prompts = {r["prompt"] for r in train_rows}
    prompt_overlap = train_prompts & golden_prompts
    if prompt_overlap:
        print(f"error: LEAK — {len(prompt_overlap)} prompt-string overlap", file=sys.stderr)
        return 1
    print("leak audit #2: 0 prompt-string overlap between train_v2 and V1 golden ✓")

    if args.dry_run:
        print("[dry-run] not writing outputs")
        return 0

    # Write train_v2.jsonl
    TRAIN_V2_OUT.write_text("".join(json.dumps(r, ensure_ascii=False) + "\n" for r in train_rows), encoding="utf-8")
    train_sha = sha256_hex(TRAIN_V2_OUT.read_bytes())

    manifest = {
        "sprint": "CLAUDE-DOCS-TRAINING-002",
        "epic": "2 (leak-safe split preserving V1 golden)",
        "source_qa_v2": str(QA_V2_IN.relative_to(REPO_ROOT)),
        "source_qa_v2_sha256": sha256_hex(QA_V2_IN.read_bytes()),
        "v1_golden_path": str(GOLDEN_V1.relative_to(REPO_ROOT)),
        "v1_golden_sha256": sha256_hex(GOLDEN_V1.read_bytes()),
        "v1_golden_row_count": len(golden_rows),
        "train_v2_path": str(TRAIN_V2_OUT.relative_to(REPO_ROOT)),
        "train_v2_row_count": len(train_rows),
        "train_v2_sha256": train_sha,
        "qa_v2_input_rows": total,
        "excluded_rows": excluded,
        "leak_audit": {
            "source_section_tuple_overlap": 0,
            "prompt_string_overlap": 0,
            "verdict": "PASS",
        },
        "comparison_note": (
            "V1 golden preserved verbatim (same file, same SHA). Enables direct A/B comparison "
            "of adapter_001 vs adapter_002 on identical fixture set. Any factuality improvement "
            "in adapter_002 is attributable to (a) chunking eliminating truncation loss on 188 "
            "over-sized sections + (b) higher epoch count."
        ),
        "generated_at_utc": datetime.now(timezone.utc).isoformat(timespec="seconds"),
    }
    MANIFEST_OUT.write_text(json.dumps(manifest, indent=2), encoding="utf-8")

    print()
    print("wrote:")
    print(f"  {TRAIN_V2_OUT.relative_to(REPO_ROOT)}  ({TRAIN_V2_OUT.stat().st_size / 1024:.1f} KB, sha={train_sha[:16]}...)")
    print(f"  {MANIFEST_OUT.relative_to(REPO_ROOT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
