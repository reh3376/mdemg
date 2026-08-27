#!/usr/bin/env python3
"""MDEMG-USAGE-CORPUS-CURATE-001 Epic 5 — manifest + distribution report.

Emits `manifest.json` in the PHASE-E2 canonical shape + a human-readable
`distribution_report.txt`. Halts (exit 1) if leak_audit.json shows clean=false.

Usage:
    python3 scripts/mdemg_usage_manifest.py \\
        --dir training_data/sft/mdemg_usage_v1

Sprint: docs/development/mdemg-usage-corpus-curate-001/
"""
from __future__ import annotations

import argparse
import datetime
import hashlib
import json
import sys
from collections import Counter
from pathlib import Path


def sha256_file(p: Path) -> str:
    h = hashlib.sha256()
    with p.open("rb") as fh:
        for chunk in iter(lambda: fh.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.strip().splitlines()[0])
    ap.add_argument("--dir", type=Path, default=Path("training_data/sft/mdemg_usage_v1"))
    ap.add_argument("--source-node-count", type=int, default=1583,
                    help="Total ingested doc nodes from Epic 1 export.")
    args = ap.parse_args()

    train = args.dir / "train.jsonl"
    leak = args.dir / "leak_audit.json"

    for p in (train, leak):
        if not p.exists():
            print(f"ERROR: not found: {p}", file=sys.stderr)
            return 2

    leak_data = json.loads(leak.read_text())
    if not leak_data.get("clean"):
        print("ERROR: leak_audit.json reports clean=false; refusing to emit manifest.", file=sys.stderr)
        return 1

    rows = [json.loads(line) for line in train.open(encoding="utf-8") if line.strip()]
    n = len(rows)
    per_surface: Counter[str] = Counter()
    per_path_prefix: Counter[str] = Counter()
    word_hist: Counter[str] = Counter()
    for r in rows:
        m = r.get("meta") or {}
        per_surface[m.get("source_surface") or "unknown"] += 1
        # top-3 path segments as prefix bucket
        p = (m.get("source_path") or "").lstrip("/")
        prefix = "/".join(p.split("/")[:3])
        per_path_prefix[prefix] += 1
        wc = m.get("word_count") or 0
        bucket = "0-49" if wc < 50 else "50-99" if wc < 100 else "100-249" if wc < 250 else "250-499" if wc < 500 else "500+"
        word_hist[bucket] += 1

    file_sha = sha256_file(train)

    manifest = {
        "sprint": "MDEMG-USAGE-CORPUS-CURATE-001",
        "epic": "1-5 (export + curate + leak-audit + manifest)",
        "family_name": "mdemg_usage_v1",
        "meta_placement": "embedded",
        "row_counts": {"train": n, "total": n},
        "file_sha256": {"train.jsonl": file_sha},
        "per_task_counts": {"mdemg.usage": {"total": n, "train": n}},
        "per_surface_counts": dict(per_surface),
        "per_path_prefix_top": dict(per_path_prefix.most_common(20)),
        "word_count_histogram": dict(word_hist),
        "source_substrate": "mdemg-dev",
        "source_ingest_sprint": "MDEMG-DOCS-INGEST-001",
        "source_node_count": args.source_node_count,
        "curation_method": "deterministic_h2_h3",
        "curation_min_words": 30,
        "leak_audit": {
            "threshold": leak_data.get("threshold"),
            "candidate_rows": leak_data.get("candidate_rows"),
            "eval_rows": leak_data.get("eval_rows"),
            "clean": leak_data.get("clean"),
            "audited_against": "training_data/eval/valid_clean.jsonl",
        },
        "trained_against_model_sha": None,
        "base_model_name": None,
        "generated_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
    }

    manifest_path = args.dir / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(f"Wrote: {manifest_path}")

    # Distribution report
    lines = [
        "MDEMG-USAGE-CORPUS-CURATE-001 — Distribution Report",
        f"Generated: {manifest['generated_at_utc']}",
        f"Family: {manifest['family_name']}",
        "",
        f"Total rows: {n}",
        f"train.jsonl SHA-256: {file_sha}",
        "",
        "Per-surface distribution:",
    ]
    for s, c in per_surface.most_common():
        pct = 100.0 * c / n if n else 0.0
        lines.append(f"  {s:<12} {c:>5}  ({pct:5.1f}%)")
    lines.extend(["", "Top 20 path prefixes:"])
    for pfx, c in per_path_prefix.most_common(20):
        lines.append(f"  {pfx:<45} {c:>5}")
    lines.extend(["", "Word-count histogram (per-row answer length):"])
    for bucket in ("0-49", "50-99", "100-249", "250-499", "500+"):
        lines.append(f"  {bucket:<10} {word_hist.get(bucket, 0):>5}")

    dist_path = args.dir / "distribution_report.txt"
    dist_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"Wrote: {dist_path}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
