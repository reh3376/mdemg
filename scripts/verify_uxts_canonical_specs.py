#!/usr/bin/env python3
"""Verify canonical UxTS spec directories only contain canonical dialect specs.

Current coverage:
- UDTS canonical specs: docs/api/api-spec/udts/specs/*.udts.json
- UVTS canonical specs: docs/tests/uvts/specs/*.uvts.json

This guard intentionally checks only canonical `specs/` paths. Legacy or draft
experiments should live under each framework's `drafts/` directory.
"""

from __future__ import annotations

import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Set

ROOT = Path(__file__).resolve().parents[1]

SEMVER_RE = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+$")


@dataclass(frozen=True)
class FrameworkCheck:
    name: str
    schema_path: Path
    spec_dir: Path
    drafts_dir: Path
    pattern: str
    disallowed_root_keys: Set[str]


def _load_json(path: Path) -> Dict:
    with path.open("r", encoding="utf-8") as f:
        return json.load(f)


def _required_fields_from_schema(schema: Dict) -> Set[str]:
    req = schema.get("required", [])
    return set(req) if isinstance(req, list) else set()


def _nested_required_fields_from_schema(schema: Dict, parent_key: str) -> Set[str]:
    props = schema.get("properties", {})
    if not isinstance(props, dict):
        return set()
    parent = props.get(parent_key, {})
    if not isinstance(parent, dict):
        return set()
    req = parent.get("required", [])
    return set(req) if isinstance(req, list) else set()


def _check_required(
    obj: Dict,
    required: Iterable[str],
    context: str,
    errors: List[str],
) -> None:
    missing = [key for key in required if key not in obj]
    if missing:
        errors.append(f"{context}: missing required field(s): {', '.join(sorted(missing))}")


def _validate_semver(value: object, context: str, errors: List[str]) -> None:
    if not isinstance(value, str) or not SEMVER_RE.match(value):
        errors.append(f"{context}: version must be semver (got {value!r})")


def _validate_framework(config: FrameworkCheck) -> tuple[int, int, List[str]]:
    errors: List[str] = []

    if not config.schema_path.exists():
        return 0, 0, [f"{config.name}: schema not found at {config.schema_path}"]
    if not config.spec_dir.exists():
        return 0, 0, [f"{config.name}: canonical spec dir not found at {config.spec_dir}"]
    if not config.drafts_dir.exists():
        errors.append(f"{config.name}: drafts dir not found at {config.drafts_dir}")

    schema = _load_json(config.schema_path)
    root_required = _required_fields_from_schema(schema)

    canonical_specs = sorted(config.spec_dir.glob(config.pattern))
    draft_specs = sorted(config.drafts_dir.glob(config.pattern)) if config.drafts_dir.exists() else []

    for spec_path in canonical_specs:
        try:
            spec = _load_json(spec_path)
        except Exception as exc:  # pragma: no cover - CLI safety path
            errors.append(f"{config.name}: {spec_path}: invalid JSON ({exc})")
            continue

        if not isinstance(spec, dict):
            errors.append(f"{config.name}: {spec_path}: root must be JSON object")
            continue

        context = f"{config.name}: {spec_path}"
        _check_required(spec, root_required, context, errors)

        for forbidden in sorted(config.disallowed_root_keys):
            if forbidden in spec:
                errors.append(
                    f"{context}: found legacy dialect key '{forbidden}' in canonical specs/ (move to drafts/)"
                )

        # Validate semver field if present in schema required set.
        version_key = next((k for k in root_required if k.endswith("_version")), None)
        if version_key and version_key in spec:
            _validate_semver(spec[version_key], f"{context}:{version_key}", errors)

        # Validate one-level nested required blocks declared in schema.
        for nested_parent in ("expected", "validation", "questions"):
            nested_required = _nested_required_fields_from_schema(schema, nested_parent)
            if not nested_required:
                continue
            nested_obj = spec.get(nested_parent)
            if not isinstance(nested_obj, dict):
                errors.append(f"{context}: '{nested_parent}' must be an object")
                continue
            _check_required(nested_obj, nested_required, f"{context}:{nested_parent}", errors)

    return len(canonical_specs), len(draft_specs), errors


def main() -> int:
    checks = [
        FrameworkCheck(
            name="UDTS",
            schema_path=ROOT / "docs/api/api-spec/udts/schema/udts.schema.json",
            spec_dir=ROOT / "docs/api/api-spec/udts/specs",
            drafts_dir=ROOT / "docs/api/api-spec/udts/drafts",
            pattern="*.udts.json",
            disallowed_root_keys={"api", "test_cases"},
        ),
        FrameworkCheck(
            name="UVTS",
            schema_path=ROOT / "docs/tests/uvts/schema/uvts.schema.json",
            spec_dir=ROOT / "docs/tests/uvts/specs",
            drafts_dir=ROOT / "docs/tests/uvts/drafts",
            pattern="*.uvts.json",
            disallowed_root_keys={"test_cases"},
        ),
    ]

    all_errors: List[str] = []
    summaries = []

    for check in checks:
        canonical_count, draft_count, errors = _validate_framework(check)
        summaries.append((check.name, canonical_count, draft_count))
        all_errors.extend(errors)

    if all_errors:
        print("UXTS canonical spec verification FAILED:")
        for err in all_errors:
            print(f"  - {err}")
        return 1

    summary_bits = [
        f"{name}: {canonical_count} canonical specs, {draft_count} drafts"
        for name, canonical_count, draft_count in summaries
    ]
    print("UXTS canonical spec verification passed (" + "; ".join(summary_bits) + ").")
    return 0


if __name__ == "__main__":
    sys.exit(main())
