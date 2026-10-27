#!/usr/bin/env python3
"""Expand the first 2 occurrences of each known acronym in the onboarding
deck to its full form plus parenthesized acronym.

Operates on docs/onboarding/mdemg-onboarding-deck.md in place. Skips content
inside fenced code blocks (```...```), inline code (`...`), HTML tags, slide
directive comments, and YAML frontmatter. Only expands acronyms whose
expansion is on an explicit allowlist — this is deliberately conservative so
we never ship a wrong expansion.

Format: first and second match of ACRONYM become "Expansion (ACRONYM)".
Third occurrence and beyond stay as "ACRONYM".

Idempotent: once "Expansion (ACRONYM)" is in the text, the regex matches it
as a bound token for ACRONYM and the next 0 occurrences are expanded — if
re-run, the first-2 counter still finds the first 2 bare acronyms (if any
remain), skipping anything already following "Expansion (".
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

DECK = Path(__file__).resolve().parents[2] / "docs" / "onboarding" / "mdemg-onboarding-deck.md"

# Vetted dictionary. Industry-standard expansions only. Project-internal
# acronyms (MDEMG, JIMINY, RSIC, J17, UXTS family specifics, F#/U# section
# labels) are intentionally omitted — they're defined by surrounding text or
# are navigation keys, not vocabulary.
EXPANSIONS: dict[str, str] = {
    "AI": "Artificial Intelligence",
    "API": "Application Programming Interface",
    "BM25": "Best Matching 25",
    "CI": "Continuous Integration",
    "CLI": "Command-Line Interface",
    "CMS": "Cognitive Memory Substrate",
    "CSV": "Comma-Separated Values",
    "CUID": "Collision-resistant Unique Identifier",
    "DOM": "Document Object Model",
    "DPO": "Direct Preference Optimization",
    "EMA": "Exponential Moving Average",
    "FT": "Fine-Tuning",
    "GGUF": "GPT-Generated Unified Format",
    "GRPO": "Group Relative Policy Optimization",
    "HITL": "Human-In-The-Loop",
    "HTTP": "Hypertext Transfer Protocol",
    "HTTPS": "HTTP Secure",
    "IDE": "Integrated Development Environment",
    "JSON": "JavaScript Object Notation",
    "KV": "Key-Value",
    "LLM": "Large Language Model",
    "MCP": "Model Context Protocol",
    "NLI": "Natural Language Inference",
    "OOM": "Out-Of-Memory",
    "OWASP": "Open Worldwide Application Security Project",
    "RAFT": "Retrieval-Augmented Fine-Tuning",
    "RRF": "Reciprocal Rank Fusion",
    "SDK": "Software Development Kit",
    "SFT": "Supervised Fine-Tuning",
    "SHA": "Secure Hash Algorithm",
    "SLO": "Service Level Objective",
    "SME": "Subject Matter Expert",
    "SNMP": "Simple Network Management Protocol",
    "SQL": "Structured Query Language",
    "TOC": "Table of Contents",
    "TSDB": "Time-Series Database",
    "UAITS": "Universal AI Training Specification",
    "UATS": "Universal API Test Specification",
    "UBENCH": "Universal Benchmark",
    "UI": "User Interface",
    "ULTS": "Universal LLM Test Specification",
    "URL": "Uniform Resource Locator",
    "UUID": "Universally Unique Identifier",
    "UVTS": "Universal Validation Test Specification",
    "UX": "User Experience",
    "UXTS": "Universal eXtensible Test Specification",
    "WSL": "Windows Subsystem for Linux",
}


def expand(deck_text: str) -> tuple[str, dict[str, int]]:
    """Return (modified_text, {acronym: substitutions_applied}).

    We walk the markdown line by line, tracking fenced-code state. For each
    non-code line we scan tokens left-to-right; a bare acronym match becomes
    "Expansion (ACRONYM)" for the first two occurrences per acronym.

    Skip conditions on a line-by-line basis:
      * fenced code block (``` toggled)
      * HTML slide-directive comments (<!-- .slide: id="…" -->)
      * lines consisting of a mermaid directive or image URL

    Within a line we also skip:
      * inline code spans (`...`)
      * HTML tags <...>
    """
    counts: dict[str, int] = {k: 0 for k in EXPANSIONS}
    out_lines: list[str] = []
    in_code = False
    # Also preserve already-expanded forms so we don't double-expand after a
    # user runs this script, we edit, then run again. The regex boundary
    # `(?<!\()` would catch "Expansion (ACRONYM)" by excluding the acronym
    # that immediately follows an open paren.
    for raw in deck_text.splitlines(keepends=True):
        line = raw
        stripped = line.strip()
        if stripped.startswith("```"):
            in_code = not in_code
            out_lines.append(line)
            continue
        if in_code:
            out_lines.append(line)
            continue
        if stripped.startswith("<!--") and stripped.endswith("-->"):
            out_lines.append(line)
            continue
        # Mask inline code spans and HTML tags: replace them with sentinels
        # (preserving original length) so regex positions stay useful; we
        # splice originals back after substitution.
        masks: list[tuple[int, int, str]] = []
        # inline code
        for m in re.finditer(r"`[^`\n]+`", line):
            masks.append((m.start(), m.end(), m.group()))
        # HTML tags
        for m in re.finditer(r"<[^>\n]+>", line):
            masks.append((m.start(), m.end(), m.group()))
        # markdown link URLs: skip URL part [text](url)
        for m in re.finditer(r"\]\([^)\n]+\)", line):
            masks.append((m.start(), m.end(), m.group()))
        # Non-overlapping sort
        masks.sort()
        merged: list[tuple[int, int, str]] = []
        for mb, me, mt in masks:
            if merged and mb < merged[-1][1]:
                continue
            merged.append((mb, me, mt))
        # Build a working string where masked segments are replaced by
        # whitespace of equal length so acronym matches never land inside.
        buf = list(line)
        for mb, me, _ in merged:
            for i in range(mb, me):
                buf[i] = " "
        scan = "".join(buf)
        # Find acronym tokens (word-boundary). Longest-first so UVTS doesn't
        # eat UV. Sort EXPANSIONS keys by length desc.
        acronym_pattern = r"(?<!\w)(" + "|".join(
            re.escape(k) for k in sorted(EXPANSIONS, key=len, reverse=True)
        ) + r")(?!\w)"
        edits: list[tuple[int, int, str]] = []
        for m in re.finditer(acronym_pattern, scan):
            tok = m.group(1)
            # Already expanded? Check if "Expansion (" precedes the match in
            # the original line. The expanded form ends with "(TOK)" so if
            # the two chars before are "( " or "(", the token is already in
            # a parenthesized expansion suffix and shouldn't count.
            start = m.start(1)
            prev_char = line[start - 1] if start > 0 else ""
            if prev_char == "(":
                continue
            if counts[tok] >= 2:
                continue
            counts[tok] += 1
            expansion = EXPANSIONS[tok]
            edits.append((m.start(1), m.end(1), f"{expansion} ({tok})"))
        if not edits:
            out_lines.append(line)
            continue
        # Apply edits right-to-left so earlier positions stay valid
        edited = line
        for es, ee, rep in reversed(edits):
            edited = edited[:es] + rep + edited[ee:]
        out_lines.append(edited)
    return "".join(out_lines), {k: v for k, v in counts.items() if v}


def main() -> int:
    text = DECK.read_text()
    new_text, applied = expand(text)
    if new_text == text:
        print("No changes needed; all acronyms already expanded or absent.")
        return 0
    DECK.write_text(new_text)
    print(f"Updated {DECK}")
    print("Expansions applied (acronym → first-N-occurrences-expanded):")
    for k in sorted(applied):
        print(f"  {k:8s}  {applied[k]}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
