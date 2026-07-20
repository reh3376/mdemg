# GRAFANA-PANEL-FILTER-001 — E1 Dashboard Audit

**Date:** 2026-07-20
**Method:** grep across `deploy/docker/grafana/dashboards/*.json` for `error IS NOT NULL`, `length(error)`, `error != ''`, `error <> ''`, and full-text `llm_interactions` references.

## Must-fix (1 panel)

| File:Line | Panel | Current SQL fragment | Filter needed |
|---|---|---|---|
| `mdemg-llm-routing.json:80` | LLM error rate % by task_name (panel description at :75) | `SUM(CASE WHEN error IS NOT NULL AND length(error) > 0 THEN 1 ELSE 0 END)` twice + no `caller_canceled` clause | Add `AND error NOT LIKE 'caller_canceled:%'` inside BOTH `CASE` predicates |

**Live evidence (24h `mdemg-dev`):** `retrieval.rerank_cross` shows 23 raw_errors of 213 calls = 10.80%. All 23 are `caller_canceled:` rows. After filter: 0/213 = 0.00%.

## Ambiguous (0 panels)

None. Every other `llm_interactions` reference is unambiguously non-error-count.

## Safe (7 panels — no filter needed)

| File:Line | Reason skipped |
|---|---|
| `mdemg-llm-routing.json:25` | Stacked model_name row counts; no error filter |
| `mdemg-llm-routing.json:49` | Latency percentiles by task+model; filters `latency_ms IS NOT NULL` only |
| `mdemg-llm-routing.json:153,160` | Grafana variable — `SELECT DISTINCT space_id`; no error math |
| `mdemg-ft-training.json:69` | LLM call rate (all calls); no error filter |
| `mdemg-ft-training.json:330` | Avg quality where quality IS NOT NULL; no error filter |
| `mdemg-rsic.json:1623` | Latency percentiles; no error filter |
| `mdemg-rsic.json:1706` | Call counts by task; no error filter |

## Pre-existing filter check

Zero `caller_canceled` references anywhere under `deploy/docker/grafana/`. Nothing to conflict with; the sprint is net-new.

## Conclusion

Single-panel edit. E2 applies the fix to `mdemg-llm-routing.json:80`; E3's pin test walks all dashboards + asserts the filter contract for every `llm_interactions.error` panel going forward.
