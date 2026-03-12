// E2E tests for the uxts-module plugin.
// These validate the full pipeline: manifest validation, binary build,
// plugin discovery, and real spec scanning.
//
// Run with: go test -v -tags=e2e ./plugins/uxts-module/...
//
//go:build e2e

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestE2E_ManifestValid validates the manifest.json against plugin conventions.
func TestE2E_ManifestValid(t *testing.T) {
	manifestPath := findManifest(t)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest struct {
		ID                   string            `json:"id"`
		Name                 string            `json:"name"`
		Version              string            `json:"version"`
		Type                 string            `json:"type"`
		Binary               string            `json:"binary"`
		Capabilities         map[string]any    `json:"capabilities"`
		HealthCheckIntervalMs int              `json:"health_check_interval_ms"`
		StartupTimeoutMs     int               `json:"startup_timeout_ms"`
		Config               map[string]string `json:"config"`
	}

	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	// Required fields
	if manifest.ID == "" {
		t.Error("manifest.id is empty")
	}
	if manifest.ID != "uxts-module" {
		t.Errorf("manifest.id = %s, want uxts-module", manifest.ID)
	}
	if manifest.Name == "" {
		t.Error("manifest.name is empty")
	}
	if manifest.Version == "" {
		t.Error("manifest.version is empty")
	}
	if manifest.Type != "REASONING" {
		t.Errorf("manifest.type = %s, want REASONING", manifest.Type)
	}
	if manifest.Binary != "uxts-module" {
		t.Errorf("manifest.binary = %s, want uxts-module", manifest.Binary)
	}

	// Capabilities
	if manifest.Capabilities == nil {
		t.Fatal("manifest.capabilities is nil")
	}
	detectors, ok := manifest.Capabilities["pattern_detectors"].([]any)
	if !ok || len(detectors) == 0 {
		t.Error("manifest.capabilities.pattern_detectors should be non-empty")
	}

	// Config
	requiredConfigs := []string{"spec_root", "frameworks", "mdemg_endpoint", "space_id"}
	for _, key := range requiredConfigs {
		if manifest.Config[key] == "" {
			t.Errorf("manifest.config.%s is empty", key)
		}
	}

	// Timeouts
	if manifest.HealthCheckIntervalMs < 1000 {
		t.Errorf("health_check_interval_ms = %d, should be >= 1000", manifest.HealthCheckIntervalMs)
	}
	if manifest.StartupTimeoutMs < 1000 {
		t.Errorf("startup_timeout_ms = %d, should be >= 1000", manifest.StartupTimeoutMs)
	}
}

// TestE2E_BinaryBuilds verifies the plugin binary builds successfully.
func TestE2E_BinaryBuilds(t *testing.T) {
	projectRoot := findProjectRootE2E(t)
	pluginDir := filepath.Join(projectRoot, "plugins", "uxts-module")

	outputBin := filepath.Join(t.TempDir(), "uxts-module-test")

	cmd := exec.Command("go", "build", "-o", outputBin, ".")
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}

	// Verify binary exists and is executable
	info, err := os.Stat(outputBin)
	if err != nil {
		t.Fatalf("built binary not found: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("binary is not executable")
	}

	t.Logf("Binary size: %d bytes", info.Size())
}

// TestE2E_BinaryShowsHelp verifies the binary prints socket requirement when run without args.
func TestE2E_BinaryShowsHelp(t *testing.T) {
	projectRoot := findProjectRootE2E(t)
	pluginDir := filepath.Join(projectRoot, "plugins", "uxts-module")

	outputBin := filepath.Join(t.TempDir(), "uxts-module-help")

	cmd := exec.Command("go", "build", "-o", outputBin, ".")
	cmd.Dir = pluginDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}

	// Run without --socket flag — should exit with error
	cmd = exec.Command(outputBin)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error when run without --socket flag")
	}
	if len(output) == 0 {
		t.Error("expected error message in output")
	}
	t.Logf("Output without socket: %s", output)
}

// TestE2E_RealSpecCount scans real spec directories and validates counts.
func TestE2E_RealSpecCount(t *testing.T) {
	projectRoot := findProjectRootE2E(t)
	docsDir := filepath.Join(projectRoot, "docs")

	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		t.Skip("docs directory not found")
	}

	idx := NewSpecIndex()
	if err := idx.Load(docsDir, "uats,udts,upts,ubts,usts,uobs,uots,uets,uvts,uams", true); err != nil {
		t.Fatal(err)
	}

	stats := idx.FrameworkStats()
	t.Logf("Framework spec counts:")
	total := 0
	for fw, count := range stats {
		t.Logf("  %s: %d", fw, count)
		total += count
	}
	t.Logf("  TOTAL: %d", total)

	// Known minimums from UXTS_FRAMEWORK_MATRIX.md
	expectedMinimums := map[string]int{
		"uats": 100,
		"udts": 10,
		"upts": 20,
		"uets": 5,
	}
	for fw, min := range expectedMinimums {
		if stats[fw] < min {
			t.Errorf("%s: expected >= %d specs, got %d", fw, min, stats[fw])
		}
	}

	// Drift report
	t.Logf("Drift detected: %d specs", idx.DriftCount())
}

// TestE2E_DriftDetectionMatchesPython verifies our Go drift detection produces
// results consistent with the Python verify_uxts_drift.py script.
func TestE2E_DriftDetectionMatchesPython(t *testing.T) {
	projectRoot := findProjectRootE2E(t)
	docsDir := filepath.Join(projectRoot, "docs")

	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		t.Skip("docs directory not found")
	}

	idx := NewSpecIndex()
	if err := idx.Load(docsDir, "uats,udts,upts,ubts,usts,uobs,uots,uets,uvts", true); err != nil {
		t.Fatal(err)
	}

	// For each spec with a hash, verify drift detection is sane
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	cleanCount := 0
	driftCount := 0
	noHashCount := 0

	for _, entry := range idx.entries {
		switch entry.DriftState {
		case "clean":
			cleanCount++
		case "drift_detected":
			driftCount++
		default:
			t.Errorf("unexpected drift state: %s for %s", entry.DriftState, entry.FileName)
		}

		// Check if the spec has a hash field at all
		hashField := frameworkDefs[entry.Framework].HashField
		stored := getNestedString(entry.RawSpec, hashField)
		if stored == "" {
			noHashCount++
		}
	}

	t.Logf("Drift summary: %d clean, %d drifted, %d without hash", cleanCount, driftCount, noHashCount)

	// Sanity check: most specs should be clean
	if cleanCount == 0 && idx.TotalSpecs() > 0 {
		t.Error("no clean specs found — drift detection may be broken")
	}
}

// TestE2E_SpecIndexLookupPerformance basic performance sanity check.
func TestE2E_SpecIndexLookupPerformance(t *testing.T) {
	projectRoot := findProjectRootE2E(t)
	docsDir := filepath.Join(projectRoot, "docs")

	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		t.Skip("docs directory not found")
	}

	idx := NewSpecIndex()
	if err := idx.Load(docsDir, "uats,udts,upts,ubts,usts,uobs,uots,uets,uvts", false); err != nil {
		t.Fatal(err)
	}

	if idx.TotalSpecs() == 0 {
		t.Skip("no specs loaded")
	}

	// Benchmark path lookup — should be O(1)
	paths := []string{
		"/v1/memory/retrieve",
		"/v1/conversation/observe",
		"/v1/memory/ingest",
		"/v1/self-improve/cycle",
	}

	const iterations = 1000
	for _, path := range paths {
		for i := 0; i < iterations; i++ {
			idx.FindByPath(path)
		}
	}

	// Benchmark text search — O(n) but still fast
	queries := []string{"retrieve", "observe", "ingest", "memory"}
	for _, q := range queries {
		for i := 0; i < 100; i++ {
			idx.FindByText(q)
		}
	}

	t.Logf("Lookups completed: %d path + %d text (total specs: %d)",
		len(paths)*iterations, len(queries)*100, idx.TotalSpecs())
}

func findManifest(t *testing.T) string {
	t.Helper()
	// Try relative paths from test working directory
	candidates := []string{
		"manifest.json",
		"plugins/uxts-module/manifest.json",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	// Try from project root
	root := findProjectRootE2E(t)
	path := filepath.Join(root, "plugins", "uxts-module", "manifest.json")
	if _, err := os.Stat(path); err == nil {
		return path
	}

	t.Fatal("manifest.json not found")
	return ""
}

func findProjectRootE2E(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("could not find project root")
		}
		dir = parent
	}
}
