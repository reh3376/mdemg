#!/usr/bin/env python3
"""CLAUDE-DOCS-TRAINING-002 Epic 1 — chunk long Q&A completions.

Reads training_data/claude-docs/curated/qa.jsonl (from CLAUDE-DOCS-TRAINING-001
Epic 3) and produces qa_v2.jsonl where any row whose completion exceeds
--max-tokens (default 1500) is SPLIT into multiple rows.

Motivation:
CLAUDE-DOCS-TRAINING-001 Epic 5 training truncated long sections at
mlx_lm.lora's --max-seq-length (2048 tokens). 112 rows (5%) overshot,
including the highest-signal reference pages:
- env-vars 'Variables' section: 129,797 tokens
- settings 'Available settings': 62,688 tokens
- commands 'All commands': 49,954 tokens
- cli-reference 'CLI flags': 25,764 tokens
The truncated tail contained exactly the specific enum values, type
signatures, and command flags that the trained adapter hallucinated
on Epic 6 smoke inference.

Chunking strategy (recursive-split):
1. Split on H4 boundaries (`#### Header`) — natural sub-section fault lines
2. If still >max_tokens, split on H5+ boundaries
3. If still >max_tokens, split on paragraph boundaries (`\n\n`)
4. If still >max_tokens, hard-split on line boundaries (last resort — preserves code-block integrity poorly)

Each chunk gets its own Q&A row with:
- row_id: `<original_row_id>__chunk_<N>_of_<M>` for auditability
- prompt: original prompt + " (part {N} of {M})" suffix for chunks
- All other metadata preserved from source row
- word_count: recomputed per chunk

Token counting: uses tiktoken cl100k_base as approximation (Qwen3 tokenizer
gives similar counts within ~10% for markdown).

Usage:
    python3 neural/training/chunk_claude_docs.py                     # default 1500 tokens
    python3 neural/training/chunk_claude_docs.py --max-tokens 1800   # tighter
    python3 neural/training/chunk_claude_docs.py --dry-run           # stats only

Sprint: docs/development/claude-docs-training-002/
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent.parent
QA_IN = REPO_ROOT / "training_data/claude-docs/curated/qa.jsonl"
QA_OUT = REPO_ROOT / "training_data/claude-docs/curated/qa_v2.jsonl"
CHUNK_MANIFEST = REPO_ROOT / "training_data/claude-docs/curated/chunk_manifest.json"


try:
    import tiktoken
    _ENCODER = tiktoken.get_encoding("cl100k_base")

    def count_tokens(text: str) -> int:
        return len(_ENCODER.encode(text))
except ImportError:
    # Fallback: char-count / 3.5 approximation
    def count_tokens(text: str) -> int:
        return len(text) // 3


def split_on_headers(body: str, min_level: int) -> list[str]:
    """Split body on headers of level exactly min_level (not deeper — those stay attached).

    Returns list of section strings.
    """
    # Match headers of exactly `min_level` hashes at line start.
    # Using string-format to sidestep f-string quirks with brace escaping.
    hashes = "#" * min_level
    pattern_str = r"^" + re.escape(hashes) + r"\s+.+?$"
    pattern = re.compile(pattern_str, re.MULTILINE)
    matches = list(pattern.finditer(body))
    if not matches:
        return [body]
    chunks: list[str] = []
    # Preamble (before first header)
    if matches[0].start() > 0:
        preamble = body[:matches[0].start()].strip()
        if preamble:
            chunks.append(preamble)
    # Each header + its content up to the next header at this level
    for i, m in enumerate(matches):
        start = m.start()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(body)
        chunk = body[start:end].strip()
        if chunk:
            chunks.append(chunk)
    return chunks


def split_on_paragraphs(body: str) -> list[str]:
    """Split body on blank lines (paragraph boundaries)."""
    paras = [p.strip() for p in re.split(r"\n\s*\n", body) if p.strip()]
    return paras or [body]


def chunk_completion(body: str, max_tokens: int, depth: int = 0) -> list[str]:
    """Recursively split body until each chunk fits under max_tokens."""
    if depth > 20:
        # Safety guard — should never hit this in normal input.
        return [body]
    if count_tokens(body) <= max_tokens:
        return [body]

    # Try H4/H5/H6 in order; only recurse when a level actually produces multiple parts.
    for level in (4, 5, 6):
        parts = split_on_headers(body, level)
        if len(parts) > 1:
            out: list[str] = []
            for p in parts:
                # Only recurse if the part is still too big AND is smaller than the input
                # (prevents infinite loop when the split produces the input back).
                if len(p) < len(body) and count_tokens(p) > max_tokens:
                    out.extend(chunk_completion(p, max_tokens, depth + 1))
                else:
                    out.append(p)
            return out

    # Fall through to paragraph split
    parts = split_on_paragraphs(body)
    if len(parts) > 1:
        # Greedy accumulate: pack paragraphs into buckets that fit under max_tokens.
        # If a paragraph itself is > max_tokens, recurse to fall through to line-split.
        out = []
        cur: list[str] = []
        cur_tokens = 0
        for p in parts:
            pt = count_tokens(p)
            if pt > max_tokens:
                # Flush current bucket first
                if cur:
                    out.append("\n\n".join(cur))
                    cur = []
                    cur_tokens = 0
                # Recurse on the oversized paragraph (will hit line-split fallback)
                out.extend(chunk_completion(p, max_tokens, depth + 1))
                continue
            if cur and cur_tokens + pt > max_tokens:
                out.append("\n\n".join(cur))
                cur = [p]
                cur_tokens = pt
            else:
                cur.append(p)
                cur_tokens += pt
        if cur:
            out.append("\n\n".join(cur))
        return out

    # Last resort: line-boundary split (may break code-blocks but preserves training progress)
    lines = body.split("\n")
    out = []
    cur = []
    cur_tokens = 0
    for line in lines:
        lt = count_tokens(line + "\n")
        if cur and cur_tokens + lt > max_tokens:
            out.append("\n".join(cur))
            cur = [line]
            cur_tokens = lt
        else:
            cur.append(line)
            cur_tokens += lt
    if cur:
        out.append("\n".join(cur))
    return out


def word_count(text: str) -> int:
    return len(text.split())


def chunked_row_id(orig_id: str, i: int, n: int) -> str:
    """Chunked row_id includes original id + chunk index for auditability."""
    return f"{orig_id}__chunk_{i+1}_of_{n}"


def render_chunked_prompt(orig_prompt: str, i: int, n: int) -> str:
    """Add "(part N of M)" suffix for chunked rows."""
    if n == 1:
        return orig_prompt
    stripped = orig_prompt.rstrip("?.")
    return f"{stripped} (part {i+1} of {n})?"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--max-tokens", type=int, default=1500,
                    help="split rows whose completion exceeds this many tokens (default: 1500)")
    ap.add_argument("--dry-run", action="store_true",
                    help="print stats but do not write outputs")
    args = ap.parse_args()

    if not QA_IN.exists():
        print(f"error: {QA_IN} not found. Run curate_claude_docs.py first.", file=sys.stderr)
        return 2

    rows: list[dict] = []
    with open(QA_IN, encoding="utf-8") as f:
        for line in f:
            rows.append(json.loads(line))
    print(f"loaded {len(rows)} rows from {QA_IN.relative_to(REPO_ROOT)}")

    over = 0
    total_chunks_produced = 0
    largest_original = 0
    out_rows: list[dict] = []
    chunk_provenance: list[dict] = []

    for r in rows:
        tokens = count_tokens(r["completion"])
        if tokens > largest_original:
            largest_original = tokens

        if tokens <= args.max_tokens:
            # No chunking needed
            r2 = dict(r)
            r2["chunk_index"] = 0
            r2["chunk_total"] = 1
            r2["token_count"] = tokens
            out_rows.append(r2)
            continue

        # Chunk it
        over += 1
        chunks = chunk_completion(r["completion"], args.max_tokens)
        n = len(chunks)
        total_chunks_produced += n

        chunk_provenance.append({
            "source_row_id": r["row_id"],
            "source_url": r["source_url"],
            "section_header": r["section_header"],
            "source_slug": r["source_slug"],
            "original_tokens": tokens,
            "chunk_count": n,
            "chunks_token_counts": [count_tokens(c) for c in chunks],
        })

        for i, chunk in enumerate(chunks):
            r2 = dict(r)
            r2["row_id"] = chunked_row_id(r["row_id"], i, n)
            r2["prompt"] = render_chunked_prompt(r["prompt"], i, n)
            r2["completion"] = chunk
            r2["word_count"] = word_count(chunk)
            r2["chunk_index"] = i
            r2["chunk_total"] = n
            r2["token_count"] = count_tokens(chunk)
            r2["original_row_id"] = r["row_id"]
            out_rows.append(r2)

    # Stats
    print(f"  original_max_tokens: {largest_original}")
    print(f"  rows_over_{args.max_tokens}_tokens: {over} of {len(rows)}")
    print(f"  chunks_produced_from_over_rows: {total_chunks_produced}")
    print(f"  final_row_count: {len(out_rows)} (vs {len(rows)} original)")
    over_still = sum(1 for r in out_rows if r["token_count"] > args.max_tokens)
    if over_still > 0:
        print(f"  WARN: {over_still} chunks STILL over {args.max_tokens} tokens (line-split fallback insufficient)")

    if args.dry_run:
        print("[dry-run] not writing outputs")
        return 0

    QA_OUT.write_text("".join(json.dumps(r, ensure_ascii=False) + "\n" for r in out_rows), encoding="utf-8")

    manifest = {
        "sprint": "CLAUDE-DOCS-TRAINING-002",
        "epic": "1 (chunking refinement)",
        "source_qa": str(QA_IN.relative_to(REPO_ROOT)),
        "output_qa": str(QA_OUT.relative_to(REPO_ROOT)),
        "max_tokens_gate": args.max_tokens,
        "input_rows": len(rows),
        "input_rows_over_gate": over,
        "output_rows": len(out_rows),
        "chunks_produced_from_over_rows": total_chunks_produced,
        "original_max_tokens": largest_original,
        "output_max_tokens": max(r["token_count"] for r in out_rows),
        "output_still_over_gate": over_still,
        "chunk_provenance": chunk_provenance,
        "output_sha256": hashlib.sha256(QA_OUT.read_bytes()).hexdigest(),
        "generated_at_utc": datetime.now(timezone.utc).isoformat(timespec="seconds"),
    }
    CHUNK_MANIFEST.write_text(json.dumps(manifest, indent=2), encoding="utf-8")

    print()
    print("wrote:")
    print(f"  {QA_OUT.relative_to(REPO_ROOT)}  ({QA_OUT.stat().st_size / 1024:.1f} KB)")
    print(f"  {CHUNK_MANIFEST.relative_to(REPO_ROOT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
