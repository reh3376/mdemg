package hidden

import (
	"context"
	"fmt"
	"sort"
	"time"

	"mdemg/internal/metrics"
)

// StepResult is the universal result type for any pipeline step.
type StepResult struct {
	NodesCreated int            `json:"nodes_created"`
	NodesUpdated int            `json:"nodes_updated"`
	EdgesCreated int            `json:"edges_created"`
	Details      map[string]any `json:"details,omitempty"`
}

// StepError records a non-fatal step failure.
type StepError struct {
	Step    string `json:"step"`
	Message string `json:"message"`
}

// NodeCreator is the interface every consolidation step implements.
type NodeCreator interface {
	// Name returns the step identifier (e.g., "hidden", "concern", "constraint").
	Name() string
	// Phase returns execution order (10=core, 20=enrichment, 30=post-processing).
	Phase() int
	// Required returns true if errors should abort the pipeline.
	Required() bool
	// Run executes the step and returns a universal result.
	Run(ctx context.Context, spaceID string) (*StepResult, error)
}

// PipelineResult holds the aggregated output of a full pipeline run.
type PipelineResult struct {
	Steps      map[string]*StepResult `json:"steps"`
	TotalNodes int                    `json:"total_nodes_created"`
	TotalEdges int                    `json:"total_edges_created"`
	DurationMs float64                `json:"duration_ms"`
	Errors     []StepError            `json:"errors,omitempty"`
}

// Pipeline is an ordered collection of NodeCreator steps.
type Pipeline struct {
	steps []NodeCreator
}

// NewPipeline creates an empty pipeline.
func NewPipeline() *Pipeline {
	return &Pipeline{}
}

// Register adds a step and keeps steps sorted by phase.
func (p *Pipeline) Register(step NodeCreator) {
	p.steps = append(p.steps, step)
	sort.Slice(p.steps, func(i, j int) bool {
		return p.steps[i].Phase() < p.steps[j].Phase()
	})
}

// Names returns the registered step names in phase order.
func (p *Pipeline) Names() []string {
	names := make([]string, len(p.steps))
	for i, s := range p.steps {
		names[i] = s.Name()
	}
	return names
}

// RunAll executes all registered steps in phase order.
// Steps listed in skip are silently bypassed.
// Required steps that fail abort the pipeline; optional steps log the error and continue.
func (p *Pipeline) RunAll(ctx context.Context, spaceID string, skip map[string]bool) (*PipelineResult, error) {
	start := time.Now()
	result := &PipelineResult{
		Steps: make(map[string]*StepResult, len(p.steps)),
	}

	for _, step := range p.steps {
		if skip[step.Name()] {
			continue
		}

		stepStart := time.Now()
		sr, err := step.Run(ctx, spaceID)
		// CONSOLIDATE-PERF-001: per-step timing so the post-clustering breakdown
		// (dynamic_edges vs emergent_l5 vs cluster_summary) is observable — it was
		// the ~29-min dynamic_edges O(n²) cross-join hiding inside post_clustering.
		metrics.RecordConsolidationPhase(spaceID, "step:"+step.Name(), stepStart)
		if err != nil {
			if step.Required() {
				return nil, fmt.Errorf("pipeline step %q: %w", step.Name(), err)
			}
			result.Errors = append(result.Errors, StepError{
				Step:    step.Name(),
				Message: err.Error(),
			})
			continue
		}

		if sr != nil {
			result.Steps[step.Name()] = sr
			result.TotalNodes += sr.NodesCreated + sr.NodesUpdated
			result.TotalEdges += sr.EdgesCreated
		}
	}

	result.DurationMs = float64(time.Since(start).Milliseconds())
	return result, nil
}

// RunPhaseRange executes only steps whose Phase() is in [minPhase, maxPhase].
// Steps outside the range are silently skipped. Skip map is also honoured.
func (p *Pipeline) RunPhaseRange(ctx context.Context, spaceID string, skip map[string]bool, minPhase, maxPhase int) (*PipelineResult, error) {
	start := time.Now()
	result := &PipelineResult{
		Steps: make(map[string]*StepResult, len(p.steps)),
	}

	for _, step := range p.steps {
		if step.Phase() < minPhase || step.Phase() > maxPhase {
			continue
		}
		if skip[step.Name()] {
			continue
		}

		stepStart := time.Now()
		sr, err := step.Run(ctx, spaceID)
		// CONSOLIDATE-PERF-001: per-step timing so the post-clustering breakdown
		// (dynamic_edges vs emergent_l5 vs cluster_summary) is observable — it was
		// the ~29-min dynamic_edges O(n²) cross-join hiding inside post_clustering.
		metrics.RecordConsolidationPhase(spaceID, "step:"+step.Name(), stepStart)
		if err != nil {
			if step.Required() {
				return nil, fmt.Errorf("pipeline step %q: %w", step.Name(), err)
			}
			result.Errors = append(result.Errors, StepError{
				Step:    step.Name(),
				Message: err.Error(),
			})
			continue
		}

		if sr != nil {
			result.Steps[step.Name()] = sr
			result.TotalNodes += sr.NodesCreated + sr.NodesUpdated
			result.TotalEdges += sr.EdgesCreated
		}
	}

	result.DurationMs = float64(time.Since(start).Milliseconds())
	return result, nil
}
