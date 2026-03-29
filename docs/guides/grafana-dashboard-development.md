# Grafana Dashboard Development Guide

> How to build, provision, and troubleshoot Grafana dashboards for MDEMG —
> covering Prometheus metrics, Neo4j graph data, and Node Graph visualization.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Required Plugins](#required-plugins)
3. [Datasource Configuration](#datasource-configuration)
4. [Dashboard Provisioning](#dashboard-provisioning)
5. [Dashboard JSON Structure](#dashboard-json-structure)
6. [Template Variables](#template-variables)
7. [Navigation: Dashboard Dropdown Links](#navigation-dashboard-dropdown-links)
8. [Panel Types and Patterns](#panel-types-and-patterns)
9. [Explicit Datasource UIDs](#explicit-datasource-uids)
10. [Row Organization](#row-organization)
11. [Issues Encountered and Solutions](#issues-encountered-and-solutions)
12. [Testing Dashboards with Playwright](#testing-dashboards-with-playwright)
13. [Quick Reference](#quick-reference)

---

## Architecture Overview

```
docker-compose.prod.yml
├── timescaledb      (metrics storage, TimescaleDB 2.25.1 on PostgreSQL 16)
├── grafana          (visualization, port 3000)
│   ├── provisioning/datasources/   (auto-configured on startup)
│   │   ├── timescaledb.yml
│   │   ├── neo4j.yml
│   │   └── nodegraph-api.yml
│   ├── provisioning/dashboards/
│   │   └── dashboards.yml          (tells Grafana where to find JSON files)
│   ├── provisioning/alerting/
│   │   └── alerts.yml              (22 alert rules across 3 groups)
│   └── dashboards/
│       ├── mdemg-overview.json      (home dashboard)
│       ├── mdemg-graph-topology.json
│       ├── mdemg-rsic.json
│       ├── mdemg-neo4j.json
│       ├── mdemg-j17.json
│       ├── mdemg-jiminy.json
│       └── mdemg-ft-training.json
├── neo4j            (graph database, port 7687)
└── mdemg            (server on port 8080/9999)
```

**Datasource**: TimescaleDB (PostgreSQL with time-series extensions). Panels use raw SQL against `metric_samples`, `llm_interactions`, `ft_benchmarks`, `ft_training_cycles`, `ft_model_versions` tables. Continuous aggregates `metrics_hourly` and `metrics_daily` provide pre-aggregated rollups.

**MetricsRecorder**: The `internal/metrics/recorder.go` `MetricsRecorder` periodically flushes collected metrics to TimescaleDB via `FlushToTSDB()`. The recorder must be wired to a writer via `SetWriter()` and started via `Start()` in `server.go`'s `SetTSDBClient()` method.

**Grafana image**: `grafana/grafana:10.2.2`

**Host access from containers**: Uses `extra_hosts: host.docker.internal:host-gateway` so containers can reach services running on the Docker host (MDEMG API, Neo4j).

**Startup**: `docker compose -f docker-compose.observability.yml up -d`

**Restart after dashboard changes**: `docker compose -f docker-compose.observability.yml restart grafana`

**Default credentials**: `admin` / `admin`

---

## Required Plugins

Two community plugins are installed automatically via `GF_INSTALL_PLUGINS` in the Grafana service definition:

| Plugin ID | Purpose | Datasource Type |
|-----------|---------|-----------------|
| `kniepdennis-neo4j-datasource` | Query Neo4j via Bolt protocol | `kniepdennis-neo4j-datasource` |
| `hamedkarbasi93-nodegraphapi-datasource` | Render node-graph panels from REST API | `hamedkarbasi93-nodegraphapi-datasource` |

In `docker-compose.observability.yml`:

```yaml
environment:
  GF_INSTALL_PLUGINS: kniepdennis-neo4j-datasource,hamedkarbasi93-nodegraphapi-datasource
```

**Important**: Plugins are installed at container startup. If the container already has a persistent volume (`grafana-data`), plugins persist across restarts. If you need to force a re-install, delete the named volume: `docker volume rm docker_grafana-data`.

---

## Datasource Configuration

Datasources are provisioned from YAML files mounted at `/etc/grafana/provisioning/datasources/` (read-only). Grafana reads these on startup and creates/updates the datasources automatically.

### TimescaleDB (`timescaledb.yml`)

```yaml
apiVersion: 1
datasources:
  - name: TimescaleDB
    uid: timescaledb          # <-- CRITICAL: explicit UID for dashboard portability
    type: postgres
    access: proxy
    url: timescaledb:5432
    isDefault: true
    editable: false
    jsonData:
      database: mdemg_metrics
      sslmode: disable
      maxOpenConns: 10
      maxIdleConns: 5
      connMaxLifetime: 14400
      postgresVersion: 1600
      timescaledb: true
    secureJsonData:
      user: mdemg
      password: ${TSDB_PASSWORD}
```

### Neo4j (`neo4j.yml`)

```yaml
apiVersion: 1
datasources:
  - name: Neo4j
    uid: neo4j               # <-- explicit UID
    type: kniepdennis-neo4j-datasource
    access: proxy
    editable: false
    jsonData:
      url: "bolt://host.docker.internal:7687"
      database: "neo4j"
      username: "neo4j"
    secureJsonData:
      password: "testpassword"
```

### Node Graph API (`nodegraph-api.yml`)

```yaml
apiVersion: 1
datasources:
  - name: MDEMG Graph API
    uid: mdemg-nodegraph      # <-- explicit UID
    type: hamedkarbasi93-nodegraphapi-datasource
    access: proxy
    editable: false
    jsonData:
      url: "http://host.docker.internal:9999"
```

### Why Explicit UIDs Matter

Every datasource **must** have an explicit `uid` field. Without it:
- Grafana auto-generates a random UID on first provision
- Dashboard JSON panels that reference `{ "uid": "prometheus" }` will fail to resolve
- Dashboards become non-portable between Grafana instances

**Rule**: Always set `uid` in the provisioning YAML, then reference that exact UID in dashboard panel `datasource` fields.

---

## Dashboard Provisioning

The file `grafana/provisioning/dashboards/dashboards.yml` tells Grafana where to find dashboard JSON files:

```yaml
apiVersion: 1
providers:
  - name: MDEMG Dashboards
    orgId: 1
    folder: MDEMG
    folderUid: mdemg-folder
    type: file
    disableDeletion: false
    updateIntervalSeconds: 30    # Grafana re-reads files every 30s
    allowUiUpdates: true
    options:
      path: /etc/grafana/dashboards
      foldersFromFilesStructure: false
```

**Key settings**:
- `folder: MDEMG` — all dashboards appear under a "MDEMG" folder in the Grafana sidebar
- `allowUiUpdates: true` — allows editing dashboards in the Grafana UI for prototyping (changes persist to the volume, not back to the JSON file)
- `updateIntervalSeconds: 30` — after editing a JSON file on disk, Grafana picks up changes within 30 seconds (or restart for immediate effect)

**Home dashboard**: Set via environment variable:
```yaml
GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH: /etc/grafana/dashboards/mdemg-overview.json
```

---

## Dashboard JSON Structure

Every dashboard JSON file follows this top-level structure:

```json
{
  "annotations": { "list": [] },
  "editable": true,
  "gnetId": null,
  "graphTooltip": 1,
  "id": null,
  "links": [ ... ],
  "panels": [ ... ],
  "schemaVersion": 39,
  "tags": ["mdemg", "<dashboard-specific-tag>"],
  "templating": { "list": [ ... ] },
  "time": { "from": "now-6h", "to": "now" },
  "timepicker": {},
  "timezone": "browser",
  "title": "Dashboard Title",
  "uid": "dashboard-uid"
}
```

**Consistency rules for MDEMG dashboards**:

| Field | Standard Value | Why |
|-------|---------------|-----|
| `graphTooltip` | `1` | Shared crosshair across all panels |
| `schemaVersion` | `39` | Grafana 10.x features (variable interpolation, panel options) |
| `tags` | Always includes `"mdemg"` | Enables tag-based dashboard dropdown navigation |
| `id` | `null` | Let Grafana assign; avoids conflicts on import |

---

## Template Variables

Template variables create dropdown selectors in the dashboard header. MDEMG uses three patterns:

### Pattern 1: Custom Variable (Static Values)

For values known at design time (instance addresses, enum-like choices):

```json
{
  "name": "instance",
  "label": "Instance",
  "type": "custom",
  "query": "localhost:9999",
  "current": {
    "text": "localhost:9999",
    "value": "localhost:9999"
  },
  "options": [
    {
      "text": "localhost:9999",
      "value": "localhost:9999",
      "selected": true
    }
  ]
}
```

For multi-value custom variables (e.g., edge types):

```json
{
  "name": "edge_types",
  "label": "Edge Types",
  "type": "custom",
  "query": "SIMILAR_TO,RELATES_TO,THEME_OF,...",
  "multi": true,
  "includeAll": true,
  "current": { "text": "All", "value": "$__all" }
}
```

### Pattern 2: TimescaleDB Query Variable

Dynamically populates from TSDB SQL:

```json
{
  "name": "space_id",
  "label": "Space ID",
  "type": "query",
  "datasource": {
    "type": "postgres",
    "uid": "timescaledb"
  },
  "query": "SELECT DISTINCT space_id FROM metric_samples WHERE metric_name = 'mdemg_rsic_health_overall' AND space_id NOT SIMILAR TO 'test-.*' ORDER BY space_id",
  "refresh": 2,
  "sort": 1,
  "current": {
    "text": "mdemg-dev",
    "value": "mdemg-dev"
  }
}
```

**Key fields**:
- `refresh: 2` — refresh on time range change (1 = on dashboard load, 2 = on time range change)
- `sort: 1` — alphabetical ascending
- `current` block **must** be present with a default value — without it, queries using `$space_id` return nothing on first load
- SQL query is a plain string (not wrapped in an object like the old Prometheus pattern)

### Pattern 3: Neo4j Cypher Query Variable

Dynamically populates from Neo4j graph data:

```json
{
  "name": "space_id",
  "label": "Space ID",
  "type": "query",
  "datasource": {
    "type": "kniepdennis-neo4j-datasource",
    "uid": "neo4j"
  },
  "query": "MATCH (n:MemoryNode) RETURN DISTINCT n.space_id AS __value, n.space_id AS __text ORDER BY n.space_id",
  "refresh": 1,
  "sort": 1
}
```

**Important**: The Neo4j plugin expects the Cypher query to return columns named `__value` and `__text`. The `query` field is a plain string (not wrapped in an object like Prometheus).

### Variable Interpolation in Queries

| Syntax | Result | Use Case |
|--------|--------|----------|
| `$space_id` | `mdemg-dev` | Single-value exact match |
| `${space_id}` | `mdemg-dev` | Same, but unambiguous in expressions |
| `$space_id` in `{space_id="$space_id"}` | `{space_id="mdemg-dev"}` | Prometheus label filter |
| `$space_id` in `{space_id=~"$space_id"}` | `{space_id=~"mdemg-dev"}` | Prometheus regex filter (use for multi-value) |
| `${edge_types:csv}` | `SIMILAR_TO,RELATES_TO` | CSV join for multi-value variables |
| `${layers:raw}` | `[0,1,2]` | Raw value without escaping |

---

## Navigation: Dashboard Dropdown Links

All MDEMG dashboards use a tag-based dropdown link for cross-dashboard navigation:

```json
"links": [
  {
    "title": "Dashboards",
    "type": "dashboards",
    "tags": ["mdemg"],
    "icon": "external link",
    "asDropdown": true
  }
]
```

This renders a "Dashboards" button in the top-right corner of the dashboard header. On **hover**, it shows a dropdown listing all dashboards tagged with `"mdemg"`. Clicking the button itself navigates to the Grafana dashboard listing page filtered by the tag.

**Requirements**:
- Every MDEMG dashboard must include `"mdemg"` in its `tags` array
- Every dashboard must include this same `links` entry
- The dropdown auto-discovers new dashboards — no manual link maintenance needed

**Gotcha**: In automated testing (Playwright), a `.click()` on the link navigates away from the dashboard. The dropdown is hover-activated — this is correct browser behavior, not a bug.

---

## Panel Types and Patterns

### Stat Panel

Single-value display with optional sparkline:

```json
{
  "title": "Overall Health",
  "type": "stat",
  "gridPos": { "h": 4, "w": 4, "x": 0, "y": 1 },
  "datasource": { "type": "prometheus", "uid": "prometheus" },
  "targets": [
    {
      "expr": "mdemg_rsic_health_overall{space_id=\"$space_id\"}",
      "legendFormat": "overall"
    }
  ],
  "options": {
    "colorMode": "value",
    "graphMode": "area"        // "area" = sparkline, "none" = number only
  },
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit",   // 0-1 displayed as percentage
      "min": 0,
      "max": 1,
      "noValue": "N/A",
      "thresholds": {
        "mode": "absolute",
        "steps": [
          { "color": "red", "value": null },    // null = base (minimum)
          { "color": "yellow", "value": 0.4 },
          { "color": "green", "value": 0.7 }
        ]
      }
    }
  }
}
```

### Gauge Panel

Multiple arc gauges in a single panel:

```json
{
  "title": "Health Sub-Scores",
  "type": "gauge",
  "targets": [
    { "expr": "metric_a{...}", "legendFormat": "Label A", "refId": "A" },
    { "expr": "metric_b{...}", "legendFormat": "Label B", "refId": "B" }
  ],
  "options": {
    "showThresholdLabels": false,
    "showThresholdMarkers": true
  },
  "fieldConfig": {
    "defaults": {
      "min": 0, "max": 1,
      "unit": "percentunit",
      "thresholds": { ... }
    }
  }
}
```

Each target with a unique `refId` renders as a separate gauge arc.

### Timeseries Panel

Line/area/bar charts over time:

```json
{
  "title": "Health Trend",
  "type": "timeseries",
  "fieldConfig": {
    "defaults": {
      "unit": "percentunit",
      "min": 0, "max": 1,
      "custom": {
        "drawStyle": "line",     // "line", "bars", "points"
        "lineWidth": 2,
        "fillOpacity": 10,
        "pointSize": 5,
        "showPoints": "auto",
        "stacking": { "mode": "none" }  // "normal" for stacked bars
      }
    },
    "overrides": [
      {
        "matcher": { "id": "byName", "options": "Buffer Entries" },
        "properties": [
          { "id": "custom.drawStyle", "value": "bars" },
          { "id": "custom.fillOpacity", "value": 50 }
        ]
      }
    ]
  },
  "options": {
    "legend": {
      "displayMode": "table",      // "list" or "table"
      "placement": "right",        // "bottom" or "right"
      "calcs": ["lastNotNull"]     // show last value in legend table
    }
  }
}
```

### Table Panel

Tabular data with transformations:

```json
{
  "title": "Action Success Rate",
  "type": "table",
  "targets": [
    { "expr": "...", "instant": true, "format": "table", "refId": "success" },
    { "expr": "...", "instant": true, "format": "table", "refId": "failed" }
  ],
  "transformations": [
    { "id": "merge" },
    {
      "id": "organize",
      "options": {
        "renameByName": {
          "Value #success": "Success",
          "Value #failed": "Failed"
        }
      }
    }
  ]
}
```

**Important**: Use `"instant": true` and `"format": "table"` for table panels with Prometheus data.

### Pie Chart Panel

```json
{
  "title": "Cycle Outcomes",
  "type": "piechart",
  "options": {
    "legend": {
      "displayMode": "table",
      "placement": "right",
      "values": ["value", "percent"]
    },
    "pieType": "donut"    // "pie" or "donut"
  }
}
```

### Text Panel

Static instructional content (Markdown):

```json
{
  "title": "How to Explore",
  "type": "text",
  "options": {
    "mode": "markdown",
    "content": "## Instructions\n\n1. Select a space...\n2. Choose layers..."
  }
}
```

### Value Mappings

Map numeric values to human-readable text (e.g., watchdog escalation levels):

```json
"mappings": [
  { "type": "value", "options": { "0": { "text": "Nominal", "color": "green" } } },
  { "type": "value", "options": { "1": { "text": "Nudge", "color": "yellow" } } },
  { "type": "value", "options": { "2": { "text": "Warn", "color": "orange" } } },
  { "type": "value", "options": { "3": { "text": "Force", "color": "red" } } }
]
```

### Node Graph Panel

Used in the Graph Topology dashboard for visualizing Neo4j graph data:

```json
{
  "title": "Graph Topology",
  "type": "nodeGraph",
  "datasource": { "type": "hamedkarbasi93-nodegraphapi-datasource", "uid": "mdemg-nodegraph" },
  "targets": [
    {
      "queryText": "type=overview&space_id=${space_id}&layers=${layers:csv}&show_hidden=${show_hidden}"
    }
  ],
  "fieldConfig": {
    "defaults": {
      "custom": {
        "nodeWidth": 120,
        "nodeHeight": 40
      }
    }
  }
}
```

The Node Graph API datasource sends the `queryText` as URL query parameters to the MDEMG API's `/v1/graph/viz` endpoint.

---

## Explicit Datasource UIDs

**Every panel must have an explicit `datasource` field**:

```json
"datasource": { "type": "prometheus", "uid": "prometheus" }
```

Without this, panels rely on the "default datasource" fallback. This works when Prometheus is marked `isDefault: true`, but:
- Fails silently if the default changes
- Makes dashboards non-portable
- Causes confusion when multiple datasources exist

**Pattern**: Set `datasource` on every panel, matching the `uid` from the provisioning YAML.

| Datasource | Reference |
|------------|-----------|
| TimescaleDB | `{ "type": "postgres", "uid": "timescaledb" }` |
| Neo4j | `{ "type": "kniepdennis-neo4j-datasource", "uid": "neo4j" }` |
| Node Graph API | `{ "type": "hamedkarbasi93-nodegraphapi-datasource", "uid": "mdemg-nodegraph" }` |

---

## Row Organization

Rows group panels into collapsible sections:

### Open Row (Default)

```json
{
  "title": "RSIC Overview",
  "type": "row",
  "gridPos": { "h": 1, "w": 24, "x": 0, "y": 18 },
  "collapsed": false
}
```

Panels below this row (higher `y` values) belong to it visually.

### Collapsed Row

```json
{
  "title": "Exploration",
  "type": "row",
  "gridPos": { "h": 1, "w": 24, "x": 0, "y": 35 },
  "collapsed": true,
  "panels": [
    { ... panel 1 ... },
    { ... panel 2 ... }
  ]
}
```

**Critical**: When `"collapsed": true`, panels must be nested **inside** the row's `"panels"` array — not as siblings in the top-level `panels` array. If they're siblings, Grafana won't hide them when the row is collapsed.

### Y-Position Management

When inserting a new row at the top, shift all existing panel `y` values down by the height of the new section:

| Scenario | Action |
|----------|--------|
| New row at y=0, height=18 | Add 18 to every existing panel's `y` |
| New collapsed row | Only the row header occupies `y` space; nested panels don't affect layout |

---

## Issues Encountered and Solutions

### Issue 1: Dashboard Panels Show "No Data" Despite Metrics Flowing

**Symptom**: Panels display "No data" even though `SELECT count(*) FROM metric_samples` returns rows.

**Root causes**:
1. **MetricsRecorder not wired**: `SetWriter()` and `Start()` must be called on the recorder in `SetTSDBClient()`. Without this, `FlushToTSDB()` exits immediately on nil writer check.
2. **Volume name mismatch**: Docker Compose creates volumes as `{project}_{name}`. Using `tsdb-data` (hyphenated) creates `docker_tsdb-data`, not `docker_tsdb_data`. Always use underscores: `tsdb_data`.
3. **space_id variable empty**: If the template variable has no `current` block, queries using `$space_id` interpolate to empty string and return nothing.

**Solution**: Check the data pipeline end-to-end:
```bash
# 1. Verify TSDB has data
PGPASSWORD=mdemg_metrics psql -h localhost -p 5433 -U mdemg -d mdemg_metrics -c "SELECT count(*) FROM metric_samples;"
# 2. Verify metrics are being flushed (check server logs)
docker logs docker-mdemg-1 2>&1 | grep "flushed.*metric"
# 3. Verify the correct volume is mounted
docker volume ls | grep tsdb
```

### Issue 2: Template Variable Dropdown Shows Empty

**Symptom**: A TimescaleDB query variable shows no options.

**Root causes**:
1. No data in `metric_samples` for the queried metric name
2. The `datasource` field in the variable definition doesn't match the provisioned UID
3. Missing `current` block — ALWAYS include a default

**Solution**: For TimescaleDB, the query is a plain SQL string:
```json
"query": "SELECT DISTINCT space_id FROM metric_samples WHERE metric_name = 'mdemg_rsic_health_overall' ORDER BY space_id"
```

For Neo4j, the query is also a plain string:
```json
"query": "MATCH (n:MemoryNode) RETURN DISTINCT n.space_id AS __value, n.space_id AS __text"
```

### Issue 3: Provisioned Datasource UID Mismatch

**Symptom**: Panels show "Datasource not found" error.

**Root cause**: The datasource provisioning YAML didn't include a `uid` field. Grafana auto-generated a random UID that doesn't match the `{ "uid": "prometheus" }` in the dashboard JSON.

**Solution**: Always set explicit `uid` in the provisioning YAML. If the datasource was already provisioned without a UID:
1. Add `uid: prometheus` to the YAML
2. Delete the Grafana volume: `docker volume rm docker_grafana-data`
3. Restart: `docker compose -f docker-compose.observability.yml up -d`

### Issue 4: Collapsed Row Panels Still Visible

**Symptom**: Setting `"collapsed": true` on a row doesn't hide its panels.

**Root cause**: Panels were placed as siblings in the top-level `panels` array instead of nested inside the row's `panels` array.

**Solution**: Move panels into the row:
```json
// WRONG: panels as siblings
{ "title": "My Row", "type": "row", "collapsed": true },
{ "title": "Panel A", "type": "stat", ... }

// CORRECT: panels nested inside row
{
  "title": "My Row",
  "type": "row",
  "collapsed": true,
  "panels": [
    { "title": "Panel A", "type": "stat", ... }
  ]
}
```

### Issue 5: Dashboard Dropdown Link Navigates Away

**Symptom**: Clicking the "Dashboards" link in the header navigates to the Grafana Dashboards listing page instead of showing a dropdown.

**Explanation**: This is expected behavior. The `"asDropdown": true` link creates a **hover-activated** dropdown. The link's `href` still points to the tag-filtered listing page. In a browser:
- **Hover** = dropdown appears with dashboard list
- **Click** = navigates to the filtered listing page

This only manifests as a "problem" in automated testing where `.click()` is used instead of `.hover()`.

### Issue 6: Neo4j Cypher Variables Require Specific Column Names

**Symptom**: Neo4j query variable shows no options despite a valid Cypher query.

**Root cause**: The `kniepdennis-neo4j-datasource` plugin expects specific column aliases.

**Solution**: Return `__value` and `__text` columns:
```cypher
MATCH (n:MemoryNode)
RETURN DISTINCT n.space_id AS __value, n.space_id AS __text
ORDER BY n.space_id
```

### Issue 7: Node Graph Panel Shows Empty

**Symptom**: The node graph visualization panel loads but shows no nodes or edges.

**Root cause**: The `hamedkarbasi93-nodegraphapi-datasource` plugin expects the API to return data in a specific format with `nodes` and `edges` arrays, where nodes have `id`, `title`, and optional fields, and edges have `source`, `target`, `id`.

**Solution**: Ensure the MDEMG API's `/v1/graph/viz` endpoint returns the correct schema. The `queryText` field in the panel target is sent as URL query parameters to the datasource URL.

### Issue 8: Gauge Arcs Not Showing for All Queries

**Symptom**: A gauge panel with multiple targets only shows some arcs.

**Root cause**: Metrics that haven't been published yet return no data, and Grafana skips them in the gauge display.

**Solution**: Ensure all metrics are populated. Set `"noValue": "0"` in `fieldConfig.defaults` to display zero instead of hiding missing data:
```json
"fieldConfig": {
  "defaults": {
    "noValue": "0",
    "min": 0,
    "max": 1
  }
}
```

---

## Testing Dashboards with Playwright

### Setup

```bash
pip install playwright
playwright install chromium
```

### Basic Pattern

```python
from playwright.sync_api import sync_playwright

with sync_playwright() as p:
    browser = p.chromium.launch()
    page = browser.new_page(viewport={"width": 1920, "height": 1080})

    # Login (required after Grafana restart)
    page.goto("http://localhost:3000/login", wait_until="networkidle")
    page.fill('input[name="user"]', 'admin')
    page.fill('input[name="password"]', 'admin')
    page.click('button[type="submit"]')
    page.wait_for_timeout(3000)

    # Navigate to dashboard
    page.goto("http://localhost:3000/d/mdemg-rsic", wait_until="networkidle")
    page.wait_for_timeout(3000)

    # Verify content
    body = page.text_content("body")
    assert "Instance" in body, "Missing Instance variable"
    assert "Dashboards" in body, "Missing Dashboards link"

    # Screenshot
    page.screenshot(path="/tmp/dashboard.png")
    browser.close()
```

### Testing Tips

| Scenario | Approach |
|----------|----------|
| Verify variable exists | `"Variable Name" in page.text_content("body")` |
| Verify panel renders | Screenshot + visual inspection (Grafana panel selectors are version-specific and unreliable) |
| Verify section exists | `page.text_content("body")` contains the row title |
| Test collapsed row expansion | `page.locator('[data-testid="dashboard-row-container"]').filter(has_text="Row Name").click()` |
| Test dropdown link | Use `.hover()` not `.click()` — click navigates away |
| Scroll to section | `page.evaluate("window.scrollTo(0, Y)")` then screenshot |

### JSON Validation Before Deploy

Always validate dashboard JSON syntax before restarting Grafana:

```bash
python3 -c "import json; json.load(open('deploy/docker/grafana/dashboards/mdemg-rsic.json')); print('OK')"
```

---

## Graceful Degradation: No-Data Handling

Every stat panel **must** handle the no-data case gracefully. Without explicit configuration, stat panels show a bare "No data" message which is confusing for operators.

### noValue Property

Add `"noValue"` to `fieldConfig.defaults`:

| Panel Type | noValue Setting | Example |
|------------|----------------|---------|
| Rate/count stats (Request Rate, Error Rate) | `"N/A"` | Overview, Neo4j stat panels |
| State indicators (Watchdog, Circuit Breaker) | `"Awaiting Data"` | RSIC Watchdog Escalation |
| FT pipeline panels | `"Not yet collected"` | FT Training panels |
| Numeric stats (counts, totals) | `"N/A"` | Neo4j Total Nodes/Edges |

### Special Value Mapping for Null/NaN

For state indicator panels where color carries meaning (e.g., green = healthy), add a `null+nan` special mapping to prevent misleading coloring when there's no data:

```json
"mappings": [
  {
    "type": "special",
    "options": {
      "match": "null+nan",
      "result": {
        "text": "Awaiting Data",
        "color": "text"
      }
    }
  },
  { "type": "value", "options": { "0": { "text": "Nominal", "color": "green" } } },
  { "type": "value", "options": { "1": { "text": "Nudge", "color": "yellow" } } }
]
```

The `"color": "text"` uses Grafana's neutral text color, avoiding false green/red signals.

### TSDB Volume Naming Convention

**Always use underscores, never hyphens** in Docker Compose volume names:

```yaml
# CORRECT
volumes:
  tsdb_data:
  neo4j_data:

# WRONG — creates different Docker volume name
volumes:
  tsdb-data:
  neo4j-data:
```

Docker Compose prefixes volumes with the project name using underscores: `docker_tsdb_data`. A hyphenated name creates `docker_tsdb-data` — a completely different volume. This caused a critical data loss bug where the container mounted an empty volume while 3.5M rows of data lived in the original.

---

## Quick Reference

### Creating a New Dashboard

1. Create `deploy/docker/grafana/dashboards/mdemg-<name>.json`
2. Set a unique `uid` (e.g., `"mdemg-<name>"`)
3. Include `"mdemg"` in the `tags` array
4. Add the Instance template variable
5. Add the Dashboard dropdown link
6. Set explicit `datasource` UIDs on every panel
7. Set `schemaVersion: 39`, `graphTooltip: 1`
8. Validate JSON: `python3 -c "import json; json.load(open('path/to/file.json'))"`
9. Restart Grafana: `docker compose -f docker-compose.observability.yml restart grafana`

### Dashboard Consistency Checklist

- [ ] `schemaVersion: 39`
- [ ] `graphTooltip: 1`
- [ ] `tags` includes `"mdemg"`
- [ ] Instance and Space ID template variables present
- [ ] Space ID variable has `"current": {"text": "mdemg-dev", "value": "mdemg-dev"}`
- [ ] Dashboard dropdown link present
- [ ] All panels have explicit `datasource` with UID
- [ ] All stat panels have `"noValue"` in `fieldConfig.defaults`
- [ ] State indicator panels have `null+nan` special value mapping
- [ ] JSON validates without errors
- [ ] Playwright e2e tests pass: `pytest tests/e2e/grafana/ -v --browser chromium`

### Metric Scoping Reference (RSIC Example)

| Scope | Metrics | Variable Filter |
|-------|---------|-----------------|
| Per-space | health sub-scores, watchdog decay/escalation, synergy | `{space_id=~"$space_id"}` |
| Instance-wide | cycle counters, action counters, trigger rejections, calibration, snapshots | No space_id filter |

Label row titles with scope: `"Cycles (Instance-Wide)"` vs `"Cognitive Health"` (per-space) to help operators understand what the Space ID variable affects.
