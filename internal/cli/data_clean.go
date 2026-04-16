package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/tsdb"
)

// newDataCleanCmd creates the `data clean` subcommand.
func newDataCleanCmd() *cobra.Command {
	var (
		spaceID        string
		dryRun         bool
		force          bool
		limit          int
		minResponseLen int
		jsonOutput     bool
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove error records and silent failures from TSDB",
		Long: `Remove LLM interaction records that are harmful to training data quality:

  1. Error records: failed LLM calls with non-empty error field
  2. Silent failures: no error field but response shorter than --min-response-len

Runs in dry-run mode by default (preview only, no deletions).
To delete records, use --dry-run=false --force.

Matching criteria align with neural/training/quality_filter.py gates.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !dryRun && !force {
				return fmt.Errorf("live deletion requires --force flag (use --dry-run=false --force)")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			cfg := tsdbConfigFromEnv()
			client, err := tsdb.NewClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("connect to TSDB: %w", err)
			}
			defer client.Close()

			result, err := tsdb.RunClean(ctx, client.Pool(), tsdb.CleanConfig{
				SpaceID:        spaceID,
				DryRun:         dryRun,
				Limit:          limit,
				MinResponseLen: minResponseLen,
			})
			if err != nil {
				return err
			}

			if jsonOutput {
				out, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(out))
				return nil
			}

			printCleanResult(result)
			return nil
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "mdemg-dev", "Space ID to clean")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Preview only — no deletions (default: true)")
	cmd.Flags().BoolVar(&force, "force", false, "Required alongside --dry-run=false to confirm deletion")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max rows to delete per category (0 = unlimited)")
	cmd.Flags().IntVar(&minResponseLen, "min-response-len", 10, "Minimum response length — shorter responses are silent failures")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func printCleanResult(r *tsdb.CleanResult) {
	mode := "DRY RUN"
	if !r.DryRun {
		mode = "LIVE"
	}

	fmt.Printf("\n═══ Data Clean (%s) ═══\n\n", mode)

	fmt.Printf("  Total before:       %d\n", r.TotalBefore)
	fmt.Printf("  Error records:      %d\n", r.ErrorsFound)
	fmt.Printf("  Silent failures:    %d\n", r.SilentFailsFound)
	fmt.Printf("  Total to remove:    %d\n", r.ErrorsFound+r.SilentFailsFound)
	fmt.Println()

	if !r.DryRun {
		fmt.Printf("  Errors deleted:     %d\n", r.ErrorsDeleted)
		fmt.Printf("  Silent fails del:   %d\n", r.SilentFailsDeleted)
		fmt.Printf("  Total after:        %d\n", r.TotalAfter)
		fmt.Println()
	}

	if len(r.Preview) > 0 {
		fmt.Println("  Preview (sample records to remove):")
		fmt.Printf("  %-12s %-28s %-20s %s\n", "Category", "Task", "Time", "Detail")
		fmt.Printf("  %s\n", strings.Repeat("─", 80))
		for _, p := range r.Preview {
			detail := p.ErrorSnippet
			if p.Category == "silent_fail" {
				detail = fmt.Sprintf("response_len=%d", p.ResponseLen)
			}
			if len(detail) > 40 {
				detail = detail[:37] + "..."
			}
			fmt.Printf("  %-12s %-28s %-20s %s\n",
				p.Category, p.TaskName, p.Time.Format("2006-01-02 15:04:05"), detail)
		}
		fmt.Println()
	}

	if len(r.PerTaskCounts) > 0 {
		fmt.Println("  Clean records per task:")
		fmt.Printf("  %-30s %8s\n", "Task", "Records")
		fmt.Printf("  %s\n", strings.Repeat("─", 40))
		var total int64
		for task, count := range r.PerTaskCounts {
			fmt.Printf("  %-30s %8d\n", task, count)
			total += count
		}
		fmt.Printf("  %s\n", strings.Repeat("─", 40))
		fmt.Printf("  %-30s %8d\n", "TOTAL", total)
		fmt.Println()
	}

	if r.DryRun {
		fmt.Println("  (dry-run — no records were deleted)")
		fmt.Println("  To delete, run with: --dry-run=false --force")
	}
	fmt.Println()
}
