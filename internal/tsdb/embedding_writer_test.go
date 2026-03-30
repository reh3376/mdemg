package tsdb

import (
	"context"
	"testing"
	"time"
)

func newTestEmbeddingWriter(pool *mockPool) *EmbeddingEventWriter {
	return &EmbeddingEventWriter{
		pool:   pool,
		buffer: make([]EmbeddingEventRow, 0, 32),
		done:   make(chan struct{}),
	}
}

func TestEmbeddingEventWriter_Record(t *testing.T) {
	pool := &mockPool{}
	w := newTestEmbeddingWriter(pool)

	w.Record(EmbeddingEventRow{
		EventType:   "ingest",
		SpaceID:     "test-space",
		TextContent: "function main() {}",
		TextHash:    "abc123",
		TextLength:  18,
		CallSite:    "ingest",
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
	if w.buffer[0].CallSite != "ingest" {
		t.Errorf("CallSite: got %q, want %q", w.buffer[0].CallSite, "ingest")
	}
}

func TestEmbeddingEventWriter_Flush(t *testing.T) {
	pool := &mockPool{}
	w := newTestEmbeddingWriter(pool)

	for range 3 {
		w.Record(EmbeddingEventRow{
			Time:        time.Now(),
			EventType:   "query",
			SpaceID:     "test-space",
			TextContent: "search text",
			TextHash:    "hash",
			TextLength:  11,
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
	if call.Table[0] != "embedding_events" {
		t.Errorf("table: got %q, want %q", call.Table[0], "embedding_events")
	}
	if len(call.Columns) != 23 {
		t.Errorf("columns: got %d, want 23", len(call.Columns))
	}

	// Buffer should be empty after flush
	w.mu.Lock()
	defer w.mu.Unlock()
	if got := len(w.buffer); got != 0 {
		t.Errorf("buffer after flush: got %d, want 0", got)
	}
}

func TestEmbeddingEventWriter_FlushEmpty(t *testing.T) {
	pool := &mockPool{}
	w := newTestEmbeddingWriter(pool)

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush empty: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if got := len(pool.calls); got != 0 {
		t.Errorf("CopyFrom calls on empty buffer: got %d, want 0", got)
	}
}

func TestEmbeddingEventWriter_Close(t *testing.T) {
	pool := &mockPool{}
	w := newTestEmbeddingWriter(pool)
	w.flushTick = time.NewTicker(time.Hour)

	w.Record(EmbeddingEventRow{
		Time:        time.Now(),
		EventType:   "ingest",
		SpaceID:     "test",
		TextContent: "close test",
		TextHash:    "hash",
		TextLength:  10,
	})

	w.Close()

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if got := len(pool.calls); got != 1 {
		t.Fatalf("CopyFrom calls after Close: got %d, want 1", got)
	}
}

func TestEmbeddingEventWriter_PrivacyScrub(t *testing.T) {
	pool := &mockPool{}
	w := newTestEmbeddingWriter(pool)

	w.Record(EmbeddingEventRow{
		Time:        time.Now(),
		EventType:   "ingest",
		SpaceID:     "test",
		TextContent: "contains sk-HvPliFZCy8ohpHoZZ1wUy0EY5ltXXXXX key",
		TextHash:    "hash",
		TextLength:  50,
	})

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buffer[0].TextContent == "contains sk-HvPliFZCy8ohpHoZZ1wUy0EY5ltXXXXX key" {
		t.Error("expected TextContent to be scrubbed, but it was not")
	}
}
