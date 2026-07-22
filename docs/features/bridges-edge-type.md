---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "75C"
---

# BRIDGES Edge Type

## Summary

**Feature**: BRIDGES Edge Type
**Summary**: A dynamic edge type that connects nodes across different layers of the memory graph when they have moderate embedding similarity (0.4-0.7), indicating cross-domain connections that are key inputs to L5 emergent concept detection.

## Vision & Goals

Cross-domain connections are the most valuable discoveries in a knowledge graph. BRIDGES edges capture relationships that span abstraction layers — connecting a low-level implementation detail to a high-level architectural concept, for example. These are the edges that power genuine insight and emergence at L5, bridging semantic domains that traditional same-layer clustering would never connect.

## Current State

### Architecture

BRIDGES edges are inferred by `InferEdgeType()` in `internal/hidden/service.go`:

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

### Workflow

**Position in Edge Type Hierarchy** — `InferEdgeType()` evaluates conditions in priority order:

| Priority | Condition | Edge Type |
|----------|-----------|-----------|
| 1 | High sim + same layer | ANALOGOUS_TO |
| 2 | Low sim + high co-activation | CONTRASTS_WITH |
| 3 | High co-activation + moderate sim | COMPOSES_WITH |
| 4 | **Cross-layer + moderate sim** | **BRIDGES** |
| 5 | Cross-layer + high sim | SPECIALIZES / GENERALIZES_TO |
| 6 | Default | INFLUENCES |

**Role in L5 Emergence:** BRIDGES edges are one of three qualifying edge types for L5 emergent concept detection (along with ANALOGOUS_TO and COMPOSES_WITH). The L5 step queries for L3+ nodes connected by these edge types and clusters them using union-find. Without BRIDGES, cross-layer patterns could not feed into L5 emergence.

BRIDGES edges are created automatically during consolidation (pipeline phase 25, `dynamic_edges` step). Since RETRIEVAL-TYPED-EDGES-002 (2026-07-03) candidate pairs come from a per-node top-K query over the `memNodeEmbedding` vector index (not a Cartesian cross-join), with endpoints gated by `DYNAMIC_EDGE_MIN_LAYER` (default 1, so L1/L2 nodes participate — not just L3+).

### Configuration

See Configuration Reference table below.

## Notes

### Known Limitations

- Similarity thresholds (0.4, 0.7) are not independently configurable — derived from AnalogousMinSim
- Only considers cosine similarity and layer distance — no temporal or co-activation weighting

### Risks & Gaps

None identified.

### Future Improvements

- Temporal weighting for BRIDGES (recently co-retrieved nodes get stronger bridges)
- Configurable similarity band for BRIDGES inference

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| POST | `/v1/memory/consolidate` | Triggers consolidation including dynamic edge creation at phase 25 | `specs/consolidate.uats.json` |

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg consolidate` | Triggers full consolidation pipeline including dynamic edges |

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `DYNAMIC_EDGES_ENABLED` | `true` | Master toggle for all dynamic edge creation |
| `DYNAMIC_EDGE_MIN_CONFIDENCE` | `0.5` | Minimum confidence for any dynamic edge |
| `DYNAMIC_EDGE_DEGREE_CAP` | `10` | Max dynamic edges per node |
| `DYNAMIC_EDGE_MIN_LAYER` | `1` | Minimum layer for dynamic semantic-edge endpoints (L0 excluded) |
| `DYNAMIC_EDGE_TOPK` | `10` | Per-node nearest-neighbor edges to consider |
| `DYNAMIC_EDGE_SIM_THRESHOLD` | `0.30` | Minimum cosine similarity for a dynamic edge |
| `DYNAMIC_EDGE_OVERSAMPLE` | `8` | Vector-index fetch multiplier (TopK×Oversample, then layer/space/degree filter) |
| `L5_SOURCE_MIN_LAYER` | `3` | Minimum layer for L5 emergence source nodes (dynamic-edge endpoints gate on `DYNAMIC_EDGE_MIN_LAYER`) |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Dynamic Edge Infrastructure (Phase 75B) | Requires — BRIDGES uses the dynamic edge creation framework |
| Split Pipeline Execution (Phase 75C) | Requires — runs in post-clustering phase 25 |
| L5 Emergent Layer (Phase 75B) | Feeds into — BRIDGES edges are key input for L5 detection |
| Multi-layer Clustering | Requires — needs nodes at different layers to create cross-layer edges |

## Related Files

- `internal/hidden/service.go` - `InferEdgeType()` BRIDGES inference logic and `CreateDynamicEdges()`
- `internal/hidden/step_dynamic_edges.go` - Pipeline step adapter (phase 25)
- `internal/config/config.go` - `DynamicEdgesEnabled`, `DynamicEdgeMinConfidence`, `DynamicEdgeDegreeCap`
- `docs/development/RELATIONSHIP_EXTRACTION.md` - Full dynamic edge documentation
- `docs/features/l5-emergent-layer.md` - L5 emergent layer (consumes BRIDGES edges)
