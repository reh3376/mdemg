#!/usr/bin/env python3
"""CLAUDE-DOCS-TRAINING-001 Epic 3 — deterministic H2/H3 section extraction.

Reads raw markdown corpus at training_data/claude-docs/raw/*.md (produced by
scripts/scrape_claude_docs.py) and emits one Q&A pair per H2/H3 section to
training_data/claude-docs/curated/qa.jsonl.

Curation approach: Option A (deterministic) per operator sign-off 2026-08-14.
- One row per `## Header` (H2) and `### Header` (H3) section.
- Q templated from the header text + doc title; A is the section body verbatim
  (code blocks preserved).
- Quality filter: drop sections below --min-words (default 40), drop sections
  whose headers match navigation patterns ("See also", "Next steps",
  "Related").
- Per-row provenance: source_url, source_sha256 (from scrape manifest),
  concept_type (h2|h3), section_index, curated_at.

Emits alongside qa.jsonl:
- manifest.json — corpus stats (row count, per-concept-type distribution,
  quality-filter counts, per-source-url row count)
- distribution_report.txt — human-readable summary

Usage:
    python3 neural/training/curate_claude_docs.py           # full run
    python3 neural/training/curate_claude_docs.py --limit 5 # smoke on 5 files
    python3 neural/training/curate_claude_docs.py --min-words 60
    python3 neural/training/curate_claude_docs.py --dry-run # print stats only

Sprint: docs/development/claude-docs-training-001/
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from collections import Counter
from dataclasses import dataclass, field, asdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parent.parent.parent
RAW_DIR = REPO_ROOT / "training_data/claude-docs/raw"
MANIFEST_PATH = REPO_ROOT / "training_data/claude-docs/scrape_manifest.json"
CURATED_DIR = REPO_ROOT / "training_data/claude-docs/curated"
QA_PATH = CURATED_DIR / "qa.jsonl"
CURATE_MANIFEST_PATH = CURATED_DIR / "manifest.json"
DISTRIBUTION_REPORT_PATH = CURATED_DIR / "distribution_report.txt"


# Headers that are navigation/boilerplate — drop.
NAV_HEADER_PATTERNS = [
    re.compile(r"^see also\b", re.IGNORECASE),
    re.compile(r"^next steps?\b", re.IGNORECASE),
    re.compile(r"^related\b", re.IGNORECASE),
    re.compile(r"^further reading\b", re.IGNORECASE),
    re.compile(r"^references?\b", re.IGNORECASE),
    re.compile(r"^table of contents\b", re.IGNORECASE),
    re.compile(r"^getting help\b", re.IGNORECASE),
]

FRONTMATTER_RE = re.compile(r"^---\n(.*?)\n---\n", re.DOTALL)
HEADER_RE = re.compile(r"^(#{1,6})\s+(.+?)\s*$", re.MULTILINE)


@dataclass
class QaRow:
    """One curated Q&A pair."""

    row_id: str
    prompt: str
    completion: str
    source_url: str
    source_sha256: str
    source_slug: str
    doc_title: str
    section_header: str
    concept_type: str  # "h2" | "h3"
    section_index: int  # 0-based within file
    word_count: int
    curated_at_utc: str


@dataclass
class CurationStats:
    total_files_scanned: int = 0
    files_with_no_sections: int = 0
    total_sections_found: int = 0
    sections_dropped_short: int = 0
    sections_dropped_nav: int = 0
    sections_emitted: int = 0
    concept_types: dict[str, int] = field(default_factory=dict)
    per_source_url: dict[str, int] = field(default_factory=dict)
    started_at_utc: str = ""
    completed_at_utc: str = ""


def strip_frontmatter(md: str) -> tuple[str, dict[str, str]]:
    """Split YAML frontmatter off the top of a markdown doc.

    Returns (body, metadata_dict).
    """
    m = FRONTMATTER_RE.match(md)
    if not m:
        return md, {}
    body = md[m.end() :]
    meta = {}
    for line in m.group(1).splitlines():
        if ":" in line:
            k, v = line.split(":", 1)
            meta[k.strip()] = v.strip().strip("\"'")
    return body, meta


def is_nav_header(header_text: str) -> bool:
    return any(p.match(header_text) for p in NAV_HEADER_PATTERNS)


def split_into_sections(md_body: str) -> list[tuple[str, str, int]]:
    """Split a markdown body into (header_text, header_level, body) sections.

    Only H2/H3 sections; H4+ are folded into the parent H3/H2.
    Returns list of (header_text, level "h2"|"h3", section_body).
    Section body excludes the header line itself.
    """
    # Find all H2/H3 header positions.
    headers: list[tuple[int, int, str]] = []  # (start_pos, level, header_text)
    for m in HEADER_RE.finditer(md_body):
        level = len(m.group(1))
        if level in (2, 3):
            headers.append((m.start(), level, m.group(2).strip()))

    sections: list[tuple[str, str, int]] = []
    for i, (start, level, header_text) in enumerate(headers):
        # Body starts after this header line, ends at next H2/H3.
        header_line_end = md_body.find("\n", start) + 1
        end = headers[i + 1][0] if i + 1 < len(headers) else len(md_body)
        body = md_body[header_line_end:end].strip()
        sections.append((header_text, f"h{level}", body))
    return sections


def render_question(doc_title: str, section_header: str) -> str:
    """Template a natural-sounding question from doc + section title.

    Rules:
    - If the header starts with a verb-shaped word ("How to X", "Setting up",
      "Configuring", "Working with"), emit "In Claude Code, {header_lc}?"
    - If the header ends with "?", use verbatim.
    - Common config-shaped headers ("Settings", "Configuration", "Options",
      "Environment variables"): "What are the {header} for {doc_title}?"
    - Reference-shaped headers ("Commands", "Flags", "Reference"):
      "What are the {header} for {doc_title}?"
    - Default: "In Claude Code, what does '{section_header}' mean in the
      context of {doc_title}?"
    """
    header = section_header.strip().rstrip(".")
    header_lc = header.lower()

    if header.endswith("?"):
        return header

    verb_shaped = (
        header_lc.startswith(("how to ", "how do ", "when to ", "setting up ",
                              "getting started", "creating ", "configuring ",
                              "installing ", "working with ", "using ",
                              "customizing ", "adding ", "removing ",
                              "enabling ", "disabling ", "running ",
                              "debugging ", "troubleshooting"))
    )
    if verb_shaped:
        return f"In Claude Code, {header_lc}?"

    config_shaped = header_lc in (
        "settings", "configuration", "options", "environment variables",
        "config", "flags", "parameters", "keys", "values",
    )
    if config_shaped:
        return f"What are the {header_lc} for {doc_title} in Claude Code?"

    reference_shaped = header_lc in (
        "commands", "reference", "api", "syntax", "spec", "schema",
    )
    if reference_shaped:
        return f"What is the {header_lc} for {doc_title} in Claude Code?"

    # Default: framed as a definition/explanation question.
    return f"In Claude Code, what does '{header}' mean in the context of {doc_title}?"


def word_count(text: str) -> int:
    return len(text.split())


def row_id_for(source_url: str, section_index: int, header: str) -> str:
    key = f"{source_url}::{section_index}::{header}"
    return "cd_" + hashlib.sha256(key.encode("utf-8")).hexdigest()[:16]


def load_scrape_manifest(path: Path) -> dict[str, dict[str, Any]]:
    """Return {slug: record} lookup so we can attach source_sha256 + doc_title."""
    if not path.exists():
        raise SystemExit(f"error: scrape manifest not found at {path}. Run scripts/scrape_claude_docs.py first.")
    data = json.loads(path.read_text(encoding="utf-8"))
    out: dict[str, dict[str, Any]] = {}
    for rec in data.get("records", []):
        out[rec["slug"]] = rec
    return out


def curate_file(md_path: Path, meta_rec: dict[str, Any], min_words: int, stats: CurationStats) -> list[QaRow]:
    md = md_path.read_text(encoding="utf-8")
    body, frontmatter = strip_frontmatter(md)
    doc_title = frontmatter.get("title") or meta_rec.get("title") or md_path.stem

    sections = split_into_sections(body)
    stats.total_sections_found += len(sections)
    if not sections:
        stats.files_with_no_sections += 1
        return []

    now = datetime.now(timezone.utc).isoformat(timespec="seconds")
    source_url = meta_rec.get("url", "")
    source_sha = meta_rec.get("content_sha256", "")

    rows: list[QaRow] = []
    for i, (header, level, section_body) in enumerate(sections):
        if is_nav_header(header):
            stats.sections_dropped_nav += 1
            continue
        wc = word_count(section_body)
        if wc < min_words:
            stats.sections_dropped_short += 1
            continue

        prompt = render_question(doc_title, header)
        completion = section_body
        row = QaRow(
            row_id=row_id_for(source_url, i, header),
            prompt=prompt,
            completion=completion,
            source_url=source_url,
            source_sha256=source_sha,
            source_slug=md_path.stem,
            doc_title=doc_title,
            section_header=header,
            concept_type=level,
            section_index=i,
            word_count=wc,
            curated_at_utc=now,
        )
        rows.append(row)
        stats.sections_emitted += 1
        stats.concept_types[level] = stats.concept_types.get(level, 0) + 1
        stats.per_source_url[source_url] = stats.per_source_url.get(source_url, 0) + 1

    return rows


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--min-words", type=int, default=40,
                    help="drop sections with body < N words (default: 40)")
    ap.add_argument("--limit", type=int, default=0,
                    help="curate only first N files (smoke)")
    ap.add_argument("--dry-run", action="store_true",
                    help="print stats but do not write qa.jsonl")
    args = ap.parse_args()

    if not RAW_DIR.exists():
        print(f"error: raw corpus not found at {RAW_DIR}. Run scripts/scrape_claude_docs.py first.", file=sys.stderr)
        return 2

    manifest_by_slug = load_scrape_manifest(MANIFEST_PATH)
    md_files = sorted(RAW_DIR.glob("*.md"))
    if args.limit:
        md_files = md_files[: args.limit]
    if not md_files:
        print(f"error: no .md files under {RAW_DIR}", file=sys.stderr)
        return 2

    stats = CurationStats()
    stats.started_at_utc = datetime.now(timezone.utc).isoformat(timespec="seconds")

    all_rows: list[QaRow] = []
    for md_path in md_files:
        stats.total_files_scanned += 1
        meta_rec = manifest_by_slug.get(md_path.stem, {})
        if not meta_rec:
            print(f"warn: no manifest record for {md_path.name} — skipping (no source_url/sha)", file=sys.stderr)
            continue
        rows = curate_file(md_path, meta_rec, args.min_words, stats)
        all_rows.extend(rows)

    stats.completed_at_utc = datetime.now(timezone.utc).isoformat(timespec="seconds")

    # Print summary
    print(f"curated {stats.sections_emitted} Q&A pairs from {stats.total_files_scanned} files:")
    print(f"  total sections found:  {stats.total_sections_found}")
    print(f"  emitted:               {stats.sections_emitted}")
    print(f"  dropped (short):       {stats.sections_dropped_short} (< {args.min_words} words)")
    print(f"  dropped (nav):         {stats.sections_dropped_nav}")
    print(f"  files w/ 0 sections:   {stats.files_with_no_sections}")
    print(f"  concept types:         {dict(stats.concept_types)}")

    if args.dry_run:
        print("\n[dry-run] not writing qa.jsonl")
        return 0

    CURATED_DIR.mkdir(parents=True, exist_ok=True)

    # Write JSONL
    with open(QA_PATH, "w", encoding="utf-8") as f:
        for row in all_rows:
            f.write(json.dumps(asdict(row), ensure_ascii=False) + "\n")

    # Manifest
    manifest_out = {
        "sprint": "CLAUDE-DOCS-TRAINING-001",
        "epic": "3 (deterministic H2/H3 extraction — Option A)",
        "scrape_manifest_source": str(MANIFEST_PATH.relative_to(REPO_ROOT)),
        "qa_output": str(QA_PATH.relative_to(REPO_ROOT)),
        "min_words_filter": args.min_words,
        "stats": asdict(stats),
    }
    CURATE_MANIFEST_PATH.write_text(json.dumps(manifest_out, indent=2), encoding="utf-8")

    # Human-readable distribution report
    lines: list[str] = []
    lines.append("CLAUDE-DOCS-TRAINING-001 Epic 3 — curation distribution report")
    lines.append(f"generated: {stats.completed_at_utc}")
    lines.append(f"total rows: {stats.sections_emitted}")
    lines.append("")
    lines.append("== Concept type distribution ==")
    for ct, n in sorted(stats.concept_types.items()):
        pct = n / max(1, stats.sections_emitted) * 100
        lines.append(f"  {ct}: {n} ({pct:.1f}%)")
    lines.append("")
    lines.append("== Top 15 source URLs by row count ==")
    top = sorted(stats.per_source_url.items(), key=lambda kv: -kv[1])[:15]
    for url, n in top:
        lines.append(f"  {n:4d} rows — {url}")
    lines.append("")
    lines.append("== Word-count distribution (of emitted rows) ==")
    wc_buckets = Counter()
    for row in all_rows:
        if row.word_count < 60:
            wc_buckets["40-59"] += 1
        elif row.word_count < 120:
            wc_buckets["60-119"] += 1
        elif row.word_count < 300:
            wc_buckets["120-299"] += 1
        elif row.word_count < 800:
            wc_buckets["300-799"] += 1
        else:
            wc_buckets["800+"] += 1
    for b in ("40-59", "60-119", "120-299", "300-799", "800+"):
        n = wc_buckets.get(b, 0)
        pct = n / max(1, stats.sections_emitted) * 100
        lines.append(f"  {b:>8} words: {n:4d} ({pct:5.1f}%)")

    DISTRIBUTION_REPORT_PATH.write_text("\n".join(lines) + "\n", encoding="utf-8")

    print()
    print("wrote:")
    print(f"  {QA_PATH.relative_to(REPO_ROOT)}  ({QA_PATH.stat().st_size / 1024:.1f} KB)")
    print(f"  {CURATE_MANIFEST_PATH.relative_to(REPO_ROOT)}")
    print(f"  {DISTRIBUTION_REPORT_PATH.relative_to(REPO_ROOT)}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
