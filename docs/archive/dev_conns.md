# MDEMG Development Connections

## Neo4j Database

| Setting | Value |
|---------|-------|
| Browser | <http://localhost:7474> |
| Bolt URI | bolt://localhost:7687 |
| Username | neo4j |
| Password | testpassword |
| Container | mdemg-neo4j |

### Cypher Shell Access

```bash
docker exec -it mdemg-neo4j cypher-shell -u neo4j -p testpassword
```

### Quick Queries

```bash
# Check schema version
docker exec mdemg-neo4j cypher-shell -u neo4j -p testpassword "MATCH (s:SchemaMeta {key:'schema'}) RETURN s.current_version"

# Show indexes
docker exec mdemg-neo4j cypher-shell -u neo4j -p testpassword "SHOW INDEXES"

# Show constraints
docker exec mdemg-neo4j cypher-shell -u neo4j -p testpassword "SHOW CONSTRAINTS"
```

## Go Service (MDEMG API)

| Setting | Value |
|---------|-------|
| Base URL | <http://localhost:8082> |
| Health Check | GET /healthz |
| Readiness | GET /readyz |
| Retrieve | POST /v1/memory/retrieve |
| Ingest | POST /v1/memory/ingest |

### Start Service

```bash
cd mdemg_build/service
source .env
go run ./cmd/server
```

### Test Endpoints

```bash
# Health check
curl -s http://localhost:8082/healthz | jq

# Readiness (checks Neo4j + schema version)
curl -s http://localhost:8082/readyz | jq

# Retrieve memories
curl -s http://localhost:8082/v1/memory/retrieve \
  -H 'content-type: application/json' \
  -d '{"space_id":"demo","query_embedding":[0.0,0.1,0.2],"candidate_k":50,"top_k":10,"hop_depth":2}' | jq
```

## Environment Variables

Located in `mdemg_build/service/.env`:

| Variable | Value |
|----------|-------|
| NEO4J_URI | bolt://localhost:7687 |
| NEO4J_USER | neo4j |
| NEO4J_PASS | testpassword |
| REQUIRED_SCHEMA_VERSION | 4 |
| VECTOR_INDEX_NAME | memNodeEmbedding |
| LISTEN_ADDR | :8082 |
| EMBEDDING_PROVIDER | openai (or ollama) |
| EMBEDDING_CACHE_ENABLED | true |

## Docker Commands

```bash
# Start Neo4j
docker compose up -d

# Stop Neo4j
docker compose down

# View logs
docker logs -f mdemg-neo4j

# Restart Neo4j
docker compose restart neo4j
```
