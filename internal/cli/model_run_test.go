package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mdemg/internal/config"
)

// Tier 1 unit tests for `mdemg model run` (follow-up #1 to Sprint
// MODEL-DIST-001). Validates the Configurability Contract for this
// command + the OpenAI-compat HTTP shape.

func TestBuildMessages_SystemFirst(t *testing.T) {
	msgs := buildMessages("you are helpful", nil, "hello")
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "you are helpful" {
		t.Errorf("msgs[0]=%+v, want system 'you are helpful'", msgs[0])
	}
	if msgs[1].Role != "user" || msgs[1].Content != "hello" {
		t.Errorf("msgs[1]=%+v, want user 'hello'", msgs[1])
	}
}

func TestBuildMessages_NoSystemSkipsIt(t *testing.T) {
	msgs := buildMessages("", nil, "hello")
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1 (system suppressed)", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("msgs[0].Role=%q, want user", msgs[0].Role)
	}
}

func TestBuildMessages_HistoryPreserved(t *testing.T) {
	history := []chatMessage{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
	}
	msgs := buildMessages("sys", history, "q2")
	want := []chatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
	}
	if len(msgs) != len(want) {
		t.Fatalf("got %d messages, want %d", len(msgs), len(want))
	}
	for i, m := range msgs {
		if m != want[i] {
			t.Errorf("msgs[%d]=%+v, want %+v", i, m, want[i])
		}
	}
}

func TestResolveRunConfig_FlagWinsOverCfg(t *testing.T) {
	cfg := config.Config{LLMEndpoint: "http://from-cfg/v1", LLMModel: "from-cfg"}
	got := resolveRunConfig(cfg, runOverrides{endpoint: "http://flag/v1", model: "flag-model"})
	if got.endpoint != "http://flag/v1" {
		t.Errorf("endpoint=%q, want flag override", got.endpoint)
	}
	if got.model != "flag-model" {
		t.Errorf("model=%q, want flag override", got.model)
	}
}

func TestResolveRunConfig_CfgFallback(t *testing.T) {
	cfg := config.Config{LLMEndpoint: "http://from-cfg/v1", LLMModel: "from-cfg"}
	got := resolveRunConfig(cfg, runOverrides{})
	// Note: EffectiveLLMEndpoint falls back to OpenAIEndpoint if LLMEndpoint
	// is empty. With LLMEndpoint set, it should return that.
	if got.endpoint != "http://from-cfg/v1" {
		t.Errorf("endpoint=%q, want cfg fallback http://from-cfg/v1", got.endpoint)
	}
	if got.model != "from-cfg" {
		t.Errorf("model=%q, want cfg fallback from-cfg", got.model)
	}
}

func TestResolveRunConfig_FinalFallback(t *testing.T) {
	cfg := config.Config{}
	got := resolveRunConfig(cfg, runOverrides{})
	if got.model != "mdemg-llm-v1" {
		t.Errorf("model=%q, want mdemg-llm-v1 (final fallback)", got.model)
	}
}

func TestCallChat_OK(t *testing.T) {
	var captured chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%q, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type=%q, want application/json", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]string{"role": "assistant", "content": "hello back"},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     5,
				"completion_tokens": 2,
				"total_tokens":      7,
			},
		})
	}))
	defer srv.Close()

	resp, err := callChat(context.Background(), srv.URL, chatRequest{
		Model:       "test-model",
		Messages:    []chatMessage{{Role: "user", Content: "hi"}},
		Temperature: 0.5,
		MaxTokens:   100,
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("callChat: %v", err)
	}
	if resp.Choices[0].Message.Content != "hello back" {
		t.Errorf("content=%q, want 'hello back'", resp.Choices[0].Message.Content)
	}
	if captured.Model != "test-model" {
		t.Errorf("captured model=%q, want test-model", captured.Model)
	}
	if captured.Temperature != 0.5 {
		t.Errorf("captured temperature=%v, want 0.5", captured.Temperature)
	}
}

func TestCallChat_EndpointError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request body", http.StatusBadRequest)
	}))
	defer srv.Close()
	_, err := callChat(context.Background(), srv.URL, chatRequest{Model: "x"}, 5*time.Second)
	if err == nil {
		t.Fatal("expected non-nil error for HTTP 400")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Errorf("error %q lacks status hint", err.Error())
	}
}

func TestCallChat_InlineErrorObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "no model loaded", "type": "engine"},
		})
	}))
	defer srv.Close()
	_, err := callChat(context.Background(), srv.URL, chatRequest{Model: "x"}, 5*time.Second)
	if err == nil {
		t.Fatal("expected error when endpoint returns {error: {...}}")
	}
	if !strings.Contains(err.Error(), "no model loaded") {
		t.Errorf("error %q should carry inline error message", err.Error())
	}
}

func TestCallChat_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	_, err := callChat(context.Background(), srv.URL, chatRequest{Model: "x"}, 5*time.Second)
	if err == nil {
		t.Fatal("expected error when endpoint returns no choices")
	}
}

func TestCallChat_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"too slow"}}]}`))
	}))
	defer srv.Close()
	_, err := callChat(context.Background(), srv.URL, chatRequest{Model: "x"}, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestCallChat_TrailingSlashEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()
	// Endpoint with trailing slash should still resolve to /chat/completions
	// (not //chat/completions).
	_, err := callChat(context.Background(), srv.URL+"/", chatRequest{Model: "x"}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path=%q, want /chat/completions (no double slash)", gotPath)
	}
}

func TestTruncateRunBody(t *testing.T) {
	if got := truncateRunBody("short", 100); got != "short" {
		t.Errorf("got %q, want short (unchanged)", got)
	}
	long := strings.Repeat("a", 600)
	got := truncateRunBody(long, 500)
	if !strings.HasSuffix(got, "...(truncated)") {
		t.Errorf("got %q, want truncation suffix", got[len(got)-30:])
	}
	if len(got) > 600 {
		t.Errorf("len(got)=%d, want <=600 (500 + suffix)", len(got))
	}
}
