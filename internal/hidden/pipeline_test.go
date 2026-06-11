package hidden

import (
	"context"
	"errors"
	"testing"
)

// mockStep is a configurable mock NodeCreator for testing.
type mockStep struct {
	name     string
	phase    int
	required bool
	result   *StepResult
	err      error
}

func (m *mockStep) Name() string   { return m.name }
func (m *mockStep) Phase() int     { return m.phase }
func (m *mockStep) Required() bool { return m.required }
func (m *mockStep) Run(_ context.Context, _ string) (*StepResult, error) {
	return m.result, m.err
}

func TestPipeline_PhaseOrdering(t *testing.T) {
	p := NewPipeline()
	// Register out of order
	p.Register(&mockStep{name: "c", phase: 30})
	p.Register(&mockStep{name: "a", phase: 10})
	p.Register(&mockStep{name: "b", phase: 20})

	names := p.Names()
	if len(names) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(names))
	}
	if names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Errorf("expected [a, b, c], got %v", names)
	}
}

func TestPipeline_RunAll_AggregatesResults(t *testing.T) {
	p := NewPipeline()
	p.Register(&mockStep{
		name: "step1", phase: 10, required: true,
		result: &StepResult{NodesCreated: 5, EdgesCreated: 3},
	})
	p.Register(&mockStep{
		name: "step2", phase: 20, required: false,
		result: &StepResult{NodesCreated: 2, NodesUpdated: 1, EdgesCreated: 4},
	})

	result, err := p.RunAll(context.Background(), "test-space", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.Steps))
	}
	// 5 + 2 + 1(updated) = 8 total nodes
	if result.TotalNodes != 8 {
		t.Errorf("expected TotalNodes=8, got %d", result.TotalNodes)
	}
	// 3 + 4 = 7 total edges
	if result.TotalEdges != 7 {
		t.Errorf("expected TotalEdges=7, got %d", result.TotalEdges)
	}

	// Verify individual step results
	s1 := result.Steps["step1"]
	if s1 == nil || s1.NodesCreated != 5 {
		t.Errorf("step1 result incorrect: %+v", s1)
	}
	s2 := result.Steps["step2"]
	if s2 == nil || s2.NodesCreated != 2 || s2.NodesUpdated != 1 {
		t.Errorf("step2 result incorrect: %+v", s2)
	}
}

func TestPipeline_RunAll_RequiredError_Aborts(t *testing.T) {
	p := NewPipeline()
	p.Register(&mockStep{
		name: "required_fail", phase: 10, required: true,
		err: errors.New("critical failure"),
	})
	p.Register(&mockStep{
		name: "never_reached", phase: 20, required: false,
		result: &StepResult{NodesCreated: 99},
	})

	result, err := p.RunAll(context.Background(), "test-space", nil)
	if err == nil {
		t.Fatal("expected error from required step, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on required failure, got %+v", result)
	}
}

func TestPipeline_RunAll_OptionalError_Continues(t *testing.T) {
	p := NewPipeline()
	p.Register(&mockStep{
		name: "core", phase: 10, required: true,
		result: &StepResult{NodesCreated: 3},
	})
	p.Register(&mockStep{
		name: "optional_fail", phase: 20, required: false,
		err: errors.New("non-critical failure"),
	})
	p.Register(&mockStep{
		name: "after_fail", phase: 30, required: false,
		result: &StepResult{NodesCreated: 1},
	})

	result, err := p.RunAll(context.Background(), "test-space", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Two successful steps
	if len(result.Steps) != 2 {
		t.Errorf("expected 2 step results, got %d", len(result.Steps))
	}
	// One error logged
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Step != "optional_fail" {
		t.Errorf("expected error from 'optional_fail', got %q", result.Errors[0].Step)
	}
	// TotalNodes: 3 + 1 = 4
	if result.TotalNodes != 4 {
		t.Errorf("expected TotalNodes=4, got %d", result.TotalNodes)
	}
}

func TestPipeline_RunAll_SkipMap(t *testing.T) {
	p := NewPipeline()
	p.Register(&mockStep{
		name: "keep", phase: 10, required: true,
		result: &StepResult{NodesCreated: 5},
	})
	p.Register(&mockStep{
		name: "skip_me", phase: 20, required: false,
		result: &StepResult{NodesCreated: 100},
	})

	skip := map[string]bool{"skip_me": true}
	result, err := p.RunAll(context.Background(), "test-space", skip)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Errorf("expected 1 step result, got %d", len(result.Steps))
	}
	if _, ok := result.Steps["skip_me"]; ok {
		t.Error("skip_me should not be in results")
	}
	if result.TotalNodes != 5 {
		t.Errorf("expected TotalNodes=5, got %d", result.TotalNodes)
	}
}

func TestPipeline_RunAll_EmptyPipeline(t *testing.T) {
	p := NewPipeline()
	result, err := p.RunAll(context.Background(), "test-space", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(result.Steps))
	}
	if result.TotalNodes != 0 || result.TotalEdges != 0 {
		t.Error("expected zero totals for empty pipeline")
	}
}

func TestPipeline_RunAll_NilResult_Handled(t *testing.T) {
	p := NewPipeline()
	p.Register(&mockStep{
		name: "nil_result", phase: 10, required: false,
		result: nil, err: nil,
	})

	result, err := p.RunAll(context.Background(), "test-space", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// nil result should not appear in Steps map
	if _, ok := result.Steps["nil_result"]; ok {
		t.Error("nil result should not be stored in Steps")
	}
}

func TestPipeline_RunAll_Details_Preserved(t *testing.T) {
	p := NewPipeline()
	p.Register(&mockStep{
		name: "with_details", phase: 10, required: false,
		result: &StepResult{
			NodesCreated: 2,
			Details: map[string]any{
				"concerns": []string{"auth", "rbac"},
			},
		},
	})

	result, err := p.RunAll(context.Background(), "test-space", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sr := result.Steps["with_details"]
	if sr == nil {
		t.Fatal("expected step result")
	}
	concerns, ok := sr.Details["concerns"].([]string)
	if !ok || len(concerns) != 2 {
		t.Errorf("expected concerns [auth, rbac], got %v", sr.Details["concerns"])
	}
}

// HIDDEN-CHURN-001: RunConsolidation must include phase 22 via the single
// range source — the hardcoded (10,20) range silently skipped emergence.
func TestNodeCreationRange_IncludesEmergencePhase(t *testing.T) {
	s := &dynamicEmergenceStep{}
	if s.Phase() < 10 || s.Phase() > 22 {
		t.Fatalf("dynamic_emergence phase %d outside the node-creation range [10,22] — RunNodeCreationPipeline/RunConsolidation will silently skip it", s.Phase())
	}
}
