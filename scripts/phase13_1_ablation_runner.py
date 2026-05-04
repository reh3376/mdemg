#!/usr/bin/env python3
"""
Phase 13.1 ablation runner.

Sweeps a list of column-weight presets through mdemg, runs UVTS quick (or full)
profile per preset, and produces an A/B verdict against the operator-provided
baseline grades.json.

Usage:
  scripts/phase13_1_ablation_runner.py \
    --presets diagnostic-set \
    --profile quick \
    --baseline /tmp/uvts-baseline/grades.json \
    --out-dir /tmp/phase13_1_runs

Per preset, the runner:
  1. Mutates .env to set RETRIEVAL_COLUMN_VOTING_ENABLED=true + the preset's
     RETRIEVAL_COLUMN_WEIGHT_* + RETRIEVAL_STRUCTURAL_HOPS + per-column enable
     flags.
  2. Restarts mdemg via launchctl bootout/bootstrap.
  3. Waits for /healthz ok.
  4. Runs uvts_runner.py against the requested space + profile, persisting to
     TSDB + writing grades.json under <out-dir>/<preset>/.
  5. Runs uvts_ab_compare.py against the baseline grades.json. Captures the
     verdict JSON.
  6. Records summary row.

At end:
  - Restores .env from .env.bak.phase13_1.
  - Restarts mdemg.
  - Writes <out-dir>/ablation_summary.md with one row per preset.

Idempotent: if <out-dir>/<preset>/grades.json already exists, the preset is
skipped (UVTS not re-run). Use --force to re-run.

Phase 13.5 stable substrate (llama-server) is required. The script does NOT
flip the watchdog; if it's enabled, transient blips during preset transitions
may produce extra V0018 health rows — that's fine, they don't affect the A/B.
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
ENV_BACKUP = REPO_ROOT / ".env.bak.phase13_1"
UVTS_RUNNER = REPO_ROOT / "docs/tests/uvts/runners/uvts_runner.py"
UVTS_AB_COMPARE = REPO_ROOT / "docs/tests/uvts/runners/uvts_ab_compare.py"
UVTS_SPEC = REPO_ROOT / "docs/tests/uvts/specs/lnl_demo_validation.uvts.json"


@dataclass
class Preset:
    name: str
    weight_embedding: float = 1.0
    weight_bm25: float = 1.0
    weight_graph: float = 1.0
    weight_structural: float = 1.0
    structural_hops: int = 2
    embedding_enabled: bool = True
    bm25_enabled: bool = True
    graph_enabled: bool = True
    structural_enabled: bool = True
    note: str = ""

    def env_vars(self) -> dict[str, str]:
        return {
            "RETRIEVAL_COLUMN_VOTING_ENABLED": "true",
            "RETRIEVAL_COLUMN_WEIGHT_EMBEDDING": f"{self.weight_embedding:g}",
            "RETRIEVAL_COLUMN_WEIGHT_BM25": f"{self.weight_bm25:g}",
            "RETRIEVAL_COLUMN_WEIGHT_GRAPH": f"{self.weight_graph:g}",
            "RETRIEVAL_COLUMN_WEIGHT_STRUCTURAL": f"{self.weight_structural:g}",
            "RETRIEVAL_STRUCTURAL_HOPS": str(self.structural_hops),
            "RETRIEVAL_COLUMN_EMBEDDING_ENABLED": "true" if self.embedding_enabled else "false",
            "RETRIEVAL_COLUMN_BM25_ENABLED": "true" if self.bm25_enabled else "false",
            "RETRIEVAL_COLUMN_GRAPH_ENABLED": "true" if self.graph_enabled else "false",
            "RETRIEVAL_COLUMN_STRUCTURAL_ENABLED": "true" if self.structural_enabled else "false",
        }


# Diagnostic-aligned preset set — designed against forensic findings in
# phase_13_1_forensic_diagnosis.md. Order: weakest-evidence first to strongest.
DIAGNOSTIC_PRESETS = [
    Preset(
        name="equal-baseline",
        note="Reproduces Phase 13 v1-rrf4 — sanity check on the runner itself",
    ),
    Preset(
        name="embedding-heavy",
        weight_embedding=0.50,
        weight_bm25=0.20,
        weight_graph=0.15,
        weight_structural=0.15,
        note="Boosts Embedding alone — original sprint plan preset",
    ),
    Preset(
        name="embedding-bm25-priority",
        weight_embedding=0.40,
        weight_bm25=0.40,
        weight_graph=0.10,
        weight_structural=0.10,
        note="Most diagnostic-aligned — caps Graph+Structural at 20% combined",
    ),
    Preset(
        name="structural-suppress",
        weight_embedding=0.40,
        weight_bm25=0.30,
        weight_graph=0.25,
        weight_structural=0.05,
        note="Near-zeroes Structural — original sprint plan preset",
    ),
    Preset(
        name="embedding-heavy-hops1",
        weight_embedding=0.50,
        weight_bm25=0.20,
        weight_graph=0.15,
        weight_structural=0.15,
        structural_hops=1,
        note="Combines weight + hop-depth fixes — diagnostic Test C generalization",
    ),
]

PRESET_SETS: dict[str, list[Preset]] = {
    "diagnostic-set": DIAGNOSTIC_PRESETS,
}


@dataclass
class PresetResult:
    name: str
    out_dir: Path
    verdict: str = "unknown"
    baseline_mean: float | None = None
    candidate_mean: float | None = None
    mean_delta: float | None = None
    regression_count: int | None = None
    improvement_count: int | None = None
    error: str | None = None
    note: str = ""


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--presets", default="diagnostic-set", choices=list(PRESET_SETS.keys()))
    p.add_argument("--profile", default="quick", choices=["quick", "standard", "full"])
    p.add_argument("--space-id", default="whk-wms")
    p.add_argument("--baseline", default="/tmp/uvts-baseline/grades.json")
    p.add_argument("--out-dir", default="/tmp/phase13_1_runs")
    p.add_argument("--retrieve-timeout-s", type=int, default=90)
    p.add_argument("--mdemg-port", type=int, default=9999)
    p.add_argument("--llmcheckin-timeout-s", type=int, default=60, help="seconds to wait for /healthz after restart")
    p.add_argument("--dry-run", action="store_true", help="print what would run; don't mutate .env or restart mdemg")
    p.add_argument("--force", action="store_true", help="re-run preset even if grades.json already exists")
    p.add_argument("--restore-on-exit", action="store_true", default=True, help="restore .env on exit (default: true)")
    return p.parse_args()


def env_load(path: Path) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in path.read_text().splitlines():
        s = line.strip()
        if not s or s.startswith("#"):
            continue
        if "=" not in s:
            continue
        k, _, v = s.partition("=")
        out[k.strip()] = v.strip()
    return out


def env_apply(path: Path, overrides: dict[str, str]) -> None:
    """Rewrite .env in place, replacing or appending each key=value override.
    Other lines (comments, blanks) preserved verbatim."""
    seen: set[str] = set()
    new_lines: list[str] = []
    for line in path.read_text().splitlines(keepends=False):
        stripped = line.strip()
        if "=" in stripped and not stripped.startswith("#"):
            k = stripped.split("=", 1)[0].strip()
            if k in overrides:
                new_lines.append(f"{k}={overrides[k]}")
                seen.add(k)
                continue
        new_lines.append(line)
    for k, v in overrides.items():
        if k not in seen:
            new_lines.append(f"{k}={v}")
    path.write_text("\n".join(new_lines) + "\n")


def restart_mdemg(timeout_s: int) -> bool:
    subprocess.run(["launchctl", "bootout", "gui/501/com.mdemg.server"], capture_output=True)
    time.sleep(2)
    plist = Path.home() / "Library/LaunchAgents/com.mdemg.server.plist"
    subprocess.run(["launchctl", "bootstrap", "gui/501", str(plist)], capture_output=True)
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        try:
            r = subprocess.run(
                ["curl", "-s", "--max-time", "2", f"http://127.0.0.1:9999/healthz"],
                capture_output=True, text=True, timeout=5,
            )
            if "ok" in r.stdout:
                return True
        except subprocess.TimeoutExpired:
            pass
        time.sleep(2)
    return False


def run_uvts(out_dir: Path, profile: str, space_id: str, retrieve_timeout: int, branch_label: str) -> bool:
    out_dir.mkdir(parents=True, exist_ok=True)
    sha = subprocess.run(["git", "rev-parse", "HEAD"], capture_output=True, text=True).stdout.strip() or "unknown"
    cmd = [
        sys.executable, str(UVTS_RUNNER),
        "--spec", str(UVTS_SPEC),
        "--base-url", "http://localhost:9999",
        "--profile", profile,
        "--retrieve-timeout-s", str(retrieve_timeout),
        "--space-id", space_id,
        "--persist-tsdb",
        "--branch-label", branch_label,
        "--codebase-sha", sha,
        "--output-dir", str(out_dir),
        "--report", str(out_dir.parent / f"{out_dir.name}-report.json"),
    ]
    print(f"  > {' '.join(cmd[:6])} ...")
    res = subprocess.run(cmd, capture_output=True, text=True, timeout=1800)
    log_path = out_dir.parent / f"{out_dir.name}.log"
    log_path.write_text(res.stdout + "\n--- STDERR ---\n" + res.stderr)
    return (out_dir / "grades.json").exists()


def run_ab_compare(baseline: Path, candidate: Path, out_path: Path) -> dict[str, Any] | None:
    cmd = [
        sys.executable, str(UVTS_AB_COMPARE),
        "--baseline", str(baseline),
        "--candidate", str(candidate),
        "--spec", str(UVTS_SPEC),
        "--out", str(out_path),
    ]
    res = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    if not out_path.exists():
        return None
    try:
        return json.loads(out_path.read_text())
    except Exception:
        return None


def write_summary(results: list[PresetResult], out_path: Path, baseline: Path, profile: str) -> None:
    lines = [
        "# Phase 13.1 Ablation Summary",
        "",
        f"Generated: {time.strftime('%Y-%m-%d %H:%M:%S %Z')}",
        f"Baseline: `{baseline}`",
        f"Profile: `{profile}`",
        "",
        "## Per-preset verdicts",
        "",
        "| # | preset | verdict | baseline | candidate | delta | regressions | improvements | note |",
        "|---|---|---|---|---|---|---|---|---|",
    ]
    for i, r in enumerate(results, 1):
        bm = f"{r.baseline_mean:.3f}" if r.baseline_mean is not None else "—"
        cm = f"{r.candidate_mean:.3f}" if r.candidate_mean is not None else "—"
        dm = f"{r.mean_delta:+.3f}" if r.mean_delta is not None else "—"
        rc = str(r.regression_count) if r.regression_count is not None else "—"
        ic = str(r.improvement_count) if r.improvement_count is not None else "—"
        verdict_cell = r.verdict.upper()
        if r.error:
            verdict_cell = f"ERROR ({r.error[:40]})"
        lines.append(f"| {i} | `{r.name}` | {verdict_cell} | {bm} | {cm} | {dm} | {rc} | {ic} | {r.note} |")
    lines.extend([
        "",
        "## Acceptance criterion (canonical)",
        "",
        "B mean ≥ A mean AND no per-question regression > 10% (per `lnl_demo_validation.uvts.json::ab_mode.regression_threshold_per_question`).",
        "",
    ])
    out_path.write_text("\n".join(lines) + "\n")


def main() -> int:
    args = parse_args()
    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    baseline = Path(args.baseline)
    if not baseline.exists():
        print(f"ERROR: baseline file not found: {baseline}", file=sys.stderr)
        return 2
    presets = PRESET_SETS[args.presets]

    if args.dry_run:
        print(f"[dry-run] {len(presets)} presets would run against baseline={baseline}")
        for p in presets:
            print(f"  - {p.name}: {p.env_vars()}")
        return 0

    # Backup .env once. Restore at exit.
    if not ENV_BACKUP.exists():
        shutil.copy2(ENV_PATH, ENV_BACKUP)
        print(f"[init] backed up {ENV_PATH} → {ENV_BACKUP}")

    results: list[PresetResult] = []
    try:
        for i, preset in enumerate(presets, 1):
            preset_dir = out_dir / preset.name
            res = PresetResult(name=preset.name, out_dir=preset_dir, note=preset.note)
            print(f"\n=== [{i}/{len(presets)}] preset: {preset.name} ===")

            if (preset_dir / "grades.json").exists() and not args.force:
                print(f"  [skip] grades.json exists at {preset_dir/'grades.json'} — use --force to re-run")
                # still run AB compare to populate verdict
                ab_path = out_dir / f"{preset.name}-ab-verdict.json"
                ab = run_ab_compare(baseline, preset_dir / "grades.json", ab_path)
                if ab and "verdict" in ab:
                    v = ab["verdict"]
                    res.verdict = v.get("verdict", "unknown")
                    res.baseline_mean = v.get("baseline_mean")
                    res.candidate_mean = v.get("candidate_mean")
                    res.mean_delta = v.get("mean_delta")
                    res.regression_count = v.get("regression_count")
                    res.improvement_count = v.get("improvement_count")
                results.append(res)
                continue

            # Apply preset env
            env_apply(ENV_PATH, preset.env_vars())
            ok = restart_mdemg(args.llmcheckin_timeout_s)
            if not ok:
                res.error = "mdemg /healthz did not become ready"
                results.append(res)
                continue

            # Run UVTS
            branch = f"phase13_1-{preset.name}"
            ok = run_uvts(preset_dir, args.profile, args.space_id, args.retrieve_timeout_s, branch)
            if not ok:
                res.error = "uvts_runner did not produce grades.json"
                results.append(res)
                continue

            # Run AB compare
            ab_path = out_dir / f"{preset.name}-ab-verdict.json"
            ab = run_ab_compare(baseline, preset_dir / "grades.json", ab_path)
            if ab and "verdict" in ab:
                v = ab["verdict"]
                res.verdict = v.get("verdict", "unknown")
                res.baseline_mean = v.get("baseline_mean")
                res.candidate_mean = v.get("candidate_mean")
                res.mean_delta = v.get("mean_delta")
                res.regression_count = v.get("regression_count")
                res.improvement_count = v.get("improvement_count")
            else:
                res.error = "ab_compare did not produce a verdict"
            results.append(res)

            # Print preset summary
            print(f"  [done] verdict={res.verdict} baseline={res.baseline_mean} candidate={res.candidate_mean} delta={res.mean_delta} regressions={res.regression_count}")
    finally:
        if args.restore_on_exit and ENV_BACKUP.exists():
            print(f"\n[cleanup] restoring {ENV_PATH} from {ENV_BACKUP}")
            shutil.copy2(ENV_BACKUP, ENV_PATH)
            restart_mdemg(args.llmcheckin_timeout_s)

    # Write summary
    summary_path = out_dir / "ablation_summary.md"
    write_summary(results, summary_path, baseline, args.profile)
    print(f"\nSummary written: {summary_path}")
    print("\nFinal verdicts:")
    for r in results:
        marker = "✓" if r.verdict == "pass" else "✗"
        print(f"  {marker} {r.name}: {r.verdict} (delta={r.mean_delta} regressions={r.regression_count})")
    n_pass = sum(1 for r in results if r.verdict == "pass")
    return 0 if n_pass > 0 else 1


if __name__ == "__main__":
    sys.exit(main())
