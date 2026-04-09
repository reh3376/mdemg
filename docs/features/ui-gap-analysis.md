---
created: 2026-04-09
author: reh3376
status: active
sprint: "UI-AUDIT-2026-04-09"
---

# Browser Dashboard UI — API Gap Analysis

## Summary

The browser dashboard at `/ui/` exposes 48 API wrappers (in `api.js`) out of 125 server routes registered in `server.go` — **38% coverage**.

This document catalogs which API endpoints have UI representation and which do not, to inform future UI development.

## Well-Covered Areas (100%)

| Area | api.js Wrappers | Routes | Coverage |
|------|----------------|--------|----------|
| Backup/Restore | 7 | 7 | 100% |
| Config Management | 2 | 2 | 100% |
| Features | 4 | 4 | 100% |
| Plugins | 6 | 6 | 100% |
| RSIC Lifecycle | 3 | 3 | 100% |
| Training Data | 3 | 3 | 100% |

## Partially Covered

| Area | Covered | Uncovered |
|------|---------|-----------|
| Learning | freeze/unfreeze/prune/stats | `/v1/learning/negative-feedback` |
| Memory | stats, distribution, export/import | 20+ operations (retrieve, reflect, ingest, cache, graph, cleanup) |
| Status | healthz, readyz | embedding health only via api.js |
| Self-Improve | health, history, calibration, cycle | assess, report, signals |

## Entirely Uncovered API Areas

### Jiminy Guidance (15+ routes)
- `/v1/jiminy/healthz`, `/v1/jiminy/ready`
- `/v1/jiminy/guide`, `/v1/jiminy/warm`, `/v1/jiminy/latest`
- `/v1/jiminy/feedback`, `/v1/jiminy/evaluate`
- `/v1/jiminy/checkpoint`, `/v1/jiminy/resume-protocol`, `/v1/jiminy/bootstrap`
- `/v1/jiminy/protocol/metrics`, `/v1/jiminy/protocol/tier-effectiveness`
- `/v1/jiminy/protocol/feedback`, `/v1/jiminy/protocol/learn`
- `/v1/jiminy/extension`

### Conversation Management (13+ routes)
- `/v1/conversation/observe`, `/v1/conversation/correct`
- `/v1/conversation/resume`, `/v1/conversation/recall`
- `/v1/conversation/consolidate`, `/v1/conversation/graduate`
- `/v1/conversation/volatile/stats`
- `/v1/conversation/session/health`, `/v1/conversation/session/anomalies`
- `/v1/conversation/templates`, `/v1/conversation/snapshot`
- `/v1/conversation/org-reviews`

### Memory Operations (25+ routes)
- Retrieval: `/v1/memory/retrieve`, `/v1/memory/reflect`
- Nodes: `/v1/memory/node/meta`, `/v1/memory/nodes/`, `/v1/memory/frontiers`
- Semantic: `/v1/memory/consult`, `/v1/memory/suggest`, `/v1/memory/symbols`
- Cache: `/v1/memory/cache/stats`, `/v1/memory/cache`
- Graph: `/v1/memory/graph/topology`, `/v1/memory/graph/neighborhood`
- Ingestion: `/v1/memory/ingest/trigger`, `/v1/memory/ingest/status/`, `/v1/memory/ingest/cancel/`, `/v1/memory/ingest/jobs`, `/v1/memory/ingest/files`
- Edges: `/v1/memory/edges/stale/refresh`
- Cleanup: `/v1/memory/cleanup/schedule`, `/v1/memory/cleanup/schedules`, `/v1/memory/cleanup/stats`
- Other: `/v1/memory/freshness`, `/v1/memory/meta-learn`, `/v1/memory/spaces/`, `/v1/memory/ingest-codebase`

### Constraints & Neural (6+ routes)
- `/v1/constraints`, `/v1/constraints/stats`, `/v1/constraints/effectiveness`
- `/v1/constraints/detect-conflicts`, `/v1/constraints/conflicts`
- `/v1/neural/status`

### Metrics & Analytics (4+ routes)
- `/v1/metrics`, `/v1/metrics/snapshot`
- `/v1/prometheus`, `/v1/metrics/trends`

### Infrastructure (10+ routes)
- File watcher: `/v1/filewatcher/start`, `/v1/filewatcher/status`, `/v1/filewatcher/stop`
- Hash verification: 7 routes (`register`, `files`, `verify-all`, `verify`, `update`, `revert`, `scan`)
- Webhooks: `/v1/webhooks/linear`, `/v1/webhooks/`, `/v1/alerts/grafana`
- Jobs: `/v1/jobs/`
- Synergy: `/v1/synergy/status`

### Other Uncovered
- Modules: `/v1/modules`, `/v1/modules/`
- APE: `/v1/ape/status`, `/v1/ape/trigger`
- Skills: `/v1/skills`, `/v1/skills/`
- Symbols: `/v1/symbols/relationships`, `/v1/symbols/`
- Linear: `/v1/linear/issues`, `/v1/linear/projects`, `/v1/linear/comments`
- Scraper: `/v1/scraper/jobs`, `/v1/scraper/spaces`
- Guardrail: `/v1/guardrail/events`
- Feedback: `/v1/feedback`
- Capability gaps: `/v1/system/capability-gaps`, `/v1/system/gap-interviews`

## Recommendations

Potential new UI tabs (ordered by impact):
1. **Jiminy** — guidance health, follow rate, protocol metrics (15 routes)
2. **Ingestion** — trigger, monitor jobs, view files, cancel (7 routes)
3. **Conversation** — session health, anomalies, snapshots, templates (13 routes)
4. **Metrics** — trends, determinism, Prometheus preview (4 routes)
5. **Constraints** — listing, effectiveness, conflict detection (6 routes)

Lower priority (mostly infrastructure/debugging):
6. Graph topology viewer (already in Grafana)
7. File watcher controls
8. Hash verification management
9. Linear integration dashboard

## Documents Accessed

- `/Users/reh3376/mdemg/internal/api/ui/api.js` — 48 API wrapper functions
- `/Users/reh3376/mdemg/internal/api/server.go` — 125 route registrations
- `/Users/reh3376/mdemg/internal/api/ui/tabs/*.js` — 10 tab source files
