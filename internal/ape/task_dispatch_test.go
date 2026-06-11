package ape

import (
	"context"
	"strings"
	"testing"
)

func TestExecutors_NilDriverReturnsError(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
		sem:         make(chan struct{}, 50),
		// driver intentionally nil
	}

	tests := []struct {
		name string
		fn   func() (map[string]any, error)
	}{
		{"executeGraduateVolatile", func() (map[string]any, error) {
			return d.executeGraduateVolatile(context.Background(), "test-space")
		}},
		{"executeTombstoneStale", func() (map[string]any, error) {
			return d.executeTombstoneStale(context.Background(), "test-space", "test-cycle")
		}},
		{"executeRefreshStaleEdges", func() (map[string]any, error) {
			return d.executeRefreshStaleEdges(context.Background(), "test-space")
		}},
		{"executeCodifyAllConstraints", func() (map[string]any, error) {
			// Needs protoEvolver set to reach driver check
			d.protoEvolver = &mockProtoEvolver{}
			defer func() { d.protoEvolver = nil }()
			return d.executeCodifyAllConstraints(context.Background(), "test-space")
		}},
		{"executeAlertMemoryBloat", func() (map[string]any, error) {
			return d.executeAlertMemoryBloat(context.Background(), "test-space")
		}},
		{"executeFlushRecoveryBuffer", func() (map[string]any, error) {
			return d.executeFlushRecoveryBuffer(context.Background(), "test-space")
		}},
		{"executeReviewNLICalibration", func() (map[string]any, error) {
			return d.executeReviewNLICalibration(context.Background(), "test-space")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.fn()
			if err == nil {
				t.Fatal("expected error with nil driver")
			}
			if !strings.Contains(err.Error(), "neo4j driver not available") {
				t.Errorf("expected 'neo4j driver not available' error, got: %v", err)
			}
		})
	}
}

// mockProtoEvolver implements ProtocolEvolverProvider for nil-driver testing.
type mockProtoEvolver struct{}

func (m *mockProtoEvolver) CodifyConstraint(_ context.Context, _, _ string) (map[string]any, error) {
	return nil, nil
}
func (m *mockProtoEvolver) RetireCode(_ context.Context, _, _ string) (map[string]any, error) {
	return nil, nil
}
func (m *mockProtoEvolver) AdjustTierThresholds(_ context.Context, _ string) (map[string]any, error) {
	return nil, nil
}
func (m *mockProtoEvolver) AdjustReplayBuffer(_ context.Context, _ string) (map[string]any, error) {
	return nil, nil
}
