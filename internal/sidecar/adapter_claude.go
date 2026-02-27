package sidecar

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// ClaudeCodeAdapter implements Adapter for Claude Code's .claude/mcp.json.
type ClaudeCodeAdapter struct{}

func (a *ClaudeCodeAdapter) Name() AdapterName { return AdapterClaudeCode }
func (a *ClaudeCodeAdapter) Version() string   { return "claude-code-v1" }
func (a *ClaudeCodeAdapter) ConfigFormat() string { return "json" }

func (a *ClaudeCodeAdapter) ConfigPath(projectDir string) string {
	return filepath.Join(projectDir, ".claude", "mcp.json")
}

// mdemgMCPBlock returns the MDEMG MCP server config map for the given endpoint.
func mdemgMCPBlock(endpoint string) map[string]any {
	return map[string]any{
		"command": "mdemg",
		"args":    []string{"mcp"},
		"env": map[string]string{
			"MDEMG_ENDPOINT": endpoint,
		},
	}
}

func (a *ClaudeCodeAdapter) BuildPayload(endpoint string) ([]byte, error) {
	payload := map[string]any{
		"mcpServers": map[string]any{
			"mdemg": mdemgMCPBlock(endpoint),
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal claude-code payload: %w", err)
	}
	data = append(data, '\n')
	return data, nil
}

func (a *ClaudeCodeAdapter) MergeInto(existing []byte, endpoint string) ([]byte, string, error) {
	// Empty or nil → create from scratch
	if len(existing) == 0 {
		data, err := a.BuildPayload(endpoint)
		if err != nil {
			return nil, "", err
		}
		return data, "create", nil
	}

	// Parse existing JSON
	var root map[string]any
	if err := json.Unmarshal(existing, &root); err != nil {
		return nil, "", fmt.Errorf("parse existing mcp.json: %w", err)
	}

	// Ensure mcpServers key exists
	servers, ok := root["mcpServers"]
	if !ok {
		root["mcpServers"] = map[string]any{}
		servers = root["mcpServers"]
	}

	serversMap, ok := servers.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("mcpServers is not an object")
	}

	// Set/overwrite the mdemg key
	serversMap["mdemg"] = mdemgMCPBlock(endpoint)
	root["mcpServers"] = serversMap

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("marshal merged mcp.json: %w", err)
	}
	data = append(data, '\n')
	return data, "merge", nil
}

func (a *ClaudeCodeAdapter) RemoveFrom(existing []byte) ([]byte, error) {
	if len(existing) == 0 {
		return existing, nil
	}

	var root map[string]any
	if err := json.Unmarshal(existing, &root); err != nil {
		return nil, fmt.Errorf("parse existing mcp.json: %w", err)
	}

	servers, ok := root["mcpServers"]
	if !ok {
		// Nothing to remove
		return existing, nil
	}

	serversMap, ok := servers.(map[string]any)
	if !ok {
		return existing, nil
	}

	delete(serversMap, "mdemg")

	// If mcpServers is now empty, remove the key entirely
	if len(serversMap) == 0 {
		delete(root, "mcpServers")
	} else {
		root["mcpServers"] = serversMap
	}

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal cleaned mcp.json: %w", err)
	}
	data = append(data, '\n')
	return data, nil
}
