package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/tsdb"
)

// newDataCmd creates the parent `data` command with subcommands.
func newDataCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: "Training data collection status and management",
		Long:  "Commands for monitoring and managing LLM interaction training data",
	}

	cmd.AddCommand(newDataStatusCmd())
	cmd.AddCommand(newDataInspectCmd())
	cmd.AddCommand(newDataStatsCmd())
	cmd.AddCommand(newDataAnnotateCmd())
	cmd.AddCommand(newDataQualityCmd())

	return cmd
}

// newDataStatusCmd creates the `data status` subcommand.
func newDataStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show training data collection status",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cfg := tsdbConfigFromEnv()
			client, err := tsdb.NewClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("connect to TSDB: %w", err)
			}
			defer client.Close()

			pool := client.Pool()

			// Per-task counts
			rows, err := pool.Query(ctx,
				`SELECT task_name, COUNT(*) as cnt,
				        COUNT(guidance_id) as with_guid,
				        COUNT(quality) as annotated
				 FROM llm_interactions
				 GROUP BY task_name ORDER BY cnt DESC`)
			if err != nil {
				return fmt.Errorf("query interactions: %w", err)
			}
			defer rows.Close()

			fmt.Println("═══ Training Data Status ═══")
			fmt.Println()
			fmt.Printf("%-30s %8s %10s %10s\n", "Task", "Total", "w/GuidID", "Annotated")
			fmt.Println(strings.Repeat("─", 62))

			var totalRows, totalGuid, totalAnnotated int64
			for rows.Next() {
				var taskName string
				var cnt, withGuid, annotated int64
				if err := rows.Scan(&taskName, &cnt, &withGuid, &annotated); err != nil {
					return err
				}
				fmt.Printf("%-30s %8d %10d %10d\n", taskName, cnt, withGuid, annotated)
				totalRows += cnt
				totalGuid += withGuid
				totalAnnotated += annotated
			}
			fmt.Println(strings.Repeat("─", 62))
			fmt.Printf("%-30s %8d %10d %10d\n", "TOTAL", totalRows, totalGuid, totalAnnotated)

			// JSONL file status
			fmt.Println()
			fmt.Println("═══ JSONL Files ═══")
			jsonlDir := ".mdemg/neural/training-data"
			if info, statErr := os.Stat(jsonlDir); statErr == nil && info.IsDir() {
				entries, _ := os.ReadDir(jsonlDir)
				var totalSize int64
				var fileCount int
				for _, e := range entries {
					if strings.HasSuffix(e.Name(), ".jsonl") {
						fileCount++
						if fi, infoErr := e.Info(); infoErr == nil {
							totalSize += fi.Size()
						}
					}
				}
				fmt.Printf("Directory: %s\n", jsonlDir)
				fmt.Printf("Files:     %d JSONL\n", fileCount)
				fmt.Printf("Size:      %s\n", formatBytes(totalSize))
			} else {
				fmt.Printf("Directory: %s (not found)\n", jsonlDir)
			}

			return nil
		},
	}
}

// newDataInspectCmd creates the `data inspect` subcommand.
func newDataInspectCmd() *cobra.Command {
	var taskName string
	var last int

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "View recent interaction records",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cfg := tsdbConfigFromEnv()
			client, err := tsdb.NewClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("connect to TSDB: %w", err)
			}
			defer client.Close()

			pool := client.Pool()

			query := `SELECT time, trace_id, task_name, guidance_id,
			                  LEFT(user_prompt, 80) as prompt_preview,
			                  LEFT(response, 80) as response_preview,
			                  latency_ms, model_name, quality
			           FROM llm_interactions`
			var args []any
			if taskName != "" {
				query += ` WHERE task_name = $1`
				args = append(args, taskName)
			}
			query += fmt.Sprintf(` ORDER BY time DESC LIMIT %d`, last)

			rows, err := pool.Query(ctx, query, args...)
			if err != nil {
				return fmt.Errorf("query: %w", err)
			}
			defer rows.Close()

			fmt.Printf("%-20s %-14s %-24s %-12s %-40s %6s %-12s %7s\n",
				"Time", "TraceID", "Task", "GuidanceID", "Prompt", "Ms", "Model", "Quality")
			fmt.Println(strings.Repeat("─", 140))

			for rows.Next() {
				var t time.Time
				var traceID, task string
				var guidanceID, promptPreview, responsePreview, modelName *string
				var latencyMs int
				var quality *float64
				if err := rows.Scan(&t, &traceID, &task, &guidanceID,
					&promptPreview, &responsePreview, &latencyMs, &modelName, &quality); err != nil {
					return err
				}
				fmt.Printf("%-20s %-14s %-24s %-12s %-40s %6d %-12s %7s\n",
					t.Format("2006-01-02 15:04:05"),
					truncate(traceID, 14),
					truncate(task, 24),
					deref(guidanceID, "-"),
					deref(promptPreview, "-"),
					latencyMs,
					deref(modelName, "-"),
					formatQuality(quality),
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&taskName, "task", "", "Filter by task name")
	cmd.Flags().IntVar(&last, "last", 20, "Number of recent records to show")
	return cmd
}

// newDataStatsCmd creates the `data stats` subcommand.
func newDataStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Per-task training data statistics",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cfg := tsdbConfigFromEnv()
			client, err := tsdb.NewClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("connect to TSDB: %w", err)
			}
			defer client.Close()

			pool := client.Pool()

			rows, err := pool.Query(ctx,
				`SELECT task_name,
				        COUNT(*) as total,
				        AVG(latency_ms) as avg_latency,
				        AVG(tokens_in + tokens_out) as avg_tokens,
				        COUNT(CASE WHEN error != '' THEN 1 END)::float / GREATEST(COUNT(*), 1) as error_rate,
				        COUNT(quality)::float / GREATEST(COUNT(*), 1) as quality_coverage,
				        AVG(quality) as avg_quality
				 FROM llm_interactions
				 GROUP BY task_name ORDER BY total DESC`)
			if err != nil {
				return fmt.Errorf("query: %w", err)
			}
			defer rows.Close()

			fmt.Println("═══ Training Data Statistics ═══")
			fmt.Println()
			fmt.Printf("%-28s %7s %9s %9s %9s %9s %9s %9s\n",
				"Task", "Total", "AvgMs", "AvgTok", "ErrRate", "QualCov", "AvgQual", "Ready")
			fmt.Println(strings.Repeat("─", 100))

			for rows.Next() {
				var task string
				var total int64
				var avgLatency, avgTokens, errorRate, qualityCoverage *float64
				var avgQuality *float64
				if err := rows.Scan(&task, &total, &avgLatency, &avgTokens,
					&errorRate, &qualityCoverage, &avgQuality); err != nil {
					return err
				}
				ready := "no"
				if total >= 500 && qualityCoverage != nil && *qualityCoverage > 0.5 {
					ready = "YES"
				}
				fmt.Printf("%-28s %7d %9.0f %9.0f %8.1f%% %8.1f%% %9s %9s\n",
					task, total,
					derefF(avgLatency), derefF(avgTokens),
					derefF(errorRate)*100, derefF(qualityCoverage)*100,
					formatQuality(avgQuality), ready)
			}
			return nil
		},
	}
}

// newDataAnnotateCmd creates the `data annotate` subcommand.
func newDataAnnotateCmd() *cobra.Command {
	var dryRun bool
	var taskFilter string

	cmd := &cobra.Command{
		Use:   "annotate",
		Short: "Run quality annotation pipeline",
		RunE: func(_ *cobra.Command, _ []string) error {
			args := []string{"-m", "training.quality_annotator",
				"--tsdb-dsn", tsdbDSN(),
				"--jsonl-dir", ".mdemg/neural/training-data/",
			}
			if dryRun {
				args = append(args, "--dry-run")
			}
			if taskFilter != "" {
				args = append(args, "--task", taskFilter)
			}

			c := exec.Command("python3", args...) //nolint:gosec // G204: args from flags
			c.Dir = filepath.Join(".", "neural")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be annotated without writing")
	cmd.Flags().StringVar(&taskFilter, "task", "", "Annotate only a specific task")
	return cmd
}

// newDataQualityCmd creates the `data quality` subcommand.
func newDataQualityCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "quality",
		Short: "Show training data quality report",
		RunE: func(_ *cobra.Command, _ []string) error {
			c := exec.Command("python3", "-m", "training.quality_report", //nolint:gosec // G204
				"--tsdb-dsn", tsdbDSN(),
			)
			c.Dir = filepath.Join(".", "neural")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

// tsdbDSN builds a PostgreSQL connection string from env vars.
func tsdbDSN() string {
	cfg := tsdbConfigFromEnv()
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.SSLMode)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func deref(s *string, fallback string) string {
	if s == nil || *s == "" {
		return fallback
	}
	return *s
}

func derefF(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func formatQuality(q *float64) string {
	if q == nil {
		return "-"
	}
	return fmt.Sprintf("%.2f", *q)
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
