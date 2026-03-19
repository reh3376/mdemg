# MDEMG API Reference

This document provides a complete reference for all MDEMG HTTP API endpoints.

**Base URL**: `http://localhost:9999` (default)

## Table of Contents

- [Health Checks](#health-checks)
- [Memory Operations](#memory-operations)
- [Retrieval & Search](#retrieval--search)
- [Consolidation](#consolidation)
- [Learning System](#learning-system)
- [Conversation Memory](#conversation-memory)
- [Constraints (Phase 45.5)](#constraints-phase-455)
- [Active Guardrails (Phase 104)](#active-guardrails-phase-104)
- [Skill Registry](#skill-registry-phase-48)
- [Capability Gaps](#capability-gaps)
- [Linear Integration](#linear-integration)
- [Web Scraper](#web-scraper)
- [Webhooks](#webhooks)
- [Symbol Relationships](#symbol-relationships)
- [Plugins & Modules](#plugins--modules)
- [Cleanup & Orphan Management](#cleanup--orphan-management)
- [File Watcher](#file-watcher-phase-94)
- [Job Streaming (SSE)](#job-streaming-sse)
- [System & Monitoring](#system--monitoring)
- [Backup & Restore](#backup--restore-phase-70)
- [Neo4j State Monitor](#neo4j-state-monitor-phase-76)
- [Meta-Cognition & Self-Improvement](#meta-cognition--self-improvement-phase-80)
- [RSIC Orchestration & Safety](#rsic-orchestration--safety-phases-87-90)
- [Space Transfer (Export/Import)](#space-transfer-exportimport)
- [Frontier Detection](#get-v1memoryfrontiers)
- [Negative Feedback](#post-v1learningnegative-feedback)
- [Jiminy Guidance Service](#jiminy-guidance-service-phase-jiminy)

---

## Health Checks

### GET /healthz

Basic liveness check.

**Response**:

```json
{"status": "ok"}
```

### GET /readyz

Readiness check (verifies Neo4j schema version).

**Response**:

```json
{
  "status": "ready",
  "embedding_provider": "openai",
  "embedding_dimensions": 1536
}
```

### GET /v1/embedding/health

Active health check for the configured embedding provider. Generates a real test embedding to measure latency and detect failures.

**Response**:

```json
{
  "status": "healthy",
  "provider": "openai",
  "model": "text-embedding-3-small",
  "dimensions": 1536,
  "latency_ms": 42.0,
  "cache_enabled": true,
  "cache_hit_rate": 0.87,
  "error_count_24h": 0,
  "success_rate_24h": 1.0,
  "circuit_breaker": "closed",
  "last_error": "",
  "last_error_at": "",
  "configured_env_var": true
}
```

**Status Values**: `healthy` (all checks pass), `degraded` (high error rate), `unhealthy` (embedding generation fails).

---

## Memory Operations

### POST /v1/memory/ingest

Store a single observation.

**Request Body**:

```json
{
  "space_id": "my-project",
  "name": "UserService.authenticate",
  "path": "src/services/user.ts",
  "content": "Authentication logic using JWT tokens...",
  "tags": ["auth", "security"],
  "timestamp": "2024-01-15T10:30:00Z",
  "source": "code-analysis"
}
```

**Response**:

```json
{
  "node_id": "mem-abc123",
  "status": "created",
  "embedding_dims": 1536
}
```

### POST /v1/memory/ingest/batch

Store multiple observations in a single request.

**Request Body**:

```json
{
  "space_id": "my-project",
  "observations": [
    {
      "name": "ConfigLoader",
      "path": "src/config/loader.ts",
      "content": "Loads environment configuration..."
    },
    {
      "name": "DatabaseConnection",
      "path": "src/db/connection.ts",
      "content": "PostgreSQL connection pool..."
    }
  ]
}
```

**Response**:

```json
{
  "success_count": 2,
  "error_count": 0,
  "results": [
    {"node_id": "mem-abc123", "status": "success"},
    {"node_id": "mem-def456", "status": "success"}
  ]
}
```

### POST /v1/memory/ingest/trigger

Trigger a background codebase re-ingestion job. Returns immediately with a job ID.

**Request Body**:

```json
{
  "space_id": "my-project",
  "path": "/path/to/codebase",
  "batch_size": 100,
  "workers": 4,
  "extract_symbols": true,
  "consolidate": true,
  "incremental": false,
  "exclude_dirs": ["vendor", "node_modules"]
}
```

**Response** (202 Accepted):

```json
{
  "job_id": "ingest-abc12345",
  "space_id": "my-project",
  "status": "pending",
  "message": "Ingestion job created. Use GET /v1/memory/ingest/status/ingest-abc12345 to check progress.",
  "created_at": "2026-02-04T10:00:00Z"
}
```

### GET /v1/memory/ingest/status/{job_id}

Check the status and progress of an ingestion job.

**Response**:

```json
{
  "job_id": "ingest-abc12345",
  "space_id": "my-project",
  "status": "running",
  "progress": {
    "total": 4522,
    "current": 1200,
    "percentage": 26.5,
    "phase": "ingestion",
    "rate": "15.2 elements/sec"
  },
  "started_at": "2026-02-04T10:00:01Z",
  "created_at": "2026-02-04T10:00:00Z"
}
```

### POST /v1/memory/ingest/cancel/{job_id}

Cancel a running or pending ingestion job.

**Response**:

```json
{
  "job_id": "ingest-abc12345",
  "status": "cancelled",
  "message": "Job cancellation requested"
}
```

### GET /v1/memory/ingest/jobs

List all ingestion jobs with their current status.

**Response**:

```json
{
  "jobs": [
    {
      "job_id": "ingest-abc12345",
      "status": "completed",
      "space_id": "my-project",
      "progress": {"total": 4522, "current": 4522, "percentage": 100},
      "created_at": "2026-02-04T10:00:00Z",
      "completed_at": "2026-02-04T10:05:06Z"
    }
  ],
  "count": 1
}
```

### POST /v1/memory/ingest/files

Re-ingest specific files into memory. Synchronous for ≤50 files; returns a background job ID for >50.

**Request Body**:

```json
{
  "space_id": "my-project",
  "files": ["/path/to/file1.go", "/path/to/file2.ts"],
  "extract_symbols": true,
  "consolidate": false
}
```

**Response** (synchronous):

```json
{
  "space_id": "my-project",
  "total_files": 2,
  "success_count": 2,
  "error_count": 0,
  "results": [
    {"file": "/path/to/file1.go", "status": "success", "node_id": "mem-abc123"},
    {"file": "/path/to/file2.ts", "status": "success", "node_id": "mem-def456"}
  ]
}
```

**Response** (>50 files, 202 Accepted):

```json
{
  "space_id": "my-project",
  "total_files": 75,
  "job_id": "ingest-files-abc12345"
}
```

### POST /v1/memory/ingest-codebase

**Deprecated** — prefer `/v1/memory/ingest/trigger`. Trigger a codebase ingestion job with fine-grained options. All responses include `Deprecation: true` and `Link` headers pointing to the successor endpoint.

**Request Body**:

```json
{
  "space_id": "my-project",
  "path": "/path/to/codebase",
  "source": { "type": "local", "branch": "main" },
  "languages": { "go": true, "typescript": true, "include_tests": false },
  "symbols": { "extract": true, "max_per_file": 500 },
  "exclusions": { "preset": "default", "directories": ["vendor", "node_modules"] },
  "processing": { "batch_size": 50, "workers": 4 },
  "options": { "incremental": false, "consolidate": true, "dry_run": false }
}
```

**Required**: `space_id`, `path`

**Response** (202 Accepted):

```json
{
  "job_id": "a1b2c3d4",
  "status": "queued",
  "space_id": "my-project",
  "path": "/path/to/codebase"
}
```

Also supports:
- `GET /v1/memory/ingest-codebase` — list all jobs
- `GET /v1/memory/ingest-codebase/{job_id}` — get job status with stats
- `DELETE /v1/memory/ingest-codebase/{job_id}` — cancel a running job

### POST /v1/memory/nodes/{node_id}/archive

Soft-delete a memory node (sets `is_archived=true`).

**Request Body** (optional):

```json
{
  "reason": "Outdated implementation"
}
```

**Response**:

```json
{
  "node_id": "mem-abc123",
  "name": "OldService",
  "archived_at": "2024-01-15T10:30:00Z",
  "reason": "Outdated implementation"
}
```

### POST /v1/memory/nodes/{node_id}/unarchive

Restore an archived memory node.

**Response**:

```json
{
  "node_id": "mem-abc123",
  "name": "OldService",
  "unarchived_at": "2024-01-15T11:00:00Z"
}
```

### DELETE /v1/memory/nodes/{node_id}?confirm=true

Permanently delete a memory node. Requires `confirm=true` query parameter.

**Response**:

```json
{
  "node_id": "mem-abc123",
  "deleted_nodes": 1,
  "deleted_edges": 5
}
```

### POST /v1/memory/archive/bulk

Archive multiple nodes in a single request.

**Request Body**:

```json
{
  "space_id": "my-project",
  "node_ids": ["mem-abc123", "mem-def456"],
  "reason": "Deprecated module"
}
```

**Response**:

```json
{
  "space_id": "my-project",
  "total_items": 2,
  "success_count": 2,
  "error_count": 0,
  "results": [...]
}
```

---

## Retrieval & Search

### POST /v1/memory/retrieve

Semantic search with optional LLM re-ranking.

**Request Body**:

```json
{
  "space_id": "my-project",
  "query": "How does authentication work?",
  "top_k": 10,
  "include_evidence": true,
  "include_hidden": true,
  "min_score": 0.5,
  "rerank": true
}
```

**Parameters**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `space_id` | string | Yes | Memory space identifier |
| `query` | string | Yes | Search query text |
| `top_k` | int | No | Max results (default: 10) |
| `include_evidence` | bool | No | Include file:line references |
| `include_hidden` | bool | No | Include hidden layer nodes |
| `min_score` | float | No | Minimum similarity score |
| `rerank` | bool | No | Apply LLM re-ranking |
| `translate_intent` | bool | No | Rewrite query via LLM before embedding (Phase 102). Requires `INTENT_ENABLED=true` |

**Response**:

```json
{
  "results": [
    {
      "node_id": "mem-abc123",
      "name": "AuthService.login",
      "score": 0.85,
      "layer": 0,
      "evidence": [
        {
          "symbol_name": "login",
          "symbol_type": "function",
          "file_path": "src/auth/service.ts",
          "line_number": 42
        }
      ]
    }
  ],
  "translated_intent": "auth service login authentication workflow session management",
  "debug": {
    "query_time_ms": 45,
    "embedding_provider": "openai"
  }
}
```

### POST /v1/memory/consult

SME-style consultation with evidence-based answers.

**Request Body**:

```json
{
  "space_id": "my-project",
  "context": "Investigating authentication flow",
  "question": "What is the session timeout value?",
  "max_suggestions": 5,
  "include_evidence": true,
  "translate_intent": true
}
```

**Response**:

```json
{
  "answer": "The session timeout is configured to 3600 seconds (1 hour) in src/config/auth.ts:15",
  "confidence": "HIGH",
  "suggestions": [
    {
      "node_id": "mem-abc123",
      "relevance": 0.92,
      "evidence": [...]
    }
  ],
  "translated_intent": "session timeout configuration value auth settings"
}
```

When `translate_intent: true`, the response includes `translated_intent` showing the keyword-dense rewrite used for vector embedding. Requires `INTENT_ENABLED=true` server config. Falls back silently to original query on error.

### POST /v1/memory/suggest

Context-triggered suggestions (proactive memory surfacing).

**Request Body**:

```json
{
  "space_id": "my-project",
  "context": "User is editing authentication code",
  "max_suggestions": 3,
  "translate_intent": true
}
```

**Response**:

```json
{
  "suggestions": [
    {
      "node_id": "mem-abc123",
      "name": "Security best practices",
      "relevance": 0.88,
      "reason": "Related to authentication patterns"
    }
  ],
  "translated_intent": "authentication code editing security patterns session management"
}
```

When `translate_intent: true`, the response includes `translated_intent` showing the keyword-dense rewrite. Requires `INTENT_ENABLED=true`. Falls back silently on error.

### POST /v1/memory/reflect

Deep exploration of a topic.

**Request Body**:

```json
{
  "space_id": "my-project",
  "topic": "database connection patterns",
  "depth": 3
}
```

### GET /v1/memory/symbols

Search the symbol store by name, type, file path, or fulltext query.

**Query Parameters**:

| Param | Type | Description |
|-------|------|-------------|
| `space_id` | string | Required. Memory space identifier |
| `q` | string | Fulltext search query (takes priority over `name`) |
| `name` | string | Symbol name pattern (supports `*` wildcard suffix for prefix match) |
| `type` | string | Symbol type filter (`const`, `var`, `function`, `class`, etc.) |
| `file` | string | File path filter |
| `exported` | bool | Filter by export status |
| `limit` | int | Max results (default 50, max 500) |

At least one of `name`, `type`, `file`, or `q` is required.

**Response**:

```json
{
  "space_id": "my-space",
  "symbols": [
    {
      "name": "MyFunc",
      "type": "function",
      "file_path": "internal/foo/bar.go",
      "line": 42,
      "line_end": 55,
      "exported": true,
      "signature": "func MyFunc(x int) error",
      "doc_comment": "MyFunc does..."
    }
  ],
  "count": 1
}
```

---

## Consolidation

### POST /v1/memory/consolidate

Trigger hidden layer creation (concept abstraction).

**Request Body**:

```json
{
  "space_id": "my-project",
  "skip_clustering": false,
  "skip_forward": false,
  "skip_backward": false,
  "enable_dynamic_emergence": false,
  "min_cluster_density": 0.0
}
```

**Response**:

```json
{
  "data": {
    "space_id": "my-project",
    "enabled": true,
    "steps": {
      "hidden":     { "nodes_created": 45, "nodes_updated": 0, "edges_created": 0 },
      "concern":    { "nodes_created": 3,  "nodes_updated": 0, "edges_created": 8, "details": {"concerns": ["auth", "logging"]} },
      "config":     { "nodes_created": 1,  "nodes_updated": 0, "edges_created": 5 },
      "comparison": { "nodes_created": 8,  "nodes_updated": 0, "edges_created": 12, "details": {"modules_compared": 4} },
      "temporal":   { "nodes_created": 1,  "nodes_updated": 0, "edges_created": 3, "details": {"patterns_detected": ["validFrom/validTo"]} },
      "ui":         { "nodes_created": 2,  "nodes_updated": 0, "edges_created": 6, "details": {"patterns_detected": ["store", "component"]} },
      "constraint":         { "nodes_created": 1,  "nodes_updated": 0, "edges_created": 1 },
      "dynamic_emergence":  { "nodes_created": 2,  "nodes_updated": 0, "edges_created": 0 },
      "dynamic_edges":      { "nodes_created": 0,  "nodes_updated": 0, "edges_created": 50 },
      "emergent_l5":        { "nodes_created": 4,  "nodes_updated": 0, "edges_created": 4 }
    },
    "hidden_nodes_created": 45,
    "hidden_nodes_updated": 150,
    "concept_nodes_created": 12,
    "concept_nodes_merged": 0,
    "concept_nodes_updated": 25,
    "edges_strengthened": 230,
    "summaries_generated": 57,
    "edges_refreshed": 0,
    "duration_ms": 12500,
    "concern_nodes_created": 3,
    "concern_edges_created": 8,
    "config_node_created": true,
    "config_edges_created": 5,
    "comparison_nodes_created": 8,
    "comparison_edges_created": 12,
    "temporal_node_created": true,
    "temporal_edges_created": 3,
    "ui_nodes_created": 2,
    "ui_edges_created": 6,
    "constraint_nodes_created": 1,
    "constraint_nodes_updated": 0,
    "constraint_edges_linked": 1,
    "dynamic_edges_created": 50,
    "l5_nodes_created": 4,
    "dynamic_emergent_nodes_created": 2
  }
}
```

> **Phase 46 (Dynamic Pipeline Registry):** The `steps` map is populated dynamically by the pipeline registry. Each registered `NodeCreator` step produces a `StepResult` entry keyed by step name. The flat fields (e.g., `concern_nodes_created`) are preserved for backward compatibility and populated from the same pipeline results. New steps added to the pipeline automatically appear in the `steps` map without API changes. See [REGISTRY.md](REGISTRY.md) for details.
>
> **Phase 75C (Split Execution):** The pipeline now supports `RunPhaseRange()` for selective phase execution. Pre-clustering steps (phases 10-22) run before multi-layer clustering, while post-clustering steps (phases 25-30: `dynamic_edges`, `emergent_l5`) run after clustering completes. This ensures dynamic edges and L5 nodes are created with full clustering context.
>
> **Phase 103 (Dynamic Emergence):** The `dynamic_emergence` step (phase 22) runs after hardcoded pattern steps (phase 20) and before dynamic edges (phase 25). It detects dense `CO_ACTIVATED_WITH` clusters that don't match known patterns and sends them to an LLM for naming. Enabled via `enable_dynamic_emergence: true` in the request body and `EMERGENCE_ENABLED=true` in config. Creates `:MemoryNode:EmergentConcept` nodes with `role_type: 'dynamic_emergent'`.

---

## Learning System

### GET /v1/learning/stats?space_id={space_id}

Get Hebbian learning edge statistics.

**Response**:

```json
{
  "space_id": "my-project",
  "total_edges": 1250,
  "avg_weight": 0.42,
  "max_weight": 0.95,
  "decay_per_day": 0.01,
  "prune_threshold": 0.1,
  "max_edges_per_node": 50
}
```

### POST /v1/learning/prune?space_id={space_id}

Prune decayed and excess learning edges.

**Response**:

```json
{
  "space_id": "my-project",
  "decayed_deleted": 45,
  "excess_deleted": 12,
  "total_deleted": 57
}
```

### POST /v1/learning/freeze

Freeze Hebbian learning edge creation/updates for a space. When frozen, no new `CO_ACTIVATED_WITH` edges are created. Useful for stable scoring during benchmarks.

**Request Body**:

```json
{
  "space_id": "my-project",
  "reason": "stable scoring for benchmark",
  "frozen_by": "claude"
}
```

**Response**:

```json
{
  "space_id": "my-project",
  "status": "frozen",
  "state": {
    "frozen": true,
    "frozen_at": "2026-02-24T10:00:00Z",
    "reason": "stable scoring for benchmark",
    "frozen_by": "claude"
  },
  "message": "Learning has been frozen for this space. No new edges will be created."
}
```

### POST /v1/learning/unfreeze

Resume Hebbian learning edge creation/updates for a previously frozen space.

**Request Body**:

```json
{
  "space_id": "my-project"
}
```

**Response**:

```json
{
  "space_id": "my-project",
  "status": "unfrozen",
  "state": {
    "frozen": false
  },
  "message": "Learning has been resumed for this space."
}
```

### GET /v1/learning/freeze/status

Get freeze state for a space (or all spaces if `space_id` is omitted).

**Query Parameters**:

- `space_id` (optional): If provided, returns status for that space only

**Response** (single space):

```json
{
  "space_id": "my-project",
  "state": {
    "frozen": true,
    "frozen_at": "2026-02-24T10:00:00Z",
    "reason": "stable scoring for benchmark",
    "frozen_by": "claude"
  }
}
```

**Response** (all spaces):

```json
{
  "frozen_spaces": {
    "my-project": {
      "frozen": true,
      "frozen_at": "2026-02-24T10:00:00Z",
      "reason": "benchmark"
    }
  },
  "count": 1
}
```

### POST /v1/learning/negative-feedback

Apply negative feedback to weaken or contradict learning edges for rejected retrieval results.

**Request Body**:

```json
{
  "space_id": "my-project",
  "query_node_ids": ["mem-abc123"],
  "rejected_node_ids": ["mem-def456", "mem-ghi789"]
}
```

- `query_node_ids`: Node IDs from the original query/context
- `rejected_node_ids`: Node IDs that were returned but deemed irrelevant (max 20 per request)

**Response (200)**:

```json
{
  "processed": 2,
  "weakened": 1,
  "contradicted": 1
}
```

- `weakened`: Existing `CO_ACTIVATED_WITH` edges had weight reduced by `LEARNING_NEGATIVE_WEIGHT` (default 0.15)
- `contradicted`: New `CONTRADICTS` edges created (or `evidence_count` incremented on existing ones)

**Errors**: 400 (missing `space_id`, empty arrays), 405 (wrong method).

**Config**: `LEARNING_NEGATIVE_WEIGHT` (0.15), `LEARNING_NEGATIVE_DECAY_MULT` (2.0), `LEARNING_NEGATIVE_MAX_PER_REQUEST` (20).

---

## Conversation Memory

Endpoints for the Conversation Memory System (CMS) - capturing, recalling, and managing conversational knowledge.

### POST /v1/conversation/observe

Capture a significant observation with auto-surprise scoring.

**Request Body**:

```json
{
  "space_id": "my-project",
  "session_id": "session-abc123",
  "content": "User prefers TypeScript over JavaScript",
  "obs_type": "preference",
  "tags": ["coding-style"],
  "metadata": {"context": "discussion about language choice"},
  "user_id": "alice",
  "visibility": "team",
  "refers_to": ["sym-validateInput-xyz"]
}
```

**Fields**:

- `obs_type`: `decision`, `learning`, `preference`, `error`, `task`, `correction`
- `visibility`: `private` (owner only), `team` (space members), `global` (everyone, default)
- `refers_to`: Array of node IDs to create REFERS_TO edges

**Response**:

```json
{
  "obs_id": "obs-abc123",
  "node_id": "mem-xyz789",
  "surprise_score": 0.65,
  "surprise_factors": {
    "term_novelty": 0.7,
    "correction_score": 0.0,
    "embedding_novelty": 0.6
  },
  "summary": "User preference for TypeScript"
}
```

### POST /v1/conversation/correct

Capture an explicit user correction (high surprise, persistent).

**Request Body**:

```json
{
  "space_id": "my-project",
  "session_id": "session-abc123",
  "incorrect": "The ORM is Hibernate",
  "correct": "The ORM is BlueSeerData, a custom framework",
  "context": "User corrected my assumption about the database layer",
  "user_id": "alice",
  "visibility": "global"
}
```

**Response**: Same as `/observe` with higher surprise score (baseline 0.5).

### POST /v1/conversation/resume

Restore context after context compaction.

**Request Body**:

```json
{
  "space_id": "my-project",
  "session_id": "session-abc123",
  "include_tasks": true,
  "include_decisions": true,
  "include_learnings": true,
  "max_observations": 20,
  "requesting_user_id": "alice"
}
```

**Response**:

```json
{
  "space_id": "my-project",
  "session_id": "session-abc123",
  "observations": [
    {
      "node_id": "mem-obs1",
      "obs_type": "decision",
      "content": "Using plugin architecture",
      "summary": "Architecture decision",
      "surprise_score": 0.5,
      "created_at": "2026-01-27T10:00:00Z"
    }
  ],
  "themes": [...],
  "emergent_concepts": [...],
  "jiminy": {
    "rationale": "Restoring 15 observations from session...",
    "confidence": 0.72,
    "score_breakdown": {
      "observation_coverage": 0.85,
      "theme_relevance": 0.65,
      "recency_boost": 0.70
    },
    "highlights": ["Decision: Using plugin architecture"]
  },
  "summary": "Resuming with 15 observations..."
}
```

### POST /v1/conversation/recall

Retrieve relevant conversation knowledge via semantic query.

**Request Body**:

```json
{
  "space_id": "my-project",
  "query": "What do I know about user preferences?",
  "top_k": 10,
  "include_themes": true,
  "include_concepts": true,
  "requesting_user_id": "alice"
}
```

**Response**:

```json
{
  "space_id": "my-project",
  "query": "What do I know about user preferences?",
  "results": [
    {
      "type": "emergent_concept",
      "node_id": "concept-123",
      "content": "User prefers modular architecture",
      "score": 0.85,
      "layer": 2
    }
  ]
}
```

### POST /v1/conversation/consolidate

Trigger consolidation to form themes and emergent concepts.

**Request Body**:

```json
{
  "space_id": "my-project"
}
```

**Response**:

```json
{
  "space_id": "my-project",
  "themes_created": 3,
  "concepts_created": 1,
  "duration_ms": 1250
}
```

### GET /v1/conversation/volatile/stats

Get statistics about volatile (ungraduated) observations.

**Query Parameters**:

- `space_id` (required)

**Response**:

```json
{
  "space_id": "my-project",
  "volatile_count": 15,
  "permanent_count": 42,
  "avg_volatile_stability": 0.35,
  "min_volatile_stability": 0.10,
  "max_volatile_stability": 0.72
}
```

### POST /v1/conversation/graduate

Manually trigger graduation processing for the Context Cooler.

**Request Body**:

```json
{
  "space_id": "my-project"
}
```

**Response**:

```json
{
  "space_id": "my-project",
  "timestamp": "2026-01-27T12:00:00Z",
  "graduated": 3,
  "tombstoned": 1,
  "remaining_volatile": 11,
  "decay_applied": 5
}
```

### GET /v1/conversation/session/health

CMS usage health metrics for a specific conversation session.

**Query Parameters**:

- `session_id` (required): Session identifier

**Response**:

```json
{
  "session_id": "claude-core",
  "space_id": "mdemg-dev",
  "resumed": true,
  "observations_since_resume": 14,
  "health_score": 0.82,
  "tracked": true,
  "last_resume_at": "2026-02-24T10:00:00Z",
  "last_observe_at": "2026-02-24T10:45:00Z",
  "last_activity_at": "2026-02-24T10:45:00Z"
}
```

If the session is not tracked, returns `"tracked": false` with zero values.

---

## Constraints (Phase 45.5)

Constraint nodes represent organizational rules extracted from observations during consolidation.

### GET /v1/constraints

List all active (non-archived) constraint nodes for a space.

**Query Parameters**:

- `space_id` (required)

**Response**:

```json
{
  "space_id": "my-project",
  "constraints": [
    {
      "node_id": "node-abc",
      "name": "no-direct-neo4j-in-handlers",
      "constraint_type": "architectural",
      "content": "Handlers should not contain direct Neo4j queries...",
      "confidence": 0.95,
      "created_at": "2026-02-20T10:00:00Z",
      "updated_at": "2026-02-20T10:00:00Z",
      "source_count": 3
    }
  ]
}
```

### GET /v1/constraints/stats

Summary statistics about constraints grouped by type.

**Query Parameters**:

- `space_id` (required)

**Response**:

```json
{
  "space_id": "my-project",
  "total_constraint_nodes": 12,
  "by_type": [
    {
      "constraint_type": "architectural",
      "count": 7,
      "avg_confidence": 0.88
    }
  ],
  "tagged_observation_count": 42
}
```

---

## Active Guardrails (Phase 104)

Validates proposed code changes against active constraint nodes. Returns Pass/Warning/Block status. Fail-open: if any pipeline step fails, returns Pass with a warning.

**Configuration**: `GUARDRAIL_ENABLED=true` required. Returns 503 when disabled.

### POST /v1/memory/guardrail/validate

Validate a proposed code change against active constraints in a space.

**Request Body**:

```json
{
  "space_id": "my-project",
  "files_changed": ["src/api/auth.go"],
  "diff": "@@ -15,7 +15,7 @@\n func Login(c *gin.Context) {\n-    token := jwt.New()\n+    token := legacy_auth.GenerateToken()\n }"
}
```

**Fields**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `space_id` | string | Yes | Memory space with constraint nodes |
| `files_changed` | []string | Yes | List of changed file paths |
| `diff` | string | Yes | Unified diff of proposed changes |

**Response** (200):

```json
{
  "data": {
    "status": "Warning",
    "violations": [],
    "warnings": [
      {
        "constraint_node_id": "node-abc",
        "description": "Change uses legacy authentication library",
        "rationale": "Constraint 'prefer-jwt-auth' (should) recommends using jwt package"
      }
    ]
  }
}
```

**Status Values**:

| Status | Meaning |
|--------|---------|
| `Pass` | No violations or warnings |
| `Warning` | Only `should`/`should_not` constraints triggered |
| `Block` | At least one `must`/`must_not` constraint violated |

**Error Codes**: `400` (missing required fields), `405` (not POST), `503` (guardrail not enabled).

---

## Jiminy Guidance Service (Phase Jiminy)

Jiminy is an active inner-voice service that provides proactive, context-aware guidance to coding agents. It orchestrates multiple knowledge sources (constraints, corrections, contradictions, patterns, frontiers) from MDEMG's knowledge graph and formats them as injectable prompt augmentation.

**Configuration**: `JIMINY_ENABLED=true` required. See also `JIMINY_TIMEOUT_MS`, `JIMINY_MAX_ITEMS`, `JIMINY_MIN_CONFIDENCE`, `JIMINY_INCLUDE_FRONTIERS`, `JIMINY_FRONTIER_MIN_SIM`.

### POST /v1/jiminy/guide

Generate proactive guidance for the current working context.

**Request Body**:

```json
{
  "space_id": "mdemg-dev",
  "context": "Refactoring authentication middleware to use new JWT library",
  "file_path": "internal/auth/middleware.go",
  "agent_output": "func NewAuthMiddleware(secret string) gin.HandlerFunc { ... }",
  "session_id": "claude-core",
  "max_items": 10
}
```

**Fields**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `space_id` | string | Yes | Memory space to query |
| `context` | string | Yes | What the agent is currently working on |
| `file_path` | string | No | Path of the file being edited |
| `agent_output` | string | No | Proposed code or action for review |
| `query` | string | No | User's original query |
| `session_id` | string | No | Session ID for correction lookup |
| `max_items` | int | No | Max guidance items (default: 10) |

**Response** (200):

```json
{
  "data": {
    "guidance": [
      {
        "type": "constraint",
        "priority": "high",
        "content": "[must_not] Never use deprecated auth middleware",
        "confidence": 0.92,
        "source_nodes": ["node-abc"]
      }
    ],
    "prompt_augmentation": "═══ JIMINY GUIDANCE ═══\nCONSTRAINTS:\n  • ...\n═══ END JIMINY GUIDANCE ═══",
    "confidence": 0.82,
    "rationale": "Found 3 guidance items: 2 constraints, 1 correction",
    "source_counts": {
      "constraints": 2,
      "corrections": 1,
      "patterns": 0,
      "conflicts": 0,
      "frontiers": 0
    }
  }
}
```

**Guidance Types**: `constraint`, `correction`, `pattern`, `conflict`, `risk`, `suggestion`, `frontier`.

**Error Codes**: `400` (missing space_id or context), `405` (not POST), `503` (Jiminy not enabled).

**Note**: The response now includes a `guidance_id` field (UUID) in the `data` object for effectiveness tracking. Pass this ID to the feedback endpoint to record whether guidance was followed.

### POST /v1/jiminy/feedback

Record whether an agent followed, ignored, or contradicted Jiminy guidance. This closes the guidance effectiveness feedback loop.

**Request Body**:

```json
{
  "guidance_id": "c6fa606f-6624-4202-8bd3-2a265ba24a44",
  "action_summary": "I validated the input before processing it",
  "space_id": "mdemg-dev"
}
```

**Fields**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `guidance_id` | string | Yes | The guidance_id returned from `/v1/jiminy/guide` |
| `action_summary` | string | No | Description of the action the agent actually took |
| `space_id` | string | No | Memory space (for context) |

**Response** (200):

```json
{
  "data": {
    "guidance_id": "c6fa606f-6624-4202-8bd3-2a265ba24a44",
    "applied": true,
    "results": [
      {
        "type": "constraint",
        "content": "[must] Always validate input",
        "outcome": "followed",
        "similarity": 0.42
      }
    ]
  }
}
```

**Outcome Values**: `followed` (action matches guidance), `ignored` (low similarity to all items), `contradicted` (action opposes guidance with negation detected), `unknown` (no action_summary provided or no tracked items).

**Error Codes**: `400` (missing guidance_id), `405` (not POST), `503` (Jiminy not enabled).

**Config**: `JIMINY_EFFECTIVENESS_ENABLED` (default: true), `JIMINY_EFFECTIVENESS_TTL_SEC` (default: 1800 — guidance tracking expires after 30 minutes).

### Hook Distribution (J6b-J6e)

Jiminy hooks can be installed into Claude Code projects via the CLI:

```bash
# Install Claude Code hooks (prompt-context + session-start)
mdemg hooks install --type claude

# Install with custom space ID and endpoint
mdemg hooks install --type claude --space-id my-project

# Uninstall Claude Code hooks
mdemg hooks uninstall --type claude
```

This embeds parameterized hook scripts (`.sh` on Unix, `.ps1` on Windows) into `.claude/hooks/` and registers them in `.claude/settings.local.json`. Templates use `{{SPACE_ID}}` and `{{MDEMG_URL}}` placeholders substituted at install time. Existing user settings are preserved via `mergeClaudeSettings()` (J6e).

`mdemg init` auto-installs Claude Code hooks when a `.claude/` directory is detected (J6c). In `--defaults`/`--quick` mode, hooks are installed non-interactively.

---

## Skill Registry (Phase 48)

Skills are CMS pinned observations with `skill:<name>` tags. The Skill Registry API provides convenience endpoints for listing, recalling, and registering skills.

### GET /v1/skills?space_id={space_id}

List all registered skills discovered from pinned observations with `skill:*` tags.

**Response:**

```json
{
  "space_id": "mdemg-dev",
  "skills": [
    {
      "name": "mdemg-api",
      "description": "# CMS Endpoints (Conversation Memory System)...",
      "sections": ["cms", "memory", "learning", "retrieval", "workflows", "system"],
      "observation_count": 6
    }
  ],
  "count": 1
}
```

### POST /v1/skills/{name}/recall

Recall skill content by tag. Uses direct Cypher query (not vector search) for reliable tag-based retrieval.

**Request:**

```json
{
  "space_id": "mdemg-dev",
  "section": "cms",
  "top_k": 10
}
```

**Response:**

```json
{
  "space_id": "mdemg-dev",
  "skill": "mdemg-api",
  "section": "cms",
  "query": "skill mdemg-api instructions",
  "results": [
    {
      "type": "conversation_observation",
      "node_id": "abc-123",
      "content": "# CMS Endpoints...",
      "score": 1.0,
      "layer": 0
    }
  ],
  "debug": {"tag_filter": "skill:mdemg-api:cms", "observation_count": 1}
}
```

### POST /v1/skills/{name}/register

Register skill sections as pinned observations. Each section becomes a permanent, non-decaying observation with `skill:<name>` and `skill:<name>:<section>` tags.

**Request:**

```json
{
  "space_id": "mdemg-dev",
  "session_id": "skill-registry",
  "description": "MDEMG API reference",
  "sections": [
    {
      "name": "cms",
      "content": "# CMS Endpoints...",
      "tags": ["api-reference"]
    }
  ]
}
```

**Response:**

```json
{
  "skill": "mdemg-api",
  "space_id": "mdemg-dev",
  "sections_created": 1,
  "observation_ids": ["abc-123"]
}
```

---

## CMS Advanced Functionality (Phase 60)

Advanced CMS features including observation templates, task snapshots, relevance scoring, smart truncation, and org-level review workflows.

### Observation Templates

Templates provide structured schemas for consistent observation capture with JSON Schema validation.

#### GET /v1/conversation/templates

List all observation templates for a space.

**Query Parameters**:

- `space_id` (required): The memory space

**Response**:

```json
{
  "templates": [
    {
      "template_id": "task_handoff",
      "space_id": "mdemg-dev",
      "name": "Task Handoff",
      "description": "Capture task state for session continuity",
      "obs_type": "context",
      "created_at": "2026-02-07T10:00:00Z"
    }
  ],
  "count": 1
}
```

#### POST /v1/conversation/templates

Create a new observation template.

**Request Body**:

```json
{
  "space_id": "mdemg-dev",
  "template_id": "task_handoff",
  "name": "Task Handoff",
  "description": "Capture task state for session continuity",
  "obs_type": "context",
  "schema": {
    "type": "object",
    "required": ["task_name", "status"],
    "properties": {
      "task_name": {"type": "string"},
      "status": {"type": "string", "enum": ["in_progress", "blocked", "completed"]}
    }
  }
}
```

**Response**:

```json
{
  "template_id": "task_handoff",
  "space_id": "mdemg-dev",
  "name": "Task Handoff",
  "created_at": "2026-02-07T10:00:00Z"
}
```

#### GET /v1/conversation/templates/{template_id}

Get a specific template by ID.

**Query Parameters**:

- `space_id` (required): The memory space

**Response**: Same as single template in list response.

#### PUT /v1/conversation/templates/{template_id}

Update an existing template.

**Request Body**: Same as create (all fields optional except `space_id`).

**Response**:

```json
{
  "template_id": "task_handoff",
  "updated": true,
  "updated_at": "2026-02-07T11:00:00Z"
}
```

#### DELETE /v1/conversation/templates/{template_id}

Delete a template.

**Query Parameters**:

- `space_id` (required): The memory space

**Response**:

```json
{
  "template_id": "task_handoff",
  "deleted": true
}
```

### Task Context Snapshots

Snapshots capture task state for session continuity, triggered manually or automatically on session end/compaction.

#### GET /v1/conversation/snapshot

List snapshots for a session.

**Query Parameters**:

- `space_id` (required)
- `session_id` (optional): Filter by session
- `limit` (optional): Max results (default: 50)

**Response**:

```json
{
  "snapshots": [
    {
      "snapshot_id": "snap-abc123",
      "space_id": "mdemg-dev",
      "session_id": "session-123",
      "trigger": "manual",
      "context": {
        "task_name": "Phase 60 implementation",
        "active_files": ["service.go"]
      },
      "created_at": "2026-02-07T10:00:00Z"
    }
  ],
  "count": 1
}
```

#### POST /v1/conversation/snapshot

Create a new task context snapshot.

**Request Body**:

```json
{
  "space_id": "mdemg-dev",
  "session_id": "session-123",
  "trigger": "manual",
  "context": {
    "task_name": "Phase 60 implementation",
    "active_files": ["service.go", "types.go"],
    "current_goal": "Add templates",
    "recent_tool_calls": ["Read", "Edit", "Bash"],
    "pending_items": ["Create tests", "Update docs"]
  }
}
```

**Response**:

```json
{
  "snapshot_id": "snap-abc123",
  "space_id": "mdemg-dev",
  "session_id": "session-123",
  "trigger": "manual",
  "created_at": "2026-02-07T10:00:00Z"
}
```

#### GET /v1/conversation/snapshot/{snapshot_id}

Get a specific snapshot.

**Query Parameters**:

- `space_id` (required)

**Response**: Same as single snapshot in list response.

#### GET /v1/conversation/snapshot/latest

Get the most recent snapshot for a session.

**Query Parameters**:

- `space_id` (required)
- `session_id` (required)

**Response**: Single snapshot object.

#### DELETE /v1/conversation/snapshot/{snapshot_id}

Delete a snapshot.

**Query Parameters**:

- `space_id` (required)

**Response**:

```json
{
  "snapshot_id": "snap-abc123",
  "deleted": true
}
```

#### POST /v1/conversation/snapshot/cleanup

Clean up old snapshots for a space.

**Request Body**:

```json
{
  "space_id": "mdemg-dev",
  "keep_count": 10,
  "older_than_days": 7
}
```

**Response**:

```json
{
  "space_id": "mdemg-dev",
  "deleted_count": 5
}
```

### Org-Level Review

Workflow for flagging observations for org-level review before promotion to team/global visibility.

#### GET /v1/conversation/org-reviews

List observations pending org-level review.

**Query Parameters**:

- `space_id` (required)
- `limit` (optional): Max results (default: 50, max: 500)

**Response**:

```json
{
  "reviews": [
    {
      "obs_id": "obs-abc123",
      "space_id": "mdemg-dev",
      "content": "Architectural decision about...",
      "obs_type": "decision",
      "flagged_at": "2026-02-07T10:00:00Z",
      "flagged_by": "agent-claude",
      "suggested_visibility": "team",
      "flag_reason": "Valuable for team reference"
    }
  ],
  "count": 1
}
```

#### GET /v1/conversation/org-reviews/stats

Get review statistics for a space.

**Query Parameters**:

- `space_id` (required)

**Response**:

```json
{
  "pending": 5,
  "approved": 42,
  "rejected": 3
}
```

#### POST /v1/conversation/observations/{obs_id}/flag

Flag an observation for org-level review.

**Request Body**:

```json
{
  "obs_id": "obs-abc123",
  "space_id": "mdemg-dev",
  "reason": "Valuable architectural decision for team reference",
  "suggested_visibility": "team",
  "flagged_by": "agent-claude"
}
```

**Response**:

```json
{
  "obs_id": "obs-abc123",
  "flagged_for_review": true,
  "review_status": "pending",
  "flagged_at": "2026-02-07T10:00:00Z",
  "flagged_by": "agent-claude"
}
```

#### POST /v1/conversation/org-reviews/{obs_id}/decision

Process an approve/reject decision on a flagged observation.

**Request Body**:

```json
{
  "obs_id": "obs-abc123",
  "space_id": "mdemg-dev",
  "decision": "approve",
  "visibility": "team",
  "reviewed_by": "user@example.com",
  "notes": "Good addition to team knowledge"
}
```

**Response**:

```json
{
  "obs_id": "obs-abc123",
  "decision": "approve",
  "new_visibility": "team",
  "reviewed_at": "2026-02-07T11:00:00Z",
  "reviewed_by": "user@example.com"
}
```

---

## Capability Gaps

Endpoints for capability gap detection and interview prompts.

### GET /v1/system/capability-gaps

List all capability gaps.

**Query Parameters**:

- `status`: `open`, `addressed`, `dismissed`
- `type`: `data_source`, `reasoning`, `query_pattern`
- `space_id`: Filter by space

**Response**:

```json
{
  "data": {
    "gaps": [
      {
        "id": "gap-abc123",
        "type": "data_source",
        "description": "Content references Slack but no integration exists",
        "evidence": ["slack"],
        "suggested_plugin": {
          "type": "INGESTION",
          "name": "slack-ingestion",
          "description": "Ingest Slack messages and channels"
        },
        "priority": 0.85,
        "status": "open",
        "occurrence_count": 15
      }
    ],
    "summary": {
      "total": 5,
      "by_type": {"data_source": 3, "reasoning": 2},
      "high_priority": 2
    }
  }
}
```

### GET /v1/system/gap-interviews

Get pending interview prompts.

**Query Parameters**:

- `space_id` (optional)

**Response**:

```json
{
  "prompts": [
    {
      "id": "interview-abc123",
      "gap_id": "gap-xyz789",
      "gap_type": "data_source",
      "question": "How should MDEMG integrate with Slack?",
      "context": "This gap has been detected 15 times.",
      "suggestions": ["Create slack-ingestion plugin", "Configure webhook"],
      "priority": 0.85,
      "status": "pending"
    }
  ],
  "stats": {
    "total": 10,
    "pending": 5,
    "answered": 4,
    "skipped": 1
  }
}
```

### POST /v1/system/gap-interviews/run

Manually trigger the weekly gap interview process.

**Request Body** (optional):

```json
{
  "max_prompts": 10,
  "min_priority": 0.3,
  "min_occurrences": 3
}
```

**Response**:

```json
{
  "total_gaps_analyzed": 8,
  "prompts_generated": 5,
  "high_priority_count": 2,
  "gaps_by_type": {"data_source": 3, "reasoning": 2},
  "processed_at": "2026-01-27T12:00:00Z",
  "next_scheduled_at": "2026-02-03T12:00:00Z"
}
```

### POST /v1/system/gap-interviews/{id}/answer

Mark an interview prompt as answered.

**Request Body**:

```json
{
  "observation_node_id": "obs-abc123"
}
```

### POST /v1/system/gap-interviews/{id}/skip

Skip an interview prompt.

**Request Body**:

```json
{
  "reason": "Not relevant to this project"
}
```

---

## Linear Integration

CRUD operations for Linear issues, projects, and comments. Requires the `linear-module` plugin to be running.

### POST /v1/linear/issues

Create a new Linear issue.

**Request Body**:

```json
{
  "title": "Fix login bug",
  "team_id": "team-uuid",
  "description": "Users cannot log in on mobile",
  "priority": 2,
  "assignee_id": "user-uuid",
  "project_id": "project-uuid",
  "label_ids": ["label-uuid-1", "label-uuid-2"]
}
```

**Required fields**: `title`, `team_id`

**Response**:

```json
{
  "id": "issue-uuid",
  "identifier": "ENG-213",
  "title": "Fix login bug",
  "state": "Triage",
  "team_key": "ENG",
  "priority": "2",
  "created_at": "2026-02-03T10:00:00Z"
}
```

### GET /v1/linear/issues

List Linear issues with optional filters.

**Query Parameters**:

| Param | Type | Description |
|-------|------|-------------|
| `team` | string | Filter by team key (e.g., "ENG") |
| `state` | string | Filter by state name (e.g., "In Progress") |
| `assignee` | string | Filter by assignee name |
| `label` | string | Filter by label name |
| `limit` | int | Max results (default: 50) |
| `cursor` | string | Pagination cursor |

**Response**:

```json
{
  "issues": [
    {
      "id": "issue-uuid",
      "identifier": "ENG-213",
      "title": "Fix login bug",
      "state": "In Progress",
      "priority": "2"
    }
  ],
  "next_cursor": "cursor-string",
  "has_more": true
}
```

### GET /v1/linear/issues/{id}

Read a single issue by ID.

**Response**: Same shape as individual issue in list response, with all fields populated.

### PUT /v1/linear/issues/{id}

Update an existing issue. Only provided fields are modified.

**Request Body**:

```json
{
  "title": "Updated title",
  "priority": 1,
  "state_id": "state-uuid",
  "assignee_id": "user-uuid"
}
```

**Response**: Updated issue fields.

### DELETE /v1/linear/issues/{id}

Archive (soft-delete) an issue.

**Response**:

```json
{
  "success": true
}
```

### POST /v1/linear/projects

Create a new Linear project.

**Request Body**:

```json
{
  "name": "Q1 Sprint",
  "description": "First quarter deliverables"
}
```

**Required fields**: `name`

### GET /v1/linear/projects

List Linear projects.

**Query Parameters**: `limit`, `cursor`

### GET /v1/linear/projects/{id}

Read a single project by ID.

### PUT /v1/linear/projects/{id}

Update an existing project.

### POST /v1/linear/comments

Add a comment to an issue.

**Request Body**:

```json
{
  "issue_id": "issue-uuid",
  "body": "This needs attention."
}
```

**Required fields**: `issue_id`, `body`

**Response**:

```json
{
  "id": "comment-uuid",
  "body": "This needs attention.",
  "issue_id": "issue-uuid",
  "created_at": "2026-02-03T10:05:00Z"
}
```

---

## Web Scraper

Web content scraping and ingestion pipeline.

### POST /v1/scraper/jobs

Create a new scrape job.

**Request Body**:

```json
{
  "urls": ["https://example.com/docs"],
  "target_space_id": "my-space",
  "options": {
    "extraction_profile": "documentation",
    "max_depth": 3,
    "max_pages": 50,
    "follow_links": true,
    "delay_ms": 500,
    "timeout_ms": 30000
  }
}
```

**Required**: `urls`

**Response** (202 Accepted):

```json
{
  "job_id": "scrape-a1b2c3d4",
  "status": "pending",
  "urls": ["https://example.com/docs"],
  "target_space_id": "my-space",
  "total_urls": 1,
  "processed_urls": 0,
  "created_at": "2026-02-24T10:00:00Z"
}
```

### GET /v1/scraper/jobs

List all scrape jobs.

**Response**:

```json
{
  "jobs": [ /* array of ScrapeJobResponse */ ],
  "count": 3
}
```

### GET /v1/scraper/jobs/{id}

Get scrape job details including scraped content.

**Response**:

```json
{
  "job_id": "scrape-a1b2c3d4",
  "status": "completed",
  "contents": [
    {
      "content_id": "cid-123",
      "url": "https://example.com/docs",
      "title": "My Docs",
      "content_preview": "...",
      "quality_score": 0.91,
      "suggested_tags": ["go", "api"],
      "status": "pending_review",
      "word_count": 1200
    }
  ]
}
```

### DELETE /v1/scraper/jobs/{id}

Cancel a scrape job.

**Response**: `{ "job_id": "...", "status": "cancelled", "message": "job cancelled" }`

### POST /v1/scraper/jobs/{id}/review

Review and approve/reject scraped content for ingestion.

**Request Body**:

```json
{
  "decisions": [
    {
      "content_id": "cid-123",
      "action": "approve",
      "space_id": "override-space"
    }
  ]
}
```

**Actions**: `approve` (ingest into memory), `reject` (discard), `edit` (modify then ingest, requires `edit_content`).

**Response**:

```json
{
  "job_id": "scrape-a1b2c3d4",
  "reviewed": 2,
  "ingested": [{ "content_id": "cid-123", "node_id": "mem-abc", "url": "https://..." }],
  "rejected": 1,
  "status": "completed"
}
```

### GET /v1/scraper/spaces

List all available target spaces with node counts.

**Response**:

```json
{
  "spaces": [
    { "space_id": "my-space", "node_count": 1500 }
  ],
  "count": 1
}
```

---

## Webhooks

### POST /v1/webhooks/linear

Receives Linear webhook events and ingests them as observations via the `linear-module` plugin.

**Authentication:** HMAC-SHA256 signature verification via the `Linear-Signature` header.

**Environment variables:**

- `LINEAR_WEBHOOK_SECRET` — HMAC-SHA256 signing secret (required)
- `LINEAR_WEBHOOK_SPACE_ID` — Target space ID for ingested observations (required)

**Supported events:**

- `Issue` create/update
- `Project` update

Other event types are acknowledged with 200 but ignored.

**Debouncing:** Rapid events for the same entity are coalesced with a 10-second window.

**Request headers:**

- `Linear-Signature` — HMAC-SHA256 hex digest of the request body

**Response:**

```json
{
  "status": "accepted",
  "type": "Issue",
  "action": "create",
  "debounce": "Issue:ISS-123"
}
```

**Error responses:**

- `401 Unauthorized` — Missing or invalid signature
- `405 Method Not Allowed` — Non-POST request
- `500 Internal Server Error` — Webhook secret not configured

### POST /v1/webhooks/{source}

Generic webhook handler for VCS and issue-tracking platforms. Supports GitHub, GitLab, Bitbucket, and custom sources.

**Path Parameters**: `{source}` — webhook source identifier (e.g., `github`, `gitlab`, `bitbucket`)

**Authentication** (source-dependent):
- GitHub: `X-Hub-Signature-256` (HMAC-SHA256)
- GitLab: `X-Gitlab-Token`
- Bitbucket: `X-Hub-Signature` (HMAC-SHA256)

**Configuration**: `WEBHOOK_CONFIGS` env var — `source:secret:space_id,...`

**Debouncing**: Rapid events for the same entity are coalesced with a 10-second window.

**Response** (202 Accepted):

```json
{
  "status": "accepted",
  "source": "github",
  "entity_type": "push",
  "action": "",
  "debounce": "github:abc123sha"
}
```

---

## Symbol Relationships

Query symbol-level code relationships (calls, imports, implements).

### GET /v1/symbols/relationships

Get relationship counts by type for a space.

**Query Parameters**:

- `space_id` (required)

**Response**:

```json
{
  "space_id": "my-space",
  "counts": {
    "CALLS": 412,
    "IMPORTS": 87,
    "IMPLEMENTS": 23
  }
}
```

### GET /v1/symbols/{id}/relationships

Get all relationships for a specific symbol node.

**Path Parameters**: `{id}` — symbol node ID

**Query Parameters**:

- `space_id` (required)

**Response**:

```json
{
  "space_id": "my-space",
  "symbol_id": "sym-abc123",
  "relationships": [
    {
      "source_id": "sym-abc123",
      "target_id": "sym-def456",
      "type": "CALLS",
      "file_path": "internal/api/handler.go",
      "line": 42
    }
  ],
  "count": 5
}
```

---

## Plugins & Modules

### GET /v1/plugins

List all plugins.

**Response**:

```json
{
  "data": {
    "plugins": [
      {
        "id": "github-issues",
        "name": "GitHub Issues Ingestion",
        "type": "INGESTION",
        "version": "1.0.0",
        "status": "running"
      }
    ]
  }
}
```

### GET /v1/plugins/{id}

Get plugin details.

### POST /v1/plugins/create

Create a new plugin scaffold.

**Request Body**:

```json
{
  "name": "my-plugin",
  "type": "INGESTION",
  "version": "1.0.0",
  "description": "Custom data ingestion",
  "capabilities": ["custom-source"]
}
```

### POST /v1/plugins/{id}/validate

Validate a plugin's manifest, proto compliance, and health.

### GET /v1/modules

List loaded plugin modules.

### POST /v1/modules/{id}/sync

Trigger a sync operation on an ingestion module.

---

## System & Monitoring

### GET /v1/metrics?space_id={space_id}

Get graph metrics (nodes, edges, hub analysis).

**Response**:

```json
{
  "total_nodes": 5000,
  "total_edges": 15000,
  "nodes_by_layer": {"0": 4500, "1": 450, "2": 50},
  "edges_by_type": {
    "ABSTRACTS_TO": 5000,
    "CO_ACTIVATED_WITH": 10000
  },
  "hub_nodes": [
    {"node_id": "mem-abc", "name": "CoreModule", "degree": 250}
  ],
  "orphan_nodes": 10,
  "avg_edge_weight": 0.45
}
```

### GET /v1/memory/stats?space_id={space_id}

Get per-space memory statistics.

**Response**:

```json
{
  "space_id": "my-project",
  "memory_count": 5000,
  "observation_count": 5500,
  "memories_by_layer": {"0": 4500, "1": 450, "2": 50},
  "embedding_coverage": 0.99,
  "avg_embedding_dimensions": 1536,
  "learning_activity": {
    "co_activated_edges": 10000,
    "avg_weight": 0.42,
    "max_weight": 0.95
  },
  "connectivity": {
    "avg_degree": 3.5,
    "max_degree": 250,
    "orphan_count": 10
  },
  "health_score": 0.85
}
```

### GET /v1/memory/cache/stats

Get query result cache statistics.

### DELETE /v1/memory/cache

Clear query result cache. Requires `confirm=true` query parameter.

**Query Parameters**:

- `confirm` (required): Must be `"true"` to proceed
- `space_id` (optional): Clear only this space's cache entries; omit for global clear

**Response**:

```json
{
  "message": "Cache cleared for space",
  "entries_cleared": 42,
  "space_id": "my-space"
}
```

### GET /v1/memory/query/metrics

Get Neo4j query execution statistics.

### GET /v1/memory/distribution

Score distribution statistics and learning phase info for a space.

**Query Parameters**:

- `space_id` (required)
- `history_limit` (optional, default 10, max 100): Number of historical distributions

**Response**:

```json
{
  "stats": {
    "phase": "warm",
    "edge_count": 12500,
    "alerts": [],
    "phase_thresholds": {
      "learning": 1,
      "warm": 10000,
      "saturated": 50000
    }
  },
  "history": []
}
```

**Learning Phases**: `cold` (0 edges), `learning` (1-10k), `warm` (10k-50k), `saturated` (50k+).

### GET /v1/memory/frontiers

Identify frontier nodes — L3+ concepts with sufficient evidence but low outgoing degree and no L5 parent. Candidates for concept expansion.

**Query Parameters**:

- `space_id` (required): The space to query
- `limit` (optional, default 20, max 100): Maximum frontier nodes to return

**Response (200)**:

```json
{
  "frontiers": [
    {
      "node_id": "mem-abc123",
      "name": "AuthenticationFlow",
      "layer": 3,
      "summary": "Authentication and authorization patterns",
      "outgoing_edges": 1,
      "evidence": 5
    }
  ],
  "count": 1
}
```

**Errors**: 400 (missing `space_id`), 405 (wrong method).

**Config**: `FRONTIER_MIN_EVIDENCE` (default 3), `FRONTIER_MAX_OUTGOING` (default 2).

### GET /v1/prometheus

Prometheus-format metrics endpoint. Returns `text/plain` in Prometheus exposition format.

Includes: circuit breaker metrics, cache hit ratios, Neo4j connection pool stats, per-space graph metrics, container resource metrics, and memory pressure metrics.

**Configuration**: `METRICS_ENABLED=true` required.

### GET /v1/memory/edges/stale/stats?space_id={space_id}

Get statistics about stale edges in a space. Edges become stale when their connected nodes' embeddings change.

**Query Parameters**:

- `space_id` (required): The space to query

**Response**:

```json
{
  "space_id": "my-project",
  "total_stale_coactivation": 15,
  "total_stale_associated": 8,
  "total_stale": 23,
  "oldest_stale_at": "2026-02-05T10:30:00Z",
  "staleness_reasons": {
    "content_changed": 20,
    "embedding_updated": 3
  }
}
```

### POST /v1/memory/edges/stale/refresh

Trigger a refresh of stale edges in a space. Recalculates semantic similarity scores for edges marked as stale.

**Request Body**:

```json
{
  "space_id": "my-project"
}
```

**Response**:

```json
{
  "space_id": "my-project",
  "edges_refreshed": 23
}
```

### GET /v1/ape/status

Get APE (Autonomous Pattern Extraction) scheduler status.

### POST /v1/ape/trigger

Manually trigger an APE event.

**Request Body**:

```json
{
  "event": "consolidation_complete"
}
```

### GET /v1/system/pool-metrics

Neo4j connection pool stats with Go runtime memory and goroutine metrics.

**Response**:

```json
{
  "connection_pool": {
    "active": 5,
    "idle": 15,
    "max": 50
  },
  "runtime": {
    "goroutines": 42,
    "heap_alloc_mb": 128.5,
    "heap_sys_mb": 256.0,
    "heap_objects": 50000,
    "gc_pause_ns": 300000,
    "gc_total_pause_ms": 12.5,
    "num_gc": 85
  }
}
```

### GET /v1/system/capability-gaps?status={status}&type={type}&space_id={space_id}

List detected capability gaps.

**Response**:

```json
{
  "data": {
    "gaps": [
      {
        "id": "gap-abc123",
        "type": "data_source",
        "description": "Missing Jira integration",
        "priority": "high",
        "status": "open"
      }
    ],
    "summary": {
      "total": 5,
      "by_type": {"data_source": 2, "reasoning": 3},
      "high_priority": 2
    }
  }
}
```

### POST /v1/system/capability-gaps/{id}/dismiss

Dismiss a capability gap.

### POST /v1/system/capability-gaps/{id}/address

Mark a capability gap as addressed.

### POST /v1/feedback

Submit feedback for capability gap detection.

**Request Body**:

```json
{
  "space_id": "my-project",
  "query_text": "How does caching work?",
  "rating": "negative",
  "comment": "Results were not relevant"
}
```

---

## Space Freshness (Phase 9.2)

### `GET /v1/memory/spaces/{space_id}/freshness`

Returns freshness and staleness information for a space's TapRoot node.

**Path Parameters**:

- `space_id` - The space to check freshness for

**Response** (`200 OK`):

```json
{
  "space_id": "my-project",
  "last_ingest_at": "2026-02-03T15:30:00Z",
  "last_ingest_type": "codebase-ingest",
  "ingest_count": 12,
  "is_stale": false,
  "stale_hours": 8,
  "threshold_hours": 24
}
```

**Fields**:

- `last_ingest_at` - ISO8601 timestamp of last ingest (omitted if never ingested)
- `last_ingest_type` - Type of last ingest (`codebase-ingest`, `file-ingest`)
- `ingest_count` - Total number of ingestions for this space
- `is_stale` - Whether the space is considered stale based on `SYNC_STALE_THRESHOLD_HOURS`
- `stale_hours` - Hours since last ingest
- `threshold_hours` - Configured staleness threshold in hours

### GET /v1/memory/freshness

Batch freshness check for multiple spaces in a single request.

**Query Parameters**:

- `space_ids` (required): Comma-separated list of space IDs (max 100)

**Response**:

```json
{
  "spaces": [
    {
      "space_id": "my-space",
      "ingest_count": 5,
      "last_ingest_at": "2026-02-20T10:00:00Z",
      "last_ingest_type": "codebase-ingest",
      "stale_hours": 96,
      "is_stale": true,
      "threshold_hours": 48
    }
  ],
  "threshold_hours": 48
}
```

---

## Cleanup & Orphan Management

### POST /v1/memory/cleanup/orphans

Detect and act on L0 nodes that were not included in the most recent re-ingestion (timestamp-based orphan detection).

**Request Body**:

```json
{
  "space_id": "my-project",
  "action": "list",
  "limit": 100,
  "dry_run": false,
  "older_than_days": 7,
  "path_prefix": "src/"
}
```

**Actions**: `list`, `count`, `archive`, `delete`

### POST /v1/memory/cleanup/graph-orphans

Cross-space zero-edge node scan and fix. Scans all (or specified) spaces for nodes with no edges.

**Request Body**:

```json
{
  "action": "scan",
  "space_ids": ["optional-filter"],
  "min_age_days": 0,
  "layers": [0, 1],
  "dry_run": true,
  "limit": 100
}
```

**Actions**: `scan` (read-only), `consolidate` (run consolidation), `archive` (set is_archived), `delete` (DETACH DELETE).

**Protected spaces**: `archive` and `delete` are blocked on protected spaces (e.g., `mdemg-dev`) — returns `skipped: true`.

**Response**:

```json
{
  "action": "scan",
  "dry_run": true,
  "total_spaces": 2,
  "total_orphans": 47,
  "total_affected": 0,
  "space_results": [
    {
      "space_id": "whk-wms",
      "orphan_count": 42,
      "affected_count": 0,
      "layer_breakdown": {"L0": 38, "L1": 3, "L2": 1},
      "nodes": [{"node_id": "...", "layer": 0, "role_type": "...", "created_at": "..."}]
    }
  ]
}
```

### POST /v1/memory/cleanup/schedule

Schedule automated orphan cleanup.

### GET /v1/memory/cleanup/schedules

List cleanup schedules.

### GET /v1/memory/cleanup/stats

Cleanup statistics for a space (`?space_id=X`).

---

## File Watcher (Phase 9.4)

### POST /v1/filewatcher/start

Start file watching for a directory.

### GET /v1/filewatcher/status

Get file watcher status.

### POST /v1/filewatcher/stop

Stop file watching.

---

## Job Streaming (SSE)

### GET /v1/jobs/{job_id}/stream

Server-Sent Events (SSE) stream for real-time job progress. Polls every 500ms, sending events only on status/progress changes. Connection times out after 5 minutes.

**Content-Type**: `text/event-stream`

**SSE Events**:

| Event | When | Data |
|-------|------|------|
| `connected` | Immediately on connection | `{ job_id, status }` |
| `progress` | On status/progress change | Full progress object |
| `complete` | Job reaches terminal state | `{ final_status, result, error }` |
| `timeout` | After 5-minute connection | — |
| `error` | Job disappears mid-stream | Error details |

**Progress event data**:

```json
{
  "job_id": "scrape-a1b2c3d4",
  "status": "running",
  "progress": {
    "total": 10,
    "current": 4,
    "percentage": 40.0,
    "phase": "scraping",
    "rate": 2.5
  }
}
```

---

## Backup & Restore (Phase 70)

Backup and restore endpoints for disaster recovery. All endpoints return `503 Service Unavailable` when `BACKUP_ENABLED=false`.

See [`docs/development/NEO4J_BACKUP.md`](NEO4J_BACKUP.md) for the full operations guide.

### POST /v1/backup/trigger

Trigger an on-demand backup (full database dump or partial space export).

**Request Body**:

```json
{
  "type": "partial_space",
  "space_ids": ["mdemg-dev"],
  "keep_forever": false,
  "label": "manual-backup"
}
```

- `type` — `"full"` (neo4j-admin dump) or `"partial_space"` (space export via `.mdemg`)
- `space_ids` — Spaces to include in partial backup (empty = all spaces). `mdemg-dev` is always included.
- `keep_forever` — Exempt from retention cleanup
- `label` — Optional label for identification

**Response** (`202 Accepted`):

```json
{
  "backup_id": "bk-20260208-022802-partial_space",
  "status": "pending",
  "message": "backup triggered"
}
```

### GET /v1/backup/status/{id}

Check backup job progress.

**Response** (`200 OK`):

```json
{
  "backup_id": "bk-20260208-022802-partial_space",
  "status": "completed",
  "progress": {
    "total": 1,
    "current": 1,
    "percentage": 100,
    "phase": "computing checksum"
  },
  "result": {
    "backup_id": "bk-20260208-022802-partial_space",
    "checksum": "sha256:c00da0f7...",
    "path": "backups/bk-20260208-022802-partial_space.mdemg",
    "size": 105898054
  }
}
```

### GET /v1/backup/list

List available backups. Optional `?type=full` or `?type=partial_space` filter.

**Response** (`200 OK`):

```json
{
  "backups": [
    {
      "backup_id": "bk-20260208-022802-partial_space",
      "type": "partial_space",
      "format_version": "1.0",
      "created_at": "2026-02-08T02:28:02Z",
      "checksum": "sha256:c00da0f7...",
      "size_bytes": 105898054,
      "spaces": ["mdemg-dev"],
      "node_count": 21033,
      "edge_count": 232434,
      "keep_forever": false,
      "label": "manual-backup"
    }
  ],
  "count": 1
}
```

### GET /v1/backup/manifest/{id}

Get full manifest details for a specific backup.

**Response** (`200 OK`): Returns the `BackupManifest` JSON (same fields as list entries).

### DELETE /v1/backup/{id}

Delete a backup (removes data file + manifest from disk).

**Response** (`200 OK`):

```json
{
  "status": "deleted",
  "backup_id": "bk-20260208-022802-partial_space"
}
```

### POST /v1/backup/restore

Trigger a restore from a full database dump. Full dump only for P0.

**Request Body**:

```json
{
  "backup_id": "bk-20260208-030000-full",
  "snapshot_before": true
}
```

**Response** (`202 Accepted`):

```json
{
  "restore_id": "restore-abc123",
  "status": "pending",
  "message": "restore triggered"
}
```

### GET /v1/backup/restore/status/{id}

Check restore job progress.

**Response** (`200 OK`):

```json
{
  "restore_id": "restore-abc123",
  "status": "completed",
  "progress": {
    "percentage": 100,
    "phase": "complete"
  }
}
```

---

## Neo4j State Monitor (Phase 76)

Consolidated view of database health, all spaces, and backup status in a single endpoint.

### GET /v1/neo4j/overview

Returns aggregated database statistics, per-space summaries, and backup overview.

**Response**:

```json
{
  "database": {
    "status": "healthy",
    "version": "0.6.0",
    "schema_version": 10,
    "total_nodes": 33329,
    "total_edges": 401908,
    "total_spaces": 85
  },
  "spaces": [
    {
      "space_id": "mdemg-dev",
      "node_count": 8689,
      "edge_count": 1631,
      "nodes_by_layer": { "0": 3995, "1": 3594, "2": 778, "3": 224, "4": 64, "5": 34 },
      "observation_count": 277,
      "health_score": 0.67,
      "last_consolidation": "2026-02-09T21:00:00Z",
      "last_ingest": "",
      "last_ingest_type": "",
      "ingest_count": 0,
      "is_stale": false,
      "learning_edges": 1631,
      "orphan_count": 92
    }
  ],
  "backups": {
    "last_full": null,
    "last_partial": null,
    "total_count": 0
  },
  "computed_at": "2026-02-09T21:59:14Z"
}
```

**Response Fields**:

| Section | Field | Description |
|---------|-------|-------------|
| `database` | `status` | `healthy`, `degraded`, or `unavailable` |
| `database` | `version` | MDEMG server version |
| `database` | `schema_version` | Neo4j migration schema version |
| `database` | `total_nodes` / `total_edges` | Global counts across all spaces |
| `database` | `total_spaces` | Number of distinct space_ids |
| `spaces[]` | `health_score` | 0.0-1.0 based on orphan ratio (60%) + edge density (40%) |
| `spaces[]` | `is_stale` | True if >10 observations and no consolidation in 7 days |
| `spaces[]` | `nodes_by_layer` | Node counts per layer (0-5) |
| `backups` | `last_full` / `last_partial` | Most recent backup summary (null if none) |

---

## Meta-Cognition & Self-Improvement (Phase 80)

Server-side anomaly detection and behavioral learning for CMS enforcement.

### GET /v1/conversation/session/anomalies

Aggregated session health: watchdog state, observation rate, active anomalies.

**Query Parameters**:

- `session_id` (required): Session identifier
- `space_id` (required): Memory space identifier

**Response**:

```json
{
  "session_id": "claude-core",
  "space_id": "mdemg-dev",
  "health_score": 0.75,
  "watchdog_state": {
    "temporal_decay": 0.02,
    "decay_rate": "normal",
    "escalation_level": "none",
    "session_health_score": 0.75,
    "obs_rate_per_hour": 2.5,
    "active_anomalies": [],
    "consolidation_age_sec": 3600
  },
  "observation_rate": 2.5,
  "active_anomalies": []
}
```

### POST /v1/self-improve/assess

Runs the RSIC assessment stage only (no reflect/plan/dispatch). Returns health metrics and confidence score for a given space.

**Request Body**:

```json
{
  "space_id": "mdemg-dev",
  "tier": "micro"
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `space_id` | Yes | — | Target memory space |
| `tier` | No | `"meso"` | Cycle tier (`micro`, `meso`, `macro`) |

**Response** (200):

```json
{
  "space_id": "mdemg-dev",
  "tier": "micro",
  "timestamp": "2026-02-19T12:00:00Z",
  "retrieval_quality": 0.85,
  "task_performance": 0.70,
  "memory_health": 0.92,
  "edge_health": 0.78,
  "overall_health": 0.81,
  "confidence": 0.75,
  "learning_phase": "warm",
  "edge_count": 12450,
  "orphan_count": 15,
  "total_nodes": 258,
  "orphan_ratio": 0.058,
  "correction_rate": 0.02,
  "consolidation_age_sec": 3600,
  "volatile_count": 20,
  "permanent_count": 238,
  "avg_edge_weight": 0.45
}
```

**Error Codes**: `400` (missing space_id), `405` (not POST), `500` (internal), `503` (RSIC not initialised).

### GET /v1/self-improve/report

Lists all active RSIC task statuses.

**Response** (200):

```json
{
  "active_tasks": {
    "task-abc123": "running",
    "task-def456": "completed"
  }
}
```

### GET /v1/self-improve/report/{taskID}

Returns progress reports for a specific RSIC task.

**Response** (200):

```json
{
  "task_id": "task-abc123",
  "reports": [
    {
      "task_id": "task-abc123",
      "cycle_id": "rsic-micro-abc123",
      "status": "completed",
      "progress_pct": 1.0,
      "milestone": "dispatch_complete",
      "summary": "Consolidated 5 volatile nodes",
      "metrics_delta": {"orphan_ratio": -0.02},
      "timestamp": "2026-02-19T12:01:00Z"
    }
  ]
}
```

### GET /v1/self-improve/calibration

Returns current per-action confidence scores from the calibrator.

**Response** (200):

```json
{
  "calibration": {
    "trigger_consolidation": 1.0,
    "prune_decayed_edges": 0.85,
    "graduate_volatile": 0.90
  }
}
```

**Error Codes**: `405` (not GET), `503` (RSIC not initialised).

### GET /v1/self-improve/signals

Signal emission/response effectiveness stats (Hebbian learning).

**Response**:

```json
{
  "signals": [
    {
      "code": "empty-resume-warning",
      "emissions": 5,
      "responses": 3,
      "strength": 0.65,
      "response_rate": 0.6
    }
  ],
  "enabled": true,
  "count": 1
}
```

### Response Extensions (Resume/Recall)

Phase 80 adds `anomalies` and `memory_state` fields to existing resume and recall responses:

```json
{
  "anomalies": [
    {
      "code": "empty-resume",
      "severity": "critical",
      "message": "Resume returned 0 observations but space has 258 nodes",
      "action": "curl -X POST http://localhost:9999/v1/self-improve/assess -d '{\"space_id\":\"mdemg-dev\",\"tier\":\"micro\"}'"
    }
  ],
  "memory_state": "degraded"
}
```

**Memory States**: `healthy` (observations + themes), `nominal` (observations but no themes), `degraded` (empty resume for populated space)

**Response Headers** (set on degraded state):

- `X-MDEMG-Memory-State: degraded`
- `X-MDEMG-Anomaly: empty-resume`

---

## RSIC Orchestration & Safety (Phases 87-90)

### POST /v1/self-improve/orchestration/reset

Reset all orchestration state (active cycles, cooldown, dedupe windows). Used for test isolation between UATS runs.

**Response (200)**:

```json
{"reset": true}
```

**Errors**: 405 (wrong method), 503 (orchestration policy not initialised).

### POST /v1/self-improve/cycle

Triggers an RSIC cycle with orchestration policy enforcement.

**Request Body**:

```json
{
  "space_id": "mdemg-dev",
  "tier": "meso",
  "trigger_source": "manual_api",
  "idempotency_key": "optional-dedup-key",
  "dry_run": false
}
```

- `trigger_source`: `manual_api` (default), `micro_auto`, `session_periodic`, `macro_cron`, `watchdog_force`
- `dry_run` (Phase 88): When `true`, runs full pipeline but applies zero mutations. Returns `deltas` array.
- `idempotency_key` (Phase 87): If provided, duplicate requests within the dedupe window return the cached result.

**Response (200)**:

```json
{
  "cycle_id": "rsic-meso-abc12345",
  "tier": "meso",
  "space_id": "mdemg-dev",
  "started_at": "2026-02-18T10:00:00Z",
  "completed_at": "2026-02-18T10:00:01Z",
  "actions_executed": 3,
  "success_count": 3,
  "failed_count": 0,
  "trigger_source": "manual_api",
  "trigger_id": "manual_api:mdemg-dev:2026-02-18T10:00",
  "triggered_at": "2026-02-18T10:00:00Z",
  "policy_version": "phase87-v1",
  "idempotency_key": "optional-dedup-key",
  "safety_version": "phase88-v1",
  "dry_run": false
}
```

**Response (409 — Cooldown/Overlap)**:

```json
{
  "error": "trigger rejected",
  "reason": "cooldown active for source=manual_api space=mdemg-dev (143s remaining)",
  "policy_version": "phase87-v1"
}
```

**Config**: `RSIC_TRIGGER_COOLDOWN_SEC` (default 300), `RSIC_TRIGGER_DEDUPE_SEC` (default 600).

### GET /v1/self-improve/health

Extended health endpoint with orchestration, safety, and persistence blocks.

**Response (200)** includes Phase 87-90 blocks:

```json
{
  "status": "idle",
  "active_tasks": 0,
  "watchdog": { "decay_score": 0.85, "cycles_seen": 12 },
  "orchestration": {
    "cooldown_sec": 300,
    "dedupe_sec": 600,
    "last_triggers": [],
    "scheduler": { "enabled": true, "macro_next_run": "2026-02-25T03:00:00Z" }
  },
  "safety": {
    "safety_version": "phase88-v1",
    "rollback_window_sec": 3600,
    "protected_spaces": ["mdemg-dev"]
  },
  "persistence": {
    "enabled": true,
    "last_flush": "2026-02-18T10:00:00Z",
    "state_nodes": 4,
    "dirty_keys": 0,
    "flush_errors": 0
  }
}
```

### GET /v1/self-improve/history

Supports Phase 87 filtering by `trigger_source`, `tier`, and `space_id` query params.

**Query Parameters**: `limit` (default 50), `trigger_source`, `tier`, `space_id`.

### GET /v1/self-improve/rollback

Lists available rollback snapshots (Phase 88).

**Response (200)**:

```json
{
  "snapshots": [
    {
      "snapshot_id": "snap-abc123",
      "cycle_id": "rsic-meso-abc12345",
      "space_id": "mdemg-dev",
      "action_type": "tombstone_stale",
      "created_at": "2026-02-18T10:00:00Z",
      "reversible": true
    }
  ],
  "count": 1
}
```

### POST /v1/self-improve/rollback

Rolls back a specific snapshot (Phase 88).

**Request Body**:

```json
{ "snapshot_id": "snap-abc123" }
```

---

## Admin — Space Lifecycle Management

### GET /v1/admin/spaces

List all spaces with metadata (node counts, prunable status, orphan detection).

**Query parameters:**

- `prunable` — Filter by prunable status (`true`, `false`, or omit for all)
- `limit` — Max spaces to return (default 100, max 500)

**Response** (`200 OK`):

```json
{
  "spaces": [
    {
      "space_id": "mdemg-dev",
      "prunable": false,
      "protected": true,
      "created_at": "2026-01-28T20:24:27.46Z",
      "last_ingest_at": "2026-02-19T22:08:14.216Z",
      "ingest_count": 182,
      "node_count": 19186,
      "observation_count": 324
    }
  ],
  "total": 1,
  "prunable_count": 0
}
```

### PATCH /v1/admin/spaces/{space_id}

Update space metadata (currently only `prunable` flag). Protected spaces cannot be marked prunable.

**Request body:**

```json
{ "prunable": true }
```

**Response** (`200 OK`):

```json
{ "space_id": "test-abc", "prunable": true, "updated": true }
```

### POST /v1/admin/spaces/prune

Execute batch pruning of prunable/orphan spaces. Supports dry-run mode.

**Request body:**

```json
{ "dry_run": true, "batch_size": 10000, "max_spaces": 50 }
```

**Response** (`200 OK`):

```json
{
  "dry_run": true,
  "spaces_pruned": 2,
  "spaces_skipped": 0,
  "total_nodes_deleted": 150,
  "results": [
    { "space_id": "test-abc", "nodes_deleted": 100, "status": "dry_run" },
    { "space_id": "uats-xyz", "nodes_deleted": 50, "status": "dry_run" }
  ]
}
```

**Auto-prune scheduler:** A background goroutine runs `runAutoSpacePrune()` on a configurable interval.

**Environment variable:**

- `SPACE_PRUNE_INTERVAL_HOURS` — Interval in hours (default: 24, 0 = disabled)

---

## Space Transfer (Export/Import)

HTTP API for exporting and importing space data. Supports profile-based filtering and conflict-aware import. Previously CLI-only; these endpoints enable UATS contract testing and programmatic transfer workflows.

### GET /v1/admin/spaces/export/preview

Lightweight estimation of what an export would contain, without transferring data.

**Query Parameters:**

- `space_id` (required): Space to preview
- `profile` (optional): Export profile — `full`, `metadata`, `shareable`, `codebase`, `cms`, `learned` (default: `full`)

**Response** (`200 OK`):

```json
{
  "space_id": "my-project",
  "profile": "shareable",
  "estimated_nodes": 42,
  "estimated_edges": 15,
  "estimated_observations": 30,
  "estimated_symbols": 0,
  "filters_applied": {
    "obs_types": ["learning", "decision", "correction", "technical_note", "insight", "preference"],
    "exclude_volatile": true,
    "only_pinned": false,
    "min_layer": 0,
    "max_layer": 0
  }
}
```

**Error Codes:** `400` (missing space_id, invalid profile), `405` (wrong method).

### POST /v1/admin/spaces/export

Export space data with profile-based filtering and optional overrides.

**Request Body:**

```json
{
  "space_id": "my-project",
  "profile": "shareable",
  "chunk_size": 500,
  "include_embeddings": true,
  "obs_types": ["learning", "decision"],
  "tags": ["important"],
  "exclude_volatile": true,
  "only_pinned": false,
  "min_layer": 0,
  "max_layer": 0,
  "no_observations": false,
  "no_symbols": false,
  "no_learned_edges": false
}
```

Only `space_id` is required. All other fields are optional and override profile defaults.

**Response** (`200 OK`):

```json
{
  "space_id": "my-project",
  "profile": "shareable",
  "header": {
    "format": "mdemg-space-transfer",
    "version": "1.0.0"
  },
  "chunks": [ ... ],
  "summary": {
    "nodes_exported": 42,
    "edges_exported": 15,
    "observations_exported": 30,
    "symbols_exported": 0,
    "duration_ms": 142,
    "next_cursor": "2026-03-16T12:00:00Z"
  }
}
```

The `chunks` array contains protobuf-JSON `SpaceChunk` objects — the same format as `.mdemg` files. The response IS the export data.

**Error Codes:** `400` (missing space_id, invalid profile), `405` (wrong method), `500` (Neo4j/export error).

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

**Response** (`200 OK`):

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

**Error Codes:** `400` (missing/null chunks, invalid conflict mode), `405` (wrong method), `500` (Neo4j/import error).

---

## Hash Verification — UNTS (Phase 38)

Hash verification REST API for tracking SHA-256 hashes of framework-protected files. Requires `UNTS_ENABLED=true`. Returns 503 when disabled.

### POST /v1/hash-verification/register

Register a file for hash tracking.

**Request Body:**

```json
{
  "path": "docs/specs/manifest.sha256",
  "framework": "manifest",
  "hash": "<sha256>",
  "source_ref": "",
  "source": "manual"
}
```

**Response (200):** `FileRecord` with `path`, `framework`, `current_hash`, `status`, `updated_at`, `history`.

**Errors:** 400 (missing path), 503 (UNTS disabled).

### GET /v1/hash-verification/files

List all tracked files. Optional query params: `?framework=manifest`, `?status=verified`.

**Response (200):**

```json
{
  "files": [...],
  "total": 42
}
```

### GET /v1/hash-verification/files/{path}

Get a single tracked file by repo-relative path (URL path suffix after `/files/`).

**Response (200):** `FileRecord` with `path`, `framework`, `current_hash`, `status`, `history` (array of last 3 hashes).

**Errors:** 404 (file not tracked).

### POST /v1/hash-verification/verify

Verify a single file's actual hash against expected.

**Request Body:**

```json
{"path": "docs/specs/manifest.sha256"}
```

**Response (200):**

```json
{
  "path": "docs/specs/manifest.sha256",
  "status": "verified",
  "expected_hash": "<sha256>",
  "actual_hash": "<sha256>"
}
```

Status values: `verified`, `mismatch`, `unknown`.

**Errors:** 404 (file not tracked).

### POST /v1/hash-verification/verify-all

Verify all tracked files. Optional `framework` filter in body.

**Response (200):**

```json
{
  "results": [...],
  "total": 42,
  "verified": 40,
  "mismatched": 2
}
```

### POST /v1/hash-verification/update

Update the expected hash for a tracked file (pushes current hash to history).

**Request Body:**

```json
{"path": "...", "hash": "<new_sha256>", "source": "manual"}
```

**Response (200):** Updated `FileRecord`.

**Errors:** 404 (file not tracked).

### POST /v1/hash-verification/revert

Revert to a previous hash from the file's history.

**Request Body:**

```json
{"path": "...", "target_hash": "<old_sha256>"}
```

**Response (200):** Updated `FileRecord` with `status: "reverted"`.

**Errors:** 400 (target hash not in history), 404 (file not tracked).

### POST /v1/hash-verification/scan

Trigger a full scan of `manifest.sha256` and UDTS spec files to auto-register hashes.

**Response (200):**

```json
{"scanned": true, "files_registered": 22}
```

**Configuration:**

- `UNTS_ENABLED` — Enable hash verification REST API (default: `false`)
- `UNTS_BASE_PATH` — Repository root for file hashing (default: `.`)

---

## Error Responses

All endpoints return errors in a consistent format:

```json
{
  "error": "error message"
}
```

**HTTP Status Codes**:

- `200` - Success
- `201` - Created
- `207` - Multi-Status (partial success in batch operations)
- `400` - Bad Request (validation error)
- `404` - Not Found
- `405` - Method Not Allowed
- `409` - Conflict (e.g., cannot delete node with children)
- `500` - Internal Server Error
- `503` - Service Unavailable

---

## Authentication

MDEMG does not currently require authentication. For production deployments, consider placing it behind a reverse proxy with authentication.
