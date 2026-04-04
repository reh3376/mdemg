---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "75B"
---

# L5 Emergent Layer

## Summary

**Feature**: L5 Emergent Layer
**Summary**: The highest layer in the MDEMG memory graph. L5 nodes represent emergent meta-patterns that span multiple lower-layer concepts and reveal cross-domain connections. They are not explicitly authored but emerge from the graph structure itself.

## Vision & Goals

L5 is the capstone of the 5-layer memory hierarchy. While L0-L4 represent progressively abstracted knowledge, L5 represents genuine emergence — patterns that no single observation or concept contains, but that arise from the relationships between concepts. This mirrors how biological cognition discovers analogies and cross-domain insights through associative memory. L5 emergence is the primary measure of whether MDEMG has moved beyond simple retrieval into genuine understanding.

## Current State

### Architecture

The layer hierarchy:

- **L0:** Raw observations (conversation, codebase)
- **L1:** Hidden patterns (DBSCAN clusters of L0)
- **L2:** Concepts (clusters of L1)
- **L3:** Higher-order concepts
- **L4:** Abstract concepts
- **L5:** Emergent concepts (meta-patterns across L3+ nodes)

### Workflow

`CreateL5EmergentNodes()` in `internal/hidden/service.go`:

1. **Query source nodes:** Find all L3+ nodes (configurable via `L5_SOURCE_MIN_LAYER`) in the space
2. **Find qualifying edges:** Query ANALOGOUS_TO, BRIDGES, and COMPOSES_WITH edges between source nodes with `evidence_count >= L5_BRIDGE_EVIDENCE_MIN`
3. **Cluster:** Use union-find to group connected nodes into clusters
4. **Create L5 nodes:** For each cluster with 2+ members, create a `:MemoryNode:EmergentConcept` node at layer 5
5. **Create edges:** Add `ABSTRACTS_TO` edges from each cluster member to its L5 node

**Pipeline Execution:** L5 runs as pipeline phase 30 (`emergent_l5` step), after dynamic edges (phase 25). This ordering is critical — dynamic edges must exist before L5 can find qualifying connections.

```
Phase 10-20: Core + enrichment  (pre-clustering)
Clustering:  Multi-layer L2-L5
Phase 25:    Dynamic edges       (post-clustering)
Phase 30:    L5 emergent nodes   (post-clustering, queries edges from phase 25)
```

**Phase 75C Unblocking Fixes** — Six bottlenecks were fixed to enable L5 emergence:

| # | Problem | Fix |
|---|---------|-----|
| 1 | `InferEdgeType` had no BRIDGES case | Added BRIDGES inference for cross-layer + moderate similarity |
| 2 | `L5_BRIDGE_EVIDENCE_MIN` default was 3 | Lowered to 1 — L5 triggers on first consolidation |
| 3 | L5 query only checked ANALOGOUS_TO + BRIDGES | Added COMPOSES_WITH (3 qualifying edge types) |
| 4 | Source nodes limited to L4-only | Changed to L3+ via `L5_SOURCE_MIN_LAYER` config (default: 3) |
| 5 | Co-activation param passed 0.0 | Fixed to use honest value — edge inference uses real inputs |
| 6 | Dynamic edges ran before clustering | Moved to pipeline phase 25 (post-clustering via `RunPhaseRange`) |

After Phase 75C, first consolidation of `mdemg-dev` produced 50 dynamic edges and 4 L5 emergent nodes.

### Configuration

See Configuration Reference table below.

## Notes

### Known Limitations

- L5 emergence requires at least L3 nodes to exist — new spaces need multiple consolidation cycles
- Union-find clustering is simple — no hierarchical L5 structure yet

### Risks & Gaps

None identified.

### Future Improvements

- Hierarchical L5 clustering (L5 nodes connecting to other L5 nodes)
- LLM-driven L5 naming (currently mechanical, Phase 103 Dynamic Emergence provides naming)

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| POST | `/v1/memory/consolidate` | Triggers full consolidation including L5 emergence at phase 30 | `specs/consolidate.uats.json` |

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg consolidate` | Triggers full consolidation pipeline including L5 |

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `L5_EMERGENT_ENABLED` | `true` | Enable/disable L5 emergent concept layer |
| `L5_BRIDGE_EVIDENCE_MIN` | `1` | Minimum evidence_count on qualifying edges |
| `L5_SOURCE_MIN_LAYER` | `3` | Minimum layer for source nodes (L3+ by default) |
| `DYNAMIC_EDGES_ENABLED` | `true` | Must be enabled for L5 to find qualifying edges |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Pipeline Registry (Phase 46) | Requires — L5 step registered in pipeline |
| Split Pipeline Execution (Phase 75C) | Requires — runs in post-clustering phase 30 |
| Dynamic Edges (Phase 75) | Requires — BRIDGES, ANALOGOUS_TO, COMPOSES_WITH edges must exist |
| Multi-layer Clustering | Requires — L3+ nodes must be created first |

## Related Files

- `internal/hidden/service.go` - `CreateL5EmergentNodes()` core detection algorithm
- `internal/hidden/step_emergent_l5.go` - Pipeline step adapter (phase 30)
- `internal/hidden/step_dynamic_edges.go` - Dynamic edge step (phase 25) prerequisite
- `internal/hidden/pipeline.go` - `RunPhaseRange()` enables split execution
- `internal/config/config.go` - `L5EmergentEnabled`, `L5BridgeEvidenceMin`, `L5SourceMinLayer`
- `docs/features/bridges-edge-type.md` - BRIDGES edge type (key L5 input)
- `docs/features/split-pipeline-execution.md` - Split execution pattern
