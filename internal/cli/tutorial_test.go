// ONBOARDING-TUTORIAL-SERVE-001 E5 — tutorial CLI pin tests.
package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTutorialCmd_URLFlagPrintsResolvedURL(t *testing.T) {
	cmd := newTutorialCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--url"})

	// Explicitly set endpoint via env so the test is hermetic — the default
	// path reads from ResolveEndpoint() which inspects .mdemg.port + env.
	t.Setenv("MDEMG_ENDPOINT", "http://127.0.0.1:19999")

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("--url should not error, got: %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := "http://127.0.0.1:19999/ui/tutorial/"
	if got != want {
		t.Errorf("--url output = %q, want %q", got, want)
	}
}

func TestTutorialCmd_CheckReports200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ui/tutorial/" {
			t.Errorf("probe hit wrong path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	t.Setenv("MDEMG_ENDPOINT", srv.URL)

	cmd := newTutorialCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--check"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("--check against 200 should not error, got: %v", err)
	}
	if !strings.Contains(out.String(), "OK") {
		t.Errorf("expected 'OK' in output, got: %s", out.String())
	}
}

func TestTutorialCmd_CheckReports503WithEnableHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	t.Setenv("MDEMG_ENDPOINT", srv.URL)

	cmd := newTutorialCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--check"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatalf("--check against 503 should error, got nil")
	}
	if !strings.Contains(err.Error(), "TUTORIAL_ENABLED") {
		t.Errorf("expected TUTORIAL_ENABLED hint in error, got: %v", err)
	}
}

func TestTutorialCmd_CheckReportsUnreachable(t *testing.T) {
	// Point at a port with no listener; connection refused → clear error.
	t.Setenv("MDEMG_ENDPOINT", "http://127.0.0.1:1") // reserved, unreachable

	cmd := newTutorialCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--check"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("--check against unreachable endpoint should error, got nil")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("expected 'unreachable' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "mdemg start") {
		t.Errorf("expected 'mdemg start' startup hint in error, got: %v", err)
	}
}

// Regression pin for the tutorialPath constant — the handler mount point
// is wired at `/ui/tutorial/` in server.go; if this const diverges the
// CLI would open the wrong URL and operators would 404.
func TestTutorialPath_MatchesHandlerMount(t *testing.T) {
	if tutorialPath != "/ui/tutorial/" {
		t.Errorf("tutorialPath = %q, must match server.go handler mount %q",
			tutorialPath, "/ui/tutorial/")
	}
}
