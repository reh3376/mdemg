#!/usr/bin/env python3
"""DOC-CURRENCY-002 E3 — doc↔code env-var drift checker.

Scans the living operator docs for tokens that look like environment
variables (UPPER_SNAKE, ≥2 segments) and verifies each one exists somewhere
in the code corpus (Go sources, compose/hook templates, workflows, Makefile).
A doc that names a variable no .go file has ever heard of is exactly the
CMS.md class of drift this sprint cleaned up (CMS_RESUME_MAX_TOKENS etc. —
documented for months, never existed, silently no-oped when set).

This is a HEURISTIC LINTER, not a contract test: prose can legitimately
contain UPPER_SNAKE tokens that are not env vars. Unknown tokens therefore
go through an allowlist (scripts/doc_env_vars_allowlist.txt) and the CI step
runs soft-fail (advisory). Use --strict locally to get a nonzero exit.

Usage:
    python3 scripts/verify_doc_env_vars.py            # report, exit 0
    python3 scripts/verify_doc_env_vars.py --strict   # exit 1 on drift
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent

# Living docs an operator configures from. Historical records (sprint plans,
# posts, research) are deliberately excluded — they describe eras.
DOC_GLOBS = [
    "CLAUDE.md",
    "CMS.md",
    "README.md",
    "docs/features/*.md",
    "docs/user/*.md",
]

# Code corpus: anywhere a real env var name would appear as a literal.
CODE_GLOBS = [
    "internal/**/*.go",
    "cmd/**/*.go",
    "internal/cli/compose_templates/*",
    "internal/cli/hook_templates/*",
    "deploy/**/*.yml",
    "deploy/**/*.yaml",
    ".github/workflows/*.yml",
    "Makefile",
    "packaging/launchd/*",
    # Python-side surface — docs legitimately reference constants/env vars
    # consumed by the training pipeline and test runners.
    "neural/**/*.py",
    "scripts/**/*.py",
    "docs/tests/**/runners/*.py",
    "docs/api/api-spec/uats/runners/*.py",
    # Shell scripts (incl. extensionless like scripts/mdemg-git-hook) read
    # operator env vars that docs legitimately describe.
    "scripts/*",
    ".claude/hooks/*",
]

TOKEN_RE = re.compile(r"\b[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+\b")

# Docs use brace shorthand for variable families: `DYNAMIC_EDGE_{MIN_LAYER,TOPK}`
# or `ALERT_RETRIEVE_{P95,P99}_MS`. Expand these into their real member names
# so neither the bare prefix nor the bare members are misread as unknowns.
BRACE_RE = re.compile(r"([A-Z][A-Z0-9_]*_)\{([A-Z0-9_,|]+)\}(_[A-Z0-9_]+|[A-Z0-9_]*)")

ALLOWLIST_PATH = REPO / "scripts" / "doc_env_vars_allowlist.txt"


def expand_braces(text: str) -> tuple[set[str], str]:
    """Return (expanded member tokens, text with brace groups blanked)."""
    expanded: set[str] = set()

    def _sub(m: re.Match) -> str:
        prefix, members, suffix = m.group(1), m.group(2), m.group(3)
        for member in re.split(r"[,|]", members):
            if member:
                expanded.add(prefix + member + suffix)
        return " "

    return expanded, BRACE_RE.sub(_sub, text)


def load_allowlist() -> set[str]:
    if not ALLOWLIST_PATH.exists():
        return set()
    out: set[str] = set()
    for line in ALLOWLIST_PATH.read_text().splitlines():
        line = line.split("#", 1)[0].strip()
        if line:
            out.add(line)
    return out


def collect(globs: list[str]) -> list[Path]:
    files: list[Path] = []
    for g in globs:
        files.extend(p for p in REPO.glob(g) if p.is_file())
    return files


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--strict", action="store_true", help="exit 1 on drift")
    args = ap.parse_args()

    code_text = ""
    for p in collect(CODE_GLOBS):
        try:
            code_text += p.read_text(errors="ignore")
        except OSError:
            continue
    code_tokens = set(TOKEN_RE.findall(code_text))

    allow = load_allowlist()
    drift: dict[str, list[str]] = {}
    doc_files = collect(DOC_GLOBS)
    for doc in doc_files:
        text = doc.read_text(errors="ignore")
        expanded, text = expand_braces(text)
        tokens = set(TOKEN_RE.findall(text))
        # Expanded family members are checked against code; if the FULL name
        # exists in code the shorthand is fine, else it drifts like any token.
        for tok in expanded:
            if tok not in code_tokens and tok not in allow:
                drift.setdefault(tok, []).append(str(doc.relative_to(REPO)) + " (brace form)")
        for tok in sorted(tokens):
            if tok in code_tokens or tok in allow:
                continue
            drift.setdefault(tok, []).append(str(doc.relative_to(REPO)))

    scanned = len(doc_files)
    if not drift:
        print(f"OK: {scanned} docs scanned, no unknown env-var-shaped tokens")
        return 0

    print(f"DRIFT: {len(drift)} env-var-shaped tokens in docs but absent from the code corpus:\n")
    for tok, docs in sorted(drift.items()):
        print(f"  {tok:<48} {', '.join(sorted(set(docs)))}")
    print(
        "\nEach is either (a) a documented variable that does not exist — fix the doc"
        "\nor the code — or (b) legitimate prose: add it to"
        f"\n{ALLOWLIST_PATH.relative_to(REPO)} with a comment."
    )
    return 1 if args.strict else 0


if __name__ == "__main__":
    sys.exit(main())
