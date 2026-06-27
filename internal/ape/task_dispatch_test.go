package ape

import (
	"context"
	"strings"
	"testing"
)

// captureAlertDispatcher records the (service,title) of each SendAlert call so
// the SF-3 contract (distinct Service per RSIC diagnostic action) can be pinned.
type captureAlertDispatcher struct {
	services []string
	titles   []string
}

func (c *captureAlertDispatcher) SendAlert(_ context.Context, service, title, _ string, _ InsightSeverity) {
	c.services = append(c.services, service)
	c.titles = append(c.titles, title)
}

// TestExecuteAlertLog_DistinctServicePerAction pins SF-3 (FT-RECURSIVE-001):
// each RSIC diagnostic action must alert under its own Service so the
// dispatcher's (Service,Severity) cooldown key does not make them suppress
// each other.
func TestExecuteAlertLog_DistinctServicePerAction(t *testing.T) {
	cap := &captureAlertDispatcher{}
	d := &Dispatcher{alertDispatcher: cap}
	actions := []string{"trigger_training_pipeline", "alert_llm_health", "alert_embedding_regression"}
	for _, a := range actions {
		if _, err := d.executeAlertLog(context.Background(), RSICTaskSpec{ActionType: a}, "msg"); err != nil {
			t.Fatalf("executeAlertLog(%s): %v", a, err)
		}
	}
	seen := make(map[string]bool)
	for i, a := range actions {
		want := "rsic-" + a
		if cap.services[i] != want {
			t.Errorf("action %s: service = %q, want %q", a, cap.services[i], want)
		}
		if seen[cap.services[i]] {
			t.Errorf("duplicate service %q — actions would suppress each other", cap.services[i])
		}
		seen[cap.services[i]] = true
	}
}

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

// COOLER-001: executeGraduateVolatile now delegates to the Context Cooler
// graduation processor instead of an inline Cypher; it fails safe (not a
// nil-driver panic) when the processor is unwired.
func TestExecuteGraduateVolatile_NilProcessor(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
		sem:         make(chan struct{}, 50),
		// graduationProcessor intentionally nil
	}
	if _, err := d.executeGraduateVolatile(context.Background(), "test-space"); err == nil {
		t.Fatal("expected error when graduation processor not wired")
	}
}

// COOLER-001: when wired, executeGraduateVolatile returns the cooler's count.
func TestExecuteGraduateVolatile_DelegatesToProcessor(t *testing.T) {
	d := &Dispatcher{
		activeTasks:         make(map[string]*activeTask),
		reports:             make(map[string][]RSICProgressReport),
		sem:                 make(chan struct{}, 50),
		graduationProcessor: stubGraduationProcessor{graduated: 7},
	}
	out, err := d.executeGraduateVolatile(context.Background(), "test-space")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["graduated"] != 7 {
		t.Errorf("expected graduated=7 from processor, got %v", out["graduated"])
	}
}

type stubGraduationProcessor struct{ graduated int }

func (s stubGraduationProcessor) ProcessGraduations(_ context.Context, _ string) (int, error) {
	return s.graduated, nil
}
