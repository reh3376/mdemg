package retrieval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLLMIntentTranslator_DisabledReturnsOriginal(t *testing.T) {
	translator := NewLLMIntentTranslator(IntentConfig{
		Enabled: false,
	}, nil)

	result, err := translator.Translate(context.Background(), "Why do we use Redis?")
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if result != "Why do we use Redis?" {
		t.Errorf("Translate() = %q, want original query", result)
	}
}

func TestLLMIntentTranslator_EmptyQueryReturnsOriginal(t *testing.T) {
	translator := NewLLMIntentTranslator(IntentConfig{
		Enabled: true,
	}, nil)

	result, err := translator.Translate(context.Background(), "")
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if result != "" {
		t.Errorf("Translate() = %q, want empty string", result)
	}

	// Also test whitespace-only
	result, err = translator.Translate(context.Background(), "   ")
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if result != "   " {
		t.Errorf("Translate() = %q, want original whitespace", result)
	}
}

func TestLLMIntentTranslator_OpenAISuccess(t *testing.T) {
	// Capture the request to verify system+user messages and temperature
	var capturedReq intentOpenAIChatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		resp := intentOpenAIChatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "Redis session state architecture decision caching"}},
			},
			Usage: struct {
				TotalTokens int `json:"total_tokens"`
			}{TotalTokens: 25},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	translator := NewLLMIntentTranslator(IntentConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		MaxTokens: 150,
		TimeoutMs: 5000,
		OpenAIKey: "test-key",
		OpenAIURL: server.URL,
	}, nil)

	result, err := translator.Translate(context.Background(), "Why do we use Redis?")
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if result != "Redis session state architecture decision caching" {
		t.Errorf("Translate() = %q, want translated query", result)
	}

	// Verify request structure
	if len(capturedReq.Messages) != 2 {
		t.Fatalf("Messages count = %d, want 2 (system + user)", len(capturedReq.Messages))
	}
	if capturedReq.Messages[0].Role != "system" {
		t.Errorf("Messages[0].Role = %q, want system", capturedReq.Messages[0].Role)
	}
	if capturedReq.Messages[1].Role != "user" {
		t.Errorf("Messages[1].Role = %q, want user", capturedReq.Messages[1].Role)
	}
	if capturedReq.Messages[1].Content != "Why do we use Redis?" {
		t.Errorf("Messages[1].Content = %q, want original query", capturedReq.Messages[1].Content)
	}
	if capturedReq.MaxTokens < 2000 {
		t.Errorf("MaxTokens = %d, want >= 2000 (reasoning model minimum)", capturedReq.MaxTokens)
	}
}

func TestLLMIntentTranslator_OpenAIFailureFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	translator := NewLLMIntentTranslator(IntentConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		TimeoutMs: 5000,
		OpenAIKey: "test-key",
		OpenAIURL: server.URL,
	}, nil)

	result, err := translator.Translate(context.Background(), "Why do we use Redis?")
	if err == nil {
		t.Error("Translate() should return error on 500 response")
	}
	// Fail-open: should return original query
	if result != "Why do we use Redis?" {
		t.Errorf("Translate() = %q, want original query on error (fail-open)", result)
	}
}

func TestLLMIntentTranslator_OllamaSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := intentOllamaGenerateResponse{
			Response: "Ollama keyword dense rewrite output",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	translator := NewLLMIntentTranslator(IntentConfig{
		Enabled:   true,
		Provider:  "ollama",
		Model:     "llama3",
		TimeoutMs: 5000,
		OllamaURL: server.URL,
	}, nil)

	result, err := translator.Translate(context.Background(), "How does auth work?")
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if result != "Ollama keyword dense rewrite output" {
		t.Errorf("Translate() = %q, want ollama response", result)
	}
}

func TestLLMIntentTranslator_TimeoutFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow server that exceeds timeout
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	translator := NewLLMIntentTranslator(IntentConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		TimeoutMs: 200, // Very short timeout
		OpenAIKey: "test-key",
		OpenAIURL: server.URL,
	}, nil)

	result, err := translator.Translate(context.Background(), "Why do we use Redis?")
	if err == nil {
		t.Error("Translate() should return error on timeout")
	}
	// Fail-open: should return original query
	if result != "Why do we use Redis?" {
		t.Errorf("Translate() = %q, want original query on timeout (fail-open)", result)
	}
}

func TestIntentSystemPrompt_Content(t *testing.T) {
	// Verify the system prompt contains expected terms
	expectedTerms := []string{
		"query rewriter",
		"knowledge graph",
		"keyword-rich",
		"vector similarity",
		"Output ONLY",
		"domain-specific",
		"abbreviations",
		"unchanged",
	}

	for _, term := range expectedTerms {
		if !strings.Contains(intentSystemPrompt, term) {
			t.Errorf("intentSystemPrompt missing expected term: %q", term)
		}
	}
}
