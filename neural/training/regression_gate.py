"""Regression Gate for MDEMG LoRA Adapter Deployment.

Compares a candidate adapter's evaluation report against a baseline to
decide whether the candidate is safe to deploy.

Gate rules:
    1. No task regresses > 5% on weighted_score
    2. At least 2 tasks improve >= 2%
    3. JSON validity >= 95% for all structured tasks (object output_schema)
    4. Overall weighted score >= baseline

Verdict:
    PASS  (exit 0) — deploy adapter
    FAIL  (exit 1) — reject
    WARN  (exit 2) — manual review needed

Usage:
    python -m training.regression_gate \
        --candidate eval_candidate.json \
        --baseline eval_baseline.json \
        --report gate_report.json
"""

import argparse
import json
import sys
from typing import Any


# Gate thresholds
MAX_REGRESSION_PCT = 0.05   # No task may regress more than 5%
MIN_IMPROVEMENT_PCT = 0.02  # Task must improve at least 2% to count
MIN_IMPROVED_TASKS = 2      # At least 2 tasks must improve
MIN_JSON_VALIDITY = 0.95    # JSON validity pass_rate for structured tasks


def load_eval_report(path: str) -> dict[str, Any]:
    """Load an evaluation report JSON file."""
    with open(path) as f:
        return json.load(f)


def index_by_task(report: dict[str, Any]) -> dict[str, dict[str, Any]]:
    """Index task_results by task name for quick lookup."""
    return {
        r["task"]: r
        for r in report.get("task_results", [])
        if r.get("status") == "evaluated"
    }


def check_regressions(
    candidate: dict[str, dict[str, Any]],
    baseline: dict[str, dict[str, Any]],
) -> list[dict[str, Any]]:
    """Find tasks that regressed beyond the allowed threshold.

    Returns list of regression details.
    """
    regressions = []
    for task, cand_result in candidate.items():
        base_result = baseline.get(task)
        if not base_result:
            continue

        cand_score = cand_result.get("weighted_score", 0)
        base_score = base_result.get("weighted_score", 0)

        if base_score == 0:
            continue

        delta = cand_score - base_score
        delta_pct = delta / base_score

        if delta_pct < -MAX_REGRESSION_PCT:
            regressions.append({
                "task": task,
                "baseline_score": base_score,
                "candidate_score": cand_score,
                "delta": round(delta, 4),
                "delta_pct": round(delta_pct, 4),
            })

    return regressions


def check_improvements(
    candidate: dict[str, dict[str, Any]],
    baseline: dict[str, dict[str, Any]],
) -> list[dict[str, Any]]:
    """Find tasks that improved beyond the minimum threshold.

    Returns list of improvement details.
    """
    improvements = []
    for task, cand_result in candidate.items():
        base_result = baseline.get(task)
        if not base_result:
            continue

        cand_score = cand_result.get("weighted_score", 0)
        base_score = base_result.get("weighted_score", 0)

        if base_score == 0:
            if cand_score > 0:
                improvements.append({
                    "task": task,
                    "baseline_score": 0,
                    "candidate_score": cand_score,
                    "delta": round(cand_score, 4),
                    "delta_pct": 1.0,
                })
            continue

        delta = cand_score - base_score
        delta_pct = delta / base_score

        if delta_pct >= MIN_IMPROVEMENT_PCT:
            improvements.append({
                "task": task,
                "baseline_score": base_score,
                "candidate_score": cand_score,
                "delta": round(delta, 4),
                "delta_pct": round(delta_pct, 4),
            })

    return improvements


def check_json_validity(
    candidate: dict[str, dict[str, Any]],
) -> list[dict[str, Any]]:
    """Find structured tasks where json_valid pass_rate < 95%.

    Returns list of failing tasks.
    """
    failures = []
    for task, result in candidate.items():
        aggregates = result.get("metric_aggregates", {})
        jv = aggregates.get("json_valid")
        if jv is None:
            continue  # Not a structured task

        pass_rate = jv.get("pass_rate", 0)
        if pass_rate < MIN_JSON_VALIDITY:
            failures.append({
                "task": task,
                "json_valid_pass_rate": round(pass_rate, 4),
                "required": MIN_JSON_VALIDITY,
            })

    return failures


def check_overall_score(
    candidate_report: dict[str, Any],
    baseline_report: dict[str, Any],
) -> dict[str, Any]:
    """Compare overall weighted scores."""
    cand_score = candidate_report.get("summary", {}).get("overall_weighted_score", 0)
    base_score = baseline_report.get("summary", {}).get("overall_weighted_score", 0)

    return {
        "candidate_score": cand_score,
        "baseline_score": base_score,
        "delta": round(cand_score - base_score, 4),
        "met": cand_score >= base_score,
    }


def run_gate(
    candidate_report: dict[str, Any],
    baseline_report: dict[str, Any],
) -> dict[str, Any]:
    """Run all gate checks and produce a verdict.

    Returns a gate report dict with verdict, details, and exit code.
    """
    candidate = index_by_task(candidate_report)
    baseline = index_by_task(baseline_report)

    regressions = check_regressions(candidate, baseline)
    improvements = check_improvements(candidate, baseline)
    json_failures = check_json_validity(candidate)
    overall = check_overall_score(candidate_report, baseline_report)

    # Determine verdict
    reasons = []

    if regressions:
        reasons.append(f"{len(regressions)} task(s) regressed > {MAX_REGRESSION_PCT:.0%}")
    if len(improvements) < MIN_IMPROVED_TASKS:
        reasons.append(
            f"only {len(improvements)} task(s) improved >= {MIN_IMPROVEMENT_PCT:.0%} "
            f"(need {MIN_IMPROVED_TASKS})"
        )
    if json_failures:
        reasons.append(f"{len(json_failures)} task(s) below {MIN_JSON_VALIDITY:.0%} JSON validity")
    if not overall["met"]:
        reasons.append("overall weighted score below baseline")

    if not reasons:
        verdict = "PASS"
        exit_code = 0
    elif regressions or json_failures:
        # Hard failures: regressions or JSON validity issues
        verdict = "FAIL"
        exit_code = 1
    else:
        # Soft failures: not enough improvements or overall score dip
        verdict = "WARN"
        exit_code = 2

    return {
        "verdict": verdict,
        "exit_code": exit_code,
        "reasons": reasons,
        "regressions": regressions,
        "improvements": improvements,
        "json_validity_failures": json_failures,
        "overall_score": overall,
        "tasks_evaluated": len(candidate),
        "tasks_in_baseline": len(baseline),
    }


def main():
    parser = argparse.ArgumentParser(
        description="Regression gate for LoRA adapter deployment",
    )
    parser.add_argument(
        "--candidate", required=True,
        help="Candidate evaluation report (from evaluate_ft.py)",
    )
    parser.add_argument(
        "--baseline", required=True,
        help="Baseline evaluation report to compare against",
    )
    parser.add_argument("--report", help="Write gate report to JSON file")
    args = parser.parse_args()

    candidate_report = load_eval_report(args.candidate)
    baseline_report = load_eval_report(args.baseline)

    gate = run_gate(candidate_report, baseline_report)

    # Print summary
    print(f"\n{'═' * 50}")
    print(f"  Regression Gate: {gate['verdict']}")
    print(f"{'═' * 50}")

    if gate["reasons"]:
        print("\nReasons:")
        for r in gate["reasons"]:
            print(f"  - {r}")

    overall = gate["overall_score"]
    print(f"\nOverall: {overall['candidate_score']:.4f} vs {overall['baseline_score']:.4f} "
          f"(delta: {overall['delta']:+.4f})")

    if gate["improvements"]:
        print(f"\nImprovements ({len(gate['improvements'])}):")
        for imp in gate["improvements"]:
            print(f"  {imp['task']}: {imp['baseline_score']:.4f} -> "
                  f"{imp['candidate_score']:.4f} ({imp['delta_pct']:+.1%})")

    if gate["regressions"]:
        print(f"\nRegressions ({len(gate['regressions'])}):")
        for reg in gate["regressions"]:
            print(f"  {reg['task']}: {reg['baseline_score']:.4f} -> "
                  f"{reg['candidate_score']:.4f} ({reg['delta_pct']:+.1%})")

    if gate["json_validity_failures"]:
        print(f"\nJSON Validity Failures ({len(gate['json_validity_failures'])}):")
        for jf in gate["json_validity_failures"]:
            print(f"  {jf['task']}: {jf['json_valid_pass_rate']:.1%} "
                  f"(need {jf['required']:.0%})")

    if args.report:
        with open(args.report, "w") as f:
            json.dump(gate, f, indent=2)
        print(f"\nReport written to {args.report}")

    raise SystemExit(gate["exit_code"])


if __name__ == "__main__":
    main()
