#!/usr/bin/env python3
"""
UBTS Runner - Universal Benchmark Test Specification Runner

Executes UBTS benchmark specifications against MDEMG endpoints.

Usage:
    python ubts_runner.py --spec specs/retrieve_latency.ubts.json --profile profiles/load.profile.json
    python ubts_runner.py --spec specs/*.ubts.json --profile profiles/smoke.profile.json --output results/
"""

import argparse
import json
import os
import statistics
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional
from urllib.parse import urljoin

import requests

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
    parser.add_argument("--output", help="Output directory for results")
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
    results = []
    assertions = profile.get("assertions", {})
    check_thresholds = assertions.get("check_thresholds", True)
    all_requests_succeed = assertions.get("all_requests_succeed", False)

    for spec_path in spec_paths:
        print(f"\nLoading spec: {spec_path}")
        spec = load_spec(spec_path)

        result = run_benchmark(spec, profile, args.base_url, args.space_id)
        results.append(result)

        thresholds = spec.get("thresholds", {}) if check_thresholds else {}
        passed = print_results(result, thresholds)
        if check_thresholds and not passed:
            all_passed = False
        if all_requests_succeed and result.failed_requests > 0:
            print(f"\n  [✗] all_requests_succeed: {result.failed_requests} failures")
            all_passed = False

    # Save results if output directory specified
    if args.output:
        output_dir = Path(args.output)
        output_dir.mkdir(parents=True, exist_ok=True)

        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        output_file = output_dir / f"ubts_results_{timestamp}.json"

        with open(output_file, "w") as f:
            json.dump({
                "profile": profile["profile_name"],
                "timestamp": timestamp,
                "results": [r.to_dict() for r in results],
            }, f, indent=2)

        print(f"\nResults saved to: {output_file}")

    print(f"\n{'='*60}")
    print(f"Overall: {'PASS' if all_passed else 'FAIL'}")
    print(f"{'='*60}")

    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    main()
