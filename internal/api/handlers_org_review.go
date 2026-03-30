package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"mdemg/internal/conversation"
)

// handleFlagForOrgReview handles POST /v1/conversation/observations/{obs_id}/flag-org
func (s *Server) handleFlagForOrgReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract obs_id from path
	path := strings.TrimPrefix(r.URL.Path, "/v1/conversation/observations/")
	path = strings.TrimSuffix(path, "/flag-org")
	obsID := path

	if obsID == "" {
		http.Error(w, "obs_id is required", http.StatusBadRequest)
		return
	}

	var req struct {
		SpaceID             string `json:"space_id"`
		Reason              string `json:"reason,omitempty"`
		SuggestedVisibility string `json:"suggested_visibility,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SpaceID == "" {
		http.Error(w, "space_id is required", http.StatusBadRequest)
		return
	}

	flagReq := &conversation.FlagForReviewRequest{
		ObsID:               obsID,
		SpaceID:             req.SpaceID,
		Reason:              req.Reason,
		SuggestedVisibility: req.SuggestedVisibility,
		FlaggedBy:           r.Header.Get("X-Agent-ID"), // Optional agent identifier
	}

	resp, err := s.orgReviewService.FlagForReview(r.Context(), flagReq)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Observation not found", http.StatusNotFound)
			return
		}
		writeInternalError(w, err, "org_review.flag")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// handleListOrgReviews handles GET /v1/conversation/org-reviews
func (s *Server) handleListOrgReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		http.Error(w, "space_id query parameter is required", http.StatusBadRequest)
		return
	}

	status := r.URL.Query().Get("status")
	if status != "" && status != "pending" {
		http.Error(w, "Only status=pending is supported", http.StatusBadRequest)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	req := &conversation.ListPendingReviewsRequest{
		SpaceID: spaceID,
		Limit:   limit,
	}

	resp, err := s.orgReviewService.ListPendingReviews(r.Context(), req)
	if err != nil {
		writeInternalError(w, err, "org_review.list")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleOrgReviewDecision handles POST /v1/conversation/org-reviews/{obs_id}/decision
func (s *Server) handleOrgReviewDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract obs_id from path
	path := strings.TrimPrefix(r.URL.Path, "/v1/conversation/org-reviews/")
	path = strings.TrimSuffix(path, "/decision")
	obsID := path

	if obsID == "" {
		http.Error(w, "obs_id is required", http.StatusBadRequest)
		return
	}

	var req struct {
		SpaceID    string `json:"space_id"`
		Decision   string `json:"decision"`
		Visibility string `json:"visibility,omitempty"`
		Notes      string `json:"notes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SpaceID == "" {
		http.Error(w, "space_id is required", http.StatusBadRequest)
		return
	}

	if !conversation.ValidOrgReviewDecision(req.Decision) {
		http.Error(w, "decision must be 'approve' or 'reject'", http.StatusBadRequest)
		return
	}

	decisionReq := &conversation.ReviewDecisionRequest{
		ObsID:         obsID,
		SpaceID:       req.SpaceID,
		Decision:      conversation.OrgReviewDecision(req.Decision),
		NewVisibility: req.Visibility,
		ReviewedBy:    r.Header.Get("X-User-ID"), // Optional user identifier
		Notes:         req.Notes,
	}

	resp, err := s.orgReviewService.ProcessDecision(r.Context(), decisionReq)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not pending") {
			http.Error(w, "Observation not found or not pending review", http.StatusNotFound)
			return
		}
		writeInternalError(w, err, "org_review.decision")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleOrgReviewStats handles GET /v1/conversation/org-reviews/stats
func (s *Server) handleOrgReviewStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		http.Error(w, "space_id query parameter is required", http.StatusBadRequest)
		return
	}

	stats, err := s.orgReviewService.GetReviewStats(r.Context(), spaceID)
	if err != nil {
		writeInternalError(w, err, "org_review.stats")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
