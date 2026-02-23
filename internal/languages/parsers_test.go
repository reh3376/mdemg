package languages

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParsersOnZed tests all parsers against the Zed codebase
func TestParsersOnZed(t *testing.T) {
	zedPath := os.ExpandEnv("$HOME/repos/zed")
	if _, err := os.Stat(zedPath); os.IsNotExist(err) {
		t.Skip("Zed codebase not found at ~/repos/zed")
	}

	// Track stats per parser
	stats := make(map[string]struct {
		files    int
		elements int
		errors   int
	})

	// Initialize stats for all parsers
	for _, p := range AllParsers() {
		stats[p.Name()] = struct {
			files    int
			elements int
			errors   int
		}{}
	}

	// Walk the Zed codebase
	err := filepath.Walk(zedPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			// Skip hidden dirs and target
			if info.Name() == ".git" || info.Name() == "target" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Find parser for this file
		parser, ok := GetParserForFile(path)
		if !ok {
			return nil // No parser for this file type
		}

		// Parse the file
		elements, parseErr := parser.ParseFile(zedPath, path, true)

		s := stats[parser.Name()]
		s.files++
		if parseErr != nil {
			s.errors++
			t.Logf("Error parsing %s: %v", path, parseErr)
		} else {
			s.elements += len(elements)
		}
		stats[parser.Name()] = s

		return nil
	})

	if err != nil {
		t.Fatalf("Walk error: %v", err)
	}

	// Report results
	t.Log("\n=== Parser Test Results on Zed Codebase ===")
	t.Logf("%-15s %8s %10s %8s", "PARSER", "FILES", "ELEMENTS", "ERRORS")
	t.Logf("%-15s %8s %10s %8s", "------", "-----", "--------", "------")

	totalFiles := 0
	totalElements := 0
	totalErrors := 0

	for _, p := range AllParsers() {
		s := stats[p.Name()]
		if s.files > 0 {
			t.Logf("%-15s %8d %10d %8d", p.Name(), s.files, s.elements, s.errors)
			totalFiles += s.files
			totalElements += s.elements
			totalErrors += s.errors
		}
	}

	t.Logf("%-15s %8s %10s %8s", "------", "-----", "--------", "------")
	t.Logf("%-15s %8d %10d %8d", "TOTAL", totalFiles, totalElements, totalErrors)

	// Assertions
	if totalFiles == 0 {
		t.Error("No files were parsed")
	}
	if totalElements == 0 {
		t.Error("No elements were extracted")
	}
	if totalErrors > totalFiles/10 {
		t.Errorf("Too many errors: %d errors out of %d files (>10%%)", totalErrors, totalFiles)
	}
}

// TestRustParserOnZed specifically tests Rust parsing on Zed's crates
func TestRustParserOnZed(t *testing.T) {
	zedPath := os.ExpandEnv("$HOME/repos/zed")
	cratesPath := filepath.Join(zedPath, "crates")
	if _, err := os.Stat(cratesPath); os.IsNotExist(err) {
		t.Skip("Zed crates not found")
	}

	parser, ok := GetParser("rust")
	if !ok {
		t.Fatal("Rust parser not registered")
	}

	// Parse a few specific Rust files
	testFiles := []string{
		"crates/zed/src/main.rs",
		"crates/editor/src/editor.rs",
		"crates/gpui/src/app.rs",
	}

	for _, relPath := range testFiles {
		fullPath := filepath.Join(zedPath, relPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Logf("Skipping %s (not found)", relPath)
			continue
		}

		elements, err := parser.ParseFile(zedPath, fullPath, true)
		if err != nil {
			t.Errorf("Failed to parse %s: %v", relPath, err)
			continue
		}

		t.Logf("%s: %d elements", relPath, len(elements))
		for _, elem := range elements {
			if elem.Kind != "module" {
				t.Logf("  - %s (%s)", elem.Name, elem.Kind)
			}
		}

		// Check that we got meaningful content
		if len(elements) == 0 {
			t.Errorf("No elements extracted from %s", relPath)
		}
	}
}

// TestMarkdownParserOnZed tests Markdown parsing on Zed docs
func TestMarkdownParserOnZed(t *testing.T) {
	zedPath := os.ExpandEnv("$HOME/repos/zed")
	readmePath := filepath.Join(zedPath, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		t.Skip("Zed README not found")
	}

	parser, ok := GetParser("markdown")
	if !ok {
		t.Fatal("Markdown parser not registered")
	}

	elements, err := parser.ParseFile(zedPath, readmePath, true)
	if err != nil {
		t.Fatalf("Failed to parse README.md: %v", err)
	}

	t.Logf("README.md: %d elements", len(elements))
	for _, elem := range elements {
		t.Logf("  - %s (%s): %d symbols", elem.Name, elem.Kind, len(elem.Symbols))
	}

	if len(elements) == 0 {
		t.Error("No elements extracted from README.md")
	}
}

// TestSQLParserOnZed tests SQL parsing
func TestSQLParserOnZed(t *testing.T) {
	zedPath := os.ExpandEnv("$HOME/repos/zed")
	sqlPath := filepath.Join(zedPath, "docker-compose.sql")
	if _, err := os.Stat(sqlPath); os.IsNotExist(err) {
		t.Skip("Zed SQL file not found")
	}

	parser, ok := GetParser("sql")
	if !ok {
		t.Fatal("SQL parser not registered")
	}

	elements, err := parser.ParseFile(zedPath, sqlPath, true)
	if err != nil {
		t.Fatalf("Failed to parse SQL: %v", err)
	}

	t.Logf("docker-compose.sql: %d elements", len(elements))
	for _, elem := range elements {
		t.Logf("  - %s (%s)", elem.Name, elem.Kind)
	}
}
