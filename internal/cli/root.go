// Package cli implements the unified MDEMG command-line interface using Cobra.
// All subcommands are registered here and wired into a single binary.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// resolveSpaceID returns the space ID using the following priority:
//  1. Local --space-id flag (if explicitly passed by the user)
//  2. Global --space-id persistent flag
//  3. MDEMG_SPACE_ID environment variable
//  4. The local flag's default value (e.g., "codebase" for ingest, "" for consolidate)
func resolveSpaceID(cmd *cobra.Command) string {
	// If user explicitly passed --space-id on this command, use that
	if f := cmd.Flags().Lookup("space-id"); f != nil && f.Changed {
		return f.Value.String()
	}
	// Global persistent --space-id flag
	if f := cmd.Root().PersistentFlags().Lookup("space-id"); f != nil && f.Changed {
		return f.Value.String()
	}
	// MDEMG_SPACE_ID environment variable
	if v := os.Getenv("MDEMG_SPACE_ID"); v != "" {
		return v
	}
	// Fall back to the local flag's default (e.g., "codebase" for ingest, "" for consolidate)
	if f := cmd.Flags().Lookup("space-id"); f != nil {
		return f.DefValue
	}
	return ""
}

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
	rootCmd.PersistentFlags().String("space-id", "", "Default space ID (overrides MDEMG_SPACE_ID env var)")

	// Command groups for organized help output
	rootCmd.AddGroup(
		&cobra.Group{ID: "start", Title: "Getting Started"},
		&cobra.Group{ID: "server", Title: "Server Lifecycle"},
		&cobra.Group{ID: "database", Title: "Database Management"},
		&cobra.Group{ID: "memory", Title: "Memory & Ingestion"},
		&cobra.Group{ID: "config", Title: "Configuration"},
		&cobra.Group{ID: "advanced", Title: "Advanced"},
	)

	// Getting Started
	initCmd := newInitCmd()
	initCmd.GroupID = "start"
	rootCmd.AddCommand(initCmd)

	versionCmd := newVersionCmd()
	versionCmd.GroupID = "start"
	rootCmd.AddCommand(versionCmd)

	// Server Lifecycle
	startCmd := newStartCmd()
	startCmd.GroupID = "server"
	rootCmd.AddCommand(startCmd)

	stopCmd := newStopCmd()
	stopCmd.GroupID = "server"
	rootCmd.AddCommand(stopCmd)

	restartCmd := newRestartCmd()
	restartCmd.GroupID = "server"
	rootCmd.AddCommand(restartCmd)

	statusCmd := newStatusCmd()
	statusCmd.GroupID = "server"
	rootCmd.AddCommand(statusCmd)

	serveCmd := newServeCmd()
	serveCmd.GroupID = "server"
	rootCmd.AddCommand(serveCmd)

	serviceCmd := newServiceCmd()
	serviceCmd.GroupID = "server"
	rootCmd.AddCommand(serviceCmd)

	// Database Management
	dbCmd := newDBCmd()
	dbCmd.GroupID = "database"
	rootCmd.AddCommand(dbCmd)

	tsdbCmd := newTSDBCmd()
	tsdbCmd.GroupID = "database"
	rootCmd.AddCommand(tsdbCmd)

	graphCmd := newGraphCmd()
	graphCmd.GroupID = "database"
	rootCmd.AddCommand(graphCmd)

	// Memory & Ingestion
	ingestCmd := newIngestCmd()
	ingestCmd.GroupID = "memory"
	rootCmd.AddCommand(ingestCmd)

	ingestClaudeMDCmd := newIngestClaudeMDCmd()
	ingestClaudeMDCmd.GroupID = "memory"
	rootCmd.AddCommand(ingestClaudeMDCmd)

	consolidateCmd := newConsolidateCmd()
	consolidateCmd.GroupID = "memory"
	rootCmd.AddCommand(consolidateCmd)

	embeddingsCmd := newEmbeddingsCmd()
	embeddingsCmd.GroupID = "memory"
	rootCmd.AddCommand(embeddingsCmd)

	extractSymbolsCmd := newExtractSymbolsCmd()
	extractSymbolsCmd.GroupID = "memory"
	rootCmd.AddCommand(extractSymbolsCmd)

	// Configuration
	configCmd := newConfigCmd()
	configCmd.GroupID = "config"
	rootCmd.AddCommand(configCmd)

	hooksCmd := newHooksCmd()
	hooksCmd.GroupID = "config"
	rootCmd.AddCommand(hooksCmd)

	sidecarCmd := newSidecarCmd()
	sidecarCmd.GroupID = "config"
	rootCmd.AddCommand(sidecarCmd)

	upgradeCmd := newUpgradeCmd()
	upgradeCmd.GroupID = "config"
	rootCmd.AddCommand(upgradeCmd)

	menubarCmd := newMenubarCmd()
	menubarCmd.GroupID = "config"
	rootCmd.AddCommand(menubarCmd)

	teardownCmd := newTeardownCmd()
	teardownCmd.GroupID = "config"
	rootCmd.AddCommand(teardownCmd)

	synergyCmd := newSynergyCmd()
	synergyCmd.GroupID = "config"
	rootCmd.AddCommand(synergyCmd)

	// Advanced
	mcpCmd := newMCPCmd()
	mcpCmd.GroupID = "advanced"
	rootCmd.AddCommand(mcpCmd)

	decayCmd := newDecayCmd()
	decayCmd.GroupID = "advanced"
	rootCmd.AddCommand(decayCmd)

	pruneCmd := newPruneCmd()
	pruneCmd.GroupID = "advanced"
	rootCmd.AddCommand(pruneCmd)

	watchCmd := newWatchCmd()
	watchCmd.GroupID = "advanced"
	rootCmd.AddCommand(watchCmd)

	spaceCmd := newSpaceCmd()
	spaceCmd.GroupID = "advanced"
	rootCmd.AddCommand(spaceCmd)

	pluginCmd := newPluginCmd()
	pluginCmd.GroupID = "advanced"
	rootCmd.AddCommand(pluginCmd)

	demoCmd := newDemoCmd()
	demoCmd.GroupID = "advanced"
	rootCmd.AddCommand(demoCmd)

	dataCmd := newDataCmd()
	dataCmd.GroupID = "advanced"
	rootCmd.AddCommand(dataCmd)

	maintenanceCmd := newMaintenanceCmd()
	maintenanceCmd.GroupID = "advanced"
	rootCmd.AddCommand(maintenanceCmd)

	return rootCmd
}

// NewRootCmdForDocs returns the root command for documentation generation.
// Used by cmd/gendocs to generate man pages and markdown docs.
func NewRootCmdForDocs() *cobra.Command {
	return newRootCmd()
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
