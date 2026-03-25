// Package cli provides Cobra command wrappers for MDEMG operations.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	"github.com/spf13/cobra"
	"mdemg/internal/config"
	"mdemg/internal/languages"
	"mdemg/internal/summarize"
)

// newIngestCmd creates the codebase ingestion command
func newIngestCmd() *cobra.Command {
	cfg := &ingestConfig{}

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest a codebase into MDEMG",
		Long:  "Walks a codebase and ingests files into MDEMG with optimized batch processing and configurable timeouts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve space ID: explicit flag > global flag > env var > default
			if resolved := resolveSpaceID(cmd); resolved != "" {
				cfg.spaceID = resolved
			}

			// Resolve speed preset: CLI flag > INGEST_SPEED env > empty (preserve defaults)
			if cfg.speed == "" {
				cfg.speed = os.Getenv("INGEST_SPEED")
			}
			if cfg.speed != "" {
				sp, ok := speedPresets[cfg.speed]
				if !ok {
					return fmt.Errorf("unknown speed preset: %s (available: fast, balanced, thorough)", cfg.speed)
				}
				// Apply speed preset values only for flags not explicitly set by the user
				if !cmd.Flags().Changed("workers") {
					cfg.workers = sp.workers
				}
				if !cmd.Flags().Changed("batch") {
					cfg.batchSize = sp.batchSize
				}
				if !cmd.Flags().Changed("llm-summary") {
					cfg.llmSummary = sp.llmSummary
				}
				if !cmd.Flags().Changed("llm-summary-batch") {
					cfg.llmSummaryBatch = sp.llmBatch
				}
				if !cmd.Flags().Changed("extract-symbols") {
					cfg.extractSymbols = sp.symbols
				}
				if !cmd.Flags().Changed("consolidate") {
					cfg.consolidate = sp.consolidate
				}
				if !cmd.Flags().Changed("delay") {
					cfg.delay = sp.delay
				}
			}

			return runIngest(cfg)
		},
	}

	// Core flags
	cmd.Flags().StringVar(&cfg.codebasePath, "path", "", "Path to codebase to ingest (required)")
	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "codebase", "MDEMG space ID")
	cmd.Flags().StringVar(&cfg.mdemgEndpoint, "endpoint", "", "MDEMG endpoint (default: from LISTEN_ADDR in .env)")
	cmd.Flags().IntVar(&cfg.batchSize, "batch", 100, "Batch size for ingestion (default: 100, optimal for ~15/s per worker)")
	cmd.Flags().IntVar(&cfg.workers, "workers", 4, "Number of parallel workers")
	cmd.Flags().IntVar(&cfg.timeout, "timeout", 300, "HTTP timeout in seconds")
	cmd.Flags().IntVar(&cfg.delay, "delay", 50, "Delay between batches in ms")
	cmd.Flags().IntVar(&cfg.maxRetries, "retries", 3, "Max retries per batch on failure")
	cmd.Flags().IntVar(&cfg.retryDelay, "retry-delay", 2000, "Initial retry delay in ms, doubles each retry")
	cmd.Flags().BoolVar(&cfg.consolidate, "consolidate", true, "Run consolidation after ingestion")
	cmd.Flags().BoolVar(&cfg.dryRun, "dry-run", false, "Print what would be ingested without actually doing it")
	cmd.Flags().BoolVar(&cfg.verbose, "verbose", false, "Verbose output")

	// Filter flags
	cmd.Flags().StringVar(&cfg.excludeDirs, "exclude", ".git,vendor,node_modules,.worktrees", "Comma-separated directories to exclude")
	cmd.Flags().BoolVar(&cfg.includeTests, "include-tests", false, "Include test files (*_test.go, *.test.ts, *.spec.ts)")
	cmd.Flags().BoolVar(&cfg.includeMd, "include-md", true, "Include markdown files (*.md)")
	cmd.Flags().BoolVar(&cfg.includeTS, "include-ts", true, "Include TypeScript/JavaScript files (*.ts, *.tsx, *.js, *.jsx)")
	cmd.Flags().BoolVar(&cfg.includePy, "include-py", true, "Include Python files (*.py)")
	cmd.Flags().BoolVar(&cfg.includeJava, "include-java", true, "Include Java files (*.java)")
	cmd.Flags().BoolVar(&cfg.includeRust, "include-rust", true, "Include Rust files (*.rs)")
	cmd.Flags().IntVar(&cfg.limitElements, "limit", 0, "Limit number of elements to ingest (0 = no limit)")
	cmd.Flags().BoolVar(&cfg.extractSymbols, "extract-symbols", true, "Extract code symbols (constants, functions, classes) for evidence-locked retrieval")

	// Incremental flags
	cmd.Flags().BoolVar(&cfg.incremental, "incremental", false, "Only ingest files changed since last commit (uses git diff)")
	cmd.Flags().StringVar(&cfg.sinceCommit, "since", "HEAD~1", "Git commit to compare against for incremental mode")
	cmd.Flags().BoolVar(&cfg.archiveDeleted, "archive-deleted", true, "Archive nodes for deleted files in incremental mode")

	// Output flags
	cmd.Flags().BoolVar(&cfg.quiet, "quiet", false, "Suppress all non-error output")
	cmd.Flags().StringVar(&cfg.logFile, "log-file", "", "Write logs to file instead of stderr")
	cmd.Flags().BoolVar(&cfg.progressJSON, "progress-json", false, "Emit structured JSON progress lines to stdout (logs go to stderr)")

	// LLM summary flags
	cmd.Flags().BoolVar(&cfg.llmSummary, "llm-summary", true, "Use LLM to generate semantic summaries (requires OPENAI_API_KEY)")
	cmd.Flags().StringVar(&cfg.llmSummaryModel, "llm-summary-model", "gpt-4o-mini", "Model for LLM summaries")
	cmd.Flags().IntVar(&cfg.llmSummaryBatch, "llm-summary-batch", 10, "Files per LLM API call for summaries")
	cmd.Flags().StringVar(&cfg.llmSummaryProvider, "llm-summary-provider", "openai", "LLM provider for summaries (openai/ollama)")

	// Performance guards
	cmd.Flags().IntVar(&cfg.maxFileSize, "max-file-size", 1048576, "Max file size in bytes to process (default: 1MB)")
	cmd.Flags().IntVar(&cfg.maxElementsPerFile, "max-elements-per-file", 500, "Max elements to extract per file")
	cmd.Flags().IntVar(&cfg.maxSymbolsPerFile, "max-symbols-per-file", 1000, "Max symbols to extract per file")
	cmd.Flags().StringVar(&cfg.preset, "preset", "", "Exclusion preset: default, ml_cuda, web_monorepo")
	cmd.Flags().StringVar(&cfg.speed, "speed", "", "Speed preset: fast, balanced, thorough (default: balanced behavior)")

	// Info flags
	cmd.Flags().BoolVar(&cfg.listLanguages, "list-languages", false, "List supported languages and exit")

	_ = cmd.MarkFlagRequired("path")

	return cmd
}

type ingestConfig struct {
	// Core settings
	codebasePath  string
	spaceID       string
	mdemgEndpoint string
	batchSize     int
	workers       int
	timeout       int
	delay         int
	maxRetries    int
	retryDelay    int
	consolidate   bool
	dryRun        bool
	verbose       bool

	// Filters
	excludeDirs    string
	includeTests   bool
	includeMd      bool
	includeTS      bool
	includePy      bool
	includeJava    bool
	includeRust    bool
	limitElements  int
	extractSymbols bool

	// Incremental
	incremental    bool
	sinceCommit    string
	archiveDeleted bool

	// Output
	quiet        bool
	logFile      string
	progressJSON bool

	// LLM summary
	llmSummary         bool
	llmSummaryModel    string
	llmSummaryBatch    int
	llmSummaryProvider string

	// Performance guards
	maxFileSize        int
	maxElementsPerFile int
	maxSymbolsPerFile  int
	preset             string
	speed              string // Speed preset: fast, balanced, thorough

	// Info
	listLanguages bool

	// Plugin bridge (optional, nil for CLI-only ingest)
	plugins pluginParser
}

type batchIngestRequest struct {
	SpaceID      string            `json:"space_id"`
	Observations []batchIngestItem `json:"observations"`
}

type batchIngestItem struct {
	Timestamp string         `json:"timestamp"`
	Source    string         `json:"source"`
	Name      string         `json:"name"`
	Path      string         `json:"path"`
	Content   string         `json:"content"`
	Summary   string         `json:"summary,omitempty"`
	Tags      []string       `json:"tags"`
	Symbols   []ingestSymbol `json:"symbols,omitempty"`
}

type ingestSymbol struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Line           int    `json:"line"`
	LineEnd        int    `json:"line_end,omitempty"`
	Exported       bool   `json:"exported"`
	Parent         string `json:"parent,omitempty"`
	Signature      string `json:"signature,omitempty"`
	Value          string `json:"value,omitempty"`
	RawValue       string `json:"raw_value,omitempty"`
	DocComment     string `json:"doc_comment,omitempty"`
	TypeAnnotation string `json:"type_annotation,omitempty"`
	Language       string `json:"language,omitempty"`
}

type codeElement struct {
	Name     string
	Kind     string
	Path     string
	Content  string
	Summary  string
	Package  string
	FilePath string
	Tags     []string
	Concerns []string
	Symbols  []ingestSymbol
}

// pluginParser is an optional interface for parsing files via plugin modules.
// When set on ingestConfig, walkCodebase delegates unrecognized file types to
// the plugin's MatchAndParse method before skipping them.
type pluginParser interface {
	// MatchAndParse attempts to parse a file via registered plugin modules.
	// Returns parsed elements and true if a module handled the file, or nil
	// and false if no module matched.
	MatchAndParse(ctx context.Context, filePath string, content []byte) ([]codeElement, bool, error)
}

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

// speedPreset controls performance-related settings for ingestion.
// These are orthogonal to exclusion presets (--preset) which control file filtering.
type speedPreset struct {
	workers     int
	batchSize   int
	llmSummary  bool
	llmBatch    int
	symbols     bool
	consolidate bool
	delay       int
}

// speedPresets maps speed preset names to their configurations.
var speedPresets = map[string]speedPreset{
	"fast":     {workers: 8, batchSize: 250, llmSummary: false, llmBatch: 0, symbols: false, consolidate: false, delay: 0},
	"balanced": {workers: 4, batchSize: 100, llmSummary: true, llmBatch: 10, symbols: true, consolidate: true, delay: 50},
	"thorough": {workers: 8, batchSize: 200, llmSummary: true, llmBatch: 20, symbols: true, consolidate: true, delay: 25},
}

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

var configFilePatterns = []string{
	".config.ts", ".config.js", ".config.json", ".config.yaml", ".config.yml",
	"config.ts", "config.js", "config.json", "config.yaml", "config.yml",
	".env.example", ".env.sample", ".env.template",
	"tsconfig.json", "package.json", "nest-cli.json", "angular.json",
	"webpack.config", "vite.config", "rollup.config", "jest.config",
	"pyproject.toml", "setup.cfg", "setup.py", "requirements.txt",
	"go.mod", "go.sum", "Makefile", "Dockerfile", "docker-compose",
}

var configDirPatterns = []string{
	"/config/", "/configuration/", "/configs/", "/settings/",
	"/env/", "/environments/",
}

type ingestStats struct {
	TotalElements int64
	Ingested      int64
	Errors        int64
	Symbols       int64
	StartTime     time.Time
}

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

type gitDiffResult struct {
	Added    []string
	Modified []string
	Deleted  []string
	Renamed  map[string]string
}

func runIngest(cfg *ingestConfig) error {
	// Log output priority
	if cfg.logFile != "" {
		f, err := os.OpenFile(cfg.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return fmt.Errorf("failed to open log file %s: %v", cfg.logFile, err)
		}
		defer f.Close()
		log.SetOutput(f)
	} else if cfg.quiet {
		log.SetOutput(io.Discard)
	} else if cfg.progressJSON {
		log.SetOutput(os.Stderr)
	}

	// Handle --list-languages
	if cfg.listLanguages {
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
		return nil
	}

	// Load config (YAML → .env → env vars)
	if cfgPath := config.FindConfigFile(); cfgPath != "" {
		_ = config.LoadYAMLConfig(cfgPath)
	}
	if err := godotenv.Load(); err != nil {
		log.Printf("Note: No .env file found, using defaults/flags")
	}

	// Resolve endpoint
	if cfg.mdemgEndpoint == "" {
		cfg.mdemgEndpoint = config.ResolveEndpoint("http://localhost:9999")
		log.Printf("Resolved endpoint: %s", cfg.mdemgEndpoint)
	}

	if cfg.codebasePath == "" {
		return fmt.Errorf("--path is required")
	}

	// Load .mdemgignore patterns
	var ignorePatterns []config.IgnorePattern
	if ignPath := config.FindIgnoreFile(cfg.codebasePath); ignPath != "" {
		var err error
		ignorePatterns, err = config.ParseIgnoreFile(ignPath)
		if err != nil {
			log.Printf("Warning: failed to parse %s: %v", ignPath, err)
		} else {
			log.Printf("Loaded %d ignore patterns from %s", len(ignorePatterns), ignPath)
		}
	}

	// Apply preset
	excludeSet := make(map[string]bool)
	var excludePatterns []string
	if cfg.preset != "" {
		if p, ok := presets[cfg.preset]; ok {
			for _, dir := range p.excludeDirs {
				excludeSet[dir] = true
			}
			excludePatterns = p.excludePatterns
			if cfg.maxFileSize == 1048576 {
				cfg.maxFileSize = p.maxFileSize
			}
			log.Printf("Applied preset: %s", cfg.preset)
		} else {
			return fmt.Errorf("unknown preset: %s (available: default, ml_cuda, web_monorepo)", cfg.preset)
		}
	}
	for _, dir := range strings.Split(cfg.excludeDirs, ",") {
		excludeSet[strings.TrimSpace(dir)] = true
	}

	log.Printf("=== MDEMG Codebase Ingestion ===")
	log.Printf("Path: %s", cfg.codebasePath)
	log.Printf("Space ID: %s", cfg.spaceID)
	log.Printf("Endpoint: %s", cfg.mdemgEndpoint)
	log.Printf("Batch size: %d, Workers: %d, Timeout: %ds", cfg.batchSize, cfg.workers, cfg.timeout)
	log.Printf("Excluded dirs: %v", getExcludeDirList(excludeSet))
	if len(excludePatterns) > 0 {
		log.Printf("Excluded patterns: %v", excludePatterns)
	}
	log.Printf("Max file size: %d bytes", cfg.maxFileSize)
	log.Printf("Max elements/file: %d, Max symbols/file: %d", cfg.maxElementsPerFile, cfg.maxSymbolsPerFile)
	log.Printf("Symbol extraction: %v", cfg.extractSymbols)
	log.Printf("Incremental mode: %v", cfg.incremental)
	log.Printf("LLM summaries: %v", cfg.llmSummary)
	if cfg.speed != "" {
		log.Printf("Speed preset: %s", cfg.speed)
	}

	// Initialize LLM summarize service
	var summarizeSvc *summarize.Service
	if cfg.llmSummary {
		apiKey := os.Getenv("OPENAI_API_KEY")
		ollamaEndpoint := os.Getenv("OLLAMA_ENDPOINT")
		if ollamaEndpoint == "" {
			ollamaEndpoint = "http://localhost:11434"
		}

		if cfg.llmSummaryProvider == "openai" && apiKey == "" {
			log.Println("Warning: LLM summaries enabled but OPENAI_API_KEY not set. Using structural fallback.")
		} else {
			sumCfg := summarize.Config{
				Enabled:        true,
				Provider:       cfg.llmSummaryProvider,
				Model:          cfg.llmSummaryModel,
				MaxTokens:      150,
				BatchSize:      cfg.llmSummaryBatch,
				TimeoutMs:      30000,
				CacheEnabled:   true,
				CacheSize:      5000,
				Debug:          cfg.verbose,
				OpenAIAPIKey:   apiKey,
				OpenAIEndpoint: os.Getenv("OPENAI_ENDPOINT"),
				OllamaEndpoint: ollamaEndpoint,
			}
			if sumCfg.OpenAIEndpoint == "" {
				sumCfg.OpenAIEndpoint = "https://api.openai.com/v1"
			}

			var err error
			summarizeSvc, err = summarize.New(sumCfg, generateSummaryAdapter)
			if err != nil {
				log.Printf("Warning: Failed to initialize LLM summarize service: %v. Using structural fallback.", err)
			} else {
				log.Printf("LLM summarize service initialized: provider=%s, model=%s, batch=%d",
					cfg.llmSummaryProvider, cfg.llmSummaryModel, cfg.llmSummaryBatch)
			}
		}
	}

	// Handle incremental mode
	var changedFiles *gitDiffResult
	if cfg.incremental {
		log.Printf("Getting changed files since %s...", cfg.sinceCommit)
		var err error
		changedFiles, err = getGitChangedFiles(cfg.codebasePath, cfg.sinceCommit)
		if err != nil {
			return fmt.Errorf("failed to get git diff: %v", err)
		}
		log.Printf("Changed files: %d added, %d modified, %d deleted, %d renamed",
			len(changedFiles.Added), len(changedFiles.Modified),
			len(changedFiles.Deleted), len(changedFiles.Renamed))

		if len(changedFiles.Added)+len(changedFiles.Modified) == 0 && len(changedFiles.Deleted) == 0 {
			log.Println("No changes detected. Exiting.")
			return nil
		}
	}

	diagSummary := languages.NewDiagnosticSummary()

	elements, err := walkCodebase(cfg, excludeSet, excludePatterns, ignorePatterns, diagSummary)
	if err != nil {
		return fmt.Errorf("failed to walk codebase: %v", err)
	}

	// Filter to changed files
	if cfg.incremental && changedFiles != nil {
		changedSet := make(map[string]bool)
		for _, f := range changedFiles.Added {
			changedSet[f] = true
		}
		for _, f := range changedFiles.Modified {
			changedSet[f] = true
		}

		var filtered []codeElement
		for _, elem := range elements {
			relPath := strings.TrimPrefix(elem.FilePath, "/")
			if changedSet[relPath] || changedSet[elem.FilePath] {
				filtered = append(filtered, elem)
			}
		}
		log.Printf("Filtered from %d to %d elements (changed files only)", len(elements), len(filtered))
		elements = filtered
	}

	log.Printf("Found %d code elements", len(elements))
	emitProgress(cfg, progressEvent{Event: "discovery_complete", Total: len(elements)})

	if diagSummary.Total > 0 {
		log.Printf("Diagnostics: %d total (info=%d, warning=%d, error=%d)",
			diagSummary.Total,
			diagSummary.BySev["info"],
			diagSummary.BySev["warning"],
			diagSummary.BySev["error"])
		if cfg.verbose {
			for code, count := range diagSummary.ByCode {
				log.Printf("  %s: %d", code, count)
			}
		}
	}

	if cfg.limitElements > 0 && len(elements) > cfg.limitElements {
		log.Printf("Limiting to first %d elements (from %d)", cfg.limitElements, len(elements))
		elements = elements[:cfg.limitElements]
	}

	if cfg.dryRun {
		log.Println("Dry run - not ingesting")
		printSample(cfg, elements)
		return nil
	}

	client := &http.Client{
		Timeout: time.Duration(cfg.timeout) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	stats := &ingestStats{
		TotalElements: int64(len(elements)),
		StartTime:     time.Now(),
	}

	var batches [][]codeElement
	for i := 0; i < len(elements); i += cfg.batchSize {
		end := i + cfg.batchSize
		if end > len(elements) {
			end = len(elements)
		}
		batches = append(batches, elements[i:end])
	}

	log.Printf("Processing %d batches with %d workers", len(batches), cfg.workers)

	batchChan := make(chan []codeElement, len(batches))
	var wg sync.WaitGroup

	for w := 0; w < cfg.workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for batch := range batchChan {
				ingested, errors, syms := ingestBatch(cfg, client, batch, summarizeSvc)
				atomic.AddInt64(&stats.Ingested, int64(ingested))
				atomic.AddInt64(&stats.Errors, int64(errors))
				atomic.AddInt64(&stats.Symbols, int64(syms))

				current := atomic.LoadInt64(&stats.Ingested)
				errCount := atomic.LoadInt64(&stats.Errors)
				elapsed := time.Since(stats.StartTime)
				rate := float64(current) / elapsed.Seconds()

				log.Printf("[Worker %d] Progress: %d/%d (%.1f/s, %d errors)",
					workerID, current, stats.TotalElements, rate, errCount)

				emitProgress(cfg, progressEvent{
					Event:   "batch_progress",
					Current: int(current),
					Total:   int(stats.TotalElements),
					Rate:    rate,
				})

				time.Sleep(time.Duration(cfg.delay) * time.Millisecond)
			}
		}(w)
	}

	for _, batch := range batches {
		batchChan <- batch
	}
	close(batchChan)

	wg.Wait()

	elapsed := time.Since(stats.StartTime)
	log.Printf("=== Ingestion Complete ===")
	log.Printf("Total: %d, Ingested: %d, Errors: %d", stats.TotalElements, stats.Ingested, stats.Errors)
	log.Printf("Symbols: %d", stats.Symbols)
	log.Printf("Time: %v, Rate: %.1f elements/sec", elapsed, float64(stats.Ingested)/elapsed.Seconds())

	if summarizeSvc != nil {
		totalCalls, cacheHits, cacheSize := summarizeSvc.Stats()
		log.Printf("LLM Summary stats: calls=%d, cache_hits=%d, cache_size=%d",
			totalCalls, cacheHits, cacheSize)
	}

	if cfg.incremental && cfg.archiveDeleted && changedFiles != nil && len(changedFiles.Deleted) > 0 {
		log.Printf("Archiving %d deleted files...", len(changedFiles.Deleted))
		archived, err := archiveDeletedNodes(client, cfg.mdemgEndpoint, cfg.spaceID, changedFiles.Deleted)
		if err != nil {
			log.Printf("Warning: archive failed: %v", err)
		} else {
			log.Printf("Archived nodes for %d deleted files", archived)
		}
	}

	emitProgress(cfg, progressEvent{
		Event:    "complete",
		Total:    int(stats.TotalElements),
		Ingested: int(stats.Ingested),
		Errors:   int(stats.Errors),
		Duration: elapsed.Round(time.Second).String(),
	})

	if cfg.consolidate && stats.Ingested > 0 {
		emitProgress(cfg, progressEvent{Event: "consolidation_start"})
		log.Println("Running consolidation...")
		if err := runConsolidation(client, cfg.mdemgEndpoint, cfg.spaceID); err != nil {
			log.Printf("Consolidation failed: %v", err)
		}
	}

	return nil
}

func emitProgress(cfg *ingestConfig, evt progressEvent) {
	if !cfg.progressJSON {
		return
	}
	data, _ := json.Marshal(evt)
	fmt.Fprintln(os.Stdout, string(data))
}

func getGitChangedFiles(repoPath, sinceCommit string) (*gitDiffResult, error) {
	result := &gitDiffResult{
		Renamed: make(map[string]string),
	}

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
			if len(parts) >= 3 {
				result.Renamed[parts[1]] = parts[2]
				result.Modified = append(result.Modified, parts[2])
			}
		}
	}

	return result, nil
}

func archiveDeletedNodes(client *http.Client, endpoint, spaceID string, deletedPaths []string) (int, error) {
	if len(deletedPaths) == 0 {
		return 0, nil
	}

	archived := 0
	for _, path := range deletedPaths {
		fullPath := "/" + path

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
			log.Printf("Warning: failed to archive nodes for %s: %v", path, err)
			continue
		}
		resp.Body.Close()
		archived++
	}

	return archived, nil
}

func getExcludeDirList(excludeSet map[string]bool) []string {
	var dirs []string
	for dir := range excludeSet {
		dirs = append(dirs, dir)
	}
	return dirs
}

func matchesExcludePattern(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
	}
	return false
}

func isConfigFile(filePath string) bool {
	lowerPath := strings.ToLower(filePath)

	for _, pattern := range configDirPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}

	for _, pattern := range configFilePatterns {
		if strings.HasSuffix(lowerPath, pattern) || strings.Contains(filepath.Base(lowerPath), pattern) {
			return true
		}
	}

	base := filepath.Base(filePath)
	baseLower := strings.ToLower(base)
	if strings.Contains(baseLower, "config") && !strings.Contains(baseLower, "test") {
		return true
	}

	return false
}

func convertLanguageElement(elem languages.CodeElement) codeElement {
	var symbols []ingestSymbol
	for _, s := range elem.Symbols {
		symbols = append(symbols, ingestSymbol{
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

	return codeElement{
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

func getEnabledLanguages(cfg *ingestConfig) map[string]bool {
	return map[string]bool{
		"go":         true,
		"rust":       cfg.includeRust,
		"python":     cfg.includePy,
		"typescript": cfg.includeTS,
		"java":       cfg.includeJava,
		"markdown":   cfg.includeMd,
		"json":       true,
		"sql":        true,
		"xml":        true,
		"c":          true,
		"cpp":        true,
		"yaml":       true,
		"toml":       true,
		"ini":        true,
		"dockerfile": true,
		"shell":      true,
		"cuda":       true,
		"cypher":     true,
		"csharp":     true,
		"kotlin":     true,
		"terraform":  true,
		"makefile":   true,
	}
}

func walkCodebase(cfg *ingestConfig, excludeSet map[string]bool, excludePatterns []string, ignorePatterns []config.IgnorePattern, diagSummary *languages.DiagnosticSummary) ([]codeElement, error) {
	var elements []codeElement
	enabledLangs := getEnabledLanguages(cfg)

	err := filepath.Walk(cfg.codebasePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Compute relative path for .mdemgignore matching
		relPath, _ := filepath.Rel(cfg.codebasePath, path)

		if info.IsDir() {
			if excludeSet[info.Name()] {
				if cfg.verbose {
					log.Printf("Skipping excluded directory: %s", path)
				}
				return filepath.SkipDir
			}
			// Check .mdemgignore patterns for directories
			if len(ignorePatterns) > 0 && relPath != "." && config.MatchesIgnorePatterns(relPath, true, ignorePatterns) {
				if cfg.verbose {
					log.Printf("Skipping directory (mdemgignore): %s", path)
				}
				return filepath.SkipDir
			}
			return nil
		}

		// Check .mdemgignore patterns for files
		if len(ignorePatterns) > 0 && config.MatchesIgnorePatterns(relPath, false, ignorePatterns) {
			if cfg.verbose {
				log.Printf("Skipping file (mdemgignore): %s", path)
			}
			return nil
		}

		if cfg.maxFileSize > 0 && info.Size() > int64(cfg.maxFileSize) {
			if cfg.verbose {
				log.Printf("Skipping oversized file (%d bytes > %d max): %s", info.Size(), cfg.maxFileSize, path)
			}
			diagSummary.Add(languages.Diagnostic{
				Severity: "warning",
				Code:     "LARGE_FILE",
				Message:  fmt.Sprintf("File exceeds size threshold (%d bytes > %d max)", info.Size(), cfg.maxFileSize),
				Context:  map[string]string{"path": path},
			})
			return nil
		}

		if matchesExcludePattern(info.Name(), excludePatterns) {
			if cfg.verbose {
				log.Printf("Skipping file matching exclude pattern: %s", path)
			}
			return nil
		}

		parser, ok := languages.GetParserForFile(path)
		if ok {
			if !enabledLangs[parser.Name()] {
				return nil
			}

			if !cfg.includeTests && parser.IsTestFile(path) {
				return nil
			}

			if parser.Name() == "json" {
				if !isConfigFile(path) {
					return nil
				}
			}

			langElements, parseErr := parser.ParseFile(cfg.codebasePath, path, cfg.extractSymbols)
			if parseErr != nil {
				if cfg.verbose {
					log.Printf("Parse error for %s: %v", path, parseErr)
				}
				return nil
			}

			if cfg.maxElementsPerFile > 0 && len(langElements) > cfg.maxElementsPerFile {
				if cfg.verbose {
					log.Printf("Capping elements for %s: %d → %d", path, len(langElements), cfg.maxElementsPerFile)
				}
				langElements = langElements[:cfg.maxElementsPerFile]
			}

			for _, elem := range langElements {
				for _, d := range elem.Diagnostics {
					diagSummary.Add(d)
					if cfg.verbose {
						log.Printf("  [diag] %s/%s: %s (%s)", d.Severity, d.Code, d.Message, path)
					}
				}
			}

			for _, elem := range langElements {
				converted := convertLanguageElement(elem)
				if cfg.maxSymbolsPerFile > 0 && len(converted.Symbols) > cfg.maxSymbolsPerFile {
					if cfg.verbose {
						log.Printf("Capping symbols for %s: %d → %d", path, len(converted.Symbols), cfg.maxSymbolsPerFile)
					}
					converted.Symbols = converted.Symbols[:cfg.maxSymbolsPerFile]
				}
				elements = append(elements, converted)
			}
			return nil
		}

		if strings.Contains(filepath.Base(path), ".env.") && !strings.HasSuffix(path, ".env") {
			envElement := parseEnvFile(cfg.codebasePath, path)
			if envElement != nil {
				elements = append(elements, *envElement)
			}
			return nil
		}

		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			if isConfigFile(path) {
				configElement := parseConfigFile(cfg.codebasePath, path)
				if configElement != nil {
					elements = append(elements, *configElement)
				}
			}
			return nil
		}

		// GAP-01: Delegate unrecognized file types to plugin modules before skipping.
		ext := filepath.Ext(path)

		if cfg.plugins != nil {
			content, readErr := os.ReadFile(path) //nolint:gosec // G122: path is from walkCodebase's own WalkDir — same trust boundary as built-in parsers
			if readErr == nil {
				pluginElems, matched, matchErr := cfg.plugins.MatchAndParse(context.Background(), path, content)
				if matchErr != nil && cfg.verbose {
					log.Printf("Plugin match error for %s: %v", path, matchErr)
				}
				if matched && len(pluginElems) > 0 {
					elements = append(elements, pluginElems...)
					if cfg.verbose {
						log.Printf("Plugin parsed %d elements from %s", len(pluginElems), path)
					}
					return nil
				}
			}
		}

		// No built-in parser and no plugin match — emit diagnostic and skip.
		if ext != "" && cfg.verbose {
			log.Printf("Skipping unrecognized file type (%s): %s", ext, path)
		}
		diagSummary.Add(languages.Diagnostic{
			Severity: "info",
			Code:     "UNRECOGNIZED_FILE_TYPE",
			Message:  fmt.Sprintf("No built-in parser for file extension %q; file skipped", ext),
			Context:  map[string]string{"path": path, "extension": ext},
		})
		return nil
	})

	return elements, err
}

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

func detectConcerns(filePath, content string) []string {
	detected := make(map[string]bool)
	lowerPath := strings.ToLower(filePath)
	lowerContent := strings.ToLower(content)

	for concern, patterns := range concernPatterns {
		for _, pattern := range patterns {
			if strings.Contains(lowerPath, pattern) {
				detected[concern] = true
				break
			}
		}
	}

	for decorator, concern := range decoratorConcerns {
		if strings.Contains(content, decorator) {
			detected[concern] = true
		}
	}

	for concern, patterns := range concernPatterns {
		if detected[concern] {
			continue
		}
		matchCount := 0
		for _, pattern := range patterns {
			if strings.Count(lowerContent, pattern) >= 2 {
				matchCount++
			}
		}
		if matchCount >= 2 {
			detected[concern] = true
		}
	}

	var concerns []string
	for concern := range detected {
		concerns = append(concerns, "concern:"+concern)
	}

	uiTags := detectUIPatterns(filePath, content)
	concerns = append(concerns, uiTags...)

	return concerns
}

func detectUIPatterns(filePath, content string) []string {
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

	for uiType, patterns := range uiPatterns {
		matchCount := 0
		for _, pattern := range patterns {
			if strings.Contains(lowerContent, pattern) {
				matchCount++
			}
		}
		if matchCount >= 1 {
			detected[uiType] = true
		}
	}

	var uiTags []string
	for uiType := range detected {
		uiTags = append(uiTags, "ui:"+uiType)
	}
	return uiTags
}

func parseConfigFile(root, path string) *codeElement {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(root, path)
	name := filepath.Base(path)
	contentStr := string(content)

	if len(contentStr) > 4000 {
		contentStr = contentStr[:4000] + "... [truncated]"
	}

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Configuration file: %s. ", name))

	if strings.HasSuffix(path, ".json") {
		keys := findAllMatches(contentStr, `"(\w+)":\s*[{\[\"]`)
		if len(keys) > 0 {
			if len(keys) > 10 {
				keys = keys[:10]
			}
			summary.WriteString(fmt.Sprintf("Configuration keys: %s. ", strings.Join(uniqueStrings(keys), ", ")))
		}
	}

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

	return &codeElement{
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

func parseEnvFile(root, path string) *codeElement {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(root, path)
	name := filepath.Base(path)
	contentStr := string(content)

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

	return &codeElement{
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

func generateSummaryAdapter(elem summarize.CodeElement) string {
	return generateSummary(codeElement{
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

// titleCase capitalizes the first letter of a string.
func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func generateSummary(elem codeElement) string {
	var summary strings.Builder
	maxLen := 700

	switch elem.Kind {
	case "package", "config":
		summary.WriteString(fmt.Sprintf("%s: %s", titleCase(elem.Kind), elem.Name))
	case "function", "method":
		summary.WriteString(fmt.Sprintf("%s %s", titleCase(elem.Kind), elem.Name))
	case "struct", "interface", "type":
		summary.WriteString(fmt.Sprintf("%s %s", titleCase(elem.Kind), elem.Name))
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

	if elem.Package != "" && elem.Package != elem.Name {
		summary.WriteString(fmt.Sprintf(" in %s", elem.Package))
	}

	if len(elem.Concerns) > 0 {
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
		if len(classNames) > 0 {
			if len(classNames) > 3 {
				summary.WriteString(fmt.Sprintf(". Defines classes: %s, and %d more", strings.Join(classNames[:3], ", "), len(classNames)-3))
			} else {
				summary.WriteString(fmt.Sprintf(". Defines classes: %s", strings.Join(classNames, ", ")))
			}
		}
		if len(methodNames) > 0 {
			keyMethods := filterKeyMethods(methodNames)
			if len(keyMethods) > 0 {
				if len(keyMethods) > 5 {
					keyMethods = keyMethods[:5]
				}
				summary.WriteString(fmt.Sprintf(". Key methods: %s", strings.Join(keyMethods, ", ")))
			}
		}
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

	content := elem.Content
	if idx := strings.Index(content, ". "); idx > 0 && idx < 200 {
		firstSentence := content[:idx]
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

func filterKeyMethods(names []string) []string {
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

		if len(name) < 4 {
			continue
		}

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

func ingestBatch(cfg *ingestConfig, client *http.Client, elements []codeElement, summarizeSvc *summarize.Service) (int, int, int) {
	items := make([]batchIngestItem, 0, len(elements))
	timestamp := time.Now().UTC().Format(time.RFC3339)
	symbolCount := 0

	var llmSummaries []string
	if summarizeSvc != nil {
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

		if cfg.verbose && len(llmSummaries) > 0 {
			log.Printf("  [llm-summary] Generated %d LLM summaries", len(llmSummaries))
		}
	}

	for i, elem := range elements {
		structuralSummary := elem.Summary
		if structuralSummary == "" {
			structuralSummary = generateSummary(elem)
		}

		finalSummary := structuralSummary
		if i < len(llmSummaries) && llmSummaries[i] != "" {
			if llmSummaries[i] != structuralSummary {
				finalSummary = summarize.CombineSummary(structuralSummary, llmSummaries[i])
				if cfg.verbose {
					log.Printf("  [llm-summary] %s: %s", elem.Name, llmSummaries[i])
				}
			}
		}

		item := batchIngestItem{
			Timestamp: timestamp,
			Source:    "codebase-ingest",
			Name:      elem.Name,
			Path:      elem.Path,
			Content:   elem.Content,
			Summary:   finalSummary,
			Tags:      elem.Tags,
		}
		if cfg.extractSymbols && len(elem.Symbols) > 0 {
			item.Symbols = elem.Symbols
			symbolCount += len(elem.Symbols)
			if cfg.verbose {
				log.Printf("  [symbols] %s: %d symbols extracted", elem.Name, len(elem.Symbols))
			}
		}
		items = append(items, item)
	}

	req := batchIngestRequest{
		SpaceID:      cfg.spaceID,
		Observations: items,
	}

	body, _ := json.Marshal(req)

	var lastErr error
	retryDelayMs := cfg.retryDelay

	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Retry %d/%d after %dms delay...", attempt, cfg.maxRetries, retryDelayMs)
			time.Sleep(time.Duration(retryDelayMs) * time.Millisecond)
			retryDelayMs *= 2
		}

		resp, err := client.Post(cfg.mdemgEndpoint+"/v1/memory/ingest/batch", "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			if cfg.verbose {
				log.Printf("Batch ingest request failed (attempt %d): %v", attempt+1, err)
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

		if resp.StatusCode == http.StatusBadRequest {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Printf("Batch rejected (non-retryable): status %d: %s", resp.StatusCode, string(bodyBytes))
			return 0, len(elements), 0
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
		if cfg.verbose {
			log.Printf("Batch ingest failed (attempt %d): %v", attempt+1, lastErr)
		}
	}

	log.Printf("Batch failed after %d retries: %v", cfg.maxRetries, lastErr)
	return 0, len(elements), 0
}

func runConsolidation(client *http.Client, endpoint, spaceID string) error {
	req := map[string]string{"space_id": spaceID}
	body, _ := json.Marshal(req)

	resp, err := client.Post(endpoint+"/v1/memory/consolidate", "application/json", bytes.NewReader(body))
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

	log.Printf("Consolidation: created=%d, updated=%d, concept=%d, duration=%.0fms",
		result.Data.HiddenNodesCreated,
		result.Data.HiddenNodesUpdated,
		result.Data.ConceptNodesUpdated,
		result.Data.DurationMs)

	return nil
}

func printSample(cfg *ingestConfig, elements []codeElement) {
	if cfg.quiet {
		return
	}

	counts := make(map[string]int)
	for _, e := range elements {
		counts[e.Kind]++
	}

	log.Println("Element breakdown:")
	for kind, count := range counts {
		log.Printf("  %s: %d", kind, count)
	}

	log.Println("\nSample elements:")
	shown := 0
	for _, e := range elements {
		if shown >= 20 && !cfg.verbose {
			break
		}
		fmt.Printf("  [%s] %s (%s)\n", e.Kind, e.Name, e.FilePath)
		shown++
	}

	if len(elements) > 20 && !cfg.verbose {
		fmt.Printf("  ... and %d more (use --verbose to see all)\n", len(elements)-20)
	}
}
