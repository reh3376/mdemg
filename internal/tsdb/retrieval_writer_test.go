package tsdb

import (
	"context"
	"fmt"
	"strings"
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

// --- Training Data Capture Verification Tests ---

func TestRetrievalWriter_ColumnValuePositions(t *testing.T) {
	pool := &mockPool{}
	w := newTestRetrievalWriter(pool)

	now := time.Now().Truncate(time.Millisecond)
	row := RetrievalEventRow{
		Time:              now,
		EventID:           "evt-ret-pos",
		SpaceID:           "test-space",
		CallSite:          "consult",
		QueryText:         "how does backpressure work?",
		QueryHash:         "qhash123",
		RecallNodeIDs:     []string{"n1", "n2", "n3"},
		RecallScores:      []float64{0.95, 0.80, 0.72},
		RecallK:           10,
		BM25NodeIDs:       []string{"b1"},
		BM25Scores:        []float64{0.60},
		RerankNodeIDs:     []string{"n1", "n3"},
		RerankScores:      []float64{0.98, 0.75},
		RerankModel:       "bge-reranker-v2",
		ResultNodeIDs:     []string{"n1"},
		ResultScores:      []float64{0.98},
		ResultCount:       1,
		GuidanceID:        "guid-456",
		DownstreamQuality: 0.92,
		RecallLatencyMs:   15,
		RerankLatencyMs:   28,
		TotalLatencyMs:    50,
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
		{1, "event_id", "evt-ret-pos"},
		{2, "space_id", "test-space"},
		{3, "call_site", "consult"},
		{4, "query_text", "how does backpressure work?"},
		{5, "query_hash", "qhash123"},
		// [6-7] slices, checked separately
		{8, "recall_k", 10},
		// [9-10] BM25 slices, checked separately
		// [11-12] rerank slices, checked separately
		{13, "rerank_model", "bge-reranker-v2"},
		// [14-15] result slices, checked separately
		{16, "result_count", 1},
		{17, "guidance_id", "guid-456"},
		{18, "downstream_quality", 0.92},
		{19, "recall_latency_ms", 15},
		{20, "rerank_latency_ms", 28},
		{21, "total_latency_ms", 50},
		{22, "instance_id", ""},
		{22, "instance_id", ""},
	}

	for _, c := range checks {
		if fmt.Sprintf("%v", v[c.pos]) != fmt.Sprintf("%v", c.want) {
			t.Errorf("position [%d] %s: got %v (%T), want %v (%T)",
				c.pos, c.name, v[c.pos], v[c.pos], c.want, c.want)
		}
	}

	// Slice fields
	sliceChecks := []struct {
		pos  int
		name string
	}{
		{6, "recall_node_ids"},
		{9, "bm25_node_ids"},
		{11, "rerank_node_ids"},
		{14, "result_node_ids"},
	}
	for _, sc := range sliceChecks {
		if _, ok := v[sc.pos].([]string); !ok {
			t.Errorf("position [%d] %s: got type %T, want []string", sc.pos, sc.name, v[sc.pos])
		}
	}

	scoreChecks := []struct {
		pos  int
		name string
	}{
		{7, "recall_scores"},
		{10, "bm25_scores"},
		{12, "rerank_scores"},
		{15, "result_scores"},
	}
	for _, sc := range scoreChecks {
		if _, ok := v[sc.pos].([]float64); !ok {
			t.Errorf("position [%d] %s: got type %T, want []float64", sc.pos, sc.name, v[sc.pos])
		}
	}
}

func TestRetrievalWriter_IntakeScrub(t *testing.T) {
	// EXPORT-SCRUB-INTAKE-001 (2026-08-11): supersedes the prior
	// TestRetrievalWriter_NoPrivacyScrub. The retrieval writer NOW scrubs
	// at intake time (skip list matches retrievalEventsSpec.textFields
	// = {"abs_path"}). Rationale: queries reference file paths BY DESIGN
	// (retrieval-by-file-path is a legit shape), but api keys, env
	// secrets, emails, neo4j creds landing in query_text are always
	// leaks — never intended, always safe to redact. Matches the
	// llm_interactions.UserPrompt intake-scrub policy.
	pool := &mockPool{}
	w := newTestRetrievalWriter(pool)

	apiKey := "sk-HvPliFZCy8ohpHoZZ1wUy0EY5ltXXXXX"
	absPath := "/Users/tester/workspace/pkg/foo.go"
	w.Record(RetrievalEventRow{
		Time:      time.Now(),
		SpaceID:   "test",
		CallSite:  "test",
		QueryText: fmt.Sprintf("find %s referenced from %s", apiKey, absPath),
		QueryHash: "hash",
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	v := pool.calls[0].Values[0]
	qt, _ := v[4].(string)

	// api_key MUST be redacted at intake (post-EXPORT-SCRUB-INTAKE-001).
	if strings.Contains(qt, apiKey) {
		t.Errorf("query_text still contains raw api key — intake scrub failed: %q", qt)
	}
	if !strings.Contains(qt, "[REDACTED_KEY]") {
		t.Errorf("api key not properly redacted: %q", qt)
	}
	// abs_path MUST be preserved (matches export spec's abs_path skip —
	// queries reference file paths by design).
	if !strings.Contains(qt, absPath) {
		t.Errorf("abs_path was scrubbed — should be preserved per retrievalEventsSpec.textFields skip list: %q", qt)
	}
}

func TestRetrievalWriter_SliceFieldsPreserved(t *testing.T) {
	pool := &mockPool{}
	w := newTestRetrievalWriter(pool)

	w.Record(RetrievalEventRow{
		Time:          time.Now(),
		SpaceID:       "test",
		CallSite:      "retrieve",
		QueryText:     "test",
		QueryHash:     "hash",
		RecallNodeIDs: []string{"n1", "n2", "n3"},
		RecallScores:  []float64{0.95, 0.80, 0.72},
	})

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	v := pool.calls[0].Values[0]

	nodeIDs, ok := v[6].([]string)
	if !ok {
		t.Fatalf("position [6] recall_node_ids: got type %T, want []string", v[6])
	}
	if len(nodeIDs) != 3 || nodeIDs[0] != "n1" || nodeIDs[1] != "n2" || nodeIDs[2] != "n3" {
		t.Errorf("position [6] recall_node_ids: got %v, want [n1 n2 n3]", nodeIDs)
	}

	scores, ok := v[7].([]float64)
	if !ok {
		t.Fatalf("position [7] recall_scores: got type %T, want []float64", v[7])
	}
	if len(scores) != 3 || scores[0] != 0.95 || scores[1] != 0.80 || scores[2] != 0.72 {
		t.Errorf("position [7] recall_scores: got %v, want [0.95 0.8 0.72]", scores)
	}
}
