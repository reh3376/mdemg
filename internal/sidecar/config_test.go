package sidecar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	mdemg := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemg, 0755); err != nil {
		t.Fatal(err)
	}
	content := `version: "1"
profile: local
runtime:
  endpoint: "http://localhost:9999"
  remote:
    host: ""
    transport: docker-context
adapters:
  - name: claude-code
    enabled: true
hooks:
  space_id_strategy: repo-basename
install:
  auto_fix: true
`
	path := filepath.Join(mdemg, "sidecar.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Version != "1" {
		t.Errorf("version = %q, want %q", cfg.Version, "1")
	}
	if cfg.Profile != ProfileLocal {
		t.Errorf("profile = %q, want %q", cfg.Profile, ProfileLocal)
	}
	if cfg.Runtime.Endpoint != "http://localhost:9999" {
		t.Errorf("endpoint = %q, want %q", cfg.Runtime.Endpoint, "http://localhost:9999")
	}
	if len(cfg.Adapters) != 1 || cfg.Adapters[0].Name != AdapterClaudeCode {
		t.Errorf("adapters = %+v, want [claude-code]", cfg.Adapters)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/sidecar.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	cfg := GenerateConfig(ProfileLocal, []AdapterName{AdapterClaudeCode}, "", "")
	errs := ValidateConfig(cfg)
	if HasErrors(errs) {
		t.Errorf("unexpected errors: %+v", errs)
	}
}

func TestValidateConfig_BadVersion(t *testing.T) {
	cfg := GenerateConfig(ProfileLocal, nil, "", "")
	cfg.Version = "99"
	errs := ValidateConfig(cfg)
	if !HasErrors(errs) {
		t.Error("expected error for bad version")
	}
}

func TestValidateConfig_BadProfile(t *testing.T) {
	cfg := GenerateConfig(ProfileLocal, nil, "", "")
	cfg.Profile = "nonexistent"
	errs := ValidateConfig(cfg)
	if !HasErrors(errs) {
		t.Error("expected error for bad profile")
	}
}

func TestValidateConfig_EmptyEndpoint(t *testing.T) {
	cfg := GenerateConfig(ProfileLocal, nil, "", "")
	cfg.Runtime.Endpoint = ""
	errs := ValidateConfig(cfg)
	if !HasErrors(errs) {
		t.Error("expected error for empty endpoint")
	}
}

func TestValidateConfig_StudioRemoteMissingHost(t *testing.T) {
	cfg := GenerateConfig(ProfileStudioRemote, nil, "", "")
	// host is empty but profile is studio-remote
	errs := ValidateConfig(cfg)
	if !HasErrors(errs) {
		t.Error("expected error for studio-remote with no host")
	}
}

func TestValidateConfig_StudioRemoteWithHost(t *testing.T) {
	cfg := GenerateConfig(ProfileStudioRemote, nil, "", "macstudio.local")
	errs := ValidateConfig(cfg)
	if HasErrors(errs) {
		t.Errorf("unexpected errors: %+v", errs)
	}
}

func TestValidateConfig_BadTransport(t *testing.T) {
	cfg := GenerateConfig(ProfileLocal, nil, "", "")
	cfg.Runtime.Remote.Transport = "pigeon"
	errs := ValidateConfig(cfg)
	if !HasErrors(errs) {
		t.Error("expected error for bad transport")
	}
}

func TestValidateConfig_DuplicateAdapter(t *testing.T) {
	cfg := GenerateConfig(ProfileLocal, []AdapterName{AdapterClaudeCode, AdapterClaudeCode}, "", "")
	errs := ValidateConfig(cfg)
	if !HasErrors(errs) {
		t.Error("expected error for duplicate adapter")
	}
}

func TestValidateConfig_UnknownAdapter(t *testing.T) {
	cfg := GenerateConfig(ProfileLocal, nil, "", "")
	cfg.Adapters = []AdapterConfig{{Name: "unknown-agent", Enabled: true}}
	errs := ValidateConfig(cfg)
	if !HasErrors(errs) {
		t.Error("expected error for unknown adapter")
	}
}

func TestValidateConfig_EmptySpaceIDStrategy(t *testing.T) {
	cfg := GenerateConfig(ProfileLocal, nil, "", "")
	cfg.Hooks.SpaceIDStrategy = ""
	errs := ValidateConfig(cfg)
	if !HasErrors(errs) {
		t.Error("expected error for empty space_id_strategy")
	}
}

func TestGenerateConfig_Defaults(t *testing.T) {
	cfg := GenerateConfig(ProfileLocal, []AdapterName{AdapterClaudeCode, AdapterCodex}, "", "")
	if cfg.Version != ConfigVersion {
		t.Errorf("version = %q, want %q", cfg.Version, ConfigVersion)
	}
	if cfg.Runtime.Endpoint != "http://localhost:9999" {
		t.Errorf("endpoint = %q, want default", cfg.Runtime.Endpoint)
	}
	if len(cfg.Adapters) != 2 {
		t.Errorf("adapters count = %d, want 2", len(cfg.Adapters))
	}
	if cfg.Hooks.SpaceIDStrategy != "repo-basename" {
		t.Errorf("space_id_strategy = %q, want repo-basename", cfg.Hooks.SpaceIDStrategy)
	}
	if !cfg.Install.AutoFix {
		t.Error("auto_fix should be true by default")
	}
}

func TestMarshalConfigYAML_RoundTrip(t *testing.T) {
	cfg := GenerateConfig(ProfileLocal, []AdapterName{AdapterClaudeCode}, "", "")
	data, err := MarshalConfigYAML(cfg)
	if err != nil {
		t.Fatalf("MarshalConfigYAML: %v", err)
	}

	// Write and reload
	dir := t.TempDir()
	path := filepath.Join(dir, "sidecar.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig round-trip: %v", err)
	}
	if cfg2.Version != cfg.Version {
		t.Errorf("round-trip version mismatch: %q vs %q", cfg2.Version, cfg.Version)
	}
	if cfg2.Profile != cfg.Profile {
		t.Errorf("round-trip profile mismatch: %q vs %q", cfg2.Profile, cfg.Profile)
	}
}

func TestHashConfig_Deterministic(t *testing.T) {
	data := []byte("test config content")
	h1 := HashConfig(data)
	h2 := HashConfig(data)
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q vs %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("hash length = %d, want 64 hex chars", len(h1))
	}
}

func TestResolveSpaceID(t *testing.T) {
	tests := []struct {
		name       string
		strategy   string
		projectDir string
		want       string
	}{
		{
			name:       "repo-basename strategy",
			strategy:   "repo-basename",
			projectDir: "/home/user/MyProject",
			want:       "myproject",
		},
		{
			name:       "default falls back to repo-basename",
			strategy:   "unknown-strategy",
			projectDir: "/home/user/Some-Repo",
			want:       "some-repo",
		},
		{
			name:       "nested path extracts basename",
			strategy:   "repo-basename",
			projectDir: "/a/b/c/MDEMG",
			want:       "mdemg",
		},
		{
			name:       "already lowercase",
			strategy:   "repo-basename",
			projectDir: "/tmp/test-repo",
			want:       "test-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Hooks: HooksConfig{SpaceIDStrategy: tt.strategy},
			}
			got := ResolveSpaceID(cfg, tt.projectDir)
			if got != tt.want {
				t.Errorf("ResolveSpaceID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindConfigFileFrom(t *testing.T) {
	dir := t.TempDir()
	mdemg := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemg, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(mdemg, "sidecar.yaml")
	if err := os.WriteFile(path, []byte("version: \"1\""), 0644); err != nil {
		t.Fatal(err)
	}

	// Find from same dir
	found := FindConfigFileFrom(dir)
	if found != path {
		t.Errorf("FindConfigFileFrom = %q, want %q", found, path)
	}

	// Find from subdirectory
	sub := filepath.Join(dir, "src", "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	found = FindConfigFileFrom(sub)
	if found != path {
		t.Errorf("FindConfigFileFrom (subdir) = %q, want %q", found, path)
	}

	// Not found
	empty := t.TempDir()
	found = FindConfigFileFrom(empty)
	if found != "" {
		t.Errorf("FindConfigFileFrom (empty) = %q, want empty", found)
	}
}
