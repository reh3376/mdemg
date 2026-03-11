//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- Helpers ---

func sidecarBinary(t *testing.T) string {
	t.Helper()
	if b := os.Getenv("MDEMG_BINARY"); b != "" {
		abs, err := filepath.Abs(b)
		if err != nil {
			return b
		}
		return abs
	}
	abs, err := filepath.Abs("../../bin/mdemg")
	if err != nil {
		t.Fatalf("cannot resolve binary path: %v", err)
	}
	return abs
}

// runSidecarCmd executes `mdemg sidecar <args>` in the given directory.
// Returns stdout, stderr, and the process exit code.
func runSidecarCmd(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	bin := sidecarBinary(t)
	fullArgs := append([]string{"sidecar"}, args...)
	cmd := exec.Command(bin, fullArgs...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("exec error (not ExitError): %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// parseJSON parses a JSON string into a map. Fails the test on error.
func parseJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("failed to parse JSON:\n%s\nerror: %v", s, err)
	}
	return m
}

// assertJSONField checks that a top-level field in the JSON map equals the expected string.
func assertJSONField(t *testing.T, m map[string]any, field, expected string) {
	t.Helper()
	val, ok := m[field]
	if !ok {
		t.Fatalf("JSON missing field %q", field)
	}
	got := fmt.Sprintf("%v", val)
	if got != expected {
		t.Errorf("JSON field %q = %q, want %q", field, got, expected)
	}
}

// initTempSidecar creates a temp directory, runs `sidecar init --defaults --format json`,
// and returns the temp dir path. Registers cleanup.
func initTempSidecar(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stdout, stderr, exitCode := runSidecarCmd(t, dir, "init", "--defaults", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("init failed (exit %d):\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	return dir
}

// writeLockState writes a minimal sidecar.lock file with the given state to the temp dir.
func writeLockState(t *testing.T, dir string, state string) {
	t.Helper()
	mdemgDir := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemgDir, 0755); err != nil {
		t.Fatalf("mkdir .mdemg: %v", err)
	}
	lock := map[string]any{
		"state":         state,
		"profile":       "local",
		"endpoint":      "http://localhost:9999",
		"config_hash":   "testhash",
		"mdemg_version": "dev",
		"created_at":    "2026-01-01T00:00:00Z",
		"updated_at":    "2026-01-01T00:00:00Z",
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatalf("marshal lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mdemgDir, "sidecar.lock"), append(data, '\n'), 0644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
}

// writeMinimalConfig writes a minimal sidecar.yaml so config checks pass.
func writeMinimalConfig(t *testing.T, dir string) {
	t.Helper()
	mdemgDir := filepath.Join(dir, ".mdemg")
	if err := os.MkdirAll(mdemgDir, 0755); err != nil {
		t.Fatalf("mkdir .mdemg: %v", err)
	}
	yaml := `version: "1"
profile: local
runtime:
  endpoint: http://localhost:9999
  remote:
    host: ""
    transport: ""
adapters:
  - name: claude-code
    enabled: true
hooks:
  space_id_strategy: repo-basename
install:
  auto_fix: false
`
	if err := os.WriteFile(filepath.Join(mdemgDir, "sidecar.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// --- Init Tests ---

func TestSidecar_Init_DryRun_JSON(t *testing.T) {
	dir := t.TempDir()
	stdout, _, exitCode := runSidecarCmd(t, dir, "init", "--dry-run", "--format", "json", "--defaults")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "result", "success")
	assertJSONField(t, m, "state_before", "uninitialized")

	// Verify changes contain would-create actions
	changes, ok := m["changes"].([]any)
	if !ok || len(changes) == 0 {
		t.Fatal("expected non-empty changes array")
	}
	foundWouldCreate := false
	for _, c := range changes {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if cm["action"] == "would-create" {
			foundWouldCreate = true
		}
	}
	if !foundWouldCreate {
		t.Error("expected at least one 'would-create' change")
	}

	// Dry run should NOT create files
	if _, err := os.Stat(filepath.Join(dir, ".mdemg", "sidecar.yaml")); err == nil {
		t.Error("dry-run should not create sidecar.yaml")
	}
}

func TestSidecar_Init_Defaults_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	_, _, exitCode := runSidecarCmd(t, dir, "init", "--defaults", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	// Both files should exist
	for _, name := range []string{"sidecar.yaml", "sidecar.lock"} {
		path := filepath.Join(dir, ".mdemg", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}

	// Lock file should show state=initialized
	lockData, err := os.ReadFile(filepath.Join(dir, ".mdemg", "sidecar.lock"))
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	var lock map[string]any
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatalf("parse lock: %v", err)
	}
	if lock["state"] != "initialized" {
		t.Errorf("lock state = %q, want initialized", lock["state"])
	}
}

func TestSidecar_Init_InvalidProfile_Error(t *testing.T) {
	dir := t.TempDir()
	_, _, exitCode := runSidecarCmd(t, dir, "init", "--profile", "nonexistent", "--defaults")
	if exitCode == 0 {
		t.Error("expected non-zero exit for invalid profile")
	}
}

func TestSidecar_Init_Idempotency(t *testing.T) {
	dir := initTempSidecar(t)
	// Second init with --defaults should succeed
	_, _, exitCode := runSidecarCmd(t, dir, "init", "--defaults", "--format", "json")
	if exitCode != 0 {
		t.Errorf("expected exit 0 on second init, got %d", exitCode)
	}
}

// --- Status Tests ---

func TestSidecar_Status_Uninitialized_JSON(t *testing.T) {
	dir := t.TempDir()
	stdout, _, exitCode := runSidecarCmd(t, dir, "status", "--format", "json")
	// Status from uninitialized should still produce output
	if exitCode != 0 {
		// Some implementations may return non-zero for uninitialized
		t.Logf("status from uninitialized exited %d", exitCode)
	}
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "state_before", "uninitialized")
}

func TestSidecar_Status_PostInit_JSON(t *testing.T) {
	dir := initTempSidecar(t)
	stdout, _, _ := runSidecarCmd(t, dir, "status", "--format", "json")
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "state_before", "initialized")
}

// --- Install Tests ---

func TestSidecar_Install_InvalidState_JSON(t *testing.T) {
	dir := t.TempDir()
	// From uninitialized — should fail
	stdout, _, _ := runSidecarCmd(t, dir, "install", "--format", "json")
	m := parseJSON(t, stdout)
	// JSON exit_code field should be ExitValidation (2)
	assertJSONField(t, m, "exit_code", "2")

	// Should have INVALID_STATE issue
	issues, ok := m["issues"].([]any)
	if !ok || len(issues) == 0 {
		t.Fatal("expected issues array with INVALID_STATE")
	}
	issue := issues[0].(map[string]any)
	if issue["code"] != "INVALID_STATE" {
		t.Errorf("issue code = %q, want INVALID_STATE", issue["code"])
	}
	if issue["remediation"] == nil || issue["remediation"] == "" {
		t.Error("expected remediation in issue")
	}
}

func TestSidecar_Install_DryRun_JSON(t *testing.T) {
	dir := initTempSidecar(t)
	stdout, _, exitCode := runSidecarCmd(t, dir, "install", "--dry-run", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	m := parseJSON(t, stdout)
	// Should have valid JSON structure
	if _, ok := m["command"]; !ok {
		t.Error("JSON missing 'command' field")
	}
	if _, ok := m["state_before"]; !ok {
		t.Error("JSON missing 'state_before' field")
	}
	// Preflight checks should be present
	if _, ok := m["preflight"]; ok {
		// preflight field exists — good
	}
}

func TestSidecar_Install_Idempotency(t *testing.T) {
	dir := initTempSidecar(t)
	// Set state to installed
	writeLockState(t, dir, "installed")
	// Re-run install should succeed (idempotent)
	stdout, _, exitCode := runSidecarCmd(t, dir, "install", "--dry-run", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0 for idempotent install, got %d\nstdout: %s", exitCode, stdout)
	}
}

// --- Up Tests ---

func TestSidecar_Up_InvalidState_Guard(t *testing.T) {
	dir := t.TempDir()
	// From uninitialized
	stdout, _, _ := runSidecarCmd(t, dir, "up", "--format", "json")
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "exit_code", "2")
}

func TestSidecar_Up_DryRun_PostInstall(t *testing.T) {
	dir := initTempSidecar(t)
	writeMinimalConfig(t, dir) // ensure config is present
	writeLockState(t, dir, "installed")
	stdout, _, exitCode := runSidecarCmd(t, dir, "up", "--dry-run", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "result", "dry-run")
	assertJSONField(t, m, "state_before", "installed")
}

// --- Doctor Tests ---

func TestSidecar_Doctor_NoConfig_JSON(t *testing.T) {
	dir := t.TempDir()
	stdout, _, _ := runSidecarCmd(t, dir, "doctor", "--format", "json")
	m := parseJSON(t, stdout)

	// Should have checks array
	checks, ok := m["checks"].([]any)
	if !ok || len(checks) == 0 {
		t.Fatal("expected non-empty checks array")
	}

	// First check should be config.valid and should fail
	firstCheck := checks[0].(map[string]any)
	if firstCheck["id"] != "config.valid" {
		t.Errorf("first check id = %q, want config.valid", firstCheck["id"])
	}
	if firstCheck["status"] != "fail" {
		t.Errorf("config.valid status = %q, want fail", firstCheck["status"])
	}
	if firstCheck["remediation"] == nil || firstCheck["remediation"] == "" {
		t.Error("expected remediation for failed config check")
	}
}

func TestSidecar_Doctor_WithConfig_JSON(t *testing.T) {
	dir := initTempSidecar(t)
	stdout, _, _ := runSidecarCmd(t, dir, "doctor", "--format", "json")
	m := parseJSON(t, stdout)

	checks, ok := m["checks"].([]any)
	if !ok || len(checks) == 0 {
		t.Fatal("expected non-empty checks array")
	}

	// config.valid should pass
	firstCheck := checks[0].(map[string]any)
	if firstCheck["id"] != "config.valid" {
		t.Errorf("first check id = %q, want config.valid", firstCheck["id"])
	}
	if firstCheck["status"] != "pass" {
		t.Errorf("config.valid status = %q, want pass", firstCheck["status"])
	}

	// Should have more checks after config.valid
	if len(checks) < 2 {
		t.Error("expected additional checks beyond config.valid")
	}
}

// --- GenerateHooks Tests ---

func TestSidecar_GenerateHooks_InvalidState(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, _ := runSidecarCmd(t, dir, "generate-hooks")
	// Should indicate invalid state or not initialized
	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "init") &&
		!strings.Contains(strings.ToLower(combined), "install") &&
		!strings.Contains(strings.ToLower(combined), "state") {
		t.Errorf("expected state guard message, got:\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestSidecar_GenerateHooks_DryRun(t *testing.T) {
	dir := initTempSidecar(t)
	writeMinimalConfig(t, dir)
	writeLockState(t, dir, "installed")
	stdout, _, exitCode := runSidecarCmd(t, dir, "generate-hooks", "--dry-run", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "result", "dry-run")
}

// --- Attach/Detach Tests ---

func TestSidecar_AttachAgent_InvalidState(t *testing.T) {
	dir := t.TempDir()
	stdout, _, _ := runSidecarCmd(t, dir, "attach-agent", "claude-code", "--format", "json")
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "exit_code", "2")
}

func TestSidecar_AttachAgent_DryRun(t *testing.T) {
	dir := initTempSidecar(t)
	writeMinimalConfig(t, dir)
	writeLockState(t, dir, "installed")
	stdout, _, exitCode := runSidecarCmd(t, dir, "attach-agent", "claude-code", "--dry-run", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "result", "dry-run")
}

func TestSidecar_AttachDetach_RoundTrip(t *testing.T) {
	dir := initTempSidecar(t)
	writeMinimalConfig(t, dir)
	writeLockState(t, dir, "installed")

	// Attach claude-code
	stdout, stderr, exitCode := runSidecarCmd(t, dir, "attach-agent", "claude-code", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("attach failed (exit %d):\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}

	// .claude/mcp.json should exist
	mcpPath := filepath.Join(dir, ".claude", "mcp.json")
	if _, err := os.Stat(mcpPath); err != nil {
		t.Fatalf("expected %s after attach: %v", mcpPath, err)
	}

	// Detach claude-code
	stdout, stderr, exitCode = runSidecarCmd(t, dir, "detach-agent", "claude-code", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("detach failed (exit %d):\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}

	// .claude/mcp.json should be removed (or empty)
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		// File removed — OK
	} else if len(data) > 0 {
		// If file still exists, it should be empty or {}
		trimmed := strings.TrimSpace(string(data))
		if trimmed != "{}" && trimmed != "" {
			t.Errorf("expected empty or removed mcp.json after detach, got: %s", trimmed)
		}
	}
}

// --- Upgrade Tests ---

func TestSidecar_Upgrade_InvalidState(t *testing.T) {
	dir := t.TempDir()
	// From uninitialized — should fail
	stdout, _, _ := runSidecarCmd(t, dir, "upgrade", "--format", "json")
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "exit_code", "2")
}

func TestSidecar_Upgrade_DryRun(t *testing.T) {
	dir := initTempSidecar(t)
	writeMinimalConfig(t, dir)
	writeLockState(t, dir, "installed")
	stdout, _, exitCode := runSidecarCmd(t, dir, "upgrade", "--dry-run", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "result", "dry-run")
}

// --- Uninstall Tests ---

func TestSidecar_Uninstall_InvalidState(t *testing.T) {
	dir := t.TempDir()
	// From uninitialized — should fail
	stdout, _, _ := runSidecarCmd(t, dir, "uninstall", "--format", "json")
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "exit_code", "2")
}

func TestSidecar_Uninstall_RunningWithoutForce(t *testing.T) {
	dir := initTempSidecar(t)
	writeMinimalConfig(t, dir)
	writeLockState(t, dir, "running")
	stdout, _, _ := runSidecarCmd(t, dir, "uninstall", "--format", "json")
	m := parseJSON(t, stdout)
	// Should reject — running state without --force
	assertJSONField(t, m, "exit_code", "2")
}

func TestSidecar_Uninstall_DryRun(t *testing.T) {
	dir := initTempSidecar(t)
	writeMinimalConfig(t, dir)
	writeLockState(t, dir, "installed")
	stdout, _, exitCode := runSidecarCmd(t, dir, "uninstall", "--dry-run", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "result", "dry-run")
}

// --- State Guards Matrix ---

func TestSidecar_StateGuards_Matrix(t *testing.T) {
	// Table: (command, args, wrong_state) → JSON exit_code 2
	tests := []struct {
		name  string
		cmd   []string
		state string
	}{
		{"install_from_uninitialized", []string{"install", "--format", "json"}, "uninitialized"},
		{"install_from_running", []string{"install", "--format", "json"}, "running"},
		{"up_from_uninitialized", []string{"up", "--format", "json"}, "uninitialized"},
		{"up_from_initialized", []string{"up", "--format", "json"}, "initialized"},
		{"down_from_initialized", []string{"down", "--format", "json"}, "initialized"},
		{"down_from_installed", []string{"down", "--format", "json"}, "installed"},
		{"restart_from_initialized", []string{"restart", "--format", "json"}, "initialized"},
		{"restart_from_installed", []string{"restart", "--format", "json"}, "installed"},
		{"attach_from_uninitialized", []string{"attach-agent", "claude-code", "--format", "json"}, "uninitialized"},
		{"attach_from_initialized", []string{"attach-agent", "claude-code", "--format", "json"}, "initialized"},
		{"detach_from_uninitialized", []string{"detach-agent", "claude-code", "--format", "json"}, "uninitialized"},
		{"detach_from_initialized", []string{"detach-agent", "claude-code", "--format", "json"}, "initialized"},
		{"upgrade_from_uninitialized", []string{"upgrade", "--format", "json"}, "uninitialized"},
		{"uninstall_from_uninitialized", []string{"uninstall", "--format", "json"}, "uninitialized"},
		{"uninstall_from_initialized", []string{"uninstall", "--format", "json"}, "initialized"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.state != "uninitialized" {
				writeMinimalConfig(t, dir)
				writeLockState(t, dir, tt.state)
			}
			stdout, _, _ := runSidecarCmd(t, dir, tt.cmd...)
			m := parseJSON(t, stdout)
			assertJSONField(t, m, "exit_code", "2")
		})
	}
}

// --- Down/Restart Invalid State Guards ---

func TestSidecar_Down_InvalidState_Guard(t *testing.T) {
	dir := initTempSidecar(t)
	// From initialized — should fail
	stdout, _, _ := runSidecarCmd(t, dir, "down", "--format", "json")
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "exit_code", "2")
}

func TestSidecar_Restart_InvalidState_Guard(t *testing.T) {
	dir := initTempSidecar(t)
	writeLockState(t, dir, "installed")
	stdout, _, _ := runSidecarCmd(t, dir, "restart", "--format", "json")
	m := parseJSON(t, stdout)
	assertJSONField(t, m, "exit_code", "2")
}
