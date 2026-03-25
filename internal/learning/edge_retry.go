package learning

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/optimistic"
)

// GetRetryConfig builds an optimistic.RetryConfig from the service's config.
func (s *Service) GetRetryConfig() optimistic.RetryConfig {
	if !s.cfg.OptimisticRetryEnabled {
		// Return config with 0 retries if disabled
		return optimistic.RetryConfig{
			MaxRetries:   0,
			BaseDelay:    time.Duration(s.cfg.OptimisticRetryBaseDelayMs) * time.Millisecond,
			MaxDelay:     time.Duration(s.cfg.OptimisticRetryMaxDelayMs) * time.Millisecond,
			Multiplier:   s.cfg.OptimisticRetryMultiplier,
			JitterFactor: 0.2,
		}
	}

	return optimistic.RetryConfig{
		MaxRetries:   s.cfg.OptimisticRetryMaxAttempts,
		BaseDelay:    time.Duration(s.cfg.OptimisticRetryBaseDelayMs) * time.Millisecond,
		MaxDelay:     time.Duration(s.cfg.OptimisticRetryMaxDelayMs) * time.Millisecond,
		Multiplier:   s.cfg.OptimisticRetryMultiplier,
		JitterFactor: 0.2,
	}
}

// EdgeVersionedUpdateResult contains the result of an edge update with version checking.
type EdgeVersionedUpdateResult struct {
	EdgeID     string
	SrcNodeID  string
	DstNodeID  string
	OldVersion int64
	NewVersion int64
	Matched    bool
}

// UpdateCoactivationEdgeWithVersion updates a CO_ACTIVATED_WITH edge only if its version matches.
// This is useful for controlled updates where version conflicts need to be handled explicitly.
func (s *Service) UpdateCoactivationEdgeWithVersion(
	ctx context.Context,
	spaceID, srcNodeID, dstNodeID string,
	expectedVersion int64,
	newWeight float64,
	evidenceIncrement int,
) (EdgeVersionedUpdateResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId":           spaceID,
		"srcNodeId":         srcNodeID,
		"dstNodeId":         dstNodeID,
		"expectedVersion":   expectedVersion,
		"newWeight":         newWeight,
		"evidenceIncrement": evidenceIncrement,
	}

	cypher := `
MATCH (a:MemoryNode {space_id: $spaceId, node_id: $srcNodeId})
      -[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->
      (b:MemoryNode {space_id: $spaceId, node_id: $dstNodeId})
WHERE r.version = $expectedVersion
SET r.version = r.version + 1,
    r.updated_at = datetime(),
    r.last_activated_at = datetime(),
    r.weight = $newWeight,
    r.evidence_count = coalesce(r.evidence_count, 0) + $evidenceIncrement
RETURN r.edge_id AS edge_id, r.version AS new_version, $expectedVersion AS old_version, 1 AS matched`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			eid, _ := rec.Get("edge_id")
			newVer, _ := rec.Get("new_version")
			oldVer, _ := rec.Get("old_version")
			matched, _ := rec.Get("matched")

			return EdgeVersionedUpdateResult{
				EdgeID:     fmt.Sprint(eid),
				SrcNodeID:  srcNodeID,
				DstNodeID:  dstNodeID,
				OldVersion: toInt64(oldVer, 0),
				NewVersion: toInt64(newVer, 0),
				Matched:    toInt(matched, 0) == 1,
			}, nil
		}

		if err := res.Err(); err != nil {
			return nil, err
		}

		// No rows returned means version mismatch or edge not found
		return EdgeVersionedUpdateResult{Matched: false}, nil
	})

	if err != nil {
		return EdgeVersionedUpdateResult{}, err
	}

	eur := result.(EdgeVersionedUpdateResult)

	if !eur.Matched {
		// Get actual version for error reporting
		actualVersion, _ := s.getEdgeVersion(ctx, spaceID, srcNodeID, dstNodeID)

		return eur, &optimistic.VersionMismatchError{
			EntityType: "edge",
			EntityID:   fmt.Sprintf("%s-CO_ACTIVATED_WITH->%s", srcNodeID, dstNodeID),
			SpaceID:    spaceID,
			Expected:   expectedVersion,
			Actual:     actualVersion,
			Operation:  "update_coactivation",
		}
	}

	return eur, nil
}

// getEdgeVersion retrieves the current version of a CO_ACTIVATED_WITH edge.
func (s *Service) getEdgeVersion(ctx context.Context, spaceID, srcNodeID, dstNodeID string) (int64, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (a:MemoryNode {space_id: $spaceId, node_id: $srcNodeId})
      -[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->
      (b:MemoryNode {space_id: $spaceId, node_id: $dstNodeId})
RETURN r.version AS version
`, map[string]any{"spaceId": spaceID, "srcNodeId": srcNodeID, "dstNodeId": dstNodeID})
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			ver, _ := res.Record().Get("version")
			return toInt64(ver, 0), nil
		}
		return int64(0), res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// UpdateEdgeWithRetry performs a versioned edge update with retry logic.
// It reads the current version, attempts the update, and retries on conflict.
func (s *Service) UpdateEdgeWithRetry(
	ctx context.Context,
	spaceID, srcNodeID, dstNodeID string,
	newWeight float64,
	evidenceIncrement int,
) (EdgeVersionedUpdateResult, error) {
	var finalResult EdgeVersionedUpdateResult
	var lastErr error

	retryCfg := s.GetRetryConfig()
	result := optimistic.WithRetry(ctx, retryCfg, func(ctx context.Context) error {
		// Get current version
		currentVersion, err := s.getEdgeVersion(ctx, spaceID, srcNodeID, dstNodeID)
		if err != nil {
			return err // Non-retryable (edge may not exist)
		}

		// Attempt versioned update
		res, err := s.UpdateCoactivationEdgeWithVersion(
			ctx, spaceID, srcNodeID, dstNodeID,
			currentVersion, newWeight, evidenceIncrement,
		)
		finalResult = res
		lastErr = err
		return err
	})

	// Log conflict metrics
	if result.VersionConflicts > 0 {
		edgeID := srcNodeID + "->" + dstNodeID
		if result.FinalError == nil {
			slog.Info("learning: edge update succeeded after retries", "attempts", result.Attempts, "conflicts", result.VersionConflicts, "edge", edgeID)
		} else {
			slog.Error("learning: edge update failed after retries", "attempts", result.Attempts, "conflicts", result.VersionConflicts, "edge", edgeID, "error", result.FinalError)
		}
	}

	if result.FinalError != nil {
		return finalResult, result.FinalError
	}

	return finalResult, lastErr
}

// toInt64 converts an interface{} to int64 with a default value.
func toInt64(v any, def int64) int64 {
	if v == nil {
		return def
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	default:
		return def
	}
}

// toInt converts an interface{} to int with a default value.
func toInt(v any, def int) int {
	if v == nil {
		return def
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	case float32:
		return int(n)
	default:
		return def
	}
}
