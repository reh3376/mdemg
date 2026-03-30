package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Store handles Neo4j persistence for scrape jobs and content.
type Store struct {
	driver neo4j.DriverWithContext
}

// NewStore creates a new scraper store.
func NewStore(driver neo4j.DriverWithContext) *Store {
	return &Store{driver: driver}
}

// CreateScrapeJob creates a new ScrapeJob node in Neo4j.
func (s *Store) CreateScrapeJob(ctx context.Context, job *ScrapeJob) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	urlsJSON, _ := json.Marshal(job.URLs)
	optJSON, _ := json.Marshal(job.Options)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MERGE (j:ScrapeJob {job_id: $job_id})
			 SET j.status = $status,
			     j.urls = $urls,
			     j.target_space_id = $target_space_id,
			     j.options = $options,
			     j.total_urls = $total_urls,
			     j.processed_urls = 0,
			     j.created_at = datetime($created_at),
			     j.updated_at = datetime($updated_at)`,
			map[string]any{
				"job_id":          job.JobID,
				"status":          job.Status,
				"urls":            string(urlsJSON),
				"target_space_id": job.TargetSpaceID,
				"options":         string(optJSON),
				"total_urls":      job.TotalURLs,
				"created_at":      job.CreatedAt.UTC().Format(time.RFC3339),
				"updated_at":      job.UpdatedAt.UTC().Format(time.RFC3339),
			})
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})
	return err
}

// UpdateScrapeJobStatus updates the status and processed count of a job.
func (s *Store) UpdateScrapeJobStatus(ctx context.Context, jobID, status string, processedURLs int) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"job_id":         jobID,
		"status":         status,
		"processed_urls": processedURLs,
		"updated_at":     now,
	}

	query := `MATCH (j:ScrapeJob {job_id: $job_id})
	          SET j.status = $status,
	              j.processed_urls = $processed_urls,
	              j.updated_at = datetime($updated_at)`

	if status == StatusCompleted || status == StatusFailed || status == StatusCancelled {
		query += `, j.completed_at = datetime($updated_at)`
	}

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})
	return err
}

// SetScrapeJobError sets the error message on a job.
func (s *Store) SetScrapeJobError(ctx context.Context, jobID, errMsg string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (j:ScrapeJob {job_id: $job_id})
			 SET j.error = $error, j.updated_at = datetime($now)`,
			map[string]any{
				"job_id": jobID,
				"error":  errMsg,
				"now":    time.Now().UTC().Format(time.RFC3339),
			})
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})
	return err
}

// SaveScrapedContent persists a ScrapedContent node with BELONGS_TO edge.
func (s *Store) SaveScrapedContent(ctx context.Context, content *ScrapedContent) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	if content.ContentID == "" {
		content.ContentID = uuid.New().String()
	}

	tagsJSON, _ := json.Marshal(content.SuggestedTags)
	similarJSON, _ := json.Marshal(content.SimilarExisting)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MERGE (c:ScrapedContent {content_id: $content_id})
			 SET c.job_id = $job_id,
			     c.url = $url,
			     c.title = $title,
			     c.content = $content,
			     c.content_preview = $content_preview,
			     c.content_hash = $content_hash,
			     c.quality_score = $quality_score,
			     c.similar_existing = $similar_existing,
			     c.suggested_tags = $suggested_tags,
			     c.summary = $summary,
			     c.status = $status,
			     c.word_count = $word_count,
			     c.created_at = datetime($now)
			 WITH c
			 MATCH (j:ScrapeJob {job_id: $job_id})
			 MERGE (c)-[:BELONGS_TO]->(j)`,
			map[string]any{
				"content_id":       content.ContentID,
				"job_id":           content.JobID,
				"url":              content.URL,
				"title":            content.Title,
				"content":          content.Content,
				"content_preview":  content.ContentPreview,
				"content_hash":     content.ContentHash,
				"quality_score":    content.QualityScore,
				"similar_existing": string(similarJSON),
				"suggested_tags":   string(tagsJSON),
				"summary":          content.Summary,
				"status":           content.Status,
				"word_count":       content.WordCount,
				"now":              time.Now().UTC().Format(time.RFC3339),
			})
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})
	return err
}

// GetScrapeJob retrieves a ScrapeJob from Neo4j.
func (s *Store) GetScrapeJob(ctx context.Context, jobID string) (*ScrapeJob, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	records, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (j:ScrapeJob {job_id: $job_id})
			 RETURN j`,
			map[string]any{"job_id": jobID})
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return nil, err
	}

	recs := records.([]*neo4j.Record)
	if len(recs) > 0 {
		node := recs[0].Values[0].(neo4j.Node)
		return scrapeJobFromNode(node), nil
	}
	return nil, fmt.Errorf("scrape job not found: %s", jobID)
}

// ListScrapeJobs returns all scrape jobs.
func (s *Store) ListScrapeJobs(ctx context.Context) ([]ScrapeJob, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	records, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (j:ScrapeJob)
			 RETURN j
			 ORDER BY j.created_at DESC
			 LIMIT 100`, nil)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return nil, err
	}

	var jobs []ScrapeJob
	for _, rec := range records.([]*neo4j.Record) {
		node := rec.Values[0].(neo4j.Node)
		jobs = append(jobs, *scrapeJobFromNode(node))
	}
	return jobs, nil
}

// GetScrapedContents returns all ScrapedContent for a job.
func (s *Store) GetScrapedContents(ctx context.Context, jobID string) ([]ScrapedContent, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	records, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (c:ScrapedContent)-[:BELONGS_TO]->(j:ScrapeJob {job_id: $job_id})
			 RETURN c
			 ORDER BY c.created_at`,
			map[string]any{"job_id": jobID})
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return nil, err
	}

	var contents []ScrapedContent
	for _, rec := range records.([]*neo4j.Record) {
		node := rec.Values[0].(neo4j.Node)
		contents = append(contents, *scrapedContentFromNode(node))
	}
	return contents, nil
}

// UpdateContentStatus updates the status of a ScrapedContent item.
func (s *Store) UpdateContentStatus(ctx context.Context, contentID, status string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (c:ScrapedContent {content_id: $content_id})
			 SET c.status = $status`,
			map[string]any{"content_id": contentID, "status": status})
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})
	return err
}

// UpdateScrapedContentContent updates the content and tags of a ScrapedContent item (for edits).
func (s *Store) UpdateScrapedContentContent(ctx context.Context, contentID, content string, tags []string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	tagsJSON, _ := json.Marshal(tags)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (c:ScrapedContent {content_id: $content_id})
			 SET c.content = $content,
			     c.suggested_tags = $tags`,
			map[string]any{"content_id": contentID, "content": content, "tags": string(tagsJSON)})
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})
	return err
}

// GetScrapedContent retrieves a single ScrapedContent by ID.
func (s *Store) GetScrapedContent(ctx context.Context, contentID string) (*ScrapedContent, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	records, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (c:ScrapedContent {content_id: $content_id})
			 RETURN c`,
			map[string]any{"content_id": contentID})
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return nil, err
	}

	recs := records.([]*neo4j.Record)
	if len(recs) > 0 {
		node := recs[0].Values[0].(neo4j.Node)
		return scrapedContentFromNode(node), nil
	}
	return nil, fmt.Errorf("scraped content not found: %s", contentID)
}

// CreateIngestedAsRelationship creates [:INGESTED_AS] between ScrapedContent and MemoryNode.
func (s *Store) CreateIngestedAsRelationship(ctx context.Context, contentID, nodeID string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (c:ScrapedContent {content_id: $content_id})
			 MATCH (m:MemoryNode {node_id: $node_id})
			 MERGE (c)-[:INGESTED_AS]->(m)
			 SET c.ingested_node_id = $node_id,
			     c.status = 'ingested'`,
			map[string]any{"content_id": contentID, "node_id": nodeID})
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})
	return err
}

// CountPendingReview counts ScrapedContent items still pending review for a job.
func (s *Store) CountPendingReview(ctx context.Context, jobID string) (int, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	records, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (c:ScrapedContent {status: 'pending_review'})-[:BELONGS_TO]->(j:ScrapeJob {job_id: $job_id})
			 RETURN count(c) AS cnt`,
			map[string]any{"job_id": jobID})
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return 0, err
	}

	recs := records.([]*neo4j.Record)
	if len(recs) > 0 {
		cnt, _ := recs[0].Values[0].(int64)
		return int(cnt), nil
	}
	return 0, nil
}

// ListSpaces returns distinct space_ids from MemoryNodes.
func (s *Store) ListSpaces(ctx context.Context) ([]SpaceInfo, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	records, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx,
			`MATCH (m:MemoryNode)
			 WHERE m.space_id IS NOT NULL
			 RETURN m.space_id AS space_id, count(m) AS node_count
			 ORDER BY node_count DESC`, nil)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return nil, err
	}

	var spaces []SpaceInfo
	for _, rec := range records.([]*neo4j.Record) {
		sid, _ := rec.Get("space_id")
		cnt, _ := rec.Get("node_count")
		spaces = append(spaces, SpaceInfo{
			SpaceID:   fmt.Sprintf("%v", sid),
			NodeCount: int(cnt.(int64)),
		})
	}
	return spaces, nil
}

// --- helpers ---

func scrapeJobFromNode(node neo4j.Node) *ScrapeJob {
	props := node.Props
	job := &ScrapeJob{
		JobID:         getStr(props, "job_id"),
		Status:        getStr(props, "status"),
		TargetSpaceID: getStr(props, "target_space_id"),
		TotalURLs:     getInt(props, "total_urls"),
		ProcessedURLs: getInt(props, "processed_urls"),
		Error:         getStr(props, "error"),
	}

	if v, ok := props["urls"]; ok {
		_ = json.Unmarshal([]byte(v.(string)), &job.URLs)
	}
	if v, ok := props["options"]; ok {
		_ = json.Unmarshal([]byte(v.(string)), &job.Options)
	}
	if v, ok := props["created_at"]; ok {
		if t, ok := v.(time.Time); ok {
			job.CreatedAt = t
		}
	}
	if v, ok := props["updated_at"]; ok {
		if t, ok := v.(time.Time); ok {
			job.UpdatedAt = t
		}
	}
	if v, ok := props["completed_at"]; ok {
		if t, ok := v.(time.Time); ok {
			job.CompletedAt = &t
		}
	}
	return job
}

func scrapedContentFromNode(node neo4j.Node) *ScrapedContent {
	props := node.Props
	c := &ScrapedContent{
		ContentID:      getStr(props, "content_id"),
		JobID:          getStr(props, "job_id"),
		URL:            getStr(props, "url"),
		Title:          getStr(props, "title"),
		Content:        getStr(props, "content"),
		ContentPreview: getStr(props, "content_preview"),
		ContentHash:    getStr(props, "content_hash"),
		QualityScore:   getFloat(props, "quality_score"),
		Summary:        getStr(props, "summary"),
		Status:         getStr(props, "status"),
		WordCount:      getInt(props, "word_count"),
		IngestedNodeID: getStr(props, "ingested_node_id"),
	}
	if v, ok := props["suggested_tags"]; ok {
		_ = json.Unmarshal([]byte(v.(string)), &c.SuggestedTags)
	}
	if v, ok := props["similar_existing"]; ok {
		_ = json.Unmarshal([]byte(v.(string)), &c.SimilarExisting)
	}
	return c
}

func getStr(props map[string]any, key string) string {
	if v, ok := props[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(props map[string]any, key string) int {
	if v, ok := props[key]; ok {
		switch n := v.(type) {
		case int64:
			return int(n)
		case int:
			return n
		case float64:
			return int(n)
		}
	}
	return 0
}

func getFloat(props map[string]any, key string) float64 {
	if v, ok := props[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int64:
			return float64(n)
		}
	}
	return 0
}
