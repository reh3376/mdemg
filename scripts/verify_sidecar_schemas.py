#!/usr/bin/env python3
"""Validate sidecar fixture JSON files against their JSON Schema definitions.

Uses Draft 2020-12 validation explicitly (schemas declare 2020-12).
Exits with code 1 if any fixture fails validation.

Usage: python3 scripts/verify_sidecar_schemas.py
"""

import json
import sys
from pathlib import Path

try:
    from jsonschema import Draft202012Validator
except ImportError:
    print("ERROR: jsonschema package required. Install: pip install jsonschema")
    sys.exit(1)

SCHEMA_DIR = Path(__file__).resolve().parent.parent / "docs" / "sidecar" / "schemas"
FIXTURE_DIR = SCHEMA_DIR / "fixtures"

# Static mapping: fixture file -> schema file
# sidecar-config.schema.json is excluded (validates YAML config, no JSON fixture)
FIXTURE_SCHEMA_PAIRS = [
    ("attach-agent-report.example.json", "attach-agent-report.schema.json"),
    ("doctor-report.example.json", "doctor-report.schema.json"),
    ("install-report.example.json", "install-report.schema.json"),
    ("status-report.example.json", "status-report.schema.json"),
    ("uninstall-report.example.json", "uninstall-report.schema.json"),
    ("upgrade-report.example.json", "upgrade-report.schema.json"),
]


def validate_fixture(fixture_name: str, schema_name: str) -> list[str]:
    """Validate a fixture against its schema. Returns list of error messages."""
    schema_path = SCHEMA_DIR / schema_name
    fixture_path = FIXTURE_DIR / fixture_name

    if not schema_path.exists():
        return [f"Schema file not found: {schema_path}"]
    if not fixture_path.exists():
        return [f"Fixture file not found: {fixture_path}"]

    with open(schema_path) as f:
        schema = json.load(f)
    with open(fixture_path) as f:
        fixture = json.load(f)

    validator = Draft202012Validator(schema)
    errors = list(validator.iter_errors(fixture))
    return [f"  {e.json_path}: {e.message}" for e in errors]


def main() -> int:
    print("=== Sidecar Schema Validation ===\n")

    total = len(FIXTURE_SCHEMA_PAIRS)
    passed = 0
    failed = 0

    for fixture_name, schema_name in FIXTURE_SCHEMA_PAIRS:
        errors = validate_fixture(fixture_name, schema_name)
        if errors:
            print(f"FAIL  {fixture_name}")
            for err in errors:
                print(err)
            failed += 1
        else:
            print(f"PASS  {fixture_name}")
            passed += 1

    print(f"\n--- Summary: {passed}/{total} passed, {failed}/{total} failed ---")

    return 1 if failed > 0 else 0


if __name__ == "__main__":
    sys.exit(main())
