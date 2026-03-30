"""Quality report for LLM interaction training data readiness.

Queries the llm_interactions TimescaleDB hypertable and produces a summary
report showing per-task row counts, quality annotation coverage, average
quality scores, quality_source breakdown, and training readiness assessment.

Usage:
    python -m training.quality_report \
        --tsdb-dsn "postgresql://mdemg:mdemg_metrics@localhost:5433/mdemg_metrics"
"""

from __future__ import annotations

import argparse
import logging
import sys

logger = logging.getLogger(__name__)

DEFAULT_DSN = "postgresql://mdemg:mdemg_metrics@localhost:5433/mdemg_metrics"

# Minimum number of quality-annotated rows for a task to be considered "ready"
READINESS_THRESHOLD = 500


def run_report(dsn: str) -> None:
    """Query llm_interactions and print the quality report.

    Connects to TimescaleDB, runs aggregation queries, and prints a
    formatted report covering:
    - Per-task row counts and quality coverage
    - Per-task average quality score
    - Per quality_source breakdown
    - Readiness assessment per task

    Args:
        dsn: PostgreSQL connection string for TimescaleDB.
    """
    import psycopg2

    logger.info("Connecting to TSDB: %s", _mask_dsn(dsn))
    conn = psycopg2.connect(dsn)
    try:
        cur = conn.cursor()

        # -------------------------------------------------------------------
        # 1. Per-task summary: row count, annotated count, avg quality
        # -------------------------------------------------------------------
        cur.execute(
            """
            SELECT
                task_name,
                COUNT(*) AS total_rows,
                COUNT(quality) AS annotated_rows,
                AVG(quality) AS avg_quality,
                MIN(quality) AS min_quality,
                MAX(quality) AS max_quality
            FROM llm_interactions
            GROUP BY task_name
            ORDER BY total_rows DESC
            """
        )
        task_rows = cur.fetchall()

        # -------------------------------------------------------------------
        # 2. Per quality_source breakdown
        # -------------------------------------------------------------------
        cur.execute(
            """
            SELECT
                quality_source,
                COUNT(*) AS row_count,
                AVG(quality) AS avg_quality
            FROM llm_interactions
            WHERE quality IS NOT NULL
            GROUP BY quality_source
            ORDER BY row_count DESC
            """
        )
        source_rows = cur.fetchall()

        # -------------------------------------------------------------------
        # 3. Global totals
        # -------------------------------------------------------------------
        cur.execute(
            """
            SELECT
                COUNT(*) AS total,
                COUNT(quality) AS annotated,
                AVG(quality) AS avg_quality
            FROM llm_interactions
            """
        )
        global_row = cur.fetchone()

    finally:
        conn.close()

    # -------------------------------------------------------------------
    # Print report
    # -------------------------------------------------------------------
    total_global = global_row[0] if global_row else 0
    annotated_global = global_row[1] if global_row else 0
    avg_global = global_row[2] if global_row else None

    print("\n" + "=" * 72)
    print("  MDEMG LLM Interaction Quality Report")
    print("=" * 72)

    print(f"\n  Total interactions:   {total_global:>8}")
    print(f"  With quality score:   {annotated_global:>8}")
    if total_global > 0:
        coverage_pct = (annotated_global / total_global) * 100
        print(f"  Coverage:             {coverage_pct:>7.1f}%")
    if avg_global is not None:
        print(f"  Global avg quality:   {avg_global:>8.3f}")

    # Per-task table
    print("\n" + "-" * 72)
    print(f"  {'Task':<30} {'Total':>7} {'Annot.':>7} {'Cov%':>6} {'AvgQ':>6} {'Ready':>7}")
    print("-" * 72)

    ready_tasks: list[str] = []
    not_ready_tasks: list[str] = []

    for row in task_rows:
        task_name = row[0]
        total = row[1]
        annotated = row[2]
        avg_q = row[3]
        cov = (annotated / total * 100) if total > 0 else 0.0
        avg_q_str = f"{avg_q:.3f}" if avg_q is not None else "  -  "
        is_ready = annotated >= READINESS_THRESHOLD
        ready_str = "  YES" if is_ready else "   no"

        if is_ready:
            ready_tasks.append(task_name)
        elif annotated > 0:
            not_ready_tasks.append(task_name)

        print(f"  {task_name:<30} {total:>7} {annotated:>7} {cov:>5.1f}% {avg_q_str:>6} {ready_str:>7}")

    # Per quality_source breakdown
    if source_rows:
        print("\n" + "-" * 72)
        print(f"  {'Quality Source':<30} {'Count':>7} {'AvgQ':>8}")
        print("-" * 72)

        for row in source_rows:
            source = row[0] if row[0] else "(NULL)"
            count = row[1]
            avg_q = row[2]
            avg_q_str = f"{avg_q:.3f}" if avg_q is not None else "   -  "
            print(f"  {source:<30} {count:>7} {avg_q_str:>8}")

    # Readiness assessment
    print("\n" + "-" * 72)
    print(f"  Readiness Assessment (threshold: >= {READINESS_THRESHOLD} annotated rows)")
    print("-" * 72)

    if ready_tasks:
        print(f"\n  READY for training ({len(ready_tasks)} tasks):")
        for t in ready_tasks:
            print(f"    + {t}")
    else:
        print("\n  No tasks are ready for training yet.")

    if not_ready_tasks:
        print(f"\n  NOT YET READY ({len(not_ready_tasks)} tasks with partial annotations):")
        for t in not_ready_tasks:
            print(f"    - {t}")

    pending_tasks = [
        task for task, rule in _get_pending_tasks()
    ]
    if pending_tasks:
        print(f"\n  PENDING quality signal ({len(pending_tasks)} tasks):")
        for t in pending_tasks:
            print(f"    ? {t}")

    print("\n" + "=" * 72 + "\n")


def _get_pending_tasks() -> list[tuple[str, str]]:
    """Return tasks from QUALITY_RULES that have source='pending'."""
    # Import here to avoid circular dependency and keep this module lightweight
    from .quality_annotator import QUALITY_RULES

    return [
        (task, rule["source"])
        for task, rule in QUALITY_RULES.items()
        if rule.get("source") == "pending"
    ]


def _mask_dsn(dsn: str) -> str:
    """Mask password in DSN for logging."""
    try:
        scheme_end = dsn.index("://") + 3
        at_pos = dsn.index("@", scheme_end)
        colon_pos = dsn.index(":", scheme_end)
        if colon_pos < at_pos:
            return dsn[:colon_pos + 1] + "****" + dsn[at_pos:]
    except ValueError:
        pass
    return dsn


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    """Parse command-line arguments."""
    parser = argparse.ArgumentParser(
        description="Quality report for LLM interaction training data readiness.",
    )
    parser.add_argument(
        "--tsdb-dsn",
        default=DEFAULT_DSN,
        help=f"TimescaleDB connection string (default: {DEFAULT_DSN})",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Enable debug logging",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    """CLI entrypoint for the quality report."""
    args = parse_args(argv)

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    try:
        run_report(dsn=args.tsdb_dsn)
    except Exception:
        logger.exception("Quality report failed")
        sys.exit(1)


if __name__ == "__main__":
    main()
