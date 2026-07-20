# GRAFANA-PANEL-FILTER-001 — E4 Live Tier-3 Verification

**Date:** 2026-07-20
**Environment:** local `mdemg-dev` (mdemg-timescaledb-1 + mdemg-grafana-1)
**Purpose:** verify the E2 panel edit changes the operator-visible dashboard from ~10.84% error rate on `retrieval.rerank_cross` to 0.00% honest error rate.

## SQL cross-check (24h, mdemg-dev)

Direct query against `llm_interactions` mirroring both the pre-fix and post-fix panel SQL:

```
task_name              | calls | real_errors | raw_errors | honest_error_pct | raw_error_pct
-----------------------|-------|-------------|------------|------------------|--------------
retrieval.rerank_cross |   249 |           0 |         27 |             0.00 |         10.84
```

- **Raw error rate** (pre-fix panel behavior): **10.84%** — 27 rows with non-empty `error`, ALL 27 tagged `caller_canceled:` by the LLM-HEALTH-INVESTIGATION-001 E1 recorder.
- **Honest error rate** (post-fix panel behavior): **0.00%** — 0 real errors (5xx / timeouts / parse failures / provider errors) in the window.

The filter correctly removes 27/27 noise rows and preserves 0/0 real errors — perfect contract fidelity vs `internal/tsdb/dataset_builder.go::LLMPerformance` (the alert-rule source-of-truth).

## Dashboard file mounted with fix

```
$ docker exec mdemg-grafana-1 grep -c "caller_canceled" /etc/grafana/dashboards/mdemg-llm-routing.json
1

$ docker exec mdemg-grafana-1 grep -c "GRAFANA-PANEL-FILTER-001" /etc/grafana/dashboards/mdemg-llm-routing.json
2
```

Mounted SQL fragment (first line only):
```sql
-- GRAFANA-PANEL-FILTER-001: mirror dataset_builder.go::LLMPerformance — exclude caller-cancellation from error rate (fails-open, not a real error).
SELECT task_name, COUNT(*) AS calls, SUM(CASE WHEN error IS NOT NULL AND error != '' AND error NOT LIKE 'caller_canceled:%' THEN 1 ELSE 0 END) AS errors, ...
```

After `docker restart mdemg-grafana-1` (5-second warmup), the container serves the new panel SQL. `/api/health` returns database:ok.

## Pin test verification

`internal/grafanapin/dashboards_test.go` (E3) walks all shipped dashboards and asserts the filter contract. Ran during E4:

```
$ go test ./internal/grafanapin/ -v
=== RUN   TestGrafanaPanel_LLMInteractionsErrorFilter
    dashboards_test.go:143: scanned 164 panel targets across dashboards; 1 matched llm_interactions.error aggregate pattern
--- PASS: TestGrafanaPanel_LLMInteractionsErrorFilter (0.00s)
=== RUN   TestGrafanaPanel_LLMInteractionsErrorFilter_DetectsMissingFilter
--- PASS: TestGrafanaPanel_LLMInteractionsErrorFilter_DetectsMissingFilter (0.00s)
PASS
```

Sanity-checked by temporarily reverting E2 and re-running the pin test:
```
$ cp /tmp/pre-fix-routing.json deploy/docker/grafana/dashboards/mdemg-llm-routing.json
$ go test ./internal/grafanapin/ -run TestGrafanaPanel_LLMInteractionsErrorFilter$
--- FAIL: TestGrafanaPanel_LLMInteractionsErrorFilter (0.00s)
    GRAFANA-PANEL-FILTER-001 violations (1):
      - mdemg-llm-routing :: LLM error rate % by task_name (selected range) — missing filter: NOT LIKE 'caller_canceled:%'
```

Pin fails on pre-fix, passes on fix — contract enforcement live.

## Summary

- ✅ E2 panel edit lands the honest filter.
- ✅ Grafana container serves the new dashboard file after restart.
- ✅ Live SQL confirms honest_error_pct = 0.00% (was raw 10.84%).
- ✅ E3 pin test passes on current tree; sanity-verified to FAIL on unfiltered tree.
- ✅ 27 caller_canceled rows filtered; 0 real errors were hidden (nothing lost).

Ready for E5 canonical docs.
