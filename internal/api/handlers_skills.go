package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/conversation"
	"mdemg/internal/models"
)

// =============================================================================
// PHASE 48: SKILL REGISTRY API
// Skills are CMS pinned observations with skill:<name> tags.
// These endpoints provide convenience wrappers over observe/recall.
// =============================================================================

// --- Request/Response types (local to skill registry) ---

type SkillRecallRequest struct {
	SpaceID string `json:"space_id" validate:"required"`
	Section string `json:"section,omitempty"`
	Query   string `json:"query,omitempty"`
	TopK    int    `json:"top_k,omitempty"`
}

type SkillRecallResponse struct {
	SpaceID string              `json:"space_id"`
	Skill   string              `json:"skill"`
	Section string              `json:"section,omitempty"`
	Query   string              `json:"query"`
	Results []models.RecallResult `json:"results"`
	Debug   map[string]any      `json:"debug,omitempty"`
}

type SkillRegisterRequest struct {
	SpaceID     string         `json:"space_id" validate:"required"`
	SessionID   string         `json:"session_id,omitempty"`
	Description string         `json:"description"`
	Sections    []SkillSection `json:"sections" validate:"required,min=1"`
}

type SkillSection struct {
	Name    string   `json:"name" validate:"required"`
	Content string   `json:"content" validate:"required"`
	Tags    []string `json:"tags,omitempty"`
}

type SkillRegisterResponse struct {
	Skill           string   `json:"skill"`
	SpaceID         string   `json:"space_id"`
	SectionsCreated int      `json:"sections_created"`
	ObservationIDs  []string `json:"observation_ids"`
}

type SkillInfo struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Sections         []string `json:"sections"`
	ObservationCount int      `json:"observation_count"`
}

type SkillListResponse struct {
	SpaceID string      `json:"space_id"`
	Skills  []SkillInfo `json:"skills"`
	Count   int         `json:"count"`
}

// --- Handlers ---

// handleSkills routes GET /v1/skills → list skills
func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	s.handleListSkills(w, r)
}

// handleSkillOperation routes /v1/skills/{name}/{action}
func (s *Server) handleSkillOperation(w http.ResponseWriter, r *http.Request) {
	// Parse path: /v1/skills/{name}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/v1/skills/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "skill name required"})
		return
	}

	name := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch action {
	case "recall":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		s.handleSkillRecall(w, r, name)
	case "register":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		s.handleSkillRegister(w, r, name)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown skill action: " + action})
	}
}

// handleListSkills discovers registered skills from pinned observations with skill:* tags.
// GET /v1/skills?space_id=X
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter required"})
		return
	}

	ctx := r.Context()
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Find all pinned observations with skill:* tags, extract skill names and sections
	query := `
		MATCH (o:MemoryNode)
		WHERE o.space_id = $space_id
		  AND o.role_type = 'conversation_observation'
		  AND o.pinned = true
		  AND any(t IN o.tags WHERE t STARTS WITH 'skill:')
		RETURN o.tags AS tags, o.content AS content
	`

	collected, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, query, map[string]any{"space_id": spaceID})
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		writeInternalError(w, err, "list skills")
		return
	}

	records := collected.([]*neo4j.Record)

	// Group by skill name
	type skillData struct {
		sections map[string]bool
		obsCount int
		desc     string
	}
	skills := make(map[string]*skillData)

	for _, record := range records {
		tagsRaw, _ := record.Get("tags")
		contentRaw, _ := record.Get("content")
		content, _ := contentRaw.(string)

		tags, ok := tagsRaw.([]any)
		if !ok {
			continue
		}

		for _, tagRaw := range tags {
			tag, ok := tagRaw.(string)
			if !ok || !strings.HasPrefix(tag, "skill:") {
				continue
			}

			parts := strings.SplitN(tag, ":", 3)
			if len(parts) < 2 {
				continue
			}

			skillName := parts[1]
			if skillName == "" {
				continue
			}

			sd, exists := skills[skillName]
			if !exists {
				sd = &skillData{sections: make(map[string]bool)}
				skills[skillName] = sd
			}

			// Only count once per observation (not per tag)
			if len(parts) == 2 {
				sd.obsCount++
				// Use first ~100 chars of content as description if not set
				if sd.desc == "" && content != "" {
					desc := content
					if len(desc) > 100 {
						desc = desc[:100] + "..."
					}
					sd.desc = desc
				}
			} else if len(parts) == 3 && parts[2] != "" {
				sd.sections[parts[2]] = true
			}
		}
	}

	// Build response
	skillList := make([]SkillInfo, 0, len(skills))
	for name, sd := range skills {
		sections := make([]string, 0, len(sd.sections))
		for sec := range sd.sections {
			sections = append(sections, sec)
		}
		skillList = append(skillList, SkillInfo{
			Name:             name,
			Description:      sd.desc,
			Sections:         sections,
			ObservationCount: sd.obsCount,
		})
	}

	writeJSON(w, http.StatusOK, SkillListResponse{
		SpaceID: spaceID,
		Skills:  skillList,
		Count:   len(skillList),
	})
}

// handleSkillRecall retrieves skill content via direct Cypher query on pinned observations.
// Uses tag-based filtering rather than vector similarity, ensuring reliable skill retrieval.
// POST /v1/skills/{name}/recall
func (s *Server) handleSkillRecall(w http.ResponseWriter, r *http.Request, name string) {
	var req SkillRecallRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	// Build tag filter
	tagFilter := fmt.Sprintf("skill:%s", name)
	if req.Section != "" {
		tagFilter = fmt.Sprintf("skill:%s:%s", name, req.Section)
	}

	query := req.Query
	if query == "" {
		query = fmt.Sprintf("skill %s instructions", name)
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}

	ctx := r.Context()
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Direct Cypher query for pinned skill observations by tag
	cypher := `
		MATCH (o:MemoryNode)
		WHERE o.space_id = $spaceId
		  AND o.role_type = 'conversation_observation'
		  AND o.pinned = true
		  AND $tagFilter IN o.tags
		RETURN o.node_id AS nodeId, o.content AS content, o.summary AS summary,
		       o.obs_type AS obsType, o.tags AS tags
		ORDER BY o.created_at DESC
		LIMIT $topK
	`

	collected, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":   req.SpaceID,
			"tagFilter": tagFilter,
			"topK":      topK,
		})
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		writeInternalError(w, err, "skill recall")
		return
	}

	records := collected.([]*neo4j.Record)
	apiResults := make([]models.RecallResult, 0)
	for _, rec := range records {
		nodeID, _ := rec.Get("nodeId")
		content, _ := rec.Get("content")
		summary, _ := rec.Get("summary")

		contentStr, _ := content.(string)
		if contentStr == "" {
			contentStr, _ = summary.(string)
		}

		nodeIDStr, _ := nodeID.(string)

		apiResults = append(apiResults, models.RecallResult{
			Type:    "conversation_observation",
			NodeID:  nodeIDStr,
			Content: contentStr,
			Score:   1.0, // Direct tag match = perfect relevance
			Layer:   0,
		})
	}

	writeJSON(w, http.StatusOK, SkillRecallResponse{
		SpaceID: req.SpaceID,
		Skill:   name,
		Section: req.Section,
		Query:   query,
		Results: apiResults,
		Debug: map[string]any{
			"tag_filter":       tagFilter,
			"observation_count": len(apiResults),
		},
	})
}

// handleSkillRegister registers skill sections as pinned observations.
// POST /v1/skills/{name}/register
func (s *Server) handleSkillRegister(w http.ResponseWriter, r *http.Request, name string) {
	if s.conversationSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "conversation service not available (embedder required)",
		})
		return
	}

	var req SkillRegisterRequest
	if !readJSON(w, r, &req) {
		return
	}
	if !validateRequest(w, &req) {
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = "skill-registry"
	}

	ctx := r.Context()
	obsIDs := make([]string, 0, len(req.Sections))

	for _, section := range req.Sections {
		// Build tags: skill:<name>, skill:<name>:<section>, plus any user tags
		tags := []string{
			fmt.Sprintf("skill:%s", name),
			fmt.Sprintf("skill:%s:%s", name, section.Name),
		}
		tags = append(tags, section.Tags...)

		observeReq := conversation.ObserveRequest{
			SpaceID:   req.SpaceID,
			SessionID: sessionID,
			Content:   section.Content,
			ObsType:   "decision",
			Tags:      tags,
			Pinned:    true,
		}

		resp, err := s.conversationSvc.Observe(ctx, observeReq)
		if err != nil {
			writeInternalError(w, err, fmt.Sprintf("skill register section %s", section.Name))
			return
		}

		obsIDs = append(obsIDs, resp.ObsID)
	}

	writeJSON(w, http.StatusOK, SkillRegisterResponse{
		Skill:           name,
		SpaceID:         req.SpaceID,
		SectionsCreated: len(req.Sections),
		ObservationIDs:  obsIDs,
	})
}
