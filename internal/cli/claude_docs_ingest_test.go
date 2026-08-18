// Sprint CLAUDE-DOCS-INGEST-001 — unit tests for the corpus-parser + payload-builder
// + endpoint plumbing. Live e2e (Tier 3) run separately against real /v1/memory/ingest.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeDocsPathSlug(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Turn on screen reader mode", "turn-on-screen-reader-mode"},
		{"query() vs ClaudeSDKClient", "query-vs-claudesdkclient"},
		{"", "unnamed"},
		{"   ", "unnamed"},
		{"McpServerStatusConfig", "mcpserverstatusconfig"},
	}
	for _, tc := range cases {
		got := claudeDocsPathSlug(tc.in)
		if got != tc.want {
			t.Errorf("claudeDocsPathSlug(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
	// Length cap: any oversized input must produce ≤80 chars.
	long := "a very very very very very very very very very very very very very very very very long section header that exceeds eighty chars"
	got := claudeDocsPathSlug(long)
	if len(got) > 80 {
		t.Errorf("length cap violated: %q (len=%d)", got, len(got))
	}
}

func TestBuildClaudeDocsIngestRequest(t *testing.T) {
	row := claudeDocsRow{
		RowID:         "cd_test_001",
		Prompt:        "What is EffortLevel?",
		Completion:    `EffortLevel = Literal["low","medium","high","xhigh","max"]`,
		SourceURL:     "https://code.claude.com/docs/en/agent-sdk/python",
		SourceSHA256:  "abc123",
		SourceSlug:    "agent-sdk--python",
		DocTitle:      "Agent SDK reference - Python",
		SectionHeader: "EffortLevel type",
		ConceptType:   "h3",
		SectionIndex:  7,
	}
	got := buildClaudeDocsIngestRequest(row, "test-space")

	if s, _ := got["space_id"].(string); s != "test-space" {
		t.Errorf("space_id=%q, want test-space", s)
	}
	if s, _ := got["source"].(string); s != "claude-docs-ingest" {
		t.Errorf("source=%q, want claude-docs-ingest", s)
	}
	wantPath := "claude-docs/agent-sdk--python/007__effortlevel-type"
	if s, _ := got["path"].(string); s != wantPath {
		t.Errorf("path=%q, want %q", s, wantPath)
	}
	if s, _ := got["content"].(string); !strings.Contains(s, row.Prompt) || !strings.Contains(s, row.Completion) {
		t.Errorf("content missing prompt or completion: %q", s)
	}
	if s, _ := got["content_hash"].(string); len(s) != 64 { // sha256 hex
		t.Errorf("content_hash len=%d, want 64", len(s))
	}
	tags, _ := got["tags"].([]string)
	haveDoc, haveSlug, haveType := false, false, false
	for _, tg := range tags {
		if tg == "docs:claude-code" {
			haveDoc = true
		}
		if tg == "docs:agent-sdk--python" {
			haveSlug = true
		}
		if tg == "docs:concept:h3" {
			haveType = true
		}
	}
	if !haveDoc || !haveSlug || !haveType {
		t.Errorf("tags missing expected members: %v (doc=%v slug=%v type=%v)", tags, haveDoc, haveSlug, haveType)
	}
}

func TestReadClaudeDocsCorpus_Basic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "qa.jsonl")
	content := `{"row_id":"a","prompt":"q1","completion":"a1","source_slug":"s","section_header":"h","section_index":0}
{"row_id":"b","prompt":"q2","completion":"a2","source_slug":"s","section_header":"h","section_index":1}
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	rows, err := readClaudeDocsCorpus(p, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].RowID != "a" || rows[1].RowID != "b" {
		t.Errorf("row order wrong: %+v", rows)
	}
}

func TestReadClaudeDocsCorpus_LimitRespected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "qa.jsonl")
	content := `{"row_id":"a","prompt":"q1","completion":"a1"}
{"row_id":"b","prompt":"q2","completion":"a2"}
{"row_id":"c","prompt":"q3","completion":"a3"}
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	rows, err := readClaudeDocsCorpus(p, 2)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("limit=2 gave %d rows", len(rows))
	}
}

func TestReadClaudeDocsCorpus_RejectsMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "qa.jsonl")
	content := `{"row_id":"a","prompt":"q1"}` + "\n" // missing completion
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readClaudeDocsCorpus(p, 0); err == nil {
		t.Error("expected error for missing completion; got nil")
	}
}
