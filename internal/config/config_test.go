package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEndpoint_Priority1_MdemgEndpoint(t *testing.T) {
	// MDEMG_ENDPOINT takes highest priority
	t.Setenv("MDEMG_ENDPOINT", "http://custom:1234")
	t.Setenv("LISTEN_ADDR", ":5555")

	got := ResolveEndpoint("http://localhost:9999")
	want := "http://custom:1234"
	if got != want {
		t.Errorf("ResolveEndpoint() = %q, want %q", got, want)
	}
}

func TestResolveEndpoint_Priority2_PortFile(t *testing.T) {
	// Port file is priority 2 when MDEMG_ENDPOINT is not set
	t.Setenv("MDEMG_ENDPOINT", "")
	t.Setenv("LISTEN_ADDR", "")

	// Write a temp port file
	dir := t.TempDir()
	portFile := filepath.Join(dir, ".mdemg.port")
	if err := os.WriteFile(portFile, []byte("7777\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORT_FILE_PATH", portFile)

	got := ResolveEndpoint("http://localhost:9999")
	want := "http://localhost:7777"
	if got != want {
		t.Errorf("ResolveEndpoint() = %q, want %q", got, want)
	}
}

func TestResolveEndpoint_Priority3_ListenAddr(t *testing.T) {
	// LISTEN_ADDR is priority 3
	t.Setenv("MDEMG_ENDPOINT", "")
	t.Setenv("PORT_FILE_PATH", "/nonexistent/.mdemg.port")
	t.Setenv("LISTEN_ADDR", ":8082")

	got := ResolveEndpoint("http://localhost:9999")
	want := "http://localhost:8082"
	if got != want {
		t.Errorf("ResolveEndpoint() = %q, want %q", got, want)
	}
}

func TestResolveEndpoint_Priority3_ListenAddrWithHost(t *testing.T) {
	// LISTEN_ADDR with host prefix (no leading colon)
	t.Setenv("MDEMG_ENDPOINT", "")
	t.Setenv("PORT_FILE_PATH", "/nonexistent/.mdemg.port")
	t.Setenv("LISTEN_ADDR", "0.0.0.0:8082")

	got := ResolveEndpoint("http://localhost:9999")
	want := "http://0.0.0.0:8082"
	if got != want {
		t.Errorf("ResolveEndpoint() = %q, want %q", got, want)
	}
}

func TestResolveEndpoint_Priority4_Default(t *testing.T) {
	// Falls back to default when nothing else is set
	t.Setenv("MDEMG_ENDPOINT", "")
	t.Setenv("PORT_FILE_PATH", "/nonexistent/.mdemg.port")
	t.Setenv("LISTEN_ADDR", "")

	got := ResolveEndpoint("http://localhost:9999")
	want := "http://localhost:9999"
	if got != want {
		t.Errorf("ResolveEndpoint() = %q, want %q", got, want)
	}
}

func TestResolveEndpoint_PortFileEmptyContent(t *testing.T) {
	// Empty port file should fall through to next priority
	t.Setenv("MDEMG_ENDPOINT", "")
	t.Setenv("LISTEN_ADDR", "")

	dir := t.TempDir()
	portFile := filepath.Join(dir, ".mdemg.port")
	if err := os.WriteFile(portFile, []byte("  \n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORT_FILE_PATH", portFile)

	got := ResolveEndpoint("http://fallback:1111")
	want := "http://fallback:1111"
	if got != want {
		t.Errorf("ResolveEndpoint() with empty port file = %q, want %q", got, want)
	}
}

func TestResolveEndpoint_WhitespaceHandling(t *testing.T) {
	// Env vars with whitespace should be trimmed
	t.Setenv("MDEMG_ENDPOINT", "  http://trimmed:2222  ")

	got := ResolveEndpoint("http://localhost:9999")
	want := "http://trimmed:2222"
	if got != want {
		t.Errorf("ResolveEndpoint() = %q, want %q", got, want)
	}
}

// JIMINY-BUDGET-001: the direct Guide() budget derives from the warm-compute
// budget when unset — the old independent 15s default starved fresh installs.
func TestEffectiveJiminyTimeout_Derivation(t *testing.T) {
	c := Config{JiminyWarmComputeTimeoutMs: 90000}
	if got := c.EffectiveJiminyTimeout(); got.Milliseconds() != 90000 {
		t.Errorf("unset JiminyTimeoutMs should derive 90000, got %d", got.Milliseconds())
	}
	c.JiminyTimeoutMs = 20000
	if got := c.EffectiveJiminyTimeout(); got.Milliseconds() != 20000 {
		t.Errorf("explicit override lost: %d", got.Milliseconds())
	}
	r := Config{JiminyWarmComputeTimeoutMs: 90000}
	if got := r.EffectiveJiminyReformulateTimeout(); got.Milliseconds() != 90000 {
		t.Errorf("reformulate derivation: %d", got.Milliseconds())
	}
	r.JiminyReformulateTimeoutMs = 12000
	if got := r.EffectiveJiminyReformulateTimeout(); got.Milliseconds() != 12000 {
		t.Errorf("reformulate override lost: %d", got.Milliseconds())
	}
	// zero warm budget falls back to the package default (90s)
	z := Config{}
	if got := z.EffectiveJiminyTimeout(); got.Milliseconds() != int64(DefaultJiminyWarmComputeTimeoutMs) {
		t.Errorf("zero-config derivation should hit DefaultJiminyWarmComputeTimeoutMs, got %d", got.Milliseconds())
	}
}
