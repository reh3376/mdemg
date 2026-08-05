package grafana_templates

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RELEASE-HYGIENE-001 E2 tests. Pins:
//   - the embed contains the expected file count (fail-fast on accidental
//     removal of a dashboard from staged/)
//   - Materialize writes to the correct compose-template-relative paths
//   - Idempotency: re-run skips identical files
//   - Idempotency: re-run PRESERVES operator-edited files (no clobber)

func TestManifest_ExpectedFileCount(t *testing.T) {
	files, err := Manifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	// 9 dashboards + 3 datasources + 1 alerting + 1 dashboards.yml + 2 notifiers = 16
	// HITL-ANALYTICS-TILE-001 (2026-08-04) added mdemg-hitl.json → 9 dashboards.
	const want = 16
	if len(files) != want {
		t.Fatalf("manifest file count = %d, want %d (a dashboard was removed or added without updating this pin)", len(files), want)
	}
	// Structural pin: certain files MUST exist for the compose template's
	// mount paths to be useful.
	required := []string{
		"dashboards/mdemg-overview.json",
		"provisioning/dashboards/dashboards.yml",
		"provisioning/datasources/timescaledb.yml",
		"provisioning/datasources/neo4j.yml",
		"provisioning/alerting/alerts.yml",
	}
	seen := map[string]bool{}
	for _, f := range files {
		seen[f] = true
	}
	for _, r := range required {
		if !seen[r] {
			t.Errorf("required file missing from embed: %s", r)
		}
	}
}

func TestMaterialize_WritesToExpectedTree(t *testing.T) {
	dst := t.TempDir()
	written, preserved, err := Materialize(dst)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if written != 16 {
		t.Errorf("first-run written = %d, want 16", written)
	}
	if preserved != 0 {
		t.Errorf("first-run preserved = %d, want 0 (nothing existed)", preserved)
	}
	// Files land at <dst>/deploy/docker/grafana/**
	for _, want := range []string{
		filepath.Join(dst, "deploy/docker/grafana/dashboards/mdemg-overview.json"),
		filepath.Join(dst, "deploy/docker/grafana/provisioning/datasources/timescaledb.yml"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("expected file missing: %s (%v)", want, err)
		}
	}
}

func TestMaterialize_Idempotent_SameContent(t *testing.T) {
	dst := t.TempDir()
	if _, _, err := Materialize(dst); err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	written, preserved, err := Materialize(dst)
	if err != nil {
		t.Fatalf("second materialize: %v", err)
	}
	if written != 0 {
		t.Errorf("re-run written = %d, want 0 (all files identical)", written)
	}
	if preserved != 16 {
		t.Errorf("re-run preserved = %d, want 16", preserved)
	}
}

func TestMaterialize_PreservesOperatorEdits(t *testing.T) {
	dst := t.TempDir()
	if _, _, err := Materialize(dst); err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	// Simulate an operator edit to a dashboard.
	dashPath := filepath.Join(dst, "deploy/docker/grafana/dashboards/mdemg-overview.json")
	operatorContent := []byte(`{"edited-by-operator":true}`)
	if err := os.WriteFile(dashPath, operatorContent, 0o644); err != nil {
		t.Fatalf("simulate edit: %v", err)
	}

	written, preserved, err := Materialize(dst)
	if err != nil {
		t.Fatalf("re-materialize: %v", err)
	}
	if written != 0 {
		t.Errorf("re-run must not overwrite anything; wrote %d", written)
	}
	if preserved != 16 {
		t.Errorf("preserved = %d, want 16", preserved)
	}
	// Confirm the operator-edited file was NOT overwritten.
	got, _ := os.ReadFile(dashPath)
	if sha256.Sum256(got) != sha256.Sum256(operatorContent) {
		t.Errorf("operator edit was clobbered — invariant violated")
	}
}

func TestMaterialize_EmptyDestErrors(t *testing.T) {
	if _, _, err := Materialize(""); err == nil {
		t.Fatalf("expected error on empty destDir")
	}
}

// TestMaterialize_ComposeMountPathsResolve pins that materialized files land
// EXACTLY where the compose template's cwd-relative mount paths expect —
// changing the DeployRelPath constant would silently break every fresh
// install's Grafana provisioning.
func TestMaterialize_ComposeMountPathsResolve(t *testing.T) {
	if DeployRelPath != "deploy/docker/grafana" {
		t.Fatalf("DeployRelPath = %q, MUST match compose template mount base (./deploy/docker/grafana)", DeployRelPath)
	}
	if !strings.HasPrefix(DeployRelPath, "deploy/") {
		t.Errorf("materialized path must live under deploy/ to match compose template mount base")
	}
}
