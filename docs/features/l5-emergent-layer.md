# L5 Emergent Layer

**Introduced:** Phase 75B, **Unblocked:** Phase 75C | **Depends on:** Phase 46 (Pipeline Registry), Phase 75B (dynamic edges)

## What It Does

L5 is the highest layer in the MDEMG memory graph. L5 nodes represent emergent meta-patterns — concepts that span multiple lower-layer concepts and reveal cross-domain connections. They are not explicitly authored but emerge from the graph structure itself.

The layer hierarchy:

- **L0:** Raw observations (conversation, codebase)
- **L1:** Hidden patterns (DBSCAN clusters of L0)
- **L2:** Concepts (clusters of L1)
- **L3:** Higher-order concepts
- **L4:** Abstract concepts
- **L5:** Emergent concepts (meta-patterns across L3+ nodes)

## How It Works

### Detection Algorithm

`CreateL5EmergentNodes()` in `internal/hidden/service.go`:

1. **Query source nodes:** Find all L3+ nodes (configurable via `L5_SOURCE_MIN_LAYER`) in the space
2. **Find qualifying edges:** Query ANALOGOUS_TO, BRIDGES, and COMPOSES_WITH edges between source nodes with `evidence_count >= L5_BRIDGE_EVIDENCE_MIN`
3. **Cluster:** Use union-find to group connected nodes into clusters
4. **Create L5 nodes:** For each cluster with 2+ members, create a `:MemoryNode:EmergentConcept` node at layer 5
5. **Create edges:** Add `ABSTRACTS_TO` edges from each cluster member to its L5 node

### Pipeline Execution

L5 runs as pipeline phase 30 (`emergent_l5` step), after dynamic edges (phase 25). This ordering is critical — dynamic edges must exist before L5 can find qualifying connections.

```
Phase 10-20: Core + enrichment  (pre-clustering)
Clustering:  Multi-layer L2-L5
Phase 25:    Dynamic edges       (post-clustering, creates ANALOGOUS_TO, BRIDGES, COMPOSES_WITH, etc.)
Phase 30:    L5 emergent nodes   (post-clustering, queries edges from phase 25)
```

### Phase 75C Unblocking Fixes

Six bottlenecks were fixed to enable L5 emergence:

| # | Problem | Fix |
|---|---------|-----|
| 1 | `InferEdgeType` had no BRIDGES case | Added BRIDGES inference for cross-layer + moderate similarity |
| 2 | `L5_BRIDGE_EVIDENCE_MIN` default was 3 | Lowered to 1 — L5 triggers on first consolidation |
| 3 | L5 query only checked ANALOGOUS_TO + BRIDGES | Added COMPOSES_WITH (3 qualifying edge types) |
| 4 | Source nodes limited to L4-only | Changed to L3+ via `L5_SOURCE_MIN_LAYER` config (default: 3) |
| 5 | Co-activation param passed 0.0 | Fixed to use honest value — edge inference uses real inputs |
| 6 | Dynamic edges ran before clustering | Moved to pipeline phase 25 (post-clustering via `RunPhaseRange`) |

### Results

After Phase 75C, first consolidation of `mdemg-dev` produced:

- 50 dynamic edges (various types including BRIDGES)
- 4 L5 emergent nodes

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `L5_EMERGENT_ENABLED` | `true` | Enable/disable L5 emergent concept layer |
| `L5_BRIDGE_EVIDENCE_MIN` | `1` | Minimum evidence_count on qualifying edges |
| `L5_SOURCE_MIN_LAYER` | `3` | Minimum layer for source nodes (L3+ by default) |
| `DYNAMIC_EDGES_ENABLED` | `true` | Must be enabled for L5 to find qualifying edges |

## Usage

```bash
# Run consolidation (includes L5)
curl -s -X POST http://localhost:9999/v1/memory/consolidate \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev"}' | jq '.data.steps.emergent_l5, .data.l5_nodes_created'

# Query L5 nodes in Neo4j
# MATCH (n:EmergentConcept {layer: 5}) RETURN n.name, n.summary LIMIT 10;

# Check L5 cluster membership
# MATCH (member)-[:ABSTRACTS_TO]->(l5:EmergentConcept {layer: 5})
# RETURN l5.name, collect(member.name) AS members;
```

## Related Files

| File | Purpose |
|------|---------|
| `internal/hidden/service.go` | `CreateL5EmergentNodes()` — core detection algorithm |
| `internal/hidden/step_emergent_l5.go` | Pipeline step adapter (phase 30) |
| `internal/hidden/step_dynamic_edges.go` | Dynamic edge step (phase 25) — prerequisite |
| `internal/hidden/pipeline.go` | `RunPhaseRange()` — enables split execution |
| `internal/config/config.go` | `L5EmergentEnabled`, `L5BridgeEvidenceMin`, `L5SourceMinLayer` |
| `docs/development/RELATIONSHIP_EXTRACTION.md` | Full L5 + dynamic edge documentation |
| `docs/features/bridges-edge-type.md` | BRIDGES edge type (key L5 input) |
| `docs/features/split-pipeline-execution.md` | Split execution pattern |
