package retrieval

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// StructuralColumn is the same-file/package/symbol-tree column. It seeds from
// a small vector-recall set, then walks structural edges (containment +
// definition) up to a configurable hop depth, returning nodes that share a
// structural neighborhood with the query's nearest matches.
//
// Phase 13 — Note 04 Column-Voting Retrieval, NEW column. Distinct from
// GraphColumn: GraphColumn uses *learning* edges (CO_ACTIVATED_WITH); this
// column uses *structural* edges (filename containment, AST parent/child,
// CODE_REL contains/defined_in). When the user asks about a function,
// structural-column results include the surrounding file and sibling
// functions — context that Embedding/BM25 may miss when the query phrasing
// doesn't mention them.
//
// Hop depth defaults to 2 (config: RETRIEVAL_STRUCTURAL_HOPS, plan §13).
// Score = exponential decay on hop distance (1 hop → 1.0, 2 → 0.5, 3 → 0.25).
type StructuralColumn struct {
	svc *Service
	// SeedK caps the seed set used to bootstrap the structural walk.
	SeedK int
	// HopDepth is the maximum structural-edge distance walked from each
	// seed. Default 2 (matches RETRIEVAL_STRUCTURAL_HOPS=2).
	HopDepth int
}

// NewStructuralColumn constructs a StructuralColumn with sensible defaults.
func NewStructuralColumn(svc *Service) *StructuralColumn {
	return &StructuralColumn{svc: svc, SeedK: 25, HopDepth: 2}
}

// Name implements [Column].
func (c *StructuralColumn) Name() string { return "structural" }

// Run implements [Column].
func (c *StructuralColumn) Run(ctx context.Context, q ColumnQuery) ColumnResult {
	start := time.Now()
	res := ColumnResult{Column: c.Name()}

	if c.svc == nil {
		res.Err = errors.New("structural column: nil Service")
		res.Latency = time.Since(start)
		return res
	}
	if len(q.QueryEmbedding) == 0 {
		res.Latency = time.Since(start)
		return res
	}

	seedK := c.SeedK
	if seedK <= 0 {
		seedK = 25
	}
	hopDepth := c.HopDepth
	if hopDepth <= 0 {
		hopDepth = 2
	}

	// Seed via a small vector recall — the structural walk only needs the
	// "closest few" nodes as starting points; the column's signal comes
	// from what those points contain/define rather than vector similarity.
	seeds, err := c.svc.vectorRecall(ctx, q.SpaceIDs, q.QueryEmbedding, seedK, q.Filter)
	if err != nil {
		res.Err = err
		res.Latency = time.Since(start)
		return res
	}
	if len(seeds) == 0 {
		res.Latency = time.Since(start)
		return res
	}

	seedIDs := make([]string, 0, len(seeds))
	for _, s := range seeds {
		seedIDs = append(seedIDs, s.NodeID)
	}

	// Walk structural edges. Cypher: variable-length traversal across edge
	// types that encode structure ('contains', 'defined_in') with hop-level
	// scoring. Edges of these types are present on code-symbol MemoryNodes;
	// for spaces without code symbols this column simply returns the seed
	// set with hop-distance=0 weighting.
	results, err := c.walk(ctx, q.SpaceIDs, seedIDs, hopDepth, q.TopN)
	res.Latency = time.Since(start)
	if err != nil {
		res.Err = err
		return res
	}

	// Merge seed metadata into the result candidates so output mirrors
	// what other columns produce. Activated-only nodes (no seed metadata)
	// are surfaced with NodeID + score; the aggregator/consumer fills in
	// path/name/etc. via downstream lookups.
	seedMap := make(map[string]Candidate, len(seeds))
	for _, s := range seeds {
		seedMap[s.NodeID] = s
	}

	cands := make([]Candidate, 0, len(results))
	for _, r := range results {
		c, ok := seedMap[r.NodeID]
		if !ok {
			c = Candidate{NodeID: r.NodeID}
		}
		c.VectorSim = r.Score // structural-distance score in VectorSim slot
		cands = append(cands, c)
	}
	res.Candidates = cands
	return res
}

// structuralResult is a single node + its hop-decay score from the walk.
type structuralResult struct {
	NodeID string
	Score  float64
	Hops   int
}

// walk performs the variable-length Cypher traversal across structural
// edges. Returns nodes ranked by exponential hop decay (closer = higher).
func (c *StructuralColumn) walk(ctx context.Context, spaceIDs []string, seedIDs []string, maxHops int, topN int) ([]structuralResult, error) {
	sess := c.svc.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Variable-length traversal across structural relationship types. The
	// undirected pattern catches both contains (parent→child) and
	// defined_in (child→parent) without enumerating both directions.
	cypher := `
MATCH (seed:MemoryNode)
WHERE seed.node_id IN $seedIds AND seed.space_id IN $spaceIds
MATCH path = (seed)-[:contains|defined_in|CONTAINS|DEFINED_IN*1..` + intToStringForCypher(maxHops) + `]-(n:MemoryNode)
WHERE n.space_id IN $spaceIds
  AND NOT coalesce(n.is_archived, false)
WITH n.node_id AS node_id, min(length(path)) AS hops
ORDER BY hops ASC
LIMIT $topN
RETURN node_id, hops
`

	params := map[string]any{
		"seedIds":  seedIDs,
		"spaceIds": spaceIDs,
		"topN":     topN,
	}

	results, err := neo4j.ExecuteRead(ctx, sess, func(tx neo4j.ManagedTransaction) ([]structuralResult, error) {
		records, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		out := make([]structuralResult, 0, topN)
		for records.Next(ctx) {
			rec := records.Record()
			nodeID, _ := rec.Get("node_id")
			hopsAny, _ := rec.Get("hops")
			hops, _ := hopsAny.(int64)
			out = append(out, structuralResult{
				NodeID: nodeID.(string),
				Hops:   int(hops),
				Score:  hopDecayScore(int(hops)),
			})
		}
		return out, records.Err()
	})
	if err != nil {
		return nil, err
	}

	// Already ORDER BY hops ASC at Cypher level, but a stable sort here
	// guarantees deterministic ranking even when multiple nodes share the
	// same hop distance.
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Hops != results[j].Hops {
			return results[i].Hops < results[j].Hops
		}
		return results[i].NodeID < results[j].NodeID
	})
	return results, nil
}

// hopDecayScore returns the per-hop score: 1 hop → 1.0, 2 → 0.5, 3 → 0.25.
// Clamped to [0, 1].
func hopDecayScore(hops int) float64 {
	if hops <= 0 {
		return 1.0
	}
	score := 1.0 / float64(int(1)<<(hops-1)) // 1<<0=1, 1<<1=2, 1<<2=4 → 1.0, 0.5, 0.25
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// intToStringForCypher renders an int as a literal for embedding in a
// variable-length pattern (Cypher doesn't accept a parameterised upper
// bound for the * range). Caller must guarantee a sane range — Run() does
// (HopDepth defaults to 2 and is capped by Config validation).
func intToStringForCypher(n int) string {
	if n <= 0 {
		return "1"
	}
	// Tiny ints only; avoid pulling strconv to keep the file lean.
	if n < 10 {
		return string(rune('0' + n))
	}
	// Above 9 hops would be pathological — clamp.
	return "9"
}
