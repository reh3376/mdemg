# Plugin-Specific Triggers (Phase 9.4)

Phase 9.4 adds plugin-specific trigger mechanisms: Linear webhook integration, file watcher REST API management, and event-driven module updates for non-APE modules.

## Components

### 9.4.1 Linear Webhook Integration (Pre-existing)

Full webhook handler with HMAC-SHA256 verification, 10s debouncing, gRPC dispatch to the Linear module, batch ingest, and APE event triggering.

- **Handler**: `internal/api/handle_webhooks.go`
- **Route**: `POST /v1/webhooks/linear`
- **Config**: `LinearWebhookSecret`, `LinearWebhookSpaceID` env vars
- Processes `Issue.created`, `Issue.updated`, `Project.updated` events
- Dispatches to `linear-module` via `IngestionClient.Parse` gRPC

### 9.4.2 File Watcher REST API

Runtime management of file watchers via REST endpoints. The core watcher (`internal/filewatcher/watcher.go`) provides Manager, debouncing, extension filtering, and exclude patterns. The new handlers expose this at runtime.

#### Endpoints

**Start Watcher** — `POST /v1/filewatcher/start`

```json
{
  "space_id": "my-project",
  "path": "/path/to/repo",
  "extensions": [".go", ".ts", ".py"],
  "excludes": ["node_modules", ".git"],
  "debounce_ms": 500
}
```

Response: `{ "space_id": "...", "path": "/abs/path", "status": "watching" }`

- Validates: space_id required, path required + must exist + must be directory
- Resolves path to absolute
- Replaces any existing watcher for the space
- Defaults: extensions from `filewatcher.DefaultExtensions`, excludes from `filewatcher.DefaultExcludes`, debounce 500ms
- On file changes: calls `handleFileWatcherChange` which ingests files and triggers `source_changed` APE event

**List Watchers** — `GET /v1/filewatcher/status`

Response: `{ "watchers": { "space-id": { "path": "...", "debounce_ms": 500, "extensions": [...] } }, "count": 1 }`

**Stop Watcher** — `POST /v1/filewatcher/stop`

```json
{ "space_id": "my-project" }
```

Response: `{ "space_id": "...", "status": "stopped" }`

#### Config-Based Startup

Watchers can also be configured at startup via env vars:

- `FILE_WATCHER_ENABLED=true`
- `FILE_WATCHER_CONFIGS=space_id:/path:ext1|ext2:debounce_ms,...`

### 9.4.3 Event-Driven Module Updates

Extends event dispatch beyond APE modules so that INGESTION and CRUD modules can subscribe to events.

#### Manifest Subscription

Modules declare `event_subscriptions` in their manifest capabilities:

```json
{
  "capabilities": {
    "ingestion_sources": ["linear://"],
    "event_subscriptions": ["source_changed"]
  }
}
```

Supported values:

- Specific event names: `"source_changed"`, `"ingest_complete"`, etc.
- Wildcard: `"*"` matches all events

#### Event Flow

```
source_changed event fired
    |
    ├─→ APE Scheduler (existing) → APE modules with matching EventTriggers
    |
    └─→ EventDispatcher (new) → Non-APE modules with matching EventSubscriptions
         ├─→ INGESTION modules: calls IngestionClient.Parse(event metadata)
         └─→ CRUD modules: logged (no OnEvent RPC yet)
```

#### Implementation

- **Types**: `EventSubscriptions []string` added to `Capabilities` in `internal/plugins/types.go`
- **Dispatcher**: `internal/plugins/events.go` — `EventDispatcher` struct with `DispatchEvent(event, ctx)`
- **Server wiring**: `TriggerAPEEventWithContext` now calls both `apeScheduler.TriggerEventWithContext` and `eventDispatcher.DispatchEvent`
- **Tests**: `internal/plugins/events_test.go`

## Files

| File | Description |
|------|-------------|
| `internal/api/handlers_filewatcher.go` | 3 REST handlers (start, status, stop) |
| `internal/api/server.go` | Routes + eventDispatcher field + wiring |
| `internal/plugins/types.go` | EventSubscriptions field on Capabilities |
| `internal/plugins/events.go` | EventDispatcher for non-APE module routing |
| `internal/plugins/events_test.go` | Unit tests for EventDispatcher |
| `internal/filewatcher/watcher.go` | Core file watcher (pre-existing) |
| `internal/api/handle_webhooks.go` | Linear webhook handler (pre-existing) |

## UATS Specs

- `filewatcher_start.uats.json` — 3 variants (valid start, missing space_id, invalid path)
- `filewatcher_status.uats.json` — 1 variant (list watchers)
- `filewatcher_stop.uats.json` — 2 variants (valid stop, missing space_id)
