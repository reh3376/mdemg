#!/usr/bin/env python3
"""
USTS Runner - Universal Security Test Specification Runner

Executes USTS security test specifications against MDEMG endpoints.

Version: 1.1.0

Usage:
    python usts_runner.py --spec specs/auth_required.usts.json
    python usts_runner.py --spec "specs/*.usts.json" --report results/usts_report.json
"""

import argparse
import json
import os
import re
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional
from urllib.parse import urljoin

import sys as _sys
_sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..'))
from uxts_report import build_result as _canonical_result, build_report as _canonical_report, print_summary as _print_summary, save_report as _save_report

import requests

USTS_VERSION = "1.1.0"


# --------------------------------------------------------------------------- #
# Schema-runner parity: hard-fail for unimplemented spec fields               #
# --------------------------------------------------------------------------- #

_USTS_KNOWN_TOP_FIELDS = {
    "usts_version", "test", "requests", "assertions", "metadata",
    # Advisory fields (present in schema, not enforced by this runner):
    "$schema", "config", "setup", "test_cases",
}
_USTS_KNOWN_ASSERTION_TYPES = {
    "status_code", "status_in", "body_contains", "body_not_contains",
    "headers_present", "headers_not_present", "response_time_ms_max",
}


def _validate_supported_features(spec_dict: Dict[str, Any]) -> List[str]:
    """Fail fast when a spec uses fields or assertion types this runner
    does not implement.

    Returns a list of error strings prefixed with 'PARITY FAILURE:'.
    Non-empty means the spec MUST be marked as fail.
    """
    errors: List[str] = []

    unknown_top = set(spec_dict.keys()) - _USTS_KNOWN_TOP_FIELDS
    if unknown_top:
        errors.append(f"PARITY FAILURE: Unimplemented top-level fields: {unknown_top}")

    # test_cases format is recognised but not executable — require 'test' key
    if "test_cases" in spec_dict and "test" not in spec_dict:
        errors.append(
            "PARITY FAILURE: test_cases format is not implemented. "
            "Runner requires 'test' + 'requests' format."
        )

    # Authentication tests require auth middleware on the server.
    # Set USTS_AUTH_ENABLED=true when auth is deployed.
    test_category = spec_dict.get("test", {}).get("category", "")
    if test_category == "authentication" and not os.environ.get("USTS_AUTH_ENABLED"):
        errors.append(
            "PARITY FAILURE: Authentication test requires auth middleware. "
            "Set USTS_AUTH_ENABLED=true when auth is deployed."
        )

    # Check assertion types
    for atype in spec_dict.get("assertions", {}):
        if atype not in _USTS_KNOWN_ASSERTION_TYPES:
            errors.append(f"PARITY FAILURE: Unimplemented assertion type: {atype}")

    return errors


@dataclass
class TestResult:
    """Result of a single security test."""
    name: str
    passed: bool
    status_code: int
    response_time_ms: float
    assertions_passed: Dict[str, bool] = field(default_factory=dict)
    failures: List[str] = field(default_factory=list)
    response_body: str = ""

@dataclass
class SecurityTestResult:
    """Results from a complete security test specification."""
    spec_name: str
    category: str
    severity: str
    total_requests: int
    passed_requests: int
    failed_requests: int
    test_results: List[TestResult] = field(default_factory=list)
    start_time: datetime = field(default_factory=datetime.now)
    end_time: Optional[datetime] = None

    @property
    def passed(self) -> bool:
        return self.failed_requests == 0

    def to_dict(self) -> Dict[str, Any]:
        return {
            "spec_name": self.spec_name,
            "category": self.category,
            "severity": self.severity,
            "passed": self.passed,
            "total_requests": self.total_requests,
            "passed_requests": self.passed_requests,
            "failed_requests": self.failed_requests,
            "test_results": [
                {
                    "name": r.name,
                    "passed": r.passed,
                    "status_code": r.status_code,
                    "response_time_ms": r.response_time_ms,
                    "assertions_passed": r.assertions_passed,
                    "failures": r.failures,
                }
                for r in self.test_results
            ],
            "start_time": self.start_time.isoformat(),
            "end_time": self.end_time.isoformat() if self.end_time else None,
        }


def load_spec(spec_path: str) -> Dict[str, Any]:
    """Load a USTS specification file."""
    with open(spec_path) as f:
        return json.load(f)


def load_payload_file(payload_path: str, base_dir: str) -> List[str]:
    """Load payloads from a file."""
    full_path = Path(base_dir) / payload_path
    if not full_path.exists():
        return []
    with open(full_path) as f:
        return [line.strip() for line in f if line.strip() and not line.startswith("#")]


def render_variables(obj: Any, variables: Dict[str, str]) -> Any:
    """Replace {{variable}} placeholders with values."""
    if isinstance(obj, str):
        for key, value in variables.items():
            obj = obj.replace(f"{{{{{key}}}}}", value)
        return obj
    elif isinstance(obj, dict):
        return {k: render_variables(v, variables) for k, v in obj.items()}
    elif isinstance(obj, list):
        return [render_variables(item, variables) for item in obj]
    return obj


def check_assertion(assertion_name: str, assertion_value: Any, response: requests.Response) -> tuple[bool, str]:
    """Check a single assertion against a response. Returns (passed, failure_message)."""
    body = response.text.lower()

    if assertion_name == "status_code":
        if response.status_code != assertion_value:
            return False, f"Expected status {assertion_value}, got {response.status_code}"
        return True, ""

    if assertion_name == "status_in":
        if response.status_code not in assertion_value:
            return False, f"Expected status in {assertion_value}, got {response.status_code}"
        return True, ""

    if assertion_name == "body_contains":
        for pattern in assertion_value:
            if pattern.lower() not in body:
                return False, f"Body does not contain: {pattern}"
        return True, ""

    if assertion_name == "body_not_contains":
        for pattern in assertion_value:
            if pattern.lower() in body:
                return False, f"Body contains forbidden pattern: {pattern}"
        return True, ""

    if assertion_name == "headers_present":
        for header in assertion_value:
            if header.lower() not in [h.lower() for h in response.headers.keys()]:
                return False, f"Missing required header: {header}"
        return True, ""

    if assertion_name == "headers_not_present":
        for header in assertion_value:
            if header.lower() in [h.lower() for h in response.headers.keys()]:
                return False, f"Forbidden header present: {header}"
        return True, ""

    if assertion_name == "response_time_ms_max":
        actual = response.elapsed.total_seconds() * 1000
        if actual > assertion_value:
            return False, f"Response time {actual:.0f}ms exceeds max {assertion_value}ms"
        return True, ""

    # Unknown assertion type -- hard-fail to prevent false passes
    return False, f"PARITY FAILURE: Unimplemented assertion type: {assertion_name}"


def run_request(
    base_url: str,
    test_spec: Dict[str, Any],
    request_spec: Dict[str, Any],
    assertions: Dict[str, Any],
    variables: Dict[str, str],
) -> TestResult:
    """Execute a single test request and check assertions."""
    endpoint = test_spec["endpoint"]
    method = test_spec.get("method", "POST")

    # Merge and render headers
    headers = {"Content-Type": "application/json"}
    headers.update(request_spec.get("headers", {}))
    headers = render_variables(headers, variables)

    # Render payload
    payload = render_variables(request_spec.get("payload", {}), variables)

    url = urljoin(base_url, endpoint)

    result = TestResult(
        name=request_spec["name"],
        passed=True,
        status_code=0,
        response_time_ms=0,
    )

    try:
        start = time.perf_counter()
        if method == "GET":
            resp = requests.get(url, headers=headers, params=payload, timeout=30)
        elif method == "POST":
            resp = requests.post(url, headers=headers, json=payload, timeout=30)
        elif method == "PUT":
            resp = requests.put(url, headers=headers, json=payload, timeout=30)
        elif method == "DELETE":
            resp = requests.delete(url, headers=headers, timeout=30)
        elif method == "OPTIONS":
            resp = requests.options(url, headers=headers, timeout=30)
        else:
            result.passed = False
            result.failures.append(f"Unsupported method: {method}")
            return result

        result.response_time_ms = (time.perf_counter() - start) * 1000
        result.status_code = resp.status_code
        result.response_body = resp.text[:500]  # Truncate for storage

        # Check expected status from request spec
        if "expected_status" in request_spec:
            if resp.status_code != request_spec["expected_status"]:
                result.passed = False
                result.failures.append(
                    f"Expected status {request_spec['expected_status']}, got {resp.status_code}"
                )

        if "expected_status_range" in request_spec:
            low, high = request_spec["expected_status_range"]
            if not (low <= resp.status_code <= high):
                result.passed = False
                result.failures.append(
                    f"Expected status in range [{low}, {high}], got {resp.status_code}"
                )

        # Check global assertions
        for assertion_name, assertion_value in assertions.items():
            passed, failure = check_assertion(assertion_name, assertion_value, resp)
            result.assertions_passed[assertion_name] = passed
            if not passed:
                result.passed = False
                result.failures.append(failure)

    except requests.RequestException as e:
        result.passed = False
        result.failures.append(f"Request failed: {e}")

    return result


def run_security_test(
    spec: Dict[str, Any],
    base_url: str,
    variables: Dict[str, str],
) -> SecurityTestResult:
    """Run all requests in a security test specification."""
    test_info = spec["test"]
    assertions = spec.get("assertions", {})
    requests_list = spec.get("requests", [{}])

    result = SecurityTestResult(
        spec_name=test_info["name"],
        category=test_info["category"],
        severity=test_info.get("severity", "medium"),
        total_requests=len(requests_list),
        passed_requests=0,
        failed_requests=0,
    )

    for request_spec in requests_list:
        test_result = run_request(base_url, test_info, request_spec, assertions, variables)
        result.test_results.append(test_result)

        if test_result.passed:
            result.passed_requests += 1
        else:
            result.failed_requests += 1

    result.end_time = datetime.now()
    return result


def print_results(result: SecurityTestResult) -> None:
    """Print security test results."""
    severity_colors = {
        "critical": "\033[91m",  # Red
        "high": "\033[93m",      # Yellow
        "medium": "\033[94m",    # Blue
        "low": "\033[92m",       # Green
    }
    reset = "\033[0m"
    color = severity_colors.get(result.severity, "")

    status = "\u2713 PASS" if result.passed else "\u2717 FAIL"
    status_color = "\033[92m" if result.passed else "\033[91m"

    print(f"\n{'='*60}")
    print(f"Test: {result.spec_name}")
    print(f"Category: {result.category}")
    print(f"Severity: {color}{result.severity.upper()}{reset}")
    print(f"Status: {status_color}{status}{reset}")
    print(f"{'='*60}")

    for tr in result.test_results:
        tr_status = "\u2713" if tr.passed else "\u2717"
        tr_color = "\033[92m" if tr.passed else "\033[91m"
        print(f"  {tr_color}{tr_status}{reset} {tr.name} (HTTP {tr.status_code}, {tr.response_time_ms:.0f}ms)")

        for failure in tr.failures:
            print(f"      \033[91m{failure}{reset}")


def main():
    parser = argparse.ArgumentParser(description="USTS Security Test Runner")
    parser.add_argument("--spec", required=True, help="Path to USTS spec file(s)")
    parser.add_argument("--base-url", default="http://localhost:9999", help="MDEMG base URL")
    parser.add_argument("--api-key", help="Valid API key for authenticated tests")
    parser.add_argument("--report", help="Output file path for canonical JSON report")
    args = parser.parse_args()

    # Set up variables for template rendering
    variables = {}
    if args.api_key:
        variables["valid_api_key"] = args.api_key

    # Handle glob patterns in spec
    spec_paths = []
    if "*" in args.spec:
        spec_dir = Path(args.spec).parent
        pattern = Path(args.spec).name
        spec_paths = list(spec_dir.glob(pattern))
    else:
        spec_paths = [Path(args.spec)]

    all_passed = True
    canonical_results = []
    report_start = datetime.now(timezone.utc)

    for spec_path in spec_paths:
        print(f"\nLoading spec: {spec_path}")
        spec = load_spec(spec_path)

        # --- Schema-runner parity: hard-fail for unimplemented fields ---
        parity_errors = _validate_supported_features(spec)
        if parity_errors:
            print("\n  Schema-runner parity FAILURES:")
            for e in parity_errors:
                print(f"    {e}")
            canonical_results.append(_canonical_result(
                spec_path=str(spec_path),
                status="fail",
                failures=parity_errors,
                hash_verified=None,
            ))
            all_passed = False
            continue

        result = run_security_test(spec, args.base_url, variables)
        print_results(result)

        if not result.passed:
            all_passed = False

        # Map SecurityTestResult to canonical fields
        # Count total assertions: each request checks global assertions + expected_status checks
        total_assertions = 0
        passed_assertions = 0
        failure_details = []

        for tr in result.test_results:
            # Count assertion checks from assertions_passed dict
            total_assertions += len(tr.assertions_passed)
            passed_assertions += sum(1 for v in tr.assertions_passed.values() if v)

            # Count expected_status / expected_status_range as assertions too
            # (they are checked in run_request but not tracked in assertions_passed)
            # We detect them from failures
            for f in tr.failures:
                if f.startswith("Expected status"):
                    total_assertions += 1
                    # It failed, so don't increment passed

            if tr.failures:
                for f in tr.failures:
                    failure_details.append(f"[{tr.name}] {f}")

        # If a request had expected_status and it passed, count it
        for rspec in spec.get("requests", [{}]):
            if "expected_status" in rspec or "expected_status_range" in rspec:
                total_assertions += 1
                # Check if this request's status check passed (no status failure in results)
                rname = rspec.get("name", "")
                status_failed = any(
                    f.startswith("Expected status")
                    for tr in result.test_results if tr.name == rname
                    for f in tr.failures
                )
                if not status_failed:
                    passed_assertions += 1

        # Compute duration_ms
        duration_ms = 0.0
        if result.end_time and result.start_time:
            duration_ms = (result.end_time - result.start_time).total_seconds() * 1000

        spec_status = "pass" if result.passed else "fail"
        canonical_results.append(_canonical_result(
            spec_path=str(spec_path),
            status=spec_status,
            duration_ms=duration_ms,
            hash_verified=None,
            assertions_evaluated=total_assertions,
            assertions_passed=passed_assertions,
            failures=failure_details if failure_details else None,
        ))

    # Build canonical report
    report = _canonical_report(
        framework="usts",
        framework_version=USTS_VERSION,
        results=canonical_results,
        start_time=report_start,
    )

    _print_summary(report)

    if args.report:
        _save_report(report, args.report)

    # Summary (keep existing severity-based summary)
    critical_failures = sum(
        1 for r in canonical_results
        if r["status"] == "fail"
    )

    print(f"\n{'='*60}")
    print(f"Overall: {'PASS' if all_passed else 'FAIL'}")
    print(f"{'='*60}")

    # Exit with error if any tests failed
    if not all_passed:
        sys.exit(1)
    else:
        sys.exit(0)


if __name__ == "__main__":
    main()
