#!/usr/bin/env python3
"""Comprehensive Grafana dashboard fixer for Prometheus→TSDB migration.

Fixes:
  1. Metric name prefixes (mdemg_ prefix in rawSql, template vars, alerts)
  2. Template variable standardization (Instance + Space ID on all dashboards)
  3. UX consistency (time ranges, refresh, schema version, nav links)
  4. FT-Training stub notice
  5. Neo4j Graph Growth Trend copy-paste error
"""

import json
import re
import sys
import os
from pathlib import Path

DASHBOARD_DIR = Path("deploy/docker/grafana/dashboards")
ALERTS_FILE = Path("deploy/docker/grafana/provisioning/alerting/alerts.yml")

# J17 tier metric name corrections (old legacy names → actual TSDB names)
TIER_NAME_MAP = {
    "j17_tier1_distribution": "mdemg_j17_tier_t1_fraction",
    "j17_tier2_distribution": "mdemg_j17_tier_t2_fraction",
    "j17_tier3_distribution": "mdemg_j17_tier_t3_fraction",
}


def add_prefix(name):
    """Add mdemg_ prefix if not already present, with tier name corrections."""
    if name in TIER_NAME_MAP:
        return TIER_NAME_MAP[name]
    if name.startswith("mdemg_"):
        return name
    return "mdemg_" + name


def fix_sql(sql):
    """Fix all metric name references in a SQL string."""
    # Fix metric_name = 'xxx'
    def fix_eq(m):
        pre, name, suf = m.group(1), m.group(2), m.group(3)
        return pre + add_prefix(name) + suf

    sql = re.sub(r"(metric_name\s*=\s*')([^']+)(')", fix_eq, sql)

    # Fix metric_name IN ('xxx', 'yyy', ...)
    def fix_in(m):
        prefix = m.group(1)  # "metric_name IN ("
        body = m.group(2)    # "'xxx','yyy',..."
        suffix = m.group(3)  # ")"
        # Fix each quoted name inside the IN list
        fixed = re.sub(r"'([^']+)'", lambda n: "'" + add_prefix(n.group(1)) + "'", body)
        return prefix + fixed + suffix

    sql = re.sub(r"(metric_name\s+IN\s*\()([^)]+)(\))", fix_in, sql, flags=re.IGNORECASE)

    # Fix metric_name LIKE 'xxx%'
    def fix_like(m):
        pre, name, suf = m.group(1), m.group(2), m.group(3)
        if not name.startswith("mdemg_"):
            return pre + "mdemg_" + name + suf
        return m.group(0)

    sql = re.sub(r"(metric_name\s+LIKE\s*')([^']+)(')", fix_like, sql, flags=re.IGNORECASE)

    return sql


def fix_panel_targets(panels):
    """Recursively fix rawSql in all panel targets."""
    count = 0
    for p in panels:
        for t in p.get("targets", []):
            if "rawSql" in t:
                original = t["rawSql"]
                t["rawSql"] = fix_sql(original)
                if t["rawSql"] != original:
                    count += 1
        # Recurse into row panels
        if "panels" in p:
            count += fix_panel_targets(p["panels"])
    return count


def fix_template_vars(d):
    """Fix metric name references inside template variable queries."""
    count = 0
    for v in d.get("templating", {}).get("list", []):
        q = v.get("query", "")
        if isinstance(q, str) and "metric_name" in q:
            fixed = fix_sql(q)
            if fixed != q:
                v["query"] = fixed
                if "definition" in v:
                    v["definition"] = fixed
                count += 1
    return count


# ── Standard template variables ──────────────────────────────────────────

INSTANCE_VAR = {
    "name": "instance",
    "label": "Instance",
    "type": "custom",
    "query": "localhost:9999",
    "current": {"text": "localhost:9999", "value": "localhost:9999", "selected": True},
    "options": [{"text": "localhost:9999", "value": "localhost:9999", "selected": True}],
    "hide": 0,
    "includeAll": False,
    "multi": False,
    "skipUrlSync": False,
}

SPACE_ID_VAR = {
    "name": "space_id",
    "label": "Space ID",
    "type": "query",
    "datasource": {"type": "postgres", "uid": "timescaledb"},
    "definition": "SELECT DISTINCT space_id FROM metric_samples WHERE time > now() - interval '24 hours' ORDER BY 1",
    "query": "SELECT DISTINCT space_id FROM metric_samples WHERE time > now() - interval '24 hours' ORDER BY 1",
    "refresh": 2,
    "sort": 1,
    "includeAll": False,
    "multi": False,
    "current": {"text": "mdemg-dev", "value": "mdemg-dev"},
    "hide": 0,
    "skipUrlSync": False,
}

STANDARD_LINK = {
    "title": "Dashboards",
    "type": "dashboards",
    "tags": ["mdemg"],
    "icon": "external link",
    "asDropdown": True,
}


def standardize_variables(d, dashboard_name):
    """Ensure Instance + Space ID variables exist in correct order."""
    if dashboard_name == "mdemg-graph-topology.json":
        return  # Already the reference pattern

    tpl = d.setdefault("templating", {})
    var_list = tpl.setdefault("list", [])

    existing_names = [v.get("name") for v in var_list]

    # Ensure instance is first
    if "instance" not in existing_names:
        var_list.insert(0, INSTANCE_VAR.copy())
    else:
        # Move instance to position 0 if not already
        idx = existing_names.index("instance")
        if idx != 0:
            inst = var_list.pop(idx)
            var_list.insert(0, inst)

    # Re-read after potential insert
    existing_names = [v.get("name") for v in var_list]

    # Ensure space_id is second
    if "space_id" not in existing_names:
        var_list.insert(1, SPACE_ID_VAR.copy())
    else:
        idx = existing_names.index("space_id")
        v = var_list[idx]
        # Convert custom space_id to query type (ft-training has custom)
        if v.get("type") == "custom":
            var_list[idx] = SPACE_ID_VAR.copy()
        # Ensure it's in position 1
        if idx != 1:
            sv = var_list.pop(idx)
            var_list.insert(1, sv)


def standardize_links(d, dashboard_name):
    """Ensure tag-based dashboard navigation link."""
    if dashboard_name == "mdemg-graph-topology.json":
        return  # Keep its existing link setup

    d["links"] = [STANDARD_LINK.copy()]


def standardize_time_and_refresh(d, dashboard_name):
    """Set consistent time ranges and refresh intervals."""
    time_map = {
        "mdemg-overview.json": ("now-6h", "now", "30s"),
        "mdemg-neo4j.json": ("now-6h", "now", "30s"),
        "mdemg-rsic.json": ("now-6h", "now", "30s"),
        "mdemg-j17.json": ("now-6h", "now", "30s"),
        "mdemg-jiminy.json": ("now-6h", "now", "30s"),
        "mdemg-ft-training.json": ("now-7d", "now", "30s"),
        "mdemg-graph-topology.json": ("now-24h", "now", ""),
    }

    if dashboard_name in time_map:
        fr, to, refresh = time_map[dashboard_name]
        d.setdefault("time", {})
        d["time"]["from"] = fr
        d["time"]["to"] = to
        if refresh:
            d["refresh"] = refresh
        elif "refresh" in d:
            d["refresh"] = ""


def fix_neo4j_graph_growth(d, dashboard_name):
    """Fix the Graph Growth Trend panel that incorrectly queries rsic_health_overall."""
    if dashboard_name != "mdemg-neo4j.json":
        return

    for p in d.get("panels", []):
        all_panels = [p] + p.get("panels", [])
        for sp in all_panels:
            if sp.get("title") == "Graph Growth Trend":
                for t in sp.get("targets", []):
                    if "rawSql" in t and "rsic_health_overall" in t["rawSql"]:
                        t["rawSql"] = (
                            "SELECT bucket AS time, metric_name AS metric, avg_value AS value "
                            "FROM metrics_hourly "
                            "WHERE metric_name IN ('mdemg_neo4j_graph_total_nodes','mdemg_neo4j_graph_total_edges') "
                            "AND space_id = '$space_id' "
                            "AND $__timeFilter(bucket) ORDER BY bucket"
                        )


def fix_ft_training_stub(d, dashboard_name):
    """Add stub notice to FT-Training dashboard."""
    if dashboard_name != "mdemg-ft-training.json":
        return

    # Add coming-soon tag
    tags = d.get("tags", [])
    if "coming-soon" not in tags:
        tags.append("coming-soon")
    d["tags"] = tags

    # Add noValue to each data panel
    for p in d.get("panels", []):
        if p.get("type") in ("timeseries", "stat", "gauge", "table"):
            fc = p.setdefault("fieldConfig", {})
            defaults = fc.setdefault("defaults", {})
            defaults["noValue"] = "Not yet collected"

    # Check if stub notice text panel already exists
    has_notice = any(
        p.get("title") == "Status Notice" for p in d.get("panels", [])
    )
    if not has_notice:
        notice_panel = {
            "title": "Status Notice",
            "type": "text",
            "gridPos": {"h": 2, "w": 24, "x": 0, "y": 0},
            "options": {
                "mode": "html",
                "content": (
                    '<div style="padding:8px 12px;font-size:14px;color:#FF9830">'
                    "<strong>Fine-tuning pipeline metrics are not yet collected.</strong> "
                    "This dashboard will activate when FT collectors are implemented."
                    "</div>"
                ),
            },
            "fieldConfig": {"defaults": {}, "overrides": []},
        }
        # Shift all existing panels down by 2
        for p in d.get("panels", []):
            gp = p.get("gridPos", {})
            if "y" in gp:
                gp["y"] += 2
            for sp in p.get("panels", []):
                sgp = sp.get("gridPos", {})
                if "y" in sgp:
                    sgp["y"] += 2
        d["panels"].insert(0, notice_panel)


def fix_schema_version(d, dashboard_name):
    """Ensure consistent schema version."""
    if d.get("schemaVersion", 39) < 39:
        d["schemaVersion"] = 39


def fix_neo4j_explicit_datasource(d, dashboard_name):
    """Ensure all panels in neo4j dashboard have explicit datasource."""
    if dashboard_name != "mdemg-neo4j.json":
        return

    ds = {"type": "postgres", "uid": "timescaledb"}
    for p in d.get("panels", []):
        if p.get("type") not in ("row", "text") and "datasource" not in p:
            p["datasource"] = ds.copy()
        for sp in p.get("panels", []):
            if sp.get("type") not in ("row", "text") and "datasource" not in sp:
                sp["datasource"] = ds.copy()


def process_dashboard(path):
    """Apply all fixes to a single dashboard file."""
    name = path.name
    with open(path) as f:
        d = json.load(f)

    # Phase 1: Fix metric names
    sql_fixes = fix_panel_targets(d.get("panels", []))
    var_fixes = fix_template_vars(d)

    # Phase 4: Standardize variables and navigation
    standardize_variables(d, name)
    standardize_links(d, name)

    # Phase 5: UX consistency
    standardize_time_and_refresh(d, name)
    fix_schema_version(d, name)
    fix_neo4j_graph_growth(d, name)
    fix_neo4j_explicit_datasource(d, name)

    # Phase 6: FT-Training stub
    fix_ft_training_stub(d, name)

    with open(path, "w") as f:
        json.dump(d, f, indent=2)
        f.write("\n")

    print(f"  {name}: {sql_fixes} SQL fixes, {var_fixes} template var fixes")
    return sql_fixes + var_fixes


def fix_alerts():
    """Fix metric name prefixes in alerts.yml."""
    if not ALERTS_FILE.exists():
        print("  alerts.yml: NOT FOUND")
        return 0

    with open(ALERTS_FILE) as f:
        content = f.read()

    original = content

    def fix_eq(m):
        pre, name, suf = m.group(1), m.group(2), m.group(3)
        return pre + add_prefix(name) + suf

    content = re.sub(r"(metric_name = ')([^']+)(')", fix_eq, content)
    content = re.sub(r"(metric_name LIKE ')([^']+)(')", fix_eq, content, flags=re.IGNORECASE)

    changes = sum(1 for a, b in zip(original.split("\n"), content.split("\n")) if a != b)

    with open(ALERTS_FILE, "w") as f:
        f.write(content)

    print(f"  alerts.yml: {changes} line fixes")
    return changes


def validate():
    """Check that no unprefixed metric names remain."""
    issues = []

    for path in sorted(DASHBOARD_DIR.glob("*.json")):
        with open(path) as f:
            content = f.read()

        # Find metric_name = 'xxx' without mdemg_ prefix (excluding TODO comments)
        for i, line in enumerate(content.split("\n"), 1):
            if "TODO" in line or line.strip().startswith("--"):
                continue
            for m in re.finditer(r"metric_name\s*=\s*'([^']+)'", line):
                if not m.group(1).startswith("mdemg_"):
                    issues.append(f"  {path.name}:{i}: {m.group(0)}")
            for m in re.finditer(r"metric_name\s+IN\s*\(([^)]+)\)", line, re.IGNORECASE):
                for n in re.finditer(r"'([^']+)'", m.group(1)):
                    if not n.group(1).startswith("mdemg_"):
                        issues.append(f"  {path.name}:{i}: IN(...{n.group(0)}...)")

    if ALERTS_FILE.exists():
        with open(ALERTS_FILE) as f:
            for i, line in enumerate(f, 1):
                if line.strip().startswith("#"):
                    continue
                for m in re.finditer(r"metric_name = '([^']+)'", line):
                    if not m.group(1).startswith("mdemg_"):
                        issues.append(f"  alerts.yml:{i}: {m.group(0)}")

    if issues:
        print(f"\nVALIDATION FAILED — {len(issues)} unprefixed metric names:")
        for issue in issues:
            print(issue)
        return False
    else:
        print("\nVALIDATION PASSED — all metric names have mdemg_ prefix")
        return True


def main():
    validate_only = "--validate-only" in sys.argv

    if validate_only:
        ok = validate()
        sys.exit(0 if ok else 1)

    print("Fixing dashboards...")
    total = 0
    for path in sorted(DASHBOARD_DIR.glob("*.json")):
        total += process_dashboard(path)

    print("\nFixing alerts...")
    total += fix_alerts()

    print(f"\nTotal fixes applied: {total}")

    validate()


if __name__ == "__main__":
    main()
