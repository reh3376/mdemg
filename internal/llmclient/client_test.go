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
