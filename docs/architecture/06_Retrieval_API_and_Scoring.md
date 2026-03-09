# Retrieval API + Scoring + Explainability

## API Endpoints Overview

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/v1/memory/retrieve` | POST | Query memories with vector search + graph expansion |
| `/v1/memory/ingest` | POST | Ingest a single observation |
| `/v1/memory/ingest/batch` | POST | Bulk ingest up to 100 observations |
| `/v1/memory/ingest/trigger` | POST | Start a background re-ingestion job |
| `/v1/memory/ingest/status/{job_id}` | GET | Check ingestion job progress |
| `/v1/memory/ingest/cancel/{job_id}` | POST | Cancel a running ingestion job |
| `/v1/memory/ingest/jobs` | GET | List all ingestion jobs |
| `/v1/memory/symbols` | GET | Search code symbols (functions, classes, constants) |
| `/v1/memory/reflect` | POST | Deep context exploration (3-stage traversal) |
| `/v1/metrics` | GET | Graph health metrics and statistics |
| `/v1/system/pool-metrics` | GET | Neo4j connection pool and runtime metrics |
| `/healthz` | GET | Liveness probe |
| `/readyz` | GET | Readiness probe with system status |

---

## 1) Retrieve Endpoint

`POST /v1/memory/retrieve`

### Request Body

```json
{
  "space_id": "ide-agent",
  "query_text": "How does authentication work?",
  "query_embedding": [0.1, 0.2, ...],
  "top_k": 20,
  "candidate_k": 200,
  "hop_depth": 2,
  "layer_range": [0, 3],
  "policy_context": {"roles": ["developer"]}
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `space_id` | Yes | - | Memory space identifier |
| `query_text` | One of query_text/query_embedding | - | Natural language query (auto-embedded) |
| `query_embedding` | One of query_text/query_embedding | - | Pre-computed embedding (1536 dims default) |
| `top_k` | No | 20 | Final results to return |
| `candidate_k` | No | 200 | Vector recall candidates |
| `hop_depth` | No | 2 | Graph expansion depth |
| `layer_range` | No | - | Filter by abstraction layer |
| `policy_context` | No | - | Access control context |
| `temporal_after` | No | - | ISO 8601 timestamp: force hard filter after this time |
| `temporal_before` | No | - | ISO 8601 timestamp: force hard filter before this time |

### Response

```json
{
  "data": {
    "results": [
      {
        "node_id": "abc-123",
        "path": "/concepts/auth",
        "name": "Authentication System",
        "summary": "OAuth2 implementation...",
        "score": 0.85,
        "vector_sim": 0.92,
        "activation": 0.78
      }
    ]
  }
}
```

---

## 2) Ingest Endpoint

`POST /v1/memory/ingest`

### Request Body

```json
{
  "space_id": "ide-agent",
  "timestamp": "2026-01-16T12:00:00Z",
  "source": "cursor",
  "content": "Description of the observation...",
  "path": "/projects/myapp/auth",
  "name": "Auth Implementation",
  "tags": ["authentication", "security"],
  "sensitivity": "internal",
  "confidence": 0.9
}
```

### Response

```json
{
  "data": {
    "node_id": "abc-123",
    "obs_id": "obs-456",
    "created_edges": [
      {"target": "def-789", "type": "ASSOCIATED_WITH", "weight": 0.85}
    ],
    "anomalies": []
  }
}
```

**Features:**

- Auto-generates embedding if not provided
- Creates semantic ASSOCIATED_WITH edges to similar nodes (configurable via `SEMANTIC_EDGE_*`)
- Runs non-blocking anomaly detection (duplicate/stale checks)

---

## 3) Batch Ingest Endpoint

`POST /v1/memory/ingest/batch`

Bulk ingest up to 100 observations in a single request.

### Request Body

```json
{
  "space_id": "ide-agent",
  "observations": [
    {
      "timestamp": "2026-01-16T12:00:00Z",
      "source": "import",
      "content": "First observation...",
      "name": "Item 1",
      "tags": ["batch"]
    },
    {
      "timestamp": "2026-01-16T12:00:01Z",
      "source": "import",
      "content": "Second observation...",
      "name": "Item 2"
    }
  ]
}
```

### Response (HTTP 200 or 207 for partial success)

```json
{
  "data": {
    "space_id": "ide-agent",
    "total": 2,
    "succeeded": 2,
    "failed": 0,
    "results": [
      {"index": 0, "status": "success", "node_id": "abc-123", "obs_id": "obs-1"},
      {"index": 1, "status": "success", "node_id": "def-456", "obs_id": "obs-2"}
    ]
  }
}
```

**Configuration:**

- `BATCH_INGEST_MAX_ITEMS=100` - Maximum observations per request

---

## 4) Symbol Search Endpoint

`GET /v1/memory/symbols`

Search for code symbols extracted during codebase ingestion.

### Query Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `space_id` | Yes | - | Memory space to search |
| `name` | No | - | Symbol name pattern (supports `*` wildcard for prefix matching) |
| `type` | No | - | Filter by type: `const`, `var`, `function`, `class`, `method`, `interface`, `enum` |
| `file` | No | - | Filter by file path (partial match) |
| `exported` | No | - | Filter by exported status (`true`/`false`) |
| `q` | No | - | Fulltext search across all symbol metadata |
| `limit` | No | 50 | Maximum results (max 500) |

### Response

```json
{
  "symbols": [
    {
      "name": "HandleLogin",
      "type": "function",
      "file": "internal/auth/handlers.go",
      "line": 42,
      "exported": true,
      "signature": "func HandleLogin(w http.ResponseWriter, r *http.Request)"
    }
  ],
  "total": 1
}
```

**Use Case:** Find specific constants, function signatures, or type definitions with file:line evidence for grounded retrieval.

---

## 5) Background Ingestion Endpoints

### Trigger Ingestion Job

`POST /v1/memory/ingest/trigger`

Start a background re-ingestion job (non-blocking).

### Request Body

```json
{
  "space_id": "my-project",
  "path": "/path/to/repo",
  "incremental": true,
  "consolidate": true,
  "extract_symbols": true
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `space_id` | Yes | - | Target memory space |
| `path` | Yes | - | Path to source directory |
| `incremental` | No | false | Only ingest changed files |
| `consolidate` | No | false | Run consolidation after ingestion |
| `extract_symbols` | No | true | Extract code symbols |

### Response

```json
{
  "job_id": "abc-123-def",
  "status": "pending"
}
```

### Check Job Status

`GET /v1/memory/ingest/status/{job_id}`

### Response

```json
{
  "job_id": "abc-123-def",
  "status": "running",
  "progress": {
    "current": 45,
    "total": 120,
    "percentage": 37.5,
    "rate": 2.3
  }
}
```

**Job States:** `pending` → `running` → `completed` | `failed` | `cancelled`

### Cancel Job

`POST /v1/memory/ingest/cancel/{job_id}`

### Response

```json
{
  "job_id": "abc-123-def",
  "status": "cancelled"
}
```

### List All Jobs

`GET /v1/memory/ingest/jobs`

### Response

```json
{
  "jobs": [
    {
      "job_id": "abc-123-def",
      "type": "ingest",
      "status": "completed",
      "created_at": "2026-01-27T10:00:00Z",
      "progress": {
        "current": 120,
        "total": 120,
        "percentage": 100
      }
    }
  ]
}
```

**Use Case:** User-triggered re-ingestion without blocking the API, with status polling for progress updates.

---

## 6) Pool Metrics Endpoint

`GET /v1/system/pool-metrics`

Returns Neo4j connection pool statistics and Go runtime metrics.

### Response

```json
{
  "connection_pool": {
    "active_connections": 5,
    "idle_connections": 10,
    "total_acquired": 1250
  },
  "runtime": {
    "goroutines": 42,
    "heap_alloc_mb": 128.5,
    "num_gc": 15
  }
}
```

**Configuration:**

```
NEO4J_MAX_POOL_SIZE=100           # Max connections (default: 100)
NEO4J_ACQUIRE_TIMEOUT_SEC=60      # Connection acquire timeout (default: 60)
NEO4J_MAX_CONN_LIFETIME_SEC=3600  # Max connection lifetime (default: 3600)
NEO4J_CONN_IDLE_TIMEOUT_SEC=0     # Idle timeout, 0=disabled (default: 0)
```

**Use Case:** Monitor and tune database connection behavior for production workloads.

---

## 8) Reflect Endpoint

`POST /v1/memory/reflect`

Deep context exploration using 3-stage traversal for comprehensive topic understanding.

### Request Body

```json
{
  "space_id": "ide-agent",
  "topic": "authentication patterns",
  "topic_embedding": [0.1, 0.2, ...],
  "max_depth": 3,
  "max_nodes": 50
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `space_id` | Yes | - | Memory space identifier |
| `topic` | Yes | - | Natural language topic to explore |
| `topic_embedding` | No | - | Pre-computed embedding (auto-generated if not provided) |
| `max_depth` | No | 3 | Maximum traversal depth |
| `max_nodes` | No | 50 | Cap on returned nodes |

### Response

```json
{
  "data": {
    "topic": "authentication patterns",
    "stages": {
      "seed": {"count": 10, "duration_ms": 45},
      "expand": {"count": 35, "duration_ms": 120},
      "abstract": {"count": 8, "duration_ms": 30}
    },
    "nodes": [
      {
        "node_id": "abc-123",
        "path": "/concepts/oauth",
        "name": "OAuth2 Pattern",
        "layer": 1,
        "stage": "seed",
        "score": 0.92
      }
    ],
    "insights": {
      "clusters": [
        {"theme": "Token Management", "node_ids": ["abc", "def", "ghi"]}
      ],
      "patterns": ["Frequent OAuth + JWT co-occurrence"],
      "knowledge_gaps": ["No memories about refresh token rotation"]
    },
    "total_nodes": 53,
    "duration_ms": 195
  }
}
```

### 3-Stage Traversal

1. **SEED** - Vector search for topic-related memories
2. **EXPAND** - Lateral traversal via CO_ACTIVATED_WITH and ASSOCIATED_WITH edges
3. **ABSTRACT** - Upward traversal via ABSTRACTS_TO edges to find generalizations

---

## 9) Metrics Endpoint

`GET /v1/metrics?space_id=ide-agent`

Returns graph health metrics and statistics.

### Query Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `space_id` | No | Filter metrics to specific space (global if omitted) |

### Response

```json
{
  "data": {
    "total_nodes": 1250,
    "total_edges": 3400,
    "nodes_by_layer": {
      "0": 1100,
      "1": 120,
      "2": 25,
      "3": 5
    },
    "edges_by_type": {
      "ASSOCIATED_WITH": 1500,
      "CO_ACTIVATED_WITH": 1200,
      "HAS_OBSERVATION": 500,
      "ABSTRACTS_TO": 200
    },
    "hub_nodes": [
      {"node_id": "abc", "name": "Common Pattern", "degree": 45}
    ],
    "orphan_count": 12,
    "avg_edge_weight": 0.34,
    "recent_activity": {
      "nodes_created_24h": 15,
      "edges_created_24h": 42
    }
  }
}
```

---

## 10) Health Check Endpoints

### Liveness Probe

`GET /healthz`

```json
{
  "data": {
    "status": "ok"
  }
}
```

### Readiness Probe

`GET /readyz`

```json
{
  "data": {
    "status": "ready",
    "neo4j": "connected",
    "schema_version": 4,
    "embedding_provider": "openai:text-embedding-3-small+cache"
  }
}
```

---

## Candidate Generation (Vector + Graph)

### Vector Recall

1. Embed query_text (or use provided query_embedding)
2. Query vector index on `:MemoryNode.embedding`

```cypher
WITH $queryEmbedding AS q
CALL db.index.vector.queryNodes('memNodeEmbedding', $candidateK, q)
YIELD node, score
WHERE node.space_id = $spaceId
RETURN node.node_id AS node_id, score
ORDER BY score DESC;
```

### Graph Expansion

For each candidate, fetch neighborhood edges within allowed types and within hop_depth.

**Recommendations:**

- Restrict to useful edge types (avoid CONTAINS exploding unless filtered)
- Apply degree caps (ignore nodes with out-degree > N unless structural anchors)
- Configurable via `MAX_NEIGHBORS_PER_NODE`, `MAX_TOTAL_EDGES_FETCHED`

---

## Activation Pass

Compute activation over fetched subgraph in-memory (see `04_Activation_and_Learning.md`).

---

## Final Ranking (Scoring Formula)

```
score = α*vector_sim + β*activation + γ_eff*recency + δ*confidence - κ*redundancy - φ*hub_penalty
```

Where `γ_eff` depends on temporal mode:

- **none** (default): `γ_eff = γ` — no change to scoring
- **soft** ("recent changes to auth"): `γ_eff = γ × TEMPORAL_SOFT_BOOST` (default 3.0×)
- **hard** ("in the last 7 days"): candidates filtered by time range before scoring

**Default Hyperparameters:**

| Symbol | Name | Default | Description |
|--------|------|---------|-------------|
| α | vector weight | 0.55 | Vector similarity contribution |
| β | activation weight | 0.30 | Activation score contribution |
| γ | recency weight | 0.10 | Recency boost |
| δ | confidence weight | 0.05 | Confidence contribution |
| φ | hub penalty | 0.08 | Penalty for high-degree nodes |
| κ | redundancy penalty | 0.12 | Penalty for path-prefix duplicates |
| ρ | recency decay | 0.05 | Decay rate per day |

**Penalties:**

- `hub_penalty`: proportional to log(degree) to avoid generic nodes dominating
- `redundancy`: penalizes near-duplicates (same abstraction parent, same path prefix)

### Temporal Retrieval

The retrieval pipeline detects temporal intent from natural language queries:

| Temporal Mode | Detection | Pipeline Effect |
|---------------|-----------|-----------------|
| `none` | No temporal language | Scoring formula unchanged |
| `soft` | "recent", "latest", "what's new" | `γ_eff = γ × 3.0` (configurable) |
| `hard` | "in the last N days", "since DATE" | Candidates filtered to `[after, before)` range |

**Pipeline integration points:**

1. `ComputeRetrievalHints()` — parses temporal intent from query text
2. After BM25 fusion — hard-mode filter removes out-of-range candidates
3. `ScoreAndRankWithBreakdown()` — soft-mode multiplies gamma for recency boost

**API override:** Pass `temporal_after` / `temporal_before` (ISO 8601) to force hard-mode filtering regardless of query text.

**Cache behavior:** Temporal queries (`soft` or `hard`) skip the query cache.

**Debug output** includes: `temporal_mode`, `temporal_keywords`, `temporal_confidence`, `temporal_constraint`

---

## Explainability

Each result includes:

- `vector_sim` - Raw cosine similarity to query
- `activation` - Computed activation score
- `temporal_boost` - Additional score from temporal recency boost (0 when mode=none)
- `top_incoming_contributors[]` - List of (src_node_id, relType, w_eff, src_activation)

For detailed explanations, return:

- Top contributing paths: (seed → ... → result) with edge weights and types
- Evidence counts and timestamps for edges used
- Temporal boost contribution when applicable

---

## Learning Updates After Retrieval

For final top-K nodes:

1. Update/create `CO_ACTIVATED_WITH` edges among nodes above activation threshold
2. Increment evidence counts, update timestamps
3. Optional: strengthen `ABSTRACTS_TO` edges if consistent

**Constraints:**

- Learning updates bounded per request (`LEARNING_EDGE_CAP_PER_REQUEST=200`)
- Hebbian formula: `Δw = η * a_i * a_j - μ * w_ij`
