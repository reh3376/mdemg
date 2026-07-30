#!/usr/bin/env python3
"""Curate the labeled guidance corpus from guidance_training_rows (TSDB).

JIMINY-RELEVANCE-001 Epic 5. Reads the training EVIDENCE persisted by Epic 1
(the guidance text + action text + verdict that was previously discarded),
prefers human-certified GOLD labels (HITL-REVIEW-001's review_grades, when that
table exists) over the LLM/heuristic auto-labels, distribution-summarizes the
production guidance_type x outcome mix, optionally leak-audits the output, and
writes a versioned corpus artifact + manifest under training_data/.

This is the artifact that MATURES over the 3-6 month collection window; the
retrain FUTURE-TRIGGER (FT Phases 6/7/9 + FT-CLASSIFY-002) reads the manifest's
gold_fraction to decide when there is enough trustworthy data to retrain.

Usage:
  python3 scripts/curate_guidance_corpus.py --version v1 \
      [--space-id mdemg-dev] [--lookback-hours 4320] \
      [--min-label-quality any|real|gold] \
      [--against a.jsonl,b.jsonl] [--leak-max-overlap 0.5] \
      [--min-gold-fraction 0.0] [--out-dir training_data/guidance_corpus]
"""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

DEFAULT_TSDB_DSN = os.environ.get(
    "TSDB_DSN", "postgresql://mdemg:mdemg_metrics@localhost:5433/mdemg_metrics"
)

# Real (trustworthy) auto-label sources — the heuristic/blank class is noise
# (diagnostic Finding 4) and is excluded by --min-label-quality real|gold.
REAL_SOURCES = {"llm", "tier1", "explicit"}


def _mask_dsn(dsn: str) -> str:
    if "@" in dsn and "//" in dsn:
        head, tail = dsn.split("//", 1)
        if "@" in tail:
            return f"{head}//***@{tail.split('@', 1)[1]}"
    return dsn


def _table_exists(cur, name: str) -> bool:
    cur.execute("SELECT to_regclass(%s) IS NOT NULL", (name,))
    return bool(cur.fetchone()[0])


def _column_exists(cur, table: str, column: str) -> bool:
    cur.execute(
        "SELECT 1 FROM information_schema.columns "
        "WHERE table_name = %s AND column_name = %s LIMIT 1",
        (table, column),
    )
    return cur.fetchone() is not None


def fetch_rows(cur, space_id: str | None, lookback_hours: int,
               have_grades: bool, have_gold_outcome: bool):
    """Fetch evidence rows, LEFT JOINing review_grades when present.

    Two capabilities are decoupled:
      - have_grades: review_grades table exists → SME suggested_guidance is
        available (REVIEW-SUGGESTED-GUIDANCE-CONSUME-001).
      - have_gold_outcome: review_grades.gold_outcome column exists → gold
        verdict overrides the row's auto-label. The shipped schema stores
        the gold verdict in gold_dimensions JSONB instead of a scalar
        column, so this is typically False on production TSDB.
    """
    where = ["g.time > now() - (%(lb)s || ' hours')::interval"]
    params: dict = {"lb": str(lookback_hours)}
    if space_id:
        where.append("g.space_id = %(sid)s")
        params["sid"] = space_id
    where_sql = " AND ".join(where)

    gold_expr = "rg.gold_outcome" if have_gold_outcome else "NULL"

    if have_grades:
        sug_select = "rg.suggested_guidance, rg.grade_id"
        gold_select = "gold_outcome, " if have_gold_outcome else ""
        join = f"""
            LEFT JOIN LATERAL (
                SELECT {gold_select}suggested_guidance, grade_id
                FROM review_grades r
                WHERE r.dataset_id = 'guidance' AND r.item_id = g.row_id
                  AND r.reversed = false
                ORDER BY r.graded_at DESC LIMIT 1
            ) rg ON TRUE
        """
    else:
        sug_select = "NULL, NULL"
        join = ""

    sql = f"""
        SELECT g.row_id, g.space_id, g.guidance_type, g.guidance_content,
               g.action_summary, g.outcome_type, g.classifier_source,
               g.source_role_type, g.source_layer, g.similarity,
               {gold_expr} AS gold_outcome,
               {sug_select}
        FROM guidance_training_rows g
        {join}
        WHERE {where_sql}
        ORDER BY g.time DESC
    """
    cur.execute(sql, params)
    return cur.fetchall()


def main() -> int:
    ap = argparse.ArgumentParser(description="Curate the guidance training corpus from TSDB.")
    ap.add_argument("--version", default="v1", help="corpus artifact version dir")
    ap.add_argument("--tsdb-dsn", default=DEFAULT_TSDB_DSN)
    ap.add_argument("--space-id", default=os.environ.get("MDEMG_SPACE_ID", "mdemg-dev"))
    ap.add_argument("--lookback-hours", type=int, default=4320, help="default 180d")
    ap.add_argument("--min-label-quality", choices=["any", "real", "gold"], default="real",
                    help="any=include heuristic/blank; real=LLM/tier1/explicit/gold only; gold=human-graded only")
    ap.add_argument("--against", default="", help="comma-separated jsonls to leak-audit against")
    ap.add_argument("--leak-max-overlap", type=float,
                    default=float(os.environ.get("GUIDANCE_CORPUS_LEAK_MAX_OVERLAP", "0.5")))
    ap.add_argument("--min-gold-fraction", type=float,
                    default=float(os.environ.get("GUIDANCE_CORPUS_MIN_GOLD_FRACTION", "0.0")))
    ap.add_argument("--out-dir", default="training_data/guidance_corpus")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--min-suggestion-length", type=int,
                    default=int(os.environ.get("GUIDANCE_CORPUS_MIN_SUGGESTION_LENGTH", "40")),
                    help="Min char length for review_grades.suggested_guidance "
                         "to emit as a synthetic corpus row (filters triage notes). "
                         "Set 0 to disable.")
    args = ap.parse_args()

    try:
        import psycopg2
    except ImportError:
        print("ERROR: psycopg2 not installed (pip install psycopg2-binary)", file=sys.stderr)
        return 2

    conn = psycopg2.connect(args.tsdb_dsn)
    try:
        cur = conn.cursor()
        have_grades = _table_exists(cur, "review_grades")
        # The gold verdict is stored in gold_dimensions (JSONB) on the shipped
        # schema, not in a scalar gold_outcome column. Check for the column
        # separately so the SME-suggestion path (which needs only review_grades)
        # keeps working when the gold-outcome column is absent.
        have_gold_outcome = have_grades and _column_exists(
            cur, "review_grades", "gold_outcome"
        )
        rows = fetch_rows(cur, args.space_id, args.lookback_hours,
                          have_grades, have_gold_outcome)
    finally:
        conn.close()

    # Build corpus records with label-source preference (gold > row auto-label).
    records = []
    label_breakdown: dict[str, int] = {}
    gtype_outcome: dict[str, int] = {}
    gtype_dist: dict[str, int] = {}
    outcome_dist: dict[str, int] = {}
    gold_count = 0
    sme_suggestions_included = 0
    sme_suggestions_skipped_short = 0
    min_sug_len = args.min_suggestion_length
    for (row_id, space_id, gtype, gcontent, action, outcome, clsfr,
         role, layer, sim, gold_outcome, suggested_guidance, grade_id) in rows:
        gtype = gtype or ""
        clsfr = clsfr or ""
        if gold_outcome:
            label_source = "gold"
            final_outcome = gold_outcome
            gold_count += 1
        else:
            label_source = clsfr if clsfr else "blank"
            final_outcome = outcome
        # Quality gate.
        if args.min_label_quality == "gold" and label_source != "gold":
            continue
        if args.min_label_quality == "real" and label_source not in REAL_SOURCES and label_source != "gold":
            continue
        if not (gcontent or "").strip():
            continue
        rec = {
            "row_id": row_id,
            "space_id": space_id,
            "guidance_type": gtype,
            "guidance_content": gcontent,
            "action_summary": action or "",
            "outcome": final_outcome,
            "label_source": label_source,
            "source_role_type": role or "",
            "source_layer": layer,
            "similarity": sim,
        }
        records.append(rec)
        label_breakdown[label_source] = label_breakdown.get(label_source, 0) + 1
        gtype_dist[gtype] = gtype_dist.get(gtype, 0) + 1
        outcome_dist[final_outcome] = outcome_dist.get(final_outcome, 0) + 1
        key = f"{gtype}|{final_outcome}"
        gtype_outcome[key] = gtype_outcome.get(key, 0) + 1

        # REVIEW-SUGGESTED-GUIDANCE-CONSUME-001: emit an additional SYNTHETIC
        # corpus row when the SME authored a "what would have been better
        # guidance" text alongside their grade. Length-gate rejects operator
        # triage notes (e.g. "This was a duplicate entry"). Set --min-suggestion-length
        # 0 to disable the gate.
        sug = (suggested_guidance or "").strip()
        if sug:
            if min_sug_len > 0 and len(sug) < min_sug_len:
                sme_suggestions_skipped_short += 1
            else:
                sme_rec = {
                    "row_id": f"{row_id}::sme_sug",
                    "space_id": space_id,
                    "guidance_type": gtype,
                    "guidance_content": sug,
                    "action_summary": action or "",
                    "outcome": "followed",
                    "label_source": "sme_suggestion",
                    "source_role_type": role or "",
                    "source_layer": layer,
                    "similarity": sim,
                    "sme_source_grade_id": grade_id,
                    "sme_source_row_id": row_id,
                }
                records.append(sme_rec)
                sme_suggestions_included += 1
                label_breakdown["sme_suggestion"] = label_breakdown.get("sme_suggestion", 0) + 1
                gtype_dist[gtype] = gtype_dist.get(gtype, 0) + 1
                outcome_dist["followed"] = outcome_dist.get("followed", 0) + 1
                sme_key = f"{gtype}|followed"
                gtype_outcome[sme_key] = gtype_outcome.get(sme_key, 0) + 1

    total = len(records)
    gold_fraction = (gold_count / total) if total else 0.0

    out_dir = Path(args.out_dir) / args.version
    corpus_path = out_dir / "corpus.jsonl"
    manifest_path = out_dir / "manifest.json"

    # Leak audit (optional) — reuse the canonical audit script against the
    # written corpus (distribution-preserving; we do not rebalance by default).
    leak_report: dict = {"ran": False}
    if not args.dry_run:
        out_dir.mkdir(parents=True, exist_ok=True)
        with corpus_path.open("w") as f:
            for r in records:
                f.write(json.dumps(r) + "\n")
        if args.against.strip():
            leak_out = out_dir / "leak_audit.json"
            cmd = [sys.executable, "scripts/audit_eval_leakage.py",
                   "--eval", str(corpus_path), "--against", args.against,
                   "--out", str(leak_out)]
            rc = subprocess.run(cmd, capture_output=True, text=True)
            leak_report = {"ran": True, "exit_code": rc.returncode,
                           "report": str(leak_out), "stderr": rc.stderr[-500:]}

    manifest = {
        "version": args.version,
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "source_table": "guidance_training_rows",
        "tsdb_dsn": _mask_dsn(args.tsdb_dsn),
        "space_id": args.space_id,
        "lookback_hours": args.lookback_hours,
        "min_label_quality": args.min_label_quality,
        "total_rows": total,
        "gold_available": have_gold_outcome,
        "grades_available": have_grades,
        "gold_fraction": round(gold_fraction, 4),
        "min_gold_fraction_target": args.min_gold_fraction,
        "label_source_breakdown": label_breakdown,
        "guidance_type_distribution": gtype_dist,
        "outcome_distribution": outcome_dist,
        "guidance_type_x_outcome": gtype_outcome,
        "sme_suggestions_included": sme_suggestions_included,
        "sme_suggestions_skipped_short": sme_suggestions_skipped_short,
        "min_suggestion_length": args.min_suggestion_length,
        "leak_audit": leak_report,
        "corpus_path": str(corpus_path),
        "dry_run": args.dry_run,
    }
    if not args.dry_run:
        with manifest_path.open("w") as f:
            json.dump(manifest, f, indent=2)

    print(json.dumps(manifest, indent=2))
    # Gates (informational; the retrain trigger reads these).
    if gold_fraction < args.min_gold_fraction:
        print(f"NOTE: gold_fraction {gold_fraction:.3f} < target {args.min_gold_fraction} "
              f"(corpus still maturing — expected early in the 3-6 month window)", file=sys.stderr)
    if leak_report.get("ran") and leak_report.get("exit_code", 0) != 0:
        print("WARNING: leak audit reported overlap — inspect the leak report", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
