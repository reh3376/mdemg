package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// mcpConfigPaths returns all potential MCP config file paths relative to the project directory.
func mcpConfigPaths(projectDir string) []string {
	return []string{
		filepath.Join(projectDir, ".mcp.json"),
		filepath.Join(projectDir, ".claude", "mcp.json"),
		filepath.Join(projectDir, ".cursor", "mcp.json"),
		filepath.Join(projectDir, ".vscode", "mcp.json"),
	}
}

// removeMDEMGFromMCPConfig removes the "mdemg" server entry from an MCP config file.
// Returns whether the entry was removed and whether the file was deleted entirely.
// If the file contains only the mdemg entry, the file is deleted.
// If it contains other servers, only the mdemg entry is removed.
func removeMDEMGFromMCPConfig(path string) (removed bool, fileDeleted bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, err
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return false, false, nil // Malformed JSON — don't touch
	}

	// Look for mcpServers key (used by various IDE MCP configs)
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		return false, false, nil
	}

	// Check if mdemg entry exists
	if _, exists := servers["mdemg"]; !exists {
		return false, false, nil
	}

	delete(servers, "mdemg")

	// If no servers remain, delete the file
	if len(servers) == 0 {
		if err := os.Remove(path); err != nil {
			return false, false, err
		}
		return true, true, nil
	}

	// Write back with remaining servers
	root["mcpServers"] = servers
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, false, err
	}
	out = append(out, '\n')
	if err := os.WriteFile(path, out, 0644); err != nil {
		return false, false, err
	}
	return true, false, nil
}

// cleanupClaudeSettings removes MDEMG hook entries from .claude/settings.local.json.
// Returns true if any entries were removed.
func cleanupClaudeSettings(projectDir string) (bool, error) {
	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return false, nil // Malformed JSON — don't touch
	}

	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return false, nil
	}

	modified := false
	for event, groups := range hooks {
		groupList, ok := groups.([]any)
		if !ok {
			continue
		}

		var filteredGroups []any
		for _, group := range groupList {
			groupMap, ok := group.(map[string]any)
			if !ok {
				filteredGroups = append(filteredGroups, group)
				continue
			}
			hookList, ok := groupMap["hooks"].([]any)
			if !ok {
				filteredGroups = append(filteredGroups, group)
				continue
			}

			var filteredHooks []any
			for _, h := range hookList {
				hMap, ok := h.(map[string]any)
				if !ok {
					filteredHooks = append(filteredHooks, h)
					continue
				}
				cmd, _ := hMap["command"].(string)
				if containsMDEMGRef(cmd) {
					modified = true
					continue // Remove this hook
				}
				filteredHooks = append(filteredHooks, h)
			}

			if len(filteredHooks) > 0 {
				groupMap["hooks"] = filteredHooks
				filteredGroups = append(filteredGroups, groupMap)
			} else {
				modified = true // Entire group removed
			}
		}

		if len(filteredGroups) > 0 {
			hooks[event] = filteredGroups
		} else {
			delete(hooks, event)
			modified = true
		}
	}

	if !modified {
		return false, nil
	}

	// If hooks map is now empty, remove the key
	if len(hooks) == 0 {
		delete(root, "hooks")
	}

	// If root is now empty, remove the file
	if len(root) == 0 {
		if err := os.Remove(settingsPath); err != nil {
			return true, err
		}
		return true, nil
	}

	// Write back
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return true, err
	}
	out = append(out, '\n')
	return true, os.WriteFile(settingsPath, out, 0644)
}

// containsMDEMGRef checks if a command string references mdemg.
func containsMDEMGRef(cmd string) bool {
	for _, term := range []string{"mdemg", "MDEMG"} {
		for i := 0; i <= len(cmd)-len(term); i++ {
			if cmd[i:i+len(term)] == term {
				return true
			}
		}
	}
	return false
}
