package consulting

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// GUIDANCE-SYNTH-001 Tier 1: the per-node constraint classifier now runs with
// bounded concurrency. These tests assert it produces the SAME result as the
// serial path (determinism), preserves order, falls back to keyword on error,
// and is race-free (run with -race).

// fakeClassifier is a deterministic stand-in for the LLM ConstraintClassifier.
// It sleeps briefly to make a serial run measurably slower than a parallel one,
// and classifies by a simple rule keyed on nodeID so output is reproducible.
type fakeClassifier struct {
	delay    time.Duration
	calls    atomic.Int64
	errOnIDs map[string]bool // nodeIDs to return an error for (→ keyword fallback)
}

func (f *fakeClassifier) Classify(ctx context.Context, nodeID, text string) (*ConstraintClassification, error) {
	f.calls.Add(1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.errOnIDs[nodeID] {
		return nil, fmt.Errorf("classifier error for %s", nodeID)
	}
	// Deterministic: nodes whose summary mentions "must" → must; else none.
	if containsFold(text, "must") {
		return &ConstraintClassification{Type: "must", Summary: "LLM:" + nodeID}, nil
	}
	return &ConstraintClassification{Type: "none"}, nil
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (indexFold(s, sub) >= 0)
}
func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			cs, cj := s[i+j], sub[j]
			if cs >= 'A' && cs <= 'Z' {
				cs += 32
			}
			if cj >= 'A' && cj <= 'Z' {
				cj += 32
			}
			if cs != cj {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func mkResults(n int) []models.RetrieveResult {
	out := make([]models.RetrieveResult, n)
	for i := range n {
		out[i] = models.RetrieveResult{
			NodeID:  fmt.Sprintf("n%02d", i),
			Name:    fmt.Sprintf("constraint-%02d", i),
			Summary: "you must do thing " + fmt.Sprintf("%d", i),
			Score:   0.6, // above the default constraint floor
		}
	}
	return out
}

func newServiceWithClassifier(c constraintClassifierIface, concurrency int) *Service {
	s := &Service{cfg: config.Config{ConsultingClassifyConcurrency: concurrency}}
	s.constraintClassifier = c
	return s
}

// TestFindApplicableConstraints_ParallelEqualsSerial is the core determinism
// guarantee: concurrency must not change which constraints surface or their order.
func TestFindApplicableConstraints_ParallelEqualsSerial(t *testing.T) {
	results := mkResults(12)

	serial := newServiceWithClassifier(&fakeClassifier{}, 1)
	par := newServiceWithClassifier(&fakeClassifier{}, 4)

	gotSerial := serial.findApplicableConstraints(context.TODO(), "sp", results, nil)
	gotPar := par.findApplicableConstraints(context.TODO(), "sp", results, nil)

	if len(gotSerial) != len(gotPar) {
		t.Fatalf("count mismatch: serial=%d parallel=%d", len(gotSerial), len(gotPar))
	}
	for i := range gotSerial {
		if gotSerial[i].Name != gotPar[i].Name || gotSerial[i].ConstraintType != gotPar[i].ConstraintType {
			t.Errorf("order/content mismatch at %d: serial=%+v parallel=%+v", i, gotSerial[i], gotPar[i])
		}
	}
	if len(gotPar) != 12 {
		t.Errorf("expected 12 constraints, got %d", len(gotPar))
	}
}

// TestFindApplicableConstraints_ParallelIsFaster sanity-checks that concurrency
// actually overlaps the per-node latency (serial ≈ N×delay, parallel ≈ N/cap×delay).
func TestFindApplicableConstraints_ParallelIsFaster(t *testing.T) {
	results := mkResults(8)
	delay := 20 * time.Millisecond

	serial := newServiceWithClassifier(&fakeClassifier{delay: delay}, 1)
	t0 := time.Now()
	serial.findApplicableConstraints(context.TODO(), "sp", results, nil)
	serialDur := time.Since(t0)

	par := newServiceWithClassifier(&fakeClassifier{delay: delay}, 4)
	t1 := time.Now()
	par.findApplicableConstraints(context.TODO(), "sp", results, nil)
	parDur := time.Since(t1)

	// 8 nodes × 20ms serial = ~160ms; parallel(4) = ~40ms. Allow generous slack.
	if parDur >= serialDur {
		t.Errorf("parallel (%v) should be faster than serial (%v)", parDur, serialDur)
	}
}

// TestFindApplicableConstraints_ErrorFallsBackToKeyword verifies a classifier
// error on a node falls back to keyword classification (unchanged behavior).
func TestFindApplicableConstraints_ErrorFallsBackToKeyword(t *testing.T) {
	results := mkResults(4)
	// Node n01 errors; its summary contains "must" so keyword fallback yields "must".
	fc := &fakeClassifier{errOnIDs: map[string]bool{"n01": true}}
	s := newServiceWithClassifier(fc, 4)

	got := s.findApplicableConstraints(context.TODO(), "sp", results, nil)
	// All 4 should still surface (LLM "must" for 3, keyword "must" for the errored one).
	if len(got) != 4 {
		t.Fatalf("expected 4 constraints (keyword fallback for errored node), got %d", len(got))
	}
}

// TestFindApplicableConstraints_ScoreGateStillApplies confirms the RRF-SCALE-001
// score gate is preserved under the new structure.
func TestFindApplicableConstraints_ScoreGateStillApplies(t *testing.T) {
	results := mkResults(3)
	results[1].Score = 0.10 // below default floor (0.45) → excluded
	s := newServiceWithClassifier(&fakeClassifier{}, 4)
	got := s.findApplicableConstraints(context.TODO(), "sp", results, nil)
	if len(got) != 2 {
		t.Fatalf("score gate should drop the sub-floor node: expected 2, got %d", len(got))
	}
}

// TestFindApplicableConstraints_ConcurrencyDefaultFallback verifies the cap
// resolves to the default when unset (zero-value config).
func TestFindApplicableConstraints_ConcurrencyDefaultFallback(t *testing.T) {
	results := mkResults(5)
	s := &Service{cfg: config.Config{}} // ConsultingClassifyConcurrency = 0 → default
	s.constraintClassifier = &fakeClassifier{}
	got := s.findApplicableConstraints(context.TODO(), "sp", results, nil)
	if len(got) != 5 {
		t.Errorf("expected 5 constraints with default concurrency, got %d", len(got))
	}
}
