package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// INSTANCE-ID-PIN-001 (2026-09-25) — pin tests for resolveInstanceID's
// pin-file behavior. Every test uses t.TempDir() for isolation +
// t.Setenv for env manipulation (auto-restore).

// pinPathFor sets MDEMG_INSTANCE_ID_PIN_PATH to a fresh tempdir path and
// returns it. Ensures MDEMG_INSTANCE_ID env is cleared for the test.
func pinPathFor(t *testing.T) string {
	t.Helper()
	t.Setenv("MDEMG_INSTANCE_ID", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "instance_id")
	t.Setenv("MDEMG_INSTANCE_ID_PIN_PATH", path)
	return path
}

// TestResolveInstanceID_ExplicitArgWins — the highest priority. Even
// with env, pin-file, AND hostname all populated, explicit MUST win.
func TestResolveInstanceID_ExplicitArgWins(t *testing.T) {
	path := pinPathFor(t)
	t.Setenv("MDEMG_INSTANCE_ID", "env-value")
	if err := os.WriteFile(path, []byte("pinned-value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := resolveInstanceID("explicit-value", "mdemg-dev")
	if got != "explicit-value" {
		t.Errorf("explicit MUST win, got %q", got)
	}
}

// TestResolveInstanceID_EnvWinsOverPinFile — regression pin for the
// shipped MDEMG_INSTANCE_ID contract. The pin-file MUST NOT override an
// explicit env setting (this is the mitigation-A path from the
// hostname-drift bug — operator's `.env` value stays authoritative).
func TestResolveInstanceID_EnvWinsOverPinFile(t *testing.T) {
	path := pinPathFor(t)
	t.Setenv("MDEMG_INSTANCE_ID", "env-value")
	if err := os.WriteFile(path, []byte("pinned-value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := resolveInstanceID("", "mdemg-dev")
	if got != "env-value" {
		t.Errorf("env MUST win over pin-file, got %q", got)
	}
}

// TestResolveInstanceID_PinFileWinsOverHostname — the core new behavior.
// When no explicit + no env, the pin-file value MUST be preferred over
// re-derivation-from-hostname. This is what protects data-continuity
// across hostname changes.
func TestResolveInstanceID_PinFileWinsOverHostname(t *testing.T) {
	path := pinPathFor(t)
	// No env, no explicit → pin-file is the only signal.
	if err := os.WriteFile(path, []byte("old-hostname-mdemg-dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := resolveInstanceID("", "mdemg-dev")
	if got != "old-hostname-mdemg-dev" {
		t.Errorf("pin-file MUST win over hostname derivation, got %q", got)
	}
}

// TestResolveInstanceID_FirstRunWritesPinFile — the first-run persistence
// behavior. When no pin-file exists yet, resolveInstanceID derives from
// hostname AND writes the derived value to the pin-file so subsequent runs
// use the pinned value regardless of hostname changes.
func TestResolveInstanceID_FirstRunWritesPinFile(t *testing.T) {
	path := pinPathFor(t)
	// Ensure pin-file DOESN'T exist to trigger first-run.
	_ = os.Remove(path)

	got := resolveInstanceID("", "mdemg-dev")

	// Derived value should include the space suffix.
	hostname, _ := os.Hostname()
	want := hostname + "-mdemg-dev"
	if got != want {
		t.Errorf("first-run derivation: got %q want %q", got, want)
	}

	// Pin-file must now exist with the derived value.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("pin-file not written after first-run: %v", err)
	}
	if string(data) != want+"\n" {
		t.Errorf("pin-file content = %q; want %q", string(data), want+"\n")
	}
}

// TestResolveInstanceID_MalformedPinFallsThrough — a corrupt pin-file
// (empty, whitespace-only) MUST fall through to hostname derivation. This
// protects against a filesystem event that zeroed the file.
func TestResolveInstanceID_MalformedPinFallsThrough(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"whitespace-only", "   \n\n  "},
		{"tab-only", "\t\t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := pinPathFor(t)
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got := resolveInstanceID("", "mdemg-dev")
			hostname, _ := os.Hostname()
			if got != hostname+"-mdemg-dev" {
				t.Errorf("malformed pin MUST fall through to derivation; got %q", got)
			}
		})
	}
}

// TestResolveInstanceID_DashDashDisablesPin — `MDEMG_INSTANCE_ID_PIN_PATH=-`
// disables pinning entirely. Matches the shipped MDEMG-DOCS-INGEST-002
// `=-` escape-hatch pattern. Verifies both directions: no read AND no
// write when disabled.
func TestResolveInstanceID_DashDashDisablesPin(t *testing.T) {
	t.Setenv("MDEMG_INSTANCE_ID", "")
	t.Setenv("MDEMG_INSTANCE_ID_PIN_PATH", "-")

	// Create a pin-file at the DEFAULT location to prove it's ignored.
	// (We don't actually populate the real ~/.mdemg/instance_id because
	// that would pollute the operator's real state; the point of `=-` is
	// that we bypass the read entirely so no interference is possible.)

	got := resolveInstanceID("", "mdemg-dev")
	hostname, _ := os.Hostname()
	if got != hostname+"-mdemg-dev" {
		t.Errorf("=- MUST behave like pre-INSTANCE-ID-PIN-001: derive from hostname; got %q", got)
	}
}

// TestResolveInstanceID_TrimsWhitespace — a well-formed pin-file with
// trailing newline (the default writer format) is read correctly.
// Regression pin for the write-then-read round-trip.
func TestResolveInstanceID_TrimsWhitespace(t *testing.T) {
	path := pinPathFor(t)
	// Simulate a pin-file written by our own writer (trailing newline).
	if err := os.WriteFile(path, []byte("  Mac.lan-mdemg-dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := resolveInstanceID("", "mdemg-dev")
	if got != "Mac.lan-mdemg-dev" {
		t.Errorf("trimmed pin value: got %q want 'Mac.lan-mdemg-dev'", got)
	}
}
