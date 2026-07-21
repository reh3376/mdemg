# PROMETHEUS-SCRAPE-INVESTIGATION-001 — Sprint Post

**Shipped:** 2026-07-21 | **Branch:** `reh3376_dev01` | **PR:** (pending push)

## What shipped

Investigation-only sprint (no code changes). Final sprint in the DASHBOARD-TRUTH-002 sweep queue. Closes the "bonus finding" that `curl :9999/metrics → 404`.

## Verdict

**(c) DELIBERATE REMOVAL, ALREADY FULLY DOCUMENTED.** No follow-up sprint warranted, no code change needed. The DASHBOARD-TRUTH-002 triage agent's classification of `/metrics 404` as a suspicious finding was itself a false-positive.

## Evidence

**E1 current-state**: no `promhttp` handler, no `prometheus/client_golang` dependency, no `prometheus.NewRegistry` references. `internal/metrics/` is home-brew. Two live sinks: `/v1/metrics/snapshot` (JSON) + TSDB `metric_samples` (161 metrics / 56,778 samples in the last hour, live).

**E2 git-history**: two commits ever touched `prometheus/client_golang`:
- `6704256` (2026-03-26) added Prometheus RSIC gauges
- `981d40d` (2026-03-28) executed the "Prometheus-to-TSDB migration" per its own commit message — deleted `prometheus_metrics.uats.json` UATS spec + built the home-brew MetricsRecorder

The Prometheus scrape endpoint existed for **two days** in March 2026 before being deliberately migrated out.

**E3 config/deploy cross-check**: no config switch, no Prometheus datasource in Grafana (only neo4j/nodegraph-api/timescaledb), no Prometheus container in compose, migration explicitly documented at `docs/features/prometheus-observability-monitoring.md:18`.

## Epics (all committed)

- **E0** — Sprint plan (bundled `40d55fa`)
- **E1-E4** — Investigation + verdict (`7056dbd`)
- **E5** — Canonical docs (this commit)

## Architectural rules pinned

1. **Metrics access model is TSDB-only.** `/v1/metrics/snapshot` for live JSON; TSDB `metric_samples` for historical; TSDB-query alert rules for alerts; Grafana with the `timescaledb` datasource for dashboards.
2. **When a triage agent flags an infra endpoint 404 as suspicious, cross-check against `docs/features/` first** — the migration may already be documented (this sprint's triage-artifact-of-a-triage class).

## What NOT to do (pinned)

- Do NOT wire `promhttp.Handler()` at `/metrics`. Contradicts shipped architecture; doubles emission (TSDB writer + Prometheus scrape); complicates the mental model.
- Do NOT re-add `prometheus/client_golang` to go.mod. Home-brew registry has been production for 4 months.

## Minor doc-hygiene follow-ups (not fixed here)

Two files have stale `prometheus` language from March 2026 that survived the migration:
- `docs/features/ui-gap-analysis.md:74` — still references `/v1/prometheus` as a UI endpoint
- `docs/features/j17-feedback-loop-closure.md:216` — test description says "prometheus has J17 metrics" (gauges now land in TSDB)

Both are low-priority language-level cleanup for the DOC-CURRENCY-001 line, not code defects.

## Rollback

- N/A (no code/data changes).

## The full DASHBOARD-TRUTH-002 sweep is now COMPLETE

Final tally across the 5-sprint sweep:

| # | Sprint | Status | Kind | Artifacts closed |
|---|--------|--------|------|------------------|
| 1 | GRAFANA-PANEL-FILTER-001 | ✅ shipped | mechanical | 1 panel filter + pin test |
| 2 | DASHBOARD-TRUTH-002 | ✅ shipped | code + panels | 9 artifacts (3 RSIC formulas, 6 panels), 7 arch rules pinned |
| 3 | JIMINY-ACTIONABILITY-INVERSION-001 | ✅ shipped | investigation | Inversion root-caused (Lever C dominant); fix spec written for JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 |
| 4 | FT-BENCH-REFRESH-001 | ✅ shipped | data + code + panel | Fresh benchmark 0.8544 (+0.021 vs 88d baseline); alert rule + panel |
| 5 | PROMETHEUS-SCRAPE-INVESTIGATION-001 | ✅ shipped | investigation | Verdict (c) already-documented; no fix needed |

**14 operator-flagged concerns resolved.** Most are ARTIFACT fixes; 3 REAL-LOW items are covered by the active JIMINY-CORPUS arc; 1 forward-looking fix spec (JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001) is deferred as the operator's next optional decision point.
