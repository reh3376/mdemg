//go:build darwin

package cli

import (
	"os"
	"path/filepath"
	"testing"

	launchd_templates "mdemg/internal/cli/launchd_templates"
)

// TestLaunchdServicesIncludesLlamaServer verifies the Phase 13.5 llama-server
// entry is in the slice and is required (Optional=false per the always-on
// LLM policy, set originally by Hotfix 11.6.3.1).
func TestLaunchdServicesIncludesLlamaServer(t *testing.T) {
	var found bool
	for _, svc := range launchdServices {
		if svc.Label == "com.mdemg.llama-server" {
			found = true
			if svc.Optional {
				t.Error("com.mdemg.llama-server entry must NOT be Optional (always-on LLM policy)")
			}
			if svc.Template != "com.mdemg.llama-server.plist" {
				t.Errorf("llama-server template=%q, want com.mdemg.llama-server.plist", svc.Template)
			}
		}
		if svc.Label == "com.mdemg.mlx-server" {
			t.Error("com.mdemg.mlx-server is decommissioned in Phase 13.5; Install() migrates legacy installs but should not register the service")
		}
	}
	if !found {
		t.Fatal("launchdServices slice missing com.mdemg.llama-server entry")
	}
}

// TestLlamaServerPlistEmbedded verifies the embedded FS picks up the new plist.
func TestLlamaServerPlistEmbedded(t *testing.T) {
	data, err := launchd_templates.FS.ReadFile("com.mdemg.llama-server.plist")
	if err != nil {
		t.Fatalf("embedded com.mdemg.llama-server.plist not found: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("embedded com.mdemg.llama-server.plist is empty")
	}
	body := string(data)
	for _, want := range []string{
		"<string>com.mdemg.llama-server</string>",
		"__LLAMA_SERVER_BIN__",
		"__MDEMG_MODEL_PATH__",
		"--ctx-size",
		"--jinja",
		"<key>KeepAlive</key>",
		"<key>SuccessfulExit</key>",
		"<key>ThrottleInterval</key>",
		"<integer>30</integer>",
	} {
		if !contains(body, want) {
			t.Errorf("llama-server plist missing %q", want)
		}
	}

	// The decommissioned mlx-server plist must not be re-embedded.
	if _, err := launchd_templates.FS.ReadFile("com.mdemg.mlx-server.plist"); err == nil {
		t.Error("decommissioned com.mdemg.mlx-server.plist must not be in embed.FS")
	}
}

// TestResolveLlamaServerBinUsesPrimaryEnv verifies MDEMG_LLAMA_SERVER_BIN
// takes precedence over PATH lookup.
func TestResolveLlamaServerBinUsesPrimaryEnv(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "fake-llama-server-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	t.Setenv("MDEMG_LLAMA_SERVER_BIN", tmp.Name())
	t.Setenv("MDEMG_MLX_LM_BIN", "")
	bin, ok := resolveLlamaServerBin()
	if !ok {
		t.Fatalf("expected primary env override to resolve %q", tmp.Name())
	}
	if bin != tmp.Name() {
		t.Errorf("resolveLlamaServerBin()=%q, want %q", bin, tmp.Name())
	}
}

// TestResolveLlamaServerBinFallsBackToMLXAlias verifies the deprecated
// MDEMG_MLX_LM_BIN env var still resolves (with a deprecation warning) when
// MDEMG_LLAMA_SERVER_BIN is unset. Phase 13.6 alias retention pattern.
func TestResolveLlamaServerBinFallsBackToMLXAlias(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "fake-llama-via-alias-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	t.Setenv("MDEMG_LLAMA_SERVER_BIN", "")
	t.Setenv("MDEMG_MLX_LM_BIN", tmp.Name())
	bin, ok := resolveLlamaServerBin()
	if !ok {
		t.Fatalf("expected MDEMG_MLX_LM_BIN alias to resolve %q", tmp.Name())
	}
	if bin != tmp.Name() {
		t.Errorf("resolveLlamaServerBin()=%q via alias, want %q", bin, tmp.Name())
	}
}

// TestResolveLlamaServerBinReturnsFalseWhenMissing verifies the false-path
// doesn't panic and the (bin, ok) tuple stays consistent.
func TestResolveLlamaServerBinReturnsFalseWhenMissing(t *testing.T) {
	t.Setenv("MDEMG_LLAMA_SERVER_BIN", "/nonexistent/path/to/llama-server")
	t.Setenv("MDEMG_MLX_LM_BIN", "/nonexistent/path/to/mlx_lm_server")
	bin, ok := resolveLlamaServerBin()
	// If host has llama-server on PATH, ok==true and we picked PATH.
	// If not, ok==false. Both are valid; just ensure tuple consistency.
	if ok && bin == "" {
		t.Error("resolver returned ok=true but empty bin")
	}
	if !ok && bin != "" {
		t.Error("resolver returned ok=false but non-empty bin")
	}
}

// TestResolveMDEMGModelPathDefaultsToProductionGGUF confirms the default
// model path is the Phase 13.5 GGUF file, not the MLX-style symlink dir.
func TestResolveMDEMGModelPathDefaultsToProductionGGUF(t *testing.T) {
	t.Setenv("MDEMG_MODEL_PATH", "")
	got := resolveMDEMGModelPath("/abc")
	want := filepath.Join("/abc", ".local-models", "mdemg-llm-v1-gguf", "mdemg-llm-v1.Q5_K_M.gguf")
	if got != want {
		t.Errorf("resolveMDEMGModelPath()=%q, want %q", got, want)
	}
}

// TestResolveMDEMGModelPathEnvOverride verifies MDEMG_MODEL_PATH takes
// precedence (operators are responsible for pointing it at a `.gguf` file
// llama-server can load).
func TestResolveMDEMGModelPathEnvOverride(t *testing.T) {
	t.Setenv("MDEMG_MODEL_PATH", "/custom/model.gguf")
	got := resolveMDEMGModelPath("/abc")
	if got != "/custom/model.gguf" {
		t.Errorf("resolveMDEMGModelPath()=%q, want /custom/model.gguf", got)
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
