package tsdb

import (
	"context"
	"testing"
	"time"
)

func newTestRetrievalWriter(pool *mockPool) *RetrievalEventWriter {
	return &RetrievalEventWriter{
		pool:   pool,
		buffer: make([]RetrievalEventRow, 0, 32),
		done:   make(chan struct{}),
	}
}

func TestRetrievalEventWriter_Record(t *testing.T) {
	pool := &mockPool{}
	w := newTestRetrievalWriter(pool)

	w.Record(RetrievalEventRow{
		SpaceID:   "test-space",
		CallSite:  "consult",
		QueryText: "how do I configure backpressure?",
		QueryHash: "abc123",
	})

	w.mu.Lock()
	defer w.mu.Unlock()
	if got := len(w.buffer); got != 1 {
		t.Fatalf("buffer length: got %d, want 1", got)
	}
	if w.buffer[0].EventID == "" {
		t.Error("expected EventID to be auto-generated, got empty")
	}
	if w.buffer[0].Time.IsZero() {
		t.Error("expected Time to be auto-set, got zero")
	}
	if w.buffer[0].CallSite != "consult" {
		t.Errorf("CallSite: got %q, want %q", w.buffer[0].CallSite, "consult")
	}
}

func TestRetrievalEventWriter_Flush(t *testing.T) {
	pool := &mockPool{}
	w := newTestRetrievalWriter(pool)

	for range 3 {
		w.Record(RetrievalEventRow{
			Time:          time.Now(),
			SpaceID:       "test-space",
			CallSite:      "retrieve",
			QueryText:     "test query",
			QueryHash:     "hash",
			RecallNodeIDs: []string{"n1", "n2"},
			RecallScores:  []float64{0.9, 0.8},
			RecallK:       10,
			ResultNodeIDs: []string{"n1"},
			ResultScores:  []float64{0.95},
			ResultCount:   1,
			TotalLatencyMs: 42,
		})
	}

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if got := len(pool.calls); got != 1 {
		t.Fatalf("CopyFrom calls: got %d, want 1", got)
	}
	call := pool.calls[0]
	if call.Rows != 3 {
		t.Errorf("rows written: got %d, want 3", call.Rows)
	}
	if call.Table[0] != "retrieval_events" {
		t.Errorf("table: got %q, want %q", call.Table[0], "retrieval_events")
	}
	if len(call.Columns) != 22 {
		t.Errorf("columns: got %d, want 22", len(call.Columns))
	}

	// Buffer should be empty after flush
	w.mu.Lock()
	defer w.mu.Unlock()
	if got := len(w.buffer); got != 0 {
		t.Errorf("buffer after flush: got %d, want 0", got)
	}
}

func TestRetrievalEventWriter_FlushEmpty(t *testing.T) {
	pool := &mockPool{}
	w := newTestRetrievalWriter(pool)

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush empty: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if got := len(pool.calls); got != 0 {
		t.Errorf("CopyFrom calls on empty buffer: got %d, want 0", got)
	}
}

func TestRetrievalEventWriter_Close(t *testing.T) {
	pool := &mockPool{}
	w := newTestRetrievalWriter(pool)
	w.flushTick = time.NewTicker(time.Hour)

	w.Record(RetrievalEventRow{
		Time:      time.Now(),
		SpaceID:   "test",
		CallSite:  "close-test",
		QueryText: "close test query",
		QueryHash: "hash",
	})

	w.Close()

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if got := len(pool.calls); got != 1 {
		t.Fatalf("CopyFrom calls after Close: got %d, want 1", got)
	}
}

func TestRetrievalEventWriter_EventIDPreserved(t *testing.T) {
	pool := &mockPool{}
	w := newTestRetrievalWriter(pool)

	customID := "custom-event-xyz"
	w.Record(RetrievalEventRow{
		EventID:   customID,
		SpaceID:   "test",
		CallSite:  "test",
		QueryText: "preserved id test",
	})

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buffer[0].EventID != customID {
		t.Errorf("EventID: got %q, want %q", w.buffer[0].EventID, customID)
	}
}
