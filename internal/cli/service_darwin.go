//go:build darwin

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	launchd_templates "mdemg/internal/cli/launchd_templates"
)

var launchdServices = []struct {
	Label    string
	Template string
}{
	{"com.mdemg.server", "com.mdemg.server.plist"},
	{"com.mdemg.neural-sidecar", "com.mdemg.neural-sidecar.plist"},
	{"com.mdemg.ingest-claude-md", "com.mdemg.ingest-claude-md.plist"},
	{"com.mdemg.training-export", "com.mdemg.training-export.plist"},
	{"com.mdemg.maintenance", "com.mdemg.maintenance.plist"},
}

type darwinServiceManager struct{}

func newPlatformServiceManager() serviceManager {
	return &darwinServiceManager{}
}

func (m *darwinServiceManager) Install(projectDir, mdemgBin, spaceID string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("create LaunchAgents directory: %w", err)
	}

	// Ensure log directory exists
	if err := os.MkdirAll(filepath.Join(home, ".mdemg", "logs"), 0755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	uid, err := currentUID()
	if err != nil {
		return err
	}

	// Resolve python binary for neural sidecar
	pythonBin := resolvePythonBin(projectDir)

	templateDir := filepath.Join(projectDir, "packaging", "launchd")

	for _, svc := range launchdServices {
		// Try disk first (repo checkout), fall back to embedded templates (Homebrew/binary)
		tmpl, err := os.ReadFile(filepath.Join(templateDir, svc.Template))
		if err != nil {
			tmpl, err = launchd_templates.FS.ReadFile(svc.Template)
			if err != nil {
				return fmt.Errorf("read template %s: %w", svc.Template, err)
			}
		}

		content := string(tmpl)
		content = strings.ReplaceAll(content, "__MDEMG_BIN__", mdemgBin)
		content = strings.ReplaceAll(content, "__PROJECT_DIR__", projectDir)
		content = strings.ReplaceAll(content, "__HOME__", home)
		content = strings.ReplaceAll(content, "__SPACE_ID__", spaceID)
		content = strings.ReplaceAll(content, "__PYTHON_BIN__", pythonBin)

		destPath := filepath.Join(launchAgentsDir, svc.Template)
		if err := os.WriteFile(destPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", destPath, err)
		}

		// Bootstrap the service (modern launchctl API)
		target := fmt.Sprintf("gui/%s", uid)
		_ = exec.Command("launchctl", "bootout", target+"/"+svc.Label).Run() // Remove if exists
		if err := exec.Command("launchctl", "bootstrap", target, destPath).Run(); err != nil {
			return fmt.Errorf("bootstrap %s: %w", svc.Label, err)
		}

		fmt.Printf("Installed and started %s\n", svc.Label)
	}

	return nil
}

func (m *darwinServiceManager) Uninstall() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	uid, err := currentUID()
	if err != nil {
		return err
	}

	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")

	for _, svc := range launchdServices {
		target := fmt.Sprintf("gui/%s/%s", uid, svc.Label)
		_ = exec.Command("launchctl", "bootout", target).Run()

		plistPath := filepath.Join(launchAgentsDir, svc.Template)
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: could not remove %s: %v\n", plistPath, err)
		} else {
			fmt.Printf("Removed %s\n", svc.Label)
		}
	}

	return nil
}

func (m *darwinServiceManager) Status() error {
	uid, err := currentUID()
	if err != nil {
		return err
	}

	fmt.Println("MDEMG Service Status (launchd)")
	fmt.Println("==============================")

	for _, svc := range launchdServices {
		target := fmt.Sprintf("gui/%s/%s", uid, svc.Label)
		out, err := exec.Command("launchctl", "print", target).CombinedOutput()
		if err != nil {
			fmt.Printf("  %-35s  ⚠ not loaded\n", svc.Label)
			continue
		}

		// Parse PID from launchctl print output
		pid := "?"
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "pid = ") {
				pid = strings.TrimPrefix(line, "pid = ")
			}
		}
		if pid == "0" {
			fmt.Printf("  %-35s  stopped\n", svc.Label)
		} else {
			fmt.Printf("  %-35s  running (pid %s)\n", svc.Label, pid)
		}
	}

	return nil
}

func (m *darwinServiceManager) Restart() error {
	uid, err := currentUID()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")

	for _, svc := range launchdServices {
		target := fmt.Sprintf("gui/%s", uid)
		serviceTarget := target + "/" + svc.Label
		plistPath := filepath.Join(launchAgentsDir, svc.Template)

		_ = exec.Command("launchctl", "bootout", serviceTarget).Run()

		if _, err := os.Stat(plistPath); err == nil {
			if err := exec.Command("launchctl", "bootstrap", target, plistPath).Run(); err != nil {
				fmt.Printf("Warning: could not restart %s: %v\n", svc.Label, err)
				continue
			}
			fmt.Printf("Restarted %s\n", svc.Label)
		} else {
			fmt.Printf("  %s — plist not found, skipping\n", svc.Label)
		}
	}

	return nil
}

func (m *darwinServiceManager) Logs(follow bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	logFiles := []string{
		filepath.Join(home, ".mdemg", "logs", "server.log"),
		filepath.Join(home, ".mdemg", "logs", "neural-sidecar.log"),
		filepath.Join(home, ".mdemg", "logs", "ingest-claude-md.log"),
		filepath.Join(home, ".mdemg", "logs", "training-export.log"),
		filepath.Join(home, ".mdemg", "logs", "maintenance.log"),
	}

	if follow {
		args := []string{"-f"}
		for _, f := range logFiles {
			if _, err := os.Stat(f); err == nil {
				args = append(args, f)
			}
		}
		if len(args) == 1 {
			return fmt.Errorf("no log files found in ~/.mdemg/logs/")
		}
		cmd := exec.Command("tail", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	for _, f := range logFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		start := 0
		if len(lines) > 20 {
			start = len(lines) - 20
		}
		fmt.Printf("=== %s (last 20 lines) ===\n", filepath.Base(f))
		for _, line := range lines[start:] {
			if line != "" {
				fmt.Println(line)
			}
		}
		fmt.Println()
	}

	return nil
}

func currentUID() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user: %w", err)
	}
	return u.Uid, nil
}

func resolvePythonBin(projectDir string) string {
	// Check project venv first
	venvPython := filepath.Join(projectDir, "neural", ".venv", "bin", "python3")
	if _, err := os.Stat(venvPython); err == nil {
		return venvPython
	}
	// Fall back to system python
	if path, err := exec.LookPath("python3"); err == nil {
		return path
	}
	return "python3"
}
