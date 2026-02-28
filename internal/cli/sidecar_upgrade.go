package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarUpgradeCmd() *cobra.Command {
	var (
		dryRun      bool
		format      string
		skipRestart bool
	)

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade sidecar version",
		Long: `Detect version drift and perform a controlled upgrade cycle.

Compares the running sidecar version (from lock file) with the current
CLI binary version. If they differ, performs: down → install → up.

Examples:
  mdemg sidecar upgrade
  mdemg sidecar upgrade --dry-run
  mdemg sidecar upgrade --skip-restart
  mdemg sidecar upgrade --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarUpgrade(sidecarUpgradeFlags{
				dryRun:      dryRun,
				format:      format,
				skipRestart: skipRestart,
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report version drift and planned actions without executing")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")
	cmd.Flags().BoolVar(&skipRestart, "skip-restart", false, "Stop at install — don't call up (leaves state as installed)")

	return cmd
}

type sidecarUpgradeFlags struct {
	dryRun      bool
	format      string
	skipRestart bool
}

func runSidecarUpgrade(flags sidecarUpgradeFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	stateBefore := sidecar.CurrentStateFrom(cwd)
	lockPath := sidecar.FindLockFileFrom(cwd)

	// State guard: must be installed, running, stopped, or degraded
	switch stateBefore {
	case sidecar.StateInstalled, sidecar.StateRunning, sidecar.StateStopped, sidecar.StateDegraded:
		// valid
	default:
		remediation := "mdemg sidecar init"
		if stateBefore == sidecar.StateInitialized {
			remediation = "mdemg sidecar install"
		}
		if flags.format == "json" {
			report := sidecar.NewUpgradeReport(stateBefore, stateBefore, sidecar.ExitValidation, "", Version, false, flags.skipRestart)
			report.Issues = append(report.Issues, sidecar.ReportIssue{
				Code:        "INVALID_STATE",
				Severity:    "error",
				Message:     fmt.Sprintf("Cannot upgrade from state %q", stateBefore),
				Remediation: remediation,
			})
			return sidecar.PrintJSON(report)
		}
		return fmt.Errorf("cannot upgrade from state %q — %s", stateBefore, remediation)
	}

	// Read lock file for current version
	if lockPath == "" {
		if flags.format == "json" {
			report := sidecar.NewUpgradeReport(stateBefore, stateBefore, sidecar.ExitValidation, "", Version, false, flags.skipRestart)
			report.Issues = append(report.Issues, sidecar.ReportIssue{
				Code:        "LOCK_NOT_FOUND",
				Severity:    "error",
				Message:     "No sidecar.lock found",
				Remediation: "Run: mdemg sidecar install",
			})
			return sidecar.PrintJSON(report)
		}
		return fmt.Errorf("no sidecar.lock found — run: mdemg sidecar install")
	}

	lf, lfErr := sidecar.ReadLock(lockPath)
	if lfErr != nil {
		if flags.format == "json" {
			report := sidecar.NewUpgradeReport(stateBefore, stateBefore, sidecar.ExitRuntime, "", Version, false, flags.skipRestart)
			report.Issues = append(report.Issues, sidecar.ReportIssue{
				Code:        "LOCK_READ_ERROR",
				Severity:    "error",
				Message:     fmt.Sprintf("Failed to read lock file: %v", lfErr),
				Remediation: "Check .mdemg/sidecar.lock syntax",
			})
			return sidecar.PrintJSON(report)
		}
		return fmt.Errorf("read lock file: %w", lfErr)
	}

	previousVersion := lf.MdemgVersion
	currentVersion := Version
	versionChanged := previousVersion != currentVersion

	// Also check config hash for changes
	configPath := sidecar.FindConfigFileFrom(cwd)
	configChanged := false
	if configPath != "" {
		configData, readErr := os.ReadFile(configPath)
		if readErr == nil {
			currentHash := sidecar.HashConfig(configData)
			if lf.ConfigHash != currentHash {
				configChanged = true
			}
		}
	}

	// If no changes needed, report and exit
	if !versionChanged && !configChanged {
		if flags.format == "json" {
			report := sidecar.NewUpgradeReport(stateBefore, stateBefore, sidecar.ExitSuccess, previousVersion, currentVersion, false, flags.skipRestart)
			report.Result = "no-op"
			report.NextActions = []string{"Already current — no upgrade needed"}
			return sidecar.PrintJSON(report)
		}
		fmt.Printf("Sidecar already current (version: %s) — no upgrade needed.\n", currentVersion)
		return nil
	}

	// Dry-run mode
	if flags.dryRun {
		report := sidecar.NewUpgradeReport(stateBefore, stateBefore, sidecar.ExitSuccess, previousVersion, currentVersion, versionChanged, flags.skipRestart)
		report.Result = "dry-run"
		actions := []string{}
		if versionChanged {
			actions = append(actions, fmt.Sprintf("Would upgrade version: %s → %s", previousVersion, currentVersion))
		}
		if configChanged {
			actions = append(actions, "Would re-evaluate dependencies (config changed)")
		}
		if stateBefore == sidecar.StateRunning || stateBefore == sidecar.StateDegraded {
			actions = append(actions, "Would stop sidecar services (down)")
		}
		actions = append(actions, "Would re-run install")
		if !flags.skipRestart {
			actions = append(actions, "Would start sidecar services (up)")
		}
		actions = append(actions, "Would update lock file version")
		report.NextActions = actions

		if flags.format == "json" {
			return sidecar.PrintJSON(report)
		}
		fmt.Println("[dry-run] Sidecar Upgrade")
		fmt.Println("=========================")
		fmt.Printf("  Version:  %s → %s\n", previousVersion, currentVersion)
		if configChanged {
			fmt.Println("  Config:   changed")
		}
		fmt.Println()
		fmt.Println("  Would perform:")
		for _, a := range report.NextActions {
			fmt.Printf("    • %s\n", a)
		}
		fmt.Println()
		return nil
	}

	// --- Mutating path ---

	// Phase 1: Down (if running/degraded)
	if stateBefore == sidecar.StateRunning || stateBefore == sidecar.StateDegraded {
		fmt.Println("=== Phase 1: Stopping services ===")
		fmt.Println()
		if downErr := runSidecarDown(sidecarDownFlags{format: "text"}); downErr != nil {
			return fmt.Errorf("down phase failed: %w", downErr)
		}
		fmt.Println()
		// Brief pause for port release
		fmt.Println("Waiting for port release...")
		time.Sleep(1 * time.Second)
	}

	// Phase 2: Install
	fmt.Println("=== Phase 2: Re-installing dependencies ===")
	fmt.Println()
	if installErr := runSidecarInstall(sidecarInstallFlags{format: "text"}); installErr != nil {
		return fmt.Errorf("install phase failed: %w", installErr)
	}
	fmt.Println()

	// Phase 3: Up (unless skip-restart)
	if !flags.skipRestart {
		fmt.Println("=== Phase 3: Starting services ===")
		fmt.Println()
		if upErr := runSidecarUp(sidecarUpFlags{format: "text"}); upErr != nil {
			return fmt.Errorf("up phase failed: %w", upErr)
		}
		fmt.Println()
	}

	// Update lock file version
	stateAfter := sidecar.CurrentStateFrom(cwd)
	lf, lfErr = sidecar.ReadLock(lockPath)
	if lfErr == nil {
		lf.MdemgVersion = currentVersion
		if writeErr := sidecar.WriteLock(lockPath, lf); writeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update lock file version: %v\n", writeErr)
		}
	}

	// Build final report
	report := sidecar.NewUpgradeReport(stateBefore, stateAfter, sidecar.ExitSuccess, previousVersion, currentVersion, versionChanged, flags.skipRestart)
	report.Changes = []sidecar.ReportChange{
		{Path: lockPath, Action: "updated"},
	}
	report.NextActions = []string{"mdemg sidecar status", "mdemg sidecar doctor"}

	if flags.format == "json" {
		return sidecar.PrintJSON(report)
	}

	// Text output
	fmt.Println("Sidecar Upgrade")
	fmt.Println("===============")
	fmt.Printf("  Version:  %s → %s\n", previousVersion, currentVersion)
	fmt.Printf("  State:    %s → %s\n", stateBefore, stateAfter)
	if flags.skipRestart {
		fmt.Println("  Restart:  skipped (--skip-restart)")
	}
	fmt.Println()
	fmt.Println("  Next steps:")
	for _, a := range report.NextActions {
		fmt.Printf("    %s\n", a)
	}
	fmt.Println()

	return nil
}
