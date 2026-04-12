package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mdemg/internal/config"
)

// TestJ17ProtocolMetrics_MethodNotAllowed verifies PUT returns 405 (GET and POST are allowed).
func TestJ17ProtocolMetrics_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPut, "/v1/jiminy/protocol/metrics", nil)
	w := httptest.NewRecorder()
	s.handleJ17ProtocolMetrics(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJ17ProtocolMetrics_ResetDisabled verifies POST returns 503 when J17 is disabled.
func TestJ17ProtocolMetrics_ResetDisabled(t *testing.T) {
	s := &Server{cfg: config.Config{J17Enabled: false}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/protocol/metrics", nil)
	w := httptest.NewRecorder()
	s.handleJ17ProtocolMetrics(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestJ17ProtocolMetrics_Disabled verifies 503 when J17 is disabled.
func TestJ17ProtocolMetrics_Disabled(t *testing.T) {
	s := &Server{cfg: config.Config{J17Enabled: false}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/protocol/metrics", nil)
	w := httptest.NewRecorder()
	s.handleJ17ProtocolMetrics(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"J17 protocol not enabled"`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestJ17TierEffectiveness_MethodNotAllowed verifies POST returns 405.
func TestJ17TierEffectiveness_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/protocol/tier-effectiveness", nil)
	w := httptest.NewRecorder()
	s.handleJ17TierEffectiveness(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJ17TierEffectiveness_NilService verifies 503 when jiminySvc is nil.
func TestJ17TierEffectiveness_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{J17Enabled: true}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/protocol/tier-effectiveness", nil)
	w := httptest.NewRecorder()
	s.handleJ17TierEffectiveness(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestJ17Extension_MethodNotAllowed verifies GET returns 405.
func TestJ17Extension_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/extension", nil)
	w := httptest.NewRecorder()
	s.handleJ17Extension(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJ17Extension_Disabled verifies 503 when J17 is disabled.
func TestJ17Extension_Disabled(t *testing.T) {
	s := &Server{cfg: config.Config{J17Enabled: false}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/extension", nil)
	w := httptest.NewRecorder()
	s.handleJ17Extension(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestJ17ProtocolFeedback_MethodNotAllowed verifies GET returns 405.
func TestJ17ProtocolFeedback_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/protocol/feedback", nil)
	w := httptest.NewRecorder()
	s.handleJ17ProtocolFeedback(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJ17ProtocolFeedback_Disabled verifies 503 when J17 is disabled.
func TestJ17ProtocolFeedback_Disabled(t *testing.T) {
	s := &Server{cfg: config.Config{J17Enabled: false}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/protocol/feedback", nil)
	w := httptest.NewRecorder()
	s.handleJ17ProtocolFeedback(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestJ17ProtocolLearn_MethodNotAllowed verifies GET returns 405.
func TestJ17ProtocolLearn_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/protocol/learn", nil)
	w := httptest.NewRecorder()
	s.handleJ17ProtocolLearn(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJ17ProtocolLearn_Disabled verifies 503 when J17 is disabled.
func TestJ17ProtocolLearn_Disabled(t *testing.T) {
	s := &Server{cfg: config.Config{J17Enabled: false}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/protocol/learn", nil)
	w := httptest.NewRecorder()
	s.handleJ17ProtocolLearn(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestJ17Bootstrap_MethodNotAllowed verifies POST returns 405.
func TestJ17Bootstrap_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/bootstrap", nil)
	w := httptest.NewRecorder()
	s.handleJ17Bootstrap(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJ17Bootstrap_Disabled verifies 503 when J17 is disabled.
func TestJ17Bootstrap_Disabled(t *testing.T) {
	s := &Server{cfg: config.Config{J17Enabled: false}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/bootstrap", nil)
	w := httptest.NewRecorder()
	s.handleJ17Bootstrap(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestJ17Checkpoint_MethodNotAllowed verifies GET returns 405.
func TestJ17Checkpoint_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/checkpoint", nil)
	w := httptest.NewRecorder()
	s.handleJ17Checkpoint(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJ17Checkpoint_Disabled verifies 503 when J17 is disabled.
func TestJ17Checkpoint_Disabled(t *testing.T) {
	s := &Server{cfg: config.Config{J17Enabled: false}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/checkpoint", nil)
	w := httptest.NewRecorder()
	s.handleJ17Checkpoint(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestJ17ResumeProtocol_MethodNotAllowed verifies GET returns 405.
func TestJ17ResumeProtocol_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/resume-protocol", nil)
	w := httptest.NewRecorder()
	s.handleJ17ResumeProtocol(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJ17ResumeProtocol_Disabled verifies 503 when J17 is disabled.
func TestJ17ResumeProtocol_Disabled(t *testing.T) {
	s := &Server{cfg: config.Config{J17Enabled: false}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/resume-protocol", nil)
	w := httptest.NewRecorder()
	s.handleJ17ResumeProtocol(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
