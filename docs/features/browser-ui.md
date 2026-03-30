# Browser Dashboard (DOCKER-P2)

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
| **Status** | Health badges, readiness checks, embedding info, Grafana links | `/healthz`, `/readyz`, `/v1/embedding/health` |
| **Memory** | Layer breakdown, temporal distribution, connectivity, export/import | `/v1/memory/stats`, `/v1/memory/distribution`, `/v1/admin/spaces/export`, `/v1/admin/spaces/import` |
| **Learning** | Hebbian edge stats, freeze/unfreeze, prune | `/v1/learning/stats`, `/v1/learning/freeze/*`, `/v1/learning/prune` |
| **Config** | Effective configuration table (key, value, source) | `/v1/admin/config` |
| **Logs** | Searchable, color-coded log viewer with level filtering | `/v1/admin/logs` |
| **RSIC** | Trigger self-improvement cycle + link to Grafana RSIC dashboard | `/v1/self-improve/cycle` |

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
| 30s | Memory, learning, freeze status | Always |
| 5s | Log entries | Only when Logs tab is active |
| On-demand | Config, RSIC trigger | On tab switch / button click |

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

## File Structure

```
internal/api/
  log_buffer.go           — LogRingBuffer (io.Writer, thread-safe ring)
  log_buffer_test.go      — 12 unit tests
  handlers_ui.go          — handleAdminConfig + handleAdminLogs
  ui_embed.go             — //go:embed ui/* + http.FileServer
  ui/
    index.html            — HTML shell + Catppuccin Mocha CSS
    main.js               — Tab switching, polling orchestration
    api.js                — All fetch() calls
    state.js              — Pub/sub reactive state
    utils/
      dom.js              — h(), infoRow(), sectionHeader(), statusBadge()
      formatting.js       — formatNumber(), formatUptime(), timeAgo()
    tabs/
      status.js           — Health badges + Grafana links
      memory.js           — Memory stats + export/import
      learning.js         — Hebbian stats + freeze/unfreeze/prune
      config.js           — Config table
      logs.js             — Log viewer
      rsic.js             — Trigger action + Grafana link
```
