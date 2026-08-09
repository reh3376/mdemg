package review

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// AUTOGRADE-SCHEDULE-001 — pin tests for the scheduled autograde loop.

// TestAutogradeScheduler_DisabledCompletes — SUPERVISOR-002 contract: nil
// return = intentional completion when disabled. Supervisor MUST NOT restart.
func TestAutogradeScheduler_DisabledCompletes(t *testing.T) {
	s := NewAutogradeScheduler(AutogradeScheduleConfig{Enabled: false}, nil)
	if err := s.Run(context.Background()); err != nil {
		t.Errorf("disabled Run must return nil (SUPERVISOR-002 contract), got %v", err)
	}
}

// TestAutogradeScheduler_NoDatasetsNoOp — enabled but no datasets configured.
// Same "nil return" contract: don't restart on a config that says nothing to do.
func TestAutogradeScheduler_NoDatasetsNoOp(t *testing.T) {
	s := NewAutogradeScheduler(AutogradeScheduleConfig{Enabled: true, MdemgBin: "/bin/true"}, nil)
	if err := s.Run(context.Background()); err != nil {
		t.Errorf("no datasets must return nil, got %v", err)
	}
}

// TestAutogradeScheduler_NoBinaryNoOp — enabled + datasets set but binary
// path can't be resolved (real case: startup on Docker where the CLI binary
// isn't on PATH). Return nil, don't restart.
func TestAutogradeScheduler_NoBinaryNoOp(t *testing.T) {
	s := NewAutogradeScheduler(AutogradeScheduleConfig{
		Enabled:  true,
		Datasets: []string{"contradicted_drafts"},
		MdemgBin: "",
	}, nil)
	if err := s.Run(context.Background()); err != nil {
		t.Errorf("no binary must return nil, got %v", err)
	}
}

// TestAutogradeScheduler_RunOne_BuildsCorrectArgs — the CLI invocation must
// match what the operator would type. Regression pin: if `postAutoGrade` or
// the CLI flag names change, this test catches the drift.
func TestAutogradeScheduler_RunOne_BuildsCorrectArgs(t *testing.T) {
	var capturedName string
	var capturedArgs []string
	s := NewAutogradeScheduler(AutogradeScheduleConfig{
		Enabled:       true,
		Datasets:      []string{"contradicted_drafts"},
		SpaceID:       "mdemg-dev",
		MinConfidence: 0.85,
		Limit:         100,
		MdemgBin:      "/opt/mdemg/bin/mdemg",
		Endpoint:      "http://127.0.0.1:9999",
	}, nil)
	s.runCmd = func(_ context.Context, name string, args []string) error {
		capturedName = name
		capturedArgs = args
		return nil
	}
	if err := s.runOne(context.Background(), "contradicted_drafts"); err != nil {
		t.Fatal(err)
	}
	if capturedName != "/opt/mdemg/bin/mdemg" {
		t.Errorf("expected mdemg binary path, got %q", capturedName)
	}
	want := []string{"review", "autograde",
		"--dataset", "contradicted_drafts",
		"--space-id", "mdemg-dev",
		"--min-confidence", "0.85",
		"--limit", "100",
		// HITL-CURATION-003: scheduled runs always request oldest-ungraded
		// ordering for starvation-free backfill (see runOne).
		"--sample-strategy", SampleStrategyOldestUngraded,
		"--endpoint", "http://127.0.0.1:9999"}
	if strings.Join(capturedArgs, " ") != strings.Join(want, " ") {
		t.Errorf("args mismatch:\ngot:  %v\nwant: %v", capturedArgs, want)
	}
}

// TestAutogradeScheduler_RunOne_AlwaysOldestUngraded — HITL-CURATION-003
// regression pin: the scheduled loop MUST always pass
// `--sample-strategy=oldest-ungraded` so the autograder doesn't sit on the
// same newest-N rows every 6h and starve low-classifier-confidence tail
// rows forever. Removing the flag would silently reintroduce the
// starvation class the sprint fixed.
func TestAutogradeScheduler_RunOne_AlwaysOldestUngraded(t *testing.T) {
	var capturedArgs []string
	s := NewAutogradeScheduler(AutogradeScheduleConfig{
		Enabled: true, Datasets: []string{"x"}, SpaceID: "y",
		MinConfidence: 0.8, Limit: 10, MdemgBin: "/x",
	}, nil)
	s.runCmd = func(_ context.Context, _ string, args []string) error {
		capturedArgs = args
		return nil
	}
	if err := s.runOne(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	found := false
	for i, a := range capturedArgs {
		if a == "--sample-strategy" && i+1 < len(capturedArgs) && capturedArgs[i+1] == SampleStrategyOldestUngraded {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("scheduled autograde MUST pass --sample-strategy=%s (HITL-CURATION-003); got args: %v",
			SampleStrategyOldestUngraded, capturedArgs)
	}
}

// TestAutogradeScheduler_RunOne_NoForceFlag — the scheduled loop must NEVER
// pass --force. Force is for backfill after sink-logic changes; the scheduled
// loop is for organic accumulation. Passing --force here would cause every
// pending item to be re-graded every 6 hours regardless of whether it already
// has a valid grade — LLM cost + noise. Regression pin.
func TestAutogradeScheduler_RunOne_NoForceFlag(t *testing.T) {
	var capturedArgs []string
	s := NewAutogradeScheduler(AutogradeScheduleConfig{
		Enabled: true, Datasets: []string{"x"}, SpaceID: "y",
		MinConfidence: 0.8, Limit: 10, MdemgBin: "/x",
	}, nil)
	s.runCmd = func(_ context.Context, _ string, args []string) error {
		capturedArgs = args
		return nil
	}
	if err := s.runOne(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	for _, a := range capturedArgs {
		if a == "--force" {
			t.Fatal("scheduled autograde MUST NEVER pass --force — regression from AUTOGRADE-SCHEDULE-001 contract")
		}
	}
}

// TestAutogradeScheduler_RunAllDatasets_IteratesAll — a failure on dataset A
// must NOT skip dataset B (the queue on A might legitimately be broken while
// B is fine). Report is invoked per-dataset with a distinct jobName so
// operators can debug per-dataset failures.
func TestAutogradeScheduler_RunAllDatasets_IteratesAll(t *testing.T) {
	var mu sync.Mutex
	calls := []string{}
	reports := []string{}
	s := NewAutogradeScheduler(AutogradeScheduleConfig{
		Enabled: true, Datasets: []string{"ds_a", "ds_b", "ds_c"},
		SpaceID: "sp", MinConfidence: 0.8, Limit: 5, MdemgBin: "/x",
	}, func(_ context.Context, jobName string, _ bool, _ int64, _ string) {
		mu.Lock()
		defer mu.Unlock()
		reports = append(reports, jobName)
	})
	s.runCmd = func(_ context.Context, _ string, args []string) error {
		mu.Lock()
		defer mu.Unlock()
		// args[3] is the dataset name — check runOne's arg order
		calls = append(calls, args[3])
		if args[3] == "ds_b" {
			return errors.New("simulated dataset B failure")
		}
		return nil
	}
	s.runAllDatasets(context.Background())
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("all 3 datasets must run despite ds_b failure, got %v", calls)
	}
	if len(reports) != 3 {
		t.Fatalf("all 3 must produce jobhealth reports, got %v", reports)
	}
	// Reports use distinct jobNames — NOSILENT-001 contract.
	for i, jn := range reports {
		want := "scheduled-autograde:" + calls[i]
		if jn != want {
			t.Errorf("report[%d] must name the dataset, got %q want %q", i, jn, want)
		}
	}
}

// TestAutogradeScheduler_RunAllDatasets_ContextCancelStopsIteration — if the
// context is cancelled mid-iteration, stop cleanly (don't chew through
// remaining datasets in a failing state).
func TestAutogradeScheduler_RunAllDatasets_ContextCancelStopsIteration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	var calls int
	s := NewAutogradeScheduler(AutogradeScheduleConfig{
		Enabled: true, Datasets: []string{"a", "b", "c"},
		SpaceID: "sp", MinConfidence: 0.8, Limit: 5, MdemgBin: "/x",
	}, nil)
	s.runCmd = func(_ context.Context, _ string, _ []string) error {
		calls++
		return nil
	}
	s.runAllDatasets(ctx)
	if calls != 0 {
		t.Errorf("pre-cancelled context must skip all datasets, got %d calls", calls)
	}
}

// TestAutogradeScheduler_IntervalDefaults — zero/negative interval falls back
// to the safe default (6h), not zero.
func TestAutogradeScheduler_IntervalDefaults(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 6}, {-1, 6}, {1, 1}, {24, 24},
	}
	for _, c := range cases {
		s := NewAutogradeScheduler(AutogradeScheduleConfig{IntervalHours: c.in}, nil)
		if got := s.intervalHours(); got != c.want {
			t.Errorf("intervalHours(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestAutogradeScheduler_RunRespectsCancellation — the timer-driven loop
// must exit promptly when ctx is cancelled.
func TestAutogradeScheduler_RunRespectsCancellation(t *testing.T) {
	s := NewAutogradeScheduler(AutogradeScheduleConfig{
		Enabled:         true,
		Datasets:        []string{"x"},
		MdemgBin:        "/x",
		InitialDelayMin: 1, // 1 minute — but we cancel before it fires
	}, nil)
	s.runCmd = func(_ context.Context, _ string, _ []string) error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("cancelled Run must return nil, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not exit within 3s of cancellation")
	}
}
