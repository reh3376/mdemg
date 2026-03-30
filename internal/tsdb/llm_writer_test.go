package tsdb

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"mdemg/internal/llmclient"
)

// mockPool captures CopyFrom calls for verification.
type mockPool struct {
	mu       sync.Mutex
	calls    []mockCopyFromCall
	returnN  int64
	returnErr error
}

type mockCopyFromCall struct {
	Table   pgx.Identifier
	Columns []string
	Rows    int
}

func (p *mockPool) CopyFrom(_ context.Context, tableName pgx.Identifier, columnNames []string,
	rowSrc pgx.CopyFromSource) (int64, error) {
	// Count rows by consuming the source
	count := 0
	for rowSrc.Next() {
		rowSrc.Values()
		count++
	}
	p.mu.Lock()
	p.calls = append(p.calls, mockCopyFromCall{
		Table:   tableName,
		Columns: columnNames,
		Rows:    count,
	})
	p.mu.Unlock()
	return p.returnN, p.returnErr
}

func newTestLLMWriter(pool *mockPool) *LLMInteractionWriter {
	return &LLMInteractionWriter{
		pool:   pool,
		buffer: make([]llmclient.InteractionRecord, 0, 32),
		done:   make(chan struct{}),
	}
}

func TestLLMInteractionWriter_Record(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	rec := llmclient.InteractionRecord{
		Time:     time.Now(),
		TaskName: "ape.reflect",
		SpaceID:  "test-space",
		Response: "hello",
	}
	w.Record(context.Background(), rec)

	w.mu.Lock()
	defer w.mu.Unlock()
	if got := len(w.buffer); got != 1 {
		t.Fatalf("buffer length: got %d, want 1", got)
	}
	if w.buffer[0].TraceID == "" {
		t.Error("expected TraceID to be auto-generated, got empty")
	}
	if w.buffer[0].TaskName != "ape.reflect" {
		t.Errorf("TaskName: got %q, want %q", w.buffer[0].TaskName, "ape.reflect")
	}
}

func TestLLMInteractionWriter_Flush(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	// Add 3 records
	for i := 0; i < 3; i++ {
		w.Record(context.Background(), llmclient.InteractionRecord{
			Time:     time.Now(),
			TaskName: "test.task",
			SpaceID:  "test-space",
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
	if call.Table[0] != "llm_interactions" {
		t.Errorf("table: got %q, want %q", call.Table[0], "llm_interactions")
	}
	if len(call.Columns) != 26 {
		t.Errorf("columns: got %d, want 26", len(call.Columns))
	}
	// Verify RAFT columns are present at the end
	lastFour := call.Columns[len(call.Columns)-4:]
	expected := []string{"retrieval_node_ids", "retrieval_scores", "oracle_node_id", "system_prompt_hash"}
	for i, col := range lastFour {
		if col != expected[i] {
			t.Errorf("column %d from end: got %q, want %q", i, col, expected[i])
		}
	}

	// Buffer should be empty after flush
	w.mu.Lock()
	defer w.mu.Unlock()
	if got := len(w.buffer); got != 0 {
		t.Errorf("buffer after flush: got %d, want 0", got)
	}
}

func TestLLMInteractionWriter_FlushEmpty(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush empty: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if got := len(pool.calls); got != 0 {
		t.Errorf("CopyFrom calls on empty buffer: got %d, want 0", got)
	}
}

func TestLLMInteractionWriter_Close(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)
	w.flushTick = time.NewTicker(time.Hour) // won't fire during test

	// Add a record then close — should flush
	w.Record(context.Background(), llmclient.InteractionRecord{
		Time:     time.Now(),
		TaskName: "close.test",
	})

	w.Close()

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if got := len(pool.calls); got != 1 {
		t.Fatalf("CopyFrom calls after Close: got %d, want 1", got)
	}
	if pool.calls[0].Rows != 1 {
		t.Errorf("rows on close flush: got %d, want 1", pool.calls[0].Rows)
	}
}

func TestLLMInteractionWriter_EnrichedColumns(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	quality := 0.85
	w.Record(context.Background(), llmclient.InteractionRecord{
		Time:          time.Now(),
		TaskName:      "jiminy.synthesize",
		GuidanceID:    "test-guidance-123",
		SourcePath:    "CLAUDE.md",
		ThinkContent:  "reasoning about constraints",
		ThinkMode:     true,
		Quality:       &quality,
		QualitySource: "feedback_outcome",
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.calls) != 1 {
		t.Fatalf("CopyFrom calls: got %d, want 1", len(pool.calls))
	}
	if pool.calls[0].Rows != 1 {
		t.Errorf("rows: got %d, want 1", pool.calls[0].Rows)
	}
}

func TestLLMInteractionWriter_RAFTColumns(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	w.Record(context.Background(), llmclient.InteractionRecord{
		Time:             time.Now(),
		TaskName:         "consulting.classify",
		SystemPromptHash: "abcd1234",
		RetrievalCtx: &llmclient.RetrievalContext{
			NodeIDs:  []string{"n1", "n2"},
			Scores:   []float64{0.95, 0.80},
			OracleID: "n1",
		},
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.calls) != 1 {
		t.Fatalf("CopyFrom calls: got %d, want 1", len(pool.calls))
	}
	if pool.calls[0].Rows != 1 {
		t.Errorf("rows: got %d, want 1", pool.calls[0].Rows)
	}
	// Verify 26 columns (22 original + 4 RAFT)
	if len(pool.calls[0].Columns) != 26 {
		t.Errorf("columns: got %d, want 26", len(pool.calls[0].Columns))
	}
}

func TestLLMInteractionWriter_RAFTColumnsNilContext(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	// No RetrievalCtx — should not panic, should write nil arrays
	w.Record(context.Background(), llmclient.InteractionRecord{
		Time:     time.Now(),
		TaskName: "ape.reflect",
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.calls) != 1 {
		t.Fatalf("CopyFrom calls: got %d, want 1", len(pool.calls))
	}
	if pool.calls[0].Rows != 1 {
		t.Errorf("rows: got %d, want 1", pool.calls[0].Rows)
	}
}

func TestLLMInteractionWriter_TraceIDPreserved(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	customTraceID := "custom-trace-abc"
	w.Record(context.Background(), llmclient.InteractionRecord{
		Time:    time.Now(),
		TraceID: customTraceID,
	})

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buffer[0].TraceID != customTraceID {
		t.Errorf("TraceID: got %q, want %q", w.buffer[0].TraceID, customTraceID)
	}
}
