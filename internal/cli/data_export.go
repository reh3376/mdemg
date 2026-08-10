package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/tsdb"
)

// Version and Commit are set in root.go via -ldflags.

func newDataExportCmd() *cobra.Command {
	var (
		output           string
		instanceID       string
		since            string
		until            string
		tables           []string
		excludeEmbedding bool
		dryRun           bool
		noValidate       bool
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export training data as UTDS archive",
		Long: `Export LLM interaction, retrieval, and embedding data from TSDB as a
UTDS-compliant .tar.gz archive for downstream curation and LoRA fine-tuning.

The export includes privacy scanning of all text fields. If any PII is
detected, the export is blocked and no archive is created.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			spaceID := resolveSpaceID(cmd)
			if spaceID == "" {
				return fmt.Errorf("--space-id is required (or set MDEMG_SPACE_ID)")
			}

			// Resolve instance ID: flag > MDEMG_INSTANCE_ID env > auto-generate
			instanceID = resolveInstanceID(instanceID, spaceID)

			// Parse time range
			var fromTime, toTime time.Time
			var err error
			if since != "" {
				fromTime, err = time.Parse(time.RFC3339, since)
				if err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
			} else {
				fromTime = time.Now().AddDate(0, -6, 0) // 180 days ago
			}
			if until != "" {
				toTime, err = time.Parse(time.RFC3339, until)
				if err != nil {
					return fmt.Errorf("invalid --until: %w", err)
				}
			} else {
				toTime = time.Now().UTC()
			}

			// Handle --exclude-embedding
			if excludeEmbedding {
				filtered := make([]string, 0, len(tables))
				for _, t := range tables {
					if t != "embedding_events" {
						filtered = append(filtered, t)
					}
				}
				tables = filtered
			}

			// Resolve output path
			if output == "" {
				output = fmt.Sprintf("mdemg-export-%s-%s.tar.gz",
					instanceID, time.Now().Format("20060102-150405"))
			}

			cfg := tsdb.ExportConfig{
				SpaceID:    spaceID,
				From:       fromTime,
				To:         toTime,
				Tables:     tables,
				InstanceID: instanceID,
				OutputPath: output,
			}

			// Connect to TSDB
			tsdbCfg := tsdbConfigFromEnv()
			client, err := tsdb.NewClient(ctx, tsdbCfg)
			if err != nil {
				return fmt.Errorf("connect to TSDB: %w", err)
			}
			defer client.Close()
			pool := client.Pool()

			// Dry run — show counts only
			if dryRun {
				counts, err := tsdb.DryRunExport(ctx, pool, cfg)
				if err != nil {
					return fmt.Errorf("dry run: %w", err)
				}
				var total int64
				for t, c := range counts {
					fmt.Printf("  %-25s %d rows\n", t, c)
					total += c
				}
				fmt.Printf("  %-25s %d rows\n", "TOTAL", total)
				return nil
			}

			// Run export
			result, err := tsdb.RunExport(ctx, pool, cfg, Version, Commit)
			if err != nil {
				return err
			}

			fmt.Printf("Export complete: %s\n", result.OutputPath)
			fmt.Printf("  Export ID:  %s\n", result.Manifest.ExportID)
			fmt.Printf("  Tables:     %d\n", len(result.Manifest.Tables))
			fmt.Printf("  Total rows: %d\n", result.TotalRows)
			fmt.Printf("  Privacy:    %d violations\n", result.Manifest.Quality.PrivacyScrubViolations)

			if result.TotalRows == 0 {
				fmt.Fprintf(os.Stderr, "WARNING: Export produced 0 rows for space_id=%q.\n", spaceID)
				if instanceID != "" {
					fmt.Fprintf(os.Stderr, "  Instance filter: %q — try without --instance-id to see all data.\n", instanceID)
				}
			}

			if !noValidate {
				fmt.Println("\nTo validate: python docs/tests/utds/runners/utds_runner.py validate --archive", result.OutputPath)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&output, "output", "", "Output archive path (default: mdemg-export-{instance}-{date}.tar.gz)")
	cmd.Flags().StringVar(&instanceID, "instance-id", "", "Instance identifier (default: all instances)")
	cmd.Flags().StringVar(&since, "since", "", "Start time (RFC3339, default: 180 days ago)")
	cmd.Flags().StringVar(&until, "until", "", "End time (RFC3339, default: now)")
	cmd.Flags().StringSliceVar(&tables, "tables", []string{"llm_interactions", "retrieval_events", "embedding_events", "guidance_training_rows"},
		"Tables to export (B5a added guidance_training_rows as the 4th default; the beta-import receiver consumes it into the local retrain corpus)")
	cmd.Flags().BoolVar(&excludeEmbedding, "exclude-embedding", false, "Skip embedding_events table")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show row counts without exporting")
	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "Skip post-export validation hint")

	return cmd
}
