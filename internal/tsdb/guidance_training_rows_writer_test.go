package tsdb

import (
	"context"
	"errors"
	"strings"
	"regexp"
	"testing"
	"time"
)

// Column index map for guidance_training_rows CopyFrom rows:
// row_id=0, time=1, space_id=2, session_id=3, instance_id=4, guidance_id=5,
// guidance_type=6, guidance_content=7, source_node_id=8, source_role_type=9,
// source_layer=10, action_summary=11, outcome_type=12, similarity=13,
// classifier_source=14, constraint_code=15.

func sampleGuidanceRow() GuidanceTrainingRow {
	l := 3
	return GuidanceTrainingRow{
		SpaceID:          "mdemg-dev",
		SessionID:        "sess-1",
		InstanceID:       "host-mdemg-dev",
		GuidanceID:       "g-123",
		GuidanceType:     "constraint",
		GuidanceContent:  "[must] never hardcode values",
		SourceNodeID:     "node-a",
		SourceRoleType:   "constraint",
		SourceLayer:      &l,
		ActionSummary:    "added a config flag",
		OutcomeType:      "followed",
		Similarity:       0.87,
		ClassifierSource: "llm",
		ConstraintCode:   "no-hardcode",
	}
}

var cuidv2Re = regexp.MustCompile(`^[a-z][a-z0-9]+$`)

func TestGuidanceTrainingWriter_RecordThenFlush_WritesAllRowsAndCUID(t *testing.T) {
	pool := &fakePool{}
	w := NewGuidanceTrainingRowsWriter(pool, time.Hour, 0)
	defer w.Close()

	for range 3 {
		w.Record(sampleGuidanceRow())
	}
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
	if pool.callCount() != 1 {
		t.Fatalf("expected 1 CopyFrom call, got %d", pool.callCount())
	}
	last := pool.lastCall()
	if got := last.tableName[0]; got != "guidance_training_rows" {
		t.Errorf("table name = %q, want guidance_training_rows", got)
	}
	if len(last.columns) != 16 {
		t.Errorf("got %d columns, want 16", len(last.columns))
	}
	if len(last.rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(last.rows))
	}
	// row_id must be a CUIDv2 (not a UUID — no dashes, leading letter).
	rowID, ok := last.rows[0][0].(string)
	if !ok || rowID == "" {
		t.Fatalf("row_id missing/not string: %v", last.rows[0][0])
	}
	if len(rowID) < 20 || !cuidv2Re.MatchString(rowID) {
		t.Errorf("row_id %q is not a CUIDv2 (want no-dash leading-letter alnum ≥20)", rowID)
	}
	stats := w.Stats()
	if stats.SuccessCount != 1 || stats.TotalRows != 3 {
		t.Errorf("stats = %+v, want SuccessCount=1 TotalRows=3", stats)
	}
}

func TestGuidanceTrainingWriter_SourceLayer_NilVsZeroDistinct(t *testing.T) {
	pool := &fakePool{}
	w := NewGuidanceTrainingRowsWriter(pool, time.Hour, 0)
	defer w.Close()

	// Row 1: unresolved layer (nil → NULL).
	r1 := sampleGuidanceRow()
	r1.SourceLayer = nil
	r1.SourceRoleType = "" // unresolved role → NULL
	// Row 2: valid layer 0 (raw L0 node) must serialize as 0, NOT NULL.
	r2 := sampleGuidanceRow()
	zero := 0
	r2.SourceLayer = &zero
	w.Record(r1)
	w.Record(r2)
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}
	rows := pool.lastCall().rows
	if rows[0][10] != nil {
		t.Errorf("row1 source_layer should be nil (unresolved), got %v", rows[0][10])
	}
	if rows[0][9] != nil {
		t.Errorf("row1 source_role_type should be nil (empty), got %v", rows[0][9])
	}
	if rows[1][10] != 0 {
		t.Errorf("row2 source_layer should be 0 (valid L0, NOT NULL), got %v (%T)", rows[1][10], rows[1][10])
	}
}

func TestGuidanceTrainingWriter_RequiredFieldsNotNull(t *testing.T) {
	pool := &fakePool{}
	w := NewGuidanceTrainingRowsWriter(pool, time.Hour, 0)
	defer w.Close()
	w.Record(sampleGuidanceRow())
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}
	r := pool.lastCall().rows[0]
	// space_id(2), guidance_id(5), outcome_type(12), classifier_source(14) are NOT NULL.
	for _, idx := range []int{2, 5, 12, 14} {
		if r[idx] == nil {
			t.Errorf("required column %d must not be nil", idx)
		}
	}
	// classifier_source "" is allowed (NOT NULL DEFAULT '') — passed raw, not nullable.
	if r[14] != "llm" {
		t.Errorf("classifier_source = %v, want llm", r[14])
	}
}

func TestGuidanceTrainingWriter_BufferFull_EvictsAndCountsDrop(t *testing.T) {
	pool := &fakePool{}
	w := NewGuidanceTrainingRowsWriter(pool, time.Hour, 3) // cap = 3
	defer w.Close()
	for i := range 5 {
		row := sampleGuidanceRow()
		row.GuidanceID = "g-" + string(rune('0'+i))
		w.Record(row)
	}
	if got := w.Stats().DroppedRows; got != 2 {
		t.Errorf("dropped = %d, want 2", got)
	}
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}
	rows := pool.lastCall().rows
	if len(rows) != 3 {
		t.Fatalf("flushed %d rows, want 3", len(rows))
	}
	// Oldest two evicted → remaining guidance_ids g-2,g-3,g-4 (column 5).
	want := []string{"g-2", "g-3", "g-4"}
	for i, wv := range want {
		if rows[i][5] != wv {
			t.Errorf("row %d guidance_id = %v, want %q", i, rows[i][5], wv)
		}
	}
}

func TestGuidanceTrainingWriter_FlushError_IncrementsFailure(t *testing.T) {
	pool := &fakePool{returnErr: errors.New("pg conn refused")}
	w := NewGuidanceTrainingRowsWriter(pool, time.Hour, 0)
	defer w.Close()
	w.Record(sampleGuidanceRow())
	if err := w.Flush(context.Background()); err == nil {
		t.Fatal("expected Flush to return the underlying error")
	}
	if got := w.Stats().FailureCount; got != 1 {
		t.Errorf("FailureCount = %d, want 1", got)
	}
}

func TestGuidanceTrainingWriter_Close_DrainsBuffer(t *testing.T) {
	pool := &fakePool{}
	w := NewGuidanceTrainingRowsWriter(pool, time.Hour, 0)
	for range 4 {
		w.Record(sampleGuidanceRow())
	}
	w.Close()
	if pool.callCount() != 1 {
		t.Fatalf("Close should final-flush; CopyFrom calls = %d", pool.callCount())
	}
	if len(pool.lastCall().rows) != 4 {
		t.Errorf("final-flush rows = %d, want 4", len(pool.lastCall().rows))
	}
}


// EXPORT-SCRUB-INTAKE-001 (2026-08-11) pin tests: assert Record scrubs
// action_summary + guidance_content at intake time per the writer's
// contract with the exporter (block-on-scrub-diff gate).

func TestGuidanceTrainingWriter_Record_ScrubsAbsPathInActionSummary(t *testing.T) {
	pool := &fakePool{}
	w := NewGuidanceTrainingRowsWriter(pool, time.Hour, 0)
	defer w.Close()

	row := sampleGuidanceRow()
	row.ActionSummary = "Ran command in /Users/tester/workspace/myrepo/pkg/foo.go and saw output"
	w.Record(row)
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}
	// action_summary is column 11 (per top-of-file comment).
	got, ok := pool.lastCall().rows[0][11].(string)
	if !ok {
		t.Fatalf("action_summary not string: %T", pool.lastCall().rows[0][11])
	}
	if got == "" {
		t.Fatal("action_summary empty after Record")
	}
	// Post-scrub: /Users/tester/workspace/myrepo/pkg/foo.go should be
	// replaced by /[PATH]/... — the raw username + workspace-parent stripped.
	if got == row.ActionSummary {
		t.Fatalf("action_summary was NOT scrubbed at intake — got raw %q; the export gate will re-block this class if intake doesn't scrub", got)
	}
	if !strings.Contains(got, "[PATH]") {
		t.Fatalf("scrubbed action_summary should contain [PATH] marker, got %q", got)
	}
	if strings.Contains(got, "/Users/tester/workspace/myrepo") {
		t.Fatalf("scrubbed action_summary still contains raw workspace path: %q", got)
	}
}

func TestGuidanceTrainingWriter_Record_PreservesAbsPathInGuidanceContent(t *testing.T) {
	pool := &fakePool{}
	w := NewGuidanceTrainingRowsWriter(pool, time.Hour, 0)
	defer w.Close()

	row := sampleGuidanceRow()
	// Operator-authored constraint that legitimately references a workspace
	// path. GuidanceContent is skipped for abs_path per the export spec, so
	// intake must preserve it (else the operator's rule loses semantic
	// context in every stored row).
	row.GuidanceContent = "Never edit /Users/reh3376/mdemg/CLAUDE.md without running lint first"
	w.Record(row)
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}
	// guidance_content is column 7.
	got, ok := pool.lastCall().rows[0][7].(string)
	if !ok {
		t.Fatalf("guidance_content not string: %T", pool.lastCall().rows[0][7])
	}
	if got != row.GuidanceContent {
		t.Fatalf("guidance_content was scrubbed at intake — abs_path SHOULD be preserved per the export spec's skip list. got %q want %q", got, row.GuidanceContent)
	}
}

func TestGuidanceTrainingWriter_Record_StillScrubsApiKeyInGuidanceContent(t *testing.T) {
	// GuidanceContent skips abs_path ONLY — api_key, email, etc still scrub.
	// Assert the skip list is targeted, not blanket-off.
	pool := &fakePool{}
	w := NewGuidanceTrainingRowsWriter(pool, time.Hour, 0)
	defer w.Close()

	row := sampleGuidanceRow()
	row.GuidanceContent = "The OpenAI API key sk-abc1234567890abcdefghij must never appear in code"
	w.Record(row)
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}
	got, _ := pool.lastCall().rows[0][7].(string)
	if strings.Contains(got, "sk-abc1234567890abcdefghij") {
		t.Fatalf("guidance_content STILL contains raw api key after scrub — skip list must not disable api_key: %q", got)
	}
	if !strings.Contains(got, "[REDACTED_KEY]") {
		t.Fatalf("api key not properly redacted in guidance_content: %q", got)
	}
}
