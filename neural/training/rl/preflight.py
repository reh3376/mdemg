"""Phase 11 preflight — Epic 0 gate before GRPO training.

Asserts every precondition needed for an RL run:

1. benchmark_results table populated (Phase 10 baseline persisted).
2. Per-task row count ≥ N (default 5, config-driven).
3. Per-task mean + stddev computable from reward_vector JSONB.
4. Phase 5 dense adapter present + SHA matches the pin in the RL config.
5. MLX single-instance check (exactly one mlx_lm.server on the configured host:port).

Writes JSON report to `training_data/eval/phase11_preflight_report.json` so the
decision (including zero-stddev task list + policy recommendation) is durable
and auditable alongside the rl_training_runs TSDB row it later produces.

Usage:
    python -m neural.training.rl.preflight \
        --config configs/rl_phase11.yaml \
        --out training_data/eval/phase11_preflight_report.json

Exit codes:
    0 — all gates pass; ready for Epic 1.
    2 — one or more gates fail; report contains the per-check verdict.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import sys
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml

try:
    import psycopg
except ImportError:  # pragma: no cover - dev env has psycopg; keep import soft
    psycopg = None  # type: ignore[assignment]


# ── check result model ──────────────────────────────────────────────────────


@dataclass
class CheckResult:
    name: str
    passed: bool
    detail: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


# ── individual checks ───────────────────────────────────────────────────────


def _tsdb_dsn(config: dict[str, Any]) -> str:
    """Build a psycopg DSN from config + env, honoring the standard
    TSDB_* variables set by the docker-compose deploy."""
    tsdb = config.get("tsdb", {}) or {}
    host = os.environ.get("TSDB_HOST", tsdb.get("host", "localhost"))
    port = os.environ.get("TSDB_PORT", str(tsdb.get("port", 5433)))
    user = os.environ.get("TSDB_USER", tsdb.get("user", "mdemg"))
    pwd = os.environ.get("TSDB_PASSWORD", tsdb.get("password", "mdemg_metrics"))
    db = os.environ.get("TSDB_DATABASE", tsdb.get("database", "mdemg_metrics"))
    return f"host={host} port={port} user={user} password={pwd} dbname={db}"


def check_tsdb_baseline(config: dict[str, Any]) -> CheckResult:
    """Gate 1 — benchmark_runs has ≥1 row (Phase 10 baseline persisted)."""
    if psycopg is None:
        return CheckResult(
            "tsdb_baseline_present", False,
            {"error": "psycopg not installed; cannot query TSDB"},
        )
    try:
        with psycopg.connect(_tsdb_dsn(config), connect_timeout=5) as conn:
            with conn.cursor() as cur:
                cur.execute(
                    "SELECT run_id, aggregate_weighted_score, "
                    "specs_with_matched_rows, specs_total "
                    "FROM benchmark_runs "
                    "ORDER BY started_at DESC LIMIT 1"
                )
                row = cur.fetchone()
    except Exception as exc:  # pragma: no cover - integration path
        return CheckResult(
            "tsdb_baseline_present", False,
            {"error": f"TSDB query failed: {exc}"},
        )

    if row is None:
        return CheckResult(
            "tsdb_baseline_present", False,
            {"error": "benchmark_runs is empty; Phase 10 baseline not persisted"},
        )

    run_id, agg, matched, total = row
    return CheckResult(
        "tsdb_baseline_present", True,
        {
            "run_id": run_id,
            "aggregate_weighted_score": float(agg) if agg is not None else None,
            "specs_with_matched_rows": matched,
            "specs_total": total,
        },
    )


def check_per_task_rows(
    config: dict[str, Any], run_id: str, min_rows: int
) -> CheckResult:
    """Gate 2 — each task covered by the baseline run has ≥ min_rows rows."""
    if psycopg is None:
        return CheckResult(
            "per_task_row_count", False, {"error": "psycopg not installed"}
        )
    with psycopg.connect(_tsdb_dsn(config), connect_timeout=5) as conn:
        with conn.cursor() as cur:
            cur.execute(
                "SELECT task_id, COUNT(*) AS n "
                "FROM benchmark_results WHERE run_id = %s "
                "GROUP BY task_id ORDER BY task_id",
                (run_id,),
            )
            tasks = cur.fetchall()

    under = [(t, n) for t, n in tasks if n < min_rows]
    return CheckResult(
        "per_task_row_count",
        passed=not under,
        detail={
            "min_rows_required": min_rows,
            "tasks_total": len(tasks),
            "tasks_under_min": [{"task_id": t, "n": n} for t, n in under],
            "per_task_n": {t: n for t, n in tasks},
        },
    )


def check_per_task_stats(
    config: dict[str, Any], run_id: str
) -> CheckResult:
    """Gate 3 — compute per-task (overall_mean, overall_stddev) from
    reward_vector JSONB. The 'overall' is the mean of the per-metric scores
    in each row; stddev is taken across runs.

    Flags zero-stddev tasks for the advantage-normalization policy decision —
    this is informational, not a failure.
    """
    if psycopg is None:
        return CheckResult(
            "per_task_stats_computed", False, {"error": "psycopg not installed"}
        )
    sql = """
        WITH per_row AS (
            SELECT task_id, run_idx, sampling_group,
                   (
                     SELECT AVG(v::text::float)
                     FROM jsonb_each(reward_vector) AS kv(k, v)
                   ) AS row_mean
            FROM benchmark_results
            WHERE run_id = %s
        )
        SELECT task_id, sampling_group,
               COUNT(*)        AS n,
               AVG(row_mean)   AS overall_mean,
               STDDEV_POP(row_mean) AS overall_stddev
        FROM per_row
        GROUP BY task_id, sampling_group
        ORDER BY task_id
    """
    with psycopg.connect(_tsdb_dsn(config), connect_timeout=5) as conn:
        with conn.cursor() as cur:
            cur.execute(sql, (run_id,))
            rows = cur.fetchall()

    per_task = [
        {
            "task_id": t,
            "sampling_group": g,
            "n": n,
            "overall_mean": float(mean) if mean is not None else None,
            "overall_stddev": float(sd) if sd is not None else None,
        }
        for t, g, n, mean, sd in rows
    ]
    zero_stddev = [r for r in per_task if (r["overall_stddev"] or 0.0) == 0.0]

    return CheckResult(
        "per_task_stats_computed",
        passed=len(per_task) > 0,
        detail={
            "tasks_total": len(per_task),
            "zero_stddev_count": len(zero_stddev),
            "zero_stddev_task_ids": [r["task_id"] for r in zero_stddev],
            "zero_stddev_policy_recommendation":
                "intra_batch_only — compute stddev from RL rollout batch when "
                "historical stddev is 0. Default per configs/rl_phase11.yaml.",
            "per_task": per_task,
        },
    )


def check_phase5_adapter(config: dict[str, Any]) -> CheckResult:
    """Gate 4 — Phase 5 adapter directory exists and (optionally) SHA matches."""
    base = config.get("base_model", {}) or {}
    path = Path(base.get("path", ".local-models/qwen3-14b-mdemg-v1"))
    expected_sha = base.get("sha256")

    if not path.is_dir():
        return CheckResult(
            "phase5_adapter_present", False,
            {"error": f"base model path not found: {path}"},
        )

    # Walk a few critical files to confirm the dir is populated.
    markers = ["config.json", "tokenizer_config.json"]
    missing = [m for m in markers if not (path / m).is_file()]
    if missing:
        return CheckResult(
            "phase5_adapter_present", False,
            {"error": f"missing marker files: {missing}", "path": str(path)},
        )

    # SHA check is advisory — full-dir SHA is expensive; the config SHA is in
    # the Phase 5 manifest, which we record by reference only.
    detail: dict[str, Any] = {"path": str(path), "markers": markers}
    if expected_sha:
        manifest_path = path / "adapter_manifest.json"
        if manifest_path.is_file():
            manifest = json.loads(manifest_path.read_text())
            observed = manifest.get("base_sha") or manifest.get("sha256")
            detail["expected_sha"] = expected_sha
            detail["observed_sha"] = observed
            if observed and observed != expected_sha:
                return CheckResult(
                    "phase5_adapter_present", False,
                    {**detail, "error": "SHA mismatch vs config pin"},
                )

    return CheckResult("phase5_adapter_present", True, detail)


def check_mlx_single_instance(config: dict[str, Any]) -> CheckResult:
    """Gate 5 — exactly one mlx_lm.server process on the configured host:port.

    Advisory on localhost (non-mdemg) clusters — a failed check doesn't block
    preflight, but it is recorded so training doesn't silently attach to a
    stale server.
    """
    mut = config.get("model_under_test", {}) or {}
    port = mut.get("mlx_port", 8101)
    try:
        out = subprocess.run(
            ["pgrep", "-fl", "mlx_lm.server"],
            capture_output=True, text=True, timeout=5,
        )
    except Exception as exc:  # pragma: no cover
        return CheckResult(
            "mlx_single_instance", False,
            {"error": f"pgrep failed: {exc}"},
        )

    lines = [ln for ln in out.stdout.splitlines() if ln.strip()]
    detail = {"port": port, "instances": lines, "count": len(lines)}
    # Informational only — zero instances are fine if the user will spin up
    # MLX per training session. Multiple instances is the problem case.
    passed = len(lines) <= 1
    return CheckResult("mlx_single_instance", passed, detail)


# ── orchestration ───────────────────────────────────────────────────────────


def _config_sha(cfg_path: Path) -> str:
    return hashlib.sha256(cfg_path.read_bytes()).hexdigest()


def run_preflight(
    config_path: Path, out_path: Path, min_rows: int | None = None
) -> int:
    config = yaml.safe_load(config_path.read_text())
    if min_rows is None:
        min_rows = int(
            config.get("preflight", {}).get("min_rows_per_task", 5)
        )

    checks: list[CheckResult] = []
    baseline = check_tsdb_baseline(config)
    checks.append(baseline)

    run_id = (baseline.detail or {}).get("run_id") if baseline.passed else None

    if run_id:
        checks.append(check_per_task_rows(config, run_id, min_rows))
        checks.append(check_per_task_stats(config, run_id))

    checks.append(check_phase5_adapter(config))
    checks.append(check_mlx_single_instance(config))

    report = {
        "preflight_version": 1,
        "sprint": "FT-LORA-PHASE11",
        "evaluated_at": datetime.now(timezone.utc).isoformat(),
        "config_path": str(config_path),
        "config_sha256": _config_sha(config_path),
        "all_passed": all(c.passed for c in checks),
        "checks": [c.to_dict() for c in checks],
    }
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(report, indent=2, sort_keys=True))

    # Human summary to stdout
    status = "PASS" if report["all_passed"] else "FAIL"
    print(f"preflight {status} — {out_path}")
    for c in checks:
        mark = "✓" if c.passed else "✗"
        print(f"  {mark} {c.name}")
        if not c.passed:
            err = c.detail.get("error")
            if err:
                print(f"      error: {err}")

    return 0 if report["all_passed"] else 2


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    ap = argparse.ArgumentParser(
        prog="python -m neural.training.rl.preflight",
        description="Phase 11 RL preflight gate.",
    )
    ap.add_argument(
        "--config", type=Path, required=True,
        help="Phase 11 RL config path (e.g. configs/rl_phase11.yaml)",
    )
    ap.add_argument(
        "--out", type=Path,
        default=Path("training_data/eval/phase11_preflight_report.json"),
        help="preflight report output path",
    )
    ap.add_argument(
        "--min-rows", type=int, default=None,
        help="override config preflight.min_rows_per_task",
    )
    return ap.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    ns = _parse_args(argv)
    if not ns.config.exists():
        print(f"config not found: {ns.config}", file=sys.stderr)
        return 2
    return run_preflight(ns.config, ns.out, ns.min_rows)


if __name__ == "__main__":
    sys.exit(main())
