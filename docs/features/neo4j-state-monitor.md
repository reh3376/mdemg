# Neo4j State Monitor & Space Overview

Phase 76 adds a single `GET /v1/neo4j/overview` endpoint that aggregates database health, per-space summaries, and backup status. Operators no longer need to call 4+ endpoints to understand system state.

## How It Works

### Batched Cypher Queries

The handler executes 6 batched queries instead of N*4 per-space queries:

1. **Nodes per space by layer** — groups all MemoryNodes by `space_id` and `layer`
2. **Edges per space** — counts `CO_ACTIVATED_WITH` edges where both endpoints share a `space_id`
3. **Observation counts** — counts nodes with `role_type='conversation_observation'` per space
4. **Orphan counts** — counts nodes with no edges per space
5. **Consolidation timestamps** — `max(created_at)` for nodes with `layer > 0` per space
6. **Ingest timestamps** — `max(created_at)` for codebase nodes per space

### Health Score

Per-space health is computed as:

- **Orphan ratio** (60% weight): `(1 - orphans/total_nodes) * 0.6`
- **Edge density** (40% weight): `min(edges/total_nodes, 1.0) * 0.4`

Score range: 0.0 (unhealthy) to 1.0 (healthy).

### Staleness Detection

A space is flagged as stale when:

- It has more than 10 conversation observations, AND
- Its last consolidation was more than 7 days ago

### Graceful Degradation

If individual Cypher queries fail, the database status is set to `"degraded"` but the response still returns with whatever data succeeded. This ensures partial visibility even during Neo4j issues.

## Usage

```bash
curl -s http://localhost:9999/v1/neo4j/overview | jq
```

Filter to a specific space:

```bash
curl -s http://localhost:9999/v1/neo4j/overview | jq '.spaces[] | select(.space_id == "mdemg-dev")'
```

Quick health check:

```bash
curl -s http://localhost:9999/v1/neo4j/overview | jq '{status: .database.status, spaces: .database.total_spaces, nodes: .database.total_nodes}'
```

## Related Files

| File | Description |
|------|-------------|
| `internal/models/models.go` | Neo4jOverviewResponse, DatabaseOverview, SpaceOverview, BackupOverview, BackupSummary types |
| `internal/api/handlers.go` | handleNeo4jOverview handler |
| `internal/api/server.go` | Route registration |
| `docs/api/api-spec/uats/specs/neo4j_overview.uats.json` | UATS contract test |

## Dependencies

- Neo4j database (for all Cypher queries)
- `backup.Service` (optional — backup section is empty if backup is disabled)
