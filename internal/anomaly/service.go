package anomaly

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/models"
)

// NewService creates a new anomaly detection service
func NewService(driver neo4j.DriverWithContext, cfg Config) *Service {
	return &Service{
		driver: driver,
		config: cfg,
	}
}

// Detect checks for anomalies during ingest. Non-blocking - errors are logged but don't fail ingest.
func (s *Service) Detect(ctx context.Context, dctx DetectionContext) []models.Anomaly {
	if !s.config.Enabled {
		return nil
	}

	// Apply timeout to prevent blocking ingest
	timeout := time.Duration(s.config.MaxCheckMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var anomalies []models.Anomaly

	// Run detection checks
	if dup := s.detectDuplicate(ctx, dctx); dup != nil {
		anomalies = append(anomalies, *dup)
	}

	if stale := s.detectStaleUpdate(ctx, dctx); stale != nil {
		anomalies = append(anomalies, *stale)
	}

	return anomalies
}

// detectDuplicate checks if a very similar node already exists
func (s *Service) detectDuplicate(ctx context.Context, dctx DetectionContext) *models.Anomaly {
	if len(dctx.Embedding) == 0 {
		return nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Vector similarity search for potential duplicates
	cypher := fmt.Sprintf(`
CALL db.index.vector.queryNodes($indexName, 5, $embedding)
YIELD node, score
WHERE node.space_id = $spaceId
  AND node.node_id <> $nodeId
  AND score >= $threshold
RETURN node.node_id AS node_id, node.name AS name, score
ORDER BY score DESC
LIMIT 1`)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"indexName": s.config.VectorIndexName,
			"embedding": dctx.Embedding,
			"spaceId":   dctx.SpaceID,
			"nodeId":    dctx.NodeID,
			"threshold": s.config.DuplicateThreshold,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			score, _ := rec.Get("score")
			return map[string]any{
				"node_id": nodeID,
				"name":    name,
				"score":   score,
			}, nil
		}
		return nil, res.Err()
	})

	if err != nil {
		// Context cancelled (timeout) or other error - log and continue
		if ctx.Err() == nil {
			slog.Warn("anomaly: duplicate check failed", "error", err)
		}
		return nil
	}

	if result == nil {
		return nil
	}

	data := result.(map[string]any)
	nodeID := fmt.Sprint(data["node_id"])
	name := fmt.Sprint(data["name"])
	score := toFloat64(data["score"])

	// Determine severity based on similarity score
	severity := "warning"
	if score >= 0.99 {
		severity = "critical"
	}

	return &models.Anomaly{
		Type:        models.AnomalyDuplicate,
		Severity:    severity,
		Message:     fmt.Sprintf("Very similar node exists: %q (similarity: %.2f)", name, score),
		RelatedNode: nodeID,
		Confidence:  score,
	}
}

// detectStaleUpdate checks if we're updating an old node
func (s *Service) detectStaleUpdate(ctx context.Context, dctx DetectionContext) *models.Anomaly {
	if !dctx.IsUpdate || dctx.NodeID == "" {
		return nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Check node's last update time
	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
WHERE n.updated_at IS NOT NULL
  AND n.updated_at < datetime() - duration({days: $staleDays})
RETURN n.updated_at AS updated_at,
       duration.between(n.updated_at, datetime()).days AS days_ago`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"spaceId":   dctx.SpaceID,
			"nodeId":    dctx.NodeID,
			"staleDays": s.config.StaleDays,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			daysAgo, _ := rec.Get("days_ago")
			return map[string]any{
				"days_ago": daysAgo,
			}, nil
		}
		return nil, res.Err()
	})

	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("anomaly: stale update check failed", "error", err)
		}
		return nil
	}

	if result == nil {
		return nil
	}

	data := result.(map[string]any)
	daysAgo := toInt64(data["days_ago"])

	// Calculate confidence based on how stale (higher = more stale = higher confidence of anomaly)
	confidence := float64(daysAgo) / float64(daysAgo+int64(s.config.StaleDays))
	if confidence > 1.0 {
		confidence = 1.0
	}

	severity := "info"
	if daysAgo > int64(s.config.StaleDays)*3 {
		severity = "warning"
	}

	return &models.Anomaly{
		Type:        models.AnomalyStaleUpdate,
		Severity:    severity,
		Message:     fmt.Sprintf("Updating node not modified in %d days", daysAgo),
		RelatedNode: dctx.NodeID,
		Confidence:  confidence,
	}
}

// toFloat64 converts various types to float64
func toFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int:
		return float64(x)
	default:
		return 0
	}
}

// toInt64 converts various types to int64
func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	default:
		return 0
	}
}
