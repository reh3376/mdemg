package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// JIMINY-RULES-UI-001 Epic 2 — READ-only endpoints for the /ui/rules tab.
//
// See docs/development/jiminy-rules-ui-001/sprint_plan.md (§5 Epic 2) and
// docs/features/jiminy-rules-ui.md for the API + shape contract.
//
// Two endpoints:
//   GET /v1/jiminy/rules             — list rules for a space with filters
//   GET /v1/jiminy/rules/{code}      — single-rule detail + recent outcomes
//
// Both are READ-only + arc-safe (no substrate mutation). Epic 3 will layer
// WRITE endpoints (create + tombstone) behind JiminyRulesUIWriteEnabled.

type rulesListItem struct {
	NodeID          string    `json:"node_id"`
	ConstraintCode  string    `json:"constraint_code"`
	RoleType        string    `json:"role_type"`
	ConstraintType  string    `json:"constraint_type"`
	IsInformational bool      `json:"is_informational"`
	Content         string    `json:"content"`
	Scope           string    `json:"scope,omitempty"`
	IsArchived      bool      `json:"is_archived"`
	ArchiveReason   string    `json:"archive_reason,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
}

type rulesOutcomeBucket struct {
	OutcomeType string `json:"outcome_type"`
	Count       int64  `json:"count"`
}

// handleRulesList — GET /v1/jiminy/rules
//
// Query params (all optional):
//   space_id         — required; falls back to RSICWatchdogSpaceID
//   type             — filter: "constraint" | "correction"
//   severity         — filter: "must" | "must_not" | "should" | "note"
//   category         — filter: "actionable" | "informational" (default: both)
//   scope            — filter: one of the LEVER-C-TIGHTEN-002 9 families
//   include_archived — bool ("true" | "false", default false)
//   limit            — int, capped at JiminyRulesListMaxLimit (default 200)
func (s *Server) handleRulesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	q := r.URL.Query()
	spaceID := q.Get("space_id")
	if spaceID == "" {
		spaceID = s.cfg.RSICWatchdogSpaceID
	}
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}
	if s.driver == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "Neo4j driver not available"})
		return
	}

	typeFilter := q.Get("type")
	severity := q.Get("severity")
	category := q.Get("category")
	scope := q.Get("scope")
	includeArchived := strings.EqualFold(q.Get("include_archived"), "true")
	limit := 50
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	maxLimit := s.cfg.JiminyRulesListMaxLimit
	if maxLimit <= 0 {
		maxLimit = 200
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	var conditions []string
	conditions = append(conditions, "n.space_id = $space_id")
	conditions = append(conditions, "n.role_type IN ['constraint','correction']")
	params := map[string]any{"space_id": spaceID, "limit": int64(limit)}

	if typeFilter == "constraint" || typeFilter == "correction" {
		conditions = append(conditions, "n.role_type = $role_type")
		params["role_type"] = typeFilter
	}
	if severity != "" {
		conditions = append(conditions, "n.constraint_type = $constraint_type")
		params["constraint_type"] = severity
	}
	switch strings.ToLower(category) {
	case "informational":
		conditions = append(conditions, "coalesce(n.is_informational, false) = true")
	case "actionable":
		conditions = append(conditions, "coalesce(n.is_informational, false) = false")
	}
	if scope != "" {
		conditions = append(conditions, "n.scope = $scope")
		params["scope"] = scope
	}
	if !includeArchived {
		conditions = append(conditions, "NOT coalesce(n.is_archived, false)")
	}

	cypher := "MATCH (n:MemoryNode) WHERE " + strings.Join(conditions, " AND ") +
		` RETURN n.node_id AS node_id,
		         coalesce(n.constraint_code, '') AS constraint_code,
		         n.role_type AS role_type,
		         coalesce(n.constraint_type, '') AS constraint_type,
		         coalesce(n.is_informational, false) AS is_informational,
		         coalesce(n.content, '') AS content,
		         coalesce(n.scope, '') AS scope,
		         coalesce(n.is_archived, false) AS is_archived,
		         coalesce(n.archive_reason, '') AS archive_reason,
		         n.created_at AS created_at
		ORDER BY n.created_at DESC
		LIMIT $limit`

	session := s.driver.NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(r.Context())
	result, err := session.ExecuteRead(r.Context(), func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(r.Context(), cypher, params)
		if err != nil {
			return nil, err
		}
		return res.Collect(r.Context())
	})
	if err != nil {
		writeInternalError(w, err, "rules list")
		return
	}
	records := result.([]*neo4j.Record)
	items := make([]rulesListItem, 0, len(records))
	for _, rec := range records {
		items = append(items, recordToRulesItem(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"items":            items,
			"count":            len(items),
			"space_id":         spaceID,
			"limit":            limit,
			"include_archived": includeArchived,
		},
	})
}

// handleRulesDetail — GET /v1/jiminy/rules/{code}
//
// Path: /v1/jiminy/rules/<constraint_code>
// Query: space_id (required; RSICWatchdogSpaceID fallback)
//
// Returns the single rule + last-N-hours outcome-count buckets from
// constraint_outcomes (N = JiminyRulesOutcomesLookbackHours; default 168 = 7d).
func (s *Server) handleRulesDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	code := strings.TrimPrefix(r.URL.Path, "/v1/jiminy/rules/")
	// Strip any trailing slash / subpath (defensive; no shipped subpath today)
	if i := strings.Index(code, "/"); i >= 0 {
		code = code[:i]
	}
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "constraint_code is required in path"})
		return
	}
	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		spaceID = s.cfg.RSICWatchdogSpaceID
	}
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "space_id query parameter is required"})
		return
	}
	if s.driver == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "Neo4j driver not available"})
		return
	}

	rule, found, err := s.fetchRuleByCode(r.Context(), spaceID, code)
	if err != nil {
		writeInternalError(w, err, "rules detail")
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":           "rule not found",
			"space_id":        spaceID,
			"constraint_code": code,
		})
		return
	}

	// Recent outcomes — from constraint_outcomes if TSDB is wired; else empty.
	outcomes := []rulesOutcomeBucket{}
	lookbackHours := s.cfg.JiminyRulesOutcomesLookbackHours
	if lookbackHours <= 0 {
		lookbackHours = 168
	}
	if s.tsdbClient != nil {
		buckets, oErr := s.fetchRuleOutcomes(r.Context(), spaceID, code, time.Duration(lookbackHours)*time.Hour)
		if oErr != nil {
			// Outcome-lookup failure is non-fatal; the rule detail itself is
			// authoritative. Log server-side, return empty outcomes bucket.
			writeJSON(w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"rule":                    rule,
					"recent_outcomes":         outcomes,
					"outcomes_lookback_hours": lookbackHours,
					"outcomes_warning":        oErr.Error(),
				},
			})
			return
		}
		outcomes = buckets
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"rule":                    rule,
			"recent_outcomes":         outcomes,
			"outcomes_lookback_hours": lookbackHours,
		},
	})
}

// fetchRuleByCode returns the single MemoryNode with the given
// constraint_code in the given space. Returns (item, found, err).
func (s *Server) fetchRuleByCode(ctx context.Context, spaceID, code string) (rulesListItem, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	cypher := `MATCH (n:MemoryNode)
		WHERE n.space_id = $space_id
		  AND n.constraint_code = $code
		  AND n.role_type IN ['constraint','correction']
		RETURN n.node_id AS node_id,
		       coalesce(n.constraint_code, '') AS constraint_code,
		       n.role_type AS role_type,
		       coalesce(n.constraint_type, '') AS constraint_type,
		       coalesce(n.is_informational, false) AS is_informational,
		       coalesce(n.content, '') AS content,
		       coalesce(n.scope, '') AS scope,
		       coalesce(n.is_archived, false) AS is_archived,
		       coalesce(n.archive_reason, '') AS archive_reason,
		       n.created_at AS created_at
		LIMIT 1`

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"space_id": spaceID, "code": code})
		if err != nil {
			return nil, err
		}
		return res.Collect(ctx)
	})
	if err != nil {
		return rulesListItem{}, false, err
	}
	records := result.([]*neo4j.Record)
	if len(records) == 0 {
		return rulesListItem{}, false, nil
	}
	return recordToRulesItem(records[0]), true, nil
}

// fetchRuleOutcomes returns per-outcome-type counts for the given
// constraint_code over the window. Uses the shipped constraint_outcomes
// TSDB hypertable (RRF-SCALE-001).
func (s *Server) fetchRuleOutcomes(ctx context.Context, spaceID, code string, window time.Duration) ([]rulesOutcomeBucket, error) {
	if s.tsdbClient == nil || s.tsdbClient.Pool() == nil {
		return nil, errors.New("TSDB pool not available")
	}
	cutoff := time.Now().Add(-window)
	const query = `
		SELECT outcome_type, COUNT(*)::bigint AS n
		FROM constraint_outcomes
		WHERE space_id = $1 AND constraint_code = $2 AND time >= $3
		GROUP BY outcome_type
		ORDER BY n DESC`
	rows, err := s.tsdbClient.Pool().Query(ctx, query, spaceID, code, cutoff)
	if err != nil {
		return nil, fmt.Errorf("rules_outcomes query: %w", err)
	}
	defer rows.Close()
	out := []rulesOutcomeBucket{}
	for rows.Next() {
		var b rulesOutcomeBucket
		if err := rows.Scan(&b.OutcomeType, &b.Count); err != nil {
			return nil, fmt.Errorf("rules_outcomes scan: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rules_outcomes rows: %w", err)
	}
	return out, nil
}

// recordToRulesItem projects a Neo4j Record into a rulesListItem.
// The scan tolerates missing / typed-nil fields via coalesce in the
// Cypher above, so all fields land as concrete Go values.
func recordToRulesItem(rec *neo4j.Record) rulesListItem {
	item := rulesListItem{}
	if v, ok := rec.Get("node_id"); ok {
		item.NodeID, _ = v.(string)
	}
	if v, ok := rec.Get("constraint_code"); ok {
		item.ConstraintCode, _ = v.(string)
	}
	if v, ok := rec.Get("role_type"); ok {
		item.RoleType, _ = v.(string)
	}
	if v, ok := rec.Get("constraint_type"); ok {
		item.ConstraintType, _ = v.(string)
	}
	if v, ok := rec.Get("is_informational"); ok {
		item.IsInformational, _ = v.(bool)
	}
	if v, ok := rec.Get("content"); ok {
		item.Content, _ = v.(string)
	}
	if v, ok := rec.Get("scope"); ok {
		item.Scope, _ = v.(string)
	}
	if v, ok := rec.Get("is_archived"); ok {
		item.IsArchived, _ = v.(bool)
	}
	if v, ok := rec.Get("archive_reason"); ok {
		item.ArchiveReason, _ = v.(string)
	}
	if v, ok := rec.Get("created_at"); ok {
		switch t := v.(type) {
		case time.Time:
			item.CreatedAt = t
		case neo4j.LocalDateTime:
			item.CreatedAt = t.Time()
		case neo4j.LocalTime:
			// unlikely — created_at is a datetime; leave zero
		}
	}
	return item
}
