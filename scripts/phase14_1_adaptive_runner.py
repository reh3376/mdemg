#!/usr/bin/env python3
"""
Phase 14.1 Epic 3 — Adaptive per-category sparse-gate A/B runner.

Sweeps three preset shapes against the current production baseline (sparse-off):
  - adaptive-arch-only      — architecture_structure: min_active=20
  - adaptive-arch-data-flow — adds data_flow_integration: min_active=15
  - adaptive-all-affected   — adds business_logic_constraints + cross_cutting_concerns

Per Phase 14.1 forensic (phase_14_1_forensic.md), the override map is conservative
→ aggressive. A/B verdict drives which preset's defaults flip default-on at sprint
close.

Usage:
  scripts/phase14_1_adaptive_runner.py --profile quick --out-dir /tmp/phase14_1_quick

Mirrors phase14_epic2_sparse_ablation.py shape — same .env mutate → restart →
uvts → ab_compare → restore lifecycle.
"""
from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

REPO_ROOT = Path(__file__).resolve().parent.parent
ENV_PATH = REPO_ROOT / ".env"
ENV_BACKUP = REPO_ROOT / ".env.bak.phase14_1"
UVTS_RUNNER = REPO_ROOT / "docs/tests/uvts/runners/uvts_runner.py"
UVTS_AB_COMPARE = REPO_ROOT / "docs/tests/uvts/runners/uvts_ab_compare.py"
UVTS_SPEC = REPO_ROOT / "docs/tests/uvts/specs/lnl_demo_validation.uvts.json"


@dataclass
class Preset:
    name: str
    sparse_enabled: bool = True
    percentile: float = 0.95
    min_active: int = 3
    max_active: int = 20
    category_overrides: dict[str, dict[str, Any]] = field(default_factory=dict)
    note: str = ""

    def env_vars(self) -> dict[str, str]:
        out = {
            "SPARSE_RETRIEVAL_ENABLED": "true" if self.sparse_enabled else "false",
            "SPARSE_ACTIVATION_PERCENTILE": f"{self.percentile:g}",
            "SPARSE_MIN_ACTIVE": str(self.min_active),
            "SPARSE_MAX_ACTIVE": str(self.max_active),
        }
        if self.category_overrides:
            out["SPARSE_GATE_CATEGORY_OVERRIDES"] = json.dumps(self.category_overrides)
        else:
            # Explicitly clear so a previous preset's override doesn't leak.
            out["SPARSE_GATE_CATEGORY_OVERRIDES"] = ""
        return out


SWEEP_PRESETS = [
    Preset(
        name="baseline-sparse-off",
        sparse_enabled=False,
        note="Current production state — Phase 13.1 weights, gate off",
    ),
    Preset(
        name="adaptive-arch-only",
        sparse_enabled=True,
        percentile=0.95,
        min_active=10,
        max_active=20,
        category_overrides={"architecture_structure": {"min_active": 20}},
        note="Conservative — arch_struct passes through (no-op for that category), gate active for 7 others",
    ),
    Preset(
        name="adaptive-arch-data-flow",
        sparse_enabled=True,
        percentile=0.95,
        min_active=10,
        max_active=20,
        category_overrides={
            "architecture_structure": {"min_active": 20},
            "data_flow_integration": {"min_active": 15},
        },
        note="Moderate — extends arch override + data_flow MIN=15",
    ),
    Preset(
        name="adaptive-all-affected",
        sparse_enabled=True,
        percentile=0.95,
        min_active=10,
        max_active=20,
        category_overrides={
            "architecture_structure": {"min_active": 20},
            "data_flow_integration": {"min_active": 15},
            "business_logic_constraints": {"min_active": 15},
            "cross_cutting_concerns": {"min_active": 15},
        },
        note="Aggressive — overrides all 4 categories with regressions in Phase 14",
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
    """Set or replace each key in .env. Empty value clears the key entirely."""
    lines = ENV_PATH.read_text().splitlines()
    seen = set()
    out = []
    for line in lines:
        if "=" in line and not line.strip().startswith("#"):
            key = line.split("=", 1)[0].strip()
            if key in updates:
                if updates[key] == "":
                    seen.add(key)
                    continue  # drop line
                out.append(f"{key}={updates[key]}")
                seen.add(key)
                continue
        out.append(line)
    for key, val in updates.items():
        if key not in seen and val != "":
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
        sys.executable, str(UVTS_AB_COMPARE),
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
    return {"verdict": "error", "exit_code": r.returncode}


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
    ap.add_argument("--out-dir", default="/tmp/phase14_1_quick")
    ap.add_argument("--force", action="store_true")
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
                if not run_uvts(preset_dir, args.profile, f"phase14_1_{preset.name}"):
                    print(f"[abort] uvts failed for {preset.name}", file=sys.stderr)
                    continue
            rows.append({
                "preset": preset.name,
                "note": preset.note,
                "summary": summarize(grades_path),
            })
    finally:
        restore_env()
        restart_mdemg()

    if not rows:
        print("[done] no rows produced", file=sys.stderr)
        return 1

    baseline_grades = out_dir / rows[0]["preset"] / "grades.json"
    print(f"\n=== A/B verdicts (baseline = {rows[0]['preset']}) ===")
    for row in rows[1:]:
        cand_grades = out_dir / row["preset"] / "grades.json"
        verdict_path = out_dir / row["preset"] / "verdict.json"
        verdict = run_ab_compare(baseline_grades, cand_grades, verdict_path)
        v = verdict.get("verdict", verdict)
        if isinstance(v, dict):
            row["verdict"] = v.get("verdict")
            row["mean_delta"] = v.get("mean_delta")
            row["regression_count"] = v.get("regression_count")
            row["improvement_count"] = v.get("improvement_count")
        else:
            row["verdict"] = "error"

    lines = [f"# Phase 14.1 Epic 3 — Adaptive Per-Category A/B Sweep ({args.profile})\n"]
    lines.append("| Preset | n | mean | regression_count | improvement_count | mean_delta | verdict |")
    lines.append("|---|---|---|---|---|---|---|")
    for row in rows:
        s = row["summary"]
        lines.append("| {p} | {n} | {m} | {r} | {imp} | {d} | {v} |".format(
            p=row["preset"], n=s.get("n", "—"), m=s.get("mean", "—"),
            r=row.get("regression_count", "—"),
            imp=row.get("improvement_count", "—"),
            d=row.get("mean_delta", "—"),
            v=row.get("verdict", "—"),
        ))
    lines.append("")
    for row in rows:
        lines.append(f"## {row['preset']}\n\n{row.get('note','')}\n")
    summary_md.write_text("\n".join(lines) + "\n")
    print(f"\n[done] summary → {summary_md}")
    for row in rows:
        print(json.dumps(row, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
