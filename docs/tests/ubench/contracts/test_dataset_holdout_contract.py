"""UBENCH dataset↔holdout contract tests (pytest wrapper).

Calls ubench_runner.py in --mode contract and asserts pass on every
.ubench.json under docs/tests/ubench/specs/. This is the programmatic
forcing function preventing the class of gap surfaced by Phase 10's
guardrail.evaluate (spec exists, golden rows missing).

Wire-in:
    pytest docs/tests/ubench/contracts/ -v
    make test-ubench-contract
"""
from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[4]
RUNNER = REPO_ROOT / "docs" / "tests" / "ubench" / "runners" / "ubench_runner.py"
SPECS_DIR = REPO_ROOT / "docs" / "tests" / "ubench" / "specs"


def _ubench_specs() -> list[Path]:
    return sorted(SPECS_DIR.glob("*.ubench.json"))


def _run_contract(spec: Path, tmp_path: Path) -> dict:
    report = tmp_path / f"{spec.stem}.report.json"
    rc = subprocess.run(
        [
            sys.executable,
            str(RUNNER),
            "--spec",
            str(spec),
            "--mode",
            "contract",
            "--report",
            str(report),
            "--quiet",
        ],
        cwd=str(REPO_ROOT),
        capture_output=True,
        text=True,
        check=False,
    )
    return {
        "rc": rc.returncode,
        "stdout": rc.stdout,
        "stderr": rc.stderr,
        "report": json.loads(report.read_text()) if report.is_file() else None,
    }


def test_at_least_one_ubench_spec_exists():
    specs = _ubench_specs()
    assert specs, f"no .ubench.json specs found under {SPECS_DIR}"


def test_each_spec_passes_contract(tmp_path):
    specs = _ubench_specs()
    failures = []
    for spec in specs:
        result = _run_contract(spec, tmp_path)
        if result["rc"] != 0:
            failures.append(
                {
                    "spec": str(spec.relative_to(REPO_ROOT)),
                    "rc": result["rc"],
                    "report": result["report"],
                    "stderr_tail": result["stderr"][-500:],
                }
            )

    assert not failures, (
        "UBENCH contract failed for one or more specs:\n"
        + json.dumps(failures, indent=2)
    )
