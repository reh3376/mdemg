package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mdemg/internal/sidecar"
)

func TestUpgradeCmd_InvalidState(t *testing.T) {
	// Uninitialized state (no .mdemg/ directory) should be rejected
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	err := runSidecarUpgrade(sidecarUpgradeFlags{format: "text"})
	if err == nil {
		t.Fatal("expected error for uninitialized state, got nil")
	}
	if want := "cannot upgrade from state"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want substring %q", err.Error(), want)
	}
}

func TestUpgradeCmd_AlreadyCurrent(t *testing.T) {
	// Create a temp dir with a lock file that has the same version as cli.Version
	dir := t.TempDir()
	mdemgDir := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemgDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a config file
	configPath := filepath.Join(mdemgDir, "sidecar.yaml")
	configData := []byte("version: \"1\"\nprofile: local\n")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Write lock file with matching version and config hash
	lf := &sidecar.LockFile{
		State:        sidecar.StateInstalled,
		Profile:      "local",
		MdemgVersion: Version,
		ConfigHash:   sidecar.HashConfig(configData),
		CreatedAt:    sidecar.NowUTC(),
		UpdatedAt:    sidecar.NowUTC(),
	}
	lockPath := filepath.Join(mdemgDir, "sidecar.lock")
	if err := sidecar.WriteLock(lockPath, lf); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Should succeed with no-op
	err := runSidecarUpgrade(sidecarUpgradeFlags{format: "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpgradeReport_NilSliceSafety(t *testing.T) {
	report := sidecar.NewUpgradeReport(
		sidecar.StateInstalled, sidecar.StateRunning, sidecar.ExitSuccess,
		"0.1.0", "0.2.0", true, false,
	)

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, field := range []string{"changes", "issues", "next_actions"} {
		v, ok := raw[field]
		if !ok {
			t.Errorf("missing field %q in JSON", field)
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			t.Errorf("field %q is not an array, got %T", field, v)
			continue
		}
		if len(arr) != 0 {
			t.Errorf("field %q should be empty, got %v", field, arr)
		}
	}
}
