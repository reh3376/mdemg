// Package cli provides Cobra commands for the unified MDEMG CLI.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
	"mdemg/internal/languages"
	"mdemg/internal/symbols"
)

// extractSymbolsConfig holds all configuration for extract-symbols command
type extractSymbolsConfig struct {
	path        string
	spaceID     string
	neo4jURI    string
	neo4jUser   string
	neo4jPass   string
	workers     int
	dryRun      bool
	verbose     bool
	excludeDirs string
	jsonOutput  bool
}

// uptsSymbol is the JSON output format for UPTS testing
type uptsSymbol struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Line       int    `json:"line"`
	LineEnd    int    `json:"line_end,omitempty"`
	Exported   bool   `json:"exported"`
	Parent     string `json:"parent,omitempty"`
	Signature  string `json:"signature,omitempty"`
	Value      string `json:"value,omitempty"`
	DocComment string `json:"doc_comment,omitempty"`
}

// uptsOutput is the JSON output structure for UPTS testing
type uptsOutput struct {
	Symbols []uptsSymbol `json:"symbols"`
}

// uptsTypeMap normalizes parser type names to UPTS canonical names
var uptsTypeMap = map[string]string{
	"const":      "constant",
	"var":        "variable",
	"func":       "function",
	"type_alias": "type",
	"typedef":    "type",
}

// newExtractSymbolsCmd creates the extract-symbols subcommand
func newExtractSymbolsCmd() *cobra.Command {
	cfg := &extractSymbolsConfig{}

	cmd := &cobra.Command{
		Use:   "extract-symbols",
		Short: "Extract code symbols from a codebase using tree-sitter and store in Neo4j",
		Long: `Extracts code symbols from a codebase using tree-sitter and stores them in Neo4j
for evidence-locked retrieval.

For UPTS (Universal Parser Test Specification) testing, use:

  mdemg extract-symbols --json path/to/file.go

This outputs JSON to stdout in UPTS-compatible format.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle positional argument for --json mode
			if cfg.jsonOutput && cfg.path == "" && len(args) > 0 {
				cfg.path = args[0]
			}

			if cfg.path == "" {
				return fmt.Errorf("--path is required")
			}

			// JSON output mode for UPTS testing
			if cfg.jsonOutput {
				runJSONMode(cfg.path)
				return nil
			}

			excludeSet := make(map[string]bool)
			for _, dir := range strings.Split(cfg.excludeDirs, ",") {
				excludeSet[strings.TrimSpace(dir)] = true
			}

			log.Printf("=== Symbol Extraction ===")
			log.Printf("Path: %s", cfg.path)
			log.Printf("Space ID: %s", cfg.spaceID)
			log.Printf("Neo4j: %s", cfg.neo4jURI)
			log.Printf("Workers: %d", cfg.workers)

			// Initialize symbol service (tree-sitter parser)
			svcCfg := symbols.ParserConfig{
				IncludeDocComments: true,
				EvaluateConstants:  true,
				IncludePrivate:     true, // Include class static fields
			}
			svc, err := symbols.NewService(svcCfg)
			if err != nil {
				return fmt.Errorf("failed to create symbol service: %w", err)
			}
			defer svc.Close()

			ctx := context.Background()

			// Initialize symbol store (only if not dry-run)
			var store *symbols.Store
			var driver neo4j.DriverWithContext
			if !cfg.dryRun {
				driver, err = neo4j.NewDriverWithContext(cfg.neo4jURI, neo4j.BasicAuth(cfg.neo4jUser, cfg.neo4jPass, ""))
				if err != nil {
					return fmt.Errorf("failed to create Neo4j driver: %w", err)
				}
				defer func() { _ = driver.Close(ctx) }()
				store = symbols.NewStore(driver)
			}

			// Collect files to process
			var files []string
			err = filepath.Walk(cfg.path, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if info.IsDir() {
					if excludeSet[info.Name()] {
						return filepath.SkipDir
					}
					return nil
				}

				// Only process supported files
				ext := filepath.Ext(path)
				lang := symbols.LanguageFromExtension(ext)
				if lang != symbols.LangUnknown {
					files = append(files, path)
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to walk codebase: %w", err)
			}

			log.Printf("Found %d files to process", len(files))

			// Process files with worker pool
			fileChan := make(chan string, len(files))
			var wg sync.WaitGroup

			var totalSymbols int64
			var totalFiles int64
			var errors int64
			startTime := time.Now()

			for w := 0; w < cfg.workers; w++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for path := range fileChan {
						result, err := svc.ParseFile(ctx, path)
						if err != nil {
							atomic.AddInt64(&errors, 1)
							if cfg.verbose {
								log.Printf("[Worker %d] Error parsing %s: %v", workerID, path, err)
							}
							continue
						}

						if len(result.Symbols) == 0 {
							continue
						}

						atomic.AddInt64(&totalFiles, 1)
						atomic.AddInt64(&totalSymbols, int64(len(result.Symbols)))

						relPath, _ := filepath.Rel(cfg.path, path)
						relPath = "/" + relPath // Make it absolute-like

						if cfg.dryRun || cfg.verbose {
							for _, sym := range result.Symbols {
								log.Printf("[Worker %d] %s: %s (%s) = %s", workerID, relPath, sym.Name, sym.Type, sym.Value)
							}
						}

						if !cfg.dryRun && store != nil {
							// Convert to SymbolRecords
							var records []symbols.SymbolRecord
							for _, sym := range result.Symbols {
								record := symbols.SymbolRecord{
									SpaceID:        cfg.spaceID,
									SymbolID:       symbols.GenerateSymbolID(cfg.spaceID, relPath, sym.Name, sym.Line),
									Name:           sym.Name,
									SymbolType:     string(sym.Type),
									Value:          sym.Value,
									RawValue:       sym.RawValue,
									FilePath:       relPath,
									Line:           sym.Line,    // UPTS standard
									LineEnd:        sym.LineEnd, // UPTS standard
									Exported:       sym.Exported,
									DocComment:     sym.DocComment,
									Signature:      sym.Signature,
									Parent:         sym.Parent,
									Language:       string(sym.Language),
									TypeAnnotation: sym.TypeAnnotation,
								}
								records = append(records, record)
							}

							if err := store.SaveSymbols(ctx, cfg.spaceID, records); err != nil {
								atomic.AddInt64(&errors, 1)
								if cfg.verbose {
									log.Printf("[Worker %d] Failed to store symbols for %s: %v", workerID, relPath, err)
								}
							}
						}

						// Progress log every 100 files
						current := atomic.LoadInt64(&totalFiles)
						if current%100 == 0 {
							elapsed := time.Since(startTime)
							rate := float64(current) / elapsed.Seconds()
							log.Printf("Progress: %d files, %d symbols (%.1f files/s)",
								current, atomic.LoadInt64(&totalSymbols), rate)
						}
					}
				}(w)
			}

			// Send files to workers
			for _, file := range files {
				fileChan <- file
			}
			close(fileChan)

			wg.Wait()

			elapsed := time.Since(startTime)
			log.Printf("=== Extraction Complete ===")
			log.Printf("Files: %d, Symbols: %d, Errors: %d", totalFiles, totalSymbols, errors)
			log.Printf("Time: %v, Rate: %.1f files/sec", elapsed, float64(totalFiles)/elapsed.Seconds())

			return nil
		},
	}

	// Define flags
	cmd.Flags().StringVar(&cfg.path, "path", "", "Path to codebase to extract symbols from (required)")
	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "codebase", "MDEMG space ID")
	cmd.Flags().StringVar(&cfg.neo4jURI, "neo4j-uri", "bolt://localhost:7687", "Neo4j URI")
	cmd.Flags().StringVar(&cfg.neo4jUser, "neo4j-user", "neo4j", "Neo4j username")
	cmd.Flags().StringVar(&cfg.neo4jPass, "neo4j-pass", "testpassword", "Neo4j password")
	cmd.Flags().IntVar(&cfg.workers, "workers", 8, "Number of parallel workers")
	cmd.Flags().BoolVar(&cfg.dryRun, "dry-run", false, "Print what would be extracted without storing")
	cmd.Flags().BoolVar(&cfg.verbose, "verbose", false, "Verbose output")
	cmd.Flags().StringVar(&cfg.excludeDirs, "exclude", ".git,node_modules,vendor,test,__tests__,.vscode", "Comma-separated directories to exclude")
	cmd.Flags().BoolVar(&cfg.jsonOutput, "json", false, "Output symbols as JSON to stdout (for UPTS testing)")

	return cmd
}

// normalizeType converts parser type to UPTS canonical type
func normalizeType(t string) string {
	if canonical, ok := uptsTypeMap[t]; ok {
		return canonical
	}
	return t
}

// runJSONMode parses a single file and outputs UPTS-compatible JSON to stdout
func runJSONMode(filePath string) {
	// For C/C++/CUDA/Rust files, prefer the ingest-codebase parser over tree-sitter
	// because the ingest-codebase parser has better accuracy for these languages
	ext := strings.ToLower(filepath.Ext(filePath))
	preferIngestParser := ext == ".c" || ext == ".h" || ext == ".cpp" || ext == ".cc" ||
		ext == ".cxx" || ext == ".hpp" || ext == ".hh" || ext == ".hxx" ||
		ext == ".cu" || ext == ".cuh" || ext == ".rs"

	// Try ingest-codebase parser first for C/C++/CUDA
	if preferIngestParser {
		if parser, found := languages.GetParserForFile(filePath); found {
			absPath, _ := filepath.Abs(filePath)
			dir := filepath.Dir(absPath)
			elements, parseErr := parser.ParseFile(dir, absPath, true)
			if parseErr == nil && len(elements) > 0 {
				var allSymbols []uptsSymbol
				for _, elem := range elements {
					for _, sym := range elem.Symbols {
						uptsSymbol := uptsSymbol{
							Name:       sym.Name,
							Type:       normalizeType(sym.Type),
							Line:       sym.Line,
							LineEnd:    sym.LineEnd,
							Exported:   sym.Exported,
							Parent:     sym.Parent,
							Signature:  sym.Signature,
							Value:      sym.Value,
							DocComment: sym.DocComment,
						}
						allSymbols = append(allSymbols, uptsSymbol)
					}
				}
				output := uptsOutput{Symbols: allSymbols}
				jsonBytes, jsonErr := json.MarshalIndent(output, "", "  ")
				if jsonErr != nil {
					log.Fatalf("Failed to marshal JSON: %v", jsonErr)
				}
				fmt.Println(string(jsonBytes))
				return
			}
		}
	}

	cfg := symbols.ParserConfig{
		IncludeDocComments: true,
		EvaluateConstants:  true,
		IncludePrivate:     true,
	}
	svc, err := symbols.NewService(cfg)
	if err != nil {
		log.Fatalf("Failed to create symbol service: %v", err)
	}
	defer svc.Close()

	ctx := context.Background()
	result, err := svc.ParseFile(ctx, filePath)

	// If tree-sitter fails or returns no symbols, try ingest-codebase parser
	treeSitterFailed := err != nil || (result != nil && len(result.Symbols) == 0) || result == nil
	if treeSitterFailed {
		// Try ingest-codebase parser first (matches UPTS specs exactly)
		if parser, found := languages.GetParserForFile(filePath); found {
			absPath, _ := filepath.Abs(filePath)
			dir := filepath.Dir(absPath)
			elements, parseErr := parser.ParseFile(dir, absPath, true)
			if parseErr == nil && len(elements) > 0 {
				// Use ingest-codebase parser symbols
				var allSymbols []uptsSymbol
				for _, elem := range elements {
					for _, sym := range elem.Symbols {
						uptsSymbol := uptsSymbol{
							Name:       sym.Name,
							Type:       normalizeType(sym.Type),
							Line:       sym.Line,
							LineEnd:    sym.LineEnd,
							Exported:   sym.Exported,
							Parent:     sym.Parent,
							Signature:  sym.Signature,
							Value:      sym.Value,
							DocComment: sym.DocComment,
						}
						allSymbols = append(allSymbols, uptsSymbol)
					}
				}
				output := uptsOutput{Symbols: allSymbols}
				jsonBytes, jsonErr := json.MarshalIndent(output, "", "  ")
				if jsonErr != nil {
					log.Fatalf("Failed to marshal JSON: %v", jsonErr)
				}
				fmt.Println(string(jsonBytes))
				return
			}
		}

		// Fall back to legacy fallback parsers if ingest-codebase parser not found
		fallbackSymbols, handled, fallbackErr := tryFallbackParser(filePath)
		if handled {
			if fallbackErr != nil {
				log.Fatalf("Fallback parser error: %v", fallbackErr)
			}
			// Use fallback symbols
			output := uptsOutput{
				Symbols: make([]uptsSymbol, 0, len(fallbackSymbols)),
			}
			for _, sym := range fallbackSymbols {
				uptsSymbol := uptsSymbol{
					Name:       sym.Name,
					Type:       normalizeType(sym.Type),
					Line:       sym.Line,
					Exported:   sym.Exported,
					Parent:     sym.Parent,
					Signature:  sym.Signature,
					Value:      sym.Value,
					DocComment: sym.DocComment,
				}
				output.Symbols = append(output.Symbols, uptsSymbol)
			}
			jsonBytes, jsonErr := json.MarshalIndent(output, "", "  ")
			if jsonErr != nil {
				log.Fatalf("Failed to marshal JSON: %v", jsonErr)
			}
			fmt.Println(string(jsonBytes))
			return
		}
		// If fallback didn't handle it and tree-sitter had an error, fail
		if err != nil {
			log.Fatalf("Failed to parse file: %v", err)
		}
	}

	output := uptsOutput{
		Symbols: make([]uptsSymbol, 0, len(result.Symbols)),
	}

	for _, sym := range result.Symbols {
		uptsSymbol := uptsSymbol{
			Name:       sym.Name,
			Type:       normalizeType(string(sym.Type)),
			Line:       sym.Line,
			LineEnd:    sym.LineEnd,
			Exported:   sym.Exported,
			Parent:     sym.Parent,
			Signature:  sym.Signature,
			Value:      sym.Value,
			DocComment: sym.DocComment,
		}
		output.Symbols = append(output.Symbols, uptsSymbol)
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}

	fmt.Println(string(jsonBytes))
}
