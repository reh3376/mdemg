package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSpaceID_FromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	mdemgDir := filepath.Join(tmpDir, ".mdemg")
	if err := os.MkdirAll(mdemgDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `server:
  port: 9999
space_id: "my-project"
`
	if err := os.WriteFile(filepath.Join(mdemgDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	got := detectSpaceID(tmpDir)
	if got != "my-project" {
		t.Errorf("detectSpaceID() = %q, want %q", got, "my-project")
	}
}

func TestDetectSpaceID_FallbackToDir(t *testing.T) {
	tmpDir := t.TempDir()
	got := detectSpaceID(tmpDir)
	want := filepath.Base(tmpDir)
	if got != want {
		t.Errorf("detectSpaceID() = %q, want %q", got, want)
	}
}

func TestRemoveMDEMGFromMCPConfig_OnlyMDEMG(t *testing.T) {
	tmpDir := t.TempDir()
	mcpPath := filepath.Join(tmpDir, ".mcp.json")

	content := `{"mcpServers":{"mdemg":{"command":"mdemg","args":["mcp"]}}}`
	if err := os.WriteFile(mcpPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	removed, fileDeleted, err := removeMDEMGFromMCPConfig(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
	if !fileDeleted {
		t.Error("expected fileDeleted=true when mdemg is the only server")
	}
	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestRemoveMDEMGFromMCPConfig_WithOtherServers(t *testing.T) {
	tmpDir := t.TempDir()
	mcpPath := filepath.Join(tmpDir, ".mcp.json")

	content := `{"mcpServers":{"mdemg":{"command":"mdemg","args":["mcp"]},"other":{"command":"other-server"}}}`
	if err := os.WriteFile(mcpPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	removed, fileDeleted, err := removeMDEMGFromMCPConfig(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
	if fileDeleted {
		t.Error("expected fileDeleted=false when other servers remain")
	}

	// Verify file still exists with other server
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	servers := root["mcpServers"].(map[string]any)
	if _, exists := servers["mdemg"]; exists {
		t.Error("mdemg entry should be removed")
	}
	if _, exists := servers["other"]; !exists {
		t.Error("other server should be preserved")
	}
}

func TestRemoveMDEMGFromMCPConfig_NoMDEMG(t *testing.T) {
	tmpDir := t.TempDir()
	mcpPath := filepath.Join(tmpDir, ".mcp.json")

	content := `{"mcpServers":{"other":{"command":"other-server"}}}`
	if err := os.WriteFile(mcpPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	removed, _, err := removeMDEMGFromMCPConfig(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("expected removed=false when no mdemg entry exists")
	}
}

func TestRemoveMDEMGFromMCPConfig_FileNotExist(t *testing.T) {
	removed, _, err := removeMDEMGFromMCPConfig("/nonexistent/path/.mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("expected removed=false for nonexistent file")
	}
}

func TestCleanupClaudeSettings_RemovesMDEMGHooks(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "/path/to/mdemg/hooks/session-start.sh",
							"timeout": 15,
						},
					},
				},
			},
		},
		"permissions": map[string]any{
			"allow": []any{"Read", "Write"},
		},
	}

	data, _ := json.MarshalIndent(settings, "", "  ")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	cleaned, err := cleanupClaudeSettings(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if !cleaned {
		t.Error("expected cleaned=true")
	}

	// File should still exist (permissions remain)
	resultData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatal(err)
	}
	if _, exists := result["hooks"]; exists {
		t.Error("hooks key should be removed (was only mdemg hooks)")
	}
	if _, exists := result["permissions"]; !exists {
		t.Error("permissions should be preserved")
	}
}

func TestCleanupClaudeSettings_NoMDEMGHooks(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	settings := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "/path/to/other-tool/hook.sh",
							"timeout": 5,
						},
					},
				},
			},
		},
	}

	data, _ := json.MarshalIndent(settings, "", "  ")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	cleaned, err := cleanupClaudeSettings(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if cleaned {
		t.Error("expected cleaned=false when no mdemg hooks exist")
	}
}

func TestDeregisterFromSidebar(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock registry file
	regDir := filepath.Join(tmpDir, "sidebar")
	if err := os.MkdirAll(regDir, 0755); err != nil {
		t.Fatal(err)
	}
	regPath := filepath.Join(regDir, "instances.json")

	instances := []sidebarInstance{
		{ID: "1", Name: "project-a", ProjectDirectory: "/path/to/project-a", SpaceID: "project-a"},
		{ID: "2", Name: "project-b", ProjectDirectory: "/path/to/project-b", SpaceID: "project-b"},
	}
	data, _ := json.MarshalIndent(instances, "", "  ")
	if err := os.WriteFile(regPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Read back and verify
	loaded, err := readSidebarRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(loaded))
	}

	// Filter out project-a
	var filtered []sidebarInstance
	for _, inst := range loaded {
		if inst.ProjectDirectory != "/path/to/project-a" {
			filtered = append(filtered, inst)
		}
	}
	if err := writeSidebarRegistry(regPath, filtered); err != nil {
		t.Fatal(err)
	}

	// Verify
	result, err := readSidebarRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 instance after removal, got %d", len(result))
	}
	if result[0].ProjectDirectory != "/path/to/project-b" {
		t.Errorf("wrong instance remaining: %s", result[0].ProjectDirectory)
	}
}

func TestContainsMDEMGRef(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/path/to/mdemg/hooks/session-start.sh", true},
		{"/path/to/MDEMG/hooks/session-start.sh", true},
		{"/path/to/other/hooks/session-start.sh", false},
		{"mdemg", true},
		{"", false},
	}

	for _, tc := range tests {
		got := containsMDEMGRef(tc.input)
		if got != tc.want {
			t.Errorf("containsMDEMGRef(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
