package llmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// LLM-HEALTH-INVESTIGATION-001 E1 test — recorder tags caller-cancellation
// with the "caller_canceled: " prefix so RSIC alert rules can exclude them
// from the LLM error rate.

// captureRecorder captures the most recent InteractionRecord for assertion.
type captureRecorder struct {
	mu  sync.Mutex
	rec InteractionRecord
	got bool
}

func (c *captureRecorder) Record(_ context.Context, r InteractionRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rec = r
	c.got = true
}
func (c *captureRecorder) get() (InteractionRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rec, c.got
}

// TestRecorder_TagsCallerCancellation — the caller's ctx is canceled while
// the LLM POST is inflight. The recorder must write "caller_canceled: <raw>"
// in the Error field (not just the raw error).
func TestRecorder_TagsCallerCancellation(t *testing.T) {
	// LLM server that delays long enough for the caller's ctx to cancel.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "x"}}},
		})
	}))
	defer server.Close()

	cr := &captureRecorder{}
	c := New(Config{
		Provider: "openai", Model: "gpt-4o", APIKey: "k",
		BaseURL: server.URL, TimeoutMs: 60000, // client's own timeout is generous
	})
	c.SetRecorder(cr)

	// Caller-side deadline WELL under the server delay.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.Complete(ctx, []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected caller-cancellation error, got nil")
	}

	// Small sleep — the recorder is invoked synchronously in Complete's
	// call path AFTER the request returns.
	time.Sleep(50 * time.Millisecond)

	rec, ok := cr.get()
	if !ok {
		t.Fatal("recorder was not called")
	}
	if rec.Error == "" {
		t.Fatal("recorded Error is empty; expected caller_canceled prefix")
	}
	if !strings.HasPrefix(rec.Error, "caller_canceled:") {
		t.Errorf("Error = %q, want prefix 'caller_canceled:'", rec.Error)
	}
	// The raw error should be preserved after the prefix (either
	// 'context deadline exceeded' or 'context canceled' depending on the
	// exact cancellation path — both are valid; both must be prefixed).
	rawSuffix := strings.TrimPrefix(rec.Error, "caller_canceled: ")
	if rawSuffix == "" {
		t.Error("raw error text lost after the prefix")
	}
}

// TestRecorder_RealErrorNotTagged — a genuine LLM error (500-response) must
// land WITHOUT the caller_canceled prefix so the RSIC alert still fires on
// real health events.
func TestRecorder_RealErrorNotTagged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"upstream boom"}`))
	}))
	defer server.Close()

	cr := &captureRecorder{}
	c := New(Config{
		Provider: "openai", Model: "gpt-4o", APIKey: "k",
		BaseURL: server.URL, TimeoutMs: 5000,
	})
	c.SetRecorder(cr)

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err == nil {
		t.Fatal("expected server error, got nil")
	}
	time.Sleep(50 * time.Millisecond)
	rec, ok := cr.get()
	if !ok {
		t.Fatal("recorder was not called")
	}
	if strings.HasPrefix(rec.Error, "caller_canceled:") {
		t.Errorf("Error = %q, must NOT be prefixed caller_canceled: for a real server error", rec.Error)
	}
	if rec.Error == "" {
		t.Error("real error must be recorded (not empty)")
	}
}

// TestRecorder_NoErrorNotTagged — a successful call must record Error="".
func TestRecorder_NoErrorNotTagged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OpenAIChatResponse{
			Choices: []OpenAIChoice{{Message: Message{Content: "ok"}}},
		})
	}))
	defer server.Close()

	cr := &captureRecorder{}
	c := New(Config{Provider: "openai", Model: "gpt-4o", APIKey: "k", BaseURL: server.URL, TimeoutMs: 5000})
	c.SetRecorder(cr)

	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompleteOpts{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	rec, ok := cr.get()
	if !ok {
		t.Fatal("recorder was not called")
	}
	if rec.Error != "" {
		t.Errorf("Error = %q, want empty on successful call", rec.Error)
	}
}
