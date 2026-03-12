package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	pb "mdemg/api/modulepb"
	"mdemg/internal/plugins"
)

// isLinearUnavailableError checks if a CRUD error indicates the service is
// unconfigured or unavailable, warranting a 503 response.
func isLinearUnavailableError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "not configured") ||
		strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "unimplemented") ||
		strings.Contains(lower, "unknown service")
}

// linearErrorStatus returns the appropriate HTTP status for a CRUD error response.
func linearErrorStatus(errMsg string) int {
	if isLinearUnavailableError(errMsg) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadRequest
}

// findCRUDModule finds the first CRUD-capable module (or a specific one by ID).
func (s *Server) findCRUDModule(moduleID string) *plugins.ModuleInfo {
	if s.pluginMgr == nil {
		return nil
	}
	modules := s.pluginMgr.GetCRUDModules()
	if moduleID != "" {
		for _, m := range modules {
			if m.Manifest.ID == moduleID {
				return m
			}
		}
		return nil
	}
	// Return the first available CRUD module (linear-module)
	if len(modules) > 0 {
		return modules[0]
	}
	return nil
}

// handleLinearIssues handles /v1/linear/issues and /v1/linear/issues/{id}
func (s *Server) handleLinearIssues(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path if present: /v1/linear/issues/{id}
	path := strings.TrimPrefix(r.URL.Path, "/v1/linear/issues")
	path = strings.TrimPrefix(path, "/")
	entityID := path

	mod := s.findCRUDModule("linear-module")
	if mod == nil || mod.CRUDClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "Linear CRUD module not available",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodPost:
		if entityID != "" {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST not allowed on /issues/{id}"})
			return
		}
		s.handleLinearIssueCreate(ctx, w, r, mod)

	case http.MethodGet:
		if entityID != "" {
			s.handleLinearIssueRead(ctx, w, mod, entityID)
		} else {
			s.handleLinearIssueList(ctx, w, r, mod)
		}

	case http.MethodPut:
		if entityID == "" {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "PUT requires /issues/{id}"})
			return
		}
		s.handleLinearIssueUpdate(ctx, w, r, mod, entityID)

	case http.MethodDelete:
		if entityID == "" {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "DELETE requires /issues/{id}"})
			return
		}
		s.handleLinearIssueDelete(ctx, w, mod, entityID)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleLinearIssueCreate(ctx context.Context, w http.ResponseWriter, r *http.Request, mod *plugins.ModuleInfo) {
	var req map[string]string
	if !readJSON(w, r, &req) {
		return
	}

	if req["title"] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "title is required"})
		return
	}
	if req["team_id"] == "" {
		// Fall back to configured default
		if s.cfg.LinearTeamID != "" {
			req["team_id"] = s.cfg.LinearTeamID
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "team_id is required"})
			return
		}
	}

	resp, err := mod.CRUDClient.Create(ctx, &pb.CRUDCreateRequest{
		EntityType: "issue",
		Fields:     req,
	})
	if err != nil {
		if isLinearUnavailableError(err.Error()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		} else {
			writeInternalError(w, err, "linear.issue.create")
		}
		return
	}
	if !resp.Success {
		writeJSON(w, linearErrorStatus(resp.Error), map[string]any{"error": resp.Error})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"entity": entityToMap(resp.Entity)})
}

func (s *Server) handleLinearIssueRead(ctx context.Context, w http.ResponseWriter, mod *plugins.ModuleInfo, id string) {
	resp, err := mod.CRUDClient.Read(ctx, &pb.CRUDReadRequest{
		EntityType: "issue",
		Id:         id,
	})
	if err != nil {
		if isLinearUnavailableError(err.Error()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		} else {
			writeInternalError(w, err, "linear.issue.read")
		}
		return
	}
	if !resp.Success {
		status := linearErrorStatus(resp.Error)
		if status == http.StatusBadRequest && strings.Contains(resp.Error, "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": resp.Error})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"entity": entityToMap(resp.Entity)})
}

func (s *Server) handleLinearIssueList(ctx context.Context, w http.ResponseWriter, r *http.Request, mod *plugins.ModuleInfo) {
	q := r.URL.Query()

	filters := make(map[string]string)
	for _, key := range []string{"team", "state", "assignee", "project", "query"} {
		if v := q.Get(key); v != "" {
			filters[key] = v
		}
	}

	limit := int32(50)
	if l := q.Get("limit"); l != "" {
		// Simple parse
		var n int32
		for _, c := range l {
			if c >= '0' && c <= '9' {
				n = n*10 + int32(c-'0')
			}
		}
		if n > 0 {
			limit = n
		}
	}

	cursor := q.Get("cursor")

	resp, err := mod.CRUDClient.List(ctx, &pb.CRUDListRequest{
		EntityType: "issue",
		Filters:    filters,
		Limit:      limit,
		Cursor:     cursor,
	})
	if err != nil {
		if isLinearUnavailableError(err.Error()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		} else {
			writeInternalError(w, err, "linear.issue.list")
		}
		return
	}
	if !resp.Success {
		writeJSON(w, linearErrorStatus(resp.Error), map[string]any{"error": resp.Error})
		return
	}

	entities := make([]map[string]any, 0, len(resp.Entities))
	for _, e := range resp.Entities {
		entities = append(entities, entityToMap(e))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entities":    entities,
		"next_cursor": resp.NextCursor,
		"total_count": resp.TotalCount,
	})
}

func (s *Server) handleLinearIssueUpdate(ctx context.Context, w http.ResponseWriter, r *http.Request, mod *plugins.ModuleInfo, id string) {
	var req map[string]string
	if !readJSON(w, r, &req) {
		return
	}

	resp, err := mod.CRUDClient.Update(ctx, &pb.CRUDUpdateRequest{
		EntityType: "issue",
		Id:         id,
		Fields:     req,
	})
	if err != nil {
		if isLinearUnavailableError(err.Error()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		} else {
			writeInternalError(w, err, "linear.issue.update")
		}
		return
	}
	if !resp.Success {
		writeJSON(w, linearErrorStatus(resp.Error), map[string]any{"error": resp.Error})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"entity": entityToMap(resp.Entity)})
}

func (s *Server) handleLinearIssueDelete(ctx context.Context, w http.ResponseWriter, mod *plugins.ModuleInfo, id string) {
	resp, err := mod.CRUDClient.Delete(ctx, &pb.CRUDDeleteRequest{
		EntityType: "issue",
		Id:         id,
	})
	if err != nil {
		if isLinearUnavailableError(err.Error()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		} else {
			writeInternalError(w, err, "linear.issue.delete")
		}
		return
	}
	if !resp.Success {
		writeJSON(w, linearErrorStatus(resp.Error), map[string]any{"error": resp.Error})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleLinearProjects handles /v1/linear/projects and /v1/linear/projects/{id}
func (s *Server) handleLinearProjects(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/linear/projects")
	path = strings.TrimPrefix(path, "/")
	entityID := path

	mod := s.findCRUDModule("linear-module")
	if mod == nil || mod.CRUDClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "Linear CRUD module not available",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodPost:
		if entityID != "" {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST not allowed on /projects/{id}"})
			return
		}
		s.handleLinearProjectCreate(ctx, w, r, mod)

	case http.MethodGet:
		if entityID != "" {
			s.handleLinearProjectRead(ctx, w, mod, entityID)
		} else {
			s.handleLinearProjectList(ctx, w, r, mod)
		}

	case http.MethodPut:
		if entityID == "" {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "PUT requires /projects/{id}"})
			return
		}
		s.handleLinearProjectUpdate(ctx, w, r, mod, entityID)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleLinearProjectCreate(ctx context.Context, w http.ResponseWriter, r *http.Request, mod *plugins.ModuleInfo) {
	var req map[string]string
	if !readJSON(w, r, &req) {
		return
	}

	if req["name"] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
		return
	}

	resp, err := mod.CRUDClient.Create(ctx, &pb.CRUDCreateRequest{
		EntityType: "project",
		Fields:     req,
	})
	if err != nil {
		if isLinearUnavailableError(err.Error()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		} else {
			writeInternalError(w, err, "linear.project.create")
		}
		return
	}
	if !resp.Success {
		writeJSON(w, linearErrorStatus(resp.Error), map[string]any{"error": resp.Error})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"entity": entityToMap(resp.Entity)})
}

func (s *Server) handleLinearProjectRead(ctx context.Context, w http.ResponseWriter, mod *plugins.ModuleInfo, id string) {
	resp, err := mod.CRUDClient.Read(ctx, &pb.CRUDReadRequest{
		EntityType: "project",
		Id:         id,
	})
	if err != nil {
		if isLinearUnavailableError(err.Error()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		} else {
			writeInternalError(w, err, "linear.project.read")
		}
		return
	}
	if !resp.Success {
		status := linearErrorStatus(resp.Error)
		if status == http.StatusBadRequest && strings.Contains(resp.Error, "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": resp.Error})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"entity": entityToMap(resp.Entity)})
}

func (s *Server) handleLinearProjectList(ctx context.Context, w http.ResponseWriter, r *http.Request, mod *plugins.ModuleInfo) {
	q := r.URL.Query()

	limit := int32(50)
	if l := q.Get("limit"); l != "" {
		var n int32
		for _, c := range l {
			if c >= '0' && c <= '9' {
				n = n*10 + int32(c-'0')
			}
		}
		if n > 0 {
			limit = n
		}
	}

	cursor := q.Get("cursor")

	resp, err := mod.CRUDClient.List(ctx, &pb.CRUDListRequest{
		EntityType: "project",
		Limit:      limit,
		Cursor:     cursor,
	})
	if err != nil {
		if isLinearUnavailableError(err.Error()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		} else {
			writeInternalError(w, err, "linear.project.list")
		}
		return
	}
	if !resp.Success {
		writeJSON(w, linearErrorStatus(resp.Error), map[string]any{"error": resp.Error})
		return
	}

	entities := make([]map[string]any, 0, len(resp.Entities))
	for _, e := range resp.Entities {
		entities = append(entities, entityToMap(e))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entities":    entities,
		"next_cursor": resp.NextCursor,
		"total_count": resp.TotalCount,
	})
}

func (s *Server) handleLinearProjectUpdate(ctx context.Context, w http.ResponseWriter, r *http.Request, mod *plugins.ModuleInfo, id string) {
	var req map[string]string
	if !readJSON(w, r, &req) {
		return
	}

	resp, err := mod.CRUDClient.Update(ctx, &pb.CRUDUpdateRequest{
		EntityType: "project",
		Id:         id,
		Fields:     req,
	})
	if err != nil {
		if isLinearUnavailableError(err.Error()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		} else {
			writeInternalError(w, err, "linear.project.update")
		}
		return
	}
	if !resp.Success {
		writeJSON(w, linearErrorStatus(resp.Error), map[string]any{"error": resp.Error})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"entity": entityToMap(resp.Entity)})
}

// handleLinearComments handles POST /v1/linear/comments
func (s *Server) handleLinearComments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "only POST is allowed"})
		return
	}

	mod := s.findCRUDModule("linear-module")
	if mod == nil || mod.CRUDClient == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "Linear CRUD module not available",
		})
		return
	}

	var req map[string]string
	if !readJSON(w, r, &req) {
		return
	}

	if req["issue_id"] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "issue_id is required"})
		return
	}
	if req["body"] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := mod.CRUDClient.Create(ctx, &pb.CRUDCreateRequest{
		EntityType: "comment",
		Fields:     req,
	})
	if err != nil {
		if isLinearUnavailableError(err.Error()) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		} else {
			writeInternalError(w, err, "linear.comment.create")
		}
		return
	}
	if !resp.Success {
		writeJSON(w, linearErrorStatus(resp.Error), map[string]any{"error": resp.Error})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"entity": entityToMap(resp.Entity)})
}

// entityToMap converts a CRUDEntity protobuf to a map for JSON serialization.
func entityToMap(e *pb.CRUDEntity) map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"id":          e.Id,
		"entity_type": e.EntityType,
		"fields":      e.Fields,
		"created_at":  e.CreatedAt,
		"updated_at":  e.UpdatedAt,
	}
}
