---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "75C"
---

# Split Pipeline Execution

## Summary

**Feature**: Split Pipeline Execution
**Summary**: Allows the consolidation pipeline to run subsets of steps at different points in the consolidation lifecycle via `RunPhaseRange(min, max)`, ensuring dynamic edges and L5 emergence operate on fully clustered graph state.

## Vision & Goals

The consolidation pipeline creates hidden nodes, enriches them, clusters into concepts, and then creates emergent edges and L5 meta-patterns. Dynamic edge creation (phase 25) and L5 emergent node detection (phase 30) require fully clustered state — running them before multi-layer clustering means they operate on stale or incomplete data. Split execution solves this by running pre-clustering steps first, clustering in the middle, then post-clustering steps.

## Current State

### Architecture

| Phase | Category | Steps | When |
|-------|----------|-------|------|
| 10 | Core | `hidden` | Pre-clustering |
| 20 | Enrichment | `concern`, `config`, `comparison`, `temporal`, `ui`, `constraint`, `correction` | Pre-clustering |
| 22 | Emergence | `dynamic_emergence` | Pre-clustering (skipped when emergence disabled) |
| 25 | Dynamic edges | `dynamic_edges` | Post-clustering |
| 30 | Post-processing | `emergent_l5` | Post-clustering |

### Workflow

```
1. RunPhaseRange(10, 22)    -> Core + enrichment steps
2. Multi-layer clustering   -> L2-L5 concept clustering with interleaved forward passes
3. RunPhaseRange(25, 30)    -> Dynamic edges + L5 emergent nodes
4. Backward pass + summaries
```

**RunPhaseRange API** (Go internal):

```go
func (p *Pipeline) RunPhaseRange(ctx context.Context, spaceID string, skip map[string]bool, minPhase, maxPhase int) (*PipelineResult, error)
```

The method iterates all registered steps, skips those outside `[minPhase, maxPhase]`, and executes the rest in phase order. Error handling follows the same required/optional semantics as `RunAll()`.

**Handler Integration** — the consolidation handler calls the pipeline twice:

```go
preResult, err := hiddenSvc.RunNodeCreationPipeline(ctx, spaceID)   // phases 10-22
// ... multi-layer clustering happens here ...
postResult, err := hiddenSvc.RunPostClusteringPipeline(ctx, spaceID) // phases 25-30
```

Results from both calls are merged into the single `steps` map in the API response.

### Configuration

No additional configuration. Phase ranges are hardcoded in the service methods:

- `RunNodeCreationPipeline()` -> `RunPhaseRange(ctx, spaceID, skip, 10, 22)`
- `RunPostClusteringPipeline()` -> `RunPhaseRange(ctx, spaceID, nil, 25, 30)`

## Notes

### Known Limitations

- Phase ranges are hardcoded — not configurable via env vars
- Adding a new phase category requires code changes to the service methods

### Risks & Gaps

None identified.

### Future Improvements

- Configurable phase range boundaries
- Event-driven step execution (steps declare prerequisites instead of fixed phases)

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| POST | `/v1/memory/consolidate` | Triggers full consolidation (both phase ranges) | `specs/consolidate.uats.json` |

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg consolidate` | Triggers full consolidation pipeline |

## Configuration Reference

None — phase ranges are hardcoded.

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Pipeline Registry (Phase 46) | Requires — `RunPhaseRange` operates on registered steps |
| Hidden Layer Clustering | Requires — runs between the two phase ranges |
| Dynamic Edges (Phase 75) | Feeds into — runs in post-clustering phase 25 |
| L5 Emergent Layer (Phase 75B) | Feeds into — runs in post-clustering phase 30 |

## Related Files

- `internal/hidden/pipeline.go` - `RunPhaseRange()` method
- `internal/hidden/service.go` - `RunNodeCreationPipeline()`, `RunPostClusteringPipeline()`
- `internal/hidden/step_dynamic_edges.go` - Phase 25 step
- `internal/hidden/step_emergent_l5.go` - Phase 30 step
- `internal/api/handlers.go` - Two-phase pipeline calls in consolidation handler
- `docs/development/REGISTRY.md` - Full pipeline registry documentation
