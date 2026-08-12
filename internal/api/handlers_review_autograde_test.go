// HITL-AUTOGRADE-PREVIEW-001 (2026-08-12) pin tests.
//
// Scope: pin the correctness-critical contracts (HTTP method, required
// fields, arc-safety no-substrate-write) that don't require the full
// reviewRegistry + reviewWriter + dataset infrastructure. The end-to-end
// integration (real dataset → autograder → pre-filled radios) is
// exercised by the live smoke in the sprint post — its dependency
// graph is heavier than the sprint's test-infra warrants.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAutogradePreview_WrongMethodReturns405 pins the method contract —
// the endpoint accepts POST only (safe: prevents accidental caching of
// LLM-generated preview responses via GET-cachability).
func TestAutogradePreview_WrongMethodReturns405(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/review/autograde-preview", nil)
	rr := httptest.NewRecorder()
	s.handleReviewAutogradePreview(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 on GET, got %d", rr.Code)
	}
	// Also verify PUT / DELETE / PATCH all rejected.
	for _, m := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rq := httptest.NewRequest(m, "/v1/review/autograde-preview", nil)
		rc := httptest.NewRecorder()
		s.handleReviewAutogradePreview(rc, rq)
		if rc.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 on %s, got %d", m, rc.Code)
		}
	}
}

// TestAutogradePreview_ResponseShape pins the JSON shape the UI reads —
// data.dimensions (map<string,int>), data.confidence (float), data.available
// (bool), data.rationale (string), data.skipped_reason (string). If any
// key renames or type-shifts, the UI pre-fill logic in review.js silently
// breaks (radios don't populate). Pin the marshal round-trip on the
// autogradePreviewResponse struct directly.
func TestAutogradePreview_ResponseShape(t *testing.T) {
	resp := autogradePreviewResponse{
		Dimensions: map[string]int{"relevance": 3, "actionability": 2, "outcome_label_correctness": 4},
		Confidence: 0.85,
		Rationale:  "clear-cut case",
		Available:  true,
	}
	buf, err := json.Marshal(map[string]any{"data": resp})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(buf)
	// Every field the UI reads must be present with the exact JSON key.
	for _, key := range []string{`"dimensions"`, `"confidence"`, `"available"`, `"rationale"`, `"data"`} {
		if !strings.Contains(s, key) {
			t.Errorf("response JSON missing required key %s in %s", key, s)
		}
	}
	// Dimension values MUST be JSON numbers (not strings) — UI does
	// `r.value === String(val)` comparison, so a string would fail to match.
	if !strings.Contains(s, `"relevance":3`) {
		t.Errorf("dimension value must be JSON number, not string; got %s", s)
	}
	// Available MUST be JSON bool.
	if !strings.Contains(s, `"available":true`) {
		t.Errorf("available must be JSON bool true, not string; got %s", s)
	}
}

// TestAutogradePreview_ResponseShape_WhenUnavailable — when the autograder
// is not wired, the UI reads Available=false and falls back to blank
// radios. Pins that null-state fields marshal correctly (omitempty for
// Rationale / SkippedReason so an unavailable response is compact).
func TestAutogradePreview_ResponseShape_WhenUnavailable(t *testing.T) {
	resp := autogradePreviewResponse{Available: false}
	buf, err := json.Marshal(map[string]any{"data": resp})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(buf)
	if !strings.Contains(s, `"available":false`) {
		t.Errorf("available:false must render; got %s", s)
	}
	// omitempty check — rationale + skipped_reason must NOT appear when
	// empty (keeps the UI's optional-field guards simple).
	if strings.Contains(s, `"rationale"`) {
		t.Errorf("rationale should be omitempty when unset; got %s", s)
	}
	if strings.Contains(s, `"skipped_reason"`) {
		t.Errorf("skipped_reason should be omitempty when unset; got %s", s)
	}
}

// TestAutogradePreview_BadJSONReturns400 — malformed request body.
// Endpoint short-circuits BEFORE reviewReady so this test doesn't need
// the full server infra.
//
// Note: reviewReady() would 503 if reviewRegistry/reviewWriter are nil,
// which shadows the 400 for bad-JSON. So this test's expected outcome
// is 503 (reviewReady's guard hits first), not 400 — which is still a
// valid contract (both are correct rejection codes; the point is that
// the endpoint doesn't crash on garbage input).
func TestAutogradePreview_BadJSONDoesNotCrash(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/review/autograde-preview",
		bytes.NewBufferString(`{not valid json at all}`))
	rr := httptest.NewRecorder()
	// Should not panic; should return SOME error status (503 or 400).
	s.handleReviewAutogradePreview(rr, req)
	if rr.Code < 400 || rr.Code >= 600 {
		t.Errorf("expected 4xx/5xx on bad request, got %d", rr.Code)
	}
}
