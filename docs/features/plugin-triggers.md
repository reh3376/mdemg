---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "9.4"
---

# Plugin-Specific Triggers

## Summary

**Feature**: Plugin-Specific Triggers
**Summary**: Plugin-specific trigger mechanisms including Linear webhook integration, file watcher REST API management, and event-driven module updates for non-APE modules.

## Vision & Goals

MDEMG's plugin system needs to react to external events — code changes on disk, Linear ticket updates, webhook payloads. Plugin triggers provide the input side of the plugin lifecycle: how data enters the system from external sources. This complements the plugin SDK (Phase 49) which provides the processing side.

## Current State

### Architecture

Three trigger components:

**9.4.1 Linear Webhook Integration** — Full webhook handler with HMAC-SHA256 verification, 10s debouncing, gRPC dispatch to the Linear module, batch ingest, and APE event triggering. Processes `Issue.created`, `Issue.updated`, `Project.updated` events.

**9.4.2 File Watcher REST API** — Runtime management of file watchers via REST endpoints. The core watcher provides Manager, debouncing, extension filtering, and exclude patterns. On file changes, calls `handleFileWatcherChange` which ingests files and triggers `source_changed` APE event.

**9.4.3 Event-Driven Module Updates** — Extends event dispatch beyond APE modules so that INGESTION and CRUD modules can subscribe to events via manifest `event_subscriptions`.

### Workflow

**Event Flow:**

```
source_changed event fired
    |
    +---> APE Scheduler (existing) -> APE modules with matching EventTriggers
    |
    +---> EventDispatcher (new) -> Non-APE modules with matching EventSubscriptions
         +---> INGESTION modules: calls IngestionClient.Parse(event metadata)
         +---> CRUD modules: logged (no OnEvent RPC yet)
```

**Module Subscription** via manifest:

```json
{
  "capabilities": {
    "ingestion_sources": ["linear://"],
    "event_subscriptions": ["source_changed"]
  }
}
```

Supported: specific event names (`"source_changed"`, `"ingest_complete"`) or wildcard `"*"`.

### Configuration

Watchers can be configured at startup via env vars or managed at runtime via REST API.

## Notes

### Known Limitations

- CRUD modules receive events but have no OnEvent RPC yet (logged only)
- File watcher debounce is per-watcher, not per-file

### Risks & Gaps

None identified.

### Future Improvements

- OnEvent gRPC RPC for CRUD modules
- Per-file debouncing in file watcher

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| POST | `/v1/webhooks/linear` | Linear webhook handler with HMAC-SHA256 verification | N/A |
| POST | `/v1/filewatcher/start` | Start file watcher for a space | `specs/filewatcher_start.uats.json` |
| GET | `/v1/filewatcher/status` | List active file watchers | `specs/filewatcher_status.uats.json` |
| POST | `/v1/filewatcher/stop` | Stop file watcher for a space | `specs/filewatcher_stop.uats.json` |

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg watch` | Start file watcher via CLI |

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `LINEAR_WEBHOOK_SECRET` | - | HMAC-SHA256 secret for Linear webhook verification |
| `LINEAR_WEBHOOK_SPACE_ID` | - | Target space for Linear webhook events |
| `FILE_WATCHER_ENABLED` | `false` | Enable file watcher at startup |
| `FILE_WATCHER_CONFIGS` | - | Startup watcher configs: `space_id:/path:ext1\|ext2:debounce_ms,...` |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Plugin System (Phase 9) | Requires — triggers dispatch to plugin modules |
| APE Scheduler | Requires — `source_changed` events trigger APE cycle |
| Ingest Pipeline | Feeds into — file changes and webhooks trigger ingestion |

## Related Files

- `internal/api/handlers_filewatcher.go` - 3 REST handlers (start, status, stop)
- `internal/api/handle_webhooks.go` - Linear webhook handler
- `internal/plugins/events.go` - EventDispatcher for non-APE module routing
- `internal/plugins/events_test.go` - Unit tests for EventDispatcher
- `internal/plugins/types.go` - EventSubscriptions field on Capabilities
- `internal/filewatcher/watcher.go` - Core file watcher
- `internal/api/server.go` - Routes + eventDispatcher wiring
