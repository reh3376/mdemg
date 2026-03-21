package api

import (
	"context"
	"net/http"
	"time"

	"mdemg/internal/jiminy"
)

// handleJiminyGuide handles POST /v1/jiminy/guide
func (s *Server) handleJiminyGuide(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy guidance is not enabled (set JIMINY_ENABLED=true)",
		})
		return
	}

	var req struct {
		SpaceID     string `json:"space_id"`
		Context     string `json:"context"`
		FilePath    string `json:"file_path,omitempty"`
		AgentOutput string `json:"agent_output,omitempty"`
		Query       string `json:"query,omitempty"`
		SessionID   string `json:"session_id,omitempty"`
		MaxItems    int    `json:"max_items,omitempty"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}
	if req.Context == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "context is required"})
		return
	}

	resp, err := s.jiminySvc.Guide(r.Context(), jiminy.GuidanceRequest{
		SpaceID:     req.SpaceID,
		Context:     req.Context,
		FilePath:    req.FilePath,
		AgentOutput: req.AgentOutput,
		Query:       req.Query,
		SessionID:   req.SessionID,
		MaxItems:    req.MaxItems,
	})
	if err != nil {
		writeInternalError(w, err, "jiminy guide")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}

// handleJiminyFeedback handles POST /v1/jiminy/feedback
func (s *Server) handleJiminyFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy guidance is not enabled (set JIMINY_ENABLED=true)",
		})
		return
	}

	var req jiminy.GuidanceFeedbackRequest
	if !readJSON(w, r, &req) {
		return
	}

	if req.GuidanceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "guidance_id is required"})
		return
	}

	resp, err := s.jiminySvc.RecordOutcome(r.Context(), req)
	if err != nil {
		writeInternalError(w, err, "jiminy feedback")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}

// handleJiminyEvaluate handles POST /v1/jiminy/evaluate (J9)
func (s *Server) handleJiminyEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy guidance is not enabled (set JIMINY_ENABLED=true)",
		})
		return
	}

	evaluator := s.jiminySvc.GetEvaluator()
	if evaluator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy evaluator is not enabled (set JIMINY_EVALUATE_ENABLED=true)",
		})
		return
	}

	var req jiminy.EvaluateRequest
	if !readJSON(w, r, &req) {
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}
	if req.AgentOutput == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "agent_output is required"})
		return
	}

	// Apply evaluate timeout
	timeoutMs := s.cfg.JiminyEvaluateTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 3000
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	resp, err := evaluator.Evaluate(ctx, req)
	if err != nil {
		writeInternalError(w, err, "jiminy evaluate")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}
