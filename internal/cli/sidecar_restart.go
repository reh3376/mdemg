package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarRestartCmd() *cobra.Command {
	var (
		dryRun bool
		format string
	)

	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart sidecar services",
		Long: `Stop and restart the MDEMG sidecar services.

Performs a graceful stop followed by a full start. Transitions
through stopped back to running.

Examples:
  mdemg sidecar restart
  mdemg sidecar restart --dry-run
  mdemg sidecar restart --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarRestart(sidecarRestartFlags{
				dryRun: dryRun,
				format: format,
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would happen without making changes")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")

	return cmd
}

type sidecarRestartFlags struct {
	dryRun bool
	format string
}

func runSidecarRestart(flags sidecarRestartFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	stateBefore := sidecar.CurrentStateFrom(cwd)

	// State guard: must be running or degraded
	if stateBefore != sidecar.StateRunning && stateBefore != sidecar.StateDegraded {
		remediation := "mdemg sidecar up"
		if stateBefore == sidecar.StateUninitialized {
			remediation = "mdemg sidecar init"
		} else if stateBefore == sidecar.StateInitialized {
			remediation = "mdemg sidecar install"
		} else if stateBefore == sidecar.StateStopped || stateBefore == sidecar.StateInstalled {
			remediation = "Not running. Use: mdemg sidecar up"
		}

		if flags.format == "json" {
			env := sidecar.NewReportEnvelope("mdemg sidecar restart", stateBefore, stateBefore, sidecar.ExitValidation)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "INVALID_STATE",
				Severity:    "error",
				Message:     fmt.Sprintf("Cannot restart from state %q", stateBefore),
				Remediation: remediation,
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("cannot restart from state %q — %s", stateBefore, remediation)
	}

	// Dry-run mode
	if flags.dryRun {
		env := sidecar.NewReportEnvelope("mdemg sidecar restart", stateBefore, sidecar.StateRunning, sidecar.ExitSuccess)
		env.Result = "dry-run"
		env.NextActions = []string{
			"Would stop MDEMG server",
			"Would stop Neo4j container",
			"Would wait 1s for port release",
			"Would start Neo4j container",
			"Would start MDEMG server",
			"Would update lock file to running",
		}
		if flags.format == "json" {
			return sidecar.PrintJSON(env)
		}
		fmt.Println("[dry-run] Sidecar Restart")
		fmt.Println("========================")
		fmt.Printf("  State:  %s → running\n", stateBefore)
		fmt.Println()
		fmt.Println("  Would perform:")
		for _, a := range env.NextActions {
			fmt.Printf("    • %s\n", a)
		}
		fmt.Println()
		return nil
	}

	// Phase 1: Stop
	fmt.Println("=== Phase 1: Stopping services ===")
	fmt.Println()
	downErr := runSidecarDown(sidecarDownFlags{format: "text"})
	if downErr != nil {
		return fmt.Errorf("down phase failed: %w", downErr)
	}

	// Brief pause for port release
	fmt.Println("Waiting for port release...")
	time.Sleep(1 * time.Second)

	// Phase 2: Start
	fmt.Println("=== Phase 2: Starting services ===")
	fmt.Println()
	upErr := runSidecarUp(sidecarUpFlags{format: "text"})
	if upErr != nil {
		return fmt.Errorf("up phase failed: %w", upErr)
	}

	// For JSON output mode, re-read state and build combined report
	if flags.format == "json" {
		stateAfter := sidecar.CurrentStateFrom(cwd)
		exitCode := sidecar.ExitSuccess
		if stateAfter != sidecar.StateRunning {
			exitCode = sidecar.ExitRuntime
		}
		env := sidecar.NewReportEnvelope("mdemg sidecar restart", stateBefore, stateAfter, exitCode)
		env.NextActions = []string{"mdemg sidecar status", "mdemg sidecar doctor"}
		return sidecar.PrintJSON(env)
	}

	return nil
}
