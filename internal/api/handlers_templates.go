package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"mdemg/internal/conversation"
)

// Template request/response types

type CreateTemplateRequest struct {
	SpaceID     string                 `json:"space_id"`
	TemplateID  string                 `json:"template_id,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	ObsType     string                 `json:"obs_type,omitempty"`
	Schema      map[string]interface{} `json:"schema"`
	AutoCapture *AutoCaptureRequest    `json:"auto_capture,omitempty"`
}

type AutoCaptureRequest struct {
	OnSessionEnd bool `json:"on_session_end"`
	OnCompaction bool `json:"on_compaction"`
	OnError      bool `json:"on_error"`
}

type TemplateResponse struct {
	TemplateID  string                 `json:"template_id"`
	SpaceID     string                 `json:"space_id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	ObsType     string                 `json:"obs_type"`
	Schema      map[string]interface{} `json:"schema"`
	AutoCapture *AutoCaptureRequest    `json:"auto_capture,omitempty"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

type ListTemplatesResponse struct {
	Templates []TemplateResponse `json:"templates"`
	Count     int                `json:"count"`
}

// handleTemplates handles /v1/conversation/templates
func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListTemplates(w, r)
	case http.MethodPost:
		s.handleCreateTemplate(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTemplateByID handles /v1/conversation/templates/{id}
func (s *Server) handleTemplateByID(w http.ResponseWriter, r *http.Request) {
	// Extract template ID from path
	path := strings.TrimPrefix(r.URL.Path, "/v1/conversation/templates/")
	templateID := strings.TrimSuffix(path, "/")

	if templateID == "" {
		http.Error(w, "template_id is required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetTemplate(w, r, templateID)
	case http.MethodPut:
		s.handleUpdateTemplate(w, r, templateID)
	case http.MethodDelete:
		s.handleDeleteTemplate(w, r, templateID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		http.Error(w, "space_id query parameter is required", http.StatusBadRequest)
		return
	}

	templates, err := s.templateService.ListTemplates(r.Context(), spaceID)
	if err != nil {
		writeInternalError(w, err, "template.list")
		return
	}

	response := ListTemplatesResponse{
		Templates: make([]TemplateResponse, 0, len(templates)),
		Count:     len(templates),
	}

	for _, t := range templates {
		response.Templates = append(response.Templates, templateToResponse(t))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SpaceID == "" {
		http.Error(w, "space_id is required", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	template := &conversation.ObservationTemplate{
		TemplateID:  req.TemplateID,
		SpaceID:     req.SpaceID,
		Name:        req.Name,
		Description: req.Description,
		ObsType:     conversation.ObservationType(req.ObsType),
		Schema:      req.Schema,
	}

	if req.AutoCapture != nil {
		template.AutoCapture = &conversation.AutoCaptureConfig{
			OnSessionEnd: req.AutoCapture.OnSessionEnd,
			OnCompaction: req.AutoCapture.OnCompaction,
			OnError:      req.AutoCapture.OnError,
		}
	}

	if err := s.templateService.CreateTemplate(r.Context(), template); err != nil {
		writeInternalError(w, err, "template.create")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(templateToResponse(template))
}

func (s *Server) handleGetTemplate(w http.ResponseWriter, r *http.Request, templateID string) {
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		http.Error(w, "space_id query parameter is required", http.StatusBadRequest)
		return
	}

	template, err := s.templateService.GetTemplate(r.Context(), spaceID, templateID)
	if err != nil {
		writeInternalError(w, err, "template.get")
		return
	}
	if template == nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(templateToResponse(template))
}

func (s *Server) handleUpdateTemplate(w http.ResponseWriter, r *http.Request, templateID string) {
	var req CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SpaceID == "" {
		http.Error(w, "space_id is required", http.StatusBadRequest)
		return
	}

	template := &conversation.ObservationTemplate{
		TemplateID:  templateID,
		SpaceID:     req.SpaceID,
		Name:        req.Name,
		Description: req.Description,
		ObsType:     conversation.ObservationType(req.ObsType),
		Schema:      req.Schema,
	}

	if req.AutoCapture != nil {
		template.AutoCapture = &conversation.AutoCaptureConfig{
			OnSessionEnd: req.AutoCapture.OnSessionEnd,
			OnCompaction: req.AutoCapture.OnCompaction,
			OnError:      req.AutoCapture.OnError,
		}
	}

	if err := s.templateService.UpdateTemplate(r.Context(), template); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Template not found", http.StatusNotFound)
			return
		}
		writeInternalError(w, err, "template.update")
		return
	}

	// Fetch updated template
	updated, _ := s.templateService.GetTemplate(r.Context(), req.SpaceID, templateID)
	if updated != nil {
		template = updated
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(templateToResponse(template))
}

func (s *Server) handleDeleteTemplate(w http.ResponseWriter, r *http.Request, templateID string) {
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		http.Error(w, "space_id query parameter is required", http.StatusBadRequest)
		return
	}

	if err := s.templateService.DeleteTemplate(r.Context(), spaceID, templateID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Template not found", http.StatusNotFound)
			return
		}
		writeInternalError(w, err, "template.delete")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func templateToResponse(t *conversation.ObservationTemplate) TemplateResponse {
	resp := TemplateResponse{
		TemplateID:  t.TemplateID,
		SpaceID:     t.SpaceID,
		Name:        t.Name,
		Description: t.Description,
		ObsType:     string(t.ObsType),
		Schema:      t.Schema,
		CreatedAt:   t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if t.AutoCapture != nil {
		resp.AutoCapture = &AutoCaptureRequest{
			OnSessionEnd: t.AutoCapture.OnSessionEnd,
			OnCompaction: t.AutoCapture.OnCompaction,
			OnError:      t.AutoCapture.OnError,
		}
	}

	return resp
}
