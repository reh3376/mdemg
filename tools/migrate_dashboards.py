#!/usr/bin/env python3
"""Migrate Grafana dashboard panels from Prometheus to TimescaleDB SQL.

Reads Grafana dashboard JSON files, converts PromQL expressions to equivalent
TimescaleDB SQL queries against the metric_samples table, and replaces the
datasource with {"type": "postgres", "uid": "timescaledb"}.

Usage:
    python tools/migrate_dashboards.py                   # migrate all dashboards
    python tools/migrate_dashboards.py --dry-run         # preview changes
    python tools/migrate_dashboards.py --dashboard mdemg-overview.json
"""

from __future__ import annotations

import argparse
import copy
import json
import os
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

DASHBOARD_DIR = Path(__file__).resolve().parent.parent / "deploy" / "docker" / "grafana" / "dashboards"

TSDB_DATASOURCE: dict[str, str] = {"type": "postgres", "uid": "timescaledb"}

# Prometheus metric prefix added by the rendering path.  The TSDB
# MetricsRecorder uses the name as registered in collectors.go (without prefix).
PROM_PREFIX = "mdemg_"

# Datasource types/uids to skip (already non-Prometheus)
SKIP_DS_TYPES = {
    "postgres",
    "kniepdennis-neo4j-datasource",
    "hamedkarbasi93-nodegraphapi-datasource",
}
SKIP_DS_UIDS = {"timescaledb", "neo4j", "mdemg-nodegraph"}

# Panel types that never have convertible queries
SKIP_PANEL_TYPES = {"row", "text"}


# ---------------------------------------------------------------------------
# Data structures
# ---------------------------------------------------------------------------

@dataclass
class MigrationRecord:
    dashboard: str
    panel_title: str
    target_idx: int
    original_expr: str
    sql: str
    metric_name: str
    reason: str = ""


@dataclass
class DashboardReport:
    file_name: str
    panels_migrated: int = 0
    panels_skipped: int = 0
    records: list[MigrationRecord] = field(default_factory=list)


# ---------------------------------------------------------------------------
# PromQL -> SQL conversion helpers
# ---------------------------------------------------------------------------

def strip_prefix(metric: str) -> str:
    """Strip the mdemg_ prefix from a Prometheus metric name."""
    if metric.startswith(PROM_PREFIX):
        return metric[len(PROM_PREFIX):]
    return metric


def parse_label_selectors(selector_str: str) -> list[tuple[str, str, str]]:
    """Parse PromQL label selectors like {space_id="$space_id", status=~"5.."}.

    Returns list of (label, operator, value) tuples.
    Operators: '=' (exact), '=~' (regex), '!=' (not equal), '!~' (not regex).
    """
    if not selector_str or selector_str.strip() == "":
        return []
    results = []
    # Match: label_name operator "value"
    pattern = re.compile(r'(\w+)\s*(=~|!=|!~|=)\s*"([^"]*)"')
    for m in pattern.finditer(selector_str):
        results.append((m.group(1), m.group(2), m.group(3)))
    return results


def label_selectors_to_sql(selectors: list[tuple[str, str, str]]) -> str:
    """Convert parsed label selectors to SQL WHERE clauses."""
    clauses = []
    for label, op, value in selectors:
        col_expr = f"labels->>'{label}'"
        if op == "=":
            clauses.append(f"AND {col_expr} = '{value}'")
        elif op == "!=":
            clauses.append(f"AND {col_expr} != '{value}'")
        elif op == "=~":
            # Convert regex to SQL: simple alternation -> IN, otherwise SIMILAR TO
            # Handle common patterns: "5.." -> LIKE '5__', "completed|error" -> IN
            if "|" in value and not any(c in value for c in ".+*?[]()^$\\"):
                # Pure alternation
                alternatives = value.split("|")
                in_list = ", ".join(f"'{a}'" for a in alternatives)
                clauses.append(f"AND {col_expr} IN ({in_list})")
            elif re.fullmatch(r'[\w]+\.\.', value):
                # Pattern like "5.." -> LIKE '5__' (Prometheus . = any char)
                like_pattern = value.replace(".", "_")
                clauses.append(f"AND {col_expr} LIKE '{like_pattern}'")
            else:
                # General regex -> use ~ operator (PostgreSQL regex)
                clauses.append(f"AND {col_expr} ~ '{value}'")
        elif op == "!~":
            clauses.append(f"AND {col_expr} !~ '{value}'")
    return " ".join(clauses)


def extract_metric_and_labels(metric_expr: str) -> tuple[str, str, list[tuple[str, str, str]]]:
    """Extract metric name and label selectors from a PromQL metric expression.

    Returns (full_metric_name, tsdb_metric_name, label_selectors).
    """
    m = re.match(r'([\w:]+)\s*(?:\{([^}]*)\})?', metric_expr.strip())
    if not m:
        return metric_expr, strip_prefix(metric_expr), []
    full_name = m.group(1)
    tsdb_name = strip_prefix(full_name)
    selectors = parse_label_selectors(m.group(2) or "")
    return full_name, tsdb_name, selectors


def detect_metric_type(tsdb_name: str) -> str:
    """Infer metric type from the TSDB metric name.

    Based on naming conventions in collectors.go:
    - *_total -> counter
    - *_seconds (without _bucket) -> histogram (registered name)
    - *_bucket -> histogram bucket
    - Everything else -> gauge
    """
    if tsdb_name.endswith("_total"):
        return "counter"
    if tsdb_name.endswith("_seconds") and not tsdb_name.endswith("_bucket"):
        return "histogram"
    if tsdb_name.endswith("_bucket"):
        return "histogram"
    if tsdb_name.endswith("_bytes"):
        return "gauge"
    return "gauge"


# ---------------------------------------------------------------------------
# PromQL -> SQL conversion engine
# ---------------------------------------------------------------------------

def convert_promql_to_sql(expr: str) -> tuple[str, str, str]:
    """Convert a PromQL expression to a TimescaleDB SQL query.

    Returns (sql_query, tsdb_metric_name, conversion_reason).
    Raises ValueError if the expression cannot be converted.
    """
    expr = expr.strip()

    # -----------------------------------------------------------------------
    # Pattern: histogram_quantile(P, sum by (...) (rate(metric_bucket[window])))
    #      or: histogram_quantile(P, sum(rate(metric_bucket[window])) by (...))
    # -> Pre-computed percentile gauge
    # -----------------------------------------------------------------------
    quantile = None
    metric_full = None
    by_labels_str = ""

    # Variant 1: sum by (...) (rate(...))
    hq_match = re.match(
        r'histogram_quantile\(\s*([\d.]+)\s*,\s*sum\s+by\s*\(([^)]*)\)\s*\(rate\((\w+?)_bucket'
        r'(?:\{([^}]*)\})?\[([^\]]+)\]\)\)\s*\)',
        expr,
    )
    if hq_match:
        quantile = hq_match.group(1)
        by_labels_str = hq_match.group(2) or ""
        metric_full = hq_match.group(3)

    if not hq_match:
        # Variant 2: sum(rate(...)) by (...)
        hq_match = re.match(
            r'histogram_quantile\(\s*([\d.]+)\s*,\s*sum\s*\(rate\((\w+?)_bucket'
            r'(?:\{([^}]*)\})?\[([^\]]+)\]\)\)\s*by\s*\(([^)]*)\)\s*\)',
            expr,
        )
        if hq_match:
            quantile = hq_match.group(1)
            metric_full = hq_match.group(2)
            by_labels_str = hq_match.group(5) or ""

    if not hq_match:
        # Variant 3: sum(rate(...)) with no explicit by clause
        hq_match = re.match(
            r'histogram_quantile\(\s*([\d.]+)\s*,\s*sum\s*\(rate\((\w+?)_bucket'
            r'(?:\{([^}]*)\})?\[([^\]]+)\]\)\)\s*\)',
            expr,
        )
        if hq_match:
            quantile = hq_match.group(1)
            metric_full = hq_match.group(2)
            by_labels_str = ""

    if hq_match and quantile is not None and metric_full is not None:
        tsdb_base = strip_prefix(metric_full)
        # Determine percentile suffix
        q_float = float(quantile)
        if q_float == 0.5:
            p_suffix = "p50"
        elif q_float == 0.95:
            p_suffix = "p95"
        elif q_float == 0.99:
            p_suffix = "p99"
        else:
            p_suffix = f"p{int(q_float * 100)}"

        tsdb_metric = f"{tsdb_base}_{p_suffix}"

        # Check for extra group-by labels beyond "le"
        extra_by = []
        if by_labels_str:
            for lbl in re.split(r'\s*,\s*', by_labels_str):
                lbl = lbl.strip()
                if lbl and lbl != "le":
                    extra_by.append(lbl)

        if extra_by:
            select_cols = ", ".join(f"labels->>'{l}' AS {l}" for l in extra_by)
            group_cols = ", ".join(f"labels->>'{l}'" for l in extra_by)
            sql = (
                f"SELECT time, {select_cols}, value FROM metric_samples "
                f"WHERE metric_name = '{tsdb_metric}' AND metric_type = 'gauge' "
                f"AND $__timeFilter(time) "
                f"GROUP BY time, {group_cols}, value ORDER BY time"
            )
        else:
            sql = (
                f"SELECT time, value FROM metric_samples "
                f"WHERE metric_name = '{tsdb_metric}' AND metric_type = 'gauge' "
                f"AND $__timeFilter(time) ORDER BY time"
            )
        return sql, tsdb_metric, "histogram_quantile -> pre-computed gauge"

    # -----------------------------------------------------------------------
    # Pattern: (expr1 / expr2 * 100) or vector(0) -- error rate ratio
    # This is a complex composite expression. Convert to a CTE-based query.
    # -----------------------------------------------------------------------
    error_rate_match = re.match(
        r'\(sum\(rate\((\w+)\{([^}]*)\}\[([^\]]+)\]\)\)\s*/\s*sum\(rate\((\w+)\[([^\]]+)\]\)\)\s*\*\s*(\d+)\)\s*or\s*vector\((\d+)\)',
        expr,
    )
    if error_rate_match:
        num_metric = error_rate_match.group(1)
        num_labels_str = error_rate_match.group(2)
        num_window = error_rate_match.group(3)
        den_metric = error_rate_match.group(4)
        _den_window = error_rate_match.group(5)
        multiplier = error_rate_match.group(6)
        fallback = error_rate_match.group(7)

        tsdb_num = strip_prefix(num_metric)
        tsdb_den = strip_prefix(den_metric)
        num_selectors = parse_label_selectors(num_labels_str)
        num_where = label_selectors_to_sql(num_selectors)

        sql = (
            f"SELECT time_bucket('$__interval', time) AS time, "
            f"COALESCE("
            f"SUM(CASE WHEN metric_name = '{tsdb_num}' {num_where} THEN value ELSE 0 END) / "
            f"NULLIF(SUM(CASE WHEN metric_name = '{tsdb_den}' THEN value ELSE 0 END), 0) * {multiplier}, "
            f"{fallback}) AS value "
            f"FROM metric_samples "
            f"WHERE metric_name IN ('{tsdb_num}', '{tsdb_den}') AND metric_type = 'counter' "
            f"AND $__timeFilter(time) "
            f"GROUP BY 1 ORDER BY 1"
        )
        return sql, f"{tsdb_num}/{tsdb_den}", "error rate ratio"

    # -----------------------------------------------------------------------
    # Pattern: (sum(rate(A{outcome="completed"}[5m])) or vector(0)) / (sum(rate(A{...}[5m])) > 0 or vector(1)) * 100
    # Cycle success rate — complex ratio
    # -----------------------------------------------------------------------
    success_rate_match = re.match(
        r'\(sum\(rate\((\w+)\{([^}]*)\}\[([^\]]+)\]\)\)\s*or\s*vector\((\d+)\)\)\s*/\s*'
        r'\(sum\(rate\((\w+)\{([^}]*)\}\[([^\]]+)\]\)\)\s*>\s*\d+\s*or\s*vector\((\d+)\)\)\s*\*\s*(\d+)',
        expr,
    )
    if success_rate_match:
        num_metric = success_rate_match.group(1)
        num_labels_str = success_rate_match.group(2)
        _num_window = success_rate_match.group(3)
        _num_fallback = success_rate_match.group(4)
        den_metric = success_rate_match.group(5)
        den_labels_str = success_rate_match.group(6)
        _den_window = success_rate_match.group(7)
        _den_fallback = success_rate_match.group(8)
        multiplier = success_rate_match.group(9)

        tsdb_metric = strip_prefix(num_metric)
        num_selectors = parse_label_selectors(num_labels_str)
        den_selectors = parse_label_selectors(den_labels_str)
        num_where = label_selectors_to_sql(num_selectors)
        den_where = label_selectors_to_sql(den_selectors)

        sql = (
            f"SELECT time_bucket('$__interval', time) AS time, "
            f"COALESCE("
            f"SUM(CASE WHEN 1=1 {num_where} THEN value ELSE 0 END) / "
            f"NULLIF(SUM(CASE WHEN 1=1 {den_where} THEN value ELSE 0 END), 0) * {multiplier}, "
            f"0) AS value "
            f"FROM metric_samples "
            f"WHERE metric_name = '{tsdb_metric}' AND metric_type = 'counter' "
            f"AND $__timeFilter(time) "
            f"GROUP BY 1 ORDER BY 1"
        )
        return sql, tsdb_metric, "success rate ratio"

    # -----------------------------------------------------------------------
    # Pattern: count(metric == N) or vector(0) -- count matching condition
    # -----------------------------------------------------------------------
    count_match = re.match(
        r'count\((\w+)(?:\{([^}]*)\})?\s*==\s*([\d.]+)\)\s*or\s*vector\((\d+)\)',
        expr,
    )
    if count_match:
        metric_name = count_match.group(1)
        _labels_str = count_match.group(2) or ""
        threshold = count_match.group(3)
        fallback = count_match.group(4)
        tsdb_name = strip_prefix(metric_name)
        selectors = parse_label_selectors(_labels_str)
        where_extra = label_selectors_to_sql(selectors)

        sql = (
            f"SELECT time_bucket('$__interval', time) AS time, "
            f"COALESCE(COUNT(*) FILTER (WHERE value = {threshold}), {fallback}) AS value "
            f"FROM metric_samples "
            f"WHERE metric_name = '{tsdb_name}' AND metric_type = 'gauge' "
            f"{where_extra} "
            f"AND $__timeFilter(time) "
            f"GROUP BY 1 ORDER BY 1"
        )
        return sql, tsdb_name, "count with condition"

    # -----------------------------------------------------------------------
    # Pattern: max(metric{labels}) -- max aggregation
    # -----------------------------------------------------------------------
    max_match = re.match(
        r'max\((\w+)(?:\{([^}]*)\})?\)',
        expr,
    )
    if max_match:
        metric_name = max_match.group(1)
        labels_str = max_match.group(2) or ""
        tsdb_name = strip_prefix(metric_name)
        selectors = parse_label_selectors(labels_str)
        where_extra = label_selectors_to_sql(selectors)

        sql = (
            f"SELECT time, MAX(value) AS value "
            f"FROM metric_samples "
            f"WHERE metric_name = '{tsdb_name}' AND metric_type = 'gauge' "
            f"{where_extra} "
            f"AND $__timeFilter(time) "
            f"GROUP BY time ORDER BY time"
        )
        return sql, tsdb_name, "max aggregation"

    # -----------------------------------------------------------------------
    # Pattern: sum by (label) (increase(metric[window]))
    # -----------------------------------------------------------------------
    increase_match = re.match(
        r'sum\s*by\s*\(([^)]+)\)\s*\(increase\((\w+)(?:\{([^}]*)\})?\[([^\]]+)\]\)\)',
        expr,
    )
    if increase_match:
        by_labels = [l.strip() for l in increase_match.group(1).split(",")]
        metric_name = increase_match.group(2)
        labels_str = increase_match.group(3) or ""
        window = increase_match.group(4)
        tsdb_name = strip_prefix(metric_name)
        selectors = parse_label_selectors(labels_str)
        where_extra = label_selectors_to_sql(selectors)

        select_cols = ", ".join(f"labels->>'{l}' AS {l}" for l in by_labels)
        group_cols = ", ".join(f"labels->>'{l}'" for l in by_labels)

        sql = (
            f"SELECT time_bucket('{window}', time) AS time, "
            f"{select_cols}, SUM(value) AS total "
            f"FROM metric_samples "
            f"WHERE metric_name = '{tsdb_name}' AND metric_type = 'counter' "
            f"{where_extra} "
            f"AND $__timeFilter(time) "
            f"GROUP BY 1, {group_cols} ORDER BY 1"
        )
        return sql, tsdb_name, "increase with group by"

    # -----------------------------------------------------------------------
    # Pattern: sum by (label) (rate(metric[window])) [* multiplier]
    # -----------------------------------------------------------------------
    rate_by_match = re.match(
        r'sum\s*by\s*\(([^)]+)\)\s*\(rate\((\w+)(?:\{([^}]*)\})?\[([^\]]+)\]\)\)(?:\s*\*\s*([\d.]+))?',
        expr,
    )
    if rate_by_match:
        by_labels = [l.strip() for l in rate_by_match.group(1).split(",")]
        metric_name = rate_by_match.group(2)
        labels_str = rate_by_match.group(3) or ""
        _window = rate_by_match.group(4)
        multiplier = rate_by_match.group(5)
        tsdb_name = strip_prefix(metric_name)
        selectors = parse_label_selectors(labels_str)
        where_extra = label_selectors_to_sql(selectors)

        select_cols = ", ".join(f"labels->>'{l}' AS {l}" for l in by_labels)
        group_cols = ", ".join(f"labels->>'{l}'" for l in by_labels)

        rate_expr = "SUM(value) / EXTRACT(EPOCH FROM '$__interval'::interval)"
        if multiplier:
            rate_expr = f"({rate_expr}) * {multiplier}"

        sql = (
            f"SELECT time_bucket('$__interval', time) AS time, "
            f"{select_cols}, {rate_expr} AS rate "
            f"FROM metric_samples "
            f"WHERE metric_name = '{tsdb_name}' AND metric_type = 'counter' "
            f"{where_extra} "
            f"AND $__timeFilter(time) "
            f"GROUP BY 1, {group_cols} ORDER BY 1"
        )
        return sql, tsdb_name, "rate with group by"

    # -----------------------------------------------------------------------
    # Pattern: sum(rate(metric{labels}[window])) by (labels) [* multiplier]
    # (by-after variant of grouped rate)
    # -----------------------------------------------------------------------
    sum_rate_by_after_match = re.match(
        r'sum\(rate\((\w+)(?:\{([^}]*)\})?\[([^\]]+)\]\)\)\s*by\s*\(([^)]+)\)(?:\s*\*\s*([\d.]+))?',
        expr,
    )
    if sum_rate_by_after_match:
        metric_name = sum_rate_by_after_match.group(1)
        labels_str = sum_rate_by_after_match.group(2) or ""
        _window = sum_rate_by_after_match.group(3)
        by_labels = [l.strip() for l in sum_rate_by_after_match.group(4).split(",")]
        multiplier = sum_rate_by_after_match.group(5)
        tsdb_name = strip_prefix(metric_name)
        selectors = parse_label_selectors(labels_str)
        where_extra = label_selectors_to_sql(selectors)

        select_cols = ", ".join(f"labels->>'{l}' AS {l}" for l in by_labels)
        group_cols = ", ".join(f"labels->>'{l}'" for l in by_labels)

        rate_expr = "SUM(value) / EXTRACT(EPOCH FROM '$__interval'::interval)"
        if multiplier:
            rate_expr = f"({rate_expr}) * {multiplier}"

        sql = (
            f"SELECT time_bucket('$__interval', time) AS time, "
            f"{select_cols}, {rate_expr} AS rate "
            f"FROM metric_samples "
            f"WHERE metric_name = '{tsdb_name}' AND metric_type = 'counter' "
            f"{where_extra} "
            f"AND $__timeFilter(time) "
            f"GROUP BY 1, {group_cols} ORDER BY 1"
        )
        return sql, tsdb_name, "rate with group by (by-after)"

    # -----------------------------------------------------------------------
    # Pattern: sum(rate(metric{labels}[window])) [* multiplier]
    # -----------------------------------------------------------------------
    sum_rate_match = re.match(
        r'sum\(rate\((\w+)(?:\{([^}]*)\})?\[([^\]]+)\]\)\)(?:\s*\*\s*([\d.]+))?',
        expr,
    )
    if sum_rate_match:
        metric_name = sum_rate_match.group(1)
        labels_str = sum_rate_match.group(2) or ""
        _window = sum_rate_match.group(3)
        multiplier = sum_rate_match.group(4)
        tsdb_name = strip_prefix(metric_name)
        selectors = parse_label_selectors(labels_str)
        where_extra = label_selectors_to_sql(selectors)

        rate_expr = "SUM(value) / EXTRACT(EPOCH FROM '$__interval'::interval)"
        if multiplier:
            rate_expr = f"({rate_expr}) * {multiplier}"

        sql = (
            f"SELECT time_bucket('$__interval', time) AS time, "
            f"{rate_expr} AS rate "
            f"FROM metric_samples "
            f"WHERE metric_name = '{tsdb_name}' AND metric_type = 'counter' "
            f"{where_extra} "
            f"AND $__timeFilter(time) "
            f"GROUP BY 1 ORDER BY 1"
        )
        return sql, tsdb_name, "sum(rate(...))"

    # -----------------------------------------------------------------------
    # Pattern: rate(metric[window]) [* multiplier]  (no sum wrapper)
    # -----------------------------------------------------------------------
    rate_match = re.match(
        r'rate\((\w+)(?:\{([^}]*)\})?\[([^\]]+)\]\)(?:\s*\*\s*([\d.]+))?',
        expr,
    )
    if rate_match:
        metric_name = rate_match.group(1)
        labels_str = rate_match.group(2) or ""
        _window = rate_match.group(3)
        multiplier = rate_match.group(4)
        tsdb_name = strip_prefix(metric_name)
        selectors = parse_label_selectors(labels_str)
        where_extra = label_selectors_to_sql(selectors)

        rate_expr = "SUM(value) / EXTRACT(EPOCH FROM '$__interval'::interval)"
        if multiplier:
            rate_expr = f"({rate_expr}) * {multiplier}"

        sql = (
            f"SELECT time_bucket('$__interval', time) AS time, "
            f"{rate_expr} AS rate "
            f"FROM metric_samples "
            f"WHERE metric_name = '{tsdb_name}' AND metric_type = 'counter' "
            f"{where_extra} "
            f"AND $__timeFilter(time) "
            f"GROUP BY 1 ORDER BY 1"
        )
        return sql, tsdb_name, "rate(counter)"

    # -----------------------------------------------------------------------
    # Pattern: metric{labels} and on(label) (other_metric @ end())
    # "and on" join pattern (Neo4j dashboard) -- convert to simple gauge query
    # -----------------------------------------------------------------------
    and_on_match = re.match(
        r'(\w+)(?:\{([^}]*)\})?\s+and\s+on\(([^)]+)\)\s+\(.+\)',
        expr,
    )
    if and_on_match:
        metric_name = and_on_match.group(1)
        labels_str = and_on_match.group(2) or ""
        _join_label = and_on_match.group(3)
        tsdb_name = strip_prefix(metric_name)
        selectors = parse_label_selectors(labels_str)
        where_extra = label_selectors_to_sql(selectors)
        m_type = detect_metric_type(tsdb_name)

        sql = (
            f"SELECT time, labels->>'space_id' AS space_id, value "
            f"FROM metric_samples "
            f"WHERE metric_name = '{tsdb_name}' AND metric_type = '{m_type}' "
            f"{where_extra} "
            f"AND $__timeFilter(time) ORDER BY time"
        )
        return sql, tsdb_name, "metric with 'and on' join -> simple query"

    # -----------------------------------------------------------------------
    # Pattern: simple gauge/counter metric{labels} (instant or range)
    # This must be last as it's the most general pattern.
    # -----------------------------------------------------------------------
    simple_match = re.match(
        r'^([\w:]+)(?:\{([^}]*)\})?\s*$',
        expr,
    )
    if simple_match:
        metric_name = simple_match.group(1)
        labels_str = simple_match.group(2) or ""
        tsdb_name = strip_prefix(metric_name)
        selectors = parse_label_selectors(labels_str)
        where_extra = label_selectors_to_sql(selectors)
        m_type = detect_metric_type(tsdb_name)

        sql = (
            f"SELECT time, value FROM metric_samples "
            f"WHERE metric_name = '{tsdb_name}' AND metric_type = '{m_type}' "
            f"{where_extra} "
            f"AND $__timeFilter(time) ORDER BY time"
        )
        return sql, tsdb_name, "simple gauge/counter"

    raise ValueError(f"Cannot convert PromQL expression: {expr}")


# ---------------------------------------------------------------------------
# Panel processing
# ---------------------------------------------------------------------------

def should_skip_panel(panel: dict[str, Any]) -> tuple[bool, str]:
    """Determine if a panel should be skipped (not migrated).

    Returns (should_skip, reason).
    """
    panel_type = panel.get("type", "")
    if panel_type in SKIP_PANEL_TYPES:
        return True, f"panel type '{panel_type}'"

    ds = panel.get("datasource")
    if ds is not None and isinstance(ds, dict):
        ds_type = ds.get("type", "")
        ds_uid = ds.get("uid", "")
        if ds_type in SKIP_DS_TYPES or ds_uid in SKIP_DS_UIDS:
            return True, f"non-Prometheus datasource ({ds_type}/{ds_uid})"

    # Panels with no targets can't be migrated
    targets = panel.get("targets", [])
    if not targets:
        return True, "no targets"

    # Check if all targets already use rawSql (already TSDB)
    if all("rawSql" in t for t in targets):
        return True, "already uses SQL"

    return False, ""


def is_prometheus_panel(panel: dict[str, Any]) -> bool:
    """Check if a panel uses Prometheus datasource (explicit or implicit default)."""
    ds = panel.get("datasource")

    # No datasource key -> relies on Grafana default (Prometheus)
    if ds is None:
        # Must have targets with 'expr' key to be a Prometheus panel
        targets = panel.get("targets", [])
        return any("expr" in t for t in targets)

    if isinstance(ds, dict):
        ds_type = ds.get("type", "")
        ds_uid = ds.get("uid", "")
        return ds_type == "prometheus" or ds_uid == "prometheus"

    # String datasource reference
    if isinstance(ds, str):
        return ds.lower() in ("prometheus", "default", "")

    return False


def migrate_target(target: dict[str, Any], panel_title: str, target_idx: int,
                   dashboard_name: str) -> MigrationRecord | None:
    """Migrate a single panel target from PromQL to SQL.

    Returns a MigrationRecord on success, None if the target cannot be migrated.
    """
    expr = target.get("expr", "").strip()
    if not expr:
        return None

    try:
        sql, metric_name, reason = convert_promql_to_sql(expr)
    except ValueError as e:
        print(f"  WARNING: {dashboard_name} / '{panel_title}' target[{target_idx}]: {e}", file=sys.stderr)
        return None

    return MigrationRecord(
        dashboard=dashboard_name,
        panel_title=panel_title,
        target_idx=target_idx,
        original_expr=expr,
        sql=sql,
        metric_name=metric_name,
        reason=reason,
    )


def migrate_panel(panel: dict[str, Any], dashboard_name: str,
                  dry_run: bool) -> list[MigrationRecord]:
    """Migrate a single panel. Modifies panel in-place unless dry_run."""
    skip, skip_reason = should_skip_panel(panel)
    if skip:
        return []

    if not is_prometheus_panel(panel):
        return []

    panel_title = panel.get("title", "<untitled>")
    targets = panel.get("targets", [])
    records: list[MigrationRecord] = []

    for idx, target in enumerate(targets):
        record = migrate_target(target, panel_title, idx, dashboard_name)
        if record is not None:
            records.append(record)

    if not records:
        return []

    # Apply changes unless dry-run
    if not dry_run:
        panel["datasource"] = copy.deepcopy(TSDB_DATASOURCE)
        for idx, target in enumerate(targets):
            matching = [r for r in records if r.target_idx == idx]
            if matching:
                rec = matching[0]
                # Remove PromQL-specific keys and add SQL keys
                target.pop("expr", None)
                target.pop("legendFormat", None)
                target.pop("instant", None)
                target["rawSql"] = rec.sql
                target["format"] = "time_series"
                if "refId" not in target:
                    target["refId"] = chr(ord("A") + idx)

    return records


def migrate_panels_recursive(panels: list[dict[str, Any]], dashboard_name: str,
                             dry_run: bool) -> list[MigrationRecord]:
    """Recursively migrate panels, including those nested in collapsed rows."""
    all_records: list[MigrationRecord] = []
    for panel in panels:
        records = migrate_panel(panel, dashboard_name, dry_run)
        all_records.extend(records)

        # Handle panels nested inside collapsed row panels
        nested = panel.get("panels", [])
        if nested:
            nested_records = migrate_panels_recursive(nested, dashboard_name, dry_run)
            all_records.extend(nested_records)

    return all_records


def migrate_template_variables(dashboard: dict[str, Any], dashboard_name: str,
                               dry_run: bool) -> int:
    """Migrate template variable queries from Prometheus to TimescaleDB.

    Returns the number of variables migrated.
    """
    count = 0
    templating = dashboard.get("templating", {})
    variables = templating.get("list", [])

    for var in variables:
        var_type = var.get("type", "")
        if var_type != "query":
            continue

        ds = var.get("datasource", {})
        if not isinstance(ds, dict):
            continue

        ds_type = ds.get("type", "")
        ds_uid = ds.get("uid", "")

        if ds_type != "prometheus" and ds_uid != "prometheus":
            continue

        # This is a Prometheus-backed template variable query
        query_obj = var.get("query", {})
        query_str = ""
        if isinstance(query_obj, dict):
            query_str = query_obj.get("query", "")
        elif isinstance(query_obj, str):
            query_str = query_obj

        # Convert label_values(metric{filters}, label) to SQL
        lv_match = re.match(
            r'label_values\((\w+)(?:\{([^}]*)\})?\s*,\s*(\w+)\)',
            query_str,
        )
        if lv_match:
            metric_name = lv_match.group(1)
            filters_str = lv_match.group(2) or ""
            label_name = lv_match.group(3)
            tsdb_name = strip_prefix(metric_name)
            selectors = parse_label_selectors(filters_str)
            where_extra = label_selectors_to_sql(selectors)

            sql = (
                f"SELECT DISTINCT labels->>'{label_name}' AS __value, "
                f"labels->>'{label_name}' AS __text "
                f"FROM metric_samples "
                f"WHERE metric_name = '{tsdb_name}' {where_extra} "
                f"ORDER BY 1"
            )

            if not dry_run:
                var["datasource"] = copy.deepcopy(TSDB_DATASOURCE)
                var["query"] = sql
                var.pop("definition", None)
            count += 1
            print(f"  Variable '${var.get('name', '?')}': label_values -> SQL")

    return count


# ---------------------------------------------------------------------------
# Dashboard file processing
# ---------------------------------------------------------------------------

def process_dashboard(file_path: Path, dry_run: bool) -> DashboardReport:
    """Process a single dashboard JSON file."""
    dashboard_name = file_path.name
    report = DashboardReport(file_name=dashboard_name)

    with open(file_path, "r") as f:
        dashboard = json.load(f)

    # Make a deep copy for comparison if dry-run
    if dry_run:
        dashboard_copy = copy.deepcopy(dashboard)
    else:
        dashboard_copy = dashboard

    panels = dashboard_copy.get("panels", [])

    # Migrate panels
    records = migrate_panels_recursive(panels, dashboard_name, dry_run)
    report.records = records
    report.panels_migrated = len(set(r.panel_title for r in records))

    # Count skipped panels (Prometheus panels we couldn't migrate)
    total_prom_panels = 0
    for panel in panels:
        if is_prometheus_panel(panel) and panel.get("type") not in SKIP_PANEL_TYPES:
            total_prom_panels += 1
        for nested in panel.get("panels", []):
            if is_prometheus_panel(nested) and nested.get("type") not in SKIP_PANEL_TYPES:
                total_prom_panels += 1

    report.panels_skipped = total_prom_panels - report.panels_migrated

    # Migrate template variables
    vars_migrated = migrate_template_variables(dashboard_copy, dashboard_name, dry_run)
    if vars_migrated > 0:
        print(f"  Migrated {vars_migrated} template variable(s)")

    # Write back if not dry-run and there were changes
    if not dry_run and records:
        with open(file_path, "w") as f:
            json.dump(dashboard_copy, f, indent=2)
            f.write("\n")

    return report


# ---------------------------------------------------------------------------
# Reporting
# ---------------------------------------------------------------------------

def print_report(reports: list[DashboardReport], dry_run: bool) -> None:
    """Print a summary of all migrations."""
    mode = "DRY RUN" if dry_run else "MIGRATION"
    print(f"\n{'='*70}")
    print(f"  {mode} SUMMARY")
    print(f"{'='*70}")

    total_panels = 0
    total_targets = 0
    all_metrics: set[str] = set()

    for report in reports:
        if not report.records:
            print(f"\n  {report.file_name}: no Prometheus panels to migrate")
            continue

        panel_count = len(set(r.panel_title for r in report.records))
        target_count = len(report.records)
        total_panels += panel_count
        total_targets += target_count

        print(f"\n  {report.file_name}: {panel_count} panels, {target_count} targets migrated")
        if report.panels_skipped > 0:
            print(f"    ({report.panels_skipped} panel(s) could not be converted)")

        for rec in report.records:
            all_metrics.add(rec.metric_name)
            print(f"    [{rec.reason}] '{rec.panel_title}' target[{rec.target_idx}]")
            print(f"      PromQL: {rec.original_expr[:80]}{'...' if len(rec.original_expr) > 80 else ''}")
            print(f"      SQL:    {rec.sql[:80]}{'...' if len(rec.sql) > 80 else ''}")

    print(f"\n{'─'*70}")
    print(f"  Total: {total_panels} panels, {total_targets} targets across {len(reports)} dashboards")
    print(f"  Unique TSDB metric names: {len(all_metrics)}")
    if all_metrics:
        for m in sorted(all_metrics):
            print(f"    - {m}")
    print(f"{'='*70}\n")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    parser = argparse.ArgumentParser(
        description="Migrate Grafana dashboard panels from Prometheus to TimescaleDB SQL."
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would change without modifying files",
    )
    parser.add_argument(
        "--dashboard",
        type=str,
        default=None,
        help="Process a single dashboard file (e.g. mdemg-overview.json)",
    )
    parser.add_argument(
        "--dashboard-dir",
        type=str,
        default=None,
        help=f"Dashboard directory (default: {DASHBOARD_DIR})",
    )
    args = parser.parse_args()

    dashboard_dir = Path(args.dashboard_dir) if args.dashboard_dir else DASHBOARD_DIR

    if not dashboard_dir.is_dir():
        print(f"ERROR: Dashboard directory not found: {dashboard_dir}", file=sys.stderr)
        return 1

    # Collect dashboard files
    if args.dashboard:
        target_file = dashboard_dir / args.dashboard
        if not target_file.exists():
            print(f"ERROR: Dashboard file not found: {target_file}", file=sys.stderr)
            return 1
        files = [target_file]
    else:
        files = sorted(dashboard_dir.glob("*.json"))

    if not files:
        print("No dashboard files found.", file=sys.stderr)
        return 1

    mode_str = "[DRY RUN] " if args.dry_run else ""
    print(f"{mode_str}Processing {len(files)} dashboard file(s) from {dashboard_dir}\n")

    reports: list[DashboardReport] = []
    for file_path in files:
        print(f"Processing: {file_path.name}")
        report = process_dashboard(file_path, dry_run=args.dry_run)
        reports.append(report)

    print_report(reports, dry_run=args.dry_run)

    return 0


if __name__ == "__main__":
    sys.exit(main())
