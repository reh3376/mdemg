"""Post-hoc quality annotation pipeline for LLM interaction records.

Reads protocol JSONL records produced by protocol_data_collector.go,
joins them with llm_interactions rows via guidance_id, computes quality
scores based on task-specific rules, and updates the quality/quality_source
columns in the llm_interactions TimescaleDB hypertable.

Usage:
    python -m training.quality_annotator \
        --tsdb-dsn "postgresql://mdemg:mdemg_metrics@localhost:5433/mdemg_metrics" \
        --jsonl-dir .mdemg/neural/training-data/ \
        --dry-run \
        --task jiminy.synthesize
"""

from __future__ import annotations

import argparse
import glob
import json
import logging
import os
import sys
from dataclasses import dataclass, field
from typing import Any

logger = logging.getLogger(__name__)

DEFAULT_DSN = "postgresql://mdemg:mdemg_metrics@localhost:5433/mdemg_metrics"
DEFAULT_JSONL_DIR = ".mdemg/neural/training-data"

# ---------------------------------------------------------------------------
# Quality scoring rules per task
# ---------------------------------------------------------------------------
QUALITY_RULES: dict[str, dict[str, Any]] = {
    "jiminy.synthesize": {
        "source": "feedback_outcome",
        "scoring": {"followed": 1.0, "ignored": 0.3, "contradicted": 0.0},
    },
    "jiminy.evaluate": {
        "source": "feedback_outcome",
        "scoring": {"use_comprehension": True},  # quality = comprehension_score
    },
    "consulting.classify": {
        "source": "deterministic",
        "scoring": {"json_valid": True, "confidence_threshold": 0.7},
    },
    "retrieval.rerank_cross": {
        "source": "rerank_scores",
    },
    # Tasks with no quality signal yet
    "ape.reflect": {"source": "pending"},
    "hidden.summarize": {"source": "pending"},
    "hidden.name_emergence": {"source": "pending"},
    "hidden.reclassify": {"source": "pending"},
    "metalearn.generalize": {"source": "pending"},
    "summarize.generate": {"source": "pending"},
}


@dataclass
class ProtocolRecord:
    """A parsed protocol JSONL record relevant to quality annotation."""

    guidance_id: str
    outcome: str
    comprehension_score: float = 0.0
    trust_score: float = 0.5
    applicability_score: float = 0.0
    source_file: str = ""
    line_num: int = 0


@dataclass
class AnnotationResult:
    """Result of annotating a single llm_interactions row."""

    guidance_id: str
    task_name: str
    quality: float
    quality_source: str


@dataclass
class AnnotationSummary:
    """Summary statistics from an annotation run."""

    total_jsonl_records: int = 0
    records_with_guidance_id: int = 0
    matched_interactions: int = 0
    annotated: int = 0
    skipped_pending: int = 0
    skipped_no_rule: int = 0
    errors: int = 0
    per_task: dict[str, int] = field(default_factory=dict)


def load_protocol_records(jsonl_dir: str) -> list[ProtocolRecord]:
    """Load protocol JSONL files and extract records that have a guidance_id and outcome.

    Scans all protocol-*.jsonl files in the given directory. Records missing
    guidance_id or outcome are silently skipped since they cannot participate
    in quality annotation.
    """
    records: list[ProtocolRecord] = []
    pattern = os.path.join(jsonl_dir, "protocol-*.jsonl")
    files = sorted(glob.glob(pattern))

    if not files:
        logger.warning("No protocol JSONL files found in %s (pattern: %s)", jsonl_dir, pattern)
        return records

    logger.info("Scanning %d JSONL files in %s", len(files), jsonl_dir)

    for filepath in files:
        with open(filepath, encoding="utf-8") as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line:
                    continue
                try:
                    obj = json.loads(line)
                except json.JSONDecodeError as exc:
                    logger.warning("Malformed JSON at %s:%d: %s", filepath, line_num, exc)
                    continue

                gid = obj.get("guidance_id", "")
                outcome = obj.get("outcome", "")
                if not gid or not outcome:
                    continue

                records.append(
                    ProtocolRecord(
                        guidance_id=gid,
                        outcome=outcome,
                        comprehension_score=float(obj.get("comprehension_score", 0.0)),
                        trust_score=float(obj.get("trust_score", 0.5)),
                        applicability_score=float(obj.get("applicability_score", 0.0)),
                        source_file=filepath,
                        line_num=line_num,
                    )
                )

    logger.info(
        "Loaded %d annotatable records (with guidance_id + outcome) from %d files",
        len(records),
        len(files),
    )
    return records


def compute_quality_synthesize(record: ProtocolRecord, scoring: dict[str, Any]) -> float | None:
    """Compute quality for jiminy.synthesize based on outcome mapping."""
    return scoring.get(record.outcome)


def compute_quality_evaluate(record: ProtocolRecord, scoring: dict[str, Any]) -> float | None:
    """Compute quality for jiminy.evaluate using comprehension_score."""
    if scoring.get("use_comprehension"):
        score = record.comprehension_score
        # Clamp to [0.0, 1.0]
        return max(0.0, min(1.0, score))
    return None


def compute_quality_classify(record: ProtocolRecord, scoring: dict[str, Any]) -> float | None:
    """Compute quality for consulting.classify using deterministic validation.

    The outcome field is expected to be a JSON string. Quality is 1.0 if:
    - The outcome parses as valid JSON, AND
    - If a 'confidence' key is present, its value >= confidence_threshold
    Otherwise quality is 0.0.
    """
    try:
        parsed = json.loads(record.outcome)
    except (json.JSONDecodeError, TypeError):
        return 0.0

    if not isinstance(parsed, dict):
        return 0.0

    threshold = scoring.get("confidence_threshold", 0.7)
    if "confidence" in parsed:
        try:
            confidence = float(parsed["confidence"])
            return 1.0 if confidence >= threshold else 0.0
        except (ValueError, TypeError):
            return 0.0

    return 1.0


def compute_quality_rerank(record: ProtocolRecord) -> float | None:
    """Compute quality for retrieval.rerank_cross using applicability_score.

    The applicability_score from the protocol record serves as the relevance
    quality signal for reranking tasks. Clamp to [0.0, 1.0].
    """
    score = record.applicability_score
    return max(0.0, min(1.0, score))


def compute_quality(
    task_name: str,
    record: ProtocolRecord,
) -> tuple[float | None, str]:
    """Compute quality score for a given task and protocol record.

    Returns (quality, quality_source) tuple. quality is None if the task
    has no scoring rule or the rule is 'pending'.
    """
    rule = QUALITY_RULES.get(task_name)
    if rule is None:
        return None, ""

    source = rule["source"]
    if source == "pending":
        return None, ""

    scoring = rule.get("scoring", {})

    if task_name == "jiminy.synthesize":
        quality = compute_quality_synthesize(record, scoring)
        return quality, "feedback_outcome"

    if task_name == "jiminy.evaluate":
        quality = compute_quality_evaluate(record, scoring)
        return quality, "feedback_outcome"

    if task_name == "consulting.classify":
        quality = compute_quality_classify(record, scoring)
        return quality, "deterministic"

    if task_name == "retrieval.rerank_cross":
        quality = compute_quality_rerank(record)
        return quality, "rerank_scores"

    return None, ""


def run_annotation(
    dsn: str,
    jsonl_dir: str,
    dry_run: bool = False,
    task_filter: str | None = None,
) -> AnnotationSummary:
    """Run the quality annotation pipeline.

    1. Load protocol JSONL records with guidance_id + outcome
    2. Look up matching llm_interactions rows in TSDB
    3. Compute quality per task's rules
    4. UPDATE quality and quality_source (or print if dry_run)
    5. Return summary

    Args:
        dsn: PostgreSQL connection string for TimescaleDB.
        jsonl_dir: Directory containing protocol-*.jsonl files.
        dry_run: If True, print annotations without writing to TSDB.
        task_filter: If set, only annotate rows for this task_name.
    """
    import psycopg2  # noqa: E402 — delay import, not needed if --help

    summary = AnnotationSummary()

    # Step 1: Load protocol records
    all_records = load_protocol_records(jsonl_dir)
    summary.total_jsonl_records = len(all_records)

    if not all_records:
        logger.warning("No annotatable protocol records found. Nothing to do.")
        return summary

    # Build guidance_id -> record lookup
    gid_map: dict[str, ProtocolRecord] = {}
    for rec in all_records:
        gid_map[rec.guidance_id] = rec
    summary.records_with_guidance_id = len(gid_map)

    # Step 2: Query matching llm_interactions
    logger.info("Connecting to TSDB: %s", _mask_dsn(dsn))
    conn = psycopg2.connect(dsn)
    try:
        conn.autocommit = False
        cur = conn.cursor()

        # Build the WHERE clause
        gids = list(gid_map.keys())
        where_parts = ["guidance_id = ANY(%s)"]
        params: list[Any] = [gids]

        if task_filter:
            where_parts.append("task_name = %s")
            params.append(task_filter)

        query = (
            "SELECT guidance_id, task_name FROM llm_interactions WHERE "
            + " AND ".join(where_parts)
        )
        cur.execute(query, params)
        rows = cur.fetchall()
        summary.matched_interactions = len(rows)

        if not rows:
            logger.warning("No matching llm_interactions rows found for %d guidance_ids.", len(gids))
            return summary

        logger.info("Found %d matching llm_interactions rows", len(rows))

        # Step 3+4: Compute quality and apply updates
        annotations: list[AnnotationResult] = []
        for gid, task_name in rows:
            protocol_rec = gid_map.get(gid)
            if protocol_rec is None:
                continue

            quality, quality_source = compute_quality(task_name, protocol_rec)

            if quality is None:
                if QUALITY_RULES.get(task_name, {}).get("source") == "pending":
                    summary.skipped_pending += 1
                else:
                    summary.skipped_no_rule += 1
                continue

            annotations.append(
                AnnotationResult(
                    guidance_id=gid,
                    task_name=task_name,
                    quality=quality,
                    quality_source=quality_source,
                )
            )

        if not annotations:
            logger.info("No annotations to apply (all tasks pending or unrecognized).")
            return summary

        # Apply updates
        if dry_run:
            logger.info("DRY RUN — would annotate %d rows:", len(annotations))
            for ann in annotations:
                logger.info(
                    "  guidance_id=%s task=%s quality=%.3f source=%s",
                    ann.guidance_id,
                    ann.task_name,
                    ann.quality,
                    ann.quality_source,
                )
                summary.annotated += 1
                summary.per_task[ann.task_name] = summary.per_task.get(ann.task_name, 0) + 1
        else:
            update_sql = (
                "UPDATE llm_interactions "
                "SET quality = %s, quality_source = %s "
                "WHERE guidance_id = %s"
            )
            for ann in annotations:
                try:
                    cur.execute(update_sql, (ann.quality, ann.quality_source, ann.guidance_id))
                    summary.annotated += 1
                    summary.per_task[ann.task_name] = summary.per_task.get(ann.task_name, 0) + 1
                except Exception:
                    logger.exception("Failed to update guidance_id=%s", ann.guidance_id)
                    summary.errors += 1

            conn.commit()
            logger.info("Committed %d quality annotations to TSDB.", summary.annotated)

    except Exception:
        logger.exception("Annotation pipeline failed")
        conn.rollback()
        raise
    finally:
        conn.close()

    return summary


def _mask_dsn(dsn: str) -> str:
    """Mask password in DSN for logging."""
    # Simple masking: replace password between : and @ after ://
    try:
        scheme_end = dsn.index("://") + 3
        at_pos = dsn.index("@", scheme_end)
        colon_pos = dsn.index(":", scheme_end)
        if colon_pos < at_pos:
            return dsn[:colon_pos + 1] + "****" + dsn[at_pos:]
    except ValueError:
        pass
    return dsn


def print_summary(summary: AnnotationSummary) -> None:
    """Print a human-readable summary of the annotation run."""
    print("\n=== Quality Annotation Summary ===")
    print(f"  JSONL records loaded:        {summary.total_jsonl_records}")
    print(f"  Unique guidance_ids:         {summary.records_with_guidance_id}")
    print(f"  Matched interactions in DB:  {summary.matched_interactions}")
    print(f"  Annotated:                   {summary.annotated}")
    print(f"  Skipped (pending rule):      {summary.skipped_pending}")
    print(f"  Skipped (no rule):           {summary.skipped_no_rule}")
    print(f"  Errors:                      {summary.errors}")

    if summary.per_task:
        print("\n  Per-task breakdown:")
        for task, count in sorted(summary.per_task.items()):
            print(f"    {task}: {count}")
    print()


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    """Parse command-line arguments."""
    parser = argparse.ArgumentParser(
        description="Post-hoc quality annotation for LLM interaction records.",
    )
    parser.add_argument(
        "--tsdb-dsn",
        default=DEFAULT_DSN,
        help=f"TimescaleDB connection string (default: {DEFAULT_DSN})",
    )
    parser.add_argument(
        "--jsonl-dir",
        default=DEFAULT_JSONL_DIR,
        help=f"Directory containing protocol-*.jsonl files (default: {DEFAULT_JSONL_DIR})",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print annotations without writing to TSDB",
    )
    parser.add_argument(
        "--task",
        default=None,
        dest="task_filter",
        help="Annotate only this task_name (e.g. jiminy.synthesize)",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Enable debug logging",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    """CLI entrypoint for the quality annotation pipeline."""
    args = parse_args(argv)

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    summary = run_annotation(
        dsn=args.tsdb_dsn,
        jsonl_dir=args.jsonl_dir,
        dry_run=args.dry_run,
        task_filter=args.task_filter,
    )

    print_summary(summary)

    if summary.errors > 0:
        sys.exit(1)


if __name__ == "__main__":
    main()
