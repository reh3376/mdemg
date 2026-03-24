package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mdemg/internal/config"
)

func TestSynergyStatusHandler_ReturnsExpectedShape(t *testing.T) {
	// Minimal server with just config — no driver, no Jiminy
	s := &Server{
		cfg: config.Config{
			JiminyEnabled:            false,
			SynergyAssessmentEnabled: true,
			SynergyTargetClaudeLines: 150,
			SynergyTargetMemoryLines: 120,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/synergy/status?space_id=test", nil)
	w := httptest.NewRecorder()
	s.handleSynergyStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	// Verify key fields are present
	for _, key := range []string{"jiminy_healthy", "claude_md_lines", "memory_md_lines", "synergy_health", "migration_status"} {
		if !contains(body, key) {
			t.Errorf("response missing key %q: %s", key, body)
		}
	}
}

func TestSynergyStatusHandler_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}

	req := httptest.NewRequest(http.MethodPost, "/v1/synergy/status", nil)
	w := httptest.NewRecorder()
	s.handleSynergyStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestSynergyStatusHandler_MissingFiles(t *testing.T) {
	s := &Server{
		cfg: config.Config{
			SynergyClaudeMDPath:      "/nonexistent/CLAUDE.md",
			SynergyMemoryMDPath:      "/nonexistent/MEMORY.md",
			SynergyTargetClaudeLines: 150,
			SynergyTargetMemoryLines: 120,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/synergy/status?space_id=test", nil)
	w := httptest.NewRecorder()
	s.handleSynergyStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even with missing files, got %d", w.Code)
	}

	body := w.Body.String()
	// Lines should be 0 for missing files
	if !contains(body, `"claude_md_lines":0`) {
		t.Errorf("expected claude_md_lines:0 for missing file, got: %s", body)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
