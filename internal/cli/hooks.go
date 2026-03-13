package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	hook_templates "mdemg/internal/cli/hook_templates"

	"github.com/spf13/cobra"
)

const mdemgHookMarker = "# MDEMG"

// claudeHookEntry maps a template filename to its Claude Code event name and timeout.
type claudeHookEntry struct {
	Template string // filename in hook_templates FS
	Event    string // Claude Code hook event
	Timeout  int    // timeout in seconds
}

// claudeHookFiles returns the platform-appropriate hook entries.
// On Windows, PowerShell scripts (.ps1) are used; on Unix, bash scripts (.sh).
func claudeHookFiles() []claudeHookEntry {
	if runtime.GOOS == "windows" {
		return []claudeHookEntry{
			{"prompt-context.ps1", "UserPromptSubmit", 12},
			{"session-start.ps1", "SessionStart", 15},
		}
	}
	return []claudeHookEntry{
		{"prompt-context.sh", "UserPromptSubmit", 12},
		{"session-start.sh", "SessionStart", 15},
	}
}

func newHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Manage MDEMG hooks",
		Long: `Manage MDEMG hooks for automatic code ingestion and Jiminy guidance.

Subcommands:
  install    Install hooks (git, claude, or all)
  uninstall  Remove MDEMG-installed hooks
  list       Show current hook status`,
	}

	cmd.AddCommand(newHooksInstallCmd())
	cmd.AddCommand(newHooksUninstallCmd())
	cmd.AddCommand(newHooksListCmd())

	return cmd
}

func newHooksInstallCmd() *cobra.Command {
	var (
		hookType  string
		force     bool
		spaceID   string
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install MDEMG hooks",
		Long: `Install MDEMG hooks for automatic code ingestion and Jiminy guidance.

Hook types:
  git    — post-commit hook for incremental code ingestion
  claude — Claude Code hooks for CMS recall, Jiminy inner voice, and session memory
  all    — both git and claude hooks

Examples:
  mdemg hooks install                          # Install git hooks (default)
  mdemg hooks install --type claude            # Install Claude Code hooks
  mdemg hooks install --type all               # Install all hooks
  mdemg hooks install --type claude --force    # Overwrite existing Claude hooks
  mdemg hooks install --space-id myproj        # Custom space ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			if spaceID == "" {
				spaceID = resolveSpaceID(cmd)
			}
			if spaceID == "" {
				spaceID = filepath.Base(cwd)
			}
			if serverURL == "" {
				serverURL = "http://localhost:9999"
			}

			switch hookType {
			case "git":
				if err := InstallGitHook(cwd, spaceID, force); err != nil {
					return err
				}
				fmt.Println("Installed .git/hooks/post-commit")
			case "claude":
				installed, err := InstallClaudeHooks(cwd, spaceID, serverURL, force)
				if err != nil {
					return err
				}
				for _, path := range installed {
					fmt.Printf("Installed %s\n", path)
				}
			case "all":
				if err := InstallGitHook(cwd, spaceID, force); err != nil {
					return err
				}
				fmt.Println("Installed .git/hooks/post-commit")
				installed, err := InstallClaudeHooks(cwd, spaceID, serverURL, force)
				if err != nil {
					return err
				}
				for _, path := range installed {
					fmt.Printf("Installed %s\n", path)
				}
			default:
				return fmt.Errorf("unknown hook type: %s (supported: git, claude, all)", hookType)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&hookType, "type", "git", "Hook type to install (git|claude|all)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing hooks")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space ID for ingestion (default: directory name)")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MDEMG server URL (default: http://localhost:9999)")

	return cmd
}

func newHooksUninstallCmd() *cobra.Command {
	var hookType string

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove MDEMG hooks",
		Long: `Remove MDEMG-installed hooks.

Only removes hooks that were installed by MDEMG (identified by the
"# MDEMG" marker comment). Non-MDEMG hooks are left untouched.

Examples:
  mdemg hooks uninstall                # Remove MDEMG git hooks
  mdemg hooks uninstall --type claude  # Remove Claude Code hooks
  mdemg hooks uninstall --type all     # Remove all MDEMG hooks`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			switch hookType {
			case "git":
				removed, err := UninstallGitHook(cwd)
				if err != nil {
					return err
				}
				if removed {
					fmt.Println("Removed .git/hooks/post-commit")
				} else {
					fmt.Println("No MDEMG git hooks found to remove")
				}
			case "claude":
				removed, err := UninstallClaudeHooks(cwd)
				if err != nil {
					return err
				}
				if removed > 0 {
					fmt.Printf("Removed %d Claude Code hook(s)\n", removed)
				} else {
					fmt.Println("No MDEMG Claude Code hooks found to remove")
				}
			case "all":
				gitRemoved, err := UninstallGitHook(cwd)
				if err != nil {
					fmt.Printf("  git: %v\n", err)
				} else if gitRemoved {
					fmt.Println("Removed .git/hooks/post-commit")
				}
				claudeRemoved, err := UninstallClaudeHooks(cwd)
				if err != nil {
					fmt.Printf("  claude: %v\n", err)
				} else if claudeRemoved > 0 {
					fmt.Printf("Removed %d Claude Code hook(s)\n", claudeRemoved)
				}
			default:
				return fmt.Errorf("unknown hook type: %s (supported: git, claude, all)", hookType)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&hookType, "type", "git", "Hook type to uninstall (git|claude|all)")

	return cmd
}

func newHooksListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show MDEMG hook status",
		Long: `Show the current status of MDEMG hooks in this project.

Reports whether git hooks and Claude Code hooks are installed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			fmt.Println("MDEMG Hook Status")
			fmt.Println("=================")

			// Git hooks
			gitDir := filepath.Join(cwd, ".git")
			if _, err := os.Stat(gitDir); os.IsNotExist(err) {
				fmt.Println("  git repo:          not detected")
			} else {
				hookPath := filepath.Join(cwd, ".git", "hooks", "post-commit")
				if _, err := os.Stat(hookPath); os.IsNotExist(err) {
					fmt.Println("  post-commit hook:  not installed")
				} else if isMdemgHook(hookPath) {
					fmt.Println("  post-commit hook:  installed (mdemg)")
				} else {
					fmt.Println("  post-commit hook:  installed (non-mdemg)")
				}
			}

			// Claude Code hooks
			for _, hf := range claudeHookFiles() {
				hookPath := filepath.Join(cwd, ".claude", "hooks", hf.Template)
				if _, err := os.Stat(hookPath); os.IsNotExist(err) {
					fmt.Printf("  %-20s not installed\n", hf.Template+":")
				} else if isMdemgHook(hookPath) {
					fmt.Printf("  %-20s installed (mdemg)\n", hf.Template+":")
				} else {
					fmt.Printf("  %-20s installed (non-mdemg)\n", hf.Template+":")
				}
			}

			// Settings registration
			settingsPath := filepath.Join(cwd, ".claude", "settings.local.json")
			if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
				fmt.Println("  settings.local:    not found")
			} else {
				fmt.Println("  settings.local:    present")
			}

			return nil
		},
	}
}

// InstallGitHook creates a post-commit hook that calls mdemg ingest.
// If force is false and a hook already exists, it returns an error.
// This is the shared implementation used by both `mdemg init` and `mdemg hooks install`.
func InstallGitHook(repoDir, spaceID string, force bool) error {
	hookDir := filepath.Join(repoDir, ".git", "hooks")
	hookPath := filepath.Join(hookDir, "post-commit")

	// Check for .git directory
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository: %s", repoDir)
	}

	// Check for existing hook
	if _, err := os.Stat(hookPath); err == nil {
		if !force {
			return fmt.Errorf("post-commit hook already exists (use --force to overwrite)")
		}
	}

	hook := fmt.Sprintf(`#!/bin/bash
# MDEMG auto-ingestion hook (installed by mdemg hooks install)
# Set MDEMG_DISABLED=true to temporarily disable

if [ "$MDEMG_DISABLED" = "true" ]; then exit 0; fi

REPO_ROOT=$(git rev-parse --show-toplevel)
SPACE_ID="${MDEMG_SPACE_ID:-%s}"

# Find mdemg binary
if command -v mdemg &> /dev/null; then
    MDEMG_BIN="mdemg"
elif [ -f "$REPO_ROOT/bin/mdemg" ]; then
    MDEMG_BIN="$REPO_ROOT/bin/mdemg"
else
    exit 0  # silently skip if mdemg not found
fi

# Run incremental ingestion in background
nohup "$MDEMG_BIN" ingest \
    --path "$REPO_ROOT" \
    --space-id "$SPACE_ID" \
    --incremental \
    --since "HEAD~1" \
    --archive-deleted \
    --quiet \
    > /dev/null 2>&1 &
`, spaceID)

	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(hookPath, []byte(hook), 0755)
}

// InstallClaudeHooks installs Claude Code hooks (prompt-context, session-start)
// from embedded templates and registers them in .claude/settings.local.json.
// Returns the list of installed file paths.
func InstallClaudeHooks(dir, spaceID, serverURL string, force bool) ([]string, error) {
	hookDir := filepath.Join(dir, ".claude", "hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return nil, fmt.Errorf("create hook directory: %w", err)
	}

	var installed []string

	for _, hf := range claudeHookFiles() {
		destPath := filepath.Join(hookDir, hf.Template)

		// Check for existing hook
		if _, err := os.Stat(destPath); err == nil {
			if !force {
				if isMdemgHook(destPath) {
					continue // Already installed by MDEMG, skip silently
				}
				return installed, fmt.Errorf("%s already exists (non-mdemg, use --force to overwrite)", destPath)
			}
		}

		// Read template from embedded FS
		tmpl, err := hook_templates.FS.ReadFile(hf.Template)
		if err != nil {
			return installed, fmt.Errorf("read template %s: %w", hf.Template, err)
		}

		// Substitute placeholders
		content := string(tmpl)
		content = strings.ReplaceAll(content, "{{SPACE_ID}}", spaceID)
		content = strings.ReplaceAll(content, "{{MDEMG_URL}}", serverURL)

		// Write hook script
		if err := os.WriteFile(destPath, []byte(content), 0755); err != nil {
			return installed, fmt.Errorf("write %s: %w", destPath, err)
		}
		installed = append(installed, filepath.Join(".claude", "hooks", hf.Template))
	}

	// Register hooks in settings.local.json
	settingsPath := filepath.Join(dir, ".claude", "settings.local.json")
	if err := mergeClaudeSettings(settingsPath, hookDir); err != nil {
		return installed, fmt.Errorf("register hooks in settings: %w", err)
	}
	installed = append(installed, ".claude/settings.local.json (hooks registered)")

	return installed, nil
}

// UninstallClaudeHooks removes MDEMG-installed Claude Code hooks.
// Returns the count of hooks removed.
func UninstallClaudeHooks(dir string) (int, error) {
	removed := 0
	for _, hf := range claudeHookFiles() {
		hookPath := filepath.Join(dir, ".claude", "hooks", hf.Template)
		if _, err := os.Stat(hookPath); os.IsNotExist(err) {
			continue
		}
		if !isMdemgHook(hookPath) {
			continue // Not ours, don't touch
		}
		if err := os.Remove(hookPath); err != nil {
			return removed, fmt.Errorf("remove %s: %w", hookPath, err)
		}
		removed++
	}
	return removed, nil
}

// UninstallGitHook removes the post-commit hook if it was installed by MDEMG.
// Returns true if a hook was removed, false if no MDEMG hook was found.
func UninstallGitHook(repoDir string) (bool, error) {
	hookPath := filepath.Join(repoDir, ".git", "hooks", "post-commit")

	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		return false, nil
	}

	if !isMdemgHook(hookPath) {
		return false, fmt.Errorf("post-commit hook exists but was not installed by MDEMG (no '%s' marker)", mdemgHookMarker)
	}

	if err := os.Remove(hookPath); err != nil {
		return false, fmt.Errorf("remove hook: %w", err)
	}
	return true, nil
}

// isMdemgHook checks if the given hook file was installed by MDEMG
// by looking for the marker comment in the first few lines.
func isMdemgHook(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := 0
	for scanner.Scan() && lines < 5 {
		if strings.Contains(scanner.Text(), mdemgHookMarker) {
			return true
		}
		lines++
	}
	return false
}

// mergeClaudeSettings reads or creates .claude/settings.local.json and ensures
// MDEMG hooks are registered. Preserves existing settings (permissions, other hooks).
func mergeClaudeSettings(settingsPath, hookDir string) error {
	var root map[string]interface{}

	// Read existing settings or start fresh
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read settings: %w", err)
		}
		root = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse settings: %w", err)
		}
	}

	// Get or create hooks map
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		hooks = make(map[string]interface{})
		root["hooks"] = hooks
	}

	// Register each hook event
	for _, hf := range claudeHookFiles() {
		hookPath := filepath.Join(hookDir, hf.Template)

		// On Windows, .ps1 scripts need powershell.exe invocation
		hookCommand := hookPath
		if strings.HasSuffix(hf.Template, ".ps1") {
			hookCommand = fmt.Sprintf("powershell.exe -ExecutionPolicy Bypass -File \"%s\"", hookPath)
		}

		// Build the hook entry
		hookEntry := map[string]interface{}{
			"type":    "command",
			"command": hookCommand,
			"timeout": hf.Timeout,
		}

		// Check if this event already has hooks registered
		existing, ok := hooks[hf.Event].([]interface{})
		if !ok {
			// No existing hooks for this event — create fresh
			hooks[hf.Event] = []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{hookEntry},
				},
			}
			continue
		}

		// Check if MDEMG hook is already registered (by command path)
		alreadyRegistered := false
		for _, group := range existing {
			groupMap, ok := group.(map[string]interface{})
			if !ok {
				continue
			}
			groupHooks, ok := groupMap["hooks"].([]interface{})
			if !ok {
				continue
			}
			for _, h := range groupHooks {
				hMap, ok := h.(map[string]interface{})
				if !ok {
					continue
				}
				cmd, _ := hMap["command"].(string)
				if strings.Contains(cmd, "mdemg") || strings.Contains(cmd, "MDEMG") {
					// Update existing entry in place
					hMap["command"] = hookCommand
					hMap["timeout"] = hf.Timeout
					alreadyRegistered = true
					break
				}
			}
			if alreadyRegistered {
				break
			}
		}

		if !alreadyRegistered {
			// Append new hook group
			existing = append(existing, map[string]interface{}{
				"hooks": []interface{}{hookEntry},
			})
			hooks[hf.Event] = existing
		}
	}

	// Write back with 2-space indentation
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	out = append(out, '\n')

	// Ensure .claude directory exists
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	return os.WriteFile(settingsPath, out, 0644)
}
