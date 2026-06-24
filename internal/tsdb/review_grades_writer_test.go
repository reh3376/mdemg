package tsdb

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeReviewPool satisfies reviewPoolIface: captures CopyFrom + stubs
// QueryRow/Exec for the unit-level buffered-path tests.
type fakeReviewPool struct {
	mu    sync.Mutex
	calls []fakeCopyFromCall
}

func (p *fakeReviewPool) CopyFrom(_ context.Context, tableName pgx.Identifier, cols []string, src pgx.CopyFromSource) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var rows [][]any
	for src.Next() {
		v, err := src.Values()
		if err != nil {
			return 0, err
		}
		rows = append(rows, v)
	}
	p.calls = append(p.calls, fakeCopyFromCall{tableName: tableName, columns: cols, rows: rows})
	return int64(len(rows)), nil
}

// QueryRow returns a row that scans to pgx.ErrNoRows (empty-table case).
func (p *fakeReviewPool) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return errNoRowsRow{}
}

func (p *fakeReviewPool) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (p *fakeReviewPool) lastCall() fakeCopyFromCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.calls) == 0 {
		return fakeCopyFromCall{}
	}
	return p.calls[len(p.calls)-1]
}

func (p *fakeReviewPool) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

type errNoRowsRow struct{}

func (errNoRowsRow) Scan(_ ...any) error { return pgx.ErrNoRows }

func sampleGradeRow() ReviewGradeRow {
	return ReviewGradeRow{
		GradeID:              "grade-cuid-0001",
		DatasetID:            "guidance",
		ItemID:               "row-1",
		SpaceID:              "mdemg-dev",
		GoldScore:            0.75,
		GoldDimensionsJSON:   `{"relevance":4,"actionability":3}`,
		GraderID:             "rh",
		RubricVersion:        "gr-v1",
		ReinforcementApplied: true,
		ReinforcementDetailJSON: `{"sink_id":"guidance","verb":"guidance_outcome:followed"}`,
	}
}

func TestReviewGradesWriter_RecordThenFlush(t *testing.T) {
	pool := &fakeReviewPool{}
	w := NewReviewGradesWriter(pool, time.Hour, 0)
	defer w.Close()
	for range 3 {
		w.Record(sampleGradeRow())
	}
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if pool.callCount() != 1 {
		t.Fatalf("expected 1 CopyFrom, got %d", pool.callCount())
	}
	last := pool.lastCall()
	if last.tableName[0] != "review_grades" {
		t.Errorf("table = %q, want review_grades", last.tableName[0])
	}
	if len(last.columns) != 16 {
		t.Errorf("got %d columns, want 16", len(last.columns))
	}
	if len(last.rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(last.rows))
	}
	if st := w.Stats(); st.SuccessCount != 1 || st.TotalRows != 3 {
		t.Errorf("stats = %+v", st)
	}
}

func TestReviewGradesWriter_NullableJSONAndIDs(t *testing.T) {
	pool := &fakeReviewPool{}
	w := NewReviewGradesWriter(pool, time.Hour, 0)
	defer w.Close()
	// Gold-only grade: no reinforcement detail, no reversal target.
	r := sampleGradeRow()
	r.ReinforcementApplied = false
	r.ReinforcementDetailJSON = ""
	r.ReversesGradeID = ""
	r.InstanceID = ""
	w.Record(r)
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	row := pool.lastCall().rows[0]
	// reinforcement_detail=11, reverses_grade_id=13, instance_id=14 → NULL.
	for _, idx := range []int{11, 13, 14} {
		if row[idx] != nil {
			t.Errorf("column %d should be nil for empty input, got %v", idx, row[idx])
		}
	}
	// gold_dimensions=6 is NOT NULL — always present.
	if row[6] == nil {
		t.Error("gold_dimensions must not be nil")
	}
}

func TestReviewGradesWriter_BufferEviction(t *testing.T) {
	pool := &fakeReviewPool{}
	w := NewReviewGradesWriter(pool, time.Hour, 3)
	defer w.Close()
	for i := range 5 {
		r := sampleGradeRow()
		r.GradeID = "g-" + string(rune('0'+i))
		w.Record(r)
	}
	if got := w.Stats().OverflowCount; got != 2 {
		t.Errorf("dropped = %d, want 2", got)
	}
}

func TestReviewGradesWriter_LatestGradeForItem_NoRows(t *testing.T) {
	pool := &fakeReviewPool{}
	w := NewReviewGradesWriter(pool, time.Hour, 0)
	defer w.Close()
	got, err := w.LatestGradeForItem(context.Background(), "guidance", "missing")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for no rows, got %+v", got)
	}
}
