package llmclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := New(Config{
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		APIKey:    "test-key",
		BaseURL:   "https://api.openai.com/v1",
		TimeoutMs: 5000,
	})

	if c.provider != "openai" {
		t.Errorf("expected provider openai, got %s", c.provider)
	}
	if c.model != "gpt-4o-mini" {
		t.Errorf("expected model gpt-4o-mini, got %s", c.model)
	}
	if c.apiKey != "test-key" {
		t.Errorf("expected apiKey test-key, got %s", c.apiKey)
	}
}

func TestNewClientDefaults(t *testing.T) {
	c := New(Config{Provider: "ollama", Model: "llama3"})

	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", c.httpClient.Timeout)
	}
}

func TestCompleteOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("wrong auth header: %s", r.Header.Get("Authorization"))
		}

		var reqBody OpenAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if reqBody.Model != "gpt-4o-mini" {
			t.Errorf("expected model gpt-4o-mini, got %s", reqBody.Model)
		}
		if len(reqBody.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(reqBody.Messages))
		}

		resp := OpenAIChatResponse{
			Choices: []OpenAIChoice{
				{Message: Message{Role: "assistant", Content: "test response"}},
			},
			Usage: OpenAIUsage{TotalTokens: 42},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(Config{
		Provider: "openai",
		Model:    "gpt-4o-mini",
		APIKey:   "test-key",
		BaseURL:  server.URL,
	})

	msgs := []Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
	}

	result, err := c.Complete(context.Background(), msgs, CompleteOpts{MaxTokens: 500})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if result != "test response" {
		t.Errorf("expected 'test response', got %q", result)
	}
}

func TestCompleteWithUsageOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := OpenAIChatResponse{
			Choices: []OpenAIChoice{
				{Message: Message{Content: "response text"}},
			},
			Usage: OpenAIUsage{PromptTokens: 42, CompletionTokens: 57, TotalTokens: 99},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL})

	text, tokensIn, tokensOut, err := c.CompleteWithUsage(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err != nil {
		t.Fatalf("CompleteWithUsage failed: %v", err)
	}
	if text != "response text" {
		t.Errorf("expected 'response text', got %q", text)
	}
	if tokensIn != 42 {
		t.Errorf("expected tokensIn=42, got %d", tokensIn)
	}
	if tokensOut != 57 {
		t.Errorf("expected tokensOut=57, got %d", tokensOut)
	}
}

func TestCompleteWithUsageOllama(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := OllamaGenerateResponse{
			Response: "ollama usage response",
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(Config{Provider: "ollama", Model: "llama3", BaseURL: server.URL})

	text, tokensIn, tokensOut, err := c.CompleteWithUsage(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err != nil {
		t.Fatalf("CompleteWithUsage failed: %v", err)
	}
	if text != "ollama usage response" {
		t.Errorf("expected 'ollama usage response', got %q", text)
	}
	if tokensIn != 0 {
		t.Errorf("expected tokensIn=0 for Ollama, got %d", tokensIn)
	}
	if tokensOut != 0 {
		t.Errorf("expected tokensOut=0 for Ollama, got %d", tokensOut)
	}
}

func TestCompleteOllama(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var reqBody OllamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if reqBody.Model != "llama3" {
			t.Errorf("expected model llama3, got %s", reqBody.Model)
		}
		if reqBody.Stream {
			t.Error("stream should be false")
		}

		resp := OllamaGenerateResponse{
			Response: "ollama response",
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(Config{
		Provider: "ollama",
		Model:    "llama3",
		BaseURL:  server.URL,
	})

	msgs := []Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "User input"},
	}

	result, err := c.Complete(context.Background(), msgs, CompleteOpts{})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if result != "ollama response" {
		t.Errorf("expected 'ollama response', got %q", result)
	}
}

func TestCompleteOllamaWithFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody OllamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if reqBody.Format == nil {
			t.Error("expected format to be set")
		}
		if reqBody.Options == nil {
			t.Error("expected options to be set")
		}

		resp := OllamaGenerateResponse{Response: `{"name":"test"}`}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(Config{Provider: "ollama", Model: "llama3", BaseURL: server.URL})

	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	result, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "test"}}, CompleteOpts{
		Format:  schema,
		Options: map[string]any{"temperature": 0.3},
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if result != `{"name":"test"}` {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestCompleteOpenAIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL})

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestCompleteOllamaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	c := New(Config{Provider: "ollama", Model: "llama3", BaseURL: server.URL})

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

func TestCompleteTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL, TimeoutMs: 50})

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestCompleteNoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := OpenAIChatResponse{Choices: []OpenAIChoice{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL})

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestCompleteOpenAIAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := OpenAIChatResponse{
			Error: &OpenAIError{Message: "rate limit exceeded"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL})

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestProviderSwitching(t *testing.T) {
	// OpenAI server
	openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "openai"}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer openaiServer.Close()

	// Ollama server
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := OllamaGenerateResponse{Response: "ollama"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ollamaServer.Close()

	msgs := []Message{{Role: "user", Content: "test"}}

	// Test OpenAI
	c1 := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: openaiServer.URL})
	r1, err := c1.Complete(context.Background(), msgs, CompleteOpts{})
	if err != nil {
		t.Fatalf("openai: %v", err)
	}
	if r1 != "openai" {
		t.Errorf("expected 'openai', got %q", r1)
	}

	// Test Ollama
	c2 := New(Config{Provider: "ollama", Model: "llama3", BaseURL: ollamaServer.URL})
	r2, err := c2.Complete(context.Background(), msgs, CompleteOpts{})
	if err != nil {
		t.Fatalf("ollama: %v", err)
	}
	if r2 != "ollama" {
		t.Errorf("expected 'ollama', got %q", r2)
	}

	// Test default (should use OpenAI)
	c3 := New(Config{Provider: "", Model: "gpt-4o", APIKey: "k", BaseURL: openaiServer.URL})
	r3, err := c3.Complete(context.Background(), msgs, CompleteOpts{})
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if r3 != "openai" {
		t.Errorf("expected 'openai', got %q", r3)
	}
}

func TestCompleteWithTemperature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody OpenAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if reqBody.Temperature == nil {
			t.Error("expected temperature to be set")
		} else if *reqBody.Temperature != 0.0 {
			t.Errorf("expected temperature 0.0, got %f", *reqBody.Temperature)
		}

		resp := OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL})
	temp := 0.0
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{
		Temperature: &temp,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStripCodeFence(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"key": "value"}`, `{"key": "value"}`},
		{"```json\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"```\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"  ```json\n{\"key\": \"value\"}\n```  ", `{"key": "value"}`},
		{"no fences here", "no fences here"},
	}

	for _, tt := range tests {
		result := StripCodeFence(tt.input)
		if result != tt.expected {
			t.Errorf("StripCodeFence(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTruncateForLog(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"longer string", 5, "longe..."},
		{"exact", 5, "exact"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := TruncateForLog(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("TruncateForLog(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

// ─── Recorder Integration ───

type mockRecorder struct {
	records []InteractionRecord
}

func (m *mockRecorder) Record(_ context.Context, rec InteractionRecord) {
	m.records = append(m.records, rec)
}

func TestClient_RecorderCalledOnComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "recorded"}}},
			Usage:   OpenAIUsage{PromptTokens: 15, CompletionTokens: 8, TotalTokens: 23},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rec := &mockRecorder{}
	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL})
	c.SetRecorder(rec)
	c = c.WithContext("test.task", "test-space")

	msgs := []Message{
		{Role: "system", Content: "sys prompt"},
		{Role: "user", Content: "user prompt"},
	}
	_, err := c.Complete(context.Background(), msgs, CompleteOpts{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("recorder calls: got %d, want 1", len(rec.records))
	}
	r := rec.records[0]
	if r.TaskName != "test.task" {
		t.Errorf("TaskName: got %q, want %q", r.TaskName, "test.task")
	}
	if r.SpaceID != "test-space" {
		t.Errorf("SpaceID: got %q, want %q", r.SpaceID, "test-space")
	}
	if r.SystemPrompt != "sys prompt" {
		t.Errorf("SystemPrompt: got %q, want %q", r.SystemPrompt, "sys prompt")
	}
	if r.UserPrompt != "user prompt" {
		t.Errorf("UserPrompt: got %q, want %q", r.UserPrompt, "user prompt")
	}
	if r.Response != "recorded" {
		t.Errorf("Response: got %q, want %q", r.Response, "recorded")
	}
	if r.ModelName != "gpt-4o" {
		t.Errorf("ModelName: got %q, want %q", r.ModelName, "gpt-4o")
	}
	if r.Provider != "openai" {
		t.Errorf("Provider: got %q, want %q", r.Provider, "openai")
	}
	if r.LatencyMs < 0 {
		t.Errorf("LatencyMs: got %d, want >= 0", r.LatencyMs)
	}
	if r.TokensIn != 15 {
		t.Errorf("TokensIn: got %d, want 15", r.TokensIn)
	}
	if r.TokensOut != 8 {
		t.Errorf("TokensOut: got %d, want 8", r.TokensOut)
	}
}

func TestComplete_RecordsTokensEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "token test"}}},
			Usage:   OpenAIUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rec := &mockRecorder{}
	c := New(Config{Provider: "openai", Model: "gpt-5.4", APIKey: "k", BaseURL: server.URL})
	c.SetRecorder(rec)
	c = c.WithContext("training.test", "mdemg-dev")

	_, err := c.Complete(context.Background(), []Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "user"},
	}, CompleteOpts{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("recorder calls: got %d, want 1", len(rec.records))
	}
	r := rec.records[0]
	if r.TokensIn != 100 {
		t.Errorf("TokensIn: got %d, want 100 (prompt_tokens from OpenAI)", r.TokensIn)
	}
	if r.TokensOut != 50 {
		t.Errorf("TokensOut: got %d, want 50 (completion_tokens from OpenAI)", r.TokensOut)
	}
	if r.TaskName != "training.test" {
		t.Errorf("TaskName: got %q, want %q", r.TaskName, "training.test")
	}
}

func TestComplete_OllamaRecordsZeroTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := OllamaGenerateResponse{Response: "ollama token test", Done: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rec := &mockRecorder{}
	c := New(Config{Provider: "ollama", Model: "llama3", BaseURL: server.URL})
	c.SetRecorder(rec)

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("recorder calls: got %d, want 1", len(rec.records))
	}
	r := rec.records[0]
	if r.TokensIn != 0 {
		t.Errorf("TokensIn: got %d, want 0 (Ollama has no token reporting)", r.TokensIn)
	}
	if r.TokensOut != 0 {
		t.Errorf("TokensOut: got %d, want 0 (Ollama has no token reporting)", r.TokensOut)
	}
}

func TestClient_RecorderCalledOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	rec := &mockRecorder{}
	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL})
	c.SetRecorder(rec)

	_, _ = c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})

	if len(rec.records) != 1 {
		t.Fatalf("recorder calls: got %d, want 1", len(rec.records))
	}
	if rec.records[0].Error == "" {
		t.Error("expected Error field to be set on failed call")
	}
}

func TestClient_NilRecorderNoOp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL})
	// No recorder set — should not panic
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetDefaultRecorder(t *testing.T) {
	// Save and restore global state
	orig := defaultRecorder
	defer func() { defaultRecorder = orig }()

	rec := &mockRecorder{}
	SetDefaultRecorder(rec)

	c := New(Config{Provider: "openai", Model: "test"})
	if c.recorder != rec {
		t.Error("expected new client to inherit defaultRecorder")
	}

	SetDefaultRecorder(nil)
	c2 := New(Config{Provider: "openai", Model: "test"})
	if c2.recorder != nil {
		t.Error("expected nil recorder after clearing default")
	}
}

func TestWithContext(t *testing.T) {
	c := New(Config{Provider: "openai", Model: "test"})
	c2 := c.WithContext("task-a", "space-b")

	if c2.taskName != "task-a" {
		t.Errorf("taskName: got %q, want %q", c2.taskName, "task-a")
	}
	if c2.spaceID != "space-b" {
		t.Errorf("spaceID: got %q, want %q", c2.spaceID, "space-b")
	}
	// Original should be unmodified
	if c.taskName != "" {
		t.Errorf("original taskName modified: got %q", c.taskName)
	}
}

func TestWithSessionID(t *testing.T) {
	ctx := context.Background()

	// No session in fresh context
	if got := SessionIDFromContext(ctx); got != "" {
		t.Errorf("SessionIDFromContext(background) = %q, want empty", got)
	}

	// Round-trip
	ctx = WithSessionID(ctx, "sess-abc-123")
	if got := SessionIDFromContext(ctx); got != "sess-abc-123" {
		t.Errorf("SessionIDFromContext = %q, want %q", got, "sess-abc-123")
	}
}

func TestRecordInteraction_SessionID(t *testing.T) {
	rec := &mockRecorder{}

	c := New(Config{Provider: "openai", Model: "test"})
	c.recorder = rec
	c.taskName = "test.task"

	// Session from context should be set on the record
	ctx := WithSessionID(context.Background(), "explicit-session")
	c.recordInteraction(ctx, []Message{{Role: "user", Content: "hello"}}, "world", 10, 5, 100, nil)

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}
	if rec.records[0].SessionID != "explicit-session" {
		t.Errorf("SessionID = %q, want %q", rec.records[0].SessionID, "explicit-session")
	}
}

func TestRecordInteraction_DefaultSessionID(t *testing.T) {
	rec := &mockRecorder{}

	oldDefault := defaultSessionID
	defer func() { defaultSessionID = oldDefault }()

	SetDefaultSessionID("instance-fallback")

	c := New(Config{Provider: "openai", Model: "test"})
	c.recorder = rec
	c.taskName = "test.task"

	// No session in context — should fall back to default
	c.recordInteraction(context.Background(), []Message{{Role: "user", Content: "hello"}}, "world", 10, 5, 100, nil)

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}
	if rec.records[0].SessionID != "instance-fallback" {
		t.Errorf("SessionID = %q, want %q", rec.records[0].SessionID, "instance-fallback")
	}
}

func TestRecordInteraction_ExplicitOverridesDefault(t *testing.T) {
	rec := &mockRecorder{}

	oldDefault := defaultSessionID
	defer func() { defaultSessionID = oldDefault }()

	SetDefaultSessionID("instance-fallback")

	c := New(Config{Provider: "openai", Model: "test"})
	c.recorder = rec
	c.taskName = "test.task"

	// Explicit session should override default
	ctx := WithSessionID(context.Background(), "request-session")
	c.recordInteraction(ctx, []Message{{Role: "user", Content: "hello"}}, "world", 10, 5, 100, nil)

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}
	if rec.records[0].SessionID != "request-session" {
		t.Errorf("SessionID = %q, want %q", rec.records[0].SessionID, "request-session")
	}
}

func TestThinkContentExtraction(t *testing.T) {
	tests := []struct {
		name          string
		response      string
		wantThink     string
		wantThinkMode bool
	}{
		{
			name:          "with think block",
			response:      "prefix <think>reasoning about constraints</think> suffix",
			wantThink:     "reasoning about constraints",
			wantThinkMode: true,
		},
		{
			name:          "no think block",
			response:      "just a normal response",
			wantThink:     "",
			wantThinkMode: false,
		},
		{
			name:          "empty response",
			response:      "",
			wantThink:     "",
			wantThinkMode: false,
		},
		{
			name:          "think at start",
			response:      "<think>first thought</think> then answer",
			wantThink:     "first thought",
			wantThinkMode: true,
		},
		{
			name:          "unclosed think tag",
			response:      "<think>unclosed reasoning",
			wantThink:     "",
			wantThinkMode: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &mockRecorder{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				json.NewEncoder(w).Encode(OpenAIChatResponse{
					Choices: []OpenAIChoice{{Message: Message{Content: tt.response}}},
				})
			}))
			defer srv.Close()

			c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
			c.SetRecorder(rec)

			_, _ = c.Complete(context.Background(), []Message{{Role: "user", Content: "test"}}, CompleteOpts{})

			if len(rec.records) != 1 {
				t.Fatalf("expected 1 record, got %d", len(rec.records))
			}
			if rec.records[0].ThinkContent != tt.wantThink {
				t.Errorf("ThinkContent: got %q, want %q", rec.records[0].ThinkContent, tt.wantThink)
			}
			if rec.records[0].ThinkMode != tt.wantThinkMode {
				t.Errorf("ThinkMode: got %v, want %v", rec.records[0].ThinkMode, tt.wantThinkMode)
			}
		})
	}
}

func TestContextGuidanceID(t *testing.T) {
	rec := &mockRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
	c.SetRecorder(rec)

	ctx := WithGuidanceID(context.Background(), "guid-abc-123")
	_, _ = c.Complete(ctx, []Message{{Role: "user", Content: "test"}}, CompleteOpts{})

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}
	if rec.records[0].GuidanceID != "guid-abc-123" {
		t.Errorf("GuidanceID: got %q, want %q", rec.records[0].GuidanceID, "guid-abc-123")
	}
}

func TestContextSourcePath(t *testing.T) {
	rec := &mockRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
	c.SetRecorder(rec)

	ctx := WithSourcePath(context.Background(), "CLAUDE.md")
	_, _ = c.Complete(ctx, []Message{{Role: "user", Content: "test"}}, CompleteOpts{})

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}
	if rec.records[0].SourcePath != "CLAUDE.md" {
		t.Errorf("SourcePath: got %q, want %q", rec.records[0].SourcePath, "CLAUDE.md")
	}
}

func TestContextBothKeys(t *testing.T) {
	rec := &mockRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
	c.SetRecorder(rec)

	ctx := WithGuidanceID(context.Background(), "guid-xyz")
	ctx = WithSourcePath(ctx, "AGENT_HANDOFF.md")
	_, _ = c.Complete(ctx, []Message{{Role: "user", Content: "test"}}, CompleteOpts{})

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}
	if rec.records[0].GuidanceID != "guid-xyz" {
		t.Errorf("GuidanceID: got %q, want %q", rec.records[0].GuidanceID, "guid-xyz")
	}
	if rec.records[0].SourcePath != "AGENT_HANDOFF.md" {
		t.Errorf("SourcePath: got %q, want %q", rec.records[0].SourcePath, "AGENT_HANDOFF.md")
	}
}

func TestSystemPromptHash(t *testing.T) {
	rec := &mockRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
	c.SetRecorder(rec)

	msgs := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "test"},
	}
	_, _ = c.Complete(context.Background(), msgs, CompleteOpts{})

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}

	hash := rec.records[0].SystemPromptHash
	if hash == "" {
		t.Fatal("SystemPromptHash should not be empty when system prompt is set")
	}
	if len(hash) != 64 {
		t.Errorf("SystemPromptHash length: got %d, want 64 (SHA-256 hex)", len(hash))
	}

	// Deterministic: same prompt → same hash
	_, _ = c.Complete(context.Background(), msgs, CompleteOpts{})
	if rec.records[1].SystemPromptHash != hash {
		t.Errorf("hash not deterministic: first %q, second %q", hash, rec.records[1].SystemPromptHash)
	}
}

func TestSystemPromptHashEmpty(t *testing.T) {
	rec := &mockRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
	c.SetRecorder(rec)

	// No system message
	_, _ = c.Complete(context.Background(), []Message{{Role: "user", Content: "test"}}, CompleteOpts{})

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}
	if rec.records[0].SystemPromptHash != "" {
		t.Errorf("SystemPromptHash should be empty when no system prompt, got %q", rec.records[0].SystemPromptHash)
	}
}

func TestContextNoKeys(t *testing.T) {
	rec := &mockRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
	c.SetRecorder(rec)

	_, _ = c.Complete(context.Background(), []Message{{Role: "user", Content: "test"}}, CompleteOpts{})

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}
	if rec.records[0].GuidanceID != "" {
		t.Errorf("GuidanceID should be empty, got %q", rec.records[0].GuidanceID)
	}
	if rec.records[0].SourcePath != "" {
		t.Errorf("SourcePath should be empty, got %q", rec.records[0].SourcePath)
	}
}

func TestWithRetrievalContext(t *testing.T) {
	rec := &mockRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
	c.SetRecorder(rec)

	rc := &RetrievalContext{
		NodeIDs:  []string{"node-1", "node-2", "node-3"},
		Scores:   []float64{0.95, 0.87, 0.72},
		OracleID: "node-1",
	}
	ctx := WithRetrievalContext(context.Background(), rc)
	_, _ = c.Complete(ctx, []Message{{Role: "user", Content: "test"}}, CompleteOpts{})

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}
	r := rec.records[0]
	if r.RetrievalCtx == nil {
		t.Fatal("RetrievalCtx should not be nil")
	}
	if len(r.RetrievalCtx.NodeIDs) != 3 {
		t.Errorf("NodeIDs: got %d, want 3", len(r.RetrievalCtx.NodeIDs))
	}
	if r.RetrievalCtx.NodeIDs[0] != "node-1" {
		t.Errorf("NodeIDs[0]: got %q, want %q", r.RetrievalCtx.NodeIDs[0], "node-1")
	}
	if r.RetrievalCtx.Scores[0] != 0.95 {
		t.Errorf("Scores[0]: got %f, want 0.95", r.RetrievalCtx.Scores[0])
	}
	if r.RetrievalCtx.OracleID != "node-1" {
		t.Errorf("OracleID: got %q, want %q", r.RetrievalCtx.OracleID, "node-1")
	}
}

func TestRetrievalContextNil(t *testing.T) {
	rec := &mockRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
	c.SetRecorder(rec)

	// No retrieval context set — should not panic, RetrievalCtx should be nil
	_, _ = c.Complete(context.Background(), []Message{{Role: "user", Content: "test"}}, CompleteOpts{})

	if len(rec.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.records))
	}
	if rec.records[0].RetrievalCtx != nil {
		t.Error("RetrievalCtx should be nil when no context is set")
	}
}

// --- Training Data Capture Verification Tests ---

func TestMultiMessageLastWins(t *testing.T) {
	rec := &mockRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "response"}}},
		})
	}))
	defer srv.Close()

	c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
	c.SetRecorder(rec)

	// Multiple system and user messages — "last wins" extraction rule
	msgs := []Message{
		{Role: "system", Content: "first system prompt"},
		{Role: "user", Content: "first user message"},
		{Role: "system", Content: "second system prompt"},
		{Role: "user", Content: "second user message"},
	}

	_, err := c.Complete(context.Background(), msgs, CompleteOpts{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("recorder calls: got %d, want 1", len(rec.records))
	}
	r := rec.records[0]

	// Last system message wins
	if r.SystemPrompt != "second system prompt" {
		t.Errorf("SystemPrompt: got %q, want %q", r.SystemPrompt, "second system prompt")
	}

	// Last user message wins
	if r.UserPrompt != "second user message" {
		t.Errorf("UserPrompt: got %q, want %q", r.UserPrompt, "second user message")
	}
}

func TestScrubBoundary(t *testing.T) {
	// Scrubbing happens in TSDB writer (Record), NOT in llmclient.
	// The recorder should receive the raw, unscrubbed record.
	rec := &mockRecorder{}
	apiKey := "sk-HvPliFZCy8ohpHoZZ1wUy0EY5ltXXXXX"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "found key: " + apiKey}}},
		})
	}))
	defer srv.Close()

	c := New(Config{Provider: "openai", Model: "test", APIKey: "k", BaseURL: srv.URL})
	c.SetRecorder(rec)

	_, _ = c.Complete(context.Background(), []Message{
		{Role: "system", Content: "you are helpful"},
		{Role: "user", Content: "find the key"},
	}, CompleteOpts{})

	if len(rec.records) != 1 {
		t.Fatalf("recorder calls: got %d, want 1", len(rec.records))
	}

	// API key should be present in the raw record (not scrubbed at client level)
	r := rec.records[0]
	if !strings.Contains(r.Response, apiKey) {
		t.Errorf("Response should contain raw API key at client level, got %q", r.Response)
	}
}

func TestScrubAllPatterns(t *testing.T) {
	rec := InteractionRecord{
		SystemPrompt: "key: sk-HvPliFZCy8ohpHoZZ1wUy0EY5ltXXXXX",
		UserPrompt:   "Check /Users/reh3376/mdemg/internal/api/server.go",
		Response:     "Found user@example.com and PASSWORD=secret123 in config",
		ThinkContent: "Connecting to neo4j://admin:pass123@localhost:7687",
	}

	Scrub(&rec)

	// SystemPrompt: API key scrubbed
	if strings.Contains(rec.SystemPrompt, "sk-HvPli") {
		t.Error("SystemPrompt: API key was not scrubbed")
	}
	if !strings.Contains(rec.SystemPrompt, "[REDACTED_KEY]") {
		t.Errorf("SystemPrompt: expected [REDACTED_KEY], got %q", rec.SystemPrompt)
	}

	// UserPrompt: absolute path scrubbed (keeps last 2 components)
	if strings.Contains(rec.UserPrompt, "/Users/reh3376") {
		t.Error("UserPrompt: absolute path was not scrubbed")
	}
	if !strings.Contains(rec.UserPrompt, "[PATH]") {
		t.Errorf("UserPrompt: expected [PATH], got %q", rec.UserPrompt)
	}
	if !strings.Contains(rec.UserPrompt, "api/server.go") {
		t.Errorf("UserPrompt: expected last 2 path components preserved, got %q", rec.UserPrompt)
	}

	// Response: email + env secret scrubbed
	if strings.Contains(rec.Response, "user@example.com") {
		t.Error("Response: email was not scrubbed")
	}
	if !strings.Contains(rec.Response, "[EMAIL]") {
		t.Errorf("Response: expected [EMAIL], got %q", rec.Response)
	}
	if strings.Contains(rec.Response, "secret123") {
		t.Error("Response: env secret was not scrubbed")
	}
	if !strings.Contains(rec.Response, "PASSWORD=[REDACTED]") {
		t.Errorf("Response: expected PASSWORD=[REDACTED], got %q", rec.Response)
	}

	// ThinkContent: neo4j credentials scrubbed
	if strings.Contains(rec.ThinkContent, "admin:pass123") {
		t.Error("ThinkContent: Neo4j credentials were not scrubbed")
	}
	if !strings.Contains(rec.ThinkContent, "neo4j://[REDACTED]@") {
		t.Errorf("ThinkContent: expected neo4j://[REDACTED]@, got %q", rec.ThinkContent)
	}
}

// ─── Retry Tests ───

func retryConfig() RetryConfig {
	return RetryConfig{
		Enabled:     true,
		MaxAttempts: 2,
		BaseDelayMs: 10,
		MaxDelayMs:  100,
		Multiplier:  2.0,
		Jitter:      0.0, // deterministic for tests
	}
}

func TestRetry_429WithRetryAfter(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited"))
			return
		}
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "success"}}},
		})
	}))
	defer server.Close()

	c := New(Config{
		Provider: "openai", Model: "test", APIKey: "k",
		BaseURL: server.URL, Retry: retryConfig(),
	})

	text, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if text != "success" {
		t.Errorf("expected 'success', got %q", text)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_503(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("service unavailable"))
			return
		}
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "recovered"}}},
		})
	}))
	defer server.Close()

	c := New(Config{
		Provider: "openai", Model: "test", APIKey: "k",
		BaseURL: server.URL, Retry: retryConfig(),
	})

	text, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err != nil {
		t.Fatalf("expected recovery, got: %v", err)
	}
	if text != "recovered" {
		t.Errorf("expected 'recovered', got %q", text)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetry_NoRetryOn400(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	c := New(Config{
		Provider: "openai", Model: "test", APIKey: "k",
		BaseURL: server.URL, Retry: retryConfig(),
	})

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
}

func TestRetry_MaxAttemptsExhausted(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limited"))
	}))
	defer server.Close()

	c := New(Config{
		Provider: "openai", Model: "test", APIKey: "k",
		BaseURL: server.URL, Retry: retryConfig(),
	})

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected error after max attempts")
	}
	if !strings.Contains(err.Error(), "failed after") {
		t.Errorf("expected 'failed after' in error, got: %v", err)
	}
	// MaxAttempts=2 means initial + 2 retries = 3 total
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetry_Disabled(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("unavailable"))
			return
		}
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		})
	}))
	defer server.Close()

	c := New(Config{
		Provider: "openai", Model: "test", APIKey: "k",
		BaseURL: server.URL,
		Retry:   RetryConfig{Enabled: false},
	})

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected error without retry")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (retry disabled), got %d", attempts)
	}
}

func TestRetry_OllamaTimeout(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("unavailable"))
			return
		}
		json.NewEncoder(w).Encode(OllamaGenerateResponse{Response: "ollama ok", Done: true})
	}))
	defer server.Close()

	c := New(Config{
		Provider: "ollama", Model: "llama3",
		BaseURL: server.URL, Retry: retryConfig(),
	})

	text, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if text != "ollama ok" {
		t.Errorf("expected 'ollama ok', got %q", text)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetry_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limited"))
	}))
	defer server.Close()

	c := New(Config{
		Provider: "openai", Model: "test", APIKey: "k",
		BaseURL: server.URL, Retry: RetryConfig{
			Enabled:     true,
			MaxAttempts: 10,
			BaseDelayMs: 5000, // long delay
			MaxDelayMs:  10000,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.Complete(ctx, []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

func TestShouldRetry(t *testing.T) {
	// Default: RetryOnDeadline = false. Matches the pre-DH-004 behavior.
	rc := RetryConfig{BaseDelayMs: 500}
	ctx := context.Background()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"429", &httpError{StatusCode: 429}, true},
		{"502", &httpError{StatusCode: 502}, true},
		{"503", &httpError{StatusCode: 503}, true},
		{"400", &httpError{StatusCode: 400}, false},
		{"401", &httpError{StatusCode: 401}, false},
		{"500", &httpError{StatusCode: 500}, false},
		{"context.Canceled", context.Canceled, false},
		{"context.DeadlineExceeded (default off)", context.DeadlineExceeded, false},
		{"network error", fmt.Errorf("http request: connection refused"), true},
		{"non-retryable", fmt.Errorf("marshal request: invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetry(ctx, rc, tt.err); got != tt.want {
				t.Errorf("shouldRetry(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// DH-004 E4.2: opt-in retry on context.DeadlineExceeded when budget allows.

func TestShouldRetry_DeadlineExceededWithBudget(t *testing.T) {
	rc := RetryConfig{BaseDelayMs: 500, RetryOnDeadline: true}
	// 10s of budget remaining — far more than 2×500ms.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !shouldRetry(ctx, rc, context.DeadlineExceeded) {
		t.Error("expected retry when budget >> 2×BaseDelayMs, got false")
	}
}

func TestShouldRetry_DeadlineExceededNoBudget(t *testing.T) {
	rc := RetryConfig{BaseDelayMs: 500, RetryOnDeadline: true}
	// Only 100ms of budget — less than 2×500ms, so a retry would certainly fail.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if shouldRetry(ctx, rc, context.DeadlineExceeded) {
		t.Error("expected no retry when budget < 2×BaseDelayMs, got true")
	}
}

func TestShouldRetry_DeadlineExceededNoParentDeadline(t *testing.T) {
	// No parent deadline → the error came from the per-request HTTP client
	// timeout, not the caller. Retry is safe.
	rc := RetryConfig{BaseDelayMs: 500, RetryOnDeadline: true}
	if !shouldRetry(context.Background(), rc, context.DeadlineExceeded) {
		t.Error("expected retry when parent ctx has no deadline, got false")
	}
}

func TestShouldRetry_DeadlineDisabled(t *testing.T) {
	// Feature explicitly off — never retry on DeadlineExceeded.
	rc := RetryConfig{BaseDelayMs: 500, RetryOnDeadline: false}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if shouldRetry(ctx, rc, context.DeadlineExceeded) {
		t.Error("expected no retry when RetryOnDeadline=false, got true")
	}
}

func TestShouldRetry_CanceledIgnoresDeadlineFlag(t *testing.T) {
	// Canceled is always non-retryable, regardless of RetryOnDeadline.
	rc := RetryConfig{BaseDelayMs: 500, RetryOnDeadline: true}
	if shouldRetry(context.Background(), rc, context.Canceled) {
		t.Error("context.Canceled must never be retried")
	}
}

func TestParseRetryAfter(t *testing.T) {
	// Seconds
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "5")
	if got := parseRetryAfter(resp); got != 5*time.Second {
		t.Errorf("parseRetryAfter(5) = %v, want 5s", got)
	}

	// Missing
	resp2 := &http.Response{Header: http.Header{}}
	if got := parseRetryAfter(resp2); got != 0 {
		t.Errorf("parseRetryAfter(missing) = %v, want 0", got)
	}
}

func TestConsecutiveFailure_FiresAtThreshold(t *testing.T) {
	// Save and restore global state.
	origCB := defaultAlertCallback
	origThreshold := defaultFailureThreshold
	defer func() {
		defaultAlertCallback = origCB
		defaultFailureThreshold = origThreshold
	}()

	var mu sync.Mutex
	var fired []int
	defaultAlertCallback = func(taskName string, count int, lastErr error) {
		mu.Lock()
		fired = append(fired, count)
		mu.Unlock()
	}
	defaultFailureThreshold = 3

	c := &Client{
		taskName:            "test-task",
		consecutiveFailures: new(atomic.Int32),
		failureThreshold:    3,
		tripped:             new(atomic.Bool),
	}

	err := fmt.Errorf("connection refused")
	c.trackResult(err) // 1
	c.trackResult(err) // 2
	c.trackResult(err) // 3 — should fire

	mu.Lock()
	defer mu.Unlock()
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(fired))
	}
	if fired[0] != 3 {
		t.Errorf("expected count=3, got %d", fired[0])
	}
}

func TestConsecutiveFailure_ResetsOnSuccess(t *testing.T) {
	origCB := defaultAlertCallback
	origThreshold := defaultFailureThreshold
	defer func() {
		defaultAlertCallback = origCB
		defaultFailureThreshold = origThreshold
	}()

	var alertCount atomic.Int32
	defaultAlertCallback = func(_ string, _ int, _ error) {
		alertCount.Add(1)
	}
	defaultFailureThreshold = 3

	c := &Client{
		taskName:            "test-task",
		consecutiveFailures: new(atomic.Int32),
		failureThreshold:    3,
		tripped:             new(atomic.Bool),
	}

	err := fmt.Errorf("timeout")
	c.trackResult(err) // 1
	c.trackResult(err) // 2
	c.trackResult(nil) // success — resets to 0
	c.trackResult(err) // 1
	c.trackResult(err) // 2

	if alertCount.Load() != 0 {
		t.Errorf("expected 0 alerts (counter reset by success), got %d", alertCount.Load())
	}
}

// LLM-HEALTH-CANCELLATION-ALERT-001: caller cancellations are neutral — they
// never trip the consecutive-failure alert regardless of count.
func TestConsecutiveFailure_CallerCancellationNeutral(t *testing.T) {
	origCB := defaultAlertCallback
	origThreshold := defaultFailureThreshold
	defer func() {
		defaultAlertCallback = origCB
		defaultFailureThreshold = origThreshold
	}()

	var alertCount atomic.Int32
	defaultAlertCallback = func(_ string, _ int, _ error) {
		alertCount.Add(1)
	}
	defaultFailureThreshold = 3

	c := &Client{
		taskName:            "test-task",
		consecutiveFailures: new(atomic.Int32),
		failureThreshold:    3,
		tripped:             new(atomic.Bool),
	}

	// Wrapped forms mirror production ("http request: Post ...: context deadline exceeded").
	canceled := fmt.Errorf("http request: %w", context.Canceled)
	deadline := fmt.Errorf("http request: %w", context.DeadlineExceeded)
	for range 3 {
		c.trackResult(canceled)
		c.trackResult(deadline)
	}

	if alertCount.Load() != 0 {
		t.Errorf("expected 0 alerts from cancellations, got %d", alertCount.Load())
	}
	if got := c.consecutiveFailures.Load(); got != 0 {
		t.Errorf("expected counter unchanged at 0, got %d", got)
	}
}

// LLM-HEALTH-CANCELLATION-ALERT-001: a cancellation interleaved in a real
// failure streak neither resets nor advances it — the alert still fires when
// real failures alone reach the threshold.
func TestConsecutiveFailure_CancellationPreservesRealStreak(t *testing.T) {
	origCB := defaultAlertCallback
	origThreshold := defaultFailureThreshold
	defer func() {
		defaultAlertCallback = origCB
		defaultFailureThreshold = origThreshold
	}()

	var mu sync.Mutex
	var fired []int
	defaultAlertCallback = func(_ string, count int, _ error) {
		mu.Lock()
		fired = append(fired, count)
		mu.Unlock()
	}
	defaultFailureThreshold = 3

	c := &Client{
		taskName:            "test-task",
		consecutiveFailures: new(atomic.Int32),
		failureThreshold:    3,
		tripped:             new(atomic.Bool),
	}

	real := fmt.Errorf("connection refused")
	c.trackResult(real)                                                  // 1
	c.trackResult(fmt.Errorf("http request: %w", context.Canceled))      // neutral
	c.trackResult(real)                                                  // 2
	c.trackResult(fmt.Errorf("http request: %w", context.DeadlineExceeded)) // neutral
	c.trackResult(real)                                                  // 3 — fires

	mu.Lock()
	defer mu.Unlock()
	if len(fired) != 1 {
		t.Fatalf("expected exactly 1 alert, got %d", len(fired))
	}
	if fired[0] != 3 {
		t.Errorf("expected count=3, got %d", fired[0])
	}
}

func TestConsecutiveFailure_NoFireBelowThreshold(t *testing.T) {
	origCB := defaultAlertCallback
	origThreshold := defaultFailureThreshold
	defer func() {
		defaultAlertCallback = origCB
		defaultFailureThreshold = origThreshold
	}()

	var alertCount atomic.Int32
	defaultAlertCallback = func(_ string, _ int, _ error) {
		alertCount.Add(1)
	}
	defaultFailureThreshold = 5

	c := &Client{
		taskName:            "test-task",
		consecutiveFailures: new(atomic.Int32),
		failureThreshold:    5,
		tripped:             new(atomic.Bool),
	}

	err := fmt.Errorf("error")
	for range 4 {
		c.trackResult(err)
	}

	if alertCount.Load() != 0 {
		t.Errorf("expected 0 alerts (below threshold 5), got %d", alertCount.Load())
	}
}

func TestConsecutiveFailure_TripGuardFiresOnce(t *testing.T) {
	origCB := defaultAlertCallback
	origThreshold := defaultFailureThreshold
	defer func() {
		defaultAlertCallback = origCB
		defaultFailureThreshold = origThreshold
	}()

	var alertCount atomic.Int32
	defaultAlertCallback = func(_ string, _ int, _ error) {
		alertCount.Add(1)
	}
	defaultFailureThreshold = 3

	c := &Client{
		taskName:            "test-task",
		consecutiveFailures: new(atomic.Int32),
		failureThreshold:    3,
		tripped:             new(atomic.Bool),
	}

	err := fmt.Errorf("connection refused")
	// 5 consecutive failures: should fire exactly once at threshold, not on 4th and 5th
	for range 5 {
		c.trackResult(err)
	}

	if got := alertCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 alert (trip guard), got %d", got)
	}
}

func TestConsecutiveFailure_RecoveryAndRetrip(t *testing.T) {
	origCB := defaultAlertCallback
	origThreshold := defaultFailureThreshold
	defer func() {
		defaultAlertCallback = origCB
		defaultFailureThreshold = origThreshold
	}()

	var alertCount atomic.Int32
	defaultAlertCallback = func(_ string, _ int, _ error) {
		alertCount.Add(1)
	}
	defaultFailureThreshold = 3

	c := &Client{
		taskName:            "test-task",
		consecutiveFailures: new(atomic.Int32),
		failureThreshold:    3,
		tripped:             new(atomic.Bool),
	}

	err := fmt.Errorf("connection refused")
	// First trip
	for range 3 {
		c.trackResult(err)
	}
	if got := alertCount.Load(); got != 1 {
		t.Fatalf("expected 1 alert after first trip, got %d", got)
	}

	// Recovery
	c.trackResult(nil)

	// Second trip
	for range 3 {
		c.trackResult(err)
	}
	if got := alertCount.Load(); got != 2 {
		t.Errorf("expected 2 alerts (trip-recovery-trip), got %d", got)
	}
}
