#!/usr/bin/env python3
"""Sprint GRAFANA-AUDIT-001 Epic 0 — per-panel audit harness.

Walks every panel in every dashboard JSON under
`deploy/docker/grafana/dashboards/`. For each panel with a SQL target,
substitutes Grafana template variables ($space_id, $instance,
$__timeFilter, $__interval, ...) with safe audit defaults, executes the
substituted SQL via `docker exec mdemg-timescaledb-1 psql`, and classifies
the panel as PASS / EMPTY / FAIL / SKIP.

Outputs `docs/development/grafana-audit-001/audit_results.json` with one
record per panel.

Usage:
    python3 scripts/grafana_panel_audit.py
    python3 scripts/grafana_panel_audit.py --dashboard mdemg-rsic.json
    python3 scripts/grafana_panel_audit.py --space-id myspace --instance localhost:9999

Exit codes:
    0  — harness ran without internal errors (panels may still FAIL/EMPTY)
    1  — harness internal error (couldn't read dashboards, psql unreachable)
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import time
from dataclasses import dataclass, field, asdict
from pathlib import Path
from typing import Optional

REPO_ROOT = Path(__file__).resolve().parent.parent
DASHBOARDS_DIR = REPO_ROOT / "deploy" / "docker" / "grafana" / "dashboards"
DEFAULT_OUTPUT = REPO_ROOT / "docs" / "development" / "grafana-audit-001" / "audit_results.json"

# Default audit substitutions — must match the operator's live data labels
# (verified in Phase 1: space_id=mdemg-dev and instance=localhost:9999 cover
# all 173K rows in the last 24h on the dev TSDB).
DEFAULT_SPACE_ID = "mdemg-dev"
DEFAULT_INSTANCE = "localhost:9999"


@dataclass
class PanelAuditResult:
    """One audit verdict per panel."""

    dashboard: str
    panel_id: int
    panel_title: str
    panel_type: str  # timeseries, stat, gauge, table, etc.
    target_index: int  # 0 if single target; N for multi-target panels
    verdict: str  # PASS | EMPTY | FAIL | SKIP
    row_count: int
    execution_ms: float
    error: Optional[str] = None
    skip_reason: Optional[str] = None
    sql_preview: str = ""  # first 200 chars of substituted SQL
    tables_referenced: list[str] = field(default_factory=list)


# ─── Template variable substitution ─────────────────────────────────────────

TIME_FILTER_RE = re.compile(r"\$__timeFilter\s*\(\s*([^\s)]+)\s*\)")
TIME_FROM_RE = re.compile(r"\$__timeFrom\s*\(\s*\)")
TIME_TO_RE = re.compile(r"\$__timeTo\s*\(\s*\)")
INTERVAL_RE = re.compile(r"\$__interval(?:_ms)?\b")
RAW_RE = re.compile(r"\$__rawTimeFrom\s*\(\s*\)|\$__rawTimeTo\s*\(\s*\)")
UNIX_EPOCH_FROM_RE = re.compile(r"\$__unixEpochFrom\s*\(\s*\)")
UNIX_EPOCH_TO_RE = re.compile(r"\$__unixEpochTo\s*\(\s*\)")


def substitute_template_vars(sql: str, *, space_id: str, instance: str, time_window: str = "24 hours") -> str:
    """Replace Grafana template variables with safe audit defaults.

    Substitutions:
      $__timeFilter(col)  → col > now() - interval '<time_window>'
      $__timeFrom()       → now() - interval '<time_window>'
      $__timeTo()         → now()
      $__interval         → '1 minute'
      $__interval_ms      → 60000
      $__rawTimeFrom()    → now() - interval '<time_window>'
      $__rawTimeTo()      → now()
      $__unixEpochFrom()  → EXTRACT(EPOCH FROM now() - interval '<time_window>')::bigint
      $__unixEpochTo()    → EXTRACT(EPOCH FROM now())::bigint
      $space_id           → '<space_id>'  (only when wrapped in quotes already)
      $instance           → '<instance>'  (same)

    Returns the substituted SQL ready for psql execution.
    """
    s = sql

    # Grafana macro substitutions
    s = TIME_FILTER_RE.sub(rf"\1 > now() - interval '{time_window}'", s)
    s = TIME_FROM_RE.sub(f"now() - interval '{time_window}'", s)
    s = TIME_TO_RE.sub("now()", s)
    # NOTE: Grafana convention is for panel SQL to provide its own outer
    # quotes (e.g. `time_bucket('$__interval', time)`), so we substitute
    # the bare value without adding quotes. If we wrapped in quotes here,
    # we'd produce doubled quotes (`'1 minute'` -> `''1 minute''`) and
    # generate false-positive syntax errors. Same applies to interval_ms.
    s = INTERVAL_RE.sub("1 minute", s)
    s = RAW_RE.sub("now()", s)
    s = UNIX_EPOCH_FROM_RE.sub(f"EXTRACT(EPOCH FROM now() - interval '{time_window}')::bigint", s)
    s = UNIX_EPOCH_TO_RE.sub("EXTRACT(EPOCH FROM now())::bigint", s)

    # Template variables ($space_id, $instance, etc.)
    s = s.replace("${space_id:raw}", space_id).replace("${space_id}", space_id).replace("$space_id", space_id)
    s = s.replace("${instance:raw}", instance).replace("${instance}", instance).replace("$instance", instance)

    # Common multi-value template variables we treat as wildcards in audit
    s = s.replace("${layers:csv}", "0,1,2,3,4,5")
    s = s.replace("${edge_types:csv}", "''")  # empty list → false predicate, but valid SQL
    s = s.replace("${focus_nodes:raw}", "").replace("${focus_nodes}", "").replace("$focus_nodes", "")
    s = s.replace("${page_size:raw}", "500").replace("${page_size}", "500").replace("$page_size", "500")
    s = s.replace("${page:raw}", "1").replace("${page}", "1").replace("$page", "1")
    s = s.replace("${hop_depth:raw}", "1").replace("${hop_depth}", "1").replace("$hop_depth", "1")
    s = s.replace("${show_hidden:raw}", "false").replace("${show_hidden}", "false").replace("$show_hidden", "false")

    return s


# ─── Table reference extraction ─────────────────────────────────────────────

TABLE_REF_RE = re.compile(r"(?:FROM|JOIN)\s+([a-z_][a-z0-9_]*)", re.IGNORECASE)


def extract_tables(sql: str) -> list[str]:
    """Return the set of distinct table-like identifiers referenced in the SQL."""
    return sorted({m.group(1).lower() for m in TABLE_REF_RE.finditer(sql)})


# ─── Panel walking ──────────────────────────────────────────────────────────


def walk_panels(panels: list[dict], parent_path: str = "") -> list[tuple[dict, str]]:
    """Yield (panel, parent_path) for every panel including nested ones in rows."""
    out: list[tuple[dict, str]] = []
    for p in panels:
        out.append((p, parent_path))
        if p.get("type") == "row" and p.get("panels"):
            row_title = p.get("title", "row")
            out.extend(walk_panels(p["panels"], f"{parent_path}/{row_title}" if parent_path else row_title))
    return out


def panel_targets_with_sql(panel: dict) -> list[tuple[int, str]]:
    """Return [(target_index, rawSql)] for each target with a SQL string."""
    out: list[tuple[int, str]] = []
    for i, t in enumerate(panel.get("targets") or []):
        sql = t.get("rawSql") or t.get("sql")
        if isinstance(sql, str) and sql.strip():
            out.append((i, sql))
    return out


# ─── psql executor ──────────────────────────────────────────────────────────


def run_psql(sql: str, *, container: str, user: str, db: str, statement_timeout_ms: int) -> tuple[int, str, float]:
    """Execute SQL via `docker exec ... psql`. Return (row_count_or_-1, error, elapsed_ms).

    row_count is the integer "(N rows)" footer psql emits, or -1 if not parsed.
    """
    # Wrap the user SQL in a CTE so we can count rows regardless of original
    # SELECT shape. Limit to 10 rows for speed — we only care about presence
    # of data, not the data itself.
    wrapped = (
        f"SET statement_timeout = {statement_timeout_ms}; "
        f"WITH audit_q AS ({sql.rstrip(';')}) "
        f"SELECT COUNT(*) AS audit_row_count FROM (SELECT * FROM audit_q LIMIT 10) sub;"
    )
    t0 = time.monotonic()
    proc = subprocess.run(
        [
            "docker", "exec", "-i", container,
            "psql", "-U", user, "-d", db,
            "-At",  # tuples-only, unaligned (single column scalar output)
            "-c", wrapped,
        ],
        capture_output=True,
        text=True,
        timeout=max(15, statement_timeout_ms // 1000 + 5),
    )
    elapsed_ms = (time.monotonic() - t0) * 1000.0
    if proc.returncode != 0:
        err = (proc.stderr or proc.stdout).strip()
        return -1, err[:1000], elapsed_ms
    out = proc.stdout.strip()
    try:
        row_count = int(out.splitlines()[-1])
        return row_count, "", elapsed_ms
    except (ValueError, IndexError):
        return -1, f"psql output not numeric: {out[:200]}", elapsed_ms


# ─── Per-panel auditor ──────────────────────────────────────────────────────


def audit_panel(
    dashboard: str,
    panel: dict,
    target_index: int,
    raw_sql: str,
    *,
    space_id: str,
    instance: str,
    container: str,
    user: str,
    db: str,
    timeout_ms: int,
) -> PanelAuditResult:
    panel_id = panel.get("id", -1)
    panel_title = panel.get("title", "(untitled)")
    panel_type = panel.get("type", "(unknown)")

    if panel_type in ("row", "text", "dashlist", "news"):
        return PanelAuditResult(
            dashboard=dashboard,
            panel_id=panel_id,
            panel_title=panel_title,
            panel_type=panel_type,
            target_index=target_index,
            verdict="SKIP",
            row_count=0,
            execution_ms=0.0,
            skip_reason=f"non-SQL panel type: {panel_type}",
        )

    substituted = substitute_template_vars(raw_sql, space_id=space_id, instance=instance)
    tables = extract_tables(substituted)
    sql_preview = " ".join(substituted.split())[:200]

    row_count, err, elapsed_ms = run_psql(
        substituted,
        container=container,
        user=user,
        db=db,
        statement_timeout_ms=timeout_ms,
    )

    if err:
        verdict = "FAIL"
    elif row_count > 0:
        verdict = "PASS"
    elif row_count == 0:
        verdict = "EMPTY"
    else:
        verdict = "FAIL"

    return PanelAuditResult(
        dashboard=dashboard,
        panel_id=panel_id,
        panel_title=panel_title,
        panel_type=panel_type,
        target_index=target_index,
        verdict=verdict,
        row_count=max(row_count, 0),
        execution_ms=round(elapsed_ms, 1),
        error=err or None,
        sql_preview=sql_preview,
        tables_referenced=tables,
    )


# ─── Driver ─────────────────────────────────────────────────────────────────


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--dashboard", help="Audit only one dashboard (basename, e.g. mdemg-rsic.json)")
    p.add_argument("--space-id", default=DEFAULT_SPACE_ID)
    p.add_argument("--instance", default=DEFAULT_INSTANCE)
    p.add_argument("--container", default="mdemg-timescaledb-1")
    p.add_argument("--user", default="mdemg")
    p.add_argument("--db", default="mdemg_metrics")
    p.add_argument("--timeout-ms", type=int, default=10000)
    p.add_argument("--output", default=str(DEFAULT_OUTPUT))
    p.add_argument("--quiet", action="store_true", help="Don't print per-panel progress")
    args = p.parse_args()

    if not DASHBOARDS_DIR.exists():
        print(f"FATAL: dashboards dir not found: {DASHBOARDS_DIR}", file=sys.stderr)
        return 1

    targets = sorted(DASHBOARDS_DIR.glob("*.json"))
    if args.dashboard:
        targets = [t for t in targets if t.name == args.dashboard]
        if not targets:
            print(f"FATAL: dashboard {args.dashboard!r} not found in {DASHBOARDS_DIR}", file=sys.stderr)
            return 1

    all_results: list[PanelAuditResult] = []
    for dpath in targets:
        try:
            d = json.loads(dpath.read_text())
        except Exception as e:
            print(f"FATAL: could not parse {dpath}: {e}", file=sys.stderr)
            return 1
        dname = dpath.stem
        for panel, _ in walk_panels(d.get("panels", [])):
            sql_targets = panel_targets_with_sql(panel)
            if not sql_targets:
                if panel.get("type") not in ("row",):  # rows are container-only
                    all_results.append(PanelAuditResult(
                        dashboard=dname,
                        panel_id=panel.get("id", -1),
                        panel_title=panel.get("title", "(untitled)"),
                        panel_type=panel.get("type", "(unknown)"),
                        target_index=0,
                        verdict="SKIP",
                        row_count=0,
                        execution_ms=0.0,
                        skip_reason="panel has no SQL targets",
                    ))
                continue
            for target_index, raw_sql in sql_targets:
                result = audit_panel(
                    dashboard=dname,
                    panel=panel,
                    target_index=target_index,
                    raw_sql=raw_sql,
                    space_id=args.space_id,
                    instance=args.instance,
                    container=args.container,
                    user=args.user,
                    db=args.db,
                    timeout_ms=args.timeout_ms,
                )
                all_results.append(result)
                if not args.quiet:
                    icon = {"PASS": "✓", "EMPTY": "○", "FAIL": "✗", "SKIP": "·"}[result.verdict]
                    print(f"  {icon} [{result.verdict:5s}] {dname}#{result.panel_id} t{target_index} {result.panel_title}")

    # Aggregate stats
    by_verdict: dict[str, int] = {}
    by_dashboard: dict[str, dict[str, int]] = {}
    for r in all_results:
        by_verdict[r.verdict] = by_verdict.get(r.verdict, 0) + 1
        by_dashboard.setdefault(r.dashboard, {}).setdefault(r.verdict, 0)
        by_dashboard[r.dashboard][r.verdict] = by_dashboard[r.dashboard].get(r.verdict, 0) + 1

    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps({
        "audit_meta": {
            "space_id": args.space_id,
            "instance": args.instance,
            "container": args.container,
            "timeout_ms": args.timeout_ms,
            "total_targets_audited": len(all_results),
            "verdict_counts": by_verdict,
            "by_dashboard": by_dashboard,
        },
        "results": [asdict(r) for r in all_results],
    }, indent=2))

    print(f"\nWrote {len(all_results)} results → {output_path}")
    print(f"Verdicts: {by_verdict}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
