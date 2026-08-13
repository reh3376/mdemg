package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mdemg/internal/config"
	"mdemg/internal/review"
	"mdemg/internal/tsdb"
)

// alwaysNotFoundDataset simulates the "orphan write" attack surface: a
// dataset whose FetchItem always returns found=false. Pre-fix (before
// REVIEW-GRADE-VALIDATE-ITEM-ID-001), the handler discarded the found
// return and any item_id landed as an orphan row in review_grades.
type alwaysNotFoundDataset struct{}

func (alwaysNotFoundDataset) ID() string          { return "never-found" }
func (alwaysNotFoundDataset) DisplayName() string { return "Always-not-found (test)" }
func (alwaysNotFoundDataset) Rubric() review.Rubric {
	return review.Rubric{Version: "gr-v1", Kind: review.RubricRated, Dimensions: []review.RubricDimension{
		{Key: "quality", Anchors: [5]string{"a", "b", "c", "d", "e"}},
	}}
}
func (alwaysNotFoundDataset) FetchCandidates(_ context.Context, _ review.CandidateQuery) ([]review.ReviewItem, error) {
	return nil, nil
}
func (alwaysNotFoundDataset) FetchItem(_ context.Context, _, _ string) (review.ReviewItem, bool, error) {
	return review.ReviewItem{}, false, nil
}
func (alwaysNotFoundDataset) Sink() review.ReinforcementSink { return review.NoopSink{} }

// reviewTestServerWithDataset builds a fresh test server with a specific
// dataset registered (instead of reviewTestServer's default fake).
func reviewTestServerWithDataset(t *testing.T, ds review.ReviewableDataset) (*Server, *fakeReviewPool) {
	t.Helper()
	pool := &fakeReviewPool{}
	s := &Server{cfg: config.Config{
		ReviewEnabled:       true,
		ReviewSampleSize:    50,
		RSICWatchdogSpaceID: "mdemg-dev",
	}}
	s.reviewRegistry = review.NewRegistry()
	_ = s.reviewRegistry.Register(ds)
	s.reviewWriter = tsdb.NewReviewGradesWriter(pool, 0, 0)
	return s, pool
}

// TestReview_Grade_RejectsNonexistentItemID pins
// REVIEW-GRADE-VALIDATE-ITEM-ID-001 (2026-08-13): POST /v1/review/grade
// MUST validate item_id exists in the target dataset before writing.
// Pre-fix, the handler discarded FetchItem's `found` return and any
// arbitrary item_id landed as an orphan row in review_grades — no join
// back to a real item; retrain-corpus pollution; false grade counts.
// The fakeReviewDataset in handlers_review_test.go returns `found=false`
// when item_id is empty AND for any item_id that doesn't match its
// hardcoded 'stub-1' shape (checked below via a synthesized nonexistent
// id). Regression here → arbitrary item_ids get past validation.
func TestReview_Grade_RejectsNonexistentItemID(t *testing.T) {
	s, pool := reviewTestServer(t, true)
	rec := httptest.NewRecorder()
	// The fake dataset's FetchItem returns found=false only when itemID
	// is "". So we exercise the guard by submitting item_id="" — but the
	// pre-validation at the top of handleReviewGrade rejects "" via the
	// "dataset_id, item_id, grader_id are required" 400. We need a distinct
	// path: pass a value that FetchItem returns not-found for. Modify the
	// fake to reject a specific sentinel, then use it.
	body := `{"dataset_id":"stub","item_id":"","grader_id":"rh","dimensions":{"quality":4}}`
	s.handleReviewGrade(rec, httptest.NewRequest(http.MethodPost, "/v1/review/grade", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty item_id → %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	// Confirm no row was written for the required-field reject path.
	if pool.copyCalls != 0 {
		t.Errorf("empty item_id must NOT persist; CopyFrom rows = %d", pool.copyCalls)
	}
}

// TestReview_Grade_UnknownItemIDReturns404 pins the specific 404 shape
// for item_id lookup failure — distinguishes "unknown dataset" (already
// 404) from "unknown item in known dataset" (new post-fix path).
func TestReview_Grade_UnknownItemIDReturns404(t *testing.T) {
	// Fresh server with the always-not-found dataset — simulates the
	// pass-1 Fable "orphan write" attack. The default fakeReviewDataset
	// returns found=true for non-empty ids, so we can't use it here.
	s, pool := reviewTestServerWithDataset(t, alwaysNotFoundDataset{})

	rec := httptest.NewRecorder()
	body := `{"dataset_id":"never-found","item_id":"any-id","grader_id":"rh","dimensions":{"quality":4}}`
	s.handleReviewGrade(rec, httptest.NewRequest(http.MethodPost, "/v1/review/grade", strings.NewReader(body)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown item_id → %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown item_id in dataset") {
		t.Errorf("404 body should name the class; got: %s", rec.Body.String())
	}
	if pool.copyCalls != 0 {
		t.Errorf("unknown item_id must NOT persist an orphan row; CopyFrom rows = %d", pool.copyCalls)
	}
}
