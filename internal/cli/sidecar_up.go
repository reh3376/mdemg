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

	// Derive project-scoped container/volume names
	projectDir := filepath.Dir(filepath.Dir(configPath)) // .mdemg/sidecar.yaml → project root
	containerName := ContainerNameForProject(projectDir)
	volumeName := VolumeNameForProject(projectDir)

	// Dry-run mode
	if flags.dryRun {
		runtimeHost := exec.Host()
		env := sidecar.NewReportEnvelope("mdemg sidecar up", stateBefore, sidecar.StateRunning, sidecar.ExitSuccess)
		env.Result = "dry-run"
		env.NextActions = []string{
			fmt.Sprintf("Would start Neo4j container '%s' on %s", containerName, runtimeHost),
			fmt.Sprintf("Would start MDEMG server on %s port %d (dynamic allocation enabled)", runtimeHost, port),
			"Would update lock file to running",
		}
		if flags.format == "json" {
			return sidecar.PrintJSON(env)
		}
		fmt.Println("[dry-run] Sidecar Up")
		fmt.Println("=====================")
		fmt.Printf("  State:     %s → running\n", stateBefore)
		fmt.Printf("  Host:      %s\n", runtimeHost)
		fmt.Printf("  Port:      %d (preferred)\n", port)
		fmt.Printf("  Container: %s\n", containerName)
		fmt.Printf("  Volume:    %s\n", volumeName)
		fmt.Println()
		fmt.Println("  Would perform:")
		for _, a := range env.NextActions {
			fmt.Printf("    • %s\n", a)
		}
		fmt.Println()
		return nil
	}

	var changes []sidecar.ReportChange

	// Start Neo4j container via executor with dynamic port allocation
	neo4jStarted := false
	actualBoltPort := neo4jDefaultPort
	actualHTTPPort := neo4jDefaultHTTP
	if exec.DockerAvailable() {
		inspOut, inspErr := exec.RunDocker("inspect", "--format", "{{.State.Status}}", containerName)
		if inspErr == nil {
			status := strings.TrimSpace(inspOut)
			if status == "running" {
				fmt.Printf("Neo4j container '%s' is already running\n", containerName)
				// Read actual ports from running container
				bp, hp := ReadContainerPorts(containerName)
				if bp > 0 {
					actualBoltPort = bp
				}
				if hp > 0 {
					actualHTTPPort = hp
				}
			} else {
				fmt.Printf("Starting Neo4j container '%s'...\n", containerName)
				if _, startErr := exec.RunDocker("start", containerName); startErr != nil {
					return sidecarUpError(flags.format, stateBefore, "NEO4J_START_FAIL",
						fmt.Sprintf("Failed to start Neo4j container: %v", startErr),
						"Check Docker: docker start "+containerName)
				}
				neo4jStarted = true
				// Read actual ports from restarted container
				bp, hp := ReadContainerPorts(containerName)
				if bp > 0 {
					actualBoltPort = bp
				}
				if hp > 0 {
					actualHTTPPort = hp
				}
			}
		} else {
			// Container doesn't exist — find free ports and create it
			var portErr error
			actualBoltPort, portErr = FindFreePort(neo4jDefaultPort, 7687, 7787)
			if portErr != nil {
				return sidecarUpError(flags.format, stateBefore, "PORT_ALLOC_FAIL",
					fmt.Sprintf("Failed to find free bolt port: %v", portErr),
					"Free a port in range 7687-7787")
			}
			actualHTTPPort, portErr = FindFreePort(neo4jDefaultHTTP, 7474, 7574)
			if portErr != nil {
				return sidecarUpError(flags.format, stateBefore, "PORT_ALLOC_FAIL",
					fmt.Sprintf("Failed to find free HTTP port: %v", portErr),
					"Free a port in range 7474-7574")
			}

			fmt.Printf("Creating Neo4j container '%s' (bolt:%d, http:%d)...\n",
				containerName, actualBoltPort, actualHTTPPort)
			dockerArgs := []string{
				"run", "-d",
				"--name", containerName,
				"-p", fmt.Sprintf("%d:7687", actualBoltPort),
				"-p", fmt.Sprintf("%d:7474", actualHTTPPort),
				"-e", fmt.Sprintf("NEO4J_AUTH=neo4j/%s", neo4jPass()),
				"-e", "NEO4J_server_memory_heap_initial__size=512m",
				"-e", "NEO4J_server_memory_heap_max__size=1g",
				"-e", "NEO4J_server_memory_pagecache_size=512m",
				"-e", `NEO4J_PLUGINS=["apoc"]`,
				"-v", volumeName + ":/data",
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
			if waitErr := exec.WaitForPort(exec.Host(), actualBoltPort, 60*time.Second); waitErr != nil {
				fmt.Println(" timeout")
				return sidecarUpError(flags.format, stateBefore, "NEO4J_TIMEOUT",
					"Neo4j did not become ready within 60s",
					"Check container logs: docker logs "+containerName)
			}
			fmt.Println(" ready")
			changes = append(changes, sidecar.ReportChange{
				Path:   containerName,
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

		// Pass sidecar config to the daemon process via environment variables.
		// The server requires these to start — without them it exits immediately.
		// LISTEN_ADDR: server listen port (from sidecar.yaml endpoint)
		// NEO4J_URI/USER/PASS: database connection (dynamic port from container creation)
		// Credentials match the NEO4J_AUTH set on the container in docker run above.
		extraEnv := []string{
			fmt.Sprintf("LISTEN_ADDR=:%d", port),
			fmt.Sprintf("NEO4J_URI=bolt://localhost:%d", actualBoltPort),
			"NEO4J_USER=neo4j",
			fmt.Sprintf("NEO4J_PASS=%s", neo4jPass()),
			"EMBEDDING_PROVIDER=ollama",
			"OLLAMA_ENDPOINT=http://localhost:11434",
			"OLLAMA_MODEL=qwen3-embedding:8b",
			"LLM_PROVIDER=ollama",
			"LLM_MODEL=llama3.2:3b-instruct-fp16",
		}

		fmt.Print("Starting MDEMG server...")

		daemonPID, startErr := exec.StartDaemon(serveArgs, extraEnv...)
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

	// Derive actual API endpoint from port file
	actualEndpoint := cfg.Runtime.Endpoint
	if portStr, portReadErr := readPortFile(); portReadErr == nil && portStr != "" {
		actualEndpoint = fmt.Sprintf("http://%s:%s", exec.Host(), portStr)
	}

	// Update lock file with runtime port metadata
	if lockPath != "" {
		lf, lfErr := sidecar.ReadLock(lockPath)
		if lfErr == nil {
			lf.State = sidecar.StateRunning
			lf.Endpoint = actualEndpoint
			lf.Neo4jBoltPort = actualBoltPort
			lf.Neo4jHTTPPort = actualHTTPPort
			lf.ContainerName = containerName
			lf.VolumeName = volumeName

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
