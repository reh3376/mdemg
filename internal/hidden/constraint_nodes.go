package hidden

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mdemg/internal/sanitize"
	"regexp"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/nrednav/cuid2"
)

// ConstraintNodeResult tracks what happened during constraint node creation.
type ConstraintNodeResult struct {
	Created  int `json:"created"`
	Updated  int `json:"updated"`
	Linked   int `json:"linked"`
	Rejected int `json:"rejected"` // JIMINY-CORPUS-001: observations blocked by the promotion gate
}

// CreateConstraintNodes promotes constraint-tagged observations to first-class
// constraint nodes (role_type='constraint') and links them via IMPLEMENTS_CONSTRAINT edges.
// Called as part of RunConsolidation.
func (s *Service) CreateConstraintNodes(ctx context.Context, spaceID string) (*ConstraintNodeResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res := &ConstraintNodeResult{}

		// Step 1: Find constraint-tagged observations not yet linked to a constraint node
		findCypher := `
			MATCH (obs:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
			WHERE any(tag IN coalesce(obs.tags, []) WHERE tag STARTS WITH 'constraint:')
			  AND NOT (obs)-[:IMPLEMENTS_CONSTRAINT]->(:MemoryNode {role_type: 'constraint'})
			RETURN obs.node_id AS nodeId,
			       obs.name AS name,
			       obs.content AS content,
			       obs.obs_type AS obsType,
			       obs.tags AS tags,
			       obs.embedding AS embedding,
			       obs.structured_data AS structuredData,
			       obs.surprise_score AS surpriseScore,
			       obs.constraint_code AS constraintCode
		`
		findRes, err := tx.Run(ctx, findCypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, fmt.Errorf("find constraint observations: %w", err)
		}

		type constraintObs struct {
			nodeID              string
			name                string
			content             string
			obsType             string // observation provenance (JIMINY-CORPUS-001 gate input)
			constraintCode      string // J17 constraint code from observation
			tags                []string
			embedding           []float64
			cTypes              []string // extracted constraint types
			detectionConfidence float64  // max confidence from constraint detection
			surpriseScore       float64  // observation surprise score
		}

		var observations []constraintObs
		for findRes.Next(ctx) {
			rec := findRes.Record()
			nodeID, _ := rec.Get("nodeId")
			name, _ := rec.Get("name")
			content, _ := rec.Get("content")
			tagsRaw, _ := rec.Get("tags")
			embRaw, _ := rec.Get("embedding")

			obs := constraintObs{
				nodeID: fmt.Sprintf("%v", nodeID),
			}
			if name != nil {
				obs.name = fmt.Sprintf("%v", name)
			}
			if content != nil {
				obs.content = fmt.Sprintf("%v", content)
			}
			if obsTypeRaw, _ := rec.Get("obsType"); obsTypeRaw != nil {
				if ot, ok := obsTypeRaw.(string); ok {
					obs.obsType = ot
				}
			}

			// Extract tags
			if tagSlice, ok := tagsRaw.([]any); ok {
				for _, t := range tagSlice {
					tag := fmt.Sprintf("%v", t)
					obs.tags = append(obs.tags, tag)
					if strings.HasPrefix(tag, "constraint:") {
						obs.cTypes = append(obs.cTypes, strings.TrimPrefix(tag, "constraint:"))
					}
				}
			}

			// Extract embedding
			if embSlice, ok := embRaw.([]any); ok {
				for _, e := range embSlice {
					if f, ok := e.(float64); ok {
						obs.embedding = append(obs.embedding, f)
					}
				}
			}

			// Extract constraint code
			if ccRaw, _ := rec.Get("constraintCode"); ccRaw != nil {
				if cc, ok := ccRaw.(string); ok {
					obs.constraintCode = cc
				}
			}

			// Extract surprise score
			surpriseRaw, _ := rec.Get("surpriseScore")
			if s, ok := surpriseRaw.(float64); ok {
				obs.surpriseScore = s
			}

			// Extract detection confidence from structured_data
			structuredRaw, _ := rec.Get("structuredData")
			if sdStr, ok := structuredRaw.(string); ok && sdStr != "" {
				var sd map[string]any
				if json.Unmarshal([]byte(sdStr), &sd) == nil {
					if constraints, ok := sd["detected_constraints"].([]any); ok {
						for _, c := range constraints {
							if cm, ok := c.(map[string]any); ok {
								if conf, ok := cm["confidence"].(float64); ok && conf > obs.detectionConfidence {
									obs.detectionConfidence = conf
								}
							}
						}
					}
				}
			}

			observations = append(observations, obs)
		}
		if err := findRes.Err(); err != nil {
			return nil, fmt.Errorf("iterate constraint observations: %w", err)
		}

		if len(observations) == 0 {
			return res, nil
		}

		// Step 2: Group by constraint type, then create/update constraint nodes
		for _, obs := range observations {
			// JIMINY-CORPUS-001: promotion gate. Rejected observations are
			// skipped entirely (no create, no reinforcement of an existing
			// node, no IMPLEMENTS_CONSTRAINT link) so transient/status junk
			// never becomes — or strengthens — a constraint node. Per-node
			// at Debug (rejected observations are re-scanned every cycle);
			// one Info summary below keeps it observable without spam.
			if reason, rejected := s.constraintGate.Reject(obs.obsType, obs.content); rejected {
				res.Rejected++
				slog.Debug("constraint promotion gate: rejected observation",
					"obs_id", obs.nodeID, "obs_type", obs.obsType, "reason", reason)
				continue
			}
			for _, cType := range obs.cTypes {
				// Extract constraint name
				cName := obs.name
				if cName == "" {
					cName = extractConstraintLabel(obs.content)
				}
				if cName == "" {
					cName = fmt.Sprintf("%s constraint", cType)
				}

				// Check if matching constraint node exists
				matchCypher := `
					MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'constraint'})
					WHERE c.constraint_type = $cType
					  AND c.name = $name
					RETURN c.node_id AS nodeId
					LIMIT 1
				`
				matchRes, err := tx.Run(ctx, matchCypher, map[string]any{
					"spaceId": spaceID,
					"cType":   cType,
					"name":    cName,
				})
				if err != nil {
					return nil, fmt.Errorf("match constraint node: %w", err)
				}

				var constraintNodeID string
				if matchRes.Next(ctx) {
					nid, _ := matchRes.Record().Get("nodeId")
					constraintNodeID = fmt.Sprintf("%v", nid)
					// Update existing node timestamp
					updateCypher := `
						MATCH (c:MemoryNode {space_id: $spaceId, node_id: $nodeId})
						SET c.updated_at = datetime(),
						    c.reinforcement_count = coalesce(c.reinforcement_count, 0) + 1
					`
					if _, err := tx.Run(ctx, updateCypher, map[string]any{
						"spaceId": spaceID,
						"nodeId":  constraintNodeID,
					}); err != nil {
						return nil, fmt.Errorf("update constraint node: %w", err)
					}
					res.Updated++
				} else {
					// Create new constraint node
					constraintNodeID = cuid2.Generate()
					now := time.Now().UTC().Format(time.RFC3339)

					createCypher := `
						CREATE (c:MemoryNode:Constraint {
							space_id: $spaceId,
							node_id: $nodeId,
							role_type: 'constraint',
							name: $name,
							constraint_type: $cType,
							constraint_code: $constraintCode,
							content: $content,
							layer: 1,
							confidence: $confidence,
							tags: $tags,
							scope: $scope,
							authority_level: $authLevel,
							created_at: datetime($now),
							updated_at: datetime($now),
							volatile: false,
							is_archived: false
						})
					`

					// Use obs embedding if available
					embParam := []float64(nil)
					if len(obs.embedding) > 0 {
						embParam = obs.embedding
					}

					// Compute confidence from detection confidence + surprise signal
					// Formula: max(0.65, detection_confidence + surprise_score * 0.15), capped at 0.95
					promotionConfidence := obs.detectionConfidence + obs.surpriseScore*0.15
					if promotionConfidence < 0.65 {
						promotionConfidence = 0.65
					}
					if promotionConfidence > 0.95 {
						promotionConfidence = 0.95
					}

					// F7: Infer scope from constraint content
					scope := inferConstraintScope(obs.content)

					// F20: Authority level — use config default, fall back to "team_standard"
					authLevel := s.cfg.ConstraintDefaultAuthority
					if authLevel == "" {
						authLevel = "team_standard"
					}

					params := map[string]any{
						"spaceId":        spaceID,
						"nodeId":         constraintNodeID,
						"name":           cName,
						"cType":          cType,
						"constraintCode": obs.constraintCode,
						"content":        obs.content,
						"confidence":     promotionConfidence,
						"tags":           []string{"constraint", "constraint:" + cType},
						"now":            now,
						"scope":          scope,     // F7: file path scope pattern
						"authLevel":      authLevel, // F20: authority level from config
					}

					if len(embParam) > 0 {
						createCypher = `
							CREATE (c:MemoryNode:Constraint {
								space_id: $spaceId,
								node_id: $nodeId,
								role_type: 'constraint',
								name: $name,
								constraint_type: $cType,
								constraint_code: $constraintCode,
								content: $content,
								layer: 1,
								confidence: $confidence,
								tags: $tags,
								embedding: $embedding,
								scope: $scope,
								authority_level: $authLevel,
								created_at: datetime($now),
								updated_at: datetime($now),
								volatile: false,
								is_archived: false
							})
						`
						params["embedding"] = embParam
					}

					if _, err := tx.Run(ctx, createCypher, params); err != nil {
						return nil, fmt.Errorf("create constraint node: %w", err)
					}
					res.Created++
					slog.Info("Created constraint node", "node_id", constraintNodeID, "type", cType, "name", cName)
				}

				// Step 3: Link observation → constraint via IMPLEMENTS_CONSTRAINT
				linkCypher := `
					MATCH (obs:MemoryNode {space_id: $spaceId, node_id: $obsNodeId})
					MATCH (c:MemoryNode {space_id: $spaceId, node_id: $constraintNodeId})
					MERGE (obs)-[r:IMPLEMENTS_CONSTRAINT]->(c)
					ON CREATE SET r.created_at = datetime(), r.weight = 1.0
					ON MATCH SET r.weight = r.weight + 0.1, r.updated_at = datetime()
				`
				if _, err := tx.Run(ctx, linkCypher, map[string]any{
					"spaceId":          spaceID,
					"obsNodeId":        obs.nodeID,
					"constraintNodeId": constraintNodeID,
				}); err != nil {
					return nil, fmt.Errorf("link constraint edge: %w", err)
				}
				res.Linked++
			}
		}

		if res.Rejected > 0 {
			slog.Info("constraint promotion gate: rejected non-constraint observations",
				"space_id", spaceID, "rejected", res.Rejected, "promoted", res.Created+res.Updated)
		}

		return res, nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*ConstraintNodeResult), nil
}

// inferConstraintScope extracts a file path pattern from constraint text.
// Returns empty string if no pattern is found (null scope = applies everywhere).
// Patterns tried in order:
//
//	"in internal/api/**"          → internal/api/**
//	"for *.go files"              → *.go
//	"under internal/api/"         → internal/api/
//	explicit file paths like foo/bar.go
var constraintScopePatterns = []struct {
	re      *regexp.Regexp
	capture int
}{
	{regexp.MustCompile(`(?i)in\s+([\w/.-]+/\*\*)`), 1},
	{regexp.MustCompile(`(?i)for\s+([\w*]+\.[\w]+)\s+files`), 1},
	{regexp.MustCompile(`(?i)(?:under|within)\s+([\w/.-]+/)`), 1},
	{regexp.MustCompile(`([\w/.-]+\.(?:go|ts|py|js|rs|java))`), 1},
}

func inferConstraintScope(text string) string {
	for _, p := range constraintScopePatterns {
		m := p.re.FindStringSubmatch(text)
		if len(m) > p.capture && m[p.capture] != "" {
			return m[p.capture]
		}
	}
	return ""
}

// extractConstraintLabel gets a short label from content (first sentence, max 120 chars).
func extractConstraintLabel(content string) string {
	if content == "" {
		return ""
	}
	name := content
	if idx := strings.IndexAny(name, ".\n"); idx > 0 {
		name = name[:idx]
	}
	name = sanitize.CutRuneSafe(name, 120)
	return strings.TrimSpace(name)
}

// ApplyConstraintDecay reduces confidence for constraints that haven't been surfaced recently.
// Called during consolidation or on a schedule. Returns the number of constraints decayed.
// org_policy constraints are excluded from decay (they are permanent policy).
// A decayRate <= 0 defaults to 0.01 (1% per call).
func ApplyConstraintDecay(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, decayRate float64) (int, error) {
	if decayRate <= 0 {
		decayRate = 0.01
	}

	sess := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Decay constraints not surfaced in the last 7 days.
		// Excludes org_policy authority-level constraints (permanent) and archived nodes.
		cypher := `
		MATCH (n:MemoryNode {space_id: $spaceID})
		WHERE n.constraint_type IS NOT NULL
		  AND coalesce(n.is_archived, false) = false
		  AND coalesce(n.status, 'active') <> 'archived'
		  AND (n.last_surfaced_at IS NULL OR
		       datetime(n.last_surfaced_at) < datetime() - duration({days: 7}))
		  AND coalesce(n.authority_level, 'team_standard') <> 'org_policy'
		SET n.confidence = CASE
		      WHEN coalesce(n.confidence, 0.5) - $decayRate < 0.0 THEN 0.0
		      ELSE coalesce(n.confidence, 0.5) - $decayRate
		    END
		RETURN count(n) AS decayed_count`

		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceID":   spaceID,
			"decayRate": decayRate,
		})
		if err != nil {
			return 0, fmt.Errorf("constraint decay query: %w", err)
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("decayed_count"); ok {
				if n, ok := v.(int64); ok {
					return int(n), nil
				}
			}
		}
		if err := res.Err(); err != nil {
			return 0, fmt.Errorf("constraint decay iterate: %w", err)
		}
		return 0, nil
	})
	if err != nil {
		return 0, err
	}
	decayed := result.(int)
	if decayed > 0 {
		slog.Info("F13: Constraint decay applied", "decayed", decayed, "space_id", spaceID, "rate", decayRate)
	}
	return decayed, nil
}
