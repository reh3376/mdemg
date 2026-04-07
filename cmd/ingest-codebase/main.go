// Command ingest-codebase walks a codebase and ingests files into MDEMG
// with optimized batch processing and configurable timeouts.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
	"mdemg/internal/config"
	"mdemg/internal/languages"
	"mdemg/internal/summarize"
)

var (
	codebasePath   = flag.String("path", "", "Path to codebase to ingest")
	spaceID        = flag.String("space-id", "codebase", "MDEMG space ID")
	mdemgEndpoint  = flag.String("endpoint", "", "MDEMG endpoint (default: from LISTEN_ADDR in .env)")
	batchSize      = flag.Int("batch", 100, "Batch size for ingestion (default: 100, optimal for ~15/s per worker)")
	workers        = flag.Int("workers", 4, "Number of parallel workers (default: 4)")
	timeout        = flag.Int("timeout", 300, "HTTP timeout in seconds (default: 300)")
	delay          = flag.Int("delay", 50, "Delay between batches in ms (default: 50)")
	maxRetries     = flag.Int("retries", 3, "Max retries per batch on failure (default: 3)")
	retryDelay     = flag.Int("retry-delay", 2000, "Initial retry delay in ms, doubles each retry (default: 2000)")
	consolidate    = flag.Bool("consolidate", true, "Run consolidation after ingestion")
	dryRun         = flag.Bool("dry-run", false, "Print what would be ingested without actually doing it")
	verbose        = flag.Bool("verbose", false, "Verbose output")
	excludeDirs    = flag.String("exclude", ".git,vendor,node_modules,.worktrees", "Comma-separated directories to exclude")
	includeTests   = flag.Bool("include-tests", false, "Include test files (*_test.go, *.test.ts, *.spec.ts)")
	includeMd      = flag.Bool("include-md", true, "Include markdown files (*.md)")
	includeTS      = flag.Bool("include-ts", true, "Include TypeScript/JavaScript files (*.ts, *.tsx, *.js, *.jsx)")
	includePy      = flag.Bool("include-py", true, "Include Python files (*.py)")
	includeJava    = flag.Bool("include-java", true, "Include Java files (*.java)")
	includeRust    = flag.Bool("include-rust", true, "Include Rust files (*.rs)")
	includePHP     = flag.Bool("include-php", true, "Include PHP files (*.php)")
	limitElements  = flag.Int("limit", 0, "Limit number of elements to ingest (0 = no limit)")
	extractSymbols = flag.Bool("extract-symbols", true, "Extract code symbols (constants, functions, classes) for evidence-locked retrieval")
	incremental    = flag.Bool("incremental", false, "Only ingest files changed since last commit (uses git diff)")
	sinceCommit    = flag.String("since", "HEAD~1", "Git commit to compare against for incremental mode (default: HEAD~1)")
	archiveDeleted = flag.Bool("archive-deleted", true, "Archive nodes for deleted files in incremental mode")
	quiet          = flag.Bool("quiet", false, "Suppress all non-error output")
	logFile        = flag.String("log-file", "", "Write logs to file instead of stderr")

	// LLM summary options
	llmSummary         = flag.Bool("llm-summary", false, "Use LLM to generate semantic summaries (requires OPENAI_API_KEY)")
	llmSummaryModel    = flag.String("llm-summary-model", "gpt-4o-mini", "Model for LLM summaries")
	llmSummaryBatch    = flag.Int("llm-summary-batch", 10, "Files per LLM API call for summaries")
	llmSummaryProvider = flag.String("llm-summary-provider", "openai", "LLM provider for summaries (openai/ollama)")

	// Progress reporting
	progressJSON = flag.Bool("progress-json", false, "Emit structured JSON progress lines to stdout (logs go to stderr)")

	// Info flags
	listLanguages = flag.Bool("list-languages", false, "List supported languages and exit")

	// Phase 2.5: Performance guards for large repos
	maxFileSize        = flag.Int("max-file-size", 1048576, "Max file size in bytes to process (default: 1MB)")
	maxElementsPerFile = flag.Int("max-elements-per-file", 500, "Max elements to extract per file (default: 500)")
	maxSymbolsPerFile  = flag.Int("max-symbols-per-file", 1000, "Max symbols to extract per file (default: 1000)")
	preset             = flag.String("preset", "", "Exclusion preset: default, ml_cuda, web_monorepo")
)

// Global summarize service (nil if disabled)
var summarizeSvc *summarize.Service

type BatchIngestRequest struct {
	SpaceID      string            `json:"space_id"`
	Observations []BatchIngestItem `json:"observations"`
}

type BatchIngestItem struct {
	Timestamp string         `json:"timestamp"`
	Source    string         `json:"source"`
	Name      string         `json:"name"`
	Path      string         `json:"path"`
	Content   string         `json:"content"`
	Summary   string         `json:"summary,omitempty"`
	Tags      []string       `json:"tags"`
	Symbols   []IngestSymbol `json:"symbols,omitempty"`
}

// IngestSymbol represents a code symbol being ingested
// IngestSymbol represents a code symbol extracted during ingestion.
// Field names follow UPTS (Universal Parser Test Specification) v1.0.0
type IngestSymbol struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Line           int    `json:"line"`               // 1-indexed line number (UPTS standard)
	LineEnd        int    `json:"line_end,omitempty"` // End line for multi-line symbols
	Exported       bool   `json:"exported"`
	Parent         string `json:"parent,omitempty"`
	Signature      string `json:"signature,omitempty"`
	Value          string `json:"value,omitempty"`
	RawValue       string `json:"raw_value,omitempty"`
	DocComment     string `json:"doc_comment,omitempty"`
	TypeAnnotation string `json:"type_annotation,omitempty"`
	Language       string `json:"language,omitempty"`
}

type CodeElement struct {
	Name     string
	Kind     string
	Path     string
	Content  string
	Summary  string // Brief summary for reranking (generated from docstrings/comments)
	Package  string
	FilePath string
	Tags     []string
	Concerns []string       // Cross-cutting concerns detected in this element
	Symbols  []IngestSymbol // Extracted code symbols (constants, functions, etc.)
}

// Phase 2.5: Exclusion presets for different repo types
type exclusionPreset struct {
	excludeDirs     []string
	excludePatterns []string
	maxFileSize     int
}

var presets = map[string]exclusionPreset{
	"default": {
		excludeDirs: []string{
			".git", "node_modules", "vendor", "__pycache__",
			".venv", "venv", "build", "dist", "target",
		},
		excludePatterns: []string{
			"*.min.js", "*.bundle.js", "*.pyc",
		},
		maxFileSize: 1048576, // 1MB
	},
	"ml_cuda": {
		excludeDirs: []string{
			".git", "node_modules", "vendor", "__pycache__",
			".venv", "venv", "build", "dist", "target",
			"third_party", "data", "datasets", "checkpoints",
			"logs", "wandb", "outputs", ".cache",
		},
		excludePatterns: []string{
			"*.min.js", "*.bundle.js", "*.pyc",
			"*.pt", "*.pth", "*.onnx", "*.bin",
			"*.safetensors", "*.npy", "*.npz",
		},
		maxFileSize: 524288, // 512KB
	},
	"web_monorepo": {
		excludeDirs: []string{
			".git", "node_modules", "vendor", "__pycache__",
			".venv", "venv", "build", "dist", "target",
			".next", ".nuxt", ".output", "coverage", "storybook-static",
		},
		excludePatterns: []string{
			"*.min.js", "*.bundle.js", "*.pyc",
			"*.chunk.js", "*.map",
		},
		maxFileSize: 1048576, // 1MB
	},
}

// Cross-cutting concern patterns for detection
var concernPatterns = map[string][]string{
	"authentication": {
		"auth", "login", "logout", "signin", "signout", "session",
		"token", "jwt", "oauth", "msal", "azure-ad", "passport",
	},
	"authorization": {
		"acl", "rbac", "permission", "role", "guard", "policy",
		"access-control", "authorize", "can-activate",
	},
	"error-handling": {
		"error", "exception", "filter", "catch", "handler",
		"fault", "failure", "recovery",
	},
	"validation": {
		"validat", "schema", "dto", "constraint", "sanitize",
		"class-validator", "joi", "zod",
	},
	"logging": {
		"logger", "logging", "log", "audit", "trace", "monitor",
	},
	"caching": {
		"cache", "redis", "memcache", "ttl", "invalidat",
	},
	"temporal": {
		"validfrom", "validto", "valid_from", "valid_to",
		"effectivedate", "effective_date", "expirationdate", "expiration_date",
		"startdate", "start_date", "enddate", "end_date",
		"createdat", "created_at", "updatedat", "updated_at", "deletedat", "deleted_at",
		"softdelete", "soft_delete", "paranoid",
		"temporal", "bitemporal", "versioned",
		"daterange", "date_range", "timerange", "time_range",
		"historiz", "audit_trail", "snapshot",
	},
}

// NestJS decorator patterns that indicate cross-cutting concerns
var decoratorConcerns = map[string]string{
	"@Guard":           "authorization",
	"@UseGuards":       "authorization",
	"@Interceptor":     "cross-cutting",
	"@UseInterceptors": "cross-cutting",
	"@Filter":          "error-handling",
	"@UseFilters":      "error-handling",
	"@Catch":           "error-handling",
	"@Injectable":      "service",
}

// Track 6: UI/UX patterns for React/Next.js applications
var uiPatterns = map[string][]string{
	"store": {
		"zustand", "create(", "usestore", "redux", "useselector",
		"usedispatch", "configurestore", "createslice",
		"recoil", "atom(", "selector(",
	},
	"component": {
		"usestate", "useeffect", "usememo", "usecallback",
		"useref", "forwardref", "memo(", "react.fc",
		"react.component", "extends component",
	},
	"routing": {
		"userouter", "usepathname", "usesearchparams",
		"usenavigate", "useparams", "uselocation",
		"next/link", "next/navigation", "react-router",
	},
	"data-fetching": {
		"usequery", "usemutation", "useswr", "tanstack",
		"react-query", "@tanstack/react-query",
		"useinfinitequery", "queryclient",
	},
	"ui-library": {
		"dnd-kit", "usedraggable", "usedroppable", "usesortable",
		"framer-motion", "motion.", "animate",
		"radix-ui", "shadcn", "@radix-ui",
		"tailwindcss", "classnames", "clsx",
	},
	"form": {
		"useform", "react-hook-form", "formik", "useformik",
		"usecontroller", "usewatch", "usefieldarray",
	},
	"context": {
		"createcontext", "usecontext", "provider value=",
		"contextprovider", ".provider>",
	},
}

// Configuration file patterns for detection
var configFilePatterns = []string{
	".config.ts", ".config.js", ".config.json", ".config.yaml", ".config.yml",
	"config.ts", "config.js", "config.json", "config.yaml", "config.yml",
	".env.example", ".env.sample", ".env.template",
	"tsconfig.json", "package.json", "nest-cli.json", "angular.json",
	"webpack.config", "vite.config", "rollup.config", "jest.config",
	"pyproject.toml", "setup.cfg", "setup.py", "requirements.txt",
	"go.mod", "go.sum", "Makefile", "Dockerfile", "docker-compose",
}

// Configuration directory patterns
var configDirPatterns = []string{
	"/config/", "/configuration/", "/configs/", "/settings/",
	"/env/", "/environments/",
}

// isConfigFile checks if a file path indicates a configuration file
func isConfigFile(filePath string) bool {
	lowerPath := strings.ToLower(filePath)

	// Check directory patterns
	for _, pattern := range configDirPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}

	// Check file patterns
	for _, pattern := range configFilePatterns {
		if strings.HasSuffix(lowerPath, pattern) || strings.Contains(filepath.Base(lowerPath), pattern) {
			return true
		}
	}

	// Check for Config suffix in filename (e.g., AppConfig.ts, DatabaseConfig.py)
	base := filepath.Base(filePath)
	baseLower := strings.ToLower(base)
	if strings.Contains(baseLower, "config") && !strings.Contains(baseLower, "test") {
		return true
	}

	return false
}

type IngestStats struct {
	TotalElements int64
	Ingested      int64
	Errors        int64
	Symbols       int64
	StartTime     time.Time
}

// progressEvent is a structured JSON progress line emitted to stdout when --progress-json is set.
type progressEvent struct {
	Event    string  `json:"event"`
	Total    int     `json:"total,omitempty"`
	Current  int     `json:"current,omitempty"`
	Ingested int     `json:"ingested,omitempty"`
	Errors   int     `json:"errors,omitempty"`
	Symbols  int     `json:"symbols,omitempty"`
	Rate     float64 `json:"rate,omitempty"`
	Duration string  `json:"duration,omitempty"`
}

// emitProgress writes a JSON progress event to stdout if --progress-json is enabled.
func emitProgress(evt progressEvent) {
	if !*progressJSON {
		return
	}
	data, _ := json.Marshal(evt)
	fmt.Fprintln(os.Stdout, string(data))
}

// GitDiffResult contains files changed between commits
type GitDiffResult struct {
	Added    []string
	Modified []string
	Deleted  []string
	Renamed  map[string]string // old -> new
}

// getGitChangedFiles returns files changed since the given commit
func getGitChangedFiles(repoPath, sinceCommit string) (*GitDiffResult, error) {
	result := &GitDiffResult{
		Renamed: make(map[string]string),
	}

	// Run git diff with name-status to get file change types
	cmd := exec.Command("git", "-C", repoPath, "diff", "--name-status", sinceCommit)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		switch {
		case status == "A":
			result.Added = append(result.Added, parts[1])
		case status == "M":
			result.Modified = append(result.Modified, parts[1])
		case status == "D":
			result.Deleted = append(result.Deleted, parts[1])
		case strings.HasPrefix(status, "R"):
			// Renamed: R100 old_path new_path
			if len(parts) >= 3 {
				result.Renamed[parts[1]] = parts[2]
				result.Modified = append(result.Modified, parts[2]) // Treat renamed as modified
			}
		}
	}

	return result, nil
}

// archiveDeletedNodes archives MemoryNodes for deleted files
func archiveDeletedNodes(client *http.Client, endpoint, spaceID string, deletedPaths []string) (int, error) {
	if len(deletedPaths) == 0 {
		return 0, nil
	}

	archived := 0
	for _, path := range deletedPaths {
		// Normalize path to match what's stored in the graph
		fullPath := "/" + path

		// Call bulk archive endpoint
		reqBody := map[string]any{
			"space_id": spaceID,
			"filter": map[string]any{
				"path_prefix": fullPath,
			},
			"reason": "File deleted from codebase",
		}
		body, _ := json.Marshal(reqBody)

		resp, err := client.Post(endpoint+"/v1/memory/archive/bulk", "application/json", bytes.NewReader(body))
		if err != nil {
			slog.Warn("failed to archive nodes", "path", path, "error", err)
			continue
		}
		resp.Body.Close()
		archived++
	}

	return archived, nil
}

// getExcludeDirList returns a sorted list of excluded directories
func getExcludeDirList(excludeSet map[string]bool) []string {
	var dirs []string
	for dir := range excludeSet {
		dirs = append(dirs, dir)
	}
	return dirs
}

// matchesExcludePattern checks if a filename matches any exclusion pattern
func matchesExcludePattern(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
	}
	return false
}

func main() {
	flag.Parse()

	// Log output priority: --log-file > --quiet > --progress-json stderr redirect > default
	var logWriter io.Writer = os.Stderr
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Error("failed to open log file", "path", *logFile, "error", err)
			os.Exit(1)
		}
		defer f.Close()
		logWriter = f
	} else if *quiet {
		logWriter = io.Discard
	}
	// When --progress-json is set, logWriter is already os.Stderr (default)
	// so stdout is reserved for structured JSON progress events.
	slog.SetDefault(slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// Handle --list-languages flag
	if *listLanguages {
		fmt.Println("Supported Languages:")
		fmt.Println()
		fmt.Printf("%-20s %-15s %s\n", "LANGUAGE", "PARSER", "EXTENSIONS")
		fmt.Printf("%-20s %-15s %s\n", "--------", "------", "----------")
		for _, parser := range languages.AllParsers() {
			fmt.Printf("%-20s %-15s %s\n",
				parser.Name(),
				parser.Name()+"_parser.go",
				strings.Join(parser.Extensions(), ", "))
		}
		fmt.Println()
		fmt.Printf("Total: %d languages\n", len(languages.AllParsers()))
		os.Exit(0)
	}

	// Load .env file for dynamic configuration
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, using defaults/flags")
	}

	// Resolve endpoint via priority chain: --endpoint flag > MDEMG_ENDPOINT env > .mdemg.port > LISTEN_ADDR > default
	if *mdemgEndpoint == "" {
		*mdemgEndpoint = config.ResolveEndpoint("http://localhost:9999")
		slog.Info("resolved endpoint", "endpoint", *mdemgEndpoint)
	}

	if *codebasePath == "" {
		slog.Error("--path is required")
		os.Exit(1)
	}

	// Phase 2.5: Apply preset if specified
	excludeSet := make(map[string]bool)
	var excludePatterns []string
	if *preset != "" {
		if p, ok := presets[*preset]; ok {
			for _, dir := range p.excludeDirs {
				excludeSet[dir] = true
			}
			excludePatterns = p.excludePatterns
			// Apply preset max file size if no CLI override
			if *maxFileSize == 1048576 { // default value
				*maxFileSize = p.maxFileSize
			}
			slog.Info("applied preset", "preset", *preset)
		} else {
			slog.Error("unknown preset", "preset", *preset, "available", "default, ml_cuda, web_monorepo")
			os.Exit(1)
		}
	}
	// Merge CLI excludeDirs with preset
	for _, dir := range strings.Split(*excludeDirs, ",") {
		excludeSet[strings.TrimSpace(dir)] = true
	}

	slog.Info("MDEMG codebase ingestion started",
		"path", *codebasePath,
		"space_id", *spaceID,
		"endpoint", *mdemgEndpoint,
		"batch_size", *batchSize,
		"workers", *workers,
		"timeout_s", *timeout,
		"excluded_dirs", getExcludeDirList(excludeSet),
		"max_file_size", *maxFileSize,
		"max_elements_per_file", *maxElementsPerFile,
		"max_symbols_per_file", *maxSymbolsPerFile,
		"extract_symbols", *extractSymbols,
		"incremental", *incremental,
		"llm_summaries", *llmSummary,
	)
	if len(excludePatterns) > 0 {
		slog.Info("exclude patterns active", "patterns", excludePatterns)
	}

	// Initialize LLM summarize service if enabled
	if *llmSummary {
		apiKey := os.Getenv("OPENAI_API_KEY")
		ollamaEndpoint := os.Getenv("OLLAMA_ENDPOINT")
		if ollamaEndpoint == "" {
			ollamaEndpoint = "http://localhost:11434"
		}

		if *llmSummaryProvider == "openai" && apiKey == "" {
			slog.Warn("LLM summaries enabled but OPENAI_API_KEY not set, using structural fallback")
		} else {
			cfg := summarize.Config{
				Enabled:        true,
				Provider:       *llmSummaryProvider,
				Model:          *llmSummaryModel,
				MaxTokens:      150,
				BatchSize:      *llmSummaryBatch,
				TimeoutMs:      30000,
				CacheEnabled:   true,
				CacheSize:      5000,
				Debug:          *verbose,
				OpenAIAPIKey:   apiKey,
				OpenAIEndpoint: os.Getenv("OPENAI_ENDPOINT"),
				OllamaEndpoint: ollamaEndpoint,
			}
			if cfg.OpenAIEndpoint == "" {
				cfg.OpenAIEndpoint = "https://api.openai.com/v1"
			}

			var err error
			summarizeSvc, err = summarize.New(cfg, generateSummaryAdapter)
			if err != nil {
				slog.Warn("failed to initialize LLM summarize service, using structural fallback", "error", err)
			} else {
				slog.Info("LLM summarize service initialized", "provider", *llmSummaryProvider, "model", *llmSummaryModel, "batch", *llmSummaryBatch)
			}
		}
	}

	// Handle incremental mode
	var changedFiles *GitDiffResult
	if *incremental {
		slog.Info("getting changed files", "since_commit", *sinceCommit)
		var err error
		changedFiles, err = getGitChangedFiles(*codebasePath, *sinceCommit)
		if err != nil {
			slog.Error("failed to get git diff", "error", err)
			os.Exit(1)
		}
		slog.Info("changed files detected", "added", len(changedFiles.Added), "modified", len(changedFiles.Modified), "deleted", len(changedFiles.Deleted), "renamed", len(changedFiles.Renamed))

		if len(changedFiles.Added)+len(changedFiles.Modified) == 0 && len(changedFiles.Deleted) == 0 {
			slog.Info("no changes detected, exiting")
			return
		}
	}

	// Diagnostics summary
	diagSummary := languages.NewDiagnosticSummary()

	// Collect all code elements
	elements, err := walkCodebase(*codebasePath, excludeSet, excludePatterns, diagSummary)
	if err != nil {
		slog.Error("failed to walk codebase", "error", err)
		os.Exit(1)
	}

	// Filter to only changed files in incremental mode
	if *incremental && changedFiles != nil {
		changedSet := make(map[string]bool)
		for _, f := range changedFiles.Added {
			changedSet[f] = true
		}
		for _, f := range changedFiles.Modified {
			changedSet[f] = true
		}

		var filtered []CodeElement
		for _, elem := range elements {
			// Convert element path to relative path for comparison
			relPath := strings.TrimPrefix(elem.FilePath, "/")
			if changedSet[relPath] || changedSet[elem.FilePath] {
				filtered = append(filtered, elem)
			}
		}
		slog.Info("filtered to changed files only", "original", len(elements), "filtered", len(filtered))
		elements = filtered
	}

	slog.Info("found code elements", "count", len(elements))
	emitProgress(progressEvent{Event: "discovery_complete", Total: len(elements)})

	// Log diagnostic summary
	if diagSummary.Total > 0 {
		slog.Info("diagnostics summary", "total", diagSummary.Total, "info", diagSummary.BySev["info"], "warning", diagSummary.BySev["warning"], "error", diagSummary.BySev["error"])
		if *verbose {
			for code, count := range diagSummary.ByCode {
				slog.Info("diagnostic code", "code", code, "count", count)
			}
		}
	}

	// Apply limit if specified
	if *limitElements > 0 && len(elements) > *limitElements {
		slog.Info("limiting elements", "limit", *limitElements, "total", len(elements))
		elements = elements[:*limitElements]
	}

	if *dryRun {
		slog.Info("dry run, not ingesting")
		printSample(elements)
		return
	}

	// Create HTTP client with extended timeout
	client := &http.Client{
		Timeout: time.Duration(*timeout) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	stats := &IngestStats{
		TotalElements: int64(len(elements)),
		StartTime:     time.Now(),
	}

	// Create batches
	var batches [][]CodeElement
	for i := 0; i < len(elements); i += *batchSize {
		end := i + *batchSize
		if end > len(elements) {
			end = len(elements)
		}
		batches = append(batches, elements[i:end])
	}

	slog.Info("processing batches", "batches", len(batches), "workers", *workers)

	// Process batches with worker pool
	batchChan := make(chan []CodeElement, len(batches))
	var wg sync.WaitGroup

	// Start workers
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for batch := range batchChan {
				ingested, errors, syms := ingestBatch(client, batch)
				atomic.AddInt64(&stats.Ingested, int64(ingested))
				atomic.AddInt64(&stats.Errors, int64(errors))
				atomic.AddInt64(&stats.Symbols, int64(syms))

				current := atomic.LoadInt64(&stats.Ingested)
				errCount := atomic.LoadInt64(&stats.Errors)
				elapsed := time.Since(stats.StartTime)
				rate := float64(current) / elapsed.Seconds()

				slog.Info("worker progress", "worker", workerID, "current", current, "total", stats.TotalElements, "rate", rate, "errors", errCount)

				emitProgress(progressEvent{
					Event:   "batch_progress",
					Current: int(current),
					Total:   int(stats.TotalElements),
					Rate:    rate,
				})

				time.Sleep(time.Duration(*delay) * time.Millisecond)
			}
		}(w)
	}

	// Send batches to workers
	for _, batch := range batches {
		batchChan <- batch
	}
	close(batchChan)

	// Wait for all workers to complete
	wg.Wait()

	elapsed := time.Since(stats.StartTime)
	slog.Info("ingestion complete", "total", stats.TotalElements, "ingested", stats.Ingested, "errors", stats.Errors, "symbols", stats.Symbols, "duration", elapsed, "rate_elements_per_sec", float64(stats.Ingested)/elapsed.Seconds())

	// Print LLM summary stats if service was used
	if summarizeSvc != nil {
		totalCalls, cacheHits, cacheSize := summarizeSvc.Stats()
		slog.Info("LLM summary stats", "calls", totalCalls, "cache_hits", cacheHits, "cache_size", cacheSize)
	}

	// Handle deleted files in incremental mode
	if *incremental && *archiveDeleted && changedFiles != nil && len(changedFiles.Deleted) > 0 {
		slog.Info("archiving deleted files", "count", len(changedFiles.Deleted))
		archived, err := archiveDeletedNodes(client, *mdemgEndpoint, *spaceID, changedFiles.Deleted)
		if err != nil {
			slog.Warn("archive failed", "error", err)
		} else {
			slog.Info("archived nodes for deleted files", "archived", archived)
		}
	}

	// Emit complete progress event
	emitProgress(progressEvent{
		Event:    "complete",
		Total:    int(stats.TotalElements),
		Ingested: int(stats.Ingested),
		Errors:   int(stats.Errors),
		Duration: elapsed.Round(time.Second).String(),
	})

	// Run consolidation
	if *consolidate && stats.Ingested > 0 {
		emitProgress(progressEvent{Event: "consolidation_start"})
		slog.Info("running consolidation")
		if err := runConsolidation(client); err != nil {
			slog.Error("consolidation failed", "error", err)
		}
	}
}

// convertLanguageElement converts a languages.CodeElement to main.CodeElement
func convertLanguageElement(elem languages.CodeElement) CodeElement {
	// Convert symbols (UPTS-compliant field mapping)
	var symbols []IngestSymbol
	for _, s := range elem.Symbols {
		symbols = append(symbols, IngestSymbol{
			Name:           s.Name,
			Type:           s.Type,
			Line:           s.Line,
			LineEnd:        s.LineEnd,
			Exported:       s.Exported,
			Parent:         s.Parent,
			Signature:      s.Signature,
			Value:          s.Value,
			RawValue:       s.RawValue,
			DocComment:     s.DocComment,
			TypeAnnotation: s.TypeAnnotation,
			Language:       s.Language,
		})
	}

	return CodeElement{
		Name:     elem.Name,
		Kind:     elem.Kind,
		Path:     elem.Path,
		Content:  elem.Content,
		Summary:  elem.Summary,
		Package:  elem.Package,
		FilePath: elem.FilePath,
		Tags:     elem.Tags,
		Concerns: elem.Concerns,
		Symbols:  symbols,
	}
}

// getEnabledLanguages returns a map of language names that are enabled via CLI flags
func getEnabledLanguages() map[string]bool {
	return map[string]bool{
		"go":         true,
		"rust":       *includeRust,
		"python":     *includePy,
		"typescript": *includeTS,
		"java":       *includeJava,
		"markdown":   *includeMd,
		"json":       true,
		"sql":        true,
		"xml":        true,
		"c":          true,
		"cpp":        true,
		// Previously missing existing parsers
		"yaml":       true,
		"toml":       true,
		"ini":        true,
		"dockerfile": true,
		"shell":      true,
		"cuda":       true,
		"cypher":     true,
		// New parsers
		"csharp":           true,
		"kotlin":           true,
		"terraform":        true,
		"makefile":         true,
		"php":              *includePHP,
		"graphql":          true,
		"lua":              true,
		"protobuf":         true,
		"openapi":          true,
		"scraper-markdown": true,
	}
}

func walkCodebase(root string, excludeSet map[string]bool, excludePatterns []string, diagSummary *languages.DiagnosticSummary) ([]CodeElement, error) {
	var elements []CodeElement
	enabledLangs := getEnabledLanguages()

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			if excludeSet[info.Name()] {
				if *verbose {
					slog.Info("skipping excluded directory", "path", path)
				}
				return filepath.SkipDir
			}
			return nil
		}

		// Phase 2.5: File size early exit
		if *maxFileSize > 0 && info.Size() > int64(*maxFileSize) {
			if *verbose {
				slog.Info("skipping oversized file", "size", info.Size(), "max_size", *maxFileSize, "path", path)
			}
			diagSummary.Add(languages.Diagnostic{
				Severity: "warning",
				Code:     "LARGE_FILE",
				Message:  fmt.Sprintf("File exceeds size threshold (%d bytes > %d max)", info.Size(), *maxFileSize),
				Context:  map[string]string{"path": path},
			})
			return nil
		}

		// Phase 2.5: Exclude files matching patterns
		if matchesExcludePattern(info.Name(), excludePatterns) {
			if *verbose {
				slog.Info("skipping file matching exclude pattern", "path", path)
			}
			return nil
		}

		// Try to find a parser for this file
		parser, ok := languages.GetParserForFile(path)
		if ok {
			// Check if this language is enabled
			if !enabledLangs[parser.Name()] {
				return nil
			}

			// Skip test files unless includeTests is set
			if !*includeTests && parser.IsTestFile(path) {
				return nil
			}

			// Special handling for JSON - only parse config files
			if parser.Name() == "json" {
				if !isConfigFile(path) {
					return nil
				}
			}

			// Parse the file using the modular parser
			langElements, parseErr := parser.ParseFile(root, path, *extractSymbols)
			if parseErr != nil {
				if *verbose {
					slog.Error("parse error", "path", path, "error", parseErr)
				}
				return nil
			}

			// Phase 2.5: Apply per-file element cap
			if *maxElementsPerFile > 0 && len(langElements) > *maxElementsPerFile {
				if *verbose {
					slog.Info("capping elements", "path", path, "original", len(langElements), "capped", *maxElementsPerFile)
				}
				langElements = langElements[:*maxElementsPerFile]
			}

			// Collect diagnostics from parsed elements
			for _, elem := range langElements {
				for _, d := range elem.Diagnostics {
					diagSummary.Add(d)
					if *verbose {
						slog.Info("diagnostic", "severity", d.Severity, "code", d.Code, "message", d.Message, "path", path)
					}
				}
			}

			// Convert and append elements with symbol caps
			for _, elem := range langElements {
				converted := convertLanguageElement(elem)
				// Phase 2.5: Apply per-file symbol cap
				if *maxSymbolsPerFile > 0 && len(converted.Symbols) > *maxSymbolsPerFile {
					if *verbose {
						slog.Info("capping symbols", "path", path, "original", len(converted.Symbols), "capped", *maxSymbolsPerFile)
					}
					converted.Symbols = converted.Symbols[:*maxSymbolsPerFile]
				}
				elements = append(elements, converted)
			}
			return nil
		}

		// Fallback: Process env example files (not handled by language parsers)
		if strings.Contains(filepath.Base(path), ".env.") && !strings.HasSuffix(path, ".env") {
			// .env.example, .env.sample, .env.template, etc.
			envElement := parseEnvFile(root, path)
			if envElement != nil {
				elements = append(elements, *envElement)
			}
			return nil
		}

		// Fallback: YAML config files (not yet in language parsers)
		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			if isConfigFile(path) {
				configElement := parseConfigFile(root, path)
				if configElement != nil {
					elements = append(elements, *configElement)
				}
			}
			return nil
		}

		return nil
	})

	return elements, err
}

// findAllMatches returns all capture group 1 matches for a pattern
func findAllMatches(content, pattern string) []string {
	re := regexp.MustCompile(`(?m)` + pattern)
	matches := re.FindAllStringSubmatch(content, -1)
	var results []string
	for _, m := range matches {
		if len(m) > 1 && m[1] != "" {
			results = append(results, m[1])
		}
	}
	return results
}

// uniqueStrings returns unique strings from a slice
func uniqueStrings(strs []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range strs {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// detectConcerns analyzes file path and content to detect cross-cutting concerns
func detectConcerns(filePath, content string) []string {
	detected := make(map[string]bool)
	lowerPath := strings.ToLower(filePath)
	lowerContent := strings.ToLower(content)

	// Check file path patterns
	for concern, patterns := range concernPatterns {
		for _, pattern := range patterns {
			if strings.Contains(lowerPath, pattern) {
				detected[concern] = true
				break
			}
		}
	}

	// Check content for NestJS decorators
	for decorator, concern := range decoratorConcerns {
		if strings.Contains(content, decorator) {
			detected[concern] = true
		}
	}

	// Check content for concern patterns (limited to avoid false positives)
	// Only check if the pattern appears multiple times or in specific contexts
	for concern, patterns := range concernPatterns {
		if detected[concern] {
			continue // Already detected from path
		}
		matchCount := 0
		for _, pattern := range patterns {
			if strings.Count(lowerContent, pattern) >= 2 {
				matchCount++
			}
		}
		// Require at least 2 different pattern matches to reduce noise
		if matchCount >= 2 {
			detected[concern] = true
		}
	}

	// Convert to slice with "concern:" prefix for tag filtering
	var concerns []string
	for concern := range detected {
		concerns = append(concerns, "concern:"+concern)
	}

	// Track 6: Detect UI patterns for React/Next.js files
	uiTags := detectUIPatterns(filePath, content)
	concerns = append(concerns, uiTags...)

	return concerns
}

// detectUIPatterns analyzes file path and content to detect UI/UX patterns (Track 6)
func detectUIPatterns(filePath, content string) []string {
	// Only check TSX/JSX/TS/JS files in UI-related directories
	lowerPath := strings.ToLower(filePath)
	isUIFile := strings.HasSuffix(lowerPath, ".tsx") ||
		strings.HasSuffix(lowerPath, ".jsx") ||
		(strings.HasSuffix(lowerPath, ".ts") && (strings.Contains(lowerPath, "/components/") ||
			strings.Contains(lowerPath, "/stores/") ||
			strings.Contains(lowerPath, "/hooks/") ||
			strings.Contains(lowerPath, "/lib/") ||
			strings.Contains(lowerPath, "/app/") ||
			strings.Contains(lowerPath, "/pages/")))

	if !isUIFile {
		return nil
	}

	detected := make(map[string]bool)
	lowerContent := strings.ToLower(content)

	// Check path patterns for specific UI types
	if strings.Contains(lowerPath, "/stores/") || strings.Contains(lowerPath, "-store.") || strings.Contains(lowerPath, "store.ts") {
		detected["store"] = true
	}
	if strings.Contains(lowerPath, "/components/") {
		detected["component"] = true
	}
	if strings.Contains(lowerPath, "/hooks/") || strings.Contains(lowerPath, "use") && strings.HasSuffix(lowerPath, ".ts") {
		detected["hook"] = true
	}
	if strings.Contains(lowerPath, "/providers/") || strings.Contains(lowerPath, "provider") {
		detected["context"] = true
	}

	// Check content for UI patterns
	for uiType, patterns := range uiPatterns {
		matchCount := 0
		for _, pattern := range patterns {
			if strings.Contains(lowerContent, pattern) {
				matchCount++
			}
		}
		// Lower threshold for UI patterns - just need 1 match for specific patterns
		if matchCount >= 1 {
			detected[uiType] = true
		}
	}

	// Convert to slice with "ui:" prefix
	var uiTags []string
	for uiType := range detected {
		uiTags = append(uiTags, "ui:"+uiType)
	}
	return uiTags
}

// parseConfigFile extracts configuration from JSON/YAML files
func parseConfigFile(root, path string) *CodeElement {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(root, path)
	name := filepath.Base(path)
	contentStr := string(content)

	// Truncate if too long
	if len(contentStr) > 4000 {
		contentStr = contentStr[:4000] + "... [truncated]"
	}

	// Build summary based on file type
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Configuration file: %s. ", name))

	// Try to extract top-level keys for JSON
	if strings.HasSuffix(path, ".json") {
		keys := findAllMatches(contentStr, `"(\w+)":\s*[{\[\"]`)
		if len(keys) > 0 {
			if len(keys) > 10 {
				keys = keys[:10]
			}
			summary.WriteString(fmt.Sprintf("Configuration keys: %s. ", strings.Join(uniqueStrings(keys), ", ")))
		}
	}

	// For YAML, extract top-level keys
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		keys := findAllMatches(contentStr, `^(\w+):`)
		if len(keys) > 0 {
			if len(keys) > 10 {
				keys = keys[:10]
			}
			summary.WriteString(fmt.Sprintf("Configuration sections: %s. ", strings.Join(uniqueStrings(keys), ", ")))
		}
	}

	summary.WriteString(contentStr)

	concerns := detectConcerns(relPath, contentStr)
	tags := []string{"config", "configuration"}
	tags = append(tags, concerns...)

	return &CodeElement{
		Name:     name,
		Kind:     "config",
		Path:     "/" + relPath,
		Content:  summary.String(),
		Package:  "config",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
	}
}

// parseEnvFile extracts environment variable definitions from .env.* files
func parseEnvFile(root, path string) *CodeElement {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(root, path)
	name := filepath.Base(path)
	contentStr := string(content)

	// Extract environment variable names (excluding values for security)
	envVars := findAllMatches(contentStr, `^([A-Z][A-Z0-9_]*)=`)

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Environment configuration file: %s. ", name))
	summary.WriteString(fmt.Sprintf("Defines %d environment variables. ", len(envVars)))

	if len(envVars) > 0 {
		if len(envVars) > 20 {
			envVars = envVars[:20]
			summary.WriteString(fmt.Sprintf("Variables: %s (and more). ", strings.Join(envVars, ", ")))
		} else {
			summary.WriteString(fmt.Sprintf("Variables: %s. ", strings.Join(envVars, ", ")))
		}
	}

	// Include comments as they often document the variables
	comments := findAllMatches(contentStr, `^#\s*(.+)`)
	if len(comments) > 0 {
		if len(comments) > 5 {
			comments = comments[:5]
		}
		summary.WriteString(fmt.Sprintf("Documentation: %s ", strings.Join(comments, "; ")))
	}

	concerns := detectConcerns(relPath, contentStr)
	tags := []string{"config", "environment", "env-vars"}
	tags = append(tags, concerns...)

	return &CodeElement{
		Name:     name,
		Kind:     "config",
		Path:     "/" + relPath,
		Content:  summary.String(),
		Package:  "config",
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
	}
}

// generateSummaryAdapter adapts generateSummary for use with the summarize package.
// This allows the LLM summarize service to fall back to structural summaries.
func generateSummaryAdapter(elem summarize.CodeElement) string {
	return generateSummary(CodeElement{
		Name:     elem.Name,
		Kind:     elem.Kind,
		Path:     elem.Path,
		Content:  elem.Content,
		Package:  elem.Package,
		FilePath: elem.FilePath,
		Tags:     elem.Tags,
		Concerns: elem.Concerns,
	})
}

// generateSummary creates a brief summary of a code element for reranking.
// This summary helps the LLM reranker understand what each node contains
// without needing to read the full content.
// NOTE: Includes key method/function names for better search precision.
func generateSummary(elem CodeElement) string {
	var summary strings.Builder
	maxLen := 700 // Increased to accommodate key method names for search

	// Start with the kind and name
	switch elem.Kind {
	case "package", "config":
		summary.WriteString(fmt.Sprintf("%s: %s", strings.Title(elem.Kind), elem.Name))
	case "function", "method":
		summary.WriteString(fmt.Sprintf("%s %s", strings.Title(elem.Kind), elem.Name))
	case "struct", "interface", "type":
		summary.WriteString(fmt.Sprintf("%s %s", strings.Title(elem.Kind), elem.Name))
	case "const", "var":
		summary.WriteString(fmt.Sprintf("Constant %s", elem.Name))
	case "export":
		summary.WriteString(fmt.Sprintf("Export %s", elem.Name))
	case "class":
		summary.WriteString(fmt.Sprintf("Class %s", elem.Name))
	case "module", "react-component", "typescript", "javascript":
		summary.WriteString(fmt.Sprintf("Module: %s", elem.Name))
	case "python-module":
		summary.WriteString(fmt.Sprintf("Python module: %s", elem.Name))
	case "documentation":
		summary.WriteString(fmt.Sprintf("Documentation: %s", elem.Name))
	default:
		summary.WriteString(fmt.Sprintf("%s: %s", elem.Kind, elem.Name))
	}

	// Add package/location context
	if elem.Package != "" && elem.Package != elem.Name {
		summary.WriteString(fmt.Sprintf(" in %s", elem.Package))
	}

	// Add concern tags if present
	if len(elem.Concerns) > 0 {
		// Filter and dedupe concern names (remove "concern:" prefix)
		var concerns []string
		seen := make(map[string]bool)
		for _, c := range elem.Concerns {
			name := strings.TrimPrefix(c, "concern:")
			name = strings.TrimPrefix(name, "ui:")
			if !seen[name] && name != "" {
				seen[name] = true
				concerns = append(concerns, name)
			}
		}
		if len(concerns) > 0 {
			if len(concerns) > 3 {
				concerns = concerns[:3]
			}
			summary.WriteString(fmt.Sprintf(". Related to: %s", strings.Join(concerns, ", ")))
		}
	}

	// Add symbol information if symbols were extracted
	if len(elem.Symbols) > 0 {
		var classNames []string
		var methodNames []string
		var funcNames []string
		for _, s := range elem.Symbols {
			switch s.Type {
			case "class":
				classNames = append(classNames, s.Name)
			case "method":
				methodNames = append(methodNames, s.Name)
			case "function":
				funcNames = append(funcNames, s.Name)
			}
		}
		// Include class names in summary (critical for re-ranking)
		if len(classNames) > 0 {
			if len(classNames) > 3 {
				summary.WriteString(fmt.Sprintf(". Defines classes: %s, and %d more", strings.Join(classNames[:3], ", "), len(classNames)-3))
			} else {
				summary.WriteString(fmt.Sprintf(". Defines classes: %s", strings.Join(classNames, ", ")))
			}
		}
		// Include key method names (critical for search - max 5 most relevant)
		if len(methodNames) > 0 {
			// Prioritize methods with meaningful names (not constructors, getters, etc)
			keyMethods := filterKeyMethods(methodNames)
			if len(keyMethods) > 0 {
				if len(keyMethods) > 5 {
					keyMethods = keyMethods[:5]
				}
				summary.WriteString(fmt.Sprintf(". Key methods: %s", strings.Join(keyMethods, ", ")))
			}
		}
		// Include key function names
		if len(funcNames) > 0 {
			keyFuncs := filterKeyMethods(funcNames)
			if len(keyFuncs) > 0 {
				if len(keyFuncs) > 5 {
					keyFuncs = keyFuncs[:5]
				}
				summary.WriteString(fmt.Sprintf(". Key functions: %s", strings.Join(keyFuncs, ", ")))
			}
		}
	}

	// Extract brief content preview (first meaningful sentence from content)
	content := elem.Content
	if idx := strings.Index(content, ". "); idx > 0 && idx < 200 {
		// Get first sentence if it's reasonable length
		firstSentence := content[:idx]
		// Check if it provides useful information beyond what we already have
		lowerSentence := strings.ToLower(firstSentence)
		if !strings.Contains(lowerSentence, "package "+strings.ToLower(elem.Name)) &&
			!strings.Contains(lowerSentence, "file: "+strings.ToLower(elem.Name)) {
			summary.WriteString(". " + firstSentence)
		}
	}

	result := summary.String()
	if len(result) > maxLen {
		result = result[:maxLen-3] + "..."
	}
	return result
}

// filterKeyMethods filters out generic method names (constructors, getters, setters)
// and returns only meaningful method names that are useful for search.
func filterKeyMethods(names []string) []string {
	// Patterns to exclude - these are too generic to be useful for search
	excludePatterns := []string{
		"constructor", "init", "__init__", "new",
		"get", "set", "is", "has",
		"toString", "valueOf", "equals", "hashCode",
		"toJSON", "fromJSON", "parse", "stringify",
		"copy", "clone", "close", "dispose",
		"ngOnInit", "ngOnDestroy", "componentDidMount", "componentWillUnmount",
		"render", "build", "create", "make",
	}

	var result []string
	for _, name := range names {
		nameLower := strings.ToLower(name)

		// Skip very short names (likely abbreviations or generic)
		if len(name) < 4 {
			continue
		}

		// Check if name matches any exclude pattern
		excluded := false
		for _, pattern := range excludePatterns {
			if nameLower == pattern || strings.HasPrefix(nameLower, pattern) {
				excluded = true
				break
			}
		}

		if !excluded {
			result = append(result, name)
		}
	}

	return result
}

func ingestBatch(client *http.Client, elements []CodeElement) (int, int, int) {
	items := make([]BatchIngestItem, 0, len(elements))
	timestamp := time.Now().UTC().Format(time.RFC3339)
	symbolCount := 0

	// Generate LLM summaries in batch if service is available
	var llmSummaries []string
	if summarizeSvc != nil {
		// Convert CodeElement to summarize.CodeElement
		sumElements := make([]summarize.CodeElement, len(elements))
		for i, elem := range elements {
			sumElements[i] = summarize.CodeElement{
				Name:     elem.Name,
				Kind:     elem.Kind,
				Path:     elem.Path,
				Content:  elem.Content,
				Package:  elem.Package,
				FilePath: elem.FilePath,
				Tags:     elem.Tags,
				Concerns: elem.Concerns,
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		llmSummaries = summarizeSvc.SummarizeBatch(ctx, sumElements)
		cancel()

		if *verbose && len(llmSummaries) > 0 {
			slog.Info("generated LLM summaries", "count", len(llmSummaries))
		}
	}

	for i, elem := range elements {
		// Generate structural summary
		structuralSummary := elem.Summary
		if structuralSummary == "" {
			structuralSummary = generateSummary(elem)
		}

		// Combine with LLM semantic summary if available
		finalSummary := structuralSummary
		if i < len(llmSummaries) && llmSummaries[i] != "" {
			// Check if LLM summary is different from structural (not just fallback)
			if llmSummaries[i] != structuralSummary {
				finalSummary = summarize.CombineSummary(structuralSummary, llmSummaries[i])
				if *verbose {
					slog.Info("LLM summary", "name", elem.Name, "summary", llmSummaries[i])
				}
			}
		}

		item := BatchIngestItem{
			Timestamp: timestamp,
			Source:    "codebase-ingest",
			Name:      elem.Name,
			Path:      elem.Path,
			Content:   elem.Content,
			Summary:   finalSummary,
			Tags:      elem.Tags,
		}
		// Include symbols if extraction is enabled
		if *extractSymbols && len(elem.Symbols) > 0 {
			item.Symbols = elem.Symbols
			symbolCount += len(elem.Symbols)
			if *verbose {
				slog.Info("symbols extracted", "name", elem.Name, "count", len(elem.Symbols))
			}
		}
		items = append(items, item)
	}

	req := BatchIngestRequest{
		SpaceID:      *spaceID,
		Observations: items,
	}

	body, _ := json.Marshal(req)

	// Retry loop with exponential backoff
	var lastErr error
	retryDelayMs := *retryDelay

	for attempt := 0; attempt <= *maxRetries; attempt++ {
		if attempt > 0 {
			slog.Warn("retrying batch ingest", "attempt", attempt, "max_retries", *maxRetries, "delay_ms", retryDelayMs)
			time.Sleep(time.Duration(retryDelayMs) * time.Millisecond)
			retryDelayMs *= 2 // Exponential backoff
		}

		resp, err := client.Post(*mdemgEndpoint+"/v1/memory/ingest/batch", "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			if *verbose {
				slog.Error("batch ingest request failed", "attempt", attempt+1, "error", err)
			}
			continue
		}

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusMultiStatus {
			var result struct {
				SuccessCount int `json:"success_count"`
				ErrorCount   int `json:"error_count"`
			}
			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()
			return result.SuccessCount, result.ErrorCount, symbolCount
		}

		// Non-retryable error (bad request, etc.)
		if resp.StatusCode == http.StatusBadRequest {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			slog.Error("batch rejected, non-retryable", "status", resp.StatusCode, "body", string(bodyBytes))
			return 0, len(elements), 0
		}

		// Retryable server error
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
		if *verbose {
			slog.Error("batch ingest failed", "attempt", attempt+1, "error", lastErr)
		}
	}

	slog.Error("batch failed after retries", "retries", *maxRetries, "error", lastErr)
	return 0, len(elements), 0
}

func runConsolidation(client *http.Client) error {
	req := map[string]string{"space_id": *spaceID}
	body, _ := json.Marshal(req)

	resp, err := client.Post(*mdemgEndpoint+"/v1/memory/consolidate", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			HiddenNodesCreated  int     `json:"hidden_nodes_created"`
			HiddenNodesUpdated  int     `json:"hidden_nodes_updated"`
			ConceptNodesUpdated int     `json:"concept_nodes_updated"`
			DurationMs          float64 `json:"duration_ms"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	slog.Info("consolidation complete", "created", result.Data.HiddenNodesCreated, "updated", result.Data.HiddenNodesUpdated, "concept", result.Data.ConceptNodesUpdated, "duration_ms", result.Data.DurationMs)

	return nil
}

func printSample(elements []CodeElement) {
	if *quiet {
		return
	}

	counts := make(map[string]int)
	for _, e := range elements {
		counts[e.Kind]++
	}

	slog.Info("element breakdown")
	for kind, count := range counts {
		slog.Info("element kind", "kind", kind, "count", count)
	}

	slog.Info("sample elements")
	shown := 0
	for _, e := range elements {
		if shown >= 20 && !*verbose {
			break
		}
		fmt.Printf("  [%s] %s (%s)\n", e.Kind, e.Name, e.FilePath)
		shown++
	}

	if len(elements) > 20 && !*verbose {
		fmt.Printf("  ... and %d more (use --verbose to see all)\n", len(elements)-20)
	}
}
