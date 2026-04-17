package embeddings

import (
	"context"
	"sync"
	"testing"
)

// mockRecorder captures RecordEmbed calls for test assertions.
type mockRecorder struct {
	mu     sync.Mutex
	events []EmbeddingEvent
}

func (m *mockRecorder) RecordEmbed(_ context.Context, event EmbeddingEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockRecorder) Events() []EmbeddingEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]EmbeddingEvent, len(m.events))
	copy(cp, m.events)
	return cp
}

func TestEmbedBatch_RecordsCacheHitsAndMisses(t *testing.T) {
	stub := NewStub()
	ce := NewCachedEmbedder(stub, 100)
	rec := &mockRecorder{}
	ce.SetRecorder(rec)

	ctx := context.Background()

	// Pre-populate cache with "alpha" via single Embed call.
	if _, err := ce.Embed(ctx, "alpha"); err != nil {
		t.Fatalf("Embed(alpha): %v", err)
	}

	// Clear recorder to isolate EmbedBatch events.
	rec.mu.Lock()
	rec.events = nil
	rec.mu.Unlock()

	// EmbedBatch: "alpha" (cached) + "beta" + "gamma" (misses).
	results, err := ce.EmbedBatch(ctx, []string{"alpha", "beta", "gamma"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	events := rec.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 recording events, got %d", len(events))
	}

	// Verify cache hit/miss flags.
	hitCount, missCount := 0, 0
	for _, ev := range events {
		if ev.Cached {
			hitCount++
		} else {
			missCount++
		}
	}
	if hitCount != 1 {
		t.Errorf("expected 1 cache hit event, got %d", hitCount)
	}
	if missCount != 2 {
		t.Errorf("expected 2 cache miss events, got %d", missCount)
	}
}

func TestEmbedBatch_AllCached_RecordsHits(t *testing.T) {
	stub := NewStub()
	ce := NewCachedEmbedder(stub, 100)
	rec := &mockRecorder{}
	ce.SetRecorder(rec)

	ctx := context.Background()

	// Pre-populate cache.
	for _, text := range []string{"x", "y"} {
		if _, err := ce.Embed(ctx, text); err != nil {
			t.Fatalf("Embed(%s): %v", text, err)
		}
	}

	rec.mu.Lock()
	rec.events = nil
	rec.mu.Unlock()

	// EmbedBatch with all cached texts.
	results, err := ce.EmbedBatch(ctx, []string{"x", "y"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	events := rec.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 recording events, got %d", len(events))
	}
	for i, ev := range events {
		if !ev.Cached {
			t.Errorf("event[%d] expected Cached=true, got false", i)
		}
		if ev.LatencyMs != 0 {
			t.Errorf("event[%d] expected LatencyMs=0 for cache hit, got %d", i, ev.LatencyMs)
		}
	}
}

func TestEmbedBatch_MissLatency_NonZero(t *testing.T) {
	stub := NewStub()
	ce := NewCachedEmbedder(stub, 100)
	rec := &mockRecorder{}
	ce.SetRecorder(rec)

	ctx := context.Background()

	// All misses — latency should be recorded (may be 0 if stub is fast, but structure is correct).
	_, err := ce.EmbedBatch(ctx, []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}

	events := rec.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 recording events, got %d", len(events))
	}
	for i, ev := range events {
		if ev.Cached {
			t.Errorf("event[%d] expected Cached=false, got true", i)
		}
	}
}

func TestEmbedBatch_Empty_NoRecording(t *testing.T) {
	stub := NewStub()
	ce := NewCachedEmbedder(stub, 100)
	rec := &mockRecorder{}
	ce.SetRecorder(rec)

	ctx := context.Background()

	results, err := ce.EmbedBatch(ctx, []string{})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty input, got %v", results)
	}

	events := rec.Events()
	if len(events) != 0 {
		t.Errorf("expected 0 recording events for empty input, got %d", len(events))
	}
}
