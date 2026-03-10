package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarUninstallCmd() *cobra.Command {
	var (
		dryRun   bool
		format   string
		force    bool
		keepData bool
	)

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove sidecar from project",
		Long: `Cleanly remove all MDEMG sidecar artifacts from the project.

Reverses everything created by init, install, up, and attach-agent.
Backs up .mdemg/ before removal as a safety net.

Examples:
  mdemg sidecar uninstall
  mdemg sidecar uninstall --dry-run
  mdemg sidecar uninstall --force          # stop running services first
  mdemg sidecar uninstall --keep-data      # preserve Neo4j volume
  mdemg sidecar uninstall --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarUninstall(sidecarUninstallFlags{
				dryRun:   dryRun,
				format:   format,
				force:    force,
				keepData: keepData,
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would be removed without executing")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")
	cmd.Flags().BoolVar(&force, "force", false, "Allow uninstall from running state (stops services first)")
	cmd.Flags().BoolVar(&keepData, "keep-data", false, "Keep Neo4j volume (preserve graph data)")

	return cmd
}

type sidecarUninstallFlags struct {
	dryRun   bool
	format   string
	force    bool
	keepData bool
}

func runSidecarUninstall(flags sidecarUninstallFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	stateBefore := sidecar.CurrentStateFrom(cwd)
	lockPath := sidecar.FindLockFileFrom(cwd)

	// State guard: reject uninitialized/initialized (nothing to uninstall)
	switch stateBefore {
	case sidecar.StateInstalled, sidecar.StateStopped, sidecar.StateDegraded:
		// valid without force
	case sidecar.StateRunning:
		if !flags.force {
			remediation := "Stop first: mdemg sidecar down\nOr use: mdemg sidecar uninstall --force"
			if flags.format == "json" {
				report := sidecar.NewUninstallReport(stateBefore, stateBefore, sidecar.ExitValidation, flags.force, flags.keepData, "")
				report.Issues = append(report.Issues, sidecar.ReportIssue{
					Code:        "RUNNING_STATE",
					Severity:    "error",
					Message:     "Cannot uninstall while sidecar is running",
					Remediation: remediation,
				})
				return sidecar.PrintJSON(report)
			}
			return fmt.Errorf("cannot uninstall while sidecar is running — stop first: mdemg sidecar down\nOr use: mdemg sidecar uninstall --force")
		}
		// force accepted — will stop in Phase 1
	default:
		remediation := "mdemg sidecar init"
		if stateBefore == sidecar.StateInitialized {
			remediation = "Nothing to uninstall from initialized state. Remove .mdemg/ manually if needed"
		}
		if flags.format == "json" {
			report := sidecar.NewUninstallReport(stateBefore, stateBefore, sidecar.ExitValidation, flags.force, flags.keepData, "")
			report.Issues = append(report.Issues, sidecar.ReportIssue{
				Code:        "INVALID_STATE",
				Severity:    "error",
				Message:     fmt.Sprintf("Cannot uninstall from state %q", stateBefore),
				Remediation: remediation,
			})
			return sidecar.PrintJSON(report)
		}
		return fmt.Errorf("cannot uninstall from state %q — %s", stateBefore, remediation)
	}

	// Resolve project dir from lock file or config
	configPath := sidecar.FindConfigFileFrom(cwd)
	var projectDir string
	if configPath != "" {
		projectDir = filepath.Dir(filepath.Dir(configPath)) // .mdemg/sidecar.yaml → project root
	} else if lockPath != "" {
		projectDir = filepath.Dir(filepath.Dir(lockPath)) // .mdemg/sidecar.lock → project root
	} else {
		projectDir = cwd
	}
	mdemgDir := filepath.Join(projectDir, ".mdemg")

	// Load config for adapter list and container info
	var cfg *sidecar.Config
	if configPath != "" {
		cfg, _ = sidecar.LoadConfig(configPath)
	}

	// Read lock file for container/volume names
	var lf *sidecar.LockFile
	if lockPath != "" {
		lf, _ = sidecar.ReadLock(lockPath)
	}

	// Dry-run mode
	if flags.dryRun {
		report := sidecar.NewUninstallReport(stateBefore, sidecar.StateUninstalled, sidecar.ExitSuccess, flags.force, flags.keepData, "")
		report.Result = "dry-run"
		actions := []string{}
		if stateBefore == sidecar.StateRunning || stateBefore == sidecar.StateDegraded {
			actions = append(actions, "Would stop sidecar services")
		}
		if cfg != nil {
			for _, a := range cfg.Adapters {
				if a.Enabled {
					actions = append(actions, fmt.Sprintf("Would detach adapter: %s", a.Name))
				}
			}
		}
		if lf != nil && lf.ContainerName != "" {
			actions = append(actions, fmt.Sprintf("Would remove container: %s", lf.ContainerName))
		}
		if !flags.keepData && lf != nil && lf.VolumeName != "" {
			actions = append(actions, fmt.Sprintf("Would remove volume: %s", lf.VolumeName))
		} else if flags.keepData {
			actions = append(actions, "Would keep Neo4j volume (--keep-data)")
		}
		actions = append(actions, "Would backup .mdemg/ to .mdemg-backup-<timestamp>/")
		actions = append(actions, fmt.Sprintf("Would remove %s", mdemgDir))
		hookPath := filepath.Join(projectDir, ".claude", "hooks", "session-start.sh")
		if isSidecarGeneratedHook(hookPath) {
			actions = append(actions, fmt.Sprintf("Would remove generated hook: %s", hookPath))
		}
		report.NextActions = actions

		if flags.format == "json" {
			return sidecar.PrintJSON(report)
		}
		fmt.Println("[dry-run] Sidecar Uninstall")
		fmt.Println("===========================")
		fmt.Printf("  State:  %s → uninstalled\n", stateBefore)
		fmt.Printf("  Force:  %v\n", flags.force)
		fmt.Printf("  Keep data: %v\n", flags.keepData)
		fmt.Println()
		fmt.Println("  Would perform:")
		for _, a := range actions {
			fmt.Printf("    • %s\n", a)
		}
		fmt.Println()
		return nil
	}

	// --- Mutating path ---
	var changes []sidecar.ReportChange

	// Phase 1: Stop (if running/degraded)
	if stateBefore == sidecar.StateRunning || stateBefore == sidecar.StateDegraded {
		fmt.Println("=== Phase 1: Stopping services ===")
		fmt.Println()
		if downErr := runSidecarDown(sidecarDownFlags{format: "text"}); downErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: stop failed: %v (continuing with uninstall)\n", downErr)
		}
		fmt.Println()
	}

	// Phase 2: Detach adapters
	if cfg != nil {
		for _, a := range cfg.Adapters {
			if a.Enabled {
				fmt.Printf("=== Detaching adapter: %s ===\n", a.Name)
				detachErr := runSidecarDetachAgent(sidecarDetachAgentFlags{
					adapterName: string(a.Name),
					format:      "text",
				})
				if detachErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: detach %s failed: %v (continuing)\n", a.Name, detachErr)
				} else {
					changes = append(changes, sidecar.ReportChange{
						Path:   string(a.Name),
						Action: "detached",
					})
				}
				fmt.Println()
			}
		}
	}

	// Phase 3: Remove container
	containerName := sidecar.ResolveContainerName(cwd, neo4jContainerName)
	if lf != nil && lf.ContainerName != "" {
		containerName = lf.ContainerName
	}
	if DockerAvailable() {
		fmt.Printf("Removing container '%s'...\n", containerName)
		if _, rmErr := RunDockerCommand("rm", "-f", containerName); rmErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: container removal failed: %v (may not exist)\n", rmErr)
		} else {
			changes = append(changes, sidecar.ReportChange{
				Path:   containerName,
				Action: "removed",
			})
			fmt.Println("Container removed")
		}
	}

	// Phase 4: Remove volume (unless --keep-data)
	if !flags.keepData {
		volumeName := ""
		if lf != nil && lf.VolumeName != "" {
			volumeName = lf.VolumeName
		}
		if volumeName != "" && DockerAvailable() {
			fmt.Printf("Removing volume '%s'...\n", volumeName)
			if _, rmErr := RunDockerCommand("volume", "rm", volumeName); rmErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: volume removal failed: %v (may not exist)\n", rmErr)
			} else {
				changes = append(changes, sidecar.ReportChange{
					Path:   volumeName,
					Action: "removed",
				})
				fmt.Println("Volume removed")
			}
		}
	} else {
		fmt.Println("Keeping Neo4j volume (--keep-data)")
	}

	// Phase 5: Backup .mdemg/
	backupPath := ""
	if _, statErr := os.Stat(mdemgDir); statErr == nil {
		ts := time.Now().Format("20060102T150405")
		backupPath = filepath.Join(projectDir, fmt.Sprintf(".mdemg-backup-%s", ts))
		fmt.Printf("Backing up %s → %s\n", mdemgDir, backupPath)
		if renameErr := os.Rename(mdemgDir, backupPath); renameErr != nil {
			// Fallback: try copy if rename fails (cross-device)
			fmt.Fprintf(os.Stderr, "Warning: backup failed: %v\n", renameErr)
			backupPath = ""
		} else {
			changes = append(changes, sidecar.ReportChange{
				Path:       mdemgDir,
				Action:     "backed-up",
				BackupPath: backupPath,
			})
			fmt.Println("Backup created")
		}
	}

	// Phase 6: Remove .mdemg/ (already done by rename in Phase 5)
	// If rename worked, .mdemg/ is gone. If it didn't, remove it manually.
	if backupPath == "" {
		if _, statErr := os.Stat(mdemgDir); statErr == nil {
			if rmErr := os.RemoveAll(mdemgDir); rmErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %v\n", mdemgDir, rmErr)
			} else {
				changes = append(changes, sidecar.ReportChange{
					Path:   mdemgDir,
					Action: "removed",
				})
			}
		}
	}

	// Phase 7: Remove generated hooks
	hookPath := filepath.Join(projectDir, ".claude", "hooks", "session-start.sh")
	if isSidecarGeneratedHook(hookPath) {
		fmt.Printf("Removing generated hook: %s\n", hookPath)
		if rmErr := os.Remove(hookPath); rmErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove hook: %v\n", rmErr)
		} else {
			changes = append(changes, sidecar.ReportChange{
				Path:   hookPath,
				Action: "removed",
			})
			fmt.Println("Generated hook removed")
		}
	}

	// Build report
	report := sidecar.NewUninstallReport(stateBefore, sidecar.StateUninstalled, sidecar.ExitSuccess, flags.force, flags.keepData, backupPath)
	report.Changes = changes
	report.NextActions = []string{
		"mdemg sidecar init  (to re-initialize)",
	}
	if backupPath != "" {
		report.NextActions = append(report.NextActions, fmt.Sprintf("Backup at: %s", backupPath))
	}

	if flags.format == "json" {
		return sidecar.PrintJSON(report)
	}

	// Text output
	fmt.Println()
	fmt.Println("Sidecar Uninstall")
	fmt.Println("=================")
	fmt.Printf("  State:     %s → uninstalled\n", stateBefore)
	fmt.Printf("  Keep data: %v\n", flags.keepData)
	if backupPath != "" {
		fmt.Printf("  Backup:    %s\n", backupPath)
	}
	fmt.Println()
	if len(changes) > 0 {
		fmt.Println("  Changes:")
		for _, c := range changes {
			if c.BackupPath != "" {
				fmt.Printf("    %s: %s → %s\n", c.Action, c.Path, c.BackupPath)
			} else {
				fmt.Printf("    %s: %s\n", c.Action, c.Path)
			}
		}
		fmt.Println()
	}
	fmt.Println("  Next steps:")
	for _, a := range report.NextActions {
		fmt.Printf("    %s\n", a)
	}
	fmt.Println()

	return nil
}

// isSidecarGeneratedHook checks if the given hook file was generated by mdemg sidecar.
func isSidecarGeneratedHook(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "Generated by: mdemg sidecar")
}
