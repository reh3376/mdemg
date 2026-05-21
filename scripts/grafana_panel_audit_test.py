#!/usr/bin/env python3
"""Tier 1 unit tests for scripts/grafana_panel_audit.py.

Runs via:  python3 scripts/grafana_panel_audit_test.py
or:        pytest scripts/grafana_panel_audit_test.py
"""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

# Allow `import grafana_panel_audit` whether invoked from repo root or scripts/.
sys.path.insert(0, str(Path(__file__).resolve().parent))
import grafana_panel_audit as gpa  # noqa: E402


class TemplateVarSubstitutionTest(unittest.TestCase):
    def test_time_filter_replaced(self):
        s = "SELECT * FROM t WHERE $__timeFilter(time)"
        out = gpa.substitute_template_vars(s, space_id="dev", instance="x")
        self.assertIn("time > now() - interval '24 hours'", out)
        self.assertNotIn("$__timeFilter", out)

    def test_time_filter_with_qualified_column(self):
        s = "SELECT * FROM t WHERE $__timeFilter(t.recorded_at)"
        out = gpa.substitute_template_vars(s, space_id="dev", instance="x")
        self.assertIn("t.recorded_at > now()", out)

    def test_time_from_to_replaced(self):
        s = "SELECT * FROM t WHERE time BETWEEN $__timeFrom() AND $__timeTo()"
        out = gpa.substitute_template_vars(s, space_id="dev", instance="x")
        self.assertNotIn("$__timeFrom", out)
        self.assertNotIn("$__timeTo", out)
        self.assertIn("now() - interval '24 hours'", out)

    def test_unix_epoch_replaced(self):
        s = "SELECT * FROM t WHERE ts BETWEEN $__unixEpochFrom() AND $__unixEpochTo()"
        out = gpa.substitute_template_vars(s, space_id="dev", instance="x")
        self.assertNotIn("$__unixEpochFrom", out)
        self.assertIn("EXTRACT(EPOCH FROM now()", out)

    def test_interval_replaced(self):
        s = "SELECT time_bucket($__interval, time) FROM t"
        out = gpa.substitute_template_vars(s, space_id="dev", instance="x")
        self.assertIn("time_bucket('1 minute', time)", out)
        self.assertNotIn("$__interval", out)

    def test_interval_ms_replaced(self):
        s = "SELECT $__interval_ms FROM t"
        out = gpa.substitute_template_vars(s, space_id="dev", instance="x")
        self.assertNotIn("$__interval_ms", out)

    def test_space_id_substitutions(self):
        for tmpl in ["$space_id", "${space_id}", "${space_id:raw}"]:
            s = f"SELECT * FROM t WHERE space_id = '{tmpl}'"
            out = gpa.substitute_template_vars(s, space_id="myspace", instance="x")
            self.assertIn("myspace", out, f"failed for {tmpl}")

    def test_instance_substitutions(self):
        for tmpl in ["$instance", "${instance}", "${instance:raw}"]:
            s = f"SELECT * FROM t WHERE labels->>'instance' = '{tmpl}'"
            out = gpa.substitute_template_vars(s, space_id="dev", instance="host:9999")
            self.assertIn("host:9999", out, f"failed for {tmpl}")

    def test_no_template_vars_left_in_basic_query(self):
        s = (
            "SELECT time_bucket($__interval, time) AS time, AVG(value) "
            "FROM metric_samples WHERE metric_name = 'mdemg_rsic_health_overall' "
            "AND space_id = '$space_id' AND labels->>'instance' = '$instance' "
            "AND $__timeFilter(time) GROUP BY 1 ORDER BY 1"
        )
        out = gpa.substitute_template_vars(s, space_id="mdemg-dev", instance="localhost:9999")
        for needle in ["$__interval", "$__timeFilter", "$space_id", "$instance"]:
            self.assertNotIn(needle, out, f"unsubstituted {needle!r} remaining in: {out}")


class TableExtractionTest(unittest.TestCase):
    def test_simple_from(self):
        self.assertEqual(gpa.extract_tables("SELECT * FROM metric_samples"), ["metric_samples"])

    def test_from_with_alias(self):
        self.assertEqual(
            gpa.extract_tables("SELECT * FROM metric_samples ms WHERE ms.value > 0"),
            ["metric_samples"],
        )

    def test_join_pulls_both(self):
        sql = "SELECT * FROM a JOIN b ON a.id = b.id LEFT JOIN c ON c.x = a.x"
        self.assertEqual(gpa.extract_tables(sql), ["a", "b", "c"])

    def test_case_insensitive(self):
        sql = "select * from METRIC_SAMPLES join Llm_Interactions on a=b"
        self.assertEqual(gpa.extract_tables(sql), ["llm_interactions", "metric_samples"])

    def test_no_tables(self):
        self.assertEqual(gpa.extract_tables("SELECT 1"), [])


class PanelWalkingTest(unittest.TestCase):
    def test_flat_panels(self):
        panels = [{"id": 1, "type": "stat"}, {"id": 2, "type": "timeseries"}]
        walked = gpa.walk_panels(panels)
        self.assertEqual(len(walked), 2)
        self.assertEqual([p["id"] for p, _ in walked], [1, 2])

    def test_nested_row_panels(self):
        panels = [
            {"id": 1, "type": "stat"},
            {"id": 2, "type": "row", "title": "RSIC", "panels": [
                {"id": 3, "type": "timeseries"},
                {"id": 4, "type": "gauge"},
            ]},
        ]
        walked = gpa.walk_panels(panels)
        ids = sorted(p["id"] for p, _ in walked)
        self.assertEqual(ids, [1, 2, 3, 4])

    def test_panel_targets_with_sql(self):
        panel = {"targets": [
            {"refId": "A", "rawSql": "SELECT 1"},
            {"refId": "B"},
            {"refId": "C", "sql": "SELECT 2"},
            {"refId": "D", "rawSql": "   "},
        ]}
        out = gpa.panel_targets_with_sql(panel)
        self.assertEqual([(i, sql) for i, sql in out], [(0, "SELECT 1"), (2, "SELECT 2")])


def main() -> int:
    loader = unittest.TestLoader()
    suite = unittest.TestSuite()
    for cls in (TemplateVarSubstitutionTest, TableExtractionTest, PanelWalkingTest):
        suite.addTests(loader.loadTestsFromTestCase(cls))
    runner = unittest.TextTestRunner(verbosity=2)
    result = runner.run(suite)
    return 0 if result.wasSuccessful() else 1


if __name__ == "__main__":
    sys.exit(main())
