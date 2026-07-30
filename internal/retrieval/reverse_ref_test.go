package retrieval

import (
	"os"
	"path/filepath"
	"testing"

	"mdemg/internal/models"
)

func TestExtractCandidateSymbols_FiltersStopWordsAndShort(t *testing.T) {
	got := extractCandidateSymbols("what consumes the constraint_outcomes table", 5)
	// Expect: constraint_outcomes only. "what", "table" are stop words.
	// "the", "consumes" too. "constraint_outcomes" length ≥ 5, distinctive.
	if len(got) != 1 || got[0] != "constraint_outcomes" {
		t.Errorf("got %v, want [constraint_outcomes]", got)
	}
}

func TestExtractCandidateSymbols_PreservesCaseOfFirstOccurrence(t *testing.T) {
	got := extractCandidateSymbols("What uses DatasetBuilder for query?", 5)
	// "What" and "uses" are stop words, "query" filtered by len; "DatasetBuilder" remains.
	if len(got) != 1 || got[0] != "DatasetBuilder" {
		t.Errorf("got %v, want [DatasetBuilder]", got)
	}
}

func TestExtractCandidateSymbols_DedupsAndKeepsOrder(t *testing.T) {
	got := extractCandidateSymbols("consumes DatasetBuilder and DatasetBuilder consumes LLMPerformance", 5)
	if len(got) != 2 {
		t.Fatalf("got %v (len=%d), want 2 unique tokens", got, len(got))
	}
	if got[0] != "DatasetBuilder" || got[1] != "LLMPerformance" {
		t.Errorf("got %v, want [DatasetBuilder LLMPerformance]", got)
	}
}

func TestExtractCandidateSymbols_EmptyText(t *testing.T) {
	if got := extractCandidateSymbols("", 5); len(got) != 0 {
		t.Errorf("empty text should yield no symbols, got %v", got)
	}
}

func TestExtractCandidateSymbols_MinLenGate(t *testing.T) {
	got := extractCandidateSymbols("foo bar baz constraint_outcomes", 5)
	// foo/bar/baz all < 5 chars → skipped
	if len(got) != 1 || got[0] != "constraint_outcomes" {
		t.Errorf("got %v, want [constraint_outcomes]", got)
	}
}

func TestGrepReferences_FindsAndRanksByHitCount(t *testing.T) {
	// Build a small fixture workspace.
	dir := t.TempDir()
	writeFile(t, dir, "consumer.go", "package p\n// SELECT * FROM constraint_outcomes WHERE guidance_type='constraint'\nfunc use() { _ = \"constraint_outcomes\" }\n")
	writeFile(t, dir, "writer.go", "package p\n// INSERT INTO constraint_outcomes VALUES ...\n")
	writeFile(t, dir, "unrelated.go", "package p\nfunc other() {}\n")
	// Excluded extension:
	writeFile(t, dir, "notes.txt", "constraint_outcomes appears here too")
	// Nested but excluded dir:
	if err := os.MkdirAll(filepath.Join(dir, "vendor", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "vendor/sub/inside.go", "constraint_outcomes constraint_outcomes constraint_outcomes")
	hits, err := grepReferences(dir, []string{"constraint_outcomes"},
		[]string{".go"}, []string{"vendor"}, 100, 10)
	if err != nil {
		t.Fatalf("grepReferences: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits (consumer.go + writer.go), got %d: %+v", len(hits), hits)
	}
	// consumer.go has 2 hits, writer.go has 1 → ordering
	if hits[0].RelPath != "consumer.go" {
		t.Errorf("expected consumer.go first (2 hits); got %s", hits[0].RelPath)
	}
	if hits[1].RelPath != "writer.go" {
		t.Errorf("expected writer.go second; got %s", hits[1].RelPath)
	}
	// unrelated.go: 0 hits, excluded. notes.txt: excluded extension.
	// vendor/sub/inside.go: excluded dir. So len == 2 is correct.
}

func TestGrepReferences_MultipleSymbols(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "foo bar")
	writeFile(t, dir, "b.go", "bar bar bar baz")
	writeFile(t, dir, "c.go", "unrelated")
	hits, err := grepReferences(dir, []string{"foo", "bar"},
		[]string{".go"}, nil, 100, 10)
	if err != nil {
		t.Fatalf("grepReferences: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d: %+v", len(hits), hits)
	}
	// b.go: bar×3 = 3, a.go: foo+bar = 2
	if hits[0].RelPath != "b.go" {
		t.Errorf("expected b.go first; got %s", hits[0].RelPath)
	}
}

func TestGrepReferences_WordBoundary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "constraint_outcomes")
	writeFile(t, dir, "b.go", "constraint_outcomesXYZ") // NOT a word-boundary match
	hits, err := grepReferences(dir, []string{"constraint_outcomes"},
		[]string{".go"}, nil, 100, 10)
	if err != nil {
		t.Fatalf("grepReferences: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit (word-boundary), got %d: %+v", len(hits), hits)
	}
	if hits[0].RelPath != "a.go" {
		t.Errorf("expected a.go; got %s", hits[0].RelPath)
	}
}

func TestGrepReferences_TopKCap(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		writeFile(t, dir, filepath.Join("f"+string(rune('0'+i))+".go"), "constraint_outcomes")
	}
	hits, err := grepReferences(dir, []string{"constraint_outcomes"},
		[]string{".go"}, nil, 100, 3)
	if err != nil {
		t.Fatalf("grepReferences: %v", err)
	}
	if len(hits) != 3 {
		t.Errorf("expected topK=3, got %d", len(hits))
	}
}

func TestGrepReferences_MaxFilesScanned(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		writeFile(t, dir, filepath.Join("f"+string(rune('0'+(i/10)))+string(rune('0'+(i%10)))+".go"), "constraint_outcomes")
	}
	// Cap scan at 5 → at most 5 hits
	hits, err := grepReferences(dir, []string{"constraint_outcomes"},
		[]string{".go"}, nil, 5, 100)
	if err != nil {
		t.Fatalf("grepReferences: %v", err)
	}
	if len(hits) > 5 {
		t.Errorf("expected maxFilesScanned=5 to cap hits at 5, got %d", len(hits))
	}
}

func TestGrepReferences_EmptyInputs(t *testing.T) {
	if _, err := grepReferences("", []string{"x"}, []string{".go"}, nil, 100, 10); err != nil {
		t.Errorf("empty root should be nil, nil; got err=%v", err)
	}
	if _, err := grepReferences(t.TempDir(), nil, []string{".go"}, nil, 100, 10); err != nil {
		t.Errorf("no symbols should be nil, nil; got err=%v", err)
	}
	if _, err := grepReferences(t.TempDir(), []string{"x"}, []string{".go"}, nil, 100, 0); err != nil {
		t.Errorf("topK=0 should be nil, nil; got err=%v", err)
	}
}

func TestIsReverseLookupQuery(t *testing.T) {
	cases := []struct {
		q    string
		want bool
	}{
		{"what consumes the constraint_outcomes table?", true},
		{"which files use DatasetBuilder?", true},
		{"who calls fetchConcreteRecall?", true},
		{"where is constraint_outcomes queried?", true},
		{"how do I enable the recursive-retrain actuator?", false},
		{"what did HITL-CURATION-002 ship?", false},
		{"", false},
		{"just some normal question", false},
	}
	for _, c := range cases {
		got := IsReverseLookupQuery(c.q)
		if got != c.want {
			t.Errorf("IsReverseLookupQuery(%q) = %v, want %v", c.q, got, c.want)
		}
	}
}

func TestParseCSVList(t *testing.T) {
	got := parseCSVList("a, b ,c,,a")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("got[%d]=%q want %q", i, got[i], v)
		}
	}
}

func TestApplyReverseRefQuota_DisabledPassthrough(t *testing.T) {
	in := []models.RetrieveResult{mkRes("a", 3, "emergent_concept")}
	extras := []models.RetrieveResult{mkRes("rr", 0, "leaf")}
	got := ApplyReverseRefQuota(in, extras, 5, ReverseRefQuotaCfg{Enabled: false, MinSlots: 1})
	if len(got) != 1 || got[0].NodeID != "a" {
		t.Errorf("disabled should passthrough; got %+v", got)
	}
}

func TestApplyReverseRefQuota_PromotesFromExtras(t *testing.T) {
	in := []models.RetrieveResult{
		mkRes("e1", 3, "emergent_concept"),
		mkRes("e2", 4, "emergent_concept"),
	}
	extras := []models.RetrieveResult{
		mkRes("rr1", 0, "leaf"),
		mkRes("rr2", 0, "leaf"),
	}
	got := ApplyReverseRefQuota(in, extras, 5, ReverseRefQuotaCfg{Enabled: true, MinSlots: 2})
	if len(got) < 2 {
		t.Fatalf("expected pool ≥2; got %d", len(got))
	}
	if got[0].NodeID != "rr1" || got[1].NodeID != "rr2" {
		t.Errorf("expected [rr1 rr2] at head; got [%s %s]", got[0].NodeID, got[1].NodeID)
	}
}

func TestApplyReverseRefQuota_AlreadySatisfiedNoOp(t *testing.T) {
	in := []models.RetrieveResult{
		mkRes("rr1", 0, "leaf"),
		mkRes("e1", 3, "emergent_concept"),
	}
	extras := []models.RetrieveResult{mkRes("rr1", 0, "leaf")}
	got := ApplyReverseRefQuota(in, extras, 5, ReverseRefQuotaCfg{Enabled: true, MinSlots: 1})
	if got[0].NodeID != "rr1" {
		t.Errorf("no-op expected; got %+v", got)
	}
}

func TestApplyReverseRefQuota_EmptyExtras(t *testing.T) {
	in := []models.RetrieveResult{mkRes("a", 3, "emergent_concept")}
	got := ApplyReverseRefQuota(in, nil, 5, ReverseRefQuotaCfg{Enabled: true, MinSlots: 1})
	if len(got) != 1 || got[0].NodeID != "a" {
		t.Errorf("empty extras should passthrough; got %+v", got)
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
}
