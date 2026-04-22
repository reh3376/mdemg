package api

import (
	"net/http"

	"mdemg/internal/guardrail"
	"mdemg/internal/llmclient"
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
