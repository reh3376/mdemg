//go:build ignore

// php_parser_verify.go — standalone tool to extract PHP symbols from a file or directory.
// Usage: go run scripts/php_parser_verify.go <path> [path2 ...]
// If path is a directory, recursively finds all .php files (excluding vendor/node_modules).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mdemg/internal/languages"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <path> [path2 ...]\n", os.Args[0])
		os.Exit(1)
	}

	parser := &languages.PHPParser{}
	var allFiles []string

	for _, arg := range os.Args[1:] {
		info, err := os.Stat(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s: %v\n", arg, err)
			continue
		}
		if info.IsDir() {
			filepath.Walk(arg, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if fi.IsDir() {
					base := filepath.Base(path)
					if base == "vendor" || base == "node_modules" || base == ".git" {
						return filepath.SkipDir
					}
					return nil
				}
				if strings.HasSuffix(path, ".php") {
					allFiles = append(allFiles, path)
				}
				return nil
			})
		} else if strings.HasSuffix(arg, ".php") {
			allFiles = append(allFiles, arg)
		}
	}

	type FileResult struct {
		File    string             `json:"file"`
		Symbols []languages.Symbol `json:"symbols"`
		Error   string             `json:"error,omitempty"`
	}

	var results []FileResult
	totalSymbols := 0
	filesWithSymbols := 0
	parseErrors := 0

	for _, f := range allFiles {
		root := filepath.Dir(f)
		elements, err := parser.ParseFile(root, f, true)
		if err != nil {
			results = append(results, FileResult{File: f, Error: err.Error()})
			parseErrors++
			continue
		}
		var syms []languages.Symbol
		for _, el := range elements {
			syms = append(syms, el.Symbols...)
		}
		if len(syms) > 0 {
			results = append(results, FileResult{File: f, Symbols: syms})
			totalSymbols += len(syms)
			filesWithSymbols++
		}
	}

	fmt.Fprintf(os.Stderr, "Scanned %d PHP files, found %d with symbols, %d total symbols", len(allFiles), filesWithSymbols, totalSymbols)
	if parseErrors > 0 {
		fmt.Fprintf(os.Stderr, " (%d parse errors)", parseErrors)
	}
	fmt.Fprintln(os.Stderr)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(results)
}
