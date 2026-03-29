# Sprint Summary — 2026-03-28

## Overview

Grafana dashboard remediation sprint (2 days). All 7 dashboards audited via Playwright, root causes identified, and fixes implemented across infrastructure, dashboard JSON, and e2e tests. Branch: `reh3376_dev01`.

---

## Problem Statement

All 6 non-topology Grafana dashboards (Overview, Neo4j, RSIC, J17, Jiminy, FT Training) showed no useful data. A comprehensive Playwright + JSON audit identified 7 distinct issues ranging from critical infrastructure bugs to cosmetic degradation problems.

---

## Fixes Applied

### Phase 1: Infrastructure (CRITICAL)

| Issue | Root Cause | Fix | File |
|-------|-----------|-----|------|
| TSDB volume empty | `docker-compose.prod.yml` used `tsdb-data` (hyphenated), creating `docker_tsdb-data` volume. Real data lived in `docker_tsdb_data` (underscored). | Standardized to `tsdb_data` | `deploy/docker/docker-compose.prod.yml` |
| TimescaleDB version mismatch | `latest-pg16` tag resolved to 2.24.0 but data was written by 2.25.1. Cannot downgrade. | Pinned to `timescale/timescaledb:2.25.1-pg16` | `deploy/docker/docker-compose.prod.yml` |
| MetricsRecorder never wired | `NewMetricsRecorder()` called with `nil` writer. `SetTSDBClient()` created writer but never called `SetWriter()` or `Start()`. `FlushToTSDB()` returned immediately on nil check. | Added `SetWriter()` + `Start()` in `SetTSDBClient()`, `Stop()` in `Shutdown()` | `internal/api/server.go` |

**Result**: First flush delivered 99 metric samples to TimescaleDB.

### Phase 2: Dashboard Bug Fixes (HIGH)

| Dashboard | Issue | Fix | File |
|-----------|-------|-----|------|
| J17 | Empty `space_id` default — queries returned nothing | Added `"current": {"text": "mdemg-dev", "value": "mdemg-dev"}` to template variable | `mdemg-j17.json` |
| Jiminy | Same empty `space_id` issue | Same fix | `mdemg-jiminy.json` |
| Neo4j | "Heap Memory" panel showed MDEMG Go process heap, not Neo4j | Renamed to "Neo4j Memory", changed metric to `mdemg_neo4j_container_mem_used_bytes` | `mdemg-neo4j.json` |
| RSIC | Watchdog Escalation showed green with no data (implied healthy) | Added `null+nan` special mapping with "Awaiting Data" text, neutral color | `mdemg-rsic.json` |

### Phase 3: FT Training Dashboard (HIGH)

| Panel | Query Source | Status |
|-------|-------------|--------|
| LLM Interaction Collection Rate | `llm_interactions` time_bucket | Real SQL |
| Model Version Timeline | `ft_model_versions` deployed_at | Real SQL |
| Benchmark Scores | `ft_benchmarks` weighted_score | Real SQL |
| Training Cycle History | `ft_training_cycles` table | Real SQL |
| Exogenous Ratio | `ft_training_cycles` completed cycles | Real SQL |
| Entropy Health | `llm_interactions` quality avg | Real SQL |

Removed `coming-soon` tag. Updated status notice text.

### Phase 4: Graceful No-Data Handling (MEDIUM)

| Dashboard | Panels Updated | noValue |
|-----------|---------------|---------|
| Overview | Request Rate, P95 Latency, Error Rate, Circuit Breakers | `"N/A"` |
| Neo4j | Total Nodes, Total Edges, Total Spaces, Pool Active, Neo4j Memory, Open Circuits | `"N/A"` |

### Phase 5: E2E Test Enhancements

| Test Class | Tests | Coverage |
|------------|-------|----------|
| TestSpaceIdDefaultValue | 7 | All dashboards load with non-empty space_id default |
| TestNoDataGraceful | 6 | All TSDB dashboards with nonexistent space — zero panel errors |
| TestFTTrainingStatusNotice | 1 | Status notice auto-populate text visible |
| TestStatPanelNoValue | 2 | Overview N/A, RSIC Awaiting Data |
| TestNoPanelErrors (expanded) | +1 | Added ft-training |
| TestDashboardNavigation (expanded) | +3 | Added j17, jiminy, ft-training |

**Total e2e tests**: 70 (49 dashboard variables + 21 graph topology). **All passing.**

---

## Documentation Updated

| Document | Changes |
|----------|---------|
| `CHANGELOG.md` | Added Grafana Dashboard Remediation section with all 8 fixes |
| `docs/guides/grafana-dashboard-development.md` | Updated architecture (TimescaleDB replaces Prometheus), added TimescaleDB datasource config, added graceful degradation section (noValue, null+nan mapping, volume naming), updated consistency checklist |
| `docs/development/SPRINT_SUMMARY_20260328.md` | This file |

---

## Verification

| Check | Result |
|-------|--------|
| All 7 dashboard JSON files valid | PASS |
| Go build clean | PASS |
| Playwright e2e: `test_dashboard_variables.py` | 49/49 PASS |
| Playwright e2e: `test_graph_topology.py` | 21/21 PASS |
| Grafana restart picks up changes | PASS |
| MetricsRecorder flushing to TSDB | 99 samples/flush |

---

## Key Lessons

1. **Docker volume naming**: Always use underscores (`tsdb_data`), never hyphens. Docker Compose prefixes with project name using underscores, so `tsdb-data` becomes `docker_tsdb-data` — a different volume.
2. **MetricsRecorder lifecycle**: Constructor accepts nil writer by design (writer created later). Must call `SetWriter()` + `Start()` after writer creation. Without this, the recorder silently drops all data.
3. **Template variable defaults**: Every template variable **must** have a `"current"` block with a sensible default. Without it, `$space_id` interpolates to empty string and all queries return nothing.
4. **Graceful degradation**: Every stat panel needs `"noValue"` in `fieldConfig.defaults`. State indicators need `null+nan` special value mappings to avoid false-positive coloring.

---

## Files Changed

| Category | Files |
|----------|-------|
| Infrastructure | `deploy/docker/docker-compose.prod.yml`, `internal/api/server.go` |
| Dashboards | `mdemg-overview.json`, `mdemg-neo4j.json`, `mdemg-rsic.json`, `mdemg-j17.json`, `mdemg-jiminy.json`, `mdemg-ft-training.json` |
| Tests | `tests/e2e/grafana/test_dashboard_variables.py` |
| Documentation | `CHANGELOG.md`, `docs/guides/grafana-dashboard-development.md`, `docs/development/SPRINT_SUMMARY_20260328.md` |

## Documents Accessed

- `deploy/docker/docker-compose.prod.yml` — TSDB volume and image config
- `deploy/docker/docker-compose.observability.yml` — reference for volume naming
- `deploy/docker/grafana/dashboards/mdemg-*.json` (all 7) — dashboard audit and fixes
- `deploy/docker/grafana/provisioning/datasources/timescaledb.yml` — datasource config
- `deploy/docker/grafana/provisioning/alerting/alerts.yml` — alert rules reference
- `internal/api/server.go` — MetricsRecorder wiring fix
- `internal/metrics/recorder.go` — recorder lifecycle understanding
- `internal/metrics/collectors.go` — metric definitions reference
- `internal/config/config.go` — TSDB config defaults
- `internal/tsdb/migrations/004_aggregate_policies.sql` — continuous aggregate policies
- `tests/e2e/grafana/test_dashboard_variables.py` — e2e test enhancements
- `tests/e2e/grafana/test_graph_topology.py` — full suite verification
- `tests/e2e/grafana/conftest.py` — test fixture reference
- `docs/guides/grafana-dashboard-development.md` — guide updates
- `CHANGELOG.md` — changelog updates
- `docs/development/SPRINT_SUMMARY_20260324.md` — prior sprint reference
