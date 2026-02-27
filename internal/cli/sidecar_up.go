package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarUpCmd() *cobra.Command {
	var (
		dryRun bool
		format string
	)

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start sidecar services",
		Long: `Start the MDEMG sidecar services (Neo4j + API server).

Transitions the sidecar state from installed/stopped to running.
Starts Neo4j container and MDEMG server as a background process.

Examples:
  mdemg sidecar up
  mdemg sidecar up --dry-run
  mdemg sidecar up --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarUp(sidecarUpFlags{
				dryRun: dryRun,
				format: format,
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would happen without making changes")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")

	return cmd
}

type sidecarUpFlags struct {
	dryRun bool
	format string
}

func runSidecarUp(flags sidecarUpFlags) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	stateBefore := sidecar.CurrentStateFrom(cwd)
	lockPath := sidecar.FindLockFileFrom(cwd)

	// State guard: must be installed, stopped, or degraded
	switch stateBefore {
	case sidecar.StateInstalled, sidecar.StateStopped, sidecar.StateDegraded:
		// valid
	default:
		remediation := "mdemg sidecar install"
		if stateBefore == sidecar.StateUninitialized {
			remediation = "mdemg sidecar init"
		} else if stateBefore == sidecar.StateInitialized {
			remediation = "mdemg sidecar install"
		} else if stateBefore == sidecar.StateRunning {
			remediation = "Already running. Use: mdemg sidecar restart"
		}

		if flags.format == "json" {
			env := sidecar.NewReportEnvelope("mdemg sidecar up", stateBefore, stateBefore, sidecar.ExitValidation)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "INVALID_STATE",
				Severity:    "error",
				Message:     fmt.Sprintf("Cannot start from state %q", stateBefore),
				Remediation: remediation,
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("cannot start from state %q — %s", stateBefore, remediation)
	}

	// Load config
	configPath := sidecar.FindConfigFileFrom(cwd)
	if configPath == "" {
		if flags.format == "json" {
			env := sidecar.NewReportEnvelope("mdemg sidecar up", stateBefore, stateBefore, sidecar.ExitValidation)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "CONFIG_NOT_FOUND",
				Severity:    "error",
				Message:     "No sidecar.yaml found",
				Remediation: "Run: mdemg sidecar init",
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("no sidecar.yaml found — run: mdemg sidecar init")
	}

	cfg, err := sidecar.LoadConfig(configPath)
	if err != nil {
		if flags.format == "json" {
			env := sidecar.NewReportEnvelope("mdemg sidecar up", stateBefore, stateBefore, sidecar.ExitValidation)
			env.Issues = append(env.Issues, sidecar.ReportIssue{
				Code:        "CONFIG_READ_ERROR",
				Severity:    "error",
				Message:     fmt.Sprintf("Failed to read config: %v", err),
				Remediation: "Check .mdemg/sidecar.yaml syntax or re-run: mdemg sidecar init",
			})
			return sidecar.PrintJSON(env)
		}
		return fmt.Errorf("read config: %w", err)
	}

	// Create executor based on profile
	exec, err := newExecutor(cfg)
	if err != nil {
		return sidecarUpError(flags.format, stateBefore, "EXECUTOR_FAIL",
			fmt.Sprintf("Failed to create executor: %v", err),
			"Check remote host configuration in sidecar.yaml")
	}
	defer func() { _ = exec.Close() }()

	port := extractPort(cfg.Runtime.Endpoint)

	// Dry-run mode
	if flags.dryRun {
		runtimeHost := exec.Host()
		env := sidecar.NewReportEnvelope("mdemg sidecar up", stateBefore, sidecar.StateRunning, sidecar.ExitSuccess)
		env.Result = "dry-run"
		env.NextActions = []string{
			fmt.Sprintf("Would start Neo4j container on %s", runtimeHost),
			fmt.Sprintf("Would start MDEMG server on %s port %d", runtimeHost, port),
			"Would update lock file to running",
		}
		if flags.format == "json" {
			return sidecar.PrintJSON(env)
		}
		fmt.Println("[dry-run] Sidecar Up")
		fmt.Println("=====================")
		fmt.Printf("  State:  %s → running\n", stateBefore)
		fmt.Printf("  Host:   %s\n", runtimeHost)
		fmt.Printf("  Port:   %d\n", port)
		fmt.Println()
		fmt.Println("  Would perform:")
		for _, a := range env.NextActions {
			fmt.Printf("    • %s\n", a)
		}
		fmt.Println()
		return nil
	}

	var changes []sidecar.ReportChange

	// Start Neo4j container via executor
	neo4jStarted := false
	if exec.DockerAvailable() {
		inspOut, inspErr := exec.RunDocker("inspect", "--format", "{{.State.Status}}", neo4jContainerName)
		if inspErr == nil {
			status := strings.TrimSpace(inspOut)
			if status == "running" {
				fmt.Printf("Neo4j container '%s' is already running\n", neo4jContainerName)
			} else {
				fmt.Printf("Starting Neo4j container '%s'...\n", neo4jContainerName)
				if _, startErr := exec.RunDocker("start", neo4jContainerName); startErr != nil {
					return sidecarUpError(flags.format, stateBefore, "NEO4J_START_FAIL",
						fmt.Sprintf("Failed to start Neo4j container: %v", startErr),
						"Check Docker: docker start "+neo4jContainerName)
				}
				neo4jStarted = true
			}
		} else {
			// Container doesn't exist — create it
			fmt.Printf("Creating Neo4j container '%s'...\n", neo4jContainerName)
			dockerArgs := []string{
				"run", "-d",
				"--name", neo4jContainerName,
				"-p", fmt.Sprintf("%d:7687", neo4jDefaultPort),
				"-p", fmt.Sprintf("%d:7474", neo4jDefaultHTTP),
				"-e", "NEO4J_AUTH=neo4j/mdemg-dev",
				"-e", "NEO4J_server_memory_heap_initial__size=512m",
				"-e", "NEO4J_server_memory_heap_max__size=1g",
				"-e", "NEO4J_server_memory_pagecache_size=512m",
				"-e", `NEO4J_PLUGINS=["apoc"]`,
				"-v", neo4jVolumeName + ":/data",
				neo4jImage,
			}
			if _, runErr := exec.RunDocker(dockerArgs...); runErr != nil {
				return sidecarUpError(flags.format, stateBefore, "NEO4J_CREATE_FAIL",
					fmt.Sprintf("Failed to create Neo4j container: %v", runErr),
					"Check Docker and try: mdemg db start")
			}
			neo4jStarted = true
		}

		if neo4jStarted {
			fmt.Print("Waiting for Neo4j...")
			if waitErr := exec.WaitForPort(exec.Host(), neo4jDefaultPort, 60*time.Second); waitErr != nil {
				fmt.Println(" timeout")
				return sidecarUpError(flags.format, stateBefore, "NEO4J_TIMEOUT",
					"Neo4j did not become ready within 60s",
					"Check container logs: docker logs "+neo4jContainerName)
			}
			fmt.Println(" ready")
			changes = append(changes, sidecar.ReportChange{
				Path:   neo4jContainerName,
				Action: "started",
			})
		}
	} else {
		fmt.Println("Warning: Docker not available, skipping Neo4j container management")
	}

	// Start MDEMG server via executor
	pidPath := pidFilePath()
	serverStarted := false

	if pid, pidErr := readPID(pidPath); pidErr == nil && exec.DaemonRunning(pid) {
		fmt.Printf("MDEMG server is already running (pid=%d)\n", pid)
	} else {
		serveArgs := []string{"serve", "--auto-migrate"}

		fmt.Print("Starting MDEMG server...")

		daemonPID, startErr := exec.StartDaemon(serveArgs)
		if startErr != nil {
			return sidecarUpError(flags.format, stateBefore, "SERVER_START_FAIL",
				fmt.Sprintf("Failed to start server: %v", startErr),
				"Check log: "+logFilePath())
		}

		if writeErr := writePID(pidPath, daemonPID); writeErr != nil {
			return sidecarUpError(flags.format, stateBefore, "PID_WRITE_FAIL",
				fmt.Sprintf("Failed to write PID file: %v", writeErr),
				"Check file permissions on .mdemg/")
		}

		// Wait briefly to detect early crashes
		time.Sleep(2 * time.Second)
		if !exec.DaemonRunning(daemonPID) {
			_ = removePID(pidPath)
			return sidecarUpError(flags.format, stateBefore, "SERVER_CRASH",
				"Server exited immediately after start",
				"Check log: "+logFilePath())
		}

		// Poll for port file (readiness)
		var portStr string
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if p, readErr := readPortFile(); readErr == nil && p != "" {
				portStr = p
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		if portStr != "" {
			fmt.Printf(" running (pid=%d, port=%s)\n", daemonPID, portStr)
		} else {
			fmt.Printf(" running (pid=%d)\n", daemonPID)
		}

		serverStarted = true
		changes = append(changes, sidecar.ReportChange{
			Path:   pidPath,
			Action: "created",
		})
	}

	// Update lock file
	if lockPath != "" {
		lf, lfErr := sidecar.ReadLock(lockPath)
		if lfErr == nil {
			lf.State = sidecar.StateRunning

			// Store remote metadata if applicable
			if cfg.Profile == sidecar.ProfileStudioRemote {
				lf.RemoteHost = exec.Host()
				lf.TransportUsed = string(cfg.Runtime.Remote.Transport)
				lf.DockerContext = mdemgDockerContext

				// For remote, store remote PID separately
				if pid, pidReadErr := readPID(pidPath); pidReadErr == nil {
					lf.RemotePID = pid
				}
			}

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
	stateAfter := sidecar.StateRunning
	env := sidecar.NewReportEnvelope("mdemg sidecar up", stateBefore, stateAfter, sidecar.ExitSuccess)
	env.Changes = changes
	env.NextActions = []string{"mdemg sidecar status", "mdemg sidecar doctor"}

	if flags.format == "json" {
		return sidecar.PrintJSON(env)
	}

	// Text output
	fmt.Println()
	fmt.Println("Sidecar Up")
	fmt.Println("==========")
	fmt.Printf("  State:  %s → %s\n", stateBefore, stateAfter)
	if cfg.Profile == sidecar.ProfileStudioRemote {
		fmt.Printf("  Host:   %s\n", exec.Host())
	}
	fmt.Println()
	if neo4jStarted {
		fmt.Println("  Started: Neo4j container")
	}
	if serverStarted {
		fmt.Println("  Started: MDEMG server")
	}
	if !neo4jStarted && !serverStarted {
		fmt.Println("  All services were already running")
	}
	fmt.Println()
	fmt.Println("  Next steps:")
	for _, a := range env.NextActions {
		fmt.Printf("    %s\n", a)
	}
	fmt.Println()

	return nil
}

// sidecarUpError builds and returns an error, optionally as JSON.
func sidecarUpError(format string, stateBefore sidecar.State, code, message, remediation string) error {
	if format == "json" {
		env := sidecar.NewReportEnvelope("mdemg sidecar up", stateBefore, stateBefore, sidecar.ExitRuntime)
		env.Issues = append(env.Issues, sidecar.ReportIssue{
			Code:        code,
			Severity:    "error",
			Message:     message,
			Remediation: remediation,
		})
		return sidecar.PrintJSON(env)
	}
	return fmt.Errorf("%s — %s", message, remediation)
}
