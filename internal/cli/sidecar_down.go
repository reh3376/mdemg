package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarDownCmd() *cobra.Command {
	var (
		dryRun bool
		format string
	)

	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop sidecar services",
		Long: `Stop the MDEMG sidecar services (API server + Neo4j container).

Sends SIGTERM to the server with 30s grace period, then stops
the Neo4j container. Transitions state from running to stopped.

Examples:
  mdemg sidecar down
  mdemg sidecar down --dry-run
  mdemg sidecar down --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarDown(sidecarDownFlags{
				dryRun: dryRun,
				format: format,
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would happen without making changes")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")

	return cmd
}

type sidecarDownFlags struct {
	dryRun bool
	format string
}

func runSidecarDown(flags sidecarDownFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	stateBefore := sidecar.CurrentStateFrom(cwd)
	lockPath := sidecar.FindLockFileFrom(cwd)

	// State guard: must be running or degraded
	if stateBefore != sidecar.StateRunning && stateBefore != sidecar.StateDegraded {
		remediation := "mdemg sidecar up"
		if stateBefore == sidecar.StateUninitialized {
			remediation = "mdemg sidecar init"
		} else if stateBefore == sidecar.StateInitialized {
			remediation = "mdemg sidecar install"
		} else if stateBefore == sidecar.StateStopped {
			remediation = "Already stopped. Use: mdemg sidecar up"
		}

		if flags.format == "json" {
			env := sidecar.NewReportEnvelope("mdemg sidecar down", stateBefore, stateBefore, sidecar.ExitValidation)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "INVALID_STATE",
				Severity:    "error",
				Message:     fmt.Sprintf("Cannot stop from state %q", stateBefore),
				Remediation: remediation,
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("cannot stop from state %q — %s", stateBefore, remediation)
	}

	// Load config for executor
	configPath := sidecar.FindConfigFileFrom(cwd)
	var cfg *sidecar.Config
	if configPath != "" {
		cfg, _ = sidecar.LoadConfig(configPath)
	}

	exec, err := newExecutor(cfg)
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}
	defer func() { _ = exec.Close() }()

	// Dry-run mode
	if flags.dryRun {
		env := sidecar.NewReportEnvelope("mdemg sidecar down", stateBefore, sidecar.StateStopped, sidecar.ExitSuccess)
		env.Result = "dry-run"
		env.NextActions = []string{
			"Would stop MDEMG server (SIGTERM → SIGKILL)",
			"Would stop Neo4j container",
			"Would update lock file to stopped",
		}
		if flags.format == "json" {
			return sidecar.PrintJSON(env)
		}
		fmt.Println("[dry-run] Sidecar Down")
		fmt.Println("======================")
		fmt.Printf("  State:  %s → stopped\n", stateBefore)
		fmt.Println()
		fmt.Println("  Would perform:")
		for _, a := range env.NextActions {
			fmt.Printf("    • %s\n", a)
		}
		fmt.Println()
		return nil
	}

	var changes []sidecar.ReportChange

	// Determine PID to stop — for remote, use lock file's RemotePID
	serverStopped := false
	pidPath := pidFilePath()
	pid, pidErr := readPID(pidPath)

	// For remote profile, prefer RemotePID from lock file
	if cfg != nil && cfg.Profile == sidecar.ProfileStudioRemote && lockPath != "" {
		if lf, lfErr := sidecar.ReadLock(lockPath); lfErr == nil && lf.RemotePID > 0 {
			pid = lf.RemotePID
			pidErr = nil
		}
	}

	if pidErr == nil && exec.DaemonRunning(pid) {
		fmt.Printf("Stopping MDEMG server (pid=%d)...\n", pid)

		if stopErr := exec.StopDaemon(pid); stopErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: stop daemon failed: %v\n", stopErr)
		}

		// Verify it stopped
		if exec.DaemonRunning(pid) {
			fmt.Println("Warning: server may still be running")
		}

		_ = removePID(pidPath)
		serverStopped = true
		changes = append(changes, sidecar.ReportChange{
			Path:   pidPath,
			Action: "removed",
		})
		fmt.Println("MDEMG server stopped")
	} else {
		fmt.Println("MDEMG server is not running")
		// Clean up stale PID file
		if pidErr == nil {
			_ = removePID(pidPath)
		}
	}

	// Stop Neo4j container via executor
	neo4jStopped := false
	if exec.DockerAvailable() {
		inspOut, inspErr := exec.RunDocker("inspect", "--format", "{{.State.Status}}", neo4jContainerName)
		if inspErr == nil && strings.TrimSpace(inspOut) == "running" {
			fmt.Printf("Stopping Neo4j container '%s'...\n", neo4jContainerName)
			if _, stopErr := exec.RunDocker("stop", neo4jContainerName); stopErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to stop Neo4j container: %v\n", stopErr)
			} else {
				neo4jStopped = true
				changes = append(changes, sidecar.ReportChange{
					Path:   neo4jContainerName,
					Action: "stopped",
				})
				fmt.Println("Neo4j container stopped")
			}
		} else if inspErr == nil {
			fmt.Printf("Neo4j container '%s' is already stopped\n", neo4jContainerName)
		}
	}

	// Update lock file
	stateAfter := sidecar.StateStopped
	if lockPath != "" {
		lf, lfErr := sidecar.ReadLock(lockPath)
		if lfErr == nil {
			lf.State = stateAfter
			// Clear remote PID on stop
			lf.RemotePID = 0
			if writeErr := sidecar.WriteLock(lockPath, lf); writeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to update lock file: %v\n", writeErr)
			} else {
				changes = append(changes, sidecar.ReportChange{
					Path:   lockPath,
					Action: "updated",
				})
			}
		}
	}

	// Build report
	env := sidecar.NewReportEnvelope("mdemg sidecar down", stateBefore, stateAfter, sidecar.ExitSuccess)
	env.Changes = changes
	env.NextActions = []string{"mdemg sidecar up"}

	if flags.format == "json" {
		return sidecar.PrintJSON(env)
	}

	// Text output
	fmt.Println()
	fmt.Println("Sidecar Down")
	fmt.Println("============")
	fmt.Printf("  State:  %s → %s\n", stateBefore, stateAfter)
	fmt.Println()
	if serverStopped {
		fmt.Println("  Stopped: MDEMG server")
	}
	if neo4jStopped {
		fmt.Println("  Stopped: Neo4j container")
	}
	if !serverStopped && !neo4jStopped {
		fmt.Println("  All services were already stopped")
	}
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("    mdemg sidecar up")
	fmt.Println()

	return nil
}
