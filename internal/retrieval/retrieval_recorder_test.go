package retrieval

import (
	"testing"
	"time"
)

func TestRetrievalEvent_Fields(t *testing.T) {
	ev := RetrievalEvent{
		Time:          time.Now(),
		EventID:       "evt-retrieval-001",
		SpaceID:       "test-space",
		CallSite:      "retrieve",
		QueryText:     "find authentication handlers",
		QueryHash:     QueryHash("find authentication handlers"),
		RecallNodeIDs: []string{"n1", "n2", "n3"},
		RecallScores:  []float64{0.95, 0.88, 0.72},
		RecallK:       10,
		BM25NodeIDs:   []string{"n2", "n4"},
		BM25Scores:    []float64{4.2, 3.1},
		RerankNodeIDs: []string{"n1", "n2", "n3", "n4"},
		RerankScores:  []float64{0.92, 0.85, 0.60, 0.55},
		RerankModel:   "qwen3-reranker",
		ResultNodeIDs: []string{"n1", "n2"},
		ResultScores:  []float64{0.92, 0.85},
		ResultCount:   2,
		GuidanceID:    "guid-abc123",
	}

	if ev.EventID != "evt-retrieval-001" {
		t.Errorf("EventID = %q, want %q", ev.EventID, "evt-retrieval-001")
	}
	if ev.CallSite != "retrieve" {
		t.Errorf("CallSite = %q, want %q", ev.CallSite, "retrieve")
	}
	if ev.RecallK != 10 {
		t.Errorf("RecallK = %d, want %d", ev.RecallK, 10)
	}
	if len(ev.RecallNodeIDs) != 3 {
		t.Errorf("RecallNodeIDs len = %d, want 3", len(ev.RecallNodeIDs))
	}
	if len(ev.RerankScores) != 4 {
		t.Errorf("RerankScores len = %d, want 4", len(ev.RerankScores))
	}
	if ev.ResultCount != 2 {
		t.Errorf("ResultCount = %d, want 2", ev.ResultCount)
	}
	if ev.GuidanceID != "guid-abc123" {
		t.Errorf("GuidanceID = %q, want %q", ev.GuidanceID, "guid-abc123")
	}
}

func TestQueryHash_Deterministic(t *testing.T) {
	h1 := QueryHash("find all functions")
	h2 := QueryHash("find all functions")
	if h1 != h2 {
		t.Errorf("QueryHash not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("QueryHash length = %d, want 64 (SHA-256 hex)", len(h1))
	}
}

func TestQueryHash_Different(t *testing.T) {
	h1 := QueryHash("query one")
	h2 := QueryHash("query two")
	if h1 == h2 {
		t.Errorf("QueryHash for different inputs should differ")
	}
}

func TestQueryHash_Empty(t *testing.T) {
	h := QueryHash("")
	if h == "" {
		t.Error("QueryHash of empty string should not be empty")
	}
	if len(h) != 64 {
		t.Errorf("QueryHash length = %d, want 64", len(h))
	}
}

func TestRetrievalEvent_ZeroValue(t *testing.T) {
	ev := RetrievalEvent{}
	if ev.EventID != "" {
		t.Errorf("Zero EventID = %q, want empty", ev.EventID)
	}
	if ev.RecallNodeIDs != nil {
		t.Errorf("Zero RecallNodeIDs = %v, want nil", ev.RecallNodeIDs)
	}
	if ev.DownstreamQuality != 0 {
		t.Errorf("Zero DownstreamQuality = %f, want 0", ev.DownstreamQuality)
	}
}

func TestRetrievalEvent_HardNegativeSignal(t *testing.T) {
	// Verify the struct can capture hard-negative mining signal:
	// high vector sim + low rerank score
	ev := RetrievalEvent{
		RecallNodeIDs: []string{"n1", "n2"},
		RecallScores:  []float64{0.95, 0.92}, // high vector similarity
		RerankNodeIDs: []string{"n1", "n2"},
		RerankScores:  []float64{0.20, 0.15}, // low rerank score = hard negative
	}

	// n1: recall=0.95, rerank=0.20 => hard negative candidate
	if ev.RecallScores[0]-ev.RerankScores[0] < 0.5 {
		t.Error("Expected large gap between recall and rerank for hard negative")
	}
}
