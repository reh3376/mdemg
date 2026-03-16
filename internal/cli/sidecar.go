package cli

import (
	"github.com/spf13/cobra"
)

// newSidecarCmd creates the parent `sidecar` command with all subcommands.
func newSidecarCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sidecar",
		Short: "Sidecar lifecycle management",
		Long: `Manage the MDEMG sidecar lifecycle for any repository.

Commands:
  init             Initialize sidecar configuration
  status           Show sidecar state and service health
  install          Install sidecar dependencies and runtime
  up               Start sidecar services
  down             Stop sidecar services
  restart          Restart sidecar services
  doctor           Run diagnostics
  attach-agent     Attach an agent adapter
  detach-agent     Detach an agent adapter
  generate-hooks   Generate project-scoped Claude Code hooks
  quickstart       One-command setup (init + install + up + attach + hooks)
  upgrade          Upgrade sidecar version
  uninstall        Remove sidecar from project`,
	}

	cmd.AddCommand(newSidecarInitCmd())
	cmd.AddCommand(newSidecarStatusCmd())
	cmd.AddCommand(newSidecarInstallCmd())
	cmd.AddCommand(newSidecarUpCmd())
	cmd.AddCommand(newSidecarDownCmd())
	cmd.AddCommand(newSidecarRestartCmd())
	cmd.AddCommand(newSidecarDoctorCmd())
	cmd.AddCommand(newSidecarAttachAgentCmd())
	cmd.AddCommand(newSidecarDetachAgentCmd())
	cmd.AddCommand(newSidecarGenerateHooksCmd())
	cmd.AddCommand(newSidecarQuickstartCmd())
	cmd.AddCommand(newSidecarUpgradeCmd())
	cmd.AddCommand(newSidecarUninstallCmd())

	return cmd
}
