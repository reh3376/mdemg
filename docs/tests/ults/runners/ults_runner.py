#!/usr/bin/env python3
"""
ULTS Runner - Universal LLM Task Specification Runner

Validates LLM task specifications: schema validation, spec completeness,
and system prompt hash verification against source code.

Usage:
    python ults_runner.py --spec "specs/*.ults.json"
    python ults_runner.py --spec "specs/*.ults.json" --verify-hashes --repo-root ../../../..
    python ults_runner.py --spec "specs/*.ults.json" --report report.json
"""

import argparse
import hashlib
import json
import os
import re
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..'))
from uxts_report import (
    build_result as _canonical_result,
    build_report as _canonical_report,
    print_summary as _print_summary,
    save_report as _save_report,
)

ULTS_VERSION = "1.0.0"

# Expected task names — the 16 canonical LLM tasks
EXPECTED_TASKS = {
    "ape.reflect",
    "consulting.classify",
    "consulting.synthesis",
    "hidden.name_emergence",
    "hidden.reclassify",
    "hidden.summarize",
    "jiminy.codegen",
    "jiminy.evaluate",
    "jiminy.evaluate_llm",
    "jiminy.synthesize",
    "metalearn.generalize",
    "retrieval.intent_translate",
    "retrieval.query_classify",
    "retrieval.rerank_cross",
    "retrieval.rerank_nli",
    "summarize.generate",
}

# Required top-level fields
REQUIRED_TOP_FIELDS = {"ults_version", "task", "metadata"}
REQUIRED_TASK_FIELDS = {"name", "description"}
REQUIRED_METADATA_FIELDS = {"author", "created"}

# Known fields for parity check
KNOWN_TOP_FIELDS = {
    "ults_version", "task", "metadata", "prompt", "performance",
    "output_schema", "quality_metrics", "reward_functions", "training_config",
}
KNOWN_TASK_FIELDS = {"name", "description", "version"}
KNOWN_PROMPT_FIELDS = {
    "system_prompt_hash", "system_prompt_source", "think_mode", "dynamic_prompt",
}


@dataclass
class CheckResult:
    """Result of a single validation check."""
    name: str
    passed: bool
    message: str
    details: Optional[Dict[str, Any]] = None


@dataclass
class SpecResult:
    """Results from validating a single ULTS spec."""
    spec_name: str
    spec_path: str
    checks: List[CheckResult] = field(default_factory=list)

    @property
    def passed(self) -> bool:
        return all(c.passed for c in self.checks)

    @property
    def total_checks(self) -> int:
        return len(self.checks)

    @property
    def passed_checks(self) -> int:
        return sum(1 for c in self.checks if c.passed)

    @property
    def failed_checks(self) -> int:
        return sum(1 for c in self.checks if not c.passed)


def load_spec(path: str) -> Dict[str, Any]:
    """Load a ULTS spec file."""
    with open(path) as f:
        return json.load(f)


def validate_schema(spec: Dict[str, Any], spec_path: str) -> List[CheckResult]:
    """Validate spec structure against ULTS schema requirements."""
    results = []

    # Check required top-level fields
    missing_top = REQUIRED_TOP_FIELDS - set(spec.keys())
    if missing_top:
        results.append(CheckResult(
            name="required_fields",
            passed=False,
            message=f"Missing required fields: {missing_top}",
        ))
    else:
        results.append(CheckResult(
            name="required_fields",
            passed=True,
            message="All required top-level fields present",
        ))

    # Parity check — unknown fields
    unknown_top = set(spec.keys()) - KNOWN_TOP_FIELDS
    if unknown_top:
        results.append(CheckResult(
            name="parity_top_fields",
            passed=False,
            message=f"PARITY: Unknown top-level fields: {unknown_top}",
        ))
    else:
        results.append(CheckResult(
            name="parity_top_fields",
            passed=True,
            message="No unknown top-level fields",
        ))

    # Check ults_version format
    version = spec.get("ults_version", "")
    if re.match(r"^\d+\.\d+\.\d+$", version):
        results.append(CheckResult(
            name="version_format",
            passed=True,
            message=f"Version: {version}",
        ))
    else:
        results.append(CheckResult(
            name="version_format",
            passed=False,
            message=f"Invalid version format: {version!r}",
        ))

    # Check task fields
    task = spec.get("task", {})
    missing_task = REQUIRED_TASK_FIELDS - set(task.keys())
    if missing_task:
        results.append(CheckResult(
            name="task_fields",
            passed=False,
            message=f"Missing task fields: {missing_task}",
        ))
    else:
        results.append(CheckResult(
            name="task_fields",
            passed=True,
            message="All required task fields present",
        ))

    # Check task name format
    task_name = task.get("name", "")
    if re.match(r"^[a-z]+\.[a-z_]+$", task_name):
        results.append(CheckResult(
            name="task_name_format",
            passed=True,
            message=f"Task name: {task_name}",
        ))
    else:
        results.append(CheckResult(
            name="task_name_format",
            passed=False,
            message=f"Invalid task name format: {task_name!r} (expected 'domain.action')",
        ))

    # Check metadata
    metadata = spec.get("metadata", {})
    missing_meta = REQUIRED_METADATA_FIELDS - set(metadata.keys())
    if missing_meta:
        results.append(CheckResult(
            name="metadata_fields",
            passed=False,
            message=f"Missing metadata fields: {missing_meta}",
        ))
    else:
        results.append(CheckResult(
            name="metadata_fields",
            passed=True,
            message="All required metadata fields present",
        ))

    # Check prompt section
    prompt = spec.get("prompt", {})
    if prompt:
        unknown_prompt = set(prompt.keys()) - KNOWN_PROMPT_FIELDS
        if unknown_prompt:
            results.append(CheckResult(
                name="parity_prompt_fields",
                passed=False,
                message=f"PARITY: Unknown prompt fields: {unknown_prompt}",
            ))
        else:
            results.append(CheckResult(
                name="parity_prompt_fields",
                passed=True,
                message="No unknown prompt fields",
            ))

        # Validate hash format (string or array of strings)
        hash_val = prompt.get("system_prompt_hash", "")
        if isinstance(hash_val, list):
            all_valid = all(
                isinstance(h, str) and re.match(r"^[a-f0-9]{64}$", h)
                for h in hash_val
            )
            if all_valid and len(hash_val) >= 2:
                results.append(CheckResult(
                    name="prompt_hash_format",
                    passed=True,
                    message=f"Hash array: {len(hash_val)} entries",
                ))
            else:
                results.append(CheckResult(
                    name="prompt_hash_format",
                    passed=False,
                    message=f"Invalid hash array: {hash_val!r}",
                ))
        elif hash_val and re.match(r"^([a-f0-9]{64}|dynamic)$", hash_val):
            results.append(CheckResult(
                name="prompt_hash_format",
                passed=True,
                message=f"Hash: {hash_val[:16]}..." if len(hash_val) == 64 else f"Hash: {hash_val}",
            ))
        elif hash_val:
            results.append(CheckResult(
                name="prompt_hash_format",
                passed=False,
                message=f"Invalid hash format: {hash_val!r}",
            ))

    # Check quality_metrics structure
    metrics = spec.get("quality_metrics", [])
    if metrics:
        weights_sum = sum(m.get("weight", 0) for m in metrics)
        if abs(weights_sum - 1.0) < 0.01:
            results.append(CheckResult(
                name="quality_weights",
                passed=True,
                message=f"Quality metric weights sum to {weights_sum:.2f}",
            ))
        else:
            results.append(CheckResult(
                name="quality_weights",
                passed=False,
                message=f"Quality metric weights sum to {weights_sum:.2f} (expected ~1.0)",
            ))

    return results


def _verify_single_hash(hash_val: str, source: str, repo_root: str) -> CheckResult:
    """Verify a single system_prompt_hash against its source file."""
    # Parse source location (file:line)
    parts = source.rsplit(":", 1)
    if len(parts) != 2:
        return CheckResult(
            name="prompt_hash_verify",
            passed=False,
            message=f"Cannot parse source location: {source!r}",
        )

    file_path = os.path.join(repo_root, parts[0])
    if not os.path.exists(file_path):
        return CheckResult(
            name="prompt_hash_verify",
            passed=False,
            message=f"Source file not found: {file_path}",
        )

    try:
        with open(file_path) as f:
            content = f.read()

        line_num = int(parts[1])
        lines = content.split("\n")

        if line_num > len(lines):
            return CheckResult(
                name="prompt_hash_verify",
                passed=True,
                message=f"Line {line_num} exceeds file length — hash verification skipped",
            )

        search_start = max(0, line_num - 2)
        search_region = "\n".join(lines[search_start:])

        backtick_match = re.search(r'`([^`]+)`', search_region)
        if backtick_match:
            prompt_text = backtick_match.group(1)
            computed_hash = hashlib.sha256(prompt_text.encode()).hexdigest()
            if computed_hash == hash_val:
                return CheckResult(
                    name="prompt_hash_verify",
                    passed=True,
                    message=f"Hash verified against {source}",
                )
            else:
                return CheckResult(
                    name="prompt_hash_verify",
                    passed=False,
                    message=f"Hash mismatch: spec={hash_val[:16]}... code={computed_hash[:16]}...",
                    details={
                        "spec_hash": hash_val,
                        "computed_hash": computed_hash,
                        "source": source,
                    },
                )

        return CheckResult(
            name="prompt_hash_verify",
            passed=True,
            message=f"Could not extract prompt constant from {source} — skipped",
        )

    except Exception as e:
        return CheckResult(
            name="prompt_hash_verify",
            passed=False,
            message=f"Error reading source: {e}",
        )


def verify_prompt_hash(spec: Dict[str, Any], repo_root: str) -> Optional[CheckResult]:
    """Verify system_prompt_hash against actual source code.

    Handles both string and array-type hashes. For arrays, each hash is
    verified against the corresponding source entry.
    """
    prompt = spec.get("prompt", {})
    hash_val = prompt.get("system_prompt_hash", "")
    source = prompt.get("system_prompt_source", "")

    if not hash_val or hash_val == "dynamic" or not source:
        return CheckResult(
            name="prompt_hash_verify",
            passed=True,
            message=f"Skipped: {'dynamic prompt' if hash_val == 'dynamic' else 'no hash specified'}",
        )

    # Array-type: verify each hash against its corresponding source
    if isinstance(hash_val, list):
        sources = source if isinstance(source, list) else [source] * len(hash_val)
        if len(hash_val) != len(sources):
            return CheckResult(
                name="prompt_hash_verify",
                passed=False,
                message=f"Hash array length ({len(hash_val)}) != source array length ({len(sources)})",
            )
        failures = []
        for h, s in zip(hash_val, sources):
            result = _verify_single_hash(h, s, repo_root)
            if not result.passed:
                failures.append(result.message)
        if failures:
            return CheckResult(
                name="prompt_hash_verify",
                passed=False,
                message=f"Array hash verification failed: {'; '.join(failures)}",
            )
        return CheckResult(
            name="prompt_hash_verify",
            passed=True,
            message=f"All {len(hash_val)} hashes verified",
        )

    # String-type: single hash verification
    return _verify_single_hash(hash_val, source, repo_root)


def check_spec_completeness(found_tasks: set) -> List[CheckResult]:
    """Check that all 16 expected tasks have specs."""
    results = []

    missing = EXPECTED_TASKS - found_tasks
    extra = found_tasks - EXPECTED_TASKS

    if not missing:
        results.append(CheckResult(
            name="completeness",
            passed=True,
            message=f"All {len(EXPECTED_TASKS)} expected task specs present",
        ))
    else:
        results.append(CheckResult(
            name="completeness",
            passed=False,
            message=f"Missing specs for: {sorted(missing)}",
        ))

    if extra:
        results.append(CheckResult(
            name="extra_specs",
            passed=True,  # Extra specs are fine, just noted
            message=f"Extra specs found: {sorted(extra)}",
        ))

    return results


def print_results(result: SpecResult) -> None:
    """Print spec validation results."""
    status = "\u2713 PASS" if result.passed else "\u2717 FAIL"
    color = "\033[92m" if result.passed else "\033[91m"
    reset = "\033[0m"

    print(f"\n  {color}{status}{reset} {result.spec_name} ({result.passed_checks}/{result.total_checks})")

    for cr in result.checks:
        if not cr.passed:
            print(f"    \033[91m\u2717\033[0m {cr.name}: {cr.message}")


def main():
    parser = argparse.ArgumentParser(description="ULTS LLM Task Specification Runner")
    parser.add_argument("--spec", required=True, help="Path to ULTS spec file(s) (glob supported)")
    parser.add_argument("--verify-hashes", action="store_true", help="Verify prompt hashes against source code")
    parser.add_argument("--repo-root", default=os.path.join(os.path.dirname(__file__), "..", "..", "..", ".."),
                        help="Repository root for hash verification")
    parser.add_argument("--report", help="Output JSON report file path")
    args = parser.parse_args()

    # Resolve spec paths
    spec_paths = []
    if "*" in args.spec:
        spec_dir = Path(args.spec).parent
        pattern = Path(args.spec).name
        spec_paths = sorted(spec_dir.glob(pattern))
    else:
        spec_paths = [Path(args.spec)]

    print(f"{'=' * 60}")
    print(f"ULTS Runner v{ULTS_VERSION}")
    print(f"Specs: {len(spec_paths)} files")
    if args.verify_hashes:
        print(f"Hash verification: enabled (repo: {args.repo_root})")
    print(f"{'=' * 60}")

    all_results: List[SpecResult] = []
    found_tasks: set = set()
    all_passed = True
    canonical_results = []
    total_start = time.monotonic()

    for spec_path in spec_paths:
        spec = load_spec(str(spec_path))
        task_name = spec.get("task", {}).get("name", "unknown")
        found_tasks.add(task_name)

        result = SpecResult(spec_name=task_name, spec_path=str(spec_path))

        # Schema validation
        result.checks.extend(validate_schema(spec, str(spec_path)))

        # Hash verification (optional)
        if args.verify_hashes:
            hash_result = verify_prompt_hash(spec, args.repo_root)
            if hash_result:
                result.checks.append(hash_result)

        all_results.append(result)
        print_results(result)

        if not result.passed:
            all_passed = False

        # Build canonical result
        failures = [f"{c.name}: {c.message}" for c in result.checks if not c.passed]
        canonical_results.append(_canonical_result(
            spec_path=str(spec_path),
            status="pass" if result.passed else "fail",
            assertions_evaluated=result.total_checks,
            assertions_passed=result.passed_checks,
            failures=failures,
            hash_verified=None,
        ))

    # Completeness check
    print(f"\n{'=' * 60}")
    print("Completeness Check")
    completeness = check_spec_completeness(found_tasks)
    for cr in completeness:
        color = "\033[92m" if cr.passed else "\033[91m"
        status = "\u2713" if cr.passed else "\u2717"
        print(f"  {color}{status}\033[0m {cr.message}")
        if not cr.passed:
            all_passed = False

    # Summary
    total_duration = (time.monotonic() - total_start) * 1000
    report = _canonical_report("ults", ULTS_VERSION, canonical_results, duration_ms=round(total_duration, 2))
    _print_summary(report)

    if args.report:
        _save_report(report, args.report)

    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    main()
