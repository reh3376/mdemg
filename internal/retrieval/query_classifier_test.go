package retrieval

import (
	"context"
	"testing"

	"mdemg/internal/config"
)

func TestQueryClassifier_ParseResponse_Valid(t *testing.T) {
	qc := &QueryClassifier{}

	result, err := qc.parseResponse(`{"types": ["code", "architecture"], "temporal": "recent"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(result.Types))
	}
	if result.Types[0] != "code" || result.Types[1] != "architecture" {
		t.Errorf("unexpected types: %v", result.Types)
	}
	if result.Temporal != "recent" {
		t.Errorf("expected temporal 'recent', got %q", result.Temporal)
	}
}

func TestQueryClassifier_ParseResponse_SingleType(t *testing.T) {
	qc := &QueryClassifier{}

	result, err := qc.parseResponse(`{"types": ["symbol_lookup"], "temporal": "none"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Types) != 1 || result.Types[0] != "symbol_lookup" {
		t.Errorf("unexpected types: %v", result.Types)
	}
}

func TestQueryClassifier_ParseResponse_InvalidTypes(t *testing.T) {
	qc := &QueryClassifier{}

	result, err := qc.parseResponse(`{"types": ["invalid_type"], "temporal": "none"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Invalid types should be filtered, fallback to "generic"
	if len(result.Types) != 1 || result.Types[0] != "generic" {
		t.Errorf("expected fallback to ['generic'], got %v", result.Types)
	}
}

func TestQueryClassifier_ParseResponse_InvalidTemporal(t *testing.T) {
	qc := &QueryClassifier{}

	result, err := qc.parseResponse(`{"types": ["code"], "temporal": "future"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Temporal != "none" {
		t.Errorf("expected fallback temporal 'none', got %q", result.Temporal)
	}
}

func TestQueryClassifier_ParseResponse_CodeFence(t *testing.T) {
	qc := &QueryClassifier{}

	result, err := qc.parseResponse("```json\n{\"types\": [\"data_flow\"], \"temporal\": \"historical\"}\n```")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Types[0] != "data_flow" {
		t.Errorf("expected 'data_flow', got %q", result.Types[0])
	}
}

func TestQueryClassifier_ParseResponse_InvalidJSON(t *testing.T) {
	qc := &QueryClassifier{}

	_, err := qc.parseResponse("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestQueryClassifier_Cache(t *testing.T) {
	qc := NewQueryClassifier(QueryClassifierConfig{Enabled: true}, nil)

	classification := QueryClassification{Types: []string{"code"}, Temporal: "none"}
	hash := hashQuery("test query")

	qc.cachePut(hash, classification)
	result := qc.cacheGet(hash)
	if result == nil {
		t.Fatal("expected cached result")
	}
	if result.Types[0] != "code" {
		t.Errorf("expected 'code', got %q", result.Types[0])
	}

	// Miss
	if qc.cacheGet("nonexistent") != nil {
		t.Error("expected nil for cache miss")
	}
}

func TestQueryClassifier_Disabled(t *testing.T) {
	qc := NewQueryClassifier(QueryClassifierConfig{Enabled: false}, nil)

	result, err := qc.Classify(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil when disabled, got %v", result)
	}
}

func TestComputeRetrievalHintsWithLLM_NilClassifier(t *testing.T) {
	cfg := config.Config{
		DefaultHopDepth:  2,
		EdgeTypeStrategy: "all",
	}

	// Should fall back to regex-based
	hints := ComputeRetrievalHintsWithLLM(context.Background(), "test query", cfg, nil)
	if hints.QueryType != "generic" {
		t.Errorf("expected 'generic' query type, got %q", hints.QueryType)
	}
}

func TestHashQuery(t *testing.T) {
	h1 := hashQuery("test query")
	h2 := hashQuery("test query")
	h3 := hashQuery("different query")

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
	if len(h1) != 32 {
		t.Errorf("expected 32 char hex hash, got %d chars", len(h1))
	}
}
