package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/jackc/pgx/v5/pgxpool"
	"mdemg/internal/tsdb"
)

// checkResult represents the outcome of a single pre-campaign check.
type checkResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "PASS", "FAIL", "WARN"
	Detail  string `json:"detail,omitempty"`
	NextStep string `json:"next_step,omitempty"`
}

func (cr checkResult) passed() bool {
	return cr.Status == "PASS"
}

// newDataCheckCmd creates the `data check` subcommand.
func newDataCheckCmd() *cobra.Command {
	var (
		jsonOutput  bool
		preCampaign bool
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run pre-campaign validation checks",
		Long: `Validate that this MDEMG instance is correctly configured for the 30-day
multi-instance data collection campaign. Runs 8 automated checks and reports
pass/fail with next-step guidance on failures.

Exit code 0 = all checks pass, 1 = one or more checks failed.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !preCampaign {
				return fmt.Errorf("specify --pre-campaign to run validation checks")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cfg := tsdbConfigFromEnv()
			client, err := tsdb.NewClient(ctx, cfg)
			if err != nil {
				// If TSDB is unreachable, report it as a failed check
				results := []checkResult{{
					Name:     "TSDB Connection",
					Status:   "FAIL",
					Detail:   fmt.Sprintf("cannot connect to TSDB: %v", err),
					NextStep: "Verify TSDB_HOST, TSDB_PORT, TSDB_USER, TSDB_PASSWORD env vars. Is TimescaleDB running?",
				}}
				return reportResults(results, jsonOutput)
			}
			defer client.Close()

			pool := client.Pool()

			var results []checkResult

			// Check 1: Schema version
			results = append(results, checkSchemaVersion(ctx, client))

			// Check 2: Instance ID
			results = append(results, checkInstanceID(ctx, pool))

			// Check 3: TSDB writable
			results = append(results, checkTSDBWritable(ctx, pool))

			// Check 4: Event logging enabled
			results = append(results, checkEventLogging())

			// Check 5: Task accumulation
			results = append(results, checkTaskAccumulation(ctx, pool))

			// Check 6: Session ID populated
			results = append(results, checkSessionIDPopulated(ctx, pool))

			// Check 7: Export dry-run
			results = append(results, checkExportDryRun(ctx, pool))

			// Check 8: Campaign config flags
			results = append(results, checkCampaignFlags())

			return reportResults(results, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output results as JSON")
	cmd.Flags().BoolVar(&preCampaign, "pre-campaign", false, "Run pre-campaign validation suite")

	return cmd
}

func checkSchemaVersion(ctx context.Context, client *tsdb.Client) checkResult {
	version, err := client.GetSchemaVersion(ctx)
	if err != nil {
		return checkResult{
			Name:     "Schema Version",
			Status:   "FAIL",
			Detail:   fmt.Sprintf("cannot read schema version: %v", err),
			NextStep: "Run migrations: mdemg start --auto-migrate",
		}
	}
	if version < 9 {
		return checkResult{
			Name:     "Schema Version",
			Status:   "FAIL",
			Detail:   fmt.Sprintf("schema version %d, need >= 9 (migration 009: space_id backfill)", version),
			NextStep: "Run migrations: mdemg start --auto-migrate",
		}
	}
	return checkResult{
		Name:   "Schema Version",
		Status: "PASS",
		Detail: fmt.Sprintf("schema version %d", version),
	}
}

func checkInstanceID(ctx context.Context, pool *pgxpool.Pool) checkResult {
	envID := os.Getenv("MDEMG_INSTANCE_ID")
	if envID == "" {
		return checkResult{
			Name:     "Instance ID",
			Status:   "FAIL",
			Detail:   "MDEMG_INSTANCE_ID env var is empty",
			NextStep: "Set MDEMG_INSTANCE_ID in your .env or environment",
		}
	}

	// Check for recent records with non-empty instance_id
	var count int64
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM llm_interactions
		 WHERE instance_id != '' AND time > now() - interval '24 hours'`).Scan(&count)
	if err != nil {
		return checkResult{
			Name:     "Instance ID",
			Status:   "WARN",
			Detail:   fmt.Sprintf("env var set to %q but cannot verify TSDB records: %v", envID, err),
			NextStep: "Verify TSDB is receiving data with instance_id populated",
		}
	}

	if count == 0 {
		return checkResult{
			Name:     "Instance ID",
			Status:   "WARN",
			Detail:   fmt.Sprintf("env var set to %q but no records with instance_id in last 24h", envID),
			NextStep: "Generate some LLM interactions and verify instance_id is populated",
		}
	}

	return checkResult{
		Name:   "Instance ID",
		Status: "PASS",
		Detail: fmt.Sprintf("env=%q, %d records with instance_id in last 24h", envID, count),
	}
}

func checkTSDBWritable(ctx context.Context, pool *pgxpool.Pool) checkResult {
	// Insert a test metric and delete it
	_, err := pool.Exec(ctx,
		`INSERT INTO metric_samples (time, name, value, labels)
		 VALUES (now(), '__pre_campaign_check__', 0, '{}')`)
	if err != nil {
		return checkResult{
			Name:     "TSDB Writable",
			Status:   "FAIL",
			Detail:   fmt.Sprintf("cannot write to metric_samples: %v", err),
			NextStep: "Check TSDB permissions and table existence",
		}
	}

	_, _ = pool.Exec(ctx,
		`DELETE FROM metric_samples WHERE name = '__pre_campaign_check__'`)

	return checkResult{
		Name:   "TSDB Writable",
		Status: "PASS",
		Detail: "insert + delete succeeded",
	}
}

func checkEventLogging() checkResult {
	tsdbEnabled := os.Getenv("TSDB_ENABLED")
	llmLogging := os.Getenv("LLM_INTERACTION_LOGGING")

	var issues []string
	if tsdbEnabled != "true" && tsdbEnabled != "1" {
		issues = append(issues, "TSDB_ENABLED is not true")
	}
	if llmLogging == "false" || llmLogging == "0" {
		issues = append(issues, "LLM_INTERACTION_LOGGING is explicitly disabled")
	}

	if len(issues) > 0 {
		return checkResult{
			Name:     "Event Logging",
			Status:   "FAIL",
			Detail:   strings.Join(issues, "; "),
			NextStep: "Set TSDB_ENABLED=true and ensure LLM_INTERACTION_LOGGING is not disabled",
		}
	}

	return checkResult{
		Name:   "Event Logging",
		Status: "PASS",
		Detail: "TSDB_ENABLED=true, LLM_INTERACTION_LOGGING enabled",
	}
}

func checkTaskAccumulation(ctx context.Context, pool *pgxpool.Pool) checkResult {
	builder := tsdb.NewDatasetBuilder(pool)
	readiness, err := builder.TrainingDataReadiness(ctx)
	if err != nil {
		return checkResult{
			Name:     "Task Accumulation",
			Status:   "FAIL",
			Detail:   fmt.Sprintf("cannot query readiness: %v", err),
			NextStep: "Verify TSDB connection and llm_interactions table",
		}
	}

	if readiness == nil || len(readiness.Tasks) == 0 {
		return checkResult{
			Name:     "Task Accumulation",
			Status:   "WARN",
			Detail:   "no tasks have any records yet",
			NextStep: "Generate LLM interactions by using MDEMG features (retrieval, consulting, jiminy)",
		}
	}

	var activeTasks, readyTasks int
	var totalRows int
	for _, t := range readiness.Tasks {
		if t.TotalRows > 0 {
			activeTasks++
			totalRows += t.TotalRows
		}
		if t.Ready {
			readyTasks++
		}
	}

	status := "PASS"
	if activeTasks < 2 {
		status = "WARN"
	}

	return checkResult{
		Name:   "Task Accumulation",
		Status: status,
		Detail: fmt.Sprintf("%d/%d tasks active, %d ready (500+ rows), %d total rows",
			activeTasks, len(readiness.Tasks), readyTasks, totalRows),
		NextStep: "Enable more task paths: QUERY_CLASSIFY_ENABLED, INTENT_ENABLED, JIMINY_ENABLED",
	}
}

func checkSessionIDPopulated(ctx context.Context, pool *pgxpool.Pool) checkResult {
	var withSession, total int64
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(CASE WHEN session_id != '' THEN 1 END)
		 FROM llm_interactions
		 WHERE time > now() - interval '24 hours'`).Scan(&total, &withSession)
	if err != nil {
		return checkResult{
			Name:     "Session ID",
			Status:   "WARN",
			Detail:   fmt.Sprintf("cannot query session_id: %v", err),
			NextStep: "Verify llm_interactions table has session_id column",
		}
	}

	if total == 0 {
		return checkResult{
			Name:     "Session ID",
			Status:   "WARN",
			Detail:   "no records in last 24h to check",
			NextStep: "Generate some LLM interactions first",
		}
	}

	pct := float64(withSession) / float64(total) * 100
	status := "PASS"
	if withSession == 0 {
		status = "FAIL"
	} else if pct < 50 {
		status = "WARN"
	}

	return checkResult{
		Name:     "Session ID",
		Status:   status,
		Detail:   fmt.Sprintf("%d/%d records (%.0f%%) have session_id in last 24h", withSession, total, pct),
		NextStep: "Ensure MDEMG server has session_id propagation (v0.5.0+)",
	}
}

func checkExportDryRun(ctx context.Context, pool *pgxpool.Pool) checkResult {
	spaceID := os.Getenv("MDEMG_SPACE_ID")
	if spaceID == "" {
		spaceID = "mdemg-dev"
	}

	// Check that export tables have data
	var llmCount, embCount, retCount int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM llm_interactions WHERE space_id = $1`, spaceID).Scan(&llmCount)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM embedding_events WHERE space_id = $1`, spaceID).Scan(&embCount)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM retrieval_events WHERE space_id = $1`, spaceID).Scan(&retCount)

	if llmCount == 0 && embCount == 0 && retCount == 0 {
		return checkResult{
			Name:     "Export Dry Run",
			Status:   "WARN",
			Detail:   fmt.Sprintf("no data for space_id=%q", spaceID),
			NextStep: "Generate some interactions first, then re-check",
		}
	}

	return checkResult{
		Name:   "Export Dry Run",
		Status: "PASS",
		Detail: fmt.Sprintf("space=%q: llm=%d, embedding=%d, retrieval=%d", spaceID, llmCount, embCount, retCount),
	}
}

func checkCampaignFlags() checkResult {
	flags := map[string]string{
		"QUERY_CLASSIFY_ENABLED": os.Getenv("QUERY_CLASSIFY_ENABLED"),
		"INTENT_ENABLED":        os.Getenv("INTENT_ENABLED"),
		"JIMINY_ENABLED":        os.Getenv("JIMINY_ENABLED"),
	}

	var enabled, disabled []string
	for name, val := range flags {
		if val == "true" || val == "1" {
			enabled = append(enabled, name)
		} else {
			disabled = append(disabled, name)
		}
	}

	if len(disabled) > 0 {
		return checkResult{
			Name:     "Campaign Flags",
			Status:   "WARN",
			Detail:   fmt.Sprintf("disabled: %s", strings.Join(disabled, ", ")),
			NextStep: "Enable recommended flags for maximum task coverage",
		}
	}

	return checkResult{
		Name:   "Campaign Flags",
		Status: "PASS",
		Detail: fmt.Sprintf("all enabled: %s", strings.Join(enabled, ", ")),
	}
}

func reportResults(results []checkResult, jsonOutput bool) error {
	passed := 0
	failed := 0
	warned := 0
	for _, r := range results {
		switch r.Status {
		case "PASS":
			passed++
		case "FAIL":
			failed++
		case "WARN":
			warned++
		}
	}

	if jsonOutput {
		report := map[string]any{
			"checks": results,
			"summary": map[string]int{
				"passed": passed,
				"failed": failed,
				"warned": warned,
				"total":  len(results),
			},
		}
		out, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Println("═══ Pre-Campaign Validation ═══")
		fmt.Println()
		for _, r := range results {
			icon := "  "
			switch r.Status {
			case "PASS":
				icon = "OK"
			case "FAIL":
				icon = "XX"
			case "WARN":
				icon = "!!"
			}
			fmt.Printf("[%s] %-20s %s\n", icon, r.Name, r.Detail)
			if r.Status != "PASS" && r.NextStep != "" {
				fmt.Printf("     -> %s\n", r.NextStep)
			}
		}
		fmt.Println()
		fmt.Printf("Result: %d passed, %d failed, %d warnings (of %d checks)\n",
			passed, failed, warned, len(results))
	}

	if failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	return nil
}
