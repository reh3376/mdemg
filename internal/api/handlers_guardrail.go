package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"mdemg/internal/guardrail"
	"mdemg/internal/llmclient"
	"mdemg/internal/metrics"
	"mdemg/internal/models"
)

// handleGuardrailValidate handles POST /v1/memory/guardrail/validate
func (s *Server) handleGuardrailValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if s.guardrailValidator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "guardrail validation is not enabled",
		})
		return
	}

	var req models.GuardrailValidateRequest
	if !readJSON(w, r, &req) {
		return
	}

	// Validate required fields
	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id is required"})
		return
	}
	if len(req.FilesChanged) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "files_changed is required"})
		return
	}
	if req.Diff == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "diff is required"})
		return
	}

	// GUARDRAIL-PRODUCER-001: fire-and-forget producer path. The evaluation
	// detaches from the request context (a hook's short curl must never
	// caller-cancel it mid-LLM) and the handler answers immediately; the
	// llm_interactions row the detached call records IS the product.
	if req.Async {
		s.handleGuardrailProducerAsync(w, req)
		return
	}

	// Sprint FT-LORA-B: propagate the per-request space_id through the llmclient
	// context so llm_interactions rows are tagged with the caller's space_id
	// (not the server default). Matches the pattern at handlers.go:482 / :2250.
	ctx := r.Context()
	if req.SpaceID != "" {
		ctx = llmclient.WithSpaceID(ctx, req.SpaceID)
	}

	result, err := s.guardrailValidator.Validate(ctx, guardrail.ValidateRequest{
		SpaceID:         req.SpaceID,
		FilesChanged:    req.FilesChanged,
		Diff:            req.Diff,
		AgentTrustLevel: req.AgentTrustLevel, // F20: optional authority-level filtering
	})
	if err != nil {
		writeInternalError(w, err, "guardrail validate")
		return
	}

	// Map internal types to API types
	violations := make([]models.GuardrailViolation, 0, len(result.Violations))
	for _, v := range result.Violations {
		violations = append(violations, models.GuardrailViolation{
			ConstraintNodeID: v.ConstraintNodeID,
			Description:      v.Description,
			Rationale:        v.Rationale,
		})
	}

	warnings := make([]models.GuardrailViolation, 0, len(result.Warnings))
	for _, gw := range result.Warnings {
		warnings = append(warnings, models.GuardrailViolation{
			ConstraintNodeID: gw.ConstraintNodeID,
			Description:      gw.Description,
			Rationale:        gw.Rationale,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": models.GuardrailValidateResponse{
			Status:     result.Status,
			Violations: violations,
			Warnings:   warnings,
		},
	})
}

// handleGuardrailProducerAsync runs the GUARDRAIL-PRODUCER-001 fire-and-forget
// path: gate → bounded concurrency → evaluation detached from the request
// context → immediate 202. The llm_interactions row recorded by the detached
// Validate call is the product; the HTTP response carries no verdict.
func (s *Server) handleGuardrailProducerAsync(w http.ResponseWriter, req models.GuardrailValidateRequest) {
	producerCounter := func(status string) {
		if m := metrics.Metrics(); m != nil {
			m.GuardrailProducer(status).Inc()
		}
	}

	if !s.cfg.GuardrailProducerEnabled {
		producerCounter("disabled")
		writeJSON(w, http.StatusOK, map[string]any{"status": "disabled"})
		return
	}

	select {
	case s.guardrailProducerSem <- struct{}{}:
	default:
		// At capacity — drop rather than queue: the producer is opportunistic
		// sampling of real edits, not a delivery guarantee, and unbounded
		// backlog is exactly the llama-server saturation mode to avoid.
		producerCounter("dropped")
		writeJSON(w, http.StatusOK, map[string]any{"status": "dropped"})
		return
	}

	timeoutMs := s.cfg.GuardrailTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	// Detached from the request context by construction (fresh Background,
	// never r.Context()): the hook's curl exits in milliseconds and must not
	// caller-cancel the evaluation mid-LLM. Bounded by GuardrailTimeoutMs.
	valueCtx := context.Background()
	if req.SpaceID != "" {
		valueCtx = llmclient.WithSpaceID(valueCtx, req.SpaceID)
	}

	vreq := guardrail.ValidateRequest{
		SpaceID:         req.SpaceID,
		FilesChanged:    req.FilesChanged,
		Diff:            req.Diff,
		AgentTrustLevel: req.AgentTrustLevel,
	}
	go func() {
		defer func() { <-s.guardrailProducerSem }()
		bgCtx, cancel := context.WithTimeout(valueCtx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
		result, err := s.guardrailValidator.Validate(bgCtx, vreq)
		if err != nil {
			slog.Warn("guardrail producer: detached evaluation errored", "space_id", vreq.SpaceID, "error", err)
			return
		}
		slog.Info("guardrail producer: evaluation complete",
			"space_id", vreq.SpaceID,
			"files", len(vreq.FilesChanged),
			"status", result.Status,
			"violations", len(result.Violations),
			"warnings", len(result.Warnings))
	}()

	producerCounter("queued")
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued"})
}
