package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mdemg/internal/config"
)

// JIMINY-RULES-UI-001 Epic 2 pin tests.
//
// The handler code paths that touch Neo4j / TSDB can't be exercised in
// unit tests without a live docker-compose stack (integration tier), so
// these pins verify the SHAPE of the request-validation + response
// contracts. Real end-to-end flow is exercised in Epic 5's live Tier-3.

func newRulesTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{cfg: config.Config{
		RSICWatchdogSpaceID:              "mdemg-dev",
		JiminyRulesListMaxLimit:          200,
		JiminyRulesOutcomesLookbackHours: 168,
	}}
}

// TestRulesList_MethodNotAllowed pins the shape: GET-only handler must
// reject POST/PUT/DELETE/PATCH. Regression guard against a copy-paste
// error that widens the method set.
func TestRulesList_MethodNotAllowed(t *testing.T) {
	s := newRulesTestServer(t)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := httptest.NewRecorder()
		s.handleRulesList(rec, httptest.NewRequest(m, "/v1/jiminy/rules", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s → %d, want 405", m, rec.Code)
		}
	}
}

// TestRulesDetail_MethodNotAllowed same-shape pin for the detail path.
func TestRulesDetail_MethodNotAllowed(t *testing.T) {
	s := newRulesTestServer(t)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := httptest.NewRecorder()
		s.handleRulesDetail(rec, httptest.NewRequest(m, "/v1/jiminy/rules/some-code", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s → %d, want 405", m, rec.Code)
		}
	}
}

// TestRulesList_NoDriverReturns503 pins the fail-fast shape when the
// Neo4j driver isn't wired. Handler must return a clean 503 with a
// named-service error, not panic or return 500.
func TestRulesList_NoDriverReturns503(t *testing.T) {
	s := newRulesTestServer(t)
	// driver is nil (test server); the RSICWatchdogSpaceID default is set
	// so we bypass the space_id required-field 400 and hit the driver
	// availability check.
	rec := httptest.NewRecorder()
	s.handleRulesList(rec, httptest.NewRequest(http.MethodGet, "/v1/jiminy/rules?space_id=mdemg-dev", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil driver → %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
}

// TestRulesDetail_NoDriverReturns503 same-shape pin for detail.
func TestRulesDetail_NoDriverReturns503(t *testing.T) {
	s := newRulesTestServer(t)
	rec := httptest.NewRecorder()
	s.handleRulesDetail(rec, httptest.NewRequest(http.MethodGet, "/v1/jiminy/rules/some-code?space_id=mdemg-dev", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil driver → %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
}

// TestRulesList_MissingSpaceIDReturns400 pins the required-field
// validator. The RSICWatchdogSpaceID fallback covers a shipped server;
// this test uses a zero-fallback config to prove the validator fires.
func TestRulesList_MissingSpaceIDReturns400(t *testing.T) {
	s := &Server{cfg: config.Config{}} // no RSICWatchdogSpaceID fallback
	rec := httptest.NewRecorder()
	s.handleRulesList(rec, httptest.NewRequest(http.MethodGet, "/v1/jiminy/rules", nil))
	// Note: 400 fires BEFORE the driver-nil check; if the driver check
	// fired first we'd see 503. Assert exact status.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing space_id → %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

// TestRulesDetail_MissingCodeReturns400 pins that a bare
// /v1/jiminy/rules/ (no code segment) returns 400 with a clear message.
func TestRulesDetail_MissingCodeReturns400(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/rules/", nil)
	s.handleRulesDetail(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty code path → %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

// TestRulesDetail_TrimsPathSuffix pins the path-parsing shape: any
// segment after {code}/ is stripped. Prevents a subtle bug where a
// future subpath (e.g. /v1/jiminy/rules/{code}/history) would leak into
// the code parse. Defensive today; matters when Epic 3 adds tombstone.
func TestRulesDetail_TrimsPathSuffix(t *testing.T) {
	s := newRulesTestServer(t)
	// With driver nil we reach 503 quickly, but the code-parse happens
	// BEFORE the driver check. Assert we don't get a 400 (code-not-empty
	// means the parse worked); 503 is expected downstream.
	rec := httptest.NewRecorder()
	s.handleRulesDetail(rec, httptest.NewRequest(http.MethodGet, "/v1/jiminy/rules/abc-def/some/subpath?space_id=mdemg-dev", nil))
	// Should be 503 (nil driver), NOT 400 (empty code)
	if rec.Code == http.StatusBadRequest {
		t.Errorf("path-parse should tolerate subpaths; got 400: %s", rec.Body.String())
	}
}
