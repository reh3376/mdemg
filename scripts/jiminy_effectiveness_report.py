#!/usr/bin/env python3
"""Jiminy Guidance Effectiveness Report Generator.

Connects to TimescaleDB, Neo4j, and the MDEMG API to produce a comprehensive
effectiveness analysis of the Jiminy guidance system. Assesses 5 hypotheses
for high ignore rates and produces actionable recommendations.

Usage:
    python scripts/jiminy_effectiveness_report.py
    python scripts/jiminy_effectiveness_report.py --space-id mdemg-dev --days 7
    python scripts/jiminy_effectiveness_report.py --format both --output /tmp/report.md
    python scripts/jiminy_effectiveness_report.py --tsdb-dsn "postgresql://..." --neo4j-url "bolt://..."
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
import urllib.request
from dataclasses import dataclass, field
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any

logger = logging.getLogger(__name__)

# Connection defaults — resolution order: CLI flag > env var > hardcoded default
DEFAULT_TSDB_DSN = os.environ.get(
    "TSDB_DSN", "postgresql://mdemg:mdemg_metrics@localhost:5433/mdemg_metrics"
)
DEFAULT_NEO4J_URL = os.environ.get("NEO4J_URI", "bolt://localhost:7687")
DEFAULT_NEO4J_USER = os.environ.get("NEO4J_USER", "neo4j")
DEFAULT_NEO4J_PASS = os.environ.get("NEO4J_PASS", "testpassword")
DEFAULT_MDEMG_URL = os.environ.get("MDEMG_URL", "http://localhost:9999")
DEFAULT_SPACE_ID = "mdemg-dev"
DEFAULT_DAYS = 7

# Classifier thresholds (must match outcome_classifier.go)
HIGH_THRESHOLD = 0.7
LOW_THRESHOLD = 0.3
HEURISTIC_THRESHOLD = 0.5


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


# ─── Section A: Outcome Distribution (Q4, Q6) ───


def section_a_outcome_distribution(neo4j_session: Any, days: int, space_id: str) -> SectionResult:
    """Overall outcome distribution and breakdown by obs_type."""
    sr = SectionResult(name="outcome_distribution")

    # Q4: Overall distribution
    result = neo4j_session.run(
        "MATCH ()-[r:GUIDANCE_OUTCOME]->() "
        "RETURN r.outcome_type AS outcome, count(r) AS count, "
        "round(avg(r.similarity) * 100) / 100.0 AS avg_similarity "
        "ORDER BY count DESC"
    )
    dist = [dict(rec) for rec in result]
    sr.data["distribution"] = dist

    total = sum(r["count"] for r in dist)
    sr.data["total_edges"] = total

    ignored_count = next((r["count"] for r in dist if r["outcome"] == "ignored"), 0)
    followed_count = next((r["count"] for r in dist if r["outcome"] == "followed"), 0)
    sr.data["ignore_rate"] = pct(ignored_count, total)
    sr.data["follow_rate"] = pct(followed_count, total)

    if sr.data["ignore_rate"] > 70:
        sr.findings.append(Finding("critical", sr.name,
                                   f"Ignore rate is {sr.data['ignore_rate']}% — guidance system ineffective"))
    if not any(r["outcome"] == "partial_compliance" for r in dist):
        sr.findings.append(Finding("warning", sr.name,
                                   "Zero partial_compliance outcomes — heuristic fallback produces binary classification"))

    # Q6: By obs_type
    result = neo4j_session.run(
        "MATCH (src:MemoryNode)-[r:GUIDANCE_OUTCOME]->(src) "
        "RETURN src.obs_type AS obs_type, r.outcome_type AS outcome, count(r) AS count "
        "ORDER BY count DESC"
    )
    by_type = [dict(rec) for rec in result]
    sr.data["by_obs_type"] = by_type

    null_count = sum(r["count"] for r in by_type if r["obs_type"] is None)
    if null_count > total * 0.5:
        sr.findings.append(Finding("warning", sr.name,
                                   f"{pct(null_count, total)}% of outcomes are on untyped MemoryNodes (not constraints)"))

    return sr


# ─── Section B: Outcome Trends (Q5) ───


def section_b_outcome_trends(neo4j_session: Any, days: int) -> SectionResult:
    """Daily outcome trends."""
    sr = SectionResult(name="outcome_trends")

    result = neo4j_session.run(
        "MATCH ()-[r:GUIDANCE_OUTCOME]->() "
        "WITH r, substring(r.created_at, 0, 10) AS day "
        "RETURN day, r.outcome_type AS outcome, count(r) AS count "
        "ORDER BY day, outcome"
    )
    trends = [dict(rec) for rec in result]
    sr.data["daily_trends"] = trends

    # Check if ignore rate is increasing
    days_data: dict[str, dict[str, int]] = {}
    for row in trends:
        d = row["day"]
        if d not in days_data:
            days_data[d] = {}
        days_data[d][row["outcome"]] = row["count"]

    sr.data["days_with_data"] = len(days_data)

    if len(days_data) >= 3:
        sorted_days = sorted(days_data.keys())
        first_half = sorted_days[:len(sorted_days) // 2]
        second_half = sorted_days[len(sorted_days) // 2:]

        def ignore_rate_for_days(day_list: list[str]) -> float:
            ign = sum(days_data[d].get("ignored", 0) for d in day_list)
            tot = sum(sum(days_data[d].values()) for d in day_list)
            return pct(ign, tot)

        early_rate = ignore_rate_for_days(first_half)
        late_rate = ignore_rate_for_days(second_half)
        sr.data["early_ignore_rate"] = early_rate
        sr.data["late_ignore_rate"] = late_rate

        if late_rate > early_rate + 10:
            sr.data["trend"] = "increasing"
            sr.findings.append(Finding("warning", sr.name, f"Ignore rate increasing: {early_rate}% → {late_rate}%"))
        elif early_rate > late_rate + 10:
            sr.data["trend"] = "decreasing"
        else:
            sr.data["trend"] = "stable"

    return sr


# ─── Section C: TSDB Guidance Correlation (Q1-Q3) ───


def section_c_tsdb_correlation(cur: Any, days: int, space_id: str) -> SectionResult:
    """TSDB guidance_id correlation rate in llm_interactions."""
    sr = SectionResult(name="tsdb_correlation")

    # Q1: Overall
    cur.execute(
        "SELECT COUNT(*) AS total, COUNT(guidance_id) AS guided, "
        "ROUND(COUNT(guidance_id)::numeric / NULLIF(COUNT(*), 0) * 100, 2) AS pct "
        "FROM llm_interactions WHERE space_id = %s",
        (space_id,),
    )
    cols = [d[0] for d in cur.description]
    row = cur.fetchone()
    sr.data["overall"] = dict(zip(cols, [float(v) if isinstance(v, Decimal) else v for v in row]))

    # Q2: By task_name
    cur.execute(
        "SELECT task_name, COUNT(*) AS total, COUNT(guidance_id) AS guided "
        "FROM llm_interactions WHERE space_id = %s "
        "GROUP BY task_name ORDER BY total DESC",
        (space_id,),
    )
    cols = [d[0] for d in cur.description]
    sr.data["by_task"] = [dict(zip(cols, row)) for row in cur.fetchall()]

    # Q3: Daily trend
    cur.execute(
        "SELECT time_bucket('1 day', time) AS day, COUNT(*) AS total, COUNT(guidance_id) AS guided "
        "FROM llm_interactions WHERE space_id = %s GROUP BY day ORDER BY day",
        (space_id,),
    )
    cols = [d[0] for d in cur.description]
    sr.data["daily"] = [
        {k: (v.isoformat() if isinstance(v, datetime) else v) for k, v in zip(cols, row)}
        for row in cur.fetchall()
    ]

    return sr


# ─── Section D: Constraint Effectiveness (Q7-Q9) ───


def section_d_constraint_effectiveness(neo4j_session: Any, days: int, space_id: str) -> SectionResult:
    """Most-ignored and most-followed constraint codes, plus zero-feedback nodes."""
    sr = SectionResult(name="constraint_effectiveness")

    # Q7: Most ignored
    result = neo4j_session.run(
        'MATCH (src:MemoryNode)-[r:GUIDANCE_OUTCOME {outcome_type: "ignored"}]->(src) '
        "WHERE src.constraint_code IS NOT NULL AND src.constraint_code <> '' "
        "RETURN src.constraint_code AS code, count(r) AS ignore_count, "
        "round(avg(r.similarity) * 100) / 100.0 AS avg_sim "
        "ORDER BY ignore_count DESC LIMIT 20"
    )
    sr.data["most_ignored"] = [dict(rec) for rec in result]

    # Q8: Most followed
    result = neo4j_session.run(
        'MATCH (src:MemoryNode)-[r:GUIDANCE_OUTCOME {outcome_type: "followed"}]->(src) '
        "WHERE src.constraint_code IS NOT NULL AND src.constraint_code <> '' "
        "RETURN src.constraint_code AS code, count(r) AS follow_count, "
        "round(avg(r.similarity) * 100) / 100.0 AS avg_sim "
        "ORDER BY follow_count DESC LIMIT 20"
    )
    sr.data["most_followed"] = [dict(rec) for rec in result]

    # Q9: Zero-feedback constraint/correction nodes
    result = neo4j_session.run(
        f'MATCH (src:MemoryNode {{space_id: "{space_id}"}}) '
        'WHERE src.obs_type IN ["constraint", "correction"] '
        "AND NOT (src)-[:GUIDANCE_OUTCOME]->() "
        "RETURN src.obs_type AS type, count(src) AS no_feedback_count"
    )
    sr.data["zero_feedback"] = [dict(rec) for rec in result]

    total_zero = sum(r["no_feedback_count"] for r in sr.data["zero_feedback"])
    if total_zero > 10:
        sr.findings.append(Finding("warning", sr.name,
                                   f"{total_zero} constraint/correction nodes have never received feedback"))

    # Q17: Never-followed constraints
    result = neo4j_session.run(
        f'MATCH (n:MemoryNode {{space_id: "{space_id}"}})-[r:GUIDANCE_OUTCOME]->(n) '
        'WHERE n.obs_type IN ["constraint", "correction"] '
        "WITH n, collect(r.outcome_type) AS outcomes "
        'WHERE NOT "followed" IN outcomes '
        "RETURN n.obs_type AS type, n.constraint_code AS code, "
        "n.content AS content, size(outcomes) AS times_surfaced "
        "ORDER BY size(outcomes) DESC LIMIT 20"
    )
    never = [dict(rec) for rec in result]
    sr.data["never_followed"] = [
        {**r, "content": (r["content"] or "")[:120]} for r in never
    ]

    if len(never) > 0:
        sr.findings.append(Finding("info", sr.name,
                                   f"{len(never)} constraints have been surfaced but never followed"))

    return sr


# ─── Section E: Similarity Distribution (Q12-Q13) ───


def section_e_similarity_distribution(neo4j_session: Any, days: int) -> SectionResult:
    """Similarity score distribution by outcome type, with false-negative analysis."""
    sr = SectionResult(name="similarity_distribution")

    # Q12: Percentiles
    result = neo4j_session.run(
        "MATCH ()-[r:GUIDANCE_OUTCOME]->() "
        "RETURN r.outcome_type AS outcome, count(r) AS count, "
        "round(min(r.similarity) * 100) / 100.0 AS min_sim, "
        "round(avg(r.similarity) * 100) / 100.0 AS avg_sim, "
        "round(max(r.similarity) * 100) / 100.0 AS max_sim, "
        "round(percentileCont(r.similarity, 0.25) * 100) / 100.0 AS p25, "
        "round(percentileCont(r.similarity, 0.50) * 100) / 100.0 AS p50, "
        "round(percentileCont(r.similarity, 0.75) * 100) / 100.0 AS p75 "
        "ORDER BY outcome"
    )
    dist = [dict(rec) for rec in result]
    sr.data["percentiles"] = dist

    # Analyze the "ignored" bucket
    ignored = next((r for r in dist if r["outcome"] == "ignored"), None)
    if ignored:
        sr.data["ignored_avg_sim"] = ignored["avg_sim"]
        sr.data["ignored_p75"] = ignored["p75"]

        if ignored["p75"] > LOW_THRESHOLD:
            in_uncertain = ignored["p75"]
            sr.findings.append(Finding("critical", sr.name,
                                       f"25%+ of 'ignored' items have similarity > {LOW_THRESHOLD} (p75={in_uncertain}) — "
                                       "these are potential false negatives"))

    # Bucket analysis: how many items fall in each threshold range
    result = neo4j_session.run(
        "MATCH ()-[r:GUIDANCE_OUTCOME]->() "
        f"RETURN CASE "
        f"  WHEN r.similarity >= {HIGH_THRESHOLD} THEN 'high (>={HIGH_THRESHOLD})' "
        f"  WHEN r.similarity >= {HEURISTIC_THRESHOLD} THEN 'mid-high ({HEURISTIC_THRESHOLD}-{HIGH_THRESHOLD})' "
        f"  WHEN r.similarity >= {LOW_THRESHOLD} THEN 'mid-low ({LOW_THRESHOLD}-{HEURISTIC_THRESHOLD})' "
        f"  ELSE 'low (<{LOW_THRESHOLD})' "
        f"END AS bucket, "
        "r.outcome_type AS outcome, count(r) AS count "
        "ORDER BY bucket, outcome"
    )
    sr.data["threshold_buckets"] = [dict(rec) for rec in result]

    return sr


# ─── Section F: Signal Learner Health (Q12a) ───


def section_f_signal_health(neo4j_session: Any) -> SectionResult:
    """SignalState node health — are signals being suppressed?"""
    sr = SectionResult(name="signal_health")

    result = neo4j_session.run(
        "MATCH (s:SignalState) RETURN s.node_id AS signal, s.data AS state_json ORDER BY s.node_id"
    )
    states = [dict(rec) for rec in result]
    sr.data["signal_count"] = len(states)

    if len(states) == 0:
        sr.findings.append(Finding("critical", sr.name,
                                   "No SignalState nodes found — signal learner persistence is not functioning"))
        return sr

    parsed = []
    for state in states:
        try:
            data = json.loads(state["state_json"]) if isinstance(state["state_json"], str) else state["state_json"]
            parsed.append({
                "signal": state["signal"],
                "emissions": data.get("Emissions", 0),
                "responses": data.get("Responses", 0),
                "strength": data.get("Strength", 0),
            })
        except (json.JSONDecodeError, TypeError):
            parsed.append({"signal": state["signal"], "error": "parse failed"})

    sr.data["signals"] = parsed

    for p in parsed:
        if "error" in p:
            continue
        if p["strength"] < 0.1:
            sr.findings.append(Finding("warning", sr.name,
                                       f"Signal {p['signal']} has strength {p['strength']:.4f} — near suppression"))
        if p["emissions"] > 0 and p["responses"] == 0:
            sr.findings.append(Finding("warning", sr.name,
                                       f"Signal {p['signal']} has {p['emissions']} emissions but 0 responses"))

    return sr


# ─── Section G: TSDB Metrics (Q10-Q11) ───


def section_g_tsdb_metrics(cur: Any, days: int, space_id: str) -> SectionResult:
    """Jiminy-related TSDB metric samples."""
    sr = SectionResult(name="tsdb_metrics")

    # Q10: Metric names
    cur.execute(
        "SELECT DISTINCT metric_name FROM metric_samples "
        "WHERE space_id = %s AND (metric_name LIKE '%%jiminy%%' OR metric_name LIKE '%%guidance%%' "
        "OR metric_name LIKE '%%follow%%' OR metric_name LIKE '%%ignore%%') "
        "ORDER BY metric_name",
        (space_id,),
    )
    sr.data["metric_names"] = [row[0] for row in cur.fetchall()]
    sr.data["metric_count"] = len(sr.data["metric_names"])

    # Q11: Daily values for key metrics
    cur.execute(
        "SELECT time_bucket('1 day', time) AS day, metric_name, "
        "avg(value) AS avg_value, count(*) AS samples "
        "FROM metric_samples WHERE space_id = %s "
        "AND (metric_name LIKE '%%jiminy%%' OR metric_name LIKE '%%guidance%%') "
        "GROUP BY day, metric_name ORDER BY day DESC, metric_name",
        (space_id,),
    )
    cols = [d[0] for d in cur.description]
    sr.data["daily_metrics"] = [
        {k: (v.isoformat() if isinstance(v, datetime) else (float(v) if isinstance(v, Decimal) else v))
         for k, v in zip(cols, row)}
        for row in cur.fetchall()
    ]

    return sr


# ─── Section H: API Health ───


def section_h_api_health(mdemg_url: str, space_id: str) -> SectionResult:
    """Jiminy API endpoint health and live metrics."""
    sr = SectionResult(name="api_health")

    endpoints = {
        "healthz": f"{mdemg_url}/v1/jiminy/healthz",
        "protocol_metrics": f"{mdemg_url}/v1/jiminy/protocol/metrics",
        "tier_effectiveness": f"{mdemg_url}/v1/jiminy/protocol/tier-effectiveness",
        "protocol_status": f"{mdemg_url}/v1/jiminy/protocol/status?session_id=claude-core",
    }

    for name, url in endpoints.items():
        try:
            req = urllib.request.Request(url, method="GET")
            with urllib.request.urlopen(req, timeout=10) as resp:
                data = json.loads(resp.read().decode())
                sr.data[name] = data
        except Exception as e:
            sr.data[name] = {"error": str(e)}
            sr.findings.append(Finding("warning", sr.name, f"API {name} failed: {e}"))

    # Check trust score
    status = sr.data.get("protocol_status", {}).get("data", {})
    trust = status.get("trust_score")
    if trust is not None and trust < 0.3:
        sr.findings.append(Finding("warning", sr.name,
                                   f"Trust score is {trust:.2f} — very low, may affect guidance quality"))

    last_feedback = status.get("last_feedback_at")
    if last_feedback:
        sr.data["last_feedback_at"] = last_feedback

    return sr


# ─── Section I: Feedback Loop Health ───


def section_i_feedback_loop(cur: Any, neo4j_session: Any, days: int, space_id: str) -> SectionResult:
    """Feedback loop integrity: cooldown suppression, TTL expiration estimates."""
    sr = SectionResult(name="feedback_loop")

    # Cooldown suppression estimate
    cur.execute(
        "SELECT avg(cnt) AS avg_per_minute, max(cnt) AS max_per_minute, count(*) AS total_minutes "
        "FROM (SELECT time_bucket('1 minute', time) AS minute, count(*) AS cnt "
        "FROM llm_interactions WHERE space_id = %s GROUP BY minute) t",
        (space_id,),
    )
    row = cur.fetchone()
    avg_per_min = float(row[0]) if row[0] else 0
    sr.data["avg_tool_uses_per_minute"] = round(avg_per_min, 1)
    sr.data["max_tool_uses_per_minute"] = row[1]
    sr.data["active_minutes"] = row[2]

    # With 30s cooldown, max ~2 feedback per minute
    if avg_per_min > 2:
        suppression = round((1 - 2 / avg_per_min) * 100, 0)
        sr.data["cooldown_suppression_pct"] = suppression
        if suppression > 50:
            sr.findings.append(Finding("warning", sr.name,
                                       f"Estimated {suppression}% of tool uses are suppressed by 30s cooldown"))
    else:
        sr.data["cooldown_suppression_pct"] = 0

    # Zero-feedback count from constraint/correction nodes
    result = neo4j_session.run(
        f'MATCH (n:MemoryNode {{space_id: "{space_id}"}}) '
        'WHERE n.obs_type IN ["constraint", "correction"] '
        "RETURN count(n) AS total"
    )
    total_typed = result.single()["total"]

    result = neo4j_session.run(
        f'MATCH (n:MemoryNode {{space_id: "{space_id}"}}) '
        'WHERE n.obs_type IN ["constraint", "correction"] '
        "AND NOT (n)-[:GUIDANCE_OUTCOME]->() "
        "RETURN count(n) AS no_feedback"
    )
    no_fb = result.single()["no_feedback"]
    sr.data["typed_nodes_total"] = total_typed
    sr.data["typed_nodes_no_feedback"] = no_fb
    sr.data["feedback_coverage_pct"] = pct(total_typed - no_fb, total_typed)

    if no_fb > total_typed * 0.5:
        sr.findings.append(Finding("warning", sr.name,
                                   f"{no_fb}/{total_typed} typed guidance nodes have never received feedback"))

    return sr


# ─── Hypothesis Assessor ───


def assess_hypotheses(sections: list[SectionResult]) -> list[dict[str, Any]]:
    """Assign probability levels to each hypothesis based on section data."""
    hypotheses = [
        {"id": "H1", "name": "Guidance irrelevant", "level": "MEDIUM", "evidence": []},
        {"id": "H2", "name": "Guidance obvious", "level": "LOW", "evidence": []},
        {"id": "H3", "name": "Guidance too late", "level": "MEDIUM", "evidence": []},
        {"id": "H4", "name": "Guidance not seen", "level": "LOW", "evidence": []},
        {"id": "H5", "name": "Measurement wrong", "level": "HIGH", "evidence": []},
    ]

    sd = {s.name: s for s in sections}

    # H1: irrelevant
    outcome = sd.get("outcome_distribution")
    if outcome:
        by_type = outcome.data.get("by_obs_type", [])
        null_pct = sum(r["count"] for r in by_type if r["obs_type"] is None)
        total = outcome.data.get("total_edges", 1)
        if null_pct > total * 0.8:
            hypotheses[0]["level"] = "MEDIUM-HIGH"
            hypotheses[0]["evidence"].append(f"{pct(null_pct, total)}% outcomes on untyped nodes")

    # H3: too late
    api = sd.get("api_health")
    if api:
        last_fb = api.data.get("last_feedback_at")
        if last_fb:
            hypotheses[2]["evidence"].append(f"Last feedback: {last_fb}")

    # H5: measurement wrong
    sim = sd.get("similarity_distribution")
    if sim:
        ignored_p75 = sim.data.get("ignored_p75", 0)
        if ignored_p75 > LOW_THRESHOLD:
            hypotheses[4]["level"] = "HIGH"
            hypotheses[4]["evidence"].append(f"25% of 'ignored' have similarity > {LOW_THRESHOLD}")

    outcome = sd.get("outcome_distribution")
    if outcome:
        dist = outcome.data.get("distribution", [])
        if not any(r["outcome"] == "partial_compliance" for r in dist):
            hypotheses[4]["evidence"].append("Zero partial_compliance — binary heuristic fallback")

    signal = sd.get("signal_health")
    if signal and signal.data.get("signal_count", 0) == 0:
        hypotheses[4]["evidence"].append("SignalState nodes absent — signal persistence broken")

    fb = sd.get("feedback_loop")
    if fb:
        supp = fb.data.get("cooldown_suppression_pct", 0)
        if supp > 40:
            hypotheses[4]["evidence"].append(f"{supp}% tool uses suppressed by cooldown")

    return hypotheses


# ─── Report Formatters ───


def format_markdown_report(sections: list[SectionResult], hypotheses: list[dict],
                           metadata: dict[str, Any]) -> str:
    """Format sections into a markdown effectiveness report."""
    lines = [
        "# Jiminy Guidance Effectiveness Report",
        "",
        f"**Generated:** {metadata.get('generated', 'unknown')}",
        f"**Space:** {metadata.get('space_id', 'unknown')}",
        f"**Window:** {metadata.get('days', '?')} days",
        "",
    ]

    sd = {s.name: s for s in sections}

    # Outcome Distribution
    outcome = sd.get("outcome_distribution")
    if outcome:
        lines.extend([
            "## Outcome Distribution", "",
            f"**Total GUIDANCE_OUTCOME edges:** {outcome.data.get('total_edges', 0)}", "",
            "| Outcome | Count | Avg Similarity |",
            "|---------|-------|---------------|",
        ])
        for r in outcome.data.get("distribution", []):
            lines.append(f"| {r['outcome']} | {r['count']} | {r.get('avg_similarity', '?')} |")
        lines.extend(["",
                       f"**Ignore rate:** {outcome.data.get('ignore_rate', '?')}%",
                       f"**Follow rate:** {outcome.data.get('follow_rate', '?')}%", ""])

    # Trends
    trends = sd.get("outcome_trends")
    if trends:
        lines.extend(["## Trends", ""])
        direction = trends.data.get("trend", "unknown")
        lines.append(f"**Trend direction:** {direction}")
        early = trends.data.get("early_ignore_rate")
        late = trends.data.get("late_ignore_rate")
        if early is not None:
            lines.append(f"**Early ignore rate:** {early}% → **Late ignore rate:** {late}%")
        lines.extend(["",
                       "| Day | Outcome | Count |",
                       "|-----|---------|-------|"])
        for r in trends.data.get("daily_trends", []):
            lines.append(f"| {r['day']} | {r['outcome']} | {r['count']} |")
        lines.append("")

    # Similarity Distribution
    sim = sd.get("similarity_distribution")
    if sim:
        lines.extend(["## Similarity Distribution", "",
                       "| Outcome | Count | Min | P25 | P50 | P75 | Max | Avg |",
                       "|---------|-------|-----|-----|-----|-----|-----|-----|"])
        for r in sim.data.get("percentiles", []):
            lines.append(f"| {r['outcome']} | {r['count']} | {r.get('min_sim', '?')} | "
                         f"{r.get('p25', '?')} | {r.get('p50', '?')} | {r.get('p75', '?')} | "
                         f"{r.get('max_sim', '?')} | {r.get('avg_sim', '?')} |")
        lines.append("")

        buckets = sim.data.get("threshold_buckets", [])
        if buckets:
            lines.extend(["### Threshold Buckets", "",
                           "| Bucket | Outcome | Count |",
                           "|--------|---------|-------|"])
            for r in buckets:
                lines.append(f"| {r['bucket']} | {r['outcome']} | {r['count']} |")
            lines.append("")

    # Constraint Effectiveness
    ce = sd.get("constraint_effectiveness")
    if ce:
        lines.extend(["## Constraint Effectiveness", ""])
        never = ce.data.get("never_followed", [])
        if never:
            lines.extend(["### Never-Followed Constraints", "",
                           "| Type | Code | Times Surfaced | Content |",
                           "|------|------|---------------|---------|"])
            for r in never:
                lines.append(f"| {r['type']} | {r.get('code', '-')} | {r['times_surfaced']} | {r.get('content', '')[:80]} |")
            lines.append("")

    # Signal Health
    signal = sd.get("signal_health")
    if signal:
        lines.extend(["## Signal Learner Health", "",
                       f"**SignalState nodes:** {signal.data.get('signal_count', 0)}", ""])
        for s in signal.data.get("signals", []):
            lines.append(f"- {s['signal']}: strength={s.get('strength', '?')}, "
                         f"emissions={s.get('emissions', '?')}, responses={s.get('responses', '?')}")
        lines.append("")

    # Feedback Loop
    fb = sd.get("feedback_loop")
    if fb:
        lines.extend(["## Feedback Loop Health", "",
                       f"- Avg tool uses/minute: {fb.data.get('avg_tool_uses_per_minute', '?')}",
                       f"- Cooldown suppression: {fb.data.get('cooldown_suppression_pct', '?')}%",
                       f"- Typed guidance nodes: {fb.data.get('typed_nodes_total', '?')} total, "
                       f"{fb.data.get('typed_nodes_no_feedback', '?')} without feedback",
                       f"- Feedback coverage: {fb.data.get('feedback_coverage_pct', '?')}%", ""])

    # API Health
    api = sd.get("api_health")
    if api:
        status = api.data.get("protocol_status", {}).get("data", {})
        lines.extend(["## API Health", "",
                       f"- Trust score: {status.get('trust_score', '?')}",
                       f"- Current tier: {status.get('tier', '?')}",
                       f"- Feedback count: {status.get('feedback_count', '?')}",
                       f"- Last feedback: {status.get('last_feedback_at', '?')}", ""])

    # Hypothesis Assessment
    lines.extend(["## Hypothesis Assessment", "",
                   "| Hypothesis | Assessment | Evidence |",
                   "|------------|-----------|----------|"])
    for h in hypotheses:
        evidence = "; ".join(h["evidence"]) if h["evidence"] else "insufficient data"
        lines.append(f"| {h['id']}: {h['name']} | **{h['level']}** | {evidence} |")
    lines.append("")

    # Findings Summary
    all_findings = []
    for s in sections:
        all_findings.extend(s.findings)

    if all_findings:
        lines.extend(["## Findings", ""])
        for f in all_findings:
            icon = {"critical": "!!!", "warning": "!", "info": "-"}
            lines.append(f"- [{f.level}] **{f.section}**: {f.message}")
        lines.append("")

    return "\n".join(lines)


def format_json_report(sections: list[SectionResult], hypotheses: list[dict],
                       metadata: dict[str, Any]) -> str:
    """Format sections as JSON."""
    report = {
        "metadata": metadata,
        "sections": {s.name: {"data": s.data, "findings": [_to_serializable(f) for f in s.findings]}
                     for s in sections},
        "hypotheses": hypotheses,
    }
    return json.dumps(report, indent=2, cls=JSONEncoder, default=str)


# ─── CLI ───


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Jiminy Guidance Effectiveness Report — diagnostic analysis of guidance follow/ignore rates",
    )
    parser.add_argument("--space-id", default=DEFAULT_SPACE_ID,
                        help=f"MDEMG space ID (default: {DEFAULT_SPACE_ID})")
    parser.add_argument("--days", type=int, default=DEFAULT_DAYS,
                        help=f"Analysis window in days (default: {DEFAULT_DAYS})")
    parser.add_argument("--tsdb-dsn", default=DEFAULT_TSDB_DSN,
                        help=f"TSDB connection string (default: {_mask_dsn(DEFAULT_TSDB_DSN)})")
    parser.add_argument("--neo4j-url", default=DEFAULT_NEO4J_URL,
                        help=f"Neo4j bolt URL (default: {DEFAULT_NEO4J_URL})")
    parser.add_argument("--neo4j-user", default=DEFAULT_NEO4J_USER,
                        help=f"Neo4j username (default: {DEFAULT_NEO4J_USER})")
    parser.add_argument("--neo4j-pass", default=DEFAULT_NEO4J_PASS,
                        help="Neo4j password")
    parser.add_argument("--mdemg-url", default=DEFAULT_MDEMG_URL,
                        help=f"MDEMG API endpoint (default: {DEFAULT_MDEMG_URL})")
    parser.add_argument("--format", default="text", choices=["text", "json", "both"],
                        dest="output_format",
                        help="Output format (default: text)")
    parser.add_argument("--output", default=None,
                        help="Output file path (default: stdout)")
    parser.add_argument("--verbose", action="store_true",
                        help="Show detailed query logging")
    return parser.parse_args(argv)


def connect_tsdb(dsn: str):
    """Connect to TSDB in read-only mode."""
    import psycopg2
    logger.info("Connecting to TSDB: %s", _mask_dsn(dsn))
    conn = psycopg2.connect(dsn)
    conn.set_session(readonly=True)
    return conn


def connect_neo4j(url: str, user: str, password: str):
    """Connect to Neo4j."""
    from neo4j import GraphDatabase
    logger.info("Connecting to Neo4j: %s", url)
    driver = GraphDatabase.driver(url, auth=(user, password))
    driver.verify_connectivity()
    return driver


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    generated = datetime.now(timezone.utc).isoformat()
    metadata = {
        "generated": generated,
        "space_id": args.space_id,
        "days": args.days,
        "tsdb_dsn": _mask_dsn(args.tsdb_dsn),
        "neo4j_url": args.neo4j_url,
    }

    # Connect to databases
    try:
        tsdb_conn = connect_tsdb(args.tsdb_dsn)
    except Exception as e:
        print(f"ERROR: Cannot connect to TSDB: {e}", file=sys.stderr)
        return 1

    try:
        neo4j_driver = connect_neo4j(args.neo4j_url, args.neo4j_user, args.neo4j_pass)
    except Exception as e:
        tsdb_conn.close()
        print(f"ERROR: Cannot connect to Neo4j: {e}", file=sys.stderr)
        return 1

    cur = tsdb_conn.cursor()
    neo4j_session = neo4j_driver.session()
    sections: list[SectionResult] = []

    section_funcs = [
        ("A", "outcome_distribution", lambda: section_a_outcome_distribution(neo4j_session, args.days, args.space_id)),
        ("B", "outcome_trends", lambda: section_b_outcome_trends(neo4j_session, args.days)),
        ("C", "tsdb_correlation", lambda: section_c_tsdb_correlation(cur, args.days, args.space_id)),
        ("D", "constraint_effectiveness", lambda: section_d_constraint_effectiveness(neo4j_session, args.days, args.space_id)),
        ("E", "similarity_distribution", lambda: section_e_similarity_distribution(neo4j_session, args.days)),
        ("F", "signal_health", lambda: section_f_signal_health(neo4j_session)),
        ("G", "tsdb_metrics", lambda: section_g_tsdb_metrics(cur, args.days, args.space_id)),
        ("H", "api_health", lambda: section_h_api_health(args.mdemg_url, args.space_id)),
        ("I", "feedback_loop", lambda: section_i_feedback_loop(cur, neo4j_session, args.days, args.space_id)),
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

    # Clean up
    cur.close()
    tsdb_conn.close()
    neo4j_session.close()
    neo4j_driver.close()

    # Assess hypotheses
    hypotheses = assess_hypotheses(sections)

    # Generate output
    if args.output_format in ("text", "both"):
        text = format_markdown_report(sections, hypotheses, metadata)
        if args.output and args.output_format == "text":
            with open(args.output, "w") as f:
                f.write(text)
            print(f"Report written to {args.output}")
        else:
            print(text)

    if args.output_format in ("json", "both"):
        json_str = format_json_report(sections, hypotheses, metadata)
        out_path = args.output or f"jiminy_effectiveness_{datetime.now().strftime('%Y-%m-%d')}.json"
        if args.output_format == "json":
            with open(out_path, "w") as f:
                f.write(json_str)
            print(f"JSON report written to {out_path}")
        elif args.output_format == "both":
            json_path = (args.output or "jiminy_effectiveness") + ".json"
            with open(json_path, "w") as f:
                f.write(json_str)

    return 0


if __name__ == "__main__":
    sys.exit(main())
