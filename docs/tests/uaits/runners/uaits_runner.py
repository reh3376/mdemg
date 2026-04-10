#!/usr/bin/env python3
"""
UAITS Runner - Universal AI Training Specification Runner

Validates UAITS specs: schema validation, dataset declarations, and
optionally validates actual training data files against spec contracts.

Usage:
    python uaits_runner.py --spec "specs/*.uaits.json"
    python uaits_runner.py --spec "specs/*.uaits.json" --data-dir /tmp/exported/
    python uaits_runner.py --spec "specs/*.uaits.json" --report report.json
"""

import argparse
import json
import os
import re
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..'))
from uxts_report import build_report as _canonical_report  # noqa: E402
from uxts_report import build_result as _canonical_result  # noqa: E402
from uxts_report import print_summary as _print_summary  # noqa: E402
from uxts_report import save_report as _save_report  # noqa: E402

UAITS_VERSION = "1.0.0"

# Valid enum values
VALID_PARADIGMS = {"sft", "dpo", "raft", "curriculum"}
VALID_OUTPUT_TYPES = {"chat", "dpo", "raft", "metadata"}
VALID_DEDUP_MODES = {"prompt", "prompt-response", "trace"}
VALID_GATE_TYPES = {"hard", "soft"}

# Paradigm -> expected output_type mapping
PARADIGM_OUTPUT_MAP = {
    "sft": "chat",
    "dpo": "dpo",
    "raft": "raft",
    "curriculum": "metadata",
}

# Known TSDB tables
KNOWN_TABLES = {
    "llm_interactions", "constraint_outcomes", "metric_samples",
    "embedding_events", "retrieval_events", "ft_benchmarks",
    "ft_training_cycles", "ft_model_versions", "ft_hitl_decisions",
}

# Required fields
REQUIRED_TOP_FIELDS = {"uaits_version", "application", "metadata", "datasets"}
KNOWN_TOP_FIELDS = {"uaits_version", "application", "metadata", "datasets"}
REQUIRED_APP_FIELDS = {"name", "version"}
REQUIRED_META_FIELDS = {"author", "created", "description"}
REQUIRED_DATASET_FIELDS = {
    "name", "paradigm", "description", "source", "quality_gates", "format", "versioning",
}
OPTIONAL_DATASET_FIELDS = {"training_config"}
KNOWN_DATASET_FIELDS = REQUIRED_DATASET_FIELDS | OPTIONAL_DATASET_FIELDS


@dataclass
class CheckResult:
    """Result of a single validation check."""
    name: str
    passed: bool
    message: str
    details: dict[str, Any] | None = None


@dataclass
class SpecResult:
    """Results from validating a single UAITS spec."""
    spec_name: str
    spec_path: str
    checks: list[CheckResult] = field(default_factory=list)

    @property
    def passed(self) -> bool:
        return all(c.passed for c in self.checks)

    @property
    def total_checks(self) -> int:
        return len(self.checks)

    @property
    def passed_checks(self) -> int:
        return sum(1 for c in self.checks if c.passed)


def load_spec(path: str) -> dict[str, Any]:
    """Load a UAITS spec file."""
    with open(path) as f:
        return json.load(f)


def validate_schema(spec: dict[str, Any]) -> list[CheckResult]:
    """Validate spec structure against UAITS schema requirements."""
    results = []

    # 1. Required top-level fields
    missing_top = REQUIRED_TOP_FIELDS - set(spec.keys())
    results.append(CheckResult(
        name="required_fields",
        passed=not missing_top,
        message=f"Missing required fields: {missing_top}" if missing_top
        else "All required top-level fields present",
    ))

    # 2. No unknown top-level fields
    unknown_top = set(spec.keys()) - KNOWN_TOP_FIELDS
    results.append(CheckResult(
        name="parity_top_fields",
        passed=not unknown_top,
        message=f"PARITY: Unknown top-level fields: {unknown_top}" if unknown_top
        else "No unknown top-level fields",
    ))

    # 3. Version format
    version = spec.get("uaits_version", "")
    version_ok = bool(re.match(r"^\d+\.\d+\.\d+$", version))
    results.append(CheckResult(
        name="version_format",
        passed=version_ok,
        message=f"Version: {version}" if version_ok
        else f"Invalid version format: {version!r}",
    ))

    # 4. Application fields
    app = spec.get("application", {})
    missing_app = REQUIRED_APP_FIELDS - set(app.keys())
    results.append(CheckResult(
        name="application_fields",
        passed=not missing_app,
        message=f"Missing application fields: {missing_app}" if missing_app
        else f"Application: {app.get('name', '?')} v{app.get('version', '?')}",
    ))

    # 5. Metadata fields
    meta = spec.get("metadata", {})
    missing_meta = REQUIRED_META_FIELDS - set(meta.keys())
    results.append(CheckResult(
        name="metadata_fields",
        passed=not missing_meta,
        message=f"Missing metadata fields: {missing_meta}" if missing_meta
        else "All required metadata fields present",
    ))

    # 6-15. Per-dataset validation
    datasets = spec.get("datasets", [])
    if not datasets:
        results.append(CheckResult(
            name="datasets_present",
            passed=False,
            message="No datasets declared",
        ))
        return results

    results.append(CheckResult(
        name="datasets_present",
        passed=True,
        message=f"{len(datasets)} dataset(s) declared",
    ))

    dataset_names = []
    for i, ds in enumerate(datasets):
        prefix = f"dataset[{i}]"
        results.extend(_validate_dataset(ds, prefix))
        dataset_names.append(ds.get("name", f"unnamed_{i}"))

    # 12. Dataset names unique
    dupes = [n for n in dataset_names if dataset_names.count(n) > 1]
    results.append(CheckResult(
        name="dataset_names_unique",
        passed=not dupes,
        message=f"Duplicate dataset names: {set(dupes)}" if dupes
        else "All dataset names unique",
    ))

    return results


def _validate_dataset(ds: dict[str, Any], prefix: str) -> list[CheckResult]:
    """Validate a single dataset declaration."""
    results = []

    # 6. Required dataset fields
    missing = REQUIRED_DATASET_FIELDS - set(ds.keys())
    results.append(CheckResult(
        name=f"{prefix}.required_fields",
        passed=not missing,
        message=f"Missing: {missing}" if missing
        else f"{ds.get('name', '?')}: all required fields present",
    ))

    # Dataset name format
    name = ds.get("name", "")
    name_ok = bool(re.match(r"^[a-z][a-z0-9_]+$", name))
    results.append(CheckResult(
        name=f"{prefix}.name_format",
        passed=name_ok,
        message=f"Invalid name format: {name!r}" if not name_ok
        else f"Name: {name}",
    ))

    # 7. Paradigm is valid enum
    paradigm = ds.get("paradigm", "")
    results.append(CheckResult(
        name=f"{prefix}.paradigm_valid",
        passed=paradigm in VALID_PARADIGMS,
        message=f"Invalid paradigm: {paradigm!r}" if paradigm not in VALID_PARADIGMS
        else f"Paradigm: {paradigm}",
    ))

    # 8. Output type matches paradigm
    fmt = ds.get("format", {})
    output_type = fmt.get("output_type", "")
    expected_output = PARADIGM_OUTPUT_MAP.get(paradigm, "")
    type_match = output_type == expected_output
    results.append(CheckResult(
        name=f"{prefix}.output_type_matches_paradigm",
        passed=type_match,
        message=(
            f"output_type '{output_type}' != paradigm '{paradigm}' (expected '{expected_output}')"
            if not type_match else f"Output type '{output_type}' matches paradigm"
        ),
    ))

    # 9. Temporal split must be true
    versioning = ds.get("versioning", {})
    temporal = versioning.get("temporal_split")
    results.append(CheckResult(
        name=f"{prefix}.temporal_split",
        passed=temporal is True,
        message="temporal_split is not true — random splits are prohibited"
        if temporal is not True else "Temporal split enforced",
    ))

    # 10. Dedup mode valid
    gates = ds.get("quality_gates", {})
    dedup = gates.get("dedup_mode", "prompt")
    results.append(CheckResult(
        name=f"{prefix}.dedup_mode_valid",
        passed=dedup in VALID_DEDUP_MODES,
        message=f"Invalid dedup_mode: {dedup!r}" if dedup not in VALID_DEDUP_MODES
        else f"Dedup mode: {dedup}",
    ))

    # 11. Split ratios sum to ~1.0
    train = versioning.get("train_ratio", 0)
    test = versioning.get("test_ratio", 0)
    val = versioning.get("val_ratio", 0)
    ratio_sum = train + test + val
    ratios_ok = abs(ratio_sum - 1.0) < 0.01
    results.append(CheckResult(
        name=f"{prefix}.split_ratios",
        passed=ratios_ok,
        message=f"Split ratios sum to {ratio_sum:.3f} (expected ~1.0)"
        if not ratios_ok else f"Split ratios: {train}/{test}/{val}",
    ))

    # 13. ULTS task names format (if present)
    tc = ds.get("training_config", {})
    task_names = tc.get("ults_task_names", [])
    if task_names:
        bad_names = [n for n in task_names if not re.match(r"^[a-z]+\.[a-z_]+$", n)]
        results.append(CheckResult(
            name=f"{prefix}.ults_task_names_format",
            passed=not bad_names,
            message=f"Invalid ULTS task name format: {bad_names}" if bad_names
            else f"{len(task_names)} ULTS task names valid",
        ))

    # 14. Source table is known
    source = ds.get("source", {})
    table = source.get("primary_table", "")
    results.append(CheckResult(
        name=f"{prefix}.source_table_known",
        passed=table in KNOWN_TABLES,
        message=f"Unknown source table: {table!r}" if table not in KNOWN_TABLES
        else f"Source table: {table}",
    ))

    # Custom gates validation
    custom_gates = gates.get("custom_gates", [])
    for cg in custom_gates:
        gate_type = cg.get("type", "")
        if gate_type not in VALID_GATE_TYPES:
            results.append(CheckResult(
                name=f"{prefix}.custom_gate_type",
                passed=False,
                message=f"Invalid custom gate type: {gate_type!r} in gate '{cg.get('name', '?')}'",
            ))

    return results


def validate_data(spec: dict[str, Any], data_dir: str) -> list[CheckResult]:
    """Validate actual data files against spec declarations."""
    results = []
    data_path = Path(data_dir)

    for ds in spec.get("datasets", []):
        name = ds.get("name", "unnamed")
        paradigm = ds.get("paradigm", "")
        prefix = f"data:{name}"

        # Check JSONL file exists
        jsonl_file = data_path / f"{name}.jsonl"
        if not jsonl_file.exists():
            results.append(CheckResult(
                name=f"{prefix}.file_exists",
                passed=False,
                message=f"Missing data file: {jsonl_file}",
            ))
            continue

        results.append(CheckResult(
            name=f"{prefix}.file_exists",
            passed=True,
            message=f"Found: {jsonl_file}",
        ))

        # Load and validate records (sample first 100)
        records = []
        try:
            with open(jsonl_file) as f:
                for i, line in enumerate(f):
                    if i >= 100:
                        break
                    line = line.strip()
                    if line:
                        records.append(json.loads(line))
        except (json.JSONDecodeError, OSError) as e:
            results.append(CheckResult(
                name=f"{prefix}.file_readable",
                passed=False,
                message=f"Error reading {jsonl_file}: {e}",
            ))
            continue

        # Row count
        with open(jsonl_file) as fh:
            total_lines = sum(1 for line in fh if line.strip())
        min_examples = ds.get("training_config", {}).get("min_examples", 0)
        meets_min = total_lines >= min_examples if min_examples else True
        results.append(CheckResult(
            name=f"{prefix}.row_count",
            passed=meets_min,
            message=f"{total_lines} records (min: {min_examples})"
            if min_examples else f"{total_lines} records",
        ))

        # Paradigm-specific field checks
        if paradigm == "sft":
            results.extend(_check_sft_records(records, prefix))
        elif paradigm == "dpo":
            results.extend(_check_dpo_records(records, prefix))
        elif paradigm == "raft":
            results.extend(_check_raft_records(records, prefix))

    return results


def _check_sft_records(records: list, prefix: str) -> list[CheckResult]:
    """Check SFT records have required fields."""
    missing = 0
    for r in records:
        if not r.get("system_prompt") or not r.get("user_prompt") or not r.get("response"):
            missing += 1
    return [CheckResult(
        name=f"{prefix}.sft_fields",
        passed=missing == 0,
        message=f"{missing}/{len(records)} records missing system_prompt/user_prompt/response"
        if missing else f"All {len(records)} sampled records have required SFT fields",
    )]


def _check_dpo_records(records: list, prefix: str) -> list[CheckResult]:
    """Check DPO records have required fields."""
    missing = 0
    for r in records:
        if not r.get("prompt") or not r.get("chosen") or not r.get("rejected"):
            missing += 1
    return [CheckResult(
        name=f"{prefix}.dpo_fields",
        passed=missing == 0,
        message=f"{missing}/{len(records)} records missing prompt/chosen/rejected"
        if missing else f"All {len(records)} sampled records have required DPO fields",
    )]


def _check_raft_records(records: list, prefix: str) -> list[CheckResult]:
    """Check RAFT records have retrieval context."""
    missing = 0
    for r in records:
        if not r.get("retrieval_node_ids"):
            missing += 1
    return [CheckResult(
        name=f"{prefix}.raft_context",
        passed=missing == 0,
        message=f"{missing}/{len(records)} records missing retrieval_node_ids"
        if missing else f"All {len(records)} sampled records have retrieval context",
    )]


def print_results(result: SpecResult) -> None:
    """Print spec validation results."""
    status = "\u2713 PASS" if result.passed else "\u2717 FAIL"
    color = "\033[92m" if result.passed else "\033[91m"
    reset = "\033[0m"

    checks = f"{result.passed_checks}/{result.total_checks}"
    print(f"\n  {color}{status}{reset} {result.spec_name} ({checks})")

    for cr in result.checks:
        if not cr.passed:
            print(f"    \033[91m\u2717\033[0m {cr.name}: {cr.message}")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="UAITS Training Data Specification Runner",
    )
    parser.add_argument(
        "--spec", required=True,
        help="Path to UAITS spec file(s) (glob supported)",
    )
    parser.add_argument(
        "--data-dir",
        help="Directory with exported JSONL data for compliance checks",
    )
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
    print(f"UAITS Runner v{UAITS_VERSION}")
    print(f"Specs: {len(spec_paths)} files")
    if args.data_dir:
        print(f"Data dir: {args.data_dir}")
    print(f"{'=' * 60}")

    all_passed = True
    canonical_results = []
    total_start = time.monotonic()

    for spec_path in spec_paths:
        spec = load_spec(str(spec_path))
        app_name = spec.get("application", {}).get("name", "unknown")

        result = SpecResult(spec_name=app_name, spec_path=str(spec_path))

        # Schema validation
        result.checks.extend(validate_schema(spec))

        # Data compliance (optional)
        if args.data_dir:
            result.checks.extend(validate_data(spec, args.data_dir))

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

    # Summary
    total_duration = (time.monotonic() - total_start) * 1000
    report = _canonical_report(
        "uaits", UAITS_VERSION, canonical_results,
        duration_ms=round(total_duration, 2),
    )
    _print_summary(report)

    if args.report:
        _save_report(report, args.report)

    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    main()
