package languages

import (
	"os"
	"path/filepath"
	"testing"
)

// --- CanParse yield tests ---
// These tests verify that general parsers correctly yield to specialized parsers
// for files that match both. This prevents nondeterministic parser selection
// caused by Go map iteration order in GetParserForFile.

func TestMarkdownParser_YieldsScrapedMd(t *testing.T) {
	md := &MarkdownParser{}
	smd := &ScraperMarkdownParser{}

	// MarkdownParser must NOT claim .scraped.md files
	if md.CanParse("docs/page.scraped.md") {
		t.Error("MarkdownParser.CanParse should return false for .scraped.md")
	}
	if md.CanParse("NOTES.SCRAPED.MD") {
		t.Error("MarkdownParser.CanParse should return false for .SCRAPED.MD (case-insensitive)")
	}

	// ScraperMarkdownParser MUST claim .scraped.md files
	if !smd.CanParse("docs/page.scraped.md") {
		t.Error("ScraperMarkdownParser.CanParse should return true for .scraped.md")
	}

	// MarkdownParser still claims plain .md files
	if !md.CanParse("docs/README.md") {
		t.Error("MarkdownParser.CanParse should return true for .md")
	}
	if !md.CanParse("notes.markdown") {
		t.Error("MarkdownParser.CanParse should return true for .markdown")
	}
}

func TestYAMLParser_YieldsOpenAPI(t *testing.T) {
	dir := t.TempDir()
	yaml := &YAMLParser{}
	oapi := &OpenAPIParser{}

	// OpenAPI YAML file — YAMLParser must yield
	openapiPath := filepath.Join(dir, "petstore.yaml")
	if err := os.WriteFile(openapiPath, []byte("openapi: \"3.0.0\"\ninfo:\n  title: Test API\n  version: \"1.0\"\npaths: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if yaml.CanParse(openapiPath) {
		t.Error("YAMLParser.CanParse should return false for OpenAPI YAML")
	}
	if !oapi.CanParse(openapiPath) {
		t.Error("OpenAPIParser.CanParse should return true for OpenAPI YAML")
	}

	// Swagger YAML — YAMLParser must also yield
	swaggerPath := filepath.Join(dir, "api.yml")
	if err := os.WriteFile(swaggerPath, []byte("swagger: \"2.0\"\ninfo:\n  title: Legacy API\n  version: \"1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if yaml.CanParse(swaggerPath) {
		t.Error("YAMLParser.CanParse should return false for Swagger YAML")
	}
	if !oapi.CanParse(swaggerPath) {
		t.Error("OpenAPIParser.CanParse should return true for Swagger YAML")
	}

	// Plain YAML — YAMLParser claims, OpenAPIParser does not
	plainPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(plainPath, []byte("name: test-app\nversion: 1\nport: 8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !yaml.CanParse(plainPath) {
		t.Error("YAMLParser.CanParse should return true for plain YAML")
	}
	if oapi.CanParse(plainPath) {
		t.Error("OpenAPIParser.CanParse should return false for plain YAML")
	}
}

func TestCParser_YieldsCppHeaders(t *testing.T) {
	dir := t.TempDir()
	cp := &CParser{}
	cpp := &CppParser{}

	// C++ header — CParser must yield
	cppHeaderPath := filepath.Join(dir, "widget.h")
	if err := os.WriteFile(cppHeaderPath, []byte("#pragma once\n\nclass Widget {\npublic:\n    void draw();\nprivate:\n    int x_, y_;\n};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cp.CanParse(cppHeaderPath) {
		t.Error("CParser.CanParse should return false for C++ header (.h with class)")
	}
	if !cpp.CanParse(cppHeaderPath) {
		t.Error("CppParser.CanParse should return true for C++ header")
	}

	// Plain C header — CParser claims, CppParser does not
	cHeaderPath := filepath.Join(dir, "util.h")
	if err := os.WriteFile(cHeaderPath, []byte("#ifndef UTIL_H\n#define UTIL_H\n\nvoid print_message(const char *msg);\nint add(int a, int b);\n\n#endif\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !cp.CanParse(cHeaderPath) {
		t.Error("CParser.CanParse should return true for plain C header")
	}
	if cpp.CanParse(cHeaderPath) {
		t.Error("CppParser.CanParse should return false for plain C header")
	}

	// .c files always go to CParser
	if !cp.CanParse("main.c") {
		t.Error("CParser.CanParse should return true for .c files")
	}
	// .cpp files always go to CppParser
	if !cpp.CanParse("main.cpp") {
		t.Error("CppParser.CanParse should return true for .cpp files")
	}
}

// --- GetParserForFile determinism tests ---
// These verify that GetParserForFile returns the correct specialized parser
// despite nondeterministic map iteration order.

func TestGetParserForFile_ScrapedMarkdown(t *testing.T) {
	parser, found := GetParserForFile("docs/page.scraped.md")
	if !found {
		t.Fatal("GetParserForFile should find a parser for .scraped.md")
	}
	if parser.Name() != "scraper-markdown" {
		t.Errorf("GetParserForFile(.scraped.md) = %q, want %q", parser.Name(), "scraper-markdown")
	}
}

func TestGetParserForFile_OpenAPIYaml(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.yaml")
	if err := os.WriteFile(path, []byte("openapi: \"3.0.0\"\ninfo:\n  title: Test\npaths: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	parser, found := GetParserForFile(path)
	if !found {
		t.Fatal("GetParserForFile should find a parser for OpenAPI YAML")
	}
	if parser.Name() != "openapi" {
		t.Errorf("GetParserForFile(openapi.yaml) = %q, want %q", parser.Name(), "openapi")
	}
}

func TestGetParserForFile_CppHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "engine.h")
	if err := os.WriteFile(path, []byte("namespace engine {\nclass Renderer {\npublic:\n    void render();\n};\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	parser, found := GetParserForFile(path)
	if !found {
		t.Fatal("GetParserForFile should find a parser for C++ header")
	}
	if parser.Name() != "cpp" {
		t.Errorf("GetParserForFile(cpp.h) = %q, want %q", parser.Name(), "cpp")
	}
}

// TestGetParserForFile_OpenAPIJson_Nondeterministic documents a known bug:
// both JSONParser and OpenAPIParser claim OpenAPI .json files, and
// GetParserForFile returns whichever parser the map iteration visits first.
// This test does NOT assert which parser wins — it documents the conflict.
// Tracked in issue #312 (JSON/OpenAPI half).
func TestGetParserForFile_OpenAPIJson_Nondeterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.json")
	if err := os.WriteFile(path, []byte(`{"openapi":"3.0.0","info":{"title":"Test","version":"1.0"},"paths":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	jp := &JSONParser{}
	oapi := &OpenAPIParser{}

	// Both parsers claim the file — this is the bug.
	if !jp.CanParse(path) {
		t.Error("JSONParser.CanParse should return true for .json (no yield implemented)")
	}
	if !oapi.CanParse(path) {
		t.Error("OpenAPIParser.CanParse should return true for OpenAPI .json")
	}

	// Demonstrate nondeterminism: call GetParserForFile multiple times.
	// Due to Go map iteration randomization, we may see different results.
	results := make(map[string]int)
	for range 100 {
		parser, found := GetParserForFile(path)
		if found {
			results[parser.Name()]++
		}
	}
	t.Logf("GetParserForFile results over 100 calls (nondeterministic): %v", results)
	t.Log("Known issue: JSONParser does not yield to OpenAPIParser for .json files (issue #312)")
}
