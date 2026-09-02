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

// Sprint MDEMG-DOCS-INGEST-002 (task #147) — exclusion tests for the
// filesystem walkers. Ensures the built-in deny-set + env override
// keep venv / bytecode-cache / node_modules / Python-packaging trees
// out of the substrate.

func TestMdemgDocsShouldExcludeDir_BuiltinSet(t *testing.T) {
	set := map[string]struct{}{
		".venv":        {},
		"__pycache__":  {},
		"node_modules": {},
		"dist-info":    {},
		"egg-info":     {},
	}
	// Exact matches
	for _, name := range []string{".venv", "__pycache__", "node_modules"} {
		if !mdemgDocsShouldExcludeDir(name, set) {
			t.Errorf("expected %q excluded", name)
		}
	}
	// Suffix matches (Python packaging shapes)
	for _, name := range []string{"mdemg-1.2.3.dist-info", "cuid2.egg-info"} {
		if !mdemgDocsShouldExcludeDir(name, set) {
			t.Errorf("expected %q excluded via suffix rule", name)
		}
	}
	// Non-matches
	for _, name := range []string{"docs", "features", "internal", "cli", ".git", "node_modules_backup"} {
		if mdemgDocsShouldExcludeDir(name, set) {
			t.Errorf("expected %q NOT excluded", name)
		}
	}
}

func TestMdemgDocsShouldExcludeDir_EmptySetDisables(t *testing.T) {
	empty := map[string]struct{}{}
	for _, name := range []string{".venv", "__pycache__", "node_modules", "mdemg-1.2.3.dist-info"} {
		if mdemgDocsShouldExcludeDir(name, empty) {
			t.Errorf("expected %q NOT excluded when set is empty (disables suffix rules too)", name)
		}
	}
}

func TestMdemgDocsExcludeDirSet_EnvOverride(t *testing.T) {
	// Extend built-in with custom entries
	t.Setenv("MDEMG_DOCS_INGEST_EXCLUDE_DIRS", "vendor,tmp")
	set := mdemgDocsExcludeDirSet()
	for _, want := range []string{".venv", "__pycache__", "node_modules", "dist-info", "egg-info", "vendor", "tmp"} {
		if _, ok := set[want]; !ok {
			t.Errorf("expected exclusion set to contain %q, got keys: %v", want, keysOf(set))
		}
	}
}

func TestMdemgDocsExcludeDirSet_EnvDashDisablesBuiltin(t *testing.T) {
	t.Setenv("MDEMG_DOCS_INGEST_EXCLUDE_DIRS", "-")
	set := mdemgDocsExcludeDirSet()
	if len(set) != 0 {
		t.Errorf("expected empty set on '-', got %v", keysOf(set))
	}
}

// TestCollectMarkdownChunks_SkipsExcludedSubtree writes a synthetic doc
// tree that includes a .venv subtree carrying a real README.md, and
// verifies the walker never returns any chunk sourced from that subtree.
func TestCollectMarkdownChunks_SkipsExcludedSubtree(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs", "features")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	// Legit doc
	legit := filepath.Join(docsDir, "real-feature.md")
	if err := os.WriteFile(legit, []byte("# Real Feature\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Junk under nested .venv — must be skipped
	venvDocs := filepath.Join(docsDir, "some-project", ".venv", "lib", "site-packages", "somepkg")
	if err := os.MkdirAll(venvDocs, 0o755); err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(venvDocs, "README.md")
	if err := os.WriteFile(junk, []byte("# Junk from venv\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Also drop a __pycache__ tree and a node_modules tree — same expectation
	pyc := filepath.Join(docsDir, "__pycache__")
	if err := os.MkdirAll(pyc, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(pyc, "cached.md"), []byte("# Junk from pycache\n"), 0o644)

	nm := filepath.Join(docsDir, "app", "node_modules", "left-pad")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(nm, "README.md"), []byte("# Junk from npm\n"), 0o644)

	// dist-info / egg-info suffix cases
	dinfo := filepath.Join(docsDir, "app", "mdemg-1.0.0.dist-info")
	_ = os.MkdirAll(dinfo, 0o755)
	_ = os.WriteFile(filepath.Join(dinfo, "METADATA.md"), []byte("# Junk from dist-info\n"), 0o644)

	einfo := filepath.Join(docsDir, "app", "cuid2.egg-info")
	_ = os.MkdirAll(einfo, 0o755)
	_ = os.WriteFile(filepath.Join(einfo, "PKG-INFO.md"), []byte("# Junk from egg-info\n"), 0o644)

	chunks, err := collectMarkdownChunks("features", docsDir, root)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	// Must have exactly the legit chunk (+ preamble split not applicable to single-H1 body → 1 whole-file chunk)
	if len(chunks) == 0 {
		t.Fatalf("expected at least the legit chunk, got 0")
	}
	for _, ch := range chunks {
		lower := strings.ToLower(ch.SourceFile)
		for _, banned := range []string{".venv", "__pycache__", "node_modules", ".dist-info", ".egg-info"} {
			if strings.Contains(lower, banned) {
				t.Errorf("chunk from excluded subtree leaked: source=%s header=%s", ch.SourceFile, ch.SectionHeader)
			}
		}
	}
	// At least one chunk must come from the real file
	sawLegit := false
	for _, ch := range chunks {
		if strings.HasSuffix(ch.SourceFile, "real-feature.md") {
			sawLegit = true
		}
	}
	if !sawLegit {
		t.Errorf("legit real-feature.md chunk missing from %d chunks", len(chunks))
	}
}

// TestCollectMarkdownChunks_EnvOverrideDisablesBuiltin verifies that
// setting MDEMG_DOCS_INGEST_EXCLUDE_DIRS=- fully disables the exclusion
// (so junk under .venv WOULD be ingested — the operator-directed escape
// hatch for advanced use).
func TestCollectMarkdownChunks_EnvOverrideDisablesBuiltin(t *testing.T) {
	t.Setenv("MDEMG_DOCS_INGEST_EXCLUDE_DIRS", "-")
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs", "features")
	venvDocs := filepath.Join(docsDir, ".venv", "lib")
	if err := os.MkdirAll(venvDocs, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(venvDocs, "leak.md"), []byte("# Leaked\n\nBody.\n"), 0o644)

	chunks, err := collectMarkdownChunks("features", docsDir, root)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	sawLeak := false
	for _, ch := range chunks {
		if strings.Contains(ch.SourceFile, ".venv") {
			sawLeak = true
		}
	}
	if !sawLeak {
		t.Errorf("with MDEMG_DOCS_INGEST_EXCLUDE_DIRS=-, expected .venv leak; got %d chunks", len(chunks))
	}
}

// helpers

func keysOf(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

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
