---
created: 2026-04-02
updated: 2026-04-09
version: v0.7.4
author: reh3376
status: active
phase: "DOCKER-P2b"
---

# Browser Dashboard (DOCKER-P2 / P2b)

## Summary

**Feature**: Browser Dashboard UI
**Summary**: 11-tab embedded web dashboard at /ui/ for status monitoring, memory exploration, learning controls, configuration, logs, RSIC, plugins, features, backups, training data, and human review (HITL).


The MDEMG browser dashboard is a lightweight web UI served at `/ui/` from the MDEMG server. It provides quick health overview, unique data not available in Grafana, and action triggers — all without leaving the browser.

## Architecture

- **Vanilla HTML + JS + CSS** — no React, no build step, no Node.js
- **ES modules** loaded via `<script type="module">`
- **Go `embed.FS`** — static files compiled into the binary, served via `http.FileServer`
- **Same-origin `fetch()`** — no CORS needed (UI is embedded in the Go server)
- **Catppuccin Mocha** dark theme

## Access

```
http://localhost:${MDEMG_PORT}/ui/
```

The port is whatever `MDEMG_PORT` is set to in your `.env` (default 9999).

## Tabs

| Tab | Purpose | Data Source |
|-----|---------|-------------|
| **Status** | Health badges, readiness checks, server state, Grafana links, restart button | `/healthz`, `/readyz`, `POST /v1/admin/restart` |
| **Memory** | Layer breakdown, temporal distribution, connectivity, export/import | `/v1/memory/stats`, `/v1/memory/distribution`, `/v1/admin/spaces/export`, `/v1/admin/spaces/import` |
| **Learning** | Hebbian edge stats, freeze/unfreeze, prune, config display, prune threshold | `/v1/learning/stats`, `/v1/learning/freeze/*`, `/v1/learning/prune` |
| **Config** | Editable config table with dropdowns, checkboxes, dirty tracking, Save All | `GET /v1/admin/config`, `PATCH /v1/admin/config` |
| **Logs** | Searchable, color-coded log viewer with level filtering | `/v1/admin/logs` |
| **RSIC** | Service status/state, start/stop/restart, trigger cycle, Grafana link | `/v1/self-improve/*`, `POST /v1/admin/rsic/*` |
| **Plugins** | Plugin cards with type/state badges, start/stop/restart/validate/details | `GET /v1/plugins`, `POST /v1/plugins/{id}/start\|stop\|restart\|validate` |
| **Features** | Controllable + config-only service listing with lifecycle controls | `GET /v1/admin/features`, `POST /v1/admin/features/start\|stop\|restart` |
| **Backup** | Trigger/list/restore/delete backups, active operation polling, type filter | `/v1/backup/trigger`, `/v1/backup/list`, `/v1/backup/status/*`, `/v1/backup/restore`, `DELETE /v1/backup/{id}` |
| **Training Data** | Export TSDB training data for LoRA fine-tuning curation pipeline | `POST /v1/training-data/export`, `GET /v1/training-data/status/{id}`, `GET /v1/training-data/download/{id}` |
| **Review** | HITL grading of curated datasets against versioned rubrics, gold certification, optional live reinforcement (HITL-REVIEW-001) | `/v1/review/*` |

## Config Tab — Editable Configuration

The Config tab provides a fully editable configuration interface:

- **Text inputs** for string/numeric fields
- **Dropdowns** for known enum fields (`embedding.provider`, `llm.provider`, `ingest.speed`, `jiminy.synthesis_provider`, `jiminy.evaluate_llm_provider`)
- **Checkboxes** for boolean fields (`plugins.enabled`, `backup.enabled`, `jiminy.enabled`, `jiminy.synthesis_enabled`, etc.)
- **Read-only** fields for env-sourced values (lock icon) and masked secrets
- **Dirty tracking** — changed fields turn red, "Save All" bar appears with count
- **Search filter** — instant case-insensitive filtering by key or value

Changes are written to the YAML config file via `PATCH /v1/admin/config`. Changes take effect after server restart.

## Features Tab — Service Management

Two groups of services:

**Controllable** (Start/Stop/Restart buttons):
- `ape_scheduler`, `rsic_watchdog`, `backup_scheduler`, `tsdb_backup_scheduler`, `metrics_recorder`, `plugin_manager`

**Config-Only** (status display, controlled via config + restart):
- `jiminy`, `learning`, `retrieval`, `embeddings`, `anomaly_detection`, `conversation`, `hidden_layer`, `scraper`, `rsic_store`, `file_watcher`

## Grafana Deduplication

The browser UI does **not** duplicate data already in Grafana dashboards. Seven Grafana dashboards cover time-series metrics comprehensively:

- **Overview** — request rate, latency percentiles, error rate, circuit breakers
- **Neo4j** — node/edge counts, container resources, connection pool
- **RSIC** — health dimensions, cycle history, watchdog, safety, calibration
- **Jiminy** — guidance follow rate, effectiveness, source diversity
- **J17** — compression, comprehension, tier distribution, NLI bias
- **FT Training** — LLM interactions, model versions, benchmarks
- **Graph Topology** — 3D/2D visualization, neighborhood explorer

The Status tab links to all 7 dashboards. The RSIC tab links to its specific dashboard.

## Polling Strategy

| Interval | What | When |
|----------|------|------|
| 10s | Health checks | Always |
| 10s | RSIC service health | Only when RSIC tab is active |
| 30s | Memory, learning, freeze status | Always |
| 5s | Log entries | Only when Logs tab is active |
| On-demand | Config, Plugins, Features, RSIC trigger | On tab switch / button click |

Polling pauses automatically when the browser tab is hidden (Page Visibility API).

## API Endpoints Added

### `GET /v1/admin/config`

Returns effective configuration with source attribution.

```json
{
  "config": [
    {"key": "neo4j.uri", "value": "bolt://neo4j:7687", "source": "yaml", "masked": false},
    {"key": "openai.api_key", "value": "****", "source": "env", "masked": true}
  ],
  "yaml_path": ".mdemg/config.yaml"
}
```

### `PATCH /v1/admin/config`

Updates YAML config values. Rejects env-sourced keys, masked/sensitive keys, and unknown keys.

**Request:**
```json
{"updates": {"learning.eta": "0.05", "plugins.enabled": "true"}}
```

**Response (200):**
```json
{
  "config": [...],
  "yaml_path": ".mdemg/config.yaml",
  "updated": 2
}
```

**Error (400):** `{"error": "key 'neo4j.uri' is set via environment variable and cannot be changed in YAML"}`

### `GET /v1/admin/logs?limit=200`

Returns recent log entries from an in-process ring buffer. Filtering is client-side.

```json
{
  "entries": [
    {"timestamp": "2026-03-30T12:00:00Z", "level": "INFO", "message": "server started", "raw": "..."}
  ],
  "seq": 42
}
```

### `POST /v1/admin/restart`

Triggers a graceful server restart via re-exec. Returns `{"status": "restarting"}` immediately. The JS UI polls `/healthz` every 2s and reloads when the server returns.

### `POST /v1/admin/rsic/start|stop|restart`

Controls the RSIC watchdog lifecycle. Returns `{"status": "started|stopped|restarted"}`.

### `GET /v1/admin/features`

Returns all known services with runtime status.

```json
{
  "features": [
    {"name": "ape_scheduler", "status": "healthy", "state": "running", "controllable": true},
    {"name": "jiminy", "status": "healthy", "state": "running", "controllable": false, "config_key": "jiminy.enabled"}
  ]
}
```

### `POST /v1/admin/features/start|stop|restart`

Controls lifecycle of controllable services. Request body: `{"name": "ape_scheduler"}`. Returns `{"status": "started", "name": "ape_scheduler"}`.

### `POST /v1/plugins/{id}/start|stop|restart`

Controls lifecycle of individual plugins. Returns `{"status": "started", "plugin_id": "..."}`.

## File Structure

```
internal/api/
  log_buffer.go           — LogRingBuffer (io.Writer, thread-safe ring)
  log_buffer_test.go      — 12 unit tests
  handlers_ui.go          — handleAdminConfig (GET+PATCH), handleAdminLogs, RSIC lifecycle, restart
  handlers_features.go    — handleFeatures (GET), handleFeatureLifecycle (POST)
  restart_unix.go         — syscall.Exec restart (Unix)
  restart_windows.go      — exec.Command restart (Windows)
  ui_embed.go             — //go:embed ui/* + http.FileServer
  ui/
    index.html            — HTML shell + Catppuccin Mocha CSS
    main.js               — Tab switching, polling orchestration (11 tabs)
    api.js                — All fetch() calls (get/post/patch/del)
    state.js              — Pub/sub reactive state
    utils/
      dom.js              — h(), infoRow(), sectionHeader(), statusBadge()
      formatting.js       — formatNumber(), formatUptime(), timeAgo()
    tabs/
      status.js           — Health badges + Grafana links + restart button
      memory.js           — Memory stats + export/import
      learning.js         — Hebbian stats + freeze/unfreeze/prune + config display
      config.js           — Editable config table (dropdowns, checkboxes, dirty tracking)
      logs.js             — Log viewer
      rsic.js             — Service status + start/stop/restart + trigger + Grafana link
      plugins.js          — Plugin cards with lifecycle controls
      features.js         — Service listing with lifecycle controls
      backup.js           — Backup trigger/list/restore/delete + status polling
      training_data.js    — Training data export for LoRA fine-tuning
      review.js           — HITL dataset grading (HITL-REVIEW-001)
```

## E2E Test Coverage (UI-AUDIT-2026-04-09)

**Suite**: `tests/e2e/browser-ui/test_browser_ui.py` — 309 tests (306 pass, 3 skip)
**Run**: `make test-e2e-browser-ui` (~40 min sequential)

| Test Class | Tests | Type |
|-----------|-------|------|
| TestPageLoad | 5 | Read-only |
| TestTabNavigation | 22 | Read-only (parametrized) |
| TestStatusTab | 18 | Read-only |
| TestMemoryTab | 15 | Read-only |
| TestLearningTab | 12 | Read-only |
| TestConfigTab | 14 | Read-only |
| TestLogsTab | 4 | Read-only |
| TestRsicTab | 14 | Read-only |
| TestPluginsTab | 8 | Read-only |
| TestFeaturesTab | 7 | Read-only |
| TestBackupTab | 11 | Read-only |
| TestTrainingDataTab | 10 | Read-only |
| TestHelpPanels | 50 | Read-only (parametrized) |
| TestTheme | 4 | Read-only |
| TestInstanceSelector | 3 | Read-only |
| TestScreenshots | 30 | Visual baselines (parametrized) |
| TestInteractiveStatus | 1 | Functional |
| TestInteractiveMemory | 3 | Functional |
| TestInteractiveLearning | 3 | Functional |
| TestInteractiveConfig | 3 | Functional |
| TestInteractiveLogs | 2 | Functional |
| TestInteractiveRsic | 3 | Functional |
| TestInteractivePlugins | 1 | Functional |
| TestInteractiveFeatures | 1 | Functional |
| TestInteractiveBackup | 4 | Functional |
| TestInteractiveTrainingData | 4 | Functional |
| TestStatusAPI | 6 | API |
| TestInstanceAPI | 5 | API |
| TestTrainingDataAPI | 3 | API |

### Interactive Test Patterns

All `TestInteractive*` classes:
- Register `page.on("dialog", handler)` in autouse fixture (handles `confirm()`, `alert()`, `prompt()`)
- Restore state in `finally` blocks via direct `requests` API calls
- Use `expect_request`/`expect_response` to verify API calls triggered by clicks

## API Coverage Gap Analysis

See `docs/features/ui-gap-analysis.md` for the full analysis.

**Summary**: 48 of 125 server routes (38%) have corresponding `api.js` wrappers and UI representation.

**Well-covered**: Backup (7/7), Config (2/2), Features (4/4), Plugins (6/6), RSIC (3/3), Training Data (3/3)
**Uncovered**: Jiminy (15 routes), Conversation (13 routes), Memory operations (25 routes), Constraints (6 routes), Metrics (4 routes), Infrastructure (10+ routes)
