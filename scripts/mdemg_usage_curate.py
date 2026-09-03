#!/usr/bin/env python3
"""MDEMG-USAGE-CORPUS-CURATE-001 Epic 2 — deterministic Q&A curator.

Reads Epic 1's nodes.jsonl and emits one Q&A row per surviving section,
in the shipped `{"messages":[{system,user,assistant}], "meta":{...}}` shape
that matches `claude_code_knowledge_v3_stripped` (canonical row-shape reference).

Quality filters (mirror `neural/training/curate_claude_docs.py`):
- --min-words (default 40): skip stubby sections
- --drop-nav-headers: regex-blacklist for "See also", "Related", "Next steps", "Table of contents", "TOC"
- --drop-junk-paths: reuse the JIMINY-CORPUS-003 narrative-junk shape to skip session logs
- --drop-template: skip mdemg-docs/features/template/* skeleton nodes

Question templating (all deterministic, no LLM):
- H2/H3 section header + path-derived context produces natural questions like:
  * "In MDEMG, what does '<Header>' cover in '<Feature>'?"
  * "How does MDEMG handle '<Header>' as documented in '<Feature>'?"
  * "What is described in the '<Header>' section of MDEMG's '<Feature>' docs?"

Usage:
    python3 scripts/mdemg_usage_curate.py \\
        --in training_data/mdemg_usage/raw/nodes.jsonl \\
        --out training_data/sft/mdemg_usage_v1/train.jsonl
    python3 scripts/mdemg_usage_curate.py --dry-run
    python3 scripts/mdemg_usage_curate.py --min-words 60

Sprint: docs/development/mdemg-usage-corpus-curate-001/
"""
from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path

SYSTEM_PROMPT = (
    "You are an expert on MDEMG (Multi-Dimensional Emergent Memory Graph), a cognitive substrate "
    "framework for AI-assisted software development. Answer the user's question accurately based on "
    "the MDEMG project's own documentation. Be concise, cite specific configuration keys, commands, "
    "endpoints, feature names, or code paths by exact name, and include verbatim snippets when they "
    "clarify the answer. If a claim would require guessing beyond documented behavior, say so."
)

NAV_HEADER_RE = re.compile(
    r"^(see also|related|next steps?|table of contents|toc|references?)\s*$",
    re.IGNORECASE,
)

# Sprint MDEMG-USAGE-CORPUS-CURATE-002 (task #148) — required prefix.
# The upstream exporter (`mdemg_usage_export_docs.py`) enforces the same
# prefix in its query WHERE clause; this curator-side gate is defense-in-
# depth against a hand-edited raw JSONL or a future exporter regression.
MDEMG_DOCS_PATH_PREFIX = "mdemg-docs/"

JUNK_PATH_PATTERNS = [
    re.compile(r"mdemg-docs/features/template/", re.IGNORECASE),
]


def is_valid_mdemg_docs_path(path: str) -> bool:
    """Reject any path that doesn't start with the mdemg-docs prefix.

    Task #148 — enforces prefix even when the raw JSONL has been hand-edited
    or produced by a pre-#148 exporter. Empty path → False; wrong prefix
    (e.g. `claude-docs/...`, `/docs/features/...`, `.venv/**/*.py`) → False.
    """
    return bool(path) and path.startswith(MDEMG_DOCS_PATH_PREFIX)

# Question template variants — picked deterministically by row_index % N so
# distribution is even without RNG-driven nondeterminism.
QUESTION_TEMPLATES = [
    "In MDEMG, what does '{header}' cover in the '{feature}' documentation?",
    "How does MDEMG handle '{header}' as documented in '{feature}'?",
    "What is described in the '{header}' section of MDEMG's '{feature}' docs?",
    "In the MDEMG framework, explain '{header}' as it relates to '{feature}'.",
    "According to the MDEMG documentation, what does '{header}' mean for '{feature}'?",
]

# For nodes without a discernible feature name (e.g. CLAUDE.md, cli-reference)
# a simpler template variant is used.
GENERIC_TEMPLATES = [
    "In MDEMG, what does '{header}' refer to?",
    "How does MDEMG document '{header}'?",
    "What does the MDEMG '{header}' section cover?",
    "Explain '{header}' as described in the MDEMG documentation.",
    "In the MDEMG framework, what is '{header}'?",
]


def derive_feature_name(path: str) -> str | None:
    """Extract a human-readable feature name from an ingested doc path.

    /mdemg-docs/features/{feature}/... → {feature}
    /docs/user/... → 'MDEMG user documentation'
    /docs/api/... → 'MDEMG API documentation'
    /CLAUDE.md   → 'CLAUDE.md' handled via generic template
    /cli-reference/... → generic template
    """
    m = re.search(r"mdemg-docs/features/([^/]+)/", path)
    if m:
        raw = m.group(1)
        # numeric prefix like 001__summary → strip
        raw = re.sub(r"^\d{2,4}__", "", raw)
        # kebab/underscore → words
        return raw.replace("-", " ").replace("_", " ").strip() or None
    if "/docs/user" in path:
        return "MDEMG user documentation"
    if "/docs/api" in path:
        return "MDEMG API documentation"
    return None


def is_nav_header(header: str) -> bool:
    return bool(header and NAV_HEADER_RE.match(header.strip()))


def is_junk_path(path: str) -> bool:
    return any(p.search(path) for p in JUNK_PATH_PATTERNS)


def word_count(s: str) -> int:
    return len(s.split())


def normalize_row_id(node_id: str) -> str:
    """Deterministic stable row_id derived from source node_id."""
    return f"mdemg_usage__{hashlib.sha256(node_id.encode()).hexdigest()[:16]}"


def build_qa_row(node_row: dict, template_idx: int) -> dict | None:
    """Convert an Epic-1 node row into a Q&A messages row, or None if skipped."""
    header = (node_row.get("section_header") or "").strip()
    content = (node_row.get("content") or "").strip()
    path = node_row.get("path") or ""
    node_id = node_row.get("node_id") or ""

    if not header or not content or not node_id:
        return None
    if not is_valid_mdemg_docs_path(path):
        # Task #148 — reject rows whose path doesn't have the mdemg-docs/
        # prefix. Defense-in-depth against a pre-fix raw JSONL or a future
        # exporter regression.
        return None
    if is_nav_header(header):
        return None
    if is_junk_path(path):
        return None

    feature = derive_feature_name(path)
    if feature:
        templates = QUESTION_TEMPLATES
        params = {"header": header, "feature": feature}
    else:
        templates = GENERIC_TEMPLATES
        params = {"header": header}

    question = templates[template_idx % len(templates)].format(**params)

    return {
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": question},
            {"role": "assistant", "content": content},
        ],
        "meta": {
            "task_name": "mdemg.usage",
            "sampling_group": "T",
            "source": "mdemg-dev-substrate",
            "source_node_id": node_id,
            "source_path": path,
            "source_surface": node_row.get("surface"),
            "section_header": header,
            "content_sha256": node_row.get("content_sha256"),
            "row_id": normalize_row_id(node_id),
            "word_count": word_count(content),
            "curator": "mdemg_usage_curate.py",
            "curator_version": "1",
        },
    }


def curate(
    in_path: Path,
    out_path: Path,
    min_words: int,
    dry_run: bool,
) -> tuple[int, dict[str, int], dict[str, int]]:
    total_in = 0
    total_out = 0
    per_surface: dict[str, int] = {}
    skip_reasons: dict[str, int] = {}

    if not dry_run:
        out_path.parent.mkdir(parents=True, exist_ok=True)
    out_fh = None if dry_run else out_path.open("w", encoding="utf-8")
    try:
        for idx, line in enumerate(in_path.open(encoding="utf-8")):
            line = line.strip()
            if not line:
                continue
            total_in += 1
            node_row = json.loads(line)
            row = build_qa_row(node_row, template_idx=idx)
            if row is None:
                # Diagnose skip reason for the report
                header = (node_row.get("section_header") or "").strip()
                content = (node_row.get("content") or "").strip()
                path = node_row.get("path") or ""
                if not header:
                    skip_reasons["no_header"] = skip_reasons.get("no_header", 0) + 1
                elif not content:
                    skip_reasons["no_content"] = skip_reasons.get("no_content", 0) + 1
                elif not is_valid_mdemg_docs_path(path):
                    # Task #148 — track prefix-gate rejects for reporting.
                    skip_reasons["wrong_prefix"] = skip_reasons.get("wrong_prefix", 0) + 1
                elif is_nav_header(header):
                    skip_reasons["nav_header"] = skip_reasons.get("nav_header", 0) + 1
                elif is_junk_path(path):
                    skip_reasons["junk_path"] = skip_reasons.get("junk_path", 0) + 1
                else:
                    skip_reasons["other"] = skip_reasons.get("other", 0) + 1
                continue
            if row["meta"]["word_count"] < min_words:
                skip_reasons["min_words"] = skip_reasons.get("min_words", 0) + 1
                continue
            if out_fh is not None:
                out_fh.write(json.dumps(row, ensure_ascii=False) + "\n")
            total_out += 1
            surf = row["meta"]["source_surface"] or "unknown"
            per_surface[surf] = per_surface.get(surf, 0) + 1
    finally:
        if out_fh is not None:
            out_fh.close()

    print(f"Curated: in={total_in} out={total_out}")
    print("Per-surface:")
    for surf, n in sorted(per_surface.items(), key=lambda kv: (-kv[1], kv[0])):
        print(f"  {surf}: {n}")
    print("Skip reasons:")
    for reason, n in sorted(skip_reasons.items(), key=lambda kv: (-kv[1], kv[0])):
        print(f"  {reason}: {n}")
    if not dry_run:
        print(f"Wrote: {out_path}")
    return total_out, per_surface, skip_reasons


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.strip().splitlines()[0])
    ap.add_argument("--in", dest="in_path", type=Path, default=Path("training_data/mdemg_usage/raw/nodes.jsonl"))
    ap.add_argument("--out", type=Path, default=Path("training_data/sft/mdemg_usage_v1/train.jsonl"))
    ap.add_argument("--min-words", type=int, default=40)
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    if not args.in_path.exists():
        print(f"ERROR: input file not found: {args.in_path}", file=sys.stderr)
        return 2

    total, per_surface, skip_reasons = curate(args.in_path, args.out, args.min_words, args.dry_run)

    if total < 1500:
        print(
            f"\nWARN: curated rows {total} < gate threshold 1500. "
            "Epic 3 teacher augment may need to fire.",
            file=sys.stderr,
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
