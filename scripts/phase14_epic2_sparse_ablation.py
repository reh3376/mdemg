#!/usr/bin/env python3
"""
Phase 14 Epic 2 — Note 06 sparse-gate A/B ablation runner.

Sweeps SPARSE_ACTIVATION_PERCENTILE ∈ {0.95, 0.98, 0.99} against the current
production baseline (SPARSE_RETRIEVAL_ENABLED=false, Phase 13.1 embedding-heavy
weights). Acceptance per Note 02 merge gate: B mean ≥ A mean AND no per-
question regression > 10%.

Usage:
  scripts/phase14_epic2_sparse_ablation.py --profile quick --out-dir /tmp/phase14_epic2

Per preset, the runner:
  1. Mutates .env (SPARSE_RETRIEVAL_ENABLED + SPARSE_ACTIVATION_PERCENTILE).
  2. Restarts mdemg (launchctl bootout/bootstrap).
  3. Waits for /healthz ok.
  4. Runs uvts_runner.py against whk-wms + requested profile.
  5. Runs uvts_ab_compare.py against the baseline grades.json.
  6. Captures verdict JSON.

Idempotent: skips if <out-dir>/<preset>/grades.json already exists. --force re-runs.

Mirrors the Phase 13.1 runner shape but trimmed to sparse-gate knobs.
"""
from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parent.parent
ENV_PATH = REPO_ROOT / ".env"
ENV_BACKUP = REPO_ROOT / ".env.bak.phase14_epic2"
UVTS_RUNNER = REPO_ROOT / "docs/tests/uvts/runners/uvts_runner.py"
UVTS_AB_COMPARE = REPO_ROOT / "docs/tests/uvts/runners/uvts_ab_compare.py"
UVTS_SPEC = REPO_ROOT / "docs/tests/uvts/specs/lnl_demo_validation.uvts.json"


@dataclass
class Preset:
    name: str
    sparse_enabled: bool = True
    percentile: float = 0.95
    note: str = ""

    def env_vars(self) -> dict[str, str]:
        return {
            "SPARSE_RETRIEVAL_ENABLED": "true" if self.sparse_enabled else "false",
            "SPARSE_ACTIVATION_PERCENTILE": f"{self.percentile:g}",
        }


# A/B sweep — one baseline (Phase 13.1 production = sparse off) + three
# candidates at the three percentile choices the spec / Epic 0 contemplated.
SWEEP_PRESETS = [
    Preset(
        name="baseline-sparse-off",
        sparse_enabled=False,
        note="Current production state — Phase 13.1 embedding-heavy weights, sparse gate disabled",
    ),
    Preset(
        name="sparse-p95",
        sparse_enabled=True,
        percentile=0.95,
        note="Epic 0 recommendation — clamp-dominated regime; identical-to-MIN_ACTIVE in K=20",
    ),
    Preset(
        name="sparse-p98",
        sparse_enabled=True,
        percentile=0.98,
        note="Spec default — strictest within-call gate",
    ),
    Preset(
        name="sparse-p99",
        sparse_enabled=True,
        percentile=0.99,
        note="Most aggressive — tests whether strictness past p98 changes outcome (it shouldn't given clamp)",
    ),
]


def backup_env() -> None:
    if not ENV_BACKUP.exists():
        shutil.copy2(ENV_PATH, ENV_BACKUP)
        print(f"[backup] saved {ENV_BACKUP}")


def restore_env() -> None:
    if ENV_BACKUP.exists():
        shutil.copy2(ENV_BACKUP, ENV_PATH)
        print(f"[restore] restored {ENV_PATH}")


def edit_env(updates: dict[str, str]) -> None:
    """Set or replace each key in .env. Idempotent."""
    lines = ENV_PATH.read_text().splitlines()
    seen = set()
    out = []
    for line in lines:
        if "=" in line and not line.strip().startswith("#"):
            key = line.split("=", 1)[0].strip()
            if key in updates:
                out.append(f"{key}={updates[key]}")
                seen.add(key)
                continue
        out.append(line)
    for key, val in updates.items():
        if key not in seen:
            out.append(f"{key}={val}")
    ENV_PATH.write_text("\n".join(out) + "\n")


def restart_mdemg() -> bool:
    uid = os.getuid()
    plist = Path.home() / "Library/LaunchAgents/com.mdemg.server.plist"
    subprocess.run(
        ["launchctl", "bootout", f"gui/{uid}/com.mdemg.server"],
        capture_output=True, check=False,
    )
    time.sleep(2)
    r = subprocess.run(
        ["launchctl", "bootstrap", f"gui/{uid}", str(plist)],
        capture_output=True, text=True,
    )
    if r.returncode != 0:
        print(f"[restart] bootstrap failed: {r.stderr}", file=sys.stderr)
        return False
    # Wait for /healthz ok (up to 30s).
    for _ in range(30):
        time.sleep(1)
        try:
            r = subprocess.run(
                ["curl", "-sf", "http://localhost:9999/healthz"],
                capture_output=True, text=True, timeout=2,
            )
            if r.returncode == 0:
                return True
        except Exception:
            continue
    print("[restart] /healthz never came up", file=sys.stderr)
    return False


def run_uvts(preset_dir: Path, profile: str, branch_label: str, space_id: str = "whk-wms") -> bool:
    preset_dir.mkdir(parents=True, exist_ok=True)
    sha = subprocess.run(["git", "rev-parse", "HEAD"], capture_output=True, text=True).stdout.strip() or "unknown"
    cmd = [
        sys.executable, str(UVTS_RUNNER),
        "--spec", str(UVTS_SPEC),
        "--base-url", "http://localhost:9999",
        "--profile", profile,
        "--space-id", space_id,
        "--persist-tsdb",
        "--branch-label", branch_label,
        "--codebase-sha", sha,
        "--output-dir", str(preset_dir),
        "--report", str(preset_dir.parent / f"{preset_dir.name}-report.json"),
    ]
    print(f"[uvts] running profile={profile} → {preset_dir}/grades.json")
    log_path = preset_dir.parent / f"{preset_dir.name}.log"
    res = subprocess.run(cmd, capture_output=True, text=True, timeout=3600)
    log_path.write_text(res.stdout + "\n--- STDERR ---\n" + res.stderr)
    grades_path = preset_dir / "grades.json"
    if not grades_path.exists():
        print(f"[uvts] failed; see {log_path}", file=sys.stderr)
        return False
    return True


def run_ab_compare(baseline: Path, candidate: Path, out: Path) -> dict[str, Any]:
    cmd = [
        "python3", str(UVTS_AB_COMPARE),
        "--baseline", str(baseline),
        "--candidate", str(candidate),
        "--spec", str(UVTS_SPEC),
        "--out", str(out),
    ]
    r = subprocess.run(cmd, capture_output=True, text=True)
    print(f"[ab_compare] rc={r.returncode}")
    if r.stdout:
        print(r.stdout[:500])
    if out.exists():
        return json.loads(out.read_text())
    return {"verdict": "error", "error": r.stderr or r.stdout, "exit_code": r.returncode}


def summarize(grades_path: Path) -> dict[str, Any]:
    d = json.load(grades_path.open())
    grades = d.get("grades", [])
    scores = [g.get("scores", {}).get("final") for g in grades]
    scores = [s for s in scores if s is not None]
    return {
        "n": len(scores),
        "mean": round(sum(scores) / len(scores), 4) if scores else None,
        "min": round(min(scores), 4) if scores else None,
        "max": round(max(scores), 4) if scores else None,
    }


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--profile", default="quick", choices=["quick", "standard", "full"])
    ap.add_argument("--out-dir", default="/tmp/phase14_epic2")
    ap.add_argument("--force", action="store_true",
                    help="Re-run presets even if grades.json exists")
    args = ap.parse_args()

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    summary_md = out_dir / "ablation_summary.md"

    backup_env()
    rows = []
    try:
        for preset in SWEEP_PRESETS:
            preset_dir = out_dir / preset.name
            grades_path = preset_dir / "grades.json"
            if grades_path.exists() and not args.force:
                print(f"[skip] {preset.name} grades.json exists; --force to re-run")
            else:
                edit_env(preset.env_vars())
                if not restart_mdemg():
                    print(f"[abort] mdemg restart failed for {preset.name}", file=sys.stderr)
                    continue
                if not run_uvts(preset_dir, args.profile, f"phase14_e2_{preset.name}"):
                    print(f"[abort] uvts failed for {preset.name}", file=sys.stderr)
                    continue

            row = {
                "preset": preset.name,
                "note": preset.note,
                "summary": summarize(grades_path),
            }
            rows.append(row)
    finally:
        # Always restore .env + restart so baseline state returns even on
        # interrupt.
        restore_env()
        restart_mdemg()

    # Now compute A/B verdicts: baseline = first row's grades.json.
    if not rows:
        print("[done] no rows produced", file=sys.stderr)
        return 1

    baseline_grades = out_dir / rows[0]["preset"] / "grades.json"
    print(f"\n=== A/B verdicts (baseline = {rows[0]['preset']}) ===")
    for row in rows[1:]:
        cand_grades = out_dir / row["preset"] / "grades.json"
        verdict_path = out_dir / row["preset"] / "verdict.json"
        verdict = run_ab_compare(baseline_grades, cand_grades, verdict_path)
        row["verdict"] = verdict.get("verdict")
        row["delta_mean"] = verdict.get("delta_mean")
        row["regressions_above_threshold"] = verdict.get("regressions_above_threshold")

    # Write summary doc.
    lines = ["# Phase 14 Epic 2 — Sparse Gate A/B Sweep\n"]
    lines.append(f"Profile: **{args.profile}**\n")
    lines.append("| Preset | n | mean | min | max | verdict | Δmean | regressions>10% |")
    lines.append("|---|---|---|---|---|---|---|---|")
    for row in rows:
        s = row["summary"]
        lines.append("| {p} | {n} | {m} | {mn} | {mx} | {v} | {d} | {r} |".format(
            p=row["preset"], n=s.get("n", "—"),
            m=s.get("mean", "—"), mn=s.get("min", "—"), mx=s.get("max", "—"),
            v=row.get("verdict", "—"),
            d=row.get("delta_mean", "—"),
            r=row.get("regressions_above_threshold", "—"),
        ))
    lines.append("")
    for row in rows:
        lines.append(f"## {row['preset']}\n\n{row.get('note','')}\n")
    summary_md.write_text("\n".join(lines) + "\n")
    print(f"\n[done] summary → {summary_md}")
    print(f"[done] {len(rows)} presets")
    for row in rows:
        print(json.dumps(row, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
