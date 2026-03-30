package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// serviceManager defines the platform-specific service management interface.
// Implementations: service_darwin.go (launchd), service_linux.go (systemd), service_stub.go (unsupported).
type serviceManager interface {
	Install(projectDir, mdemgBin, spaceID string) error
	Uninstall() error
	Status() error
	Restart() error
	Logs(follow bool) error
}

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage MDEMG background services (launchd/systemd)",
		Long: `Manage OS-level process supervision for MDEMG services.

Unlike 'mdemg start' (manual, PID-file-based), 'mdemg service install'
configures the OS to automatically restart MDEMG on crash or reboot.

Subcommands:
  install    Install LaunchAgent (macOS) or systemd units (Linux)
  uninstall  Remove supervision configuration
  status     Show supervised service states
  restart    Restart all supervised services
  logs       Tail service log files`,
	}

	cmd.AddCommand(newServiceInstallCmd())
	cmd.AddCommand(newServiceUninstallCmd())
	cmd.AddCommand(newServiceStatusCmd())
	cmd.AddCommand(newServiceRestartCmd())
	cmd.AddCommand(newServiceLogsCmd())

	return cmd
}

func newServiceInstallCmd() *cobra.Command {
	var spaceID string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install OS-level process supervision for MDEMG services",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := newPlatformServiceManager()

			cwd, err := resolveProjectDir()
			if err != nil {
				return err
			}

			bin, err := resolveMdemgBin()
			if err != nil {
				return err
			}

			if spaceID == "" {
				spaceID = resolveSpaceID(cmd)
			}
			if spaceID == "" {
				spaceID = "mdemg-dev"
			}

			return mgr.Install(cwd, bin, spaceID)
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space ID for ingest timer (default: mdemg-dev)")
	return cmd
}

func newServiceUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove OS-level process supervision",
		RunE: func(cmd *cobra.Command, args []string) error {
			return newPlatformServiceManager().Uninstall()
		},
	}
}

func newServiceStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show supervised service states",
		RunE: func(cmd *cobra.Command, args []string) error {
			return newPlatformServiceManager().Status()
		},
	}
}

func newServiceRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart all supervised services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return newPlatformServiceManager().Restart()
		},
	}
}

func newServiceLogsCmd() *cobra.Command {
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Tail service log files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return newPlatformServiceManager().Logs(follow)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	return cmd
}

// resolveProjectDir returns the current working directory as the project root.
func resolveProjectDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}
	return cwd, nil
}

// resolveMdemgBin finds the mdemg binary path.
func resolveMdemgBin() (string, error) {
	cwd, _ := os.Getwd()
	localBin := cwd + "/bin/mdemg"
	if _, err := os.Stat(localBin); err == nil {
		return localBin, nil
	}

	paths := []string{
		"/opt/homebrew/bin/mdemg",
		"/usr/local/bin/mdemg",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("mdemg binary not found (checked ./bin/mdemg, /opt/homebrew/bin/mdemg, /usr/local/bin/mdemg)")
}
