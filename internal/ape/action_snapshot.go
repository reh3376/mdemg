package ape

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// NodeEdgeState captures the pre-mutation state of a node or edge.
type NodeEdgeState struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"` // "node" or "edge"
	Properties map[string]any `json:"properties"`
}

// ActionSnapshot records the pre-mutation state for a single RSIC action.
type ActionSnapshot struct {
	SnapshotID    string          `json:"snapshot_id"`
	CycleID       string          `json:"cycle_id"`
	Action        string          `json:"action"`
	SpaceID       string          `json:"space_id"`
	CapturedAt    time.Time       `json:"captured_at"`
	AffectedCount int             `json:"affected_count"`
	PreState      []NodeEdgeState `json:"pre_state"`
	Reversible    bool            `json:"reversible"`
	ExpiresAt     time.Time       `json:"expires_at"`
}

// RollbackResult describes the outcome of a rollback operation.
type RollbackResult struct {
	RolledBack   bool      `json:"rolled_back"`
	SnapshotID   string    `json:"snapshot_id"`
	Action       string    `json:"action"`
	RestoredCount int      `json:"restored_count"`
	RolledBackAt time.Time `json:"rolled_back_at"`
}

// SnapshotStore manages in-memory pre-mutation snapshots for RSIC rollback.
type SnapshotStore struct {
	mu        sync.RWMutex
	snapshots map[string]*ActionSnapshot
	order     []string // insertion order for capacity eviction
	windowSec int
	maxCap    int
	driver    neo4j.DriverWithContext
}

// NewSnapshotStore creates a snapshot store with the given rollback window.
func NewSnapshotStore(driver neo4j.DriverWithContext, rollbackWindowSec int) *SnapshotStore {
	if rollbackWindowSec <= 0 {
		rollbackWindowSec = 3600
	}
	return &SnapshotStore{
		snapshots: make(map[string]*ActionSnapshot),
		windowSec: rollbackWindowSec,
		maxCap:    50,
		driver:    driver,
	}
}

// CaptureSnapshot records pre-mutation state before an action executes.
func (ss *SnapshotStore) CaptureSnapshot(ctx context.Context, cycleID, action, spaceID string) (*ActionSnapshot, error) {
	snap := &ActionSnapshot{
		SnapshotID: fmt.Sprintf("snap-%s", uuid.New().String()[:8]),
		CycleID:    cycleID,
		Action:     action,
		SpaceID:    spaceID,
		CapturedAt: time.Now(),
		Reversible: true,
		ExpiresAt:  time.Now().Add(time.Duration(ss.windowSec) * time.Second),
	}

	// Capture pre-state based on action type
	preState, err := ss.capturePreState(ctx, action, spaceID)
	if err != nil {
		slog.Warn("RSIC snapshot: pre-state capture failed, snapshot created without pre-state", "action", action, "error", err)
		snap.Reversible = false
	} else {
		snap.PreState = preState
		snap.AffectedCount = len(preState)
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Evict oldest if at capacity
	for len(ss.snapshots) >= ss.maxCap && len(ss.order) > 0 {
		oldest := ss.order[0]
		ss.order = ss.order[1:]
		delete(ss.snapshots, oldest)
	}

	ss.snapshots[snap.SnapshotID] = snap
	ss.order = append(ss.order, snap.SnapshotID)

	return snap, nil
}

// Rollback reverts a snapshot's mutations by restoring pre-state.
func (ss *SnapshotStore) Rollback(ctx context.Context, snapshotID string) (*RollbackResult, error) {
	ss.mu.Lock()
	snap, ok := ss.snapshots[snapshotID]
	if !ok {
		ss.mu.Unlock()
		return nil, fmt.Errorf("snapshot not found or expired: %s", snapshotID)
	}
	if time.Now().After(snap.ExpiresAt) {
		delete(ss.snapshots, snapshotID)
		ss.mu.Unlock()
		return nil, fmt.Errorf("snapshot expired: %s", snapshotID)
	}
	if !snap.Reversible {
		ss.mu.Unlock()
		return nil, fmt.Errorf("snapshot %s is not reversible (pre-state capture failed)", snapshotID)
	}
	// Remove snapshot after use
	delete(ss.snapshots, snapshotID)
	ss.mu.Unlock()

	restored, err := ss.applyRollback(ctx, snap)
	if err != nil {
		return nil, fmt.Errorf("rollback failed for %s: %w", snapshotID, err)
	}

	return &RollbackResult{
		RolledBack:    true,
		SnapshotID:    snapshotID,
		Action:        snap.Action,
		RestoredCount: restored,
		RolledBackAt:  time.Now(),
	}, nil
}

// ListSnapshots returns all non-expired snapshots.
func (ss *SnapshotStore) ListSnapshots() []ActionSnapshot {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	now := time.Now()
	result := make([]ActionSnapshot, 0, len(ss.snapshots))
	for _, snap := range ss.snapshots {
		if now.Before(snap.ExpiresAt) {
			result = append(result, *snap)
		}
	}
	return result
}

// CleanupExpired removes expired snapshots.
func (ss *SnapshotStore) CleanupExpired() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	now := time.Now()
	newOrder := make([]string, 0, len(ss.order))
	for _, id := range ss.order {
		if snap, ok := ss.snapshots[id]; ok {
			if now.After(snap.ExpiresAt) {
				delete(ss.snapshots, id)
			} else {
				newOrder = append(newOrder, id)
			}
		}
	}
	ss.order = newOrder
}

// GetRollbackWindowSec returns the configured rollback window.
func (ss *SnapshotStore) GetRollbackWindowSec() int {
	return ss.windowSec
}

// GetSnapshotCount returns the number of held snapshots.
func (ss *SnapshotStore) GetSnapshotCount() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.snapshots)
}

// GetOldestSnapshotAgeSec returns the age of the oldest snapshot in seconds.
func (ss *SnapshotStore) GetOldestSnapshotAgeSec() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if len(ss.order) == 0 {
		return 0
	}
	if snap, ok := ss.snapshots[ss.order[0]]; ok {
		return int(time.Since(snap.CapturedAt).Seconds())
	}
	return 0
}

// capturePreState captures the state that will be affected by the action.
func (ss *SnapshotStore) capturePreState(ctx context.Context, action, spaceID string) ([]NodeEdgeState, error) {
	switch action {
	case "prune_decayed_edges":
		return ss.captureEdgeState(ctx, spaceID,
			`MATCH ()-[e:CO_ACTIVATED_WITH {space_id: $spaceId}]-()
			 WHERE e.weight < 0.1
			 RETURN elementId(e) AS id, e.weight AS weight, e.last_activated AS last_activated
			 LIMIT 200`)

	case "prune_excess_edges":
		return ss.captureEdgeState(ctx, spaceID,
			`MATCH (n:MemoryNode {space_id: $spaceId})-[e:CO_ACTIVATED_WITH]-()
			 WITH n, e ORDER BY e.weight ASC
			 WITH n, collect(e) AS edges WHERE size(edges) > 50
			 UNWIND edges[50..] AS e
			 RETURN elementId(e) AS id, e.weight AS weight, e.last_activated AS last_activated
			 LIMIT 200`)

	case "tombstone_stale":
		return ss.captureNodeState(ctx, spaceID,
			`MATCH (correction:MemoryNode {space_id: $spaceId, obs_type: 'correction'})
			 WHERE correction.created_at > datetime() - duration('P7D')
			 WITH correction
			 MATCH (stale:MemoryNode {space_id: $spaceId})
			 WHERE stale.role_type = 'conversation_observation'
			   AND stale.obs_type <> 'correction'
			   AND stale.created_at < correction.created_at
			   AND NOT coalesce(stale.is_archived, false)
			 WITH DISTINCT stale LIMIT 50
			 RETURN stale.node_id AS id, stale.is_archived AS archived, stale.obs_type AS obs_type`)

	case "graduate_volatile":
		return ss.captureNodeState(ctx, spaceID,
			`MATCH (n:MemoryNode {space_id: $spaceId})
			 WHERE n.role_type = 'conversation_observation'
			   AND n.volatile = true
			   AND coalesce(n.stability_score, 0.1) >= 0.7
			 RETURN n.node_id AS id, n.volatile AS volatile, n.stability_score AS stability_score
			 LIMIT 200`)

	case "refresh_stale_edges":
		return ss.captureEdgeState(ctx, spaceID,
			`MATCH ()-[e:LEARNING_EDGE {space_id: $spaceId}]->()
			 WHERE e.last_activated < datetime() - duration('P30D')
			 RETURN elementId(e) AS id, e.last_activated AS last_activated
			 LIMIT 100`)

	default:
		return nil, nil
	}
}

func (ss *SnapshotStore) captureEdgeState(ctx context.Context, spaceID, cypher string) ([]NodeEdgeState, error) {
	sess := ss.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var states []NodeEdgeState
		for res.Next(ctx) {
			rec := res.Record()
			state := NodeEdgeState{Type: "edge", Properties: make(map[string]any)}
			if v, ok := rec.Get("id"); ok {
				state.ID = fmt.Sprintf("%v", v)
			}
			for _, key := range rec.Keys {
				if key != "id" {
					if v, ok := rec.Get(key); ok {
						state.Properties[key] = v
					}
				}
			}
			states = append(states, state)
		}
		return states, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]NodeEdgeState), nil
}

func (ss *SnapshotStore) captureNodeState(ctx context.Context, spaceID, cypher string) ([]NodeEdgeState, error) {
	sess := ss.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var states []NodeEdgeState
		for res.Next(ctx) {
			rec := res.Record()
			state := NodeEdgeState{Type: "node", Properties: make(map[string]any)}
			if v, ok := rec.Get("id"); ok {
				state.ID = fmt.Sprintf("%v", v)
			}
			for _, key := range rec.Keys {
				if key != "id" {
					if v, ok := rec.Get(key); ok {
						state.Properties[key] = v
					}
				}
			}
			states = append(states, state)
		}
		return states, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]NodeEdgeState), nil
}

// applyRollback restores pre-mutation state from a snapshot.
func (ss *SnapshotStore) applyRollback(ctx context.Context, snap *ActionSnapshot) (int, error) {
	switch snap.Action {
	case "tombstone_stale":
		return ss.rollbackTombstone(ctx, snap)
	case "graduate_volatile":
		return ss.rollbackGraduate(ctx, snap)
	case "refresh_stale_edges":
		// Timestamp-only changes are low-impact; just log
		slog.Info("RSIC rollback: refresh_stale_edges rollback is a no-op", "reason", "timestamp-only change")
		return 0, nil
	case "trigger_consolidation":
		slog.Info("RSIC rollback: trigger_consolidation rollback not applicable", "reason", "constructive action")
		return 0, fmt.Errorf("consolidation rollback not supported (constructive action creates new items)")
	case "prune_decayed_edges", "prune_excess_edges":
		slog.Warn("RSIC rollback: edge rollback not supported, edges were permanently removed", "action", snap.Action)
		return 0, fmt.Errorf("edge deletion rollback not supported (edges were permanently removed)")
	default:
		return 0, fmt.Errorf("unknown action for rollback: %s", snap.Action)
	}
}

func (ss *SnapshotStore) rollbackTombstone(ctx context.Context, snap *ActionSnapshot) (int, error) {
	if len(snap.PreState) == 0 {
		return 0, nil
	}

	ids := make([]string, 0, len(snap.PreState))
	for _, s := range snap.PreState {
		ids = append(ids, s.ID)
	}

	sess := ss.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `MATCH (n:MemoryNode)
			WHERE n.node_id IN $ids AND n.is_archived = true
			SET n.is_archived = false
			RETURN count(n) AS restored`
		res, err := tx.Run(ctx, cypher, map[string]any{"ids": ids})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("restored"); ok {
				return v, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return 0, err
	}
	return int(result.(int64)), nil
}

func (ss *SnapshotStore) rollbackGraduate(ctx context.Context, snap *ActionSnapshot) (int, error) {
	if len(snap.PreState) == 0 {
		return 0, nil
	}

	ids := make([]string, 0, len(snap.PreState))
	for _, s := range snap.PreState {
		ids = append(ids, s.ID)
	}

	sess := ss.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `MATCH (n:MemoryNode)
			WHERE n.node_id IN $ids AND n.volatile = false
			SET n.volatile = true
			RETURN count(n) AS restored`
		res, err := tx.Run(ctx, cypher, map[string]any{"ids": ids})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("restored"); ok {
				return v, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return 0, err
	}
	return int(result.(int64)), nil
}
