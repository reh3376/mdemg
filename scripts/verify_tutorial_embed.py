#!/usr/bin/env python3
"""CI drift gate for the tutorial embed.

Mirrors the shape of `verify_grafana_embed` (RELEASE-HYGIENE-001). Fails
with exit code 1 if any configured source file was edited without
re-running `make sync-tutorial-embed`.

Approach: at build time, tutorial-forge (`scripts/tutorial-forge/`) writes
`<output>/SOURCE_SHA256.txt` recording the sha256 of each source input
(deck markdown, CSS override, SVG viewer). This verifier reads the config
(`tutorial.config.json`), re-computes those hashes against the current
source files, and compares.

- Match → no drift (exit 0)
- Mismatch → drift (exit 1, prints which file(s) changed)
- Missing breadcrumb → embed was never synced (exit 1)

Pure stdlib — no third-party deps. Runs in CI on every PR.
"""

from __future__ import annotations

import hashlib
import json
import pathlib
import sys


REPO_ROOT = pathlib.Path(__file__).resolve().parent.parent
CONFIG_PATH = REPO_ROOT / "tutorial.config.json"
FRAMEWORK_DIR = REPO_ROOT / "scripts" / "tutorial-forge"


def sha256_of(path: pathlib.Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def parse_breadcrumb(path: pathlib.Path) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        k, v = line.split("=", 1)
        out[k.strip()] = v.strip()
    return out


def resolve_source(repo_root: pathlib.Path, config_dir: pathlib.Path,
                   config_value: str | None, framework_default: pathlib.Path) -> pathlib.Path:
    """Resolve a source path from the config: config value is relative to the
    config file's own dir; a None/missing value falls back to the framework
    default."""
    if not config_value:
        return framework_default
    return (config_dir / config_value).resolve()


def main() -> int:
    if not CONFIG_PATH.exists():
        print(f"ERROR: tutorial config missing: {CONFIG_PATH.relative_to(REPO_ROOT)}")
        return 1

    config = json.loads(CONFIG_PATH.read_text())
    config_dir = CONFIG_PATH.parent
    output_dir = (config_dir / config["output"]).resolve()
    breadcrumb_path = output_dir / "SOURCE_SHA256.txt"

    if not breadcrumb_path.exists():
        print(f"ERROR: breadcrumb missing: {breadcrumb_path.relative_to(REPO_ROOT)}")
        print("Embed was never built. Run: make sync-tutorial-embed")
        return 1

    deck_src = resolve_source(REPO_ROOT, config_dir,
                              config.get("deck"),
                              FRAMEWORK_DIR / "templates" / "default.css")  # deck has no framework default
    css_src = resolve_source(REPO_ROOT, config_dir,
                             config.get("cssPath"),
                             FRAMEWORK_DIR / "templates" / "default.css")
    viewer_src = resolve_source(REPO_ROOT, config_dir,
                                config.get("svgViewerPath"),
                                FRAMEWORK_DIR / "templates" / "svg-viewer.js")

    for label, path in [("deck source", deck_src), ("CSS source", css_src), ("SVG viewer", viewer_src)]:
        if not path.exists():
            print(f"ERROR: {label} missing: {path}")
            return 1

    breadcrumb = parse_breadcrumb(breadcrumb_path)
    expected_deck = breadcrumb.get("deck_sha256", "")
    expected_css = breadcrumb.get("css_sha256", "")
    expected_viewer = breadcrumb.get("svg_viewer_sha256", "")

    if not expected_deck or not expected_css or not expected_viewer:
        print("ERROR: breadcrumb malformed: missing deck_sha256, css_sha256, or svg_viewer_sha256")
        print("  Re-run: make sync-tutorial-embed")
        return 1

    actual_deck = sha256_of(deck_src)
    actual_css = sha256_of(css_src)
    actual_viewer = sha256_of(viewer_src)

    def report(label: str, path: pathlib.Path, expected: str, actual: str) -> bool:
        if expected == actual:
            return False
        try:
            display = path.relative_to(REPO_ROOT)
        except ValueError:
            display = path
        print(f"DRIFT: {display}")
        print(f"  embed was built against: {expected}")
        print(f"  current source hash:     {actual}")
        return True

    drifted = False
    drifted |= report("deck", deck_src, expected_deck, actual_deck)
    drifted |= report("css", css_src, expected_css, actual_css)
    drifted |= report("viewer", viewer_src, expected_viewer, actual_viewer)

    if drifted:
        print()
        print("A tutorial source file changed without re-running the embed sync.")
        print("Fix: cd into the repo root and run:")
        print("    make sync-tutorial-embed")
        print("    git add internal/cli/tutorial_templates/staged")
        print("    git commit --amend --no-edit")
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
