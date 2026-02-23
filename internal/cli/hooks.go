package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const mdemgHookMarker = "# MDEMG"

func newHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Manage MDEMG git hooks",
		Long: `Manage MDEMG git hooks for automatic code ingestion.

Subcommands:
  install    Install git hooks (post-commit)
  uninstall  Remove MDEMG-installed git hooks
  list       Show current hook status`,
	}

	cmd.AddCommand(newHooksInstallCmd())
	cmd.AddCommand(newHooksUninstallCmd())
	cmd.AddCommand(newHooksListCmd())

	return cmd
}

func newHooksInstallCmd() *cobra.Command {
	var (
		hookType string
		force    bool
		spaceID  string
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install MDEMG git hooks",
		Long: `Install MDEMG git hooks for automatic code ingestion.

The post-commit hook triggers incremental ingestion after each commit,
running in the background to avoid blocking your workflow.

Examples:
  mdemg hooks install                     # Install with default space ID
  mdemg hooks install --space-id myproj   # Install with custom space ID
  mdemg hooks install --force             # Overwrite existing hook`,
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

			switch hookType {
			case "git", "all":
				if err := InstallGitHook(cwd, spaceID, force); err != nil {
					return err
				}
				fmt.Println("Installed .git/hooks/post-commit")
			default:
				return fmt.Errorf("unknown hook type: %s (supported: git, all)", hookType)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&hookType, "type", "git", "Hook type to install (git|all)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing hooks")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space ID for ingestion (default: directory name)")

	return cmd
}

func newHooksUninstallCmd() *cobra.Command {
	var hookType string

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove MDEMG git hooks",
		Long: `Remove MDEMG-installed git hooks.

Only removes hooks that were installed by MDEMG (identified by the
"# MDEMG" marker comment). Non-MDEMG hooks are left untouched.

Examples:
  mdemg hooks uninstall          # Remove MDEMG git hooks
  mdemg hooks uninstall --type all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			switch hookType {
			case "git", "all":
				removed, err := UninstallGitHook(cwd)
				if err != nil {
					return err
				}
				if removed {
					fmt.Println("Removed .git/hooks/post-commit")
				} else {
					fmt.Println("No MDEMG hooks found to remove")
				}
			default:
				return fmt.Errorf("unknown hook type: %s (supported: git, all)", hookType)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&hookType, "type", "git", "Hook type to uninstall (git|all)")

	return cmd
}

func newHooksListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show MDEMG hook status",
		Long: `Show the current status of MDEMG hooks in this repository.

Reports whether git hooks are installed, and whether the standalone
hook script is present.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			fmt.Println("MDEMG Hook Status")
			fmt.Println("=================")

			// Check git repo
			gitDir := filepath.Join(cwd, ".git")
			if _, err := os.Stat(gitDir); os.IsNotExist(err) {
				fmt.Println("  Not a git repository")
				return nil
			}

			// Check post-commit hook
			hookPath := filepath.Join(cwd, ".git", "hooks", "post-commit")
			if _, err := os.Stat(hookPath); os.IsNotExist(err) {
				fmt.Println("  post-commit hook:  not installed")
			} else if isMdemgHook(hookPath) {
				fmt.Println("  post-commit hook:  installed (mdemg)")
			} else {
				fmt.Println("  post-commit hook:  installed (non-mdemg)")
			}

			// Check for standalone hook script
			scriptPath := filepath.Join(cwd, "scripts", "mdemg-git-hook")
			if _, err := os.Stat(scriptPath); err == nil {
				fmt.Println("  hook script:       present (scripts/mdemg-git-hook)")
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
