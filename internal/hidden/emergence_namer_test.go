package hidden

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmergenceNamer_Disabled(t *testing.T) {
	namer := NewEmergenceNamer(EmergenceNamerConfig{Enabled: false}, nil)
	_, err := namer.Name(context.Background(), []ClusterNodeSummary{{NodeID: "a"}})
	if err == nil {
		t.Fatal("expected error when namer is disabled")
	}
}

func TestEmergenceNamer_EmptyCluster(t *testing.T) {
	namer := NewEmergenceNamer(EmergenceNamerConfig{Enabled: true}, nil)
	_, err := namer.Name(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for empty cluster")
	}
}

func TestEmergenceNamer_OpenAISuccess(t *testing.T) {
	result := EmergenceNamingResult{
		Name:          "Error Recovery Pipeline",
		Description:   "Nodes related to error handling and retry logic",
		ProposedLabel: "pattern",
	}
	respBody, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]string{"content": mustJSON(t, result)}},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respBody)
	}))
	defer srv.Close()

	namer := NewEmergenceNamer(EmergenceNamerConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		MaxTokens: 500,
		TimeoutMs: 5000,
		OpenAIKey: "test-key",
		OpenAIURL: srv.URL,
	}, nil)

	nodes := []ClusterNodeSummary{
		{NodeID: "n1", Name: "retry.go", Path: "internal/retry.go", Summary: "Retry logic"},
		{NodeID: "n2", Name: "errors.go", Path: "internal/errors.go", Summary: "Error types"},
		{NodeID: "n3", Name: "backoff.go", Path: "internal/backoff.go", Summary: "Backoff strategy"},
	}

	got, err := namer.Name(context.Background(), nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Error Recovery Pipeline" {
		t.Errorf("name = %q, want %q", got.Name, "Error Recovery Pipeline")
	}
	if got.ProposedLabel != "pattern" {
		t.Errorf("label = %q, want %q", got.ProposedLabel, "pattern")
	}
}

func TestEmergenceNamer_OpenAIFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	namer := NewEmergenceNamer(EmergenceNamerConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		TimeoutMs: 2000,
		OpenAIKey: "test-key",
		OpenAIURL: srv.URL,
	}, nil)

	_, err := namer.Name(context.Background(), []ClusterNodeSummary{{NodeID: "n1", Name: "a"}})
	if err == nil {
		t.Fatal("expected error from failed OpenAI call")
	}
}

func TestEmergenceNamer_OllamaSuccess(t *testing.T) {
	result := EmergenceNamingResult{
		Name:          "Data Pipeline Orchestration",
		Description:   "Nodes form a data processing pipeline",
		ProposedLabel: "workflow",
	}
	respBody, _ := json.Marshal(map[string]any{
		"response": mustJSON(t, result),
	})

	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = readBody(r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(respBody)
	}))
	defer srv.Close()

	namer := NewEmergenceNamer(EmergenceNamerConfig{
		Enabled:   true,
		Provider:  "ollama",
		Model:     "llama3.2:latest",
		TimeoutMs: 5000,
		OllamaURL: srv.URL,
	}, nil)

	nodes := []ClusterNodeSummary{
		{NodeID: "n1", Name: "pipeline.go", Summary: "Pipeline runner"},
		{NodeID: "n2", Name: "transform.go", Summary: "Data transforms"},
		{NodeID: "n3", Name: "sink.go", Summary: "Output sink"},
	}

	got, err := namer.Name(context.Background(), nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Data Pipeline Orchestration" {
		t.Errorf("name = %q, want %q", got.Name, "Data Pipeline Orchestration")
	}
	if got.ProposedLabel != "workflow" {
		t.Errorf("label = %q, want %q", got.ProposedLabel, "workflow")
	}

	// Verify format field (JSON schema) is present in the request
	var reqMap map[string]any
	if err := json.Unmarshal(receivedBody, &reqMap); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	formatVal, ok := reqMap["format"]
	if !ok {
		t.Fatal("expected 'format' field in Ollama request body")
	}
	formatMap, ok := formatVal.(map[string]any)
	if !ok {
		t.Fatalf("expected 'format' to be a JSON object, got %T", formatVal)
	}
	if formatMap["type"] != "object" {
		t.Errorf("format.type = %v, want 'object'", formatMap["type"])
	}
	props, ok := formatMap["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected format.properties to be an object")
	}
	for _, field := range []string{"name", "description", "proposed_label"} {
		if _, exists := props[field]; !exists {
			t.Errorf("format.properties missing field %q", field)
		}
	}

	// Verify options.temperature is set
	opts, ok := reqMap["options"].(map[string]any)
	if !ok {
		t.Fatal("expected 'options' field in Ollama request body")
	}
	if temp, ok := opts["temperature"].(float64); !ok || temp != 0.3 {
		t.Errorf("options.temperature = %v, want 0.3", opts["temperature"])
	}
}

func TestEmergenceNamer_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "not valid json at all"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	namer := NewEmergenceNamer(EmergenceNamerConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		TimeoutMs: 2000,
		OpenAIKey: "test-key",
		OpenAIURL: srv.URL,
	}, nil)

	_, err := namer.Name(context.Background(), []ClusterNodeSummary{{NodeID: "n1", Name: "a"}})
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestEmergenceNamer_MarkdownCodeFenceStripping(t *testing.T) {
	wrapped := "```json\n" + `{"name":"Test Concept","description":"test","proposed_label":"pattern"}` + "\n```"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": wrapped}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	namer := NewEmergenceNamer(EmergenceNamerConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		TimeoutMs: 2000,
		OpenAIKey: "test-key",
		OpenAIURL: srv.URL,
	}, nil)

	got, err := namer.Name(context.Background(), []ClusterNodeSummary{{NodeID: "n1", Name: "a"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Test Concept" {
		t.Errorf("name = %q, want %q", got.Name, "Test Concept")
	}
}

func TestEmergenceNamer_InvalidLabel(t *testing.T) {
	result := `{"name":"Test","description":"test","proposed_label":"invalid_label"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": result}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	namer := NewEmergenceNamer(EmergenceNamerConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		TimeoutMs: 2000,
		OpenAIKey: "test-key",
		OpenAIURL: srv.URL,
	}, nil)

	_, err := namer.Name(context.Background(), []ClusterNodeSummary{{NodeID: "n1", Name: "a"}})
	if err == nil {
		t.Fatal("expected error for invalid proposed_label")
	}
}

func TestEmergenceNamer_PromptContainsClusterMembers(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = readBody(r)
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"name":"X","description":"x","proposed_label":"pattern"}`}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	namer := NewEmergenceNamer(EmergenceNamerConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		TimeoutMs: 2000,
		OpenAIKey: "test-key",
		OpenAIURL: srv.URL,
	}, nil)

	nodes := []ClusterNodeSummary{
		{NodeID: "n1", Name: "auth.go", Path: "internal/auth.go", Summary: "Auth handler"},
		{NodeID: "n2", Name: "token.go", Path: "internal/token.go", Summary: "Token parsing"},
	}

	_, err := namer.Name(context.Background(), nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the prompt contains node info
	bodyStr := string(receivedBody)
	if !containsStr(bodyStr, "auth.go") {
		t.Error("prompt should contain node name 'auth.go'")
	}
	if !containsStr(bodyStr, "Token parsing") {
		t.Error("prompt should contain node summary 'Token parsing'")
	}
}

// --- helpers ---

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return string(b)
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func containsStr(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && // avoid false positives
		jsonContains(s, sub)
}

func jsonContains(s, sub string) bool {
	// Simple substring check — the request body is JSON-encoded so node names appear as strings
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
