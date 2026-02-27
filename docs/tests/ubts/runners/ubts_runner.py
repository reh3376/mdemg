#!/usr/bin/env python3
"""
UBTS Runner - Universal Benchmark Test Specification Runner

Executes UBTS benchmark specifications against MDEMG endpoints.

Version: 1.2.0

Usage:
    python ubts_runner.py --spec specs/retrieve_latency.ubts.json --profile profiles/load.profile.json
    python ubts_runner.py --spec specs/*.ubts.json --profile profiles/smoke.profile.json --report results/ubts_report.json
"""

import argparse
import json
import os
import statistics
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional
from urllib.parse import urljoin

import sys as _sys
_sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..'))
from uxts_report import build_result as _canonical_result, build_report as _canonical_report, print_summary as _print_summary, save_report as _save_report

import requests

UBTS_VERSION = "1.2.0"

@dataclass
class BenchmarkResult:
    """Results from a single benchmark run."""
    spec_name: str
    profile_name: str
    total_requests: int
    successful_requests: int
    failed_requests: int
    latencies_ms: List[float] = field(default_factory=list)
    errors: List[str] = field(default_factory=list)
    start_time: datetime = field(default_factory=datetime.now)
    end_time: Optional[datetime] = None

    @property
    def success_rate(self) -> float:
        if self.total_requests == 0:
            return 0.0
        return (self.successful_requests / self.total_requests) * 100

    @property
    def error_rate(self) -> float:
        return 100.0 - self.success_rate

    @property
    def p50_ms(self) -> float:
        if not self.latencies_ms:
            return 0.0
        return statistics.median(self.latencies_ms)

    @property
    def p95_ms(self) -> float:
        if not self.latencies_ms:
            return 0.0
        return statistics.quantiles(self.latencies_ms, n=20)[18]  # 95th percentile

    @property
    def p99_ms(self) -> float:
        if not self.latencies_ms:
            return 0.0
        return statistics.quantiles(self.latencies_ms, n=100)[98]  # 99th percentile

    @property
    def max_ms(self) -> float:
        if not self.latencies_ms:
            return 0.0
        return max(self.latencies_ms)

    @property
    def throughput_rps(self) -> float:
        if self.end_time is None:
            return 0.0
        duration = (self.end_time - self.start_time).total_seconds()
        if duration == 0:
            return 0.0
        return self.successful_requests / duration

    def check_thresholds(self, thresholds: Dict[str, float]) -> Dict[str, bool]:
        """Check if results meet threshold requirements."""
        results = {}

        if "p50_ms" in thresholds:
            results["p50_ms"] = self.p50_ms <= thresholds["p50_ms"]
        if "p95_ms" in thresholds:
            results["p95_ms"] = self.p95_ms <= thresholds["p95_ms"]
        if "p99_ms" in thresholds:
            results["p99_ms"] = self.p99_ms <= thresholds["p99_ms"]
        if "max_ms" in thresholds:
            results["max_ms"] = self.max_ms <= thresholds["max_ms"]
        if "error_rate_pct" in thresholds:
            results["error_rate_pct"] = self.error_rate <= thresholds["error_rate_pct"]
        if "throughput_rps" in thresholds:
            results["throughput_rps"] = self.throughput_rps >= thresholds["throughput_rps"]

        return results

    def to_dict(self) -> Dict[str, Any]:
        return {
            "spec_name": self.spec_name,
            "profile_name": self.profile_name,
            "total_requests": self.total_requests,
            "successful_requests": self.successful_requests,
            "failed_requests": self.failed_requests,
            "success_rate_pct": round(self.success_rate, 2),
            "error_rate_pct": round(self.error_rate, 2),
            "latency": {
                "p50_ms": round(self.p50_ms, 2),
                "p95_ms": round(self.p95_ms, 2),
                "p99_ms": round(self.p99_ms, 2),
                "max_ms": round(self.max_ms, 2),
            },
            "throughput_rps": round(self.throughput_rps, 2),
            "start_time": self.start_time.isoformat(),
            "end_time": self.end_time.isoformat() if self.end_time else None,
            "error_samples": self.errors[:10],  # First 10 errors
        }


def load_spec(spec_path: str) -> Dict[str, Any]:
    """Load a UBTS specification file."""
    with open(spec_path) as f:
        return json.load(f)


def load_profile(profile_path: str) -> Dict[str, Any]:
    """Load a benchmark profile file."""
    with open(profile_path) as f:
        return json.load(f)


# --------------------------------------------------------------------------- #
# Schema-runner parity: hard-fail for unimplemented spec/profile fields       #
# --------------------------------------------------------------------------- #

# Schema fields recognised by this runner. Anything outside this set triggers
# a hard fail so specs never silently ignore new schema additions.
_KNOWN_SPEC_KEYS = {
    "ubts_version", "benchmark", "thresholds", "setup", "metadata",
}
_KNOWN_BENCHMARK_KEYS = {
    "name", "description", "endpoint", "method", "payload_template", "headers",
}
_KNOWN_THRESHOLD_KEYS = {
    "p50_ms", "p95_ms", "p99_ms", "max_ms", "error_rate_pct", "throughput_rps",
}
_KNOWN_SETUP_KEYS = {"seed_data", "warmup_requests", "use_benchmark_space"}
_KNOWN_PROFILE_PARAM_KEYS = {
    "total_requests", "concurrent_users", "ramp_up_seconds",
    "duration_seconds", "think_time_ms",
}
_KNOWN_ASSERTION_KEYS = {
    "all_requests_succeed", "check_thresholds",
    "min_success_rate", "max_p99_degradation_pct",
}


def validate_supported_features(
    spec: Dict[str, Any],
    profile: Dict[str, Any],
) -> List[str]:
    """Fail fast when spec/profile uses fields this runner does not implement.

    Returns a list of error strings prefixed with 'PARITY FAILURE:'.
    Non-empty means the spec MUST be marked as fail -- not warned and skipped.
    """
    errors: List[str] = []

    # --- Spec root keys ---
    for key in spec:
        if key not in _KNOWN_SPEC_KEYS:
            errors.append(f"PARITY FAILURE: Unimplemented spec root key: {key}")

    # --- Benchmark sub-keys ---
    benchmark = spec.get("benchmark", {})
    for key in benchmark:
        if key not in _KNOWN_BENCHMARK_KEYS:
            errors.append(f"PARITY FAILURE: Unimplemented benchmark field: {key}")

    # --- Threshold sub-keys ---
    thresholds = spec.get("thresholds", {})
    for key in thresholds:
        if key not in _KNOWN_THRESHOLD_KEYS:
            errors.append(f"PARITY FAILURE: Unimplemented threshold field: {key}")

    # --- Setup sub-keys ---
    setup = spec.get("setup", {})
    for key in setup:
        if key not in _KNOWN_SETUP_KEYS:
            errors.append(f"PARITY FAILURE: Unimplemented setup field: {key}")
    if "seed_data" in setup and setup["seed_data"]:
        errors.append("PARITY FAILURE: setup.seed_data is defined but seeding is not implemented")

    # --- Profile parameter keys ---
    params = profile.get("parameters", {})
    for key in params:
        if key not in _KNOWN_PROFILE_PARAM_KEYS:
            errors.append(f"PARITY FAILURE: Unimplemented profile parameter: {key}")
    if params.get("ramp_up_seconds"):
        errors.append("PARITY FAILURE: profile.parameters.ramp_up_seconds is defined but ramp-up is not implemented")
    if params.get("duration_seconds"):
        errors.append("PARITY FAILURE: profile.parameters.duration_seconds is defined but duration-based runs are not implemented")

    # --- Profile assertion keys ---
    assertions = profile.get("assertions", {})
    for key in assertions:
        if key not in _KNOWN_ASSERTION_KEYS:
            errors.append(f"PARITY FAILURE: Unimplemented profile assertion: {key}")

    return errors


def render_value(value: Any, variables: Dict[str, Any]) -> Any:
    """Recursively render template variables in any value type."""
    if isinstance(value, str) and value.startswith("{{") and value.endswith("}}"):
        var_name = value[2:-2]
        return variables.get(var_name, value)
    elif isinstance(value, dict):
        return {k: render_value(v, variables) for k, v in value.items()}
    elif isinstance(value, list):
        return [render_value(item, variables) for item in value]
    return value


def render_payload(template: Dict[str, Any], variables: Dict[str, Any]) -> Dict[str, Any]:
    """Render a payload template with variables."""
    return render_value(template, variables)


def make_request(
    base_url: str,
    endpoint: str,
    method: str,
    payload: Dict[str, Any],
    headers: Dict[str, str],
    timeout: float = 30.0,
) -> tuple[float, Optional[str]]:
    """Make a single HTTP request and return (latency_ms, error_message)."""
    url = urljoin(base_url, endpoint)

    start = time.perf_counter()
    try:
        if method == "GET":
            resp = requests.get(url, headers=headers, params=payload, timeout=timeout)
        elif method == "POST":
            resp = requests.post(url, headers=headers, json=payload, timeout=timeout)
        elif method == "PUT":
            resp = requests.put(url, headers=headers, json=payload, timeout=timeout)
        elif method == "DELETE":
            resp = requests.delete(url, headers=headers, timeout=timeout)
        else:
            return 0.0, f"Unsupported method: {method}"

        latency_ms = (time.perf_counter() - start) * 1000

        if resp.status_code >= 400:
            return latency_ms, f"HTTP {resp.status_code}: {resp.text[:200]}"

        return latency_ms, None

    except requests.Timeout:
        latency_ms = (time.perf_counter() - start) * 1000
        return latency_ms, "Request timeout"
    except requests.RequestException as e:
        latency_ms = (time.perf_counter() - start) * 1000
        return latency_ms, str(e)


def run_benchmark(
    spec: Dict[str, Any],
    profile: Dict[str, Any],
    base_url: str,
    space_id: str = "benchmark-test",
) -> BenchmarkResult:
    """Run a benchmark according to spec and profile."""
    benchmark = spec["benchmark"]
    params = profile["parameters"]
    variations = profile.get("query_variations", [{}])

    result = BenchmarkResult(
        spec_name=benchmark["name"],
        profile_name=profile["profile_name"],
        total_requests=params["total_requests"],
        successful_requests=0,
        failed_requests=0,
    )

    endpoint = benchmark["endpoint"]
    method = benchmark.get("method", "POST")
    headers = benchmark.get("headers", {"Content-Type": "application/json"})
    payload_template = benchmark.get("payload_template", {})

    # Warmup
    warmup = spec.get("setup", {}).get("warmup_requests", 0)
    for i in range(warmup):
        variables = {
            "space_id": space_id,
            "path_1": f"ubts/warmup/{i}/a",
            "path_2": f"ubts/warmup/{i}/b",
            "path_3": f"ubts/warmup/{i}/c",
            **variations[i % len(variations)],
        }
        payload = render_payload(payload_template, variables)
        make_request(base_url, endpoint, method, payload, headers)

    # Main benchmark
    concurrent = params.get("concurrent_users", 1)
    think_time = params.get("think_time_ms", 0) / 1000.0

    result.start_time = datetime.now()

    def worker(idx: int) -> tuple[float, Optional[str]]:
        variation = variations[idx % len(variations)]
        # Generate unique paths per request to avoid constraint violations
        variables = {
            "space_id": space_id,
            "path_1": f"ubts/bench/{idx}/a",
            "path_2": f"ubts/bench/{idx}/b",
            "path_3": f"ubts/bench/{idx}/c",
            **variation,
        }
        payload = render_payload(payload_template, variables)

        if think_time > 0:
            time.sleep(think_time)

        return make_request(base_url, endpoint, method, payload, headers)

    with ThreadPoolExecutor(max_workers=concurrent) as executor:
        futures = [executor.submit(worker, i) for i in range(params["total_requests"])]

        for future in as_completed(futures):
            latency_ms, error = future.result()
            result.latencies_ms.append(latency_ms)

            if error:
                result.failed_requests += 1
                if len(result.errors) < 100:  # Limit error collection
                    result.errors.append(error)
            else:
                result.successful_requests += 1

    result.end_time = datetime.now()
    return result


def print_results(result: BenchmarkResult, thresholds: Dict[str, float]) -> bool:
    """Print benchmark results and return True if all thresholds passed."""
    print(f"\n{'='*60}")
    print(f"Benchmark: {result.spec_name} ({result.profile_name})")
    print(f"{'='*60}")
    print(f"Total Requests:     {result.total_requests}")
    print(f"Successful:         {result.successful_requests}")
    print(f"Failed:             {result.failed_requests}")
    print(f"Success Rate:       {result.success_rate:.2f}%")
    print(f"Throughput:         {result.throughput_rps:.2f} rps")
    print(f"\nLatency Percentiles:")
    print(f"  p50:              {result.p50_ms:.2f} ms")
    print(f"  p95:              {result.p95_ms:.2f} ms")
    print(f"  p99:              {result.p99_ms:.2f} ms")
    print(f"  max:              {result.max_ms:.2f} ms")

    checks = result.check_thresholds(thresholds)
    print(f"\nThreshold Checks:")
    all_passed = True
    for name, passed in checks.items():
        status = "PASS" if passed else "FAIL"
        symbol = "\u2713" if passed else "\u2717"
        actual = getattr(result, name, None)
        if actual is None:
            if name == "error_rate_pct":
                actual = result.error_rate
            elif name == "throughput_rps":
                actual = result.throughput_rps
        threshold = thresholds.get(name, "N/A")
        print(f"  [{symbol}] {name}: {actual:.2f} (threshold: {threshold}) - {status}")
        if not passed:
            all_passed = False

    if result.errors:
        print(f"\nSample Errors ({len(result.errors)} total):")
        for err in result.errors[:5]:
            print(f"  - {err[:100]}")

    return all_passed


def main():
    parser = argparse.ArgumentParser(description="UBTS Benchmark Runner")
    parser.add_argument("--spec", required=True, help="Path to UBTS spec file(s)")
    parser.add_argument("--profile", required=True, help="Path to profile file")
    parser.add_argument("--base-url", default="http://localhost:9999", help="MDEMG base URL")
    parser.add_argument("--space-id", default="benchmark-test", help="Space ID for tests")
    parser.add_argument("--report", help="Output file path for canonical JSON report")
    args = parser.parse_args()

    profile = load_profile(args.profile)

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
    assertions = profile.get("assertions", {})
    check_thresholds = assertions.get("check_thresholds", True)
    all_requests_succeed = assertions.get("all_requests_succeed", False)
    min_success_rate = assertions.get("min_success_rate")  # e.g. 95.0
    max_p99_degradation_pct = assertions.get("max_p99_degradation_pct")  # e.g. 200

    for spec_path in spec_paths:
        print(f"\nLoading spec: {spec_path}")
        spec = load_spec(spec_path)

        # --- Schema-runner parity: hard-fail for unimplemented fields ---
        parity_errors = validate_supported_features(spec, profile)
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

        result = run_benchmark(spec, profile, args.base_url, args.space_id)

        thresholds = spec.get("thresholds", {}) if check_thresholds else {}
        passed = print_results(result, thresholds)

        # Build assertion counts and failure details
        threshold_checks = result.check_thresholds(thresholds)
        spec_failures = []
        assertions_evaluated = len(threshold_checks)
        assertions_passed_count = sum(1 for v in threshold_checks.values() if v)

        if check_thresholds and not passed:
            all_passed = False
            for name, ok in threshold_checks.items():
                if not ok:
                    actual = getattr(result, name, None)
                    if actual is None:
                        if name == "error_rate_pct":
                            actual = result.error_rate
                        elif name == "throughput_rps":
                            actual = result.throughput_rps
                    spec_failures.append(
                        f"Threshold {name}: actual={actual:.2f}, threshold={thresholds.get(name)}"
                    )

        if all_requests_succeed and result.failed_requests > 0:
            print(f"\n  [\u2717] all_requests_succeed: {result.failed_requests} failures")
            all_passed = False
            assertions_evaluated += 1
            spec_failures.append(f"all_requests_succeed: {result.failed_requests} failures")
        elif all_requests_succeed:
            assertions_evaluated += 1
            assertions_passed_count += 1

        # --- B1: Enforce min_success_rate ---
        if min_success_rate is not None:
            assertions_evaluated += 1
            actual_rate = result.success_rate
            if actual_rate < min_success_rate:
                print(f"\n  [\u2717] min_success_rate: {actual_rate:.2f}% < {min_success_rate}% - FAIL")
                all_passed = False
                spec_failures.append(f"min_success_rate: {actual_rate:.2f}% < {min_success_rate}%")
            else:
                print(f"\n  [\u2713] min_success_rate: {actual_rate:.2f}% >= {min_success_rate}% - PASS")
                assertions_passed_count += 1

        # --- B1: Enforce max_p99_degradation_pct ---
        # Baseline is ALWAYS the spec's p99_ms threshold (fixed, deterministic).
        # Degradation = ((actual - spec_p99) / spec_p99) * 100.
        # This avoids ambiguity from "previous run" comparisons.
        if max_p99_degradation_pct is not None and check_thresholds:
            spec_p99 = spec.get("thresholds", {}).get("p99_ms")
            if spec_p99 and spec_p99 > 0:
                assertions_evaluated += 1
                actual_p99 = result.p99_ms
                degradation_pct = ((actual_p99 - spec_p99) / spec_p99) * 100
                if degradation_pct > max_p99_degradation_pct:
                    print(
                        f"\n  [\u2717] max_p99_degradation_pct: {degradation_pct:.1f}% > "
                        f"{max_p99_degradation_pct}% (actual p99={actual_p99:.2f}ms vs "
                        f"spec p99={spec_p99}ms) - FAIL"
                    )
                    all_passed = False
                    spec_failures.append(
                        f"max_p99_degradation_pct: {degradation_pct:.1f}% > {max_p99_degradation_pct}%"
                    )
                else:
                    print(
                        f"\n  [\u2713] max_p99_degradation_pct: {degradation_pct:.1f}% <= "
                        f"{max_p99_degradation_pct}% - PASS"
                    )
                    assertions_passed_count += 1
            else:
                print(
                    f"\n  [\u26a0] max_p99_degradation_pct: spec has no p99_ms threshold "
                    f"to compare against - SKIP"
                )

        # Compute duration_ms from benchmark result
        duration_ms = 0.0
        if result.end_time and result.start_time:
            duration_ms = (result.end_time - result.start_time).total_seconds() * 1000

        spec_status = "pass" if not spec_failures else "fail"
        canonical_results.append(_canonical_result(
            spec_path=str(spec_path),
            status=spec_status,
            duration_ms=duration_ms,
            hash_verified=None,
            assertions_evaluated=assertions_evaluated,
            assertions_passed=assertions_passed_count,
            failures=spec_failures if spec_failures else None,
        ))

    # Build canonical report
    report = _canonical_report(
        framework="ubts",
        framework_version=UBTS_VERSION,
        results=canonical_results,
        start_time=report_start,
    )

    _print_summary(report)

    if args.report:
        _save_report(report, args.report)

    print(f"\n{'='*60}")
    print(f"Overall: {'PASS' if all_passed else 'FAIL'}")
    print(f"{'='*60}")

    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    main()
