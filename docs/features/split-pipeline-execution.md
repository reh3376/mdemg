# Split Pipeline Execution

**Introduced:** Phase 75C | **Depends on:** Phase 46 (Pipeline Registry)

## What It Does

Split pipeline execution allows the consolidation pipeline to run subsets of steps at different points in the consolidation lifecycle. Instead of running all steps sequentially via `RunAll()`, the handler calls `RunPhaseRange(min, max)` to execute only steps within a specific phase range.

This was needed because dynamic edge creation (phase 25) and L5 emergent node detection (phase 30) require fully clustered graph state. Running them before multi-layer clustering would mean they operate on stale or incomplete data.

## How It Works

### Pipeline Phases

| Phase | Category | Steps | When |
|-------|----------|-------|------|
| 10 | Core | `hidden` | Pre-clustering |
| 20 | Enrichment | `concern`, `config`, `comparison`, `temporal`, `ui`, `constraint` | Pre-clustering |
| 25 | Dynamic edges | `dynamic_edges` | Post-clustering |
| 30 | Post-processing | `emergent_l5` | Post-clustering |

### Execution Flow

```
1. RunPhaseRange(10, 20)    → Core + enrichment steps
2. Multi-layer clustering   → L2-L5 concept clustering with interleaved forward passes
3. RunPhaseRange(25, 30)    → Dynamic edges + L5 emergent nodes
4. Backward pass + summaries
```

### RunPhaseRange API

```go
// RunPhaseRange executes only steps whose Phase() is in [minPhase, maxPhase].
// Steps outside the range are silently skipped. Skip map is also honoured.
func (p *Pipeline) RunPhaseRange(ctx context.Context, spaceID string, skip map[string]bool, minPhase, maxPhase int) (*PipelineResult, error)
```

The method iterates all registered steps, skips those outside `[minPhase, maxPhase]`, and executes the rest in phase order. Error handling follows the same required/optional semantics as `RunAll()`.

### Handler Integration

In `internal/api/handlers.go`, the consolidation handler calls the pipeline twice:

```go
// Pre-clustering: node creation + enrichment
preResult, err := hiddenSvc.RunNodeCreationPipeline(ctx, spaceID)  // phases 10-20

// ... multi-layer clustering happens here ...

// Post-clustering: dynamic edges + L5
postResult, err := hiddenSvc.RunPostClusteringPipeline(ctx, spaceID)  // phases 25-30
```

Results from both calls are merged into the single `steps` map in the API response.

## Configuration

No additional configuration. Phase ranges are hardcoded in the service methods:

- `RunNodeCreationPipeline()` → `RunPhaseRange(ctx, spaceID, nil, 10, 20)`
- `RunPostClusteringPipeline()` → `RunPhaseRange(ctx, spaceID, nil, 25, 30)`

## Adding a Post-Clustering Step

1. Create the step adapter with `Phase()` returning 25-30
2. Register it in `buildPipeline()` in `internal/hidden/service.go`
3. It will automatically run in the post-clustering phase

## Related Files

| File | Purpose |
|------|---------|
| `internal/hidden/pipeline.go` | `RunPhaseRange()` method (line 109) |
| `internal/hidden/service.go` | `RunNodeCreationPipeline()`, `RunPostClusteringPipeline()` |
| `internal/hidden/step_dynamic_edges.go` | Phase 25 step |
| `internal/hidden/step_emergent_l5.go` | Phase 30 step |
| `internal/api/handlers.go` | Two-phase pipeline calls in consolidation handler |
| `docs/development/REGISTRY.md` | Full pipeline registry documentation |
