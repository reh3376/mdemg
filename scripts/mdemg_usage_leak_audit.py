#!/usr/bin/env python3
"""MDEMG-USAGE-CORPUS-CURATE-001 Epic 4 — leak audit.

Verify zero row-level overlap between the newly-curated `mdemg_usage_v1/train.jsonl`
and the held-out eval `training_data/eval/valid_clean.jsonl`.

Same shape as PHASE-E1-CORPUS-AUDIT-001 + PHASE-E2 leak_audit (task #132/#133):
    - asymmetric 3-gram overlap: candidate.assistant → eval.assistant
    - overlap >= threshold (default 0.30) = PROVEN_OVERLAP → hard fail

Reports:
    - `training_data/sft/mdemg_usage_v1/leak_audit.json`:
        {"threshold": 0.30, "candidate_rows": N, "eval_rows": M,
         "overlap_hits": [{...}], "clean": bool}

Hard gate: any PROVEN_OVERLAP → exit code 1 → downstream (manifest, PR) must halt.

Usage:
    python3 scripts/mdemg_usage_leak_audit.py \\
        --candidate training_data/sft/mdemg_usage_v1/train.jsonl \\
        --eval training_data/eval/valid_clean.jsonl \\
        --out training_data/sft/mdemg_usage_v1/leak_audit.json

Sprint: docs/development/mdemg-usage-corpus-curate-001/
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path


def load_jsonl(p: Path) -> list[dict]:
    return [json.loads(line) for line in p.open(encoding="utf-8") if line.strip()]


def extract_assistant(row: dict) -> str:
    """Get the assistant reply string from either {messages:[...]} shape."""
    msgs = row.get("messages") or []
    for m in reversed(msgs):
        if m.get("role") == "assistant":
            return m.get("content") or ""
    return ""


_WORD_RE = re.compile(r"[A-Za-z0-9_]+")


def tri_grams(s: str) -> set[tuple[str, str, str]]:
    """Word-level 3-grams. Lowercased. Punctuation stripped."""
    words = _WORD_RE.findall(s.lower())
    if len(words) < 3:
        return set()
    return {tuple(words[i : i + 3]) for i in range(len(words) - 2)}


def overlap(candidate_txt: str, eval_txt: str) -> float:
    """Asymmetric candidate → eval overlap (fraction of candidate 3-grams present in eval)."""
    c = tri_grams(candidate_txt)
    if not c:
        return 0.0
    e = tri_grams(eval_txt)
    if not e:
        return 0.0
    return len(c & e) / len(c)


def audit(cand_path: Path, eval_path: Path, threshold: float) -> dict:
    candidates = load_jsonl(cand_path)
    evals = load_jsonl(eval_path)
    print(f"Loaded: candidate={len(candidates)} eval={len(evals)}", file=sys.stderr)

    # Pre-compute eval 3-grams once
    eval_texts = [extract_assistant(r) for r in evals]

    hits: list[dict] = []
    for i, c in enumerate(candidates):
        c_txt = extract_assistant(c)
        c_meta = c.get("meta") or {}
        row_id = c_meta.get("row_id") or f"idx_{i}"
        node_id = c_meta.get("source_node_id")
        # asymmetric — candidate side
        for j, e_txt in enumerate(eval_texts):
            o = overlap(c_txt, e_txt)
            if o >= threshold:
                e_meta = evals[j].get("meta") or {}
                hits.append(
                    {
                        "candidate_row_id": row_id,
                        "candidate_source_node_id": node_id,
                        "candidate_path": c_meta.get("source_path"),
                        "eval_row_id": e_meta.get("row_id") or f"eval_idx_{j}",
                        "eval_source_url": e_meta.get("source_url"),
                        "eval_task_name": e_meta.get("task_name"),
                        "overlap": round(o, 4),
                    }
                )
        if (i + 1) % 250 == 0:
            print(f"  audited {i + 1}/{len(candidates)}: hits so far = {len(hits)}", file=sys.stderr)

    return {
        "threshold": threshold,
        "candidate_rows": len(candidates),
        "eval_rows": len(evals),
        "overlap_hits": hits,
        "clean": len(hits) == 0,
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.strip().splitlines()[0])
    ap.add_argument("--candidate", type=Path, default=Path("training_data/sft/mdemg_usage_v1/train.jsonl"))
    ap.add_argument("--eval", dest="eval_path", type=Path, default=Path("training_data/eval/valid_clean.jsonl"))
    ap.add_argument("--out", type=Path, default=Path("training_data/sft/mdemg_usage_v1/leak_audit.json"))
    ap.add_argument("--threshold", type=float, default=0.30)
    args = ap.parse_args()

    for p in (args.candidate, args.eval_path):
        if not p.exists():
            print(f"ERROR: not found: {p}", file=sys.stderr)
            return 2

    result = audit(args.candidate, args.eval_path, args.threshold)
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(result, indent=2), encoding="utf-8")

    print(f"\nLeak audit complete: candidate={result['candidate_rows']} eval={result['eval_rows']} hits={len(result['overlap_hits'])} clean={result['clean']}")
    print(f"Wrote: {args.out}")

    if not result["clean"]:
        print("\nHARD FAIL: leak detected — downstream manifest emission MUST halt.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
