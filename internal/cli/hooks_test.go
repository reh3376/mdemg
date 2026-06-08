package cli

import (
	"encoding/json"
	hook_templates "mdemg/internal/cli/hook_templates"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClaudeHookFilesReturnsAllHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		entries := claudeHookFiles()
		if len(entries) != 2 {
			t.Fatalf("expected 2 Windows hook entries, got %d", len(entries))
		}
		return
	}

	entries := claudeHookFiles()
	if len(entries) != 6 {
		t.Fatalf("expected 6 hook entries, got %d", len(entries))
	}

	expected := map[string]struct {
		Event   string
		Timeout int
		Matcher string
	}{
		"session-start.sh":     {"SessionStart", 15, ""},
		"prompt-context.sh":    {"UserPromptSubmit", 12, ""},
		"post-tool-observe.py": {"PostToolUse", 10, "Bash|Write|Edit"},
		"pre-compact.sh":       {"PreCompact", 10, ""},
		"pre-bash-check.py":    {"PreToolUse", 5, "Bash"},
		"pre-write-check.py":   {"PreToolUse", 8, "Write|Edit"},
	}

	for _, entry := range entries {
		want, ok := expected[entry.Template]
		if !ok {
			t.Errorf("unexpected template: %s", entry.Template)
			continue
		}
		if entry.Event != want.Event {
			t.Errorf("%s: event = %q, want %q", entry.Template, entry.Event, want.Event)
		}
		if entry.Timeout != want.Timeout {
			t.Errorf("%s: timeout = %d, want %d", entry.Template, entry.Timeout, want.Timeout)
		}
		if entry.Matcher != want.Matcher {
			t.Errorf("%s: matcher = %q, want %q", entry.Template, entry.Matcher, want.Matcher)
		}
	}
}

func TestClaudeHookFilesTemplatesExist(t *testing.T) {
	for _, entry := range claudeHookFiles() {
		_, err := hook_templates.FS.ReadFile(entry.Template)
		if err != nil {
			t.Errorf("template %s not found in embedded FS: %v", entry.Template, err)
		}
	}
}

func TestMergeClaudeSettingsMatcherAtGroupLevel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("matcher test requires Unix hook entries")
	}

	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.local.json")
	hookDir := filepath.Join(tmpDir, "hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create dummy hook files so the commands reference real paths
	for _, hf := range claudeHookFiles() {
		if err := os.WriteFile(filepath.Join(hookDir, hf.Template), []byte("#!/bin/bash\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := mergeClaudeSettings(settingsPath, hookDir); err != nil {
		t.Fatalf("mergeClaudeSettings: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse settings: %v", err)
	}

	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("missing hooks key in settings")
	}

	// Check PostToolUse has matcher at group level
	postToolGroups, ok := hooks["PostToolUse"].([]interface{})
	if !ok || len(postToolGroups) == 0 {
		t.Fatal("missing PostToolUse hook groups")
	}
	group := postToolGroups[0].(map[string]interface{})
	matcher, ok := group["matcher"].(string)
	if !ok || matcher != "Bash|Write|Edit" {
		t.Errorf("PostToolUse group matcher = %q, want %q", matcher, "Bash|Write|Edit")
	}

	// Check PreToolUse has matcher "Bash"
	preToolGroups, ok := hooks["PreToolUse"].([]interface{})
	if !ok || len(preToolGroups) == 0 {
		t.Fatal("missing PreToolUse hook groups")
	}
	preGroup := preToolGroups[0].(map[string]interface{})
	preMatcher, ok := preGroup["matcher"].(string)
	if !ok || preMatcher != "Bash" {
		t.Errorf("PreToolUse group matcher = %q, want %q", preMatcher, "Bash")
	}

	// Check SessionStart has NO matcher
	sessionGroups, ok := hooks["SessionStart"].([]interface{})
	if !ok || len(sessionGroups) == 0 {
		t.Fatal("missing SessionStart hook groups")
	}
	sessionGroup := sessionGroups[0].(map[string]interface{})
	if _, hasMatcher := sessionGroup["matcher"]; hasMatcher {
		t.Error("SessionStart group should not have a matcher")
	}

	// Verify hook entries are inside the group
	hookEntries, ok := group["hooks"].([]interface{})
	if !ok || len(hookEntries) == 0 {
		t.Fatal("PostToolUse group has no hook entries")
	}
	hookEntry := hookEntries[0].(map[string]interface{})
	if hookEntry["type"] != "command" {
		t.Errorf("hook entry type = %q, want %q", hookEntry["type"], "command")
	}
	if hookEntry["timeout"] != float64(10) {
		t.Errorf("hook entry timeout = %v, want 10", hookEntry["timeout"])
	}
}

func TestMergeClaudeSettingsPreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.local.json")
	hookDir := filepath.Join(tmpDir, "hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatal(err)
	}

	for _, hf := range claudeHookFiles() {
		if err := os.WriteFile(filepath.Join(hookDir, hf.Template), []byte("#!/bin/bash\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Write existing settings with custom permissions
	existing := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []string{"Read", "Write"},
		},
		"hooks": map[string]interface{}{
			"CustomEvent": []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "/usr/local/bin/custom-hook",
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := mergeClaudeSettings(settingsPath, hookDir); err != nil {
		t.Fatalf("mergeClaudeSettings: %v", err)
	}

	result, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(result, &root); err != nil {
		t.Fatal(err)
	}

	// Permissions preserved
	if _, ok := root["permissions"]; !ok {
		t.Error("existing permissions were not preserved")
	}

	// Custom hook preserved
	hooks := root["hooks"].(map[string]interface{})
	if _, ok := hooks["CustomEvent"]; !ok {
		t.Error("existing CustomEvent hook was not preserved")
	}
}

func TestHookTemplates_HaveMDEMGMarker(t *testing.T) {
	for _, hf := range claudeHookFiles() {
		data, err := hook_templates.FS.ReadFile(hf.Template)
		if err != nil {
			t.Fatalf("read %s: %v", hf.Template, err)
		}
		lines := strings.SplitN(string(data), "\n", 6) // first 5 lines
		found := false
		for _, line := range lines {
			if strings.Contains(line, "# MDEMG") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: missing '# MDEMG' marker in first 5 lines", hf.Template)
		}
	}
}

func TestHookTemplates_NoMDEMGURLPlaceholder(t *testing.T) {
	for _, hf := range claudeHookFiles() {
		data, err := hook_templates.FS.ReadFile(hf.Template)
		if err != nil {
			t.Fatalf("read %s: %v", hf.Template, err)
		}
		if strings.Contains(string(data), "{{MDEMG_URL}}") {
			t.Errorf("%s: still contains {{MDEMG_URL}} placeholder", hf.Template)
		}
	}
}

func TestHookTemplates_ShellPortDiscovery(t *testing.T) {
	shellTemplates := []string{"session-start.sh", "prompt-context.sh", "pre-compact.sh"}
	for _, name := range shellTemplates {
		data, err := hook_templates.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		content := string(data)
		if !strings.Contains(content, ".mdemg.port") {
			t.Errorf("%s: missing .mdemg.port port discovery", name)
		}
		if !strings.Contains(content, "MDEMG_PORT") {
			t.Errorf("%s: missing .env MDEMG_PORT fallback", name)
		}
	}
}

func TestHookTemplates_PythonPortDiscovery(t *testing.T) {
	data, err := hook_templates.FS.ReadFile("post-tool-observe.py")
	if err != nil {
		t.Fatalf("read post-tool-observe.py: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "_resolve_mdemg_url") {
		t.Error("post-tool-observe.py: missing _resolve_mdemg_url function")
	}
	if !strings.Contains(content, ".mdemg.port") {
		t.Error("post-tool-observe.py: missing .mdemg.port port discovery")
	}
}
