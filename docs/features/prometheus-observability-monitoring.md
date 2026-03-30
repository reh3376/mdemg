# Prometheus Observability Monitoring

> **Note (2026-03-29):** The `/v1/prometheus` endpoint has been replaced by `/v1/metrics/snapshot` (JSON format). Prometheus has been replaced by TimescaleDB as the metrics backend. Alert rules are now managed via Grafana alerting with SQL queries against TimescaleDB.

**Phase**: TSDB Sprint — Observability Infrastructure
**Status**: Complete
**Date**: 2026-03-27

---

## 1. Overview

Three observability gaps were identified and closed during the TSDB sprint:

1. **Cache hit metrics bypass** — Guidance served from cache skipped J17 protocol metric recording, keeping `TotalEvents` at 0 and preventing RSIC from detecting codification opportunities
2. **Bootstrap RSIC assessment** — RSIC health gauges started at zero because no assessment ran until the first periodic cycle (minutes after startup)
3. **Prometheus self-monitoring** — Prometheus was monitoring MDEMG but not itself; no alerting existed for scrape failures

---

## 2. Cache Hit Metrics Recording

### The Problem

`service.go:318-321` returned cached guidance responses without calling `RecordGuidance()` or `RecordConstraintCoverage()`. Since ~90% of guidance requests are cache hits, `TotalEvents` stayed at 0 despite active usage. This caused:

- `scoreProtocol()` comprehension = 0 (no events to measure)
- RSIC `j17_cold_start_codification` insight never triggered (requires `TotalEvents > 0`)
- Protocol health stuck at 0.325

### The Fix

New method `recordCacheHitMetrics(resp GuidanceResponse)` on `*Service` replicates the exact recording logic from the cache-miss path (lines 745-773):

```go
func (s *Service) recordCacheHitMetrics(resp GuidanceResponse) {
    if s.protocolMetrics == nil || !s.cfg.J17Enabled || s.encoder == nil {
        return
    }
    tier := s.encoder.CurrentTier()
    for _, item := range resp.Guidance {
        s.protocolMetrics.RecordGuidance(tier, len(item.Content))
    }
    // ... constraint coverage recording
}
```

Called at line 320 before the cache return. Protocol metrics now reflect actual usage regardless of cache status.

### Tests

10 unit tests in `internal/jiminy/service_cache_metrics_test.go`:

| Test | Verifies |
|------|----------|
| `NilProtocolMetrics` | No panic when metrics collector is nil |
| `J17Disabled` | No recording when J17 is disabled |
| `NilEncoder` | No recording when encoder is nil |
| `EmptyGuidance` | No recording for empty guidance lists |
| `RecordsAllTiers` | Metrics recorded for T1, T2, T3 tiers |
| `ConstraintCoverage` | Coded/uncoded constraint tracking |
| `UncodedConstraintTrackedByNodeID` | Uncoded constraints use node_id fallback |
| `ConstraintCoverage_AllCoded` | 100% code coverage scenario |
| `ConstraintCoverage_NoneCoded` | 0% code coverage scenario |
| `OnlyNonConstraints` | Non-constraint items skip coverage tracking |

---

## 3. Bootstrap RSIC Assessment

### The Problem

`CollectHealthMetrics()` returns early when `cachedReport` is nil. After server startup, no RSIC assessment has run yet, so all health gauges (`mdemg_rsic_health_overall`, `mdemg_rsic_health_retrieval`, etc.) remain at 0.0 until the first periodic cycle fires.

### The Fix

A goroutine in `server.go` runs a one-time bootstrap assessment 10 seconds after startup:

```go
go func() {
    time.Sleep(10 * time.Second)
    if rsicAssessor != nil {
        report, err := rsicAssessor.Assess(context.Background(),
            cfg.RSICWatchdogSpaceID, ape.TierMeso)
        // ... log results
    }
}()
```

This populates the cached report so that health gauges reflect actual system state from the first Prometheus scrape.

**Placement**: Inside the `if cfg.LiveMetricsEnabled` block in `server.go`, after the RSIC watchdog goroutine.

---

## 4. Prometheus Self-Monitoring

### Blackbox Probe

Added `http://localhost:9090/-/healthy` to the `service-health` blackbox probe targets in `deploy/docker/prometheus.yml`. Prometheus now monitors its own health alongside MDEMG's `/healthz`.

### Alert Rules

New file: `deploy/docker/prometheus/alerts/observability.yaml`

| Alert | Severity | For | Condition |
|-------|----------|-----|-----------|
| `MDEMGScrapeTargetDown` | critical | 2m | Any Prometheus target is down |
| `MDEMGPrometheusUnhealthy` | critical | 1m | Prometheus self-health probe fails |
| `MDEMGPrometheusScrapeSlowdown` | warning | 5m | Scrape duration exceeds 5 seconds |
| `MDEMGPrometheusStorageHigh` | warning | 10m | TSDB storage exceeds 1GB |

---

## 5. Configuration

No new environment variables. All changes use existing infrastructure:

| Setting | Location | Purpose |
|---------|----------|---------|
| `LIVE_METRICS_ENABLED` | `config.go` | Gates bootstrap assessment and live metric collection |
| `RSIC_WATCHDOG_SPACE_ID` | `config.go` | Space ID for bootstrap RSIC assessment |
| Blackbox probe targets | `prometheus.yml` | List of HTTP endpoints to monitor |
| Alert rules path | `prometheus.yml` | `rule_files` includes `alerts/*.yaml` |

---

## 6. Key Files

| File | Action | Description |
|------|--------|-------------|
| `internal/jiminy/service.go` | Modified | `recordCacheHitMetrics()` method, called on cache hits |
| `internal/api/server.go` | Modified | Bootstrap assessment goroutine, `CutSuffix` lint fix |
| `deploy/docker/prometheus.yml` | Modified | Self-monitoring blackbox probe target |
| `deploy/docker/prometheus/alerts/observability.yaml` | New | 4 alert rules |
| `internal/jiminy/service_cache_metrics_test.go` | New | 10 unit tests |
| `tests/integration/j17_metrics_test.go` | New | `TestJ17_GuidanceThenMetrics`, `TestJ17_PrometheusScrapeable` |
| `tests/integration/rsic_health_test.go` | New | `TestRSIC_HealthGaugesExist`, `TestRSIC_ReadyzContainsRSIC` |

## 7. Verification

```bash
# Check RSIC health gauges are non-zero after restart
curl -s http://localhost:9999/v1/metrics/snapshot | jq '.data.gauges | with_entries(select(.key | test("rsic_health_overall")))'

# Check J17 events recording
curl -s http://localhost:9999/v1/jiminy/protocol/metrics | jq .data.total_events

# Check Prometheus self-health probe
curl -s http://localhost:9090/-/healthy

# Check alert rules loaded
curl -s http://localhost:9090/api/v1/rules | jq '.data.groups[].rules | length'
```

## Dependencies

- **RSIC Core (Phase 60b)** — `Assessor.Assess()` for bootstrap health assessment
- **Live Metrics Collector (`internal/ape/`)** — Publishes gauge values from RSIC reports
- **Prometheus** — `deploy/docker/docker-compose.observability.yml` for alertmanager + blackbox_exporter

## Documents Accessed

- `internal/jiminy/service.go` (modified)
- `internal/api/server.go` (modified)
- `deploy/docker/prometheus.yml` (modified)
- `deploy/docker/prometheus/alerts/observability.yaml` (new)
- `internal/jiminy/service_cache_metrics_test.go` (new)
- `tests/integration/j17_metrics_test.go` (new)
- `tests/integration/rsic_health_test.go` (new)
- `internal/ape/self_assess.go` (read — `scoreProtocol()` formula)
- `internal/ape/live_collectors.go` (read — gauge registration)
