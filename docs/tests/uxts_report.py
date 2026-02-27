"""
UxTS Canonical Report Builder — Portable Agent Spec v2.2.0, Section 8A

Shared utility for all UxTS framework runners. Provides functions to build
spec-compliant JSON reports with canonical field names, types, and semantics.

Usage:
    from uxts_report import build_result, build_report, print_summary, save_report
"""

import json
from datetime import datetime, timezone
from pathlib import Path


def build_result(
    spec_path,
    status,
    duration_ms=0.0,
    hash_verified=None,
    hash_mismatches=None,
    assertions_evaluated=0,
    assertions_passed=0,
    failures=None,
    warnings=None,
    error=None,
):
    """Build a single spec result conforming to Section 8A.

    Args:
        spec_path: Relative path to the spec file.
        status: One of 'pass', 'fail', 'skip', 'error'.
        duration_ms: Execution time in milliseconds.
        hash_verified: True (match), False (mismatch), or None (no hash field).
        hash_mismatches: List of mismatch detail strings.
        assertions_evaluated: Number of assertions checked.
        assertions_passed: Number that passed.
        failures: List of human-readable failure descriptions.
        warnings: List of advisory messages (do not affect status).
        error: Error message string if status is 'error', else None.
    """
    if status == "pass" and assertions_evaluated == 0:
        status = "fail"
        if not failures:
            failures = []
        failures.append("0/0 false pass: no assertions evaluated")

    return {
        "spec_path": str(spec_path),
        "status": status,
        "duration_ms": round(duration_ms, 2),
        "hash_verified": hash_verified,
        "hash_mismatches": hash_mismatches or [],
        "assertions_evaluated": assertions_evaluated,
        "assertions_passed": assertions_passed,
        "failures": failures or [],
        "warnings": warnings or [],
        "error": error,
    }


def build_integrity(results):
    """Compute integrity block from a list of result dicts."""
    total_hashed = 0
    verified = 0
    mismatched = 0
    no_hash = 0

    for r in results:
        hv = r.get("hash_verified")
        if hv is True:
            total_hashed += 1
            verified += 1
        elif hv is False:
            total_hashed += 1
            mismatched += 1
        else:
            no_hash += 1

    return {
        "total_hashed": total_hashed,
        "verified": verified,
        "mismatched": mismatched,
        "no_hash": no_hash,
    }


def build_report(framework, framework_version, results, start_time=None, duration_ms=None):
    """Build the full canonical report conforming to Section 8A.

    Args:
        framework: Framework acronym (will be lowercased).
        framework_version: Runner/spec version string.
        results: List of result dicts (from build_result).
        start_time: datetime or ISO string. Defaults to now (UTC).
        duration_ms: Total wall-clock time. If None, summed from results.
    """
    if start_time is None:
        start_time = datetime.now(timezone.utc)

    if isinstance(start_time, datetime):
        ts = start_time.isoformat()
        if ts.endswith("+00:00"):
            ts = ts[:-6] + "Z"
    else:
        ts = start_time

    passed = sum(1 for r in results if r["status"] == "pass")
    failed = sum(1 for r in results if r["status"] == "fail")
    skipped = sum(1 for r in results if r["status"] == "skip")
    errors = sum(1 for r in results if r["status"] == "error")
    total = len(results)

    if duration_ms is None:
        duration_ms = sum(r.get("duration_ms", 0) for r in results)

    return {
        "timestamp": ts,
        "framework": framework.lower(),
        "framework_version": framework_version,
        "summary": {
            "total_specs": total,
            "passed": passed,
            "failed": failed,
            "skipped": skipped,
            "errors": errors,
            "pass_rate": round((passed / total) * 100, 2) if total > 0 else 0.0,
            "duration_ms": round(duration_ms, 2),
        },
        "integrity": build_integrity(results),
        "results": results,
    }


def print_summary(report):
    """Print human-readable summary to stdout with both assertion and integrity lines."""
    s = report["summary"]
    i = report["integrity"]
    fw = report["framework"].upper()

    print(f"\n{'=' * 60}")
    print(f" {fw} v{report['framework_version']} — Report Summary")
    print(f"{'=' * 60}")
    print(f"  Total specs : {s['total_specs']}")
    print(f"  Passed      : {s['passed']}")
    print(f"  Failed      : {s['failed']}")
    print(f"  Skipped     : {s['skipped']}")
    print(f"  Errors      : {s['errors']}")
    print(f"  Pass rate   : {s['pass_rate']}%")
    print(f"  Duration    : {s['duration_ms']:.0f} ms")
    print()
    print(f"  Integrity:")
    print(f"    Hashed    : {i['total_hashed']}")
    print(f"    Verified  : {i['verified']}")
    print(f"    Mismatched: {i['mismatched']}")
    print(f"    No hash   : {i['no_hash']}")
    print(f"{'=' * 60}")


def save_report(report, path):
    """Save report as JSON to the given file path."""
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    Path(path).write_text(json.dumps(report, indent=2) + "\n")
    print(f"\nReport saved: {path}")
