package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "mdemg/api/modulepb"
)

// DocsScraperHandler implements ModuleLifecycle and IngestionModule gRPC services.
type DocsScraperHandler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedIngestionModuleServer

	mu           sync.Mutex
	startTime    time.Time
	config       map[string]string
	httpClient   *http.Client
	requestCount uint64
	pagesScraped uint64

	// Configurable from manifest/handshake
	defaultProfile     string
	rateLimitMs        int
	respectRobotsTxt   bool
	maxContentLengthKB int
	userAgent          string
}

// NewDocsScraperHandler creates a new handler instance.
func NewDocsScraperHandler(httpClient *http.Client) *DocsScraperHandler {
	return &DocsScraperHandler{
		startTime:          time.Now(),
		config:             make(map[string]string),
		httpClient:         httpClient,
		defaultProfile:     "documentation",
		rateLimitMs:        1000,
		respectRobotsTxt:   true,
		maxContentLengthKB: 500,
		userAgent:          "MDEMG-Scraper/1.0",
	}
}

// applyConfig applies configuration from handshake.
func (h *DocsScraperHandler) applyConfig(cfg map[string]string) {
	if v, ok := cfg["default_profile"]; ok {
		h.defaultProfile = v
	}
	if v, ok := cfg["rate_limit_ms"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			h.rateLimitMs = n
		}
	}
	if v, ok := cfg["respect_robots_txt"]; ok {
		h.respectRobotsTxt = v == "true"
	}
	if v, ok := cfg["max_content_length_kb"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			h.maxContentLengthKB = n
		}
	}
	if v, ok := cfg["user_agent"]; ok {
		h.userAgent = v
	}
}

// Matches checks if this module can handle the given source.
func (h *DocsScraperHandler) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
	atomic.AddUint64(&h.requestCount, 1)

	matches := strings.HasPrefix(req.SourceUri, "http://") || strings.HasPrefix(req.SourceUri, "https://")
	confidence := float32(0.0)
	reason := "not an HTTP(S) URL"

	if matches {
		confidence = 0.8
		reason = "matches HTTP(S) URL"
		// Higher confidence for known doc patterns
		lower := strings.ToLower(req.SourceUri)
		if strings.Contains(lower, "/docs") || strings.Contains(lower, "/documentation") ||
			strings.Contains(lower, "/api") || strings.Contains(lower, "/guide") ||
			strings.Contains(lower, "/reference") || strings.Contains(lower, "/manual") {
			confidence = 0.95
			reason = "matches documentation URL pattern"
		}
	}

	return &pb.MatchResponse{
		Matches:    matches,
		Confidence: confidence,
		Reason:     reason,
	}, nil
}

// Parse converts a single URL into MDEMG observations.
func (h *DocsScraperHandler) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
	atomic.AddUint64(&h.requestCount, 1)

	url := req.SourceUri
	if url == "" {
		return &pb.ParseResponse{Error: "source_uri is required"}, nil
	}

	// Determine profile from metadata or default
	profile := h.defaultProfile
	if v, ok := req.Metadata["extraction_profile"]; ok && v != "" {
		profile = v
	}

	// Determine timeout
	timeout := 30 * time.Second
	if v, ok := req.Metadata["timeout_ms"]; ok {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			timeout = time.Duration(ms) * time.Millisecond
		}
	}

	// Parse auth if provided
	var auth *ScrapeAuth
	if v, ok := req.Metadata["auth"]; ok && v != "" {
		var a ScrapeAuth
		if err := json.Unmarshal([]byte(v), &a); err == nil {
			auth = &a
		}
	}

	// Fetch the URL
	fetcher := NewFetcher(h.httpClient, h.userAgent, h.respectRobotsTxt, timeout)
	body, contentType, err := fetcher.Fetch(ctx, url, auth)
	if err != nil {
		return &pb.ParseResponse{
			Error: fmt.Sprintf("fetch failed: %v", err),
		}, nil
	}

	// Only process HTML
	if !strings.Contains(contentType, "html") && !strings.Contains(contentType, "text") {
		return &pb.ParseResponse{
			Error: fmt.Sprintf("unsupported content type: %s", contentType),
		}, nil
	}

	// Truncate if too large
	maxBytes := h.maxContentLengthKB * 1024
	if len(body) > maxBytes {
		body = body[:maxBytes]
	}

	// Extract content
	extractor := NewExtractor()
	extracted, err := extractor.Extract(body, url, profile)
	if err != nil {
		return &pb.ParseResponse{
			Error: fmt.Sprintf("extraction failed: %v", err),
		}, nil
	}

	// Score quality
	scorer := NewQualityScorer()
	qualityScore := scorer.Score(extracted.Content, extracted.WordCount)

	// Suggest tags
	tagger := NewTagger()
	tags := tagger.SuggestTags(extracted.Title, extracted.Content, url)

	atomic.AddUint64(&h.pagesScraped, 1)

	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("scrape-%d", time.Now().UnixNano()),
		Path:        url,
		Name:        extracted.Title,
		Content:     extracted.Content,
		ContentType: "web-page",
		Tags:        tags,
		Metadata: map[string]string{
			"quality_score": fmt.Sprintf("%.3f", qualityScore),
			"content_hash":  extracted.ContentHash,
			"word_count":    fmt.Sprintf("%d", extracted.WordCount),
			"profile":       profile,
			"links_count":   fmt.Sprintf("%d", len(extracted.Links)),
			"discovered_links": func() string {
				b, _ := json.Marshal(extracted.Links)
				return string(b)
			}(),
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Source:    moduleID,
	}

	return &pb.ParseResponse{
		Observations: []*pb.Observation{obs},
		Metadata: map[string]string{
			"parsed_at":    time.Now().Format(time.RFC3339),
			"profile":      profile,
			"content_type": contentType,
		},
	}, nil
}

// Sync performs batch/crawl scraping (streaming responses).
func (h *DocsScraperHandler) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
	atomic.AddUint64(&h.requestCount, 1)

	// Decode URL list from config
	var urls []string
	if v, ok := req.Config["urls"]; ok {
		_ = json.Unmarshal([]byte(v), &urls)
	}
	if len(urls) == 0 {
		return fmt.Errorf("no urls provided in sync config")
	}

	profile := h.defaultProfile
	if v, ok := req.Config["extraction_profile"]; ok {
		profile = v
	}

	maxPages := 100
	if v, ok := req.Config["max_pages"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPages = n
		}
	}

	fetcher := NewFetcher(h.httpClient, h.userAgent, h.respectRobotsTxt, 30*time.Second)
	extractor := NewExtractor()
	scorer := NewQualityScorer()
	tagger := NewTagger()

	processed := 0
	for _, url := range urls {
		if processed >= maxPages {
			break
		}

		body, contentType, err := fetcher.Fetch(stream.Context(), url, nil)
		if err != nil {
			slog.Error("sync fetch failed", "module", moduleID, "url", url, "error", err)
			processed++
			continue
		}

		if !strings.Contains(contentType, "html") {
			processed++
			continue
		}

		maxBytes := h.maxContentLengthKB * 1024
		if len(body) > maxBytes {
			body = body[:maxBytes]
		}

		extracted, err := extractor.Extract(body, url, profile)
		if err != nil {
			slog.Error("sync extract failed", "module", moduleID, "url", url, "error", err)
			processed++
			continue
		}

		qualityScore := scorer.Score(extracted.Content, extracted.WordCount)
		tags := tagger.SuggestTags(extracted.Title, extracted.Content, url)

		atomic.AddUint64(&h.pagesScraped, 1)

		obs := &pb.Observation{
			NodeId:      fmt.Sprintf("scrape-%d", time.Now().UnixNano()),
			Path:        url,
			Name:        extracted.Title,
			Content:     extracted.Content,
			ContentType: "web-page",
			Tags:        tags,
			Metadata: map[string]string{
				"quality_score": fmt.Sprintf("%.3f", qualityScore),
				"content_hash":  extracted.ContentHash,
				"word_count":    fmt.Sprintf("%d", extracted.WordCount),
				"profile":       profile,
			},
			Timestamp: time.Now().Format(time.RFC3339),
			Source:    moduleID,
		}

		if err := stream.Send(&pb.SyncResponse{
			Observations: []*pb.Observation{obs},
			Cursor:       fmt.Sprintf("page-%d", processed),
			HasMore:      processed < len(urls)-1,
			Stats: &pb.SyncStats{
				ItemsProcessed: int32(processed + 1),
				ItemsCreated:   1,
			},
		}); err != nil {
			return err
		}

		processed++

		// Rate limit
		if h.rateLimitMs > 0 && processed < len(urls) {
			time.Sleep(time.Duration(h.rateLimitMs) * time.Millisecond)
		}
	}

	return nil
}

// ScrapeAuth mirrors the core auth structure.
type ScrapeAuth struct {
	Type        string            `json:"type"`
	Credentials map[string]string `json:"credentials"`
}
