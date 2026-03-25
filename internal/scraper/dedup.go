package scraper

import (
	"context"
	"log/slog"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/embeddings"
)

// DedupChecker checks scraped content against existing MemoryNodes for similarity.
type DedupChecker struct {
	driver    neo4j.DriverWithContext
	embedder  embeddings.Embedder
	threshold float64
}

// NewDedupChecker creates a new dedup checker.
func NewDedupChecker(driver neo4j.DriverWithContext, embedder embeddings.Embedder, threshold float64) *DedupChecker {
	if threshold <= 0 {
		threshold = 0.85
	}
	return &DedupChecker{
		driver:    driver,
		embedder:  embedder,
		threshold: threshold,
	}
}

// CheckSimilar embeds content and queries the vector index for similar existing nodes.
// Returns node_ids of similar existing MemoryNodes (similarity >= threshold).
func (d *DedupChecker) CheckSimilar(ctx context.Context, spaceID, content string) ([]string, error) {
	if d.embedder == nil {
		return nil, nil
	}

	embedding, err := d.embedder.Embed(ctx, content)
	if err != nil {
		return nil, err
	}
	if len(embedding) == 0 {
		return nil, nil
	}

	session := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Query vector index for top-5 similar nodes in the target space
	result, err := session.Run(ctx,
		`CALL db.index.vector.queryNodes('memNodeEmbedding', 5, $embedding)
		 YIELD node, score
		 WHERE node.space_id = $space_id AND score >= $threshold
		 RETURN node.node_id AS node_id, score
		 ORDER BY score DESC`,
		map[string]any{
			"embedding": embedding,
			"space_id":  spaceID,
			"threshold": d.threshold,
		})
	if err != nil {
		slog.Warn("scraper dedup: vector query failed", "error", err)
		return nil, nil // Non-fatal
	}

	var similar []string
	for result.Next(ctx) {
		record := result.Record()
		if nid, ok := record.Get("node_id"); ok {
			if s, ok := nid.(string); ok {
				similar = append(similar, s)
			}
		}
	}
	return similar, nil
}
