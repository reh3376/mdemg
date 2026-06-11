package api

import (
	"net/http"
	"strings"

	"mdemg/internal/gaps"
)

// handleCapabilityGaps handles GET /v1/system/capability-gaps
// Returns all capability gaps with optional filtering
func (s *Server) handleCapabilityGaps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	status := r.URL.Query().Get("status") // "open", "addressed", "dismissed"
	gapType := r.URL.Query().Get("type")  // "data_source", "reasoning", "query_pattern"
	spaceID := r.URL.Query().Get("space_id")

	ctx := r.Context()

	// Get gaps from store
	gapsList, err := s.gapDetector.GetStore().ListGaps(ctx, gaps.GapStatus(status), gapType)
	if err != nil {
		writeInternalError(w, err, "list capability gaps")
		return
	}

	// Filter by space_id if provided
	if spaceID != "" {
		var filtered []gaps.CapabilityGap
		for _, gap := range gapsList {
			if gap.SpaceID == spaceID || gap.SpaceID == "" {
				filtered = append(filtered, gap)
			}
		}
		gapsList = filtered
	}

	// Sort by priority
	gaps.SortGapsByPriority(gapsList)

	// Build summary
	summary, err := s.gapDetector.GetGapsSummary(ctx)
	if err != nil {
		summary = &gaps.GapsSummary{
			Total:  len(gapsList),
			ByType: make(map[string]int),
		}
	}

	// Convert to response format
	gapsResponse := make([]map[string]any, 0, len(gapsList))
	for _, gap := range gapsList {
		gapsResponse = append(gapsResponse, map[string]any{
			"id":          gap.ID,
			"type":        gap.Type,
			"description": gap.Description,
			"evidence":    gap.Evidence,
			"suggested_plugin": map[string]any{
				"type":         gap.SuggestedPlugin.Type,
				"name":         gap.SuggestedPlugin.Name,
				"description":  gap.SuggestedPlugin.Description,
				"capabilities": gap.SuggestedPlugin.Capabilities,
			},
			"priority":         gap.Priority,
			"detected_at":      gap.DetectedAt,
			"updated_at":       gap.UpdatedAt,
			"status":           gap.Status,
			"occurrence_count": gap.OccurrenceCount,
			"space_id":         gap.SpaceID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"gaps": gapsResponse,
			"summary": map[string]any{
				"total":         summary.Total,
				"by_type":       summary.ByType,
				"high_priority": summary.HighPriority,
			},
		},
	})
}

// handleCapabilityGapDismiss handles POST /v1/system/capability-gaps/{id}/dismiss
func (s *Server) handleCapabilityGapDismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract gap ID from path
	gapID := extractGapID(r.URL.Path, "/dismiss")
	if gapID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid gap_id in path"})
		return
	}

	ctx := r.Context()

	// Check if gap exists
	gap, err := s.gapDetector.GetStore().GetGap(ctx, gapID)
	if err != nil {
		writeInternalError(w, err, "get capability gap")
		return
	}
	if gap == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "gap not found"})
		return
	}

	// Update status
	if err := s.gapDetector.GetStore().UpdateGapStatus(ctx, gapID, gaps.GapStatusDismissed); err != nil {
		writeInternalError(w, err, "dismiss capability gap")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":     gapID,
		"status": "dismissed",
	})
}

// handleCapabilityGapAddress handles POST /v1/system/capability-gaps/{id}/address
func (s *Server) handleCapabilityGapAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract gap ID from path
	gapID := extractGapID(r.URL.Path, "/address")
	if gapID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid gap_id in path"})
		return
	}

	ctx := r.Context()

	// Check if gap exists
	gap, err := s.gapDetector.GetStore().GetGap(ctx, gapID)
	if err != nil {
		writeInternalError(w, err, "get capability gap")
		return
	}
	if gap == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "gap not found"})
		return
	}

	// Update status
	if err := s.gapDetector.GetStore().UpdateGapStatus(ctx, gapID, gaps.GapStatusAddressed); err != nil {
		writeInternalError(w, err, "address capability gap")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":     gapID,
		"status": "addressed",
	})
}

// handleFeedback handles POST /v1/feedback
// Accepts user feedback on retrieval results for gap detection
func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req gaps.Feedback
	if !readJSON(w, r, &req) {
		return
	}

	// Validate required fields
	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}
	if req.QueryText == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "query_text is required"})
		return
	}

	ctx := r.Context()

	if err := s.gapDetector.ProcessFeedback(ctx, req); err != nil {
		writeInternalError(w, err, "process feedback")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "recorded",
		"message": "Feedback recorded for capability gap analysis",
	})
}

// handleGapAnalyze handles POST /v1/system/capability-gaps/analyze
// Triggers a full gap analysis
func (s *Server) handleGapAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	ctx := r.Context()

	gapsList, err := s.gapDetector.RunFullAnalysis(ctx, spaceID)
	if err != nil {
		writeInternalError(w, err, "gap analysis")
		return
	}

	// Sort by priority
	gaps.SortGapsByPriority(gapsList)

	// Get metrics
	metrics := s.gapDetector.GetMetrics()

	writeJSON(w, http.StatusOK, map[string]any{
		"gaps_found": len(gapsList),
		"metrics":    metrics,
	})
}

// handleGapMetrics handles GET /v1/system/capability-gaps/metrics
// Returns gap detection metrics
func (s *Server) handleGapMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get in-memory metrics
	metrics := s.gapDetector.GetMetrics()

	// Get stored stats
	stats, err := s.gapDetector.GetStore().GetGapStats(ctx)
	if err != nil {
		stats = make(map[string]any)
	}

	// Merge
	for k, v := range stats {
		metrics[k] = v
	}

	writeJSON(w, http.StatusOK, metrics)
}

// handleCapabilityGapOperation routes requests to the appropriate handler
func (s *Server) handleCapabilityGapOperation(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.HasSuffix(path, "/dismiss"):
		s.handleCapabilityGapDismiss(w, r)
	case strings.HasSuffix(path, "/address"):
		s.handleCapabilityGapAddress(w, r)
	case strings.HasSuffix(path, "/analyze"):
		s.handleGapAnalyze(w, r)
	case strings.HasSuffix(path, "/metrics"):
		s.handleGapMetrics(w, r)
	default:
		// Single gap by ID
		s.handleCapabilityGapGet(w, r)
	}
}

// handleCapabilityGapGet handles GET /v1/system/capability-gaps/{id}
func (s *Server) handleCapabilityGapGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract gap ID from path
	path := strings.TrimPrefix(r.URL.Path, "/v1/system/capability-gaps/")
	gapID := path
	if gapID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid gap_id in path"})
		return
	}

	ctx := r.Context()

	gap, err := s.gapDetector.GetStore().GetGap(ctx, gapID)
	if err != nil {
		writeInternalError(w, err, "get capability gap")
		return
	}
	if gap == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "gap not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          gap.ID,
		"type":        gap.Type,
		"description": gap.Description,
		"evidence":    gap.Evidence,
		"suggested_plugin": map[string]any{
			"type":         gap.SuggestedPlugin.Type,
			"name":         gap.SuggestedPlugin.Name,
			"description":  gap.SuggestedPlugin.Description,
			"capabilities": gap.SuggestedPlugin.Capabilities,
		},
		"priority":         gap.Priority,
		"detected_at":      gap.DetectedAt,
		"updated_at":       gap.UpdatedAt,
		"status":           gap.Status,
		"occurrence_count": gap.OccurrenceCount,
		"space_id":         gap.SpaceID,
	})
}

// Helper function to extract gap ID from path
func extractGapID(path, suffix string) string {
	// Path format: /v1/system/capability-gaps/{id}/suffix
	path = strings.TrimSuffix(path, suffix)
	path = strings.TrimPrefix(path, "/v1/system/capability-gaps/")
	return path
}

// =============================================================================
// GAP INTERVIEW ENDPOINTS
// =============================================================================

// handleGapInterviews handles GET /v1/system/gap-interviews
// Returns pending interview prompts
func (s *Server) handleGapInterviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.gapInterviewer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "gap interviewer not available",
		})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	ctx := r.Context()

	prompts, err := s.gapInterviewer.GetPendingPrompts(ctx, spaceID)
	if err != nil {
		writeInternalError(w, err, "get pending prompts")
		return
	}

	// Get stats
	stats, err := s.gapInterviewer.GetInterviewStats(ctx)
	if err != nil {
		stats = map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"prompts": prompts,
		"stats":   stats,
	})
}

// handleRunGapInterview handles POST /v1/system/gap-interviews/run
// Manually triggers the weekly gap interview process
func (s *Server) handleRunGapInterview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.gapInterviewer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "gap interviewer not available",
		})
		return
	}

	// Parse optional config overrides
	var reqConfig struct {
		MaxPrompts     int     `json:"max_prompts"`
		MinPriority    float64 `json:"min_priority"`
		MinOccurrences int     `json:"min_occurrences"`
	}
	if r.ContentLength > 0 {
		if !readJSON(w, r, &reqConfig) {
			return
		}
	}

	// Build config
	cfg := gaps.DefaultInterviewConfig()
	if reqConfig.MaxPrompts > 0 {
		cfg.MaxPromptsPerRun = reqConfig.MaxPrompts
	}
	if reqConfig.MinPriority > 0 {
		cfg.MinPriority = reqConfig.MinPriority
	}
	if reqConfig.MinOccurrences > 0 {
		cfg.MinOccurrenceCount = reqConfig.MinOccurrences
	}

	ctx := r.Context()

	result, err := s.gapInterviewer.RunWeeklyInterview(ctx, cfg)
	if err != nil {
		writeInternalError(w, err, "run gap interview")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleAnswerGapInterview handles POST /v1/system/gap-interviews/{id}/answer
// Marks a prompt as answered
func (s *Server) handleAnswerGapInterview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.gapInterviewer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "gap interviewer not available",
		})
		return
	}

	// Extract prompt ID from path
	promptID := extractInterviewID(r.URL.Path, "/answer")
	if promptID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid prompt_id in path"})
		return
	}

	var req struct {
		ObservationNodeID string `json:"observation_node_id"`
	}
	if r.ContentLength > 0 {
		if !readJSON(w, r, &req) {
			return
		}
	}

	ctx := r.Context()

	if err := s.gapInterviewer.AnswerPrompt(ctx, promptID, req.ObservationNodeID); err != nil {
		writeInternalError(w, err, "answer gap interview")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"prompt_id": promptID,
		"status":    "answered",
	})
}

// handleSkipGapInterview handles POST /v1/system/gap-interviews/{id}/skip
// Marks a prompt as skipped
func (s *Server) handleSkipGapInterview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.gapInterviewer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "gap interviewer not available",
		})
		return
	}

	// Extract prompt ID from path
	promptID := extractInterviewID(r.URL.Path, "/skip")
	if promptID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid prompt_id in path"})
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	ctx := r.Context()

	if err := s.gapInterviewer.SkipPrompt(ctx, promptID, req.Reason); err != nil {
		writeInternalError(w, err, "skip gap interview")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"prompt_id": promptID,
		"status":    "skipped",
	})
}

// handleGapInterviewStats handles GET /v1/system/gap-interviews/stats
// Returns interview statistics
func (s *Server) handleGapInterviewStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.gapInterviewer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "gap interviewer not available",
		})
		return
	}

	ctx := r.Context()

	stats, err := s.gapInterviewer.GetInterviewStats(ctx)
	if err != nil {
		writeInternalError(w, err, "get interview stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleGapInterviewOperation routes interview requests to appropriate handler
func (s *Server) handleGapInterviewOperation(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.HasSuffix(path, "/run"):
		s.handleRunGapInterview(w, r)
	case strings.HasSuffix(path, "/stats"):
		s.handleGapInterviewStats(w, r)
	case strings.HasSuffix(path, "/answer"):
		s.handleAnswerGapInterview(w, r)
	case strings.HasSuffix(path, "/skip"):
		s.handleSkipGapInterview(w, r)
	default:
		// List all pending prompts
		s.handleGapInterviews(w, r)
	}
}

// Helper to extract interview prompt ID from path
func extractInterviewID(path, suffix string) string {
	// Path format: /v1/system/gap-interviews/{id}/suffix
	path = strings.TrimSuffix(path, suffix)
	path = strings.TrimPrefix(path, "/v1/system/gap-interviews/")
	return path
}
