package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"mdemg/internal/jiminy"
	"mdemg/internal/llmclient"
	"mdemg/internal/metrics"
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
		"evaluate_llm":       s.cfg.JiminyEvaluateLLMEnabled,
		"outcome_llm":        s.cfg.JiminyOutcomeLLMEnabled,
		"outcome_classifier": s.cfg.JiminyOutcomeClassifierEnabled,
		"escalation":         s.cfg.JiminyEscalationEnabled,
		"persistence":        s.cfg.JiminyPersistenceEnabled,
		"cache":              s.cfg.JiminyCacheEnabled,
		"j17":                s.cfg.J17Enabled,
		"ml_tier_prediction": s.cfg.J17MLTierPredictionEnabled,
		"nli_comprehension":  s.cfg.J17NLIComprehensionEnabled,
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

	// Sidecar health probe
	if s.cfg.J17SidecarURL == "" {
		services["sidecar"] = "not_configured"
	} else {
		sidecarClient := &http.Client{Timeout: 1 * time.Second}
		resp, err := sidecarClient.Get(s.cfg.J17SidecarURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				services["sidecar"] = "available"
			} else {
				services["sidecar"] = "unavailable"
			}
		} else {
			services["sidecar"] = "unavailable"
		}
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
		config["sidecar_mode"] = s.cfg.J17SidecarMode
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

	// GUIDANCE-SYNTH-001 fix-commit: the direct /guide handler had the same
	// hardcoded 30s cap as the warm path — it starved synthesis (up to ~50s on
	// the local model). Use the single config-driven compute budget.
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.JiminyWarmComputeTimeout())
	defer cancel()
	ctx = llmclient.WithSessionID(ctx, req.SessionID)

	resp, err := s.jiminySvc.Guide(ctx, jiminy.GuidanceRequest{
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

	// FEEDBACK-CTX fix (found live during SUPERVISOR-002): the hook fires
	// this endpoint with a 5s curl --max-time, but per-item Tier-2 outcome
	// classification (jiminy.evaluate_llm) routinely outlives the
	// connection — the request ctx then cancelled every in-flight LLM call
	// (657 "context canceled" rows/24h; outcomes silently degraded to the
	// heuristic). Detach from the connection lifetime and give processing
	// its own server-side budget (JIMINY_FEEDBACK_TIMEOUT_MS).
	ctx := context.WithoutCancel(r.Context())
	timeout := time.Duration(s.cfg.JiminyFeedbackTimeoutMs) * time.Millisecond
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	ctx = llmclient.WithSessionID(ctx, req.SessionID)

	resp, err := s.jiminySvc.RecordOutcome(ctx, req)
	if err != nil {
		writeInternalError(w, err, "jiminy feedback")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}

// handleJiminyWarm handles POST /v1/jiminy/warm
// Fire-and-forget async guidance pre-computation. Returns 202 immediately.
// The warm store decouples Guide() from the prompt hook timeout.
func (s *Server) handleJiminyWarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil || !s.cfg.JiminyEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy guidance is not enabled",
		})
		return
	}

	if s.warmStore == nil || !s.cfg.JiminyWarmEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy warm store is not enabled (set JIMINY_WARM_ENABLED=true)",
		})
		return
	}

	var req struct {
		SpaceID     string `json:"space_id"`
		ContextHint string `json:"context_hint,omitempty"`
		SessionID   string `json:"session_id,omitempty"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}

	// Debounce: skip if guidance computed recently
	ageMs := s.warmStore.Age(req.SpaceID)
	debounceMs := int64(s.cfg.JiminyWarmDebounceSec) * 1000
	if ageMs >= 0 && ageMs < debounceMs {
		if m := metrics.Metrics(); m != nil {
			m.JiminyWarmDebounced(req.SpaceID).Inc()
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":      "debounced",
			"age_ms":      ageMs,
			"debounce_ms": debounceMs,
		})
		return
	}

	// Fire-and-forget: run Guide() in background goroutine.
	// GUIDANCE-SYNTH-001: timeout is config-driven (was a hardcoded 30s that
	// starved synthesis — per-node classifier + synthesis exceeded 30s).
	warmComputeTimeout := s.cfg.JiminyWarmComputeTimeout()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), warmComputeTimeout)
		defer cancel()
		ctx = llmclient.WithSessionID(ctx, req.SessionID)

		start := time.Now()
		resp, err := s.jiminySvc.Guide(ctx, jiminy.GuidanceRequest{
			SpaceID:   req.SpaceID,
			Context:   req.ContextHint,
			SessionID: req.SessionID,
		})
		computeMs := time.Since(start).Milliseconds()

		if err != nil {
			slog.Warn("jiminy warm: Guide() failed", "space_id", req.SpaceID, "error", err, "compute_ms", computeMs)
			if m := metrics.Metrics(); m != nil {
				m.JiminyWarmErrors(req.SpaceID).Inc()
			}
			return
		}

		s.warmStore.Put(req.SpaceID, resp, req.ContextHint, computeMs)
		if m := metrics.Metrics(); m != nil {
			m.JiminyWarmCompleted(req.SpaceID).Inc()
			m.JiminyGuideCalls(req.SpaceID).Inc()
			if len(resp.Guidance) == 0 {
				m.JiminyGuideEmpty(req.SpaceID).Inc()
			}
		}
		slog.Info("jiminy warm: guidance pre-computed", "space_id", req.SpaceID, "items", len(resp.Guidance), "compute_ms", computeMs)
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "warming",
	})
}

// handleJiminyLatest handles GET /v1/jiminy/latest
// Instant read of pre-computed guidance from the warm store.
// Response time: <100ms (no Guide() computation).
func (s *Server) handleJiminyLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil || !s.cfg.JiminyEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy guidance is not enabled",
		})
		return
	}

	if s.warmStore == nil || !s.cfg.JiminyWarmEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy warm store is not enabled",
		})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query param is required"})
		return
	}

	entry := s.warmStore.Get(spaceID)
	if entry == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"warm":   false,
			"status": "no_guidance_available",
		})
		return
	}

	// Re-register in effectiveness tracker so late feedback can still correlate
	if s.jiminySvc != nil && entry.Response.GuidanceID != "" {
		s.jiminySvc.RefreshTrackedGuidance(entry.Response.GuidanceID, entry.Response.Guidance)
	}

	ageMs := time.Since(entry.ComputedAt).Milliseconds()
	maxAgeMs := int64(s.cfg.JiminyWarmMaxAgeSec) * 1000

	if m := metrics.Metrics(); m != nil {
		m.JiminyLatestServed(spaceID).Inc()
		m.JiminyLatestAge(spaceID).Set(float64(ageMs))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"warm":         true,
		"data":         entry.Response,
		"age_ms":       ageMs,
		"compute_ms":   entry.ComputeMs,
		"context_hint": entry.ContextHint,
		"stale":        ageMs > maxAgeMs,
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
	ctx = llmclient.WithSessionID(ctx, req.SessionID)

	resp, err := evaluator.Evaluate(ctx, req)
	if err != nil {
		writeInternalError(w, err, "jiminy evaluate")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}

// handleJiminyProtocolStatus handles GET /v1/jiminy/protocol/status (B4)
// Returns the current J17 protocol status for a session: trust score, tier,
// feedback count, and enabled state.
func (s *Server) handleJiminyProtocolStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil || !s.cfg.JiminyEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy guidance is not enabled (set JIMINY_ENABLED=true)",
		})
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "session_id query parameter is required",
		})
		return
	}

	status := s.jiminySvc.GetProtocolStatus(sessionID)
	writeJSON(w, http.StatusOK, map[string]any{
		"data": status,
	})
}

// handleJiminyClassify handles POST /v1/jiminy/classify — strict mode response classification.
func (s *Server) handleJiminyClassify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil || !s.cfg.JiminyEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy guidance is not enabled",
		})
		return
	}

	var req jiminy.ClassifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.SpaceID == "" || req.AgentOutput == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id and agent_output are required"})
		return
	}

	// 5-second deadline for PreToolUse hook timeout budget
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	ctx = llmclient.WithSessionID(ctx, req.SessionID)

	resp, err := s.jiminySvc.Classify(ctx, req)
	if err != nil {
		// Fail open on error
		writeJSON(w, http.StatusOK, map[string]any{
			"data": jiminy.ClassifyResponse{Verdict: "pass", Confidence: 0},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}

// handleJiminyReformulate handles POST /v1/jiminy/reformulate — strict mode directive generation.
func (s *Server) handleJiminyReformulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil || !s.cfg.JiminyEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy guidance is not enabled",
		})
		return
	}

	var req jiminy.GuidanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}
	// UATS-GAP-001 (live-caught while authoring the contract spec): the
	// service rejects an empty context, but the handler surfaced that
	// request-validation failure as a 500. Validate at the edge → 400.
	if req.Context == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "context is required"})
		return
	}

	// JIMINY-BUDGET-001: was a hardcoded 10s — shorter than every other
	// guidance budget while running the same synthesis-class work.
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.EffectiveJiminyReformulateTimeout())
	defer cancel()
	ctx = llmclient.WithSessionID(ctx, req.SessionID)

	resp, err := s.jiminySvc.Reformulate(ctx, req)
	if err != nil {
		writeInternalError(w, err, "jiminy.reformulate")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": resp,
	})
}

// handleJiminyStrict handles POST /v1/jiminy/strict — toggle strict mode.
func (s *Server) handleJiminyStrict(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "jiminy guidance is not enabled",
		})
		return
	}

	mgr := s.jiminySvc.GetStrictMode()
	if mgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "strict mode manager not initialized",
		})
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Enabled   bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "session_id is required"})
		return
	}

	var err error
	var msg string
	if req.Enabled {
		err = mgr.Enable(req.SessionID)
		msg = "strict mode enabled"
	} else {
		err = mgr.Disable(req.SessionID)
		msg = "strict mode disabled"
	}
	if err != nil {
		writeInternalError(w, err, "jiminy.strict")
		return
	}

	slog.Info("jiminy: strict mode toggled", "session_id", req.SessionID, "enabled", req.Enabled)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"session_id": req.SessionID,
			"strict":     req.Enabled,
			"message":    msg,
		},
	})
}
