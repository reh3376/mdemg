#!/usr/bin/env python3
"""
UDTS Runner — Go test wrapper for Section 8A report compliance.

Invokes `go test -json ./tests/udts/...` and translates the JSON output
stream into a canonical UxTS report.
"""
import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..', '..', '..', 'tests'))
from uxts_report import build_result, build_report, print_summary, save_report

UDTS_VERSION = "1.0.0"


def run_go_tests(project_root):
    """Run Go UDTS tests with JSON output and return parsed events."""
    env = os.environ.copy()
    if not env.get("UDTS_TARGET"):
        return None, "UDTS_TARGET not set — tests will be skipped"

    cmd = ["go", "test", "-json", "-v", "-count=1", "./tests/udts/..."]
    try:
        proc = subprocess.run(
            cmd,
            cwd=str(project_root),
            capture_output=True,
            text=True,
            timeout=120,
        )
    except subprocess.TimeoutExpired:
        return None, "Go test timed out after 120s"
    except FileNotFoundError:
        return None, "Go binary not found"

    events = []
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            continue

    return events, None


def parse_events(events):
    """Parse Go test JSON events into canonical results."""
    # Group by test name
    tests = {}
    for ev in events:
        test_name = ev.get("Test", "")
        if not test_name:
            continue
        if test_name not in tests:
            tests[test_name] = {"action": None, "output": [], "elapsed": 0}

        action = ev.get("Action", "")
        if action in ("pass", "fail", "skip"):
            tests[test_name]["action"] = action
            tests[test_name]["elapsed"] = ev.get("Elapsed", 0)
        if action == "output":
            tests[test_name]["output"].append(ev.get("Output", ""))

    results = []
    for test_name, data in tests.items():
        # Skip sub-test entries (only count top-level tests)
        if "/" in test_name:
            continue

        action = data["action"]
        if action is None:
            continue

        # Check for hash mismatch in output
        hash_verified = None
        hash_mismatches = []
        for line in data["output"]:
            if "proto hash mismatch" in line:
                hash_verified = False
                hash_mismatches.append(line.strip())
            elif "proto_sha256" in line.lower() and "PASS" in line:
                hash_verified = True

        # If test accesses proto hash but no mismatch detected, mark verified
        has_hash_check = any("proto" in l and "hash" in l for l in data["output"])
        if has_hash_check and hash_verified is None:
            hash_verified = True

        failures = []
        if action == "fail":
            for line in data["output"]:
                if line.strip().startswith("---") or "FAIL" in line:
                    failures.append(line.strip())

        status = action  # Go test uses same names: pass, fail, skip
        results.append(build_result(
            spec_path=f"tests/udts/{test_name}",
            status=status,
            duration_ms=data["elapsed"] * 1000,
            hash_verified=hash_verified,
            hash_mismatches=hash_mismatches,
            assertions_evaluated=1 if action in ("pass", "fail") else 0,
            assertions_passed=1 if action == "pass" else 0,
            failures=failures,
            error="Test skipped (UDTS_TARGET not set?)" if action == "skip" else None,
        ))

    return results


def main():
    parser = argparse.ArgumentParser(description="UDTS Runner — Go test wrapper")
    parser.add_argument("--report", type=Path, help="Path to write Section 8A JSON report")
    parser.add_argument("--project-root", type=Path, default=None,
                        help="Project root (default: auto-detect from script location)")
    args = parser.parse_args()

    # Auto-detect project root
    project_root = args.project_root
    if project_root is None:
        # Script is at docs/api/api-spec/udts/runners/udts_runner.py
        project_root = Path(__file__).resolve().parent.parent.parent.parent.parent.parent

    start = time.time()
    events, error = run_go_tests(project_root)
    duration_ms = (time.time() - start) * 1000

    if error:
        print(f"UDTS: {error}")
        results = [build_result(
            spec_path="tests/udts/contract_test.go",
            status="error",
            error=error,
        )]
    elif not events:
        results = [build_result(
            spec_path="tests/udts/contract_test.go",
            status="error",
            error="No test events produced",
        )]
    else:
        results = parse_events(events)

    report = build_report("udts", UDTS_VERSION, results, duration_ms=duration_ms)
    print_summary(report)

    if args.report:
        save_report(report, args.report)

    failed = report["summary"]["failed"] + report["summary"]["errors"]
    return 1 if failed > 0 else 0


if __name__ == "__main__":
    sys.exit(main())
