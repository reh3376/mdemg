package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mdemg/internal/circuitbreaker"
)

func newTestServerWithBreakers(t *testing.T) *Server {
	t.Helper()
	reg := circuitbreaker.NewRegistry(circuitbreaker.Config{
		Enabled:          true,
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          5 * time.Second,
		MaxConcurrent:    1,
	})
	// Seed a few breakers so list/reset have something to act on.
	_ = reg.Get("openai-constraint-classify")
	_ = reg.Get("jiminy-synthesis")
	return &Server{cbRegistry: reg}
}

func TestBreakersList_HappyPath(t *testing.T) {
	s := newTestServerWithBreakers(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/breakers", nil)
	w := httptest.NewRecorder()
	s.handleBreakersList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp BreakersListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("count = %d, want 2", resp.Count)
	}
	names := map[string]bool{}
	for _, b := range resp.Breakers {
		names[b.Name] = true
		if b.State != "closed" {
			t.Errorf("breaker %q state = %q, want closed (fresh registry)", b.Name, b.State)
		}
	}
	for _, want := range []string{"openai-constraint-classify", "jiminy-synthesis"} {
		if !names[want] {
			t.Errorf("expected breaker %q in list, got %v", want, names)
		}
	}
}

func TestBreakersList_MethodNotAllowed(t *testing.T) {
	s := newTestServerWithBreakers(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/breakers", nil)
	w := httptest.NewRecorder()
	s.handleBreakersList(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestBreakersList_NoRegistry(t *testing.T) {
	s := &Server{cbRegistry: nil}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/breakers", nil)
	w := httptest.NewRecorder()
	s.handleBreakersList(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (empty response)", w.Code)
	}
	var resp BreakersListResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 0 {
		t.Errorf("count = %d, want 0", resp.Count)
	}
}

func TestBreakersReset_HappyPath(t *testing.T) {
	s := newTestServerWithBreakers(t)

	// Trip the breaker: FailureThreshold=2 → 2 failures opens it.
	b := s.cbRegistry.Get("openai-constraint-classify")
	fn := func(ctx context.Context) error { return errors.New("boom") }
	_ = b.Execute(context.Background(), fn)
	_ = b.Execute(context.Background(), fn)
	if b.State() != circuitbreaker.StateOpen {
		t.Fatalf("expected breaker to be open after 2 failures, got %s", b.State())
	}

	body, _ := json.Marshal(BreakerResetRequest{Name: "openai-constraint-classify"})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/breakers/reset", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleBreakersReset(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp BreakerResetResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("ok = false, want true")
	}
	if resp.WasState != "open" {
		t.Errorf("was_state = %q, want open", resp.WasState)
	}
	if resp.NowState != "closed" {
		t.Errorf("now_state = %q, want closed", resp.NowState)
	}
	if b.State() != circuitbreaker.StateClosed {
		t.Errorf("breaker state after reset = %s, want closed", b.State())
	}
}

func TestBreakersReset_UnknownBreaker(t *testing.T) {
	s := newTestServerWithBreakers(t)
	body, _ := json.Marshal(BreakerResetRequest{Name: "does-not-exist"})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/breakers/reset", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleBreakersReset(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	var body404 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body404)
	if _, ok := body404["available"]; !ok {
		t.Error("expected 'available' list in 404 body for discoverability")
	}
}

func TestBreakersReset_MissingName(t *testing.T) {
	s := newTestServerWithBreakers(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/breakers/reset",
		bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	s.handleBreakersReset(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestBreakersReset_BadJSON(t *testing.T) {
	s := newTestServerWithBreakers(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/breakers/reset",
		bytes.NewReader([]byte(`not-json`)))
	w := httptest.NewRecorder()
	s.handleBreakersReset(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestBreakersReset_MethodNotAllowed(t *testing.T) {
	s := newTestServerWithBreakers(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/breakers/reset", nil)
	w := httptest.NewRecorder()
	s.handleBreakersReset(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestBreakersReset_NoRegistry(t *testing.T) {
	s := &Server{cbRegistry: nil}
	body, _ := json.Marshal(BreakerResetRequest{Name: "x"})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/breakers/reset", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleBreakersReset(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}
