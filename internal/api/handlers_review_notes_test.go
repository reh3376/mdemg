package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"mdemg/internal/tsdb"
)

// capturingPool records the values written via CopyFrom so tests can assert
// per-column contents. Extends the fakeReviewPool shape with row capture.
type capturingPool struct {
	rows [][]any
	cols []string
}

func (p *capturingPool) CopyFrom(_ context.Context, _ pgx.Identifier, cols []string, src pgx.CopyFromSource) (int64, error) {
	p.cols = cols
	var n int64
	for src.Next() {
		vals, err := src.Values()
		if err != nil {
			return n, err
		}
		p.rows = append(p.rows, vals)
		n++
	}
	return n, nil
}

func (p *capturingPool) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row { return noRow{} }
func (p *capturingPool) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

// TestReview_Grade_NotesRoundTrip pins REVIEW-GRADE-NOTES-FIELD-001:
// a grade submitted with a `notes` field lands on the notes column of the
// review_grades write. Pre-fix, the field didn't exist and readJSON's
// DisallowUnknownFields would 400 the request.
func TestReview_Grade_NotesRoundTrip(t *testing.T) {
	s, _ := reviewTestServer(t, true)
	// Rebuild the writer with a capturing pool so we can assert per-column.
	cap := &capturingPool{}
	s.reviewWriter = tsdb.NewReviewGradesWriter(cap, 0, 0)

	rec := httptest.NewRecorder()
	body := `{"dataset_id":"stub","item_id":"stub-1","grader_id":"rh","dimensions":{"quality":4},"notes":"low confidence — verdict axis ambiguous"}`
	s.handleReviewGrade(rec, httptest.NewRequest(http.MethodPost, "/v1/review/grade", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("grade with notes → %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	s.reviewWriter.Close()

	if len(cap.rows) != 1 {
		t.Fatalf("expected 1 row captured, got %d", len(cap.rows))
	}
	// Find the notes column index.
	notesIdx := -1
	for i, c := range cap.cols {
		if c == "notes" {
			notesIdx = i
			break
		}
	}
	if notesIdx < 0 {
		t.Fatalf("notes column absent from CopyFrom column list: %v", cap.cols)
	}
	got, _ := cap.rows[0][notesIdx].(string)
	if got != "low confidence — verdict axis ambiguous" {
		t.Errorf("notes value did not round-trip: got %q", got)
	}
}

// TestReadJSON_UnknownFieldSurfacesName pins REVIEW-GRADE-NOTES-FIELD-001
// pt2: readJSON's DisallowUnknownFields error surfaces the offending field
// name in the response body (previously opaque "invalid request body").
// This is the cross-handler win — every readJSON caller benefits.
func TestReadJSON_UnknownFieldSurfacesName(t *testing.T) {
	s, _ := reviewTestServer(t, true)
	rec := httptest.NewRecorder()
	body := `{"dataset_id":"stub","item_id":"stub-1","grader_id":"rh","dimensions":{"quality":4},"totally_unknown_key":42}`
	s.handleReviewGrade(rec, httptest.NewRequest(http.MethodPost, "/v1/review/grade", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown-field request → %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "totally_unknown_key") {
		t.Errorf("400 response should name the offending field; got: %s", rec.Body.String())
	}
	// Sanity: the old opaque message must NOT be the whole body.
	if strings.TrimSpace(rec.Body.String()) == `{"error":"invalid request body"}` {
		t.Errorf("400 response is still opaque; field name should be surfaced")
	}
}
