#!/usr/bin/env python3
"""
quality_filter.py — Training Data Quality Gate Engine

Applies quality gates to exported LLM interaction JSONL data, filtering
out records that would degrade LoRA fine-tuning quality.

Quality gates (from 05_DATA_COLLECTION_v2.md §5.2):
  1. Non-empty response (len > 10)
  2. Non-error (error field empty/null)
  3. Non-duplicate (SHA-256 of system_prompt + user_prompt)
  4. Latency reasonable (< 60000ms)
  5. Model version known (non-empty model_name)
  6. System prompt current (hash matches known ULTS spec hashes)
  7. ULTS output schema valid (if spec defines output_schema)
  P. Privacy hard gate (5 regex patterns — HARD REJECT)

Usage:
    python -m training.quality_filter \\
        --input /tmp/export-raw/llm_interactions.jsonl \\
        --output /tmp/filtered/llm_interactions.jsonl \\
        --ults-dir ../docs/tests/ults/specs/ \\
        --report /tmp/filter_report.json \\
        [--dry-run]
"""

import argparse
import hashlib
import json
import os
import re
import sys
from pathlib import Path
from typing import Any

# Privacy scrub patterns (mirrors internal/llmclient/scrubber.go)
PRIVACY_PATTERNS = {
    "api_key": re.compile(
        r"(?i)(sk-[a-zA-Z0-9]{20,}|ghp_[a-zA-Z0-9]{36}|AKIA[A-Z0-9]{16}|Bearer\s+[a-zA-Z0-9._\-]{20,})"
    ),
    "abs_path": re.compile(r"(?:/Users/[^\s\"']+|/home/[^\s\"']+|[A-Z]:\\\\Users\\\\[^\s\"']+)"),
    "env_secret": re.compile(
        r"(?i)(PASSWORD|SECRET|TOKEN|API_KEY|PRIVATE_KEY)\s*[=:]\s*[\"']?([^\s\"',;]+)[\"']?"
    ),
    "email": re.compile(r"[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}"),
    "neo4j_cred": re.compile(r"neo4j://[^:]+:[^@]+@"),
}

TEXT_FIELDS = ["system_prompt", "user_prompt", "response", "think_content", "source_path"]

MAX_LATENCY_MS = 60_000
MIN_RESPONSE_LENGTH = 10


def load_ults_specs(ults_dir: str | None) -> dict[str, dict[str, Any]]:
    """Load ULTS specs to get valid system_prompt_hashes and output_schemas.

    Returns dict keyed by task_name with keys:
        - system_prompt_hash: str, list[str], or None
        - output_schema: dict or None
    """
    specs: dict[str, dict[str, Any]] = {}
    if not ults_dir or not os.path.isdir(ults_dir):
        return specs

    for f in Path(ults_dir).glob("*.ults.json"):
        try:
            with open(f) as fh:
                spec = json.load(fh)
            task_name = spec.get("task", {}).get("name", "")
            if not task_name:
                continue
            specs[task_name] = {
                "system_prompt_hash": spec.get("prompt", {}).get("system_prompt_hash"),
                "output_schema": spec.get("output_schema"),
            }
        except (json.JSONDecodeError, OSError):
            continue

    return specs


def check_privacy(record: dict[str, Any]) -> bool:
    """Return True if ANY text field contains PII. Hard reject."""
    for field in TEXT_FIELDS:
        val = record.get(field)
        if not val or not isinstance(val, str):
            continue
        for pattern in PRIVACY_PATTERNS.values():
            if pattern.search(val):
                return True
    return False


def validate_output_schema(response: str, schema: dict[str, Any]) -> bool:
    """Basic JSON structure validation against an output_schema.

    Checks: response is valid JSON and has all required top-level keys.
    String-type schemas accept any non-empty response without JSON parsing.
    Does NOT use jsonschema library — keeps this zero-dependency.
    """
    schema_type = schema.get("type", "")

    # String-type schemas: response is plain text, not JSON
    if schema_type == "string":
        return bool(response and response.strip())

    try:
        parsed = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return False

    required = schema.get("required", [])
    if isinstance(parsed, dict):
        return all(k in parsed for k in required)

    return len(required) == 0


DEDUP_MODES = {"prompt", "prompt-response", "trace"}


def run_filter(
    input_path: str,
    output_path: str | None,
    ults_dir: str | None = None,
    dry_run: bool = False,
    dedup_key: str = "prompt",
) -> dict[str, Any]:
    """Run the quality filter pipeline.

    dedup_key modes:
        prompt: SHA-256(system_prompt + user_prompt) — keeps earliest response (SFT)
        prompt-response: SHA-256(system_prompt + user_prompt + response) — keeps diverse responses (DPO)
        trace: SHA-256(trace_id) — no dedup (debugging only)

    Returns a filter report dict.
    """
    specs = load_ults_specs(ults_dir)
    known_hashes: set[str] = set()
    dynamic_prompt_tasks: set[str] = set()
    for task, s in specs.items():
        hash_val = s.get("system_prompt_hash")
        if hash_val == "dynamic":
            dynamic_prompt_tasks.add(task)
        elif isinstance(hash_val, list):
            for h in hash_val:
                if h and h != "dynamic":
                    known_hashes.add(h)
        elif hash_val:
            known_hashes.add(hash_val)

    # Track exclusion reasons
    excluded = {
        "empty_response": 0,
        "error_present": 0,
        "duplicate_prompt": 0,
        "latency_exceeded": 0,
        "unknown_model": 0,
        "stale_prompt_hash": 0,
        "ults_invalid": 0,
        "privacy_violation": 0,
    }
    task_distribution: dict[str, int] = {}
    seen_hashes: set[str] = set()
    input_rows = 0
    output_rows = 0
    output_records: list[dict[str, Any]] = []

    with open(input_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            input_rows += 1

            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                excluded["empty_response"] += 1
                continue

            # Gate P: Privacy hard gate (check first — most critical)
            if check_privacy(record):
                excluded["privacy_violation"] += 1
                continue

            # Gate 1: Non-empty response
            response = record.get("response", "") or ""
            if len(response.strip()) <= MIN_RESPONSE_LENGTH:
                excluded["empty_response"] += 1
                continue

            # Gate 2: Non-error
            error = record.get("error", "") or ""
            if error.strip():
                excluded["error_present"] += 1
                continue

            # Gate 3: Non-duplicate (dedup key determines hash input)
            sp = record.get("system_prompt", "") or ""
            up = record.get("user_prompt", "") or ""
            if dedup_key == "prompt-response":
                dup_input = sp + up + response
            elif dedup_key == "trace":
                dup_input = record.get("trace_id", "") or ""
            else:
                dup_input = sp + up
            dup_hash = hashlib.sha256(dup_input.encode()).hexdigest()
            if dup_hash in seen_hashes:
                excluded["duplicate_prompt"] += 1
                continue
            seen_hashes.add(dup_hash)

            # Gate 4: Latency reasonable
            latency = record.get("latency_ms", 0) or 0
            if isinstance(latency, (int, float)) and latency > MAX_LATENCY_MS:
                excluded["latency_exceeded"] += 1
                continue

            # Gate 5: Model version known
            model = record.get("model_name", "") or ""
            if not model.strip():
                excluded["unknown_model"] += 1
                continue

            # Gate 6: System prompt current (if we have ULTS specs)
            # Skip for tasks with dynamic prompts (hash varies per call)
            task_name_for_hash = record.get("task_name", "") or ""
            if known_hashes and task_name_for_hash not in dynamic_prompt_tasks:
                sph = record.get("system_prompt_hash", "") or ""
                if sph and sph not in known_hashes:
                    excluded["stale_prompt_hash"] += 1
                    continue

            # Gate 7: ULTS output schema validation
            task_name = record.get("task_name", "") or ""
            if task_name in specs and specs[task_name].get("output_schema"):
                if not validate_output_schema(response, specs[task_name]["output_schema"]):
                    excluded["ults_invalid"] += 1
                    continue

            # Record passed all gates
            output_rows += 1
            task_distribution[task_name] = task_distribution.get(task_name, 0) + 1
            if not dry_run:
                output_records.append(record)

    # Write output
    if not dry_run and output_path:
        os.makedirs(os.path.dirname(output_path) or ".", exist_ok=True)
        with open(output_path, "w") as f:
            for rec in output_records:
                f.write(json.dumps(rec) + "\n")

    report = {
        "input_file": os.path.basename(input_path),
        "input_rows": input_rows,
        "output_rows": output_rows,
        "excluded": excluded,
        "task_distribution": task_distribution,
        "ults_specs_loaded": len(specs),
        "known_prompt_hashes": len(known_hashes),
    }

    return report


def main():
    parser = argparse.ArgumentParser(description="Training Data Quality Filter")
    parser.add_argument("--input", required=True, help="Input JSONL file path")
    parser.add_argument("--output", help="Output filtered JSONL file path")
    parser.add_argument("--ults-dir", help="Directory containing ULTS spec files")
    parser.add_argument("--report", help="Output filter report JSON path")
    parser.add_argument("--dry-run", action="store_true", help="Count only, do not write output")
    parser.add_argument(
        "--dedup-key",
        choices=sorted(DEDUP_MODES),
        default="prompt",
        help="Dedup hash mode: prompt (SFT), prompt-response (DPO), trace (debug). Default: prompt",
    )
    args = parser.parse_args()

    if not args.output and not args.dry_run:
        parser.error("--output is required unless --dry-run is set")

    report = run_filter(
        input_path=args.input,
        output_path=args.output,
        ults_dir=args.ults_dir,
        dry_run=args.dry_run,
        dedup_key=args.dedup_key,
    )

    print(f"Input:  {report['input_rows']} rows")
    print(f"Output: {report['output_rows']} rows")
    print(f"Excluded:")
    for reason, count in report["excluded"].items():
        if count > 0:
            print(f"  {reason}: {count}")
    print(f"Task distribution:")
    for task, count in sorted(report["task_distribution"].items()):
        print(f"  {task}: {count}")

    if args.report:
        os.makedirs(os.path.dirname(args.report) or ".", exist_ok=True)
        with open(args.report, "w") as f:
            json.dump(report, f, indent=2)
        print(f"\nReport saved: {args.report}")

    if report["excluded"]["privacy_violation"] > 0:
        print("\nWARNING: Privacy violations detected — review data source!")
        sys.exit(2)


if __name__ == "__main__":
    main()
