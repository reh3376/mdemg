package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Sprint MDEMG-DOCS-INGEST-001 (task #142) — Tier 1 pin tests for the
// mdemg-docs-ingest CLI + chunker + payload builder + CLAUDE.md filter +
// cobra Long extractor. Six tests per sprint plan §Epic 2.

func TestMdemgDocsChunker_H2Split(t *testing.T) {
	body := `# Title

Intro line.

## Section A

Body A.

## Section B

Body B.

## Section C

Body C.
`
	sections := splitMdemgDocsH2(body)
	// Expected: preamble + 3 H2 sections = 4 chunks
	if len(sections) != 4 {
		t.Fatalf("got %d sections, want 4: %+v", len(sections), sections)
	}
	if sections[0].Header != "(preamble)" {
		t.Errorf("first section header = %q, want (preamble)", sections[0].Header)
	}
	if !strings.Contains(sections[0].Body, "Intro line.") {
		t.Errorf("preamble body missing intro: %q", sections[0].Body)
	}
	wantHeaders := []string{"(preamble)", "Section A", "Section B", "Section C"}
	for i, want := range wantHeaders {
		if sections[i].Header != want {
			t.Errorf("section[%d].Header = %q, want %q", i, sections[i].Header, want)
		}
	}
	// Each H2 section body should START with the H2 header line (preserved verbatim)
	for i := 1; i < 4; i++ {
		if !strings.HasPrefix(sections[i].Body, "## ") {
			t.Errorf("section[%d].Body does not start with '## ': %q", i, sections[i].Body[:min(30, len(sections[i].Body))])
		}
	}
}

func TestMdemgDocsChunker_NoH2(t *testing.T) {
	body := "A tiny markdown file with no H2 headers at all.\nJust a paragraph."
	sections := splitMdemgDocsH2(body)
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1: %+v", len(sections), sections)
	}
	if sections[0].Header != "(whole file)" {
		t.Errorf("no-H2 header = %q, want (whole file)", sections[0].Header)
	}
	if sections[0].Body != strings.TrimSpace(body) {
		t.Errorf("no-H2 body mismatch")
	}
}

// TestMdemgDocsChunker_CLAUDEmd_ExcludesNarrative constructs a synthetic
// CLAUDE.md fragment with Architecture Notes (keep), Enforced Protocols
// (keep), and a Sprint-record H2 (drop per JIMINY-CORPUS-003 class).
func TestMdemgDocsChunker_CLAUDEmd_ExcludesNarrative(t *testing.T) {
	tmp := t.TempDir()
	claude := filepath.Join(tmp, "CLAUDE.md")
	body := `# CLAUDE.md project instructions

Preamble.

## Architecture Notes

Durable arch content.

## Sprint PHASE-E3 shipping notes

Session-specific narrative that should NOT be ingested.

## Enforced Protocols (Hook-Backed)

Durable protocol content.

## Recent commits log

Historic session log — reject.
`
	if err := os.WriteFile(claude, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	chunks, err := collectClaudeMdChunks(claude, tmp)
	if err != nil {
		t.Fatalf("collectClaudeMdChunks: %v", err)
	}
	// Expected kept: (preamble), "Architecture Notes", "Enforced Protocols (Hook-Backed)" = 3
	// Rejected: "Sprint PHASE-E3 shipping notes", "Recent commits log"
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3 (preamble + 2 durable): %+v", len(chunks), headersOf(chunks))
	}
	want := []string{"(preamble)", "Architecture Notes", "Enforced Protocols (Hook-Backed)"}
	for i, w := range want {
		if chunks[i].SectionHeader != w {
			t.Errorf("chunk[%d].SectionHeader = %q, want %q", i, chunks[i].SectionHeader, w)
		}
	}
	// Every chunk must carry surface=claude
	for _, ch := range chunks {
		if ch.Surface != "claude" {
			t.Errorf("chunk %q has surface %q, want claude", ch.SectionHeader, ch.Surface)
		}
	}
}

// TestMdemgDocsChunker_CobraLongExtractor parses a synthetic Go source with
// two cobra.Command literals — one with a Long, one without — and asserts
// only the non-empty Long is extracted.
func TestMdemgDocsChunker_CobraLongExtractor(t *testing.T) {
	src := "package cli\n" +
		"import (\n" +
		"\t\"github.com/spf13/cobra\"\n" +
		")\n" +
		"func newFooCmd() *cobra.Command {\n" +
		"\treturn &cobra.Command{\n" +
		"\t\tUse:   \"foo\",\n" +
		"\t\tShort: \"Do foo\",\n" +
		"\t\tLong:  `Foo does the foo operation.\n\nMore details here.`,\n" +
		"\t}\n" +
		"}\n" +
		"func newBarCmd() *cobra.Command {\n" +
		"\treturn &cobra.Command{\n" +
		"\t\tUse:   \"bar\",\n" +
		"\t\tShort: \"Do bar\",\n" +
		"\t\t// no Long\n" +
		"\t}\n" +
		"}\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	commandName, longs := extractCobraLongs(file)
	if commandName != "foo" {
		t.Errorf("commandName = %q, want foo", commandName)
	}
	if len(longs) != 1 {
		t.Fatalf("got %d longs, want 1: %+v", len(longs), longs)
	}
	if !strings.Contains(longs[0], "Foo does the foo operation.") {
		t.Errorf("long content missing: %q", longs[0])
	}
	if !strings.Contains(longs[0], "More details here.") {
		t.Errorf("long multiline body missing: %q", longs[0])
	}
}

// TestBuildMdemgDocsIngestRequest_ShapeParity asserts the POST payload has
// the same required-field set as the shipped claude-docs-ingest payload —
// so /v1/memory/ingest treats them identically.
func TestBuildMdemgDocsIngestRequest_ShapeParity(t *testing.T) {
	ch := mdemgDocsChunk{
		Surface:       "features",
		SourceFile:    "docs/features/alert-dispatcher.md",
		SectionIndex:  0,
		SectionHeader: "## How it works",
		Content:       "The alert dispatcher writes to ~/.mdemg/alerts/current.json.",
	}
	req := buildMdemgDocsIngestRequest(ch, "mdemg-dev")

	// Required fields present + non-empty
	required := []string{"space_id", "timestamp", "source", "content", "path", "name", "summary", "tags", "content_hash"}
	for _, key := range required {
		if _, ok := req[key]; !ok {
			t.Errorf("payload missing required field: %s", key)
		}
	}

	// Field type + value checks
	if req["space_id"].(string) != "mdemg-dev" {
		t.Errorf("space_id = %v, want mdemg-dev", req["space_id"])
	}
	if req["source"].(string) != "mdemg-docs-ingest" {
		t.Errorf("source = %v, want mdemg-docs-ingest", req["source"])
	}
	if req["content"].(string) != ch.Content {
		t.Errorf("content mismatch")
	}
	// content_hash is deterministic SHA256(content)
	wantSum := sha256.Sum256([]byte(ch.Content))
	wantHex := hex.EncodeToString(wantSum[:])
	if req["content_hash"].(string) != wantHex {
		t.Errorf("content_hash = %v, want %s", req["content_hash"], wantHex)
	}
	// Path prefix must be mdemg-docs (NOT claude-docs)
	path := req["path"].(string)
	if !strings.HasPrefix(path, "mdemg-docs/") {
		t.Errorf("path %q does not start with mdemg-docs/", path)
	}
	// Tags must include the surface tag
	tags := req["tags"].([]string)
	found := false
	for _, tag := range tags {
		if tag == "docs:mdemg:features" {
			found = true
		}
	}
	if !found {
		t.Errorf("tags missing docs:mdemg:features: %v", tags)
	}
}

// TestPathSlug_MdemgDocs pins the path shape: mdemg-docs/<surface>/<file>/<idx>__<header>.
// Also verifies non-collision with claude-docs (different prefix).
func TestPathSlug_MdemgDocs(t *testing.T) {
	// Two sibling features with the same H2 header should get DIFFERENT paths
	// (filename differentiates).
	chA := mdemgDocsChunk{
		Surface:       "features",
		SourceFile:    "docs/features/alert-dispatcher.md",
		SectionIndex:  1,
		SectionHeader: "How it works",
		Content:       "A body.",
	}
	chB := mdemgDocsChunk{
		Surface:       "features",
		SourceFile:    "docs/features/hitl-auto-curation.md",
		SectionIndex:  1,
		SectionHeader: "How it works",
		Content:       "B body.",
	}
	pathA := buildMdemgDocsIngestRequest(chA, "mdemg-dev")["path"].(string)
	pathB := buildMdemgDocsIngestRequest(chB, "mdemg-dev")["path"].(string)
	if pathA == pathB {
		t.Fatalf("path collision on same H2 across different files: %q", pathA)
	}
	// Both paths start with expected shape
	for _, p := range []string{pathA, pathB} {
		if !strings.HasPrefix(p, "mdemg-docs/features/") {
			t.Errorf("path %q does not start with mdemg-docs/features/", p)
		}
		if !strings.Contains(p, "how-it-works") {
			t.Errorf("path %q does not contain slugified header 'how-it-works'", p)
		}
	}
	// Paths must NOT start with claude-docs (namespace isolation)
	if strings.HasPrefix(pathA, "claude-docs") {
		t.Errorf("path %q collides with claude-docs namespace", pathA)
	}
}

// helpers

func headersOf(chunks []mdemgDocsChunk) []string {
	out := make([]string, len(chunks))
	for i, ch := range chunks {
		out[i] = ch.SectionHeader
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
