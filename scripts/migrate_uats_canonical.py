#!/usr/bin/env python3
"""
GAP-28: Migrate UATS legacy-dialect body assertions to canonical op/expected grammar.

Transforms:
  {"path": "$.x", "type": "string"}          → {"path": "$.x", "op": "type_is", "expected": "string"}
  {"path": "$.x", "equals": "val"}           → {"path": "$.x", "op": "equals", "expected": "val"}
  {"path": "$.x", "contains": "val"}         → {"path": "$.x", "op": "contains", "expected": "val"}
  {"path": "$.x", "not_contains": "val"}     → {"path": "$.x", "op": "not_contains", "expected": "val"}
  {"path": "$.x", "not_equals": "val"}       → {"path": "$.x", "op": "not_equals", "expected": "val"}
  {"path": "$.x", "regex": "pat"}            → {"path": "$.x", "op": "matches", "expected": "pat"}
  {"path": "$.x", "exists": true}            → {"path": "$.x", "op": "exists"}
  {"path": "$.x", "exists": false}           → {"path": "$.x", "op": "not_exists"}
  {"path": "$.x", "in": [...]}               → {"path": "$.x", "op": "one_of", "expected": [...]}
  {"path": "$.x", "schema": {...}}           → {"path": "$.x", "op": "schema_match", "expected": {...}}
  {"path": "$.x", "schema_match": {...}}     → {"path": "$.x", "op": "schema_match", "expected": {...}}
  {"path": "$.x", "items_all_match": {...}}  → {"path": "$.x", "op": "items_all_match", "expected": {...}}
  {"path": "$.x", "range": {"min":0,"max":1}} → {"path": "$.x", "op": "range", "expected": {"min":0,"max":1}}
  {"path": "$.x", "length": {"min":1}}       → {"path": "$.x", "op": "length", "expected": {"min":1}}

Preserves: name, message, optional, capture_as fields.
Regenerates SHA256 hashes after conversion.

Usage:
    python scripts/migrate_uats_canonical.py --spec-dir docs/api/api-spec/uats/specs/
    python scripts/migrate_uats_canonical.py --spec-dir docs/api/api-spec/uats/specs/ --dry-run
"""

from __future__ import annotations

import argparse
import hashlib
import json
import glob
import os
import sys
from typing import Any, Dict, List, Optional, Tuple

# Add tests/ to path for shared utilities
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'docs', 'tests'))
try:
    from uxts_runner_core import sha256_spec_without_field
except ImportError:
    # Fallback: inline hash computation
    def sha256_spec_without_field(spec: Dict, field_path: Tuple[str, ...]) -> str:
        """Compute SHA256 excluding a nested field."""
        import copy
        s = copy.deepcopy(spec)
        obj = s
        for key in field_path[:-1]:
            if key not in obj:
                break
            obj = obj[key]
        else:
            obj.pop(field_path[-1], None)
        canonical = json.dumps(s, sort_keys=True, separators=(',', ':')).encode('utf-8')
        return hashlib.sha256(canonical).hexdigest()


# Legacy field → canonical op mapping (matches runner's _normalize_assertion)
LEGACY_MAPPING: List[Tuple[str, str]] = [
    ("equals", "equals"),
    ("not_equals", "not_equals"),
    ("contains", "contains"),
    ("not_contains", "not_contains"),
    ("regex", "matches"),
    ("type", "type_is"),
    ("in", "one_of"),
    ("schema", "schema_match"),
    ("schema_match", "schema_match"),
    ("items_all_match", "items_all_match"),
    ("range", "range"),
    ("length", "length"),
]

# Fields preserved as-is (not legacy predicates)
PRESERVED_FIELDS = {"path", "name", "message", "optional", "capture_as"}


def is_canonical(assertion: Dict) -> bool:
    """Check if assertion already uses canonical op/expected grammar."""
    return "op" in assertion


def convert_assertion(assertion: Dict) -> Dict:
    """Convert a single legacy assertion to canonical grammar."""
    if is_canonical(assertion):
        return assertion  # Already canonical

    canonical: Dict[str, Any] = {}

    # Copy preserved fields
    for field in PRESERVED_FIELDS:
        if field in assertion:
            canonical[field] = assertion[field]

    # Find the legacy predicate key and map it
    for legacy_field, op_name in LEGACY_MAPPING:
        if legacy_field in assertion:
            canonical["op"] = op_name

            # Special cases: exists/not_exists don't need expected
            if legacy_field == "exists":
                # exists: true → op: exists; exists: false → op: not_exists
                # Neither needs an expected field
                pass
            else:
                canonical["expected"] = assertion[legacy_field]
            break
    else:
        # Check exists separately (it's a boolean, not in the simple mapping)
        if "exists" in assertion:
            exists_val = assertion["exists"]
            canonical["op"] = "exists" if exists_val else "not_exists"
        else:
            # Unknown format — return unchanged with warning
            print(f"  WARNING: Unknown assertion format: {json.dumps(assertion)}")
            return assertion

    return canonical


def convert_assertions_list(assertions: List[Dict]) -> Tuple[List[Dict], int]:
    """Convert a list of assertions. Returns (converted_list, count_converted)."""
    converted = []
    count = 0
    for a in assertions:
        if not is_canonical(a):
            converted.append(convert_assertion(a))
            count += 1
        else:
            converted.append(a)
    return converted, count


def process_spec(spec: Dict) -> Tuple[Dict, int]:
    """Process a single spec, converting all legacy assertions. Returns (modified_spec, total_converted)."""
    total_converted = 0

    # Convert base body_assertions
    base_ba = spec.get("expected", {}).get("body_assertions", [])
    if base_ba:
        converted, count = convert_assertions_list(base_ba)
        if count > 0:
            spec["expected"]["body_assertions"] = converted
            total_converted += count

    # Convert variant body_assertions
    for variant in spec.get("variants", []):
        vba = variant.get("expected", {}).get("body_assertions", [])
        if vba:
            converted, count = convert_assertions_list(vba)
            if count > 0:
                variant["expected"]["body_assertions"] = converted
                total_converted += count

    return spec, total_converted


def rehash_spec(spec: Dict) -> Dict:
    """Regenerate SHA256 hash for a spec."""
    if "config" not in spec:
        spec["config"] = {}
    spec["config"]["sha256"] = sha256_spec_without_field(spec, ("config", "sha256"))
    return spec


def main():
    parser = argparse.ArgumentParser(description="Migrate UATS specs from legacy to canonical grammar")
    parser.add_argument("--spec-dir", required=True, help="Directory containing .uats.json specs")
    parser.add_argument("--dry-run", action="store_true", help="Show what would change without writing")
    parser.add_argument("--verbose", action="store_true", help="Show per-file details")
    args = parser.parse_args()

    spec_files = sorted(glob.glob(os.path.join(args.spec_dir, "*.uats.json")))
    if not spec_files:
        print(f"ERROR: No .uats.json files found in {args.spec_dir}")
        sys.exit(1)

    total_files_modified = 0
    total_assertions_converted = 0
    total_files_skipped = 0
    errors = []

    for spec_path in spec_files:
        basename = os.path.basename(spec_path)
        try:
            with open(spec_path) as f:
                spec = json.load(f)

            modified_spec, count = process_spec(spec)

            if count == 0:
                total_files_skipped += 1
                if args.verbose:
                    print(f"  SKIP {basename} (already canonical or no assertions)")
                continue

            # Rehash after conversion
            modified_spec = rehash_spec(modified_spec)

            total_files_modified += 1
            total_assertions_converted += count

            if args.dry_run:
                print(f"  WOULD CONVERT {basename}: {count} assertion(s)")
            else:
                with open(spec_path, 'w') as f:
                    json.dump(modified_spec, f, indent=2, ensure_ascii=False)
                    f.write('\n')
                if args.verbose:
                    print(f"  CONVERTED {basename}: {count} assertion(s)")

        except Exception as e:
            errors.append((basename, str(e)))
            print(f"  ERROR {basename}: {e}")

    # Summary
    print()
    print("=" * 60)
    print(f"Migration {'(DRY RUN) ' if args.dry_run else ''}Summary")
    print(f"  Total spec files:      {len(spec_files)}")
    print(f"  Files modified:        {total_files_modified}")
    print(f"  Files skipped:         {total_files_skipped}")
    print(f"  Assertions converted:  {total_assertions_converted}")
    print(f"  Errors:                {len(errors)}")
    print("=" * 60)

    if errors:
        print("\nErrors:")
        for name, err in errors:
            print(f"  {name}: {err}")
        sys.exit(1)

    sys.exit(0)


if __name__ == "__main__":
    main()
