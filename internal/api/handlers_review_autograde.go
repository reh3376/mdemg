// HITL-AUTOGRADE-PREVIEW-001 (2026-08-12) — /v1/review/autograde-preview.
//
// Deferred JIMINY-HITL-VELOCITY-001 MVP follow-up. Returns the autograder's
// PROPOSED rubric grades for a specific review item WITHOUT recording. The
// UI (internal/api/ui/tabs/review.js) uses this to pre-fill radios on item
// load so the operator hits `space` to accept-as-is OR `0-4` to override
// per-dimension. Cuts operator time-per-grade from ~10s to ~2s for items
// the autograder gets right.
//
// Arc-safety: this endpoint is READ-ONLY on the substrate. It calls the
// LLM (adds rows to llm_interactions) but does NOT write to review_grades
// or constraint_outcomes — the JIMINY-CEILING-BREAK-2 measurement window
// (2026-08-19) is unaffected.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"mdemg/internal/llmclient"
	"mdemg/internal/review"
)

// autogradePreviewRequest is the POST body — small, symmetric with the
// existing review-flow shape (dataset_id + item_id + space_id).
type autogradePreviewRequest struct {
	DatasetID string `json:"dataset_id"`
	ItemID    string `json:"item_id"`
	SpaceID   string `json:"space_id,omitempty"`
}

// autogradePreviewResponse is the shape the UI radios read.
// Dimensions is a map<dim_key, 0..4> aligned with the rubric's dimensions;
// the UI pre-fills the matching radio for each key.
type autogradePreviewResponse struct {
	Dimensions map[string]int `json:"dimensions"`
	Confidence float64        `json:"confidence"`
	Rationale  string         `json:"rationale,omitempty"`
	// Available is false when the autograder is not wired (no LLM endpoint
	// configured in this deployment); UI falls back to blank radios.
	Available bool `json:"available"`
	// SkippedReason names why no proposal was produced when Available=true
	// but Dimensions is empty (e.g. "confidence below min_confidence").
	SkippedReason string `json:"skipped_reason,omitempty"`
}

// getReviewAutograder builds the singleton lazily on first use. Reads
// LLM_ENDPOINT + LLM_MODEL from env (falls back to local defaults). Returns
// nil when no LLM endpoint is reachable — caller returns Available:false.
//
// Mirrors internal/cli/review.go::buildAutograder shape so the server
// autograder behaves identically to the CLI one (same model, same grader_id
// pattern).
func (s *Server) getReviewAutograder() *review.Autograder {
	s.reviewAutograderMu.Lock()
	defer s.reviewAutograderMu.Unlock()
	if s.reviewAutograder != nil {
		return s.reviewAutograder
	}
	// Config: reuse the server's LLM endpoint + model. Fall back to local
	// llama-server on the shipped port if unset.
	baseURL := s.cfg.LLMEndpoint
	if baseURL == "" {
		baseURL = os.Getenv("LLM_ENDPOINT")
	}
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8102/v1"
	}
	model := s.cfg.LLMModel
	if model == "" {
		model = os.Getenv("LLM_MODEL")
	}
	if model == "" {
		model = "mdemg-llm-v1"
	}
	llm := llmclient.New(llmclient.Config{
		Provider:  "openai", // openai-compat protocol; local llama-server
		Model:     model,
		BaseURL:   baseURL,
		TimeoutMs: 60000,
	}).WithContext("review.autograde-preview", "")
	adapter := &serverLLMGraderAdapter{c: llm}
	// MinConfidence=0 — the preview endpoint returns whatever the autograder
	// proposes; UI's operator confirms. The CLI autograder uses ≥0.80 as its
	// gate for AUTO-WRITE; that gate is not applicable here (no write).
	s.reviewAutograder = review.NewAutograder(review.AutograderConfig{
		LLM:           adapter,
		ModelID:       model,
		BinarySHA:     "preview",
		MinConfidence: 0,
	})
	return s.reviewAutograder
}

// serverLLMGraderAdapter satisfies review.LLMGrader against llmclient.Client.
// Mirrors internal/cli/review.go::llmGraderAdapter — kept package-local so
// api/ doesn't import cli/.
type serverLLMGraderAdapter struct {
	c *llmclient.Client
}

func (a *serverLLMGraderAdapter) CompleteJSON(ctx context.Context, sys, usr string, maxTokens int) (string, error) {
	msgs := []llmclient.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: usr},
	}
	temperature := 0.0
	return a.c.Complete(ctx, msgs, llmclient.CompleteOpts{
		MaxTokens:   maxTokens,
		Temperature: &temperature,
	})
}

// POST /v1/review/autograde-preview
func (s *Server) handleReviewAutogradePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if !s.reviewReady(w) {
		return
	}
	var req autogradePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.DatasetID == "" || req.ItemID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "dataset_id and item_id are required"})
		return
	}
	d, ok := s.reviewRegistry.Get(req.DatasetID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown dataset_id"})
		return
	}
	spaceID := req.SpaceID
	if spaceID == "" {
		spaceID = s.cfg.RSICWatchdogSpaceID
	}

	// Fetch the specific item.
	item, found, err := d.FetchItem(r.Context(), spaceID, req.ItemID)
	if err != nil {
		writeInternalError(w, err, "review autograde-preview fetch item")
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "item not found"})
		return
	}

	ag := s.getReviewAutograder()
	if ag == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": autogradePreviewResponse{Available: false},
		})
		return
	}

	// Dataset-specific autograde-prompt hint, if implemented (mirrors CLI).
	var hint string
	if hinter, ok := d.(interface{ AutogradePromptHint() string }); ok {
		hint = hinter.AutogradePromptHint()
	}

	rubric := d.Rubric()
	res, _, err := ag.GradeWithHint(r.Context(), req.DatasetID, item, rubric, hint)
	if err != nil {
		// Non-fatal: return Available:true + SkippedReason so UI shows blank
		// radios + a small warning. Better than 500-ing the whole preview.
		writeJSON(w, http.StatusOK, map[string]any{
			"data": autogradePreviewResponse{
				Available:     true,
				SkippedReason: fmt.Sprintf("autograde error: %v", err),
			},
		})
		return
	}

	resp := autogradePreviewResponse{
		Dimensions: res.Submission.Dimensions,
		Confidence: res.Confidence,
		Rationale:  res.Rationale,
		Available:  true,
	}
	if len(resp.Dimensions) == 0 {
		resp.SkippedReason = "autograder returned no dimensions"
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}
