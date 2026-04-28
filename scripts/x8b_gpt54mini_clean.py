"""Phase 11.5c Epic 3 — Re-baseline gpt-5.4-mini against valid_clean.jsonl."""
from __future__ import annotations
import json, os, sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import yaml
from neural.benchmarks.run_benchmark import RunnerOptions, run
from x7_gpt54mini_benchmark import openai_http_post


def main() -> int:
    config = yaml.safe_load(Path("configs/benchmark_phase10.yaml").read_text())
    n_runs = int(config.get("n_runs", 5))
    out_path = Path(os.environ.get("X8B_OUT", "training_data/eval/baseline_gpt54mini_clean.json"))
    golden_path = Path(os.environ.get("X8B_GOLDEN", "training_data/eval/valid_clean.jsonl"))

    opts = RunnerOptions(
        model_path="openai/gpt-5.4-mini",
        mlx_base_url="https://api.openai.com/v1",
        n_runs=n_runs,
        golden_path=golden_path,
        specs_dir=Path(config["ults"]["specs_dir"]),
        task_filter=None,
        enable_judge=False,
        judge_metrics=[],
        config=config,
        mlx_model_name="gpt-5.4-mini",
        persist_tsdb=False,
        mlx_timeout_s=60.0,
        mlx_http_post=openai_http_post,
        judge_http_post=None,
    )

    print("X8B: gpt-5.4-mini @ OpenAI vs valid_clean.jsonl")
    print(f"  golden: {golden_path}")
    print(f"  out:    {out_path}")
    print(f"  n_runs: {n_runs}")
    print()

    report = run(opts)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(report, indent=2, default=str))
    print(f"\nX8B complete: aggregate = {report.get('aggregate_weighted_score', 0):.4f}")
    print(f"  specs_total: {report.get('specs_total', 0)}")
    print(f"  specs_matched: {report.get('specs_with_matched_rows', 0)}")
    print(f"  specs_with_errors: {report.get('specs_with_errors', 0)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
