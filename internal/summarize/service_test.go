package summarize

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mdemg/internal/llmclient"
)

func TestCacheOperations(t *testing.T) {
	cache := newSummaryCache(3)

	// Test Put and Get
	cache.Put("key1", "summary1")
	cache.Put("key2", "summary2")
	cache.Put("key3", "summary3")

	if v, ok := cache.Get("key1"); !ok || v != "summary1" {
		t.Errorf("Expected summary1, got %s (found=%v)", v, ok)
	}

	// Test LRU eviction
	cache.Put("key4", "summary4") // Should evict key1

	if _, ok := cache.Get("key1"); ok {
		t.Error("key1 should have been evicted")
	}

	if v, ok := cache.Get("key4"); !ok || v != "summary4" {
		t.Errorf("Expected summary4, got %s", v)
	}

	// Test update existing key
	cache.Put("key2", "updated2")
	if v, _ := cache.Get("key2"); v != "updated2" {
		t.Errorf("Expected updated2, got %s", v)
	}

	// Size should still be 3
	if cache.Len() != 3 {
		t.Errorf("Expected cache size 3, got %d", cache.Len())
	}
}

func TestCacheKey(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Provider:     "openai",
		OpenAIAPIKey: "test-key",
	}

	s, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	elem1 := CodeElement{
		Name:    "TestFunc",
		Kind:    "function",
		Path:    "/pkg/test.go#TestFunc",
		Content: "func TestFunc() {}",
	}

	elem2 := CodeElement{
		Name:    "TestFunc",
		Kind:    "function",
		Path:    "/pkg/test.go#TestFunc",
		Content: "func TestFunc() { changed }",
	}

	key1 := s.cacheKey(elem1)
	key2 := s.cacheKey(elem2)

	if key1 == key2 {
		t.Error("Different content should produce different cache keys")
	}

	// Same element should produce same key
	key1Again := s.cacheKey(elem1)
	if key1 != key1Again {
		t.Error("Same element should produce same cache key")
	}
}

func TestBuildPrompt(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Provider:     "openai",
		OpenAIAPIKey: "test-key",
	}

	s, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	elements := []CodeElement{
		{
			Name:     "UserService",
			Kind:     "class",
			Path:     "/src/services/user.ts#UserService",
			Package:  "services",
			Content:  "export class UserService { async findById(id: string) { return this.db.users.find(id); } }",
			Concerns: []string{"database", "users"},
		},
	}

	prompt := s.buildPrompt(elements)

	// Check prompt contains key elements
	if !strings.Contains(prompt, "UserService") {
		t.Error("Prompt should contain element name")
	}
	if !strings.Contains(prompt, "class") {
		t.Error("Prompt should contain element kind")
	}
	if !strings.Contains(prompt, "database, users") {
		t.Error("Prompt should contain concerns")
	}
	if !strings.Contains(prompt, "JSON array") {
		t.Error("Prompt should mention JSON array format")
	}
	if !strings.Contains(prompt, "PRIMARY PURPOSE") {
		t.Error("Prompt should emphasize describing behavior")
	}
}

func TestOpenAIMock(t *testing.T) {
	// Create mock OpenAI server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Error("Missing or wrong Authorization header")
		}

		// Return mock response
		resp := openAIChatResponse{
			Choices: []llmclient.OpenAIChoice{
				{Message: llmclient.Message{Content: `["UserService manages user data persistence and retrieval, integrating with the database layer."]`}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		Enabled:        true,
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		OpenAIAPIKey:   "test-api-key",
		OpenAIEndpoint: server.URL,
		MaxTokens:      150,
		BatchSize:      10,
		TimeoutMs:      5000,
		CacheEnabled:   true,
		CacheSize:      100,
	}

	s, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	elements := []CodeElement{
		{
			Name:    "UserService",
			Kind:    "class",
			Path:    "/src/services/user.ts#UserService",
			Content: "export class UserService { async findById(id: string) { return this.db.users.find(id); } }",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	summaries := s.SummarizeBatch(ctx, elements)
	if len(summaries) != 1 {
		t.Fatalf("Expected 1 summary, got %d", len(summaries))
	}

	if !strings.Contains(summaries[0], "UserService") {
		t.Errorf("Unexpected summary: %s", summaries[0])
	}

	// Test cache hit
	summaries2 := s.SummarizeBatch(ctx, elements)
	if summaries[0] != summaries2[0] {
		t.Error("Cache should return same result")
	}

	calls, hits, size := s.Stats()
	if calls != 2 {
		t.Errorf("Expected 2 calls, got %d", calls)
	}
	if hits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", hits)
	}
	if size != 1 {
		t.Errorf("Expected cache size 1, got %d", size)
	}
}

func TestFallback(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	fallbackCalled := false
	fallbackFn := func(elem CodeElement) string {
		fallbackCalled = true
		return "Fallback: " + elem.Name
	}

	cfg := Config{
		Enabled:        true,
		Provider:       "openai",
		OpenAIAPIKey:   "test-key",
		OpenAIEndpoint: server.URL,
		TimeoutMs:      1000,
		CacheEnabled:   false,
	}

	s, err := New(cfg, fallbackFn)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	elements := []CodeElement{
		{Name: "TestFunc", Kind: "function", Content: "func TestFunc() {}"},
	}

	ctx := context.Background()
	summaries := s.SummarizeBatch(ctx, elements)

	if !fallbackCalled {
		t.Error("Fallback should have been called on error")
	}

	if summaries[0] != "Fallback: TestFunc" {
		t.Errorf("Unexpected fallback result: %s", summaries[0])
	}
}

func TestCombineSummary(t *testing.T) {
	tests := []struct {
		structural string
		semantic   string
		expected   string
	}{
		{
			structural: "Function auth in package main",
			semantic:   "Validates JWT tokens and manages session lifecycle.",
			expected:   "Function auth in package main SEMANTIC: Validates JWT tokens and manages session lifecycle.",
		},
		{
			structural: "Module: config.ts",
			semantic:   "",
			expected:   "Module: config.ts",
		},
		{
			structural: "",
			semantic:   "Handles user authentication.",
			expected:   "Handles user authentication.",
		},
		{
			structural: "Contains: Handles user authentication.",
			semantic:   "Handles user authentication.",
			expected:   "Contains: Handles user authentication.", // No duplication
		},
	}

	for _, tt := range tests {
		result := CombineSummary(tt.structural, tt.semantic)
		if result != tt.expected {
			t.Errorf("CombineSummary(%q, %q) = %q, want %q",
				tt.structural, tt.semantic, result, tt.expected)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Provider:     "openai",
		OpenAIAPIKey: "test-key",
		// All other fields empty/zero
	}

	s, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	if s.config.Model != "gpt-4o-mini" {
		t.Errorf("Expected default model gpt-4o-mini, got %s", s.config.Model)
	}
	if s.config.MaxTokens != 150 {
		t.Errorf("Expected default MaxTokens 150, got %d", s.config.MaxTokens)
	}
	if s.config.BatchSize != 10 {
		t.Errorf("Expected default BatchSize 10, got %d", s.config.BatchSize)
	}
	if s.config.TimeoutMs != 30000 {
		t.Errorf("Expected default TimeoutMs 30000, got %d", s.config.TimeoutMs)
	}
	if s.config.CacheSize != 5000 {
		t.Errorf("Expected default CacheSize 5000, got %d", s.config.CacheSize)
	}
	if s.config.OpenAIEndpoint != "https://api.openai.com/v1" {
		t.Errorf("Expected default OpenAIEndpoint, got %s", s.config.OpenAIEndpoint)
	}
}

func TestProviderValidation(t *testing.T) {
	// Test missing API key for OpenAI
	cfg := Config{
		Enabled:  true,
		Provider: "openai",
		// OpenAIAPIKey missing
	}

	_, err := New(cfg, nil)
	if err == nil {
		t.Error("Expected error for missing OpenAI API key")
	}

	// Test unknown provider
	cfg = Config{
		Enabled:  true,
		Provider: "unknown",
	}

	_, err = New(cfg, nil)
	if err == nil {
		t.Error("Expected error for unknown provider")
	}

	// Test disabled service
	cfg = Config{
		Enabled: false,
	}

	_, err = New(cfg, nil)
	if err == nil {
		t.Error("Expected error for disabled service")
	}
}

func TestMarkdownCodeBlockParsing(t *testing.T) {
	// Test that markdown code blocks in response are handled
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIChatResponse{
			Choices: []llmclient.OpenAIChoice{
				{Message: llmclient.Message{Content: "```json\n[\"Summary with markdown wrapper\"]\n```"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		Enabled:        true,
		Provider:       "openai",
		OpenAIAPIKey:   "test-key",
		OpenAIEndpoint: server.URL,
		TimeoutMs:      5000,
		CacheEnabled:   false,
	}

	s, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	elements := []CodeElement{{Name: "Test", Kind: "function", Content: "test"}}
	ctx := context.Background()

	summaries := s.SummarizeBatch(ctx, elements)
	if len(summaries) != 1 || summaries[0] != "Summary with markdown wrapper" {
		t.Errorf("Expected parsed summary, got: %v", summaries)
	}
}

func TestContentTruncation(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Provider:     "openai",
		OpenAIAPIKey: "test-key",
	}

	s, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Create element with very long content
	longContent := strings.Repeat("x", 3000)
	elements := []CodeElement{
		{Name: "LongFunc", Kind: "function", Content: longContent},
	}

	prompt := s.buildPrompt(elements)

	// Prompt should contain truncation indicator
	if !strings.Contains(prompt, "[truncated]") {
		t.Error("Long content should be truncated with indicator")
	}

	// Prompt should not contain full content
	if strings.Contains(prompt, strings.Repeat("x", 2000)) {
		t.Error("Content should be truncated to ~1500 chars")
	}
}
