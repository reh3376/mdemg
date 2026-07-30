package retrieval

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"mdemg/internal/models"

	neo4j "github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ConcreteCandidatesToResults converts concrete-recall Candidates into
// RetrieveResults for downstream injection by the quota promoter.
// Uses VectorSim as the initial Score (no BM25/graph column ran on these).
// The quota only cares about Layer + RoleType for gating; the score orders
// among promoted concretes so the highest-similarity concrete goes first.
func ConcreteCandidatesToResults(cands []Candidate) []models.RetrieveResult {
	if len(cands) == 0 {
		return nil
	}
	out := make([]models.RetrieveResult, 0, len(cands))
	for _, c := range cands {
		out = append(out, models.RetrieveResult{
			NodeID:    c.NodeID,
			Path:      c.Path,
			Name:      c.Name,
			Summary:   c.Summary,
			RoleType:  c.RoleType,
			ObsType:   c.ObsType,
			Layer:     c.Layer,
			Score:     c.VectorSim,
			VectorSim: c.VectorSim,
		})
	}
	return out
}

// fetchConcreteRecall runs a supplementary role/layer-filtered cosine query
// over the "concrete" partition (default: L0/L1 leaf/observation/constraint/
// correction nodes) so those candidates enter the RRF pool even when the
// primary Embedding column's global vector search is dominated by higher-layer
// emergent-concepts.
//
// RETRIEVAL-LAYER-BALANCE-001 (RQA-001 cluster C). Mirrors the shipped
// JIMINY-ACTIONABILITY-001 Lever C shape: the concrete partition is a small
// fraction of total nodes (~1% on production spaces) so the actionables /
// concretes almost never rank into the global top-N — a targeted cosine
// scan over ONLY the filtered partition is cheap and GUARANTEES the top-K
// concretes are found.
//
// ⚠️ RRF-SCALE-001-safe: gates on the vector-index cosine `sim` value ([0,1]
// stable across scorer changes), NEVER on any RRF Score. Returned candidates
// enter the pool as regular Candidates and compete normally on all columns.
//
// Returns nil on empty embedding, empty spaces, or disabled config; returns
// nil on any Neo4j error (fail-open — the primary retrieval path is
// authoritative).
// Caller is responsible for gating on RetrievalConcreteRecallEnabled (+ any
// per-request override) — this function itself only guards on data / driver
// preconditions, so a per-request `?concrete=true` can turn it on even when
// the config default is off.
func (s *Service) fetchConcreteRecall(ctx context.Context, spaceIDs []string, embedding []float32) []Candidate {
	if len(embedding) == 0 || len(spaceIDs) == 0 || s.driver == nil {
		return nil
	}
	topK := s.cfg.RetrievalConcreteRecallTopK
	if topK <= 0 {
		return nil
	}
	layerMax := s.cfg.RetrievalConcreteRecallLayerMax
	if layerMax < 0 {
		layerMax = 0
	}
	simFloor := s.cfg.RetrievalConcreteRecallSimFloor
	if simFloor < 0 {
		simFloor = 0
	}
	// Parse the role-types list. Empty = accept any role_type under layer_max.
	roleTypes := parseConcreteRoleTypes(s.cfg.RetrievalConcreteRecallRoleTypes)

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	slog.Debug("retrieval: concrete-recall query start",
		"spaceIds_len", len(spaceIDs),
		"embedding_len", len(embedding),
		"layerMax", layerMax,
		"simFloor", simFloor,
		"roleTypes_count", len(roleTypes),
		"topK", topK)

	// The role_type filter is conditional — a nil/empty list bypasses it so
	// operators can widen the concrete partition without editing code.
	roleFilter := ""
	if len(roleTypes) > 0 {
		roleFilter = "AND n.role_type IN $roleTypes"
	}
	// The size(n.embedding) = size($embedding) guard is REQUIRED — some
	// MemoryNodes have wrong-dim or empty-list embeddings that pass the
	// IS NOT NULL check but crash vector.similarity.cosine with
	// "Argument a is not a valid vector" (Neo4j is strict about dim match).
	// Live-caught during Tier-3 smoke; Lever C's smaller partition
	// (~hundreds of nodes) happened to have uniform embeddings so it
	// never surfaced the class.
	cypher := `
	MATCH (n:MemoryNode)
	WHERE n.space_id IN $spaceIds
	  AND n.layer <= $layerMax
	  AND NOT coalesce(n.is_archived, false)
	  AND n.embedding IS NOT NULL
	  AND size(n.embedding) = size($embedding)
	  ` + roleFilter + `
	WITH n, vector.similarity.cosine(n.embedding, $embedding) AS sim
	WHERE sim >= $simFloor
	RETURN n.node_id AS nodeId, coalesce(n.name, '') AS name,
	       coalesce(n.path, '') AS path,
	       coalesce(n.summary, '') AS summary,
	       coalesce(n.role_type, '') AS roleType,
	       coalesce(n.obs_type, '') AS obsType,
	       coalesce(n.layer, 0) AS layer,
	       coalesce(n.confidence, 0.5) AS confidence,
	       n.updated_at AS updatedAt,
	       sim
	ORDER BY sim DESC LIMIT $topK`

	params := map[string]any{
		"spaceIds":  spaceIDs,
		"embedding": embedding,
		"layerMax":  layerMax,
		"simFloor":  simFloor,
		"topK":      topK,
	}
	if len(roleTypes) > 0 {
		params["roleTypes"] = roleTypes
	}

	out, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, txErr := tx.Run(ctx, cypher, params)
		if txErr != nil {
			return nil, txErr
		}
		var cands []Candidate
		for res.Next(ctx) {
			rec := res.Record()
			getStr := func(k string) string {
				v, _ := rec.Get(k)
				if s, ok := v.(string); ok {
					return s
				}
				return ""
			}
			nodeID := getStr("nodeId")
			if nodeID == "" {
				continue
			}
			simVal, _ := rec.Get("sim")
			sim, _ := simVal.(float64)
			confVal, _ := rec.Get("confidence")
			conf, _ := confVal.(float64)
			layerVal, _ := rec.Get("layer")
			layer := 0
			switch v := layerVal.(type) {
			case int64:
				layer = int(v)
			case int:
				layer = v
			case float64:
				layer = int(v)
			}
			var updatedAt time.Time
			if u, ok := rec.Get("updatedAt"); ok && u != nil {
				if t, ok := u.(time.Time); ok {
					updatedAt = t
				}
			}
			cands = append(cands, Candidate{
				NodeID:     nodeID,
				Path:       getStr("path"),
				Name:       getStr("name"),
				Summary:    getStr("summary"),
				RoleType:   getStr("roleType"),
				ObsType:    getStr("obsType"),
				Layer:      layer,
				Confidence: conf,
				VectorSim:  sim,
				UpdatedAt:  updatedAt,
			})
		}
		return cands, res.Err()
	})
	if err != nil {
		slog.Warn("retrieval: concrete-recall query failed (fail-open)", "error", err.Error())
		return nil
	}
	cands, _ := out.([]Candidate)
	return cands
}

// mergeConcreteCandidates appends concrete candidates to the primary pool,
// deduplicating by NodeID (the primary pool wins — its scores already reflect
// vector + BM25 signal, so we don't want to overwrite them with the concrete-
// recall VectorSim-only values). Returns the merged pool + the count of
// concrete candidates that were NEW to the pool.
func mergeConcreteCandidates(primary, concrete []Candidate) ([]Candidate, int) {
	if len(concrete) == 0 {
		return primary, 0
	}
	seen := make(map[string]struct{}, len(primary))
	for _, c := range primary {
		seen[c.NodeID] = struct{}{}
	}
	merged := primary
	added := 0
	for _, c := range concrete {
		if _, dup := seen[c.NodeID]; dup {
			continue
		}
		merged = append(merged, c)
		seen[c.NodeID] = struct{}{}
		added++
	}
	return merged, added
}

// parseConcreteRoleTypes splits the comma-separated env value into a
// deduplicated non-empty slice of role_type strings. Empty string returns nil
// (meaning "accept any role_type" in the query).
func parseConcreteRoleTypes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
