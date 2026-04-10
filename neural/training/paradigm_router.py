#!/usr/bin/env python3
"""
Paradigm Router — spec-driven training data curation pipeline.

Reads a UAITS spec and dispatches each declared dataset through its
paradigm-specific pipeline (SFT, DPO, RAFT, or curriculum).

Usage:
    python -m training.paradigm_router \
        --spec docs/tests/uaits/specs/mdemg.uaits.json \
        --input-dir /tmp/exported/ \
        --output-dir /tmp/curated/ \
        [--version v1] \
        [--dry-run]
"""

import argparse
import json
import os
import sys
import time
from typing import Any


def load_uaits_spec(spec_path: str) -> dict[str, Any]:
    """Load and basic-validate a UAITS spec."""
    with open(spec_path) as f:
        spec = json.load(f)

    required = {"uaits_version", "application", "datasets"}
    missing = required - set(spec.keys())
    if missing:
        raise ValueError(f"Invalid UAITS spec: missing {missing}")

    datasets = spec.get("datasets", [])
    if not datasets:
        raise ValueError("UAITS spec has no datasets declared")

    return spec


def route_dataset(
    dataset_config: dict[str, Any],
    input_dir: str,
    output_dir: str,
    spec_path: str | None = None,
    version: str = "v1",
    dry_run: bool = False,
) -> dict[str, Any]:
    """Route a single dataset through its paradigm pipeline.

    Returns a per-dataset report dict.
    """
    name = dataset_config.get("name", "unnamed")
    paradigm = dataset_config.get("paradigm", "")
    ds_output = os.path.join(output_dir, name)

    report: dict[str, Any] = {
        "dataset": name,
        "paradigm": paradigm,
        "status": "pending",
    }

    if paradigm == "sft":
        report = _route_sft(dataset_config, input_dir, ds_output, spec_path, version, dry_run)
    elif paradigm == "dpo":
        report = _route_dpo(dataset_config, input_dir, ds_output, spec_path, version, dry_run)
    elif paradigm == "raft":
        report = _route_raft(dataset_config, input_dir, ds_output, spec_path, version, dry_run)
    elif paradigm == "curriculum":
        report = _route_curriculum(dataset_config, input_dir, ds_output, version, dry_run)
    else:
        raise ValueError(f"Unknown paradigm: {paradigm!r}")

    report["dataset"] = name
    report["paradigm"] = paradigm
    return report


def _route_sft(
    ds: dict[str, Any],
    input_dir: str,
    output_dir: str,
    spec_path: str | None,
    version: str,
    dry_run: bool,
) -> dict[str, Any]:
    """SFT: quality_filter -> format_converter (chat) -> dataset_versioner."""
    from training.format_converter import run_converter
    from training.quality_filter import run_filter

    gates = ds.get("quality_gates", {})
    dedup = gates.get("dedup_mode", "prompt")
    interactions_file = os.path.join(input_dir, "llm_interactions.jsonl")

    if not os.path.exists(interactions_file):
        return {"status": "skipped", "reason": f"missing {interactions_file}"}

    if dry_run:
        return {"status": "dry_run"}

    os.makedirs(output_dir, exist_ok=True)
    filtered_path = os.path.join(output_dir, "filtered.jsonl")
    converted_path = os.path.join(output_dir, "converted.jsonl")

    # Step 1: Quality filter
    filter_report = run_filter(
        interactions_file, filtered_path,
        dedup_key=dedup,
        uaits_spec_path=spec_path,
    )

    # Step 2: Format conversion
    raft_ratio = ds.get("format", {}).get("raft_ratio", 0.8)
    convert_report = run_converter(filtered_path, converted_path, raft_ratio=raft_ratio)

    # Step 3: Dataset versioning
    versioning = ds.get("versioning", {})
    versioner_report = _run_versioner_step(
        [output_dir], output_dir, version, versioning, dedup, "sft",
        converted_path,
    )

    return {
        "status": "completed",
        "filter": filter_report,
        "convert": convert_report,
        "versioner": versioner_report,
    }


def _route_dpo(
    ds: dict[str, Any],
    input_dir: str,
    output_dir: str,
    spec_path: str | None,
    version: str,
    dry_run: bool,
) -> dict[str, Any]:
    """DPO: dpo_builder -> quality_filter -> format_converter (dpo) -> versioner."""
    from training.dpo_builder import run_dpo_builder
    from training.format_converter import run_dpo_converter
    from training.quality_filter import run_filter

    interactions_file = os.path.join(input_dir, "llm_interactions.jsonl")
    outcomes_file = os.path.join(input_dir, "constraint_outcomes.jsonl")

    if not os.path.exists(outcomes_file):
        return {"status": "skipped", "reason": f"missing {outcomes_file}"}
    if not os.path.exists(interactions_file):
        return {"status": "skipped", "reason": f"missing {interactions_file}"}

    if dry_run:
        return {"status": "dry_run"}

    os.makedirs(output_dir, exist_ok=True)
    pairs_path = os.path.join(output_dir, "pairs.jsonl")
    filtered_path = os.path.join(output_dir, "filtered.jsonl")
    converted_path = os.path.join(output_dir, "converted.jsonl")

    # Step 1: Build DPO pairs
    dpo_report = run_dpo_builder(
        interactions_file, outcomes_file, pairs_path,
        spec_path=spec_path,
    )

    if dpo_report.get("pairs_built", 0) == 0:
        return {"status": "skipped", "reason": "no DPO pairs built", "dpo": dpo_report}

    # Step 2: Quality filter (dedup on prompt-response)
    filter_report = run_filter(
        pairs_path, filtered_path,
        dedup_key="prompt-response",
        uaits_spec_path=spec_path,
    )

    # Step 3: Format conversion (DPO)
    convert_report = run_dpo_converter(filtered_path, converted_path)

    # Step 4: Versioning
    versioning = ds.get("versioning", {})
    versioner_report = _run_versioner_step(
        [output_dir], output_dir, version, versioning, "prompt-response", "dpo",
        converted_path,
    )

    return {
        "status": "completed",
        "dpo": dpo_report,
        "filter": filter_report,
        "convert": convert_report,
        "versioner": versioner_report,
    }


def _route_raft(
    ds: dict[str, Any],
    input_dir: str,
    output_dir: str,
    spec_path: str | None,
    version: str,
    dry_run: bool,
) -> dict[str, Any]:
    """RAFT: quality_filter -> format_converter (chat+raft) -> versioner."""
    from training.format_converter import run_converter
    from training.quality_filter import run_filter

    interactions_file = os.path.join(input_dir, "llm_interactions.jsonl")

    if not os.path.exists(interactions_file):
        return {"status": "skipped", "reason": f"missing {interactions_file}"}

    if dry_run:
        return {"status": "dry_run"}

    os.makedirs(output_dir, exist_ok=True)
    filtered_path = os.path.join(output_dir, "filtered.jsonl")
    converted_path = os.path.join(output_dir, "converted.jsonl")

    # Step 1: Quality filter
    gates = ds.get("quality_gates", {})
    dedup = gates.get("dedup_mode", "prompt")
    filter_report = run_filter(
        interactions_file, filtered_path,
        dedup_key=dedup,
        uaits_spec_path=spec_path,
    )

    # Step 2: Format conversion with RAFT ratio
    raft_ratio = ds.get("format", {}).get("raft_ratio", 0.8)
    convert_report = run_converter(filtered_path, converted_path, raft_ratio=raft_ratio)

    # Step 3: Versioning
    versioning = ds.get("versioning", {})
    versioner_report = _run_versioner_step(
        [output_dir], output_dir, version, versioning, dedup, "raft",
        converted_path,
    )

    return {
        "status": "completed",
        "filter": filter_report,
        "convert": convert_report,
        "versioner": versioner_report,
    }


def _route_curriculum(
    ds: dict[str, Any],
    input_dir: str,
    output_dir: str,
    version: str,
    dry_run: bool,
) -> dict[str, Any]:
    """Curriculum: passthrough -> versioner (metadata only)."""
    source_table = ds.get("source", {}).get("primary_table", "metric_samples")
    metrics_file = os.path.join(input_dir, f"{source_table}.jsonl")

    if not os.path.exists(metrics_file):
        return {"status": "skipped", "reason": f"missing {metrics_file}"}

    if dry_run:
        return {"status": "dry_run"}

    os.makedirs(output_dir, exist_ok=True)

    # Count records
    with open(metrics_file) as f:
        count = sum(1 for line in f if line.strip())

    return {
        "status": "completed",
        "records": count,
        "note": "curriculum metadata — used for quality weighting, not direct training",
    }


def _run_versioner_step(
    input_dirs: list[str],
    output_dir: str,
    version: str,
    versioning: dict[str, Any],
    dedup_key: str,
    paradigm: str,
    converted_path: str,
) -> dict[str, Any]:
    """Run dataset_versioner with spec-driven parameters."""
    from training.dataset_versioner import run_versioner

    return run_versioner(
        input_dirs=[os.path.dirname(converted_path)],
        output_dir=os.path.join(output_dir, "versioned"),
        version=version,
        train_ratio=versioning.get("train_ratio", 0.8),
        test_ratio=versioning.get("test_ratio", 0.1),
        val_ratio=versioning.get("val_ratio", 0.1),
        min_per_task=versioning.get("min_per_task", 10),
        exogenous_ratio=versioning.get("exogenous_ratio_min", 0.4),
        dedup_key=dedup_key,
        first_cycle=True,
        paradigm=paradigm,
    )


def run_curation(
    spec_path: str,
    input_dir: str,
    output_dir: str,
    version: str = "v1",
    dry_run: bool = False,
) -> dict[str, Any]:
    """Run spec-driven curation for all datasets in a UAITS spec."""
    spec = load_uaits_spec(spec_path)
    app_name = spec.get("application", {}).get("name", "unknown")

    results = {
        "application": app_name,
        "spec": spec_path,
        "datasets": [],
        "start_time": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }

    for ds in spec.get("datasets", []):
        ds_report = route_dataset(
            ds, input_dir, output_dir,
            spec_path=spec_path, version=version, dry_run=dry_run,
        )
        results["datasets"].append(ds_report)

    completed = sum(1 for d in results["datasets"] if d.get("status") == "completed")
    skipped = sum(1 for d in results["datasets"] if d.get("status") == "skipped")
    results["summary"] = {
        "total": len(results["datasets"]),
        "completed": completed,
        "skipped": skipped,
        "dry_run": dry_run,
    }

    return results


def main() -> None:
    parser = argparse.ArgumentParser(
        description="UAITS Paradigm Router — spec-driven training data curation",
    )
    parser.add_argument(
        "--spec", required=True,
        help="UAITS spec file path",
    )
    parser.add_argument(
        "--input-dir", required=True,
        help="Directory with exported JSONL data",
    )
    parser.add_argument(
        "--output-dir", required=True,
        help="Output directory for curated datasets",
    )
    parser.add_argument(
        "--version", default="v1",
        help="Dataset version label (default: v1)",
    )
    parser.add_argument(
        "--dry-run", action="store_true",
        help="Show what would be curated without writing",
    )
    args = parser.parse_args()

    results = run_curation(
        args.spec, args.input_dir, args.output_dir,
        version=args.version, dry_run=args.dry_run,
    )

    print(json.dumps(results, indent=2))
    summary = results.get("summary", {})
    if summary.get("completed", 0) == 0 and not summary.get("dry_run"):
        print("\nNo datasets curated.", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
