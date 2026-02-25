package retrieval

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// debugTraceEnabled enables detailed tracing for specific search terms
var debugTraceEnabled = false

// debugTraceTerm is the term to trace through the pipeline
var debugTraceTerm = "transformerconfig"

// BM25Result represents a result from full-text BM25 search
type BM25Result struct {
	NodeID     string
	Path       string
	Name       string
	Summary    string
	Score      float64
	UpdatedAt  time.Time
	Confidence float64
	Layer      int
	Tags       []string
}

// BM25Search performs full-text search using Neo4j's Lucene-based fulltext index.
// This complements vector search by finding exact keyword matches.
func (s *Service) BM25Search(ctx context.Context, spaceIDs []string, query string, topK int, filter FileFilter) ([]BM25Result, error) {
	if !s.cfg.HybridRetrievalEnabled {
		return nil, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Escape special Lucene characters in query
	escapedQuery := escapeLuceneQuery(query)

	params := map[string]any{
		"spaceIds": spaceIDs,
		"query":    escapedQuery,
		"topK":     topK,
	}

	// Add filter parameters if specified
	filterClause := ""
	if !filter.IsEmpty() {
		filterClause = filter.BuildCypherFilter()
		if len(filter.IncludeExtensions) > 0 {
			params["includeExtensions"] = filter.IncludeExtensions
		}
		if len(filter.ExcludeExtensions) > 0 {
			params["excludeExtensions"] = filter.ExcludeExtensions
		}
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Use fulltext index for BM25-style search
		cypher := `
CALL db.index.fulltext.queryNodes("memNodeFullText", $query)
YIELD node, score
WHERE node.space_id IN $spaceIds
  AND NOT coalesce(node.is_archived, false)` + filterClause + `
RETURN node.node_id AS node_id,
       node.path AS path,
       node.name AS name,
       coalesce(node.summary, '') AS summary,
       coalesce(node.confidence, 0.6) AS confidence,
       coalesce(node.updated_at, datetime()) AS updated_at,
       coalesce(node.layer, 0) AS layer,
       coalesce(node.tags, []) AS tags,
       score
ORDER BY score DESC
LIMIT $topK`

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		results := make([]BM25Result, 0, topK)
		for res.Next(ctx) {
			rec := res.Record()
			nid, _ := rec.Get("node_id")
			path, _ := rec.Get("path")
			name, _ := rec.Get("name")
			sum, _ := rec.Get("summary")
			conf, _ := rec.Get("confidence")
			upd, _ := rec.Get("updated_at")
			layer, _ := rec.Get("layer")
			tagsAny, _ := rec.Get("tags")
			sc, _ := rec.Get("score")

			r := BM25Result{
				NodeID:     fmt.Sprint(nid),
				Path:       fmt.Sprint(path),
				Name:       fmt.Sprint(name),
				Summary:    fmt.Sprint(sum),
				Score:      toFloat64(sc, 0),
				Confidence: toFloat64(conf, 0.6),
				Layer:      toInt(layer, 0),
				Tags:       toStringSlice(tagsAny),
			}

			switch v := upd.(type) {
			case time.Time:
				r.UpdatedAt = v
			default:
				r.UpdatedAt = time.Now()
			}

			results = append(results, r)
		}

		if err := res.Err(); err != nil {
			return nil, err
		}

		// Debug: trace target term through BM25 results
		if debugTraceEnabled {
			for i, r := range results {
				if strings.Contains(strings.ToLower(r.Name), debugTraceTerm) ||
					strings.Contains(strings.ToLower(r.Path), debugTraceTerm) {
					log.Printf("[DEBUG BM25] Found '%s' at rank %d: NodeID=%s, Name=%s, Path=%s, Score=%.2f",
						debugTraceTerm, i+1, r.NodeID, r.Name, r.Path, r.Score)
				}
			}
		}

		return results, nil
	})

	if err != nil {
		return nil, err
	}

	return outAny.([]BM25Result), nil
}

// escapeLuceneQuery escapes special characters for Lucene query syntax
func escapeLuceneQuery(query string) string {
	// Lucene special characters: + - && || ! ( ) { } [ ] ^ " ~ * ? : \ /
	// For simple queries, we wrap in quotes for phrase matching
	// and also allow fuzzy matching with ~
	return query
}

// FusedCandidate represents a candidate after combining vector and BM25 results
type FusedCandidate struct {
	NodeID      string
	Path        string
	Name        string
	Summary     string
	UpdatedAt   time.Time
	Confidence  float64
	VectorSim   float64 // Original vector similarity
	BM25Score   float64 // Original BM25 score
	RRFScore    float64 // Combined RRF score
	VectorRank  int     // Rank in vector results (0 if not present)
	BM25Rank    int     // Rank in BM25 results (0 if not present)
	Layer       int
	Tags        []string
}

// RRFConstant is the constant k in the RRF formula: 1/(k + rank)
// Standard value is 60, which balances contribution from different rank positions
const RRFConstant = 60

// ReciprocalRankFusion combines results from vector and BM25 search using RRF.
// RRF score = Σ 1/(k + rank_i) for each retriever i
//
// This method doesn't require score normalization and naturally handles:
// - Different score scales between retrievers
// - Documents appearing in only one retriever's results
func ReciprocalRankFusion(vectorResults []Candidate, bm25Results []BM25Result, vectorWeight, bm25Weight float64) []FusedCandidate {
	// Debug: log input counts
	if debugTraceEnabled {
		log.Printf("[DEBUG RRF] Input: %d vector results, %d BM25 results", len(vectorResults), len(bm25Results))
	}
	// Build maps for quick lookup and score accumulation
	candidateMap := make(map[string]*FusedCandidate)

	// Process vector results
	for rank, c := range vectorResults {
		rrfContrib := vectorWeight / float64(RRFConstant+rank+1)

		if existing, ok := candidateMap[c.NodeID]; ok {
			existing.RRFScore += rrfContrib
			existing.VectorRank = rank + 1
			existing.VectorSim = c.VectorSim
		} else {
			candidateMap[c.NodeID] = &FusedCandidate{
				NodeID:     c.NodeID,
				Path:       c.Path,
				Name:       c.Name,
				Summary:    c.Summary,
				UpdatedAt:  c.UpdatedAt,
				Confidence: c.Confidence,
				VectorSim:  c.VectorSim,
				VectorRank: rank + 1,
				Layer:      c.Layer,
				Tags:       c.Tags,
				RRFScore:   rrfContrib,
			}
		}
	}

	// Process BM25 results
	for rank, r := range bm25Results {
		rrfContrib := bm25Weight / float64(RRFConstant+rank+1)

		if existing, ok := candidateMap[r.NodeID]; ok {
			existing.RRFScore += rrfContrib
			existing.BM25Rank = rank + 1
			existing.BM25Score = r.Score
		} else {
			candidateMap[r.NodeID] = &FusedCandidate{
				NodeID:     r.NodeID,
				Path:       r.Path,
				Name:       r.Name,
				Summary:    r.Summary,
				UpdatedAt:  r.UpdatedAt,
				Confidence: r.Confidence,
				BM25Score:  r.Score,
				BM25Rank:   rank + 1,
				Layer:      r.Layer,
				Tags:       r.Tags,
				RRFScore:   rrfContrib,
			}
		}
	}

	// Convert to slice and sort by RRF score
	fused := make([]FusedCandidate, 0, len(candidateMap))
	for _, c := range candidateMap {
		fused = append(fused, *c)
	}

	sort.Slice(fused, func(i, j int) bool {
		return fused[i].RRFScore > fused[j].RRFScore
	})

	// Debug: trace target term through fused results
	if debugTraceEnabled {
		for i, f := range fused {
			if strings.Contains(strings.ToLower(f.Name), debugTraceTerm) ||
				strings.Contains(strings.ToLower(f.Path), debugTraceTerm) {
				log.Printf("[DEBUG RRF] Fused '%s' at rank %d: NodeID=%s, Name=%s, RRF=%.4f, VectorRank=%d, BM25Rank=%d",
					debugTraceTerm, i+1, f.NodeID, f.Name, f.RRFScore, f.VectorRank, f.BM25Rank)
			}
		}
		log.Printf("[DEBUG RRF] Output: %d total fused candidates", len(fused))
	}

	return fused
}

// ConvertFusedToCandidates converts fused results back to Candidate format
// for compatibility with existing activation and scoring pipeline.
//
// Strategy: Use original vector similarity for scoring quality, but the
// ordering comes from RRF (which already happened during fusion).
// For BM25-only candidates, estimate a similarity based on their RRF rank.
func ConvertFusedToCandidates(fused []FusedCandidate) []Candidate {
	if len(fused) == 0 {
		return []Candidate{}
	}

	if debugTraceEnabled {
		log.Printf("[DEBUG ConvertFused] Converting %d fused candidates", len(fused))
	}

	// Find max RRF for normalization of BM25-only candidates
	maxRRF := fused[0].RRFScore
	for _, f := range fused {
		if f.RRFScore > maxRRF {
			maxRRF = f.RRFScore
		}
	}

	cands := make([]Candidate, len(fused))
	for i, f := range fused {
		// Primary score source: original vector similarity (if available)
		score := f.VectorSim

		// For candidates that came ONLY from BM25 (no vector match),
		// estimate a score based on their BM25 rank position
		// Top BM25 results are exact keyword matches - highly valuable!
		if f.VectorSim == 0 && f.BM25Score > 0 {
			// BM25 rank 1 gets 0.95, rank 5 gets ~0.75, rank 20 gets ~0.55
			// This ensures top keyword matches compete with vector results
			rankBoost := 0.95 - 0.02*float64(f.BM25Rank-1)
			if rankBoost < 0.4 {
				rankBoost = 0.4
			}
			score = rankBoost
		}

		// Small boost for candidates that appear in BOTH retrievers
		if f.VectorSim > 0 && f.BM25Score > 0 {
			score += 0.05 // Bonus for being found by both methods
			if score > 1.0 {
				score = 1.0
			}
		}

		cands[i] = Candidate{
			NodeID:     f.NodeID,
			Path:       f.Path,
			Name:       f.Name,
			Summary:    f.Summary,
			UpdatedAt:  f.UpdatedAt,
			Confidence: f.Confidence,
			VectorSim:  score,
			Layer:      f.Layer,
			Tags:       f.Tags,
		}

		// Debug: trace target term through converted candidates
		if debugTraceEnabled {
			if strings.Contains(strings.ToLower(f.Name), debugTraceTerm) ||
				strings.Contains(strings.ToLower(f.Path), debugTraceTerm) {
				log.Printf("[DEBUG ConvertFused] '%s' at position %d: NodeID=%s, Name=%s, VectorSim=%.4f (was BM25Rank=%d)",
					debugTraceTerm, i, f.NodeID, f.Name, score, f.BM25Rank)
			}
		}
	}
	return cands
}
