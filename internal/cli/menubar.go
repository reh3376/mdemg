//go:build darwin

package cli

import (
	"fmt"
	"os"
	osExec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const menubarProcessName = "MdemgMenuBar"

func newMenubarCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "menubar",
		Short: "Manage the MDEMG menu bar app (macOS only)",
		Long: `Start, stop, restart, or check the status of the MDEMG menu bar companion app.

The menu bar app provides a system tray icon that shows server health status
and quick access to MDEMG features.

Examples:
  mdemg menubar start
  mdemg menubar stop
  mdemg menubar status`,
	}

	cmd.AddCommand(
		newMenubarStartCmd(),
		newMenubarStopCmd(),
		newMenubarRestartCmd(),
		newMenubarStatusCmd(),
	)

	return cmd
}

func newMenubarStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Launch the MDEMG menu bar app",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMenubarStart()
		},
	}
}

func newMenubarStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the MDEMG menu bar app",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMenubarStop()
		},
	}
}

func newMenubarRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the MDEMG menu bar app",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = runMenubarStop()
			time.Sleep(1 * time.Second)
			return runMenubarStart()
		},
	}
}

func newMenubarStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show menu bar app status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMenubarStatus()
		},
	}
}

// --- Implementation ---

func runMenubarStart() error {
	if pid := menubarPID(); pid > 0 {
		fmt.Printf("MDEMG menu bar is already running (pid=%d)\n", pid)
		return nil
	}

	appPath, err := findMenubarApp()
	if err != nil {
		return fmt.Errorf("menu bar app not found: %w", err)
	}

	cmd := osExec.Command("open", appPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("launch menu bar app: %w", err)
	}

	fmt.Printf("MDEMG menu bar started (%s)\n", appPath)
	return nil
}

func runMenubarStop() error {
	pid := menubarPID()
	if pid == 0 {
		fmt.Println("MDEMG menu bar is not running")
		return nil
	}

	fmt.Printf("Stopping MDEMG menu bar (pid=%d)...\n", pid)
	if err := terminateProcess(pid); err != nil {
		return fmt.Errorf("terminate menu bar: %w", err)
	}

	// Poll for exit (up to 10s)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !isProcessAlive(pid) {
			fmt.Println("MDEMG menu bar stopped")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Force kill
	fmt.Println("Warning: graceful shutdown timed out, force killing")
	_ = forceKillProcess(pid)
	time.Sleep(1 * time.Second)
	fmt.Println("MDEMG menu bar killed")
	return nil
}

func runMenubarStatus() error {
	pid := menubarPID()
	if pid == 0 {
		fmt.Println("MDEMG menu bar: stopped")
	} else {
		fmt.Printf("MDEMG menu bar: running (pid=%d)\n", pid)
	}

	if appPath, err := findMenubarApp(); err == nil {
		fmt.Printf("App path:       %s\n", appPath)
	} else {
		fmt.Println("App path:       not found")
	}

	return nil
}

// menubarPID returns the PID of the running MdemgMenuBar process, or 0 if not running.
func menubarPID() int {
	out, err := osExec.Command("pgrep", "-x", menubarProcessName).Output()
	if err != nil {
		return 0
	}
	// pgrep may return multiple PIDs (one per line); take the first
	line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	pid, err := strconv.Atoi(line)
	if err != nil {
		return 0
	}
	return pid
}

// findMenubarApp searches for MdemgMenuBar.app in standard locations.
// Search order: ~/Applications, /Applications, Spotlight fallback.
func findMenubarApp() (string, error) {
	home, _ := os.UserHomeDir()

	candidates := []string{
		filepath.Join(home, "Applications", "MdemgMenuBar.app"),
		"/Applications/MdemgMenuBar.app",
	}

	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path, nil
		}
	}

	// Spotlight fallback
	out, err := osExec.Command("mdfind", "kMDItemCFBundleIdentifier == 'com.reh3376.mdemg-menubar'").Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				if info, err := os.Stat(line); err == nil && info.IsDir() {
					return line, nil
				}
			}
		}
	}

	return "", fmt.Errorf("MdemgMenuBar.app not found in ~/Applications, /Applications, or Spotlight")
}
