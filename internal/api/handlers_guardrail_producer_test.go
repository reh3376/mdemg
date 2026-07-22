package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/guardrail"
)

// fakeValidator captures the context + request of each Validate call and can
// block until released, for exercising the semaphore drop path.
type fakeValidator struct {
	called    chan guardrail.ValidateRequest
	ctxErrs   chan error
	blockCh   chan struct{} // when non-nil, Validate blocks until closed
	sleepThen time.Duration // optional settle time before checking ctx
}

func (f *fakeValidator) Validate(ctx context.Context, req guardrail.ValidateRequest) (*guardrail.ValidateResponse, error) {
	if f.blockCh != nil {
		<-f.blockCh
	}
	if f.sleepThen > 0 {
		time.Sleep(f.sleepThen)
	}
	if f.ctxErrs != nil {
		f.ctxErrs <- ctx.Err()
	}
	if f.called != nil {
		f.called <- req
	}
	return &guardrail.ValidateResponse{Status: "Pass"}, nil
}

func postGuardrailAsync(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/memory/guardrail/validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleGuardrailValidate(w, req)
	return w
}

const asyncBody = `{"space_id":"test-space","files_changed":["a.go"],"diff":"+x := 1","async":true}`

func decodeStatus(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v (%s)", err, w.Body.String())
	}
	st, _ := resp["status"].(string)
	return st
}

func TestGuardrailProducer_DisabledGate(t *testing.T) {
	fake := &fakeValidator{called: make(chan guardrail.ValidateRequest, 1)}
	s := &Server{
		cfg:                  config.Config{GuardrailProducerEnabled: false},
		guardrailValidator:   fake,
		guardrailProducerSem: make(chan struct{}, 1),
	}

	w := postGuardrailAsync(t, s, asyncBody)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	if st := decodeStatus(t, w); st != "disabled" {
		t.Errorf("status field: got %q, want disabled", st)
	}
	select {
	case <-fake.called:
		t.Error("validator must NOT be called when producer disabled")
	case <-time.After(100 * time.Millisecond):
	}
}

// The detached evaluation must survive the HTTP request context ending —
// the producer's whole point (a hook curl exits in milliseconds).
func TestGuardrailProducer_QueuedAndDetached(t *testing.T) {
	fake := &fakeValidator{
		called:    make(chan guardrail.ValidateRequest, 1),
		ctxErrs:   make(chan error, 1),
		sleepThen: 50 * time.Millisecond, // let the request context fully end first
	}
	s := &Server{
		cfg: config.Config{
			GuardrailProducerEnabled: true,
			GuardrailTimeoutMs:       5000,
		},
		guardrailValidator:   fake,
		guardrailProducerSem: make(chan struct{}, 1),
	}

	// Give the request an already-canceled context — the strictest form of
	// "the client is gone" — and assert the detached ctx is unaffected.
	req := httptest.NewRequest(http.MethodPost, "/v1/memory/guardrail/validate", strings.NewReader(asyncBody))
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	s.handleGuardrailValidate(w, req)
	cancel() // client gone immediately after the 202

	if w.Code != http.StatusAccepted {
		t.Fatalf("status: got %d, want 202 (body: %s)", w.Code, w.Body.String())
	}
	if st := decodeStatus(t, w); st != "queued" {
		t.Errorf("status field: got %q, want queued", st)
	}

	select {
	case ctxErr := <-fake.ctxErrs:
		if ctxErr != nil {
			t.Errorf("detached evaluation context was canceled: %v", ctxErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("validator never called")
	}
	got := <-fake.called
	if got.SpaceID != "test-space" || len(got.FilesChanged) != 1 {
		t.Errorf("request not forwarded: %+v", got)
	}
}

func TestGuardrailProducer_DropWhenBusy(t *testing.T) {
	block := make(chan struct{})
	fake := &fakeValidator{blockCh: block}
	s := &Server{
		cfg: config.Config{
			GuardrailProducerEnabled: true,
			GuardrailTimeoutMs:       5000,
		},
		guardrailValidator:   fake,
		guardrailProducerSem: make(chan struct{}, 1),
	}

	w1 := postGuardrailAsync(t, s, asyncBody)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first call: got %d, want 202", w1.Code)
	}

	// Slot held by the blocked evaluation — second call must drop, not queue.
	w2 := postGuardrailAsync(t, s, asyncBody)
	if w2.Code != http.StatusOK {
		t.Fatalf("second call: got %d, want 200", w2.Code)
	}
	if st := decodeStatus(t, w2); st != "dropped" {
		t.Errorf("second call status: got %q, want dropped", st)
	}

	close(block) // release; slot frees via the deferred receive

	// After release the slot becomes available again (poll — the deferred
	// semaphore receive races with this assertion).
	deadline := time.Now().Add(2 * time.Second)
	for {
		w3 := postGuardrailAsync(t, s, asyncBody)
		if w3.Code == http.StatusAccepted {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("slot never freed after release: last status %d", w3.Code)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
