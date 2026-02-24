package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	osExec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
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
	err := syscall.Kill(pid, 0)
	return err == nil
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
	if pid, err := readPID(pidPath); err == nil && isProcessAlive(pid) {
		fmt.Printf("MDEMG server is already running (pid=%d)\n", pid)
		return nil
	}

	// Auto-start Neo4j if needed
	if !noDB && DockerAvailable() {
		state, err := InspectContainer(neo4jContainerName)
		if err == nil && state.Exists && !state.Running {
			fmt.Printf("Starting Neo4j container '%s'...\n", neo4jContainerName)
			if _, err := RunDockerCommand("start", neo4jContainerName); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to start Neo4j container: %v\n", err)
			} else {
				fmt.Print("Waiting for Neo4j...")
				if err := WaitForPort("localhost", neo4jDefaultPort, 60*time.Second); err != nil {
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

	// Start detached process
	daemon := osExec.Command(execPath, serveArgs...)
	daemon.Stdout = logFile
	daemon.Stderr = logFile
	daemon.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Inherit environment
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

	// Poll for port file (up to 10s)
	var portStr string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if p, err := readPortFile(); err == nil && p != "" {
			portStr = p
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if portStr != "" {
		fmt.Printf("MDEMG server started (pid=%d, port=%s, log=%s)\n", daemonPID, portStr, logPath)
	} else {
		fmt.Printf("MDEMG server started (pid=%d, log=%s)\n", daemonPID, logPath)
		fmt.Println("Note: port file not yet available — server may still be starting")
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
		// Clean up stale PID file
		if err == nil {
			_ = removePID(pidPath)
		}
		return nil
	}

	// Send SIGTERM
	fmt.Printf("Stopping MDEMG server (pid=%d)...\n", pid)
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	// Poll for process exit (up to 30s)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			_ = removePID(pidPath)
			fmt.Println("MDEMG server stopped")
			fmt.Println("Note: Neo4j container may still be running (stop with: mdemg db stop)")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Force kill
	fmt.Println("Warning: graceful shutdown timed out, sending SIGKILL")
	_ = syscall.Kill(pid, syscall.SIGKILL)
	time.Sleep(1 * time.Second)
	_ = removePID(pidPath)
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

	if running {
		fmt.Printf("  Server:    running (pid=%d)\n", pid)

		// Show port
		if portStr, err := readPortFile(); err == nil && portStr != "" {
			fmt.Printf("  Port:      %s\n", portStr)
		}

		// Show uptime
		if uptime, err := processUptime(pidPath); err == nil {
			fmt.Printf("  Uptime:    %s\n", formatDuration(uptime))
		}

		fmt.Printf("  Log:       %s\n", logPath)

		// Health check
		if portStr, err := readPortFile(); err == nil && portStr != "" {
			client := &http.Client{Timeout: 3 * time.Second}
			resp, err := client.Get(fmt.Sprintf("http://localhost:%s/healthz", portStr))
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
		// Clean up stale PID file
		if err == nil {
			_ = removePID(pidPath)
		}
	}

	// Neo4j container status
	if DockerAvailable() {
		state, err := InspectContainer(neo4jContainerName)
		if err == nil {
			if state.Exists {
				fmt.Printf("  Neo4j:     %s (%s)\n", state.Status, neo4jContainerName)
			} else {
				fmt.Printf("  Neo4j:     not created\n")
			}
		}
	} else {
		fmt.Println("  Neo4j:     docker not available")
	}

	return nil
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
