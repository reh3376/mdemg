//go:build darwin

package cli

import (
	"fmt"
	"log/slog"
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
	// Optional services are skipped at install time if their prerequisites
	// are missing. Required services fail Install() loudly when their
	// template can't be rendered or their binary can't be located.
	Optional bool
}{
	{"com.mdemg.server", "com.mdemg.server.plist", false},
	{"com.mdemg.neural-sidecar", "com.mdemg.neural-sidecar.plist", false},
	{"com.mdemg.ingest-claude-md", "com.mdemg.ingest-claude-md.plist", false},
	{"com.mdemg.training-export", "com.mdemg.training-export.plist", false},
	{"com.mdemg.maintenance", "com.mdemg.maintenance.plist", false},
	// Phase 13.5 cutover (2026-05-03) — production LLM runtime is
	// llama.cpp llama-server (port 8102, GGUF Q5_K_M). Replaces mlx_lm.server
	// (port 8101) which exhibited unbounded KV-cache → Metal-OOM crashes.
	// Per always-on policy (memory: feedback_mlx_required_when_mdemg_running.md
	// — name predates Phase 13.5 rename), Install() fails loudly when
	// `llama-server` cannot be resolved (PATH or MDEMG_LLAMA_SERVER_BIN env
	// override). Hosts without llama-server must explicitly set
	// MDEMG_ALLOW_NO_LLM=1 at startup to bypass.
	{"com.mdemg.llama-server", "com.mdemg.llama-server.plist", false},
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

	// Resolve llama-server binary + default model path for the llama-server
	// plist. Values are looked up once; install fails loudly if the required
	// llama-server isn't found.
	llamaServerBin, llamaServerFound := resolveLlamaServerBin()
	modelPath := resolveMDEMGModelPath(projectDir)

	// Phase 13.5 migration: if a pre-cutover com.mdemg.mlx-server plist is
	// already bootstrapped, bootout and disable it before installing the new
	// llama-server agent. Two services on different ports won't conflict, but
	// the old one is non-functional under the Phase 13.5 stack and would
	// continue restarting under launchd KeepAlive forever, burning resources.
	migrateLegacyMLXServerPlist(launchAgentsDir, uid)

	templateDir := filepath.Join(projectDir, "packaging", "launchd")

	for _, svc := range launchdServices {
		// Phase 13.5 — llama-server is required (Optional=false). Fail loudly
		// with actionable guidance if llama-server can't be located.
		if svc.Label == "com.mdemg.llama-server" && !llamaServerFound {
			return fmt.Errorf(
				"install failed: llama-server not found on PATH (per always-on LLM policy, llama-server is required for mdemg).\n"+
					"  Resolve via one of:\n"+
					"    1. Set MDEMG_LLAMA_SERVER_BIN=/path/to/llama-server in your shell or .env\n"+
					"    2. Install llama.cpp via Homebrew: `brew install llama.cpp`\n"+
					"    3. (Emergency only) Skip this install with `MDEMG_ALLOW_NO_LLM=1 mdemg start ...` —\n"+
					"       mdemg will refuse to serve LLM-dependent endpoints; intended for Linux/Docker-only setups.")
		}
		if svc.Optional && !llamaServerFound {
			// Reserved for future optional services. llama-server is required.
			fmt.Printf("Skipping optional service %s\n", svc.Label)
			continue
		}

		// Try disk first (repo checkout), fall back to embedded templates (Homebrew/binary)
		tmpl, err := os.ReadFile(filepath.Join(templateDir, svc.Template))
		if err != nil {
			tmpl, err = launchd_templates.FS.ReadFile(svc.Template)
			if err != nil {
				return fmt.Errorf("read template %s: %w", svc.Template, err)
			}
		}

		content := renderLaunchdTemplate(string(tmpl), mdemgBin, projectDir, home, spaceID, pythonBin, llamaServerBin, modelPath)

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

// resolveLlamaServerBin locates llama-server: env override
// (MDEMG_LLAMA_SERVER_BIN), then deprecated alias (MDEMG_MLX_LM_BIN, kept for
// ≥1 release cycle per the Phase 13.6 deprecation pattern), then PATH lookup
// for `llama-server`. Returns ("", false) if nothing resolves.
func resolveLlamaServerBin() (string, bool) {
	if env := os.Getenv("MDEMG_LLAMA_SERVER_BIN"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, true
		}
	}
	if env := os.Getenv("MDEMG_MLX_LM_BIN"); env != "" {
		if _, err := os.Stat(env); err == nil {
			slog.Warn("config: env var deprecated, please rename",
				"old", "MDEMG_MLX_LM_BIN", "new", "MDEMG_LLAMA_SERVER_BIN")
			return env, true
		}
	}
	if path, err := exec.LookPath("llama-server"); err == nil {
		return path, true
	}
	return "", false
}

// resolveMDEMGModelPath returns the model path for the llama-server plist.
// Order: MDEMG_MODEL_PATH env override → the FT-RECURSIVE-003 serving
// indirection symlink when it exists (promotion/rollback retarget it without
// touching the plist) → the canonical Phase 13.5 GGUF. llama-server takes a
// `.gguf` filepath (not a HF-style directory).
func resolveMDEMGModelPath(projectDir string) string {
	if env := os.Getenv("MDEMG_MODEL_PATH"); env != "" {
		return env
	}
	serving := filepath.Join(projectDir, ".local-models", "serving", "current.gguf")
	if _, err := os.Lstat(serving); err == nil {
		return serving
	}
	return filepath.Join(projectDir, ".local-models", "mdemg-llm-v1-gguf", "mdemg-llm-v1.Q5_K_M.gguf")
}

// migrateLegacyMLXServerPlist boots out and disables a pre-Phase-13.5
// com.mdemg.mlx-server LaunchAgent if one is still bootstrapped, then renames
// the plist file to .disabled-phase13_5 (matching the manual operator
// convention). Best-effort — failures are logged but never block the install.
func migrateLegacyMLXServerPlist(launchAgentsDir, uid string) {
	const legacyLabel = "com.mdemg.mlx-server"
	const legacyPlist = "com.mdemg.mlx-server.plist"

	plistPath := filepath.Join(launchAgentsDir, legacyPlist)
	if _, err := os.Stat(plistPath); err != nil {
		return // not present, nothing to migrate
	}

	target := fmt.Sprintf("gui/%s/%s", uid, legacyLabel)
	_ = exec.Command("launchctl", "bootout", target).Run()

	disabledPath := plistPath + ".disabled-phase13_5"
	if err := os.Rename(plistPath, disabledPath); err != nil {
		fmt.Printf("Warning: could not rename %s to %s: %v (Phase 13.5 migration)\n",
			plistPath, disabledPath, err)
		return
	}
	fmt.Printf("Migrated %s → %s (decommissioned in Phase 13.5)\n", legacyLabel, filepath.Base(disabledPath))
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

// RefreshInstalledDarwinServices re-renders ALREADY-INSTALLED mdemg
// LaunchAgent plists from the new binary's embedded templates and reloads
// them (MAINT-LIVE-001). Without this, plist fixes ship in releases but
// never reach installed machines — the maintenance --dry-run override sat
// unreachable next to upgraded binaries. Only refreshes services whose
// plist already exists (upgrade must not install new services); returns
// the count refreshed.
func RefreshInstalledDarwinServices(projectDir, mdemgBin, spaceID string) (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("get home directory: %w", err)
	}
	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")
	uid, err := currentUID()
	if err != nil {
		return 0, err
	}

	pythonBin := resolvePythonBin(projectDir)
	llamaServerBin, _ := resolveLlamaServerBin()
	modelPath := resolveMDEMGModelPath(projectDir)
	templateDir := filepath.Join(projectDir, "packaging", "launchd")

	refreshed := 0
	for _, svc := range launchdServices {
		destPath := filepath.Join(launchAgentsDir, svc.Template)
		if _, err := os.Stat(destPath); err != nil {
			continue // not installed — upgrade refreshes, never installs
		}

		// Disk first (repo checkout), embedded fallback (Homebrew/binary).
		tmpl, err := os.ReadFile(filepath.Join(templateDir, svc.Template))
		if err != nil {
			tmpl, err = launchd_templates.FS.ReadFile(svc.Template)
			if err != nil {
				fmt.Printf("  skip %s: template unavailable: %v\n", svc.Label, err)
				continue
			}
		}

		content := renderLaunchdTemplate(string(tmpl), mdemgBin, projectDir, home, spaceID, pythonBin, llamaServerBin, modelPath)
		if err := os.WriteFile(destPath, []byte(content), 0644); err != nil { //nolint:gosec // launchd plists are world-readable by convention
			fmt.Printf("  skip %s: write failed: %v\n", svc.Label, err)
			continue
		}

		target := fmt.Sprintf("gui/%s", uid)
		_ = exec.Command("launchctl", "bootout", target+"/"+svc.Label).Run()
		if err := exec.Command("launchctl", "bootstrap", target, destPath).Run(); err != nil {
			fmt.Printf("  warn %s: bootstrap failed: %v\n", svc.Label, err)
			continue
		}
		fmt.Printf("  refreshed %s\n", svc.Label)
		refreshed++
	}
	return refreshed, nil
}

// renderLaunchdTemplate applies the standard plist placeholder substitutions
// (single source for Install + Refresh — drift here is how the sidecar's raw
// __HOME__ copy exit-78'd during HOOKSYNC-001 live smoke).
func renderLaunchdTemplate(tmpl, mdemgBin, projectDir, home, spaceID, pythonBin, llamaServerBin, modelPath string) string {
	content := tmpl
	content = strings.ReplaceAll(content, "__MDEMG_BIN__", mdemgBin)
	content = strings.ReplaceAll(content, "__PROJECT_DIR__", projectDir)
	content = strings.ReplaceAll(content, "__HOME__", home)
	content = strings.ReplaceAll(content, "__SPACE_ID__", spaceID)
	content = strings.ReplaceAll(content, "__PYTHON_BIN__", pythonBin)
	content = strings.ReplaceAll(content, "__LLAMA_SERVER_BIN__", llamaServerBin)
	content = strings.ReplaceAll(content, "__MDEMG_MODEL_PATH__", modelPath)
	return content
}
