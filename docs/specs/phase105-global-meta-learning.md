<!-- markdownlint-disable MD013 MD022 MD032 MD040 -->
# Feature Spec: Global Meta-Learning

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: 105
**Status**: Complete
**Author**: Agent
**Date**: 2026-02-23

---

## Overview

Global Meta-Learning addresses Gap 5 in the Cognitive Intelligence Gap Analysis. It implements the "Collective Learning Aggregation" component outlined in `VISION.md`.

Currently, powerful meta-patterns built by the L5 Emergent Layer and Phase 103's Dynamic Emergence remain isolated within their origin `space_id`. This feature introduces an "Org-Level" global graph space (`space_id: 'mdemg-global'`). High-value, highly-connected Layer 4/5 concepts discovered in individual spaces are automatically abstracted and promoted to this global space, allowing an AI agent working in an entirely different repository to benefit from organizational SME knowledge without requiring a full DevSpace merge.

## Requirements

### Functional Requirements
1. **FR-1**: Define a reserved system space ID (e.g., `mdemg-global`) meant for cross-space meta-concepts.
2. **FR-2**: Extend the consolidation background job or create a dedicated `POST /v1/memory/meta-learn` endpoint to run cross-space promotion.
3. **FR-3**: Identify Layer 4/5 nodes (e.g., `AbstractionNode`, `EmergentConcept`) in local spaces that meet a high `activation_count` and `degree_centrality` threshold.
4. **FR-4**: Use the LLM Semantic Summary Service to rewrite these local concepts into generalized, space-agnostic principles (removing repo-specific variable names or paths).
5. **FR-5**: Insert the generalized concept into the `mdemg-global` space.
6. **FR-6**: Modify the retrieval pipeline (`/v1/memory/retrieve`, `/consult`, `/suggest`) to optionally include `mdemg-global` in vector searches alongside the requested local `space_id`.

### Non-Functional Requirements
1. **NFR-1**: Privacy/Security — The LLM abstraction step *must* explicitly strip proprietary secrets, credentials, or highly sensitive internal paths before promoting to the global space, especially if `mdemg-global` is shared across distinct organizational units.
2. **NFR-2**: Vector Indexing — The global space nodes must be embedded in the same vector space as local nodes to allow seamless cross-space cosine similarity searches.

## API Contract

### Endpoints

```
POST /v1/memory/meta-learn
```

**Request:**

```json
{
  "source_space_id": "auth-service-repo",
  "min_layer": 4,
  "min_activation_count": 50
}
```

**Response:**

```json
{
  "data": {
    "status": "completed",
    "concepts_evaluated": 5,
    "concepts_promoted": 1,
    "promoted_nodes": [
      {
        "id": "global-uuid-1",
        "original_id": "local-uuid-42",
        "name": "Global JWT Validation Middleware",
        "global_space_id": "mdemg-global"
      }
    ]
  }
}
```

### Retrieval Endpoint Update

```
POST /v1/memory/retrieve
```

**Request (Extended):**

```json
{
  "space_id": "billing-service-repo",
  "query": "How do we handle auth?",
  "include_global_space": true
}
```

## Data Model

### Neo4j Schema Changes

```cypher
// Global nodes are identical to local nodes but reside under a specific TapRoot
MERGE (t:TapRoot {space_id: 'mdemg-global'})
// A new edge type to track provenance
// (:MemoryNode {space_id: 'mdemg-global'})-[:ORIGINATED_FROM]->(:MemoryNode {space_id: 'auth-service-repo'})
```

### Go Types

```go
// In internal/models/models.go

type MetaLearnRequest struct {
    SourceSpaceID      string `json:"source_space_id"`
    MinLayer           int    `json:"min_layer,omitempty"`
    MinActivationCount int    `json:"min_activation_count,omitempty"`
}

type PromotedConcept struct {
    ID            string `json:"id"`
    OriginalID    string `json:"original_id"`
    Name          string `json:"name"`
    GlobalSpaceID string `json:"global_space_id"`
}

type MetaLearnResponse struct {
    Status            string            `json:"status"`
    ConceptsEvaluated int               `json:"concepts_evaluated"`
    ConceptsPromoted  int               `json:"concepts_promoted"`
    PromotedNodes     []PromotedConcept `json:"promoted_nodes"`
}

// Extend RetrieveRequest
type RetrieveRequest struct {
    // ... existing fields ...
    IncludeGlobalSpace bool `json:"include_global_space,omitempty"`
}
```

## Internal Implementation Plan

1. **New API Handler**: Create `internal/api/handlers_meta.go` for the `/meta-learn` endpoint.
2. **Meta-Learning Service**: Create `internal/metalearn/service.go`.
    * Query Neo4j for nodes in `source_space_id` matching layer and activation thresholds.
    * For each candidate, send its content to the LLM with a "Generalization Prompt" (instructing it to strip local specifics and output a generalized principle).
    * Write the resulting node to `space_id: 'mdemg-global'` and draw an `ORIGINATED_FROM` edge back to the local node.
3. **Update Retrieval**: Modify `internal/retrieval/service.go`. If `IncludeGlobalSpace` is true, the initial vector recall Cypher query must be updated to `WHERE n.space_id IN [$spaceId, 'mdemg-global']`.

## Test Plan (UxTS Framework)

### UATS (Universal API Test Specification)
* [ ] `docs/api/api-spec/uats/drafts/meta_learn_promotion.phase105.uats.json`: Verify the `/v1/memory/meta-learn` contract and response counts.
* [ ] `docs/api/api-spec/uats/drafts/retrieve_global_space.phase105.uats.json`: Verify `/v1/memory/retrieve` accepts the `include_global_space` flag.

### UDTS (Universal DevSpace Test Specification)
<!-- (Applying UDTS as this involves cross-space data boundaries and graph topology validation) -->
* [ ] `docs/api/api-spec/udts/drafts/global_space_topology.phase105.udts.json`: Validate that promoted nodes exist in `mdemg-global` and have valid `ORIGINATED_FROM` edges pointing back to the correct local `space_id`.

### Unit Tests
* [ ] `internal/metalearn/service_test.go`: Mock LLM generalization and verify `ORIGINATED_FROM` Cypher generation.
* [ ] `internal/retrieval/service_test.go`: Verify Cypher query generation when `include_global_space` is toggled.

## Acceptance Criteria

* [ ] AC-1: The `/v1/memory/meta-learn` endpoint successfully promotes high-layer nodes to `mdemg-global`.
* [ ] AC-2: Promoted nodes are generalized via the LLM service.
* [ ] AC-3: `ORIGINATED_FROM` edges link global nodes to their local source.
* [ ] AC-4: Retrieval endpoints can query both the requested `space_id` and `mdemg-global` simultaneously.
* [ ] AC-5: UATS and UDTS draft specs are written and hashes generated.
* [ ] AC-6: SHA256 hash added to `docs/specs/manifest.sha256`.

## Dependencies

* Depends on: Phase 103 (Dynamic Emergence) to provide high-quality candidate nodes, Phase 11.2 (LLM Semantic Summary Service).

## Files Changed

### Modified Files
* `internal/models/models.go`
* `internal/api/server.go`
* `internal/retrieval/service.go`

### New Files
* `internal/api/handlers_meta.go`
* `internal/metalearn/service.go`
* `docs/api/api-spec/uats/drafts/meta_learn_promotion.phase105.uats.json`
* `docs/api/api-spec/uats/drafts/retrieve_global_space.phase105.uats.json`
* `docs/api/api-spec/udts/drafts/global_space_topology.phase105.udts.json`
