package sidecar

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// CodexAdapter implements Adapter for Codex's .codex/config.toml.
type CodexAdapter struct{}

func (a *CodexAdapter) Name() AdapterName   { return AdapterCodex }
func (a *CodexAdapter) Version() string     { return "codex-v1" }
func (a *CodexAdapter) ConfigFormat() string { return "toml" }

func (a *CodexAdapter) ConfigPath(projectDir string) string {
	return filepath.Join(projectDir, ".codex", "config.toml")
}

// codexMdemgSection returns the MDEMG MCP server config for TOML encoding.
func codexMdemgSection(endpoint string) map[string]any {
	return map[string]any{
		"command": "mdemg",
		"args":    []string{"mcp"},
		"env": map[string]string{
			"MDEMG_ENDPOINT": endpoint,
		},
	}
}

func (a *CodexAdapter) BuildPayload(endpoint string) ([]byte, error) {
	payload := map[string]any{
		"mcp_servers": map[string]any{
			"mdemg": codexMdemgSection(endpoint),
		},
	}
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		return nil, fmt.Errorf("encode codex payload: %w", err)
	}
	return buf.Bytes(), nil
}

func (a *CodexAdapter) MergeInto(existing []byte, endpoint string) ([]byte, string, error) {
	// Empty or nil → create from scratch
	if len(existing) == 0 {
		data, err := a.BuildPayload(endpoint)
		if err != nil {
			return nil, "", err
		}
		return data, "create", nil
	}

	// Parse existing TOML
	var root map[string]any
	if _, err := toml.Decode(string(existing), &root); err != nil {
		return nil, "", fmt.Errorf("parse existing config.toml: %w", err)
	}

	// Ensure mcp_servers key exists
	servers, ok := root["mcp_servers"]
	if !ok {
		root["mcp_servers"] = map[string]any{}
		servers = root["mcp_servers"]
	}

	serversMap, ok := servers.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("mcp_servers is not a table")
	}

	// Set/overwrite the mdemg key
	serversMap["mdemg"] = codexMdemgSection(endpoint)
	root["mcp_servers"] = serversMap

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(root); err != nil {
		return nil, "", fmt.Errorf("encode merged config.toml: %w", err)
	}
	return buf.Bytes(), "merge", nil
}

func (a *CodexAdapter) RemoveFrom(existing []byte) ([]byte, error) {
	if len(existing) == 0 {
		return existing, nil
	}

	var root map[string]any
	if _, err := toml.Decode(string(existing), &root); err != nil {
		return nil, fmt.Errorf("parse existing config.toml: %w", err)
	}

	servers, ok := root["mcp_servers"]
	if !ok {
		// Nothing to remove
		return existing, nil
	}

	serversMap, ok := servers.(map[string]any)
	if !ok {
		return existing, nil
	}

	delete(serversMap, "mdemg")

	// If mcp_servers is now empty, remove the key entirely
	if len(serversMap) == 0 {
		delete(root, "mcp_servers")
	} else {
		root["mcp_servers"] = serversMap
	}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("encode cleaned config.toml: %w", err)
	}
	return buf.Bytes(), nil
}
