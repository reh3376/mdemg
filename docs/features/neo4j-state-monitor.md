---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "76"
---

# Neo4j State Monitor & Space Overview

## Summary

**Feature**: Neo4j State Monitor
**Summary**: A single `GET /v1/neo4j/overview` endpoint that aggregates database health, per-space summaries, and backup status. Operators no longer need to call 4+ endpoints to understand system state.

## Vision & Goals

Observability is critical for a self-improving memory system. The state monitor provides a single-pane view of Neo4j health, space utilization, and staleness — feeding directly into RSIC health assessments and Grafana dashboards. It replaces fragmented per-space queries with batched Cypher execution for efficiency.

## Current State

### Architecture

The handler executes 6 batched queries instead of N*4 per-space queries:

1. **Nodes per space by layer** — groups all MemoryNodes by `space_id` and `layer`
2. **Edges per space** — counts `CO_ACTIVATED_WITH` edges where both endpoints share a `space_id`
3. **Observation counts** — counts nodes with `role_type='conversation_observation'` per space
4. **Orphan counts** — counts nodes with no edges per space
5. **Consolidation timestamps** — `max(created_at)` for nodes with `layer > 0` per space
6. **Ingest timestamps** — `max(created_at)` for codebase nodes per space

### Workflow

**Health Score** — Per-space health is computed as:

- **Orphan ratio** (60% weight): `(1 - orphans/total_nodes) * 0.6`
- **Edge density** (40% weight): `min(edges/total_nodes, 1.0) * 0.4`

Score range: 0.0 (unhealthy) to 1.0 (healthy).

**Staleness Detection** — A space is flagged as stale when:

- It has more than 10 conversation observations, AND
- Its last consolidation was more than 7 days ago

**Graceful Degradation** — If individual Cypher queries fail, the database status is set to `"degraded"` but the response still returns with whatever data succeeded. This ensures partial visibility even during Neo4j issues.

### Configuration

No configuration required — the endpoint is always available when the server is running.

## Notes

### Known Limitations

- Health score formula uses fixed weights (60/40 orphan/edge split) — not configurable
- Staleness threshold (7 days, 10 observations) is hardcoded

### Risks & Gaps

None identified.

### Future Improvements

- Configurable health score weights and staleness thresholds
- Historical health trend tracking via TSDB

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| GET | `/v1/neo4j/overview` | Aggregated database health, per-space summaries, backup status | `specs/neo4j_overview.uats.json` |

## CLI Commands

None — API-only endpoint.

## Configuration Reference

None — no configurable parameters.

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Neo4j Database | Requires — all Cypher queries execute against Neo4j |
| Backup Service | Enhances — backup section included if backup enabled |
| RSIC Health Assessment | Feeds into — health scores used by RSIC assessor |

## Related Files

- `internal/models/models.go` - Neo4jOverviewResponse, DatabaseOverview, SpaceOverview, BackupOverview types
- `internal/api/handlers.go` - handleNeo4jOverview handler
- `internal/api/server.go` - Route registration
- `docs/api/api-spec/uats/specs/neo4j_overview.uats.json` - UATS contract test
