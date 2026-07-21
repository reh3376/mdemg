# FT-BENCH-REFRESH-001 — E1 Preflight

**Date:** 2026-07-21

## Checks

| Check | Status | Detail |
|---|---|---|
| llama-server up on :8102 | ✅ | `models: ['mdemg-llm-v1.Q5_K_M.gguf']` |
| `training_data/eval/valid_clean.jsonl` present | ✅ | 1.7 MB, 2026-06-13 |
| `benchmark_runs` table writable | ✅ | 1 lifetime row |
| Config template present | ✅ | `configs/benchmark_phase10.yaml` |
| TimescaleDB up | ✅ | container `mdemg-timescaledb-1` healthy |

## Baseline (sole existing row)

```
run_id: q283a23bz59mrg6faxo32ydx2
model_path: /Users/reh3376/mdemg/.local-models/qwen3-14b-mdemg-v1 (MLX, pre-Phase-13.5)
started_at: 2026-04-24 13:18:55Z
completed_at: 2026-04-24 13:32:58Z (13 min wall)
aggregate_weighted_score: 0.8338
```

87 days stale. Points at the pre-cutover MLX adapter form; production is now the GGUF Q5_K_M via llama-server.

## E2 planned invocation

```
python3 -m neural.benchmarks.run_benchmark \
  --config configs/benchmark_phase10.yaml \
  --mlx-base-url http://127.0.0.1:8102/v1 \
  --mlx-model-name mdemg-llm-v1.Q5_K_M.gguf \
  --golden training_data/eval/valid_clean.jsonl \
  --rows-per-spec 0 \
  --mlx-timeout-s 300 \
  --persist-tsdb \
  --out training_data/eval/benchmark_ft_bench_refresh_001_<ts>.json
```

Expected wall: 30-60 min (9 tasks × 20 rows × 5 n_runs = ~900 calls). Started in background.
