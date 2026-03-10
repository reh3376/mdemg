# Phase VSX: VS Code Extension + Real-Time Memory Sidebar

**Status**: Planned
**Date**: 2026-03-10
**Scope**: VS Code extension only (Cursor extension removed from scope)

---

## Overview

A VS Code extension for MDEMG providing real-time memory visualization, inline observation, and semantic recall search. Communicates directly with the MDEMG REST API at `localhost:9999`.

---

## Phase Breakdown

### VSX-1: Core Extension Scaffold + Connection Status

**Effort**: M (3-4 days) | **Dependencies**: None | **Status**: Not started

**Goal**: Independently shippable extension that detects MDEMG server and shows connection status in the status bar.

**API Endpoints**:
- `GET /healthz` — server liveness check

**File Structure**:
```
vscode-extension/
  package.json              -- Extension manifest
  tsconfig.json             -- TypeScript config
  .vscodeignore             -- Exclude dev files from .vsix
  .eslintrc.json            -- Linting config
  src/
    extension.ts            -- activate()/deactivate() entry point
    mdemgClient.ts          -- HTTP client wrapper (base URL, timeout, error handling)
    connectionMonitor.ts    -- Polling health check, EventEmitter for status changes
    statusBar.ts            -- Status bar item (green/red/yellow + "MDEMG: Connected")
    config.ts               -- VS Code settings adapter
  test/
    suite/
      mdemgClient.test.ts
      connectionMonitor.test.ts
```

**Technical Decisions**:
- **Language**: TypeScript
- **HTTP Client**: Node.js native `fetch` (available in Node 18+, shipped with VS Code)
- **Bundling**: esbuild (fast, standard for VS Code extensions)
- **VS Code Engine**: `^1.85.0`
- **Activation**: `onStartupFinished` — checks for `.mdemg/` directory or `.mdemg.port` file
- **Polling**: 5-second interval for health checks (configurable)

**Configuration Settings** (`contributes.configuration`):
```json
{
  "mdemg.serverUrl": { "type": "string", "default": "http://localhost:9999" },
  "mdemg.defaultSpaceId": { "type": "string", "default": "" },
  "mdemg.autoRefreshInterval": { "type": "number", "default": 10 },
  "mdemg.autoDetect": { "type": "boolean", "default": true }
}
```

**Status Bar Behavior**:
- Green circle + "MDEMG: Connected (v0.2.0)" when healthy
- Red circle + "MDEMG: Disconnected" when unreachable
- Yellow circle + "MDEMG: Connecting..." during initial check
- Click opens MDEMG sidebar or quick-pick with actions

**Acceptance Criteria**:
- Extension activates without errors
- Status bar shows correct connection state
- Auto-detects server URL from `.mdemg.port` file
- Settings respected (custom URL, interval)
- No impact on VS Code startup time

---

### VSX-2: Memory Sidebar — TreeView with Stats

**Effort**: L (5-6 days) | **Dependencies**: VSX-1 | **Status**: Not started

**Goal**: TreeView sidebar showing real-time memory state, space freshness, RSIC health, and learning phase.

**API Endpoints**:
- `GET /v1/memory/stats?space_id=X` — node counts by layer, embedding coverage, health score
- `GET /v1/memory/distribution?space_id=X` — learning phase, edge count, score distribution
- `GET /v1/memory/spaces/{space_id}/freshness` — staleness info
- `GET /v1/self-improve/health` — RSIC status
- `GET /v1/learning/stats?space_id=X` — Hebbian edge stats
- `GET /v1/learning/freeze/status?space_id=X` — learning freeze state

**New Files**:
```
src/
  sidebar/
    memoryTreeProvider.ts     -- TreeDataProvider for sidebar view
    treeItems.ts              -- TreeItem subclasses
    refreshTimer.ts           -- Auto-refresh coordinator
  types/
    api.ts                    -- TypeScript interfaces for API responses
```

**Key Decision — TreeView vs WebView**: Use **TreeView** (not WebView). Rationale:
- Native VS Code UI — consistent theming, keyboard nav, context menus
- Lower complexity (no HTML/CSS, no message passing)
- WebView reserved for Phase VSX-5 where custom rendering is needed

**TreeView Structure**:
```
MDEMG Memory                          [refresh icon]
  Connection
    Status: Connected (v0.2.0)        [green dot]
    Server: localhost:9999
  Space: mdemg-dev
    Freshness
      Last Ingest: 2h ago             [green/yellow/red]
      Ingest Count: 127
      Stale: No
    Memory Stats
      Total Nodes: 34,416
      L0 Observations: 28,000
      L1 Themes: 4,200
      L2 Concepts: 1,800
      L3+ Emergent: 416
      Embedding Coverage: 98.2%
      Health Score: 0.87
    Learning
      Phase: warm (15,230 edges)
      Avg Weight: 0.42
      Freeze: Unfrozen
      Decay Rate: 0.01/day
    Temporal Activity
      Last 24h: 45 observations
      Last 7d: 312 observations
      Last 30d: 1,240 observations
  RSIC Health
    Status: OK
    Active Tasks: 2
    Watchdog: Active
    Orchestration: Running
```

**Auto-Refresh**: Timer every N seconds (default 10), only when sidebar visible, rate-limited.

---

### VSX-3: Inline Observation from Editor

**Effort**: M (3-4 days) | **Dependencies**: VSX-1 | **Status**: Not started

**Goal**: Select text → right-click → observe as learning/decision/correction/preference.

**API Endpoints**:
- `POST /v1/conversation/observe` — store observation

**New Files**:
```
src/
  commands/
    observeCommand.ts         -- Command handler
    observeQuickPick.ts       -- Quick pick UI for obs_type
```

**Commands**:
- `mdemg.observeSelection` — generic (shows quick pick for type)
- `mdemg.observeAsLearning` — direct
- `mdemg.observeAsDecision` — direct
- `mdemg.observeAsCorrection` — direct
- `mdemg.observeAsPreference` — direct

**Context Menu**: Submenu under editor context menu, only visible when `editorHasSelection && mdemg.connected`.

**Observation Payload**:
```json
{
  "space_id": "<from config>",
  "session_id": "vscode-<workspace-name>",
  "content": "<selected text>",
  "obs_type": "<user choice>",
  "metadata": {"source": "vscode-extension", "file": "path/to/file.ts", "lines": "42-58"}
}
```

---

### VSX-4: Recall Search Panel

**Effort**: L (5-6 days) | **Dependencies**: VSX-1, VSX-2 | **Status**: Not started

**Goal**: Search memory from sidebar, display semantic search results.

**API Endpoints**:
- `POST /v1/conversation/recall` — semantic search
- `POST /v1/memory/retrieve` — general memory retrieval

**New Files**:
```
src/
  commands/
    recallCommand.ts          -- Command handler for "MDEMG: Search Memory"
  sidebar/
    recallTreeProvider.ts     -- TreeDataProvider for results
    recallResultItem.ts       -- TreeItem for individual results
```

**UX**: Input box (`vscode.window.showInputBox`) → results as expandable tree items showing score, content preview, layer, tags, created date. Click opens full content. Keyboard shortcut: `Ctrl+Shift+M`.

**Result Format**:
```
Search Results (5)
  [0.92] "Config priority chain: defaults > yaml > env"    [L0]
    Content: "The config resolution order is..."
    Tags: [golang, config]
    Created: 2026-03-01
  [0.85] "Neo4j volume naming convention"                   [L0]
```

---

### VSX-5: Memory Distribution Visualization (WebView)

**Effort**: L (5-7 days) | **Dependencies**: VSX-2 | **Status**: Not started

**Goal**: WebView panel with charts showing memory distribution, learning health, score trends, temporal activity.

**API Endpoints**:
- `GET /v1/memory/stats?space_id=X`
- `GET /v1/memory/distribution?space_id=X&history_limit=50`
- `GET /v1/learning/stats?space_id=X`

**New Files**:
```
src/
  webview/
    dashboardPanel.ts         -- WebviewPanel manager
    dashboardMessage.ts       -- Message types
  media/
    dashboard.html            -- WebView HTML template
    dashboard.css             -- Dark/light theme aware styles
    dashboard.js              -- Chart rendering
```

**Charting**: Chart.js (MIT, 65KB minified, vendored in `media/`). Supports bar, line, doughnut charts.

**Dashboard Layout**:
```
+------------------------------------------+
| MDEMG Memory Dashboard       [refresh]   |
+------------------------------------------+
| Connection: Connected | Space: mdemg-dev |
+------------------------------------------+
|  Memory by Layer      |  Learning Health  |
|  [bar chart]          |  [gauge/ring]     |
+------------------------------------------+
|  Score Distribution Trend (last 50)      |
|  [line chart: mean, p10, p90]            |
+------------------------------------------+
|  Temporal Activity                        |
|  [bar chart: 24h / 7d / 30d]            |
+------------------------------------------+
```

**Communication**: Extension host fetches API data, pushes to WebView via `postMessage()`. WebView never calls HTTP directly. Theme support via VS Code CSS variables.

---

## Phase Dependency Graph

```
VSX-1 (Scaffold + Connection)
  |
  +---> VSX-2 (Sidebar TreeView)
  |       |
  |       +---> VSX-5 (Dashboard WebView)
  |
  +---> VSX-3 (Inline Observe)
  |
  +---> VSX-4 (Recall Search)
```

VSX-2, VSX-3, VSX-4 can develop in parallel after VSX-1.

---

## Architectural Constraints

1. **No CORS needed for extension host**: VS Code extensions run in Node.js, not browser. HTTP to `localhost:9999` works without CORS. WebView panels (VSX-5) proxy through extension host.

2. **Port Discovery**: Read `.mdemg.port` file (written by `mdemg serve`), fall back to `mdemg.serverUrl` setting. Mirrors `config.ResolveEndpoint()`.

3. **Session ID Strategy**: `"vscode-{workspace-basename}"` — workspace-level isolation, deterministic across restarts.

4. **Space ID Resolution**: Check `.mdemg/config.yaml` in workspace root → parse `default_space_id` → fall back to workspace folder name → allow override via settings.

5. **Rate Limiting**: Auto-refresh timer (10s default) keeps request rate at ~6-8 per cycle across all endpoints. Batch sensibly.

---

## Server-Side Changes

**VSX-1 through VSX-4**: No server-side changes required. All endpoints exist.

**Future Enhancement (post VSX-5)**: Consider dedicated SSE endpoint for push notifications:
```
GET /v1/events/stream?space_id=X
```
Eliminates polling. NOT a blocker for any phase.

---

## Package & Distribution

- **Dev**: `npx vsce package` → `.vsix` → `code --install-extension mdemg-*.vsix`
- **Release**: Publish to VS Code Marketplace under publisher `reh3376`, or distribute via GitHub Releases
- **Versioning**: Independent from MDEMG server version. Include `mdemg.minServerVersion` check.

---

## Estimated Total Effort

| Phase | Effort | Cumulative |
|-------|--------|------------|
| VSX-1 | M (3-4 days) | 3-4 days |
| VSX-2 | L (5-6 days) | 8-10 days |
| VSX-3 | M (3-4 days) | 11-14 days |
| VSX-4 | L (5-6 days) | 16-20 days |
| VSX-5 | L (5-7 days) | 21-27 days |

---

## Critical Reference Files

- `internal/api/server.go:1200-1374` — Complete route registration (authoritative API reference)
- `internal/models/models.go` — All API request/response types → TypeScript interfaces
- `internal/cli/mcp.go` — Reference implementation for MDEMG API client pattern
- `internal/config/yaml_config.go` — YAML config parser (`.mdemg/config.yaml` format)

---

## Documents Accessed

- `internal/api/server.go` — route registrations, CORS config, SSE endpoints
- `internal/cli/serve.go` — MCP server integration, port file writing
- `internal/cli/mcp.go` — MCP tool implementations, HTTP client pattern
- `internal/models/models.go` — API type definitions
- `internal/config/yaml_config.go` — YAML config parsing
- `docs/specs/phase96-ide-repo-integration.md` — existing IDE integration spec
- `docs/features/ide-repo-integration.md` — existing IDE features doc
