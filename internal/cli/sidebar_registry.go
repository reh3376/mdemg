package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// sidebarInstance represents an entry in the sidebar/menubar instance registry.
// Matches the JSON format used by both macOS menubar and Linux sidebar apps.
type sidebarInstance struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ProjectDirectory string `json:"projectDirectory"`
	ServerURL        string `json:"serverURL,omitempty"`
	SpaceID          string `json:"spaceId"`
	AddedAt          string `json:"addedAt"`
	Source           string `json:"source"`
}

// sidebarRegistryPaths returns the paths to all sidebar/menubar instance registry files
// for the current platform. Multiple paths may be returned (e.g., macOS has menubar,
// Linux has sidebar).
func sidebarRegistryPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var paths []string

	switch runtime.GOOS {
	case "darwin":
		paths = append(paths,
			filepath.Join(home, "Library", "Application Support", "com.reh3376.mdemg-menubar", "instances.json"),
		)
	case "linux":
		// XDG_CONFIG_HOME or ~/.config
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			configDir = filepath.Join(home, ".config")
		}
		paths = append(paths,
			filepath.Join(configDir, "mdemg-sidebar", "instances.json"),
		)
	}

	return paths
}

// readSidebarRegistry reads and parses the sidebar instance registry from the given path.
func readSidebarRegistry(path string) ([]sidebarInstance, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var instances []sidebarInstance
	if err := json.Unmarshal(data, &instances); err != nil {
		return nil, err
	}
	return instances, nil
}

// writeSidebarRegistry atomically writes the sidebar instance registry to the given path.
func writeSidebarRegistry(path string, instances []sidebarInstance) error {
	data, err := json.MarshalIndent(instances, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// deregisterFromSidebar removes entries matching the given project directory from
// all sidebar/menubar instance registries. Returns the total number of entries removed.
func deregisterFromSidebar(projectDir string) (int, error) {
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return 0, err
	}

	totalRemoved := 0
	for _, regPath := range sidebarRegistryPaths() {
		instances, err := readSidebarRegistry(regPath)
		if err != nil {
			continue // File may not exist — skip silently
		}

		var filtered []sidebarInstance
		removed := 0
		for _, inst := range instances {
			if inst.ProjectDirectory == absDir {
				removed++
			} else {
				filtered = append(filtered, inst)
			}
		}

		if removed > 0 {
			if err := writeSidebarRegistry(regPath, filtered); err != nil {
				return totalRemoved, err
			}
			totalRemoved += removed
		}
	}

	return totalRemoved, nil
}
