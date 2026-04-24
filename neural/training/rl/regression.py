"""Phase 11 dual regression gate — Epic 5.

Orchestrates two back-to-back Phase 10 benchmark runs and compares both
against the Phase 5 SFT baseline (0.8338) + each other for adapter-merge
corruption detection.

Gate 5a (vs Phase 5 SFT baseline 0.8338):
    aggregate_sandbox ≥ 0.8338 × aggregate_target_multiplier  (default 1.02)
    AND ∀task: sandbox[task] ≥ baseline[task] − per_task_max_regression
Gate 5b (vs fresh dense-14B re-merge):
    |aggregate_sandbox − aggregate_fresh| ≤ fresh_merge_max_delta

Both gates must PASS for the Phase 11 adapter to be blessed to
``.local-models/qwen3-14b-mdemg-v1-rl/``. On any failure the adapter stays
in its sandbox path for post-mortem.

This module is framework-light — the Phase 10 benchmark runner is injected
as a callable so the Tier 2 integration test can mock it. Real runs invoke
``python -m neural.benchmarks.run_benchmark --config …``.
"""
from __future__ import annotations

import json
import logging
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable

logger = logging.getLogger(__name__)


# Shape the benchmark runner must return (subset of run_benchmark.py's report).
BenchmarkReport = dict[str, Any]
BenchmarkRunner = Callable[[str, str], BenchmarkReport]
# runner(config_path, adapter_path) → report dict


@dataclass
class RegressionConfig:
    """Values read from configs/rl_phase11.yaml §regression."""

    phase5_baseline_aggregate: float = 0.8338
    aggregate_target_multiplier: float = 1.02
    per_task_max_regression: float = 0.02
    fresh_merge_max_delta: float = 0.005


@dataclass
class GateResult:
    """One gate's pass/fail + diagnostics."""

    gate_id: str
    passed: bool
    aggregate_actual: float
    aggregate_target: float
    per_task_regressions: dict[str, float] = field(default_factory=dict)
    failures: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "gate_id": self.gate_id,
            "passed": self.passed,
            "aggregate_actual": self.aggregate_actual,
            "aggregate_target": self.aggregate_target,
            "per_task_regressions": self.per_task_regressions,
            "failures": self.failures,
        }


@dataclass
class DualGateReport:
    """Output artifact written to phase11_regression_report.json."""

    overall_verdict: str  # "pass" | "fail"
    gate_5a: GateResult
    gate_5b: GateResult
    adapter_paths: dict[str, str]
    baseline_paths: dict[str, str]
    # Raw benchmark reports saved inline for auditing.
    sandbox_report: BenchmarkReport | None = None
    fresh_report: BenchmarkReport | None = None

    def to_dict(self) -> dict[str, Any]:
        return {
            "overall_verdict": self.overall_verdict,
            "gate_5a": self.gate_5a.to_dict(),
            "gate_5b": self.gate_5b.to_dict(),
            "adapter_paths": self.adapter_paths,
            "baseline_paths": self.baseline_paths,
            "sandbox_report": self.sandbox_report,
            "fresh_report": self.fresh_report,
        }


# ── comparison logic ────────────────────────────────────────────────────────


def _per_task_scores(report: BenchmarkReport) -> dict[str, float]:
    """Pull per-task aggregate scores from a Phase 10 runner report.

    Phase 10's report persists ``per_task_aggregate`` as a dict keyed by
    task_id with a ``weighted_score`` field (see neural/benchmarks/run_benchmark.py).
    Fall back to ``per_task_scores`` for legacy shapes.
    """
    agg = report.get("per_task_aggregate") or {}
    if agg and all(isinstance(v, dict) for v in agg.values()):
        return {
            task: float(body.get("weighted_score", body.get("score", 0.0)))
            for task, body in agg.items()
        }
    # Legacy fallback.
    return {task: float(v) for task, v in (report.get("per_task_scores") or {}).items()}


def evaluate_gate_5a(
    sandbox_report: BenchmarkReport,
    baseline_report: BenchmarkReport,
    cfg: RegressionConfig,
) -> GateResult:
    """Gate 5a: sandbox adapter vs Phase 5 baseline.

    Pass iff:
        sandbox_aggregate ≥ baseline_aggregate × multiplier
        AND ∀task: sandbox[task] ≥ baseline[task] − per_task_max_regression
    """
    sandbox_agg = float(sandbox_report.get("aggregate_weighted_score", 0.0))
    target = cfg.phase5_baseline_aggregate * cfg.aggregate_target_multiplier

    failures: list[str] = []
    if sandbox_agg < target:
        failures.append(
            f"aggregate {sandbox_agg:.4f} below target "
            f"{target:.4f} (={cfg.phase5_baseline_aggregate:.4f} × "
            f"{cfg.aggregate_target_multiplier:.2f})"
        )

    sandbox_tasks = _per_task_scores(sandbox_report)
    baseline_tasks = _per_task_scores(baseline_report)
    per_task_reg: dict[str, float] = {}
    for task, baseline_score in baseline_tasks.items():
        sandbox_score = sandbox_tasks.get(task)
        if sandbox_score is None:
            failures.append(f"task '{task}' missing from sandbox report")
            continue
        delta = baseline_score - sandbox_score  # positive = regression
        per_task_reg[task] = delta
        if delta > cfg.per_task_max_regression:
            failures.append(
                f"task '{task}' regressed {delta*100:.2f}pp "
                f"(sandbox={sandbox_score:.4f} baseline={baseline_score:.4f}, "
                f"cap={cfg.per_task_max_regression*100:.2f}pp)"
            )

    return GateResult(
        gate_id="5a",
        passed=not failures,
        aggregate_actual=sandbox_agg,
        aggregate_target=target,
        per_task_regressions=per_task_reg,
        failures=failures,
    )


def evaluate_gate_5b(
    sandbox_report: BenchmarkReport,
    fresh_report: BenchmarkReport,
    cfg: RegressionConfig,
) -> GateResult:
    """Gate 5b: sandbox vs fresh-merge — adapter corruption check."""
    sandbox_agg = float(sandbox_report.get("aggregate_weighted_score", 0.0))
    fresh_agg = float(fresh_report.get("aggregate_weighted_score", 0.0))
    delta = abs(sandbox_agg - fresh_agg)

    failures: list[str] = []
    if delta > cfg.fresh_merge_max_delta:
        failures.append(
            f"sandbox-vs-fresh aggregate delta {delta:.4f} exceeds "
            f"{cfg.fresh_merge_max_delta:.4f} — possible adapter-merge corruption"
        )

    return GateResult(
        gate_id="5b",
        passed=not failures,
        aggregate_actual=delta,
        aggregate_target=cfg.fresh_merge_max_delta,
        failures=failures,
    )


# ── orchestrator ────────────────────────────────────────────────────────────


def run_dual_regression(
    *,
    cfg: RegressionConfig,
    sandbox_adapter_path: str,
    fresh_adapter_path: str,
    phase10_config_path: str,
    baseline_report_path: Path,
    run_benchmark: BenchmarkRunner,
    report_path: Path | None = None,
) -> DualGateReport:
    """Run both benchmark passes + evaluate both gates.

    Parameters
    ----------
    sandbox_adapter_path : path to the Phase 11 adapter (training output)
    fresh_adapter_path   : path to a freshly re-merged dense-14B copy
    phase10_config_path  : configs/benchmark_phase10.yaml
    baseline_report_path : training_data/eval/benchmark_qwen3_14b_v1_baseline.json
    run_benchmark        : injectable runner (mocked in tests)
    """
    logger.info("Loading Phase 5 baseline from %s", baseline_report_path)
    baseline_report = json.loads(baseline_report_path.read_text())

    logger.info("Gate 5a: benchmarking sandbox adapter %s", sandbox_adapter_path)
    sandbox_report = run_benchmark(phase10_config_path, sandbox_adapter_path)

    logger.info("Gate 5b: benchmarking fresh-merge adapter %s", fresh_adapter_path)
    fresh_report = run_benchmark(phase10_config_path, fresh_adapter_path)

    gate_5a = evaluate_gate_5a(sandbox_report, baseline_report, cfg)
    gate_5b = evaluate_gate_5b(sandbox_report, fresh_report, cfg)

    verdict = "pass" if gate_5a.passed and gate_5b.passed else "fail"
    dual = DualGateReport(
        overall_verdict=verdict,
        gate_5a=gate_5a,
        gate_5b=gate_5b,
        adapter_paths={"sandbox": sandbox_adapter_path, "fresh": fresh_adapter_path},
        baseline_paths={"phase5": str(baseline_report_path),
                        "phase10_config": phase10_config_path},
        sandbox_report=sandbox_report,
        fresh_report=fresh_report,
    )

    if report_path is not None:
        report_path.parent.mkdir(parents=True, exist_ok=True)
        report_path.write_text(json.dumps(dual.to_dict(), indent=2, sort_keys=True))
        logger.info("Regression report written to %s (verdict=%s)",
                     report_path, verdict)

    return dual


# ── CLI (compute-bound; real Phase 11 adapter runs this) ────────────────────


def main() -> int:  # pragma: no cover — compute-bound, exercised in Epic 6 e2e
    import argparse
    import yaml  # type: ignore[import-not-found]
    import subprocess
    import tempfile

    p = argparse.ArgumentParser(description="Phase 11 dual regression gate")
    p.add_argument("--config", required=True, help="configs/rl_phase11.yaml")
    p.add_argument("--sandbox-adapter", required=True)
    p.add_argument("--fresh-adapter", required=True)
    p.add_argument("--phase10-config", default="configs/benchmark_phase10.yaml")
    p.add_argument("--baseline", default="training_data/eval/benchmark_qwen3_14b_v1_baseline.json")
    p.add_argument("--out", default="training_data/eval/phase11_regression_report.json")
    args = p.parse_args()

    with open(args.config) as f:
        yaml_cfg = yaml.safe_load(f)
    reg = yaml_cfg["regression"]
    cfg = RegressionConfig(
        phase5_baseline_aggregate=reg["phase5_baseline_aggregate"],
        aggregate_target_multiplier=reg["aggregate_target_multiplier"],
        per_task_max_regression=reg["per_task_max_regression"],
        fresh_merge_max_delta=reg["fresh_merge_max_delta"],
    )

    def shell_runner(cfg_path: str, adapter: str) -> BenchmarkReport:
        """Real runner: invokes Phase 10 benchmark as subprocess."""
        with tempfile.NamedTemporaryFile(
            suffix=".json", delete=False, mode="w", encoding="utf-8"
        ) as tmp:
            out_path = tmp.name
        subprocess.run(
            ["python", "-m", "neural.benchmarks.run_benchmark",
             "--config", cfg_path, "--adapter", adapter, "--out", out_path],
            check=True,
        )
        return json.loads(Path(out_path).read_text())

    dual = run_dual_regression(
        cfg=cfg,
        sandbox_adapter_path=args.sandbox_adapter,
        fresh_adapter_path=args.fresh_adapter,
        phase10_config_path=args.phase10_config,
        baseline_report_path=Path(args.baseline),
        run_benchmark=shell_runner,
        report_path=Path(args.out),
    )

    print(f"Verdict: {dual.overall_verdict}")
    print(f"Gate 5a: {'PASS' if dual.gate_5a.passed else 'FAIL'}")
    for f in dual.gate_5a.failures:
        print(f"  - {f}")
    print(f"Gate 5b: {'PASS' if dual.gate_5b.passed else 'FAIL'}")
    for f in dual.gate_5b.failures:
        print(f"  - {f}")
    return 0 if dual.overall_verdict == "pass" else 1


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
