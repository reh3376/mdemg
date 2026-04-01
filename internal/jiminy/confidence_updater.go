package jiminy

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
)

// ConfidenceUpdater applies Bayesian confidence updates to constraint nodes
// based on guidance outcomes, and archives nodes whose confidence falls below
// the configured threshold.
type ConfidenceUpdater struct {
	driver           neo4j.DriverWithContext
	boostPerPositive float64 // confidence increase per followed guidance (default 0.02)
	decayPerNegative float64 // confidence decrease per ignored guidance (default 0.03)
	archiveThreshold float64 // confidence below which the node is archived (default 0.3)
}

// NewConfidenceUpdater creates a ConfidenceUpdater using the provided config for
// threshold and rate parameters.
func NewConfidenceUpdater(driver neo4j.DriverWithContext, cfg config.Config) *ConfidenceUpdater {
	boost := cfg.ConstraintConfidenceBoostPerPos
	if boost <= 0 {
		boost = 0.02
	}
	decay := cfg.ConstraintConfidenceDecayPerNeg
	if decay <= 0 {
		decay = 0.03
	}
	threshold := cfg.ConstraintArchiveThreshold
	if threshold <= 0 {
		threshold = 0.3
	}
	return &ConfidenceUpdater{
		driver:           driver,
		boostPerPositive: boost,
		decayPerNegative: decay,
		archiveThreshold: threshold,
	}
}

// UpdateConfidence applies a confidence delta to a constraint node identified by
// nodeID, increments the appropriate outcome counter, and archives the node if its
// new confidence falls below the archive threshold.
//
// Delta rules:
//   - OutcomeFollowed:     +boostPerPositive, capped at 0.95
//   - OutcomeIgnored:      -decayPerNegative
//   - OutcomeContradicted: -decayPerNegative * 1.5
//   - OutcomeUnknown:      no change (returns nil immediately)
func (u *ConfidenceUpdater) UpdateConfidence(ctx context.Context, nodeID string, outcome GuidanceOutcome) error {
	if outcome == OutcomeUnknown {
		return nil
	}

	// Compute the raw delta for this outcome type.
	var delta float64
	switch outcome {
	case OutcomeFollowed:
		delta = u.boostPerPositive
	case OutcomeIgnored:
		delta = -u.decayPerNegative
	case OutcomeContradicted:
		delta = -(u.decayPerNegative * 1.5)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	archiveThreshold := u.archiveThreshold

	// Single Cypher MATCH + SET with CASE logic:
	//   1. Compute new_confidence clamped to [0, 0.95].
	//   2. Set status to "archived" when new_confidence < archiveThreshold.
	//   3. Increment the appropriate counter.
	//   4. Update last_surfaced_at.
	cypher := `
MATCH (n:MemoryNode {node_id: $nodeId})
WITH n, coalesce(n.confidence, 0.5) + $delta AS raw
WITH n,
     CASE
       WHEN raw > 0.95 THEN 0.95
       WHEN raw < 0.0  THEN 0.0
       ELSE raw
     END AS new_confidence
SET
  n.confidence        = new_confidence,
  n.last_surfaced_at  = $now,
  n.total_surfaced    = coalesce(n.total_surfaced, 0) + 1,
  n.total_followed    = coalesce(n.total_followed, 0)    + CASE WHEN $outcome = 'followed'     THEN 1 ELSE 0 END,
  n.total_ignored     = coalesce(n.total_ignored, 0)     + CASE WHEN $outcome = 'ignored'      THEN 1 ELSE 0 END,
  n.total_contradicted = coalesce(n.total_contradicted, 0) + CASE WHEN $outcome = 'contradicted' THEN 1 ELSE 0 END,
  n.status            = CASE WHEN new_confidence < $archiveThreshold AND n.constraint_type IS NOT NULL THEN 'archived' ELSE coalesce(n.status, 'active') END
RETURN n.node_id AS node_id, n.confidence AS confidence, n.status AS status`

	sess := u.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer sess.Close(ctx)

	_, err := neo4j.ExecuteWrite(ctx, sess, func(tx neo4j.ManagedTransaction) (bool, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"nodeId":           nodeID,
			"delta":            delta,
			"outcome":          string(outcome),
			"now":              now,
			"archiveThreshold": archiveThreshold,
		})
		if err != nil {
			return false, err
		}
		// Consume the result to surface any query-level errors.
		_, err = res.Consume(ctx)
		return err == nil, err
	})
	if err != nil {
		return fmt.Errorf("confidence updater: update node %q (outcome=%s): %w", nodeID, outcome, err)
	}
	return nil
}

// ArchiveStaleConstraints finds all constraint nodes in the given space whose
// confidence is below the archive threshold and sets their status to "archived".
// It returns the count of nodes that were archived.
func (u *ConfidenceUpdater) ArchiveStaleConstraints(ctx context.Context, spaceID string) (int, error) {
	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.constraint_type IS NOT NULL
  AND coalesce(n.confidence, 0.5) < $archiveThreshold
  AND coalesce(n.status, 'active') <> 'archived'
SET n.status = 'archived'
RETURN count(n) AS archived_count`

	sess := u.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer sess.Close(ctx)

	count, err := neo4j.ExecuteWrite(ctx, sess, func(tx neo4j.ManagedTransaction) (int, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":          spaceID,
			"archiveThreshold": u.archiveThreshold,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			raw, _ := rec.Get("archived_count")
			return int(asFloat64(raw)), res.Err()
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, fmt.Errorf("confidence updater: archive stale constraints (space=%s): %w", spaceID, err)
	}

	return count, nil
}
