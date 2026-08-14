package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

// TestRulesList_MethodNotAllowed pins the method dispatch: after Epic 3,
// GET → list, POST → create (flag-gated); PUT/DELETE/PATCH still 405.
func TestRulesList_MethodNotAllowed(t *testing.T) {
	s := newRulesTestServer(t)
	for _, m := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := httptest.NewRecorder()
		s.handleRulesList(rec, httptest.NewRequest(m, "/v1/jiminy/rules", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s → %d, want 405", m, rec.Code)
		}
	}
}

// TestRulesDetail_MethodNotAllowed same-shape pin. After Epic 3 GET →
// detail; PUT/DELETE/PATCH still 405 on the {code} path (POST is only
// valid on the /tombstone subpath — that's tested separately).
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

// --- JIMINY-RULES-UI-001 Epic 3 pin tests ---

func newRulesTestServerWithWriteFlag(t *testing.T, writeEnabled bool) *Server {
	t.Helper()
	return &Server{cfg: config.Config{
		RSICWatchdogSpaceID:              "mdemg-dev",
		JiminyRulesListMaxLimit:          200,
		JiminyRulesOutcomesLookbackHours: 168,
		JiminyRulesUIWriteEnabled:        writeEnabled,
		JiminyRulesDedupSimThreshold:     0.75,
	}}
}

// TestRulesCreate_FlagOffReturns503 pins the arc-safety contract: with
// JiminyRulesUIWriteEnabled=false (the default during the JIMINY-CEILING-BREAK-2
// arc window), POST /v1/jiminy/rules MUST return 503 with a named-flag
// error. Regression here → an operator flipping the flag off would find
// their edits silently landing anyway.
func TestRulesCreate_FlagOffReturns503(t *testing.T) {
	s := newRulesTestServerWithWriteFlag(t, false)
	rec := httptest.NewRecorder()
	body := `{"space_id":"mdemg-dev","role_type":"constraint","constraint_type":"must","content":"test"}`
	s.handleRulesList(rec, httptest.NewRequest(http.MethodPost, "/v1/jiminy/rules", strings.NewReader(body)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST with flag off → %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "JIMINY_RULES_UI_WRITE_ENABLED") {
		t.Errorf("503 body should name the flag; got: %s", rec.Body.String())
	}
}

// TestRulesTombstone_FlagOffReturns503 same arc-safety pin for tombstone.
func TestRulesTombstone_FlagOffReturns503(t *testing.T) {
	s := newRulesTestServerWithWriteFlag(t, false)
	rec := httptest.NewRecorder()
	body := `{"space_id":"mdemg-dev","reason":"test"}`
	s.handleRulesDetail(rec, httptest.NewRequest(http.MethodPost, "/v1/jiminy/rules/some-code/tombstone", strings.NewReader(body)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST tombstone with flag off → %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "JIMINY_RULES_UI_WRITE_ENABLED") {
		t.Errorf("503 body should name the flag; got: %s", rec.Body.String())
	}
}

// TestRulesCreate_ValidatesRoleType pins the enum-guard: role_type must
// be one of {constraint, correction}. Regression would allow writing
// nodes with an arbitrary role_type value → substrate corruption.
func TestRulesCreate_ValidatesRoleType(t *testing.T) {
	s := newRulesTestServerWithWriteFlag(t, true)
	rec := httptest.NewRecorder()
	body := `{"space_id":"mdemg-dev","role_type":"bogus-type","constraint_type":"must","content":"test"}`
	s.handleRulesList(rec, httptest.NewRequest(http.MethodPost, "/v1/jiminy/rules", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus role_type → %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "role_type must be one of") {
		t.Errorf("400 body should name the allowed values; got: %s", rec.Body.String())
	}
}

// TestRulesCreate_ValidatesConstraintType same-shape enum pin.
func TestRulesCreate_ValidatesConstraintType(t *testing.T) {
	s := newRulesTestServerWithWriteFlag(t, true)
	rec := httptest.NewRecorder()
	body := `{"space_id":"mdemg-dev","role_type":"constraint","constraint_type":"maybe","content":"test"}`
	s.handleRulesList(rec, httptest.NewRequest(http.MethodPost, "/v1/jiminy/rules", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus constraint_type → %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "must, must_not, should, note") {
		t.Errorf("400 body should name the allowed values; got: %s", rec.Body.String())
	}
}

// TestRulesCreate_RejectsEmptyContent pins that content is required.
// Whitespace-only content is treated as empty (strings.TrimSpace).
func TestRulesCreate_RejectsEmptyContent(t *testing.T) {
	s := newRulesTestServerWithWriteFlag(t, true)
	for _, empty := range []string{"", "   ", "\n\t"} {
		rec := httptest.NewRecorder()
		body := `{"space_id":"mdemg-dev","role_type":"constraint","constraint_type":"must","content":"` + empty + `"}`
		s.handleRulesList(rec, httptest.NewRequest(http.MethodPost, "/v1/jiminy/rules", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("content=%q → %d, want 400 (%s)", empty, rec.Code, rec.Body.String())
		}
	}
}

// TestRulesTombstone_UnknownSubpath pins that a POST to an unknown
// subpath (e.g. /{code}/frobnicate) returns 404, NOT 405 (405 would
// imply the subpath exists but wrong method).
func TestRulesTombstone_UnknownSubpath(t *testing.T) {
	s := newRulesTestServerWithWriteFlag(t, true)
	rec := httptest.NewRecorder()
	s.handleRulesDetail(rec, httptest.NewRequest(http.MethodPost, "/v1/jiminy/rules/some-code/frobnicate", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown subpath → %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

// TestRulesTombstone_WrongMethodOnTombstonePath — /tombstone requires POST.
// GET /some-code/tombstone should return 405 (not 200-detail-of-tombstone).
func TestRulesTombstone_WrongMethodOnTombstonePath(t *testing.T) {
	s := newRulesTestServerWithWriteFlag(t, true)
	rec := httptest.NewRecorder()
	s.handleRulesDetail(rec, httptest.NewRequest(http.MethodGet, "/v1/jiminy/rules/some-code/tombstone", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET on /tombstone → %d, want 405 (%s)", rec.Code, rec.Body.String())
	}
}

// TestRulesCreate_EmbedCallSitesAttached pins EMBED-CALLSITE-002 (2026-08-14):
// doRulesCreate MUST wrap its embedder.Embed calls with
// embeddings.WithEmbeddingMeta(ctx, EmbeddingMeta{CallSite:"jiminy.rules.create", SpaceID:...})
// before invoking Embed. The recorder-wired embedder reads call_site + space_id
// from the context meta; a metaless embed records an empty call_site, which
// the RSIC self-reflect check #28 fires alert_embedding_regression on.
// Live-caught in this session: 6 empty-call_site rows from the shipped Save
// flow used by JIMINY-CORPUS-AUDIT-004's 7 content rewrites. Source-string
// pin — this is the shape assertion the recorder integration can't unit-test
// directly without a wired-up recorder + TSDB round-trip.
func TestRulesCreate_EmbedCallSitesAttached(t *testing.T) {
	b, err := readSourceFileAPI("handlers_jiminy_rules.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	src := string(b)
	start := strings.Index(src, "func (s *Server) doRulesCreate(")
	if start < 0 {
		t.Fatal("doRulesCreate not found in handlers_jiminy_rules.go")
	}
	// end of function — take a generous window; the two embed sites are near-top
	end := start + 4000
	if end > len(src) {
		end = len(src)
	}
	body := src[start:end]

	required := []string{
		`embeddings.WithEmbeddingMeta`,
		`CallSite: "jiminy.rules.create"`,
		`SpaceID:  req.SpaceID`,
	}
	for _, r := range required {
		if !strings.Contains(body, r) {
			t.Errorf("doRulesCreate missing required embed-meta wiring %q — EMBED-CALLSITE-002 regression", r)
		}
	}

	// Both Embed sites MUST use the meta-wrapped context, not r.Context() directly.
	nBareEmbed := strings.Count(body, "s.embedder.Embed(r.Context()")
	if nBareEmbed > 0 {
		t.Errorf("doRulesCreate has %d bare s.embedder.Embed(r.Context(), ...) calls — EMBED-CALLSITE-002 requires embedCtx", nBareEmbed)
	}
}

func readSourceFileAPI(name string) ([]byte, error) {
	return os.ReadFile(name)
}
