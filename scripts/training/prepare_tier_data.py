#!/usr/bin/env python3
"""Prepare J17 protocol training data for the neural sidecar tier model.

Reads JSONL files from the protocol data directory, filters to records with
outcomes, computes optimal tiers, and outputs train/val/test splits.

Usage:
    python scripts/training/prepare_tier_data.py \
        --input .mdemg/neural/protocol-data/ \
        --output /tmp/tier_training/
"""

import argparse
import json
import os
import random
import sys
from collections import Counter
from pathlib import Path


def load_records(input_dir: str) -> list[dict]:
    """Load all JSONL records from the input directory."""
    records = []
    input_path = Path(input_dir)

    if not input_path.exists():
        print(f"Error: input directory does not exist: {input_dir}", file=sys.stderr)
        sys.exit(1)

    for jsonl_file in sorted(input_path.glob("*.jsonl")):
        with open(jsonl_file) as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line:
                    continue
                try:
                    record = json.loads(line)
                    record["_source_file"] = jsonl_file.name
                    record["_source_line"] = line_num
                    records.append(record)
                except json.JSONDecodeError as e:
                    print(
                        f"Warning: skipping malformed JSON in {jsonl_file.name}:{line_num}: {e}",
                        file=sys.stderr,
                    )

    return records


def filter_with_outcomes(records: list[dict]) -> list[dict]:
    """Filter to records that have outcome data (comprehension scores)."""
    return [
        r
        for r in records
        if r.get("comprehension_score") is not None
        and r.get("comprehension_score", 0) > 0
    ]


def compute_optimal_tier(record: dict) -> int | None:
    """Determine the optimal tier for a record based on comprehension.

    Returns the tier that achieved the highest comprehension score.
    If only one tier was used, returns that tier if comprehension >= 0.7.
    """
    tier = record.get("tier")
    comp = record.get("comprehension_score", 0)

    if tier is None:
        return None

    # If comprehension is high, the tier used was good
    if comp >= 0.7:
        return tier

    # Low comprehension with T1 -> should have used T2 or T3
    if tier == 1 and comp < 0.7:
        return 2 if comp >= 0.4 else 3

    # Low comprehension with T2 -> should have used T3
    if tier == 2 and comp < 0.7:
        return 3

    return tier


def prepare_training_record(record: dict) -> dict | None:
    """Convert a raw protocol record to a training example."""
    optimal = compute_optimal_tier(record)
    if optimal is None:
        return None

    return {
        "constraint_code": record.get("constraint_code", ""),
        "constraint_text": record.get("constraint_text", ""),
        "trust_score": record.get("trust_score", 0.5),
        "tier_used": record.get("tier", 0),
        "comprehension_score": record.get("comprehension_score", 0),
        "optimal_tier": optimal,
        "ml_tier": record.get("ml_tier", 0),
        "rule_tier": record.get("rule_tier", 0),
        "ml_confidence": record.get("ml_confidence", 0),
        "tier_source": record.get("tier_source", ""),
        "sidecar_mode": record.get("sidecar_mode", ""),
    }


def split_data(
    records: list[dict], train_ratio: float = 0.8, val_ratio: float = 0.1, seed: int = 42
) -> tuple[list[dict], list[dict], list[dict]]:
    """Split records into train/val/test sets."""
    random.seed(seed)
    shuffled = list(records)
    random.shuffle(shuffled)

    n = len(shuffled)
    train_end = int(n * train_ratio)
    val_end = int(n * (train_ratio + val_ratio))

    return shuffled[:train_end], shuffled[train_end:val_end], shuffled[val_end:]


def write_jsonl(records: list[dict], path: Path) -> None:
    """Write records to a JSONL file."""
    with open(path, "w") as f:
        for record in records:
            f.write(json.dumps(record) + "\n")


def print_statistics(records: list[dict], label: str) -> None:
    """Print statistics about a set of records."""
    if not records:
        print(f"\n{label}: 0 records")
        return

    print(f"\n{label}: {len(records)} records")

    # Tier distribution
    tier_counts = Counter(r.get("tier_used", 0) for r in records)
    print(f"  Tier distribution: T1={tier_counts.get(1, 0)}, T2={tier_counts.get(2, 0)}, T3={tier_counts.get(3, 0)}")

    # Optimal tier distribution
    optimal_counts = Counter(r.get("optimal_tier", 0) for r in records)
    print(
        f"  Optimal tier dist: T1={optimal_counts.get(1, 0)}, T2={optimal_counts.get(2, 0)}, T3={optimal_counts.get(3, 0)}"
    )

    # Comprehension stats
    comp_scores = [r.get("comprehension_score", 0) for r in records]
    avg_comp = sum(comp_scores) / len(comp_scores) if comp_scores else 0
    print(f"  Avg comprehension: {avg_comp:.3f}")

    # Agreement (tier_used == optimal_tier)
    agreement = sum(1 for r in records if r.get("tier_used") == r.get("optimal_tier"))
    print(f"  Tier agreement: {agreement}/{len(records)} ({100 * agreement / len(records):.1f}%)")

    # Source distribution
    source_counts = Counter(r.get("tier_source", "unknown") for r in records)
    sources_str = ", ".join(f"{k}={v}" for k, v in sorted(source_counts.items()))
    print(f"  Tier sources: {sources_str}")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Prepare J17 protocol training data for tier prediction model"
    )
    parser.add_argument(
        "--input",
        required=True,
        help="Directory containing protocol JSONL files",
    )
    parser.add_argument(
        "--output",
        required=True,
        help="Output directory for train/val/test splits",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=42,
        help="Random seed for reproducible splits (default: 42)",
    )
    parser.add_argument(
        "--min-records",
        type=int,
        default=100,
        help="Minimum records required to proceed (default: 100)",
    )
    args = parser.parse_args()

    # Load raw records
    print(f"Loading records from {args.input}...")
    raw_records = load_records(args.input)
    print(f"Loaded {len(raw_records)} raw records")

    # Filter to records with outcomes
    outcome_records = filter_with_outcomes(raw_records)
    print(f"Records with outcomes: {len(outcome_records)}")

    if len(outcome_records) < args.min_records:
        print(
            f"Error: only {len(outcome_records)} records with outcomes "
            f"(minimum: {args.min_records}). Collect more data in shadow/compare mode.",
            file=sys.stderr,
        )
        sys.exit(1)

    # Prepare training records
    training_records = []
    for record in outcome_records:
        prepared = prepare_training_record(record)
        if prepared is not None:
            training_records.append(prepared)

    print(f"Training-ready records: {len(training_records)}")

    # Print overall statistics
    print_statistics(training_records, "Overall")

    # Split
    train, val, test = split_data(training_records, seed=args.seed)
    print_statistics(train, "Train")
    print_statistics(val, "Validation")
    print_statistics(test, "Test")

    # Write output
    output_path = Path(args.output)
    output_path.mkdir(parents=True, exist_ok=True)

    write_jsonl(train, output_path / "train.jsonl")
    write_jsonl(val, output_path / "val.jsonl")
    write_jsonl(test, output_path / "test.jsonl")

    # Write metadata
    metadata = {
        "input_dir": args.input,
        "seed": args.seed,
        "raw_records": len(raw_records),
        "outcome_records": len(outcome_records),
        "training_records": len(training_records),
        "train_count": len(train),
        "val_count": len(val),
        "test_count": len(test),
    }
    with open(output_path / "metadata.json", "w") as f:
        json.dump(metadata, f, indent=2)

    print(f"\nOutput written to {args.output}/")
    print(f"  train.jsonl: {len(train)} records")
    print(f"  val.jsonl:   {len(val)} records")
    print(f"  test.jsonl:  {len(test)} records")
    print(f"  metadata.json")


if __name__ == "__main__":
    main()
