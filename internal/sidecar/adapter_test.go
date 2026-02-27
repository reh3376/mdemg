package sidecar

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// --- Factory tests ---

func TestNewAdapter_ClaudeCode(t *testing.T) {
	a, err := NewAdapter(AdapterClaudeCode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name() != AdapterClaudeCode {
		t.Errorf("expected %q, got %q", AdapterClaudeCode, a.Name())
	}
	if a.Version() != "claude-code-v1" {
		t.Errorf("expected claude-code-v1, got %s", a.Version())
	}
	if a.ConfigFormat() != "json" {
		t.Errorf("expected json, got %s", a.ConfigFormat())
	}
}

func TestNewAdapter_Codex(t *testing.T) {
	a, err := NewAdapter(AdapterCodex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name() != AdapterCodex {
		t.Errorf("expected %q, got %q", AdapterCodex, a.Name())
	}
	if a.Version() != "codex-v1" {
		t.Errorf("expected codex-v1, got %s", a.Version())
	}
	if a.ConfigFormat() != "toml" {
		t.Errorf("expected toml, got %s", a.ConfigFormat())
	}
}

func TestNewAdapter_Invalid(t *testing.T) {
	_, err := NewAdapter(AdapterName("unknown"))
	if err == nil {
		t.Fatal("expected error for unknown adapter")
	}
	if !strings.Contains(err.Error(), "unknown adapter") {
		t.Errorf("error should mention 'unknown adapter', got: %s", err.Error())
	}
}

// --- Claude Code adapter tests ---

func TestClaudeCodeAdapter_ConfigPath(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	path := a.ConfigPath("/home/user/project")
	expected := "/home/user/project/.claude/mcp.json"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestClaudeCodeAdapter_BuildPayload(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	data, err := a.BuildPayload("http://localhost:9999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	servers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers key missing or not an object")
	}
	mdemg, ok := servers["mdemg"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers.mdemg key missing or not an object")
	}
	if mdemg["command"] != "mdemg" {
		t.Errorf("expected command=mdemg, got %v", mdemg["command"])
	}
	env, ok := mdemg["env"].(map[string]any)
	if !ok {
		t.Fatal("env key missing or not an object")
	}
	if env["MDEMG_ENDPOINT"] != "http://localhost:9999" {
		t.Errorf("expected endpoint http://localhost:9999, got %v", env["MDEMG_ENDPOINT"])
	}
}

func TestClaudeCodeAdapter_MergeInto_Empty(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	data, strategy, err := a.MergeInto(nil, "http://localhost:9999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strategy != "create" {
		t.Errorf("expected strategy 'create', got %q", strategy)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := parsed["mcpServers"]; !ok {
		t.Error("expected mcpServers key in output")
	}
}

func TestClaudeCodeAdapter_MergeInto_Existing(t *testing.T) {
	existing := []byte(`{
  "mcpServers": {
    "other-tool": {
      "command": "other"
    }
  }
}`)
	a := &ClaudeCodeAdapter{}
	data, strategy, err := a.MergeInto(existing, "http://localhost:9999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strategy != "merge" {
		t.Errorf("expected strategy 'merge', got %q", strategy)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	servers := parsed["mcpServers"].(map[string]any)
	if _, ok := servers["other-tool"]; !ok {
		t.Error("merge should preserve existing other-tool key")
	}
	if _, ok := servers["mdemg"]; !ok {
		t.Error("merge should add mdemg key")
	}
}

func TestClaudeCodeAdapter_MergeInto_Idempotent(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	first, _, err := a.MergeInto(nil, "http://localhost:9999")
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	second, strategy, err := a.MergeInto(first, "http://localhost:9999")
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if strategy != "merge" {
		t.Errorf("expected strategy 'merge' on second run, got %q", strategy)
	}

	// Parse both and compare mdemg blocks
	var p1, p2 map[string]any
	json.Unmarshal(first, &p1)
	json.Unmarshal(second, &p2)

	s1 := p1["mcpServers"].(map[string]any)["mdemg"]
	s2 := p2["mcpServers"].(map[string]any)["mdemg"]

	j1, _ := json.Marshal(s1)
	j2, _ := json.Marshal(s2)
	if string(j1) != string(j2) {
		t.Error("idempotent merge produced different mdemg blocks")
	}
}

func TestClaudeCodeAdapter_MergeInto_NoMcpServers(t *testing.T) {
	existing := []byte(`{"someOtherKey": true}`)
	a := &ClaudeCodeAdapter{}
	data, strategy, err := a.MergeInto(existing, "http://localhost:9999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strategy != "merge" {
		t.Errorf("expected strategy 'merge', got %q", strategy)
	}

	var parsed map[string]any
	json.Unmarshal(data, &parsed)
	if _, ok := parsed["someOtherKey"]; !ok {
		t.Error("should preserve existing keys")
	}
	servers := parsed["mcpServers"].(map[string]any)
	if _, ok := servers["mdemg"]; !ok {
		t.Error("should add mdemg key")
	}
}

func TestClaudeCodeAdapter_RemoveFrom(t *testing.T) {
	existing := []byte(`{
  "mcpServers": {
    "mdemg": {"command": "mdemg"},
    "other": {"command": "other"}
  }
}`)
	a := &ClaudeCodeAdapter{}
	data, err := a.RemoveFrom(existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal(data, &parsed)
	servers := parsed["mcpServers"].(map[string]any)
	if _, ok := servers["mdemg"]; ok {
		t.Error("mdemg key should be removed")
	}
	if _, ok := servers["other"]; !ok {
		t.Error("other key should be preserved")
	}
}

func TestClaudeCodeAdapter_RemoveFrom_OnlyMdemg(t *testing.T) {
	existing := []byte(`{
  "mcpServers": {
    "mdemg": {"command": "mdemg"}
  }
}`)
	a := &ClaudeCodeAdapter{}
	data, err := a.RemoveFrom(existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal(data, &parsed)
	if _, ok := parsed["mcpServers"]; ok {
		t.Error("empty mcpServers should be removed entirely")
	}
}

func TestClaudeCodeAdapter_RemoveFrom_Empty(t *testing.T) {
	a := &ClaudeCodeAdapter{}
	data, err := a.RemoveFrom(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Error("remove from nil should return nil")
	}
}

func TestClaudeCodeAdapter_RemoveFrom_NoMdemg(t *testing.T) {
	existing := []byte(`{"mcpServers": {"other": {"command": "other"}}}`)
	a := &ClaudeCodeAdapter{}
	data, err := a.RemoveFrom(existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal(data, &parsed)
	servers := parsed["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Error("should preserve existing keys when mdemg not present")
	}
}

// --- Codex adapter tests ---

func TestCodexAdapter_ConfigPath(t *testing.T) {
	a := &CodexAdapter{}
	path := a.ConfigPath("/home/user/project")
	expected := "/home/user/project/.codex/config.toml"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestCodexAdapter_BuildPayload(t *testing.T) {
	a := &CodexAdapter{}
	data, err := a.BuildPayload("http://localhost:9999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if _, err := toml.Decode(string(data), &parsed); err != nil {
		t.Fatalf("invalid TOML: %v", err)
	}

	servers, ok := parsed["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatal("mcp_servers key missing or not a table")
	}
	mdemg, ok := servers["mdemg"].(map[string]any)
	if !ok {
		t.Fatal("mcp_servers.mdemg key missing or not a table")
	}
	if mdemg["command"] != "mdemg" {
		t.Errorf("expected command=mdemg, got %v", mdemg["command"])
	}
}

func TestCodexAdapter_MergeInto_Empty(t *testing.T) {
	a := &CodexAdapter{}
	data, strategy, err := a.MergeInto(nil, "http://localhost:9999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strategy != "create" {
		t.Errorf("expected strategy 'create', got %q", strategy)
	}

	var parsed map[string]any
	if _, err := toml.Decode(string(data), &parsed); err != nil {
		t.Fatalf("invalid TOML: %v", err)
	}
	if _, ok := parsed["mcp_servers"]; !ok {
		t.Error("expected mcp_servers key in output")
	}
}

func TestCodexAdapter_MergeInto_Existing(t *testing.T) {
	existing := []byte(`[mcp_servers.other]
command = "other"

[settings]
theme = "dark"
`)
	a := &CodexAdapter{}
	data, strategy, err := a.MergeInto(existing, "http://localhost:9999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strategy != "merge" {
		t.Errorf("expected strategy 'merge', got %q", strategy)
	}

	var parsed map[string]any
	if _, err := toml.Decode(string(data), &parsed); err != nil {
		t.Fatalf("invalid TOML: %v", err)
	}
	servers := parsed["mcp_servers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Error("merge should preserve existing other key")
	}
	if _, ok := servers["mdemg"]; !ok {
		t.Error("merge should add mdemg key")
	}
	if _, ok := parsed["settings"]; !ok {
		t.Error("merge should preserve settings section")
	}
}

func TestCodexAdapter_MergeInto_Idempotent(t *testing.T) {
	a := &CodexAdapter{}
	first, _, err := a.MergeInto(nil, "http://localhost:9999")
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	second, strategy, err := a.MergeInto(first, "http://localhost:9999")
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if strategy != "merge" {
		t.Errorf("expected strategy 'merge' on second run, got %q", strategy)
	}

	var p1, p2 map[string]any
	toml.Decode(string(first), &p1)
	toml.Decode(string(second), &p2)

	s1, _ := json.Marshal(p1["mcp_servers"].(map[string]any)["mdemg"])
	s2, _ := json.Marshal(p2["mcp_servers"].(map[string]any)["mdemg"])
	if string(s1) != string(s2) {
		t.Error("idempotent merge produced different mdemg sections")
	}
}

func TestCodexAdapter_RemoveFrom(t *testing.T) {
	existing := []byte(`[mcp_servers.mdemg]
command = "mdemg"

[mcp_servers.other]
command = "other"
`)
	a := &CodexAdapter{}
	data, err := a.RemoveFrom(existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	toml.Decode(string(data), &parsed)
	servers := parsed["mcp_servers"].(map[string]any)
	if _, ok := servers["mdemg"]; ok {
		t.Error("mdemg section should be removed")
	}
	if _, ok := servers["other"]; !ok {
		t.Error("other section should be preserved")
	}
}

func TestCodexAdapter_RemoveFrom_OnlyMdemg(t *testing.T) {
	existing := []byte(`[mcp_servers.mdemg]
command = "mdemg"
`)
	a := &CodexAdapter{}
	data, err := a.RemoveFrom(existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	toml.Decode(string(data), &parsed)
	if _, ok := parsed["mcp_servers"]; ok {
		t.Error("empty mcp_servers should be removed entirely")
	}
}

func TestCodexAdapter_RemoveFrom_Empty(t *testing.T) {
	a := &CodexAdapter{}
	data, err := a.RemoveFrom(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Error("remove from nil should return nil")
	}
}

// --- AttachAgentReport nil-slice safety ---

func TestAttachAgentReport_NilSliceSafety(t *testing.T) {
	r := NewAttachAgentReport("mdemg sidecar attach-agent", StateRunning, StateRunning, ExitSuccess)
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(data)
	// Verify slices are [] not null
	if strings.Contains(s, `"changes":null`) {
		t.Error("changes should be [] not null")
	}
	if strings.Contains(s, `"issues":null`) {
		t.Error("issues should be [] not null")
	}
	if strings.Contains(s, `"next_actions":null`) {
		t.Error("next_actions should be [] not null")
	}
}
