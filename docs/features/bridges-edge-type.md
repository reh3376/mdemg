# BRIDGES Edge Type

**Introduced:** Phase 75C | **Depends on:** Phase 75B (dynamic edge infrastructure)

## What It Does

BRIDGES is a dynamic edge type that connects nodes across different layers of the memory graph. When two nodes at different layers have moderate embedding similarity (0.4-0.7), a BRIDGES edge is created to indicate a cross-domain connection. These edges are key inputs to L5 emergent concept detection.

## How It Works

### Inference Logic

BRIDGES edges are inferred by `InferEdgeType()` in `internal/hidden/service.go` (line 2357):

```go
// Cross-layer with moderate similarity = BRIDGES
case metrics.LayerDistance > 0 && metrics.CosineSimilarity >= 0.4 && metrics.CosineSimilarity < thresholds.AnalogousMinSim:
    inferredType = EdgeBridges
    confidence = metrics.CosineSimilarity * (1.0 + 0.1*float64(metrics.LayerDistance))
```

**Triggering conditions:**

- Layer distance > 0 (nodes must be at different layers)
- Cosine similarity >= 0.4 (moderate semantic overlap)
- Cosine similarity < `AnalogousMinSim` threshold (~0.7, so not high enough for ANALOGOUS_TO)

**Confidence formula:** `similarity * (1.0 + 0.1 * layerDistance)`, capped at 1.0. Higher layer distances slightly boost confidence, reflecting that cross-layer connections spanning more layers are more structurally significant.

### Position in Edge Type Hierarchy

`InferEdgeType()` evaluates conditions in priority order:

| Priority | Condition | Edge Type |
|----------|-----------|-----------|
| 1 | High sim + same layer | ANALOGOUS_TO |
| 2 | Low sim + high co-activation | CONTRASTS_WITH |
| 3 | High co-activation + moderate sim | COMPOSES_WITH |
| 4 | **Cross-layer + moderate sim** | **BRIDGES** |
| 5 | Cross-layer + high sim | SPECIALIZES / GENERALIZES_TO |
| 6 | Default | INFLUENCES |

### Role in L5 Emergence

BRIDGES edges are one of three qualifying edge types for L5 emergent concept detection (along with ANALOGOUS_TO and COMPOSES_WITH). The L5 step queries for L3+ nodes connected by these edge types and clusters them using union-find. Without BRIDGES, cross-layer patterns could not feed into L5 emergence.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DYNAMIC_EDGES_ENABLED` | `true` | Master toggle for all dynamic edge creation |
| `DYNAMIC_EDGE_MIN_CONFIDENCE` | `0.5` | Minimum confidence for any dynamic edge |
| `DYNAMIC_EDGE_DEGREE_CAP` | `10` | Max dynamic edges per node |
| `L5_SOURCE_MIN_LAYER` | `3` | Minimum layer for source nodes (affects which nodes get BRIDGES edges) |

## Usage

BRIDGES edges are created automatically during consolidation (pipeline phase 25, `dynamic_edges` step). Check them via:

```bash
# See dynamic edge counts in consolidation output
curl -s -X POST http://localhost:9999/v1/memory/consolidate \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev"}' | jq '.data.steps.dynamic_edges'

# Query BRIDGES edges directly in Neo4j
# MATCH ()-[r:BRIDGES]->() RETURN count(r)
```

## Related Files

| File | Purpose |
|------|---------|
| `internal/hidden/service.go` | `InferEdgeType()` — BRIDGES inference logic (line 2357) |
| `internal/hidden/service.go` | `CreateDynamicEdges()` — creates all dynamic edges including BRIDGES |
| `internal/hidden/step_dynamic_edges.go` | Pipeline step adapter (phase 25) |
| `internal/config/config.go` | `DynamicEdgesEnabled`, `DynamicEdgeMinConfidence`, `DynamicEdgeDegreeCap` |
| `docs/development/RELATIONSHIP_EXTRACTION.md` | Full dynamic edge documentation |
