---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "103"
---

# Dynamic Emergence

## Summary

**Feature**: Dynamic Emergence
**Summary**: LLM-driven concept naming for unclassified clusters of co-activated nodes during consolidation. Dense Hebbian edge clusters that don't match any hardcoded pattern are sent to an LLM, which invents a name, description, and classification for the emergent concept.

## Vision & Goals

Implements the "Emergence Principle" from VISION.md: "Let structure arise from data, don't impose it." Rather than requiring human-authored taxonomies, dynamic emergence allows the memory graph to discover and name its own organizational structures. This addresses Gap 3 (Static Hardcoded Abstractions) by making concept creation data-driven rather than rule-driven.

## Current State

### Architecture

The dynamic emergence pipeline operates during consolidation:

1. Query `CO_ACTIVATED_WITH` edges between L0-L4 nodes with `weight >= EMERGENCE_MIN_WEIGHT`
2. Exclude nodes with existing `role_type` (concern, config, comparison, temporal, ui, constraint, dynamic_emergent)
3. Exclude pairs already sharing a `dynamic_emergent` parent (idempotency)
4. Union-find groups connected pairs into clusters
5. Clusters smaller than `EMERGENCE_MIN_CLUSTER_SIZE` are dropped
6. Up to `EMERGENCE_MAX_CLUSTERS` densest clusters are sent to the LLM
7. LLM returns JSON: `{"name": "...", "description": "...", "proposed_label": "..."}`
8. A `:MemoryNode:EmergentConcept` node is created at `layer = max(member.layer) + 1` with `role_type = 'dynamic_emergent'`
9. `ABSTRACTS_TO` edges link each member to the new concept

### Workflow

**Pipeline Placement:** Phase 22 — after hardcoded patterns (phase 20), before dynamic edges (phase 25). Hardcoded patterns claim nodes first; remaining unclassified clusters go to the LLM.

**Proposed Labels** — the LLM must choose from a constrained set validated in Go:
- `pattern` — recurring implementation pattern
- `principle` — architectural or design principle
- `bridge` — connects two otherwise separate domains
- `concern` — cross-cutting concern
- `workflow` — sequence of related operations

**Failure Handling:**
- **Per-cluster fail-open**: LLM errors skip individual clusters (logged as warnings), don't abort the run
- **Idempotency**: Clusters already abstracted to a `dynamic_emergent` node are excluded by `NOT EXISTS` subquery
- **Circuit breaker**: `openai-emergence` and `ollama-emergence` breakers protect against cascading LLM failures
- **Disabled gracefully**: `EMERGENCE_ENABLED=false` or `enable_dynamic_emergence=false` returns 0 nodes without error

### Configuration

See Configuration Reference table below.

## Notes

### Known Limitations

- Requires LLM API access (OpenAI or Ollama) — disabled by default
- LLM naming quality depends on model capability — smaller models may produce generic names

### Risks & Gaps

- No human-in-the-loop review of LLM-generated concept names

### Future Improvements

- Concept name quality scoring and automatic refinement
- User approval workflow for generated concepts

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| POST | `/v1/memory/consolidate` | Triggers consolidation with `enable_dynamic_emergence: true` field | `specs/consolidate.uats.json` |

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg consolidate` | Triggers full consolidation (emergence enabled via config) |

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `EMERGENCE_ENABLED` | `false` | Master toggle for dynamic emergence |
| `EMERGENCE_PROVIDER` | `openai` | LLM provider: `openai` or `ollama` |
| `EMERGENCE_MODEL` | `gpt-4o-mini` | Model for concept naming |
| `EMERGENCE_MAX_TOKENS` | `500` | Max tokens for naming response (100-4000) |
| `EMERGENCE_TIMEOUT_MS` | `10000` | Timeout for naming call in ms (min: 1000) |
| `EMERGENCE_MIN_WEIGHT` | `0.3` | Min CO_ACTIVATED_WITH weight (0.0-1.0) |
| `EMERGENCE_MIN_CLUSTER_SIZE` | `3` | Min nodes per cluster (min: 2) |
| `EMERGENCE_MAX_CLUSTERS` | `10` | Max clusters to name per run (min: 1) |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Consolidation Pipeline | Requires — runs as phase 22 step |
| Hebbian Learning | Requires — operates on CO_ACTIVATED_WITH edges |
| Split Pipeline Execution | Requires — runs in pre-clustering phase range |
| LLM Client Infrastructure | Requires — OpenAI or Ollama provider |
| Circuit Breaker Registry | Enhances — protects against LLM failures |

## Related Files

- `internal/hidden/emergence_namer.go` - LLM namer (config, prompt, OpenAI/Ollama clients, JSON parse)
- `internal/hidden/step_dynamic_emergence.go` - Pipeline step adapter (phase 22, optional)
- `internal/hidden/service.go` - `CreateDynamicEmergentNodes()` method
- `internal/hidden/emergence_namer_test.go` - 9 namer unit tests
- `internal/hidden/step_dynamic_emergence_test.go` - 2 step tests
- `internal/config/config.go` - 8 `EMERGENCE_*` config fields
- `docs/specs/phase103-dynamic-emergence.md` - Full specification
