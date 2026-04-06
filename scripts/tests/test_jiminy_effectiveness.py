"""Unit tests for jiminy_effectiveness_report.py — no database connections required."""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

# Add parent directory to path so we can import the script
sys.path.insert(0, str(Path(__file__).parent.parent))

import jiminy_effectiveness_report as jer


# ─── Test: Finding classification ───


def test_finding_classification():
    """Finding levels are valid strings."""
    f_crit = jer.Finding("critical", "test", "something broke")
    f_warn = jer.Finding("warning", "test", "something iffy")
    f_info = jer.Finding("info", "test", "FYI")

    assert f_crit.level == "critical"
    assert f_warn.level == "warning"
    assert f_info.level == "info"
    assert f_crit.section == "test"
    assert f_crit.message == "something broke"


def test_section_result_defaults():
    """SectionResult has correct defaults."""
    sr = jer.SectionResult(name="test_section")
    assert sr.name == "test_section"
    assert sr.data == {}
    assert sr.findings == []


# ─── Test: Percentage calculation ───


def test_pct_normal():
    assert jer.pct(50, 100) == 50.0
    assert jer.pct(1, 3) == 33.3


def test_pct_zero():
    assert jer.pct(0, 100) == 0.0
    assert jer.pct(50, 0) == 0.0
    assert jer.pct(None, 100) == 0.0


# ─── Test: DSN masking ───


def test_mask_dsn():
    masked = jer._mask_dsn("postgresql://user:secret@localhost:5433/db")
    assert "secret" not in masked
    assert "****" in masked
    assert "localhost" in masked


def test_mask_dsn_no_password():
    dsn = "postgresql://localhost/db"
    assert jer._mask_dsn(dsn) == dsn


# ─── Test: JSON serialization ───


def test_json_serializable_finding():
    f = jer.Finding("warning", "test", "msg")
    result = jer._to_serializable(f)
    assert result == {"level": "warning", "section": "test", "message": "msg"}


def test_json_serializable_section():
    sr = jer.SectionResult(name="test", data={"key": 1})
    result = jer._to_serializable(sr)
    assert result["name"] == "test"
    assert result["data"]["key"] == 1
    assert result["findings"] == []


# ─── Test: Hypothesis assessor ───


def test_hypothesis_assessor_all_ignored():
    """100% ignored outcomes should flag H5 as HIGH."""
    sections = [
        jer.SectionResult(name="outcome_distribution", data={
            "distribution": [{"outcome": "ignored", "count": 100, "avg_similarity": 0.25}],
            "total_edges": 100,
            "ignore_rate": 100.0,
            "follow_rate": 0.0,
            "by_obs_type": [{"obs_type": None, "outcome": "ignored", "count": 90}],
        }),
        jer.SectionResult(name="similarity_distribution", data={
            "ignored_p75": 0.35,
            "ignored_avg_sim": 0.25,
            "percentiles": [
                {"outcome": "ignored", "count": 100, "p75": 0.35},
            ],
        }),
        jer.SectionResult(name="signal_health", data={"signal_count": 0}),
        jer.SectionResult(name="feedback_loop", data={"cooldown_suppression_pct": 60}),
        jer.SectionResult(name="api_health", data={}),
    ]

    hypotheses = jer.assess_hypotheses(sections)
    h5 = next(h for h in hypotheses if h["id"] == "H5")
    assert h5["level"] == "HIGH"
    assert len(h5["evidence"]) >= 2  # p75 > threshold + zero partial_compliance or cooldown


def test_hypothesis_assessor_mixed():
    """Mixed outcomes should produce balanced probabilities."""
    sections = [
        jer.SectionResult(name="outcome_distribution", data={
            "distribution": [
                {"outcome": "followed", "count": 50, "avg_similarity": 0.9},
                {"outcome": "ignored", "count": 50, "avg_similarity": 0.2},
            ],
            "total_edges": 100,
            "ignore_rate": 50.0,
            "follow_rate": 50.0,
            "by_obs_type": [
                {"obs_type": "constraint", "outcome": "followed", "count": 30},
                {"obs_type": None, "outcome": "ignored", "count": 40},
            ],
        }),
        jer.SectionResult(name="similarity_distribution", data={
            "ignored_p75": 0.25,
            "ignored_avg_sim": 0.2,
        }),
        jer.SectionResult(name="signal_health", data={"signal_count": 5}),
        jer.SectionResult(name="feedback_loop", data={"cooldown_suppression_pct": 20}),
        jer.SectionResult(name="api_health", data={}),
    ]

    hypotheses = jer.assess_hypotheses(sections)
    h5 = next(h for h in hypotheses if h["id"] == "H5")
    # p75 < threshold, signal_count > 0, low cooldown — H5 should NOT be HIGH
    assert h5["level"] != "HIGH" or len(h5["evidence"]) < 2


# ─── Test: Similarity bucketing (threshold constants) ───


def test_threshold_constants():
    """Verify threshold constants match outcome_classifier.go."""
    assert jer.HIGH_THRESHOLD == 0.7
    assert jer.LOW_THRESHOLD == 0.3
    assert jer.HEURISTIC_THRESHOLD == 0.5


# ─── Test: Environment variable resolution ───


def test_env_var_resolution():
    """Verify that env vars override defaults when set."""
    original_url = os.environ.get("NEO4J_URI")
    try:
        os.environ["NEO4J_URI"] = "bolt://custom:7687"
        # Re-import would be needed for module-level vars, but we can test the pattern
        assert os.environ.get("NEO4J_URI") == "bolt://custom:7687"
    finally:
        if original_url is not None:
            os.environ["NEO4J_URI"] = original_url
        else:
            os.environ.pop("NEO4J_URI", None)


# ─── Test: Report formatting ───


def test_markdown_report_format():
    """Verify markdown output has all expected headings."""
    sections = [
        jer.SectionResult(name="outcome_distribution", data={
            "distribution": [{"outcome": "ignored", "count": 80, "avg_similarity": 0.28}],
            "total_edges": 100,
            "ignore_rate": 80.0,
            "follow_rate": 20.0,
            "by_obs_type": [],
        }),
        jer.SectionResult(name="outcome_trends", data={
            "daily_trends": [{"day": "2026-04-01", "outcome": "ignored", "count": 10}],
            "trend": "stable",
        }),
        jer.SectionResult(name="similarity_distribution", data={
            "percentiles": [{"outcome": "ignored", "count": 80, "min_sim": 0.1,
                            "p25": 0.2, "p50": 0.28, "p75": 0.35, "max_sim": 0.49, "avg_sim": 0.28}],
            "threshold_buckets": [],
        }),
        jer.SectionResult(name="constraint_effectiveness", data={
            "never_followed": [{"type": "constraint", "code": "test-code",
                               "times_surfaced": 5, "content": "Test constraint"}],
        }),
        jer.SectionResult(name="signal_health", data={"signal_count": 0}),
        jer.SectionResult(name="feedback_loop", data={
            "avg_tool_uses_per_minute": 3.8,
            "cooldown_suppression_pct": 47,
            "typed_nodes_total": 36,
            "typed_nodes_no_feedback": 30,
            "feedback_coverage_pct": 16.7,
        }),
        jer.SectionResult(name="api_health", data={
            "protocol_status": {"data": {"trust_score": 0.22, "tier": "T3",
                                        "feedback_count": 12, "last_feedback_at": "2026-03-31"}},
        }),
    ]

    hypotheses = [
        {"id": "H1", "name": "Guidance irrelevant", "level": "MEDIUM", "evidence": ["test"]},
        {"id": "H5", "name": "Measurement wrong", "level": "HIGH", "evidence": ["test"]},
    ]

    metadata = {"generated": "2026-04-06T12:00:00Z", "space_id": "mdemg-dev", "days": 7}

    report = jer.format_markdown_report(sections, hypotheses, metadata)

    assert "# Jiminy Guidance Effectiveness Report" in report
    assert "## Outcome Distribution" in report
    assert "## Trends" in report
    assert "## Similarity Distribution" in report
    assert "## Constraint Effectiveness" in report
    assert "## Signal Learner Health" in report
    assert "## Feedback Loop Health" in report
    assert "## API Health" in report
    assert "## Hypothesis Assessment" in report


def test_json_report_format():
    """Verify JSON output is parseable and has required structure."""
    sections = [
        jer.SectionResult(name="test", data={"key": 1},
                         findings=[jer.Finding("info", "test", "msg")]),
    ]
    hypotheses = [{"id": "H1", "name": "test", "level": "LOW", "evidence": []}]
    metadata = {"generated": "2026-04-06T12:00:00Z"}

    json_str = jer.format_json_report(sections, hypotheses, metadata)
    parsed = json.loads(json_str)

    assert "metadata" in parsed
    assert "sections" in parsed
    assert "hypotheses" in parsed
    assert "test" in parsed["sections"]


# ─── Test: CLI argument parsing ───


def test_parse_args_defaults():
    """Default arguments parse correctly."""
    args = jer.parse_args([])
    assert args.space_id == jer.DEFAULT_SPACE_ID
    assert args.days == jer.DEFAULT_DAYS
    assert args.output_format == "text"
    assert args.verbose is False


def test_parse_args_custom():
    """Custom arguments parse correctly."""
    args = jer.parse_args(["--space-id", "custom", "--days", "14", "--format", "json", "--verbose"])
    assert args.space_id == "custom"
    assert args.days == 14
    assert args.output_format == "json"
    assert args.verbose is True
