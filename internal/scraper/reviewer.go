package scraper

import (
	"context"
	"fmt"
	"log/slog"

	"mdemg/internal/jobs"
)

// ConversationService is the interface for ingesting content into the memory graph.
type ConversationService interface {
	Observe(ctx context.Context, req ObserveParams) (*ObserveResult, error)
}

// ObserveParams mirrors the fields needed from conversation.ObserveRequest.
type ObserveParams struct {
	SpaceID        string
	Content        string
	ObsType        string
	Tags           []string
	Metadata       map[string]any
	Pinned         bool
	StructuredData map[string]any
}

// ObserveResult mirrors the fields needed from conversation.ObserveResponse.
type ObserveResult struct {
	NodeID string
}

// Reviewer handles the review workflow for scraped content.
type Reviewer struct {
	store   *Store
	convSvc ConversationService
}

// NewReviewer creates a new reviewer.
func NewReviewer(store *Store, convSvc ConversationService) *Reviewer {
	return &Reviewer{store: store, convSvc: convSvc}
}

// ProcessReview processes review decisions for a scrape job.
func (r *Reviewer) ProcessReview(ctx context.Context, jobID string, decisions []ReviewDecision, defaultSpaceID string) (*ReviewResponse, error) {
	resp := &ReviewResponse{JobID: jobID}

	for _, d := range decisions {
		switch d.Action {
		case "approve":
			nodeID, err := r.ingestContent(ctx, d.ContentID, d.SpaceID, defaultSpaceID)
			if err != nil {
				slog.Error("scraper review: ingest failed", "content_id", d.ContentID, "error", err)
				continue
			}
			resp.Ingested = append(resp.Ingested, IngestedItem{
				ContentID: d.ContentID,
				NodeID:    nodeID,
			})

		case "reject":
			if err := r.store.UpdateContentStatus(ctx, d.ContentID, ContentRejected); err != nil {
				slog.Error("scraper review: reject failed", "content_id", d.ContentID, "error", err)
			}
			resp.Rejected++

		case "edit":
			// Update content first, then ingest
			tags := d.EditTags
			if tags == nil {
				// Preserve existing tags
				existing, err := r.store.GetScrapedContent(ctx, d.ContentID)
				if err == nil {
					tags = existing.SuggestedTags
				}
			}
			if d.EditContent != "" {
				if err := r.store.UpdateScrapedContentContent(ctx, d.ContentID, d.EditContent, tags); err != nil {
					slog.Error("scraper review: edit failed", "content_id", d.ContentID, "error", err)
					continue
				}
			}
			nodeID, err := r.ingestContent(ctx, d.ContentID, d.SpaceID, defaultSpaceID)
			if err != nil {
				slog.Error("scraper review: ingest after edit failed", "content_id", d.ContentID, "error", err)
				continue
			}
			resp.Ingested = append(resp.Ingested, IngestedItem{
				ContentID: d.ContentID,
				NodeID:    nodeID,
			})

		default:
			return nil, fmt.Errorf("unknown action: %s", d.Action)
		}

		resp.Reviewed++
	}

	// Check if all content is reviewed
	pending, err := r.store.CountPendingReview(ctx, jobID)
	if err != nil {
		slog.Error("scraper review: count pending failed", "error", err)
	}

	if pending == 0 {
		// Complete the job
		_ = r.store.UpdateScrapeJobStatus(ctx, jobID, StatusCompleted, -1) // -1 = don't update count
		// Complete the queue job too
		queue := jobs.GetQueue()
		if qJob, ok := queue.GetJob(jobID); ok {
			qJob.Complete(map[string]any{
				"ingested": len(resp.Ingested),
				"rejected": resp.Rejected,
			})
		}
		resp.Status = StatusCompleted
	} else {
		resp.Status = StatusAwaitingReview
	}

	return resp, nil
}

// ingestContent parses ScrapedContent into sections (via the UPTS-backed parser),
// then creates a MemoryNode per section via conversation.Observe.
// Returns the NodeID of the first section (used for the INGESTED_AS edge).
func (r *Reviewer) ingestContent(ctx context.Context, contentID, overrideSpaceID, defaultSpaceID string) (string, error) {
	sc, err := r.store.GetScrapedContent(ctx, contentID)
	if err != nil {
		return "", err
	}

	spaceID := defaultSpaceID
	if overrideSpaceID != "" {
		spaceID = overrideSpaceID
	}

	baseTags := sc.SuggestedTags
	baseTags = append(baseTags, "source:web-scraper", fmt.Sprintf("url:%s", sc.URL))

	// Parse content into sections using the UPTS-backed markdown parser
	parser := NewParser(DefaultParserConfig())
	parseResult := parser.Parse(sc.Title, sc.Content, sc.URL, baseTags, sc.QualityScore)

	if parseResult.WasChunked {
		slog.Info("scraper: chunked content into sections", "title", sc.Title, "sections", len(parseResult.Sections), "total_words", parseResult.TotalWordCount)
	}

	var firstNodeID string
	totalSections := len(parseResult.Sections)

	for _, sec := range parseResult.Sections {
		metadata := map[string]any{
			"source_url":    sc.URL,
			"quality_score": sc.QualityScore,
			"scrape_job_id": sc.JobID,
			"content_hash":  sc.ContentHash,
			"word_count":    sec.WordCount,
		}
		if sc.Title != "" {
			metadata["title"] = sc.Title
		}
		if parseResult.WasChunked {
			metadata["section_index"] = sec.SectionIndex
			metadata["section_title"] = sec.Title
			metadata["total_sections"] = totalSections
		}

		// Convert symbols to structured data
		var structuredData map[string]any
		if len(sec.Symbols) > 0 {
			symbolMaps := make([]map[string]any, len(sec.Symbols))
			for i, sym := range sec.Symbols {
				symbolMaps[i] = map[string]any{
					"name":   sym.Name,
					"type":   sym.Type,
					"line":   sym.Line,
					"value":  sym.Value,
					"parent": sym.Parent,
				}
			}
			structuredData = map[string]any{"symbols": symbolMaps}
		}

		result, err := r.convSvc.Observe(ctx, ObserveParams{
			SpaceID:        spaceID,
			Content:        sec.Content,
			ObsType:        "learning",
			Tags:           sec.Tags,
			Metadata:       metadata,
			Pinned:         true,
			StructuredData: structuredData,
		})
		if err != nil {
			return "", fmt.Errorf("observe failed for section %d: %w", sec.SectionIndex, err)
		}

		if firstNodeID == "" {
			firstNodeID = result.NodeID
		}
	}

	// Create INGESTED_AS relationship pointing to the first section's node
	if err := r.store.CreateIngestedAsRelationship(ctx, contentID, firstNodeID); err != nil {
		slog.Error("scraper: failed to create INGESTED_AS edge", "error", err)
	}

	sc.IngestedNodeID = firstNodeID
	return firstNodeID, nil
}
