#!/usr/bin/env python3
"""
openai_ft_compare.py — Compare baseline vs fine-tuned eval results.

Reads two ``results.jsonl`` files (plus their ``summary.json`` siblings)
produced by ``openai_ft_baseline_eval.py`` and emits a Markdown report
comparing them record-by-record (matched by record index `i`).

Report contains:
    - Overview: mean cosine delta, parse-pass-rate delta, per-task counts
    - Win / loss / tie table at a configurable cosine-delta threshold
    - Per-task breakdown showing which tasks improved, which regressed
    - Qualitative appendices: 5 worst regressions, 5 best improvements

Usage:
    python scripts/openai_ft_compare.py \\
        --baseline training_data/openai_ft/20260420/eval/baseline \\
        --ft       training_data/openai_ft/20260420/eval/ft \\
        --output   training_data/openai_ft/20260420/eval_comparison.md
"""

from __future__ import annotations

import argparse
import json
import math
import os
import sys
from collections import defaultdict


def _load(dir_path: str) -> tuple[dict, list[dict]]:
    summary_path = os.path.join(dir_path, "summary.json")
    results_path = os.path.join(dir_path, "results.jsonl")
    with open(summary_path, encoding="utf-8") as f:
        summary = json.load(f)
    records: list[dict] = []
    with open(results_path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            records.append(json.loads(line))
    return summary, records


def _delta(ft: float | None, base: float | None) -> str:
    if ft is None or base is None:
        return "—"
    d = ft - base
    sign = "+" if d > 0 else ("−" if d < 0 else "±")
    return f"{sign}{abs(d):.3f}"


def _truncate(text: str, n: int = 400) -> str:
    t = (text or "").replace("\n", " ")
    return t if len(t) <= n else t[:n] + "…"


def compare(baseline_dir: str, ft_dir: str, win_threshold: float) -> str:
    base_summary, base_recs = _load(baseline_dir)
    ft_summary, ft_recs = _load(ft_dir)

    # Match by index `i` (deterministic sampling → same records if same seed).
    base_by_i = {r.get("i"): r for r in base_recs}
    ft_by_i = {r.get("i"): r for r in ft_recs}
    matched_i = sorted(set(base_by_i) & set(ft_by_i))

    wins = losses = ties = 0
    cos_deltas: list[float] = []
    per_task: dict[str, dict] = defaultdict(
        lambda: {"n": 0, "wins": 0, "losses": 0, "ties": 0, "base_cos": [], "ft_cos": []}
    )
    regressions: list[tuple[float, dict, dict]] = []  # (delta, base, ft)
    improvements: list[tuple[float, dict, dict]] = []

    for i in matched_i:
        b = base_by_i[i]
        f = ft_by_i[i]
        if "error" in b or "error" in f:
            continue
        bc = b.get("cosine")
        fc = f.get("cosine")
        if bc is None or fc is None:
            continue
        if isinstance(bc, float) and math.isnan(bc):
            continue
        if isinstance(fc, float) and math.isnan(fc):
            continue
        delta = fc - bc
        cos_deltas.append(delta)
        task = b.get("task") or f.get("task") or "__unattributed__"
        pt = per_task[task]
        pt["n"] += 1
        pt["base_cos"].append(bc)
        pt["ft_cos"].append(fc)
        if delta > win_threshold:
            wins += 1
            pt["wins"] += 1
            improvements.append((delta, b, f))
        elif delta < -win_threshold:
            losses += 1
            pt["losses"] += 1
            regressions.append((delta, b, f))
        else:
            ties += 1
            pt["ties"] += 1

    mean_delta = sum(cos_deltas) / len(cos_deltas) if cos_deltas else None

    # --- Build markdown ---
    lines: list[str] = []
    lines.append("# Fine-Tuning Evaluation — Baseline vs Fine-Tuned\n")
    lines.append(f"Generated: {ft_summary.get('generated_at_utc', '(ft summary missing timestamp)')}\n")
    lines.append("## Headline\n")
    lines.append("| Metric | Baseline | Fine-Tuned | Δ |")
    lines.append("|---|---|---|---|")
    lines.append(
        f"| Model | `{base_summary.get('model')}` | `{ft_summary.get('model')}` | — |"
    )
    lines.append(
        f"| Sample size | {base_summary.get('sample_size_actual')} | "
        f"{ft_summary.get('sample_size_actual')} | — |"
    )
    lines.append(
        f"| Mean cosine similarity | "
        f"{base_summary.get('mean_cosine'):.4f} | "
        f"{ft_summary.get('mean_cosine'):.4f} | "
        f"**{_delta(ft_summary.get('mean_cosine'), base_summary.get('mean_cosine'))}** |"
    )
    lines.append(
        f"| JSON parse-pass rate | "
        f"{base_summary.get('parse_pass_rate'):.3f} | "
        f"{ft_summary.get('parse_pass_rate'):.3f} | "
        f"**{_delta(ft_summary.get('parse_pass_rate'), base_summary.get('parse_pass_rate'))}** |"
    )
    lines.append(
        f"| Errors | {base_summary.get('n_errors', 0)} | "
        f"{ft_summary.get('n_errors', 0)} | — |"
    )
    lines.append("")
    lines.append(
        f"**Mean cosine delta across {len(cos_deltas)} matched records: "
        f"`{mean_delta:+.4f}`** (threshold ±{win_threshold:.2f} for win/loss)"
    )
    lines.append("")

    # --- Win/Loss/Tie ---
    lines.append("## Win / Loss / Tie\n")
    total = wins + losses + ties
    if total:
        lines.append("| Outcome | Count | % of matched |")
        lines.append("|---|---|---|")
        lines.append(f"| Wins (FT better by > {win_threshold:.2f}) | {wins} | {wins/total*100:.1f}% |")
        lines.append(f"| Losses (FT worse by > {win_threshold:.2f}) | {losses} | {losses/total*100:.1f}% |")
        lines.append(f"| Ties (within ±{win_threshold:.2f}) | {ties} | {ties/total*100:.1f}% |")
    lines.append("")

    # --- Per-task ---
    lines.append("## Per-Task Breakdown\n")
    lines.append("Sorted by per-task mean cosine delta (best → worst).")
    lines.append("")
    lines.append("| Task | N | Base cos | FT cos | Δ | Wins | Losses | Ties |")
    lines.append("|---|---|---|---|---|---|---|---|")
    rows = []
    for task, pt in per_task.items():
        base_mean = sum(pt["base_cos"]) / len(pt["base_cos"]) if pt["base_cos"] else None
        ft_mean = sum(pt["ft_cos"]) / len(pt["ft_cos"]) if pt["ft_cos"] else None
        d = (ft_mean - base_mean) if (base_mean is not None and ft_mean is not None) else None
        rows.append((task, pt, base_mean, ft_mean, d))
    rows.sort(key=lambda r: (r[4] if r[4] is not None else -999), reverse=True)
    for task, pt, base_mean, ft_mean, d in rows:
        lines.append(
            f"| `{task}` | {pt['n']} | "
            f"{'—' if base_mean is None else f'{base_mean:.3f}'} | "
            f"{'—' if ft_mean is None else f'{ft_mean:.3f}'} | "
            f"{'—' if d is None else f'{d:+.3f}'} | "
            f"{pt['wins']} | {pt['losses']} | {pt['ties']} |"
        )
    lines.append("")

    # --- Appendix: Regressions ---
    regressions.sort(key=lambda x: x[0])  # most negative first
    if regressions:
        lines.append("## Appendix A — 5 Worst Regressions (FT under-performed baseline most)\n")
        for delta, b, f in regressions[:5]:
            lines.append(f"### i={b['i']} · task=`{b.get('task')}` · Δ={delta:+.3f}")
            lines.append(f"**Ground truth:** `{_truncate(b.get('ground_truth', ''))}`\n")
            lines.append(f"**Baseline response:** `{_truncate(b.get('response', ''))}`\n")
            lines.append(f"**FT response:** `{_truncate(f.get('response', ''))}`\n")

    if improvements:
        improvements.sort(key=lambda x: -x[0])
        lines.append("## Appendix B — 5 Best Improvements (FT most above baseline)\n")
        for delta, b, f in improvements[:5]:
            lines.append(f"### i={b['i']} · task=`{b.get('task')}` · Δ={delta:+.3f}")
            lines.append(f"**Ground truth:** `{_truncate(b.get('ground_truth', ''))}`\n")
            lines.append(f"**Baseline response:** `{_truncate(b.get('response', ''))}`\n")
            lines.append(f"**FT response:** `{_truncate(f.get('response', ''))}`\n")

    # --- Verdict ---
    lines.append("## Verdict\n")
    if mean_delta is None:
        lines.append("_Insufficient matched records to form a verdict._")
    elif mean_delta >= 0.05:
        lines.append(
            f"**IMPROVED** — mean cosine delta {mean_delta:+.4f} meets the +0.05 threshold. "
            f"Fine-tuning moved the needle."
        )
    elif mean_delta > 0:
        lines.append(
            f"**MARGINAL** — mean cosine delta {mean_delta:+.4f} is positive but below +0.05. "
            f"Inspect per-task breakdown; the effect may be concentrated in specific tasks."
        )
    else:
        lines.append(
            f"**NO IMPROVEMENT** — mean cosine delta {mean_delta:+.4f}. "
            f"Review per-task breakdown for any tasks that did improve; consider more epochs, "
            f"different data, or a different base model for v2."
        )
    return "\n".join(lines) + "\n"


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--baseline", required=True, help="Baseline eval output dir")
    p.add_argument("--ft", required=True, help="FT eval output dir")
    p.add_argument("--output", required=True, help="Markdown output path")
    p.add_argument("--win-threshold", type=float, default=0.05,
                   help="Cosine delta threshold to count a record as a win/loss.")
    args = p.parse_args(argv)

    md = compare(args.baseline, args.ft, args.win_threshold)
    os.makedirs(os.path.dirname(args.output) or ".", exist_ok=True)
    with open(args.output, "w", encoding="utf-8") as f:
        f.write(md)
    print(f"wrote {args.output}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
