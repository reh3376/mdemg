# Phase 91: RSIC Observability & Operations

**Status**: Complete
**Priority**: Medium
**Date**: 2026-02-19
**Depends On**: Phase 90 (`docs/specs/phase90-rsic-conformance-ci-gating.md`)
**Related Handoff Section**: `AGENT_HANDOFF.md` → `RSIC Hardening Phases`
**Gap References**: `docs/development/RSIC_GAP_ANALYSIS.md` — Gap #6 (Observability & Alerting)

---

## Purpose

Phases 87-90 built the full RSIC runtime (orchestration, safety, persistence, multi-space, CI gating). All 109 UATS pass and 6 integration tests validate the vertical stack. However, there were **zero RSIC-specific Prometheus metrics** — operators had no visibility into cycle health, action outcomes, trigger rejections, watchdog state, or calibration confidence. This gap made it impossible to detect degradation, diagnose failures, or set SLOs for the RSIC subsystem.

Phase 91 closes this gap with:
1. **12 Prometheus metrics** across 6 RSIC files
2. **16-panel Grafana dashboard** for real-time RSIC monitoring
3. **8 alert rules** for automated degradation detection
4. **Operations runbook** with failure mode playbooks, safe mode, and SLOs

---

## Scope

### Metrics (12 metric families)

| Category | Metric | Type | Labels |
|----------|--------|------|--------|
| Cycle | `rsic_cycle_total` | Counter | tier, source, outcome |
| Cycle | `rsic_cycle_duration_seconds` | Histogram | tier |
| Trigger | `rsic_trigger_rejected_total` | Counter | source, reason |
| Action | `rsic_action_total` | Counter | action, status |
| Action | `rsic_action_duration_seconds` | Histogram | action |
| Safety | `rsic_safety_blocked_total` | Counter | action, reason |
| Watchdog | `rsic_watchdog_decay_score` | Gauge | space_id |
| Watchdog | `rsic_watchdog_escalation_level` | Gauge | space_id |
| Watchdog | `rsic_watchdog_force_total` | Counter | (none) |
| Calibration | `rsic_calibration_confidence` | Gauge | action |
| Snapshot | `rsic_snapshot_created_total` | Counter | action |

All metrics are prefixed with `mdemg_` by the registry namespace.

### Instrumentation Approach

Uses the existing `metrics.Metrics()` global singleton pattern — the same approach used by HTTP handlers and CMS metrics. Each RSIC file adds `import "mdemg/internal/metrics"` and calls the singleton at instrumentation points. Metric emissions are fire-and-forget — failures never block RSIC execution.

### Dashboard (16 panels, 4 rows)

- **Overview**: Cycle rate, success rate, rejection rate, watchdog escalation level
- **Cycles**: Duration p95 by tier, cycles by source, trigger rejections by reason, outcome breakdown (pie)
- **Actions**: Success/failure table, duration p95, safety blocks, calibration confidence gauges
- **Watchdog**: Decay score, escalation timeline, force triggers rate, snapshots created

### Alert Rules (8 rules)

| Alert | Severity | Condition | For |
|-------|----------|-----------|-----|
| `MDEMGRSICHighFailureRate` | warning | >25% cycles not completing | 10m |
| `MDEMGRSICRepeatedForceTriggers` | critical | >0.5 force triggers/hr | 30m |
| `MDEMGRSICHighRejectionRate` | warning | >50% triggers rejected | 15m |
| `MDEMGRSICActionFailureSpike` | warning | >50% per-action failure | 10m |
| `MDEMGRSICSafetyRejectionSpike` | warning | >0.1/min safety blocks | 10m |
| `MDEMGRSICLowConfidence` | warning | confidence <0.3 | 30m |
| `MDEMGRSICHighDecayScore` | warning | decay >8.0 | 10m |
| `MDEMGRSICCycleDurationSpike` | warning | p95 >300s | 10m |

### Operations Runbook (§11)

Added to `docs/architecture/14_Operations_Runbook.md`:
- §11.1 RSIC Health Indicators — metric table with healthy ranges
- §11.2 Failure Mode Playbooks — diagnosis commands and remediation for all 8 alerts
- §11.3 RSIC Safe Mode — disable all automatic triggers
- §11.4 SLOs — cycle success >95%, meso p95 <5min, action success >95%, rejection <10%, force triggers <1/day
- §11.5 Grafana Dashboard — panel overview

---

## Files Modified

| File | Changes |
|------|---------|
| `internal/metrics/collectors.go` | +12 metric fields + factory initializers (~80 lines) |
| `internal/ape/cycle.go` | +8 emission points (started, completed, error, low_confidence, dry_run, duration) |
| `internal/ape/orchestration_policy.go` | +5 rejection counters (tier_mismatch, dedupe, overlap, cooldown, priority) |
| `internal/ape/safety_validator.go` | +2 safety block counters (protected_space, blast_radius) |
| `internal/ape/watchdog.go` | +3 emissions (decay gauge, escalation gauge, force counter) |
| `internal/ape/calibration.go` | +confidence gauge loop in UpdateCalibration |
| `internal/ape/task_dispatch.go` | +4 emissions (dispatched, success, failed + durations, snapshot created) |
| `docs/architecture/14_Operations_Runbook.md` | +§11 RSIC operations section (~150 lines) |

## Files Created

| File | Content |
|------|---------|
| `deploy/docker/grafana/dashboards/mdemg-rsic.json` | 16-panel Grafana dashboard |
| `deploy/docker/prometheus/alerts/rsic.yaml` | 8 alert rules |
| `docs/api/api-spec/uats/specs/prometheus_rsic_metrics.phase91.uats.json` | UATS spec for RSIC Prometheus metrics |

---

## Verification

1. `go build ./...` — zero errors
2. `go vet ./...` — clean
3. `go test ./internal/ape/...` — all unit tests pass
4. `go test ./internal/metrics/...` — all tests pass
5. `go test ./...` — full suite passes
6. Dashboard JSON and alert YAML validated
7. UATS spec validates RSIC metrics in Prometheus output
