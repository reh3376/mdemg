// ONBOARDING-TUTORIAL-SERVE-001 E2 — handler unit tests.
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTutorialTestServer builds a minimal Server wrapping just enough state
// to exercise tutorialHandler. We don't need a full New() — the handler
// closes over only its enabled-flag argument + the embedded FS.
func newTutorialTestServer() *Server {
	return &Server{}
}

func TestTutorialHandler_DisabledReturns503(t *testing.T) {
	s := newTutorialTestServer()
	h := http.StripPrefix("/ui/tutorial/", s.tutorialHandler(false))

	req := httptest.NewRequest(http.MethodGet, "/ui/tutorial/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when disabled, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "tutorial disabled") {
		t.Errorf("expected 503 body to name the reason, got: %s", body)
	}
	if !strings.Contains(body, "TUTORIAL_ENABLED") {
		t.Errorf("expected 503 body to point at the operator fix, got: %s", body)
	}
}

func TestTutorialHandler_EnabledServesIndex(t *testing.T) {
	s := newTutorialTestServer()
	h := http.StripPrefix("/ui/tutorial/", s.tutorialHandler(true))

	req := httptest.NewRequest(http.MethodGet, "/ui/tutorial/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on root, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Pivoted 2026-10-26 to a plain HTML/CSS long-form document: the deck is
	// now wrapped in <main class="deck">; this marker catches a regression
	// to the reveal.js shell as cleanly as the old `reveal` substring check.
	if !strings.Contains(body, `<main class="deck">`) {
		t.Errorf("expected <main class=\"deck\"> marker in response body, got first 200 chars: %q", body[:min(200, len(body))])
	}
}

func TestTutorialHandler_ServesAsset(t *testing.T) {
	s := newTutorialTestServer()
	h := http.StripPrefix("/ui/tutorial/", s.tutorialHandler(true))

	// Serve the slides-overrides.css asset at its new embedded path.
	req := httptest.NewRequest(http.MethodGet, "/ui/tutorial/assets/css/slides-overrides.css", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for slides-overrides.css, got %d", rec.Code)
	}
	body := rec.Body.String()
	// Post-pivot CSS targets `.deck` and `.svg-figure`, no `.reveal` selectors.
	// Catch the pivot regression by looking for a stable new-pipeline selector.
	if !strings.Contains(body, ".svg-figure") {
		t.Errorf("expected .svg-figure selector in CSS, got first 200 chars: %q", body[:min(200, len(body))])
	}
}

func TestTutorialHandler_RejectsDotDot(t *testing.T) {
	s := newTutorialTestServer()
	h := http.StripPrefix("/ui/tutorial/", s.tutorialHandler(true))

	// A manually-constructed path traversal attempt. http.FileServer rejects
	// many of these on its own via URL cleaning, but we defend-in-depth.
	for _, p := range []string{
		"/ui/tutorial/../../etc/passwd",
		"/ui/tutorial/..",
	} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("path traversal %q should NOT return 200; got %d", p, rec.Code)
		}
	}
}

func TestTutorialHandler_404OnMissingAsset(t *testing.T) {
	s := newTutorialTestServer()
	h := http.StripPrefix("/ui/tutorial/", s.tutorialHandler(true))

	req := httptest.NewRequest(http.MethodGet, "/ui/tutorial/does-not-exist.html", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 on missing asset, got %d", rec.Code)
	}
}

func TestContainsDotDot(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"/", false},
		{"foo.html", false},
		{"foo/bar.html", false},
		{"foo/..", true},
		{"../foo", true},
		{"foo/../bar", true},
		{"foo/./bar", false}, // single dot is not .. — different semantic
		{"a.b.c", false},
	}
	for _, c := range cases {
		got := containsDotDot(c.in)
		if got != c.want {
			t.Errorf("containsDotDot(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
