package retrieval

import (
	"context"
	"log/slog"
	"time"

	"mdemg/internal/config"
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

// BuildRetryConfigFromEnv creates a RetryConfig from config.Config.
// This is a standalone function for use in other packages.
func BuildRetryConfigFromEnv(cfg config.Config) optimistic.RetryConfig {
	if !cfg.OptimisticRetryEnabled {
		return optimistic.RetryConfig{
			MaxRetries:   0,
			BaseDelay:    time.Duration(cfg.OptimisticRetryBaseDelayMs) * time.Millisecond,
			MaxDelay:     time.Duration(cfg.OptimisticRetryMaxDelayMs) * time.Millisecond,
			Multiplier:   cfg.OptimisticRetryMultiplier,
			JitterFactor: 0.2,
		}
	}

	return optimistic.RetryConfig{
		MaxRetries:   cfg.OptimisticRetryMaxAttempts,
		BaseDelay:    time.Duration(cfg.OptimisticRetryBaseDelayMs) * time.Millisecond,
		MaxDelay:     time.Duration(cfg.OptimisticRetryMaxDelayMs) * time.Millisecond,
		Multiplier:   cfg.OptimisticRetryMultiplier,
		JitterFactor: 0.2,
	}
}

// IngestWithRetry wraps an ingest operation with optimistic retry logic.
// This is useful when ingesting updates to existing nodes where version
// conflicts may occur due to concurrent updates.
//
// The provided operation function should:
//   - Return nil on success
//   - Return optimistic.ErrVersionMismatch to trigger retry
//   - Return any other error to fail immediately
//
// Returns the result from WithRetry including statistics about attempts.
func (s *Service) IngestWithRetry(ctx context.Context, op optimistic.OperationFunc) optimistic.RetryResult {
	cfg := s.GetRetryConfig()
	result := optimistic.WithRetry(ctx, cfg, op)

	// Log conflict resolution if conflicts occurred
	if result.VersionConflicts > 0 {
		if result.FinalError == nil {
			slog.Info("ingest_retry: succeeded", "attempts", result.Attempts, "conflicts", result.VersionConflicts, "duration", result.TotalDuration)
			// Log resolved conflict for metrics
			LogConflict(ConflictEvent{
				Type:       ConflictVersionMismatch,
				Operation:  "ingest",
				Resolved:   true,
				Resolution: ResolutionRetrySucceeded,
				Details:    formatRetryDetails(result),
			})
		} else {
			slog.Error("ingest_retry: failed", "attempts", result.Attempts, "conflicts", result.VersionConflicts, "duration", result.TotalDuration, "error", result.FinalError)
			// Log unresolved conflict for metrics
			LogConflict(ConflictEvent{
				Type:       ConflictVersionMismatch,
				Operation:  "ingest",
				Resolved:   false,
				Resolution: ResolutionUnresolved,
				Details:    formatRetryDetails(result),
			})
		}
	}

	return result
}

// formatRetryDetails formats retry statistics for logging.
func formatRetryDetails(result optimistic.RetryResult) string {
	return "attempts=" + itoa(result.Attempts) + " conflicts=" + itoa(result.VersionConflicts) + " duration=" + result.TotalDuration.String()
}

// itoa is a simple int to string converter.
func itoa(i int) string {
	return string('0' + rune(i%10))
}

// UpdateNodeWithRetry performs a versioned node update with retry logic.
// It first reads the current version, then attempts the update with version checking.
// If a version mismatch occurs, it retries with exponential backoff.
func (s *Service) UpdateNodeWithRetry(
	ctx context.Context,
	spaceID, nodeID string,
	updateFn func() map[string]any,
) (VersionedUpdateResult, error) {
	var finalResult VersionedUpdateResult
	var lastErr error

	retryCfg := s.GetRetryConfig()
	result := optimistic.WithRetry(ctx, retryCfg, func(ctx context.Context) error {
		// Get current version
		currentVersion, err := s.getNodeVersion(ctx, spaceID, nodeID)
		if err != nil {
			return err // Non-retryable
		}

		// Attempt versioned update
		res, err := s.UpdateNodeWithVersion(ctx, spaceID, nodeID, currentVersion, updateFn)
		finalResult = res
		lastErr = err
		return err
	})

	// Log conflict metrics
	if result.VersionConflicts > 0 {
		if result.FinalError == nil {
			LogConflictResolved(ConflictEvent{
				Type:        ConflictVersionMismatch,
				SpaceID:     spaceID,
				NodeID:      nodeID,
				Operation:   "update_node",
				ExpectedVer: finalResult.OldVersion,
				ActualVer:   finalResult.NewVersion,
			}, ResolutionRetrySucceeded)
		} else {
			LogVersionMismatch(spaceID, nodeID, "update_node", finalResult.OldVersion, finalResult.NewVersion)
		}
	}

	if result.FinalError != nil {
		return finalResult, result.FinalError
	}

	return finalResult, lastErr
}

// UpdateEdgeWithRetry performs a versioned edge update with retry logic.
func (s *Service) UpdateEdgeWithRetry(
	ctx context.Context,
	spaceID, srcNodeID, dstNodeID string,
	updateFn func() map[string]any,
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
		res, err := s.UpdateEdgeWithVersion(ctx, spaceID, srcNodeID, dstNodeID, currentVersion, updateFn)
		finalResult = res
		lastErr = err
		return err
	})

	// Log conflict metrics
	if result.VersionConflicts > 0 {
		edgeID := srcNodeID + "->" + dstNodeID
		if result.FinalError == nil {
			LogConflictResolved(ConflictEvent{
				Type:        ConflictVersionMismatch,
				SpaceID:     spaceID,
				NodeID:      edgeID,
				Operation:   "update_edge",
				ExpectedVer: finalResult.OldVersion,
				ActualVer:   finalResult.NewVersion,
			}, ResolutionRetrySucceeded)
		} else {
			LogVersionMismatch(spaceID, edgeID, "update_edge", finalResult.OldVersion, finalResult.NewVersion)
		}
	}

	if result.FinalError != nil {
		return finalResult, result.FinalError
	}

	return finalResult, lastErr
}

// PropagateEdgeStalenessAfterIngest should be called after a successful ingest
// that changed a node's embedding. It propagates staleness to connected edges.
func (s *Service) PropagateEdgeStalenessAfterIngest(ctx context.Context, spaceID, nodeID string) {
	if !s.cfg.EdgeStalenessCascadeEnabled {
		return
	}

	result, err := s.PropagateEdgeStaleness(ctx, spaceID, nodeID)
	if err != nil {
		slog.Warn("failed to propagate edge staleness", "node_id", nodeID, "error", err)
		return
	}

	if result.CoactivationEdgesMarked > 0 || result.AssociatedEdgesMarked > 0 {
		// Invalidate cache since edge relationships may have changed
		s.InvalidateSpaceCache(spaceID)
	}
}
