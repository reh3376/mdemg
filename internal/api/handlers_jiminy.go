package api

import (
	"context"
	"net/http"
	"time"

	"mdemg/internal/jiminy"
)

// handleJiminyHealthz handles GET /v1/jiminy/healthz
// Lightweight liveness check for the Jiminy guidance subsystem.
func (s *Server) handleJiminyHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil || !s.cfg.JiminyEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "disabled",
			"enabled": false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"enabled": true,
	})
}

// handleJiminyReady handles GET /v1/jiminy/ready
// Comprehensive readiness check for the Jiminy guidance subsystem.
// Reports all feature flags, sub-service availability, config, and optional stats.
func (s *Server) handleJiminyReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil || !s.cfg.JiminyEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "disabled",
			"enabled": false,
			"message": "jiminy guidance is not enabled (set JIMINY_ENABLED=true)",
		})
		return
	}

	// Feature flags
	features := map[string]bool{
		"synthesis":          s.cfg.JiminySynthesisEnabled,
		"evaluate_llm":      s.cfg.JiminyEvaluateLLMEnabled,
		"outcome_llm":       s.cfg.JiminyOutcomeLLMEnabled,
		"outcome_classifier": s.cfg.JiminyOutcomeClassifierEnabled,
		"escalation":        s.cfg.JiminyEscalationEnabled,
		"persistence":       s.cfg.JiminyPersistenceEnabled,
		"cache":             s.cfg.JiminyCacheEnabled,
		"j17":               s.cfg.J17Enabled,
	}

	// Sub-service availability
	services := map[string]string{}
	if s.jiminySvc.GetEvaluator() != nil {
		services["evaluator"] = "available"
	} else {
		services["evaluator"] = "unavailable"
	}
	if s.jiminySvc.GetSequenceTracker() != nil {
		services["sequence_tracker"] = "available"
	} else {
		services["sequence_tracker"] = "unavailable"
	}
	if s.jiminySvc.GetTicketManager() != nil {
		services["ticket_manager"] = "available"
	} else {
		services["ticket_manager"] = "unavailable"
	}
	if s.jiminySvc.GetProtocolMetricsCollector() != nil {
		services["protocol_metrics"] = "available"
	} else {
		services["protocol_metrics"] = "unavailable"
	}

	// Config
	config := map[string]any{
		"timeout_ms":     s.cfg.JiminyTimeoutMs,
		"max_items":      s.cfg.JiminyMaxItems,
		"min_confidence": s.cfg.JiminyMinConfidence,
	}
	if s.cfg.JiminySynthesisEnabled {
		config["synthesis_provider"] = s.cfg.JiminySynthesisProvider
		config["synthesis_model"] = s.cfg.JiminySynthesisModel
	}
	if s.cfg.J17Enabled {
		config["j17_sidecar_url"] = s.cfg.J17SidecarURL
	}

	result := map[string]any{
		"status":   "ready",
		"enabled":  true,
		"features": features,
		"services": services,
		"config":   config,
	}

	// Optional: include guidance stats if ?stats=true
	if r.URL.Query().Get("stats") == "true" {
		spaceID := r.URL.Query().Get("space_id")
		if spaceID == "" {
			spaceID = "mdemg-dev"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if stats, err := s.jiminySvc.GetGuidanceStats(ctx, spaceID); err == nil {
			result["stats"] = stats
		}
		if metrics := s.jiminySvc.GetProtocolMetricsSnapshot(); metrics != nil {
			result["protocol_metrics"] = metrics
		}
	}

	writeJSON(w, http.StatusOK, result)
}

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
		timeoutMs = 30000
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
