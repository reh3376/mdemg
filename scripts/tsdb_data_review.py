#!/usr/bin/env python3
"""MDEMG TSDB Data Governance Report.

Connects to TimescaleDB and produces a comprehensive data quality
report across all TSDB tables. Identifies structural issues, data
quality gaps, and training data readiness.

Usage:
    python scripts/tsdb_data_review.py
    python scripts/tsdb_data_review.py --format both --output report.json
    python scripts/tsdb_data_review.py --tsdb-dsn "postgresql://user:pass@host:port/db"
"""

from __future__ import annotations

import argparse
import json
import logging
import re
import sys
import urllib.request
from dataclasses import dataclass, field
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any

logger = logging.getLogger(__name__)

DEFAULT_DSN = "postgresql://mdemg:mdemg_metrics@localhost:5433/mdemg_metrics"
DEFAULT_MDEMG_URL = "http://localhost:9999"

# Privacy scrub patterns (must mirror internal/llmclient/scrubber.go exactly)
# Application order: apiKey -> absPath -> envSecret -> email -> neo4j
PRIVACY_PATTERNS = {
    "api_key": re.compile(
        r"(?i)(sk-[a-zA-Z0-9]{20,}|ghp_[a-zA-Z0-9]{36}|AKIA[A-Z0-9]{16}|Bearer\s+[a-zA-Z0-9._\-]{20,})"
    ),
    "abs_path": re.compile(r"(?:/Users/[^\s\"']+|/home/[^\s\"']+|[A-Z]:\\\\Users\\\\[^\s\"']+)"),
    "env_secret": re.compile(r"(?i)(PASSWORD|SECRET|TOKEN|API_KEY|PRIVATE_KEY)\s*[=:]\s*[\"']?([^\s\"',;]+)[\"']?"),
    "email": re.compile(r"[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}"),
    "neo4j_cred": re.compile(r"neo4j://[^:]+:[^@]+@"),
}

EXPECTED_SCHEMA_VERSION = 7
EXPECTED_TABLES = [
    "metric_samples", "llm_interactions", "embedding_events", "retrieval_events",
    "ft_benchmarks", "ft_training_cycles", "ft_model_versions", "ft_hitl_decisions",
]
EXPECTED_TASK_NAMES = [
    "ape.reflect", "consulting.classify", "consulting.synthesis",
    "hidden.summarize", "hidden.name_emergence", "hidden.reclassify",
    "jiminy.evaluate", "jiminy.evaluate_llm", "jiminy.outcome",
    "jiminy.synthesize", "jiminy.codegen",
    "metalearn.generalize",
    "retrieval.intent_translate", "retrieval.query_classify",
    "retrieval.rerank_cross", "retrieval.rerank_nli",
    "summarize.generate",
]
EXPECTED_CALL_SITES = [
    "conversation", "jiminy.guide", "jiminy.evaluate", "jiminy.outcome",
    "consult.suggest", "ingest", "retrieve", "conversation.recall",
    "scraper.dedup", "metalearn", "batch_ingest", "reflect", "module_ingest",
]


@dataclass
class Finding:
    level: str  # "critical", "warning", "info"
    section: str
    message: str


@dataclass
class SectionResult:
    name: str
    data: dict[str, Any] = field(default_factory=dict)
    findings: list[Finding] = field(default_factory=list)


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


def pct(part: int | float | None, total: int | float | None) -> float:
    """Safe percentage calculation."""
    if not part or not total:
        return 0.0
    return round(100.0 * float(part) / float(total), 1)


def _to_serializable(obj: Any) -> Any:
    """Convert non-JSON-serializable types."""
    if isinstance(obj, Decimal):
        return float(obj)
    if isinstance(obj, datetime):
        return obj.isoformat()
    if isinstance(obj, Finding):
        return {"level": obj.level, "section": obj.section, "message": obj.message}
    if isinstance(obj, SectionResult):
        return {"name": obj.name, "data": obj.data, "findings": [_to_serializable(f) for f in obj.findings]}
    return obj


class JSONEncoder(json.JSONEncoder):
    def default(self, o: Any) -> Any:
        result = _to_serializable(o)
        if result is not o:
            return result
        return super().default(o)


# ─── Section A: Schema Health ───

def section_a_schema_health(cur: Any, verbose: bool) -> SectionResult:
    result = SectionResult(name="schema_health")
    findings = result.findings

    # Schema version
    cur.execute("SELECT value FROM tsdb_schema_meta WHERE key = 'schema_version'")
    row = cur.fetchone()
    schema_version = int(row[0]) if row else None
    result.data["schema_version"] = schema_version
    if schema_version != EXPECTED_SCHEMA_VERSION:
        findings.append(Finding("critical", "schema_health", f"Schema version is {schema_version}, expected {EXPECTED_SCHEMA_VERSION}"))

    # Table list
    cur.execute("SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename")
    tables = [r[0] for r in cur.fetchall()]
    result.data["tables_found"] = tables
    missing = [t for t in EXPECTED_TABLES if t not in tables]
    result.data["tables_missing"] = missing
    result.data["tables_present"] = len(EXPECTED_TABLES) - len(missing)
    result.data["tables_expected"] = len(EXPECTED_TABLES)
    if missing:
        findings.append(Finding("critical", "schema_health", f"Missing tables: {', '.join(missing)}"))

    # Indexes
    cur.execute("SELECT tablename, indexname FROM pg_indexes WHERE schemaname = 'public' ORDER BY tablename, indexname")
    indexes = [(r[0], r[1]) for r in cur.fetchall()]
    result.data["index_count"] = len(indexes)
    if verbose:
        result.data["indexes"] = [{"table": t, "index": i} for t, i in indexes]

    # Continuous aggregates
    try:
        cur.execute("""
            SELECT view_name FROM timescaledb_information.continuous_aggregates
        """)
        ca_views = [r[0] for r in cur.fetchall()]
        result.data["continuous_aggregates"] = ca_views
        expected_ca = ["metrics_hourly", "metrics_daily"]
        missing_ca = [c for c in expected_ca if c not in ca_views]
        if missing_ca:
            findings.append(Finding("warning", "schema_health", f"Missing continuous aggregates: {', '.join(missing_ca)}"))
    except Exception as e:
        logger.warning("Could not query continuous aggregates: %s", e)
        result.data["continuous_aggregates"] = []

    return result


# ─── Section B: metric_samples ───

def section_b_metric_samples(cur: Any, verbose: bool) -> SectionResult:
    result = SectionResult(name="metric_samples")
    findings = result.findings

    # Overview
    cur.execute("SELECT count(*), min(time), max(time) FROM metric_samples")
    row = cur.fetchone()
    total, min_time, max_time = row[0], row[1], row[2]
    result.data["total_rows"] = total
    result.data["min_time"] = min_time
    result.data["max_time"] = max_time

    if total == 0:
        findings.append(Finding("critical", "metric_samples", "Table is empty"))
        return result

    # Duration
    if min_time and max_time:
        duration_hours = (max_time - min_time).total_seconds() / 3600
        result.data["duration_hours"] = round(duration_hours, 1)

    # Daily row counts
    cur.execute("""
        SELECT time_bucket('1 day', time) AS day, count(*)
        FROM metric_samples GROUP BY day ORDER BY day
    """)
    daily = [(r[0], r[1]) for r in cur.fetchall()]
    result.data["daily_counts"] = [{"day": d.isoformat() if d else None, "count": c} for d, c in daily]
    zero_days = [d for d, c in daily if c == 0]
    if zero_days:
        findings.append(Finding("warning", "metric_samples", f"{len(zero_days)} days with zero rows (collection gap)"))

    # Metric catalog
    cur.execute("""
        SELECT metric_name, count(*) as cnt,
               min(value), max(value), avg(value),
               max(time) as last_seen
        FROM metric_samples
        GROUP BY metric_name ORDER BY cnt DESC
    """)
    catalog = []
    for r in cur.fetchall():
        catalog.append({
            "metric_name": r[0], "count": r[1],
            "min": float(r[2]) if r[2] is not None else None,
            "max": float(r[3]) if r[3] is not None else None,
            "avg": float(r[4]) if r[4] is not None else None,
            "last_seen": r[5],
        })
    result.data["distinct_metrics"] = len(catalog)
    result.data["metric_catalog"] = catalog if verbose else [{"metric_name": c["metric_name"], "count": c["count"]} for c in catalog]

    # Prefix distribution
    cur.execute("""
        SELECT split_part(metric_name, '_', 2) as prefix,
               count(DISTINCT metric_name) as metrics,
               count(*) as rows
        FROM metric_samples GROUP BY prefix ORDER BY rows DESC
    """)
    result.data["prefix_distribution"] = [{"prefix": r[0], "metrics": r[1], "rows": r[2]} for r in cur.fetchall()]

    # Source distribution
    cur.execute("SELECT source, count(*) FROM metric_samples GROUP BY source")
    result.data["source_distribution"] = {r[0]: r[1] for r in cur.fetchall()}

    # Quality tag distribution
    cur.execute("SELECT quality_tag, count(*) FROM metric_samples GROUP BY quality_tag")
    result.data["quality_tag_distribution"] = {r[0]: r[1] for r in cur.fetchall()}

    # Labels JSONB usage
    cur.execute("""
        SELECT count(*) as total,
               count(CASE WHEN labels IS NOT NULL AND labels != '{}' AND labels != 'null' THEN 1 END) as has_labels
        FROM metric_samples
    """)
    row = cur.fetchone()
    result.data["labels_total"] = row[0]
    result.data["labels_populated"] = row[1]

    # Space distribution
    cur.execute("SELECT space_id, count(*) FROM metric_samples GROUP BY space_id")
    result.data["space_distribution"] = {r[0]: r[1] for r in cur.fetchall()}

    # Continuous aggregate row counts
    try:
        cur.execute("SELECT count(*) FROM metrics_hourly")
        result.data["metrics_hourly_rows"] = cur.fetchone()[0]
        cur.execute("SELECT count(*) FROM metrics_daily")
        result.data["metrics_daily_rows"] = cur.fetchone()[0]
    except Exception as e:
        logger.warning("Could not query continuous aggregates: %s", e)
        result.data["metrics_hourly_rows"] = None
        result.data["metrics_daily_rows"] = None

    # Near-duplicate metric name detection
    names = [c["metric_name"] for c in catalog]
    duplicates = _find_near_duplicate_names(names)
    if duplicates:
        result.data["near_duplicates"] = duplicates
        findings.append(Finding("info", "metric_samples", f"{len(duplicates)} near-duplicate metric name pairs detected"))

    return result


def _find_near_duplicate_names(names: list[str]) -> list[tuple[str, str]]:
    """Find metric names that differ only by suffix."""
    dupes = []
    for i, a in enumerate(names):
        parts_a = a.rsplit("_", 1)
        if len(parts_a) != 2:
            continue
        prefix_a = parts_a[0]
        for b in names[i + 1:]:
            parts_b = b.rsplit("_", 1)
            if len(parts_b) == 2 and parts_b[0] == prefix_a and parts_a[1] != parts_b[1]:
                dupes.append((a, b))
    return dupes


# ─── Section C: llm_interactions ───

def section_c_llm_interactions(cur: Any, verbose: bool) -> SectionResult:
    result = SectionResult(name="llm_interactions")
    findings = result.findings

    # Overview
    cur.execute("SELECT count(*), min(time), max(time) FROM llm_interactions")
    row = cur.fetchone()
    total, min_time, max_time = row[0], row[1], row[2]
    result.data["total_rows"] = total
    result.data["min_time"] = min_time
    result.data["max_time"] = max_time

    if total == 0:
        findings.append(Finding("warning", "llm_interactions", "Table is empty"))
        return result

    # Task name distribution
    cur.execute("SELECT task_name, count(*) FROM llm_interactions GROUP BY task_name ORDER BY count(*) DESC")
    task_dist = {r[0]: r[1] for r in cur.fetchall()}
    result.data["task_distribution"] = task_dist
    result.data["task_names_producing"] = len(task_dist)

    # Compare against expected
    missing_tasks = [t for t in EXPECTED_TASK_NAMES if t not in task_dist]
    result.data["missing_task_names"] = missing_tasks
    if missing_tasks:
        findings.append(Finding("info", "llm_interactions",
                                f"{len(missing_tasks)}/{len(EXPECTED_TASK_NAMES)} task_names never produced records"))

    # Provider/model distribution
    cur.execute("""
        SELECT provider, model_name, count(*)
        FROM llm_interactions GROUP BY provider, model_name ORDER BY count(*) DESC
    """)
    result.data["provider_model_distribution"] = [
        {"provider": r[0], "model": r[1], "count": r[2]} for r in cur.fetchall()
    ]

    # Error rate
    cur.execute("""
        SELECT count(*) as total,
               count(CASE WHEN error != '' AND error IS NOT NULL THEN 1 END) as errors
        FROM llm_interactions
    """)
    row = cur.fetchone()
    error_total, error_count = row[0], row[1]
    error_rate = pct(error_count, error_total)
    result.data["error_count"] = error_count
    result.data["error_rate_pct"] = error_rate
    if error_rate > 5.0:
        findings.append(Finding("warning", "llm_interactions", f"Error rate is {error_rate}% (>{5}% threshold)"))

    # Column population analysis
    cur.execute("""
        SELECT
            count(*) as total,
            count(CASE WHEN task_name = '' OR task_name IS NULL THEN 1 END) as empty_task_name,
            count(CASE WHEN system_prompt = '' OR system_prompt IS NULL THEN 1 END) as empty_system_prompt,
            count(CASE WHEN user_prompt = '' OR user_prompt IS NULL THEN 1 END) as empty_user_prompt,
            count(CASE WHEN response = '' OR response IS NULL THEN 1 END) as empty_response,
            count(CASE WHEN model_name = '' OR model_name IS NULL THEN 1 END) as empty_model_name,
            count(CASE WHEN provider = '' OR provider IS NULL THEN 1 END) as empty_provider,
            count(CASE WHEN system_prompt_hash = '' OR system_prompt_hash IS NULL THEN 1 END) as empty_prompt_hash,
            count(CASE WHEN guidance_id IS NOT NULL AND guidance_id != '' THEN 1 END) as has_guidance_id,
            count(CASE WHEN source_path IS NOT NULL AND source_path != '' THEN 1 END) as has_source_path,
            count(CASE WHEN retrieval_node_ids IS NOT NULL AND array_length(retrieval_node_ids, 1) > 0 THEN 1 END) as has_raft_context,
            count(CASE WHEN think_content IS NOT NULL AND think_content != '' THEN 1 END) as has_think_content,
            count(CASE WHEN quality IS NOT NULL THEN 1 END) as has_quality,
            count(CASE WHEN used_for_train = true THEN 1 END) as used_for_train
        FROM llm_interactions
    """)
    row = cur.fetchone()
    col_pop = {
        "total": row[0],
        "empty_task_name": row[1], "empty_system_prompt": row[2],
        "empty_user_prompt": row[3], "empty_response": row[4],
        "empty_model_name": row[5], "empty_provider": row[6],
        "empty_prompt_hash": row[7],
        "has_guidance_id": row[8], "has_source_path": row[9],
        "has_raft_context": row[10], "has_think_content": row[11],
        "has_quality": row[12], "used_for_train": row[13],
    }
    result.data["column_population"] = col_pop

    # Flag empty critical columns (where no error)
    for col_name in ["empty_system_prompt", "empty_user_prompt", "empty_response"]:
        if col_pop[col_name] > 0:
            findings.append(Finding("warning", "llm_interactions",
                                    f"{col_pop[col_name]} rows have empty {col_name.replace('empty_', '')}"))

    if col_pop["has_raft_context"] == 0:
        findings.append(Finding("info", "llm_interactions", "RAFT context: 0% populated"))
    if col_pop["has_quality"] == 0:
        findings.append(Finding("info", "llm_interactions", "Quality annotations: 0% populated"))

    # System prompt hash distribution
    cur.execute("""
        SELECT system_prompt_hash, task_name, count(*)
        FROM llm_interactions
        WHERE system_prompt_hash IS NOT NULL AND system_prompt_hash != ''
        GROUP BY system_prompt_hash, task_name ORDER BY count(*) DESC
    """)
    result.data["prompt_hash_distribution"] = [
        {"hash": r[0], "task": r[1], "count": r[2]} for r in cur.fetchall()
    ]

    # Latency percentiles
    cur.execute("""
        SELECT task_name,
               percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) as p50,
               percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) as p95,
               percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms) as p99,
               max(latency_ms) as max_ms
        FROM llm_interactions WHERE latency_ms IS NOT NULL
        GROUP BY task_name
    """)
    result.data["latency_by_task"] = [
        {"task": r[0], "p50": _f(r[1]), "p95": _f(r[2]), "p99": _f(r[3]), "max": _f(r[4])}
        for r in cur.fetchall()
    ]

    # Token usage
    cur.execute("""
        SELECT task_name, avg(tokens_in) as avg_in, max(tokens_in) as max_in,
               avg(tokens_out) as avg_out, max(tokens_out) as max_out
        FROM llm_interactions WHERE tokens_in IS NOT NULL
        GROUP BY task_name
    """)
    result.data["token_usage"] = [
        {"task": r[0], "avg_in": _f(r[1]), "max_in": _f(r[2]), "avg_out": _f(r[3]), "max_out": _f(r[4])}
        for r in cur.fetchall()
    ]

    # Privacy scrub verification
    cur.execute("""
        SELECT system_prompt, user_prompt, response, think_content
        FROM llm_interactions ORDER BY time DESC LIMIT 50
    """)
    privacy_rows = cur.fetchall()
    privacy_violations = _check_privacy(privacy_rows, ["system_prompt", "user_prompt", "response", "think_content"])
    result.data["privacy_samples_checked"] = len(privacy_rows)
    result.data["privacy_violations"] = len(privacy_violations)
    if privacy_violations:
        for v in privacy_violations:
            findings.append(Finding("critical", "llm_interactions", f"Privacy scrub miss: {v}"))
    else:
        result.data["privacy_clean"] = True

    return result


def _f(val: Any) -> float | None:
    """Convert Decimal/numeric to float, None passthrough."""
    if val is None:
        return None
    return float(val)


def _check_privacy(rows: list[tuple], col_names: list[str]) -> list[str]:
    """Check rows for unscrubbed privacy-sensitive content."""
    violations = []
    for row in rows:
        for i, col_name in enumerate(col_names):
            text = row[i]
            if not text:
                continue
            for pattern_name, pattern in PRIVACY_PATTERNS.items():
                if pattern.search(text):
                    violations.append(f"{pattern_name} in {col_name}")
                    break  # one violation per column per row is enough
    return violations


# ─── Section D: embedding_events ───

def section_d_embedding_events(cur: Any, verbose: bool) -> SectionResult:
    result = SectionResult(name="embedding_events")
    findings = result.findings

    # Overview
    cur.execute("SELECT count(*), min(time), max(time) FROM embedding_events")
    row = cur.fetchone()
    total, min_time, max_time = row[0], row[1], row[2]
    result.data["total_rows"] = total
    result.data["min_time"] = min_time
    result.data["max_time"] = max_time

    if total == 0:
        findings.append(Finding("warning", "embedding_events", "Table is empty"))
        return result

    # Event type distribution
    cur.execute("SELECT event_type, count(*) FROM embedding_events GROUP BY event_type ORDER BY count(*) DESC")
    result.data["event_type_distribution"] = {r[0]: r[1] for r in cur.fetchall()}

    # Call site distribution
    cur.execute("SELECT call_site, count(*) FROM embedding_events GROUP BY call_site ORDER BY count(*) DESC")
    result.data["call_site_distribution"] = {r[0]: r[1] for r in cur.fetchall()}

    # Element kind distribution
    cur.execute("""
        SELECT element_kind, count(*) FROM embedding_events
        WHERE element_kind IS NOT NULL GROUP BY element_kind ORDER BY count(*) DESC
    """)
    result.data["element_kind_distribution"] = {r[0]: r[1] for r in cur.fetchall()}

    # Language distribution
    cur.execute("""
        SELECT language, count(*) FROM embedding_events
        WHERE language IS NOT NULL GROUP BY language ORDER BY count(*) DESC
    """)
    result.data["language_distribution"] = {r[0]: r[1] for r in cur.fetchall()}

    # Cache hit rate
    cur.execute("""
        SELECT count(*) as total,
               count(CASE WHEN cached = true THEN 1 END) as cached
        FROM embedding_events
    """)
    row = cur.fetchone()
    cache_total, cache_hit = row[0], row[1]
    cache_pct = pct(cache_hit, cache_total)
    result.data["cache_total"] = cache_total
    result.data["cache_hits"] = cache_hit
    result.data["cache_hit_pct"] = cache_pct
    if cache_pct > 98.0:
        findings.append(Finding("info", "embedding_events", f"Cache hit rate {cache_pct}% — very few unique texts being embedded"))

    # Model consistency
    cur.execute("SELECT model_name, dimensions, count(*) FROM embedding_events GROUP BY model_name, dimensions")
    model_combos = [{"model": r[0], "dimensions": r[1], "count": r[2]} for r in cur.fetchall()]
    result.data["model_dimension_combos"] = model_combos
    if len(model_combos) > 1:
        findings.append(Finding("warning", "embedding_events",
                                f"{len(model_combos)} different model/dimension combos detected — may indicate inconsistency"))

    # Empty call_site check (Sprint 1 regression test)
    cur.execute("SELECT count(*) FROM embedding_events WHERE call_site = '' OR call_site IS NULL")
    empty_cs = cur.fetchone()[0]
    result.data["empty_call_sites"] = empty_cs
    if empty_cs > 0:
        findings.append(Finding("critical", "embedding_events",
                                f"{empty_cs} rows have empty call_site (Sprint 1 regression)"))

    # Latency percentiles (non-cached only)
    cur.execute("""
        SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) as p50,
               percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) as p95,
               percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms) as p99
        FROM embedding_events WHERE cached = false AND latency_ms > 0
    """)
    row = cur.fetchone()
    if row and row[0] is not None:
        result.data["latency"] = {"p50": _f(row[0]), "p95": _f(row[1]), "p99": _f(row[2])}

    # Scrub asymmetry: query_text must NOT be scrubbed
    cur.execute("""
        SELECT query_text FROM embedding_events
        WHERE query_text IS NOT NULL AND query_text != ''
        ORDER BY time DESC LIMIT 50
    """)
    query_rows = cur.fetchall()
    scrubbed_query_count = sum(1 for r in query_rows if r[0] and "[REDACTED" in r[0])
    result.data["query_text_samples_checked"] = len(query_rows)
    result.data["query_text_scrubbed_count"] = scrubbed_query_count
    if scrubbed_query_count > 0:
        findings.append(Finding("critical", "embedding_events",
                                f"{scrubbed_query_count} query_text values contain [REDACTED — breaks contrastive training"))

    return result


# ─── Section E: retrieval_events ───

def section_e_retrieval_events(cur: Any, verbose: bool) -> SectionResult:
    result = SectionResult(name="retrieval_events")
    findings = result.findings

    # Overview
    cur.execute("SELECT count(*), min(time), max(time) FROM retrieval_events")
    row = cur.fetchone()
    total, min_time, max_time = row[0], row[1], row[2]
    result.data["total_rows"] = total
    result.data["min_time"] = min_time
    result.data["max_time"] = max_time

    if total == 0:
        findings.append(Finding("warning", "retrieval_events", "Table is empty"))
        return result

    # Call site distribution
    cur.execute("SELECT call_site, count(*) FROM retrieval_events GROUP BY call_site ORDER BY count(*) DESC")
    result.data["call_site_distribution"] = {r[0]: r[1] for r in cur.fetchall()}

    # Pipeline stage completeness
    cur.execute("""
        SELECT count(*) as total,
               count(CASE WHEN recall_node_ids IS NOT NULL AND array_length(recall_node_ids, 1) > 0 THEN 1 END) as has_recall,
               count(CASE WHEN bm25_node_ids IS NOT NULL AND array_length(bm25_node_ids, 1) > 0 THEN 1 END) as has_bm25,
               count(CASE WHEN rerank_node_ids IS NOT NULL AND array_length(rerank_node_ids, 1) > 0 THEN 1 END) as has_rerank,
               count(CASE WHEN result_node_ids IS NOT NULL AND array_length(result_node_ids, 1) > 0 THEN 1 END) as has_result,
               count(CASE WHEN guidance_id IS NOT NULL AND guidance_id != '' THEN 1 END) as has_guidance
        FROM retrieval_events
    """)
    row = cur.fetchone()
    stages = {
        "total": row[0], "has_recall": row[1], "has_bm25": row[2],
        "has_rerank": row[3], "has_result": row[4], "has_guidance": row[5],
    }
    result.data["pipeline_stages"] = stages

    recall_pct = pct(stages["has_recall"], stages["total"])
    guidance_pct = pct(stages["has_guidance"], stages["total"])
    result.data["recall_pct"] = recall_pct
    result.data["guidance_pct"] = guidance_pct

    if recall_pct < 100.0:
        findings.append(Finding("critical", "retrieval_events",
                                f"Recall population only {recall_pct}% — every retrieval should have recall"))
    if guidance_pct < 80.0:
        findings.append(Finding("info", "retrieval_events",
                                f"guidance_id population {guidance_pct}% (direct queries don't have guidance)"))

    # Hard-negative mining viability
    cur.execute("""
        SELECT count(*) as hard_negative_candidates
        FROM retrieval_events
        WHERE array_length(recall_scores, 1) > 0
          AND array_length(rerank_scores, 1) > 0
    """)
    hn_count = cur.fetchone()[0]
    result.data["hard_negative_candidates"] = hn_count
    if hn_count == 0:
        findings.append(Finding("info", "retrieval_events", "Zero hard-negative candidates (need both recall + rerank scores)"))

    # Downstream quality population
    cur.execute("SELECT count(CASE WHEN downstream_quality IS NOT NULL THEN 1 END) as has_quality FROM retrieval_events")
    result.data["has_downstream_quality"] = cur.fetchone()[0]

    # Latency breakdown
    cur.execute("""
        SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY total_latency_ms) as p50_total,
               percentile_cont(0.95) WITHIN GROUP (ORDER BY total_latency_ms) as p95_total,
               percentile_cont(0.5) WITHIN GROUP (ORDER BY rerank_latency_ms) FILTER (WHERE rerank_latency_ms > 0) as p50_rerank,
               percentile_cont(0.95) WITHIN GROUP (ORDER BY rerank_latency_ms) FILTER (WHERE rerank_latency_ms > 0) as p95_rerank
        FROM retrieval_events WHERE total_latency_ms IS NOT NULL
    """)
    row = cur.fetchone()
    if row and row[0] is not None:
        result.data["latency"] = {
            "p50_total": _f(row[0]), "p95_total": _f(row[1]),
            "p50_rerank": _f(row[2]), "p95_rerank": _f(row[3]),
        }

    return result


# ─── Section F: ft_* Tables ───

def section_f_ft_tables(cur: Any, verbose: bool) -> SectionResult:
    result = SectionResult(name="ft_tables")
    findings = result.findings

    cur.execute("""
        SELECT 'ft_benchmarks' as tbl, count(*) FROM ft_benchmarks
        UNION ALL SELECT 'ft_training_cycles', count(*) FROM ft_training_cycles
        UNION ALL SELECT 'ft_model_versions', count(*) FROM ft_model_versions
        UNION ALL SELECT 'ft_hitl_decisions', count(*) FROM ft_hitl_decisions
    """)
    counts = {r[0]: r[1] for r in cur.fetchall()}
    result.data["table_counts"] = counts

    all_empty = all(v == 0 for v in counts.values())
    result.data["all_empty"] = all_empty

    if all_empty:
        findings.append(Finding("info", "ft_tables", "All 4 ft_* tables empty (expected — FT pipeline not yet built)"))
    else:
        for tbl, cnt in counts.items():
            if cnt > 0:
                findings.append(Finding("info", "ft_tables", f"{tbl} has {cnt} rows (unexpected)"))

    return result


# ─── Section G: Cross-Cutting Analysis ───

def section_g_cross_cutting(cur: Any, mdemg_url: str, verbose: bool) -> SectionResult:
    result = SectionResult(name="cross_cutting")
    findings = result.findings

    # Growth rate per table
    growth_query = """
        SELECT 'metric_samples' as tbl,
               count(*) / GREATEST(EXTRACT(EPOCH FROM max(time) - min(time)) / 3600, 1) as rows_per_hour
        FROM metric_samples WHERE time > now() - interval '24 hours'
        UNION ALL
        SELECT 'llm_interactions',
               count(*) / GREATEST(EXTRACT(EPOCH FROM max(time) - min(time)) / 3600, 1)
        FROM llm_interactions WHERE time > now() - interval '24 hours'
        UNION ALL
        SELECT 'embedding_events',
               count(*) / GREATEST(EXTRACT(EPOCH FROM max(time) - min(time)) / 3600, 1)
        FROM embedding_events WHERE time > now() - interval '24 hours'
        UNION ALL
        SELECT 'retrieval_events',
               count(*) / GREATEST(EXTRACT(EPOCH FROM max(time) - min(time)) / 3600, 1)
        FROM retrieval_events WHERE time > now() - interval '24 hours'
    """
    cur.execute(growth_query)
    growth = {r[0]: _f(r[1]) for r in cur.fetchall()}
    result.data["growth_rates"] = growth

    for tbl, rate in growth.items():
        if rate is not None and rate == 0:
            findings.append(Finding("warning", "cross_cutting", f"{tbl}: zero growth in last 24h"))

    # Flush gap detection — use time_bucket for performance on large tables
    cur.execute("""
        SELECT count(*) as gap_count
        FROM (
            SELECT bucket, lead(bucket) OVER (ORDER BY bucket) as next_bucket
            FROM (
                SELECT time_bucket('30 seconds', time) as bucket
                FROM metric_samples
                WHERE time > now() - interval '24 hours'
                GROUP BY bucket
            ) buckets
        ) gaps
        WHERE EXTRACT(EPOCH FROM next_bucket - bucket) > 120
    """)
    gap_count = cur.fetchone()[0]
    result.data["flush_gaps_gt_120s"] = gap_count
    if gap_count > 5:
        findings.append(Finding("warning", "cross_cutting", f"{gap_count} flush gaps >120s in last 24h"))

    # Temporal coverage (rows per hour per table — last 24h)
    if verbose:
        cur.execute("""
            SELECT time_bucket('1 hour', time) as hour, count(*)
            FROM metric_samples
            WHERE time > now() - interval '24 hours'
            GROUP BY hour ORDER BY hour
        """)
        result.data["temporal_coverage"] = [
            {"hour": r[0].isoformat() if r[0] else None, "count": r[1]} for r in cur.fetchall()
        ]

    # Config flag check via HTTP
    config_flags = _check_config_flags(mdemg_url)
    result.data["config_flags"] = config_flags
    if "error" in config_flags:
        findings.append(Finding("warning", "cross_cutting", f"Could not fetch config: {config_flags['error']}"))
    else:
        tsdb_enabled = config_flags.get("TSDB_ENABLED", config_flags.get("tsdb_enabled"))
        if tsdb_enabled is not None and str(tsdb_enabled).lower() in ("false", "0"):
            findings.append(Finding("critical", "cross_cutting", "TSDB_ENABLED is false in running config"))

    return result


def _check_config_flags(mdemg_url: str) -> dict[str, Any]:
    """Fetch /v1/admin/config and extract TSDB-related flags."""
    try:
        req = urllib.request.Request(f"{mdemg_url}/v1/admin/config")
        with urllib.request.urlopen(req, timeout=5) as resp:
            config = json.loads(resp.read())
        flags: dict[str, Any] = {}
        if isinstance(config, list):
            for entry in config:
                key = entry.get("key", "").lower()
                if any(k in key for k in ["tsdb", "hybrid", "rerank", "llm_interaction", "embedding_event", "retrieval_event"]):
                    flags[entry["key"]] = entry["value"]
        elif isinstance(config, dict):
            for key, value in config.items():
                k_lower = key.lower()
                if any(k in k_lower for k in ["tsdb", "hybrid", "rerank", "llm_interaction", "embedding_event", "retrieval_event"]):
                    flags[key] = value
        return flags
    except Exception as e:
        return {"error": str(e)}


# ─── Output Formatting ───

# ANSI colors
_RESET = "\033[0m"
_BOLD = "\033[1m"
_RED = "\033[31m"
_YELLOW = "\033[33m"
_GREEN = "\033[32m"
_BLUE = "\033[34m"
_DIM = "\033[2m"

_CHECK = "\u2705"
_WARN = "\u26a0\ufe0f"
_INFO = "\u2139\ufe0f"
_CRIT = "\U0001f534"


def format_text(sections: list[SectionResult], all_findings: list[Finding], generated: str, duration_hours: float | None) -> str:
    """Format the report as colored terminal text."""
    lines: list[str] = []
    lines.append(f"\n{_BOLD}MDEMG TSDB Data Governance Report{_RESET}")
    lines.append("=" * 50)
    lines.append(f"Generated: {generated}")
    schema_v = None
    for s in sections:
        if s.name == "schema_health":
            schema_v = s.data.get("schema_version")
    if schema_v is not None:
        lines.append(f"Schema Version: {schema_v}")
    if duration_hours is not None:
        lines.append(f"Data Collection Duration: {duration_hours} hours")
    lines.append("")

    for section in sections:
        lines.append(_format_section_text(section))
        lines.append("")

    # Findings summary
    lines.append("=" * 50)
    lines.append(f"  {_BOLD}FINDINGS{_RESET}")
    lines.append("=" * 50)
    crit = sum(1 for f in all_findings if f.level == "critical")
    warn = sum(1 for f in all_findings if f.level == "warning")
    info = sum(1 for f in all_findings if f.level == "info")
    lines.append(f"  {_CRIT} CRITICAL: {crit}")
    lines.append(f"  {_WARN} WARNING:  {warn}")
    lines.append(f"  {_INFO} INFO:     {info}")

    for i, f in enumerate(all_findings, 1):
        icon = {
            "critical": f"{_RED}{_CRIT}{_RESET}",
            "warning": f"{_YELLOW}{_WARN}{_RESET}",
            "info": f"{_BLUE}{_INFO}{_RESET}",
        }.get(f.level, "  ")
        lines.append(f"    {i}. {icon} [{f.section}] {f.message}")

    lines.append("")
    return "\n".join(lines)


def _format_section_text(section: SectionResult) -> str:
    """Format a single section for terminal output."""
    lines: list[str] = []
    d = section.data
    name = section.name.replace("_", " ").title()

    # Section header with row count if available
    total = d.get("total_rows")
    if total is not None:
        lines.append(f"{_DIM}\u2500\u2500 Section: {name} ({total:,} rows) \u2500\u2500{_RESET}")
    else:
        lines.append(f"{_DIM}\u2500\u2500 Section: {name} \u2500\u2500{_RESET}")

    if section.name == "schema_health":
        tp = d.get("tables_present", 0)
        te = d.get("tables_expected", 0)
        lines.append(f"  Tables: {tp}/{te} present{_status_icon(tp == te)}")
        lines.append(f"  Indexes: {d.get('index_count', 0)} found")
        ca = d.get("continuous_aggregates", [])
        lines.append(f"  Continuous aggregates: {len(ca)}/2 present{_status_icon(len(ca) >= 2)}")

    elif section.name == "metric_samples":
        lines.append(f"  Date range: {d.get('min_time', 'N/A')} \u2192 {d.get('max_time', 'N/A')}")
        lines.append(f"  Distinct metrics: {d.get('distinct_metrics', 0)}")
        prefix_dist = d.get("prefix_distribution", [])
        if prefix_dist:
            lines.append("  Prefix distribution:")
            for p in prefix_dist[:8]:
                lines.append(f"    {p['prefix']:14s} \u2014 {p['metrics']} metrics, {p['rows']:,} rows")
        mh = d.get("metrics_hourly_rows")
        md = d.get("metrics_daily_rows")
        if mh is not None:
            lines.append("  Continuous aggregates:")
            lines.append(f"    metrics_hourly: {mh:,} rows{_status_icon(mh > 0)}")
            lines.append(f"    metrics_daily:  {md:,} rows{_status_icon(md is not None and md > 0)}")

    elif section.name == "llm_interactions":
        lines.append(f"  Date range: {d.get('min_time', 'N/A')} \u2192 {d.get('max_time', 'N/A')}")
        td = d.get("task_distribution", {})
        lines.append(f"  Task coverage: {len(td)}/{len(EXPECTED_TASK_NAMES)} {_WARN if len(td) < len(EXPECTED_TASK_NAMES) else _CHECK}")
        for task, cnt in td.items():
            lines.append(f"    {task:35s} {cnt}")
        mt = d.get("missing_task_names", [])
        if mt:
            lines.append(f"    ({len(mt)} task_names never produced records)")
        lines.append(f"  Error rate: {d.get('error_rate_pct', 0)}%{_status_icon(d.get('error_rate_pct', 0) <= 5)}")
        cp = d.get("column_population", {})
        if cp:
            critical_empty = cp.get("empty_system_prompt", 0) + cp.get("empty_user_prompt", 0) + cp.get("empty_response", 0)
            lines.append(f"  Critical columns: {'100% populated' if critical_empty == 0 else f'{critical_empty} empty'}{_status_icon(critical_empty == 0)}")
        ps = d.get("privacy_samples_checked", 0)
        pv = d.get("privacy_violations", 0)
        lines.append(f"  Privacy scrub: {ps}/{ps} samples clean{_status_icon(pv == 0)}" if pv == 0 else
                     f"  Privacy scrub: {pv} VIOLATIONS{_status_icon(False)}")
        raft = cp.get("has_raft_context", 0) if cp else 0
        lines.append(f"  RAFT context: {pct(raft, cp.get('total', 1))}% populated  {_INFO}")
        lat = d.get("latency_by_task", [])
        for lt in lat:
            lines.append(f"  Latency ({lt['task']}): p50={_ms(lt['p50'])} p95={_ms(lt['p95'])} p99={_ms(lt['p99'])}")

    elif section.name == "embedding_events":
        lines.append(f"  Date range: {d.get('min_time', 'N/A')} \u2192 {d.get('max_time', 'N/A')}")
        et = d.get("event_type_distribution", {})
        if et:
            lines.append("  Event types:")
            for k, v in et.items():
                lines.append(f"    {k:20s} {v:,}")
        cs = d.get("call_site_distribution", {})
        if cs:
            lines.append("  Call sites:")
            for k, v in cs.items():
                lines.append(f"    {k:20s} {v:,}")
        lines.append(f"  Cache hit rate: {d.get('cache_hit_pct', 0)}%")
        ecs = d.get("empty_call_sites", 0)
        lines.append(f"  Empty call_sites: {ecs}{_status_icon(ecs == 0)}")
        qt_scrub = d.get("query_text_scrubbed_count", 0)
        lines.append(f"  Query text scrub check: {d.get('query_text_samples_checked', 0)} samples, {qt_scrub} scrubbed{_status_icon(qt_scrub == 0)}")

    elif section.name == "retrieval_events":
        lines.append(f"  Date range: {d.get('min_time', 'N/A')} \u2192 {d.get('max_time', 'N/A')}")
        cs = d.get("call_site_distribution", {})
        if cs:
            lines.append("  Call sites:")
            for k, v in cs.items():
                lines.append(f"    {k:20s} {v:,}")
        stages = d.get("pipeline_stages", {})
        if stages:
            t = stages.get("total", 1)
            lines.append("  Pipeline stage completeness:")
            for stage in ["has_recall", "has_bm25", "has_rerank", "has_result", "has_guidance"]:
                cnt = stages.get(stage, 0)
                lines.append(f"    {stage:15s} {cnt}/{t} ({pct(cnt, t)}%)")
        lat = d.get("latency", {})
        if lat:
            lines.append(f"  Latency: total p50={_ms(lat.get('p50_total'))} p95={_ms(lat.get('p95_total'))}")

    elif section.name == "ft_tables":
        counts = d.get("table_counts", {})
        for tbl, cnt in counts.items():
            lines.append(f"  {tbl}: {cnt} rows{_status_icon(cnt == 0)}")
        if d.get("all_empty"):
            lines.append(f"  All 4 tables empty (expected)  {_CHECK}")

    elif section.name == "cross_cutting":
        gr = d.get("growth_rates", {})
        if gr:
            lines.append("  Growth rates (last 24h):")
            for tbl, rate in gr.items():
                r_str = f"{rate:.1f}" if rate is not None else "N/A"
                lines.append(f"    {tbl:20s} {r_str} rows/hour")
        gaps = d.get("flush_gaps_gt_120s", 0)
        lines.append(f"  Flush gaps (>120s): {gaps}{_status_icon(gaps <= 5)}")
        flags = d.get("config_flags", {})
        if flags and "error" not in flags:
            lines.append("  Config flags:")
            for k, v in flags.items():
                lines.append(f"    {k}: {v}{_status_icon(str(v).lower() not in ('false', '0'))}")

    return "\n".join(lines)


def _status_icon(ok: bool) -> str:
    return f"  {_CHECK}" if ok else f"  {_WARN}"


def _ms(val: float | None) -> str:
    if val is None:
        return "N/A"
    return f"{val:.0f}ms"


def format_json(sections: list[SectionResult], all_findings: list[Finding],
                generated: str, duration_hours: float | None) -> str:
    """Format the report as JSON."""
    schema_v = None
    for s in sections:
        if s.name == "schema_health":
            schema_v = s.data.get("schema_version")

    report = {
        "generated": generated,
        "schema_version": schema_v,
        "collection_duration_hours": duration_hours,
        "sections": {s.name: s.data for s in sections},
        "findings": [{"level": f.level, "section": f.section, "message": f.message} for f in all_findings],
        "summary": {
            "critical": sum(1 for f in all_findings if f.level == "critical"),
            "warning": sum(1 for f in all_findings if f.level == "warning"),
            "info": sum(1 for f in all_findings if f.level == "info"),
        },
    }
    return json.dumps(report, cls=JSONEncoder, indent=2, default=str)


# ─── CLI ───

def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="MDEMG TSDB Data Governance Report — comprehensive data quality diagnostic",
    )
    parser.add_argument("--tsdb-dsn", default=DEFAULT_DSN,
                        help=f"PostgreSQL connection string (default: {_mask_dsn(DEFAULT_DSN)})")
    parser.add_argument("--format", default="text", choices=["text", "json", "both"],
                        dest="output_format",
                        help="Output format (default: text)")
    parser.add_argument("--output", default=None,
                        help="JSON output file path (default: tsdb_data_review_YYYY-MM-DD.json)")
    parser.add_argument("--mdemg-url", default=DEFAULT_MDEMG_URL,
                        help=f"MDEMG API endpoint for config check (default: {DEFAULT_MDEMG_URL})")
    parser.add_argument("--verbose", action="store_true",
                        help="Show detailed query results")
    return parser.parse_args(argv)


def connect(dsn: str):
    """Connect to TSDB in read-only mode."""
    import psycopg2
    logger.info("Connecting to TSDB: %s", _mask_dsn(dsn))
    conn = psycopg2.connect(dsn)
    conn.set_session(readonly=True)
    return conn


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    generated = datetime.now(timezone.utc).isoformat()

    try:
        conn = connect(args.tsdb_dsn)
    except Exception as e:
        print(f"ERROR: Cannot connect to TSDB at {_mask_dsn(args.tsdb_dsn)}: {e}", file=sys.stderr)
        return 1

    cur = conn.cursor()
    sections: list[SectionResult] = []

    # Run each section, catching errors individually
    section_funcs = [
        ("A", "schema_health", lambda: section_a_schema_health(cur, args.verbose)),
        ("F", "ft_tables", lambda: section_f_ft_tables(cur, args.verbose)),
        ("E", "retrieval_events", lambda: section_e_retrieval_events(cur, args.verbose)),
        ("D", "embedding_events", lambda: section_d_embedding_events(cur, args.verbose)),
        ("B", "metric_samples", lambda: section_b_metric_samples(cur, args.verbose)),
        ("C", "llm_interactions", lambda: section_c_llm_interactions(cur, args.verbose)),
        ("G", "cross_cutting", lambda: section_g_cross_cutting(cur, args.mdemg_url, args.verbose)),
    ]

    for label, name, func in section_funcs:
        try:
            logger.info("Running Section %s: %s", label, name)
            result = func()
            sections.append(result)
        except Exception as e:
            logger.warning("Section %s (%s) failed: %s", label, name, e)
            err_result = SectionResult(name=name)
            err_result.findings.append(Finding("warning", name, f"Section failed: {e}"))
            sections.append(err_result)

    cur.close()
    conn.close()

    # Reorder sections for output: A, B, C, D, E, F, G
    section_order = ["schema_health", "metric_samples", "llm_interactions",
                     "embedding_events", "retrieval_events", "ft_tables", "cross_cutting"]
    sections.sort(key=lambda s: section_order.index(s.name) if s.name in section_order else 99)

    # Collect all findings
    all_findings = []
    for s in sections:
        all_findings.extend(s.findings)

    # Compute duration from metric_samples if available
    duration_hours = None
    for s in sections:
        if s.name == "metric_samples" and "duration_hours" in s.data:
            duration_hours = s.data["duration_hours"]
            break

    # Output
    if args.output_format in ("text", "both"):
        print(format_text(sections, all_findings, generated, duration_hours))

    if args.output_format in ("json", "both"):
        json_str = format_json(sections, all_findings, generated, duration_hours)
        output_path = args.output or f"tsdb_data_review_{datetime.now().strftime('%Y-%m-%d')}.json"
        with open(output_path, "w") as f:
            f.write(json_str)
        logger.info("JSON report written to %s", output_path)

    # Exit code: non-zero if any critical findings
    crit_count = sum(1 for f in all_findings if f.level == "critical")
    return 1 if crit_count > 0 else 0


if __name__ == "__main__":
    sys.exit(main())
