"""Phase 11 dual regression gate — Epic 5.

Orchestrates two back-to-back Phase 10 benchmark runs and compares both
against the Phase 5 SFT baseline (recomputed 0.8655 — BASELINE-RECOMPUTE-001) + each other for adapter-merge
corruption detection.

Gate 5a (vs Phase 5 SFT baseline, recomputed 0.8655):
    aggregate_sandbox ≥ baseline_report_aggregate × aggregate_target_multiplier  (default 1.02)
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

    # BASELINE-RECOMPUTE-001 (2026-06-15): recomputed through the FIXED harness
    # (valid_clean leak-free eval + RC-001/002 corrected rewards + GGUF llama-server
    # :8102) = 0.8655, replacing the stale frozen 0.8338 (valid_golden-leaked eval,
    # old length-biased rewards, decommissioned MLX serving — NOT comparable). This
    # is now only the drift TRIPWIRE; the gate derives the live target from the
    # baseline REPORT's aggregate_weighted_score (evaluate_gate_5a). Recompute was
    # --rows-per-spec 10 (50 samples/task); a full-corpus recompute can refine it.
    phase5_baseline_aggregate: float = 0.8655
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

    Phase 10's actual report shape (verified against
    ``training_data/eval/benchmark_qwen3_14b_v1_baseline.json``):

        per_task_aggregate: [
            {"task_id": "<task>", "overall_mean": <float>, "n": <int>, ...},
            ...
        ]

    The first list item may be a global summary lacking ``task_id`` — those
    are skipped. ``overall_mean`` is the per-task mean across the N runs;
    ``weighted_score`` shows up only at the top level (``aggregate_weighted_score``).

    Falls back to legacy dict-of-dicts and ``per_task_scores`` shapes if
    encountered, so older fixtures stay usable in tests.
    """
    agg = report.get("per_task_aggregate")
    if isinstance(agg, list):
        out: dict[str, float] = {}
        for item in agg:
            if not isinstance(item, dict):
                continue
            tid = item.get("task_id")
            if not tid:
                continue
            score = item.get("overall_mean")
            if score is None:
                score = item.get("weighted_score", item.get("score", 0.0))
            out[tid] = float(score)
        return out
    if isinstance(agg, dict) and agg and all(isinstance(v, dict) for v in agg.values()):
        return {
            task: float(body.get("weighted_score", body.get("overall_mean", body.get("score", 0.0))))
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
    # BASELINE-RECOMPUTE-001: derive the baseline aggregate from the loaded
    # baseline REPORT (the single source of truth — recomputed through the fixed
    # harness), not the frozen `phase5_baseline_aggregate` constant. The constant
    # is retained as a drift tripwire: a large divergence means the report on
    # disk no longer matches the recorded baseline and the report should be
    # re-verified before trusting the gate.
    baseline_agg = float(baseline_report.get("aggregate_weighted_score", cfg.phase5_baseline_aggregate))
    target = baseline_agg * cfg.aggregate_target_multiplier

    failures: list[str] = []
    if abs(baseline_agg - cfg.phase5_baseline_aggregate) > 0.05:
        logger.warning(
            "baseline report aggregate %.4f diverges >5pp from recorded "
            "phase5_baseline_aggregate %.4f — re-verify the baseline report",
            baseline_agg, cfg.phase5_baseline_aggregate,
        )
    if sandbox_agg < target:
        failures.append(
            f"aggregate {sandbox_agg:.4f} below target "
            f"{target:.4f} (={baseline_agg:.4f} × "
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

    # Resolve base model + server host/port from the Phase 10 config — the
    # benchmark hits a running mlx_lm.server (it does not load the model
    # itself). For each gate run, we spin up a server with --adapter-path
    # pointing at the gate's adapter, wait for /v1/models to respond, run
    # the benchmark, then terminate the server cleanly.
    import time  # noqa: PLC0415
    import urllib.request  # noqa: PLC0415
    import urllib.error  # noqa: PLC0415
    with open(args.phase10_config) as f:
        phase10_cfg = yaml.safe_load(f)
    mut = phase10_cfg["model_under_test"]
    base_model_path = mut["path"]
    server_host = mut.get("mlx_host", "127.0.0.1")
    server_port = int(mut.get("mlx_port", 8101))
    server_ready_url = f"http://{server_host}:{server_port}/v1/models"

    def _wait_for_server(timeout_s: float = 180.0) -> bool:
        deadline = time.monotonic() + timeout_s
        while time.monotonic() < deadline:
            try:
                with urllib.request.urlopen(server_ready_url, timeout=2.0):
                    return True
            except (urllib.error.URLError, ConnectionError, OSError):
                time.sleep(2.0)
        return False

    def shell_runner(cfg_path: str, adapter: str) -> BenchmarkReport:
        """Real runner — spins up mlx_lm.server with the given adapter,
        runs the Phase 10 benchmark against it, tears the server down.

        Note: ``run_benchmark.py`` is client-mode; it doesn't accept an
        ``--adapter`` flag. The adapter has to be loaded server-side via
        ``mlx_lm.server --adapter-path``. We orchestrate that lifecycle
        here so the regression harness has a single self-contained call.
        """
        with tempfile.NamedTemporaryFile(
            suffix=".json", delete=False, mode="w", encoding="utf-8"
        ) as tmp:
            out_path = tmp.name

        logger.info(
            "Starting mlx_lm.server (model=%s adapter=%s port=%d)",
            base_model_path, adapter, server_port,
        )
        import sys as _sys  # noqa: PLC0415
        server = subprocess.Popen(
            [_sys.executable, "-m", "mlx_lm", "server",
             "--model", base_model_path,
             "--adapter-path", adapter,
             "--host", server_host,
             "--port", str(server_port)],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
        try:
            if not _wait_for_server():
                raise RuntimeError(
                    f"mlx_lm.server failed to come up at {server_ready_url} "
                    "within 180s"
                )
            logger.info("mlx_lm.server ready; running Phase 10 benchmark")
            import os as _os  # noqa: PLC0415
            mlx_name = _os.path.abspath(base_model_path)
            subprocess.run(
                [_sys.executable, "-m", "neural.benchmarks.run_benchmark",
                 "--config", cfg_path,
                 "--mlx-model-name", mlx_name,
                 "--out", out_path],
                check=True,
            )
        finally:
            logger.info("Terminating mlx_lm.server (pid=%d)", server.pid)
            server.terminate()
            try:
                server.wait(timeout=20.0)
            except subprocess.TimeoutExpired:
                server.kill()
                server.wait(timeout=5.0)
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
