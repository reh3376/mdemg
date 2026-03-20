package embeddings

import (
	"context"
	"math"
	"testing"
)

func TestStub_Dimensions(t *testing.T) {
	s := NewStub()
	if s.Dimensions() != stubDimensions {
		t.Errorf("Dimensions() = %d, want %d", s.Dimensions(), stubDimensions)
	}
}

func TestStub_Name(t *testing.T) {
	s := NewStub()
	if s.Name() != "stub" {
		t.Errorf("Name() = %q, want %q", s.Name(), "stub")
	}
}

func TestStub_Embed_Deterministic(t *testing.T) {
	s := NewStub()
	ctx := context.Background()

	v1, err := s.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	v2, err := s.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}

	if len(v1) != stubDimensions {
		t.Fatalf("expected %d dimensions, got %d", stubDimensions, len(v1))
	}

	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("not deterministic: v1[%d]=%f != v2[%d]=%f", i, v1[i], i, v2[i])
		}
	}
}

func TestStub_Embed_DifferentInputs(t *testing.T) {
	s := NewStub()
	ctx := context.Background()

	v1, _ := s.Embed(ctx, "hello")
	v2, _ := s.Embed(ctx, "world")

	same := true
	for i := range v1 {
		if v1[i] != v2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different inputs produced identical vectors")
	}
}

func TestStub_Embed_UnitVector(t *testing.T) {
	s := NewStub()
	ctx := context.Background()

	v, _ := s.Embed(ctx, "test normalization")

	var sumSq float64
	for _, val := range v {
		sumSq += float64(val) * float64(val)
	}
	norm := math.Sqrt(sumSq)
	if math.Abs(norm-1.0) > 0.01 {
		t.Errorf("vector norm = %f, want ~1.0", norm)
	}
}

func TestStub_EmbedBatch(t *testing.T) {
	s := NewStub()
	ctx := context.Background()

	texts := []string{"alpha", "beta", "gamma"}
	results, err := s.EmbedBatch(ctx, texts)
	if err != nil {
		t.Fatalf("EmbedBatch error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if len(r) != stubDimensions {
			t.Errorf("results[%d] has %d dimensions, want %d", i, len(r), stubDimensions)
		}
	}
}

func TestOllama_QwenDimensions(t *testing.T) {
	o, err := NewOllama(Config{OllamaModel: "qwen3-embedding:4b"})
	if err != nil {
		t.Fatalf("NewOllama error: %v", err)
	}
	if o.Dimensions() != 2560 {
		t.Errorf("Dimensions() = %d, want 2560", o.Dimensions())
	}
}

func TestOllama_QwenBaseNameDimensions(t *testing.T) {
	o, err := NewOllama(Config{OllamaModel: "qwen3-embedding"})
	if err != nil {
		t.Fatalf("NewOllama error: %v", err)
	}
	if o.Dimensions() != 2560 {
		t.Errorf("Dimensions() = %d, want 2560", o.Dimensions())
	}
}

func TestOllama_DefaultModelIsQwen(t *testing.T) {
	o, err := NewOllama(Config{})
	if err != nil {
		t.Fatalf("NewOllama error: %v", err)
	}
	if o.model != "qwen3-embedding:8b" {
		t.Errorf("model = %q, want %q", o.model, "qwen3-embedding:8b")
	}
	if o.Dimensions() != 3072 {
		t.Errorf("Dimensions() = %d, want 3072", o.Dimensions())
	}
}

func TestOllama_8bTargetDimensions(t *testing.T) {
	o, err := NewOllama(Config{OllamaModel: "qwen3-embedding:8b"})
	if err != nil {
		t.Fatalf("NewOllama error: %v", err)
	}
	if o.Dimensions() != 3072 {
		t.Errorf("Dimensions() = %d, want 3072 (truncated from 4096)", o.Dimensions())
	}
	if o.targetDimensions != 3072 {
		t.Errorf("targetDimensions = %d, want 3072", o.targetDimensions)
	}
}

func TestOllama_ExplicitTargetDimsOverride(t *testing.T) {
	o, err := NewOllama(Config{OllamaModel: "qwen3-embedding:8b", TargetDimensions: 2048})
	if err != nil {
		t.Fatalf("NewOllama error: %v", err)
	}
	if o.Dimensions() != 2048 {
		t.Errorf("Dimensions() = %d, want 2048", o.Dimensions())
	}
	if o.targetDimensions != 2048 {
		t.Errorf("targetDimensions = %d, want 2048", o.targetDimensions)
	}
}

func TestTruncateAndNormalize_Basic(t *testing.T) {
	input := make([]float32, 4096)
	for i := range input {
		input[i] = float32(i + 1)
	}
	result := truncateAndNormalize(input, 3072)
	if len(result) != 3072 {
		t.Fatalf("len = %d, want 3072", len(result))
	}
	// Verify L2 normalization
	var sumSq float64
	for _, v := range result {
		sumSq += float64(v) * float64(v)
	}
	norm := math.Sqrt(sumSq)
	if math.Abs(norm-1.0) > 0.001 {
		t.Errorf("norm = %f, want ~1.0", norm)
	}
}

func TestTruncateAndNormalize_NoOp(t *testing.T) {
	input := make([]float32, 3072)
	for i := range input {
		input[i] = float32(i)
	}
	result := truncateAndNormalize(input, 3072)
	if len(result) != 3072 {
		t.Fatalf("len = %d, want 3072", len(result))
	}
	// Should return original (no truncation needed)
	if &result[0] != &input[0] {
		t.Error("expected same slice returned when no truncation needed")
	}
}

func TestTruncateAndNormalize_SmallerInput(t *testing.T) {
	input := make([]float32, 1024)
	for i := range input {
		input[i] = 1.0
	}
	result := truncateAndNormalize(input, 3072)
	if len(result) != 1024 {
		t.Fatalf("len = %d, want 1024 (unchanged)", len(result))
	}
}

func TestTruncateAndNormalize_ZeroVector(t *testing.T) {
	input := make([]float32, 4096) // all zeros
	result := truncateAndNormalize(input, 3072)
	if len(result) != 3072 {
		t.Fatalf("len = %d, want 3072", len(result))
	}
	// Zero vector should remain zero (no division by zero)
	for i, v := range result {
		if v != 0 {
			t.Errorf("result[%d] = %f, want 0", i, v)
			break
		}
	}
}

func TestStub_FactoryIntegration(t *testing.T) {
	emb, err := New(Config{Provider: "stub"})
	if err != nil {
		t.Fatalf("New(stub) error: %v", err)
	}
	if emb.Name() != "stub" {
		t.Errorf("Name() = %q, want %q", emb.Name(), "stub")
	}
	if emb.Dimensions() != stubDimensions {
		t.Errorf("Dimensions() = %d, want %d", emb.Dimensions(), stubDimensions)
	}
}
