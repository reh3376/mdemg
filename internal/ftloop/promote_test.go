package ftloop

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func servingFixture(t *testing.T, healthURL string) (ServingConfig, string, string) {
	t.Helper()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.gguf")
	b := filepath.Join(dir, "b.gguf")
	for _, f := range []string{a, b} {
		if err := os.WriteFile(f, []byte("gguf"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "current.gguf")
	if err := os.Symlink(a, link); err != nil {
		t.Fatal(err)
	}
	return ServingConfig{
		SymlinkPath:      link,
		HealthURL:        healthURL,
		HealthTimeout:    2 * time.Second,
		KickstartCommand: []string{"true"}, // no-op success
	}, a, b
}

func TestSwapServing_HappyPath(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer healthy.Close()
	cfg, a, b := servingFixture(t, healthy.URL)

	res, err := SwapServing(context.Background(), cfg, b)
	if err != nil {
		t.Fatalf("swap: %v", err)
	}
	if res.Previous != a || res.Target != b || res.Reverted {
		t.Errorf("result: %+v", res)
	}
	if cur, _ := CurrentServingTarget(cfg); cur != b {
		t.Errorf("symlink: got %s, want %s", cur, b)
	}
}

// Fail-closed: an unhealthy candidate must never keep serving — the symlink
// reverts to the previous target.
func TestSwapServing_UnhealthyReverts(t *testing.T) {
	calls := 0
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		// waitHealth probes every 3s, so a 1s timeout = exactly one probe per
		// wait. Probe 1 (candidate) is unhealthy; probe 2+ (revert
		// verification) healthy.
		if calls <= 1 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer flaky.Close()
	cfg, a, b := servingFixture(t, flaky.URL)
	cfg.HealthTimeout = 1 * time.Second

	res, err := SwapServing(context.Background(), cfg, b)
	if err == nil {
		t.Fatal("expected error for unhealthy candidate")
	}
	if !res.Reverted {
		t.Errorf("expected Reverted=true: %+v", res)
	}
	if cur, _ := CurrentServingTarget(cfg); cur != a {
		t.Errorf("symlink must be reverted to %s, got %s", a, cur)
	}
}

func TestSwapServing_TargetValidation(t *testing.T) {
	cfg, _, _ := servingFixture(t, "http://127.0.0.1:1/health")
	if _, err := SwapServing(context.Background(), cfg, "/nonexistent.gguf"); err == nil {
		t.Error("nonexistent target must error")
	}
	notGguf := filepath.Join(t.TempDir(), "model.bin")
	_ = os.WriteFile(notGguf, []byte("x"), 0o644)
	if _, err := SwapServing(context.Background(), cfg, notGguf); err == nil {
		t.Error("non-gguf target must error")
	}
}

func TestSwapServing_NoopWhenAlreadyActive(t *testing.T) {
	cfg, a, _ := servingFixture(t, "http://127.0.0.1:1/health")
	res, err := SwapServing(context.Background(), cfg, a)
	if err != nil {
		t.Fatalf("noop swap: %v", err)
	}
	if res.Previous != a || res.Reverted {
		t.Errorf("result: %+v", res)
	}
}

func TestSwapServing_KickstartFailureReverts(t *testing.T) {
	cfg, a, b := servingFixture(t, "http://127.0.0.1:1/health")
	cfg.KickstartCommand = []string{"false"} // always fails
	res, err := SwapServing(context.Background(), cfg, b)
	if err == nil {
		t.Fatal("expected kickstart failure")
	}
	if !res.Reverted {
		t.Errorf("expected revert: %+v", res)
	}
	if cur, _ := CurrentServingTarget(cfg); cur != a {
		t.Errorf("symlink must be reverted to %s, got %s", a, cur)
	}
}
