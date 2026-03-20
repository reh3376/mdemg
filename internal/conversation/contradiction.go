package conversation

import (
	"context"
	"log"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ContradictionConfig controls contradiction detection behavior.
type ContradictionConfig struct {
	Enabled       bool
	SimThreshold  float64
	MaxCandidates int

	// F2b: NLI-Enhanced Contradiction Detection
	NLIEnabled    bool   // Use NLI sidecar instead of heuristic negation detection
	NLISidecarURL string // URL of the sidecar (e.g. "http://localhost:8100")
	NLITimeoutMs  int    // Timeout for NLI call (default 1000)
}

// DefaultContradictionConfig returns reasonable defaults for contradiction detection.
func DefaultContradictionConfig() ContradictionConfig {
	return ContradictionConfig{
		Enabled:       true,
		SimThreshold:  0.75,
		MaxCandidates: 20,
	}
}

// ContradictionChecker detects contradictions between a new observation and
// existing observations in the same space using vector similarity + negation heuristics.
type ContradictionChecker struct {
	embedder Embedder
	driver   neo4j.DriverWithContext
}

// NewContradictionChecker creates a new ContradictionChecker.
func NewContradictionChecker(embedder Embedder, driver neo4j.DriverWithContext) *ContradictionChecker {
	return &ContradictionChecker{
		embedder: embedder,
		driver:   driver,
	}
}

// negationMarkers are phrases whose presence in one text but absence in a
// highly similar counterpart signals a potential contradiction.
var negationMarkers = []string{
	"not",
	"never",
	"don't",
	"instead of",
	"no longer",
	"stopped",
	"removed",
	"shouldn't",
	"must not",
	"forbidden",
}

// candidateNode holds a similar observation retrieved from the graph.
type candidateNode struct {
	NodeID     string
	Content    string
	Similarity float64
}

// CheckContradictions searches for existing observations that are semantically
// similar to obs yet contain negation signals suggesting a contradiction.
//
// It returns an aggregate contradiction score in the range [0.0, 1.0]
// representing the maximum contradiction strength found.
func (c *ContradictionChecker) CheckContradictions(ctx context.Context, obs Observation, cfg ContradictionConfig) (float64, error) {
	if !cfg.Enabled {
		return 0.0, nil
	}

	// No embedding means we cannot perform vector search.
	if len(obs.Embedding) == 0 {
		return 0.0, nil
	}

	if cfg.MaxCandidates <= 0 {
		cfg.MaxCandidates = 20
	}
	if cfg.SimThreshold <= 0 {
		cfg.SimThreshold = 0.75
	}

	// Retrieve similar nodes from Neo4j.
	candidates, err := c.findSimilarNodes(ctx, obs, cfg)
	if err != nil {
		return 0.0, err
	}

	// Evaluate each candidate for contradiction using NLI (F2b) or heuristics (F2a).
	maxScore := 0.0
	for _, cand := range candidates {
		isContradiction := false

		if cfg.NLIEnabled && cfg.NLISidecarURL != "" {
			// F2b: Use NLI sidecar for more accurate contradiction detection.
			nliResult, nliErr := classifyNLI(ctx, cfg.NLISidecarURL, cand.Content, obs.Content, cfg.NLITimeoutMs)
			if nliErr != nil {
				log.Printf("[WARN] contradiction NLI call failed, falling back to heuristics: %v", nliErr)
				// Fallback to heuristic on error
				isContradiction = detectNegation(obs.Content, cand.Content)
			} else {
				isContradiction = nliResult.Label == "contradiction"
			}
		} else {
			// F2a: Heuristic negation detection.
			isContradiction = detectNegation(obs.Content, cand.Content)
		}

		if isContradiction {
			// Scale contradiction strength by similarity —
			// closer semantic similarity with opposing negation = stronger contradiction.
			score := cand.Similarity
			if score > maxScore {
				maxScore = score
			}

			// Persist a CONTRADICTS edge in the graph (best-effort).
			_ = c.upsertContradictionEdge(ctx, obs, cand)
		}
	}

	// Clamp to [0.0, 1.0].
	if maxScore > 1.0 {
		maxScore = 1.0
	}
	return maxScore, nil
}

// findSimilarNodes queries Neo4j for the top-N similar conversation observations
// in the same space that exceed the similarity threshold.
func (c *ContradictionChecker) findSimilarNodes(ctx context.Context, obs Observation, cfg ContradictionConfig) ([]candidateNode, error) {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND n.embedding IS NOT NULL
			  AND n.node_id <> $obsId
			WITH n, vector.similarity.cosine(n.embedding, $embedding) AS sim
			WHERE sim > $threshold
			ORDER BY sim DESC
			LIMIT $limit
			RETURN n.node_id AS nodeId, n.content AS content, sim
		`
		params := map[string]any{
			"spaceId":   obs.SpaceID,
			"obsId":     obs.ObsID,
			"embedding": obs.Embedding,
			"threshold": cfg.SimThreshold,
			"limit":     cfg.MaxCandidates,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var candidates []candidateNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			content, _ := rec.Get("content")
			sim, _ := rec.Get("sim")

			cn := candidateNode{}
			if v, ok := nodeID.(string); ok {
				cn.NodeID = v
			}
			if v, ok := content.(string); ok {
				cn.Content = v
			}
			if v, ok := sim.(float64); ok {
				cn.Similarity = v
			}
			candidates = append(candidates, cn)
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return candidates, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]candidateNode), nil
}

// upsertContradictionEdge creates or updates a CONTRADICTS relationship
// between the observation node and the contradicting candidate.
func (c *ContradictionChecker) upsertContradictionEdge(ctx context.Context, obs Observation, cand candidateNode) error {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (a:MemoryNode {node_id: $obsId, space_id: $spaceId})
			MATCH (b:MemoryNode {node_id: $candId, space_id: $spaceId})
			MERGE (a)-[r:CONTRADICTS]->(b)
			SET r.similarity = $similarity,
			    r.detected_at = datetime()
			RETURN r
		`
		params := map[string]any{
			"obsId":      obs.ObsID,
			"candId":     cand.NodeID,
			"spaceId":    obs.SpaceID,
			"similarity": cand.Similarity,
		}
		_, err := tx.Run(ctx, cypher, params)
		return nil, err
	})
	return err
}

// detectNegation returns true when one text contains negation markers that the
// other lacks, suggesting the two semantically similar texts may contradict.
func detectNegation(textA, textB string) bool {
	lowerA := strings.ToLower(textA)
	lowerB := strings.ToLower(textB)

	for _, marker := range negationMarkers {
		aHas := containsWord(lowerA, marker)
		bHas := containsWord(lowerB, marker)
		if aHas != bHas {
			return true
		}
	}
	return false
}

// containsWord checks whether text contains the given marker as a
// substring (case-insensitive; caller must pass lowercase text).
// Multi-word markers (e.g. "instead of") are matched as plain substrings.
// Single-word markers are matched with word-boundary awareness to avoid
// false positives (e.g. "note" should not match "not").
func containsWord(lowerText, marker string) bool {
	if strings.Contains(marker, " ") {
		// Multi-word marker: plain substring match is sufficient.
		return strings.Contains(lowerText, marker)
	}

	// Single-word marker: ensure it appears as a whole word.
	idx := 0
	for {
		pos := strings.Index(lowerText[idx:], marker)
		if pos < 0 {
			return false
		}
		absPos := idx + pos
		endPos := absPos + len(marker)

		// Check left boundary.
		leftOK := absPos == 0 || !isWordChar(rune(lowerText[absPos-1]))
		// Check right boundary.
		rightOK := endPos >= len(lowerText) || !isWordChar(rune(lowerText[endPos]))

		if leftOK && rightOK {
			return true
		}
		idx = absPos + 1
		if idx >= len(lowerText) {
			return false
		}
	}
}

// isWordChar returns true for characters that are part of a word (letters, digits, apostrophe).
func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '\''
}
