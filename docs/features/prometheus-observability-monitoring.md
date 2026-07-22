---
created: 2026-03-27
updated: 2026-07-21
version: v0.10.x
author: reh3376
status: current (rewritten by DOC-CURRENCY-002 — now documents the TSDB-native metrics access model; Prometheus-era content moved to the Historical appendix)
phase: "TSDB-Sprint → PROMETHEUS-SCRAPE-INVESTIGATION-001"
---

# Observability & Metrics Access Model

## Summary

MDEMG's metrics plane is **TSDB-native — there is no Prometheus server and no
`/metrics` scrape endpoint, by design**. The internal metrics registry
(`internal/metrics/`) keeps Prometheus-*style* gauge/counter/histogram
semantics, but values surface through three doors:

| Door | What | Use for |
|------|------|---------|
| `GET /v1/metrics/snapshot` | Live JSON snapshot of every registered gauge/counter/histogram | Ad-hoc inspection, hooks, scripts |
| TSDB `metric_samples` hypertable | Periodic flush of registry values (column is `time`, not `recorded_at`) | Historical queries, Grafana panels |
| Server-native alert evaluator (`internal/alert/`) | 24+ TSDB-query rules evaluated in-process | Alerting — Grafana is NOT required |

Grafana (container `mdemg-grafana-1`) reads the `timescaledb` datasource
directly; dashboards are provisioned from
`deploy/docker/grafana/dashboards/*.json` and pin-tested in
`internal/grafanapin/`.

This model was confirmed deliberate by **PROMETHEUS-SCRAPE-INVESTIGATION-001**
(2026-07, `docs/development/prometheus-scrape-investigation-001/`): the
Grafana-observed `/metrics` 404 is expected — no `promhttp` handler is wired
and none should be. Verdict: *"No code should be written; the metrics access
model is clean."*

## Why not Prometheus?

The original TSDB sprint (2026-03) replaced the Prometheus backend because
MDEMG already runs TimescaleDB as a first-class dependency: one storage
engine for metrics + telemetry + training data beats running a second TSDB
(Prometheus) alongside it. Alert rules moved from PromQL YAML to SQL — first
in Grafana, then (SR-001/SNA-001) into the in-process alert evaluator so
**alerting works even with Grafana down**.

What remains Prometheus-*shaped*:

- Metric names/types in `internal/metrics/registry.go` follow Prometheus
  conventions (`mdemg_*` prefix, `_total` counters, histogram buckets).
- The `deploy/docker/blackbox/` config and
  `docker-compose.observability.yml` are **legacy artifacts** of the
  pre-migration stack (the referenced `deploy/docker/prometheus.yml` no
  longer exists).

## Operator verification

```bash
# Live snapshot — RSIC health gauges
curl -s http://localhost:9999/v1/metrics/snapshot | \
  jq '.data.gauges | with_entries(select(.key | test("rsic_health_overall")))'

# Historical — same gauge from TSDB (column is `time`)
docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -c \
  "SELECT time, value FROM metric_samples
   WHERE metric_name = 'mdemg_rsic_health_overall'
   ORDER BY time DESC LIMIT 5"

# Alert evaluator output (file backend)
cat ~/.mdemg/alerts/current.json | jq '.alerts[] | {service, severity, title}'

# J17 protocol events
curl -s http://localhost:9999/v1/jiminy/protocol/metrics | jq .data.total_events
```

Contract reminders when touching this plane (see CLAUDE.md for full context):

- Alert-rule SQL must be idle-safe: aggregate + `COALESCE`, never
  `ORDER BY … LIMIT 1` (TSDB-CONSUME-001).
- Each rule needs a distinct `Service` label — the dispatcher cooldown key is
  `(Service, Severity)` (NOSILENT-001).
- `metric_samples.time` vs `retrieval_audit.recorded_at` — crossing the
  column names silently kills a rule.
- When validating gauge changes right after a restart, exclude
  `metric_samples` rows older than the new pid's start time — the dying
  process's final recorder flush lands after the restart
  (DASHBOARD-TRUTH-001 stale-binary lesson).

---

## Historical appendix — TSDB-sprint fixes (2026-03-27, still in effect)

Two of the three fixes shipped by the original sprint remain live code; the
third (Prometheus self-monitoring) was removed with the Prometheus stack.

### 1. Cache-hit metrics recording (live)

`internal/jiminy/service.go::recordCacheHitMetrics` — guidance served from
cache records J17 protocol metrics identically to the cache-miss path.
Before this, ~90% of guidance requests (cache hits) skipped recording,
pinning `TotalEvents` at 0 and blocking the RSIC
`j17_cold_start_codification` insight. 10 unit tests in
`internal/jiminy/service_cache_metrics_test.go`.

### 2. Bootstrap RSIC assessment (live)

`internal/api/server.go` runs a one-time RSIC assessment ~10s after startup
(inside the `LiveMetricsEnabled` block) so health gauges reflect real state
from the first snapshot instead of reading 0.0 until the first periodic
cycle.

### 3. Prometheus self-monitoring (removed)

The blackbox probe of `:9090/-/healthy` and
`deploy/docker/prometheus/alerts/observability.yaml` (4 PromQL rules) were
deleted with the Prometheus stack. Their intent — "the monitor must itself
be monitored" — is covered today by the in-process health prober
(`HEALTH_PROBE_ENABLED`), the supervised alert-evaluator loop with the
`alert-evaluator-degraded` meta-alert (SUPERVISOR-002), and
`scheduled_job_events` staleness rules (NOSILENT-001).

## Documents Accessed

- `docs/development/prometheus-scrape-investigation-001/{investigation,post}.md`
- `internal/metrics/registry.go`, `internal/alert/rules.go`
- `internal/jiminy/service.go`, `internal/api/server.go`
- `deploy/docker/` tree (confirmed `prometheus.yml` absent)
- CLAUDE.md (TSDB-CONSUME-001 / NOSILENT-001 / DASHBOARD-TRUTH-001 contracts)
