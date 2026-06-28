package ftloop

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestController_DisabledRunReturnsNil(t *testing.T) {
	c := NewController(nil, nil, nil, ControllerConfig{Enabled: false})
	if err := c.Run(context.Background()); err != nil {
		t.Errorf("disabled controller Run should return nil, got %v", err)
	}
}

// fakeQuiescer records Quiesce calls.
type fakeQuiescer struct{ calls []time.Time }

func (f *fakeQuiescer) Quiesce(until time.Time) { f.calls = append(f.calls, until) }

// TestController_RunCycle_AllStages: a stub runStage walks curate→train→gate;
// the lease is acquired + the quiescer is set then cleared. nil pool → ledger
// writes no-op (state-machine logic verified without a DB).
func TestController_RunCycle_AllStages(t *testing.T) {
	q := &fakeQuiescer{}
	c := NewController(nil, q, nil, ControllerConfig{
		Enabled:       true,
		LeasePath:     filepath.Join(t.TempDir(), "ft.lease"),
		LeaseMax:      time.Hour,
		MinFreeDiskGB: 0, // skip disk floor
		RepoDir:       t.TempDir(),
	})
	var ran []string
	c.runStage = func(_ context.Context, stage string, _ []string) error {
		ran = append(ran, stage)
		return nil
	}
	c.runCycle(context.Background(), "cyc-1", "mdemg-llm-v1")

	want := []string{"curate", "train", "gate"}
	if len(ran) != 3 || ran[0] != want[0] || ran[1] != want[1] || ran[2] != want[2] {
		t.Errorf("stages ran = %v, want %v", ran, want)
	}
	// Quiesce set (lease window) then cleared (zero) on exit.
	if len(q.calls) != 2 {
		t.Fatalf("expected Quiesce set+clear, got %d calls", len(q.calls))
	}
	if q.calls[0].IsZero() {
		t.Error("first Quiesce should be the lease window, not zero")
	}
	if !q.calls[1].IsZero() {
		t.Error("second Quiesce should clear (zero)")
	}
}

// TestController_RunCycle_StageFailure: a failing stage halts the pipeline.
func TestController_RunCycle_StageFailure(t *testing.T) {
	c := NewController(nil, nil, nil, ControllerConfig{
		Enabled:   true,
		LeasePath: filepath.Join(t.TempDir(), "ft.lease"),
		LeaseMax:  time.Hour,
		RepoDir:   t.TempDir(),
	})
	var ran []string
	c.runStage = func(_ context.Context, stage string, _ []string) error {
		ran = append(ran, stage)
		if stage == "train" {
			return errors.New("mlx OOM")
		}
		return nil
	}
	c.runCycle(context.Background(), "cyc-2", "mdemg-llm-v1")
	// curate + train ran; gate must NOT (pipeline halts on failure).
	if len(ran) != 2 || ran[1] != "train" {
		t.Errorf("expected halt after train, ran = %v", ran)
	}
}

// TestController_RunCycle_DiskFloor: below the disk floor, no stage runs.
func TestController_RunCycle_DiskFloor(t *testing.T) {
	c := NewController(nil, nil, nil, ControllerConfig{
		Enabled:       true,
		LeasePath:     filepath.Join(t.TempDir(), "ft.lease"),
		LeaseMax:      time.Hour,
		MinFreeDiskGB: 1e9, // impossibly high → floor fails
		RepoDir:       t.TempDir(),
	})
	ran := false
	c.runStage = func(_ context.Context, _ string, _ []string) error { ran = true; return nil }
	c.runCycle(context.Background(), "cyc-3", "mdemg-llm-v1")
	if ran {
		t.Error("no stage should run when below the disk floor")
	}
}
