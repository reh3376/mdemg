package cli

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	osExec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
	"mdemg/internal/config"
	"mdemg/internal/db"
	"mdemg/internal/sidecar"
)

// --- PID file utilities ---

func pidFilePath() string {
	abs, _ := filepath.Abs(".mdemg/mdemg.pid")
	return abs
}

func logFilePath() string {
	abs, _ := filepath.Abs(".mdemg/logs/mdemg.log")
	return abs
}

func writePID(path string, pid int) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(pid)+"\n"), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID: %w", err)
	}
	return pid, nil
}

func removePID(path string) error {
	return os.Remove(path)
}

func isProcessAlive(pid int) bool {
	return checkProcessAlive(pid)
}

func processUptime(pidPath string) (time.Duration, error) {
	info, err := os.Stat(pidPath)
	if err != nil {
		return 0, err
	}
	return time.Since(info.ModTime()), nil
}

// readPortFile reads the port number from the .mdemg.port file.
func readPortFile() (string, error) {
	portFile, _ := filepath.Abs(".mdemg.port")
	data, err := os.ReadFile(portFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// removePortFile removes the .mdemg.port file (stale cleanup).
func removePortFile() error {
	portFile, _ := filepath.Abs(".mdemg.port")
	return os.Remove(portFile)
}

// --- Commands ---

func newStartCmd() *cobra.Command {
	var (
		port        int
		dbURI       string
		autoMigrate bool
		mcpEnabled  bool
		noDB        bool
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the MDEMG server in the background",
		Long: `Start the MDEMG server as a background (daemon) process.

The server process is detached from the current terminal session.
Logs are written to .mdemg/logs/mdemg.log and the PID is stored
in .mdemg/mdemg.pid.

If Docker is available and a Neo4j container exists but is stopped,
it will be started automatically. Use --no-db to skip this.

Examples:
  mdemg start
  mdemg start --port 9999 --auto-migrate
  mdemg start --no-db`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(port, dbURI, autoMigrate, mcpEnabled, noDB)
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "Listen port (overrides LISTEN_ADDR env var)")
	cmd.Flags().StringVar(&dbURI, "db-uri", "", "Neo4j URI (overrides NEO4J_URI env var)")
	cmd.Flags().BoolVar(&autoMigrate, "auto-migrate", false, "Apply pending database migrations before starting")
	cmd.Flags().BoolVar(&mcpEnabled, "mcp", false, "Start MCP server subprocess alongside HTTP server")
	cmd.Flags().BoolVar(&noDB, "no-db", false, "Do not auto-start Neo4j container")

	return cmd
}

func runStart(port int, dbURI string, autoMigrate, mcpEnabled, noDB bool) error {
	pidPath := pidFilePath()

	// Check if already running
	if pid, err := readPID(pidPath); err == nil {
		if isProcessAlive(pid) {
			fmt.Printf("MDEMG server is already running (pid=%d)\n", pid)
			return nil
		}
		// Process died — clean up stale PID and port files
		_ = removePID(pidPath)
		_ = removePortFile()
	}

	// Auto-start Neo4j if needed (prefer project-scoped, fall back to lock file)
	startContainerName, _, cnErr := resolveProjectContainer()
	if cnErr != nil {
		startContainerName = sidecar.ResolveContainerName(".", neo4jContainerName)
	}
	if !noDB && DockerAvailable() {
		state, err := InspectContainer(startContainerName)
		if err == nil && state.Exists && !state.Running {
			fmt.Printf("Starting Neo4j container '%s'...\n", startContainerName)
			if _, err := RunDockerCommand("start", startContainerName); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to start Neo4j container: %v\n", err)
			} else {
				// Discover actual bolt port from container
				waitPort := neo4jDefaultPort
				if actualBolt, _ := ReadContainerPorts(startContainerName); actualBolt > 0 {
					waitPort = actualBolt
				}
				fmt.Print("Waiting for Neo4j...")
				if err := WaitForPort("localhost", waitPort, 60*time.Second); err != nil {
					fmt.Println(" timeout")
					fmt.Fprintf(os.Stderr, "Warning: Neo4j did not become ready: %v\n", err)
				} else {
					fmt.Println(" ready")
				}
			}
		}
	}

	// Resolve binary path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	// Build args for serve subcommand
	serveArgs := []string{"serve"}
	if port > 0 {
		serveArgs = append(serveArgs, fmt.Sprintf("--port=%d", port))
	}
	if dbURI != "" {
		serveArgs = append(serveArgs, fmt.Sprintf("--db-uri=%s", dbURI))
	}
	if autoMigrate {
		serveArgs = append(serveArgs, "--auto-migrate")
	}
	if mcpEnabled {
		serveArgs = append(serveArgs, "--mcp")
	}

	// Ensure log directory exists
	logPath := logFilePath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	// Open log file (truncate on each start)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	// Load .env and YAML config into parent environment before forking,
	// so the daemon child inherits the correct environment variables.
	// .env is loaded FIRST so YAML skip-if-set logic respects .env values.
	_ = godotenv.Load()
	if cfgPath := config.FindConfigFile(); cfgPath != "" {
		loadYAMLConfigOrWarn(cfgPath)
	}

	// Start detached process
	daemon := osExec.Command(execPath, serveArgs...)
	daemon.Stdout = logFile
	daemon.Stderr = logFile
	setDaemonSysProcAttr(daemon)
	// Inherit environment (now includes .env and YAML vars)
	daemon.Env = os.Environ()

	if err := daemon.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}

	daemonPID := daemon.Process.Pid

	// Write PID file
	if err := writePID(pidPath, daemonPID); err != nil {
		logFile.Close()
		return fmt.Errorf("write PID file: %w", err)
	}

	// Release the process (we don't wait for it)
	_ = daemon.Process.Release()
	logFile.Close()

	// Wait briefly to detect early crashes
	time.Sleep(2 * time.Second)
	if !isProcessAlive(daemonPID) {
		_ = removePID(pidPath)
		return fmt.Errorf("server exited immediately — check log: %s", logPath)
	}

	// Poll for port file (up to 30s — Neo4j startup + migrations can be slow)
	var portStr string
	fmt.Print("Waiting for server... ")
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(daemonPID) {
			fmt.Println("FAILED")
			_ = removePID(pidPath)
			return fmt.Errorf("server exited during startup — check log: %s", logPath)
		}
		if p, err := readPortFile(); err == nil && p != "" {
			portStr = p
			break
		}
		time.Sleep(1 * time.Second)
	}

	if portStr != "" {
		fmt.Println("ok")
		fmt.Printf("MDEMG server started (pid=%d, port=%s, log=%s)\n", daemonPID, portStr, logPath)
	} else {
		fmt.Println("still starting")
		fmt.Printf("MDEMG server started (pid=%d, log=%s)\n", daemonPID, logPath)
		fmt.Println("Tip: run 'mdemg status' to check when it's ready")
	}

	return nil
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the MDEMG server",
		Long: `Stop the running MDEMG server process.

Sends SIGTERM and waits up to 30 seconds for graceful shutdown.
This does NOT stop Neo4j — use 'mdemg db stop' for that.

Examples:
  mdemg stop`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop()
		},
	}
}

func runStop() error {
	pidPath := pidFilePath()

	pid, err := readPID(pidPath)
	if err != nil || !isProcessAlive(pid) {
		fmt.Println("MDEMG server is not running")
		// Clean up stale PID and port files
		if err == nil {
			_ = removePID(pidPath)
			_ = removePortFile()
		}
		return nil
	}

	// Send SIGTERM
	fmt.Printf("Stopping MDEMG server (pid=%d)...\n", pid)
	if err := terminateProcess(pid); err != nil {
		return fmt.Errorf("terminate process: %w", err)
	}

	// Poll for process exit (up to 30s)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			_ = removePID(pidPath)
			_ = removePortFile()
			fmt.Println("MDEMG server stopped")
			fmt.Println("Note: Neo4j container may still be running (stop with: mdemg db stop)")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Force kill
	fmt.Println("Warning: graceful shutdown timed out, force killing process")
	_ = forceKillProcess(pid)
	time.Sleep(1 * time.Second)
	_ = removePID(pidPath)
	_ = removePortFile()
	fmt.Println("MDEMG server killed")
	fmt.Println("Note: Neo4j container may still be running (stop with: mdemg db stop)")

	return nil
}

func newRestartCmd() *cobra.Command {
	var (
		port        int
		dbURI       string
		autoMigrate bool
		mcpEnabled  bool
		noDB        bool
	)

	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the MDEMG server",
		Long: `Stop and restart the MDEMG server.

All flags from 'mdemg start' are supported.

Examples:
  mdemg restart
  mdemg restart --port 9999`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Stop first (ignore "not running" errors)
			_ = runStop()
			// Brief pause to let port release
			time.Sleep(1 * time.Second)
			return runStart(port, dbURI, autoMigrate, mcpEnabled, noDB)
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "Listen port (overrides LISTEN_ADDR env var)")
	cmd.Flags().StringVar(&dbURI, "db-uri", "", "Neo4j URI (overrides NEO4J_URI env var)")
	cmd.Flags().BoolVar(&autoMigrate, "auto-migrate", false, "Apply pending database migrations before starting")
	cmd.Flags().BoolVar(&mcpEnabled, "mcp", false, "Start MCP server subprocess alongside HTTP server")
	cmd.Flags().BoolVar(&noDB, "no-db", false, "Do not auto-start Neo4j container")

	return cmd
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show MDEMG server and database status",
		Long: `Display the status of the MDEMG server and Neo4j database.

Shows PID, port, uptime, log path, Neo4j container status, and
server health (if reachable).

Examples:
  mdemg status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	fmt.Println("MDEMG Status")
	fmt.Println("============")

	pidPath := pidFilePath()
	logPath := logFilePath()

	pid, err := readPID(pidPath)
	running := err == nil && isProcessAlive(pid)

	var serverPort string

	if running {
		fmt.Printf("  Server:    running (pid=%d)\n", pid)

		// Show port
		if portStr, err := readPortFile(); err == nil && portStr != "" {
			serverPort = portStr
			fmt.Printf("  Port:      %s\n", portStr)
		}

		// Show uptime
		if uptime, err := processUptime(pidPath); err == nil {
			fmt.Printf("  Uptime:    %s\n", formatDuration(uptime))
		}

		fmt.Printf("  Log:       %s\n", logPath)

		// Health check
		if serverPort != "" {
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Get(fmt.Sprintf("http://localhost:%s/healthz", serverPort))
			if err == nil {
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				if resp.StatusCode == 200 {
					fmt.Println("  Health:    ok")
				} else {
					fmt.Printf("  Health:    %s (%s)\n", resp.Status, strings.TrimSpace(string(body)))
				}
			} else {
				fmt.Println("  Health:    unreachable")
			}
		}
	} else {
		fmt.Println("  Server:    stopped")
		// Clean up stale PID and port files
		if err == nil {
			_ = removePID(pidPath)
			_ = removePortFile()
		}
	}

	// Embedding provider info
	fmt.Println()
	cfg, cfgErr := loadConfig()
	if cfgErr == nil {
		embProvider := cfg.EmbeddingProvider
		if embProvider == "" {
			embProvider = "disabled"
		}
		// Redact values that look like API keys (sk-*, key-*, etc.)
		if strings.HasPrefix(embProvider, "sk-") || strings.HasPrefix(embProvider, "key-") || len(embProvider) > 30 {
			embProvider = "REDACTED (invalid provider — re-run mdemg init)"
		}
		fmt.Printf("  Embedding: %s", embProvider)
		switch embProvider {
		case "openai":
			fmt.Printf(" (model: %s)", cfg.OpenAIModel)
		case "ollama":
			fmt.Printf(" (model: %s)", cfg.OllamaModel)
		}
		fmt.Println()
	}

	// Neo4j container status (prefer project-scoped, fall back to lock file)
	statusContainerName, _, scnErr := resolveProjectContainer()
	if scnErr != nil {
		statusContainerName = sidecar.ResolveContainerName(".", neo4jContainerName)
	}
	if DockerAvailable() {
		state, err := InspectContainer(statusContainerName)
		if err == nil {
			if state.Exists {
				fmt.Printf("  Neo4j:     %s (%s)\n", state.Status, statusContainerName)
				// Show node count if running and config loaded
				if state.Running && cfgErr == nil {
					if nodeCount, ncErr := getNeo4jNodeCount(cfg); ncErr == nil {
						fmt.Printf("  Nodes:     %d\n", nodeCount)
					}
				}
			} else {
				fmt.Printf("  Neo4j:     not created\n")
			}
		}
	} else {
		fmt.Println("  Neo4j:     docker not available")
	}

	// J17 Sidecar status (if configured)
	if cfgErr == nil && cfg.J17SidecarURL != "" {
		sidecarClient := &http.Client{Timeout: 1 * time.Second}
		resp, err := sidecarClient.Get(cfg.J17SidecarURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				fmt.Printf("  Sidecar:   available (%s, mode=%s)\n", cfg.J17SidecarURL, cfg.J17SidecarMode)
			} else {
				fmt.Printf("  Sidecar:   unavailable (status %d)\n", resp.StatusCode)
			}
		} else {
			fmt.Printf("  Sidecar:   unreachable (%s)\n", cfg.J17SidecarURL)
		}
	}

	return nil
}

// getNeo4jNodeCount queries Neo4j for the total node count.
func getNeo4jNodeCount(cfg config.Config) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Suppress driver log output during status check
	origOutput := log.Writer()
	log.SetOutput(io.Discard)
	driver, err := db.NewDriver(cfg)
	log.SetOutput(origOutput)
	if err != nil {
		return 0, err
	}
	defer driver.Close(ctx)

	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	raw, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, "MATCH (n) RETURN count(n) AS cnt", nil)
		if err != nil {
			return nil, err
		}
		rec, err := result.Single(ctx)
		if err != nil {
			return nil, err
		}
		return rec.Values[0].(int64), nil
	})
	if err != nil {
		return 0, err
	}
	return raw.(int64), nil
}

// formatDuration returns a human-friendly duration string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}
