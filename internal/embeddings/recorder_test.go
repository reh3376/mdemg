package embeddings

import (
	"context"
	"testing"
)

func TestEmbeddingEvent_Fields(t *testing.T) {
	ev := EmbeddingEvent{
		EventID:     "test-event-id",
		EventType:   "ingest",
		SpaceID:     "test-space",
		TextContent: "hello world",
		ElementKind: "function",
		Language:    "go",
		FilePath:    "internal/foo/bar.go",
		CallSite:    "ingest",
		ModelName:   "text-embedding-3-large",
		Provider:    "openai",
		Dimensions:  3072,
		LatencyMs:   42,
		Cached:      false,
		NodeID:      "node-123",
	}

	if ev.EventID != "test-event-id" {
		t.Errorf("EventID = %q, want %q", ev.EventID, "test-event-id")
	}
	if ev.CallSite != "ingest" {
		t.Errorf("CallSite = %q, want %q", ev.CallSite, "ingest")
	}
	if ev.Dimensions != 3072 {
		t.Errorf("Dimensions = %d, want %d", ev.Dimensions, 3072)
	}
}

func TestWithEmbeddingMeta_RoundTrip(t *testing.T) {
	ctx := context.Background()

	meta := EmbeddingMeta{
		ElementKind: "struct",
		Language:    "go",
		FilePath:    "internal/models/types.go",
		CallSite:    "ingest",
		PackageName: "models",
		Signature:   "type Foo struct",
		ChunkStart:  10,
		ChunkEnd:    50,
		Tags:        []string{"core", "api"},
		SpaceID:     "test-space",
		QueryText:   "find all structs",
		NodeID:      "node-abc",
	}

	ctx = WithEmbeddingMeta(ctx, meta)
	got := GetEmbeddingMeta(ctx)

	if got == nil {
		t.Fatal("GetEmbeddingMeta returned nil")
	}
	if got.ElementKind != "struct" {
		t.Errorf("ElementKind = %q, want %q", got.ElementKind, "struct")
	}
	if got.Language != "go" {
		t.Errorf("Language = %q, want %q", got.Language, "go")
	}
	if got.FilePath != "internal/models/types.go" {
		t.Errorf("FilePath = %q, want %q", got.FilePath, "internal/models/types.go")
	}
	if got.CallSite != "ingest" {
		t.Errorf("CallSite = %q, want %q", got.CallSite, "ingest")
	}
	if got.PackageName != "models" {
		t.Errorf("PackageName = %q, want %q", got.PackageName, "models")
	}
	if got.ChunkStart != 10 {
		t.Errorf("ChunkStart = %d, want %d", got.ChunkStart, 10)
	}
	if got.ChunkEnd != 50 {
		t.Errorf("ChunkEnd = %d, want %d", got.ChunkEnd, 50)
	}
	if got.SpaceID != "test-space" {
		t.Errorf("SpaceID = %q, want %q", got.SpaceID, "test-space")
	}
	if got.QueryText != "find all structs" {
		t.Errorf("QueryText = %q, want %q", got.QueryText, "find all structs")
	}
	if got.NodeID != "node-abc" {
		t.Errorf("NodeID = %q, want %q", got.NodeID, "node-abc")
	}
	if len(got.Tags) != 2 || got.Tags[0] != "core" || got.Tags[1] != "api" {
		t.Errorf("Tags = %v, want [core api]", got.Tags)
	}
}

func TestGetEmbeddingMeta_Nil(t *testing.T) {
	ctx := context.Background()
	got := GetEmbeddingMeta(ctx)
	if got != nil {
		t.Errorf("GetEmbeddingMeta on empty ctx = %v, want nil", got)
	}
}

func TestWithEmbeddingMeta_Overwrite(t *testing.T) {
	ctx := context.Background()

	ctx = WithEmbeddingMeta(ctx, EmbeddingMeta{CallSite: "ingest", SpaceID: "space-1"})
	ctx = WithEmbeddingMeta(ctx, EmbeddingMeta{CallSite: "retrieve", SpaceID: "space-2"})

	got := GetEmbeddingMeta(ctx)
	if got == nil {
		t.Fatal("GetEmbeddingMeta returned nil")
	}
	if got.CallSite != "retrieve" {
		t.Errorf("CallSite = %q, want %q (latest value)", got.CallSite, "retrieve")
	}
	if got.SpaceID != "space-2" {
		t.Errorf("SpaceID = %q, want %q (latest value)", got.SpaceID, "space-2")
	}
}

func TestTextHash_Deterministic(t *testing.T) {
	h1 := TextHash("hello world")
	h2 := TextHash("hello world")
	if h1 != h2 {
		t.Errorf("TextHash not deterministic: %q != %q", h1, h2)
	}

	if len(h1) != 64 {
		t.Errorf("TextHash length = %d, want 64 (SHA-256 hex)", len(h1))
	}
}

func TestTextHash_Different(t *testing.T) {
	h1 := TextHash("hello")
	h2 := TextHash("world")
	if h1 == h2 {
		t.Errorf("TextHash for different inputs should differ")
	}
}

func TestTextHash_Empty(t *testing.T) {
	h := TextHash("")
	if h == "" {
		t.Error("TextHash of empty string should not be empty")
	}
	if len(h) != 64 {
		t.Errorf("TextHash length = %d, want 64", len(h))
	}
}
