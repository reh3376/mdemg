package jiminy

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// EscalationStore provides write-behind persistence for J12 escalation state using Neo4j.
// Follows the TrustStore pattern: dirty-mark + periodic flush + hydrate-on-startup.
type EscalationStore struct {
	driver neo4j.DriverWithContext
	mu     sync.Mutex
	dirty  map[string]bool // sessionID → needs flush

	cancel      context.CancelFunc
	lastFlush   time.Time
	flushErrors int64
}

// NewEscalationStore creates a new escalation persistence store.
func NewEscalationStore(driver neo4j.DriverWithContext) *EscalationStore {
	return &EscalationStore{
		driver: driver,
		dirty:  make(map[string]bool),
	}
}

// Start sets up the cancellation context for clean shutdown.
func (es *EscalationStore) Start(ctx context.Context) {
	_, cancel := context.WithCancel(ctx)
	es.cancel = cancel
}

// Stop cancels the store context for clean shutdown.
func (es *EscalationStore) Stop() {
	if es.cancel != nil {
		es.cancel()
	}
}

// MarkDirty marks a session for the next flush cycle.
func (es *EscalationStore) MarkDirty(sessionID string) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.dirty[sessionID] = true
}

// DrainDirty returns and clears all dirty session IDs.
func (es *EscalationStore) DrainDirty() []string {
	es.mu.Lock()
	defer es.mu.Unlock()
	if len(es.dirty) == 0 {
		return nil
	}
	keys := make([]string, 0, len(es.dirty))
	for k := range es.dirty {
		keys = append(keys, k)
	}
	es.dirty = make(map[string]bool)
	return keys
}

// RemarkDirty re-adds a session to the dirty set (used on flush failure for retry).
func (es *EscalationStore) RemarkDirty(sessionID string) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.dirty[sessionID] = true
}

// FlushSnapshots writes escalation snapshots to Neo4j.
func (es *EscalationStore) FlushSnapshots(ctx context.Context, snapshots []EscalationSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	var lastErr error
	for _, snap := range snapshots {
		if err := es.writeSnapshot(ctx, snap); err != nil {
			slog.Warn("j12: escalation persistence flush failed",
				"session_id", snap.SessionID, "error", err)
			es.RemarkDirty(snap.SessionID)
			es.mu.Lock()
			es.flushErrors++
			es.mu.Unlock()
			lastErr = err
		}
	}

	es.mu.Lock()
	es.lastFlush = time.Now()
	es.mu.Unlock()

	return lastErr
}

// LoadAll loads all persisted escalation snapshots from Neo4j.
func (es *EscalationStore) LoadAll(ctx context.Context) (map[string]*EscalationSnapshot, error) {
	sess := es.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (s:MemoryNode:J12EscalationState)
			RETURN s.session_id AS session_id, s.data AS data
		`, nil)
		if err != nil {
			return nil, err
		}

		snapshots := make(map[string]*EscalationSnapshot)
		for res.Next(ctx) {
			record := res.Record()
			sessionID, _ := record.Get("session_id")
			dataStr, _ := record.Get("data")

			sid, ok := sessionID.(string)
			if !ok || sid == "" {
				continue
			}
			ds, ok := dataStr.(string)
			if !ok || ds == "" {
				continue
			}

			var snap EscalationSnapshot
			if err := json.Unmarshal([]byte(ds), &snap); err != nil {
				slog.Warn("j12: failed to unmarshal escalation snapshot", "session_id", sid, "error", err)
				continue
			}
			snapshots[sid] = &snap
		}
		return snapshots, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]*EscalationSnapshot), nil
}

// GetStatus returns persistence info for health endpoints.
func (es *EscalationStore) GetStatus() map[string]any {
	es.mu.Lock()
	defer es.mu.Unlock()

	status := map[string]any{
		"enabled":      true,
		"dirty_keys":   len(es.dirty),
		"flush_errors": es.flushErrors,
	}

	if !es.lastFlush.IsZero() {
		status["last_flush"] = es.lastFlush.UTC().Format(time.RFC3339)
	}

	return status
}

// writeSnapshot persists a single escalation snapshot to Neo4j.
func (es *EscalationStore) writeSnapshot(ctx context.Context, snap EscalationSnapshot) error {
	jsonBytes, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	nodeID := "j12-esc-" + snap.SessionID

	sess := es.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
			MERGE (s:MemoryNode:J12EscalationState {node_id: $nodeId})
			SET s.session_id = $sessionId,
			    s.data = $data,
			    s.updated_at = datetime()
			RETURN s.node_id
		`, map[string]any{
			"nodeId":    nodeID,
			"sessionId": snap.SessionID,
			"data":      string(jsonBytes),
		})
		return nil, err
	})
	return err
}
