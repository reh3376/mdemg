package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mdemg/internal/sanitize"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"mdemg/internal/config"
	"mdemg/internal/tsdb"
)

// newDataCmd creates the parent `data` command with subcommands.
func newDataCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: "Training data collection status and management",
		Long:  "Commands for monitoring and managing LLM interaction training data",
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			_ = godotenv.Load() // Load .env for TSDB_HOST_PORT and other settings
		},
	}

	cmd.AddCommand(newDataStatusCmd())
	cmd.AddCommand(newDataInspectCmd())
	cmd.AddCommand(newDataStatsCmd())
	cmd.AddCommand(newDataAnnotateCmd())
	cmd.AddCommand(newDataQualityCmd())
	cmd.AddCommand(newDataAuditCmd())
	cmd.AddCommand(newDataExportCmd())
	cmd.AddCommand(newDataExportAutoCmd())
	cmd.AddCommand(newDataCheckCmd())
	cmd.AddCommand(newDataCurateCmd())
	cmd.AddCommand(newDataCurateGuidanceCmd())
	cmd.AddCommand(newDataValidateCmd())
	cmd.AddCommand(newDataCleanCmd())

	return cmd
}

// newDataStatusCmd creates the `data status` subcommand.
func newDataStatusCmd() *cobra.Command {
	var (
		warn       bool
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show training data collection status",
		Long: `Show per-task row counts, accumulation rates, and projected days to readiness.
Use --warn to exit non-zero if any task is below the readiness threshold.
Use --json for structured output suitable for monitoring pipelines.`,
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

			type taskRow struct {
				Name      string `json:"task_name"`
				Total     int64  `json:"total"`
				WithGuid  int64  `json:"with_guidance_id"`
				Annotated int64  `json:"annotated"`
			}
			var taskRows []taskRow
			var totalRows, totalGuid, totalAnnotated int64
			for rows.Next() {
				var tr taskRow
				if err := rows.Scan(&tr.Name, &tr.Total, &tr.WithGuid, &tr.Annotated); err != nil {
					return err
				}
				taskRows = append(taskRows, tr)
				totalRows += tr.Total
				totalGuid += tr.WithGuid
				totalAnnotated += tr.Annotated
			}

			// Readiness with accumulation rates (AMD-7: honor the configured
			// threshold + per-task overrides so the CLI matches what RSIC sees).
			builder := tsdb.NewDatasetBuilder(pool)
			if cfg, cErr := config.FromEnv(); cErr == nil {
				builder.SetReadinessThresholds(cfg.TrainingReadinessThreshold, cfg.TrainingReadinessThresholdOverrides)
			}
			readiness, readErr := builder.TrainingDataReadiness(ctx)

			// Embedding + retrieval counts
			var embTotal, embCached int64
			_ = pool.QueryRow(ctx,
				`SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(CASE WHEN cached THEN 1 ELSE 0 END), 0)
				 FROM embedding_events`).Scan(&embTotal, &embCached)
			var retTotal int64
			_ = pool.QueryRow(ctx,
				`SELECT COALESCE(COUNT(*), 0) FROM retrieval_events`).Scan(&retTotal)

			// JSON output
			if jsonOutput {
				report := map[string]any{
					"tasks":            taskRows,
					"total_rows":       totalRows,
					"embedding_events": embTotal,
					"retrieval_events": retTotal,
				}
				if readErr == nil && readiness != nil {
					report["readiness"] = readiness.Tasks
				}
				out, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(out))

				if warn {
					return checkWarnThresholds(readiness)
				}
				return nil
			}

			// Human-readable output
			fmt.Println("═══ Training Data Status ═══")
			fmt.Println()
			fmt.Printf("%-30s %8s %10s %10s\n", "Task", "Total", "w/GuidID", "Annotated")
			fmt.Println(strings.Repeat("─", 62))
			for _, tr := range taskRows {
				fmt.Printf("%-30s %8d %10d %10d\n", tr.Name, tr.Total, tr.WithGuid, tr.Annotated)
			}
			fmt.Println(strings.Repeat("─", 62))
			fmt.Printf("%-30s %8d %10d %10d\n", "TOTAL", totalRows, totalGuid, totalAnnotated)

			// Readiness + rates
			if readErr == nil && readiness != nil {
				fmt.Println()
				fmt.Println("═══ Readiness & Accumulation ═══")
				fmt.Printf("%-30s %8s %10s %10s %8s\n", "Task", "Rows", "Rate/day", "Days Left", "Ready")
				fmt.Println(strings.Repeat("─", 72))
				for _, t := range readiness.Tasks {
					readyStr := "NO"
					if t.Ready {
						readyStr = "YES"
					}
					daysStr := "-"
					if t.ProjectedDaysToReady > 0 {
						daysStr = fmt.Sprintf("%d", t.ProjectedDaysToReady)
					}
					fmt.Printf("%-30s %8d %10.1f %10s %8s\n",
						t.TaskName, t.TotalRows, t.AvgDailyRate, daysStr, readyStr)
					// SF-7: surface why a task is not ready (which gate failed).
					for _, reason := range t.NotReadyReasons {
						fmt.Printf("%-30s   └─ %s\n", "", reason)
					}
				}
			}

			// Embedding event counts
			fmt.Println()
			fmt.Println("═══ Embedding Events ═══")
			if embTotal > 0 {
				cacheRate := 0.0
				if embTotal > 0 {
					cacheRate = float64(embCached) / float64(embTotal) * 100
				}
				fmt.Printf("Total events:  %d\n", embTotal)
				fmt.Printf("Cache hits:    %d (%.1f%%)\n", embCached, cacheRate)
			} else {
				fmt.Println("  (no events)")
			}

			// Retrieval event counts
			fmt.Println()
			fmt.Println("═══ Retrieval Events ═══")
			if retTotal > 0 {
				fmt.Printf("Total events:  %d\n", retTotal)
			} else {
				fmt.Println("  (no events)")
			}

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

			if warn {
				return checkWarnThresholds(readiness)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&warn, "warn", false, "Exit non-zero if any task is below readiness threshold")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON for monitoring pipelines")

	return cmd
}

// checkWarnThresholds returns an error if any readiness threshold is unmet.
func checkWarnThresholds(readiness *tsdb.TrainingDataReadiness) error {
	if readiness == nil {
		return fmt.Errorf("WARNING: could not check readiness (TSDB query failed)")
	}
	var warnings []string
	for _, t := range readiness.Tasks {
		if !t.Ready {
			warnings = append(warnings, fmt.Sprintf("  %s: %d rows (need %d)",
				t.TaskName, t.TotalRows, tsdb.DefaultReadinessThreshold))
		}
		if t.DistinctPromptHashes > 1 {
			warnings = append(warnings, fmt.Sprintf("  %s: %d distinct prompt hashes (drift?)",
				t.TaskName, t.DistinctPromptHashes))
		}
	}
	if len(warnings) > 0 {
		fmt.Fprintln(os.Stderr, "\nWARNINGS:")
		for _, w := range warnings {
			fmt.Fprintln(os.Stderr, w)
		}
		return fmt.Errorf("%d warning(s) — thresholds not met", len(warnings))
	}
	return nil
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

			// Embedding event breakdown
			fmt.Println()
			fmt.Println("═══ Embedding Events by Type ═══")
			embRows, embErr := pool.Query(ctx,
				`SELECT COALESCE(call_site, 'unknown'), COUNT(*),
				        SUM(CASE WHEN cached THEN 1 ELSE 0 END)
				 FROM embedding_events
				 GROUP BY call_site ORDER BY count DESC`)
			if embErr != nil {
				fmt.Println("  (table not yet created)")
			} else {
				defer embRows.Close()
				fmt.Printf("%-20s %8s %10s\n", "Call Site", "Total", "Cached")
				fmt.Println(strings.Repeat("─", 40))
				for embRows.Next() {
					var site string
					var cnt, cached int64
					if scanErr := embRows.Scan(&site, &cnt, &cached); scanErr == nil {
						fmt.Printf("%-20s %8d %10d\n", site, cnt, cached)
					}
				}
			}

			// Retrieval event breakdown
			fmt.Println()
			fmt.Println("═══ Retrieval Events by Call Site ═══")
			retRows, retErr := pool.Query(ctx,
				`SELECT call_site, COUNT(*), AVG(total_latency_ms)
				 FROM retrieval_events
				 GROUP BY call_site ORDER BY count DESC`)
			if retErr != nil {
				fmt.Println("  (table not yet created)")
			} else {
				defer retRows.Close()
				fmt.Printf("%-20s %8s %12s\n", "Call Site", "Total", "AvgLatency")
				fmt.Println(strings.Repeat("─", 42))
				for retRows.Next() {
					var site string
					var cnt int64
					var avgLat *float64
					if scanErr := retRows.Scan(&site, &cnt, &avgLat); scanErr == nil {
						fmt.Printf("%-20s %8d %10.0fms\n", site, cnt, derefF(avgLat))
					}
				}
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
	return sanitize.CutRuneSafeSuffix(s, maxLen-3, "...")
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

// newDataAuditCmd creates the `data audit` subcommand that compares disk state
// against CMS state for all tracked claude-md files.
func newDataAuditCmd() *cobra.Command {
	var spaceID string
	var serverURL string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit ingestion state of tracked claude-md files",
		Long: `Compare disk state against CMS state for all tracked claude-md files.

Reports each file as: current, stale (>24h since last ingest), deleted, or shrank.
Also shows pending ingest buffer entries and service health.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if spaceID == "" {
				spaceID = resolveSpaceID(cmd)
			}
			if spaceID == "" {
				spaceID = "codebase"
			}
			if serverURL == "" {
				serverURL = os.Getenv("MDEMG_URL")
			}
			if serverURL == "" {
				serverURL = "http://localhost:9999"
			}

			return runDataAudit(spaceID, serverURL)
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space ID to audit (default: codebase)")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MDEMG server URL (default: http://localhost:9999)")
	return cmd
}

func runDataAudit(spaceID, serverURL string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}

	// Check server health
	serverUp := false
	fmt.Println("═══ Service Health ═══")
	resp, err := client.Get(serverURL + "/healthz")
	if err != nil {
		fmt.Printf("  Server:     down (%s unreachable)\n", serverURL)
	} else {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			fmt.Printf("  Server:     up (%s)\n", serverURL)
			serverUp = true
		} else {
			fmt.Printf("  Server:     unhealthy (status %d)\n", resp.StatusCode)
		}
	}

	// Check Neo4j via server health detail if available
	if serverUp {
		healthResp, healthErr := client.Get(serverURL + "/healthz?detail=true")
		if healthErr == nil {
			body, _ := io.ReadAll(healthResp.Body)
			healthResp.Body.Close()
			var detail map[string]any
			if json.Unmarshal(body, &detail) == nil {
				if neo4j, ok := detail["neo4j"].(string); ok {
					fmt.Printf("  Neo4j:      %s\n", neo4j)
				}
				if sidecar, ok := detail["sidecar"].(string); ok {
					fmt.Printf("  Sidecar:    %s\n", sidecar)
				}
			}
		}
	}
	fmt.Println()

	// Discover tracked files
	projHash := projectPathHash(cwd)
	files := discoverClaudeMDFiles(cwd, projHash)

	fmt.Println("═══ Tracked Files ═══")
	fmt.Printf("%-50s %8s %10s %s\n", "File", "Lines", "Size", "Status")
	fmt.Println(strings.Repeat("─", 90))

	staleThreshold := 24 * time.Hour

	for _, f := range files {
		absPath := f.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(cwd, f.Path)
		}

		info, statErr := os.Stat(absPath)
		if statErr != nil {
			fmt.Printf("%-50s %8s %10s %s\n", truncate(f.Path, 50), "-", "-", "DELETED")
			continue
		}

		lineCount := countFileLines(absPath)
		sizeStr := formatBytes(info.Size())

		if !serverUp {
			fmt.Printf("%-50s %8d %10s %s\n", truncate(f.Path, 50), lineCount, sizeStr, "(server down)")
			continue
		}

		// Query CMS for stored metadata
		status := auditFileStatus(client, serverURL, spaceID, f.Path, lineCount, staleThreshold)
		fmt.Printf("%-50s %8d %10s %s\n", truncate(f.Path, 50), lineCount, sizeStr, status)
	}
	fmt.Println()

	// Check ingest buffer
	bufferPath := resolveIngestBufferPath()
	fmt.Println("═══ Ingest Buffer ═══")
	bufferInfo, bufErr := os.Stat(bufferPath)
	if bufErr != nil || bufferInfo.Size() == 0 {
		fmt.Printf("  Buffer:     empty (%s)\n", bufferPath)
	} else {
		entries := countBufferEntries(bufferPath)
		fmt.Printf("  Buffer:     %d pending entries (%s, %s)\n",
			entries, bufferPath, formatBytes(bufferInfo.Size()))
	}

	return nil
}

// auditFileStatus queries CMS /v1/memory/node/meta for the file and returns a status string.
func auditFileStatus(client *http.Client, serverURL, spaceID, filePath string, diskLines int, staleThreshold time.Duration) string {
	baseName := filepath.Base(filePath)
	url := fmt.Sprintf("%s/v1/memory/node/meta?space_id=%s&path=%s", serverURL, spaceID, baseName)

	resp, err := client.Get(url)
	if err != nil {
		return "error (unreachable)"
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "NOT INGESTED"
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("error (status %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "error (read)"
	}

	var meta struct {
		LineCount      int    `json:"line_count"`
		LastIngestedAt string `json:"last_ingested_at"`
		ContentHash    string `json:"content_hash"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return "error (parse)"
	}

	// Check for shrinkage
	if meta.LineCount > 0 && diskLines < meta.LineCount-10 {
		return fmt.Sprintf("SHRANK (%d → %d lines)", meta.LineCount, diskLines)
	}

	// Check staleness
	if meta.LastIngestedAt != "" {
		if t, err := time.Parse(time.RFC3339, meta.LastIngestedAt); err == nil {
			if time.Since(t) > staleThreshold {
				return fmt.Sprintf("STALE (ingested %s ago)", formatAgeDuration(time.Since(t)))
			}
		}
	}

	return "current"
}

func countFileLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count
}

func countBufferEntries(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	return count
}

func formatAgeDuration(d time.Duration) string {
	hours := int(d.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd", days)
}
