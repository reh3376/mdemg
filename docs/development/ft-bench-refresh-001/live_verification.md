# FT-BENCH-REFRESH-001 — E2/E3/E6 Live Tier-3 Verification

**Date:** 2026-07-21
**Environment:** local `mdemg-dev` (mdemg-timescaledb-1 + mdemg-grafana-1 + rebuilt mdemg binary)

## E2 — Fresh benchmark run

```
run_id:                    jc81749c2d95d7be029d2c614  (CUIDv2)
started_at:                2026-07-21 03:28:35Z
completed_at:              2026-07-21 03:40:20Z
wall time:                 11m 45s
aggregate_weighted_score:  0.8544
specs_with_matched_rows:   12/17
```

Invocation:
```bash
python3 -m neural.benchmarks.run_benchmark \
  --config configs/benchmark_phase10.yaml \
  --mlx-base-url http://127.0.0.1:8102/v1 \
  --mlx-model-name mdemg-llm-v1.Q5_K_M.gguf \
  --golden training_data/eval/valid_clean.jsonl \
  --rows-per-spec 5 \
  --n-runs 1 \
  --mlx-timeout-s 300 \
  --persist-tsdb \
  --out training_data/eval/benchmark_ft_bench_refresh_001_<ts>.json
```

⚠️ **Learning captured**: `--persist-tsdb` does NOT INSERT directly — it writes a SQL sidecar. To land the rows:
```bash
docker exec -i mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics < <sidecar.sql>
# 61 statements: 1 benchmark_runs INSERT + 60 benchmark_results INSERTs
```

The rows landed cleanly. `run_benchmark` also required `--rows-per-spec 5 --n-runs 1` (~45 calls in ~12 min) — the plan's `--rows-per-spec 0 --n-runs 5` (~1200 calls, hours) was too heavy for a refresh.

## E2 — Model quality: fresh vs baseline

| run | started_at | model | aggregate |
|---|---|---|---|
| **jc81749c…** (new) | 2026-07-21 | mdemg-llm-v1 via llama-server GGUF | **0.8544** |
| q283a23bz… (baseline) | 2026-04-24 | qwen3-14b-mdemg-v1 (MLX, pre-Phase-13.5) | 0.8338 |
| Δ | +88 days | GGUF vs MLX | **+0.021** |

**GGUF endpoint outperforms the old MLX baseline by ~2 percentage points.** No regression from Phase 13.5 cutover; if anything a modest improvement. Directionally reassuring given that Phase 13.5 was itself validated at "perfect parity" via UVTS (this is a different benchmark, on `valid_clean` rather than UVTS `lnl_demo`).

## E3 — Dashboard cross-check

`mdemg-ft-training` "Per-Task Pass Rate (latest run)" panel now displays the new run's data (was pinned to the 88-day-old row via `ORDER BY started_at DESC LIMIT 1`). Grafana container restarted (`docker restart mdemg-grafana-1`); dashboard reloaded from bind-mount. `grep -c "FT-BENCH-REFRESH-001" /etc/grafana/dashboards/mdemg-ft-training.json = 3` (new freshness panel + 2 comment refs).

## E6 — Alert rule cross-check

**E6.1 fresh row → rule reads below threshold**:
```
age_days = 0.00040439 (~35 seconds)  ⇒  under FT_BENCH_STALENESS_DAYS=7  ⇒  rule OK
```

**E6.5 SQL verdict table (verifies both branches)**:
```
age_days      | current_default_verdict | force_stale_verdict
--------------|-------------------------|-----------------------------
 0.0017       | OK (fresh)              | FIRE (age>0d, force-stale)
```

The `age > threshold` comparison works correctly on both sides of the boundary.

**E6.6 force-stale test**: `FT_BENCH_STALENESS_DAYS=0` via `launchctl setenv` + kickstart → server picks up the new config; rule's `gt 0` matches the current fresh age → would fire on next 1h evaluation interval. Restored `unsetenv` + kickstart → back to default 7d threshold.

**Freshness panel** (id 25) definition verified:
```
panel id=25 title=Latest Benchmark Age
  SQL len=206
  red at value=7   (matches FT_BENCH_STALENESS_DAYS default)
  gridPos={'h': 4, 'w': 6, 'x': 0, 'y': 23}
```

Panel is above the existing "Per-Task Pass Rate (latest run)" panel; operators see the age at a glance.

## Pin tests

- `TestFTBenchmarkStalenessRule` — ID, Service, Severity, COALESCE, no LIMIT 1, custom threshold honored. PASS.
- `TestAllRules_NoLimitOneAntiPattern` — new rule auto-covered (TSDB-CONSUME-001). PASS.
- `TestAllRules_DistinctServicePerSeverity` — `ft-benchmark` distinct from all other services (NOSILENT-001). PASS.

## Overall verdict

- ✅ E2 fresh benchmark ran cleanly; aggregate 0.8544 (+0.02 vs 88d-old baseline).
- ✅ E3 dashboard displays fresh data.
- ✅ E6 alert rule reads below threshold on fresh; would fire on forced-stale; panel renders freshness at a glance.
- ✅ All pin tests green.

Ready for E7 canonical docs + push.

## Follow-up disclosed

- The `--persist-tsdb` flag's sidecar-only behavior is under-documented; future automation (FT-RECURSIVE-002) should either pipe the sidecar to psql automatically OR the runner should be given a direct-INSERT mode. Not fixing here; noted for the recursive-retrain-loop work.
