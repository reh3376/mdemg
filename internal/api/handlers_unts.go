package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// handleHashVerificationRegister handles POST /v1/hash-verification/register
func (s *Server) handleHashVerificationRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.untsRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "hash verification not enabled"})
		return
	}

	var req struct {
		Path      string `json:"path"`
		Framework string `json:"framework"`
		Hash      string `json:"hash"`
		SourceRef string `json:"source_ref"`
		Source    string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path is required"})
		return
	}

	if err := s.untsRegistry.Register(req.Path, req.Framework, req.Hash, req.SourceRef, req.Source); err != nil {
		writeInternalError(w, err, "hash_verification.register")
		return
	}
	if err := s.untsRegistry.Save(); err != nil {
		slog.Error("hash-verification save after register failed", "error", err)
	}

	rec, ok := s.untsRegistry.Get(req.Path)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "registered file not found"})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleHashVerificationFileRoute routes /v1/hash-verification/files/{path...}
// Path suffix present = Get single file, no suffix = List
func (s *Server) handleHashVerificationFileRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.untsRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "hash verification not enabled"})
		return
	}

	filePath := strings.TrimPrefix(r.URL.Path, "/v1/hash-verification/files/")
	if filePath == "" {
		// No path suffix — delegate to list
		s.handleHashVerificationList(w, r)
		return
	}

	rec, ok := s.untsRegistry.Get(filePath)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "file not tracked"})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleHashVerificationList handles GET /v1/hash-verification/files
func (s *Server) handleHashVerificationList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.untsRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "hash verification not enabled"})
		return
	}

	framework := r.URL.Query().Get("framework")
	status := r.URL.Query().Get("status")

	files := s.untsRegistry.List(framework, status)
	writeJSON(w, http.StatusOK, map[string]any{
		"files": files,
		"total": len(files),
	})
}

// handleHashVerificationVerify handles POST /v1/hash-verification/verify
func (s *Server) handleHashVerificationVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.untsRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "hash verification not enabled"})
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path is required"})
		return
	}

	result, err := s.untsRegistry.Verify(req.Path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "file not tracked"})
		return
	}
	if err := s.untsRegistry.Save(); err != nil {
		slog.Error("hash-verification save after verify failed", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":          result.Path,
		"status":        result.Status,
		"expected_hash": result.ExpectedHash,
		"actual_hash":   result.ActualHash,
	})
}

// handleHashVerificationVerifyAll handles POST /v1/hash-verification/verify-all
func (s *Server) handleHashVerificationVerifyAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.untsRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "hash verification not enabled"})
		return
	}

	var req struct {
		Framework string `json:"framework"`
	}
	// Body is optional for verify-all
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	results := s.untsRegistry.VerifyAll(req.Framework)
	if err := s.untsRegistry.Save(); err != nil {
		slog.Error("hash-verification save after verify-all failed", "error", err)
	}

	verified := 0
	mismatched := 0
	resultList := make([]map[string]any, 0, len(results))
	for _, r := range results {
		switch r.Status {
		case "verified":
			verified++
		case "mismatch":
			mismatched++
		}
		resultList = append(resultList, map[string]any{
			"path":          r.Path,
			"status":        r.Status,
			"expected_hash": r.ExpectedHash,
			"actual_hash":   r.ActualHash,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"results":    resultList,
		"total":      len(results),
		"verified":   verified,
		"mismatched": mismatched,
	})
}

// handleHashVerificationUpdate handles POST /v1/hash-verification/update
func (s *Server) handleHashVerificationUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.untsRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "hash verification not enabled"})
		return
	}

	var req struct {
		Path   string `json:"path"`
		Hash   string `json:"hash"`
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path is required"})
		return
	}

	if err := s.untsRegistry.UpdateHash(req.Path, req.Hash, req.Source); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "file not tracked"})
		return
	}
	if err := s.untsRegistry.Save(); err != nil {
		slog.Error("hash-verification save after update failed", "error", err)
	}

	rec, ok := s.untsRegistry.Get(req.Path)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "updated file not found"})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleHashVerificationRevert handles POST /v1/hash-verification/revert
func (s *Server) handleHashVerificationRevert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.untsRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "hash verification not enabled"})
		return
	}

	var req struct {
		Path       string `json:"path"`
		TargetHash string `json:"target_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path is required"})
		return
	}

	if err := s.untsRegistry.RevertHash(req.Path, req.TargetHash); err != nil {
		if strings.Contains(err.Error(), "not tracked") {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "file not tracked"})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid revert request"})
		}
		return
	}
	if err := s.untsRegistry.Save(); err != nil {
		slog.Error("hash-verification save after revert failed", "error", err)
	}

	rec, ok := s.untsRegistry.Get(req.Path)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "reverted file not found"})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleHashVerificationScan handles POST /v1/hash-verification/scan
func (s *Server) handleHashVerificationScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.untsRegistry == nil || s.untsScanner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "hash verification not enabled"})
		return
	}

	if err := s.untsScanner.ScanAll(); err != nil {
		writeInternalError(w, err, "hash_verification.scan")
		return
	}
	if err := s.untsRegistry.Save(); err != nil {
		slog.Error("hash-verification save after scan failed", "error", err)
	}

	files := s.untsRegistry.List("", "")
	writeJSON(w, http.StatusOK, map[string]any{
		"scanned":          true,
		"files_registered": len(files),
	})
}
