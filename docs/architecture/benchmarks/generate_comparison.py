#!/usr/bin/env python3
"""
Cognitive Gap Benchmark Comparison Generator

Reads graded results from baseline and MDEMG runs, produces:
  - comparison.json: Machine-readable statistical comparison
  - BENCHMARK_RESULTS.md: Human-readable report

Usage:
    python generate_comparison.py [--results-dir <path>]
    python generate_comparison.py --results-dir docs/architecture/benchmarks/whk-wms/cognitive_gap_validation_20260225
"""

import json
import math
import statistics
import argparse
from pathlib import Path
from datetime import datetime
from typing import Dict, List, Optional, Tuple


SCRIPT_DIR = Path(__file__).parent
DEFAULT_RESULTS_DIR = SCRIPT_DIR / "whk-wms" / "cognitive_gap_validation_20260226"
CANONICAL_BASELINE_MEAN = 0.783  # temporal_validation_20260203


# ─── Data Loading ────────────────────────────────────────────────────────────

def load_grades(grades_file: Path) -> Optional[Dict]:
    """Load a grades.json file."""
    if not grades_file.exists():
        return None
    with open(grades_file) as f:
        return json.load(f)


def discover_runs(results_dir: Path) -> Tuple[List[Dict], List[Dict]]:
    """Discover all graded runs in the results directory.

    Returns:
        (baseline_grades, mdemg_grades) — lists of grade dicts
    """
    baseline_grades = []
    mdemg_grades = []

    for mode, grade_list in [("baseline", baseline_grades), ("mdemg", mdemg_grades)]:
        mode_dir = results_dir / mode
        if not mode_dir.exists():
            continue
        for run_dir in sorted(mode_dir.iterdir()):
            if not run_dir.is_dir():
                continue
            grades_file = run_dir / "grades.json"
            grades = load_grades(grades_file)
            if grades:
                grades["_run_dir"] = str(run_dir)
                grade_list.append(grades)

    return baseline_grades, mdemg_grades


# ─── Statistical Tests ───────────────────────────────────────────────────────

def welch_t_test(group1: List[float], group2: List[float]) -> Tuple[float, float]:
    """Welch's t-test for independent samples with unequal variance.

    Returns:
        (t_statistic, approximate_p_value)
    """
    n1, n2 = len(group1), len(group2)
    if n1 < 2 or n2 < 2:
        return 0.0, 1.0

    mean1, mean2 = statistics.mean(group1), statistics.mean(group2)
    var1 = statistics.variance(group1)
    var2 = statistics.variance(group2)

    se = math.sqrt(var1 / n1 + var2 / n2)
    if se == 0:
        return 0.0, 1.0

    t_stat = (mean2 - mean1) / se

    # Welch-Satterthwaite degrees of freedom
    num = (var1 / n1 + var2 / n2) ** 2
    denom = (var1 / n1) ** 2 / (n1 - 1) + (var2 / n2) ** 2 / (n2 - 1)
    df = num / denom if denom > 0 else 1

    # Approximate p-value using t-distribution CDF approximation
    # For large df this approaches the normal distribution
    p_value = _t_distribution_p(abs(t_stat), df)
    return round(t_stat, 4), round(p_value, 6)


def _t_distribution_p(t: float, df: float) -> float:
    """Approximate two-tailed p-value from t-distribution.

    Uses the approximation: p ≈ 2 * (1 - Φ(t * (1 - 1/(4*df))))
    which is reasonable for df > 5.
    """
    if df <= 0:
        return 1.0
    # Correct t for finite df
    corrected_t = t * (1 - 1 / (4 * df))
    # Standard normal CDF approximation (Abramowitz & Stegun)
    p_one_tail = _normal_cdf(-abs(corrected_t))
    return 2 * p_one_tail


def _normal_cdf(x: float) -> float:
    """Standard normal CDF approximation."""
    return 0.5 * (1 + math.erf(x / math.sqrt(2)))


def mann_whitney_u(group1: List[float], group2: List[float]) -> Tuple[float, float]:
    """Mann-Whitney U test (non-parametric).

    Returns:
        (u_statistic, approximate_p_value)
    """
    n1, n2 = len(group1), len(group2)
    if n1 == 0 or n2 == 0:
        return 0.0, 1.0

    # Count how many times group2 > group1
    u = 0
    for x in group1:
        for y in group2:
            if y > x:
                u += 1
            elif y == x:
                u += 0.5

    # Normal approximation for large samples
    mu = n1 * n2 / 2
    sigma = math.sqrt(n1 * n2 * (n1 + n2 + 1) / 12)
    if sigma == 0:
        return u, 1.0

    z = (u - mu) / sigma
    p_value = 2 * _normal_cdf(-abs(z))
    return round(u, 1), round(p_value, 6)


# ─── Aggregate Analysis ─────────────────────────────────────────────────────

def analyze_group(grade_files: List[Dict]) -> Dict:
    """Compute aggregate statistics for a group of runs."""
    if not grade_files:
        return {"runs": 0}

    run_means = []
    all_scores = []
    evidence_rates = []
    high_score_rates = []
    correct_file_rates = []

    for grades in grade_files:
        agg = grades.get("aggregate", {})
        run_means.append(agg.get("mean", 0))
        evidence_rates.append(agg.get("evidence_rate", 0))
        high_score_rates.append(agg.get("high_score_rate", 0))
        correct_file_rates.append(agg.get("correct_file_rate", 0))

        # Collect all per-question scores
        for pq in grades.get("per_question", []):
            scores = pq.get("scores", {})
            all_scores.append(scores.get("final", 0))

    return {
        "runs": len(grade_files),
        "mean_scores": [round(m, 3) for m in run_means],
        "aggregate_mean": round(statistics.mean(run_means), 3) if run_means else 0,
        "aggregate_std": round(statistics.stdev(run_means), 4) if len(run_means) > 1 else 0,
        "all_questions_mean": round(statistics.mean(all_scores), 3) if all_scores else 0,
        "all_questions_std": round(statistics.stdev(all_scores), 3) if len(all_scores) > 1 else 0,
        "evidence_rate": round(statistics.mean(evidence_rates), 3) if evidence_rates else 0,
        "high_score_rate": round(statistics.mean(high_score_rates), 3) if high_score_rates else 0,
        "correct_file_rate": round(statistics.mean(correct_file_rates), 3) if correct_file_rates else 0,
        "total_questions_graded": len(all_scores),
    }


def analyze_by_category(grade_files: List[Dict]) -> Dict[str, Dict]:
    """Compute per-category statistics across runs."""
    cat_scores = {}
    for grades in grade_files:
        for pq in grades.get("per_question", []):
            cat = pq.get("category", "unknown")
            score = pq.get("scores", {}).get("final", 0)
            cat_scores.setdefault(cat, []).append(score)

    result = {}
    for cat, scores in sorted(cat_scores.items()):
        result[cat] = {
            "count": len(scores),
            "mean": round(statistics.mean(scores), 3),
            "std": round(statistics.stdev(scores), 3) if len(scores) > 1 else 0,
        }
    return result


def compute_per_question_delta(baseline_grades: List[Dict], mdemg_grades: List[Dict]) -> List[Dict]:
    """Compute per-question improvement from baseline to MDEMG.

    Averages scores across runs for each question, then computes delta.
    """
    # Collect scores per question ID
    baseline_by_q = {}
    mdemg_by_q = {}

    for grades in baseline_grades:
        for pq in grades.get("per_question", []):
            qid = str(pq["id"])
            baseline_by_q.setdefault(qid, []).append(pq["scores"]["final"])

    for grades in mdemg_grades:
        for pq in grades.get("per_question", []):
            qid = str(pq["id"])
            mdemg_by_q.setdefault(qid, []).append(pq["scores"]["final"])

    deltas = []
    def sort_key(x):
        try:
            return (0, int(x))
        except ValueError:
            return (1, x)
    for qid in sorted(baseline_by_q.keys(), key=sort_key):
        if qid in mdemg_by_q:
            b_mean = statistics.mean(baseline_by_q[qid])
            m_mean = statistics.mean(mdemg_by_q[qid])
            deltas.append({
                "id": qid,
                "baseline_mean": round(b_mean, 3),
                "mdemg_mean": round(m_mean, 3),
                "delta": round(m_mean - b_mean, 3),
            })

    return sorted(deltas, key=lambda x: x["delta"], reverse=True)


# ─── Report Generation ───────────────────────────────────────────────────────

def generate_report(
    baseline_stats: Dict,
    mdemg_stats: Dict,
    delta: Dict,
    baseline_cats: Dict,
    mdemg_cats: Dict,
    per_q_deltas: List[Dict],
    output_file: Path,
):
    """Generate BENCHMARK_RESULTS.md report."""
    lines = []

    lines.append("# Cognitive Gap Benchmark Results")
    lines.append(f"\n**Date**: {datetime.now().strftime('%Y-%m-%d')}")
    lines.append(f"**Model**: Claude Haiku")
    lines.append(f"**Questions**: 120 (whk-wms codebase)")
    lines.append(f"**Baseline runs**: {baseline_stats['runs']} | **MDEMG runs**: {mdemg_stats['runs']}")
    lines.append(f"\n## Executive Summary\n")

    mean_imp = delta.get("mean_improvement_pct", "N/A")
    sig = "Yes" if delta.get("statistically_significant") else "No"
    lines.append(f"MDEMG-assisted Haiku agents scored **{mdemg_stats['aggregate_mean']:.3f}** mean "
                 f"vs baseline Haiku at **{baseline_stats['aggregate_mean']:.3f}** mean "
                 f"({mean_imp} improvement).")
    lines.append(f"Statistical significance (p < 0.05): **{sig}** (p = {delta.get('welch_p_value', 'N/A')})")

    # Aggregate comparison table
    lines.append(f"\n## Aggregate Comparison\n")
    lines.append("| Metric | Baseline | MDEMG | Delta |")
    lines.append("|--------|----------|-------|-------|")

    def fmt_delta(b, m, pct=False):
        d = m - b
        sign = "+" if d >= 0 else ""
        if pct:
            return f"{sign}{d*100:.1f}pp"
        return f"{sign}{d:.3f}"

    b, m = baseline_stats, mdemg_stats
    lines.append(f"| Mean Score | {b['aggregate_mean']:.3f} | {m['aggregate_mean']:.3f} | {fmt_delta(b['aggregate_mean'], m['aggregate_mean'])} |")
    lines.append(f"| Evidence Rate | {b['evidence_rate']:.1%} | {m['evidence_rate']:.1%} | {fmt_delta(b['evidence_rate'], m['evidence_rate'], True)} |")
    lines.append(f"| High Score Rate (>=0.7) | {b['high_score_rate']:.1%} | {m['high_score_rate']:.1%} | {fmt_delta(b['high_score_rate'], m['high_score_rate'], True)} |")
    lines.append(f"| Correct File Rate | {b['correct_file_rate']:.1%} | {m['correct_file_rate']:.1%} | {fmt_delta(b['correct_file_rate'], m['correct_file_rate'], True)} |")

    # Per-run scores
    lines.append(f"\n## Per-Run Scores\n")
    lines.append("| Run | Baseline Mean | MDEMG Mean |")
    lines.append("|-----|---------------|------------|")
    max_runs = max(len(b.get("mean_scores", [])), len(m.get("mean_scores", [])))
    for i in range(max_runs):
        b_score = b["mean_scores"][i] if i < len(b.get("mean_scores", [])) else "—"
        m_score = m["mean_scores"][i] if i < len(m.get("mean_scores", [])) else "—"
        lines.append(f"| Run {i+1} | {b_score} | {m_score} |")

    # Statistical tests
    lines.append(f"\n## Statistical Tests\n")
    lines.append(f"- **Welch's t-test**: t = {delta.get('welch_t', 'N/A')}, p = {delta.get('welch_p_value', 'N/A')}")
    lines.append(f"- **Mann-Whitney U**: U = {delta.get('mann_whitney_u', 'N/A')}, p = {delta.get('mann_whitney_p', 'N/A')}")
    lines.append(f"- **Significant at p < 0.05**: {sig}")

    # Category breakdown
    lines.append(f"\n## By Category\n")
    all_cats = sorted(set(list(baseline_cats.keys()) + list(mdemg_cats.keys())))
    lines.append("| Category | Baseline Mean | MDEMG Mean | Delta |")
    lines.append("|----------|---------------|------------|-------|")
    for cat in all_cats:
        b_cat = baseline_cats.get(cat, {})
        m_cat = mdemg_cats.get(cat, {})
        b_mean = b_cat.get("mean", 0)
        m_mean = m_cat.get("mean", 0)
        d = m_mean - b_mean
        sign = "+" if d >= 0 else ""
        lines.append(f"| {cat} | {b_mean:.3f} | {m_mean:.3f} | {sign}{d:.3f} |")

    # Top improved questions
    lines.append(f"\n## Top 10 Most Improved Questions (MDEMG vs Baseline)\n")
    lines.append("| Q ID | Baseline | MDEMG | Delta |")
    lines.append("|------|----------|-------|-------|")
    for pq in per_q_deltas[:10]:
        lines.append(f"| {pq['id']} | {pq['baseline_mean']:.3f} | {pq['mdemg_mean']:.3f} | +{pq['delta']:.3f} |")

    # Top regressed questions
    regressed = [pq for pq in per_q_deltas if pq["delta"] < 0]
    if regressed:
        lines.append(f"\n## Top 10 Most Regressed Questions\n")
        lines.append("| Q ID | Baseline | MDEMG | Delta |")
        lines.append("|------|----------|-------|-------|")
        for pq in sorted(regressed, key=lambda x: x["delta"])[:10]:
            lines.append(f"| {pq['id']} | {pq['baseline_mean']:.3f} | {pq['mdemg_mean']:.3f} | {pq['delta']:.3f} |")

    # Canonical baseline comparison
    lines.append(f"\n## Canonical Baseline Comparison\n")
    lines.append(f"- **Temporal baseline (2026-02-03, Sonnet)**: {CANONICAL_BASELINE_MEAN}")
    lines.append(f"- **Current MDEMG (Haiku)**: {m['aggregate_mean']:.3f}")
    delta_canon = m["aggregate_mean"] - CANONICAL_BASELINE_MEAN
    sign = "+" if delta_canon >= 0 else ""
    lines.append(f"- **Delta from canonical**: {sign}{delta_canon:.3f}")

    lines.append(f"\n---\n*Generated by generate_comparison.py on {datetime.now().isoformat()}*\n")

    with open(output_file, "w") as f:
        f.write("\n".join(lines))

    print(f"Report written to {output_file}")


# ─── Main ────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="Generate cognitive gap benchmark comparison")
    parser.add_argument(
        "--results-dir", type=str, default=str(DEFAULT_RESULTS_DIR),
        help="Directory containing baseline/ and mdemg/ run directories"
    )
    args = parser.parse_args()

    results_dir = Path(args.results_dir)
    analysis_dir = results_dir / "analysis"
    analysis_dir.mkdir(parents=True, exist_ok=True)

    print(f"Scanning results in {results_dir}")

    # Discover runs
    baseline_grades, mdemg_grades = discover_runs(results_dir)
    print(f"Found {len(baseline_grades)} baseline runs, {len(mdemg_grades)} MDEMG runs")

    if not baseline_grades or not mdemg_grades:
        print("ERROR: Need at least 1 baseline and 1 MDEMG run to compare.")
        return

    # Aggregate stats
    baseline_stats = analyze_group(baseline_grades)
    mdemg_stats = analyze_group(mdemg_grades)

    # Category breakdowns
    baseline_cats = analyze_by_category(baseline_grades)
    mdemg_cats = analyze_by_category(mdemg_grades)

    # Per-question deltas
    per_q_deltas = compute_per_question_delta(baseline_grades, mdemg_grades)

    # Statistical tests (using all per-question scores pooled across runs)
    baseline_all_scores = []
    mdemg_all_scores = []
    for grades in baseline_grades:
        for pq in grades.get("per_question", []):
            baseline_all_scores.append(pq["scores"]["final"])
    for grades in mdemg_grades:
        for pq in grades.get("per_question", []):
            mdemg_all_scores.append(pq["scores"]["final"])

    welch_t, welch_p = welch_t_test(baseline_all_scores, mdemg_all_scores)
    mw_u, mw_p = mann_whitney_u(baseline_all_scores, mdemg_all_scores)

    mean_diff = mdemg_stats["aggregate_mean"] - baseline_stats["aggregate_mean"]
    mean_imp_pct = f"+{mean_diff/max(baseline_stats['aggregate_mean'], 0.001)*100:.1f}%" if mean_diff >= 0 else f"{mean_diff/max(baseline_stats['aggregate_mean'], 0.001)*100:.1f}%"

    delta = {
        "mean_improvement": round(mean_diff, 3),
        "mean_improvement_pct": mean_imp_pct,
        "evidence_improvement_pp": round((mdemg_stats["evidence_rate"] - baseline_stats["evidence_rate"]) * 100, 1),
        "high_score_improvement_pp": round((mdemg_stats["high_score_rate"] - baseline_stats["high_score_rate"]) * 100, 1),
        "welch_t": welch_t,
        "welch_p_value": welch_p,
        "mann_whitney_u": mw_u,
        "mann_whitney_p": mw_p,
        "statistically_significant": welch_p < 0.05,
    }

    # Comparison JSON
    comparison = {
        "generated": datetime.now().isoformat(),
        "baseline": baseline_stats,
        "mdemg": mdemg_stats,
        "delta": delta,
        "by_category": {
            "baseline": baseline_cats,
            "mdemg": mdemg_cats,
        },
        "per_question_deltas": per_q_deltas[:20],
        "canonical_baseline_comparison": {
            "temporal_20260203_mean": CANONICAL_BASELINE_MEAN,
            "current_mdemg_mean": mdemg_stats["aggregate_mean"],
            "delta": round(mdemg_stats["aggregate_mean"] - CANONICAL_BASELINE_MEAN, 3),
        },
    }

    comparison_file = analysis_dir / "comparison.json"
    with open(comparison_file, "w") as f:
        json.dump(comparison, f, indent=2)
    print(f"Comparison written to {comparison_file}")

    # Summary
    print(f"\n{'='*60}")
    print(f"  COMPARISON SUMMARY")
    print(f"{'='*60}")
    print(f"  Baseline: {baseline_stats['aggregate_mean']:.3f} mean ({baseline_stats['runs']} runs)")
    print(f"  MDEMG:    {mdemg_stats['aggregate_mean']:.3f} mean ({mdemg_stats['runs']} runs)")
    print(f"  Delta:    {mean_imp_pct}")
    print(f"  Welch p:  {welch_p} ({'significant' if welch_p < 0.05 else 'NOT significant'})")
    print(f"  MW p:     {mw_p} ({'significant' if mw_p < 0.05 else 'NOT significant'})")
    print(f"{'='*60}")

    # Generate markdown report
    report_file = analysis_dir / "BENCHMARK_RESULTS.md"
    generate_report(
        baseline_stats, mdemg_stats, delta,
        baseline_cats, mdemg_cats, per_q_deltas,
        report_file,
    )


if __name__ == "__main__":
    main()
