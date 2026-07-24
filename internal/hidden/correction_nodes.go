package hidden

// JIMINY-CORRECTION-PRODUCER-001 Epic 1: L0 obs → L1 correction-role producer.
//
// CreateCorrectionNodes mirrors CreateConstraintNodes (JIMINY-CORPUS-001) but
// for the correction side of the actionable-guidance ontology. It closes the
// last gap disclosed by JIMINY-ROLETYPE-ADAPTER-001: the retrieval + classifier
// know how to carry role_type='correction' end-to-end, but no producer ever
// mints those L1 nodes.
//
// Semantics: an L0 obs_type='correction' observation IS a durable rule
// ("prefer X over Y", "this recurring bug requires Z"). Unlike constraints
// which may have multiple constraint_type tags per obs (must, must_not,
// deadline, …), a correction is a single durable lesson — the promotion is
// 1:1, keyed by content. Idempotency guard is the IMPLEMENTS_CORRECTION edge:
// once linked, a re-scan of the same L0 obs never mints a duplicate L1 node.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mdemg/internal/sanitize"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/nrednav/cuid2"
)

// CorrectionNodeResult tracks what happened during correction node creation.
type CorrectionNodeResult struct {
	Created  int `json:"created"`
	Updated  int `json:"updated"`
	Linked   int `json:"linked"`
	Rejected int `json:"rejected"` // JIMINY-CORRECTION-PRODUCER-001: observations blocked by the promotion gate
}

// CreateCorrectionNodes promotes L0 obs_type='correction' observations to
// first-class correction nodes (role_type='correction') and links them via
// IMPLEMENTS_CORRECTION edges. Called as part of RunConsolidation.
func (s *Service) CreateCorrectionNodes(ctx context.Context, spaceID string) (*CorrectionNodeResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res := &CorrectionNodeResult{}

		// Step 1: Find correction obs not yet linked to a correction node.
		findCypher := `
			MATCH (obs:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
			WHERE obs.obs_type = 'correction'
			  AND NOT coalesce(obs.is_archived, false)
			  AND NOT (obs)-[:IMPLEMENTS_CORRECTION]->(:MemoryNode {role_type: 'correction'})
			RETURN obs.node_id AS nodeId,
			       obs.name AS name,
			       obs.content AS content,
			       obs.embedding AS embedding,
			       obs.tags AS tags,
			       obs.surprise_score AS surpriseScore,
			       obs.structured_data AS structuredData
		`
		findRes, err := tx.Run(ctx, findCypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, fmt.Errorf("find correction observations: %w", err)
		}

		type correctionObs struct {
			nodeID        string
			name          string
			content       string
			tags          []string
			embedding     []float64
			surpriseScore float64
			// JIMINY-STRUCTURED-CORRECTION-001: parsed from structured_data.correction
			corrIncorrect string
			corrCorrect   string
			corrContext   string
		}

		var observations []correctionObs
		for findRes.Next(ctx) {
			rec := findRes.Record()
			nodeID, _ := rec.Get("nodeId")
			name, _ := rec.Get("name")
			content, _ := rec.Get("content")
			tagsRaw, _ := rec.Get("tags")
			embRaw, _ := rec.Get("embedding")

			obs := correctionObs{nodeID: fmt.Sprintf("%v", nodeID)}
			if name != nil {
				obs.name = fmt.Sprintf("%v", name)
			}
			if content != nil {
				obs.content = fmt.Sprintf("%v", content)
			}
			if tagSlice, ok := tagsRaw.([]any); ok {
				for _, t := range tagSlice {
					obs.tags = append(obs.tags, fmt.Sprintf("%v", t))
				}
			}
			if embSlice, ok := embRaw.([]any); ok {
				for _, e := range embSlice {
					if f, ok := e.(float64); ok {
						obs.embedding = append(obs.embedding, f)
					}
				}
			}
			surpriseRaw, _ := rec.Get("surpriseScore")
			if sv, ok := surpriseRaw.(float64); ok {
				obs.surpriseScore = sv
			}

			// JIMINY-STRUCTURED-CORRECTION-001: parse structured_data.correction
			// (present on obs authored via conversation.Correct after v0.11.4;
			// absent on older obs — those L1s get empty structured fields and
			// can be repaired via `mdemg corrections rehydrate-structured`).
			if sdRaw, _ := rec.Get("structuredData"); sdRaw != nil {
				if sdStr, ok := sdRaw.(string); ok && sdStr != "" {
					var sd map[string]any
					if json.Unmarshal([]byte(sdStr), &sd) == nil {
						if corr, ok := sd["correction"].(map[string]any); ok {
							if v, _ := corr["incorrect"].(string); v != "" {
								obs.corrIncorrect = v
							}
							if v, _ := corr["correct"].(string); v != "" {
								obs.corrCorrect = v
							}
							if v, _ := corr["context"].(string); v != "" {
								obs.corrContext = v
							}
						}
					}
				}
			}

			observations = append(observations, obs)
		}
		if err := findRes.Err(); err != nil {
			return nil, fmt.Errorf("iterate correction observations: %w", err)
		}

		if len(observations) == 0 {
			return res, nil
		}

		// Step 2: Promote each observation to a correction node.
		for _, obs := range observations {
			// Gate: content-shape rejection (Epic 2). Skip entirely — no
			// create, no reinforce, no link — so pathological content never
			// becomes an L1 correction node.
			if reason, rejected := s.correctionGate.Reject(obs.content); rejected {
				res.Rejected++
				slog.Debug("correction promotion gate: rejected observation",
					"obs_id", obs.nodeID, "reason", reason)
				continue
			}

			// Correction identity: content-derived label. Corrections are 1:1
			// with their obs (no type-grouping like constraints), so the label
			// is the shortened content unless the obs carries an explicit name.
			cName := obs.name
			if cName == "" {
				cName = extractCorrectionLabel(obs.content)
			}
			if cName == "" {
				cName = "correction"
			}

			// Idempotency: match by (space_id, role_type='correction', name).
			// A rerun of the same obs would already be blocked by the
			// IMPLEMENTS_CORRECTION guard in the find query, but the identity
			// query catches a case where an operator re-authored the same
			// correction via /v1/conversation/correct as a fresh L0 obs — we
			// reinforce the existing L1 rather than mint a duplicate.
			matchCypher := `
				MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'correction'})
				WHERE c.name = $name
				RETURN c.node_id AS nodeId
				LIMIT 1
			`
			matchRes, err := tx.Run(ctx, matchCypher, map[string]any{
				"spaceId": spaceID,
				"name":    cName,
			})
			if err != nil {
				return nil, fmt.Errorf("match correction node: %w", err)
			}

			var correctionNodeID string
			if matchRes.Next(ctx) {
				nid, _ := matchRes.Record().Get("nodeId")
				correctionNodeID = fmt.Sprintf("%v", nid)
				updateCypher := `
					MATCH (c:MemoryNode {space_id: $spaceId, node_id: $nodeId})
					SET c.updated_at = datetime(),
					    c.reinforcement_count = coalesce(c.reinforcement_count, 0) + 1
				`
				if _, err := tx.Run(ctx, updateCypher, map[string]any{
					"spaceId": spaceID,
					"nodeId":  correctionNodeID,
				}); err != nil {
					return nil, fmt.Errorf("update correction node: %w", err)
				}
				res.Updated++
			} else {
				correctionNodeID = cuid2.Generate()
				now := time.Now().UTC().Format(time.RFC3339)

				// Confidence formula mirrors constraint promotion:
				// max(0.65, surprise * 0.15 + 0.65), capped at 0.95.
				promotionConfidence := 0.65 + obs.surpriseScore*0.15
				if promotionConfidence < 0.65 {
					promotionConfidence = 0.65
				}
				if promotionConfidence > 0.95 {
					promotionConfidence = 0.95
				}

				createCypher := `
					CREATE (c:MemoryNode:Correction {
						space_id: $spaceId,
						node_id: $nodeId,
						role_type: 'correction',
						name: $name,
						content: $content,
						layer: 1,
						confidence: $confidence,
						tags: $tags,
						correction_incorrect: $corrIncorrect,
						correction_correct:   $corrCorrect,
						correction_context:   $corrContext,
						created_at: datetime($now),
						updated_at: datetime($now),
						volatile: false,
						is_archived: false
					})
				`
				params := map[string]any{
					"spaceId":       spaceID,
					"nodeId":        correctionNodeID,
					"name":          cName,
					"content":       obs.content,
					"confidence":    promotionConfidence,
					"tags":          []string{"correction"},
					"now":           now,
					"corrIncorrect": obs.corrIncorrect,
					"corrCorrect":   obs.corrCorrect,
					"corrContext":   obs.corrContext,
				}

				if len(obs.embedding) > 0 {
					createCypher = `
						CREATE (c:MemoryNode:Correction {
							space_id: $spaceId,
							node_id: $nodeId,
							role_type: 'correction',
							name: $name,
							content: $content,
							layer: 1,
							confidence: $confidence,
							tags: $tags,
							embedding: $embedding,
							correction_incorrect: $corrIncorrect,
							correction_correct:   $corrCorrect,
							correction_context:   $corrContext,
							created_at: datetime($now),
							updated_at: datetime($now),
							volatile: false,
							is_archived: false
						})
					`
					params["embedding"] = obs.embedding
				}

				if _, err := tx.Run(ctx, createCypher, params); err != nil {
					return nil, fmt.Errorf("create correction node: %w", err)
				}
				res.Created++
				slog.Info("Created correction node", "node_id", correctionNodeID, "name", cName)
			}

			// Step 3: link L0 obs → L1 correction via IMPLEMENTS_CORRECTION.
			linkCypher := `
				MATCH (obs:MemoryNode {space_id: $spaceId, node_id: $obsNodeId})
				MATCH (c:MemoryNode {space_id: $spaceId, node_id: $correctionNodeId})
				MERGE (obs)-[r:IMPLEMENTS_CORRECTION]->(c)
				ON CREATE SET r.created_at = datetime(), r.weight = 1.0
				ON MATCH SET r.weight = r.weight + 0.1, r.updated_at = datetime()
			`
			if _, err := tx.Run(ctx, linkCypher, map[string]any{
				"spaceId":          spaceID,
				"obsNodeId":        obs.nodeID,
				"correctionNodeId": correctionNodeID,
			}); err != nil {
				return nil, fmt.Errorf("link correction edge: %w", err)
			}
			res.Linked++
		}

		if res.Rejected > 0 {
			slog.Info("correction promotion gate: rejected observations",
				"space_id", spaceID, "rejected", res.Rejected, "promoted", res.Created+res.Updated)
		}

		return res, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*CorrectionNodeResult), nil
}

// extractCorrectionLabel gets a short label from correction content
// (first sentence, max 120 chars). Mirrors extractConstraintLabel in
// constraint_nodes.go — kept separate so the two paths can diverge without
// touching each other.
func extractCorrectionLabel(content string) string {
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
