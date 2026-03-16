package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureProjectMcpEnabled_CreateNew(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	if err := ensureProjectMcpEnabled(settingsPath); err != nil {
		t.Fatalf("ensureProjectMcpEnabled: %v", err)
	}

	// File should exist
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse settings: %v", err)
	}

	val, ok := parsed["enableAllProjectMcpServers"].(bool)
	if !ok || !val {
		t.Errorf("enableAllProjectMcpServers = %v, want true", parsed["enableAllProjectMcpServers"])
	}
}

func TestEnsureProjectMcpEnabled_ExistingSettings(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write existing settings with other fields
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	existing := map[string]any{
		"customSetting": "value",
		"otherFlag":     42,
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	if err := ensureProjectMcpEnabled(settingsPath); err != nil {
		t.Fatalf("ensureProjectMcpEnabled: %v", err)
	}

	// Read back
	updated, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(updated, &parsed); err != nil {
		t.Fatalf("parse settings: %v", err)
	}

	// New field should be set
	val, ok := parsed["enableAllProjectMcpServers"].(bool)
	if !ok || !val {
		t.Errorf("enableAllProjectMcpServers = %v, want true", parsed["enableAllProjectMcpServers"])
	}

	// Existing fields should be preserved
	if parsed["customSetting"] != "value" {
		t.Errorf("customSetting = %v, want 'value'", parsed["customSetting"])
	}
}

func TestEnsureProjectMcpEnabled_AlreadySet(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	existing := map[string]any{
		"enableAllProjectMcpServers": true,
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	// Get file mod time before
	infoBefore, _ := os.Stat(settingsPath)

	if err := ensureProjectMcpEnabled(settingsPath); err != nil {
		t.Fatalf("ensureProjectMcpEnabled: %v", err)
	}

	// File should not be rewritten (idempotent)
	infoAfter, _ := os.Stat(settingsPath)
	if infoBefore.Size() != infoAfter.Size() {
		t.Error("file was rewritten despite already having the setting")
	}
}

func TestEnsureProjectMcpEnabled_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	// Don't create .claude/ — the function should create it
	settingsPath := filepath.Join(dir, ".claude", "settings.local.json")

	if err := ensureProjectMcpEnabled(settingsPath); err != nil {
		t.Fatalf("ensureProjectMcpEnabled: %v", err)
	}

	// File should exist
	if _, err := os.Stat(settingsPath); err != nil {
		t.Errorf("settings file not created: %v", err)
	}
}
