//go:build darwin

package cli

import (
	"os"
	"path/filepath"
	"testing"

	launchd_templates "mdemg/internal/cli/launchd_templates"
)

// TestLaunchdServicesIncludesMLXServer verifies the Phase 11.6.3 mlx-server
// entry was added to the slice and is marked Optional.
func TestLaunchdServicesIncludesMLXServer(t *testing.T) {
	var found bool
	for _, svc := range launchdServices {
		if svc.Label == "com.mdemg.mlx-server" {
			found = true
			if !svc.Optional {
				t.Error("com.mdemg.mlx-server entry must be Optional (skipped if mlx_lm not on PATH)")
			}
			if svc.Template != "com.mdemg.mlx-server.plist" {
				t.Errorf("mlx-server template=%q, want com.mdemg.mlx-server.plist", svc.Template)
			}
		}
	}
	if !found {
		t.Fatal("launchdServices slice missing com.mdemg.mlx-server entry")
	}
}

// TestMLXServerPlistEmbedded verifies the embedded FS picks up the new plist.
func TestMLXServerPlistEmbedded(t *testing.T) {
	data, err := launchd_templates.FS.ReadFile("com.mdemg.mlx-server.plist")
	if err != nil {
		t.Fatalf("embedded com.mdemg.mlx-server.plist not found: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("embedded com.mdemg.mlx-server.plist is empty")
	}
	// Spot-check a few critical keys are present in the template.
	body := string(data)
	for _, want := range []string{
		"<string>com.mdemg.mlx-server</string>",
		"__MLX_LM_BIN__",
		"__MDEMG_MODEL_PATH__",
		"<key>KeepAlive</key>",
		"<key>SuccessfulExit</key>",
		"<key>ThrottleInterval</key>",
		"<integer>60</integer>",
	} {
		if !contains(body, want) {
			t.Errorf("mlx-server plist missing %q", want)
		}
	}
}

// TestResolveMLXLMBinUsesEnvOverride verifies MDEMG_MLX_LM_BIN takes precedence.
func TestResolveMLXLMBinUsesEnvOverride(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "fake-mlx-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	t.Setenv("MDEMG_MLX_LM_BIN", tmp.Name())
	bin, ok := resolveMLXLMBin()
	if !ok {
		t.Fatalf("expected env override to resolve %q", tmp.Name())
	}
	if bin != tmp.Name() {
		t.Errorf("resolveMLXLMBin()=%q, want %q", bin, tmp.Name())
	}
}

// TestResolveMLXLMBinReturnsFalseWhenMissing verifies the false-path doesn't
// panic and signals correctly.
func TestResolveMLXLMBinReturnsFalseWhenMissing(t *testing.T) {
	// Point env at a definitely-missing path.
	t.Setenv("MDEMG_MLX_LM_BIN", "/nonexistent/path/to/mlx_lm_does_not_exist")
	bin, ok := resolveMLXLMBin()
	// If host has mlx_lm.server on PATH, ok==true and we picked PATH.
	// If not, ok==false. Both are valid; just ensure returned bin matches ok.
	if ok && bin == "" {
		t.Error("resolver returned ok=true but empty bin")
	}
	if !ok && bin != "" {
		t.Error("resolver returned ok=false but non-empty bin")
	}
}

// TestResolveMDEMGModelPathDefaultsToProjectDir confirms the default path is
// <projectDir>/.local-models/mdemg-llm-v1 when no env override is set.
func TestResolveMDEMGModelPathDefaultsToProjectDir(t *testing.T) {
	t.Setenv("MDEMG_MODEL_PATH", "")
	got := resolveMDEMGModelPath("/abc")
	want := filepath.Join("/abc", ".local-models", "mdemg-llm-v1")
	if got != want {
		t.Errorf("resolveMDEMGModelPath()=%q, want %q", got, want)
	}
}

// TestResolveMDEMGModelPathEnvOverride verifies MDEMG_MODEL_PATH takes precedence.
func TestResolveMDEMGModelPathEnvOverride(t *testing.T) {
	t.Setenv("MDEMG_MODEL_PATH", "/custom/model")
	got := resolveMDEMGModelPath("/abc")
	if got != "/custom/model" {
		t.Errorf("resolveMDEMGModelPath()=%q, want /custom/model", got)
	}
}

// contains is a tiny strings.Contains shim so we don't pull strings into
// every test (the package's test files import strings only when needed).
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
