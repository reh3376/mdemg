#!/usr/bin/env python3
"""UVTS A/B Compare Harness — Phase 12 Epic 4.

Compares two UVTS validation runs and applies the merge-gate criterion from
the post-FT-LORA roadmap §RD-9 / Note 02:

    "B mean >= A mean AND no individual question regresses more than
     ab_mode.regression_threshold_per_question (default 0.10)."

The harness reads two grades.json files (the per-question detail emitted by
uvts_runner.py via --output-dir) plus the spec the runs targeted. It
asserts both runs cover the same question_ids (no question-set drift),
computes per-question deltas, applies the gate, and emits a structured
A/B verdict suitable for CI consumption.

Optional --persist-tsdb writes an A/B row to uvts_runs (V0016) with
branch_label='A_vs_B' and links the source run_ids. Requires both source
runs to have been written to TSDB themselves (otherwise the FK linking
columns stay NULL, which is allowed per the V0016 schema).

Usage:
    # File-based comparison (no TSDB required):
    python uvts_ab_compare.py \\
        --baseline /path/to/run-A/grades.json \\
        --candidate /path/to/run-B/grades.json \\
        --spec docs/tests/uvts/specs/lnl_demo_validation.uvts.json \\
        --out /tmp/ab_verdict.json

    # File-based + TSDB row linking (requires --baseline-run-id + --candidate-run-id):
    python uvts_ab_compare.py \\
        --baseline /path/to/run-A/grades.json \\
        --candidate /path/to/run-B/grades.json \\
        --spec docs/tests/uvts/specs/lnl_demo_validation.uvts.json \\
        --baseline-run-id <cuidv2> --candidate-run-id <cuidv2> \\
        --persist-tsdb \\
        --out /tmp/ab_verdict.json

Exit codes (for CI):
    0 — verdict='pass' (B passes the merge gate)
    1 — verdict='fail' (B regresses)
    2 — input error (missing files, schema drift, etc.)
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

# Phase 12 Epic 2: optional TSDB persistence (V0016 schema).
try:
    import psycopg
    from cuid2 import cuid_wrapper
    _CUID = cuid_wrapper()
    HAS_TSDB = True
except ImportError:
    HAS_TSDB = False


def _load_grades(grades_path: str) -> Tuple[Dict[str, Any], Dict[str, Dict[str, Any]]]:
    """Load a grades.json file written by uvts_runner.py.

    Returns (summary, grades_by_id) where grades_by_id maps question_id
    (always coerced to str) to the per-question grade record.
    """
    with open(grades_path) as f:
        data = json.load(f)
    summary = data.get("summary", {}) or {}
    grades = data.get("grades", []) or []
    by_id: Dict[str, Dict[str, Any]] = {}
    for g in grades:
        qid = str(g.get("id", ""))
        if not qid:
            continue
        by_id[qid] = g
    return summary, by_id


def _final_score(grade: Dict[str, Any]) -> float:
    """Pull the canonical blended score out of a grade record (handles missing fields)."""
    scores = grade.get("scores") or {}
    try:
        return float(scores.get("final", 0.0))
    except (TypeError, ValueError):
        return 0.0


def compute_ab_verdict(baseline_summary: Dict[str, Any],
                       baseline_by_id: Dict[str, Dict[str, Any]],
                       candidate_summary: Dict[str, Any],
                       candidate_by_id: Dict[str, Dict[str, Any]],
                       regression_threshold: float) -> Dict[str, Any]:
    """Apply the A/B merge-gate criterion. Returns a JSON-serializable verdict."""
    baseline_ids = set(baseline_by_id.keys())
    candidate_ids = set(candidate_by_id.keys())
    only_in_baseline = sorted(baseline_ids - candidate_ids)
    only_in_candidate = sorted(candidate_ids - baseline_ids)

    if only_in_baseline or only_in_candidate:
        # Question-set drift — the comparison is meaningless.
        return {
            "verdict": "error",
            "error": "question-set drift between baseline and candidate",
            "only_in_baseline": only_in_baseline[:20],
            "only_in_candidate": only_in_candidate[:20],
            "baseline_mean": baseline_summary.get("mean_score"),
            "candidate_mean": candidate_summary.get("mean_score"),
        }

    shared = sorted(baseline_ids & candidate_ids)
    regressions: List[Dict[str, Any]] = []
    improvements_count = 0

    # Phase 14.1 Epic 2 — eps tolerance for floating-point boundary cases.
    # Phase 14's 7 reported regressions all had delta=-0.10000000…ε which is
    # display-equivalent to -0.10 but tripped a strict `<` check. eps=1e-6 is
    # well below the 0.001 grading granularity (so no real regression escapes)
    # while eliminating the false-positive boundary cases.
    eps = 1e-6
    for qid in shared:
        a = _final_score(baseline_by_id[qid])
        b = _final_score(candidate_by_id[qid])
        delta = b - a
        if delta < -(regression_threshold + eps):
            regressions.append({
                "question_id": qid,
                "category": baseline_by_id[qid].get("category"),
                "baseline_score": round(a, 4),
                "candidate_score": round(b, 4),
                "delta": round(delta, 4),
            })
        elif delta > 0:
            improvements_count += 1

    baseline_mean = float(baseline_summary.get("mean_score", 0.0) or 0.0)
    candidate_mean = float(candidate_summary.get("mean_score", 0.0) or 0.0)

    mean_gate_passed = candidate_mean >= baseline_mean
    no_question_regressions = len(regressions) == 0

    verdict = "pass" if mean_gate_passed and no_question_regressions else "fail"

    return {
        "verdict": verdict,
        "criterion": "B mean >= A mean AND no per-question regression > threshold",
        "regression_threshold_per_question": regression_threshold,
        "shared_question_count": len(shared),
        "mean_gate_passed": mean_gate_passed,
        "baseline_mean": round(baseline_mean, 4),
        "candidate_mean": round(candidate_mean, 4),
        "mean_delta": round(candidate_mean - baseline_mean, 4),
        "regressions": regressions,
        "regression_count": len(regressions),
        "improvement_count": improvements_count,
    }


def _persist_ab_row(spec_path: str, verdict: Dict[str, Any],
                    baseline_run_id: Optional[str],
                    candidate_run_id: Optional[str],
                    branch_label: str) -> Optional[str]:
    """Write the A/B verdict as an uvts_runs row (V0016). Returns ab_run_id or None on failure."""
    if not HAS_TSDB:
        print("WARN: psycopg/cuid2 unavailable; skipping --persist-tsdb")
        return None
    try:
        from psycopg.types.json import Jsonb
        ab_run_id = _CUID()
        dsn = (
            f"host={os.environ.get('TSDB_HOST', 'localhost')} "
            f"port={os.environ.get('TSDB_PORT', '5433')} "
            f"user={os.environ.get('TSDB_USER', 'mdemg')} "
            f"password={os.environ.get('TSDB_PASSWORD', 'mdemg_metrics')} "
            f"dbname={os.environ.get('TSDB_DB', 'mdemg_metrics')}"
        )
        gate_verdict = verdict.get("verdict", "fail")
        if gate_verdict not in ("pass", "fail"):
            gate_verdict = "fail"  # 'error' collapses to 'fail' for the V0016 CHECK.
        with psycopg.connect(dsn, connect_timeout=5) as conn:
            with conn.cursor() as cur:
                cur.execute(
                    """INSERT INTO uvts_runs
                           (run_id, spec_path, branch_label, profile,
                            aggregate_score, gate_verdict, threshold_json,
                            ab_baseline_run_id, ab_candidate_run_id,
                            started_at, completed_at)
                       VALUES (%s, %s, %s, 'ab-compare', %s, %s, %s, %s, %s, NOW(), NOW())""",
                    (
                        ab_run_id, str(spec_path), branch_label,
                        verdict.get("candidate_mean", 0.0),
                        gate_verdict,
                        Jsonb({
                            "regression_threshold_per_question":
                                verdict.get("regression_threshold_per_question"),
                            "regression_count": verdict.get("regression_count"),
                            "improvement_count": verdict.get("improvement_count"),
                        }),
                        baseline_run_id, candidate_run_id,
                    ),
                )
            conn.commit()
        return ab_run_id
    except Exception as e:
        print(f"WARN: TSDB ab-row insert failed ({type(e).__name__}: {e})")
        return None


def main() -> int:
    parser = argparse.ArgumentParser(description="UVTS A/B Compare Harness (Phase 12 Epic 4)")
    parser.add_argument("--baseline", required=True,
                        help="Path to baseline grades.json (from uvts_runner.py --output-dir)")
    parser.add_argument("--candidate", required=True,
                        help="Path to candidate grades.json")
    parser.add_argument("--spec", required=True,
                        help="Path to the UVTS spec used for both runs (provides ab_mode.regression_threshold_per_question)")
    parser.add_argument("--out", required=True,
                        help="Path to write the structured A/B verdict JSON")
    parser.add_argument("--persist-tsdb", action="store_true",
                        help="Also write an A/B row to uvts_runs (V0016) with FK links to source runs")
    parser.add_argument("--baseline-run-id", default=None,
                        help="CUIDv2 of baseline run for FK link in --persist-tsdb mode")
    parser.add_argument("--candidate-run-id", default=None,
                        help="CUIDv2 of candidate run for FK link in --persist-tsdb mode")
    parser.add_argument("--branch-label", default=None,
                        help="Override branch label for the A/B row (default: 'A_vs_B_<short>')")
    args = parser.parse_args()

    # Validate inputs.
    for label, path in [("baseline", args.baseline), ("candidate", args.candidate),
                       ("spec", args.spec)]:
        if not Path(path).exists():
            print(f"ERROR: {label} file not found: {path}", file=sys.stderr)
            return 2

    # Load spec to pull the regression threshold.
    spec = json.load(open(args.spec))
    ab_mode = spec.get("ab_mode") or {}
    regression_threshold = float(
        ab_mode.get("regression_threshold_per_question", 0.10)
    )
    if regression_threshold < 0:
        print(f"ERROR: ab_mode.regression_threshold_per_question must be >= 0, got {regression_threshold}",
              file=sys.stderr)
        return 2

    baseline_summary, baseline_by_id = _load_grades(args.baseline)
    candidate_summary, candidate_by_id = _load_grades(args.candidate)

    verdict = compute_ab_verdict(
        baseline_summary, baseline_by_id,
        candidate_summary, candidate_by_id,
        regression_threshold,
    )

    # Persist verdict JSON.
    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    payload = {
        "framework": "uvts-ab",
        "framework_version": "1.0.0",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "spec_path": str(args.spec),
        "baseline_grades_path": str(args.baseline),
        "candidate_grades_path": str(args.candidate),
        "verdict": verdict,
    }
    with open(out_path, "w") as f:
        json.dump(payload, f, indent=2)
    print(f"\nA/B verdict written: {out_path}")
    print(f"  verdict: {verdict['verdict']}")
    print(f"  shared questions: {verdict.get('shared_question_count', 0)}")
    if verdict["verdict"] == "error":
        print(f"  error: {verdict.get('error')}")
    else:
        print(f"  baseline_mean: {verdict['baseline_mean']:.4f}")
        print(f"  candidate_mean: {verdict['candidate_mean']:.4f}")
        print(f"  mean_delta: {verdict['mean_delta']:+.4f}")
        print(f"  regression_count: {verdict['regression_count']} (threshold {regression_threshold})")
        print(f"  improvement_count: {verdict['improvement_count']}")

    # Optional TSDB row.
    if args.persist_tsdb:
        branch_label = args.branch_label
        if branch_label is None:
            ab_run_id_short = (args.baseline_run_id or "Aknown")[:6] + "_vs_" + \
                              (args.candidate_run_id or "Bknown")[:6]
            branch_label = f"A_vs_B_{ab_run_id_short}"
        ab_run_id = _persist_ab_row(
            spec_path=args.spec,
            verdict=verdict,
            baseline_run_id=args.baseline_run_id,
            candidate_run_id=args.candidate_run_id,
            branch_label=branch_label,
        )
        if ab_run_id:
            print(f"  TSDB persist: A/B run_id={ab_run_id} branch_label={branch_label}")

    if verdict["verdict"] == "pass":
        return 0
    elif verdict["verdict"] == "error":
        return 2
    else:
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
