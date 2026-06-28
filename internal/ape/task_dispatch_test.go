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

// fakeTriggerGate is a stub TrainingTriggerGate for executor tests.
type fakeTriggerGate struct {
	decision   string
	suppressed bool
	err        error
}

func (f fakeTriggerGate) EvaluateTrigger(_ context.Context) (string, bool, error) {
	return f.decision, f.suppressed, f.err
}

// TestExecuteTriggerTrainingPipeline_SF2 pins the SF-2 behavior: a suppressed
// gate decision produces NO alert (ends the per-cycle spam); a trigger decision
// alerts; a nil gate falls back to the legacy alert.
func TestExecuteTriggerTrainingPipeline_SF2(t *testing.T) {
	spec := RSICTaskSpec{ActionType: "trigger_training_pipeline", TargetSpace: "mdemg-dev"}

	// Suppressed → no alert.
	capSup := &captureAlertDispatcher{}
	d := &Dispatcher{alertDispatcher: capSup, trainingTriggerGate: fakeTriggerGate{decision: "suppress_disabled", suppressed: true}}
	out, err := d.executeTriggerTrainingPipeline(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capSup.services) != 0 {
		t.Errorf("suppressed trigger must NOT alert, got %v", capSup.services)
	}
	if out["suppressed"] != true || out["decision"] != "suppress_disabled" {
		t.Errorf("expected suppressed deliverables, got %v", out)
	}

	// Trigger → the gate opened a cycle; the executor does NOT alert (the
	// controller owns outcome alerts) — cycle_opened=true, no spam.
	capTrig := &captureAlertDispatcher{}
	d2 := &Dispatcher{alertDispatcher: capTrig, trainingTriggerGate: fakeTriggerGate{decision: "trigger", suppressed: false}}
	out2, err := d2.executeTriggerTrainingPipeline(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(capTrig.services) != 0 {
		t.Errorf("trigger must NOT alert (ledger is the signal), got %v", capTrig.services)
	}
	if out2["cycle_opened"] != true {
		t.Errorf("trigger should report cycle_opened, got %v", out2)
	}

	// Nil gate → legacy alert (backward compat).
	capNil := &captureAlertDispatcher{}
	d3 := &Dispatcher{alertDispatcher: capNil}
	if _, err := d3.executeTriggerTrainingPipeline(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if len(capNil.services) != 1 {
		t.Errorf("nil gate should fall back to the legacy alert, got %v", capNil.services)
	}
}
