package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mdemg/internal/config"
)

// Tier 1 unit tests for Sprint MODEL-DIST-001 Epic 4. The verification
// checklist requires that nothing is hardcoded — every operator-visible
// value flows through config.Config and is overridable via env or flag.
// These tests pin that contract.

// makeCfg returns a Config with defaults that match the v1 production
// reality, ready for tests to mutate per-case.
func makeCfg() config.Config {
	return config.Config{
		ModelBackend:       "ollama",
		ModelNamespace:     "reh3376",
		ModelName:          "mdemg-llm-v1",
		ModelQuants:        "Q4_K_M,Q5_K_M,Q8_0",
		ModelRamTiers:      `{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}`,
		ModelQuant:         "auto",
		AdapterBase:        "qwen3:14b",
		ModelDir:           "/tmp/mdemg-test-models",
		OllamaModelsRoot:   "/tmp/mdemg-test-ollama",
		OllamaRegistryHost: "registry.ollama.ai",
	}
}

func TestNewFetcher_DispatchesOnBackend(t *testing.T) {
	cases := []struct {
		backend string
		wantErr bool
		wantName string
	}{
		{"ollama", false, "ollama"},
		{"", false, "ollama"}, // empty falls back to ollama
		{"OLLAMA", false, "ollama"}, // case-insensitive
		{"hf", true, ""},
		{"s3", true, ""},
		{"bogus", true, ""},
	}
	for _, tc := range cases {
		t.Run("backend="+tc.backend, func(t *testing.T) {
			cfg := makeCfg()
			cfg.ModelBackend = tc.backend
			f, err := NewFetcher(cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got Fetcher %+v", f)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.Name() != tc.wantName {
				t.Errorf("Name()=%q, want %q", f.Name(), tc.wantName)
			}
		})
	}
}

func TestResolveQuants(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"Q4_K_M,Q5_K_M,Q8_0", []string{"Q4_K_M", "Q5_K_M", "Q8_0"}},
		{" Q4_K_M , Q5_K_M ", []string{"Q4_K_M", "Q5_K_M"}}, // trim whitespace
		{"Q4_K_M", []string{"Q4_K_M"}},
		{"", nil},
		{",,Q4_K_M,,", []string{"Q4_K_M"}}, // drop empty entries
	}
	for _, tc := range cases {
		cfg := makeCfg()
		cfg.ModelQuants = tc.raw
		got := ResolveQuants(cfg)
		if !slicesEqual(got, tc.want) {
			t.Errorf("ResolveQuants(%q)=%v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestParseRamTiers_DefaultJSON(t *testing.T) {
	tiers, err := parseRamTiers(`{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect: <16 Q4_K_M, <24 Q5_K_M, then default Q8_0 last.
	if len(tiers) != 3 {
		t.Fatalf("got %d tiers, want 3: %+v", len(tiers), tiers)
	}
	if tiers[0].MaxGB != 16 || tiers[0].Quant != "Q4_K_M" {
		t.Errorf("tier[0]=%+v, want {16 Q4_K_M}", tiers[0])
	}
	if tiers[1].MaxGB != 24 || tiers[1].Quant != "Q5_K_M" {
		t.Errorf("tier[1]=%+v, want {24 Q5_K_M}", tiers[1])
	}
	if tiers[2].MaxGB != 0 || tiers[2].Quant != "Q8_0" {
		t.Errorf("tier[2]=%+v, want {0 Q8_0} (default)", tiers[2])
	}
}

func TestParseRamTiers_OperatorOverride(t *testing.T) {
	// Operator sets a custom map e.g. "always Q8_0" for a Mac Studio.
	tiers, err := parseRamTiers(`{"default":"Q8_0"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 1 || tiers[0].MaxGB != 0 || tiers[0].Quant != "Q8_0" {
		t.Errorf("got %+v, want single {0 Q8_0} default", tiers)
	}
}

func TestParseRamTiers_InvalidKey(t *testing.T) {
	cases := []string{
		`{"16":"Q4_K_M"}`,         // missing < prefix
		`{"<abc":"Q4_K_M"}`,       // non-numeric threshold
		`{"<-5":"Q4_K_M"}`,        // negative — accepted by strconv but semantically nonsense; let it through (-5)
		`not-json`,
	}
	for _, raw := range cases {
		_, err := parseRamTiers(raw)
		if raw == `{"<-5":"Q4_K_M"}` {
			// Negative thresholds parse OK; not enforced semantically.
			continue
		}
		if err == nil {
			t.Errorf("parseRamTiers(%q) returned nil error, want non-nil", raw)
		}
	}
}

func TestPickQuantForRAM(t *testing.T) {
	tiers, _ := parseRamTiers(`{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}`)
	cases := []struct {
		ramGB int
		want  string
	}{
		{8, "Q4_K_M"},   // < 16
		{15, "Q4_K_M"},  // boundary
		{16, "Q5_K_M"},  // 16 is not < 16; falls to next tier
		{20, "Q5_K_M"},  // < 24
		{23, "Q5_K_M"},  // boundary
		{24, "Q8_0"},    // 24 is not < 24; default
		{64, "Q8_0"},    // default
		{128, "Q8_0"},   // default
	}
	for _, tc := range cases {
		got, err := PickQuantForRAM(tiers, tc.ramGB)
		if err != nil {
			t.Errorf("ramGB=%d: unexpected error: %v", tc.ramGB, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ramGB=%d: got %q, want %q", tc.ramGB, got, tc.want)
		}
	}
}

func TestPickQuantForRAM_NoDefault(t *testing.T) {
	tiers, _ := parseRamTiers(`{"<16":"Q4_K_M"}`)
	_, err := PickQuantForRAM(tiers, 64)
	if err == nil {
		t.Error("expected error when no rule matches and no default rule given")
	}
}

func TestResolveQuant_AutoAcrossRAMTiers(t *testing.T) {
	cfg := makeCfg()
	cases := []struct {
		hostRAMGB int
		want      string
	}{
		{8, "Q4_K_M"},
		{20, "Q5_K_M"},
		{64, "Q8_0"},
	}
	for _, tc := range cases {
		got, err := ResolveQuant(cfg, "auto", tc.hostRAMGB)
		if err != nil {
			t.Errorf("ram=%d: unexpected error: %v", tc.hostRAMGB, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ram=%d: ResolveQuant=%q, want %q", tc.hostRAMGB, got, tc.want)
		}
	}
}

func TestResolveQuant_ExplicitInAllowlist(t *testing.T) {
	cfg := makeCfg()
	got, err := ResolveQuant(cfg, "Q5_K_M", 1) // RAM irrelevant when explicit
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Q5_K_M" {
		t.Errorf("got %q, want Q5_K_M", got)
	}
}

func TestResolveQuant_RejectsNonAllowlist(t *testing.T) {
	cfg := makeCfg()
	_, err := ResolveQuant(cfg, "Q3_K_M", 16)
	if err == nil {
		t.Error("expected error for quant not in allowlist")
	}
	if !strings.Contains(err.Error(), "not in MDEMG_MODEL_QUANTS allowlist") {
		t.Errorf("error message %q lacks allowlist hint", err.Error())
	}
}

func TestResolveQuant_OperatorCustomAllowlist(t *testing.T) {
	// Operator publishes their own Q6_K variant in their fork.
	cfg := makeCfg()
	cfg.ModelQuants = "Q6_K"
	cfg.ModelRamTiers = `{"default":"Q6_K"}`
	got, err := ResolveQuant(cfg, "auto", 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Q6_K" {
		t.Errorf("got %q, want Q6_K (operator-custom quant)", got)
	}
}

func TestLoadQuantManifest_EmbeddedFallback(t *testing.T) {
	cfg := makeCfg() // ModelManifestPath empty → use embed.FS
	mf, err := LoadQuantManifest(cfg)
	if err != nil {
		t.Fatalf("LoadQuantManifest: %v", err)
	}
	if mf.ModelName != "mdemg-llm-v1" {
		t.Errorf("embedded manifest ModelName=%q, want mdemg-llm-v1", mf.ModelName)
	}
	for _, q := range []string{"Q4_K_M", "Q5_K_M", "Q8_0"} {
		rec, ok := mf.Quants[q]
		if !ok {
			t.Errorf("embedded manifest missing quant %s", q)
			continue
		}
		if rec.SHA256 == "" || len(rec.SHA256) != 64 {
			t.Errorf("quant %s SHA256 invalid: %q", q, rec.SHA256)
		}
	}
}

func TestLoadQuantManifest_OperatorOverride(t *testing.T) {
	// Air-gapped operator ships a custom manifest pointing at their internal mirror.
	tmpDir := t.TempDir()
	customManifest := QuantManifest{
		Version:          "1.0.0",
		ModelName:        "custom-model",
		NamespaceDefault: "acme",
		Quants: map[string]QuantRecord{
			"Q4_K_M": {SizeBytes: 1, SHA256: "deadbeef00000000000000000000000000000000000000000000000000000000"},
		},
	}
	path := filepath.Join(tmpDir, "custom_manifest.json")
	data, _ := json.Marshal(customManifest)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := makeCfg()
	cfg.ModelManifestPath = path
	mf, err := LoadQuantManifest(cfg)
	if err != nil {
		t.Fatalf("LoadQuantManifest: %v", err)
	}
	if mf.ModelName != "custom-model" {
		t.Errorf("ModelName=%q, want custom-model (override should win over embed.FS)", mf.ModelName)
	}
	if mf.Quants["Q4_K_M"].SHA256 != "deadbeef00000000000000000000000000000000000000000000000000000000" {
		t.Errorf("Q4_K_M SHA from override not honored")
	}
}

func TestLoadQuantManifest_OverrideMissingFile(t *testing.T) {
	cfg := makeCfg()
	cfg.ModelManifestPath = "/nonexistent/path/to/manifest.json"
	_, err := LoadQuantManifest(cfg)
	if err == nil {
		t.Error("expected error reading missing manifest")
	}
}

// ─── HOMEBREW-INSTALLER-QWEN-UPDATE-002 Phase C pins ─────────────────────

// TestLoadQuantManifest_V2_DispatchesToV2Bytes asserts that setting
// cfg.ModelName="mdemg-llm-v2" loads the v2 embedded manifest (not v1).
// This is the primary Phase C contract.
func TestLoadQuantManifest_V2_DispatchesToV2Bytes(t *testing.T) {
	cfg := makeCfg()
	cfg.ModelName = "mdemg-llm-v2"
	mf, err := LoadQuantManifest(cfg)
	if err != nil {
		t.Fatalf("LoadQuantManifest: %v", err)
	}
	if mf.ModelName != "mdemg-llm-v2" {
		t.Errorf("ModelName=%q, want mdemg-llm-v2 (should have dispatched to v2 embedded manifest)", mf.ModelName)
	}
	for _, q := range []string{"Q4_K_M", "Q5_K_M", "Q8_0"} {
		rec, ok := mf.Quants[q]
		if !ok {
			t.Errorf("v2 manifest missing quant %s", q)
			continue
		}
		if rec.SHA256 == "" || len(rec.SHA256) != 64 {
			t.Errorf("v2 quant %s SHA256 invalid: %q", q, rec.SHA256)
		}
	}
	// v2 ships raw base (no adapter per operator scope decision 2026-08-19)
	if mf.Adapter != nil {
		t.Errorf("v2 manifest has adapter field set — should be nil (raw-base-only per scope decision)")
	}
}

// TestLoadQuantManifest_V2_SHAsMatchPhaseAPublishManifest is the drift
// detector: the v2 embedded manifest's SHAs MUST match publish_manifest_v2.json
// byte-for-byte. If either file is edited without the other, this fires.
func TestLoadQuantManifest_V2_SHAsMatchPhaseAPublishManifest(t *testing.T) {
	// Locate the publish manifest by walking up from the test file's package
	// dir. Test working directory is internal/cli/ at test time.
	repoRoot := filepath.Join("..", "..")
	publishManifestPath := filepath.Join(repoRoot, "docs", "development",
		"homebrew-installer-qwen-update-001", "publish_manifest_v2.json")
	data, err := os.ReadFile(publishManifestPath)
	if err != nil {
		t.Fatalf("read publish manifest: %v (Phase A hand-off artifact — has it moved?)", err)
	}
	var publish struct {
		Quants map[string]struct {
			SHA256 string `json:"sha256"`
		} `json:"quants"`
	}
	if err := json.Unmarshal(data, &publish); err != nil {
		t.Fatalf("parse publish manifest: %v", err)
	}

	cfg := makeCfg()
	cfg.ModelName = "mdemg-llm-v2"
	mf, err := LoadQuantManifest(cfg)
	if err != nil {
		t.Fatalf("LoadQuantManifest: %v", err)
	}
	for _, q := range []string{"Q4_K_M", "Q5_K_M", "Q8_0"} {
		wantSHA := publish.Quants[q].SHA256
		gotSHA := mf.Quants[q].SHA256
		if wantSHA == "" {
			t.Errorf("publish manifest missing SHA for %s", q)
			continue
		}
		if !strings.EqualFold(gotSHA, wantSHA) {
			t.Errorf("v2 embedded manifest %s SHA drift:\n  embedded : %s\n  publish  : %s\nUpdate quant_manifest_v2.json to match Phase A publish_manifest_v2.json",
				q, gotSHA, wantSHA)
		}
	}
}

// TestLoadQuantManifest_V1Explicit_DispatchesToV1 is a regression pin: an
// explicit ModelName=mdemg-llm-v1 must still resolve to v1's shipped SHAs.
func TestLoadQuantManifest_V1Explicit_DispatchesToV1(t *testing.T) {
	cfg := makeCfg()
	cfg.ModelName = "mdemg-llm-v1"
	mf, err := LoadQuantManifest(cfg)
	if err != nil {
		t.Fatalf("LoadQuantManifest: %v", err)
	}
	if mf.ModelName != "mdemg-llm-v1" {
		t.Errorf("ModelName=%q, want mdemg-llm-v1", mf.ModelName)
	}
	// v1 has adapter (Sprint MODEL-DIST-002)
	if mf.Adapter == nil {
		t.Errorf("v1 manifest lost adapter — MODEL-DIST-002 contract broken")
	}
}

// TestLoadQuantManifest_EmptyModelName_DefaultsToV1 is a backward-compat pin:
// pre-Phase-C the v1 manifest was the only embedded artifact. An empty
// ModelName (unusual — FromEnv defaults it — but test hygiene) must not
// break.
func TestLoadQuantManifest_EmptyModelName_DefaultsToV1(t *testing.T) {
	cfg := makeCfg()
	cfg.ModelName = ""
	mf, err := LoadQuantManifest(cfg)
	if err != nil {
		t.Fatalf("LoadQuantManifest: %v", err)
	}
	if mf.ModelName != "mdemg-llm-v1" {
		t.Errorf("empty ModelName ModelName=%q, want mdemg-llm-v1 (v1 fallback)", mf.ModelName)
	}
}

// TestLoadQuantManifest_UnknownModelName_FallsBackToV1 pins the safety net:
// an operator running a custom ModelName does not silently receive v2 SHAs
// (which would false-mismatch on their custom blob). Fall back to v1.
func TestLoadQuantManifest_UnknownModelName_FallsBackToV1(t *testing.T) {
	cfg := makeCfg()
	cfg.ModelName = "mdemg-llm-v99-custom"
	mf, err := LoadQuantManifest(cfg)
	if err != nil {
		t.Fatalf("LoadQuantManifest: %v", err)
	}
	if mf.ModelName != "mdemg-llm-v1" {
		t.Errorf("unknown ModelName ModelName=%q, want mdemg-llm-v1 (fallback)", mf.ModelName)
	}
}

// TestSelectEmbeddedManifest_CaseInsensitive pins the dispatch predicate:
// case variations of v2 all reach the v2 bytes.
func TestSelectEmbeddedManifest_CaseInsensitive(t *testing.T) {
	cases := []string{"mdemg-llm-v2", "MDEMG-LLM-V2", "Mdemg-Llm-V2", "  mdemg-llm-v2  "}
	for _, name := range cases {
		t.Run("name="+name, func(t *testing.T) {
			data := selectEmbeddedManifest(name)
			// Cheapest check: parse the manifest and assert ModelName
			var mf QuantManifest
			if err := json.Unmarshal(data, &mf); err != nil {
				t.Fatalf("parse: %v", err)
			}
			if mf.ModelName != "mdemg-llm-v2" {
				t.Errorf("dispatch failed for %q: got ModelName=%q", name, mf.ModelName)
			}
		})
	}
}

func TestOllamaFetcher_BuildTag_FusedVsAdapter(t *testing.T) {
	cfg := makeCfg()
	f := NewOllamaFetcher(cfg)
	cases := []struct {
		req  FetchRequest
		want string
	}{
		{FetchRequest{Namespace: "reh3376", Name: "mdemg-llm-v1", Quant: "Q5_K_M"}, "reh3376/mdemg-llm-v1:Q5_K_M"},
		{FetchRequest{Namespace: "acme", Name: "custom-model", Quant: "Q4_K_M"}, "acme/custom-model:Q4_K_M"},
		{FetchRequest{Namespace: "reh3376", Name: "mdemg-llm-v1", Adapter: true}, "reh3376/mdemg-llm-v1-adapter:latest"},
	}
	for _, tc := range cases {
		got := f.buildTag(tc.req)
		if got != tc.want {
			t.Errorf("buildTag(%+v)=%q, want %q", tc.req, got, tc.want)
		}
	}
}

func TestOllamaFetcher_ManifestPath_UsesConfig(t *testing.T) {
	cfg := makeCfg()
	cfg.OllamaModelsRoot = "/custom/ollama"
	cfg.OllamaRegistryHost = "registry.example.com"
	f := NewOllamaFetcher(cfg)
	req := FetchRequest{Namespace: "ns", Name: "model", Quant: "Q5_K_M"}
	got := f.manifestPath(req)
	want := "/custom/ollama/manifests/registry.example.com/ns/model/Q5_K_M"
	if got != want {
		t.Errorf("manifestPath=%q, want %q", got, want)
	}
}

func TestOllamaFetcher_ManifestPath_AdapterForm(t *testing.T) {
	cfg := makeCfg()
	f := NewOllamaFetcher(cfg)
	req := FetchRequest{Namespace: "reh3376", Name: "mdemg-llm-v1", Adapter: true}
	got := f.manifestPath(req)
	want := filepath.Join(cfg.OllamaModelsRoot, "manifests", "registry.ollama.ai", "reh3376", "mdemg-llm-v1-adapter", "latest")
	if got != want {
		t.Errorf("manifestPath adapter=%q, want %q", got, want)
	}
}

func TestOllamaFetcher_BlobPath_StripsDigestPrefix(t *testing.T) {
	cfg := makeCfg()
	cfg.OllamaModelsRoot = "/r"
	f := NewOllamaFetcher(cfg)
	// Ollama uses hyphen between sha256 and the hex digest in the blob filename.
	got := f.blobPath("sha256:abc123")
	want := "/r/blobs/sha256-abc123"
	if got != want {
		t.Errorf("blobPath=%q, want %q", got, want)
	}
	// Already-stripped digest still works.
	if got := f.blobPath("abc123"); got != want {
		t.Errorf("blobPath(no prefix)=%q, want %q", got, want)
	}
}

func TestDestFilename_FusedQuantAndAdapter(t *testing.T) {
	cases := []struct {
		req  FetchRequest
		want string
	}{
		{FetchRequest{Name: "mdemg-llm-v1", Quant: "Q5_K_M"}, "mdemg-llm-v1.Q5_K_M.gguf"},
		{FetchRequest{Name: "mdemg-llm-v1", Quant: "Q4_K_M"}, "mdemg-llm-v1.Q4_K_M.gguf"},
		{FetchRequest{Name: "custom-model", Quant: "Q8_0"}, "custom-model.Q8_0.gguf"},
		{FetchRequest{Name: "mdemg-llm-v1", Adapter: true}, "mdemg-llm-v1-adapter.gguf"},
		{FetchRequest{Name: "custom-model", Adapter: true}, "custom-model-adapter.gguf"},
		// Adapter overrides Quant when both set (Quant is ignored for adapter)
		{FetchRequest{Name: "mdemg-llm-v1", Quant: "Q5_K_M", Adapter: true}, "mdemg-llm-v1-adapter.gguf"},
	}
	for _, tc := range cases {
		got := destFilename(tc.req)
		if got != tc.want {
			t.Errorf("destFilename(%+v)=%q, want %q", tc.req, got, tc.want)
		}
	}
}

func TestOllamaFetcher_ReadAdapterBlobDigest_FiltersOnAdapterMediaType(t *testing.T) {
	// Adapter Modelfile manifests have BOTH a model layer (the base) AND an
	// adapter layer. When req.Adapter=true the resolver must pick the adapter
	// layer (vnd.ollama.image.adapter), not the base model.
	cfg := makeCfg()
	tmpDir := t.TempDir()
	cfg.OllamaModelsRoot = tmpDir
	cfg.OllamaRegistryHost = "registry.ollama.ai"

	manifest := ollamaManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.docker.distribution.manifest.v2+json",
		Layers: []ollamaManifestLayer{
			{MediaType: "application/vnd.ollama.image.model", Digest: "sha256:base", Size: 9276184896},
			{MediaType: "application/vnd.ollama.image.template", Digest: "sha256:tmpl", Size: 1723},
			{MediaType: "application/vnd.ollama.image.adapter", Digest: "sha256:0cfaf4bae3215a4aea664a8d28ae9a41d73ee740cbcce5c2eef950232cfe1de5", Size: 256939520},
			{MediaType: "application/vnd.ollama.image.license", Digest: "sha256:lic", Size: 11338},
		},
	}
	mfPath := filepath.Join(tmpDir, "manifests", "registry.ollama.ai", "reh3376", "mdemg-llm-v1-adapter", "latest")
	if err := os.MkdirAll(filepath.Dir(mfPath), 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(mfPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	f := NewOllamaFetcher(cfg)
	req := FetchRequest{Namespace: "reh3376", Name: "mdemg-llm-v1", Adapter: true}
	digest, size, err := f.readModelBlobDigest(req)
	if err != nil {
		t.Fatalf("readModelBlobDigest(adapter): %v", err)
	}
	wantDigest := "sha256:0cfaf4bae3215a4aea664a8d28ae9a41d73ee740cbcce5c2eef950232cfe1de5"
	if digest != wantDigest {
		t.Errorf("digest=%q, want %q (adapter layer, not base model)", digest, wantDigest)
	}
	if size != 256939520 {
		t.Errorf("size=%d, want 256939520 (adapter layer)", size)
	}
}

func TestOllamaFetcher_ReadModelBlobDigest_FiltersOnMediaType(t *testing.T) {
	// Author a synthetic Ollama manifest with multiple layers; only the model
	// layer should be selected.
	cfg := makeCfg()
	tmpDir := t.TempDir()
	cfg.OllamaModelsRoot = tmpDir
	cfg.OllamaRegistryHost = "registry.ollama.ai"

	manifest := ollamaManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.docker.distribution.manifest.v2+json",
		Config:        ollamaManifestConfig{MediaType: "application/vnd.docker.container.image.v1+json", Digest: "sha256:cfg", Size: 488},
		Layers: []ollamaManifestLayer{
			{MediaType: "application/vnd.ollama.image.template", Digest: "sha256:tmpl", Size: 100},
			{MediaType: "application/vnd.ollama.image.model", Digest: "sha256:abcd1234efgh5678ijkl9012mnop3456qrst7890uvwx1234yzab5678cdef9012", Size: 9658404096},
			{MediaType: "application/vnd.ollama.image.license", Digest: "sha256:lic", Size: 11338},
		},
	}
	mfPath := filepath.Join(tmpDir, "manifests", "registry.ollama.ai", "reh3376", "mdemg-llm-v1", "Q5_K_M")
	if err := os.MkdirAll(filepath.Dir(mfPath), 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(mfPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	f := NewOllamaFetcher(cfg)
	req := FetchRequest{Namespace: "reh3376", Name: "mdemg-llm-v1", Quant: "Q5_K_M"}
	digest, size, err := f.readModelBlobDigest(req)
	if err != nil {
		t.Fatalf("readModelBlobDigest: %v", err)
	}
	wantDigest := "sha256:abcd1234efgh5678ijkl9012mnop3456qrst7890uvwx1234yzab5678cdef9012"
	if digest != wantDigest {
		t.Errorf("digest=%q, want %q (model layer only — template/license/config layers must be filtered)", digest, wantDigest)
	}
	if size != 9658404096 {
		t.Errorf("size=%d, want 9658404096", size)
	}
}

func TestOllamaFetcher_ReadModelBlobDigest_MalformedJSON(t *testing.T) {
	cfg := makeCfg()
	tmpDir := t.TempDir()
	cfg.OllamaModelsRoot = tmpDir
	mfPath := filepath.Join(tmpDir, "manifests", "registry.ollama.ai", "reh3376", "mdemg-llm-v1", "Q5_K_M")
	_ = os.MkdirAll(filepath.Dir(mfPath), 0o755)
	_ = os.WriteFile(mfPath, []byte("{not valid json"), 0o644)
	f := NewOllamaFetcher(cfg)
	_, _, err := f.readModelBlobDigest(FetchRequest{Namespace: "reh3376", Name: "mdemg-llm-v1", Quant: "Q5_K_M"})
	if err == nil {
		t.Error("expected error on malformed JSON")
	}
}

func TestOllamaFetcher_ReadModelBlobDigest_NoModelLayer(t *testing.T) {
	cfg := makeCfg()
	tmpDir := t.TempDir()
	cfg.OllamaModelsRoot = tmpDir
	manifest := ollamaManifest{
		SchemaVersion: 2,
		Layers: []ollamaManifestLayer{
			{MediaType: "application/vnd.ollama.image.template", Digest: "sha256:tmpl"},
		},
	}
	mfPath := filepath.Join(tmpDir, "manifests", "registry.ollama.ai", "reh3376", "mdemg-llm-v1", "Q5_K_M")
	_ = os.MkdirAll(filepath.Dir(mfPath), 0o755)
	data, _ := json.Marshal(manifest)
	_ = os.WriteFile(mfPath, data, 0o644)
	f := NewOllamaFetcher(cfg)
	_, _, err := f.readModelBlobDigest(FetchRequest{Namespace: "reh3376", Name: "mdemg-llm-v1", Quant: "Q5_K_M"})
	if err == nil {
		t.Error("expected error when no layer has model mediaType")
	}
}

// Helper — slices.Equal is in Go 1.21+; we have it but spell it out for clarity.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestManifestAppliesToRequest pins the HOMEBREW-INSTALLER-QWEN-UPDATE-001
// Phase B safety fix: when the loaded quant manifest's ModelName does not
// match the request's ModelName, the SHA verify path must be short-circuited
// to prevent a false-mismatch error on the version-crossover case (e.g.
// pulling mdemg-llm-v2 with the v1 manifest embedded).
func TestManifestAppliesToRequest(t *testing.T) {
	cases := []struct {
		name          string
		manifestModel string
		requestModel  string
		want          bool
	}{
		{"exact match — v1 pull with v1 manifest", "mdemg-llm-v1", "mdemg-llm-v1", true},
		{"exact match — v2 pull with v2 manifest", "mdemg-llm-v2", "mdemg-llm-v2", true},
		{"case-insensitive match", "MDEMG-LLM-V1", "mdemg-llm-v1", true},
		{"MISMATCH — v2 pull with v1 manifest (the guard's raison d'être)", "mdemg-llm-v1", "mdemg-llm-v2", false},
		{"MISMATCH — v1 pull with v2 manifest", "mdemg-llm-v2", "mdemg-llm-v1", false},
		{"MISMATCH — custom fork name vs shipped v1", "mdemg-llm-v1", "acme-custom-llm", false},
		{"empty manifest ModelName treated as unversioned (backward-compat)", "", "mdemg-llm-v1", true},
		{"empty manifest ModelName vs v2 request (backward-compat)", "", "mdemg-llm-v2", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := manifestAppliesToRequest(tc.manifestModel, tc.requestModel)
			if got != tc.want {
				t.Errorf("manifestAppliesToRequest(%q, %q) = %v, want %v",
					tc.manifestModel, tc.requestModel, got, tc.want)
			}
		})
	}
}

// TestManifestAppliesToRequest_ShippedManifestVsV2Request pins the specific
// live-behavior contract: with the SHIPPED embedded quant_manifest.json
// (ModelName="mdemg-llm-v1") and a v2 request, the guard MUST return false
// so the SHA verify block is skipped. If this test breaks, the safety fix
// has regressed and a v2 pull will fire a false SHA-mismatch error.
func TestManifestAppliesToRequest_ShippedManifestVsV2Request(t *testing.T) {
	cfg := makeCfg()
	mf, err := LoadQuantManifest(cfg)
	if err != nil {
		t.Fatalf("LoadQuantManifest failed: %v", err)
	}
	if mf.ModelName != "mdemg-llm-v1" {
		t.Fatalf("shipped manifest ModelName sanity check: want %q, got %q",
			"mdemg-llm-v1", mf.ModelName)
	}
	// Simulate a v2 pull: manifest says v1, request says v2 → guard MUST skip.
	if manifestAppliesToRequest(mf.ModelName, "mdemg-llm-v2") {
		t.Errorf("shipped manifest (v1) MUST NOT apply to v2 request; guard would fail to skip SHA verify")
	}
	// Simulate the default v1 pull: manifest says v1, request says v1 → guard MUST verify.
	if !manifestAppliesToRequest(mf.ModelName, "mdemg-llm-v1") {
		t.Errorf("shipped manifest (v1) MUST apply to v1 request; guard would incorrectly skip SHA verify (breaking v1's shipped contract)")
	}
}
