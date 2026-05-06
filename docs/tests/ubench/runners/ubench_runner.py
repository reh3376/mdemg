#!/usr/bin/env python3
"""UBENCH Runner — Universal Benchmark Specification Runner.

Wraps neural.benchmarks.run_benchmark as a UxTS-pattern framework:

  * Validates the .ubench.json spec against the schema.
  * Pins config + golden_holdout SHA256 (mismatch => fail).
  * Optionally invokes neural.benchmarks.run_benchmark as subprocess (--run).
  * Emits canonical UxTS report via uxts_report.build_report.

Modes:

  * `--mode lint`     — schema + SHA verification only (no subprocess).
                         Default. Cheap; safe in CI.
  * `--mode contract` — lint + dataset↔holdout contract test (every ULTS
                         spec has min_rows_per_task golden rows). No model calls.
  * `--mode run`      — full benchmark execution (requires running LLM endpoint).

Usage:
    python docs/tests/ubench/runners/ubench_runner.py \\
        --spec docs/tests/ubench/specs/mdemg.ubench.json \\
        --mode contract \\
        --report training_data/eval/ubench_report.json

Exit codes:
    0 = pass  | 1 = fail (hash mismatch / contract violation / threshold miss)
    2 = error (spec invalid / schema load failure / subprocess crash)
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import sys
import time
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parents[4]
sys.path.insert(0, str(REPO_ROOT / "docs" / "tests"))

from uxts_report import build_report, build_result, print_summary, save_report  # noqa: E402

UBENCH_VERSION = "1.0.0"
SCHEMA_PATH = REPO_ROOT / "docs" / "tests" / "ubench" / "schema" / "ubench.schema.json"


# ── helpers ──────────────────────────────────────────────────────────────────


def sha256_file(path: Path, chunk: int = 8192) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for blk in iter(lambda: f.read(chunk), b""):
            h.update(blk)
    return h.hexdigest()


def load_jsonl(path: Path) -> list[dict[str, Any]]:
    rows = []
    with path.open() as f:
        for line in f:
            line = line.strip()
            if line:
                rows.append(json.loads(line))
    return rows


def load_ults_specs(specs_dir: Path) -> list[dict[str, Any]]:
    specs = []
    for p in sorted(specs_dir.glob("*.ults.json")):
        if p.name.endswith(".bak"):
            continue
        try:
            specs.append({"path": p, "data": json.loads(p.read_text())})
        except json.JSONDecodeError as e:
            specs.append({"path": p, "data": None, "error": f"json parse: {e}"})
    return specs


def schema_validate(spec: dict[str, Any], schema: dict[str, Any]) -> list[str]:
    """Minimal validation — required keys + bounded numeric types.

    Avoids a hard dependency on jsonschema; the schema document itself is the
    contract, and a stronger validator can be plugged in later by importing
    jsonschema if installed.
    """
    try:
        import jsonschema  # type: ignore[import-not-found]
    except ImportError:
        jsonschema = None

    if jsonschema is not None:
        v = jsonschema.Draft202012Validator(schema)
        return [
            f"{'/'.join(str(p) for p in err.absolute_path) or '<root>'}: {err.message}"
            for err in v.iter_errors(spec)
        ]

    # Fallback: required-key check for top-level + each declared object.
    failures: list[str] = []
    required = schema.get("required", [])
    for key in required:
        if key not in spec:
            failures.append(f"<root>: missing required key '{key}'")
    return failures


# ── modes ────────────────────────────────────────────────────────────────────


def run_lint(spec_path: Path, spec: dict[str, Any], schema: dict[str, Any]) -> dict[str, Any]:
    failures: list[str] = []
    warnings: list[str] = []
    assertions = 0
    passed = 0
    start = time.monotonic()

    # Schema validation.
    assertions += 1
    schema_errors = schema_validate(spec, schema)
    if schema_errors:
        failures.extend(f"schema: {e}" for e in schema_errors)
    else:
        passed += 1

    # Config hash.
    cfg = spec.get("config", {})
    cfg_path = REPO_ROOT / cfg.get("path", "")
    expected_cfg_sha = cfg.get("sha256")
    hash_verified: bool | None = None
    hash_mismatches: list[str] = []
    if expected_cfg_sha:
        assertions += 1
        if not cfg_path.is_file():
            failures.append(f"config: file missing at {cfg_path}")
            hash_verified = False
            hash_mismatches.append(f"config.path missing: {cfg_path}")
        else:
            actual = sha256_file(cfg_path)
            if actual == expected_cfg_sha:
                hash_verified = True
                passed += 1
            else:
                hash_verified = False
                hash_mismatches.append(
                    f"config.sha256 mismatch: spec={expected_cfg_sha[:12]} actual={actual[:12]}"
                )
                failures.append(hash_mismatches[-1])

    # Golden holdout hash.
    gh = spec.get("golden_holdout", {})
    gh_path = REPO_ROOT / gh.get("path", "")
    expected_gh_sha = gh.get("sha256")
    if expected_gh_sha:
        assertions += 1
        if not gh_path.is_file():
            failures.append(f"golden_holdout: file missing at {gh_path}")
            hash_mismatches.append(f"golden_holdout.path missing: {gh_path}")
            hash_verified = False
        else:
            actual = sha256_file(gh_path)
            if actual == expected_gh_sha:
                if hash_verified is None:
                    hash_verified = True
                passed += 1
            else:
                hash_verified = False
                hash_mismatches.append(
                    f"golden_holdout.sha256 mismatch: spec={expected_gh_sha[:12]} actual={actual[:12]}"
                )
                failures.append(hash_mismatches[-1])

    duration_ms = (time.monotonic() - start) * 1000
    status = "pass" if not failures else "fail"
    return build_result(
        spec_path=spec_path.relative_to(REPO_ROOT),
        status=status,
        duration_ms=duration_ms,
        hash_verified=hash_verified,
        hash_mismatches=hash_mismatches,
        assertions_evaluated=assertions,
        assertions_passed=passed,
        failures=failures,
        warnings=warnings,
    )


def run_contract(spec_path: Path, spec: dict[str, Any]) -> dict[str, Any]:
    failures: list[str] = []
    warnings: list[str] = []
    assertions = 0
    passed = 0
    start = time.monotonic()

    ults_dir = REPO_ROOT / spec["ults"]["specs_dir"]
    expected_specs = spec["ults"]["expected_specs"]
    gh_path = REPO_ROOT / spec["golden_holdout"]["path"]
    expected_rows = spec["golden_holdout"]["expected_rows"]
    expected_tasks = spec["golden_holdout"]["expected_tasks"]
    min_rows = spec["golden_holdout"].get("min_rows_per_task", 1)

    # Spec count.
    assertions += 1
    ults_specs = load_ults_specs(ults_dir)
    valid_specs = [s for s in ults_specs if s["data"] is not None]
    if len(valid_specs) != expected_specs:
        failures.append(
            f"ults.expected_specs: spec={expected_specs} discovered={len(valid_specs)} "
            f"(invalid={len(ults_specs) - len(valid_specs)})"
        )
    else:
        passed += 1

    # Golden holdout shape.
    if not gh_path.is_file():
        failures.append(f"golden_holdout.path missing: {gh_path}")
        duration_ms = (time.monotonic() - start) * 1000
        return build_result(
            spec_path=spec_path.relative_to(REPO_ROOT),
            status="fail",
            duration_ms=duration_ms,
            assertions_evaluated=assertions + 1,
            assertions_passed=passed,
            failures=failures,
            warnings=warnings,
        )

    rows = load_jsonl(gh_path)
    task_counter: Counter[str] = Counter()
    for row in rows:
        meta = row.get("meta", {})
        task = meta.get("task_name") or meta.get("task_id")
        if task:
            task_counter[task] += 1

    assertions += 1
    if len(rows) != expected_rows:
        failures.append(
            f"golden_holdout.expected_rows: spec={expected_rows} actual={len(rows)}"
        )
    else:
        passed += 1

    assertions += 1
    if len(task_counter) != expected_tasks:
        failures.append(
            f"golden_holdout.expected_tasks: spec={expected_tasks} actual={len(task_counter)}"
        )
    else:
        passed += 1

    # Dataset↔holdout contract: every ULTS task has ≥ min_rows golden rows.
    spec_task_names = {
        s["data"].get("task", {}).get("name") for s in valid_specs
    } - {None}
    missing = []
    insufficient = []
    for task_name in sorted(spec_task_names):
        assertions += 1
        n = task_counter.get(task_name, 0)
        if n == 0:
            missing.append(task_name)
        elif n < min_rows:
            insufficient.append(f"{task_name} ({n} < {min_rows})")
        else:
            passed += 1

    if missing:
        failures.append(
            f"contract: ULTS tasks with zero golden rows: {sorted(missing)}"
        )
    if insufficient:
        failures.append(
            f"contract: ULTS tasks below min_rows_per_task={min_rows}: {sorted(insufficient)}"
        )

    duration_ms = (time.monotonic() - start) * 1000
    status = "pass" if not failures else "fail"
    return build_result(
        spec_path=spec_path.relative_to(REPO_ROOT),
        status=status,
        duration_ms=duration_ms,
        assertions_evaluated=assertions,
        assertions_passed=passed,
        failures=failures,
        warnings=warnings,
    )


def run_full(spec_path: Path, spec: dict[str, Any]) -> dict[str, Any]:
    """Execute the wrapped benchmark via subprocess."""
    failures: list[str] = []
    warnings: list[str] = []
    assertions = 0
    passed = 0
    start = time.monotonic()

    cfg_path = REPO_ROOT / spec["config"]["path"]
    out_path = REPO_ROOT / spec["output"]["report_path"]
    out_path.parent.mkdir(parents=True, exist_ok=True)

    cmd = [
        sys.executable,
        "-m",
        "neural.benchmarks.run_benchmark",
        "--config",
        str(cfg_path),
        "--out",
        str(out_path),
    ]
    try:
        proc = subprocess.run(
            cmd, cwd=str(REPO_ROOT), capture_output=True, text=True, check=False
        )
    except FileNotFoundError as e:
        return build_result(
            spec_path=spec_path.relative_to(REPO_ROOT),
            status="error",
            error=f"subprocess launch failed: {e}",
        )

    assertions += 1
    if proc.returncode != 0:
        failures.append(
            f"benchmark subprocess exit={proc.returncode}; stderr tail: {proc.stderr[-500:]}"
        )
    else:
        passed += 1

    # Threshold checks against the produced report.
    thresholds = spec.get("thresholds", {})
    if proc.returncode == 0 and out_path.is_file():
        report = json.loads(out_path.read_text())
        agg = report.get("aggregate_weighted_score") or report.get("aggregate", {}).get(
            "weighted_score"
        )
        min_agg = thresholds.get("min_aggregate_weighted_score")
        if min_agg is not None and agg is not None:
            assertions += 1
            if agg < min_agg:
                failures.append(
                    f"thresholds.min_aggregate_weighted_score: spec={min_agg} actual={agg:.4f}"
                )
            else:
                passed += 1

        # Truncation count.
        max_trunc = thresholds.get("max_truncated_rows", 0)
        rows_data = report.get("rows", [])
        truncated = sum(1 for r in rows_data if r.get("finish_reason") == "length")
        assertions += 1
        if truncated > max_trunc:
            failures.append(
                f"thresholds.max_truncated_rows: spec={max_trunc} actual={truncated}"
            )
        else:
            passed += 1

    duration_ms = (time.monotonic() - start) * 1000
    status = "pass" if not failures else "fail"
    return build_result(
        spec_path=spec_path.relative_to(REPO_ROOT),
        status=status,
        duration_ms=duration_ms,
        assertions_evaluated=assertions,
        assertions_passed=passed,
        failures=failures,
        warnings=warnings,
    )


# ── main ─────────────────────────────────────────────────────────────────────


def main() -> int:
    ap = argparse.ArgumentParser(description="UBENCH runner")
    ap.add_argument("--spec", required=True, help="Path to .ubench.json spec")
    ap.add_argument(
        "--mode",
        choices=["lint", "contract", "run"],
        default="lint",
        help="Validation depth (default: lint)",
    )
    ap.add_argument("--report", help="Path to write canonical UxTS report JSON")
    ap.add_argument("--quiet", action="store_true", help="Suppress summary output")
    args = ap.parse_args()

    spec_path = Path(args.spec).resolve()
    if not spec_path.is_file():
        print(f"error: spec not found: {spec_path}", file=sys.stderr)
        return 2

    try:
        spec = json.loads(spec_path.read_text())
    except json.JSONDecodeError as e:
        print(f"error: spec invalid JSON: {e}", file=sys.stderr)
        return 2

    try:
        schema = json.loads(SCHEMA_PATH.read_text())
    except (FileNotFoundError, json.JSONDecodeError) as e:
        print(f"error: schema load: {e}", file=sys.stderr)
        return 2

    started = datetime.now(timezone.utc)
    results = []

    # Always run lint first.
    lint_res = run_lint(spec_path, spec, schema)
    results.append(lint_res)

    if args.mode in ("contract", "run") and lint_res["status"] == "pass":
        results.append(run_contract(spec_path, spec))

    if args.mode == "run" and all(r["status"] == "pass" for r in results):
        results.append(run_full(spec_path, spec))

    report = build_report(
        framework="ubench",
        framework_version=UBENCH_VERSION,
        results=results,
        start_time=started,
    )

    if args.report:
        save_report(report, args.report)

    if not args.quiet:
        print_summary(report)

    has_error = any(r["status"] == "error" for r in results)
    has_fail = any(r["status"] == "fail" for r in results)
    if has_error:
        return 2
    if has_fail:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
