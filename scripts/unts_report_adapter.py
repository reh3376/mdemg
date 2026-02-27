#!/usr/bin/env python3
"""
UNTS Report Adapter — Reads UNTS registry and produces Section 8A report.

UNTS is a meta-framework (hash verification service), not a traditional
spec runner. This adapter translates its registry into the canonical
report format for governance tooling.
"""
import argparse
import json
import os
import sys
from pathlib import Path

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'docs', 'tests'))
from uxts_report import build_result, build_report, print_summary, save_report

UNTS_VERSION = "1.0.0"
DEFAULT_REGISTRY = "docs/specs/unts-registry.json"


def load_registry(path):
    """Load the UNTS registry JSON file."""
    with open(path) as f:
        return json.load(f)


def registry_to_results(registry):
    """Convert UNTS registry entries to canonical results.

    Handles both dict format ({"files": {"path": {...}}}) and
    list format ({"files": [{"path": "...", "status": "..."}]}).
    """
    results = []
    files = registry.get("files", {})

    # Normalize list format to dict format
    if isinstance(files, list):
        files = {entry.get("path", f"entry_{i}"): entry for i, entry in enumerate(files)}

    for file_path, record in files.items():
        status_val = record.get("status", "unknown")
        hash_verified = None
        hash_mismatches = []

        if status_val == "verified":
            hash_verified = True
            status = "pass"
        elif status_val == "mismatch":
            hash_verified = False
            hash_mismatches.append(
                f"Hash mismatch: stored={record.get('stored_hash', '?')}, "
                f"current={record.get('current_hash', '?')}"
            )
            status = "pass"  # UNTS reports integrity, not assertions
        elif status_val == "unknown":
            hash_verified = None
            status = "pass"
        elif status_val == "reverted":
            hash_verified = False
            hash_mismatches.append("Hash reverted to previous value")
            status = "pass"
        else:
            status = "error"

        results.append(build_result(
            spec_path=file_path,
            status=status,
            hash_verified=hash_verified,
            hash_mismatches=hash_mismatches,
            assertions_evaluated=1 if hash_verified is not None else 0,
            assertions_passed=1 if hash_verified is True else 0,
        ))

    return results


def main():
    parser = argparse.ArgumentParser(description="UNTS Report Adapter")
    parser.add_argument("--registry", type=Path, default=DEFAULT_REGISTRY,
                        help=f"Path to UNTS registry JSON (default: {DEFAULT_REGISTRY})")
    parser.add_argument("--report", type=Path, help="Path to write Section 8A JSON report")
    args = parser.parse_args()

    if not args.registry.exists():
        print(f"UNTS registry not found: {args.registry}")
        print("Run UNTS scanner first to generate the registry.")
        return 1

    registry = load_registry(args.registry)
    results = registry_to_results(registry)
    report = build_report("unts", UNTS_VERSION, results)
    print_summary(report)

    if args.report:
        save_report(report, args.report)

    return 0


if __name__ == "__main__":
    sys.exit(main())
