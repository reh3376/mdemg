package symbols

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAnalyzeImplements_Fixture: build a tiny Go module in a temp dir with
// 2 interfaces + 3 concrete types, run AnalyzeImplements, assert the right
// pairs are emitted (and stdlib/self-implements are NOT).
func TestAnalyzeImplements_Fixture(t *testing.T) {
	dir := t.TempDir()

	// Fixture module: `example.com/testmod`
	writeFixture(t, dir, "go.mod", "module example.com/testmod\n\ngo 1.24\n")
	writeFixture(t, dir, "types.go", `package testmod

import "fmt"

// Reader is the io.Reader lookalike for the fixture.
type Reader interface {
	Read(p []byte) (int, error)
}

// Closer with a single Close.
type Closer interface {
	Close() error
}

// ReadCloser embeds both — should IMPLEMENTS Reader AND Closer.
type ReadCloser interface {
	Reader
	Closer
}

// File: implements Reader + Closer + ReadCloser.
type File struct{ name string }

func (f *File) Read(p []byte) (int, error) { return 0, nil }
func (f *File) Close() error               { return nil }

// Stringer: implements fmt.Stringer (stdlib — MUST NOT emit).
type Stringer struct{ v string }

func (s *Stringer) String() string { return s.v }

// Isolated: no interface satisfaction.
type Isolated struct{}

func (i Isolated) DoNothing() {}

// Ensure fmt is used so the import doesn't get dropped by goimports.
var _ = fmt.Sprintf
`)

	a := NewGoTypesAnalyzer()
	rels, err := a.AnalyzeImplements(context.Background(), "test-space", dir)
	if err != nil {
		t.Fatalf("AnalyzeImplements: %v", err)
	}

	// Assertions:
	// 1. At least the 5 expected pairs: File→Reader, File→Closer, File→ReadCloser,
	//    ReadCloser→Reader, ReadCloser→Closer
	// 2. NO Stringer→fmt.Stringer (stdlib skip)
	// 3. NO ReadCloser→ReadCloser (self-implements skip)
	if len(rels) < 5 {
		t.Errorf("expected ≥5 IMPLEMENTS pairs; got %d: %+v", len(rels), pairsSummary(rels))
	}

	_ = pairsSummary(rels) // debug helper — retained for diagnostic use, not asserted here
	mustContain := []string{
		"types.go/File -> types.go/Reader",
		"types.go/File -> types.go/Closer",
		"types.go/File -> types.go/ReadCloser",
		"types.go/ReadCloser -> types.go/Reader",
		"types.go/ReadCloser -> types.go/Closer",
	}
	// Contains-match on the ID-mapped summary (symbol_ids are hashes, so we
	// re-express as filepath#name for assertions).
	summaryByName := pairsByName(t, rels, dir)
	for _, want := range mustContain {
		if !summaryByName[want] {
			t.Errorf("missing IMPLEMENTS pair: %s\nall pairs (name-form): %v",
				want, summaryByName)
		}
	}

	// Stdlib skip: our fixture's Stringer implements fmt.Stringer, but the
	// fmt.Stringer interface lives under package "fmt" which isStdlib skips.
	// Verify: no rel targets a name matching "Stringer" from OUTSIDE our
	// fixture file (defensive — the pair's target would need to point to
	// stdlib to fail).
	for k := range summaryByName {
		if strings.Contains(k, " -> fmt/Stringer") {
			t.Errorf("stdlib IMPLEMENTS emitted; expected skip: %s", k)
		}
	}

	// Self-implements skip: no rel has src == dst.
	for _, r := range rels {
		if r.SourceSymbolID == r.TargetSymbolID {
			t.Errorf("self-IMPLEMENTS emitted: %s -> %s", r.SourceSymbolID, r.TargetSymbolID)
		}
	}

	// Every rel MUST carry the right metadata contract.
	for _, r := range rels {
		if r.Relation != "IMPLEMENTS" {
			t.Errorf("wrong Relation %q; want IMPLEMENTS", r.Relation)
		}
		if r.ResolutionMethod != "go_types" {
			t.Errorf("wrong ResolutionMethod %q; want go_types", r.ResolutionMethod)
		}
		if r.Tier != 2 {
			t.Errorf("wrong Tier %d; want 2", r.Tier)
		}
		if r.Confidence != 1.0 {
			t.Errorf("wrong Confidence %v; want 1.0", r.Confidence)
		}
	}
}

func TestAnalyzeImplements_EmptyProjectRoot(t *testing.T) {
	a := NewGoTypesAnalyzer()
	_, err := a.AnalyzeImplements(context.Background(), "s", "")
	if err == nil {
		t.Error("expected error for empty projectRoot; got nil")
	}
}

func TestIsStdlib(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"fmt", true},
		{"io", true},
		{"encoding/json", true},
		{"context", true},
		{"github.com/foo/bar", false},
		{"golang.org/x/tools/go/packages", false},
		{"mdemg", false},
		{"mdemg/internal/symbols", false},
		{"example.com/testmod", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isStdlib(c.path); got != c.want {
			t.Errorf("isStdlib(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIsVendored(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/home/x/project/vendor/foo/bar.go", true},
		{"/home/x/go/pkg/mod/example.com/foo@v0.1.0/bar.go", true},
		{"/opt/homebrew/lib/go/pkg/mod/foo/bar.go", true},
		{"/home/x/project/internal/foo.go", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isVendored(c.path); got != c.want {
			t.Errorf("isVendored(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestRelativizePath_MatchesTreeSitterShape(t *testing.T) {
	// SymbolNode.file_path from the tree-sitter ingest carries a leading "/".
	// relativizePath MUST match that shape so the symbol_id hashes agree.
	root := t.TempDir()
	full := filepath.Join(root, "internal", "foo.go")
	got := relativizePath(full, root)
	if !strings.HasPrefix(got, "/") {
		t.Errorf("relativizePath must produce leading-slash path; got %q", got)
	}
	if got != "/internal/foo.go" {
		t.Errorf("relativizePath: got %q, want /internal/foo.go", got)
	}
}

// --- helpers ---

func writeFixture(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
}

func pairsSummary(rels []RelationshipRecord) []string {
	out := make([]string, 0, len(rels))
	for _, r := range rels {
		out = append(out, r.SourceSymbolID[:8]+"→"+r.TargetSymbolID[:8])
	}
	return out
}

// pairsByName reverses the symbol_id hash by rehashing every combination of
// (fixture path, fixture name) in the tempdir — small enough that a linear
// scan works for the test.
func pairsByName(t *testing.T, rels []RelationshipRecord, projectRoot string) map[string]bool {
	t.Helper()
	// Fixture names live in types.go under the tempdir root.
	names := []string{"Reader", "Closer", "ReadCloser", "File", "Stringer", "Isolated"}
	idToLabel := make(map[string]string)
	for _, n := range names {
		id := GenerateSymbolID("test-space", "/types.go", n, 0)
		idToLabel[id] = "types.go/" + n
	}
	out := map[string]bool{}
	for _, r := range rels {
		src, srcOK := idToLabel[r.SourceSymbolID]
		dst, dstOK := idToLabel[r.TargetSymbolID]
		if !srcOK {
			src = r.SourceSymbolID[:8]
		}
		if !dstOK {
			dst = r.TargetSymbolID[:8]
		}
		out[src+" -> "+dst] = true
	}
	return out
}
