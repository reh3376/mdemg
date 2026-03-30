package llmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			Usage: OpenAIUsage{TotalTokens: 99},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL})

	text, tokens, err := c.CompleteWithUsage(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err != nil {
		t.Fatalf("CompleteWithUsage failed: %v", err)
	}
	if text != "response text" {
		t.Errorf("expected 'response text', got %q", text)
	}
	if tokens != 99 {
		t.Errorf("expected 99 tokens, got %d", tokens)
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

	text, tokens, err := c.CompleteWithUsage(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err != nil {
		t.Fatalf("CompleteWithUsage failed: %v", err)
	}
	if text != "ollama usage response" {
		t.Errorf("expected 'ollama usage response', got %q", text)
	}
	if tokens != 0 {
		t.Errorf("expected tokens=0 for Ollama (no usage reporting), got %d", tokens)
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
			Usage:   OpenAIUsage{TotalTokens: 10},
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
