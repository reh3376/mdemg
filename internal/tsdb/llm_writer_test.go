package tsdb

import (
	"context"
	"fmt"
	"strings"
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
	Values  [][]any
}

func (p *mockPool) CopyFrom(_ context.Context, tableName pgx.Identifier, columnNames []string,
	rowSrc pgx.CopyFromSource) (int64, error) {
	count := 0
	var values [][]any
	for rowSrc.Next() {
		vals, _ := rowSrc.Values()
		values = append(values, vals)
		count++
	}
	p.mu.Lock()
	p.calls = append(p.calls, mockCopyFromCall{
		Table:   tableName,
		Columns: columnNames,
		Rows:    count,
		Values:  values,
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
	if len(call.Columns) != 27 {
		t.Errorf("columns: got %d, want 27", len(call.Columns))
	}
	// Verify RAFT columns are present at the end
	lastFive := call.Columns[len(call.Columns)-5:]
	expected := []string{"retrieval_node_ids", "retrieval_scores", "oracle_node_id", "system_prompt_hash", "instance_id"}
	for i, col := range lastFive {
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
	// Verify 27 columns (22 original + 4 RAFT + 1 instance_id)
	if len(pool.calls[0].Columns) != 27 {
		t.Errorf("columns: got %d, want 27", len(pool.calls[0].Columns))
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

// --- Training Data Capture Verification Tests ---

func TestLLMWriter_ColumnValuePositions(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	now := time.Now().Truncate(time.Millisecond)
	quality := 0.85
	rec := llmclient.InteractionRecord{
		Time:             now,
		TraceID:          "trace-pos-test",
		TaskName:         "ape.reflect",
		SpaceID:          "test-space",
		SessionID:        "sess-123",
		SystemPrompt:     "You are a helpful assistant",
		UserPrompt:       "Explain backpressure",
		Response:         "Backpressure is...",
		ThinkContent:     "reasoning about response",
		ThinkMode:        true,
		LatencyMs:        142,
		TokensIn:         50,
		TokensOut:        80,
		ModelName:        "gpt-4o",
		Provider:         "openai",
		Error:            "",
		Quality:          &quality,
		QualitySource:    "feedback_outcome",
		GuidanceID:       "guid-abc",
		SourcePath:       "CLAUDE.md",
		SystemPromptHash: "deadbeef1234",
		RetrievalCtx: &llmclient.RetrievalContext{
			NodeIDs:  []string{"n1", "n2"},
			Scores:   []float64{0.95, 0.80},
			OracleID: "n1",
		},
	}
	w.Record(context.Background(), rec)

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if len(pool.calls) != 1 {
		t.Fatalf("CopyFrom calls: got %d, want 1", len(pool.calls))
	}
	if len(pool.calls[0].Values) != 1 {
		t.Fatalf("Values rows: got %d, want 1", len(pool.calls[0].Values))
	}
	v := pool.calls[0].Values[0]
	if len(v) != 27 {
		t.Fatalf("column count: got %d, want 27", len(v))
	}

	// Positional assertions — column index : expected value
	checks := []struct {
		pos  int
		name string
		want any
	}{
		{0, "time", now},
		// [1] TraceID: auto-generated by Record() if empty; we set it explicitly
		{1, "trace_id", "trace-pos-test"},
		{2, "task_name", "ape.reflect"},
		{3, "space_id", "test-space"},
		{4, "session_id", "sess-123"},
		// [5-7] scrubbed prompts/response — checked as strings
		{8, "think_content", "reasoning about response"},
		{9, "think_mode", true},
		{10, "latency_ms", 142},
		{11, "tokens_in", 50},
		{12, "tokens_out", 80},
		{13, "model_name", "gpt-4o"},
		{14, "provider", "openai"},
		{15, "error", ""},
		// [16] Quality is *float64, checked separately
		{17, "quality_source", "feedback_outcome"},
		{18, "used_for_train", false},
		{19, "dataset_ver", ""},
		{20, "guidance_id", "guid-abc"},
		{21, "source_path", "CLAUDE.md"},
		// [22-24] retrieval arrays, checked separately
		{25, "system_prompt_hash", "deadbeef1234"},
		{26, "instance_id", ""},
		{26, "instance_id", ""},
	}

	for _, c := range checks {
		if fmt.Sprintf("%v", v[c.pos]) != fmt.Sprintf("%v", c.want) {
			t.Errorf("position [%d] %s: got %v (%T), want %v (%T)",
				c.pos, c.name, v[c.pos], v[c.pos], c.want, c.want)
		}
	}

	// Quality pointer
	qPtr, ok := v[16].(*float64)
	if !ok {
		t.Errorf("position [16] quality: got type %T, want *float64", v[16])
	} else if qPtr == nil || *qPtr != 0.85 {
		t.Errorf("position [16] quality: got %v, want 0.85", v[16])
	}

	// Retrieval arrays
	nodeIDs, ok := v[22].([]string)
	if !ok {
		t.Errorf("position [22] retrieval_node_ids: got type %T, want []string", v[22])
	} else if len(nodeIDs) != 2 || nodeIDs[0] != "n1" || nodeIDs[1] != "n2" {
		t.Errorf("position [22] retrieval_node_ids: got %v, want [n1 n2]", nodeIDs)
	}

	scores, ok := v[23].([]float64)
	if !ok {
		t.Errorf("position [23] retrieval_scores: got type %T, want []float64", v[23])
	} else if len(scores) != 2 || scores[0] != 0.95 || scores[1] != 0.80 {
		t.Errorf("position [23] retrieval_scores: got %v, want [0.95 0.8]", scores)
	}

	oracleID, ok := v[24].(string)
	if !ok {
		t.Errorf("position [24] oracle_node_id: got type %T, want string", v[24])
	} else if oracleID != "n1" {
		t.Errorf("position [24] oracle_node_id: got %q, want %q", oracleID, "n1")
	}
}

func TestLLMWriter_NilRetrievalCtxPositions(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	w.Record(context.Background(), llmclient.InteractionRecord{
		Time:     time.Now(),
		TaskName: "ape.reflect",
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	v := pool.calls[0].Values[0]

	// Positions 22, 23 are nil Go slices ([]string(nil), []float64(nil)).
	// pgx.CopyFromRows preserves them as-is; they map to SQL NULL arrays.
	nodeIDs, _ := v[22].([]string)
	if len(nodeIDs) != 0 {
		t.Errorf("position [22] retrieval_node_ids: got %v, want empty/nil", nodeIDs)
	}
	scores, _ := v[23].([]float64)
	if len(scores) != 0 {
		t.Errorf("position [23] retrieval_scores: got %v, want empty/nil", scores)
	}
	if v[24] != "" {
		t.Errorf("position [24] oracle_node_id: got %v, want empty string", v[24])
	}
}

func TestLLMWriter_QualityNilPosition(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	w.Record(context.Background(), llmclient.InteractionRecord{
		Time:     time.Now(),
		TaskName: "test.task",
		Quality:  nil, // explicitly nil — not yet annotated
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	v := pool.calls[0].Values[0]

	// Position 16 should be (*float64)(nil), not 0.0
	if v[16] != (*float64)(nil) {
		t.Errorf("position [16] quality: got %v (%T), want (*float64)(nil)", v[16], v[16])
	}
}

func TestLLMWriter_PrivacyScrubInValues(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	w.Record(context.Background(), llmclient.InteractionRecord{
		Time:         time.Now(),
		TaskName:     "test.scrub",
		SystemPrompt: "key: sk-HvPliFZCy8ohpHoZZ1wUy0EY5ltXXXXX",
		UserPrompt:   "Check /Users/reh3376/mdemg/internal/api/server.go",
		Response:     "Found user@example.com in config",
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	v := pool.calls[0].Values[0]

	// Position [5] SystemPrompt: API key should be scrubbed
	sysPrompt, _ := v[5].(string)
	if strings.Contains(sysPrompt, "sk-HvPli") {
		t.Error("position [5] system_prompt: API key was not scrubbed")
	}
	if !strings.Contains(sysPrompt, "[REDACTED_KEY]") {
		t.Errorf("position [5] system_prompt: expected [REDACTED_KEY], got %q", sysPrompt)
	}

	// Position [6] UserPrompt: absolute path should be scrubbed
	userPrompt, _ := v[6].(string)
	if strings.Contains(userPrompt, "/Users/reh3376") {
		t.Error("position [6] user_prompt: absolute path was not scrubbed")
	}
	if !strings.Contains(userPrompt, "[PATH]") {
		t.Errorf("position [6] user_prompt: expected [PATH], got %q", userPrompt)
	}

	// Position [7] Response: email should be scrubbed
	response, _ := v[7].(string)
	if strings.Contains(response, "user@example.com") {
		t.Error("position [7] response: email was not scrubbed")
	}
	if !strings.Contains(response, "[EMAIL]") {
		t.Errorf("position [7] response: expected [EMAIL], got %q", response)
	}
}

func TestLLMWriter_TrainingColumns(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	w.Record(context.Background(), llmclient.InteractionRecord{
		Time:     time.Now(),
		TaskName: "test.training",
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	v := pool.calls[0].Values[0]

	// Position [18] used_for_train: always false
	if v[18] != false {
		t.Errorf("position [18] used_for_train: got %v, want false", v[18])
	}

	// Position [19] dataset_ver: always empty string
	if v[19] != "" {
		t.Errorf("position [19] dataset_ver: got %v, want empty string", v[19])
	}
}

func TestLLMWriter_MultiRecordBatch(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	taskNames := []string{"ape.reflect", "jiminy.evaluate", "jiminy.synthesize", "consulting.classify", "ape.score"}
	for _, tn := range taskNames {
		w.Record(context.Background(), llmclient.InteractionRecord{
			Time:     time.Now(),
			TaskName: tn,
		})
	}

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if len(pool.calls[0].Values) != 5 {
		t.Fatalf("batch rows: got %d, want 5", len(pool.calls[0].Values))
	}

	for i, tn := range taskNames {
		got, _ := pool.calls[0].Values[i][2].(string)
		if got != tn {
			t.Errorf("Values[%d][2] TaskName: got %q, want %q", i, got, tn)
		}
	}
}

func TestLLMWriter_EmptyTaskNameRegression(t *testing.T) {
	pool := &mockPool{}
	w := newTestLLMWriter(pool)

	// Simulates a consumer that forgot WithContext() — TaskName is ""
	w.Record(context.Background(), llmclient.InteractionRecord{
		Time:     time.Now(),
		TaskName: "",
		Response: "some response",
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	v := pool.calls[0].Values[0]

	// Empty TaskName is accepted — documents silent failure mode.
	// The curation pipeline cannot route records with empty TaskName.
	got, _ := v[2].(string)
	if got != "" {
		t.Errorf("position [2] task_name: got %q, want empty string", got)
	}
}
