package tsdb

import (
	"bufio"
	"context"
	"strings"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mdemg/internal/llmclient"
)

// ExportConfig holds parameters for a training data export.
type ExportConfig struct {
	SpaceID    string
	From       time.Time
	To         time.Time
	Tables     []string // e.g. ["llm_interactions", "retrieval_events", "embedding_events", "guidance_training_rows"]
	InstanceID string
	OutputPath string
}

// ExportManifest is the manifest.json written into the export archive.
type ExportManifest struct {
	UTDSVersion   string                    `json:"utds_version"`
	ExportID      string                    `json:"export_id"`
	InstanceID    string                    `json:"instance_id"`
	SpaceID       string                    `json:"space_id"`
	MDEMGVersion  string                    `json:"mdemg_version"`
	MDEMGCommit   string                    `json:"mdemg_commit,omitempty"`
	SchemaVersion int                       `json:"schema_version"`
	ExportedAt    string                    `json:"exported_at"`
	DataRange     ExportDataRange           `json:"data_range"`
	Tables        map[string]*ExportTable   `json:"tables"`
	Quality       ExportQualitySummary      `json:"quality_summary"`
	Environment   ExportEnvironment         `json:"environment"`
}

// ExportDataRange is the time range of exported data.
type ExportDataRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ExportTable holds metadata about an exported table.
type ExportTable struct {
	RowCount           int64    `json:"row_count"`
	SHA256             string   `json:"sha256"`
	Columns            []string `json:"columns"`
	TaskNames          []string `json:"task_names,omitempty"`
	SystemPromptHashes []string `json:"system_prompt_hashes,omitempty"`
}

// ExportQualitySummary holds quality metrics for the export.
type ExportQualitySummary struct {
	LLMErrorRate            float64 `json:"llm_error_rate"`
	LLMEmptySystemPrompt    int     `json:"llm_empty_system_prompt"`
	LLMEmptyResponse        int     `json:"llm_empty_response"`
	EmbeddingEmptyCallSites int     `json:"embedding_empty_call_sites,omitempty"`
	RetrievalRecallRate     float64 `json:"retrieval_recall_rate,omitempty"`
	PrivacyScrubViolations  int     `json:"privacy_scrub_violations"`
}

// ExportEnvironment captures the runtime environment.
type ExportEnvironment struct {
	OS                  string `json:"os"`
	Arch                string `json:"arch"`
	GoVersion           string `json:"go_version"`
	LLMProvider         string `json:"llm_provider,omitempty"`
	LLMModel            string `json:"llm_model,omitempty"`
	EmbeddingModel      string `json:"embedding_model,omitempty"`
	EmbeddingDimensions int    `json:"embedding_dimensions,omitempty"`
}

// ExportResult is returned from RunExport with summary info.
type ExportResult struct {
	Manifest   *ExportManifest
	OutputPath string
	TotalRows  int64
}

// tableSpec defines the columns and text fields for a table.
type tableSpec struct {
	columns    []string
	textFields map[int][]string // column index → pattern names to SKIP (nil = apply all)
}

var llmInteractionsSpec = tableSpec{
	columns: []string{
		"time", "trace_id", "task_name", "space_id", "session_id",
		"system_prompt", "user_prompt", "response", "think_content", "think_mode",
		"latency_ms", "tokens_in", "tokens_out", "model_name", "provider",
		"error", "quality", "quality_source", "used_for_train", "dataset_ver",
		"guidance_id", "source_path",
		"retrieval_node_ids", "retrieval_scores", "oracle_node_id",
		"system_prompt_hash",
		"instance_id",
	},
	// Text fields: skip abs_path on source_path(21) — field IS a file path
	textFields: map[int][]string{5: nil, 6: nil, 7: nil, 8: nil, 21: {"abs_path"}},
}

var retrievalEventsSpec = tableSpec{
	columns: []string{
		"time", "event_id", "space_id", "call_site",
		"query_text", "query_hash",
		"recall_node_ids", "recall_scores", "recall_k",
		"bm25_node_ids", "bm25_scores",
		"rerank_node_ids", "rerank_scores", "rerank_model",
		"result_node_ids", "result_scores", "result_count",
		"guidance_id", "downstream_quality",
		"recall_latency_ms", "rerank_latency_ms", "total_latency_ms",
		"instance_id",
	},
	// query_text(4): skip abs_path — queries reference file paths by design
	textFields: map[int][]string{4: {"abs_path"}},
}

var embeddingEventsSpec = tableSpec{
	columns: []string{
		"time", "event_id", "event_type", "space_id",
		"text_content", "text_hash", "text_length",
		"element_kind", "language", "file_path",
		"chunk_start", "chunk_end",
		"package_name", "signature", "tags",
		"call_site", "query_text",
		"model_name", "provider", "dimensions",
		"latency_ms", "cached", "node_id",
		"instance_id",
	},
	// Embedding fields contain code chunks that were already scrubbed at write-time.
	// Export-time scanning produces false positives on scrubbed output (e.g., neo4j://[REDACTED]@
	// re-triggers the neo4j_cred pattern) and on code literals (regex definitions, docker configs).
	//
	// text_content(4): scrubbed at write-time. Skip patterns that trigger on code/scrubbed text.
	// file_path(9): field IS a path — skip abs_path.
	// signature(13): function signatures may reference paths — skip abs_path.
	// query_text(16): internal embedding search key, not user input. Skip all patterns.
	textFields: map[int][]string{
		4:  {"abs_path", "env_secret", "neo4j_cred"},
		9:  {"abs_path"},
		13: {"abs_path"},
		16: {"abs_path", "api_key", "env_secret", "email", "neo4j_cred"},
	},
}

// guidanceTrainingRowsSpec projects the corpus rows that the receiver
// (mdemg beta-import, sprint B5) imports into the local
// guidance_training_rows table for retrain use. Added at manifest
// schema_version 9. Excludes `row_id` from the projection because the
// receiver re-mints it via cuid2.Generate() to avoid the (time, row_id)
// PRIMARY KEY collisions across independently-produced bundles.
//
// Text fields for privacy re-scrub:
//   guidance_content (col 6): operator-authored constraint/correction text —
//     may reference file paths + code snippets that survived write-time scrub.
//   action_summary   (col 10): agent action text — same shape as
//     llm_interactions.user_prompt; needs full scrub.
var guidanceTrainingRowsSpec = tableSpec{
	columns: []string{
		"time", "space_id", "session_id", "instance_id",
		"guidance_id", "guidance_type", "guidance_content",
		"source_node_id", "source_role_type", "source_layer",
		"action_summary", "outcome_type", "similarity",
		"classifier_source", "constraint_code",
	},
	textFields: map[int][]string{6: {"abs_path"}, 10: nil},
}

var tableSpecs = map[string]tableSpec{
	"llm_interactions":       llmInteractionsSpec,
	"retrieval_events":       retrievalEventsSpec,
	"embedding_events":       embeddingEventsSpec,
	"guidance_training_rows": guidanceTrainingRowsSpec,
}

// RunExport executes the training data export pipeline.
// It streams rows from TSDB, applies privacy scanning, and packages
// the result as a .tar.gz archive with manifest.json + JSONL data files.
func RunExport(ctx context.Context, pool *pgxpool.Pool, cfg ExportConfig, version string, commit string) (*ExportResult, error) {
	if cfg.SpaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}
	if len(cfg.Tables) == 0 {
		// B5a: guidance_training_rows added as the corpus-carrying 4th
		// default. beta-import (B5b) consumes it into the local retrain
		// corpus with per-tester space attribution.
		cfg.Tables = []string{"llm_interactions", "retrieval_events", "embedding_events", "guidance_training_rows"}
	}

	// Validate requested tables
	for _, t := range cfg.Tables {
		if _, ok := tableSpecs[t]; !ok {
			return nil, fmt.Errorf("unknown table: %s", t)
		}
	}

	// Create temp directory for staging
	tmpDir, err := os.MkdirTemp("", "mdemg-export-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	now := time.Now().UTC()
	exportID := fmt.Sprintf("exp-%s-%s", cfg.InstanceID, now.Format("20060102-150405"))

	manifest := &ExportManifest{
		UTDSVersion:   "1.0.0",
		ExportID:      exportID,
		InstanceID:    cfg.InstanceID,
		SpaceID:       cfg.SpaceID,
		MDEMGVersion:  version,
		MDEMGCommit:   commit,
		// Schema 9 (B5a, 2026-08-10): added guidance_training_rows to the
		// export projection. beta-import receiver requires >= 9 to run
		// corpus-import mode; older bundles fall back to telemetry-only.
		SchemaVersion: 9,
		ExportedAt:    now.Format(time.RFC3339),
		DataRange: ExportDataRange{
			From: cfg.From.Format(time.RFC3339),
			To:   cfg.To.Format(time.RFC3339),
		},
		Tables: make(map[string]*ExportTable),
		Environment: ExportEnvironment{
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			GoVersion: runtime.Version()[2:], // strip "go" prefix
		},
	}

	var totalRows int64
	totalViolations := 0

	// Export each table
	for _, tableName := range cfg.Tables {
		spec, ok := tableSpecs[tableName]
		if !ok {
			continue
		}

		slog.Info("exporting table", "table", tableName, "space_id", cfg.SpaceID)

		// Log per-field privacy scan configuration (avoid silent skips)
		for colIdx, skipPats := range spec.textFields {
			colName := "?"
			if colIdx < len(spec.columns) {
				colName = spec.columns[colIdx]
			}
			if len(skipPats) > 0 {
				slog.Info("privacy scan: patterns skipped",
					"table", tableName,
					"column", colName,
					"skipped_patterns", skipPats,
				)
			}
		}

		result, violations, err := exportTable(ctx, pool, cfg, tableName, spec, tmpDir)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", tableName, err)
		}

		totalViolations += violations

		if result.RowCount > 0 {
			manifest.Tables[tableName] = result
			totalRows += result.RowCount
		}
	}

	// Hard gate: privacy violations
	if totalViolations > 0 {
		manifest.Quality.PrivacyScrubViolations = totalViolations
		return nil, fmt.Errorf("EXPORT BLOCKED: %d privacy scrub violations detected — training data contains PII", totalViolations)
	}

	// Require at least llm_interactions
	if _, ok := manifest.Tables["llm_interactions"]; !ok {
		return nil, fmt.Errorf("no llm_interactions data found for space %q in the specified time range", cfg.SpaceID)
	}

	// Compute quality summary from LLM data
	if llm, ok := manifest.Tables["llm_interactions"]; ok && llm.RowCount > 0 {
		errCount, emptyPrompt, emptyResp, err := computeLLMQuality(ctx, pool, cfg)
		if err != nil {
			slog.Warn("quality computation failed", "error", err)
		} else {
			manifest.Quality.LLMErrorRate = float64(errCount) / float64(llm.RowCount)
			manifest.Quality.LLMEmptySystemPrompt = emptyPrompt
			manifest.Quality.LLMEmptyResponse = emptyResp
		}
	}

	// Compute embedding quality
	if _, ok := manifest.Tables["embedding_events"]; ok {
		emptyCalls, err := computeEmbeddingQuality(ctx, pool, cfg)
		if err != nil {
			slog.Warn("embedding quality computation failed", "error", err)
		} else {
			manifest.Quality.EmbeddingEmptyCallSites = emptyCalls
		}
	}

	// Compute retrieval quality
	if _, ok := manifest.Tables["retrieval_events"]; ok {
		recallRate, err := computeRetrievalQuality(ctx, pool, cfg)
		if err != nil {
			slog.Warn("retrieval quality computation failed", "error", err)
		} else {
			manifest.Quality.RetrievalRecallRate = recallRate
		}
	}

	// Remove JSONL files for tables with 0 rows (not in manifest)
	for _, tableName := range cfg.Tables {
		if _, inManifest := manifest.Tables[tableName]; !inManifest {
			_ = os.Remove(filepath.Join(tmpDir, tableName+".jsonl"))
		}
	}

	// Write manifest
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	mf, err := os.Create(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("create manifest: %w", err)
	}
	enc := json.NewEncoder(mf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		mf.Close()
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	mf.Close()

	// Package as tar.gz
	if err := tarGzDirectory(tmpDir, cfg.OutputPath); err != nil {
		return nil, fmt.Errorf("create archive: %w", err)
	}

	slog.Info("export complete",
		"export_id", exportID,
		"total_rows", totalRows,
		"tables", len(manifest.Tables),
		"output", cfg.OutputPath,
	)

	return &ExportResult{
		Manifest:   manifest,
		OutputPath: cfg.OutputPath,
		TotalRows:  totalRows,
	}, nil
}

// exportTable streams a single table to a JSONL file and returns metadata.
func exportTable(ctx context.Context, pool *pgxpool.Pool, cfg ExportConfig, tableName string, spec tableSpec, tmpDir string) (*ExportTable, int, error) {
	colList := strings.Join(spec.columns, ", ")

	var query string
	var args []any
	if cfg.InstanceID != "" {
		query = fmt.Sprintf(
			`SELECT %s FROM %s WHERE (space_id = $1 OR space_id IS NULL OR space_id = '') AND (instance_id = $4 OR instance_id = '') AND time >= $2 AND time <= $3 ORDER BY time`,
			colList, tableName,
		)
		args = []any{cfg.SpaceID, cfg.From, cfg.To, cfg.InstanceID}
	} else {
		query = fmt.Sprintf(
			`SELECT %s FROM %s WHERE (space_id = $1 OR space_id IS NULL OR space_id = '') AND time >= $2 AND time <= $3 ORDER BY time`,
			colList, tableName,
		)
		args = []any{cfg.SpaceID, cfg.From, cfg.To}
	}

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query %s: %w", tableName, err)
	}
	defer rows.Close()

	// Open output file
	jsonlPath := filepath.Join(tmpDir, tableName+".jsonl")
	f, err := os.Create(jsonlPath)
	if err != nil {
		return nil, 0, fmt.Errorf("create jsonl: %w", err)
	}
	defer f.Close()

	bw := bufio.NewWriterSize(f, 64*1024)
	hasher := sha256.New()
	mw := io.MultiWriter(bw, hasher)

	var rowCount int64
	violations := 0
	taskSet := map[string]bool{}
	hashSet := map[string]bool{}

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, violations, fmt.Errorf("scan row: %w", err)
		}

		record := make(map[string]any, len(spec.columns))
		for i, col := range spec.columns {
			val := values[i]

			// Fill empty space_id with config value
			if col == "space_id" {
				if val == nil || val == "" {
					val = cfg.SpaceID
				}
			}

			// Privacy scan on text fields
			if skipPatterns, ok := spec.textFields[i]; ok {
				if s, ok := val.(string); ok && s != "" {
					scrubbed := llmclient.ScrubStringExcluding(s, skipPatterns)
					if scrubbed != s {
						violations++
						slog.Warn("privacy violation detected",
							"table", tableName,
							"column", col,
							"row", rowCount+1,
						)
					}
				}
			}

			record[col] = val
		}

		// Track task names and prompt hashes for llm_interactions
		if tableName == "llm_interactions" {
			if tn, ok := record["task_name"].(string); ok && tn != "" {
				taskSet[tn] = true
			}
			if ph, ok := record["system_prompt_hash"].(string); ok && ph != "" {
				hashSet[ph] = true
			}
		}

		line, err := json.Marshal(record)
		if err != nil {
			return nil, violations, fmt.Errorf("marshal row: %w", err)
		}
		line = append(line, '\n')
		if _, err := mw.Write(line); err != nil {
			return nil, violations, fmt.Errorf("write row: %w", err)
		}
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return nil, violations, fmt.Errorf("rows iteration: %w", err)
	}
	if err := bw.Flush(); err != nil {
		return nil, violations, fmt.Errorf("flush buffer: %w", err)
	}

	result := &ExportTable{
		RowCount: rowCount,
		SHA256:   hex.EncodeToString(hasher.Sum(nil)),
		Columns:  spec.columns,
	}

	if tableName == "llm_interactions" {
		for t := range taskSet {
			result.TaskNames = append(result.TaskNames, t)
		}
		for h := range hashSet {
			result.SystemPromptHashes = append(result.SystemPromptHashes, h)
		}
	}

	return result, violations, nil
}

// computeLLMQuality queries quality metrics for llm_interactions.
func computeLLMQuality(ctx context.Context, pool *pgxpool.Pool, cfg ExportConfig) (errCount, emptyPrompt, emptyResp int, err error) {
	row := pool.QueryRow(ctx,
		`SELECT
			-- LLM-HEALTH-INVESTIGATION-001 E3: exclude caller_canceled from the
			-- error count (see llmclient/client.go recorder tagging + dataset_builder
			-- LLMPerformance). Export manifests carry the same honest signal.
			COALESCE(SUM(CASE WHEN error IS NOT NULL AND error != '' AND error NOT LIKE 'caller_canceled:%' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN system_prompt IS NULL OR system_prompt = '' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN response IS NULL OR response = '' THEN 1 ELSE 0 END), 0)
		FROM llm_interactions
		WHERE (space_id = $1 OR space_id IS NULL OR space_id = '')
			AND time >= $2 AND time <= $3`,
		cfg.SpaceID, cfg.From, cfg.To,
	)
	err = row.Scan(&errCount, &emptyPrompt, &emptyResp)
	return
}

// computeEmbeddingQuality queries quality metrics for embedding_events.
func computeEmbeddingQuality(ctx context.Context, pool *pgxpool.Pool, cfg ExportConfig) (int, error) {
	var count int
	err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(CASE WHEN call_site IS NULL OR call_site = '' THEN 1 ELSE 0 END), 0)
		FROM embedding_events
		WHERE (space_id = $1 OR space_id IS NULL OR space_id = '')
			AND time >= $2 AND time <= $3`,
		cfg.SpaceID, cfg.From, cfg.To,
	).Scan(&count)
	return count, err
}

// computeRetrievalQuality queries quality metrics for retrieval_events.
func computeRetrievalQuality(ctx context.Context, pool *pgxpool.Pool, cfg ExportConfig) (float64, error) {
	var total, withRecall int
	err := pool.QueryRow(ctx,
		`SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN recall_node_ids IS NOT NULL AND array_length(recall_node_ids, 1) > 0 THEN 1 ELSE 0 END), 0)
		FROM retrieval_events
		WHERE (space_id = $1 OR space_id IS NULL OR space_id = '')
			AND time >= $2 AND time <= $3`,
		cfg.SpaceID, cfg.From, cfg.To,
	).Scan(&total, &withRecall)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	return float64(withRecall) / float64(total), nil
}

// DryRunExport returns row counts without actually exporting.
func DryRunExport(ctx context.Context, pool *pgxpool.Pool, cfg ExportConfig) (map[string]int64, error) {
	if cfg.SpaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}
	if len(cfg.Tables) == 0 {
		// B5a: guidance_training_rows added as the corpus-carrying 4th
		// default. beta-import (B5b) consumes it into the local retrain
		// corpus with per-tester space attribution.
		cfg.Tables = []string{"llm_interactions", "retrieval_events", "embedding_events", "guidance_training_rows"}
	}

	counts := make(map[string]int64)
	for _, tableName := range cfg.Tables {
		if _, ok := tableSpecs[tableName]; !ok {
			continue
		}
		var count int64
		err := pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE (space_id = $1 OR space_id IS NULL OR space_id = '') AND time >= $2 AND time <= $3`,
				tableName),
			cfg.SpaceID, cfg.From, cfg.To,
		).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("count %s: %w", tableName, err)
		}
		counts[tableName] = count
	}

	return counts, nil
}

// tarGzExport creates a tar.gz archive from the export staging directory.
// This reuses the existing tarGzDirectory function from backup.go.
// If that function isn't exported, we provide our own here — but since
// tarGzDirectory is already in this package (lowercase = unexported), we
// can call it directly.
