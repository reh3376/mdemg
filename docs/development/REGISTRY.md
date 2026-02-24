# Dynamic Pipeline Registry

**Phase 46** | Introduced: 2026-02-07

## Problem

Before Phase 46, adding a new consolidation node type (concern, config, comparison, temporal, UI, constraint) required changes in **four separate files**:

1. `internal/hidden/service.go` — add step to `RunConsolidation()`
2. `internal/api/handlers.go` — add step to `handleConsolidate()` (duplicate logic)
3. `internal/hidden/types.go` — add field to `ConsolidationResult`
4. `internal/models/models.go` — add flat fields to `ConsolidateResponse`

This violated the Open/Closed Principle. The handler and service maintained duplicate step lists that had to stay in sync. Every new node type risked bugs from missed or inconsistent updates across the four files.

## Solution

A pipeline registry pattern where consolidation steps are self-registering units. Adding a new node type is now a **two-file operation**: create the step file, register it in `buildPipeline()`.

## Architecture

### Core Interface

```go
// internal/hidden/pipeline.go

type NodeCreator interface {
    Name() string                                              // Step identifier
    Phase() int                                                // Execution order (10=core, 20=enrichment, 25=dynamic edges, 30=post-processing)
    Required() bool                                            // If true, errors abort the pipeline
    Run(ctx context.Context, spaceID string) (*StepResult, error)
}

type StepResult struct {
    NodesCreated int            `json:"nodes_created"`
    NodesUpdated int            `json:"nodes_updated"`
    EdgesCreated int            `json:"edges_created"`
    Details      map[string]any `json:"details,omitempty"`
}
```

Every consolidation step implements `NodeCreator`. The `Pipeline` struct holds registered steps sorted by `Phase()` and executes them via `RunAll()` or selectively via `RunPhaseRange(min, max)` for split execution.

### Step Adapters

Each existing `Create*Nodes()` method has a thin adapter in its own file:

| File | Step Name | Phase | Required | Wraps |
|------|-----------|-------|----------|-------|
| `step_hidden.go` | `hidden` | 10 | yes | `CreateHiddenNodes` |
| `step_concern.go` | `concern` | 20 | no | `CreateConcernNodes` |
| `step_config.go` | `config` | 20 | no | `CreateConfigNodes` |
| `step_comparison.go` | `comparison` | 20 | no | `CreateComparisonNodes` |
| `step_temporal.go` | `temporal` | 20 | no | `CreateTemporalNodes` |
| `step_ui.go` | `ui` | 20 | no | `CreateUINodes` |
| `step_constraint.go` | `constraint` | 20 | no | `CreateConstraintNodes` |
| `step_dynamic_edges.go` | `dynamic_edges` | 25 | no | `CreateDynamicEdges` |
| `step_emergent_l5.go` | `emergent_l5` | 30 | no | `CreateL5EmergentNodes` |

### Error Handling

- **Required steps** (phase 10, `hidden`): failure aborts the pipeline and returns an error.
- **Optional steps** (phase 20/25/30, enrichment and post-processing): failure is logged in `PipelineResult.Errors` and execution continues. This matches the pre-pipeline behavior where enrichment steps logged warnings without failing consolidation.

### Registration

```go
// internal/hidden/service.go

func (s *Service) buildPipeline() *Pipeline {
    p := NewPipeline()
    p.Register(&hiddenStep{svc: s})
    p.Register(&concernStep{svc: s})
    p.Register(&configStep{svc: s})
    p.Register(&comparisonStep{svc: s})
    p.Register(&temporalStep{svc: s})
    p.Register(&uiStep{svc: s})
    p.Register(&constraintStep{svc: s})
    p.Register(&dynamicEdgesStep{svc: s})  // phase 25 — dynamic edges (after clustering)
    p.Register(&emergentL5Step{svc: s})    // phase 30 — post-processing
    return p
}
```

The pipeline is built once in `NewService()` and stored on the `Service` struct.

## API Response

The consolidate endpoint (`POST /v1/memory/consolidate`) now returns a dynamic `steps` map alongside the existing flat fields:

```json
{
  "data": {
    "space_id": "example",
    "enabled": true,
    "steps": {
      "hidden":     { "nodes_created": 5, "nodes_updated": 0, "edges_created": 0 },
      "concern":    { "nodes_created": 2, "nodes_updated": 0, "edges_created": 4, "details": {"concerns": ["auth"]} },
      "constraint": { "nodes_created": 1, "nodes_updated": 0, "edges_created": 1 }
    },
    "hidden_nodes_created": 5,
    "concept_nodes_created": 3,
    "constraint_nodes_created": 1,
    "duration_ms": 2400
  }
}
```

The `steps` map auto-expands when new steps are registered. The flat fields (`hidden_nodes_created`, `concern_nodes_created`, etc.) are preserved for backward compatibility and populated from the pipeline results.

## How to Add a New Node Type

1. **Create the step adapter** (`internal/hidden/step_yourtype.go`):

```go
package hidden

import "context"

type yourTypeStep struct{ svc *Service }

func (s *yourTypeStep) Name() string   { return "your_type" }
func (s *yourTypeStep) Phase() int     { return 20 }
func (s *yourTypeStep) Required() bool { return false }

func (s *yourTypeStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
    r, err := s.svc.CreateYourTypeNodes(ctx, spaceID)
    if err != nil {
        return nil, err
    }
    return &StepResult{
        NodesCreated: r.Created,
        EdgesCreated: r.Linked,
    }, nil
}
```

1. **Register it** in `buildPipeline()` (`internal/hidden/service.go`):

```go
p.Register(&yourTypeStep{svc: s})
```

That's it. The handler, response model, and UATS automatically pick up the new step through the `steps` map. No other files need modification.

## Files

### New files (Phase 46)

| File | Purpose |
|------|---------|
| `internal/hidden/pipeline.go` | `NodeCreator` interface, `Pipeline` struct, `StepResult`, `PipelineResult` |
| `internal/hidden/pipeline_test.go` | 8 unit tests covering phase ordering, aggregation, error handling, skip map |
| `internal/hidden/step_hidden.go` | Adapter for `CreateHiddenNodes` |
| `internal/hidden/step_concern.go` | Adapter for `CreateConcernNodes` |
| `internal/hidden/step_config.go` | Adapter for `CreateConfigNodes` |
| `internal/hidden/step_comparison.go` | Adapter for `CreateComparisonNodes` |
| `internal/hidden/step_temporal.go` | Adapter for `CreateTemporalNodes` |
| `internal/hidden/step_ui.go` | Adapter for `CreateUINodes` |
| `internal/hidden/step_constraint.go` | Adapter for `CreateConstraintNodes` |
| `internal/hidden/step_dynamic_edges.go` | Adapter for `CreateDynamicEdges` (Phase 75C) |
| `internal/hidden/step_emergent_l5.go` | Adapter for `CreateL5EmergentNodes` (Phase 75C) |

### Modified files

| File | Change |
|------|--------|
| `internal/hidden/service.go` | Added `pipeline` field, `buildPipeline()`, `RunNodeCreationPipeline()`. Rewired `RunConsolidation()` to use pipeline internally. |
| `internal/hidden/types.go` | Added `PipelineSteps` field to `ConsolidationResult` |
| `internal/api/handlers.go` | Replaced 7 individual step calls with single `RunNodeCreationPipeline()` call + backward-compat field mapping |
| `internal/models/models.go` | Added `StepResultAPI` struct and `Steps` map to `ConsolidateResponse` |

## Scope and Boundaries

The pipeline covers **node-creation and post-processing steps** across two execution phases:

- **Pre-clustering** (phases 10-20): Core node creation (`hidden`) and enrichment steps (`concern`, `config`, `comparison`, `temporal`, `ui`, `constraint`). Executed via `RunPhaseRange(10, 20)`.
- **Post-clustering** (phases 25-30): `dynamic_edges` (phase 25) and `emergent_l5` (phase 30). Executed via `RunPhaseRange(25, 30)` after multi-layer clustering completes.

This split execution (introduced in Phase 75C) ensures dynamic edges and L5 emergent nodes have access to fully clustered graph state.

The following operations remain explicit in the handler and `RunConsolidation()` because they depend on pipeline output or operate differently:

- Forward pass (updates embeddings after node creation)
- Multi-layer concept clustering (iterative L2-L5 with interleaved forward passes)
- Backward pass (propagates signals down)
- Summary generation
- Stale edge refresh

## Future Application

This pattern is designed to be adopted codebase-wide for all pipeline-style processing:

- **Conversation consolidation** (`RunFullConversationConsolidation`) — same step-by-step pattern
- **RSIC task dispatch** (`Dispatcher.executeTask`) — switch-case that could use registered handlers
- **Retrieval pipeline** — multi-stage retrieval (vector, rerank, filter) as pipeline steps

These are follow-up refactors, not part of Phase 46.

## Testing

```bash
# Pipeline unit tests (no server needed)
go test ./internal/hidden/ -run TestPipeline -v

# Full API contract validation (requires running server)
make test-api
```

Pipeline unit tests verify: phase ordering, result aggregation, required/optional error handling, skip map, nil safety, and details preservation.
