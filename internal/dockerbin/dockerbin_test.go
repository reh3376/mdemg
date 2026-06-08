package dockerbin

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// resolve() is the uncached lookup — test it directly so the package-level
// sync.Once cache doesn't bleed across cases.

func TestResolve_EnvOverrideWins(t *testing.T) {
	t.Setenv(EnvOverride, "/custom/path/to/docker")
	if got := resolve(); got != "/custom/path/to/docker" {
		t.Fatalf("env override should win, got %q", got)
	}
}

func TestResolve_WellKnownWhenNotOnPath(t *testing.T) {
	// Force the override empty and PATH empty so LookPath fails, then make a
	// fake docker at a well-known location via a temp shim is impossible
	// (paths are absolute system locations). Instead assert the contract: when
	// override is empty and nothing is found, resolve() returns "".
	t.Setenv(EnvOverride, "")
	t.Setenv("PATH", "") // LookPath will fail
	got := resolve()
	// On a dev/CI box docker may genuinely exist at a well-known path; accept
	// either "" (none) or one of the documented well-known locations.
	if got != "" && !slices.Contains(wellKnownPaths, got) {
		t.Fatalf("with empty PATH, resolve() must return \"\" or a well-known path, got %q", got)
	}
}

func TestResolve_FindsExecutableOnPath(t *testing.T) {
	// Create a fake `docker` executable in a temp dir and put it on PATH.
	dir := t.TempDir()
	fake := filepath.Join(dir, "docker")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvOverride, "")
	t.Setenv("PATH", dir)
	if got := resolve(); got != fake {
		t.Fatalf("resolve() should find docker on PATH at %q, got %q", fake, got)
	}
}

func TestPath_FallsBackToBareDocker(t *testing.T) {
	// Path() must never return empty — when nothing resolves it returns the
	// historical bare "docker" so the caller's existing error handling fires.
	// We can't reset the package cache, so just assert Path() is non-empty and
	// is either an absolute path or "docker".
	p := Path()
	if p == "" {
		t.Fatal("Path() must never return empty")
	}
	if p != "docker" && !filepath.IsAbs(p) {
		t.Fatalf("Path() should be \"docker\" or an absolute path, got %q", p)
	}
}
