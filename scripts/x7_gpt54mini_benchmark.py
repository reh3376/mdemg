"""X7 — Phase 10 benchmark against gpt-5.4-mini.

Reuses neural/benchmarks/run_benchmark.run() with a custom HTTP transport
that adds the OpenAI Authorization header. Drops --enable-judge since
we're benchmarking gpt-5.4-mini AS the model under test (judging itself
would be circular).
"""
from __future__ import annotations

import json
import os
import sys
from dataclasses import asdict
from pathlib import Path
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import yaml  # noqa: E402

from neural.benchmarks.run_benchmark import RunnerOptions, run  # noqa: E402


# OpenAI's chat-completions API does NOT accept these mlx_lm-specific params:
_OPENAI_INCOMPATIBLE = {"top_k", "min_p", "repetition_penalty", "draft_model", "num_draft_tokens"}

def openai_http_post(url: str, headers: dict, body: bytes, timeout: float) -> bytes:
    """HTTP transport that adds OpenAI Authorization + strips mlx-only kwargs."""
    api_key = os.environ.get("OPENAI_API_KEY")
    if not api_key:
        raise RuntimeError("OPENAI_API_KEY not set in environment")
    # Strip OpenAI-incompatible sampling params + rename max_tokens
    payload = json.loads(body.decode("utf-8"))
    stripped = {k: v for k, v in payload.items() if k not in _OPENAI_INCOMPATIBLE}
    if "max_tokens" in stripped:
        stripped["max_completion_tokens"] = stripped.pop("max_tokens")
    body = json.dumps(stripped).encode("utf-8")
    headers = {**headers, "Authorization": f"Bearer {api_key}"}
    req = Request(url, data=body, headers=headers, method="POST")
    try:
        with urlopen(req, timeout=timeout) as resp:
            return resp.read()
    except HTTPError as e:
        err_body = e.read().decode("utf-8", errors="replace") if hasattr(e, "read") else ""
        raise RuntimeError(f"OpenAI HTTP {e.code}: {e.reason} — {err_body[:500]}") from e
    except (URLError, TimeoutError) as e:
        raise RuntimeError(f"OpenAI network/timeout: {e}") from e


def main() -> int:
    config_path = Path("configs/benchmark_phase10.yaml")
    out_path = Path("training_data/eval/phase11_5_diagnostics/x7_gpt54mini_benchmark.json")

    config = yaml.safe_load(config_path.read_text())
    n_runs = int(config.get("n_runs", 5))
    golden = Path(config["golden_holdout"]["out_path"])
    specs = Path(config["ults"]["specs_dir"])

    opts = RunnerOptions(
        model_path="openai/gpt-5.4-mini",
        mlx_base_url="https://api.openai.com/v1",
        n_runs=n_runs,
        golden_path=golden,
        specs_dir=specs,
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

    print("X7: pointing benchmark at OpenAI gpt-5.4-mini")
    print(f"  url:    {opts.mlx_base_url}")
    print(f"  model:  {opts.mlx_model_name}")
    print(f"  specs:  {specs}  (16 ULTS tasks)")
    print(f"  n_runs: {n_runs}/task")
    print(f"  total prompt calls: ~{16 * n_runs}")
    print()

    report = run(opts)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(report, indent=2, default=str))
    print(f"\nX7 complete: aggregate = {report.get('aggregate_weighted_score', 0):.4f}")
    print(f"  specs_total: {report.get('specs_total', 0)}")
    print(f"  specs_with_errors: {report.get('specs_with_errors', 0)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
