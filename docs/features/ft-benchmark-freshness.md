# FT Benchmark Freshness

**Sprint**: FT-BENCH-REFRESH-001 (2026-07-20 → 2026-07-21)
**Status**: shipped — alert rule + gauge + panel; benchmark refresh executed
**Related**: FT-RECURSIVE-001/002 (the autonomous retraining loop, default-off)

## Why this exists

`benchmark_runs` is populated exclusively when `python -m neural.benchmarks.run_benchmark --persist-tsdb` is invoked. Nothing schedules that today: `ftloop.controller_stages.stageGate` is the only automated caller, and FT-RECURSIVE-002 is default-off by design (opt-in gate for autonomous retraining).

The `mdemg-ft-training` dashboard's "Per-Task Pass Rate (latest run)" panel does `ORDER BY started_at DESC LIMIT 1` — so it will display the SAME row until a new benchmark runs. Operators reading the panel assume they're seeing current model quality; they're actually reading whatever historical row survives.

DASHBOARD-TRUTH-002 triage discovered this on `mdemg-dev`: the sole `benchmark_runs` row was from 2026-04-24 (88 days stale) against the pre-Phase-13.5 MLX model, but the dashboard presented `consulting.classify.mean_score = 0.7667` as current.

## What this ships

Three components — all default-on, all zero-effort for the operator to consume:

### 1. Alert rule (`ft_benchmark_stale`)

Registered unconditionally at server start. Reads `benchmark_runs` directly:

```sql
SELECT COALESCE(EXTRACT(EPOCH FROM (now() - MAX(completed_at))) / 86400.0, 999.0) AS age_days
FROM benchmark_runs
WHERE completed_at IS NOT NULL
```

- Threshold: `age_days > FT_BENCH_STALENESS_DAYS` (default 7).
- Severity: HIGH.
- Service label: `ft-benchmark` (distinct per NOSILENT-001 cooldown-key contract).
- Evaluation interval: 1h.
- Idle-safe: aggregate + COALESCE (999 when zero rows) means the rule always returns exactly one row.

### 2. Freshness stat panel (Grafana)

New "Latest Benchmark Age" panel on `mdemg-ft-training` dashboard, placed above the existing "Per-Task Pass Rate (latest run)" panel. Reads the same SQL as the alert rule. Colors mirror the alert:

- green: <3 days
- yellow: 3-7 days
- red: >7 days (matches the alert trigger)

Operators reading the "Latest Run" panel now see the freshness right next to the scores.

### 3. Executed refresh

FT-BENCH-REFRESH-001 E2 ran the benchmark against the current GGUF endpoint on `valid_clean.jsonl`, populating a fresh row in `benchmark_runs` (evidence in `docs/development/ft-bench-refresh-001/live_verification.md`).

## Config

| Env var | Default | Notes |
|---|---|---|
| `FT_BENCH_STALENESS_DAYS` | 7 | Days after `completed_at` before the alert fires. Fallback default on ≤0 with a WARN log. Set high (e.g. 30) to effectively disable while keeping the panel. |

## How to refresh manually

```bash
cd /Users/reh3376/mdemg
neural/.venv/bin/python -m neural.benchmarks.run_benchmark \
  --config configs/benchmark_phase10.yaml \
  --mlx-base-url http://127.0.0.1:8102/v1 \
  --mlx-model-name mdemg-llm-v1.Q5_K_M.gguf \
  --golden training_data/eval/valid_clean.jsonl \
  --rows-per-spec 5 \
  --n-runs 1 \
  --mlx-timeout-s 300 \
  --apply-tsdb \
  --out training_data/eval/benchmark_$(date +%s).json
```

`--apply-tsdb` (BENCH-SIDECAR-APPLY-001) writes the SQL sidecar (audit artifact) AND applies it directly — no manual `psql < sidecar.sql` step. Apply failure is non-fatal: the sidecar remains the recovery path. Requires psycopg (pinned in `neural/pyproject.toml` [training]; use `neural/.venv/bin/python`). The older `--persist-tsdb` (sidecar-only) still works.

Adjust `--rows-per-spec` / `--n-runs` per how thorough you want the refresh; larger values increase wall time proportionally. Once the run completes, the Latest Benchmark Age panel drops to <1d, the alert rule reads back below threshold, and the Per-Task Pass Rate panel displays the fresh data.

## Rollback

- Alert rule: revert the `serve.go` append line OR set `FT_BENCH_STALENESS_DAYS` to a very large value.
- Panel: revert the mdemg-ft-training.json edit (panel id 25).
- No data changes.

## Follow-ups (deferred)

- Automatic scheduling of the benchmark (FT-RECURSIVE-002 already ships the actuator; enabling `FT_LOOP_ENABLED=true` triggers the autonomous loop with additional guardrails).
- Prometheus gauge emission for the same age signal (currently TSDB-only per the metrics-access-model decision; see PROMETHEUS-SCRAPE-INVESTIGATION-001).
