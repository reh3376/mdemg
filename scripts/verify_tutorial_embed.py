#!/usr/bin/env python3
"""ONBOARDING-TUTORIAL-SERVE-001 — CI drift gate for the tutorial embed.

Mirrors the shape of `verify_grafana_embed` (RELEASE-HYGIENE-001). Fails
with exit code 1 if the deck source or the CSS override was edited without
re-running `make sync-tutorial-embed`.

Approach: at export time, `scripts/tutorial/export.mjs` writes a breadcrumb
file `internal/cli/tutorial_templates/staged/SOURCE_SHA256.txt` recording
the sha256 of both source files. This verifier re-computes those hashes
against the CURRENT source files and compares.

- Match → no drift (exit 0)
- Mismatch → drift (exit 1, prints which file(s) changed)
- Missing breadcrumb → embed was never synced (exit 1)

Pure stdlib — no third-party deps. Runs in CI on every PR.
"""

from __future__ import annotations

import hashlib
import pathlib
import sys


REPO_ROOT = pathlib.Path(__file__).resolve().parent.parent
BREADCRUMB = REPO_ROOT / "internal" / "cli" / "tutorial_templates" / "staged" / "SOURCE_SHA256.txt"
DECK_SRC = REPO_ROOT / "docs" / "onboarding" / "mdemg-onboarding-deck.md"
CSS_SRC = REPO_ROOT / "docs" / "onboarding" / "slides-overrides.css"


def sha256_of(path: pathlib.Path) -> str:
    """Return the hex sha256 of a file."""
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def parse_breadcrumb(path: pathlib.Path) -> dict[str, str]:
    """Parse `key=value` lines, skipping comments + blanks."""
    out: dict[str, str] = {}
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            continue
        k, v = line.split("=", 1)
        out[k.strip()] = v.strip()
    return out


def main() -> int:
    if not BREADCRUMB.exists():
        print(f"ERROR: breadcrumb missing: {BREADCRUMB.relative_to(REPO_ROOT)}")
        print("Embed was never built. Run: make sync-tutorial-embed")
        return 1

    if not DECK_SRC.exists():
        print(f"ERROR: deck source missing: {DECK_SRC.relative_to(REPO_ROOT)}")
        return 1

    if not CSS_SRC.exists():
        print(f"ERROR: CSS source missing: {CSS_SRC.relative_to(REPO_ROOT)}")
        return 1

    breadcrumb = parse_breadcrumb(BREADCRUMB)
    expected_deck = breadcrumb.get("deck_sha256", "")
    expected_css = breadcrumb.get("css_sha256", "")

    if not expected_deck or not expected_css:
        print(f"ERROR: breadcrumb malformed: missing deck_sha256 or css_sha256")
        print(f"  Re-run: make sync-tutorial-embed")
        return 1

    actual_deck = sha256_of(DECK_SRC)
    actual_css = sha256_of(CSS_SRC)

    deck_drift = actual_deck != expected_deck
    css_drift = actual_css != expected_css

    if deck_drift:
        print(f"DRIFT: {DECK_SRC.relative_to(REPO_ROOT)}")
        print(f"  embed was built against: {expected_deck}")
        print(f"  current source hash:     {actual_deck}")

    if css_drift:
        print(f"DRIFT: {CSS_SRC.relative_to(REPO_ROOT)}")
        print(f"  embed was built against: {expected_css}")
        print(f"  current source hash:     {actual_css}")

    if deck_drift or css_drift:
        print()
        print("The deck source changed without re-running the embed sync.")
        print("Fix: cd into the repo root and run:")
        print("    make sync-tutorial-embed")
        print("    git add internal/cli/tutorial_templates/staged")
        print("    git commit --amend --no-edit")
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
