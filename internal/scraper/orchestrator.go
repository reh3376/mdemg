package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"mdemg/internal/embeddings"
	"mdemg/internal/jobs"
	"mdemg/internal/plugins"

	pb "mdemg/api/modulepb"
)

// Orchestrator manages the scraping pipeline for a single job.
type Orchestrator struct {
	store     *Store
	pluginMgr *plugins.Manager
	embedder  embeddings.Embedder
	dedup     *DedupChecker
	cfg       Config
}

// NewOrchestrator creates a new job orchestrator.
func NewOrchestrator(store *Store, pluginMgr *plugins.Manager, embedder embeddings.Embedder, dedup *DedupChecker, cfg Config) *Orchestrator {
	return &Orchestrator{
		store:     store,
		pluginMgr: pluginMgr,
		embedder:  embedder,
		dedup:     dedup,
		cfg:       cfg,
	}
}

// crawlEntry is a URL queued for BFS crawling at a given depth.
type crawlEntry struct {
	url   string
	depth int
}

// RunJob executes the scraping pipeline for a job.
// When follow_links is true, performs BFS link-following up to max_depth levels
// and max_pages total pages. The job runs asynchronously (caller uses `go`).
func (o *Orchestrator) RunJob(ctx context.Context, queueJob *jobs.Job, req ScrapeJobRequest) {
	queue := jobs.GetQueue()
	queue.StartJob(queueJob.ID)

	// Apply defaults
	if req.Options.ExtractionProfile == "" {
		req.Options.ExtractionProfile = ProfileDocumentation
	}
	if req.Options.DelayMs == 0 {
		req.Options.DelayMs = o.cfg.DefaultDelayMs
	}
	if req.Options.TimeoutMs == 0 {
		req.Options.TimeoutMs = o.cfg.DefaultTimeoutMs
	}
	if req.TargetSpaceID == "" {
		req.TargetSpaceID = o.cfg.DefaultSpaceID
	}
	if req.Options.FollowLinks {
		if req.Options.MaxDepth <= 0 {
			req.Options.MaxDepth = 2
		}
		if req.Options.MaxPages <= 0 {
			req.Options.MaxPages = 50
		}
	}

	// Create ScrapeJob in Neo4j
	now := time.Now().UTC()
	scrapeJob := &ScrapeJob{
		JobID:         queueJob.ID,
		Status:        StatusRunning,
		URLs:          req.URLs,
		TargetSpaceID: req.TargetSpaceID,
		Options:       req.Options,
		TotalURLs:     len(req.URLs),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := o.store.CreateScrapeJob(ctx, scrapeJob); err != nil {
		slog.Error("scraper: failed to create job in Neo4j", "error", err)
		queueJob.Fail(err)
		return
	}

	// Find the docs-scraper plugin
	module, _, err := o.pluginMgr.MatchIngestionModule(ctx, "https://example.com", "text/html")
	if err != nil || module == nil {
		errMsg := "no scraper plugin available"
		if err != nil {
			errMsg = err.Error()
		}
		slog.Error("scraper: no plugin available", "error", errMsg)
		_ = o.store.UpdateScrapeJobStatus(ctx, queueJob.ID, StatusFailed, 0)
		_ = o.store.SetScrapeJobError(ctx, queueJob.ID, errMsg)
		queueJob.Fail(fmt.Errorf("%s", errMsg))
		return
	}

	// Build metadata for plugin calls
	optMeta := map[string]string{
		"extraction_profile": req.Options.ExtractionProfile,
		"target_space_id":    req.TargetSpaceID,
	}
	if req.Options.TimeoutMs > 0 {
		optMeta["timeout_ms"] = fmt.Sprintf("%d", req.Options.TimeoutMs)
	}
	if req.Options.Auth.Type != "" {
		authJSON, _ := json.Marshal(req.Options.Auth)
		optMeta["auth"] = string(authJSON)
	}

	// Collect seed domain(s) for same-domain filtering during BFS
	seedDomains := make(map[string]bool)
	for _, u := range req.URLs {
		if parsed, err := url.Parse(u); err == nil {
			seedDomains[parsed.Hostname()] = true
		}
	}

	// Initialize BFS queue with seed URLs
	visited := make(map[string]bool)
	var crawlQueue []crawlEntry
	for _, u := range req.URLs {
		normalized := normalizeURL(u)
		if !visited[normalized] {
			visited[normalized] = true
			crawlQueue = append(crawlQueue, crawlEntry{url: u, depth: 0})
		}
	}

	queueJob.SetTotal(len(crawlQueue))
	processed := 0

	for len(crawlQueue) > 0 {
		// Pop front of queue
		entry := crawlQueue[0]
		crawlQueue = crawlQueue[1:]

		// Check max_pages limit (only relevant when following links)
		if req.Options.FollowLinks && processed >= req.Options.MaxPages {
			slog.Info("scraper: job reached max_pages limit", "job_id", queueJob.ID, "max_pages", req.Options.MaxPages)
			break
		}

		select {
		case <-ctx.Done():
			_ = o.store.UpdateScrapeJobStatus(ctx, queueJob.ID, StatusCancelled, processed)
			queueJob.Cancel()
			return
		default:
		}

		queueJob.UpdateProgress(processed, fmt.Sprintf("scraping %s (depth %d)", entry.url, entry.depth))

		// Parse the URL via plugin
		resp, err := module.IngestionClient.Parse(ctx, &pb.ParseRequest{
			SourceUri:   entry.url,
			ContentType: "text/html",
			Metadata:    optMeta,
		})
		if err != nil {
			slog.Error("scraper: Parse failed", "url", entry.url, "error", err)
			_ = o.store.SaveScrapedContent(ctx, &ScrapedContent{
				ContentID: uuid.New().String(),
				JobID:     queueJob.ID,
				URL:       entry.url,
				Status:    StatusFailed,
			})
			processed++
			o.rateLimitDelay(ctx, req.Options.DelayMs)
			continue
		}

		if resp.Error != "" {
			slog.Warn("scraper: Parse returned error", "url", entry.url, "error", resp.Error)
		}

		// Process observations and extract discovered links
		for _, obs := range resp.Observations {
			o.saveObservation(ctx, queueJob.ID, entry.url, obs, req.TargetSpaceID)

			// BFS: enqueue discovered links if following links and within depth
			if req.Options.FollowLinks && entry.depth < req.Options.MaxDepth {
				newLinks := extractDiscoveredLinks(obs)
				for _, link := range newLinks {
					normalized := normalizeURL(link)
					if visited[normalized] {
						continue
					}
					// Same-domain filter
					if parsed, err := url.Parse(link); err == nil {
						if !seedDomains[parsed.Hostname()] {
							continue
						}
					}
					visited[normalized] = true
					crawlQueue = append(crawlQueue, crawlEntry{url: link, depth: entry.depth + 1})
				}
				// Update total estimate for progress reporting
				queueJob.SetTotal(processed + len(crawlQueue) + 1)
			}
		}

		processed++
		o.rateLimitDelay(ctx, req.Options.DelayMs)
	}

	// Update job to awaiting_review
	_ = o.store.UpdateScrapeJobStatus(ctx, queueJob.ID, StatusAwaitingReview, processed)
	queueJob.UpdateProgress(processed, "awaiting review")
	slog.Info("scraper: job complete, awaiting review", "job_id", queueJob.ID, "urls_processed", processed, "follow_links", req.Options.FollowLinks, "max_depth", req.Options.MaxDepth)
}

// saveObservation converts a plugin observation to ScrapedContent and persists it.
func (o *Orchestrator) saveObservation(ctx context.Context, jobID, pageURL string, obs *pb.Observation, targetSpaceID string) {
	content := obs.Content
	preview := content
	if len(preview) > 500 {
		preview = preview[:500] + "..."
	}

	qualityScore := 0.0
	if v, ok := obs.Metadata["quality_score"]; ok {
		fmt.Sscanf(v, "%f", &qualityScore)
	}

	wordCount := len(strings.Fields(content))

	sc := &ScrapedContent{
		ContentID:      uuid.New().String(),
		JobID:          jobID,
		URL:            pageURL,
		Title:          obs.Name,
		Content:        content,
		ContentPreview: preview,
		ContentHash:    obs.Metadata["content_hash"],
		QualityScore:   qualityScore,
		SuggestedTags:  obs.Tags,
		Summary:        obs.Metadata["summary"],
		Status:         ContentPendingReview,
		WordCount:      wordCount,
	}

	// Dedup check
	if o.dedup != nil && o.embedder != nil {
		similar, dedupErr := o.dedup.CheckSimilar(ctx, targetSpaceID, content)
		if dedupErr != nil {
			slog.Warn("scraper: dedup check failed", "url", pageURL, "error", dedupErr)
		} else {
			sc.SimilarExisting = similar
		}
	}

	if err := o.store.SaveScrapedContent(ctx, sc); err != nil {
		slog.Error("scraper: failed to save content", "url", pageURL, "error", err)
	}
}

// extractDiscoveredLinks pulls the discovered_links from observation metadata.
func extractDiscoveredLinks(obs *pb.Observation) []string {
	raw, ok := obs.Metadata["discovered_links"]
	if !ok || raw == "" {
		return nil
	}
	var links []string
	if err := json.Unmarshal([]byte(raw), &links); err != nil {
		return nil
	}
	return links
}

// normalizeURL strips fragments and trailing slashes for dedup.
func normalizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.Fragment = ""
	result := parsed.String()
	return strings.TrimRight(result, "/")
}

// rateLimitDelay sleeps for the configured delay, respecting context cancellation.
func (o *Orchestrator) rateLimitDelay(ctx context.Context, delayMs int) {
	if delayMs <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(delayMs) * time.Millisecond):
	}
}
