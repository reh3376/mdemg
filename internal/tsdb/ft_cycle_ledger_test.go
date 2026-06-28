package tsdb

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestFtCycleStatus_TerminalAndValid(t *testing.T) {
	terminal := []FtCycleStatus{FtCyclePromoted, FtCycleFailed, FtCycleRolledBack}
	open := []FtCycleStatus{FtCycleTriggered, FtCycleCurating, FtCycleTraining, FtCycleGating, FtCyclePromotePending}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	for _, s := range open {
		if s.IsTerminal() {
			t.Errorf("%s should NOT be terminal", s)
		}
	}
	for _, s := range append(append([]FtCycleStatus{}, terminal...), open...) {
		if !IsValidCycleStatus(s) {
			t.Errorf("%s should be valid", s)
		}
	}
	for _, s := range []FtCycleStatus{"", "bogus", "TRIGGERED", "done"} {
		if IsValidCycleStatus(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

// capturePool implements jobEventPool, recording the args of the last Exec.
type capturePool struct {
	lastArgs []any
	calls    int
}

func (c *capturePool) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	c.calls++
	c.lastArgs = args
	return pgconn.CommandTag{}, nil
}

func TestRecordCycleEvent_Validation(t *testing.T) {
	ctx := context.Background()

	// nil pool → no-op, no error.
	if err := RecordCycleEvent(ctx, nil, FtCycleEvent{CycleID: "c1", Status: FtCycleTriggered}); err != nil {
		t.Errorf("nil pool should no-op, got %v", err)
	}

	p := &capturePool{}
	// invalid status → error, no insert.
	if err := RecordCycleEvent(ctx, p, FtCycleEvent{CycleID: "c1", Status: "bogus"}); err == nil {
		t.Error("invalid status should error")
	}
	// empty cycle_id → error.
	if err := RecordCycleEvent(ctx, p, FtCycleEvent{Status: FtCycleTriggered}); err == nil {
		t.Error("empty cycle_id should error")
	}
	if p.calls != 0 {
		t.Errorf("no insert should occur on validation failure, got %d calls", p.calls)
	}

	// happy path: model_version defaults to "pending" when unset.
	if err := RecordCycleEvent(ctx, p, FtCycleEvent{CycleID: "c1", Status: FtCycleTriggered}); err != nil {
		t.Fatalf("valid event errored: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("expected 1 insert, got %d", p.calls)
	}
	// args order: time, cycle_id, model_version, status, ...
	if p.lastArgs[1] != "c1" {
		t.Errorf("cycle_id arg = %v, want c1", p.lastArgs[1])
	}
	if p.lastArgs[2] != "pending" {
		t.Errorf("model_version default = %v, want pending", p.lastArgs[2])
	}
	if p.lastArgs[3] != string(FtCycleTriggered) {
		t.Errorf("status arg = %v, want triggered", p.lastArgs[3])
	}
}
