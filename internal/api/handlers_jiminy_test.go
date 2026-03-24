package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mdemg/internal/config"
)

// TestJiminyHealthz_MethodNotAllowed verifies POST returns 405.
func TestJiminyHealthz_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/healthz", nil)
	w := httptest.NewRecorder()
	s.handleJiminyHealthz(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"method not allowed"`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestJiminyHealthz_NilService verifies 503 when jiminySvc is nil.
func TestJiminyHealthz_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{JiminyEnabled: true}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/healthz", nil)
	w := httptest.NewRecorder()
	s.handleJiminyHealthz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, `"status":"disabled"`) {
		t.Errorf("expected disabled status: %s", body)
	}
	if !contains(body, `"enabled":false`) {
		t.Errorf("expected enabled:false: %s", body)
	}
}

// TestJiminyHealthz_Disabled verifies 503 when JiminyEnabled is false.
func TestJiminyHealthz_Disabled(t *testing.T) {
	s := &Server{cfg: config.Config{JiminyEnabled: false}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/healthz", nil)
	w := httptest.NewRecorder()
	s.handleJiminyHealthz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, `"status":"disabled"`) {
		t.Errorf("expected disabled status: %s", body)
	}
}

// TestJiminyReady_MethodNotAllowed verifies POST returns 405.
func TestJiminyReady_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/ready", nil)
	w := httptest.NewRecorder()
	s.handleJiminyReady(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJiminyReady_NilService verifies 503 when jiminySvc is nil.
func TestJiminyReady_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{JiminyEnabled: true}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/ready", nil)
	w := httptest.NewRecorder()
	s.handleJiminyReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, `"message":"jiminy guidance is not enabled (set JIMINY_ENABLED=true)"`) {
		t.Errorf("expected disabled message: %s", body)
	}
}

// TestJiminyGuide_MethodNotAllowed verifies GET returns 405.
func TestJiminyGuide_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/guide", nil)
	w := httptest.NewRecorder()
	s.handleJiminyGuide(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJiminyGuide_NilService verifies 503 when jiminySvc is nil.
func TestJiminyGuide_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/guide", nil)
	w := httptest.NewRecorder()
	s.handleJiminyGuide(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"jiminy guidance is not enabled`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestJiminyFeedback_MethodNotAllowed verifies GET returns 405.
func TestJiminyFeedback_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/feedback", nil)
	w := httptest.NewRecorder()
	s.handleJiminyFeedback(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJiminyFeedback_NilService verifies 503 when jiminySvc is nil.
func TestJiminyFeedback_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/feedback", nil)
	w := httptest.NewRecorder()
	s.handleJiminyFeedback(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"jiminy guidance is not enabled`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestJiminyEvaluate_MethodNotAllowed verifies GET returns 405.
func TestJiminyEvaluate_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/evaluate", nil)
	w := httptest.NewRecorder()
	s.handleJiminyEvaluate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJiminyEvaluate_NilService verifies 503 when jiminySvc is nil.
func TestJiminyEvaluate_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/evaluate", nil)
	w := httptest.NewRecorder()
	s.handleJiminyEvaluate(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"jiminy guidance is not enabled`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}
