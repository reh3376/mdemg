package retrieval

import (
	"context"
	"fmt"
	"log/slog"
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
					slog.Debug("bm25: found trace term", "term", debugTraceTerm, "rank", i+1, "node_id", r.NodeID, "name", r.Name, "path", r.Path, "score", r.Score)
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

// luceneSpecials contains characters that must be escaped in Lucene queries
var luceneSpecials = map[byte]bool{
	'+': true, '-': true, '!': true, '(': true, ')': true,
	'{': true, '}': true, '[': true, ']': true, '^': true,
	'"': true, '~': true, '*': true, '?': true, ':': true,
	'\\': true, '/': true,
}

// escapeLuceneQuery escapes special characters for Lucene query syntax
func escapeLuceneQuery(query string) string {
	var b strings.Builder
	b.Grow(len(query) * 2)
	for i := 0; i < len(query); i++ {
		c := query[i]
		if i+1 < len(query) {
			if c == '&' && query[i+1] == '&' {
				b.WriteString(`\&\&`)
				i++
				continue
			}
			if c == '|' && query[i+1] == '|' {
				b.WriteString(`\|\|`)
				i++
				continue
			}
		}
		if luceneSpecials[c] {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
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

// ReciprocalRankFusion combines results from vector and BM25 search using RRF.
// RRF score = Σ 1/(k + rank_i) for each retriever i
// rrfK is the RRF constant (typically 60) that balances contribution from different rank positions.
//
// This method doesn't require score normalization and naturally handles:
// - Different score scales between retrievers
// - Documents appearing in only one retriever's results
func ReciprocalRankFusion(vectorResults []Candidate, bm25Results []BM25Result, vectorWeight, bm25Weight float64, rrfK int) []FusedCandidate {
	// Debug: log input counts
	if debugTraceEnabled {
		slog.Debug("rrf: input counts", "vector_results", len(vectorResults), "bm25_results", len(bm25Results))
	}
	// Build maps for quick lookup and score accumulation
	candidateMap := make(map[string]*FusedCandidate)

	// Process vector results
	for rank, c := range vectorResults {
		rrfContrib := vectorWeight / float64(rrfK+rank+1)

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
		rrfContrib := bm25Weight / float64(rrfK+rank+1)

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
				slog.Debug("rrf: fused trace term", "term", debugTraceTerm, "rank", i+1, "node_id", f.NodeID, "name", f.Name, "rrf_score", f.RRFScore, "vector_rank", f.VectorRank, "bm25_rank", f.BM25Rank)
			}
		}
		slog.Debug("rrf: output count", "total_fused", len(fused))
	}

	return fused
}

// ConvertFusedToCandidates converts fused results back to Candidate format
// for compatibility with existing activation and scoring pipeline.
//
// No score fabrication: VectorSim is the real vector similarity (0.0 for BM25-only),
// and BM25Score is the real BM25 score (0.0 for vector-only). The scoring formula
// handles both signals separately via ScoringBM25Weight.
func ConvertFusedToCandidates(fused []FusedCandidate) []Candidate {
	if len(fused) == 0 {
		return []Candidate{}
	}

	if debugTraceEnabled {
		slog.Debug("convert_fused: converting candidates", "count", len(fused))
	}

	cands := make([]Candidate, len(fused))
	for i, f := range fused {
		cands[i] = Candidate{
			NodeID:     f.NodeID,
			Path:       f.Path,
			Name:       f.Name,
			Summary:    f.Summary,
			UpdatedAt:  f.UpdatedAt,
			Confidence: f.Confidence,
			VectorSim:  f.VectorSim,  // Real value, 0.0 for BM25-only
			BM25Score:  f.BM25Score,   // Real value, 0.0 for vector-only
			Layer:      f.Layer,
			Tags:       f.Tags,
		}

		// Debug: trace target term through converted candidates
		if debugTraceEnabled {
			if strings.Contains(strings.ToLower(f.Name), debugTraceTerm) ||
				strings.Contains(strings.ToLower(f.Path), debugTraceTerm) {
				slog.Debug("convert_fused: trace term", "term", debugTraceTerm, "position", i, "node_id", f.NodeID, "name", f.Name, "vector_sim", f.VectorSim, "bm25_score", f.BM25Score)
			}
		}
	}
	return cands
}
