package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newSidecarCmd creates the parent `sidecar` command with all subcommands.
func newSidecarCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sidecar",
		Short: "Sidecar lifecycle management",
		Long: `Manage the MDEMG sidecar lifecycle for any repository.

Commands:
  init           Initialize sidecar configuration
  status         Show sidecar state and service health
  install        Install sidecar dependencies and runtime
  up             Start sidecar services
  down           Stop sidecar services
  restart        Restart sidecar services
  doctor         Run diagnostics
  attach-agent   Attach an agent adapter
  upgrade        Upgrade sidecar version
  uninstall      Remove sidecar from project`,
	}

	cmd.AddCommand(newSidecarInitCmd())
	cmd.AddCommand(newSidecarStatusCmd())
	cmd.AddCommand(newSidecarInstallCmd())
	cmd.AddCommand(newSidecarUpCmd())
	cmd.AddCommand(newSidecarDownCmd())
	cmd.AddCommand(newSidecarRestartCmd())
	cmd.AddCommand(newSidecarDoctorCmd())
	cmd.AddCommand(newSidecarStubCmd("attach-agent", "Attach an agent adapter"))
	cmd.AddCommand(newSidecarStubCmd("upgrade", "Upgrade sidecar version"))
	cmd.AddCommand(newSidecarStubCmd("uninstall", "Remove sidecar from project"))

	return cmd
}

// newSidecarStubCmd creates a placeholder subcommand that prints "not yet implemented".
func newSidecarStubCmd(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "mdemg sidecar %s: not yet implemented (planned for a future phase)\n", name)
			return nil
		},
	}
}
