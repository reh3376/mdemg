package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"mdemg/internal/jobs"
	"mdemg/internal/scraper"
)

// handleScraperJobs routes POST /v1/scraper/jobs (create) and GET /v1/scraper/jobs (list).
func (s *Server) handleScraperJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleCreateScrapeJob(w, r)
	case http.MethodGet:
		s.handleListScrapeJobs(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleScraperJobByID routes /v1/scraper/jobs/{id} and /v1/scraper/jobs/{id}/review.
func (s *Server) handleScraperJobByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/scraper/jobs/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "job_id required"})
		return
	}

	// Check for /review suffix
	if strings.HasSuffix(path, "/review") {
		jobID := strings.TrimSuffix(path, "/review")
		s.handleScraperJobReview(w, r, jobID)
		return
	}

	jobID := path
	switch r.Method {
	case http.MethodGet:
		s.handleGetScrapeJob(w, r, jobID)
	case http.MethodDelete:
		s.handleCancelScrapeJob(w, r, jobID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleCreateScrapeJob creates a new scrape job (POST /v1/scraper/jobs).
func (s *Server) handleCreateScrapeJob(w http.ResponseWriter, r *http.Request) {
	if s.scraperSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "scraper not enabled"})
		return
	}

	var req scraper.ScrapeJobRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	if len(req.URLs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "urls required"})
		return
	}

	// Validate URLs
	for _, u := range req.URLs {
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("invalid URL: %s (must be http/https)", u)})
			return
		}
	}

	cfg := s.scraperSvc.GetConfig()
	if req.TargetSpaceID == "" {
		req.TargetSpaceID = cfg.DefaultSpaceID
	}

	// Create job in queue
	jobID := fmt.Sprintf("scrape-%s", uuid.New().String()[:8])
	queue := jobs.GetQueue()
	queueJob, ctx := queue.CreateJob(jobID, "web-scraper", map[string]any{
		"urls":            req.URLs,
		"target_space_id": req.TargetSpaceID,
	})

	// Run job asynchronously
	orch := s.scraperSvc.GetOrchestrator()
	if orch == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "scraper orchestrator not initialized (requires conversation service)"})
		return
	}
	go orch.RunJob(ctx, queueJob, req)

	writeJSON(w, http.StatusAccepted, scraper.ScrapeJobResponse{
		JobID:         jobID,
		Status:        scraper.StatusPending,
		URLs:          req.URLs,
		TargetSpaceID: req.TargetSpaceID,
		TotalURLs:     len(req.URLs),
		ProcessedURLs: 0,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	})
}

// handleListScrapeJobs lists all scrape jobs (GET /v1/scraper/jobs).
func (s *Server) handleListScrapeJobs(w http.ResponseWriter, r *http.Request) {
	if s.scraperSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "scraper not enabled"})
		return
	}

	store := s.scraperSvc.GetStore()
	jobList, err := store.ListScrapeJobs(r.Context())
	if err != nil {
		writeInternalError(w, err, "list scrape jobs")
		return
	}

	resp := scraper.ScrapeJobListResponse{
		Count: len(jobList),
	}
	for _, j := range jobList {
		resp.Jobs = append(resp.Jobs, jobToResponse(j))
	}
	if resp.Jobs == nil {
		resp.Jobs = []scraper.ScrapeJobResponse{}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetScrapeJob returns a scrape job with its content (GET /v1/scraper/jobs/{id}).
func (s *Server) handleGetScrapeJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if s.scraperSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "scraper not enabled"})
		return
	}

	store := s.scraperSvc.GetStore()
	job, err := store.GetScrapeJob(r.Context(), jobID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("job not found: %s", jobID)})
		return
	}

	resp := jobToResponse(*job)

	// Include scraped content
	contents, err := store.GetScrapedContents(r.Context(), jobID)
	if err == nil {
		resp.Contents = contents
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleCancelScrapeJob cancels a scrape job (DELETE /v1/scraper/jobs/{id}).
func (s *Server) handleCancelScrapeJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if s.scraperSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "scraper not enabled"})
		return
	}

	queue := jobs.GetQueue()
	cancelled := queue.CancelJob(jobID)

	if !cancelled {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("job not found or not cancellable: %s", jobID)})
		return
	}

	// Update Neo4j status
	store := s.scraperSvc.GetStore()
	if err := store.UpdateScrapeJobStatus(r.Context(), jobID, scraper.StatusCancelled, -1); err != nil {
		slog.Warn("scraper job status update failed", "job_id", jobID, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":  jobID,
		"status":  scraper.StatusCancelled,
		"message": "job cancelled",
	})
}

// handleScraperJobReview processes review decisions (POST /v1/scraper/jobs/{id}/review).
func (s *Server) handleScraperJobReview(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.scraperSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "scraper not enabled"})
		return
	}

	var req scraper.ReviewRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	reviewer := s.scraperSvc.GetReviewer()
	if reviewer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "review service not initialized"})
		return
	}

	cfg := s.scraperSvc.GetConfig()
	resp, err := reviewer.ProcessReview(r.Context(), jobID, req.Decisions, cfg.DefaultSpaceID)
	if err != nil {
		writeInternalError(w, err, "process review")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleListScrapeSpaces lists available target spaces (GET /v1/scraper/spaces).
func (s *Server) handleListScrapeSpaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.scraperSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "scraper not enabled"})
		return
	}

	store := s.scraperSvc.GetStore()
	spaces, err := store.ListSpaces(r.Context())
	if err != nil {
		writeInternalError(w, err, "list spaces")
		return
	}

	if spaces == nil {
		spaces = []scraper.SpaceInfo{}
	}

	writeJSON(w, http.StatusOK, scraper.SpaceListResponse{
		Spaces: spaces,
		Count:  len(spaces),
	})
}

// --- helpers ---

func jobToResponse(j scraper.ScrapeJob) scraper.ScrapeJobResponse {
	resp := scraper.ScrapeJobResponse{
		JobID:         j.JobID,
		Status:        j.Status,
		URLs:          j.URLs,
		TargetSpaceID: j.TargetSpaceID,
		TotalURLs:     j.TotalURLs,
		ProcessedURLs: j.ProcessedURLs,
		CreatedAt:     j.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     j.UpdatedAt.UTC().Format(time.RFC3339),
		Error:         j.Error,
	}
	if j.CompletedAt != nil {
		resp.CompletedAt = j.CompletedAt.UTC().Format(time.RFC3339)
	}
	if resp.URLs == nil {
		resp.URLs = []string{}
	}
	return resp
}
