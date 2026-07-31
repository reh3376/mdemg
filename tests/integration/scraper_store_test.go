//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/embeddings"
	"mdemg/internal/scraper"
)

// mockConvSvc satisfies scraper.ConversationService for Reviewer tests.
// It creates a real MemoryNode in Neo4j so that CreateIngestedAsRelationship
// can MATCH the node and SET status = 'ingested'.
type mockConvSvc struct {
	driver neo4j.DriverWithContext
}

func (m *mockConvSvc) Observe(ctx context.Context, _ scraper.ObserveParams) (*scraper.ObserveResult, error) {
	nodeID := "mock-node-" + uuid.New().String()
	if m.driver != nil {
		sess := m.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer sess.Close(ctx)
		_, err := sess.Run(ctx,
			`MERGE (n:MemoryNode {node_id: $nodeID}) SET n.space_id = 'test-scraper-space', n.content = 'mock'`,
			map[string]any{"nodeID": nodeID})
		if err != nil {
			return nil, fmt.Errorf("mockConvSvc: failed to create MemoryNode: %w", err)
		}
	}
	return &scraper.ObserveResult{NodeID: nodeID}, nil
}

// cleanupScrapeData removes all ScrapedContent and ScrapeJob nodes for a given jobID.
func cleanupScrapeData(t *testing.T, driver neo4j.DriverWithContext, jobID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	queries := []string{
		`MATCH (n:ScrapedContent {job_id: $jobID}) DETACH DELETE n`,
		`MATCH (n:ScrapeJob {job_id: $jobID}) DETACH DELETE n`,
	}
	for _, q := range queries {
		_, err := sess.Run(ctx, q, map[string]any{"jobID": jobID})
		if err != nil {
			t.Logf("cleanupScrapeData: query failed (non-fatal): %v", err)
		}
	}
}

// newTestJob builds a minimal ScrapeJob for tests.
func newTestJob(jobID string) *scraper.ScrapeJob {
	now := time.Now().UTC()
	return &scraper.ScrapeJob{
		JobID:         jobID,
		Status:        scraper.StatusPending,
		URLs:          []string{"https://example.com/page1"},
		TargetSpaceID: "test-scraper-space",
		TotalURLs:     1,
		ProcessedURLs: 0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// newTestContent builds a minimal ScrapedContent for tests.
func newTestContent(contentID, jobID string) *scraper.ScrapedContent {
	return &scraper.ScrapedContent{
		ContentID:      contentID,
		JobID:          jobID,
		URL:            "https://example.com/page1",
		Title:          "Test Page",
		Content:        "This is test content for the scraper integration tests.",
		ContentPreview: "This is test content",
		ContentHash:    fmt.Sprintf("hash-%s", contentID),
		QualityScore:   0.85,
		SuggestedTags:  []string{"test", "integration"},
		Summary:        "A test page summary.",
		Status:         scraper.ContentPendingReview,
		WordCount:      10,
	}
}

// ─── Store Tests ───

func TestStore_CreateAndGetScrapeJob(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)
	t.Cleanup(func() { cleanupScrapeData(t, driver, jobID) })

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	got, err := store.GetScrapeJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetScrapeJob failed: %v", err)
	}

	if got.JobID != jobID {
		t.Errorf("JobID: got %q, want %q", got.JobID, jobID)
	}
	if got.Status != scraper.StatusPending {
		t.Errorf("Status: got %q, want %q", got.Status, scraper.StatusPending)
	}
	if got.TargetSpaceID != job.TargetSpaceID {
		t.Errorf("TargetSpaceID: got %q, want %q", got.TargetSpaceID, job.TargetSpaceID)
	}
	if got.TotalURLs != job.TotalURLs {
		t.Errorf("TotalURLs: got %d, want %d", got.TotalURLs, job.TotalURLs)
	}
	if len(got.URLs) != len(job.URLs) {
		t.Errorf("URLs len: got %d, want %d", len(got.URLs), len(job.URLs))
	}
}

func TestStore_UpdateScrapeJobStatus(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)
	t.Cleanup(func() { cleanupScrapeData(t, driver, jobID) })

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	if err := store.UpdateScrapeJobStatus(ctx, jobID, scraper.StatusRunning, 1); err != nil {
		t.Fatalf("UpdateScrapeJobStatus failed: %v", err)
	}

	got, err := store.GetScrapeJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetScrapeJob failed: %v", err)
	}

	if got.Status != scraper.StatusRunning {
		t.Errorf("Status: got %q, want %q", got.Status, scraper.StatusRunning)
	}
	if got.ProcessedURLs != 1 {
		t.Errorf("ProcessedURLs: got %d, want 1", got.ProcessedURLs)
	}
}

func TestStore_SetScrapeJobError(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)
	t.Cleanup(func() { cleanupScrapeData(t, driver, jobID) })

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	errMsg := "network timeout after 30s"
	if err := store.SetScrapeJobError(ctx, jobID, errMsg); err != nil {
		t.Fatalf("SetScrapeJobError failed: %v", err)
	}

	got, err := store.GetScrapeJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetScrapeJob failed: %v", err)
	}

	if got.Error != errMsg {
		t.Errorf("Error: got %q, want %q", got.Error, errMsg)
	}
}

func TestStore_SaveAndGetScrapedContent(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)
	t.Cleanup(func() { cleanupScrapeData(t, driver, jobID) })

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	contentID := "test-content-" + uuid.New().String()
	content := newTestContent(contentID, jobID)

	if err := store.SaveScrapedContent(ctx, content); err != nil {
		t.Fatalf("SaveScrapedContent failed: %v", err)
	}

	got, err := store.GetScrapedContent(ctx, contentID)
	if err != nil {
		t.Fatalf("GetScrapedContent failed: %v", err)
	}

	if got.ContentID != contentID {
		t.Errorf("ContentID: got %q, want %q", got.ContentID, contentID)
	}
	if got.JobID != jobID {
		t.Errorf("JobID: got %q, want %q", got.JobID, jobID)
	}
	if got.URL != content.URL {
		t.Errorf("URL: got %q, want %q", got.URL, content.URL)
	}
	if got.Title != content.Title {
		t.Errorf("Title: got %q, want %q", got.Title, content.Title)
	}
	if got.Status != scraper.ContentPendingReview {
		t.Errorf("Status: got %q, want %q", got.Status, scraper.ContentPendingReview)
	}
	if got.QualityScore != content.QualityScore {
		t.Errorf("QualityScore: got %f, want %f", got.QualityScore, content.QualityScore)
	}
}

func TestStore_GetScrapedContents(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)
	t.Cleanup(func() { cleanupScrapeData(t, driver, jobID) })

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	// Save 3 content items
	for i := 0; i < 3; i++ {
		contentID := fmt.Sprintf("test-content-%d-%s", i, uuid.New().String())
		c := newTestContent(contentID, jobID)
		c.URL = fmt.Sprintf("https://example.com/page%d", i)
		if err := store.SaveScrapedContent(ctx, c); err != nil {
			t.Fatalf("SaveScrapedContent[%d] failed: %v", i, err)
		}
	}

	contents, err := store.GetScrapedContents(ctx, jobID)
	if err != nil {
		t.Fatalf("GetScrapedContents failed: %v", err)
	}

	if len(contents) != 3 {
		t.Errorf("contents len: got %d, want 3", len(contents))
	}
}

func TestStore_UpdateContentStatus(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)
	t.Cleanup(func() { cleanupScrapeData(t, driver, jobID) })

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	contentID := "test-content-" + uuid.New().String()
	content := newTestContent(contentID, jobID)

	if err := store.SaveScrapedContent(ctx, content); err != nil {
		t.Fatalf("SaveScrapedContent failed: %v", err)
	}

	if err := store.UpdateContentStatus(ctx, contentID, scraper.ContentApproved); err != nil {
		t.Fatalf("UpdateContentStatus failed: %v", err)
	}

	got, err := store.GetScrapedContent(ctx, contentID)
	if err != nil {
		t.Fatalf("GetScrapedContent failed: %v", err)
	}

	if got.Status != scraper.ContentApproved {
		t.Errorf("Status: got %q, want %q", got.Status, scraper.ContentApproved)
	}
}

func TestStore_UpdateContentBody(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)
	t.Cleanup(func() { cleanupScrapeData(t, driver, jobID) })

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	contentID := "test-content-" + uuid.New().String()
	content := newTestContent(contentID, jobID)

	if err := store.SaveScrapedContent(ctx, content); err != nil {
		t.Fatalf("SaveScrapedContent failed: %v", err)
	}

	newBody := "Updated content body after editorial review."
	newTags := []string{"updated", "editorial"}

	if err := store.UpdateScrapedContentContent(ctx, contentID, newBody, newTags); err != nil {
		t.Fatalf("UpdateScrapedContentContent failed: %v", err)
	}

	got, err := store.GetScrapedContent(ctx, contentID)
	if err != nil {
		t.Fatalf("GetScrapedContent failed: %v", err)
	}

	if got.Content != newBody {
		t.Errorf("Content: got %q, want %q", got.Content, newBody)
	}
	if len(got.SuggestedTags) != len(newTags) {
		t.Errorf("SuggestedTags len: got %d, want %d", len(got.SuggestedTags), len(newTags))
	}
}

func TestStore_ListScrapeJobs(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	jobID1 := "test-job-" + uuid.New().String()
	jobID2 := "test-job-" + uuid.New().String()
	t.Cleanup(func() {
		cleanupScrapeData(t, driver, jobID1)
		cleanupScrapeData(t, driver, jobID2)
	})

	job1 := newTestJob(jobID1)
	job2 := newTestJob(jobID2)
	job2.URLs = []string{"https://example.com/other"}

	if err := store.CreateScrapeJob(ctx, job1); err != nil {
		t.Fatalf("CreateScrapeJob job1 failed: %v", err)
	}
	if err := store.CreateScrapeJob(ctx, job2); err != nil {
		t.Fatalf("CreateScrapeJob job2 failed: %v", err)
	}

	jobs, err := store.ListScrapeJobs(ctx)
	if err != nil {
		t.Fatalf("ListScrapeJobs failed: %v", err)
	}

	// There may be pre-existing jobs; we only require at least 2 are returned.
	if len(jobs) < 2 {
		t.Errorf("ListScrapeJobs: got %d jobs, want at least 2", len(jobs))
	}

	// Verify both our jobs appear
	found := map[string]bool{}
	for _, j := range jobs {
		found[j.JobID] = true
	}
	if !found[jobID1] {
		t.Errorf("jobID1 %q not found in list", jobID1)
	}
	if !found[jobID2] {
		t.Errorf("jobID2 %q not found in list", jobID2)
	}
}

func TestStore_CountPendingReview(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)
	t.Cleanup(func() { cleanupScrapeData(t, driver, jobID) })

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	// Create 3 pending_review + 1 approved
	for i := 0; i < 3; i++ {
		contentID := fmt.Sprintf("test-content-pending-%d-%s", i, uuid.New().String())
		c := newTestContent(contentID, jobID)
		c.Status = scraper.ContentPendingReview
		if err := store.SaveScrapedContent(ctx, c); err != nil {
			t.Fatalf("SaveScrapedContent[%d] failed: %v", i, err)
		}
	}
	// One approved item
	approvedID := "test-content-approved-" + uuid.New().String()
	approved := newTestContent(approvedID, jobID)
	approved.Status = scraper.ContentApproved
	if err := store.SaveScrapedContent(ctx, approved); err != nil {
		t.Fatalf("SaveScrapedContent approved failed: %v", err)
	}

	count, err := store.CountPendingReview(ctx, jobID)
	if err != nil {
		t.Fatalf("CountPendingReview failed: %v", err)
	}

	if count != 3 {
		t.Errorf("CountPendingReview: got %d, want 3", count)
	}
}

func TestStore_CreateIngestedAsRelationship(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)

	nodeID := "test-memnode-" + uuid.New().String()
	spaceID := "test-scraper-space"

	t.Cleanup(func() {
		cleanupScrapeData(t, driver, jobID)
		// Remove the manually created MemoryNode
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sess := driver.NewSession(cleanupCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer sess.Close(cleanupCtx)
		_, _ = sess.Run(cleanupCtx,
			`MATCH (n:MemoryNode {node_id: $nodeID}) DETACH DELETE n`,
			map[string]any{"nodeID": nodeID})
	})

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	contentID := "test-content-" + uuid.New().String()
	content := newTestContent(contentID, jobID)
	if err := store.SaveScrapedContent(ctx, content); err != nil {
		t.Fatalf("SaveScrapedContent failed: %v", err)
	}

	// Create a MemoryNode directly via Cypher. ExecuteWrite commits the
	// transaction synchronously before returning — critical for
	// cross-session read-your-writes. sess.Run inside a deferred-close
	// session commits only when the outer function returns (which is AFTER
	// CreateIngestedAsRelationship runs), so the MATCH in that call could
	// silently return zero rows and the SET clauses would never fire.
	// Fix: SCRAPER-TEST-TX-VISIBILITY-001.
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, runErr := tx.Run(ctx,
			`CREATE (n:MemoryNode {node_id: $nodeID, space_id: $spaceID, content: "test"}) RETURN n`,
			map[string]any{"nodeID": nodeID, "spaceID": spaceID})
		if runErr != nil {
			return nil, runErr
		}
		return res.Consume(ctx)
	})
	if err != nil {
		t.Fatalf("failed to create MemoryNode: %v", err)
	}

	if err := store.CreateIngestedAsRelationship(ctx, contentID, nodeID); err != nil {
		t.Fatalf("CreateIngestedAsRelationship failed: %v", err)
	}

	// Verify the relationship and status update
	got, err := store.GetScrapedContent(ctx, contentID)
	if err != nil {
		t.Fatalf("GetScrapedContent failed: %v", err)
	}

	if got.IngestedNodeID != nodeID {
		t.Errorf("IngestedNodeID: got %q, want %q", got.IngestedNodeID, nodeID)
	}
	if got.Status != "ingested" {
		t.Errorf("Status: got %q, want %q", got.Status, "ingested")
	}
}

func TestStore_ListSpaces(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	spaces, err := store.ListSpaces(ctx)
	if err != nil {
		t.Fatalf("ListSpaces failed: %v", err)
	}

	// Result may be empty (no MemoryNodes with space_id), but must be non-nil
	if spaces == nil {
		// nil is acceptable — the function returns nil when there are no results
		// but not an error; just log
		t.Log("ListSpaces returned nil (no MemoryNode spaces found — acceptable)")
	}
	// No error is the key assertion
}

func TestStore_GetScrapeJob_NotFound(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	_, err := store.GetScrapeJob(ctx, "nonexistent-job-id-"+uuid.New().String())
	if err == nil {
		t.Fatal("expected error for nonexistent job, got nil")
	}
}

func TestStore_GetScrapedContent_NotFound(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	ctx := context.Background()

	_, err := store.GetScrapedContent(ctx, "nonexistent-content-id-"+uuid.New().String())
	if err == nil {
		t.Fatal("expected error for nonexistent content, got nil")
	}
}

// ─── DedupChecker Tests ───

func TestDedupChecker_CheckSimilar(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx := context.Background()

	embedder := embeddings.NewStub()
	checker := scraper.NewDedupChecker(driver, embedder, 0.85)

	// CheckSimilar may return empty results (no matching nodes) — that's fine.
	// The key assertion is that it does NOT return an error.
	similar, err := checker.CheckSimilar(ctx, "test-scraper-space", "test content for dedup checking")
	if err != nil {
		t.Fatalf("CheckSimilar failed: %v", err)
	}

	// similar may be nil or empty — both are valid when no vector index match exists
	t.Logf("CheckSimilar returned %d similar nodes (may be 0 in test environment)", len(similar))
}

// ─── Reviewer Tests ───

func TestReviewer_ProcessReview_Approve(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	convSvc := &mockConvSvc{driver: driver}
	reviewer := scraper.NewReviewer(store, convSvc)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)
	t.Cleanup(func() { cleanupScrapeData(t, driver, jobID) })

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	contentID := "test-content-" + uuid.New().String()
	content := newTestContent(contentID, jobID)
	if err := store.SaveScrapedContent(ctx, content); err != nil {
		t.Fatalf("SaveScrapedContent failed: %v", err)
	}

	decisions := []scraper.ReviewDecision{
		{ContentID: contentID, Action: "approve", SpaceID: "test-scraper-space"},
	}

	resp, err := reviewer.ProcessReview(ctx, jobID, decisions, "test-scraper-space")
	if err != nil {
		t.Fatalf("ProcessReview failed: %v", err)
	}

	if len(resp.Ingested) == 0 {
		t.Error("expected at least one ingested item, got none")
	}
	if resp.Rejected != 0 {
		t.Errorf("Rejected: got %d, want 0", resp.Rejected)
	}

	// Clean up mock MemoryNodes created by convSvc.Observe
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sess := driver.NewSession(cleanupCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer sess.Close(cleanupCtx)
		for _, item := range resp.Ingested {
			_, _ = sess.Run(cleanupCtx,
				`MATCH (n:MemoryNode {node_id: $nodeID}) DETACH DELETE n`,
				map[string]any{"nodeID": item.NodeID})
		}
	})

	// Verify content status was updated to ingested
	got, err := store.GetScrapedContent(ctx, contentID)
	if err != nil {
		t.Fatalf("GetScrapedContent after approve failed: %v", err)
	}
	if got.Status != scraper.ContentIngested {
		t.Errorf("Status after approve: got %q, want %q", got.Status, scraper.ContentIngested)
	}
}

func TestReviewer_ProcessReview_Reject(t *testing.T) {
	driver := SetupTestNeo4j(t)
	store := scraper.NewStore(driver)
	convSvc := &mockConvSvc{driver: driver}
	reviewer := scraper.NewReviewer(store, convSvc)
	ctx := context.Background()

	jobID := "test-job-" + uuid.New().String()
	job := newTestJob(jobID)
	t.Cleanup(func() { cleanupScrapeData(t, driver, jobID) })

	if err := store.CreateScrapeJob(ctx, job); err != nil {
		t.Fatalf("CreateScrapeJob failed: %v", err)
	}

	contentID := "test-content-" + uuid.New().String()
	content := newTestContent(contentID, jobID)
	if err := store.SaveScrapedContent(ctx, content); err != nil {
		t.Fatalf("SaveScrapedContent failed: %v", err)
	}

	decisions := []scraper.ReviewDecision{
		{ContentID: contentID, Action: "reject"},
	}

	resp, err := reviewer.ProcessReview(ctx, jobID, decisions, "test-scraper-space")
	if err != nil {
		t.Fatalf("ProcessReview failed: %v", err)
	}

	if resp.Rejected != 1 {
		t.Errorf("Rejected: got %d, want 1", resp.Rejected)
	}
	if len(resp.Ingested) != 0 {
		t.Errorf("Ingested: got %d, want 0", len(resp.Ingested))
	}

	// Verify content status was updated to rejected
	got, err := store.GetScrapedContent(ctx, contentID)
	if err != nil {
		t.Fatalf("GetScrapedContent after reject failed: %v", err)
	}
	if got.Status != scraper.ContentRejected {
		t.Errorf("Status after reject: got %q, want %q", got.Status, scraper.ContentRejected)
	}
}
