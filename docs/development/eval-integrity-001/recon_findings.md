# EVAL-INTEGRITY-001 Recon (2026-06-13, HEAD) — change-point map

Source: the training-pipeline correctness audit + a focused recon lane;
all file:line-verified. Five fixes + three landmines.

## Five fixes (by work type)
1. **Eval set** (config + SHA re-pin): `benchmark_phase10.yaml:96`
   `golden_holdout.out_path=valid_golden.jsonl` (99% leaked) →
   rebuilt clean set; `mdemg.ubench.json:40` path + `sha256` +
   `expected_rows`/`expected_tasks` re-pinned same PR (ubench_runner
   recomputes sha + asserts counts). `--golden` CLI override exists
   (run_benchmark.py:549).
2. **Leak audit in-gate** (code/make): `scripts/audit_eval_leakage.py`
   (`--eval --against <csv> --out`; exit 1 on exact-match leak per task)
   has ZERO callers; add a preflight that aborts before model calls +
   `make test-eval-leak`. Compare against SFT train/valid splits.
3. **Baseline recompute** (code + operational): `regression.py:42`
   `phase5_baseline_aggregate=0.8338` is a frozen constant + a stale
   Apr-24 MLX-served report (`benchmark_qwen3_14b_v1_baseline.json`,
   agg 0.83380). NO recompute path. Baseline model =
   `.local-models/qwen3-14b-mdemg-v1` (Phase 5 dense, no adapter; dense→
   GGUF via `quantize_deploy.py`).
4. **GGUF serving** (code): `regression.py:344` spawns `mlx_lm server
   --adapter-path` on `rl_phase11.yaml mlx_port:8101` (decommissioned).
   Switch to llama-server :8102 GGUF (Phase 13.5 form). run_benchmark.py
   is endpoint-configurable (`--mlx-base-url`:545, `--mlx-model-name`:547,
   `--mlx-timeout-s`:563); `benchmark_phase10.yaml mlx_port:8102` already
   correct — only rl_phase11 + the regression spawner are wrong.
5. **Zero-call hard-fail** (code): `variance.py:96` returns
   overall_mean=0.0 on n==0; `run_benchmark.py:409-441` silently
   `continue`s errored/zero-row tasks and reports aggregate 0.0 (not
   error). No `successful_calls==0` guard anywhere. Add one.

## Three landmines
- **valid_clean is stale + wrong shape**: 319 rows / 16 tasks (Apr 30;
  180 prod + 139 synthetic) vs the gate's 108 / **17 tasks**. Swapping
  it breaks `expected_tasks:17` + `min_rows_per_task:3`. Must REBUILD
  via `build_clean_eval.py` with a deliberate 17-task target set, not
  drop-in. (Row counts live-verified: 319 vs 108.)
- **SHA re-pin mandatory same-PR** or the ubench runner fails on the
  recomputed holdout hash.
- **Baseline recompute interacts with the NEXT (reward) sprint**: an
  honest baseline must use the FIXED reward, which lands next. So this
  sprint stamps the trustworthy HARNESS + a `provisional:true` baseline;
  the reward sprint re-runs the baseline as its closing step.

## CI reality
`ci.yml:321` runs `make test-ubench-lint` only (schema + config-SHA;
holdout SHA skipped — gitignored). `test-ubench-run`/`-contract` +
regression gate are LOCAL/operator-run, never CI-gated. So the
trustworthy-gate work lands in the local/operational harness, not CI.
