package api

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func resetMdemgBinCache() {
	mdemgBinOnce = sync.Once{}
	mdemgBinPath = ""
}

func TestResolveMdemgBin_EnvWins(t *testing.T) {
	resetMdemgBinCache()
	t.Cleanup(resetMdemgBinCache)

	fake := filepath.Join(t.TempDir(), "mdemg")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MDEMG_BIN", fake)
	if got := resolveMdemgBin(); got != fake {
		t.Fatalf("MDEMG_BIN override ignored: got %q want %q", got, fake)
	}
}

func TestResolveMdemgBin_MissingEnvFallsThrough(t *testing.T) {
	resetMdemgBinCache()
	t.Cleanup(resetMdemgBinCache)

	t.Setenv("MDEMG_BIN", "/nonexistent/mdemg-binary")
	got := resolveMdemgBin()
	if got == "/nonexistent/mdemg-binary" {
		t.Fatalf("nonexistent MDEMG_BIN must not be used")
	}
	// In tests os.Executable() is the test binary — still a valid absolute path,
	// proving the chain advances past the bad env value.
	if got == "" || got == "./bin/mdemg" && os.Getenv("CI") == "" {
		t.Logf("resolved %q (environment-dependent fallback)", got)
	}
}
