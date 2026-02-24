<!-- markdownlint-disable MD013 MD040 -->
# Feature: Dynamic Emergence

**Phase**: 103
**Status**: Complete
**Gap**: Gap 3 — Static Hardcoded Abstractions

---

## Overview

Dynamic Emergence enables LLM-driven concept naming for unclassified clusters of co-activated nodes during consolidation. Dense `CO_ACTIVATED_WITH` (Hebbian learning) edge clusters that don't match any hardcoded pattern are sent to an LLM, which invents a name, description, and classification for the emergent concept.

This implements the "Emergence Principle" from VISION.md: "Let structure arise from data, don't impose it."

## How It Works

1. During consolidation, the pipeline queries `CO_ACTIVATED_WITH` edges between L0-L4 nodes with `weight >= EMERGENCE_MIN_WEIGHT`
2. Nodes with existing `role_type` (concern, config, comparison, temporal, ui, constraint, dynamic_emergent) are excluded
3. Pairs already sharing a `dynamic_emergent` parent are excluded (idempotency)
4. Union-find groups connected pairs into clusters
5. Clusters smaller than `EMERGENCE_MIN_CLUSTER_SIZE` are dropped
6. Up to `EMERGENCE_MAX_CLUSTERS` densest clusters are sent to the LLM
7. The LLM returns a JSON response: `{"name": "...", "description": "...", "proposed_label": "..."}`
8. A `:MemoryNode:EmergentConcept` node is created at `layer = max(member.layer) + 1` with `role_type = 'dynamic_emergent'`
9. `ABSTRACTS_TO` edges link each member to the new concept

## API Usage

```json
POST /v1/memory/consolidate
{
  "space_id": "my-space",
  "enable_dynamic_emergence": true
}
```

Response includes `dynamic_emergent_nodes_created` in the flat fields and `dynamic_emergence` in the `steps` map.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `EMERGENCE_ENABLED` | `false` | Master toggle for dynamic emergence |
| `EMERGENCE_PROVIDER` | `openai` | LLM provider: `openai` or `ollama` |
| `EMERGENCE_MODEL` | `gpt-4o-mini` | Model for concept naming |
| `EMERGENCE_MAX_TOKENS` | `500` | Max tokens for naming response (100-4000) |
| `EMERGENCE_TIMEOUT_MS` | `10000` | Timeout for naming call in ms (min: 1000) |
| `EMERGENCE_MIN_WEIGHT` | `0.3` | Min CO_ACTIVATED_WITH weight (0.0-1.0) |
| `EMERGENCE_MIN_CLUSTER_SIZE` | `3` | Min nodes per cluster (min: 2) |
| `EMERGENCE_MAX_CLUSTERS` | `10` | Max clusters to name per run (min: 1) |

## Pipeline Placement

Phase 22 — after hardcoded patterns (phase 20), before dynamic edges (phase 25). Hardcoded patterns claim nodes first; remaining unclassified clusters go to the LLM.

## Proposed Labels

The LLM must choose from a constrained set validated in Go:
- `pattern` — recurring implementation pattern
- `principle` — architectural or design principle
- `bridge` — connects two otherwise separate domains
- `concern` — cross-cutting concern
- `workflow` — sequence of related operations

## Failure Handling

- **Per-cluster fail-open**: LLM errors skip individual clusters (logged as warnings), don't abort the run
- **Idempotency**: Clusters already abstracted to a `dynamic_emergent` node are excluded by `NOT EXISTS` subquery
- **Circuit breaker**: `openai-emergence` and `ollama-emergence` breakers protect against cascading LLM failures
- **Disabled gracefully**: `EMERGENCE_ENABLED=false` or `enable_dynamic_emergence=false` returns 0 nodes without error

## Key Files

| File | Description |
|------|-------------|
| `internal/hidden/emergence_namer.go` | LLM namer (config, prompt, OpenAI/Ollama clients, JSON parse) |
| `internal/hidden/step_dynamic_emergence.go` | Pipeline step adapter (phase 22, optional) |
| `internal/hidden/service.go` | `CreateDynamicEmergentNodes()` method |
| `internal/hidden/emergence_namer_test.go` | 9 namer unit tests |
| `internal/hidden/step_dynamic_emergence_test.go` | 2 step tests |
| `internal/config/config.go` | 8 `EMERGENCE_*` config fields |
| `internal/models/models.go` | Request/response fields |
| `docs/specs/phase103-dynamic-emergence.md` | Full specification |
