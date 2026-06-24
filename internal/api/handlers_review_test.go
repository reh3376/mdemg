package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"mdemg/internal/config"
	"mdemg/internal/review"
	"mdemg/internal/tsdb"
)

// fakeReviewPool satisfies the review writer's pool interface for handler tests
// (CopyFrom captured-and-OK; QueryRow → no rows; Exec → no-op).
type fakeReviewPool struct{ copyCalls int }

func (p *fakeReviewPool) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, src pgx.CopyFromSource) (int64, error) {
	n := 0
	for src.Next() {
		n++
	}
	p.copyCalls += n
	return int64(n), nil
}
func (p *fakeReviewPool) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row { return noRow{} }
func (p *fakeReviewPool) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

type noRow struct{}

func (noRow) Scan(_ ...any) error { return pgx.ErrNoRows }

// fakeReviewDataset is a test-only ReviewableDataset (the prod stub was removed).
type fakeReviewDataset struct{}

func (fakeReviewDataset) ID() string          { return "stub" }
func (fakeReviewDataset) DisplayName() string { return "Stub (test)" }
func (fakeReviewDataset) Rubric() review.Rubric {
	return review.Rubric{Version: "gr-v1", Kind: review.RubricRated, Dimensions: []review.RubricDimension{
		{Key: "quality", Anchors: [5]string{"0", "1", "2", "3", "4"}},
	}}
}
func (fakeReviewDataset) Sink() review.ReinforcementSink { return review.NoopSink{} }
func (fakeReviewDataset) FetchCandidates(_ context.Context, q review.CandidateQuery) ([]review.ReviewItem, error) {
	n := q.Limit
	if n <= 0 || n > 3 {
		n = 3
	}
	out := make([]review.ReviewItem, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, review.ReviewItem{ItemID: "stub-1", Content: "c", Context: "x", AutoLabel: "unknown", AutoScore: 0.5, Stratum: "s"})
	}
	return out, nil
}
func (fakeReviewDataset) FetchItem(_ context.Context, _, itemID string) (review.ReviewItem, bool, error) {
	if itemID == "" {
		return review.ReviewItem{}, false, nil
	}
	return review.ReviewItem{ItemID: itemID, Content: "c", Context: "x", AutoLabel: "unknown", AutoScore: 0.5}, true, nil
}

func reviewTestServer(t *testing.T, enabled bool) (*Server, *fakeReviewPool) {
	t.Helper()
	pool := &fakeReviewPool{}
	s := &Server{cfg: config.Config{
		ReviewEnabled:       enabled,
		ReviewSampleSize:    50,
		RSICWatchdogSpaceID: "mdemg-dev",
	}}
	if enabled {
		s.reviewRegistry = review.NewRegistry()
		_ = s.reviewRegistry.Register(fakeReviewDataset{})
		s.reviewWriter = tsdb.NewReviewGradesWriter(pool, 0, 0)
	}
	return s, pool
}

func TestReview_503WhenDisabled(t *testing.T) {
	s, _ := reviewTestServer(t, false)
	rec := httptest.NewRecorder()
	s.handleReviewDatasets(rec, httptest.NewRequest(http.MethodGet, "/v1/review/datasets", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("disabled → %d, want 503", rec.Code)
	}
}

func TestReview_MethodNotAllowed(t *testing.T) {
	s, _ := reviewTestServer(t, true)
	rec := httptest.NewRecorder()
	s.handleReviewGrade(rec, httptest.NewRequest(http.MethodGet, "/v1/review/grade", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET grade → %d, want 405", rec.Code)
	}
}

func TestReview_GradeValidationAndUnknownDataset(t *testing.T) {
	s, _ := reviewTestServer(t, true)
	// Missing required fields → 400.
	rec := httptest.NewRecorder()
	s.handleReviewGrade(rec, httptest.NewRequest(http.MethodPost, "/v1/review/grade",
		strings.NewReader(`{"dataset_id":"stub"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing fields → %d, want 400", rec.Code)
	}
	// Unknown dataset → 404.
	rec = httptest.NewRecorder()
	s.handleReviewGrade(rec, httptest.NewRequest(http.MethodPost, "/v1/review/grade",
		strings.NewReader(`{"dataset_id":"nope","item_id":"x","grader_id":"rh","dimensions":{"quality":3}}`)))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown dataset → %d, want 404", rec.Code)
	}
}

func TestReview_GradeDryRunDoesNotPersist(t *testing.T) {
	s, pool := reviewTestServer(t, true)
	rec := httptest.NewRecorder()
	body := `{"dataset_id":"stub","item_id":"stub-1","grader_id":"rh","dimensions":{"quality":4},"dry_run":true,"reinforce":true}`
	s.handleReviewGrade(rec, httptest.NewRequest(http.MethodPost, "/v1/review/grade", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("dry-run grade → %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"dry_run":true`) {
		t.Errorf("dry-run response missing dry_run flag: %s", rec.Body.String())
	}
	if pool.copyCalls != 0 {
		t.Errorf("dry-run must NOT persist a row; CopyFrom rows = %d", pool.copyCalls)
	}
}

func TestReview_GradePersists(t *testing.T) {
	s, pool := reviewTestServer(t, true)
	rec := httptest.NewRecorder()
	body := `{"dataset_id":"stub","item_id":"stub-1","grader_id":"rh","dimensions":{"quality":4}}`
	s.handleReviewGrade(rec, httptest.NewRequest(http.MethodPost, "/v1/review/grade", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("grade → %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"grade_id"`) {
		t.Errorf("grade response missing grade_id: %s", rec.Body.String())
	}
	s.reviewWriter.Close() // force flush
	if pool.copyCalls != 1 {
		t.Errorf("grade should persist exactly 1 row; CopyFrom rows = %d", pool.copyCalls)
	}
}
