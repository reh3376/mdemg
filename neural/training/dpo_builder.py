#!/usr/bin/env python3
"""
DPO Pair Builder — construct preference pairs from constraint outcomes.

Joins constraint_outcomes with llm_interactions on guidance_id to produce
(prompt, chosen, rejected) triples for Direct Preference Optimization training.

Usage:
    python -m training.dpo_builder \
        --interactions exported/llm_interactions.jsonl \
        --outcomes exported/constraint_outcomes.jsonl \
        --output curated/dpo_preferences.jsonl \
        [--spec path/to/mdemg.uaits.json] \
        [--dry-run]
"""

import argparse
import json
import sys
from pathlib import Path
from typing import Any

from training.quality_filter import check_privacy

# Default outcome type mapping
DEFAULT_CHOSEN = {"followed"}
DEFAULT_REJECTED = {"contradicted"}


def load_constraint_outcomes(path: str) -> list[dict[str, Any]]:
    """Load constraint outcome records from JSONL."""
    records = []
    p = Path(path)
    if not p.exists():
        return records
    with open(p) as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))
    return records


def build_interactions_index(
    path: str,
) -> dict[str, list[dict[str, Any]]]:
    """Build guidance_id -> [interaction_record] index from JSONL."""
    index: dict[str, list[dict[str, Any]]] = {}
    p = Path(path)
    if not p.exists():
        return index
    with open(p) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            gid = record.get("guidance_id")
            if gid:
                index.setdefault(gid, []).append(record)
    return index


def _format_prompt(record: dict[str, Any]) -> str:
    """Format a prompt from system_prompt and user_prompt fields."""
    sys_prompt = record.get("system_prompt", "")
    user_prompt = record.get("user_prompt", "")
    if sys_prompt:
        return f"{sys_prompt}\n\n{user_prompt}"
    return user_prompt


def build_preference_pairs(
    outcomes: list[dict[str, Any]],
    interactions_index: dict[str, list[dict[str, Any]]],
    spec: dict[str, Any] | None = None,
) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Build DPO preference pairs from outcomes and interactions.

    Returns (pairs, report) where report has stats about the build.
    """
    chosen_types = DEFAULT_CHOSEN
    rejected_types = DEFAULT_REJECTED
    min_pair_diversity = 10

    # Spec-driven overrides
    if spec:
        ds = _find_dpo_dataset(spec)
        if ds:
            filt = ds.get("source", {}).get("filter", {})
            types = filt.get("outcome_types", [])
            if types:
                chosen_types = {t for t in types if t in DEFAULT_CHOSEN}
                rejected_types = {t for t in types if t in DEFAULT_REJECTED}

    # Group outcomes by guidance_id
    outcome_groups: dict[str, list[dict[str, Any]]] = {}
    for o in outcomes:
        gid = o.get("guidance_id")
        if gid:
            outcome_groups.setdefault(gid, []).append(o)

    pairs = []
    skipped_no_interaction = 0
    skipped_no_pair = 0
    skipped_privacy = 0
    skipped_diversity = 0

    for gid, group in outcome_groups.items():
        interactions = interactions_index.get(gid, [])
        if not interactions:
            skipped_no_interaction += len(group)
            continue

        # Find chosen and rejected outcomes for this guidance_id
        chosen_outcomes = [o for o in group if o.get("outcome_type") in chosen_types]
        rejected_outcomes = [o for o in group if o.get("outcome_type") in rejected_types]

        if not chosen_outcomes or not rejected_outcomes:
            skipped_no_pair += 1
            continue

        # Use first interaction as the prompt source
        interaction = interactions[0]
        prompt = _format_prompt(interaction)

        # Privacy check on the interaction
        if check_privacy(interaction):
            skipped_privacy += 1
            continue

        chosen_response = interaction.get("response", "")
        # For the rejected response, find a different interaction if available
        rejected_response = chosen_response  # fallback
        if len(interactions) > 1:
            rejected_response = interactions[1].get("response", chosen_response)
        else:
            # Use the same response text — the outcome signal is the differentiator
            # In real DPO, we want different actual responses
            # If only one interaction exists for this guidance_id, use it for both
            # but the outcome metadata differentiates the pair
            pass

        # Actually, the correct approach: chosen = response from the interaction
        # that led to "followed", rejected = response from interaction that led to
        # "contradicted". But outcomes reference the same guidance_id which may
        # map to a single interaction. In that case, we skip — we need true pairs.
        # For this first version: use the interaction response as the chosen,
        # and check if there's a second interaction with a different response.
        if len(interactions) > 1:
            chosen_response = interactions[0].get("response", "")
            rejected_response = interactions[1].get("response", "")
        else:
            # Single interaction with both followed and contradicted outcomes
            # This is a degenerate case — skip it
            skipped_no_pair += 1
            continue

        # Diversity gate
        if abs(len(chosen_response) - len(rejected_response)) < min_pair_diversity:
            if chosen_response == rejected_response:
                skipped_diversity += 1
                continue

        pair = {
            "prompt": prompt,
            "chosen": chosen_response,
            "rejected": rejected_response,
            "metadata": {
                "guidance_id": gid,
                "constraint_id": chosen_outcomes[0].get("constraint_id", ""),
                "constraint_code": chosen_outcomes[0].get("constraint_code", ""),
                "chosen_similarity": chosen_outcomes[0].get("similarity", 0.0),
                "rejected_similarity": rejected_outcomes[0].get("similarity", 0.0),
                "task_name": interaction.get("task_name", ""),
                "time": interaction.get("time", ""),
            },
        }
        pairs.append(pair)

    report = {
        "total_outcomes": len(outcomes),
        "unique_guidance_ids": len(outcome_groups),
        "pairs_built": len(pairs),
        "skipped_no_interaction": skipped_no_interaction,
        "skipped_no_pair": skipped_no_pair,
        "skipped_privacy": skipped_privacy,
        "skipped_diversity": skipped_diversity,
    }

    return pairs, report


def _find_dpo_dataset(spec: dict[str, Any]) -> dict[str, Any] | None:
    """Find the DPO dataset declaration in a UAITS spec."""
    for ds in spec.get("datasets", []):
        if ds.get("paradigm") == "dpo":
            return ds
    return None


def run_dpo_builder(
    interactions_path: str,
    outcomes_path: str,
    output_path: str,
    spec_path: str | None = None,
    dry_run: bool = False,
) -> dict[str, Any]:
    """Build DPO pairs and write to JSONL.

    Returns a report dict with build statistics.
    """
    spec = None
    if spec_path:
        with open(spec_path) as f:
            spec = json.load(f)

    outcomes = load_constraint_outcomes(outcomes_path)
    interactions_index = build_interactions_index(interactions_path)
    pairs, report = build_preference_pairs(outcomes, interactions_index, spec)

    if not dry_run and pairs:
        out = Path(output_path)
        out.parent.mkdir(parents=True, exist_ok=True)
        with open(out, "w") as f:
            for pair in pairs:
                f.write(json.dumps(pair) + "\n")
        report["output_file"] = str(out)

    return report


def main() -> None:
    parser = argparse.ArgumentParser(
        description="DPO Preference Pair Builder",
    )
    parser.add_argument(
        "--interactions", required=True,
        help="Path to llm_interactions JSONL",
    )
    parser.add_argument(
        "--outcomes", required=True,
        help="Path to constraint_outcomes JSONL",
    )
    parser.add_argument(
        "--output", required=True,
        help="Output JSONL path for DPO pairs",
    )
    parser.add_argument("--spec", help="UAITS spec for filtering")
    parser.add_argument(
        "--dry-run", action="store_true",
        help="Report only, no output file",
    )
    args = parser.parse_args()

    report = run_dpo_builder(
        args.interactions, args.outcomes, args.output,
        spec_path=args.spec, dry_run=args.dry_run,
    )

    print(json.dumps(report, indent=2))
    if report["pairs_built"] == 0:
        print("\nNo pairs built.", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
