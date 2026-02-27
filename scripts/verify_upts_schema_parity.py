#!/usr/bin/env python3
"""Validate that active UPTS specs conform to the UPTS schema enums.

This check is intentionally lightweight and dependency-free:
- language values in specs must be listed in schema language enum
- expected.symbols[*].type must be listed in schema symbol type enum
- patterns_covered[*] must be listed in schema patterns_covered enum
- duplicate schema file (upts.schema.json) must stay byte-equivalent in parsed JSON
"""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


def load_json(path: Path) -> Any:
    with path.open("r", encoding="utf-8") as f:
        return json.load(f)


def main() -> int:
    repo_root = Path(__file__).resolve().parent.parent

    canonical_schema_path = repo_root / "docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json"
    duplicate_schema_path = repo_root / "docs/lang-parser/lang-parse-spec/upts.schema.json"
    spec_dir = repo_root / "docs/lang-parser/lang-parse-spec/upts/specs"

    if not canonical_schema_path.exists():
        print(f"ERROR: missing canonical UPTS schema: {canonical_schema_path}")
        return 1
    if not spec_dir.exists():
        print(f"ERROR: missing UPTS specs directory: {spec_dir}")
        return 1

    schema = load_json(canonical_schema_path)

    allowed_languages = set(schema["properties"]["language"]["enum"])
    allowed_symbol_types = set(schema["$defs"]["Symbol"]["properties"]["type"]["enum"])
    allowed_patterns = set(schema["properties"]["patterns_covered"]["items"]["enum"])

    issues: list[str] = []
    spec_count = 0

    for spec_path in sorted(spec_dir.glob("*.upts.json")):
        spec_count += 1
        try:
            spec = load_json(spec_path)
        except Exception as exc:  # pylint: disable=broad-except
            issues.append(f"{spec_path}: invalid JSON ({exc})")
            continue

        lang = spec.get("language")
        if lang not in allowed_languages:
            issues.append(f"{spec_path}: language '{lang}' missing from schema enum")

        for idx, symbol in enumerate(spec.get("expected", {}).get("symbols", [])):
            sym_type = symbol.get("type")
            if sym_type and sym_type not in allowed_symbol_types:
                issues.append(
                    f"{spec_path}: symbol[{idx}] type '{sym_type}' missing from schema enum"
                )

        for idx, pattern in enumerate(spec.get("patterns_covered") or []):
            if pattern not in allowed_patterns:
                issues.append(
                    f"{spec_path}: patterns_covered[{idx}] '{pattern}' missing from schema enum"
                )

    if duplicate_schema_path.exists():
        try:
            duplicate_schema = load_json(duplicate_schema_path)
            if duplicate_schema != schema:
                issues.append(
                    "UPTS schema mismatch: duplicate schema file differs from canonical "
                    f"({duplicate_schema_path} != {canonical_schema_path})"
                )
        except Exception as exc:  # pylint: disable=broad-except
            issues.append(f"{duplicate_schema_path}: invalid JSON ({exc})")

    if issues:
        print("UPTS schema parity check failed:")
        for issue in issues:
            print(f"  - {issue}")
        print(f"\nSummary: {len(issues)} issue(s) across {spec_count} spec(s).")
        return 1

    print(
        "UPTS schema parity check passed "
        f"({spec_count} specs, {len(allowed_languages)} languages, "
        f"{len(allowed_symbol_types)} symbol types, {len(allowed_patterns)} patterns)."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
