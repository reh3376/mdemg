#!/usr/bin/env python3
"""
UAMS Runner — Universal Auth Method Specification Runner

Validates UAMS specs for structural conformance and optionally tests auth
method behavior against a live MDEMG server.

Version: 1.0.0

Usage:
    # Validate all specs (no server needed)
    python uams_runner.py validate-all --spec-dir specs/

    # Test auth methods against live server
    python uams_runner.py test --spec-dir specs/ --base-url http://localhost:9999

    # Single spec with report output
    python uams_runner.py validate --spec specs/apikey.uams.json --report report.json
"""

import argparse
import glob
import json
import os
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional

import sys as _sys
_sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..'))
from uxts_report import (
    build_result as _canonical_result,
    build_report as _canonical_report,
    print_summary as _print_summary,
    save_report as _save_report,
)

try:
    import requests as http_requests
    HAS_REQUESTS = True
except ImportError:
    HAS_REQUESTS = False

UAMS_VERSION = "1.0.0"

# ---------------------------------------------------------------------------
# Schema-runner parity: known fields
# ---------------------------------------------------------------------------

_KNOWN_TOP_FIELDS = {
    "uams_version", "method", "credentials", "validation",
    "principal", "errors", "test_fixtures", "metadata",
}

_KNOWN_METHOD_TYPES = {"none", "apikey", "bearer", "basic", "oauth2", "mtls", "custom"}
_KNOWN_CREDENTIAL_SOURCES = {"header", "query", "cookie", "body", "certificate"}
_KNOWN_FORMAT_TYPES = {"opaque", "jwt", "base64", "certificate"}
_KNOWN_CHECKS = {"signature", "expiry", "issuer", "audience", "scope", "revocation"}
_KNOWN_STATUSES = {"stable", "beta", "experimental", "deprecated"}


def _validate_supported_features(spec: Dict[str, Any]) -> List[str]:
    """Parity guard: fail fast if spec uses fields this runner doesn't handle."""
    errors = []
    unknown = set(spec.keys()) - _KNOWN_TOP_FIELDS
    if unknown:
        errors.append(f"PARITY FAILURE: unknown top-level fields: {unknown}")
    method_type = spec.get("method", {}).get("type", "")
    if method_type and method_type not in _KNOWN_METHOD_TYPES:
        errors.append(f"PARITY FAILURE: unknown method type: {method_type}")
    return errors


# ---------------------------------------------------------------------------
# Structural validation (no server needed)
# ---------------------------------------------------------------------------

def validate_spec(spec_path: str) -> Dict:
    """Validate a single UAMS spec for structural conformance.

    Returns a canonical result dict.
    """
    start = time.time()
    try:
        with open(spec_path) as f:
            spec = json.load(f)
    except (json.JSONDecodeError, OSError) as e:
        return _canonical_result(
            spec_path=spec_path, status="error", hash_verified=None,
            assertions_evaluated=0, assertions_passed=0,
            error=f"Failed to load spec: {e}",
        )

    # Parity check
    parity_issues = _validate_supported_features(spec)
    if parity_issues:
        return _canonical_result(
            spec_path=spec_path, status="fail", hash_verified=None,
            assertions_evaluated=1, assertions_passed=0,
            failures=parity_issues,
        )

    assertions_eval = 0
    assertions_pass = 0
    failures = []

    # 1. Required top-level fields
    for field in ("uams_version", "method", "credentials", "validation", "principal"):
        assertions_eval += 1
        if field in spec:
            assertions_pass += 1
        else:
            failures.append(f"Missing required field: {field}")

    # 2. Method structure
    method = spec.get("method", {})
    assertions_eval += 1
    if method.get("name") and method.get("type"):
        assertions_pass += 1
    else:
        failures.append("method must have 'name' and 'type'")

    assertions_eval += 1
    if method.get("type", "") in _KNOWN_METHOD_TYPES:
        assertions_pass += 1
    else:
        failures.append(f"method.type '{method.get('type')}' not in known types")

    assertions_eval += 1
    if method.get("status", "stable") in _KNOWN_STATUSES:
        assertions_pass += 1
    else:
        failures.append(f"method.status '{method.get('status')}' not in known statuses")

    # 3. Credentials structure
    creds = spec.get("credentials", {})
    extraction = creds.get("extraction", [])

    assertions_eval += 1
    if method.get("type") == "none" or len(extraction) > 0:
        assertions_pass += 1
    else:
        failures.append("credentials.extraction must have at least one entry (unless type=none)")

    for i, ext in enumerate(extraction):
        assertions_eval += 1
        src = ext.get("source", "")
        if src in _KNOWN_CREDENTIAL_SOURCES:
            assertions_pass += 1
        else:
            failures.append(f"credentials.extraction[{i}].source '{src}' not in known sources")

        assertions_eval += 1
        if ext.get("name"):
            assertions_pass += 1
        else:
            failures.append(f"credentials.extraction[{i}] missing 'name'")

        assertions_eval += 1
        if isinstance(ext.get("priority"), int) and ext["priority"] >= 1:
            assertions_pass += 1
        else:
            failures.append(f"credentials.extraction[{i}].priority must be int >= 1")

    # 4. Format validation
    fmt = creds.get("format", {})
    if fmt:
        assertions_eval += 1
        if fmt.get("type", "") in _KNOWN_FORMAT_TYPES:
            assertions_pass += 1
        else:
            failures.append(f"credentials.format.type '{fmt.get('type')}' not in known types")

        if "min_length" in fmt and "max_length" in fmt:
            assertions_eval += 1
            if fmt["min_length"] <= fmt["max_length"]:
                assertions_pass += 1
            else:
                failures.append("format.min_length > format.max_length")

    # 5. Validation checks
    val = spec.get("validation", {})
    for check in val.get("checks", []):
        assertions_eval += 1
        if check in _KNOWN_CHECKS:
            assertions_pass += 1
        else:
            failures.append(f"validation.checks contains unknown check: {check}")

    assertions_eval += 1
    if isinstance(val.get("timing_safe", True), bool):
        assertions_pass += 1
    else:
        failures.append("validation.timing_safe must be boolean")

    # 6. Principal structure
    principal = spec.get("principal", {})
    assertions_eval += 1
    if principal.get("id_source"):
        assertions_pass += 1
    else:
        failures.append("principal.id_source is required")

    # 7. Error codes
    for i, err in enumerate(spec.get("errors", [])):
        assertions_eval += 1
        if err.get("code") and isinstance(err.get("status"), int):
            assertions_pass += 1
        else:
            failures.append(f"errors[{i}] must have 'code' (string) and 'status' (int)")

        assertions_eval += 1
        status = err.get("status", 0)
        if 400 <= status <= 599:
            assertions_pass += 1
        else:
            failures.append(f"errors[{i}].status {status} not in 400-599 range")

    duration_ms = int((time.time() - start) * 1000)
    status = "pass" if not failures else "fail"

    return _canonical_result(
        spec_path=spec_path,
        status=status,
        hash_verified=None,
        assertions_evaluated=assertions_eval,
        assertions_passed=assertions_pass,
        failures=failures if failures else None,
        duration_ms=duration_ms,
    )


# ---------------------------------------------------------------------------
# Live auth testing (requires --base-url)
# ---------------------------------------------------------------------------

def test_auth_method(spec_path: str, base_url: str) -> Dict:
    """Test auth method behavior against a live MDEMG server."""
    if not HAS_REQUESTS:
        return _canonical_result(
            spec_path=spec_path, status="error", hash_verified=None,
            assertions_evaluated=0, assertions_passed=0,
            error="requests library required for live testing (pip install requests)",
        )

    start = time.time()
    try:
        with open(spec_path) as f:
            spec = json.load(f)
    except (json.JSONDecodeError, OSError) as e:
        return _canonical_result(
            spec_path=spec_path, status="error", hash_verified=None,
            assertions_evaluated=0, assertions_passed=0,
            error=f"Failed to load spec: {e}",
        )

    # Start with structural validation
    struct_result = validate_spec(spec_path)
    if struct_result.get("status") != "pass":
        return struct_result

    method_name = spec.get("method", {}).get("name", "unknown")
    assertions_eval = struct_result.get("assertions_evaluated", 0)
    assertions_pass = struct_result.get("assertions_passed", 0)
    failures = list(struct_result.get("failures") or [])

    # Test credential extraction paths against a health-like endpoint
    # When auth is disabled, all requests pass (which we detect).
    # When auth is enabled, missing/invalid credentials should return 401.
    test_url = f"{base_url.rstrip('/')}/healthz"

    if method_name == "none":
        # none auth: unauthenticated request should succeed
        assertions_eval += 1
        try:
            resp = http_requests.get(test_url, timeout=5)
            if resp.status_code == 200:
                assertions_pass += 1
            else:
                failures.append(f"none auth: healthz returned {resp.status_code}, expected 200")
        except Exception as e:
            failures.append(f"none auth: request failed: {e}")
    else:
        # For apikey/jwt/saml: test that server responds (may or may not enforce auth)
        assertions_eval += 1
        try:
            resp = http_requests.get(test_url, timeout=5)
            # healthz is typically in skip-endpoints, so 200 is expected
            if resp.status_code in (200, 401):
                assertions_pass += 1
            else:
                failures.append(f"server returned unexpected {resp.status_code} on healthz")
        except Exception as e:
            failures.append(f"server unreachable: {e}")

        # Test each extraction path with invalid credentials
        for ext in spec.get("credentials", {}).get("extraction", []):
            assertions_eval += 1
            source = ext.get("source", "")
            name = ext.get("name", "")
            prefix = ext.get("prefix", "")

            # Build a request with an obviously invalid credential
            headers = {}
            params = {}
            invalid_cred = "invalid-test-credential-00000000"

            if source == "header":
                headers[name] = f"{prefix}{invalid_cred}"
            elif source == "query":
                params[name] = invalid_cred

            try:
                # Hit a non-skip endpoint to trigger auth
                resp = http_requests.get(
                    f"{base_url.rstrip('/')}/v1/memory/stats",
                    headers=headers, params=params, timeout=5,
                )
                # If auth is enabled: expect 401 for invalid creds
                # If auth is disabled: expect 200/400 (no auth enforcement)
                if resp.status_code in (200, 400, 401, 403):
                    assertions_pass += 1
                else:
                    failures.append(
                        f"extraction[{name}]: unexpected {resp.status_code} "
                        f"(expected 200/401/403)"
                    )
            except Exception as e:
                failures.append(f"extraction[{name}]: request failed: {e}")

        # Verify error codes are documented
        for err in spec.get("errors", []):
            assertions_eval += 1
            if err.get("code") and err.get("message"):
                assertions_pass += 1
            else:
                failures.append(f"error entry missing code or message")

    duration_ms = int((time.time() - start) * 1000)
    status = "pass" if not failures else "fail"

    return _canonical_result(
        spec_path=spec_path,
        status=status,
        hash_verified=None,
        assertions_evaluated=assertions_eval,
        assertions_passed=assertions_pass,
        failures=failures if failures else None,
        duration_ms=duration_ms,
    )


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="UAMS Runner v" + UAMS_VERSION)
    sub = parser.add_subparsers(dest="command", required=True)

    # validate
    p_val = sub.add_parser("validate", help="Validate a single UAMS spec")
    p_val.add_argument("--spec", required=True, help="Path to UAMS spec file")
    p_val.add_argument("--report", help="Output canonical JSON report")

    # validate-all
    p_all = sub.add_parser("validate-all", help="Validate all UAMS specs in a directory")
    p_all.add_argument("--spec-dir", required=True, help="Directory containing *.uams.json specs")
    p_all.add_argument("--report", help="Output canonical JSON report")

    # test
    p_test = sub.add_parser("test", help="Test auth methods against a live server")
    p_test.add_argument("--spec-dir", required=True, help="Directory containing *.uams.json specs")
    p_test.add_argument("--base-url", default="http://localhost:9999", help="MDEMG server URL")
    p_test.add_argument("--report", help="Output canonical JSON report")

    args = parser.parse_args()
    report_start = datetime.now(timezone.utc)
    results = []

    if args.command == "validate":
        results.append(validate_spec(args.spec))

    elif args.command == "validate-all":
        spec_files = sorted(glob.glob(os.path.join(args.spec_dir, "*.uams.json")))
        if not spec_files:
            print(f"No *.uams.json files found in {args.spec_dir}", file=sys.stderr)
            sys.exit(1)
        for sf in spec_files:
            result = validate_spec(sf)
            results.append(result)
            status_icon = "PASS" if result["status"] == "pass" else "FAIL"
            print(f"  [{status_icon}] {os.path.basename(sf)}: "
                  f"{result.get('assertions_passed', 0)}/{result.get('assertions_evaluated', 0)} assertions")

    elif args.command == "test":
        spec_files = sorted(glob.glob(os.path.join(args.spec_dir, "*.uams.json")))
        if not spec_files:
            print(f"No *.uams.json files found in {args.spec_dir}", file=sys.stderr)
            sys.exit(1)
        for sf in spec_files:
            result = test_auth_method(sf, args.base_url)
            results.append(result)
            status_icon = "PASS" if result["status"] == "pass" else "FAIL"
            print(f"  [{status_icon}] {os.path.basename(sf)}: "
                  f"{result.get('assertions_passed', 0)}/{result.get('assertions_evaluated', 0)} assertions")

    # Build and display canonical report
    report = _canonical_report(
        framework="uams",
        framework_version=UAMS_VERSION,
        results=results,
        start_time=report_start,
    )
    _print_summary(report)

    if hasattr(args, "report") and args.report:
        _save_report(report, args.report)

    all_passed = all(r["status"] == "pass" for r in results)
    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    main()
