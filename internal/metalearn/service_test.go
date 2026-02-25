package metalearn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeneralizer_StripCodeFence(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"name":"test","description":"desc"}`, `{"name":"test","description":"desc"}`},
		{"```json\n{\"name\":\"test\"}\n```", `{"name":"test"}`},
		{"```\n{\"name\":\"test\"}\n```", `{"name":"test"}`},
		{"  \n```json\n{\"name\":\"test\"}\n```\n  ", `{"name":"test"}`},
	}
	for _, tt := range tests {
		got := stripCodeFence(tt.input)
		if got != tt.want {
			t.Errorf("stripCodeFence(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGeneralizer_Truncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	if got := truncate("a very long string here", 10); got != "a very lon..." {
		t.Errorf("truncate long = %q", got)
	}
}

func TestGeneralizer_Disabled(t *testing.T) {
	g := NewGeneralizer(GeneralizerConfig{Enabled: false}, nil)
	_, err := g.Generalize(t.Context(), "name", "summary", "desc")
	if err == nil {
		t.Fatal("expected error for disabled generalizer")
	}
}

func TestGeneralizer_OpenAI_Success(t *testing.T) {
	// Mock OpenAI server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"content": `{"name":"Dependency Injection Pattern","description":"A design pattern where dependencies are provided externally rather than created internally."}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := GeneralizerConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		MaxTokens: 500,
		TimeoutMs: 5000,
		OpenAIKey: "test-key",
		OpenAIURL: srv.URL,
	}
	g := NewGeneralizer(cfg, nil)

	result, err := g.Generalize(t.Context(), "Go DI in server.go", "Dependency injection via NewServer constructor", "Uses constructor injection pattern in internal/api/server.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Dependency Injection Pattern" {
		t.Errorf("name = %q, want %q", result.Name, "Dependency Injection Pattern")
	}
	if result.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestGeneralizer_OpenAI_EmptyName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"content": `{"name":"","description":"some desc"}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := GeneralizerConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		MaxTokens: 500,
		TimeoutMs: 5000,
		OpenAIKey: "test-key",
		OpenAIURL: srv.URL,
	}
	g := NewGeneralizer(cfg, nil)

	_, err := g.Generalize(t.Context(), "test", "test", "test")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestGeneralizer_Ollama_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"response": `{"name":"Event-Driven Architecture","description":"A pattern where components communicate through asynchronous events."}`,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := GeneralizerConfig{
		Enabled:   true,
		Provider:  "ollama",
		Model:     "llama3.2",
		MaxTokens: 500,
		TimeoutMs: 5000,
		OllamaURL: srv.URL,
	}
	g := NewGeneralizer(cfg, nil)

	result, err := g.Generalize(t.Context(), "APE Event Dispatch", "Event dispatching in plugin system", "internal/plugins/event_dispatcher.go sends events to subscribers")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Event-Driven Architecture" {
		t.Errorf("name = %q, want %q", result.Name, "Event-Driven Architecture")
	}
}

func TestGeneralizer_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"content": `not valid json at all`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := GeneralizerConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		MaxTokens: 500,
		TimeoutMs: 5000,
		OpenAIKey: "test-key",
		OpenAIURL: srv.URL,
	}
	g := NewGeneralizer(cfg, nil)

	_, err := g.Generalize(t.Context(), "test", "test", "test")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
