# MDEMG API Reference

Complete HTTP API reference for the Multi-Dimensional Emergent Memory Graph (MDEMG) server. All endpoints use JSON request/response bodies unless otherwise noted.

The MDEMG HTTP API is identical on all platforms (macOS, Linux, Windows). Only the shell syntax for `curl` examples differs. See [Platform-Specific Notes](#platform-specific-notes) at the end of this document for Windows PowerShell and `cmd.exe` equivalents.

---

## Table of Contents

1. [Base URL & Authentication](#base-url--authentication)
2. [Health & Readiness](#health--readiness)
3. [Memory Operations](#memory-operations)
4. [Learning Edges](#learning-edges)
5. [Constraints](#constraints)
6. [Constraint Effectiveness](#constraint-effectiveness)
7. [Constraint Conflicts](#constraint-conflicts)
8. [Constraint Scope](#constraint-scope)
9. [Skills Registry](#skills-registry)
10. [Conversation Memory](#conversation-memory)
11. [Templates](#templates)
12. [Snapshots](#snapshots)
13. [Org Reviews](#org-reviews)
14. [Meta-Learning](#meta-learning)
15. [Guardrail Validation](#guardrail-validation)
16. [Guardrail Events](#guardrail-events)
17. [Jiminy Inner-Voice](#jiminy-inner-voice)
18. [J17 AI-to-AI Protocol](#j17-ai-to-ai-protocol)
19. [Synergy Optimization](#synergy-optimization)
20. [Spaces & Freshness](#spaces--freshness)
21. [Jobs (SSE)](#jobs-sse)
22. [Codebase Ingestion API](#codebase-ingestion-api)
23. [Ingestion Pipeline API](#ingestion-pipeline-api)
24. [Scraper API](#scraper-api)
25. [Linear Integration API](#linear-integration-api)
26. [Webhooks](#webhooks)
27. [File Watcher API](#file-watcher-api)
28. [Admin](#admin)
29. [Space Transfer (Export/Import)](#space-transfer-exportimport)
30. [Self-Improvement (RSIC) API](#self-improvement-rsic-api)
31. [Backup & Restore](#backup--restore)
32. [Symbols & Relationships](#symbols--relationships)
33. [Cleanup](#cleanup)
34. [Edge Consistency](#edge-consistency)
35. [Metrics & Monitoring](#metrics--monitoring)
36. [Determinism Metrics](#determinism-metrics)
37. [Neural Sidecar](#neural-sidecar)
38. [Hash Verification (UNTS)](#hash-verification-unts)
39. [Plugins & Modules](#plugins--modules)
40. [System](#system)
41. [Event Graph Federation](#event-graph-federation)
41. [Training Data Export](#training-data-export)
42. [Dashboard / Visualization (internal)](#dashboard--visualization-internal)
43. [MCP Server Tools](#mcp-server-tools)
44. [Common Status Codes](#common-status-codes)
45. [Common Headers](#common-headers)
46. [Protected Spaces](#protected-spaces)
43. [Platform-Specific Notes](#platform-specific-notes)

---

## Base URL & Authentication

**Base URL:** `http://localhost:<MDEMG_PORT>`

> **Dynamic Ports:** MDEMG uses dynamically assigned ports. Your actual port is in `.env` (`MDEMG_PORT`), the `.mdemg.port` file, or via `mdemg status`. All `curl` examples in this document use `localhost:9999` as a placeholder — replace with your assigned port.

**Authentication** (optional, controlled by `AUTH_ENABLED` env var):

When enabled, requests must include one of:
- **API Key mode:** `Authorization: Bearer <api-key>` header
- **JWT mode:** `Authorization: Bearer <jwt-token>` header

Authentication mode is set via `AUTH_MODE` (values: `api_key`, `jwt`).

Endpoints `/healthz`, `/readyz`, and `/v1/metrics` are exempt from authentication by default.

**Rate Limiting** (optional, controlled by `RATE_LIMIT_ENABLED` env var):
- Configurable via `RATE_LIMIT_RPS` (requests/sec) and `RATE_LIMIT_BURST`
- `/healthz`, `/readyz`, `/v1/metrics` are exempt

**CORS** (optional, controlled by `CORS_ENABLED` env var):
- Configurable allowed origins, methods, and headers

---

## Health & Readiness

### GET /healthz

Liveness probe with lightweight subsystem checks. Returns immediately if server is running.

**Response (healthy):**
```json
{
  "status": "ok",
  "version": "0.6.0",
  "commit": "abc1234",
  "checks": {
    "neo4j": "ok",
    "circuit_breakers": "ok",
    "tsdb": "ok",
    "jiminy": "ok"
  }
}
```

**Response (degraded):**
```json
{
  "status": "degraded",
  "version": "0.6.0",
  "commit": "abc1234",
  "checks": {
    "neo4j": "no_driver",
    "circuit_breakers": "open:2",
    "tsdb": "ok",
    "jiminy": "not_initialized"
  }
}
```

The `checks` map reports subsystem status. Possible values:
- `neo4j`: `"ok"` or `"no_driver"` (nil check only — no live DB query)
- `circuit_breakers`: `"ok"` or `"open:N"` (count of open circuits)
- `tsdb`: `"ok"` or `"no_client"` (only when `TSDB_ENABLED=true`)
- `jiminy`: `"ok"`, `"not_initialized"`, or omitted (only when `JIMINY_ENABLED=true`)

**Status Codes:** `200 OK` (always returns 200, even when degraded — this is a liveness endpoint)

```bash
curl -s http://localhost:9999/healthz | jq .
```

---

### GET /readyz

Readiness probe. Runs live checks against Neo4j (schema version), embeddings, plugins, circuit breakers, conversation service (CMS ping), Jiminy, TimescaleDB (schema version), and the neural sidecar.

**Response:**
```json
{
  "status": "ready",
  "version": "v0.11.0",
  "checks": {
    "neo4j":        { "status": "healthy", "message": "schema version 26", "latency": "12ms" },
    "embeddings":   { "status": "healthy", "message": "openai (1536 dimensions)" },
    "plugins":      { "status": "healthy", "message": "2/2 modules active" },
    "circuit_breakers": { "status": "healthy", "message": "8 circuits monitored" },
    "conversation": { "status": "healthy", "message": "CMS available" },
    "jiminy":       { "status": "healthy", "message": "enabled, synthesis=on" },
    "timescaledb":  { "status": "healthy", "message": "schema version 31", "latency": "4ms" },
    "neural_sidecar": { "status": "healthy", "message": "J17 sidecar available", "latency": "6ms" }
  }
}
```

`status` is `ready`, `degraded` (any check degraded — still `200`), or `not_ready` (`503`). Each check has `status` (`healthy`/`degraded`/`unhealthy`) plus optional `message` and `latency`.

**Status Codes:** `200 OK` (ready or degraded), `503 Service Unavailable` (not_ready)

```bash
curl -s http://localhost:9999/readyz
```

---

### GET /v1/embedding/health

Check embedding provider health.

**Response:**
```json
{
  "provider": "openai",
  "status": "healthy",
  "dimensions": 1536
}
```

**Status Codes:** `200 OK`, `503 Service Unavailable`

```bash
curl -s http://localhost:9999/v1/embedding/health
```

---

## Memory Operations

### POST /v1/memory/ingest

Ingest a single memory observation into the graph.

**Request Body:**
```json
{
  "space_id": "my-project",          // required: namespace for isolation
  "timestamp": "2026-01-15T10:00:00Z", // required: when this knowledge was captured
  "source": "code-analysis",          // required: origin identifier (max 64 chars)
  "content": "...",                    // required: the knowledge content (string or object)
  "tags": ["api", "config"],           // optional: filtering tags
  "node_id": "custom-id",             // optional: custom node ID (auto-generated if omitted)
  "path": "src/main.go",              // optional: file path (max 512 chars)
  "name": "Main Configuration",       // optional: display name
  "summary": "Brief summary...",       // optional: for reranking (max 1000 chars)
  "sensitivity": "internal",          // optional: public | internal | confidential
  "confidence": 0.95,                 // optional: 0.0-1.0
  "embedding": [0.1, 0.2, ...],       // optional: pre-computed embedding vector
  "canonical_time": "2026-01-15T10:00:00Z", // optional: content-relevant time
  "timestamp_format": "rfc3339"        // optional: rfc3339 | unix | unix_ms | date_only
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "node_id": "abc123",
  "obs_id": "obs-abc123",
  "embedding_dims": 1536,
  "anomalies": [
    {
      "type": "duplicate",
      "severity": "warning",
      "message": "Very similar node exists",
      "related_node": "xyz789",
      "confidence": 0.92
    }
  ]
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `500 Internal Server Error`

```bash
curl -s -X POST http://localhost:9999/v1/memory/ingest \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","timestamp":"2026-01-15T10:00:00Z","source":"manual","content":"Example data"}'
```

---

### POST /v1/memory/ingest/batch

Batch ingest multiple observations in a single request (max 2000 items).

**Request Body:**
```json
{
  "space_id": "my-project",
  "observations": [
    {
      "timestamp": "2026-01-15T10:00:00Z",
      "source": "code-analysis",
      "content": "...",
      "tags": ["api"],
      "path": "src/main.go",
      "name": "Function A",
      "summary": "Brief summary",
      "symbols": [
        {
          "name": "MAX_TIMEOUT",
          "type": "const",
          "value": "60000",
          "line": 42,
          "exported": true
        }
      ]
    }
  ]
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "total_items": 5,
  "success_count": 5,
  "error_count": 0,
  "results": [
    { "index": 0, "status": "success", "node_id": "abc123", "obs_id": "obs-abc123" }
  ]
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `500 Internal Server Error`

```bash
curl -s -X POST http://localhost:9999/v1/memory/ingest/batch \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","observations":[{"timestamp":"2026-01-15T10:00:00Z","source":"manual","content":"Item 1"}]}'
```

---

### POST /v1/memory/retrieve

Retrieve relevant memories via vector similarity + graph activation spreading.

**Request Body:**
```json
{
  "space_id": "my-project",                // required
  "query_text": "How does authentication work?", // required (or query_embedding)
  "query_embedding": [0.1, ...],            // alternative to query_text
  "candidate_k": 50,                       // optional: candidates for reranking (1-1000)
  "top_k": 10,                             // optional: final results (1-100)
  "hop_depth": 2,                          // optional: graph traversal depth (0-5)
  "jiminy_enabled": true,                  // optional: enable explainable retrieval
  "include_evidence": true,                // optional: include symbol evidence
  "include_extensions": ["go", "py"],      // optional: filter by file extension
  "exclude_extensions": ["md"],            // optional: exclude file extensions
  "code_only": false,                      // optional: exclude non-code files
  "temporal_after": "2026-01-01T00:00:00Z",  // optional: hard temporal filter
  "temporal_before": "2026-02-01T00:00:00Z", // optional: hard temporal filter
  "translate_intent": true,                // optional: LLM query rewriting (Phase 102)
  "session_id": "my-session-001",         // optional: propagated to TSDB for session-level training data analysis
  "include_global_space": true,            // optional: include mdemg-global space (Phase 105)
  "cursor": "node-id-123",                // optional: cursor pagination
  "limit": 50,                            // optional: max results per page (max 500)
  "policy_context": {}                     // optional: policy context for retrieval
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "results": [
    {
      "node_id": "abc123",
      "path": "src/auth/handler.go",
      "name": "AuthHandler",
      "summary": "JWT authentication handler",
      "layer": 0,
      "score": 0.89,
      "normalized_confidence": 95.0,
      "confidence_level": "HIGH",
      "vector_sim": 0.85,
      "activation": 0.92,
      "jiminy": {
        "rationale": "High vector similarity with graph reinforcement",
        "confidence": 0.89,
        "retrieval_path": ["vector_recall", "activation_spread", "rerank"],
        "score_breakdown": { "vector": 0.6, "graph": 0.2, "rerank": 0.2 }
      },
      "evidence": [
        {
          "symbol_name": "AuthHandler",
          "symbol_type": "function",
          "file_path": "src/auth/handler.go",
          "line": 15
        }
      ]
    }
  ],
  "evidence_metrics": {
    "total_results": 10,
    "results_with_evidence": 8,
    "total_symbols": 24,
    "compliance_rate": 0.80,
    "avg_symbols_per_result": 2.4
  },
  "next_cursor": "node-id-456",
  "has_more": true,
  "translated_intent": "rewritten query text",
  "debug": {}
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `500 Internal Server Error`

```bash
curl -s -X POST http://localhost:9999/v1/memory/retrieve \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","query_text":"How does authentication work?","top_k":5}'
```

---

### POST /v1/memory/reflect

Deep context exploration via multi-hop graph traversal from a topic seed.

**Request Body:**
```json
{
  "space_id": "my-project",
  "topic": "error handling patterns",     // required (or topic_embedding)
  "topic_embedding": [0.1, ...],          // alternative to topic
  "max_depth": 3,                         // optional: hop depth (1-10, default: 3)
  "max_nodes": 50                         // optional: result cap (1-500, default: 50)
}
```

**Response (200):**
```json
{
  "topic": "error handling patterns",
  "core_memories": [
    { "node_id": "abc", "name": "ErrorHandler", "path": "src/errors.go", "layer": 0, "score": 0.9, "distance": 0 }
  ],
  "related_concepts": [
    { "node_id": "def", "name": "Retry Logic", "layer": 1, "score": 0.7, "distance": 2 }
  ],
  "abstractions": [
    { "node_id": "ghi", "name": "Resilience Pattern", "layer": 2, "score": 0.6, "distance": 3 }
  ],
  "insights": [
    { "type": "pattern", "description": "Consistent retry-with-backoff pattern across services", "node_ids": ["abc","def"] }
  ],
  "graph_context": {
    "nodes_explored": 120,
    "edges_traversed": 250,
    "max_layer_reached": 3
  }
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `500 Internal Server Error`

```bash
curl -s -X POST http://localhost:9999/v1/memory/reflect \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","topic":"error handling","max_depth":2}'
```

---

### POST /v1/memory/consult

Agent Consulting Service (SME). Provides contextual suggestions based on accumulated knowledge.

**Request Body:**
```json
{
  "space_id": "my-project",
  "context": "I'm implementing a new REST endpoint...",  // required (max 10000 chars)
  "question": "What patterns should I follow?",          // required (max 2000 chars)
  "tags": ["api"],                                        // optional
  "max_suggestions": 5,                                   // optional (1-20)
  "include_evidence": true,                               // optional
  "llm_synthesis": true,                                  // optional: enable LLM narrative (Phase 101)
  "translate_intent": true,                               // optional: query rewriting (Phase 102)
  "session_id": "my-session-001"                          // optional: propagated to TSDB for session-level analysis
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "suggestions": [
    {
      "type": "context",
      "content": "This codebase uses handler pattern with validation middleware...",
      "confidence": 0.85,
      "source_nodes": ["abc123"],
      "evidence": []
    }
  ],
  "related_concepts": [
    { "node_id": "def456", "name": "API Design Patterns", "layer": 2, "relevance": 0.8 }
  ],
  "confidence": 0.82,
  "rationale": "Based on 12 matching patterns in the graph",
  "synthesis": "LLM-generated narrative summary...",
  "translated_intent": "rewritten query",
  "debug": {}
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `500 Internal Server Error`

```bash
curl -s -X POST http://localhost:9999/v1/memory/consult \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","context":"Building a new API","question":"What patterns to follow?"}'
```

---

### POST /v1/memory/suggest

Context-triggered proactive suggestions. Surfaces relevant knowledge without explicit questions.

**Request Body:**
```json
{
  "space_id": "my-project",
  "context": "func handleAuth(w http.ResponseWriter...",  // required (max 20000 chars)
  "file_path": "src/auth.go",                            // optional
  "tags": ["auth"],                                       // optional
  "max_suggestions": 5,                                   // optional (1-20)
  "min_confidence": 0.5,                                  // optional (0.0-1.0)
  "include_evidence": true,                               // optional
  "include_conflicts": true,                              // optional
  "include_constraints": true,                            // optional
  "translate_intent": true                                // optional
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "triggers": [
    { "trigger_type": "pattern_match", "matched": "authentication", "keywords": ["auth","jwt"] }
  ],
  "suggestions": [],
  "conflicts": [
    { "severity": "medium", "description": "...", "conflicts_with": "node-id", "source_nodes": [] }
  ],
  "constraints": [
    { "name": "JWT Required", "description": "All API endpoints must use JWT", "constraint_type": "must", "confidence": 0.9, "source_nodes": [] }
  ],
  "related_concepts": [],
  "confidence": 0.75,
  "debug": {}
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `500 Internal Server Error`

```bash
curl -s -X POST http://localhost:9999/v1/memory/suggest \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","context":"func handleAuth(w http.ResponseWriter..."}'
```

---

### GET /v1/memory/stats?space_id=X

Per-space memory statistics including health indicators.

**Query Parameters:**
- `space_id` (required): space identifier

**Response (200):**
```json
{
  "space_id": "my-project",
  "memory_count": 1500,
  "observation_count": 800,
  "memories_by_layer": { "0": 1200, "1": 200, "2": 80, "3": 20 },
  "embedding_coverage": 0.95,
  "avg_embedding_dimensions": 1536,
  "learning_activity": {
    "co_activated_edges": 5000,
    "avg_weight": 0.45,
    "max_weight": 0.98
  },
  "temporal_distribution": {
    "last_24h": 50,
    "last_7d": 200,
    "last_30d": 600
  },
  "connectivity": {
    "avg_degree": 3.5,
    "max_degree": 42,
    "orphan_count": 15
  },
  "health_score": 0.87,
  "computed_at": "2026-01-15T10:00:00Z"
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `500 Internal Server Error`

```bash
curl -s "http://localhost:9999/v1/memory/stats?space_id=demo"
```

---

### GET /v1/memory/node/meta?space_id=X&path=Y

Content-hash metadata for a single memory node. Used by hooks and CLI to detect whether a file has changed before re-ingesting.

**Query Parameters:**
- `space_id` (required): space identifier
- `path` (required): file path of the memory node

**Response (200):**
```json
{
  "node_id": "abc123",
  "path": "CLAUDE.md",
  "content_hash": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "file_size": 4200,
  "line_count": 85,
  "last_ingested_at": "2026-03-25T14:30:00Z",
  "status": "active"
}
```

**Response (404):** Node with the given `space_id` + `path` has never been ingested.

**Status Codes:** `200 OK`, `400 Bad Request`, `404 Not Found`, `500 Internal Server Error`

```bash
curl -s "http://localhost:9999/v1/memory/node/meta?space_id=mdemg-dev&path=CLAUDE.md"
```

---

### GET /v1/memory/distribution?space_id=X

Learning edge distribution statistics including learning phase.

**Query Parameters:**
- `space_id` (required): space identifier

**Response (200):**
```json
{
  "stats": {
    "phase": "warm",
    "edge_count": 15000,
    "alerts": []
  }
}
```

Learning phases: `cold` (0) -> `learning` (1-10k) -> `warm` (10k-50k) -> `saturated` (50k+)

```bash
curl -s "http://localhost:9999/v1/memory/distribution?space_id=demo" | jq '{phase: .stats.phase, edges: .stats.edge_count}'
```

---

### POST /v1/memory/consolidate

Trigger hidden layer creation (DBSCAN clustering + message passing).

**Request Body:**
```json
{
  "space_id": "my-project",
  "skip_clustering": false,    // optional: skip DBSCAN, only message passing
  "skip_forward": false,       // optional: skip forward pass
  "skip_backward": false       // optional: skip backward pass
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "status": "completed",
  "clusters_created": 12,
  "nodes_processed": 500
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `500 Internal Server Error`

```bash
curl -s -X POST http://localhost:9999/v1/memory/consolidate \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo"}'
```

---

### POST /v1/memory/nodes/{node_id}/archive

Archive a memory node (soft delete).

**Request Body:**
```json
{
  "reason": "outdated information"  // optional
}
```

**Response (200):**
```json
{
  "node_id": "abc123",
  "name": "Old Config",
  "archived_at": "2026-01-15T10:00:00Z",
  "reason": "outdated information"
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `403 Forbidden` (protected space), `404 Not Found`

```bash
curl -s -X POST http://localhost:9999/v1/memory/nodes/abc123/archive \
  -H "Content-Type: application/json" \
  -d '{"reason":"outdated"}'
```

---

### POST /v1/memory/nodes/{node_id}/unarchive

Restore an archived node.

**Response (200):**
```json
{
  "node_id": "abc123",
  "name": "Old Config",
  "unarchived_at": "2026-01-15T10:00:00Z"
}
```

**Status Codes:** `200 OK`, `404 Not Found`

```bash
curl -s -X POST http://localhost:9999/v1/memory/nodes/abc123/unarchive
```

---

### DELETE /v1/memory/nodes/{node_id}

Permanently delete a node and its relationships.

**Response (200):**
```json
{
  "node_id": "abc123",
  "deleted_nodes": 1,
  "deleted_edges": 5
}
```

**Status Codes:** `200 OK`, `403 Forbidden` (protected space), `404 Not Found`

```bash
curl -s -X DELETE http://localhost:9999/v1/memory/nodes/abc123
```

---

### POST /v1/memory/archive/bulk

Bulk archive multiple nodes.

**Request Body:**
```json
{
  "space_id": "my-project",
  "node_ids": ["abc123", "def456"],
  "reason": "batch cleanup"           // optional
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "total_items": 2,
  "success_count": 2,
  "error_count": 0,
  "results": [
    { "node_id": "abc123", "status": "success", "archived_at": "2026-01-15T10:00:00Z" }
  ]
}
```

**Status Codes:** `200 OK`, `400 Bad Request`

---

### GET /v1/memory/symbols?space_id=X&query=Y

Search for code symbols in the graph.

**Query Parameters:**
- `space_id` (required)
- `query` (required): symbol name or pattern

**Status Codes:** `200 OK`, `400 Bad Request`

```bash
curl -s "http://localhost:9999/v1/memory/symbols?space_id=demo&query=handleAuth"
```

---

### GET /v1/memory/cache/stats

Return embedding and query cache statistics.

**Response (200):**
```json
{
  "query_cache": { "hits": 100, "misses": 20, "size": 50 },
  "embedding_cache": { "hits": 500, "misses": 50, "size": 200 }
}
```

```bash
curl -s http://localhost:9999/v1/memory/cache/stats
```

---

### DELETE /v1/memory/cache

Clear embedding and query caches.

**Status Codes:** `200 OK`

```bash
curl -s -X DELETE http://localhost:9999/v1/memory/cache
```

---

### GET /v1/memory/query/metrics

Return query performance metrics.

**Status Codes:** `200 OK`

```bash
curl -s http://localhost:9999/v1/memory/query/metrics
```

---

### GET /v1/memory/frontiers

Detect frontier nodes -- L3+ nodes with sufficient evidence but few outgoing edges, ready for expansion.

**Query Parameters:** `space_id` (required), `limit` (default 20, max 100).

```bash
curl -s "http://localhost:9999/v1/memory/frontiers?space_id=myproject&limit=10"
```

**Response (200):**
```json
{
  "frontiers": [
    {
      "node_id": "hidden-abc123",
      "name": "Authentication Patterns",
      "layer": 3,
      "summary": "Recurring auth middleware patterns",
      "outgoing_edges": 1,
      "evidence": 5
    }
  ],
  "count": 1
}
```

**Config:** `FRONTIER_MIN_EVIDENCE`, `FRONTIER_MAX_OUTGOING`.

### GET /v1/memory/graph/topology

Return the global graph topology (node counts per layer, edge counts per type) for a space. Used by the Status dashboard's topology panel.

**Query**: `space_id` (string, required), `limit` (int, default 1000 — caps node count returned).

**Response (200):**
```json
{
  "space_id": "myproject",
  "nodes_by_layer": {"L0": 12500, "L1": 240, "L2": 38, "L3": 7, "L5": 2},
  "edges_by_type": {"CO_ACTIVATED_WITH": 45000, "GENERALIZES": 280, "CONFLICTS_WITH": 18},
  "orphan_count": 12,
  "computed_at": "2026-05-21T17:00:00Z"
}
```

### GET /v1/memory/graph/neighborhood

Return the local neighborhood (1- or 2-hop) of a specific node. Used by the dashboard's graph-explorer for click-to-expand interactions.

**Query**: `space_id` (string, required), `node_id` (string, required), `hops` (int, default 1, max 3), `edge_types` (comma-separated, optional filter).

**Response (200):**
```json
{
  "center": {"node_id": "obs-abc123", "layer": 0, "name": "api/server.go"},
  "nodes": [{"node_id": "...", "layer": 1, "name": "..."}],
  "edges": [{"from": "obs-abc123", "to": "...", "type": "CO_ACTIVATED_WITH", "weight": 0.72}]
}
```

### GET /v1/memory/spaces

List all knowledge spaces visible to the running server with their high-level health stats. Returns active and tombstoned spaces; filter via `include_tombstoned=false` for active-only.

**Query**: `include_tombstoned` (bool, default true), `limit` (int, default 100).

**Response (200):**
```json
{
  "spaces": [
    {
      "space_id": "mdemg-dev",
      "node_count": 12780,
      "active": true,
      "last_observation_at": "2026-05-21T17:00:00Z",
      "protected": true
    }
  ],
  "count": 1
}
```

Note: `/v1/memory/spaces/{space_id}/...` sub-routes (e.g. `/freshness`) are documented in their own sections; this endpoint covers the root list.

---

## Learning Edges

### POST /v1/learning/freeze

Freeze Hebbian learning for a space (useful for stable scoring/benchmarks).

**Request Body:**
```json
{
  "space_id": "my-project",
  "reason": "stable scoring",
  "frozen_by": "claude"
}
```

**Status Codes:** `200 OK`, `400 Bad Request`

```bash
curl -s -X POST http://localhost:9999/v1/learning/freeze \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","reason":"stable scoring","frozen_by":"claude"}'
```

---

### POST /v1/learning/unfreeze

Unfreeze Hebbian learning for a space.

**Request Body:**
```json
{
  "space_id": "my-project"
}
```

**Status Codes:** `200 OK`, `400 Bad Request`

```bash
curl -s -X POST http://localhost:9999/v1/learning/unfreeze \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo"}'
```

---

### GET /v1/learning/freeze/status

Check freeze status for a space.

**Query Parameters:**
- `space_id` (required)

**Response (200):**
```json
{
  "space_id": "my-project",
  "frozen": true,
  "reason": "stable scoring",
  "frozen_by": "claude",
  "frozen_at": "2026-01-15T10:00:00Z"
}
```

```bash
curl -s "http://localhost:9999/v1/learning/freeze/status?space_id=demo"
```

---

### GET /v1/learning/stats

Learning edge statistics for a space.

**Query Parameters:**
- `space_id` (required)

```bash
curl -s "http://localhost:9999/v1/learning/stats?space_id=demo"
```

---

### POST /v1/learning/prune

Prune weak learning edges.

**Status Codes:** `200 OK`, `400 Bad Request`

---

### POST /v1/learning/negative-feedback

Weaken or contradict learning edges for rejected retrieval results.

**Request:**
```json
{
  "space_id": "myproject",
  "query_node_ids": ["node-1", "node-2"],
  "rejected_node_ids": ["node-3", "node-4"]
}
```

All fields required. `query_node_ids` and `rejected_node_ids` must be non-empty arrays.

**Response (200):** Stats about weakened/contradicted edges.

**Status Codes:** `200 OK`, `400 Bad Request` (empty arrays or missing space_id), `500 Internal Server Error`.

**Config:** `LEARNING_NEGATIVE_WEIGHT`, `LEARNING_NEGATIVE_MAX_PER_REQUEST`.

---

## Constraints

### GET /v1/constraints?space_id=X

List constraint nodes for a space (architectural constraints detected from observations).

**Query Parameters:**
- `space_id` (required)

**Response (200):**
```json
{
  "space_id": "my-project",
  "constraints": [
    {
      "node_id": "const-abc",
      "name": "JWT Required for API",
      "constraint_type": "must",
      "content": "All API endpoints must use JWT authentication",
      "confidence": 0.92,
      "created_at": "2026-01-15T10:00:00Z",
      "updated_at": "2026-01-15T10:00:00Z",
      "source_count": 5
    }
  ]
}
```

**Status Codes:** `200 OK`, `400 Bad Request`

```bash
curl -s "http://localhost:9999/v1/constraints?space_id=demo"
```

---

### GET /v1/constraints/stats?space_id=X

Constraint statistics grouped by type.

**Response (200):**
```json
{
  "space_id": "my-project",
  "total_constraint_nodes": 12,
  "by_type": [
    { "constraint_type": "must", "count": 5, "avg_confidence": 0.88 },
    { "constraint_type": "should", "count": 7, "avg_confidence": 0.72 }
  ],
  "tagged_observation_count": 45
}
```

```bash
curl -s "http://localhost:9999/v1/constraints/stats?space_id=demo"
```

---

## Constraint Effectiveness

### GET /v1/constraints/effectiveness?space_id=X

Per-constraint effectiveness metrics (follow/ignore/contradict rates, confidence trends).

Requires `JIMINY_PERSISTENCE_ENABLED=true`.

**Query Parameters:**
- `space_id` (required)

**Response (200):**
```json
{
  "space_id": "my-project",
  "constraints": [
    {
      "node_id": "const-abc",
      "total_surfaced": 25,
      "total_followed": 20,
      "total_ignored": 3,
      "total_contradicted": 2,
      "effectiveness_score": 0.80,
      "confidence": 0.88
    }
  ]
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `503 Service Unavailable`

```bash
curl -s "http://localhost:9999/v1/constraints/effectiveness?space_id=demo"
```

---

## Constraint Conflicts

### POST /v1/constraints/detect-conflicts

Trigger pairwise conflict scan across constraints in a space. Finds constraints with opposing types (`must` vs `must_not`) and high embedding similarity.

Requires `CONSTRAINT_CONFLICT_DETECTION_ENABLED=true`.

**Request Body:**
```json
{
  "space_id": "my-project"
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "conflicts_found": 2,
  "new_conflicts": 1
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `503 Service Unavailable`

```bash
curl -s -X POST http://localhost:9999/v1/constraints/detect-conflicts \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo"}'
```

---

### GET /v1/constraints/conflicts?space_id=X

List unresolved constraint conflicts for a space.

**Query Parameters:**
- `space_id` (required)

**Response (200):**
```json
{
  "space_id": "my-project",
  "conflicts": [
    {
      "source_id": "const-abc",
      "target_id": "const-def",
      "similarity_score": 0.85,
      "detection_method": "embedding_similarity",
      "resolution_status": "unresolved",
      "detected_at": "2026-03-19T10:00:00Z"
    }
  ]
}
```

**Status Codes:** `200 OK`, `400 Bad Request`

```bash
curl -s "http://localhost:9999/v1/constraints/conflicts?space_id=demo"
```

---

### PATCH /v1/constraints/conflicts/{id}/resolve

Resolve a constraint conflict with precedence.

**Request Body:**
```json
{
  "space_id": "my-project",
  "resolution_text": "JWT constraint takes precedence over optional auth"
}
```

**Response (200):**
```json
{
  "resolved": true
}
```

**Status Codes:** `200 OK`, `400 Bad Request`

```bash
curl -s -X PATCH http://localhost:9999/v1/constraints/conflicts/conflict-abc/resolve \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","resolution_text":"JWT constraint takes precedence over optional auth"}'
```

---

## Constraint Scope

### PATCH /v1/constraints/scope/{node_id}

Manually override the scope of a constraint (file path glob pattern limiting where it applies).

Requires `CONSTRAINT_SCOPE_FILTERING_ENABLED=true`.

**Request Body:**
```json
{
  "space_id": "my-project",
  "scope": "internal/api/**"
}
```

**Response (200):**
```json
{
  "updated": true,
  "node_id": "const-abc",
  "scope": "internal/api/**"
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `503 Service Unavailable`

```bash
curl -s -X PATCH http://localhost:9999/v1/constraints/scope/const-abc \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","scope":"internal/api/**"}'
```

---

## Skills Registry

Skills are stored as pinned CMS observations with `skill:<name>` tags.

### GET /v1/skills?space_id=X

List all registered skills for a space.

**Response (200):**
```json
{
  "space_id": "my-project",
  "skills": [
    {
      "name": "code-review",
      "description": "Instructions for performing code reviews...",
      "sections": ["setup", "checklist"],
      "observation_count": 3
    }
  ],
  "count": 1
}
```

```bash
curl -s "http://localhost:9999/v1/skills?space_id=mdemg-dev"
```

---

### POST /v1/skills/{name}/recall

Recall skill content by tag-based filtering on pinned observations.

**Request Body:**
```json
{
  "space_id": "mdemg-dev",
  "section": "checklist",    // optional: filter by section
  "query": "review steps",  // optional: search query (default: "skill {name} instructions")
  "top_k": 10               // optional: max results (default: 10)
}
```

**Response (200):**
```json
{
  "space_id": "mdemg-dev",
  "skill": "code-review",
  "section": "checklist",
  "query": "review steps",
  "results": [
    { "type": "conversation_observation", "node_id": "obs-abc", "content": "...", "score": 1.0, "layer": 0 }
  ],
  "debug": { "tag_filter": "skill:code-review:checklist", "observation_count": 1 }
}
```

```bash
curl -s -X POST http://localhost:9999/v1/skills/code-review/recall \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev"}'
```

---

### POST /v1/skills/{name}/register

Register skill sections as pinned observations.

**Request Body:**
```json
{
  "space_id": "mdemg-dev",
  "session_id": "skill-registry",  // optional (default: "skill-registry")
  "description": "Code review skill",
  "sections": [
    {
      "name": "checklist",
      "content": "1. Check error handling\n2. Review naming...",
      "tags": ["review"]
    }
  ]
}
```

**Response (200):**
```json
{
  "skill": "code-review",
  "space_id": "mdemg-dev",
  "sections_created": 1,
  "observation_ids": ["obs-abc123"]
}
```

```bash
curl -s -X POST http://localhost:9999/v1/skills/code-review/register \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","sections":[{"name":"setup","content":"Setup instructions..."}]}'
```

---

## Conversation Memory

CMS (Conversation Memory System) endpoints for AI session memory.

### POST /v1/conversation/observe

Capture a conversation observation with surprise detection.

**Request Body:**
```json
{
  "space_id": "mdemg-dev",             // required
  "session_id": "claude-core",          // required
  "content": "User prefers Go over Python for CLI tools", // required
  "obs_type": "preference",             // required: decision | learning | preference | correction | error | progress | task | technical_note | insight | context | blocker | context_signal | note | constraint | self_improvement
  "tags": ["language", "cli"],           // optional
  "metadata": {},                        // optional: arbitrary metadata
  "user_id": "user-123",                // optional: multi-user support
  "visibility": "private",              // optional: private | team | org
  "agent_id": "claude",                 // optional: which agent made this observation
  "refers_to": "obs-xyz",               // optional: reference to another observation
  "pinned": false                        // optional: pin for skill registry
}
```

**Response (200):**
```json
{
  "obs_id": "obs-abc123",
  "node_id": "node-abc123",
  "surprise_score": 0.73,
  "surprise_factors": { "novelty": 0.8, "contradiction": 0.1 },
  "summary": "LLM-generated summary of the observation",
  "detected_constraints": [
    { "constraint_type": "should", "name": "Use Go for CLI", "confidence": 0.7 }
  ]
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `503 Service Unavailable` (no embedder)

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","content":"Important learning","obs_type":"learning"}'
```

---

### POST /v1/conversation/correct

Capture an explicit user correction (creates a correction observation with high surprise).

**Request Body:**
```json
{
  "space_id": "mdemg-dev",
  "session_id": "claude-core",
  "incorrect": "The timeout is 30 seconds",     // required: what was wrong
  "correct": "The timeout is 60 seconds",        // required: what is right
  "context": "When discussing API timeouts",      // optional
  "user_id": "user-123",
  "visibility": "private",
  "agent_id": "claude",
  "refers_to": "obs-xyz"
}
```

**Response (200):** Same shape as `/v1/conversation/observe` response.

```bash
curl -s -X POST http://localhost:9999/v1/conversation/correct \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","incorrect":"Wrong info","correct":"Right info"}'
```

---

### POST /v1/conversation/resume

Restore context after session start or context compaction. This is the primary "memory restore" endpoint.

**Request Body:**
```json
{
  "space_id": "mdemg-dev",          // required
  "session_id": "claude-core",       // required
  "include_tasks": true,             // optional: include task-type observations
  "include_decisions": true,         // optional: include decision-type observations
  "include_learnings": true,         // optional: include learning-type observations
  "max_observations": 10,            // optional: max observations to return
  "requesting_user_id": "user-123",  // optional: filter by user visibility
  "agent_id": "claude"               // optional: agent identifier
}
```

**Response (200):**
```json
{
  "space_id": "mdemg-dev",
  "session_id": "claude-core",
  "observations": [
    {
      "node_id": "obs-abc",
      "obs_type": "decision",
      "content": "Decided to use Cobra for CLI",
      "summary": "CLI framework decision",
      "session_id": "claude-core",
      "surprise_score": 0.5,
      "score": 0.89,
      "tags": ["cli"],
      "created_at": "2026-01-15T10:00:00Z"
    }
  ],
  "themes": [
    {
      "node_id": "theme-123",
      "name": "CLI Architecture",
      "summary": "Decisions about CLI structure",
      "member_count": 5,
      "dominant_obs_type": "decision",
      "avg_surprise_score": 0.45,
      "score": 0.8
    }
  ],
  "emergent_concepts": [
    {
      "node_id": "concept-456",
      "name": "Developer Experience",
      "summary": "...",
      "layer": 4,
      "keywords": ["dx", "cli", "ergonomics"],
      "session_count": 12,
      "score": 0.7
    }
  ],
  "summary": "Key context from recent sessions",
  "jiminy": {
    "rationale": "Selected based on recency and relevance",
    "confidence": 0.85,
    "score_breakdown": {},
    "highlights": []
  },
  "anomalies": [],
  "memory_state": "healthy",
  "debug": {}
}
```

**Response Headers (when meta-cognition enabled):**
- `X-MDEMG-Memory-State`: `healthy` | `nominal` | `degraded`
- `X-MDEMG-Anomaly`: anomaly code if degraded

```bash
curl -s -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","max_observations":10}'
```

---

### POST /v1/conversation/recall

Semantic search over conversation memory (observations, themes, concepts).

**Request Body:**
```json
{
  "space_id": "mdemg-dev",
  "query": "What was decided about CLI architecture?",  // required
  "query_embedding": [0.1, ...],       // optional: pre-computed embedding
  "top_k": 10,                         // optional
  "include_themes": true,              // optional
  "include_concepts": true,            // optional
  "requesting_user_id": "user-123",    // optional
  "agent_id": "claude",               // optional
  "temporal_after": "2026-01-01",      // optional
  "temporal_before": "2026-02-01",     // optional
  "filter_tags": ["cli"]              // optional
}
```

**Response (200):**
```json
{
  "space_id": "mdemg-dev",
  "query": "What was decided about CLI architecture?",
  "results": [
    {
      "type": "conversation_observation",
      "node_id": "obs-abc",
      "content": "Decided to use Cobra for CLI framework",
      "score": 0.92,
      "layer": 0,
      "metadata": {}
    }
  ],
  "anomalies": [],
  "memory_state": "nominal",
  "debug": {}
}
```

```bash
curl -s -X POST http://localhost:9999/v1/conversation/recall \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","query":"CLI architecture decisions"}'
```

---

### POST /v1/conversation/consolidate

Run conversation-specific consolidation (theme creation + concept extraction).

**Request Body:**
```json
{
  "space_id": "mdemg-dev"   // required
}
```

**Response (200):**
```json
{
  "space_id": "mdemg-dev",
  "themes_created": 3,
  "concepts_created": 2,
  "duration_ms": 1500
}
```

```bash
curl -s -X POST http://localhost:9999/v1/conversation/consolidate \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev"}'
```

---

### GET /v1/conversation/volatile/stats?space_id=X

Statistics about volatile (not yet graduated) conversation observations.

**Response (200):** Volatile observation counts and decay information.

```bash
curl -s "http://localhost:9999/v1/conversation/volatile/stats?space_id=mdemg-dev"
```

---

### POST /v1/conversation/graduate

Trigger graduation processing (applies decay then graduates eligible observations).

**Request Body:**
```json
{
  "space_id": "mdemg-dev"   // required
}
```

**Response (200):**
```json
{
  "graduated": 5,
  "decay_applied": 12
}
```

```bash
curl -s -X POST http://localhost:9999/v1/conversation/graduate \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev"}'
```

---

### GET /v1/conversation/session/health?session_id=X

CMS usage health for a session.

**Response (200):**
```json
{
  "session_id": "claude-core",
  "space_id": "mdemg-dev",
  "resumed": true,
  "observations_since_resume": 15,
  "health_score": 0.85,
  "tracked": true,
  "last_resume_at": "2026-01-15T10:00:00Z",
  "last_observe_at": "2026-01-15T11:30:00Z"
}
```

```bash
curl -s "http://localhost:9999/v1/conversation/session/health?session_id=claude-core"
```

---

### GET /v1/conversation/session/anomalies?session_id=X&space_id=Y

Aggregated anomaly summary for a session.

**Query Parameters:**
- `session_id` (required)
- `space_id` (required)

**Response (200):**
```json
{
  "session_id": "claude-core",
  "space_id": "mdemg-dev",
  "health_score": 0.85,
  "observation_count": 15,
  "watchdog": { "decay_score": 0.1, "escalation_level": 0 },
  "active_anomalies": []
}
```

```bash
curl -s "http://localhost:9999/v1/conversation/session/anomalies?session_id=claude-core&space_id=mdemg-dev"
```

---

## Templates

Observation templates for structured data capture.

### GET /v1/conversation/templates?space_id=X

List all templates for a space.

### POST /v1/conversation/templates

Create a new template.

**Request Body:**
```json
{
  "space_id": "mdemg-dev",
  "name": "Bug Report",
  "description": "Template for bug observations",
  "obs_type": "error",
  "schema": { "severity": "string", "steps": "array" },
  "auto_capture": {
    "on_session_end": false,
    "on_compaction": true,
    "on_error": true
  }
}
```

**Response (201):** Template object with `template_id`, `created_at`, `updated_at`.

### GET /v1/conversation/templates/{id}?space_id=X

Get a template by ID.

### PUT /v1/conversation/templates/{id}

Update a template.

### DELETE /v1/conversation/templates/{id}?space_id=X

Delete a template. Returns `204 No Content`.

---

## Snapshots

Context snapshots capture session state for recovery across compaction boundaries.

### GET /v1/conversation/snapshot?space_id=X&session_id=Y&limit=N

List snapshots. `session_id` and `limit` are optional.

### POST /v1/conversation/snapshot

Create a snapshot.

**Request Body:**
```json
{
  "space_id": "mdemg-dev",
  "session_id": "claude-core",
  "trigger": "manual",           // manual | compaction | session_end | error
  "context": { "key": "value" }  // arbitrary context data
}
```

**Response (201):** Snapshot object.

### GET /v1/conversation/snapshot/latest?space_id=X&session_id=Y

Get the most recent snapshot. `session_id` is optional.

### GET /v1/conversation/snapshot/{id}

Get a snapshot by ID.

### DELETE /v1/conversation/snapshot/{id}

Delete a snapshot. Returns `204 No Content`.

### POST /v1/conversation/snapshot/cleanup

Clean up old snapshots.

**Request Body:**
```json
{
  "space_id": "mdemg-dev",
  "retention_days": 30    // optional
}
```

**Response (200):**
```json
{
  "deleted": 5,
  "retention_days": 30
}
```

---

## Org Reviews

Multi-agent organizational review workflow for shared observations.

### POST /v1/conversation/observations/{obs_id}/flag-org

Flag an observation for organizational review.

**Request Body:**
```json
{
  "space_id": "mdemg-dev",
  "reason": "May contain sensitive info",
  "suggested_visibility": "team"
}
```

**Headers (optional):** `X-Agent-ID` for agent identification.

**Response (201):** Review request object.

### GET /v1/conversation/org-reviews?space_id=X&status=pending&limit=50

List pending reviews.

### POST /v1/conversation/org-reviews/{obs_id}/decision

Process a review decision.

**Request Body:**
```json
{
  "space_id": "mdemg-dev",
  "decision": "approve",           // approve | reject
  "visibility": "team",            // optional: new visibility level
  "notes": "Approved for team use" // optional
}
```

**Headers (optional):** `X-User-ID` for reviewer identification.

### GET /v1/conversation/org-reviews/stats?space_id=X

Review statistics for a space.

---

## Meta-Learning

### POST /v1/memory/meta-learn

Promote high-value L4/L5 concepts from a source space to the global space (`mdemg-global`).

**Request Body:**
```json
{
  "source_space_id": "my-project",  // required
  "min_layer": 4,                    // optional: minimum layer threshold
  "min_update_count": 3              // optional: minimum update count
}
```

**Response (200):**
```json
{
  "data": {
    "status": "completed",
    "concepts_evaluated": 25,
    "concepts_promoted": 3,
    "promoted_nodes": [
      {
        "id": "global-abc",
        "original_id": "local-abc",
        "name": "Resilience Pattern",
        "global_space_id": "mdemg-global"
      }
    ]
  }
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `503 Service Unavailable` (meta-learning or embedder not enabled)

```bash
curl -s -X POST http://localhost:9999/v1/memory/meta-learn \
  -H "Content-Type: application/json" \
  -d '{"source_space_id":"mdemg-dev","min_layer":4}'
```

---

## Guardrail Validation

### POST /v1/memory/guardrail/validate

Validate code changes against learned constraints. Used in git pre-commit hooks.

**Request Body:**
```json
{
  "space_id": "my-project",
  "files_changed": ["src/auth.go", "src/config.go"],  // required
  "diff": "diff --git a/src/auth.go...",               // required: git diff output
  "agent_trust_level": "standard"                      // optional: "restricted", "standard", "elevated"
}
```

**Response (200):**
```json
{
  "data": {
    "status": "Warning",       // Pass | Warning | Block
    "violations": [
      {
        "constraint_node_id": "const-abc",
        "description": "Missing JWT validation",
        "rationale": "All API endpoints must validate JWT tokens per constraint const-abc"
      }
    ],
    "warnings": []
  }
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `503 Service Unavailable`

```bash
curl -s -X POST http://localhost:9999/v1/memory/guardrail/validate \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","files_changed":["src/main.go"],"diff":"..."}'
```

---

## Guardrail Events

### GET /v1/guardrail/events?space_id=X

Query the guardrail enforcement event log (recent validate calls and their outcomes).

**Query Parameters:**
- `space_id` (required)
- `limit` (optional, default 50)

**Response (200):**
```json
{
  "space_id": "my-project",
  "events": []
}
```

**Status Codes:** `200 OK`, `400 Bad Request`

```bash
curl -s "http://localhost:9999/v1/guardrail/events?space_id=demo"
```

---

## Jiminy Inner-Voice

Proactive guidance for AI agents -- surfaces constraints, prior corrections, contradictions, and frontier knowledge relevant to the current context. Acts as an "inner voice" that reviews what the agent is about to do and injects domain-specific warnings before mistakes happen.

Jiminy must be explicitly enabled via `JIMINY_ENABLED=true`. When disabled, all endpoints return `503 Service Unavailable`.

### GET /v1/jiminy/healthz

Lightweight liveness check for the Jiminy subsystem. Returns immediately.

**Response (200):**
```json
{ "status": "ok", "enabled": true }
```

**Response (503 — Jiminy disabled):**
```json
{ "status": "disabled", "enabled": false }
```

```bash
curl -s http://localhost:9999/v1/jiminy/healthz
```

---

### GET /v1/jiminy/ready

Comprehensive readiness check. Reports feature flags, sub-service availability, and configuration. Append `?stats=true&space_id=<id>` to include guidance effectiveness stats and J17 protocol metrics.

**Response (200):**
```json
{
  "status": "ready",
  "enabled": true,
  "features": {
    "synthesis": true, "evaluate_llm": false, "outcome_llm": false,
    "outcome_classifier": true, "escalation": true, "persistence": false,
    "cache": true, "j17": true
  },
  "services": {
    "evaluator": "available", "sequence_tracker": "available",
    "ticket_manager": "available", "protocol_metrics": "available"
  },
  "config": {
    "timeout_ms": 30000, "max_items": 10, "min_confidence": 0.3
  }
}
```

**Query Parameters:** `stats` (optional, `true` to include stats), `space_id` (optional, defaults to `mdemg-dev`)

**Status Codes:** `200 OK`, `503 Service Unavailable` (Jiminy disabled)

```bash
curl -s "http://localhost:9999/v1/jiminy/ready?stats=true&space_id=mdemg-dev"
```

---

### POST /v1/jiminy/guide

Generate guidance items for the given context. Fans out to four parallel knowledge sources (constraints, corrections, contradictions, frontiers), merges and ranks results.

**Request Body:**
```json
{
  "space_id": "my-project",
  "context": "Implementing JWT auth middleware for the API gateway",
  "session_id": "claude-core",
  "query": "How should I handle token refresh?",
  "file_path": "src/middleware/auth.go",
  "agent_output": "func AuthMiddleware(next http.Handler) http.Handler { ... }",
  "max_items": 5
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `space_id` | string | yes | Memory space to search |
| `context` | string | yes | What the agent is currently doing (user prompt, task description) |
| `session_id` | string | no | Session identifier for correction lookup |
| `query` | string | no | User's original query |
| `file_path` | string | no | File being worked on (used to refine embedding search) |
| `agent_output` | string | no | Agent's proposed output to review |
| `max_items` | int | no | Max guidance items returned (default: configured via `JIMINY_MAX_ITEMS`, fallback 10) |

**Response (200):**
```json
{
  "data": {
    "guidance": [
      {
        "type": "constraint",
        "priority": "high",
        "content": "All API endpoints must validate JWT tokens -- see constraint const-jwt-001",
        "confidence": 0.92,
        "source_nodes": ["const-jwt-001"]
      },
      {
        "type": "correction",
        "priority": "medium",
        "content": "Previous session: token refresh was broken when using HS256 -- switch to RS256",
        "confidence": 0.85,
        "source_nodes": ["obs-abc123"]
      }
    ],
    "prompt_augmentation": "Based on your memory: (1) JWT validation is mandatory...",
    "confidence": 0.88,
    "rationale": "Found 1 constraint and 1 prior correction relevant to auth middleware",
    "source_counts": {
      "constraints": 1,
      "corrections": 1,
      "patterns": 0,
      "conflicts": 0,
      "frontiers": 0
    }
  }
}
```

**Guidance types:** `constraint`, `correction`, `pattern`, `conflict`, `risk`, `suggestion`, `frontier`

**Priority levels:** `high`, `medium`, `low`

**Response (503):** Jiminy is disabled (`JIMINY_ENABLED=false`)
```json
{
  "error": "jiminy guidance is not enabled (set JIMINY_ENABLED=true)"
}
```

**Status Codes:** `200 OK`, `400 Bad Request` (missing space_id or context), `503 Service Unavailable`

```bash
curl -s -X POST http://localhost:9999/v1/jiminy/guide \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "context": "Adding database migration for user sessions table",
    "session_id": "dev-session-01"
  }' | jq .
```

**Related environment variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `JIMINY_ENABLED` | `true` | Enable/disable Jiminy guidance. Note: the binary default is `true`, but the Docker compose template defaults the container env to `false` unless `JIMINY_ENABLED` is set in `.env` (`mdemg init --defaults` writes `true`) |
| `JIMINY_MAX_ITEMS` | `10` | Default max guidance items per request |
| `JIMINY_TIMEOUT_MS` | `6000` | Timeout for guidance generation (ms) |
| `JIMINY_EFFECTIVENESS_TTL_SEC` | `86400` | TTL for tracked guidance (tracking itself is unconditional while Jiminy is enabled) |
| `JIMINY_EFFECTIVENESS_TTL_SEC` | `1800` | TTL for tracked guidance (seconds) |
| `JIMINY_SYNTHESIS_ENABLED` | `true` | Enable LLM-powered guidance synthesis (J15). When enabled, guidance items are synthesized into a coherent narrative |
| `JIMINY_SYNTHESIS_PROVIDER` | inherits `LLM_PROVIDER` | LLM provider for synthesis |
| `JIMINY_SYNTHESIS_MODEL` | inherits `LLM_MODEL` | LLM model for synthesis |
| `JIMINY_SYNTHESIS_MAX_TOKENS` | `3000` | Max tokens for synthesis response |
| `JIMINY_SYNTHESIS_TIMEOUT_MS` | `30000` | Synthesis LLM timeout (ms) |
| `JIMINY_SYNTHESIS_TEMPERATURE` | API default | Optional temperature override for synthesis |

**Note:** The guide response now includes a `guidance_id` (CUID2 unique identifier) in the `data` object for effectiveness tracking.

**Context Correlation:** The `guidance_id` flows through the entire Jiminy feedback loop via `context.WithValue`. LLM interactions from guidance synthesis, evaluation, and outcome classification are all correlated by `guidance_id` in the `llm_interactions` table (see [LLM Interaction Records](#llm-interaction-records)). Protocol JSONL records also include `guidance_id` for cross-record joins.

### POST /v1/jiminy/feedback

Record whether Jiminy guidance was followed, ignored, or contradicted. Requires a `guidance_id` from a prior `/v1/jiminy/guide` response.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `guidance_id` | string | Yes | The guidance_id from the guide response |
| `action_summary` | string | No | Description of the action the agent took |
| `space_id` | string | No | Memory space (for context) |

```bash
# Get guidance (note the guidance_id)
GUID=$(curl -s -X POST http://localhost:9999/v1/jiminy/guide \
  -H "Content-Type: application/json" \
  -d '{"space_id":"my-project","context":"adding input validation"}' \
  | jq -r '.data.guidance_id')

# Send feedback after acting
curl -s -X POST http://localhost:9999/v1/jiminy/feedback \
  -H "Content-Type: application/json" \
  -d "{\"guidance_id\":\"$GUID\",\"action_summary\":\"validated all inputs\",\"space_id\":\"my-project\"}" | jq .
```

**Response (200):**
```json
{
  "data": {
    "guidance_id": "...",
    "applied": true,
    "results": [{"type":"constraint","content":"...","outcome":"followed","similarity":0.42}]
  }
}
```

**Outcome values:** `followed`, `partial_compliance`, `ignored`, `contradicted`, `unknown`

When LLM outcome classification is enabled (`JIMINY_OUTCOME_LLM_ENABLED=true`), responses include a `reasoning` field explaining the classification.

**Related environment variables (J14):**

| Variable | Default | Description |
|----------|---------|-------------|
| `JIMINY_OUTCOME_CLASSIFIER_ENABLED` | `true` | Enable semantic outcome classification |
| `JIMINY_OUTCOME_LLM_ENABLED` | `true` | Enable LLM Tier 2 classification for uncertain cases |
| `JIMINY_OUTCOME_LLM_MAX_TOKENS` | `100` | Max tokens for LLM classification response |
| `JIMINY_OUTCOME_CACHE_SIZE` | `256` | LRU cache capacity for classification results |

**Status Codes:** `200 OK`, `400 Bad Request` (empty guidance_id), `503 Service Unavailable`

### POST /v1/jiminy/evaluate

Evaluate agent output (code, actions) against stored constraints and corrections (J9). Called automatically by the `post-tool-observe.py` hook after Write/Edit completions.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `space_id` | string | Yes | Knowledge space to search |
| `agent_output` | string | Yes | Code or action to evaluate |
| `file_path` | string | No | File being modified |
| `tool_name` | string | No | Tool that produced output (Write, Edit) |
| `session_id` | string | No | Session identifier |

```bash
curl -s -X POST http://localhost:9999/v1/jiminy/evaluate \
  -H "Content-Type: application/json" \
  -d '{"space_id":"my-project","agent_output":"const KEY = \"hardcoded\""}' | jq .
```

**Response (200):**
```json
{
  "data": {
    "evaluation_id": "uuid",
    "status": "warning",
    "items": [
      {"type": "constraint", "content": "[must_not] No hardcoded values (sim: 0.87)", "severity": "high", "source_node": "node-id"}
    ],
    "summary": "Found 1 potential concern(s) in agent output"
  }
}
```

**Status values:** `pass` (no concerns), `warning` (medium severity), `concern` (high severity)

**Status Codes:** `200 OK`, `400 Bad Request` (missing space_id or agent_output), `405 Method Not Allowed`, `503 Service Unavailable`

**Related environment variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `JIMINY_EVALUATE_ENABLED` | `true` | Enable/disable output evaluation |
| `JIMINY_EVALUATE_TIMEOUT_MS` | `3000` | Evaluation timeout (ms) |
| `JIMINY_EVALUATE_MAX_CONSTRAINTS` | `10` | Max constraints to check per evaluation |
| `JIMINY_EVALUATE_LLM_ENABLED` | `false` | Enable LLM Tier 2 reasoning (J13). When enabled, vector-matched candidates are re-evaluated by an LLM for actual violation detection |
| `JIMINY_EVALUATE_LLM_PROVIDER` | inherits `LLM_PROVIDER` | LLM provider for Tier 2 evaluation (`openai` or `ollama`) |
| `JIMINY_EVALUATE_LLM_MODEL` | inherits `LLM_MODEL` | LLM model for Tier 2 evaluation |
| `JIMINY_EVALUATE_LLM_TIMEOUT_MS` | `5000` | LLM request timeout for evaluation |
| `JIMINY_EVALUATE_LLM_MAX_TOKENS` | `2000` | Max response tokens for LLM evaluation |

When LLM Tier 2 is enabled, response items include additional fields:

| Field | Type | Description |
|-------|------|-------------|
| `reasoning` | string | LLM explanation of why the output violates (or doesn't violate) the constraint |
| `remediation` | string | Suggested fix for the violation |

Re-validation rules (J13): `must`/`must_not` violations stay severity `high`. `should`/`should_not` violations are demoted to `medium` warnings. Unknown constraint nodes are demoted to warnings.

---

### J17 AI-to-AI Protocol

Compact agent-to-agent communication protocol for Jiminy guidance. Requires `J17_ENABLED=true`. Returns 503 when disabled.

#### GET /v1/jiminy/bootstrap

Returns the J17 bootstrap payload for new agent sessions — includes protocol version, encoding guide, active T1 constraint codes, and session ticket.

**Query Parameters:**
- `space_id` (required)
- `session_id` (optional)

```bash
curl -s "http://localhost:9999/v1/jiminy/bootstrap?space_id=my-project"
```

**Response (200):**
```json
{
  "data": {
    "bootstrap": "J17v1 BOOT|seq:0|trust:0.50|...",
    "version": "j17v1",
    "first_session": true
  }
}
```

**Status Codes:** `200 OK`, `400 Bad Request` (missing space_id), `503 Service Unavailable`

---

#### POST /v1/jiminy/protocol/feedback

Submit comprehension feedback trials for protocol evolution. Each trial records how accurately an agent interpreted a constraint at a given encoding tier.

**Request Body:**
```json
{
  "trials": [
    {
      "constraint_code": "no-force-push-main",
      "tier": 1,
      "score": 9.5,
      "interpretation": "Never force push to main branch",
      "sender_intent": "Prevent destructive git operations on main"
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `trials` | array | yes | Array of feedback trial objects |
| `trials[].constraint_code` | string | yes | T1 mnemonic code for the constraint |
| `trials[].tier` | int | yes | Encoding tier used (1, 2, or 3) |
| `trials[].score` | float | yes | Comprehension score (0.0-10.0) |
| `trials[].interpretation` | string | no | Agent's interpretation of the constraint |
| `trials[].sender_intent` | string | no | Original sender's intent |

**Response (200):**
```json
{
  "data": {
    "ingested": 1
  }
}
```

```bash
curl -s -X POST http://localhost:9999/v1/jiminy/protocol/feedback \
  -H "Content-Type: application/json" \
  -d '{"trials":[{"constraint_code":"no-force-push","tier":1,"score":9.0}]}'
```

**Status Codes:** `200 OK`, `400 Bad Request` (empty trials array), `503 Service Unavailable`

---

#### POST /v1/jiminy/protocol/learn

Request constraint code re-generation when an existing code is ambiguous or causes comprehension failures. Requires an LLM to be configured.

**Request Body:**
```json
{
  "constraint_type": "must",
  "description": "always run tests before committing",
  "old_code": "test-first",
  "failure_reason": "ambiguous — could mean TDD or pre-commit testing"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `constraint_type` | string | yes | Constraint type (`must`, `must_not`, `should`, `should_not`) |
| `description` | string | yes | Natural language constraint description |
| `old_code` | string | yes | Current T1 code being replaced |
| `failure_reason` | string | no | Why the old code failed comprehension |

**Response (200):**
```json
{
  "data": {
    "old_code": "test-first",
    "new_code": "run-tests-before-commit"
  }
}
```

```bash
curl -s -X POST http://localhost:9999/v1/jiminy/protocol/learn \
  -H "Content-Type: application/json" \
  -d '{"constraint_type":"must","description":"run tests before commit","old_code":"test-first","failure_reason":"ambiguous"}'
```

**Status Codes:** `200 OK`, `400 Bad Request` (missing required fields), `503 Service Unavailable`

---

**J17 environment variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `J17_ENABLED` | `false` | Enable J17 protocol endpoints and encoding |
| `J17_CODEGEN_ENABLED` | `false` | Enable LLM-powered constraint code generation |
| `J17_CODEGEN_PROVIDER` | inherits `LLM_PROVIDER` | LLM provider for code generation (`openai` or `ollama`) |
| `J17_CODEGEN_MODEL` | inherits `LLM_MODEL` | LLM model for code generation |
| `NEURAL_TIER_MODEL` | `""` | Path/name of tier prediction model (empty = disabled, rule-based fallback) |
| `J17_SIDECAR_MODE` | `shadow` | Sidecar arbitration mode: `shadow`, `compare`, `canary`, `active` |
| `J17_SIDECAR_CANARY_PERCENTAGE` | `100` | % of eligible requests routed to ML in canary mode (0-100) |
| `J17_SIDECAR_CONFIDENCE_FLOOR` | `0.6` | ML confidence below this falls back to rule-based (0.0-1.0) |
| `J17_NLI_SCORE_OF_RECORD` | `false` | Use NLI as comprehension score-of-record in canary/active mode |
| `J17_PRECEDENT_PROTECTED_CODES` | `""` | Comma-separated constraint codes that NEVER use ML tier |
| `J17_PRECEDENT_LOG_ENABLED` | `true` | Audit log when ML would change a protected constraint's tier |
| `J17_SIDECAR_CB_ENABLED` | `true` | Enable circuit breaker for sidecar HTTP calls |
| `J17_SIDECAR_CB_FAILURE_THRESHOLD` | `3` | Consecutive failures before circuit opens |
| `J17_SIDECAR_CB_TIMEOUT_SEC` | `15` | Seconds before half-open probe after circuit opens |
| `J17_NLI_OBSERVATIONAL_ENABLED` | `true` | NLI scores flow to metrics in all sidecar modes |
| `J17_TIER_EFFECTIVENESS_MIN_SAMPLES` | `5` | Min outcomes per tier/code before grading |
| `J17_TIER_INEFFECTIVE_THRESHOLD` | `0.6` | Comprehension below this flags tier as ineffective |
| `J17_TIER_DRIFT_DETECTION_ENABLED` | `true` | Enable tier drift RSIC reflection pattern |
| `J17_NLI_CALIBRATION_WINDOW_SIZE` | `500` | NLI/heuristic calibration ring buffer size |
| `J17_NLI_CALIBRATION_BIAS_THRESHOLD` | `0.15` | Max acceptable NLI-vs-heuristic mean bias |

### GET /v1/jiminy/protocol/tier-effectiveness

Returns per-tier comprehension grading, cross-tier delta analysis, and NLI calibration data.

**Query**: `space_id` (string, required)

**Response** (200):
```json
{
  "data": {
    "overall_tier_comprehension": [0.92, 0.87, 0.95],
    "tier_outcome_count": [450, 120, 30],
    "code_tier_delta": [...],
    "ineffective_tiers": [...],
    "nli_calibration": {
      "mean_nli": 0.85,
      "mean_heuristic": 0.80,
      "mean_bias": 0.05,
      "bias_alert": false
    }
  }
}
```

### GET|POST /v1/jiminy/protocol/metrics

GET returns the current J17 protocol metrics snapshot. POST resets the snapshot (operator action). Gated on `J17_ENABLED=true`.

**Response (200, GET):**
```json
{
  "data": {
    "tier_outcome_count": [450, 120, 30],
    "comprehension_rolling_mean": 0.89,
    "nli_fallback_count": 12,
    "ticket_restore_total": 8,
    "ticket_restore_success_rate": 1.0,
    "snapshot_age_seconds": 142
  }
}
```

**Response (200, POST):** `{"status": "reset"}`

### GET /v1/jiminy/protocol/status

Returns the current per-session J17 protocol state (active ticket, current tier, escalation state). Used by `prompt-context.sh` to render strict-mode guidance.

**Query**: `session_id` (string, required)

**Response (200):**
```json
{
  "session_id": "claude-core",
  "active_ticket": "j17-x9y2",
  "current_tier": 2,
  "strict_mode": false,
  "escalations_in_window": 0
}
```

### POST /v1/jiminy/checkpoint

Records a J17 protocol checkpoint (tier transition or major guidance milestone). Body is a `jiminy.CheckpointRequest`. Gated on `J17_ENABLED=true`.

**Request Body:**
```json
{
  "session_id": "claude-core",
  "space_id": "my-project",
  "tier_from": 1,
  "tier_to": 2,
  "reason": "comprehension_drop"
}
```

**Response (200):** `{"checkpoint_id": "...", "status": "recorded"}`

### POST /v1/jiminy/resume-protocol

Resume a J17 protocol session from a prior checkpoint. Body is a `jiminy.ResumeProtocolRequest`.

**Request Body:**
```json
{
  "session_id": "claude-core",
  "ticket_id": "j17-x9y2"
}
```

**Response (200):** restored tier state + protocol context.

### POST /v1/jiminy/extension

Request a J17 protocol extension (operator-driven tier hold or override). Body is a `jiminy.ExtensionRequest`.

**Request Body:**
```json
{
  "session_id": "claude-core",
  "space_id": "my-project",
  "duration_sec": 600,
  "reason": "operator_pin"
}
```

**Response (200):** extension acknowledgement with expiry timestamp.

### POST /v1/jiminy/strict

Toggle Jiminy strict mode for a session. In strict mode, classify and reformulate hooks become blocking gates (vs advisory). State persists in `~/.mdemg/.jiminy-strict-mode` for `pre-write-check.py` hook lookup.

**Request Body:**
```json
{
  "session_id": "claude-core",
  "enabled": true
}
```

**Response (200):** `{"session_id": "...", "enabled": true, "state_file": "~/.mdemg/.jiminy-strict-mode"}`

### POST /v1/jiminy/reformulate

Rewrite an advisory guidance string into an imperative directive (used by strict-mode prompt assembly). Body is a `jiminy.GuidanceRequest` (same shape as `/v1/jiminy/guide`).

**Response (200):**
```json
{
  "guidance": "Do not hardcode connection pool sizes. Use MDEMG_DB_POOL_SIZE env var.",
  "rewritten_from": "..."
}
```

### POST /v1/jiminy/classify

Classify a candidate agent output (proposed code or action) against current strict-mode constraints. Used by `pre-write-check.py` PreToolUse hook to determine pass/deny before Write/Edit. Gated on `JIMINY_ENABLED=true`.

**Request Body:**
```json
{
  "space_id": "my-project",
  "agent_output": "const POOL_SIZE = 50",
  "tool_name": "Write",
  "file_path": "internal/db/pool.go"
}
```

**Response (200):**
```json
{
  "verdict": "deny",
  "reason": "Hardcoded pool size violates constraint J9-002 (escalated, WARNED tier).",
  "constraint_id": "j9-002",
  "severity": "WARNED"
}
```

Fail-open: if the server is unreachable, the hook treats this as `pass`.

### GET /v1/jiminy/latest

Get the most recent guidance entry for a session (used by `prompt-context.sh` to render the current advisory in chat-prompt context). Gated on `JIMINY_ENABLED=true && JIMINY_WARM_ENABLED=true` (warm store is the data source).

**Query**: `session_id` (string, required), `space_id` (string, required)

**Response (200):** the most recent guidance JSON record. 404 if none.

### POST /v1/jiminy/warm

Warm Jiminy's in-memory caches for a space (eager constraint + correction load). Used by `mdemg start` post-boot hook so first guidance request doesn't pay cold-start cost. Gated on `JIMINY_WARM_ENABLED=true`.

**Request Body:**
```json
{"space_id": "my-project"}
```

**Response (202):** `{"status": "warming", "started_at": "..."}`

---

## Synergy Optimization

Token overhead monitoring for Claude Code ↔ MDEMG integration. Tracks file sizes, overflow events, migration status, and overall synergy health.

### GET /v1/synergy/status

Returns synergy health metrics: file line counts, overflow events, migration status, and health score.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `space_id` | string | yes | Memory space to check for overflow events |

**Response (200):**
```json
{
  "data": {
    "jiminy_healthy": true,
    "claude_md_lines": 124,
    "memory_md_lines": 40,
    "auto_memory_files": 3,
    "auto_memory_lines": 56,
    "overflow_events_24h": 0,
    "synergy_health": 1.0,
    "migration_status": "v1",
    "migration_date": "2026-03-24T15:22:34Z"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `jiminy_healthy` | bool | Whether Jiminy is enabled and running |
| `claude_md_lines` | int | Current CLAUDE.md line count |
| `memory_md_lines` | int | Current MEMORY.md line count |
| `auto_memory_files` | int | Count of auto-memory `.md` files (excluding MEMORY.md) |
| `auto_memory_lines` | int | Total lines across auto-memory files |
| `overflow_events_24h` | int | CMS observations tagged `auto-overflow` in last 24h |
| `synergy_health` | float | Health score (0.0–1.0). 0.0 if Jiminy unhealthy |
| `migration_status` | string | Migration version if applied, empty if not |
| `migration_date` | string | ISO8601 timestamp of migration, empty if not |

**Health Score Penalties:**
- Jiminy unhealthy → 0.0 (immediate)
- CLAUDE.md > target+50 → -0.3; > target → -0.1
- MEMORY.md > target+60 → -0.3; > target → -0.1
- Overflow events > 10/24h → -0.3; > 5/24h → -0.1

**Status Codes:** `200 OK`, `405 Method Not Allowed` (non-GET)

```bash
curl -s "http://localhost:9999/v1/synergy/status?space_id=mdemg-dev"
```

**Configuration:**

| Variable | Default | Description |
|----------|---------|-------------|
| `SYNERGY_TARGET_CLAUDE_LINES` | `150` | Target line count for CLAUDE.md |
| `SYNERGY_TARGET_MEMORY_LINES` | `120` | Target line count for MEMORY.md |
| `SYNERGY_CLAUDE_MD_PATH` | auto-detect | Override CLAUDE.md path |
| `SYNERGY_MEMORY_MD_PATH` | auto-detect | Override MEMORY.md path |
| `SYNERGY_ASSESSMENT_ENABLED` | `true` | Enable synergy RSIC dimension |

---

## Spaces & Freshness

### GET /v1/memory/spaces/{space_id}/freshness

Freshness/staleness information for a single space.

**Response (200):**
```json
{
  "space_id": "my-project",
  "last_ingest_at": "2026-01-15T10:00:00Z",
  "last_ingest_type": "codebase-ingest",
  "ingest_count": 15,
  "stale_hours": 24,
  "is_stale": false,
  "threshold_hours": 48
}
```

```bash
curl -s "http://localhost:9999/v1/memory/spaces/demo/freshness"
```

---

### GET /v1/memory/freshness?space_ids=a,b,c

Batch freshness check for multiple spaces (max 100).

**Response (200):**
```json
{
  "spaces": [
    { "space_id": "a", "is_stale": false, "stale_hours": 2, "ingest_count": 10, "threshold_hours": 48 },
    { "space_id": "b", "is_stale": true, "stale_hours": 72, "ingest_count": 3, "threshold_hours": 48 }
  ],
  "threshold_hours": 48
}
```

```bash
curl -s "http://localhost:9999/v1/memory/freshness?space_ids=demo,mdemg-dev"
```

---

## Jobs (SSE)

### GET /v1/jobs/{job_id}

Server-Sent Events (SSE) stream for job progress. Returns `text/event-stream`.

Used to monitor async jobs (backup, restore, scraper, etc.).

**Response:** SSE stream with job status events.

```bash
curl -N "http://localhost:9999/v1/jobs/backup-abc123"
```

---

## Codebase Ingestion API

**Removed in DORMANT-CENSUS-001.** The legacy `/v1/memory/ingest-codebase[/…]` endpoints (deprecated with a `Deprecation` header since Phase 94) no longer exist. Use the Ingestion Pipeline API below (`/v1/memory/ingest/trigger` and related endpoints).

---

## Ingestion Pipeline API

The ingestion trigger API (successor to the removed `/v1/memory/ingest-codebase`).

### POST /v1/memory/ingest/trigger

Trigger an ingestion pipeline run.

### GET /v1/memory/ingest/status/{job_id}

Get ingestion pipeline job status.

### DELETE /v1/memory/ingest/cancel/{job_id}

Cancel an ingestion pipeline job.

### GET /v1/memory/ingest/jobs

List all ingestion pipeline jobs.

### POST /v1/memory/ingest/files

Ingest specific files.

---

## Scraper API

Web scraping with LLM content review.

### POST /v1/scraper/jobs

Create a new scrape job.

**Request Body:**
```json
{
  "urls": ["https://example.com/docs"],    // required
  "target_space_id": "my-project"          // optional (uses default if omitted)
}
```

**Response (202):**
```json
{
  "job_id": "scrape-abc12345",
  "status": "pending",
  "urls": ["https://example.com/docs"],
  "target_space_id": "my-project",
  "total_urls": 1,
  "processed_urls": 0,
  "created_at": "2026-01-15T10:00:00Z",
  "updated_at": "2026-01-15T10:00:00Z"
}
```

### GET /v1/scraper/jobs

List all scrape jobs.

**Response (200):**
```json
{
  "jobs": [...],
  "count": 5
}
```

### GET /v1/scraper/jobs/{id}

Get a scrape job with its scraped content.

### DELETE /v1/scraper/jobs/{id}

Cancel a scrape job.

**Response (200):**
```json
{
  "job_id": "scrape-abc",
  "status": "cancelled",
  "message": "job cancelled"
}
```

### POST /v1/scraper/jobs/{id}/review

Process review decisions for scraped content.

**Request Body:**
```json
{
  "decisions": [
    { "url": "https://example.com/docs", "action": "approve" }
  ]
}
```

### GET /v1/scraper/spaces

List available target spaces for scraping.

**Response (200):**
```json
{
  "spaces": [
    { "space_id": "demo", "node_count": 100 }
  ],
  "count": 1
}
```

---

## Linear Integration API

CRUD operations for Linear issues, projects, and comments via the Linear plugin module.

### POST /v1/linear/issues

Create a Linear issue.

**Request Body:**
```json
{
  "title": "Bug: Login fails",    // required
  "team_id": "TEAM-1",            // required (or configured default)
  "description": "Details...",
  "priority": "2"
}
```

**Response (201):**
```json
{
  "entity": {
    "id": "ISS-123",
    "entity_type": "issue",
    "fields": { "title": "Bug: Login fails", ... },
    "created_at": "2026-01-15T10:00:00Z"
  }
}
```

### GET /v1/linear/issues

List issues with optional filters.

**Query Parameters:**
- `team` - filter by team
- `state` - filter by state
- `assignee` - filter by assignee
- `project` - filter by project
- `query` - search query
- `limit` - max results (default: 50)
- `cursor` - pagination cursor

### GET /v1/linear/issues/{id}

Get a single issue by ID.

### PUT /v1/linear/issues/{id}

Update an issue.

### DELETE /v1/linear/issues/{id}

Delete an issue.

### POST /v1/linear/projects

Create a Linear project.

**Request Body:**
```json
{
  "name": "Q1 Sprint",    // required
  "description": "..."
}
```

### GET /v1/linear/projects

List projects.

### GET /v1/linear/projects/{id}

Get a single project.

### PUT /v1/linear/projects/{id}

Update a project.

### POST /v1/linear/comments

Add a comment to an issue.

**Request Body:**
```json
{
  "issue_id": "ISS-123",  // required
  "body": "Comment text"   // required
}
```

**Response (201):** Entity object.

---

## Human Review API (HITL)

Dataset-agnostic human-in-the-loop review surface (HITL-REVIEW-001). All four endpoints require an admin-scoped API key when `AUTH_API_KEYS` is set. See `docs/features/hitl-review.md`.

- `GET /v1/review/datasets` — list registered reviewable datasets with candidate counts.
- `POST /v1/review/next` — fetch the next ungraded item for a dataset.
- `POST /v1/review/grade` — submit a rubric grade (optionally reinforcing the live substrate).
- `POST /v1/review/reverse` — reverse a prior grade's reinforcement.

---

## Alerts & Hooks

### POST /v1/alerts/grafana

**Removed in DORMANT-CENSUS-001** (superseded by the server-native alert evaluator). Pending alerts live in `~/.mdemg/alerts/current.json` and are acknowledged via `POST /v1/alerts/clear`.

### POST /v1/alerts/clear

Clear pending alerts after delivery — called by the hooks once alerts have been displayed (cleared = delivered). See `docs/features/hook-channel-health.md`.

### POST /v1/hooks/event

Record a hook heartbeat/lifecycle event (feeds the V0024 hook-event telemetry and the `hook_channel_silent` evaluator rule). See `docs/features/hook-channel-health.md`.

---

### POST /v1/webhooks/linear

Linear webhook handler. Receives Linear webhook events and processes them.

### POST /v1/webhooks/{provider}

Generic webhook handler for GitHub, GitLab, Bitbucket, and other providers.

Path suffix determines the provider (e.g., `/v1/webhooks/github`).

---

## File Watcher API

### POST /v1/filewatcher/start

Start a file watcher for a space.

**Request Body:**
```json
{
  "space_id": "my-project",                // required
  "path": "/path/to/watch",                // required: directory path
  "extensions": [".go", ".py", ".ts"],      // optional (defaults to common code extensions)
  "excludes": ["node_modules", ".git"],     // optional (defaults to common excludes)
  "debounce_ms": 500                        // optional (default: 500)
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "path": "/absolute/path/to/watch",
  "status": "watching"
}
```

```bash
curl -s -X POST http://localhost:9999/v1/filewatcher/start \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","path":"/path/to/project"}'
```

---

### GET /v1/filewatcher/status

List all active file watchers.

**Response (200):**
```json
{
  "watchers": [
    { "space_id": "my-project", "path": "/path/to/watch", "status": "active" }
  ],
  "count": 1
}
```

```bash
curl -s http://localhost:9999/v1/filewatcher/status
```

---

### POST /v1/filewatcher/stop

Stop a file watcher.

**Request Body:**
```json
{
  "space_id": "my-project"   // required
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "status": "stopped"
}
```

```bash
curl -s -X POST http://localhost:9999/v1/filewatcher/stop \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo"}'
```

---

## Admin

### GET /v1/admin/spaces

List all spaces with metadata.

**Query Parameters:**
- `prunable` (optional): `"true"` or `"false"` to filter
- `limit` (optional): max results (default: 100, max: 500)

**Response (200):**
```json
{
  "spaces": [
    {
      "space_id": "my-project",
      "prunable": false,
      "protected": false,
      "created_at": "2026-01-01T00:00:00Z",
      "last_ingest_at": "2026-01-15T10:00:00Z",
      "ingest_count": 15,
      "node_count": 1500,
      "observation_count": 800
    }
  ],
  "total": 5,
  "prunable_count": 2
}
```

```bash
curl -s "http://localhost:9999/v1/admin/spaces"
```

---

### PATCH /v1/admin/spaces/{space_id}

Update space metadata (currently: set prunable flag).

**Request Body:**
```json
{
  "prunable": true    // required
}
```

**Response (200):**
```json
{
  "space_id": "old-project",
  "prunable": true,
  "updated": true
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `403 Forbidden` (protected space), `404 Not Found`

```bash
curl -s -X PATCH http://localhost:9999/v1/admin/spaces/old-project \
  -H "Content-Type: application/json" \
  -d '{"prunable":true}'
```

---

### POST /v1/admin/spaces/prune

Execute batch pruning of prunable spaces. Deletes all nodes, edges, TapRoots, and RSICState for eligible spaces. Protected spaces (`mdemg-dev`, `mdemg-global`) are never pruned.

**Request Body:**
```json
{
  "dry_run": true,      // optional: preview without deleting
  "batch_size": 10000,  // optional (default: 10000)
  "max_spaces": 50      // optional (default: 50)
}
```

**Response (200, dry_run=true):**
```json
{
  "dry_run": true,
  "results": [
    { "space_id": "old-project", "nodes_deleted": 500, "status": "dry_run" }
  ],
  "spaces_pruned": 1,
  "total_nodes_deleted": 500
}
```

**Response (200, dry_run=false):**
```json
{
  "dry_run": false,
  "results": [],
  "spaces_pruned": 2,
  "total_nodes_deleted": 1200,
  "spaces_skipped": 0
}
```

```bash
curl -s -X POST http://localhost:9999/v1/admin/spaces/prune \
  -H "Content-Type: application/json" \
  -d '{"dry_run":true}'
```

---

## Admin — Circuit Breakers

Operator endpoints for inspecting and resetting the LLM circuit breakers registered in `internal/llmclient/breaker.go`. Useful when a breaker trips on a transient incident and hasn't auto-recovered by the half-open probe cycle. All endpoints are gated by `AUTH_API_KEYS`.

### GET /v1/admin/breakers

List every registered breaker with current state and counts.

**Response (200):**
```json
{
  "breakers": [
    {
      "name": "openai-constraint-classify",
      "state": "closed",
      "consecutive_failures": 0,
      "consecutive_successes": 0,
      "last_failure_time": "0001-01-01T00:00:00Z",
      "failure_threshold": 5,
      "timeout_sec": 60
    },
    {
      "name": "jiminy-synthesis",
      "state": "open",
      "consecutive_failures": 5,
      "consecutive_successes": 0,
      "last_failure_time": "2026-04-17T18:03:12Z",
      "failure_threshold": 5,
      "timeout_sec": 60
    }
  ]
}
```

```bash
curl -s http://localhost:9999/v1/admin/breakers \
  -H "X-API-Key: $MDEMG_API_KEY" | jq
```

**Status Codes:** `200 OK`, `401 Unauthorized` (missing/invalid API key).

---

### POST /v1/admin/breakers/reset

Force a named breaker to `StateClosed`. Clears failure counters and allows the next call through. Does not modify breaker configuration.

**Request Body:**
```json
{ "name": "openai-constraint-classify" }
```

**Response (200):**
```json
{
  "name": "openai-constraint-classify",
  "previous_state": "open",
  "state": "closed",
  "reset_at": "2026-04-17T18:04:22Z"
}
```

```bash
curl -s -X POST http://localhost:9999/v1/admin/breakers/reset \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $MDEMG_API_KEY" \
  -d '{"name":"openai-constraint-classify"}' | jq
```

**Status Codes:** `200 OK`, `400 Bad Request` (missing/empty name), `401 Unauthorized`, `404 Not Found` (unknown breaker name).

---

## Space Transfer (Export/Import)

HTTP API for exporting and importing space data. Supports profile-based filtering and conflict-aware import.

### GET /v1/admin/spaces/export/preview

Lightweight estimation of what an export would contain, without transferring data.

**Query Parameters:**
- `space_id` (required): Space to preview
- `profile` (optional): Export profile -- `full`, `metadata`, `shareable`, `codebase`, `cms`, `learned` (default: `full`)

**Response (200):**
```json
{
  "space_id": "my-project",
  "profile": "shareable",
  "estimated_nodes": 42,
  "estimated_edges": 15,
  "estimated_observations": 30,
  "estimated_symbols": 0,
  "filters_applied": {
    "obs_types": ["learning", "decision", "correction", "technical_note", "insight", "preference", "constraint"],
    "exclude_volatile": true,
    "only_pinned": false,
    "min_layer": 0,
    "max_layer": 0
  }
}
```

**Status Codes:** `200 OK`, `400 Bad Request` (missing space_id or invalid profile)

```bash
curl -s "http://localhost:9999/v1/admin/spaces/export/preview?space_id=my-project&profile=shareable"
```

---

### POST /v1/admin/spaces/export

Export space data with profile-based filtering and optional overrides.

**Request Body:**
```json
{
  "space_id": "my-project",
  "profile": "shareable",
  "obs_types": ["learning", "decision"],
  "tags": ["important"],
  "exclude_volatile": true,
  "only_pinned": false,
  "no_observations": false,
  "no_symbols": false
}
```

Only `space_id` is required. All other fields are optional and override profile defaults.

**Response (200):**
```json
{
  "space_id": "my-project",
  "profile": "shareable",
  "header": { "format": "mdemg-space-transfer", "version": "1.0.0" },
  "chunks": [ ... ],
  "summary": {
    "nodes_exported": 42,
    "edges_exported": 15,
    "observations_exported": 30,
    "symbols_exported": 0,
    "duration_ms": 142
  }
}
```

The `chunks` array contains protobuf-JSON `SpaceChunk` objects -- the same format as `.mdemg` files.

**Status Codes:** `200 OK`, `400 Bad Request` (missing space_id or invalid profile)

```bash
curl -s -X POST http://localhost:9999/v1/admin/spaces/export \
  -H "Content-Type: application/json" \
  -d '{"space_id":"my-project","profile":"shareable"}'
```

---

### POST /v1/admin/spaces/import

Import space data from export chunks with conflict handling.

**Request Body:**
```json
{
  "space_id": "target-space",
  "conflict": "skip",
  "chunks": [ ... ]
}
```

- `space_id` (optional): If provided, remaps all chunk space_ids to this target
- `conflict` (optional): `skip` (default), `overwrite`, or `error`
- `chunks` (required): Array of `SpaceChunk` objects from an export response

**Response (200):**
```json
{
  "space_id": "target-space",
  "nodes_created": 42,
  "nodes_skipped": 0,
  "nodes_overwritten": 0,
  "edges_created": 15,
  "edges_skipped": 0,
  "edges_merged": 0,
  "observations_created": 30,
  "symbols_created": 0,
  "warnings": [],
  "duration_ms": 87
}
```

**Status Codes:** `200 OK`, `400 Bad Request` (missing/null chunks, invalid conflict mode)

```bash
# Export then import to a new space
EXPORT=$(curl -s -X POST http://localhost:9999/v1/admin/spaces/export \
  -H "Content-Type: application/json" \
  -d '{"space_id":"source-space","profile":"full"}')

CHUNKS=$(echo "$EXPORT" | jq '.chunks')

curl -s -X POST http://localhost:9999/v1/admin/spaces/import \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"target-space\",\"conflict\":\"skip\",\"chunks\":$CHUNKS}"
```

### GET /v1/admin/config

Returns the effective configuration with source attribution for each key.

**Query Parameters:** None.

**Response (200):**
```json
{
  "config": [
    {"key": "neo4j.uri", "value": "bolt://neo4j:7687", "source": "yaml", "masked": false},
    {"key": "openai.api_key", "value": "****", "source": "env", "masked": true}
  ],
  "yaml_path": ".mdemg/config.yaml"
}
```

Each entry includes a `source` field (`env`, `yaml`, or `default`) and a `masked` flag for sensitive values.

```bash
curl -s "http://localhost:${MDEMG_PORT:-9999}/v1/admin/config" | python3 -m json.tool
```

### GET /v1/admin/logs

Returns recent log entries from an in-process ring buffer. Filtering (by level or text search) is performed client-side in the browser UI.

**Query Parameters:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `limit` | int | 200 | Max entries to return (most recent first) |

**Response (200):**
```json
{
  "entries": [
    {"timestamp": "2026-03-30T12:00:00Z", "level": "INFO", "message": "server started", "raw": "time=2026-03-30T12:00:00Z level=INFO msg=\"server started\""}
  ],
  "seq": 42
}
```

```bash
curl -s "http://localhost:${MDEMG_PORT:-9999}/v1/admin/logs?limit=10" | python3 -m json.tool
```

### PATCH /v1/admin/config

Updates YAML config values. Rejects env-sourced keys, masked/sensitive keys, and unknown keys. Validates values before writing.

**Request Body:**
```json
{"updates": {"learning.eta": "0.05", "plugins.enabled": "true"}}
```

**Response (200):**
```json
{
  "config": [...],
  "yaml_path": ".mdemg/config.yaml",
  "updated": 2
}
```

**Error (400):** Returns error if key is env-sourced, unknown, or sensitive.

```bash
curl -s -X PATCH "http://localhost:${MDEMG_PORT:-9999}/v1/admin/config" \
  -H "Content-Type: application/json" \
  -d '{"updates":{"learning.eta":"0.05"}}' | python3 -m json.tool
```

### POST /v1/admin/restart

Triggers a graceful server restart via re-exec. Returns immediately; the server restarts after 500ms.

**Response (200):**
```json
{"status": "restarting"}
```

### GET /v1/admin/instances

Discover running MDEMG instances. Returns the current server and any additional instances found via sidebar/menubar registry files.

**Response (200):**
```json
{
  "instances": [
    {
      "id": "self",
      "name": "mdemg-dev",
      "url": "http://localhost:9999",
      "status": "healthy",
      "version": "v0.5.3"
    }
  ]
}
```

### POST /v1/admin/rsic/start

Starts the RSIC watchdog. Returns `{"status": "started"}` or `{"status": "already_running"}`.

### POST /v1/admin/rsic/stop

Stops the RSIC watchdog. Returns `{"status": "stopped"}` or `{"status": "already_stopped"}`.

### POST /v1/admin/rsic/restart

Restarts the RSIC watchdog. Returns `{"status": "restarted"}`.

### GET /v1/admin/features

Returns all known services/features with runtime status.

**Response (200):**
```json
{
  "features": [
    {"name": "ape_scheduler", "status": "healthy", "state": "running", "controllable": true},
    {"name": "jiminy", "status": "healthy", "state": "running", "controllable": false, "config_key": "jiminy.enabled"}
  ]
}
```

Status values: `healthy`, `stopped`, `unavailable`. Controllable services can be started/stopped at runtime.

```bash
curl -s "http://localhost:${MDEMG_PORT:-9999}/v1/admin/features" | python3 -m json.tool
```

### POST /v1/admin/features/start|stop|restart

Controls lifecycle of controllable services.

**Request Body:**
```json
{"name": "ape_scheduler"}
```

**Response (200):**
```json
{"status": "started", "name": "ape_scheduler"}
```

### POST /v1/plugins/{id}/start|stop|restart

Controls lifecycle of individual plugins.

**Response (200):**
```json
{"status": "started", "plugin_id": "my-plugin"}
```

---

## Self-Improvement (RSIC) API

Recursive Self-Improvement Cycle endpoints. RSIC enables automated assessment, planning, and improvement of memory quality.

### POST /v1/self-improve/assess

Run an assessment of memory quality for a space.

**Request Body:**
```json
{
  "space_id": "my-project",  // required
  "tier": "meso"             // optional: micro | meso | macro (default: meso)
}
```

**Response (200):** Assessment report with quality metrics and recommendations.

The response embeds a `SelfAssessmentReport` with 7 sub-dimension scores plus — as of DH-005 (2026-04-17) — 7 per-dimension **data-sufficiency confidences**. Confidence tells you whether a `0.0` score means "broken" or "no data." Dimensions with `confidence == 0` are excluded from `overall_health`, not penalised.

| Score field | Confidence field | Confidence meaning |
|-------------|------------------|--------------------|
| `retrieval_quality` | `retrieval_confidence` | `LearningPhase` lookup: cold=0.4, learning=0.7, warm/saturated=1.0 |
| `memory_health` | `memory_confidence` | `min(1, TotalNodes/100)` |
| `edge_health` | `edge_confidence` | `min(1, EdgeCount/50)`; 0 when graph has no edges |
| `task_performance` | `task_confidence` | `min(1, (Volatile+Permanent)/50)`; 0 when no observations |
| `guidance_health` | `guidance_confidence` | `min(1, TotalGuidanceIssued/30)`; 0 when Jiminy has issued none |
| `protocol_health` | `protocol_confidence` | `min(1, TotalEvents/30)`; 0 when none recorded |
| `synergy_health` | `synergy_confidence` | Binary: 1 when Jiminy healthy AND files present, else 0 |

`overall_health` is a normalised weighted-confidence sum: `Σ(w·c·s) / Σ(w·c)`. Default weights are returned by `DefaultHealthWeights` (Protocol 0.20, Task 0.20, Guidance 0.17, Memory 0.15, Edge 0.15, Retrieval 0.08, Synergy 0.05) and can be overridden via `RSIC_HEALTH_WEIGHT_<DIM>` env vars. See `docs/features/rsic-feedback-loop.md` for derivation.

```bash
curl -s -X POST http://localhost:9999/v1/self-improve/assess \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","tier":"meso"}' | jq '{
    overall_health,
    scores: {retrieval_quality, memory_health, edge_health, task_performance, guidance_health, protocol_health, synergy_health},
    confidences: {retrieval_confidence, memory_confidence, edge_confidence, task_confidence, guidance_confidence, protocol_confidence, synergy_confidence}
  }'
```

---

### GET /v1/self-improve/report

Get all active RSIC tasks.

**Response (200):**
```json
{
  "active_tasks": [...]
}
```

---

### GET /v1/self-improve/report/{taskID}

Get reports for a specific RSIC task.

**Response (200):**
```json
{
  "task_id": "task-abc",
  "reports": [...]
}
```

---

### POST /v1/self-improve/cycle

Run a full RSIC cycle (assess -> reflect -> plan -> execute -> monitor).

**Request Body:**
```json
{
  "space_id": "my-project",            // required
  "tier": "meso",                      // optional: micro | meso | macro
  "trigger_source": "manual_api",      // optional: manual_api | micro_auto | session_periodic | cron_macro | watchdog_escalation
  "idempotency_key": "unique-key-123", // optional: deduplication key
  "dry_run": false                      // optional: simulate without executing
}
```

**Response (200):** Cycle outcome with cycle_id, actions taken, and metrics.

**Response (409 Conflict):** When trigger is rejected by orchestration policy:
```json
{
  "error": "trigger rejected",
  "reason": "concurrent cycle in progress",
  "policy_version": "v1"
}
```

```bash
curl -s -X POST http://localhost:9999/v1/self-improve/cycle \
  -H "Content-Type: application/json" \
  -d '{"space_id":"demo","tier":"meso"}'
```

---

### GET /v1/self-improve/history

Get RSIC cycle history.

**Query Parameters:**
- `limit` (optional): max results (default: 10)
- `trigger_source` (optional): filter by trigger source
- `tier` (optional): filter by tier
- `space_id` (optional): filter by space

**Response (200):**
```json
{
  "history": [...],
  "count": 5,
  "filters": { "tier": "meso" }
}
```

```bash
curl -s "http://localhost:9999/v1/self-improve/history?limit=5&tier=meso"
```

---

### GET /v1/self-improve/calibration

Get RSIC calibration parameters.

**Response (200):**
```json
{
  "calibration": { ... }
}
```

---

### GET /v1/self-improve/health

RSIC system health including watchdog, orchestration, persistence, and safety status.

**Response (200):**
```json
{
  "status": "ok",
  "active_tasks": 0,
  "watchdog": { "decay_score": 0.05, "escalation_level": 0 },
  "orchestration": {
    "policy_version": "v1",
    "concurrent_cycles": {},
    "total_triggers": 50
  },
  "persistence": { "status": "ok" },
  "safety": {
    "enforcement_active": true,
    "safety_version": "v1",
    "bounds": {
      "max_nodes_affected": 100,
      "max_edges_affected": 100,
      "protected_spaces": ["mdemg-dev"]
    },
    "rollback": {
      "window_sec": 3600,
      "snapshots_held": 5,
      "oldest_snapshot_age_sec": 1800
    }
  }
}
```

```bash
curl -s http://localhost:9999/v1/self-improve/health
```

---

### GET /v1/self-improve/signals

Get Hebbian signal learner effectiveness metrics.

**Response (200):**
```json
{
  "signals": [...],
  "enabled": true,
  "count": 15
}
```

---

### GET /v1/self-improve/rollback

List available rollback snapshots.

**Response (200):**
```json
{
  "snapshots": [...],
  "count": 5,
  "rollback_window_sec": 3600
}
```

### POST /v1/self-improve/rollback

Execute a rollback to a previous state.

**Request Body:**
```json
{
  "snapshot_id": "snap-abc123"   // required
}
```

**Response (200):** Rollback result.
**Response (404):** Snapshot not found or rollback window expired.

```bash
curl -s -X POST http://localhost:9999/v1/self-improve/rollback \
  -H "Content-Type: application/json" \
  -d '{"snapshot_id":"snap-abc123"}'
```

---

### POST /v1/self-improve/orchestration/reset

Reset RSIC orchestration state (cooldowns, active tasks). Used for test isolation.

No request body.

**Response (200):**
```json
{"reset": true}
```

**Status Codes:** `200 OK`, `503 Service Unavailable` (orchestration not initialized).

---

## Backup & Restore

### POST /v1/backup/trigger

Trigger a backup.

**Request Body:**
```json
{
  "type": "full",                     // required: "full" or "partial_space"
  "space_ids": ["my-project"],         // required for partial_space
  "keep_forever": false                // optional: protect from deletion
}
```

**Response (202):**
```json
{
  "backup_id": "bak-abc123",
  "status": "pending",
  "message": "backup triggered"
}
```

```bash
curl -s -X POST http://localhost:9999/v1/backup/trigger \
  -H "Content-Type: application/json" \
  -d '{"type":"partial_space","space_ids":["demo"]}'
```

---

### GET /v1/backup/status/{id}

Get backup job status.

**Response (200):**
```json
{
  "backup_id": "bak-abc123",
  "status": "completed",
  "type": "full",
  "checksum": "sha256:...",
  "size": 1048576,
  "created": "2026-01-15T10:00:00Z"
}
```

---

### GET /v1/backup/list?type=X

List backups with optional type filter.

**Response (200):**
```json
{
  "backups": [...],
  "count": 5
}
```

---

### GET /v1/backup/manifest/{id}

Get full backup manifest.

**TSDB Backup Manifest Fields:**

TSDB backups (triggered via the backup system) include additional manifest fields for TimescaleDB and training data:

| Field | Type | Description |
|-------|------|-------------|
| `format_version` | int | Manifest format version |
| `backup_id` | string | Unique backup identifier |
| `created_at` | string | ISO8601 creation timestamp |
| `size_bytes` | int | Backup file size |
| `checksum` | string | SHA-256 checksum |
| `schema_version` | int | TSDB schema version (currently 5) |
| `row_count` | int | Number of rows backed up |
| `database` | string | Database name |
| `label` | string | Optional label |
| `jsonl_tar_path` | string | Path to `training-data.tar.gz` containing JSONL training data |
| `jsonl_tar_size` | int | Size of the JSONL training data archive in bytes |

> **Note:** Backups now include JSONL training data (exported `llm_interactions` records) as a `training-data.tar.gz` archive alongside the TSDB dump. The `jsonl_tar_path` and `jsonl_tar_size` fields are omitted from the manifest when no training data is included.

---

### DELETE /v1/backup/{id}

Delete a backup.

**Status Codes:** `200 OK`, `404 Not Found`, `409 Conflict` (keep_forever protected)

---

### POST /v1/backup/restore

Trigger a restore from backup.

**Request Body:**
```json
{
  "backup_id": "bak-abc123"   // required
}
```

**Response (202):**
```json
{
  "restore_id": "restore-abc",
  "backup_id": "bak-abc123",
  "status": "pending",
  "message": "restore triggered"
}
```

---

### GET /v1/backup/restore/status/{id}

Get restore job status.

---

## Symbols & Relationships

### GET /v1/symbols/relationships?space_id=X

Relationship counts by type for a space.

**Response (200):**
```json
{
  "space_id": "my-project",
  "counts": { "CALLS": 150, "IMPORTS": 80, "INHERITS": 20 }
}
```

---

### GET /v1/symbols/{id}/relationships?space_id=X

Get relationships for a specific symbol node.

**Response (200):**
```json
{
  "space_id": "my-project",
  "symbol_id": "sym-abc",
  "relationships": [
    { "source": "sym-abc", "target": "sym-def", "type": "CALLS", "weight": 1.0 }
  ],
  "count": 5
}
```

---

## Cleanup

### POST /v1/memory/cleanup/orphans

Detect and optionally archive/delete orphan L0 nodes (not included in latest ingestion).

**Request Body:**
```json
{
  "space_id": "my-project",     // required
  "action": "archive",           // required: list | count | archive | delete
  "dry_run": false,              // optional
  "limit": 100,                  // optional (1-1000, default: 100)
  "older_than_days": 30,         // optional: only nodes older than N days
  "path_prefix": "src/"          // optional: filter by path prefix
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "orphans_found": 15,
  "orphans_acted": 15,
  "action": "archive",
  "dry_run": false,
  "orphans": [
    { "node_id": "abc", "path": "src/old.go", "name": "OldFile", "last_ingested_at": "...", "status": "archived" }
  ]
}
```

---

### POST /v1/memory/cleanup/schedule

Set up scheduled orphan cleanup.

**Request Body:**
```json
{
  "space_id": "my-project",
  "interval_hours": 24,
  "action": "archive",
  "limit": 100
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "schedule_id": "cleanup-abc12345",
  "interval_hours": 24,
  "action": "archive",
  "status": "enabled",
  "next_run_at": "2026-01-16T10:00:00Z"
}
```

---

### GET /v1/memory/cleanup/schedules?space_id=X

List cleanup schedules.

---

### GET /v1/memory/cleanup/stats?space_id=X

Cleanup statistics (orphan count, archived count).

**Response (200):**
```json
{
  "space_id": "my-project",
  "orphan_count": 15,
  "archived_count": 50
}
```

---

### POST /v1/memory/cleanup/graph-orphans

Detect zero-edge (disconnected) nodes across spaces and optionally fix them.

**Request Body:**
```json
{
  "space_ids": ["my-project"],   // optional: specific spaces (all if omitted)
  "action": "scan",               // required: scan | consolidate | archive | delete
  "dry_run": false,               // optional
  "limit": 100,                   // optional (1-1000)
  "min_age_days": 7,              // optional
  "layers": [0, 1]                // optional: filter by layer
}
```

**Response (200):**
```json
{
  "action": "scan",
  "dry_run": false,
  "total_spaces": 1,
  "total_orphans": 25,
  "total_affected": 0,
  "space_results": [
    {
      "space_id": "my-project",
      "orphan_count": 25,
      "affected_count": 0,
      "layer_breakdown": { "L0": 20, "L1": 5 },
      "nodes": [...]
    }
  ],
  "warnings": []
}
```

---

## Edge Consistency

### GET /v1/memory/edges/stale/stats?space_id=X

Statistics about stale edges in a space.

```bash
curl -s "http://localhost:9999/v1/memory/edges/stale/stats?space_id=demo"
```

---

### POST /v1/memory/edges/stale/refresh

Refresh stale edges in a space.

**Request Body:**
```json
{
  "space_id": "my-project"
}
```

**Response (200):**
```json
{
  "space_id": "my-project",
  "edges_refreshed": 42
}
```

---

## Metrics & Monitoring

### GET /v1/metrics?space_id=X

Graph metrics (node counts, edge counts, hub nodes, etc.).

**Response (200):**
```json
{
  "total_nodes": 5000,
  "total_edges": 15000,
  "nodes_by_layer": { "0": 4000, "1": 700, "2": 200, "3": 80, "4": 20 },
  "edges_by_type": { "BELONGS_TO": 4000, "CO_ACTIVATED_WITH": 8000, "ABSTRACTS": 300 },
  "avg_edge_weight": 0.45,
  "hub_nodes": [
    { "node_id": "abc", "name": "CoreConfig", "degree": 42 }
  ],
  "orphan_nodes": 50,
  "recent_activity": { "nodes_created": 100, "edges_created": 500, "retrievals": 200 }
}
```

---

### GET /v1/metrics/snapshot

Returns a JSON metrics snapshot including counters, gauges, and histograms from the MetricsRecorder. Metrics are persisted to TimescaleDB on each flush cycle (TSDB schema version 5).

Includes:
- HTTP request metrics (latency, status codes)
- Circuit breaker states
- Cache hit ratios
- Neo4j pool metrics
- Neo4j graph per-space metrics (nodes, edges, orphans, health score)
- Neo4j container resource metrics (CPU, memory)
- Memory pressure metrics
- TSDB writer health

```bash
curl -s http://localhost:9999/v1/metrics/snapshot
```

**Response (200):**
```json
{
  "data": {
    "timestamp": "2026-03-29T12:00:00Z",
    "counters": { "mdemg_http_requests_total": 1234 },
    "gauges": { "mdemg_memory_heap_bytes": 52428800 },
    "histograms": {
      "mdemg_http_request_duration_seconds": {
        "count": 500,
        "sum": 12.5,
        "buckets": { "0.01": 100, "0.05": 300, "0.1": 450 },
        "p95": 0.085,
        "p99": 0.12
      }
    },
    "writer_health": {
      "last_successful_write": "2026-03-29T11:59:30Z",
      "consecutive_failures": 0,
      "buffer_size": 0
    }
  }
}
```

**Status Codes:** `200 OK`, `405 Method Not Allowed`, `503 Service Unavailable`

> **Note:** The previous `/v1/prometheus` endpoint has been removed and returns `410 Gone`. Use `/v1/metrics/snapshot` instead.

---

### LLM Interaction Records

Every generative LLM call is recorded in the `llm_interactions` TimescaleDB hypertable. Records are buffered in memory and flushed periodically by the `LLMInteractionWriter`.

**Schema (TSDB schema version 5):**

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `time` | TIMESTAMPTZ | No | Timestamp of the LLM call |
| `trace_id` | TEXT | No | CUIDv2 unique trace identifier |
| `task_name` | TEXT | No | Subsystem task (e.g. `ape.reflect`, `jiminy.evaluate`) |
| `space_id` | TEXT | No | Memory space identifier |
| `session_id` | TEXT | Yes | Session identifier |
| `system_prompt` | TEXT | Yes | System prompt sent to LLM |
| `user_prompt` | TEXT | No | User prompt sent to LLM |
| `response` | TEXT | Yes | LLM response text |
| `think_content` | TEXT | Yes | Extracted `<think>...</think>` block content |
| `think_mode` | BOOLEAN | No | Whether think mode was detected in the response |
| `latency_ms` | INTEGER | Yes | Round-trip latency in milliseconds |
| `tokens_in` | INTEGER | Yes | Input token count |
| `tokens_out` | INTEGER | Yes | Output token count |
| `model_name` | TEXT | Yes | Model identifier (e.g. `gpt-4o`) |
| `provider` | TEXT | Yes | LLM provider (e.g. `openai`, `anthropic`) |
| `error` | TEXT | Yes | Error message if the call failed |
| `guidance_id` | TEXT | Yes | Jiminy `guidance_id` for feedback loop correlation |
| `source_path` | TEXT | Yes | Source file path for ingest-triggered classifier calls |
| `quality` | FLOAT | Yes | Quality score 0.0--1.0, populated by annotation pipeline |
| `quality_source` | TEXT | Yes | Source of quality annotation: `feedback_outcome`, `llm_judge`, `deterministic`, `human` |
| `used_for_train` | BOOLEAN | No | Whether this record has been exported for training |
| `dataset_ver` | TEXT | Yes | Dataset version if exported |

> **Migration history:** The base table was created in migration 002. Migration 005 added the `guidance_id` and `source_path` columns with conditional indexes (indexed only where non-NULL to avoid bloat).

#### Privacy Scrubbing

All LLM interaction records are automatically privacy-scrubbed before storage. The scrubber runs on `system_prompt`, `user_prompt`, `response`, and `think_content` fields.

| Pattern | Replacement |
|---------|-------------|
| API keys (`sk-*`, `ghp_*`, `AKIA*`, `Bearer` tokens) | `[REDACTED_KEY]` |
| Absolute paths (`/Users/*`, `/home/*`) | `/[PATH]/last/two/components` |
| Environment secrets (`PASSWORD=value`, `SECRET=...`, etc.) | `PASSWORD=[REDACTED]` |
| Email addresses | `[EMAIL]` |
| Neo4j credentials in connection strings | `neo4j://[REDACTED]@` |

---

### GET /v1/neo4j/overview

Neo4j database overview with container stats and graph metrics.

---

### GET /v1/system/pool-metrics

Neo4j connection pool metrics.

---

### GET /v1/ape/status

APE (Automatic Processing Engine) scheduler status.

### POST /v1/ape/trigger

Manually trigger an APE processing cycle.

### GET /v1/metrics/trends

Return time-series trends for a named metric across a window. Backed by the TSDB hierarchical roll-up (raw <24h, hourly 1–30d, daily >30d). Used by the Status dashboard's metric-trend sparklines.

**Query**: `metric_name` (string, required), `space_id` (string, required), `from`/`to` (RFC3339 timestamps, default last 24h), `granularity` (`raw|hourly|daily|auto`, default `auto`).

**Response (200):**
```json
{
  "metric_name": "mdemg_retrieval_consensus_strength",
  "space_id": "myproject",
  "granularity": "hourly",
  "points": [
    {"time": "2026-05-20T18:00:00Z", "value": 0.42, "min_value": 0.31, "max_value": 0.48, "count": 142, "quality_tag": ""}
  ]
}
```

---

## Determinism Metrics

### GET /v1/metrics/determinism?space_id=X

Compute a determinism score measuring how consistently the system influences agent behavior.

D = (informed_actions / total_actions) x compliance_rate x coverage_ratio

Requires `DETERMINISM_SCORING_ENABLED=true`.

**Query Parameters:**
- `space_id` (required)

**Response (200):**
```json
{
  "space_id": "my-project",
  "score": 0.72,
  "informed_actions": 180,
  "total_actions": 250,
  "compliance_rate": 0.88,
  "coverage_ratio": 0.90
}
```

**Status Codes:** `200 OK`, `400 Bad Request`, `503 Service Unavailable`

```bash
curl -s "http://localhost:9999/v1/metrics/determinism?space_id=demo"
```

---

## Neural Sidecar

Optional Python FastAPI sidecar providing cross-encoder re-ranking, NLI classification, and J17 tier prediction. Runs on port 8100 by default.

### GET /v1/neural/status

Status of the neural sidecar integration from the Go server's perspective.

**Response (200):**
```json
{
  "sidecar_enabled": false,
  "neural_rerank_enabled": false,
  "data_collection_enabled": false,
  "rerank_url": "http://localhost:8100"
}
```

**Status Codes:** `200 OK`

```bash
curl -s http://localhost:9999/v1/neural/status
```

---

### POST /protocol/predict-tier (Sidecar)

Predict the optimal J17 encoding tier for a constraint. Called by the Go server's tier predictor with fallback to rule-based selection if unavailable.

**Note:** This endpoint runs on the neural sidecar (default port 8100), not the main MDEMG server.

**Request Body:**
```json
{
  "constraint_text": "never force push to main",
  "agent_context": "implementing git workflow",
  "trust_score": 0.8
}
```

**Response (200):**
```json
{
  "predicted_tier": 1,
  "confidence": 0.87,
  "model": "cross-encoder/ms-marco-MiniLM-L-6-v2",
  "latency_ms": 12.3
}
```

Returns `predicted_tier: 0` with `confidence: 0.0` when no tier model is loaded (Go uses rule-based fallback).

**Config:** Set `NEURAL_TIER_MODEL` to a model path or HuggingFace model name to enable. Empty = disabled.

```bash
curl -s -X POST http://localhost:8100/protocol/predict-tier \
  -H "Content-Type: application/json" \
  -d '{"constraint_text":"run tests before commit","agent_context":"working on CI","trust_score":0.7}'
```

---

## Hash Verification (UNTS)

Unified Node Test Specification hash verification system for ensuring spec file integrity.

### POST /v1/hash-verification/register

Register a file for hash tracking.

**Request Body:**
```json
{
  "path": "specs/my-spec.json",
  "framework": "uats",
  "hash": "sha256:...",
  "source_ref": "commit:abc123",
  "source": "ci-pipeline"
}
```

---

### GET /v1/hash-verification/files

List tracked files.

**Query Parameters:**
- `framework` (optional): filter by framework
- `status` (optional): filter by verification status

---

### GET /v1/hash-verification/files/{path}

Get tracking info for a specific file.

---

### POST /v1/hash-verification/verify

Verify a single file's hash.

**Request Body:**
```json
{
  "path": "specs/my-spec.json"
}
```

**Response (200):**
```json
{
  "path": "specs/my-spec.json",
  "status": "verified",
  "expected_hash": "sha256:abc...",
  "actual_hash": "sha256:abc..."
}
```

---

### POST /v1/hash-verification/verify-all

Verify all tracked files.

**Request Body (optional):**
```json
{
  "framework": "uats"   // optional: verify only specific framework
}
```

**Response (200):**
```json
{
  "results": [...],
  "total": 50,
  "verified": 48,
  "mismatched": 2
}
```

---

### POST /v1/hash-verification/update

Update a file's expected hash.

**Request Body:**
```json
{
  "path": "specs/my-spec.json",
  "hash": "sha256:newHash...",
  "source": "manual-update"
}
```

---

### POST /v1/hash-verification/revert

Revert a file's hash to a previous value.

**Request Body:**
```json
{
  "path": "specs/my-spec.json",
  "target_hash": "sha256:previousHash..."
}
```

---

### POST /v1/hash-verification/scan

Scan for new files and auto-register them.

**Response (200):**
```json
{
  "scanned": true,
  "files_registered": 50
}
```

---

## Plugins & Modules

### GET /v1/plugins

List registered plugins.

### POST /v1/plugins

Register or manage a plugin.

### GET /v1/modules

List active modules.

### POST /v1/modules/{module_id}/sync

Trigger a module sync operation.

---

## Training Data Export

API endpoints for exporting TSDB training data as `.tar.gz` archives for the LoRA fine-tuning curation pipeline.

### POST /v1/training-data/export

Trigger an async training data export. The export streams TSDB rows to JSONL, applies privacy scanning on all text fields, packages into a `.tar.gz` archive with a UTDS-compliant manifest.

**Auth:** `ScopeAdminSpaces`

**Request Body:**
```json
{
  "space_id": "mdemg-dev",
  "from": "2026-01-01T00:00:00Z",
  "to": "2026-04-01T00:00:00Z",
  "tables": ["llm_interactions", "retrieval_events"]
}
```

**Response (202):**
```json
{
  "export_id": "exp-reh3376-mdemg-dev-20260401-120000",
  "status": "pending"
}
```

### GET /v1/training-data/status/{id}

Poll export job status.

**Response (200):**
```json
{
  "export_id": "exp-reh3376-mdemg-dev-20260401-120000",
  "status": "completed",
  "progress": 100,
  "output_path": "/tmp/mdemg-export.tar.gz"
}
```

Status values: `pending`, `running`, `completed`, `failed`.

### GET /v1/training-data/download/{id}

Download the completed export archive. Returns `application/gzip` with `Content-Disposition: attachment`.

**Response:** Binary `.tar.gz` file.

---

## System

### GET /v1/system/capability-gaps

List detected capability gaps.

### GET /v1/system/capability-gaps/{id}

Get details for a specific capability gap.

### POST /v1/feedback

**Removed in DORMANT-CENSUS-001** (zero producers). The live feedback channel is `POST /v1/jiminy/feedback` (called by the `post-tool-observe.py` hook).

### GET /v1/system/gap-interviews

List gap interview sessions.

### GET/POST /v1/system/gap-interviews/{id}

Manage a specific gap interview.

---

## Dashboard / Visualization (internal)

These endpoints back the built-in browser dashboard at `http://localhost:${MDEMG_PORT}/ui/`. They're documented for completeness; operators typically consume them through the UI rather than calling them directly. Authentication: same as other `/api/*` and `/v1/*` routes (`AUTH_API_KEYS` if configured).

### GET /api/graph/data

Return node + edge data shaped for the dashboard's force-directed graph view.

**Query**: `space_id` (string, required), `layer` (int, optional filter), `limit` (default 200).

**Response (200):** `{"nodes": [...], "edges": [...], "stats": {...}}` — same general shape as `/v1/memory/graph/neighborhood` but optimized for the UI's rendering.

### GET /api/graph/fields

Return the schema field catalog (per-layer node properties available for filtering/coloring in the dashboard).

**Response (200):** `{"layers": {"L0": ["path", "summary", ...], "L1": [...]}}`.

### GET /api/graph/health

Dashboard-side health check. Returns a quick summary of graph reachability + cache hit ratio for the explorer view.

**Response (200):** `{"reachable": true, "cache_hit_ratio": 0.78, "last_query_ms": 42}`.

### GET /viz/topology

Returns an HTML page with an embedded D3-based topology visualization, served directly (not JSON). Used by `mdemg ui topology` and the dashboard's "Topology" tab link.

**Response (200, text/html):** standalone HTML.

---

## MCP Server Tools

The MDEMG MCP server provides 20 tools for IDE integration. Start with `mdemg mcp` (stdio mode) or `mdemg serve --mcp` (co-launch with HTTP server).

**Memory Tools:**
| Tool | Description |
|------|-------------|
| `memory_store` | Store an observation in the knowledge graph |
| `memory_recall` | Semantic search across the knowledge graph |
| `memory_associate` | Create typed edges between nodes |
| `memory_reflect` | Generate a reflection on a topic |
| `memory_status` | Get space health and statistics |
| `memory_symbols` | Search extracted code symbols |

**Ingestion Tools:**
| Tool | Description |
|------|-------------|
| `memory_ingest_trigger` | Trigger codebase ingestion job |
| `memory_ingest_status` | Check ingestion job status |
| `memory_ingest_cancel` | Cancel a running ingestion job |
| `memory_ingest_jobs` | List all ingestion jobs |
| `memory_ingest_files` | Ingest specific files |

**Space Tools:**
| Tool | Description |
|------|-------------|
| `memory_space_freshness` | Check space freshness and staleness |

**Linear Integration Tools:**
| Tool | Description |
|------|-------------|
| `linear_create_issue` | Create a Linear issue |
| `linear_list_issues` | List Linear issues with filters |
| `linear_read_issue` | Read a specific Linear issue |
| `linear_update_issue` | Update a Linear issue |
| `linear_add_comment` | Add a comment to a Linear issue |
| `linear_search` | Search Linear issues |

**Cognitive Tools:**
| Tool | Description |
|------|-------------|
| `validate_changes` | Validate proposed changes against learned constraints |
| `jiminy_guide` | Get proactive guidance for the current context |

**Connection:** `mdemg mcp` runs in stdio mode. The MCP server connects to the MDEMG HTTP API (resolved via `MDEMG_ENDPOINT`, `.mdemg.port` file, or `LISTEN_ADDR`). Configure in `.claude/mcp.json` or your IDE's MCP settings.

---

## Common Status Codes

| Code | Meaning |
|------|---------|
| `200 OK` | Success |
| `201 Created` | Resource created |
| `202 Accepted` | Async job started |
| `204 No Content` | Success with no body (deletes) |
| `400 Bad Request` | Invalid request body or parameters |
| `403 Forbidden` | Protected space operation |
| `404 Not Found` | Resource not found |
| `405 Method Not Allowed` | Wrong HTTP method |
| `409 Conflict` | Concurrent operation (RSIC policy rejection, backup protected) |
| `500 Internal Server Error` | Server error (details logged, not exposed to client) |
| `503 Service Unavailable` | Required service not initialized (embedder, scraper, etc.) |

---

## Common Headers

**Request Headers:**
- `Content-Type: application/json` - required for POST/PUT/PATCH bodies
- `Authorization: Bearer <token>` - when authentication is enabled
- `X-Agent-ID` - optional agent identifier (used by org reviews)
- `X-User-ID` - optional user identifier (used by org reviews)

**Response Headers:**
- `Content-Type: application/json` - all JSON responses
- `X-MDEMG-Memory-State` - memory health on resume/recall (`healthy`, `nominal`, `degraded`)
- `X-MDEMG-Anomaly` - anomaly code when memory state is degraded
- `X-Session-Warning` - warning when session has not called resume
- `Deprecation: true` - on deprecated endpoints (with `Link` header to successor)

---

## Protected Spaces

The following spaces are protected from destructive operations (pruning, deletion):
- `mdemg-dev` - Claude's conversation memory
- `mdemg-global` - Global meta-learning space

Protected spaces cannot be marked as prunable, and the API will return `403 Forbidden` for delete/prune operations targeting them.

---

## Platform-Specific Notes

### macOS / Linux

All `curl` examples in this document use standard Unix shell syntax (backslash `\` for line continuation, single quotes for JSON). These work as-is on macOS and Linux.

### Windows

#### curl on Windows

Windows 10 (build 17063+) and Windows 11 ship with `curl.exe`. Important differences from macOS/Linux:

- **Line continuation:** Use `^` instead of `\` for multi-line commands in `cmd.exe`. In PowerShell, use backtick `` ` ``.
- **JSON quoting:** In `cmd.exe`, use escaped double quotes `\"` inside the `-d` string. In PowerShell, use single quotes for the outer string or `ConvertTo-Json`.
- **PowerShell alias conflict:** PowerShell aliases `curl` to `Invoke-WebRequest`. To use actual curl, call `curl.exe` explicitly, or use the native `Invoke-RestMethod` cmdlet as shown below.

#### PowerShell Examples

```powershell
# Health check
Invoke-RestMethod -Uri "http://localhost:9999/healthz"

# Memory retrieve
$body = @{
    space_id   = "demo"
    query_text = "How does authentication work?"
    top_k      = 5
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/memory/retrieve" `
  -ContentType "application/json" -Body $body

# Conversation observe
$body = @{
    space_id   = "mdemg-dev"
    session_id = "claude-core"
    content    = "Important learning"
    obs_type   = "learning"
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/conversation/observe" `
  -ContentType "application/json" -Body $body

# Conversation resume
$body = @{
    space_id         = "mdemg-dev"
    session_id       = "claude-core"
    max_observations = 10
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/conversation/resume" `
  -ContentType "application/json" -Body $body
```

#### PowerShell Tips

```powershell
# Tip: Create a reusable base URI variable
$base = "http://localhost:9999"

# Tip: Pretty-print JSON responses
Invoke-RestMethod -Uri "$base/healthz" | ConvertTo-Json -Depth 10

# Tip: Check response headers (use Invoke-WebRequest instead of Invoke-RestMethod)
$response = Invoke-WebRequest -Uri "$base/v1/conversation/resume" `
  -Method Post -ContentType "application/json" `
  -Body '{"space_id":"mdemg-dev","session_id":"claude-core","max_observations":10}'
$response.Headers["X-MDEMG-Memory-State"]

# Tip: Use splatting for complex requests
$params = @{
    Method      = "Post"
    Uri         = "$base/v1/memory/retrieve"
    ContentType = "application/json"
    Body        = (@{
        space_id   = "demo"
        query_text = "How does authentication work?"
        top_k      = 5
    } | ConvertTo-Json)
}
Invoke-RestMethod @params
```

#### Environment Variables on Windows

```cmd
:: cmd.exe
set NEO4J_URI=bolt://localhost:7687
set EMBEDDING_PROVIDER=openai
```

```powershell
# PowerShell (session only)
$env:NEO4J_URI = "bolt://localhost:7687"
$env:EMBEDDING_PROVIDER = "openai"

# PowerShell (persistent for user)
[Environment]::SetEnvironmentVariable("NEO4J_URI", "bolt://localhost:7687", "User")
```

---

## Event Graph Federation

Pattern Y1 federation: a graph walk in Neo4j combined with the time-series events touching that neighborhood in TSDB, joined in Go. Both endpoints require `EVENTGRAPH_ENABLED=true` (else `503 {"error":"eventgraph disabled"}`) and share the `/v1/admin/breakers` auth convention (gated when `AUTH_API_KEYS` is set). See the feature doc `docs/features/event-graph-federation.md` and the CLI consumers `mdemg eventgraph …`.

Both endpoints share the same optional-field convention: `hops`/`since_seconds`/`limit` are omitted to take the server config defaults. `hops` must be ≥ 0 and ≤ the server ceiling (`2 × EVENTGRAPH_FEDERATION_DEFAULT_HOPS`). Errors: `400` (bad JSON / missing `space_id` or `seed_node_id` / bad hops), `405` (non-POST), `503` (disabled / service uninitialized), `500` (`{"error":"federation query failed","detail":"…"}`).

### POST /v1/eventgraph/reinforcement-neighborhood

Reinforcement (Hebbian co-activation) events in a node's graph neighborhood (EVENTGRAPH-001).

**Request body:**

| Field | Type | Required | Notes |
|---|---|---|---|
| `space_id` | string | yes | |
| `seed_node_id` | string | yes | walk origin |
| `hops` | int | no | nil → `EVENTGRAPH_FEDERATION_DEFAULT_HOPS` (default 2) |
| `since_seconds` | int | no | nil → `EVENTGRAPH_FEDERATION_DEFAULT_LOOKBACK_HOURS` × 3600 |
| `limit` | int | no | nil → `EVENTGRAPH_MAX_EVENTS_PER_QUERY` (default 500) |

**Response 200:**

```json
{
  "events": [{
    "event_id": "…", "recorded_at": "2026-06-08T17:49:01Z",
    "src_node_id": "n_…", "dst_node_id": "n_…",
    "prev_weight": 0.10, "new_weight": 0.11, "delta_weight": 0.008,
    "evidence_count_after": 2, "direction": "bidirectional",
    "session_id": "…", "created_new_edge": true, "trigger_path": "apply_coactivation",
    "src_in_neighborhood": true, "dst_in_neighborhood": true
  }],
  "neighbor_node_ids": ["n_…"],
  "graph_hops": 2,
  "tsdb_rows_scanned": 20,
  "truncated": false
}
```

`events` and `neighbor_node_ids` always serialize as `[]` (never `null`) when empty.

### POST /v1/eventgraph/guidance-outcome-neighborhood

Guidance outcomes (constraint effectiveness) in a constraint's graph neighborhood (EVENTGRAPH-002). Walks the neighborhood, collects each neighbor's `constraint_code`, and joins `constraint_outcomes` on those codes.

**Request body:** identical fields + validation to the reinforcement endpoint (`space_id`, `seed_node_id` required; `hops`/`since_seconds`/`limit` optional with the same defaults + ceiling).

**Response 200:**

```json
{
  "outcomes": [{
    "time": "2026-06-08T15:15:19Z",
    "constraint_id": "40e8a524-…",          // TSDB source UUID (NOT a Neo4j node_id)
    "constraint_code": "no-direct-main-commits",  // the join key
    "guidance_id": "…", "session_id": "claude-core",
    "outcome_type": "followed",             // followed|ignored|contradicted|partial_compliance|not_applicable
    "similarity": 1.0, "guidance_type": "pattern",
    "constraint_node_id": "n_…",            // Neo4j constraint node whose code matched
    "in_neighborhood": true
  }],
  "neighbor_node_ids": ["n_…"],
  "neighbor_constraint_codes": ["no-direct-main-commits"],
  "graph_hops": 2,
  "tsdb_rows_scanned": 11,
  "truncated": false
}
```

All three array fields (`outcomes`, `neighbor_node_ids`, `neighbor_constraint_codes`) always serialize as `[]` (never `null`) when empty. Outcomes recorded without a `constraint_code` are not joinable and won't appear.

---

## Links

- [CLI Reference](cli-reference.md)
- [Ingestion Guide](ingestion-guide.md)
- [CMS & RSIC Guide](cms-rsic-guide.md)
