"""Unit tests for tsdb_data_review.py — no TSDB connection required."""

from __future__ import annotations

import json
import sys
from pathlib import Path

# Add parent directory to path so we can import the script
sys.path.insert(0, str(Path(__file__).parent.parent))

import tsdb_data_review as tdr


# ─── Test: Finding classification ───

def test_finding_classification():
    """Finding levels are valid strings."""
    f_crit = tdr.Finding("critical", "test", "something broke")
    f_warn = tdr.Finding("warning", "test", "something iffy")
    f_info = tdr.Finding("info", "test", "FYI")

    assert f_crit.level == "critical"
    assert f_warn.level == "warning"
    assert f_info.level == "info"
    assert f_crit.section == "test"
    assert f_crit.message == "something broke"


# ─── Test: Privacy patterns match known sensitive data ───

def test_privacy_patterns_match():
    """All 5 scrubber patterns detect known sensitive data."""
    # API key patterns
    assert tdr.PRIVACY_PATTERNS["api_key"].search("sk-abcdefghijklmnopqrstuvwxyz")
    assert tdr.PRIVACY_PATTERNS["api_key"].search("ghp_abcdefghijklmnopqrstuvwxyz0123456789")
    assert tdr.PRIVACY_PATTERNS["api_key"].search("AKIAIOSFODNN7EXAMPLE")
    assert tdr.PRIVACY_PATTERNS["api_key"].search("Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")

    # Absolute paths
    assert tdr.PRIVACY_PATTERNS["abs_path"].search("/Users/roger/secrets/key.pem")
    assert tdr.PRIVACY_PATTERNS["abs_path"].search("/home/deploy/.ssh/id_rsa")

    # Environment secrets
    assert tdr.PRIVACY_PATTERNS["env_secret"].search("PASSWORD=mysecretpassword")
    assert tdr.PRIVACY_PATTERNS["env_secret"].search("API_KEY: sk-test123")
    assert tdr.PRIVACY_PATTERNS["env_secret"].search('SECRET="very_secret_value"')

    # Email addresses
    assert tdr.PRIVACY_PATTERNS["email"].search("user@example.com")
    assert tdr.PRIVACY_PATTERNS["email"].search("admin+test@company.co.uk")

    # Neo4j credentials
    assert tdr.PRIVACY_PATTERNS["neo4j_cred"].search("neo4j://neo4j:testpassword@localhost:7687")


# ─── Test: Privacy patterns no false positives ───

def test_privacy_patterns_no_false_positives():
    """Normal code content doesn't trigger scrub alerts."""
    normal_texts = [
        "The retrieval pipeline returned 5 results with avg score 0.85",
        "Memory node consolidation completed in 342ms",
        "SELECT count(*) FROM metric_samples WHERE space_id = 'mdemg-dev'",
        "func (s *Service) Retrieve(ctx context.Context) error",
        "T1 compression achieved: 15 tokens from 80 token input",
        "RSIC health score: 0.73 (retrieval: 0.70, memory: 1.00)",
        "constraint_code: AXM-7K2 applied to guidance item",
    ]
    for text in normal_texts:
        for name, pattern in tdr.PRIVACY_PATTERNS.items():
            assert not pattern.search(text), f"False positive: {name} matched '{text}'"


# ─── Test: Report JSON valid ───

def test_report_json_valid():
    """JSON output is parseable and has required structure."""
    sections = [
        tdr.SectionResult(name="schema_health", data={"schema_version": 8, "tables_present": 8}),
        tdr.SectionResult(name="metric_samples", data={"total_rows": 1000, "duration_hours": 24.5}),
    ]
    findings = [
        tdr.Finding("info", "test", "All good"),
        tdr.Finding("warning", "test", "Something to watch"),
    ]

    json_str = tdr.format_json(sections, findings, "2026-04-01T00:00:00Z", 24.5)
    parsed = json.loads(json_str)

    assert "generated" in parsed
    assert "schema_version" in parsed
    assert "collection_duration_hours" in parsed
    assert "sections" in parsed
    assert "findings" in parsed
    assert "summary" in parsed

    assert parsed["schema_version"] == 8
    assert parsed["collection_duration_hours"] == 24.5
    assert parsed["summary"]["critical"] == 0
    assert parsed["summary"]["warning"] == 1
    assert parsed["summary"]["info"] == 1
    assert len(parsed["findings"]) == 2
    assert "schema_health" in parsed["sections"]
    assert "metric_samples" in parsed["sections"]


# ─── Test: Empty table handling ───

def test_empty_table_handling():
    """Zero-row results don't crash (no division by zero)."""
    # pct with zero denominator
    assert tdr.pct(0, 0) == 0.0
    assert tdr.pct(5, 0) == 0.0
    assert tdr.pct(None, 100) == 0.0
    assert tdr.pct(0, None) == 0.0

    # SectionResult with empty data
    s = tdr.SectionResult(name="test")
    assert s.data == {}
    assert s.findings == []

    # Format empty section without crash
    text = tdr._format_section_text(s)
    assert "test" in text.lower()


# ─── Test: Percentage calculation ───

def test_percentage_calculation():
    """Percentage helpers handle edge cases."""
    assert tdr.pct(50, 100) == 50.0
    assert tdr.pct(1, 3) == 33.3
    assert tdr.pct(0, 100) == 0.0
    assert tdr.pct(100, 100) == 100.0
    assert tdr.pct(None, None) == 0.0
    assert tdr.pct(0, 0) == 0.0


# ─── Test: Expected task names ───

def test_expected_task_names():
    """EXPECTED_TASK_NAMES has correct count and known names."""
    assert len(tdr.EXPECTED_TASK_NAMES) == 17

    # Verify some key task names are present
    assert "retrieval.rerank_cross" in tdr.EXPECTED_TASK_NAMES
    assert "hidden.reclassify" in tdr.EXPECTED_TASK_NAMES
    assert "jiminy.evaluate" in tdr.EXPECTED_TASK_NAMES
    assert "consulting.classify" in tdr.EXPECTED_TASK_NAMES
    assert "ape.reflect" in tdr.EXPECTED_TASK_NAMES
    assert "summarize.generate" in tdr.EXPECTED_TASK_NAMES

    # No duplicates
    assert len(tdr.EXPECTED_TASK_NAMES) == len(set(tdr.EXPECTED_TASK_NAMES))


# ─── Test: Expected call sites ───

def test_expected_call_sites():
    """EXPECTED_CALL_SITES has correct count and key sites."""
    assert len(tdr.EXPECTED_CALL_SITES) == 13

    assert "ingest" in tdr.EXPECTED_CALL_SITES
    assert "retrieve" in tdr.EXPECTED_CALL_SITES
    assert "conversation" in tdr.EXPECTED_CALL_SITES

    # No duplicates
    assert len(tdr.EXPECTED_CALL_SITES) == len(set(tdr.EXPECTED_CALL_SITES))


# ─── Test: Mask DSN ───

def test_mask_dsn():
    """Password masking works correctly."""
    assert tdr._mask_dsn("postgresql://user:secret@localhost:5433/db") == "postgresql://user:****@localhost:5433/db"
    assert tdr._mask_dsn("postgresql://user:longpassword@host:5432/db") == "postgresql://user:****@host:5432/db"

    # No password — unchanged
    assert tdr._mask_dsn("postgresql://localhost/db") == "postgresql://localhost/db"

    # Empty string — unchanged
    assert tdr._mask_dsn("") == ""


# ─── Test: Parse args defaults ───

def test_parse_args_defaults():
    """CLI defaults are correct."""
    args = tdr.parse_args([])
    assert args.tsdb_dsn == tdr.DEFAULT_DSN
    assert args.output_format == "text"
    assert args.output is None
    assert args.mdemg_url == tdr.DEFAULT_MDEMG_URL
    assert args.verbose is False


def test_parse_args_custom():
    """CLI custom args are parsed."""
    args = tdr.parse_args([
        "--tsdb-dsn", "postgresql://other:pass@host:5432/db",
        "--format", "json",
        "--output", "/tmp/report.json",
        "--mdemg-url", "http://other:9999",
        "--verbose",
    ])
    assert args.tsdb_dsn == "postgresql://other:pass@host:5432/db"
    assert args.output_format == "json"
    assert args.output == "/tmp/report.json"
    assert args.mdemg_url == "http://other:9999"
    assert args.verbose is True


# ─── Test: Privacy check function ───

def test_check_privacy_clean():
    """_check_privacy returns empty list for clean data."""
    rows = [
        ("System prompt about memory retrieval", "How does consolidation work?", "It uses Hebbian learning", None),
        ("Guidance evaluation context", "Classify this constraint", "Type: behavioral", "Thinking about categories"),
    ]
    violations = tdr._check_privacy(rows, ["system_prompt", "user_prompt", "response", "think_content"])
    assert violations == []


def test_check_privacy_detects_violations():
    """_check_privacy flags actual sensitive data."""
    rows = [
        ("Use key sk-abcdefghijklmnopqrstuvwxyz for auth", "query", "response", None),
    ]
    violations = tdr._check_privacy(rows, ["system_prompt", "user_prompt", "response", "think_content"])
    assert len(violations) >= 1
    assert "api_key" in violations[0]


# ─── Test: JSON encoder ───

def test_json_encoder_decimal():
    """JSONEncoder handles Decimal types."""
    from decimal import Decimal
    data = {"value": Decimal("3.14"), "count": 42}
    result = json.dumps(data, cls=tdr.JSONEncoder)
    parsed = json.loads(result)
    assert parsed["value"] == 3.14
    assert parsed["count"] == 42


def test_json_encoder_datetime():
    """JSONEncoder handles datetime types."""
    from datetime import datetime, timezone
    data = {"time": datetime(2026, 4, 1, 12, 0, 0, tzinfo=timezone.utc)}
    result = json.dumps(data, cls=tdr.JSONEncoder)
    parsed = json.loads(result)
    assert "2026-04-01" in parsed["time"]


# ─── Test: Near-duplicate detection ───

def test_near_duplicate_detection():
    """Suffix-based near-duplicate detection works."""
    names = ["mdemg_foo_total", "mdemg_foo_count", "mdemg_bar_total", "mdemg_baz_ratio"]
    dupes = tdr._find_near_duplicate_names(names)
    assert ("mdemg_foo_total", "mdemg_foo_count") in dupes

    # No false positives for completely different names
    names2 = ["mdemg_retrieval_latency", "mdemg_embedding_count", "mdemg_health_score"]
    dupes2 = tdr._find_near_duplicate_names(names2)
    assert len(dupes2) == 0


# ─── Test: Format text doesn't crash ───

def test_format_text_basic():
    """format_text produces non-empty output without errors."""
    sections = [
        tdr.SectionResult(name="schema_health", data={"schema_version": 8, "tables_present": 8, "tables_expected": 8, "index_count": 14, "continuous_aggregates": ["metrics_hourly", "metrics_daily"]}),
        tdr.SectionResult(name="ft_tables", data={"table_counts": {"ft_benchmarks": 0}, "all_empty": True}),
    ]
    findings = [tdr.Finding("info", "ft_tables", "All empty")]
    text = tdr.format_text(sections, findings, "2026-04-01T00:00:00Z", 24.5)
    assert "MDEMG TSDB Data Governance Report" in text
    assert "FINDINGS" in text
    assert len(text) > 100
