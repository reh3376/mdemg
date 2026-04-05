package cli

import "github.com/spf13/cobra"

// newGraphCmd creates the parent `graph` command with subcommands.
func newGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Graph maintenance and repair commands",
		Long:  "Commands for inspecting and repairing the MDEMG knowledge graph.",
	}

	cmd.AddCommand(newGraphRepairCmd())

	return cmd
}
