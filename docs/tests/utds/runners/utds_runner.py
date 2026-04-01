#!/usr/bin/env python3
"""
UTDS Runner - Universal Training Data Specification Runner

Validates MDEMG training data export archives: manifest schema validation,
SHA-256 checksums, row counts, privacy gates, and JSONL field verification.

Usage:
    python utds_runner.py validate --archive export.tar.gz
    python utds_runner.py validate --archive export.tar.gz --strict
    python utds_runner.py validate-all --dir exports/
    python utds_runner.py self-check
    python utds_runner.py self-check --report report.json
"""

import argparse
import gzip
import hashlib
import io
import json
import os
import sys
import tarfile
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, List, Optional

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..'))
from uxts_report import (
    build_result as _canonical_result,
    build_report as _canonical_report,
    print_summary as _print_summary,
    save_report as _save_report,
)

UTDS_VERSION = "1.0.0"
SCHEMA_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'schema')
SPECS_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'specs')

# Expected JSONL columns per table
EXPECTED_COLUMNS = {
    "llm_interactions": {
        "time", "trace_id", "task_name", "space_id", "session_id",
        "system_prompt", "user_prompt", "response", "think_content", "think_mode",
        "latency_ms", "tokens_in", "tokens_out", "model_name", "provider",
        "error", "quality", "quality_source", "used_for_train", "dataset_ver",
        "guidance_id", "source_path", "retrieval_node_ids", "retrieval_scores",
        "oracle_node_id", "system_prompt_hash",
        "instance_id",
    },
    "retrieval_events": {
        "time", "event_id", "space_id", "call_site", "query_text", "query_hash",
        "recall_node_ids", "recall_scores", "recall_k",
        "bm25_node_ids", "bm25_scores",
        "rerank_node_ids", "rerank_scores", "rerank_model",
        "result_node_ids", "result_scores", "result_count",
        "guidance_id", "downstream_quality",
        "recall_latency_ms", "rerank_latency_ms", "total_latency_ms",
        "instance_id",
    },
    "embedding_events": {
        "time", "event_id", "event_type", "space_id", "text_content", "text_hash",
        "text_length", "element_kind", "language", "file_path",
        "chunk_start", "chunk_end", "package_name", "signature", "tags",
        "call_site", "query_text", "model_name", "provider", "dimensions",
        "latency_ms", "cached", "node_id",
        "instance_id",
    },
}


@dataclass
class CheckResult:
    """Result of a single validation check."""
    name: str
    passed: bool
    message: str
    details: Optional[Dict[str, Any]] = None


@dataclass
class ValidationResult:
    """Results from validating a single archive or spec."""
    target_name: str
    target_path: str
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


def load_utds_schema() -> Dict[str, Any]:
    """Load the UTDS JSON Schema."""
    schema_path = os.path.join(SCHEMA_DIR, 'utds.schema.json')
    with open(schema_path) as f:
        return json.load(f)


def validate_manifest_against_schema(manifest: Dict[str, Any], schema: Dict[str, Any]) -> List[CheckResult]:
    """Validate manifest against UTDS JSON Schema using manual checks.

    We avoid jsonschema dependency in the runner — it's a test-time tool that
    should work with zero pip installs. Instead, we enforce the schema's
    constraints directly.
    """
    results = []

    # Required top-level fields
    required = schema.get("required", [])
    missing = [f for f in required if f not in manifest]
    if missing:
        results.append(CheckResult("required_fields", False, f"Missing required fields: {missing}"))
    else:
        results.append(CheckResult("required_fields", True, f"All {len(required)} required fields present"))

    # No unknown top-level fields
    allowed = set(schema.get("properties", {}).keys())
    unknown = set(manifest.keys()) - allowed
    if unknown:
        results.append(CheckResult("no_extra_fields", False, f"Unknown top-level fields: {unknown}"))
    else:
        results.append(CheckResult("no_extra_fields", True, "No unknown top-level fields"))

    # utds_version format
    v = manifest.get("utds_version", "")
    import re
    if re.match(r"^\d+\.\d+\.\d+$", v):
        results.append(CheckResult("utds_version_format", True, f"Version: {v}"))
    else:
        results.append(CheckResult("utds_version_format", False, f"Invalid version format: {v!r}"))

    # export_id pattern
    eid = manifest.get("export_id", "")
    if eid.startswith("exp-"):
        results.append(CheckResult("export_id_pattern", True, f"Export ID: {eid}"))
    else:
        results.append(CheckResult("export_id_pattern", False, f"Export ID must start with 'exp-': {eid!r}"))

    # instance_id and space_id non-empty
    for fld in ("instance_id", "space_id"):
        val = manifest.get(fld, "")
        if val:
            results.append(CheckResult(f"{fld}_present", True, f"{fld}: {val}"))
        else:
            results.append(CheckResult(f"{fld}_present", False, f"{fld} is empty"))

    # schema_version >= 8 (migration 008 adds instance_id)
    sv = manifest.get("schema_version", 0)
    if isinstance(sv, int) and sv >= 8:
        results.append(CheckResult("schema_version", True, f"Schema version: {sv}"))
    else:
        results.append(CheckResult("schema_version", False, f"Schema version must be >= 8, got: {sv}"))

    # exported_at format (ISO 8601)
    ea = manifest.get("exported_at", "")
    if re.match(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}", ea):
        results.append(CheckResult("exported_at_format", True, f"Exported at: {ea}"))
    else:
        results.append(CheckResult("exported_at_format", False, f"Invalid exported_at: {ea!r}"))

    # data_range
    dr = manifest.get("data_range", {})
    if "from" in dr and "to" in dr:
        results.append(CheckResult("data_range", True, f"Range: {dr['from']} → {dr['to']}"))
    else:
        results.append(CheckResult("data_range", False, "data_range missing 'from' or 'to'"))

    # tables — llm_interactions required
    tables = manifest.get("tables", {})
    if "llm_interactions" in tables:
        results.append(CheckResult("llm_interactions_required", True, "llm_interactions table present"))
    else:
        results.append(CheckResult("llm_interactions_required", False, "Missing required table: llm_interactions"))

    # No unknown tables
    allowed_tables = {"llm_interactions", "retrieval_events", "embedding_events"}
    unknown_tables = set(tables.keys()) - allowed_tables
    if unknown_tables:
        results.append(CheckResult("no_extra_tables", False, f"Unknown tables: {unknown_tables}"))
    else:
        results.append(CheckResult("no_extra_tables", True, "No unknown tables"))

    # Validate each table's structure
    for tname, tmeta in tables.items():
        # row_count
        rc = tmeta.get("row_count")
        if isinstance(rc, int) and rc >= 0:
            results.append(CheckResult(f"{tname}_row_count", True, f"{tname}: {rc} rows"))
        else:
            results.append(CheckResult(f"{tname}_row_count", False, f"{tname}: invalid row_count: {rc}"))

        # sha256
        sha = tmeta.get("sha256", "")
        if re.match(r"^[a-f0-9]{64}$", sha):
            results.append(CheckResult(f"{tname}_sha256_format", True, f"{tname}: SHA-256 valid format"))
        else:
            results.append(CheckResult(f"{tname}_sha256_format", False, f"{tname}: invalid SHA-256: {sha!r}"))

        # columns
        cols = tmeta.get("columns", [])
        if isinstance(cols, list) and len(cols) >= 1:
            results.append(CheckResult(f"{tname}_columns", True, f"{tname}: {len(cols)} columns"))
        else:
            results.append(CheckResult(f"{tname}_columns", False, f"{tname}: columns must be non-empty array"))

    # quality_summary
    qs = manifest.get("quality_summary", {})
    qs_required = ["llm_error_rate", "llm_empty_system_prompt", "llm_empty_response", "privacy_scrub_violations"]
    qs_missing = [f for f in qs_required if f not in qs]
    if qs_missing:
        results.append(CheckResult("quality_summary_fields", False, f"Missing quality fields: {qs_missing}"))
    else:
        results.append(CheckResult("quality_summary_fields", True, "All required quality fields present"))

    # HARD GATE: privacy_scrub_violations must be 0
    psv = qs.get("privacy_scrub_violations")
    if psv is not None and psv == 0:
        results.append(CheckResult("privacy_hard_gate", True, "Privacy scrub violations: 0"))
    else:
        results.append(CheckResult("privacy_hard_gate", False,
                                   f"PRIVACY HARD GATE FAILED: privacy_scrub_violations={psv} (must be 0)"))

    # llm_error_rate in [0, 1]
    er = qs.get("llm_error_rate")
    if er is not None and isinstance(er, (int, float)) and 0 <= er <= 1:
        results.append(CheckResult("llm_error_rate_range", True, f"LLM error rate: {er}"))
    elif er is not None:
        results.append(CheckResult("llm_error_rate_range", False, f"LLM error rate out of range [0,1]: {er}"))

    return results


def validate_archive(archive_path: str, strict: bool = False) -> ValidationResult:
    """Validate a UTDS export archive (.tar.gz).

    Checks:
      1. Archive structure (manifest.json + expected .jsonl files)
      2. Manifest validates against UTDS schema
      3. SHA-256 checksums match actual file contents
      4. Row counts match actual JSONL line counts
      5. privacy_scrub_violations == 0 (hard gate)
      6. Strict mode: every JSONL line parses with expected fields
    """
    result = ValidationResult(
        target_name=os.path.basename(archive_path),
        target_path=archive_path,
    )

    # Open archive
    try:
        tf = tarfile.open(archive_path, "r:gz")
    except Exception as e:
        result.checks.append(CheckResult("archive_open", False, f"Cannot open archive: {e}"))
        return result

    result.checks.append(CheckResult("archive_open", True, "Archive opened successfully"))

    # List members
    members = {m.name: m for m in tf.getmembers()}
    member_names = set(members.keys())

    # manifest.json must exist
    if "manifest.json" not in member_names:
        result.checks.append(CheckResult("manifest_exists", False, "manifest.json not found in archive"))
        tf.close()
        return result

    result.checks.append(CheckResult("manifest_exists", True, "manifest.json found"))

    # Load and parse manifest
    try:
        mf = tf.extractfile("manifest.json")
        manifest = json.load(mf)
    except Exception as e:
        result.checks.append(CheckResult("manifest_parse", False, f"Cannot parse manifest.json: {e}"))
        tf.close()
        return result

    result.checks.append(CheckResult("manifest_parse", True, "manifest.json parsed successfully"))

    # Schema validation
    schema = load_utds_schema()
    result.checks.extend(validate_manifest_against_schema(manifest, schema))

    # Check expected JSONL files exist
    tables = manifest.get("tables", {})
    for tname in tables:
        jsonl_name = f"{tname}.jsonl"
        if jsonl_name in member_names:
            result.checks.append(CheckResult(f"{tname}_file_exists", True, f"{jsonl_name} found in archive"))
        else:
            result.checks.append(CheckResult(f"{tname}_file_exists", False, f"{jsonl_name} missing from archive"))

    # No unexpected files (only manifest.json + expected .jsonl)
    # Filter out directory entries (e.g., '.' or 'subdir/')
    actual_files = {n for n in member_names if not n.endswith("/") and n != "."}
    expected_files = {"manifest.json"} | {f"{t}.jsonl" for t in tables}
    unexpected = actual_files - expected_files
    if unexpected:
        result.checks.append(CheckResult("no_extra_files", False, f"Unexpected files in archive: {unexpected}"))
    else:
        result.checks.append(CheckResult("no_extra_files", True, "No unexpected files in archive"))

    # SHA-256 and row count verification per table
    for tname, tmeta in tables.items():
        jsonl_name = f"{tname}.jsonl"
        if jsonl_name not in member_names:
            continue

        try:
            jf = tf.extractfile(jsonl_name)
            content = jf.read()
        except Exception as e:
            result.checks.append(CheckResult(f"{tname}_read", False, f"Cannot read {jsonl_name}: {e}"))
            continue

        # SHA-256
        actual_sha = hashlib.sha256(content).hexdigest()
        expected_sha = tmeta.get("sha256", "")
        if actual_sha == expected_sha:
            result.checks.append(CheckResult(f"{tname}_sha256_match", True, f"{tname}: SHA-256 verified"))
        else:
            result.checks.append(CheckResult(f"{tname}_sha256_match", False,
                                             f"{tname}: SHA-256 mismatch (expected={expected_sha[:16]}... actual={actual_sha[:16]}...)"))

        # Row count
        lines = content.decode("utf-8").strip().split("\n") if content.strip() else []
        actual_count = len(lines)
        expected_count = tmeta.get("row_count", -1)
        if actual_count == expected_count:
            result.checks.append(CheckResult(f"{tname}_row_count_match", True,
                                             f"{tname}: row count verified ({actual_count})"))
        else:
            result.checks.append(CheckResult(f"{tname}_row_count_match", False,
                                             f"{tname}: row count mismatch (expected={expected_count} actual={actual_count})"))

        # Strict mode: validate every JSONL line
        if strict and lines:
            expected_cols = EXPECTED_COLUMNS.get(tname, set())
            parse_errors = 0
            field_errors = 0
            for i, line in enumerate(lines):
                if not line.strip():
                    continue
                try:
                    record = json.loads(line)
                except json.JSONDecodeError:
                    parse_errors += 1
                    continue

                if expected_cols:
                    record_keys = set(record.keys())
                    missing = expected_cols - record_keys
                    if missing:
                        field_errors += 1

            if parse_errors == 0:
                result.checks.append(CheckResult(f"{tname}_jsonl_parse", True,
                                                 f"{tname}: all {len(lines)} lines parse as valid JSON"))
            else:
                result.checks.append(CheckResult(f"{tname}_jsonl_parse", False,
                                                 f"{tname}: {parse_errors} lines failed JSON parsing"))

            if field_errors == 0:
                result.checks.append(CheckResult(f"{tname}_jsonl_fields", True,
                                                 f"{tname}: all lines have expected fields"))
            else:
                result.checks.append(CheckResult(f"{tname}_jsonl_fields", False,
                                                 f"{tname}: {field_errors} lines missing expected fields"))

    tf.close()
    return result


def validate_spec_manifest(spec_path: str) -> ValidationResult:
    """Validate a standalone UTDS spec fixture (manifest JSON file) against the schema."""
    result = ValidationResult(
        target_name=os.path.basename(spec_path),
        target_path=spec_path,
    )

    try:
        with open(spec_path) as f:
            manifest = json.load(f)
    except Exception as e:
        result.checks.append(CheckResult("spec_parse", False, f"Cannot parse spec: {e}"))
        return result

    result.checks.append(CheckResult("spec_parse", True, "Spec file parsed successfully"))

    schema = load_utds_schema()
    result.checks.extend(validate_manifest_against_schema(manifest, schema))

    return result


def self_check() -> List[ValidationResult]:
    """Validate the UTDS schema itself and all fixture specs against it."""
    results = []

    # Validate schema loads
    schema_result = ValidationResult(target_name="utds.schema.json", target_path=os.path.join(SCHEMA_DIR, 'utds.schema.json'))
    try:
        schema = load_utds_schema()
        schema_result.checks.append(CheckResult("schema_load", True, "UTDS schema loaded successfully"))

        # Check schema has required structure
        if "$schema" in schema:
            schema_result.checks.append(CheckResult("schema_meta", True, f"JSON Schema draft: {schema['$schema']}"))
        if "required" in schema:
            schema_result.checks.append(CheckResult("schema_required", True,
                                                     f"{len(schema['required'])} required fields defined"))
        if "$defs" in schema and "table_meta" in schema["$defs"]:
            schema_result.checks.append(CheckResult("schema_defs", True, "table_meta definition present"))
        else:
            schema_result.checks.append(CheckResult("schema_defs", False, "Missing $defs.table_meta"))

    except Exception as e:
        schema_result.checks.append(CheckResult("schema_load", False, f"Failed to load schema: {e}"))

    results.append(schema_result)

    # Validate all fixture specs
    specs_dir = Path(SPECS_DIR)
    if specs_dir.exists():
        spec_files = sorted(specs_dir.glob("*.utds.json"))
        for spec_path in spec_files:
            results.append(validate_spec_manifest(str(spec_path)))
    else:
        no_specs = ValidationResult(target_name="specs_dir", target_path=str(specs_dir))
        no_specs.checks.append(CheckResult("specs_dir_exists", False, f"Specs directory not found: {specs_dir}"))
        results.append(no_specs)

    return results


def print_results(result: ValidationResult) -> None:
    """Print validation results with color."""
    status = "\u2713 PASS" if result.passed else "\u2717 FAIL"
    color = "\033[92m" if result.passed else "\033[91m"
    reset = "\033[0m"

    print(f"\n  {color}{status}{reset} {result.target_name} ({result.passed_checks}/{result.total_checks})")

    for cr in result.checks:
        if not cr.passed:
            print(f"    \033[91m\u2717\033[0m {cr.name}: {cr.message}")


def main():
    parser = argparse.ArgumentParser(description="UTDS Training Data Specification Runner")
    subparsers = parser.add_subparsers(dest="command", help="Command to run")

    # validate
    val_parser = subparsers.add_parser("validate", help="Validate a single export archive")
    val_parser.add_argument("--archive", required=True, help="Path to .tar.gz export archive")
    val_parser.add_argument("--strict", action="store_true", help="Validate every JSONL line for expected fields")
    val_parser.add_argument("--report", help="Output JSON report file path")

    # validate-all
    val_all_parser = subparsers.add_parser("validate-all", help="Validate all archives in a directory")
    val_all_parser.add_argument("--dir", required=True, help="Directory containing .tar.gz archives")
    val_all_parser.add_argument("--strict", action="store_true", help="Validate every JSONL line")
    val_all_parser.add_argument("--report", help="Output JSON report file path")

    # self-check
    self_parser = subparsers.add_parser("self-check", help="Validate UTDS schema and fixture specs")
    self_parser.add_argument("--report", help="Output JSON report file path")

    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        sys.exit(1)

    print(f"{'=' * 60}")
    print(f"UTDS Runner v{UTDS_VERSION}")
    print(f"Command: {args.command}")
    print(f"{'=' * 60}")

    all_results: List[ValidationResult] = []
    all_passed = True
    canonical_results = []
    total_start = time.monotonic()

    if args.command == "validate":
        result = validate_archive(args.archive, strict=getattr(args, 'strict', False))
        all_results.append(result)
        print_results(result)
        if not result.passed:
            all_passed = False

    elif args.command == "validate-all":
        archive_dir = Path(args.dir)
        archives = sorted(archive_dir.glob("*.tar.gz"))
        if not archives:
            print(f"\n  No .tar.gz archives found in {args.dir}")
            sys.exit(1)
        print(f"Archives: {len(archives)}")
        for archive_path in archives:
            result = validate_archive(str(archive_path), strict=getattr(args, 'strict', False))
            all_results.append(result)
            print_results(result)
            if not result.passed:
                all_passed = False

    elif args.command == "self-check":
        all_results = self_check()
        for result in all_results:
            print_results(result)
            if not result.passed:
                all_passed = False

    # Build canonical report
    total_duration = (time.monotonic() - total_start) * 1000
    for result in all_results:
        failures = [f"{c.name}: {c.message}" for c in result.checks if not c.passed]
        canonical_results.append(_canonical_result(
            spec_path=result.target_path,
            status="pass" if result.passed else "fail",
            assertions_evaluated=result.total_checks,
            assertions_passed=result.passed_checks,
            failures=failures,
            hash_verified=None,
        ))

    report = _canonical_report("utds", UTDS_VERSION, canonical_results, duration_ms=round(total_duration, 2))
    _print_summary(report)

    report_path = getattr(args, 'report', None)
    if report_path:
        _save_report(report, report_path)

    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    main()
