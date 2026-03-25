// Package api provides HTTP handlers for the MDEMG API.
// This file implements the /v1/memory/ingest-codebase endpoint.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// handleIngestCodebaseRoute routes requests to the appropriate handler.
// DEPRECATED: Use /v1/memory/ingest/trigger instead for new integrations.
func (s *Server) handleIngestCodebaseRoute(w http.ResponseWriter, r *http.Request) {
	// Signal deprecation on all responses from this endpoint
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Link", `</v1/memory/ingest/trigger>; rel="successor-version"`)

	path := strings.TrimPrefix(r.URL.Path, "/v1/memory/ingest-codebase")
	path = strings.TrimPrefix(path, "/")

	switch {
	case path == "" && r.Method == http.MethodPost:
		s.handleIngestCodebase(w, r)
	case path == "" && r.Method == http.MethodGet:
		s.handleIngestCodebaseList(w, r)
	case path != "" && r.Method == http.MethodGet:
		s.handleIngestCodebaseStatus(w, r)
	case path != "" && r.Method == http.MethodDelete:
		s.handleIngestCodebaseCancel(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// IngestCodebaseRequest defines the request body for /v1/memory/ingest-codebase
type IngestCodebaseRequest struct {
	SpaceID string `json:"space_id"`
	Path    string `json:"path"`

	// Source configuration
	Source *IngestSourceConfig `json:"source,omitempty"`

	// Language filters
	Languages *IngestLanguageConfig `json:"languages,omitempty"`

	// Symbol extraction
	Symbols *IngestSymbolConfig `json:"symbols,omitempty"`

	// Exclusion rules
	Exclusions *IngestExclusionConfig `json:"exclusions,omitempty"`

	// Processing parameters
	Processing *IngestProcessingConfig `json:"processing,omitempty"`

	// LLM summary generation
	LLMSummary *IngestLLMSummaryConfig `json:"llm_summary,omitempty"`

	// General options
	Options *IngestOptionsConfig `json:"options,omitempty"`

	// Retry configuration
	Retry *IngestRetryConfig `json:"retry,omitempty"`
}

// IngestSourceConfig defines source type and git options
type IngestSourceConfig struct {
	Type   string `json:"type,omitempty"`  // "local" or "git"
	Branch string `json:"branch,omitempty"` // for git sources
	Since  string `json:"since,omitempty"`  // for incremental mode
}

// IngestLanguageConfig defines which languages to include
type IngestLanguageConfig struct {
	TypeScript   *bool `json:"typescript,omitempty"`
	Python       *bool `json:"python,omitempty"`
	Java         *bool `json:"java,omitempty"`
	Rust         *bool `json:"rust,omitempty"`
	Go           *bool `json:"go,omitempty"`
	Markdown     *bool `json:"markdown,omitempty"`
	IncludeTests *bool `json:"include_tests,omitempty"`
}

// IngestSymbolConfig defines symbol extraction options
type IngestSymbolConfig struct {
	Extract    *bool `json:"extract,omitempty"`
	MaxPerFile *int  `json:"max_per_file,omitempty"`
}

// IngestExclusionConfig defines what to exclude
type IngestExclusionConfig struct {
	Preset      string   `json:"preset,omitempty"`      // "default", "ml_cuda", "web_monorepo"
	Directories []string `json:"directories,omitempty"` // additional dirs to exclude
	MaxFileSize *int     `json:"max_file_size,omitempty"`
}

// IngestProcessingConfig defines processing parameters
type IngestProcessingConfig struct {
	BatchSize          *int `json:"batch_size,omitempty"`
	Workers            *int `json:"workers,omitempty"`
	MaxElementsPerFile *int `json:"max_elements_per_file,omitempty"`
	DelayMs            *int `json:"delay_ms,omitempty"`
}

// IngestLLMSummaryConfig defines LLM summary options
type IngestLLMSummaryConfig struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Provider  string `json:"provider,omitempty"` // "openai" or "ollama"
	Model     string `json:"model,omitempty"`
	BatchSize *int   `json:"batch_size,omitempty"`
}

// IngestOptionsConfig defines general options
type IngestOptionsConfig struct {
	Incremental    *bool `json:"incremental,omitempty"`
	ArchiveDeleted *bool `json:"archive_deleted,omitempty"`
	Consolidate    *bool `json:"consolidate,omitempty"`
	DryRun         *bool `json:"dry_run,omitempty"`
	Verbose        *bool `json:"verbose,omitempty"`
	Limit          *int  `json:"limit,omitempty"`
}

// IngestRetryConfig defines retry behavior
type IngestRetryConfig struct {
	MaxAttempts    *int `json:"max_attempts,omitempty"`
	InitialDelayMs *int `json:"initial_delay_ms,omitempty"`
	TimeoutSeconds *int `json:"timeout_seconds,omitempty"`
}

// IngestCodebaseResponse defines the response for /v1/memory/ingest-codebase
type IngestCodebaseResponse struct {
	JobID   string             `json:"job_id"`
	Status  string             `json:"status"` // "queued", "running", "completed", "failed"
	SpaceID string             `json:"space_id"`
	Path    string             `json:"path"`
	Stats   *IngestCodebaseStats `json:"stats,omitempty"`
	Error   string             `json:"error,omitempty"`
}

// IngestCodebaseStats provides ingestion statistics
type IngestCodebaseStats struct {
	FilesFound      int64   `json:"files_found"`
	FilesProcessed  int64   `json:"files_processed"`
	SymbolsExtracted int64  `json:"symbols_extracted"`
	Errors          int64   `json:"errors"`
	Rate            float64 `json:"rate"`
	Duration        string  `json:"duration,omitempty"`
}

// IngestJob tracks a running ingestion job
type IngestJob struct {
	ID        string
	SpaceID   string
	Path      string
	Status    string
	Stats     IngestCodebaseStats
	StartTime time.Time
	EndTime   time.Time
	Error     string
	Cancel    context.CancelFunc
	mu        sync.Mutex
}

// Active ingestion jobs
var (
	ingestJobs   = make(map[string]*IngestJob)
	ingestJobsMu sync.RWMutex
)

// handleIngestCodebase handles POST /v1/memory/ingest-codebase
func (s *Server) handleIngestCodebase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req IngestCodebaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("ingest codebase: invalid JSON", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Validate required fields
	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}

	// Validate path exists (for local sources)
	sourceType := "local"
	if req.Source != nil && req.Source.Type != "" {
		sourceType = req.Source.Type
	}

	if sourceType == "local" {
		if _, err := os.Stat(req.Path); os.IsNotExist(err) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path does not exist: " + req.Path})
			return
		}
	}

	// Create job
	jobID := uuid.New().String()[:8]
	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel stored in IngestJob.Cancel for job cancellation

	job := &IngestJob{
		ID:        jobID,
		SpaceID:   req.SpaceID,
		Path:      req.Path,
		Status:    "queued",
		StartTime: time.Now(),
		Cancel:    cancel,
	}

	ingestJobsMu.Lock()
	ingestJobs[jobID] = job
	ingestJobsMu.Unlock()

	// Start ingestion in background
	go s.runIngestionJob(ctx, job, &req) //nolint:gosec // G118: ctx from Background(), not request-scoped — job outlives handler

	// Return immediately with job ID
	writeJSON(w, http.StatusAccepted, IngestCodebaseResponse{
		JobID:   jobID,
		Status:  "queued",
		SpaceID: req.SpaceID,
		Path:    req.Path,
	})
}

// ingestSemaphore limits concurrent ingestion jobs to prevent resource exhaustion.
var ingestSemaphore = make(chan struct{}, 4)

// runIngestionJob executes the ingestion using the CLI tool.
// Wrapped in recover() so a panic in a background goroutine does not crash the server.
func (s *Server) runIngestionJob(ctx context.Context, job *IngestJob, req *IngestCodebaseRequest) {
	defer func() {
		if r := recover(); r != nil {
			job.mu.Lock()
			job.Status = "failed"
			job.Error = fmt.Sprintf("panic in ingestion job: %v", r)
			job.EndTime = time.Now()
			job.mu.Unlock()
			slog.Error("ingest-codebase: panic in job", "job_id", job.ID, "panic", r)
		}
	}()

	// Acquire semaphore slot (or respect context cancellation)
	select {
	case ingestSemaphore <- struct{}{}:
		defer func() { <-ingestSemaphore }()
	case <-ctx.Done():
		job.mu.Lock()
		job.Status = "failed"
		job.Error = "context cancelled while waiting for ingestion slot"
		job.EndTime = time.Now()
		job.mu.Unlock()
		return
	}

	job.mu.Lock()
	job.Status = "running"
	job.mu.Unlock()

	// Build CLI arguments
	args := s.buildIngestArgs(req)

	slog.Info("ingest-codebase: starting job", "job_id", job.ID, "space_id", job.SpaceID, "path", job.Path)

	// Run unified CLI ingest subcommand from current working directory
	cmd := exec.CommandContext(ctx, "./bin/mdemg", append([]string{"ingest"}, args...)...)

	output, err := cmd.CombinedOutput()

	job.mu.Lock()
	defer job.mu.Unlock()

	job.EndTime = time.Now()
	duration := job.EndTime.Sub(job.StartTime)
	job.Stats.Duration = duration.Round(time.Second).String()

	if err != nil {
		job.Status = "failed"
		job.Error = fmt.Sprintf("ingestion failed: %v\nOutput: %s", err, string(output))
		slog.Error("ingest-codebase: job failed", "job_id", job.ID, "error", err)
		return
	}

	// Parse output for stats
	job.Stats = parseIngestOutput(string(output))
	job.Stats.Duration = duration.Round(time.Second).String()
	job.Status = "completed"

	slog.Info("ingest-codebase: job completed", "job_id", job.ID, "files_processed", job.Stats.FilesProcessed, "symbols_extracted", job.Stats.SymbolsExtracted, "errors", job.Stats.Errors)

	// Update TapRoot freshness on completion
	freshnessCtx, freshnessCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer freshnessCancel()
	if s.retriever != nil {
		if err := s.retriever.UpdateTapRootFreshness(freshnessCtx, job.SpaceID, "codebase-ingest", IsPrunablePrefix(job.SpaceID)); err != nil {
			slog.Warn("ingest-codebase: failed to update TapRoot freshness", "space_id", job.SpaceID, "error", err)
		}
	}
	s.TriggerAPEEventWithContext("source_changed", map[string]string{
		"space_id":    job.SpaceID,
		"ingest_type": "codebase-ingest",
	})
	s.TriggerAPEEventWithContext("ingest_complete", map[string]string{
		"space_id":    job.SpaceID,
		"ingest_type": "codebase-ingest",
	})
}

// buildIngestArgs constructs CLI arguments from the request
func (s *Server) buildIngestArgs(req *IngestCodebaseRequest) []string {
	// Use configured listen address or default
	endpoint := s.cfg.ListenAddr
	if endpoint == "" {
		endpoint = "http://localhost:9999"
	} else if !strings.HasPrefix(endpoint, "http") {
		// Handle port-only format like ":9999" -> "http://localhost:9999"
		if strings.HasPrefix(endpoint, ":") {
			endpoint = "http://localhost" + endpoint
		} else {
			endpoint = "http://" + endpoint
		}
	}

	args := []string{
		"--path", req.Path,
		"--space-id", req.SpaceID,
		"--endpoint", endpoint,
	}

	// Source options
	if req.Source != nil {
		if req.Source.Since != "" {
			args = append(args, "--since", req.Source.Since)
		}
	}

	// Language options
	if req.Languages != nil {
		if req.Languages.TypeScript != nil {
			args = append(args, fmt.Sprintf("--include-ts=%t", *req.Languages.TypeScript))
		}
		if req.Languages.Python != nil {
			args = append(args, fmt.Sprintf("--include-py=%t", *req.Languages.Python))
		}
		if req.Languages.Java != nil {
			args = append(args, fmt.Sprintf("--include-java=%t", *req.Languages.Java))
		}
		if req.Languages.Rust != nil {
			args = append(args, fmt.Sprintf("--include-rust=%t", *req.Languages.Rust))
		}
		if req.Languages.Markdown != nil {
			args = append(args, fmt.Sprintf("--include-md=%t", *req.Languages.Markdown))
		}
		if req.Languages.IncludeTests != nil && *req.Languages.IncludeTests {
			args = append(args, "--include-tests")
		}
	}

	// Symbol options
	if req.Symbols != nil {
		if req.Symbols.Extract != nil {
			args = append(args, fmt.Sprintf("--extract-symbols=%t", *req.Symbols.Extract))
		}
		if req.Symbols.MaxPerFile != nil {
			args = append(args, fmt.Sprintf("--max-symbols-per-file=%d", *req.Symbols.MaxPerFile))
		}
	}

	// Exclusion options
	if req.Exclusions != nil {
		if req.Exclusions.Preset != "" {
			args = append(args, "--preset", req.Exclusions.Preset)
		}
		if len(req.Exclusions.Directories) > 0 {
			args = append(args, "--exclude", strings.Join(req.Exclusions.Directories, ","))
		}
		if req.Exclusions.MaxFileSize != nil {
			args = append(args, fmt.Sprintf("--max-file-size=%d", *req.Exclusions.MaxFileSize))
		}
	}

	// Processing options
	if req.Processing != nil {
		if req.Processing.BatchSize != nil {
			args = append(args, fmt.Sprintf("--batch=%d", *req.Processing.BatchSize))
		}
		if req.Processing.Workers != nil {
			args = append(args, fmt.Sprintf("--workers=%d", *req.Processing.Workers))
		}
		if req.Processing.MaxElementsPerFile != nil {
			args = append(args, fmt.Sprintf("--max-elements-per-file=%d", *req.Processing.MaxElementsPerFile))
		}
		if req.Processing.DelayMs != nil {
			args = append(args, fmt.Sprintf("--delay=%d", *req.Processing.DelayMs))
		}
	}

	// LLM Summary options
	if req.LLMSummary != nil && req.LLMSummary.Enabled != nil && *req.LLMSummary.Enabled {
		args = append(args, "--llm-summary")
		if req.LLMSummary.Provider != "" {
			args = append(args, "--llm-summary-provider", req.LLMSummary.Provider)
		}
		if req.LLMSummary.Model != "" {
			args = append(args, "--llm-summary-model", req.LLMSummary.Model)
		}
		if req.LLMSummary.BatchSize != nil {
			args = append(args, fmt.Sprintf("--llm-summary-batch=%d", *req.LLMSummary.BatchSize))
		}
	}

	// General options
	if req.Options != nil {
		if req.Options.Incremental != nil && *req.Options.Incremental {
			args = append(args, "--incremental")
		}
		if req.Options.ArchiveDeleted != nil {
			args = append(args, fmt.Sprintf("--archive-deleted=%t", *req.Options.ArchiveDeleted))
		}
		if req.Options.Consolidate != nil {
			args = append(args, fmt.Sprintf("--consolidate=%t", *req.Options.Consolidate))
		}
		if req.Options.DryRun != nil && *req.Options.DryRun {
			args = append(args, "--dry-run")
		}
		if req.Options.Verbose != nil && *req.Options.Verbose {
			args = append(args, "--verbose")
		}
		if req.Options.Limit != nil && *req.Options.Limit > 0 {
			args = append(args, fmt.Sprintf("--limit=%d", *req.Options.Limit))
		}
	}

	// Retry options
	if req.Retry != nil {
		if req.Retry.MaxAttempts != nil {
			args = append(args, fmt.Sprintf("--retries=%d", *req.Retry.MaxAttempts))
		}
		if req.Retry.InitialDelayMs != nil {
			args = append(args, fmt.Sprintf("--retry-delay=%d", *req.Retry.InitialDelayMs))
		}
		if req.Retry.TimeoutSeconds != nil {
			args = append(args, fmt.Sprintf("--timeout=%d", *req.Retry.TimeoutSeconds))
		}
	}

	return args
}

// Regex patterns for parsing CLI output (handles any log timestamp prefix)
var (
	reFound   = regexp.MustCompile(`Found (\d+) code elements`)
	reTotal   = regexp.MustCompile(`Total: (\d+), Ingested: (\d+), Errors: (\d+)`)
	reRate    = regexp.MustCompile(`Rate: ([\d.]+) elements/sec`)
	reSymbols = regexp.MustCompile(`Symbols: (\d+)`)
)

// parseIngestOutput extracts stats from CLI output using regex.
// The CLI uses log.Printf which prepends timestamps, so regex is used
// to match patterns anywhere in each line regardless of prefix.
func parseIngestOutput(output string) IngestCodebaseStats {
	stats := IngestCodebaseStats{}

	if m := reFound.FindStringSubmatch(output); len(m) == 2 {
		stats.FilesFound, _ = strconv.ParseInt(m[1], 10, 64)
	}
	if m := reTotal.FindStringSubmatch(output); len(m) == 4 {
		stats.FilesFound, _ = strconv.ParseInt(m[1], 10, 64)
		stats.FilesProcessed, _ = strconv.ParseInt(m[2], 10, 64)
		stats.Errors, _ = strconv.ParseInt(m[3], 10, 64)
	}
	if m := reRate.FindStringSubmatch(output); len(m) == 2 {
		stats.Rate, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := reSymbols.FindStringSubmatch(output); len(m) == 2 {
		stats.SymbolsExtracted, _ = strconv.ParseInt(m[1], 10, 64)
	}

	return stats
}

// handleIngestCodebaseStatus handles GET /v1/memory/ingest-codebase/{job_id}
func (s *Server) handleIngestCodebaseStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path
	jobID := strings.TrimPrefix(r.URL.Path, "/v1/memory/ingest-codebase/")
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job_id is required"})
		return
	}

	ingestJobsMu.RLock()
	job, exists := ingestJobs[jobID]
	ingestJobsMu.RUnlock()

	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	job.mu.Lock()
	resp := IngestCodebaseResponse{
		JobID:   job.ID,
		Status:  job.Status,
		SpaceID: job.SpaceID,
		Path:    job.Path,
		Stats:   &job.Stats,
		Error:   job.Error,
	}
	job.mu.Unlock()

	writeJSON(w, http.StatusOK, resp)
}

// handleIngestCodebaseCancel handles DELETE /v1/memory/ingest-codebase/{job_id}
func (s *Server) handleIngestCodebaseCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path
	jobID := strings.TrimPrefix(r.URL.Path, "/v1/memory/ingest-codebase/")
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job_id is required"})
		return
	}

	ingestJobsMu.RLock()
	job, exists := ingestJobs[jobID]
	ingestJobsMu.RUnlock()

	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	job.mu.Lock()
	if job.Status == "running" || job.Status == "queued" {
		job.Cancel()
		job.Status = "cancelled"
	}
	job.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "job_id": jobID})
}

// handleIngestCodebaseList handles GET /v1/memory/ingest-codebase
func (s *Server) handleIngestCodebaseList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		// Fall through to POST handler
		s.handleIngestCodebase(w, r)
		return
	}

	ingestJobsMu.RLock()
	jobs := make([]IngestCodebaseResponse, 0, len(ingestJobs))
	for _, job := range ingestJobs {
		job.mu.Lock()
		jobs = append(jobs, IngestCodebaseResponse{
			JobID:   job.ID,
			Status:  job.Status,
			SpaceID: job.SpaceID,
			Path:    job.Path,
			Stats:   &job.Stats,
			Error:   job.Error,
		})
		job.mu.Unlock()
	}
	ingestJobsMu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": jobs})
}

