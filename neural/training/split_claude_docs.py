#!/usr/bin/env python3
"""CLAUDE-DOCS-TRAINING-001 Epic 4 — leak-safe split of curated Q&A corpus.

Reads training_data/claude-docs/curated/qa.jsonl (from Epic 3), splits into:
- training_data/claude-docs/curated/train.jsonl — training corpus (LoRA input)
- training_data/eval/claude_code_knowledge_golden.jsonl — golden holdout (ULTS)

Leak-safe by construction:
- Split on the (source_url, section_index) tuple. A row lands in EITHER
  train OR golden, never both.
- On the golden side, the Q is REPHRASED via an alternative template
  distinct from the one used during Epic 3 curation, so simple string
  memorization cannot pass the eval.
- The A (completion) is kept verbatim so evaluators can score semantic
  recall + response fidelity against the source-of-truth doc content.

Sampling strategy:
- Stratified across the top N source URLs (default 20) to ensure the
  golden holdout represents concept diversity across Claude Code surface
  areas (CLI reference, hooks, MCP, Agent SDK, plugins, etc).
- --golden-size (default 50) rows sampled stratified.
- Deterministic: uses SHA-based selection (no rng seed drift), so re-runs
  produce byte-identical splits from the same qa.jsonl.

Leak audit runs inline: after split, verify 0 (source_url, section_index)
overlap between the two files. Exit non-zero on any overlap detected.

Usage:
    python3 neural/training/split_claude_docs.py                # default 50-row golden
    python3 neural/training/split_claude_docs.py --golden-size 100
    python3 neural/training/split_claude_docs.py --top-sources 30
    python3 neural/training/split_claude_docs.py --dry-run

Sprint: docs/development/claude-docs-training-001/
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent.parent
QA_PATH = REPO_ROOT / "training_data/claude-docs/curated/qa.jsonl"
TRAIN_PATH = REPO_ROOT / "training_data/claude-docs/curated/train.jsonl"
GOLDEN_PATH = REPO_ROOT / "training_data/eval/claude_code_knowledge_golden.jsonl"
SPLIT_MANIFEST_PATH = REPO_ROOT / "training_data/claude-docs/curated/split_manifest.json"


def rephrase_question(row: dict) -> str:
    """Alternative Q template distinct from Epic 3's rendering.

    Epic 3 uses templates like "In Claude Code, what does '{H}' mean in the
    context of {D}?". Here we use "Explain the concept of '{H}' in Claude
    Code's {D} documentation." Different lexical surface, same semantic
    intent. Model that memorized Epic 3 phrasing can't string-match.

    A future round can layer LLM-based paraphrase; for the first shipped
    golden this mechanical rewrite gives an honest strength-of-recall
    signal.
    """
    header = row["section_header"].strip().rstrip(".")
    doc = row["doc_title"]

    if header.endswith("?"):
        # Already a question; add prefix.
        return f"According to the Claude Code documentation for '{doc}': {header}"

    # Rotate template based on concept_type + header shape.
    if row["concept_type"] == "h2":
        return f"Explain '{header}' from the Claude Code documentation on {doc}."
    else:  # h3
        return f"Within '{doc}' in the Claude Code docs, describe what '{header}' covers."


def stratified_select(rows: list[dict], golden_size: int, top_sources: int) -> set[str]:
    """Return set of row_ids selected for the golden holdout.

    Deterministic stratified sampling: for each of the top N source URLs by
    row count, take proportional share (round-robin fill) so no single
    doc dominates. Selection within a source is by SHA-order (stable across
    reruns).
    """
    # Group rows by source_url
    by_source: dict[str, list[dict]] = defaultdict(list)
    for r in rows:
        by_source[r["source_url"]].append(r)

    # Rank sources by row count, take top N
    ranked = sorted(by_source.items(), key=lambda kv: -len(kv[1]))[:top_sources]
    total_across_top = sum(len(rs) for _, rs in ranked)

    # Allocate golden slots proportional to source share (min 1 per source)
    quotas: dict[str, int] = {}
    remaining = golden_size
    for url, rs in ranked:
        q = max(1, round(golden_size * len(rs) / total_across_top))
        quotas[url] = q
        remaining -= q

    # If we over/under-allocated, adjust largest source
    if remaining != 0 and ranked:
        largest_url = ranked[0][0]
        quotas[largest_url] = max(1, quotas[largest_url] + remaining)

    selected: set[str] = set()
    for url, rs in ranked:
        # Sort rows deterministically by SHA of row_id for stable selection
        sorted_rows = sorted(rs, key=lambda r: hashlib.sha256(r["row_id"].encode()).hexdigest())
        take = min(quotas.get(url, 0), len(sorted_rows))
        for r in sorted_rows[:take]:
            selected.add(r["row_id"])

    return selected


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--golden-size", type=int, default=50,
                    help="target row count for golden holdout (default: 50)")
    ap.add_argument("--top-sources", type=int, default=20,
                    help="stratify golden across top N source URLs (default: 20)")
    ap.add_argument("--dry-run", action="store_true",
                    help="print stats but do not write outputs")
    args = ap.parse_args()

    if not QA_PATH.exists():
        print(f"error: qa.jsonl not found at {QA_PATH}. Run curate_claude_docs.py first.", file=sys.stderr)
        return 2

    # Load
    rows: list[dict] = []
    with open(QA_PATH, encoding="utf-8") as f:
        for line in f:
            rows.append(json.loads(line))

    print(f"loaded {len(rows)} rows from {QA_PATH.relative_to(REPO_ROOT)}")

    # Stratified selection
    golden_ids = stratified_select(rows, args.golden_size, args.top_sources)
    print(f"selected {len(golden_ids)} rows for golden holdout (stratified across top {args.top_sources} sources)")

    # Split
    train_rows: list[dict] = []
    golden_rows: list[dict] = []
    for r in rows:
        if r["row_id"] in golden_ids:
            # Golden: paraphrase Q; keep A verbatim; add source-of-truth marker
            g = dict(r)  # copy
            g["prompt"] = rephrase_question(r)
            g["original_prompt"] = r["prompt"]  # for reference
            g["split"] = "golden"
            golden_rows.append(g)
        else:
            t = dict(r)
            t["split"] = "train"
            train_rows.append(t)

    print(f"split: {len(train_rows)} training rows, {len(golden_rows)} golden rows")

    # LEAK AUDIT: verify (source_url, section_index) tuples disjoint
    train_keys = {(r["source_url"], r["section_index"]) for r in train_rows}
    golden_keys = {(r["source_url"], r["section_index"]) for r in golden_rows}
    overlap = train_keys & golden_keys
    if overlap:
        print(f"error: LEAK DETECTED — {len(overlap)} (source_url, section_index) tuples appear in BOTH train and golden:", file=sys.stderr)
        for k in list(overlap)[:5]:
            print(f"  {k}", file=sys.stderr)
        return 1
    print("leak audit: 0 (source_url, section_index) overlap between train and golden ✓")

    # SECOND LEAK CHECK: verify golden Qs differ from any train Q
    train_prompts = {r["prompt"] for r in train_rows}
    golden_q_overlap = [g for g in golden_rows if g["prompt"] in train_prompts]
    if golden_q_overlap:
        print(f"error: LEAK DETECTED — {len(golden_q_overlap)} golden prompts appear in train prompts (rephrase failed to differentiate)", file=sys.stderr)
        return 1
    print("leak audit: golden prompts all distinct from train prompts ✓")

    if args.dry_run:
        print("\n[dry-run] not writing outputs")
        return 0

    # Ensure dirs
    TRAIN_PATH.parent.mkdir(parents=True, exist_ok=True)
    GOLDEN_PATH.parent.mkdir(parents=True, exist_ok=True)

    # Write
    with open(TRAIN_PATH, "w", encoding="utf-8") as f:
        for r in train_rows:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")
    with open(GOLDEN_PATH, "w", encoding="utf-8") as f:
        for r in golden_rows:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")

    # Compute SHAs for UBENCH pin
    train_sha = hashlib.sha256(TRAIN_PATH.read_bytes()).hexdigest()
    golden_sha = hashlib.sha256(GOLDEN_PATH.read_bytes()).hexdigest()

    manifest = {
        "sprint": "CLAUDE-DOCS-TRAINING-001",
        "epic": "4 (leak-safe train/golden split)",
        "source_qa": str(QA_PATH.relative_to(REPO_ROOT)),
        "train_path": str(TRAIN_PATH.relative_to(REPO_ROOT)),
        "train_row_count": len(train_rows),
        "train_sha256": train_sha,
        "golden_path": str(GOLDEN_PATH.relative_to(REPO_ROOT)),
        "golden_row_count": len(golden_rows),
        "golden_sha256": golden_sha,
        "leak_audit": {
            "source_section_tuple_overlap": 0,
            "prompt_string_overlap": 0,
            "verdict": "PASS",
        },
        "sampling": {
            "strategy": "stratified across top source URLs, deterministic SHA-order within source",
            "top_sources_pool": args.top_sources,
            "target_golden_size": args.golden_size,
            "actual_golden_size": len(golden_rows),
        },
        "generated_at_utc": datetime.now(timezone.utc).isoformat(timespec="seconds"),
    }
    SPLIT_MANIFEST_PATH.write_text(json.dumps(manifest, indent=2), encoding="utf-8")

    # Report golden distribution
    print()
    print("golden holdout distribution (top source URLs):")
    src_counts: dict[str, int] = defaultdict(int)
    for g in golden_rows:
        src_counts[g["source_url"]] += 1
    for url, n in sorted(src_counts.items(), key=lambda kv: -kv[1])[:15]:
        print(f"  {n:3d} rows — {url}")

    print()
    print("wrote:")
    print(f"  {TRAIN_PATH.relative_to(REPO_ROOT)}  ({TRAIN_PATH.stat().st_size / 1024:.1f} KB, sha256={train_sha[:16]}...)")
    print(f"  {GOLDEN_PATH.relative_to(REPO_ROOT)}  ({GOLDEN_PATH.stat().st_size / 1024:.1f} KB, sha256={golden_sha[:16]}...)")
    print(f"  {SPLIT_MANIFEST_PATH.relative_to(REPO_ROOT)}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
