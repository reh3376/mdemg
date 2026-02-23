// Package cli implements the unified MDEMG command-line interface using Cobra.
// All subcommands are registered here and wired into a single binary.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Build-time variables set via -ldflags
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// newRootCmd creates the root command for the unified MDEMG CLI.
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "mdemg",
		Short: "Multi-Dimensional Emergent Memory Graph",
		Long: `MDEMG — Multi-Dimensional Emergent Memory Graph

A persistent memory system for LLMs providing vector-based semantic search,
graph-based knowledge representation, hidden layer concept abstraction,
learning edges (Hebbian reinforcement), and LLM re-ranking for improved retrieval.

Use "mdemg <command> --help" for more information about a command.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
	}

	// Global persistent flags
	rootCmd.PersistentFlags().Bool("verbose", false, "Enable verbose output")

	// Register all subcommands
	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newMCPCmd())
	rootCmd.AddCommand(newIngestCmd())
	rootCmd.AddCommand(newConsolidateCmd())
	rootCmd.AddCommand(newDecayCmd())
	rootCmd.AddCommand(newPruneCmd())
	rootCmd.AddCommand(newExtractSymbolsCmd())
	rootCmd.AddCommand(newWatchCmd())
	rootCmd.AddCommand(newDBCmd())
	rootCmd.AddCommand(newSpaceCmd())
	rootCmd.AddCommand(newPluginCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

// Execute runs the root command. This is the main entry point for the CLI.
func Execute() error {
	return newRootCmd().Execute()
}

// ExecuteSubcommand runs a specific subcommand with the given args.
// Used by legacy deprecation shims to delegate to the unified CLI.
// subcmdPath is the command path (e.g., ["serve"] or ["db", "reset"]).
func ExecuteSubcommand(subcmdPath []string, args []string) error {
	root := newRootCmd()
	cmdArgs := append(subcmdPath, args...)
	root.SetArgs(cmdArgs)
	return root.Execute()
}

// RunLegacyShim prints a deprecation warning and delegates to the unified CLI.
// oldName is the deprecated binary name (e.g., "mdemg-server").
// replacement is the new command (e.g., "mdemg serve").
// subcmdPath is the Cobra subcommand path (e.g., ["serve"] or ["db", "reset"]).
func RunLegacyShim(oldName, replacement string, subcmdPath []string) {
	fmt.Fprintf(os.Stderr, "WARNING: '%s' is deprecated. Use '%s' instead.\n\n", oldName, replacement)
	if err := ExecuteSubcommand(subcmdPath, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
