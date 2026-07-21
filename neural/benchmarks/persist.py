"""Persistence helpers for Phase 10 benchmark runs.

Epic 3 MVP scope: no runtime DB dep. Emits plain SQL INSERT statements
for the V0012 schema (``benchmark_runs`` + ``benchmark_results``) that the
operator can feed via ``psql -f <report>.sql``.

Full live-writer (pgx/asyncpg) wiring is deferred to the Phase 11 operational
scaffolding sprint. This module already carries the column layout + JSONB
encoding, so the live-writer only needs to swap ``_render_insert_*`` for
parameterized-query dispatch.
"""
from __future__ import annotations

import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


def _q(value: Any) -> str:
    """SQL-literal quoting. NULL for None, escaped quotes for strings."""
    if value is None:
        return "NULL"
    if isinstance(value, bool):
        return "TRUE" if value else "FALSE"
    if isinstance(value, (int, float)):
        return repr(value)
    if isinstance(value, (dict, list)):
        # JSONB literal: single-quoted JSON cast
        return "'" + json.dumps(value, sort_keys=True).replace("'", "''") + "'::jsonb"
    s = str(value)
    return "'" + s.replace("'", "''") + "'"


def _iso_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def render_benchmark_runs_insert(report: dict[str, Any]) -> str:
    """Produce the INSERT into benchmark_runs for this report."""
    cols = [
        "run_id", "started_at", "completed_at", "model_path",
        "base_sha", "adapter_manifest_sha", "dataset_sha",
        "config_sha", "n_runs_per_task", "specs_total",
        "specs_with_matched_rows", "aggregate_weighted_score",
        "stagnation_flag", "stagnation_reason", "report_path",
    ]
    values = [
        report.get("run_id"),
        report.get("started_at"),
        report.get("completed_at"),
        report.get("model_path"),
        report.get("base_sha"),
        report.get("adapter_manifest_sha"),
        report.get("dataset_sha"),
        report.get("config_sha"),
        report.get("n_runs_per_task"),
        report.get("specs_total"),
        report.get("specs_with_matched_rows"),
        report.get("aggregate_weighted_score"),
        bool((report.get("stagnation") or {}).get("flag", False)),
        (report.get("stagnation") or {}).get("reason"),
        report.get("report_path"),
    ]
    col_list = ", ".join(cols)
    val_list = ", ".join(_q(v) for v in values)
    return f"INSERT INTO benchmark_runs ({col_list}) VALUES ({val_list});"


def render_benchmark_results_inserts(report: dict[str, Any]) -> list[str]:
    """Produce one INSERT per (run, task, run_idx) row."""
    now = _iso_now()
    run_id = report["run_id"]
    base_sha = report.get("base_sha")
    adapter_sha = report.get("adapter_manifest_sha")
    dataset_sha = report.get("dataset_sha")
    stmts: list[str] = []
    for r in report.get("benchmark_results_rows", []):
        cols = [
            "recorded_at", "run_id", "task_id", "run_idx",
            "sampling_group", "response_text", "reward_vector",
            "judge_scores", "judge_meta", "latency_ms",
            "adapter_sha", "base_sha", "dataset_sha",
        ]
        values = [
            now,
            run_id,
            r.get("task_id"),
            r.get("run_idx"),
            r.get("sampling_group"),
            r.get("response_text"),
            r.get("reward_vector") or {},
            r.get("judge_scores") or {},
            r.get("judge_meta") or {},
            r.get("latency_ms"),
            adapter_sha,
            base_sha,
            dataset_sha,
        ]
        col_list = ", ".join(cols)
        val_list = ", ".join(_q(v) for v in values)
        stmts.append(
            f"INSERT INTO benchmark_results ({col_list}) VALUES ({val_list});"
        )
    return stmts


def write_sql_sidecar(report: dict[str, Any], out_sql: Path) -> int:
    """Write all INSERTs to `out_sql`, return count of INSERT statements."""
    results_inserts = render_benchmark_results_inserts(report)
    lines: list[str] = [
        "-- Phase 10 benchmark persistence (V0012 schema)",
        f"-- run_id: {report.get('run_id')}",
        f"-- started_at: {report.get('started_at')}",
        "BEGIN;",
        render_benchmark_runs_insert(report),
    ]
    lines.extend(results_inserts)
    lines.append("COMMIT;")
    out_sql.write_text("\n".join(lines) + "\n")
    # 1 benchmark_runs INSERT + N benchmark_results INSERTs
    return 1 + len(results_inserts)


# ─── BENCH-SIDECAR-APPLY-001: direct TSDB apply ──────────────────────────────

def tsdb_dsn_from_env() -> str:
    """libpq DSN from TSDB_* env vars — same names/defaults as
    docs/tests/uvts/runners/uvts_runner.py::_tsdb_dsn (the established
    Python-side pattern)."""
    import os
    return (
        f"host={os.environ.get('TSDB_HOST', 'localhost')} "
        f"port={os.environ.get('TSDB_PORT', '5433')} "
        f"user={os.environ.get('TSDB_USER', 'mdemg')} "
        f"password={os.environ.get('TSDB_PASSWORD', 'mdemg_metrics')} "
        f"dbname={os.environ.get('TSDB_DB', 'mdemg_metrics')}"
    )


def apply_to_tsdb(report: dict[str, Any], dsn: str | None = None, _connect=None) -> int:
    """Execute the V0012 INSERTs directly against TSDB in one transaction.

    Returns the number of statements applied. Raises on connect/execute
    failure — the CALLER decides whether that is fatal (run_benchmark treats
    it as non-fatal: the sidecar file remains the recovery path, mirroring
    the UVTS runner's best-effort persistence).

    `_connect` is a test seam (defaults to psycopg.connect).
    """
    if _connect is None:
        import psycopg  # deferred: not a hard dep for JSON-only users
        _connect = psycopg.connect

    stmts = [render_benchmark_runs_insert(report)]
    stmts.extend(render_benchmark_results_inserts(report))

    conn = _connect(dsn or tsdb_dsn_from_env(), connect_timeout=5)
    try:
        with conn.cursor() as cur:
            for stmt in stmts:
                cur.execute(stmt)
        conn.commit()
    finally:
        conn.close()
    return len(stmts)
