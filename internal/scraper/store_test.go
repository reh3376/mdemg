package scraper

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
)

// --- getStr tests ---

func TestGetStr_Present(t *testing.T) {
	props := map[string]any{"name": "hello"}
	if got := getStr(props, "name"); got != "hello" {
		t.Errorf("getStr = %q, want %q", got, "hello")
	}
}

func TestGetStr_Missing(t *testing.T) {
	props := map[string]any{}
	if got := getStr(props, "name"); got != "" {
		t.Errorf("getStr = %q, want empty", got)
	}
}

func TestGetStr_NonString(t *testing.T) {
	props := map[string]any{"count": 42}
	if got := getStr(props, "count"); got != "" {
		t.Errorf("getStr(int) = %q, want empty", got)
	}
}

func TestGetStr_NilValue(t *testing.T) {
	props := map[string]any{"key": nil}
	if got := getStr(props, "key"); got != "" {
		t.Errorf("getStr(nil) = %q, want empty", got)
	}
}

// --- getInt tests ---

func TestGetInt_Int64(t *testing.T) {
	props := map[string]any{"count": int64(42)}
	if got := getInt(props, "count"); got != 42 {
		t.Errorf("getInt(int64) = %d, want 42", got)
	}
}

func TestGetInt_Int(t *testing.T) {
	props := map[string]any{"count": 7}
	if got := getInt(props, "count"); got != 7 {
		t.Errorf("getInt(int) = %d, want 7", got)
	}
}

func TestGetInt_Float64(t *testing.T) {
	props := map[string]any{"count": float64(3.9)}
	if got := getInt(props, "count"); got != 3 {
		t.Errorf("getInt(float64) = %d, want 3", got)
	}
}

func TestGetInt_Missing(t *testing.T) {
	props := map[string]any{}
	if got := getInt(props, "count"); got != 0 {
		t.Errorf("getInt(missing) = %d, want 0", got)
	}
}

func TestGetInt_StringType(t *testing.T) {
	props := map[string]any{"count": "not a number"}
	if got := getInt(props, "count"); got != 0 {
		t.Errorf("getInt(string) = %d, want 0", got)
	}
}

// --- getFloat tests ---

func TestGetFloat_Float64(t *testing.T) {
	props := map[string]any{"score": float64(0.95)}
	if got := getFloat(props, "score"); got != 0.95 {
		t.Errorf("getFloat(float64) = %f, want 0.95", got)
	}
}

func TestGetFloat_Int64(t *testing.T) {
	props := map[string]any{"score": int64(5)}
	if got := getFloat(props, "score"); got != 5.0 {
		t.Errorf("getFloat(int64) = %f, want 5.0", got)
	}
}

func TestGetFloat_Missing(t *testing.T) {
	props := map[string]any{}
	if got := getFloat(props, "score"); got != 0 {
		t.Errorf("getFloat(missing) = %f, want 0.0", got)
	}
}

func TestGetFloat_StringType(t *testing.T) {
	props := map[string]any{"score": "high"}
	if got := getFloat(props, "score"); got != 0 {
		t.Errorf("getFloat(string) = %f, want 0.0", got)
	}
}

func TestGetFloat_NilValue(t *testing.T) {
	props := map[string]any{"score": nil}
	if got := getFloat(props, "score"); got != 0 {
		t.Errorf("getFloat(nil) = %f, want 0.0", got)
	}
}

// --- scrapeJobFromNode tests ---

func TestScrapeJobFromNode_FullFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	completed := now.Add(5 * time.Minute)
	urlsJSON, _ := json.Marshal([]string{"https://example.com", "https://test.com"})
	optsJSON, _ := json.Marshal(map[string]any{"depth": 2})

	node := dbtype.Node{
		Props: map[string]any{
			"job_id":          "job-123",
			"status":          "completed",
			"target_space_id": "test-space",
			"total_urls":      int64(2),
			"processed_urls":  int64(2),
			"error":           "",
			"urls":            string(urlsJSON),
			"options":         string(optsJSON),
			"created_at":      now,
			"updated_at":      now,
			"completed_at":    completed,
		},
	}

	job := scrapeJobFromNode(node)

	if job.JobID != "job-123" {
		t.Errorf("JobID = %q, want %q", job.JobID, "job-123")
	}
	if job.Status != "completed" {
		t.Errorf("Status = %q, want %q", job.Status, "completed")
	}
	if job.TargetSpaceID != "test-space" {
		t.Errorf("TargetSpaceID = %q, want %q", job.TargetSpaceID, "test-space")
	}
	if job.TotalURLs != 2 {
		t.Errorf("TotalURLs = %d, want 2", job.TotalURLs)
	}
	if job.ProcessedURLs != 2 {
		t.Errorf("ProcessedURLs = %d, want 2", job.ProcessedURLs)
	}
	if len(job.URLs) != 2 {
		t.Errorf("URLs len = %d, want 2", len(job.URLs))
	}
	if job.CreatedAt != now {
		t.Errorf("CreatedAt = %v, want %v", job.CreatedAt, now)
	}
	if job.CompletedAt == nil || *job.CompletedAt != completed {
		t.Errorf("CompletedAt = %v, want %v", job.CompletedAt, completed)
	}
}

func TestScrapeJobFromNode_MinimalFields(t *testing.T) {
	node := dbtype.Node{
		Props: map[string]any{
			"job_id": "job-min",
			"status": "pending",
		},
	}

	job := scrapeJobFromNode(node)

	if job.JobID != "job-min" {
		t.Errorf("JobID = %q, want %q", job.JobID, "job-min")
	}
	if job.TotalURLs != 0 {
		t.Errorf("TotalURLs = %d, want 0", job.TotalURLs)
	}
	if job.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil", job.CompletedAt)
	}
	if job.URLs != nil {
		t.Errorf("URLs = %v, want nil", job.URLs)
	}
}

func TestScrapeJobFromNode_EmptyProps(t *testing.T) {
	node := dbtype.Node{Props: map[string]any{}}
	job := scrapeJobFromNode(node)

	if job.JobID != "" {
		t.Errorf("JobID = %q, want empty", job.JobID)
	}
	if job.Status != "" {
		t.Errorf("Status = %q, want empty", job.Status)
	}
}

// --- scrapedContentFromNode tests ---

func TestScrapedContentFromNode_FullFields(t *testing.T) {
	tagsJSON, _ := json.Marshal([]string{"go", "testing"})
	similarJSON, _ := json.Marshal([]map[string]any{{"node_id": "n1", "score": 0.9}})

	node := dbtype.Node{
		Props: map[string]any{
			"content_id":       "c-123",
			"job_id":           "j-456",
			"url":              "https://example.com/page",
			"title":            "Test Page",
			"content":          "Full content here",
			"content_preview":  "Full content...",
			"content_hash":     "abc123",
			"quality_score":    float64(0.85),
			"summary":          "A test page summary",
			"status":           "pending_review",
			"word_count":       int64(150),
			"ingested_node_id": "",
			"suggested_tags":   string(tagsJSON),
			"similar_existing": string(similarJSON),
		},
	}

	c := scrapedContentFromNode(node)

	if c.ContentID != "c-123" {
		t.Errorf("ContentID = %q, want %q", c.ContentID, "c-123")
	}
	if c.URL != "https://example.com/page" {
		t.Errorf("URL = %q, want %q", c.URL, "https://example.com/page")
	}
	if c.QualityScore != 0.85 {
		t.Errorf("QualityScore = %f, want 0.85", c.QualityScore)
	}
	if c.WordCount != 150 {
		t.Errorf("WordCount = %d, want 150", c.WordCount)
	}
	if len(c.SuggestedTags) != 2 {
		t.Errorf("SuggestedTags len = %d, want 2", len(c.SuggestedTags))
	}
	if len(c.SimilarExisting) != 1 {
		t.Errorf("SimilarExisting len = %d, want 1", len(c.SimilarExisting))
	}
}

func TestScrapedContentFromNode_MinimalFields(t *testing.T) {
	node := dbtype.Node{
		Props: map[string]any{
			"content_id": "c-min",
			"status":     "approved",
		},
	}

	c := scrapedContentFromNode(node)

	if c.ContentID != "c-min" {
		t.Errorf("ContentID = %q, want %q", c.ContentID, "c-min")
	}
	if c.Status != "approved" {
		t.Errorf("Status = %q, want %q", c.Status, "approved")
	}
	if c.QualityScore != 0 {
		t.Errorf("QualityScore = %f, want 0", c.QualityScore)
	}
	if c.SuggestedTags != nil {
		t.Errorf("SuggestedTags = %v, want nil", c.SuggestedTags)
	}
}

func TestScrapedContentFromNode_EmptyProps(t *testing.T) {
	node := dbtype.Node{Props: map[string]any{}}
	c := scrapedContentFromNode(node)

	if c.ContentID != "" {
		t.Errorf("ContentID = %q, want empty", c.ContentID)
	}
	if c.WordCount != 0 {
		t.Errorf("WordCount = %d, want 0", c.WordCount)
	}
}

func TestScrapedContentFromNode_InvalidTagsJSON(t *testing.T) {
	node := dbtype.Node{
		Props: map[string]any{
			"content_id":     "c-bad",
			"suggested_tags": "not-valid-json",
		},
	}

	c := scrapedContentFromNode(node)
	if c.ContentID != "c-bad" {
		t.Errorf("ContentID = %q, want %q", c.ContentID, "c-bad")
	}
	// Tags should be nil since JSON unmarshal fails silently
	if c.SuggestedTags != nil {
		t.Errorf("SuggestedTags = %v, want nil on bad JSON", c.SuggestedTags)
	}
}

// --- NewStore test ---

func TestNewStore_NilDriver(t *testing.T) {
	store := NewStore(nil)
	if store == nil {
		t.Error("NewStore(nil) returned nil, want non-nil")
	}
	if store.driver != nil {
		t.Error("store.driver should be nil")
	}
}
