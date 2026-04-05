package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestResolveInstanceID_ExplicitFlag(t *testing.T) {
	// Explicit flag value takes precedence over env and auto-generate
	t.Setenv("MDEMG_INSTANCE_ID", "env-value")
	got := resolveInstanceID("flag-value", "space1")
	if got != "flag-value" {
		t.Errorf("expected flag-value, got %s", got)
	}
}

func TestResolveInstanceID_EnvFallback(t *testing.T) {
	// When no explicit value, env var takes precedence
	t.Setenv("MDEMG_INSTANCE_ID", "env-value")
	got := resolveInstanceID("", "space1")
	if got != "env-value" {
		t.Errorf("expected env-value, got %s", got)
	}
}

func TestResolveInstanceID_AutoGenerate(t *testing.T) {
	// When both empty, auto-generate as {hostname}-{spaceID}
	t.Setenv("MDEMG_INSTANCE_ID", "")
	got := resolveInstanceID("", "mdemg-dev")
	hostname, _ := os.Hostname()
	expected := fmt.Sprintf("%s-mdemg-dev", hostname)
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestResolveInstanceID_NonEmpty(t *testing.T) {
	// Auto-generated value must never be empty
	t.Setenv("MDEMG_INSTANCE_ID", "")
	got := resolveInstanceID("", "test-space")
	if got == "" {
		t.Error("resolveInstanceID returned empty string — this causes invalid exports")
	}
}

func TestExportIDNoDoubleDash(t *testing.T) {
	// Simulate the export_id construction pattern from tsdb.RunExport:
	// export_id = "exp-{instanceID}-{timestamp}"
	// When instanceID was empty, this produced "exp--{timestamp}" (double-dash)
	t.Setenv("MDEMG_INSTANCE_ID", "")
	instanceID := resolveInstanceID("", "mdemg-dev")
	exportID := fmt.Sprintf("exp-%s-%s", instanceID, time.Now().Format("20060102-150405"))
	if strings.Contains(exportID, "--") {
		t.Errorf("export_id contains double-dash (blocking bug): %s", exportID)
	}
}

func TestExportFilenameNoDoubleDash(t *testing.T) {
	// Simulate the output filename pattern:
	// "mdemg-export-{instanceID}-{timestamp}.tar.gz"
	// When instanceID was empty, this produced "mdemg-export--{timestamp}.tar.gz"
	t.Setenv("MDEMG_INSTANCE_ID", "")
	instanceID := resolveInstanceID("", "mdemg-dev")
	filename := fmt.Sprintf("mdemg-export-%s-%s.tar.gz",
		instanceID, time.Now().Format("20060102-150405"))
	if strings.Contains(filename, "--") {
		t.Errorf("filename contains double-dash: %s", filename)
	}
}
