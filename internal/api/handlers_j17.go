package api

import (
	"encoding/json"
	"net/http"

	"mdemg/internal/jiminy"
)

// handleJ17ProtocolMetrics returns a J17 protocol metrics snapshot.
// GET /v1/jiminy/protocol/metrics?space_id=...
func (s *Server) handleJ17ProtocolMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if !s.cfg.J17Enabled || s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "J17 protocol not enabled"})
		return
	}

	snapshot := s.jiminySvc.GetProtocolMetricsSnapshot()
	if snapshot == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"message": "protocol metrics collection not enabled",
			},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": snapshot})
}

// handleJ17Extension processes an agent's request for a protocol extension.
// POST /v1/jiminy/extension
func (s *Server) handleJ17Extension(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if !s.cfg.J17Enabled || s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "J17 protocol not enabled"})
		return
	}

	var req jiminy.ExtensionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if req.Extension == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "extension is required"})
		return
	}
	if req.SessionID == "" {
		req.SessionID = "claude-core"
	}

	resp := s.jiminySvc.NegotiateExtension(req)

	status := http.StatusOK
	if !resp.Granted {
		status = http.StatusForbidden
	}
	writeJSON(w, status, map[string]any{"data": resp})
}

// handleJ17ProtocolFeedback ingests comprehension test results into the learning loop.
// POST /v1/jiminy/protocol/feedback
func (s *Server) handleJ17ProtocolFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if !s.cfg.J17Enabled || s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "J17 protocol not enabled"})
		return
	}

	var req jiminy.ProtocolFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if len(req.Trials) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "trials array is required"})
		return
	}

	resp := s.jiminySvc.RecordProtocolFeedback(req)
	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// handleJ17ProtocolLearn regenerates a constraint code using LLM, given failure context.
// POST /v1/jiminy/protocol/learn
func (s *Server) handleJ17ProtocolLearn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if !s.cfg.J17Enabled || s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "J17 protocol not enabled"})
		return
	}

	var req struct {
		ConstraintType string `json:"constraint_type"`
		Description    string `json:"description"`
		OldCode        string `json:"old_code"`
		FailureReason  string `json:"failure_reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if req.OldCode == "" || req.Description == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "old_code and description are required"})
		return
	}

	newCode, err := s.jiminySvc.RegenerateCode(r.Context(), req.ConstraintType, req.Description, req.OldCode, req.FailureReason)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"old_code": req.OldCode,
			"new_code": newCode,
			"reason":   req.FailureReason,
		},
	})
}

// handleJ17Bootstrap returns the J17 protocol spec header for first sessions.
// GET /v1/jiminy/bootstrap?space_id=...
func (s *Server) handleJ17Bootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if !s.cfg.J17Enabled || s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "J17 protocol not enabled"})
		return
	}

	// Include glossary (DICT section) if space_id is provided and codes exist
	spaceID := r.URL.Query().Get("space_id")
	var bootstrap string
	if spaceID != "" {
		glossary := s.jiminySvc.GetGlossary(r.Context(), spaceID)
		if len(glossary) > 0 {
			bootstrap = jiminy.FormatBootstrapWithGlossary(glossary)
		} else {
			bootstrap = jiminy.FormatBootstrap()
		}
	} else {
		bootstrap = jiminy.FormatBootstrap()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"bootstrap":    bootstrap,
			"version":      jiminy.J17Version,
			"first_session": true,
		},
	})
}

// handleJ17Checkpoint issues a session ticket capturing current Jiminy state.
// POST /v1/jiminy/checkpoint
func (s *Server) handleJ17Checkpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if !s.cfg.J17Enabled || s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "J17 protocol not enabled"})
		return
	}

	var req jiminy.CheckpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}
	if req.SessionID == "" {
		req.SessionID = "claude-core"
	}

	resp, err := s.jiminySvc.Checkpoint(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// handleJ17ResumeProtocol restores Jiminy state from a session ticket.
// POST /v1/jiminy/resume-protocol
func (s *Server) handleJ17ResumeProtocol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if !s.cfg.J17Enabled || s.jiminySvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "J17 protocol not enabled"})
		return
	}

	var req jiminy.ResumeProtocolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if req.SpaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}
	if req.SessionID == "" {
		req.SessionID = "claude-core"
	}

	resp, err := s.jiminySvc.ResumeProtocol(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}
