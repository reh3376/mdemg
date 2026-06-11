#!/usr/bin/env python3
"""UxTS Framework Drift Checker.

Detects drift between on-disk reality and documented state across all UxTS
frameworks.  Checks:

  1. Spec count — actual files vs documented count in UXTS_FRAMEWORK_MATRIX.md
  2. Runner existence — does the declared runner file actually exist?
  3. Fixture existence — do specs reference fixtures that exist on disk?
  4. Hash coverage — do specs with sha256 fields contain a real hash (not PENDING)?

Exit code 0 = all checks pass.  Exit code 1 = drift detected.
"""

from __future__ import annotations

import json
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import List

ROOT = Path(__file__).resolve().parents[1]

# ---------------------------------------------------------------------------
# Framework definitions — add new frameworks here
# ---------------------------------------------------------------------------

@dataclass
class Framework:
    name: str
    spec_dir: str
    spec_glob: str
    runner_path: str | None = None
    hash_json_paths: list[str] = field(default_factory=list)
    # Regex to extract documented spec count from the matrix.  Group 1 = count.
    # We match the row by framework name and extract the first integer in the
    # "Specs" column.
    documented_count: int | None = None


FRAMEWORKS = [
    Framework("UATS", "docs/api/api-spec/uats/specs", "*.uats.json",
              runner_path="docs/api/api-spec/uats/runners/uats_runner.py",
              hash_json_paths=["config.sha256"]),
    Framework("UDTS", "docs/api/api-spec/udts/specs", "*.udts.json",
              runner_path="tests/udts/contract_test.go",
              hash_json_paths=["config.proto_sha256"]),
    Framework("UPTS", "docs/lang-parser/lang-parse-spec/upts/specs", "*.upts.json",
              runner_path="docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py",
              hash_json_paths=["fixture.sha256"]),
    Framework("UBTS", "docs/tests/ubts/specs", "*.ubts.json",
              runner_path="docs/tests/ubts/runners/ubts_runner.py",
              hash_json_paths=["config.sha256"]),
    Framework("USTS", "docs/tests/usts/specs", "*.usts.json",
              runner_path="docs/tests/usts/runners/usts_runner.py",
              hash_json_paths=["config.sha256"]),
    Framework("UAMS", "docs/tests/uams/specs", "*.uams.json",
              runner_path=None,
              hash_json_paths=["config.sha256"]),
    Framework("UOBS", "docs/tests/uobs/specs", "*.uobs.json",
              runner_path="docs/tests/uobs/runners/uobs_runner.py",
              hash_json_paths=["config.sha256"]),
    Framework("UOTS", "docs/api/api-spec/uots/specs", "*.uots.json",
              runner_path="docs/api/api-spec/uots/runners/uots_runner.py",
              hash_json_paths=["config.sha256"]),
    Framework("UVTS", "docs/tests/uvts/specs", "*.uvts.json",
              runner_path="docs/tests/uvts/runners/uvts_runner.py",
              hash_json_paths=["config.sha256"]),
    Framework("UETS", "docs/tests/uets/specs", "*.uets.json",
              runner_path="docs/tests/uets/runners/uets_runner.py",
              hash_json_paths=["fixture.sha256"]),
    # UXTS-CI-001: the five previously-omitted frameworks.
    Framework("ULTS", "docs/tests/ults/specs", "*.ults.json",
              runner_path="docs/tests/ults/runners/ults_runner.py",
              hash_json_paths=[]),  # prompt hashes verified by the runner itself (--verify-hashes)
    Framework("UITS", "docs/tests/uits/specs", "*.uits.json",
              runner_path="docs/tests/uits/runners/uits_runner.py",
              hash_json_paths=["config.sha256"]),
    Framework("UTDS", "docs/tests/utds/specs", "*.utds.json",
              runner_path="docs/tests/utds/runners/utds_runner.py",
              hash_json_paths=["config.sha256"]),
    Framework("UAITS", "docs/tests/uaits/specs", "*.uaits.json",
              runner_path="docs/tests/uaits/runners/uaits_runner.py",
              hash_json_paths=[]),  # validated live via `mdemg data validate`
    Framework("UBENCH", "docs/tests/ubench/specs", "*.ubench.json",
              runner_path="docs/tests/ubench/runners/ubench_runner.py",
              hash_json_paths=[]),  # holdout SHAs verified by test-ubench-contract
]

MATRIX_PATH = ROOT / "docs/development/UXTS_FRAMEWORK_MATRIX.md"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def extract_hash(doc: dict, path: str) -> str | None:
    """Walk a dot-separated JSON path and return the leaf value."""
    parts = path.split(".")
    current = doc
    for p in parts:
        if not isinstance(current, dict):
            return None
        current = current.get(p)
    return current if isinstance(current, str) else None


def parse_documented_counts(matrix_path: Path) -> dict[str, int]:
    """Parse spec counts from the framework matrix markdown table."""
    if not matrix_path.exists():
        return {}
    text = matrix_path.read_text(encoding="utf-8")
    counts: dict[str, int] = {}
    # Match rows like: | UATS | ... | active (124 specs, CI-gated) | 124 canonical, 7 drafts |
    for line in text.splitlines():
        if not line.startswith("|"):
            continue
        cells = [c.strip() for c in line.split("|")]
        if len(cells) < 6:
            continue
        name = cells[1]
        specs_col = cells[5]  # "Specs" column (6th cell, 1-indexed = cells[5])
        # Extract first integer from specs column
        m = re.search(r"(\d+)", specs_col)
        if m and name in {f.name for f in FRAMEWORKS}:
            counts[name] = int(m.group(1))
    return counts


# ---------------------------------------------------------------------------
# Checks
# ---------------------------------------------------------------------------

def check_spec_counts(documented: dict[str, int]) -> list[str]:
    """Compare on-disk spec count to documented count."""
    errors: list[str] = []
    for fw in FRAMEWORKS:
        spec_dir = ROOT / fw.spec_dir
        if not spec_dir.exists():
            continue
        actual = len(list(spec_dir.glob(fw.spec_glob)))
        doc_count = documented.get(fw.name)
        if doc_count is not None and actual != doc_count:
            errors.append(
                f"{fw.name}: documented {doc_count} specs but found {actual} on disk"
            )
    return errors


def check_runner_existence() -> list[str]:
    """Verify declared runner files exist."""
    errors: list[str] = []
    for fw in FRAMEWORKS:
        if fw.runner_path is None:
            continue
        path = ROOT / fw.runner_path
        if not path.exists():
            errors.append(f"{fw.name}: runner not found at {fw.runner_path}")
    return errors


def check_fixtures() -> list[str]:
    """Check that specs referencing fixtures point to existing files."""
    errors: list[str] = []
    for fw in FRAMEWORKS:
        spec_dir = ROOT / fw.spec_dir
        if not spec_dir.exists():
            continue
        for spec_file in sorted(spec_dir.glob(fw.spec_glob)):
            try:
                doc = json.loads(spec_file.read_text(encoding="utf-8"))
            except (json.JSONDecodeError, OSError):
                continue
            # Check fixture.path
            fixture = doc.get("fixture", {})
            if isinstance(fixture, dict) and fixture.get("type") == "file":
                fixture_path = fixture.get("path", "")
                if fixture_path:
                    resolved = (spec_file.parent / fixture_path).resolve()
                    if not resolved.exists():
                        errors.append(
                            f"{fw.name}: {spec_file.name} references missing fixture {fixture_path}"
                        )
    return errors


def check_hash_coverage() -> list[str]:
    """Check that hash fields are populated (not empty or PENDING)."""
    warnings: list[str] = []
    for fw in FRAMEWORKS:
        spec_dir = ROOT / fw.spec_dir
        if not spec_dir.exists():
            continue
        for spec_file in sorted(spec_dir.glob(fw.spec_glob)):
            try:
                doc = json.loads(spec_file.read_text(encoding="utf-8"))
            except (json.JSONDecodeError, OSError):
                continue
            for hash_path in fw.hash_json_paths:
                value = extract_hash(doc, hash_path)
                if value is not None and (value == "PENDING" or value == ""):
                    warnings.append(
                        f"{fw.name}: {spec_file.name} has {hash_path}={value!r} (not populated)"
                    )
    return warnings


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    documented = parse_documented_counts(MATRIX_PATH)

    all_errors: list[str] = []
    all_warnings: list[str] = []

    all_errors.extend(check_spec_counts(documented))
    all_errors.extend(check_runner_existence())
    all_errors.extend(check_fixtures())
    all_warnings.extend(check_hash_coverage())

    if all_warnings:
        print("UxTS drift warnings:")
        for w in all_warnings:
            print(f"  ⚠ {w}")

    if all_errors:
        print("UxTS drift check FAILED:")
        for e in all_errors:
            print(f"  ✗ {e}")
        return 1

    fw_summary = ", ".join(f"{fw.name}: {len(list((ROOT / fw.spec_dir).glob(fw.spec_glob))) if (ROOT / fw.spec_dir).exists() else 0}" for fw in FRAMEWORKS)
    print(f"UxTS drift check passed ({fw_summary}).")
    if all_warnings:
        print(f"  ({len(all_warnings)} hash warnings — non-blocking)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
