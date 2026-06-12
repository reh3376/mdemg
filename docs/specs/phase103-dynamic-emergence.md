<!-- markdownlint-disable MD013 MD022 MD032 MD040 -->
# Feature Spec: Dynamic Emergence

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: 103
**Status**: Complete
**Author**: Agent
**Date**: 2026-02-23

---

## Overview

Dynamic Emergence addresses Gap 3 in the Cognitive Intelligence Gap Analysis. Currently, MDEMG relies on hardcoded string/regex matching in Go to generate specific Layer 2+ abstraction nodes (e.g., matching "*auth*" to create a ConcernNode).

This phase overhauls the consolidation pipeline to achieve true "data-driven structure." By detecting structurally dense clusters of `CO_ACTIVATED_WITH` edges that do not match known patterns, we can pass these clusters to the LLM Semantic Summary Service. The LLM will then dynamically invent a name, description, and classification for the newly emerged abstraction, moving us away from pre-programmed concepts toward organic intelligence.

## Requirements

### Functional Requirements
1. **FR-1**: Extend the existing Union-Find / DBSCAN clustering logic used in the L5 Emergent Layer to operate across lower layers (L1-L4) based purely on `CO_ACTIVATED_WITH` edge density.
2. **FR-2**: Identify dense clusters that fail to trigger any of the existing hardcoded Go regex rules.
3. **FR-3**: Send the aggregated content of these "unclassified dense clusters" to the LLM Semantic Summary Service via a dedicated "Naming Prompt."
4. **FR-4**: Parse the LLM's response to extract a synthesized node `Name`, `Description`, and `ProposedLabel` (e.g., `:MemoryNode:EmergentConcept`).
5. **FR-5**: Create the new node in Neo4j with the LLM-generated properties, linking it back to its constituent cluster members via `ABSTRACTS_TO` edges.
6. **FR-6**: Support an `enable_dynamic_emergence` flag in the `/v1/memory/consolidate` API endpoint payload (default: false, for backward compatibility).

### Non-Functional Requirements
1. **NFR-1**: Fallback — If the LLM call fails, the cluster should be skipped during this consolidation run (logged as a warning), leaving the base nodes untouched for future attempts.
2. **NFR-2**: Prompt Engineering — The prompt must instruct the LLM to return strictly formatted JSON to ensure reliable parsing in Go.
3. **NFR-3**: Testing — This complex transformation logic must be governed by the Universal Validation Test Specification (UVTS) to benchmark semantic naming quality, alongside standard UATS for the API contract.

## API Contract

### Endpoints

```
POST /v1/memory/consolidate
```

**Request (Extended):**

```json
{
  "space_id": "mdemg-dev",
  "enable_dynamic_emergence": true,
  "min_cluster_density": 0.75
}
```

**Response (Extended):**

```json
{
  "data": {
    "status": "completed",
    "nodes_created": 4,
    "dynamic_emergent_nodes_created": 1,
    "duration_ms": 4500
  }
}
```

## Data Model

### Neo4j Schema Changes

```cypher
// No hard schema changes, but a new secondary label will be dynamically applied
// to nodes created by this process.
// Example: (:MemoryNode:EmergentConcept)
```

### Go Types

```go
// In internal/models/models.go

// Extend ConsolidateRequest
type ConsolidateRequest struct {
    // ... existing fields ...
    EnableDynamicEmergence bool    `json:"enable_dynamic_emergence,omitempty"`
    MinClusterDensity      float64 `json:"min_cluster_density,omitempty"`
}

// Extend ConsolidateResponse
type ConsolidateResponse struct {
    // ... existing fields ...
    DynamicEmergentNodesCreated int `json:"dynamic_emergent_nodes_created,omitempty"`
}

// New struct for LLM JSON Response parsing
type LLMEmergenceResponse struct {
    Name          string `json:"name"`
    Description   string `json:"description"`
    ProposedLabel string `json:"proposed_label"`
}
```

## Internal Implementation Plan

1. **Modify Request/Response Models**: Update `internal/models/models.go` with the new fields for the `/consolidate` endpoint.
2. **Extend Clustering Logic**: In `internal/consolidation/cluster.go` (or equivalent), add logic to collect "unclassified" clusters that meet the `MinClusterDensity` threshold but fail legacy regex checks.
3. **LLM Integration**: In `internal/consolidation/service.go`, add a `processDynamicEmergence()` function. This iterates over unclassified clusters, concatenates their textual summaries, and invokes the LLM client with a strict JSON-schema prompt.
4. **Graph Mutations**: Parse the `LLMEmergenceResponse`. Execute Cypher to create the new node (with the generated properties and the `:EmergentConcept` label) and draw `ABSTRACTS_TO` edges to the cluster members.

## Test Plan (UxTS Framework)

To adhere to the Framework Governance rules (`docs/specs/FRAMEWORK_GOVERNANCE.md`), this feature requires multi-framework coverage:

### UATS (Universal API Test Specification)
- [ ] `docs/api/api-spec/uats/drafts/consolidate_dynamic_emergence.phase103.uats.json`: Verify the API contract (request flags and response counts).

### UVTS (Universal Validation Test Specification)
*(Note: As per `UXTS_FRAMEWORK_MATRIX.md`, UVTS is currently spec-only without a runner. We will draft the spec format to pilot the runner implementation later).*
- [ ] `docs/tests/uvts/drafts/dynamic_emergence_quality.phase103.uvts.json`: Define a benchmark cluster of text nodes and assert that the LLM-generated `Name` contains expected semantic concepts (e.g., if we feed it 5 nodes about database connections, the output name MUST contain "Database" or "Connection").

### Unit Tests
- [ ] `internal/consolidation/service_test.go`: Mock the LLM client returning valid JSON and verify the corresponding Cypher query generation.
- [ ] `internal/consolidation/service_test.go`: Mock an LLM timeout and verify the pipeline degrades gracefully without crashing.

## Acceptance Criteria

- [ ] AC-1: The `/consolidate` API accepts `enable_dynamic_emergence`.
- [ ] AC-2: Unclassified dense clusters result in LLM API calls.
- [ ] AC-3: Valid LLM JSON responses result in new `:EmergentConcept` nodes linked via `ABSTRACTS_TO`.
- [ ] AC-4: Invalid LLM responses or timeouts are logged and skipped safely.
- [ ] AC-5: UATS draft spec is written and hashes generated.
- [ ] AC-6: UVTS draft spec is written to guide future semantic validation.

## Dependencies

- Depends on: Phase 11.2 (LLM Semantic Summary Service), L5 Emergent Layer features.
- Blocks: Phase 105 (Global Meta-Learning), which will rely on these dynamically generated concepts to cross-pollinate spaces.

## Files Changed

### Modified Files
- `internal/models/models.go`
- `internal/consolidation/service.go`
- `internal/api/handlers.go`

### New Files
- `docs/api/api-spec/uats/drafts/consolidate_dynamic_emergence.phase103.uats.json`
- `docs/tests/uvts/drafts/dynamic_emergence_quality.phase103.uvts.json`

---

## Phase 103b: Emergence Model Evaluation & MLX Server Integration

**Status**: Complete
**Date**: 2026-02-24

### Changes

1. **`LLM_ENDPOINT` Config Separation**: Added `LLMEndpoint` field and `EffectiveLLMEndpoint()` method to `internal/config/config.go`. All LLM text-generation features (synthesis, intent translation, emergence naming, reranking) use `EffectiveLLMEndpoint()` which returns `LLM_ENDPOINT` if set, otherwise falls back to `OPENAI_ENDPOINT`. Embeddings stay on `OPENAI_ENDPOINT`. This allows pointing LLM calls at a local MLX server while keeping embeddings on OpenAI.

2. **Ollama JSON Schema Enforcement**: Added `format` field with full JSON schema to Ollama requests in `emergence_namer.go`. Uses Ollama v0.5+ grammar-constrained output — model physically cannot produce non-conforming JSON. Also added `options.temperature: 0.3` for deterministic output.

3. **UETS Framework**: New UxTS framework (Universal Emergence Test Specification) for evaluating LLM emergence naming quality. 5 evaluation patterns: E1_JSON_CONFORMANCE, E2_LABEL_CONSTRAINT, E3_NAME_QUALITY, E4_DESCRIPTION_QUALITY, E5_LATENCY. Python runner replicates exact Go emergence prompt format with `--endpoint` override for remote execution and `num_ctx` config support. 8 model specs (7/7 passing): llama3.2-3b-macstudio Q4 (100%/100%/86%, 1262ms), llama3.2-3b-ollama Q4 (100%/100%/86%, 1457ms), llama3.2-3b-fp16-macstudio (100%/100%/86%, 1568ms), qwen3-8b (100%/100%/29%, 2126ms), qwen2.5-14b (100%/100%/0%, 4398ms), qwen2.5-72b-mlx (100%/100%/57%, 4553ms), llama3.3-70b-ollama (100%/100%/86%, 24866ms). **Recommendation**: `llama3.2:3b` Q4_K_M — fastest latency with top-tier name quality; FP16 adds no measurable accuracy benefit.

### Files Modified
- `internal/config/config.go` — `LLMEndpoint` field + `EffectiveLLMEndpoint()` + FromEnv parsing
- `internal/api/server.go` — Synthesis and intent configs use `EffectiveLLMEndpoint()`
- `internal/hidden/step_dynamic_emergence.go` — Uses `EffectiveLLMEndpoint()`
- `internal/hidden/emergence_namer.go` — Ollama `format` JSON schema + `options` temperature
- `internal/hidden/emergence_namer_test.go` — Updated for format/options fields
- `internal/retrieval/rerank.go` — Uses `EffectiveLLMEndpoint()`
- `.env.example` — `LLM_ENDPOINT` documentation

### New Files
- `docs/tests/uets/schema/uets.schema.json` — UETS JSON schema
- `docs/tests/uets/specs/llama3.2-3b-ollama.uets.json` — Local Ollama Q4_K_M spec
- `docs/tests/uets/specs/llama3.2-3b-macstudio.uets.json` — Mac Studio Q4_K_M spec
- `docs/tests/uets/specs/llama3.2-3b-fp16-macstudio.uets.json` — Mac Studio FP16 spec
- `docs/tests/uets/specs/llama3.3-70b-ollama.uets.json` — Local Ollama 70B spec
- `docs/tests/uets/specs/llama3.3-70b-macstudio.uets.json` — Mac Studio 70B spec
- `docs/tests/uets/specs/qwen2.5-72b-mlx.uets.json` — MLX Server 72B spec
- `docs/tests/uets/specs/qwen2.5-14b-ollama.uets.json` — Local Ollama 14B spec
- `docs/tests/uets/specs/qwen3-8b-ollama.uets.json` — Local Ollama 8B spec
- `docs/tests/uets/fixtures/clusters.json` — 7 CO_ACTIVATED_WITH clusters from Neo4j
- `docs/tests/uets/runners/uets_runner.py` — Python runner (~550 lines)
- `docs/tests/uets/README.md` — Framework documentation

### Documents Accessed
- `internal/config/config.go`, `internal/api/server.go`, `internal/hidden/emergence_namer.go`
- `internal/hidden/step_dynamic_emergence.go`, `internal/retrieval/rerank.go`
- `internal/hidden/emergence_namer_test.go`, `internal/hidden/step_dynamic_emergence_test.go`
- `.env.example`, `docs/specs/FRAMEWORK_GOVERNANCE.md`, `docs/development/UXTS_FRAMEWORK_MATRIX.md`
- `docs/tests/uets/README.md`, `docs/lang-parser/lang-parse-spec/upts/` (UPTS as template)
- `AGENT_HANDOFF.md`, `CHANGELOG.md`, `docs/specs/phase103-dynamic-emergence.md`
