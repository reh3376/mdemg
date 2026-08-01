// Package cli provides Cobra commands for the unified MDEMG CLI.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"

	"mdemg/internal/symbols"
)

// newAnalyzeGoImplementsCmd creates the `symbols analyze-go-implements`
// subcommand — GO-IMPLEMENTS-001. Runs the go/types-backed IMPLEMENTS
// analyzer against a Go project root and writes the discovered edges into
// Neo4j via the existing symbols.Store.SaveRelationships path.
//
// Why a separate command (not a batch-ingest hook): go/packages.Load needs
// the WHOLE module to resolve imports transitively; the batch-ingest path
// is per-observation. Operators run this manually (or on a schedule) when
// they want fresh IMPLEMENTS data.
func newAnalyzeGoImplementsCmd() *cobra.Command {
	var (
		root      string
		spaceID   string
		neo4jURI  string
		neo4jUser string
		neo4jPass string
		dryRun    bool
	)

	cmd := &cobra.Command{
		Use:   "analyze-go-implements",
		Short: "Discover Go IMPLEMENTS edges via go/types and write to Neo4j",
		Long: `Discover Go's implicit interface satisfaction across a project and
write IMPLEMENTS edges into Neo4j (GO-IMPLEMENTS-001).

Go has no ` + "`implements`" + ` keyword — a struct implements an interface
just by having the right method set — so the tree-sitter query path used
for other languages CANNOT detect Go IMPLEMENTS. This command uses Go's own
type checker (go/types) to compute the method-set matches.

symbol_ids for both sides are computed via GenerateSymbolID(spaceID, path,
name, 0) to match the deterministic hash the SymbolNode ingest path uses
— so SaveRelationships' MATCH clauses find the existing nodes.

Examples:
  mdemg symbols analyze-go-implements --root . --space-id mdemg-dev
  mdemg symbols analyze-go-implements --root /path/to/repo --space-id foo --dry-run`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if root == "" {
				return fmt.Errorf("--root required")
			}
			if spaceID == "" {
				return fmt.Errorf("--space-id required")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			start := time.Now()
			analyzer := symbols.NewGoTypesAnalyzer()
			slog.Info("analyze-go-implements: starting",
				"root", root, "space_id", spaceID, "dry_run", dryRun)

			rels, err := analyzer.AnalyzeImplements(ctx, spaceID, root)
			if err != nil {
				return fmt.Errorf("analyze: %w", err)
			}

			fmt.Fprintf(os.Stdout, "\nDiscovered %d IMPLEMENTS pair(s) in %v\n",
				len(rels), time.Since(start).Round(time.Millisecond))

			if len(rels) == 0 {
				fmt.Fprintln(os.Stdout, "No pairs to write.")
				return nil
			}

			// Show a sample (first 10) for operator sanity.
			max := 10
			if len(rels) < max {
				max = len(rels)
			}
			fmt.Fprintf(os.Stdout, "Sample (first %d):\n", max)
			for i := 0; i < max; i++ {
				r := rels[i]
				fmt.Fprintf(os.Stdout, "  %s -[IMPLEMENTS]-> %s\n",
					r.SourceSymbolID, r.TargetSymbolID)
			}
			if len(rels) > max {
				fmt.Fprintf(os.Stdout, "  ... (%d more)\n", len(rels)-max)
			}

			if dryRun {
				fmt.Fprintln(os.Stdout, "\n[dry-run] not writing to Neo4j.")
				return nil
			}

			driver, err := neo4j.NewDriverWithContext(neo4jURI,
				neo4j.BasicAuth(neo4jUser, neo4jPass, ""))
			if err != nil {
				return fmt.Errorf("neo4j driver: %w", err)
			}
			defer func() { _ = driver.Close(ctx) }()

			store := symbols.NewStore(driver)
			writeStart := time.Now()
			if err := store.SaveRelationships(ctx, spaceID, rels); err != nil {
				return fmt.Errorf("save relationships: %w", err)
			}
			fmt.Fprintf(os.Stdout, "\nWrote %d IMPLEMENTS edges to Neo4j in %v.\n",
				len(rels), time.Since(writeStart).Round(time.Millisecond))
			return nil
		},
	}

	cmd.Flags().StringVar(&root, "root", "", "Project root to analyze (required)")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "Neo4j space to write into (required)")
	cmd.Flags().StringVar(&neo4jURI, "neo4j-uri", "bolt://localhost:7687", "Neo4j URI")
	cmd.Flags().StringVar(&neo4jUser, "neo4j-user", "neo4j", "Neo4j username")
	cmd.Flags().StringVar(&neo4jPass, "neo4j-pass", "testpassword", "Neo4j password")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Compute + print the pairs without writing")

	return cmd
}

// newSymbolsCmd creates the `symbols` command group. Currently just
// analyze-go-implements; the shipped `mdemg extract-symbols` stays as a
// top-level command for backward compatibility.
func newSymbolsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "symbols",
		Short: "Symbol-graph maintenance commands",
	}
	cmd.AddCommand(newAnalyzeGoImplementsCmd())
	return cmd
}
