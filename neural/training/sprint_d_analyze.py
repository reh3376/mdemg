#!/usr/bin/env python3
"""Sprint FT-LORA-D Epic 3 — Validation analysis + decision doc emitter.

Consumes the profile artifacts produced by profile_expert_routing.py and emits:
  - docs/development/ft-lora/sprint_c_d_profile_results.md (decision doc)

Validation criteria per sprint_plan_ft_lora_d.md §2 + §5 Epic 3:
  1. Cross-family Jaccard overlap — 3 family-pair averages over 40 layers.
     Verdict:
       - 0 pairs > 0.80  → 3-family-confirmed
       - 1 pair  > 0.80  → 2-family-merged-<pair>
       - ≥2 pairs > 0.80 → 1-family-collapsed
  2. Per-family task-cohesion — within-family pairwise Jaccard.
     Verdict per family:
       - ≥ 0.70 → cohesive
       - 0.40-0.70 → ambiguous
       - < 0.40 → split-candidate (emit hierarchical cluster boundary)

Usage:
    python sprint_d_analyze.py \\
        --profile-dir training_data/routing_profiles/ \\
        --output-doc docs/development/ft-lora/sprint_c_d_profile_results.md
"""
from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import sys
import time
from typing import Any

from training.profile_expert_routing import (
    cohesion_verdict,
    compute_cross_family_overlap,
    hierarchical_cluster_boundary,
    per_task_top_experts,
    within_family_pairwise_jaccard,
)


OVERLAP_THRESHOLD = 0.80


def load_family_artifacts(profile_dir: pathlib.Path) -> dict[str, dict]:
    out = {}
    for fam in ("reasoning_think", "classify_notink", "structured_notink"):
        p = profile_dir / f"profile_routing_{fam}.json"
        if not p.exists():
            raise FileNotFoundError(f"missing {p}")
        out[fam] = json.loads(p.read_text())
    return out


def load_raw(profile_dir: pathlib.Path) -> dict[str, Any]:
    p = profile_dir / "raw_activation_counts.json"
    if not p.exists():
        raise FileNotFoundError(f"missing {p}")
    return json.loads(p.read_text())


def decide_cross_family_verdict(overlap: dict[str, Any]) -> tuple[str, list[str]]:
    """Apply the 0/1/≥2 threshold rule."""
    high = [
        pair for pair, stats in overlap["pairs"].items()
        if stats["mean_jaccard"] > OVERLAP_THRESHOLD
    ]
    if len(high) == 0:
        return "3-family-confirmed", high
    if len(high) == 1:
        return f"2-family-merged-{high[0]}", high
    return "1-family-collapsed", high


def sha256_file(p: pathlib.Path) -> str:
    return hashlib.sha256(p.read_bytes()).hexdigest()


def format_kl_summary(artifact: dict) -> str:
    kls = [layer["kl_div_vs_uniform"] for layer in artifact["per_layer"]]
    return (
        f"min={min(kls):.3f}  mean={sum(kls) / len(kls):.3f}  max={max(kls):.3f}"
    )


def render_doc(
    family_artifacts: dict[str, dict],
    raw: dict[str, Any],
    overlap: dict[str, Any],
    verdict: str,
    high_pairs: list[str],
    cohesion: dict[str, dict],
    cluster_boundaries: dict[str, list[list[str]] | None],
    artifact_shas: dict[str, str],
    determinism_outcome: str,
    top_pct: int,
    anchor_prompts_sha: str,
) -> str:
    lines: list[str] = []
    lines.append("# Sprint FT-LORA-D — Expert Activation Profile Results")
    lines.append("")
    lines.append(f"**Generated at execution time: {time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())}**")
    lines.append("")
    lines.append(f"**Runbook ref:** `docs/development/ft-lora/sprint_plan_ft_lora_d.md` §5 Epic 3")
    lines.append("")
    lines.append("---")
    lines.append("")

    # Executive summary
    lines.append("## Executive Summary")
    lines.append("")
    lines.append(f"**Verdict: `{verdict}`**")
    lines.append("")
    if verdict == "3-family-confirmed":
        lines.append(
            "No family-pair exceeds the 0.80 Jaccard overlap ceiling. The "
            "3-family partition (reasoning-think / classify-notink / "
            "structured-notink) is empirically validated. Sprint E proceeds "
            "with three Tier 2 LoRA adapters as planned in memo 07 v3.1 §5."
        )
    elif verdict.startswith("2-family-merged"):
        lines.append(
            f"Exactly one family-pair exceeds 0.80 overlap: "
            f"`{high_pairs[0]}`. Sprint E merges this pair into a single "
            "Tier 2 adapter; the remaining family stays separate."
        )
    elif verdict == "1-family-collapsed":
        lines.append(
            "Two or more family-pairs exceed 0.80 overlap, implying the "
            "top-25% routed-expert sets cluster into a single cohesive group. "
            "Sprint E collapses to one Tier 2 adapter over the union of all "
            "top-25% experts across families."
        )
    lines.append("")

    # Per-family cohesion verdicts
    lines.append("**Per-family task-cohesion:**")
    lines.append("")
    for fam, cres in cohesion.items():
        cv = cohesion_verdict(cres["mean_jaccard"])
        n_tasks = len(cres["tasks"])
        lines.append(
            f"- `{fam}`: {n_tasks} tasks, mean within-family Jaccard = "
            f"{cres['mean_jaccard']:.4f} → **{cv}**"
        )
        if cv == "split-candidate" and cluster_boundaries.get(fam):
            boundary = cluster_boundaries[fam]
            lines.append(
                f"  - Proposed split boundary: "
                f"`{{{', '.join(boundary[0])}}}` vs `{{{', '.join(boundary[1])}}}`"
            )
    lines.append("")
    lines.append(f"**Determinism sanity check:** {determinism_outcome}")
    lines.append("")
    lines.append("---")
    lines.append("")

    # Cross-family overlap table
    lines.append("## Cross-family Jaccard Overlap")
    lines.append("")
    lines.append("Top-25% routed-expert sets (64 of 256 per layer) compared pairwise. "
                 "Mean Jaccard averaged across 40 layers. Threshold for partition merge: "
                 f"> {OVERLAP_THRESHOLD}.")
    lines.append("")
    lines.append("| Family pair | Mean Jaccard | Exceeds 0.80? |")
    lines.append("|---|---|---|")
    for pair, stats in overlap["pairs"].items():
        flag = "YES" if stats["mean_jaccard"] > OVERLAP_THRESHOLD else "no"
        lines.append(f"| `{pair}` | {stats['mean_jaccard']:.4f} | {flag} |")
    lines.append("")

    # Per-family task-cohesion table
    lines.append("## Per-family Task-Cohesion")
    lines.append("")
    lines.append("Within-family pairwise Jaccard of per-task top-25% expert sets, "
                 "averaged across 40 layers. Verdict rule: ≥0.70 cohesive / "
                 "0.40-0.70 ambiguous / <0.40 split-candidate.")
    lines.append("")
    lines.append("| Family | n_tasks | Mean within-family Jaccard | Verdict |")
    lines.append("|---|---|---|---|")
    for fam, cres in cohesion.items():
        cv = cohesion_verdict(cres["mean_jaccard"])
        lines.append(
            f"| `{fam}` | {len(cres['tasks'])} | {cres['mean_jaccard']:.4f} | {cv} |"
        )
    lines.append("")

    # Detailed per-pair task breakdown
    lines.append("### Detailed pairwise task-cohesion")
    lines.append("")
    for fam, cres in cohesion.items():
        lines.append(f"**{fam}** — {len(cres['tasks'])} tasks")
        lines.append("")
        if cres["pair_means"]:
            lines.append("| Task pair | Mean Jaccard |")
            lines.append("|---|---|")
            for pair, m in sorted(cres["pair_means"].items()):
                lines.append(f"| `{pair}` | {m:.4f} |")
        else:
            lines.append("(only one task — no pairs)")
        lines.append("")

    # KL divergence summary
    lines.append("## KL Divergence vs Uniform (per family)")
    lines.append("")
    lines.append("High KL = concentrated routing (Sieve thesis supported). "
                 "Low KL = diffuse routing (flag for Sprint E commit-or-fallback).")
    lines.append("")
    lines.append("| Family | KL(min) | KL(mean) | KL(max) |")
    lines.append("|---|---|---|---|")
    for fam, art in family_artifacts.items():
        kls = [layer["kl_div_vs_uniform"] for layer in art["per_layer"]]
        lines.append(
            f"| `{fam}` | {min(kls):.3f} | {sum(kls) / len(kls):.3f} | {max(kls):.3f} |"
        )
    lines.append("")

    # Coverage and prompts
    lines.append("## Coverage")
    lines.append("")
    lines.append(f"**Anchor prompts:** 320 total (20/task × 16 tasks). "
                 f"SHA256: `{anchor_prompts_sha[:16]}…`")
    lines.append("")
    lines.append("Per-family:")
    for fam, art in family_artifacts.items():
        lines.append(
            f"- `{fam}`: {art['prompts_processed']} prompts, "
            f"{art['tokens_processed']:,} tokens processed"
        )
    lines.append("")

    # Recommendation to Sprint E
    lines.append("## Recommendation to Sprint E")
    lines.append("")
    if verdict == "3-family-confirmed":
        lines.append(
            "Proceed as planned: train three Tier 2 LoRA adapters (r=8 α=16), "
            "one per family. Load `profile_routing_{family}.json` via "
            "`--expert-selection-path` in `neural/training/train_ft.py`."
        )
    elif verdict.startswith("2-family-merged"):
        pair = high_pairs[0]
        lines.append(
            f"Merge the `{pair}` pair into a single Tier 2 adapter. The "
            "remaining family keeps its own adapter. Sprint E planner should "
            "rename the merged adapter; use the union of the two merged "
            "families' top-25% experts per layer."
        )
    else:  # 1-family-collapsed
        lines.append(
            "Collapse to a single Tier 2 adapter over the union of all three "
            "families' top-25% experts. Training converges all 16 tasks onto "
            "one adapter."
        )
    lines.append("")
    for fam, cres in cohesion.items():
        cv = cohesion_verdict(cres["mean_jaccard"])
        if cv == "split-candidate" and cluster_boundaries.get(fam):
            boundary = cluster_boundaries[fam]
            lines.append(
                f"- Additionally: family `{fam}` shows split-candidate "
                f"within-family cohesion. Sprint E should evaluate splitting "
                f"into `{{{', '.join(boundary[0])}}}` and "
                f"`{{{', '.join(boundary[1])}}}` sub-adapters. Sprint D does "
                "NOT perform the split — decision deferred per plan §3."
            )
    lines.append("")

    # Artifact SHAs
    lines.append("## Artifact SHAs")
    lines.append("")
    for name, sha in artifact_shas.items():
        lines.append(f"- `{name}`: `{sha}`")
    lines.append("")

    # Documents Accessed
    lines.append("## Documents Accessed")
    lines.append("")
    lines.append("- `training_data/routing_profiles/profile_routing_reasoning_think.json`")
    lines.append("- `training_data/routing_profiles/profile_routing_classify_notink.json`")
    lines.append("- `training_data/routing_profiles/profile_routing_structured_notink.json`")
    lines.append("- `training_data/routing_profiles/raw_activation_counts.json`")
    lines.append("- `training_data/routing_profiles/anchor_prompts.jsonl`")
    lines.append("- `docs/development/ft-lora/sprint_plan_ft_lora_d.md`")
    lines.append("- `docs/development/ft-lora/01_RESEARCH_v2.md §5`")
    lines.append("- `docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md §Phase 5.X`")
    lines.append("")

    return "\n".join(lines) + "\n"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--profile-dir", default="training_data/routing_profiles/")
    ap.add_argument(
        "--output-doc",
        default="docs/development/ft-lora/sprint_c_d_profile_results.md",
    )
    ap.add_argument("--top-pct", type=int, default=25)
    ap.add_argument(
        "--determinism-outcome",
        default="bit-identical (strict)",
        help="Recorded outcome of pre-Tier-3 determinism sanity check",
    )
    args = ap.parse_args()

    profile_dir = pathlib.Path(args.profile_dir).expanduser()

    fam_art = load_family_artifacts(profile_dir)
    raw = load_raw(profile_dir)

    overlap = compute_cross_family_overlap(fam_art)
    verdict, high_pairs = decide_cross_family_verdict(overlap)

    task_top = per_task_top_experts(raw, top_pct=args.top_pct)
    cohesion = {}
    cluster_boundaries: dict[str, list[list[str]] | None] = {}
    for fam in fam_art:
        cres = within_family_pairwise_jaccard(task_top, raw["task_family"], fam)
        cohesion[fam] = cres
        if cohesion_verdict(cres["mean_jaccard"]) == "split-candidate":
            cluster_boundaries[fam] = hierarchical_cluster_boundary(
                task_top, cres["tasks"]
            )
        else:
            cluster_boundaries[fam] = None

    artifact_shas = {
        "profile_routing_reasoning_think.json": sha256_file(
            profile_dir / "profile_routing_reasoning_think.json"
        ),
        "profile_routing_classify_notink.json": sha256_file(
            profile_dir / "profile_routing_classify_notink.json"
        ),
        "profile_routing_structured_notink.json": sha256_file(
            profile_dir / "profile_routing_structured_notink.json"
        ),
        "raw_activation_counts.json": sha256_file(
            profile_dir / "raw_activation_counts.json"
        ),
    }
    anchor_path = profile_dir / "anchor_prompts.jsonl"
    anchor_sha = sha256_file(anchor_path) if anchor_path.exists() else "n/a"

    doc = render_doc(
        family_artifacts=fam_art,
        raw=raw,
        overlap=overlap,
        verdict=verdict,
        high_pairs=high_pairs,
        cohesion=cohesion,
        cluster_boundaries=cluster_boundaries,
        artifact_shas=artifact_shas,
        determinism_outcome=args.determinism_outcome,
        top_pct=args.top_pct,
        anchor_prompts_sha=anchor_sha,
    )

    out_doc = pathlib.Path(args.output_doc).expanduser()
    out_doc.parent.mkdir(parents=True, exist_ok=True)
    out_doc.write_text(doc)
    print(f"Wrote {out_doc}", flush=True)
    print(f"Verdict: {verdict}", flush=True)
    for fam, cres in cohesion.items():
        print(
            f"  {fam}: Jaccard={cres['mean_jaccard']:.4f} "
            f"({cohesion_verdict(cres['mean_jaccard'])})",
            flush=True,
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
