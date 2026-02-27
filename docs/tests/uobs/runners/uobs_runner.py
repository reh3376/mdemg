#!/usr/bin/env python3
"""
UOBS Runner - Universal Observability Specification Runner

Validates observability infrastructure against UOBS specifications.

Usage:
    python uobs_runner.py --spec specs/prometheus_metrics.uobs.json
    python uobs_runner.py --spec "specs/*.uobs.json" --output results/
"""

import argparse
import json
import re
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional
from urllib.parse import urljoin

import requests

@dataclass
class CheckResult:
    """Result of a single observability check."""
    name: str
    passed: bool
    message: str
    details: Optional[Dict[str, Any]] = None

@dataclass
class ObservabilityTestResult:
    """Results from a complete observability test."""
    spec_name: str
    test_type: str
    total_checks: int
    passed_checks: int
    failed_checks: int
    results: List[CheckResult] = field(default_factory=list)
    start_time: datetime = field(default_factory=datetime.now)
    end_time: Optional[datetime] = None

    @property
    def passed(self) -> bool:
        return self.failed_checks == 0

    def to_dict(self) -> Dict[str, Any]:
        return {
            "spec_name": self.spec_name,
            "test_type": self.test_type,
            "passed": self.passed,
            "total_checks": self.total_checks,
            "passed_checks": self.passed_checks,
            "failed_checks": self.failed_checks,
            "results": [
                {
                    "name": r.name,
                    "passed": r.passed,
                    "message": r.message,
                    "details": r.details,
                }
                for r in self.results
            ],
            "start_time": self.start_time.isoformat(),
            "end_time": self.end_time.isoformat() if self.end_time else None,
        }


def load_spec(spec_path: str) -> Dict[str, Any]:
    """Load a UOBS specification file."""
    with open(spec_path) as f:
        return json.load(f)


def validate_prometheus_format(content: str) -> tuple[bool, str]:
    """Validate Prometheus exposition format."""
    lines = content.strip().split("\n")
    errors = []

    for i, line in enumerate(lines, 1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue

        # Basic format: metric_name{labels} value
        # or: metric_name value
        if not re.match(r'^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[^}]*\})?\s+[\d.eE+-]+$', line):
            errors.append(f"Line {i}: Invalid format: {line[:50]}")

    if errors:
        return False, "; ".join(errors[:5])
    return True, "Valid Prometheus format"


def check_metric_exists(content: str, metric_name: str, metric_type: Optional[str] = None) -> tuple[bool, str]:
    """Check if a metric exists in Prometheus output."""
    # Check for TYPE declaration
    type_pattern = f"# TYPE {metric_name}"
    has_type = type_pattern in content

    # Check for actual metric values
    # For histograms/summaries, also check for _bucket, _sum, _count suffixes
    metric_patterns = [f"^{metric_name}(\\{{|\\s)"]
    if metric_type in ("histogram", "summary"):
        metric_patterns.extend([
            f"^{metric_name}_bucket(\\{{|\\s)",
            f"^{metric_name}_sum(\\{{|\\s)",
            f"^{metric_name}_count(\\{{|\\s)",
        ])

    has_values = False
    for pattern in metric_patterns:
        if any(re.match(pattern, line) for line in content.split("\n")):
            has_values = True
            break

    if not has_values:
        return False, f"Metric {metric_name} not found"

    if metric_type:
        expected_type = f"# TYPE {metric_name} {metric_type}"
        if expected_type not in content:
            return False, f"Metric {metric_name} exists but type mismatch (expected {metric_type})"

    return True, f"Metric {metric_name} present"


def run_metrics_test(spec: Dict[str, Any], base_url: str) -> ObservabilityTestResult:
    """Run Prometheus metrics validation."""
    metrics_config = spec.get("metrics", {})
    endpoint = metrics_config.get("endpoint", "/v1/prometheus")
    required_metrics = metrics_config.get("required_metrics", [])

    result = ObservabilityTestResult(
        spec_name=spec["test"]["name"],
        test_type="metrics",
        total_checks=len(required_metrics) + 1,  # +1 for format check
        passed_checks=0,
        failed_checks=0,
    )

    # Fetch metrics
    try:
        url = urljoin(base_url, endpoint)
        resp = requests.get(url, timeout=10)
        if resp.status_code != 200:
            result.failed_checks = result.total_checks
            result.results.append(CheckResult(
                name="fetch_metrics",
                passed=False,
                message=f"Failed to fetch metrics: HTTP {resp.status_code}",
            ))
            result.end_time = datetime.now()
            return result

        content = resp.text

    except requests.RequestException as e:
        result.failed_checks = result.total_checks
        result.results.append(CheckResult(
            name="fetch_metrics",
            passed=False,
            message=f"Request failed: {e}",
        ))
        result.end_time = datetime.now()
        return result

    # Check format
    if metrics_config.get("format_validation", True):
        valid, msg = validate_prometheus_format(content)
        result.results.append(CheckResult(
            name="prometheus_format",
            passed=valid,
            message=msg,
        ))
        if valid:
            result.passed_checks += 1
        else:
            result.failed_checks += 1

    # Check required metrics
    for metric in required_metrics:
        name = metric["name"]
        metric_type = metric.get("type")

        exists, msg = check_metric_exists(content, name, metric_type)
        result.results.append(CheckResult(
            name=f"metric_{name}",
            passed=exists,
            message=msg,
            details={"metric": name, "type": metric_type},
        ))
        if exists:
            result.passed_checks += 1
        else:
            result.failed_checks += 1

    result.end_time = datetime.now()
    return result


def run_health_test(spec: Dict[str, Any], base_url: str) -> ObservabilityTestResult:
    """Run health endpoint validation."""
    health_config = spec.get("health", {})
    endpoints = health_config.get("endpoints", [])

    result = ObservabilityTestResult(
        spec_name=spec["test"]["name"],
        test_type="health",
        total_checks=len(endpoints),
        passed_checks=0,
        failed_checks=0,
    )

    for ep in endpoints:
        path = ep["path"]
        method = ep.get("method", "GET")
        expected_status = ep.get("expected_status", 200)
        required_fields = ep.get("required_fields", [])
        max_time = ep.get("max_response_time_ms")

        try:
            url = urljoin(base_url, path)
            start = time.perf_counter()
            resp = requests.request(method, url, timeout=10)
            elapsed_ms = (time.perf_counter() - start) * 1000

            passed = True
            messages = []

            # Check status
            if resp.status_code != expected_status:
                passed = False
                messages.append(f"Expected status {expected_status}, got {resp.status_code}")

            # Check response time
            if max_time and elapsed_ms > max_time:
                passed = False
                messages.append(f"Response time {elapsed_ms:.0f}ms exceeds {max_time}ms")

            # Check required fields
            try:
                body = resp.json()
                for field in required_fields:
                    if field not in body:
                        passed = False
                        messages.append(f"Missing required field: {field}")
            except json.JSONDecodeError:
                if required_fields:
                    passed = False
                    messages.append("Response is not valid JSON")

            result.results.append(CheckResult(
                name=f"health_{path}",
                passed=passed,
                message="; ".join(messages) if messages else f"OK ({elapsed_ms:.0f}ms)",
                details={"path": path, "status": resp.status_code, "time_ms": elapsed_ms},
            ))

            if passed:
                result.passed_checks += 1
            else:
                result.failed_checks += 1

        except requests.RequestException as e:
            result.results.append(CheckResult(
                name=f"health_{path}",
                passed=False,
                message=f"Request failed: {e}",
            ))
            result.failed_checks += 1

    result.end_time = datetime.now()
    return result


def run_tracing_test(spec: Dict[str, Any], base_url: str) -> ObservabilityTestResult:
    """Run distributed tracing validation."""
    tracing_config = spec.get("tracing", {})
    trace_header = tracing_config.get("trace_header", "X-Trace-ID")
    request_id_header = tracing_config.get("request_id_header", "X-Request-ID")

    result = ObservabilityTestResult(
        spec_name=spec["test"]["name"],
        test_type="tracing",
        total_checks=3,
        passed_checks=0,
        failed_checks=0,
    )

    # Test 1: Check headers are returned
    try:
        url = urljoin(base_url, "/healthz")
        resp = requests.get(url, timeout=10)

        # Check trace ID is returned
        if trace_header.lower() in [h.lower() for h in resp.headers.keys()]:
            result.results.append(CheckResult(
                name="trace_id_returned",
                passed=True,
                message=f"{trace_header} header present",
            ))
            result.passed_checks += 1
        else:
            result.results.append(CheckResult(
                name="trace_id_returned",
                passed=False,
                message=f"{trace_header} header not returned",
            ))
            result.failed_checks += 1

        # Check request ID is returned
        if request_id_header.lower() in [h.lower() for h in resp.headers.keys()]:
            result.results.append(CheckResult(
                name="request_id_returned",
                passed=True,
                message=f"{request_id_header} header present",
            ))
            result.passed_checks += 1
        else:
            result.results.append(CheckResult(
                name="request_id_returned",
                passed=False,
                message=f"{request_id_header} header not returned",
            ))
            result.failed_checks += 1

        # Test 2: Trace ID propagation (send trace ID, verify it's echoed)
        if tracing_config.get("propagation_check", True):
            test_trace_id = "test-trace-12345"
            resp = requests.get(url, headers={trace_header: test_trace_id}, timeout=10)

            returned_trace = resp.headers.get(trace_header, "")
            if returned_trace == test_trace_id:
                result.results.append(CheckResult(
                    name="trace_id_propagation",
                    passed=True,
                    message="Trace ID propagated correctly",
                ))
                result.passed_checks += 1
            else:
                result.results.append(CheckResult(
                    name="trace_id_propagation",
                    passed=False,
                    message=f"Trace ID not propagated (sent {test_trace_id}, got {returned_trace})",
                ))
                result.failed_checks += 1

    except requests.RequestException as e:
        result.results.append(CheckResult(
            name="tracing_test",
            passed=False,
            message=f"Request failed: {e}",
        ))
        result.failed_checks = result.total_checks

    result.end_time = datetime.now()
    return result


def run_dependency_test(spec: Dict[str, Any], base_url: str) -> ObservabilityTestResult:
    """Run dependency health validation (embedding, database, cache, etc.)."""
    dep_config = spec.get("dependency", {})
    service = dep_config.get("service", "unknown")
    endpoint = dep_config.get("endpoint", f"/v1/{service}/health")
    thresholds = dep_config.get("thresholds", {})
    active_check = dep_config.get("active_check", {})
    config_validation = dep_config.get("configuration_validation", {})
    circuit_breaker = dep_config.get("circuit_breaker", {})

    total_checks = 2  # Basic checks: connectivity + response format
    if active_check.get("enabled", False):
        total_checks += 1
    if thresholds:
        total_checks += len(thresholds)
    if config_validation:
        total_checks += 1
    if circuit_breaker.get("check_state", False):
        total_checks += 1

    result = ObservabilityTestResult(
        spec_name=spec["test"]["name"],
        test_type="dependency",
        total_checks=total_checks,
        passed_checks=0,
        failed_checks=0,
    )

    # Fetch health endpoint
    try:
        url = urljoin(base_url, endpoint)
        start = time.perf_counter()
        resp = requests.get(url, timeout=10)
        elapsed_ms = (time.perf_counter() - start) * 1000

        # Check 1: Connectivity
        if resp.status_code == 200:
            result.results.append(CheckResult(
                name=f"{service}_connectivity",
                passed=True,
                message=f"Endpoint reachable ({elapsed_ms:.0f}ms)",
            ))
            result.passed_checks += 1
        else:
            result.results.append(CheckResult(
                name=f"{service}_connectivity",
                passed=False,
                message=f"HTTP {resp.status_code}",
            ))
            result.failed_checks += 1
            result.end_time = datetime.now()
            return result

        # Parse response
        try:
            body = resp.json()
        except json.JSONDecodeError:
            result.results.append(CheckResult(
                name=f"{service}_response_format",
                passed=False,
                message="Response is not valid JSON",
            ))
            result.failed_checks += 1
            result.end_time = datetime.now()
            return result

        # Check 2: Status field
        status = body.get("status", "unknown")
        if status == "healthy":
            result.results.append(CheckResult(
                name=f"{service}_status",
                passed=True,
                message=f"Status: {status}",
            ))
            result.passed_checks += 1
        elif status == "degraded":
            result.results.append(CheckResult(
                name=f"{service}_status",
                passed=True,  # Degraded is acceptable
                message=f"Status: {status} (warning)",
                details=body,
            ))
            result.passed_checks += 1
        else:
            result.results.append(CheckResult(
                name=f"{service}_status",
                passed=False,
                message=f"Status: {status}",
                details=body,
            ))
            result.failed_checks += 1

        # Check 3: Active health check - verify service actually works
        if active_check.get("enabled", False):
            latency_ms = body.get("latency_ms", -1)
            # Accept latency >= 0 (cache hits report 0ms, which is valid)
            if latency_ms >= 0 and status in ("healthy", "degraded"):
                cache_note = " (cached)" if latency_ms == 0 else ""
                result.results.append(CheckResult(
                    name=f"{service}_active_probe",
                    passed=True,
                    message=f"Active probe succeeded ({latency_ms:.0f}ms{cache_note})",
                ))
                result.passed_checks += 1
            else:
                result.results.append(CheckResult(
                    name=f"{service}_active_probe",
                    passed=False,
                    message="Active probe failed or did not report latency",
                ))
                result.failed_checks += 1

        # Check 4: Thresholds
        if "max_latency_ms" in thresholds:
            max_latency = thresholds["max_latency_ms"]
            actual_latency = body.get("latency_ms", 0)
            if actual_latency <= max_latency:
                result.results.append(CheckResult(
                    name=f"{service}_latency_threshold",
                    passed=True,
                    message=f"Latency {actual_latency:.0f}ms <= {max_latency}ms",
                ))
                result.passed_checks += 1
            else:
                result.results.append(CheckResult(
                    name=f"{service}_latency_threshold",
                    passed=False,
                    message=f"Latency {actual_latency:.0f}ms > {max_latency}ms",
                ))
                result.failed_checks += 1

        if "min_success_rate_percent" in thresholds:
            min_rate = thresholds["min_success_rate_percent"]
            actual_rate = body.get("success_rate_24h", 100)
            if actual_rate >= min_rate:
                result.results.append(CheckResult(
                    name=f"{service}_success_rate",
                    passed=True,
                    message=f"Success rate {actual_rate:.1f}% >= {min_rate}%",
                ))
                result.passed_checks += 1
            else:
                result.results.append(CheckResult(
                    name=f"{service}_success_rate",
                    passed=False,
                    message=f"Success rate {actual_rate:.1f}% < {min_rate}%",
                ))
                result.failed_checks += 1

        if "max_error_rate_percent" in thresholds:
            max_error = thresholds["max_error_rate_percent"]
            error_count = body.get("error_count_24h", 0)
            # Calculate error rate (assuming some baseline)
            actual_error_rate = 100 - body.get("success_rate_24h", 100)
            if actual_error_rate <= max_error:
                result.results.append(CheckResult(
                    name=f"{service}_error_rate",
                    passed=True,
                    message=f"Error rate {actual_error_rate:.1f}% <= {max_error}%",
                ))
                result.passed_checks += 1
            else:
                result.results.append(CheckResult(
                    name=f"{service}_error_rate",
                    passed=False,
                    message=f"Error rate {actual_error_rate:.1f}% > {max_error}%",
                ))
                result.failed_checks += 1

        # Check 5: Configuration validation
        if config_validation:
            config_passed = True
            config_messages = []

            # Check env vars
            required_env = config_validation.get("required_env_vars", [])
            if required_env:
                env_configured = body.get("configured_env_var", False)
                if not env_configured:
                    config_passed = False
                    config_messages.append("Required env vars not configured")

            # Check expected values
            expected_values = config_validation.get("expected_values", {})
            if "dimensions" in expected_values:
                dims = body.get("dimensions", 0)
                valid_dims = expected_values["dimensions"]
                if dims not in valid_dims:
                    config_passed = False
                    config_messages.append(f"Dimensions {dims} not in {valid_dims}")

            if "provider" in expected_values:
                provider = body.get("provider", "")
                valid_providers = expected_values["provider"]
                if provider not in valid_providers:
                    config_passed = False
                    config_messages.append(f"Provider {provider} not in {valid_providers}")

            result.results.append(CheckResult(
                name=f"{service}_configuration",
                passed=config_passed,
                message="; ".join(config_messages) if config_messages else "Configuration valid",
            ))
            if config_passed:
                result.passed_checks += 1
            else:
                result.failed_checks += 1

        # Check 6: Circuit breaker state
        if circuit_breaker.get("check_state", False):
            expected_state = circuit_breaker.get("expected_state", "closed")
            actual_state = body.get("circuit_breaker", "closed")
            if actual_state == expected_state:
                result.results.append(CheckResult(
                    name=f"{service}_circuit_breaker",
                    passed=True,
                    message=f"Circuit breaker: {actual_state}",
                ))
                result.passed_checks += 1
            else:
                result.results.append(CheckResult(
                    name=f"{service}_circuit_breaker",
                    passed=False,
                    message=f"Circuit breaker: {actual_state} (expected {expected_state})",
                ))
                result.failed_checks += 1

    except requests.RequestException as e:
        result.results.append(CheckResult(
            name=f"{service}_connectivity",
            passed=False,
            message=f"Request failed: {e}",
        ))
        result.failed_checks = result.total_checks

    result.end_time = datetime.now()
    return result


def run_observability_test(spec: Dict[str, Any], base_url: str) -> ObservabilityTestResult:
    """Run the appropriate test based on spec type."""
    test_type = spec["test"]["type"]

    if test_type == "metrics":
        return run_metrics_test(spec, base_url)
    elif test_type == "health":
        return run_health_test(spec, base_url)
    elif test_type == "dependency":
        return run_dependency_test(spec, base_url)
    elif test_type == "logging":
        # Avoid false PASS for unimplemented logging validation.
        return ObservabilityTestResult(
            spec_name=spec["test"]["name"],
            test_type="logging",
            total_checks=1,
            passed_checks=0,
            failed_checks=1,
            results=[CheckResult(
                name="logging_test",
                passed=False,
                message="Logging validation is not implemented in uobs_runner.py",
            )],
            end_time=datetime.now(),
        )
    elif test_type == "tracing":
        return run_tracing_test(spec, base_url)
    else:
        return ObservabilityTestResult(
            spec_name=spec["test"]["name"],
            test_type=test_type,
            total_checks=0,
            passed_checks=0,
            failed_checks=1,
            results=[CheckResult(
                name="unknown_type",
                passed=False,
                message=f"Unknown test type: {test_type}",
            )],
            end_time=datetime.now(),
        )


def print_results(result: ObservabilityTestResult) -> None:
    """Print observability test results."""
    status = "\u2713 PASS" if result.passed else "\u2717 FAIL"
    status_color = "\033[92m" if result.passed else "\033[91m"
    reset = "\033[0m"

    print(f"\n{'='*60}")
    print(f"Test: {result.spec_name}")
    print(f"Type: {result.test_type}")
    print(f"Status: {status_color}{status}{reset}")
    print(f"{'='*60}")
    print(f"Checks: {result.passed_checks}/{result.total_checks} passed")

    for cr in result.results:
        cr_status = "\u2713" if cr.passed else "\u2717"
        cr_color = "\033[92m" if cr.passed else "\033[91m"
        print(f"  {cr_color}{cr_status}{reset} {cr.name}: {cr.message}")


def main():
    parser = argparse.ArgumentParser(description="UOBS Observability Test Runner")
    parser.add_argument("--spec", required=True, help="Path to UOBS spec file(s)")
    parser.add_argument("--base-url", default="http://localhost:9999", help="MDEMG base URL")
    parser.add_argument("--output", help="Output directory for results")
    args = parser.parse_args()

    # Handle glob patterns in spec
    spec_paths = []
    if "*" in args.spec:
        spec_dir = Path(args.spec).parent
        pattern = Path(args.spec).name
        spec_paths = list(spec_dir.glob(pattern))
    else:
        spec_paths = [Path(args.spec)]

    all_passed = True
    results = []

    for spec_path in spec_paths:
        print(f"\nLoading spec: {spec_path}")
        spec = load_spec(spec_path)

        result = run_observability_test(spec, args.base_url)
        results.append(result)
        print_results(result)

        if not result.passed:
            all_passed = False

    # Save results if output directory specified
    if args.output:
        output_dir = Path(args.output)
        output_dir.mkdir(parents=True, exist_ok=True)

        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        output_file = output_dir / f"uobs_results_{timestamp}.json"

        with open(output_file, "w") as f:
            json.dump({
                "timestamp": timestamp,
                "overall_passed": all_passed,
                "results": [r.to_dict() for r in results],
            }, f, indent=2)

        print(f"\nResults saved to: {output_file}")

    print(f"\n{'='*60}")
    print(f"Observability Test Summary")
    print(f"{'='*60}")
    print(f"Total Tests:  {len(results)}")
    print(f"Passed:       {sum(1 for r in results if r.passed)}")
    print(f"Failed:       {sum(1 for r in results if not r.passed)}")
    print(f"{'='*60}")
    print(f"Overall: {'PASS' if all_passed else 'FAIL'}")
    print(f"{'='*60}")

    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    main()
