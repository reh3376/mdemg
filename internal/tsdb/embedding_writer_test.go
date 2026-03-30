package tsdb

import (
	"context"
	"fmt"
	"strings"
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

// --- Training Data Capture Verification Tests ---

func TestEmbeddingWriter_ColumnValuePositions(t *testing.T) {
	pool := &mockPool{}
	w := newTestEmbeddingWriter(pool)

	now := time.Now().Truncate(time.Millisecond)
	row := EmbeddingEventRow{
		Time:        now,
		EventID:     "evt-pos-test",
		EventType:   "ingest",
		SpaceID:     "test-space",
		TextContent: "function main() {}",
		TextHash:    "sha256abc",
		TextLength:  18,
		ElementKind: "function",
		Language:    "go",
		FilePath:    "internal/api/server.go",
		ChunkStart:  10,
		ChunkEnd:    28,
		PackageName: "api",
		Signature:   "func main()",
		Tags:        []string{"ingest", "code"},
		CallSite:    "ingest-pipeline",
		QueryText:   "",
		ModelName:   "text-embedding-3-large",
		Provider:    "openai",
		Dimensions:  3072,
		LatencyMs:   42,
		Cached:      true,
		NodeID:      "node-xyz",
	}
	w.Record(row)

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if len(pool.calls[0].Values) != 1 {
		t.Fatalf("Values rows: got %d, want 1", len(pool.calls[0].Values))
	}
	v := pool.calls[0].Values[0]
	if len(v) != 23 {
		t.Fatalf("column count: got %d, want 23", len(v))
	}

	checks := []struct {
		pos  int
		name string
		want any
	}{
		{0, "time", now},
		{1, "event_id", "evt-pos-test"},
		{2, "event_type", "ingest"},
		{3, "space_id", "test-space"},
		// [4] TextContent — scrubbed, check separately
		{5, "text_hash", "sha256abc"},
		{6, "text_length", 18},
		{7, "element_kind", "function"},
		{8, "language", "go"},
		{9, "file_path", "internal/api/server.go"},
		{10, "chunk_start", 10},
		{11, "chunk_end", 28},
		{12, "package_name", "api"},
		{13, "signature", "func main()"},
		// [14] Tags — slice, check separately
		{15, "call_site", "ingest-pipeline"},
		{16, "query_text", ""},
		{17, "model_name", "text-embedding-3-large"},
		{18, "provider", "openai"},
		{19, "dimensions", 3072},
		{20, "latency_ms", 42},
		{21, "cached", true},
		{22, "node_id", "node-xyz"},
	}

	for _, c := range checks {
		if fmt.Sprintf("%v", v[c.pos]) != fmt.Sprintf("%v", c.want) {
			t.Errorf("position [%d] %s: got %v (%T), want %v (%T)",
				c.pos, c.name, v[c.pos], v[c.pos], c.want, c.want)
		}
	}

	// Position [4] TextContent — should contain original (no scrub pattern in this input)
	tc, _ := v[4].(string)
	if tc != "function main() {}" {
		t.Errorf("position [4] text_content: got %q, want %q", tc, "function main() {}")
	}

	// Position [14] Tags — slice check
	tags, ok := v[14].([]string)
	if !ok {
		t.Errorf("position [14] tags: got type %T, want []string", v[14])
	} else if len(tags) != 2 || tags[0] != "ingest" || tags[1] != "code" {
		t.Errorf("position [14] tags: got %v, want [ingest code]", tags)
	}
}

func TestEmbeddingWriter_ScrubAsymmetry(t *testing.T) {
	pool := &mockPool{}
	w := newTestEmbeddingWriter(pool)

	apiKey := "sk-HvPliFZCy8ohpHoZZ1wUy0EY5ltXXXXX"
	w.Record(EmbeddingEventRow{
		Time:        time.Now(),
		EventType:   "query",
		SpaceID:     "test",
		TextContent: fmt.Sprintf("key: %s and /Users/reh3376/mdemg/foo.go and user@example.com and PASSWORD=secret and neo4j://admin:pass@host:7687", apiKey),
		TextHash:    "hash",
		TextLength:  200,
		QueryText:   fmt.Sprintf("search for %s in codebase", apiKey),
	})

	w.mu.Lock()
	tc := w.buffer[0].TextContent
	qt := w.buffer[0].QueryText
	w.mu.Unlock()

	// TextContent IS scrubbed (all 5 patterns)
	if strings.Contains(tc, apiKey) {
		t.Error("TextContent: API key was not scrubbed")
	}
	if !strings.Contains(tc, "[REDACTED_KEY]") {
		t.Error("TextContent: expected [REDACTED_KEY]")
	}
	if !strings.Contains(tc, "[PATH]") {
		t.Error("TextContent: expected [PATH] for absolute path")
	}
	if !strings.Contains(tc, "[EMAIL]") {
		t.Error("TextContent: expected [EMAIL]")
	}
	if !strings.Contains(tc, "PASSWORD=[REDACTED]") {
		t.Error("TextContent: expected PASSWORD=[REDACTED]")
	}
	if !strings.Contains(tc, "neo4j://[REDACTED]@") {
		t.Error("TextContent: expected neo4j://[REDACTED]@")
	}

	// QueryText is NOT scrubbed — preserves API key for contrastive training pairs
	if !strings.Contains(qt, apiKey) {
		t.Errorf("QueryText: API key was incorrectly scrubbed, got %q", qt)
	}
}

func TestEmbeddingWriter_TagsPreserved(t *testing.T) {
	pool := &mockPool{}
	w := newTestEmbeddingWriter(pool)

	w.Record(EmbeddingEventRow{
		Time:        time.Now(),
		EventType:   "ingest",
		SpaceID:     "test",
		TextContent: "test content",
		TextHash:    "hash",
		TextLength:  12,
		Tags:        []string{"auto-overflow", "synergy"},
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	v := pool.calls[0].Values[0]
	tags, ok := v[14].([]string)
	if !ok {
		t.Fatalf("position [14] tags: got type %T, want []string", v[14])
	}
	if len(tags) != 2 || tags[0] != "auto-overflow" || tags[1] != "synergy" {
		t.Errorf("position [14] tags: got %v, want [auto-overflow synergy]", tags)
	}
}
