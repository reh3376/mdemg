package jiminy

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// TrustSnapshot holds serializable trust + feedback state for a session.
type TrustSnapshot struct {
	SessionID     string    `json:"session_id"`
	Score         float64   `json:"score"`
	LastUpdate    time.Time `json:"last_update"`
	FeedbackCount int       `json:"feedback_count"`
	LastFeedAt    time.Time `json:"last_feed_at"`
}

// TrustStore provides write-behind persistence for J17 trust state using Neo4j.
// Follows the RSICStore pattern: dirty-mark + periodic flush + hydrate-on-startup.
type TrustStore struct {
	driver neo4j.DriverWithContext
	mu     sync.Mutex
	dirty  map[string]bool // sessionID → needs flush

	cancel context.CancelFunc

	lastFlush   time.Time
	flushErrors int64
}

// NewTrustStore creates a new trust persistence store.
func NewTrustStore(driver neo4j.DriverWithContext) *TrustStore {
	return &TrustStore{
		driver: driver,
		dirty:  make(map[string]bool),
	}
}

// Start sets up the cancellation context for clean shutdown.
// The actual flush loop lives in Service.StartTrustPersistence, which owns the ticker
// and calls FlushTrust with access to scorer + feedbackCounts.
func (ts *TrustStore) Start(ctx context.Context) {
	_, cancel := context.WithCancel(ctx)
	ts.cancel = cancel
}

// Stop cancels the store context for clean shutdown.
func (ts *TrustStore) Stop() {
	if ts.cancel != nil {
		ts.cancel()
	}
}

// MarkDirty marks a session for the next flush cycle.
func (ts *TrustStore) MarkDirty(sessionID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.dirty[sessionID] = true
}

// DrainDirty returns and clears all dirty session IDs.
func (ts *TrustStore) DrainDirty() []string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.dirty) == 0 {
		return nil
	}
	keys := make([]string, 0, len(ts.dirty))
	for k := range ts.dirty {
		keys = append(keys, k)
	}
	ts.dirty = make(map[string]bool)
	return keys
}

// RemarkDirty re-adds a session to the dirty set (used on flush failure for retry).
func (ts *TrustStore) RemarkDirty(sessionID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.dirty[sessionID] = true
}

// FlushSnapshots writes trust snapshots to Neo4j.
// NOTE: Eventual consistency — after crash recovery, feedbackCounts may lag by up
// to 30s (the flush interval). This is acceptable because feedbackCounts affect only
// the protocolStatus display, not trust score computation or guidance decisions.
func (ts *TrustStore) FlushSnapshots(ctx context.Context, snapshots []TrustSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	var lastErr error
	for _, snap := range snapshots {
		if err := ts.writeSnapshot(ctx, snap); err != nil {
			slog.Warn("j17: trust persistence flush failed",
				"session_id", snap.SessionID, "error", err)
			ts.RemarkDirty(snap.SessionID)
			ts.mu.Lock()
			ts.flushErrors++
			ts.mu.Unlock()
			lastErr = err
		}
	}

	ts.mu.Lock()
	ts.lastFlush = time.Now()
	ts.mu.Unlock()

	return lastErr
}

// LoadAll loads all persisted trust snapshots from Neo4j.
func (ts *TrustStore) LoadAll(ctx context.Context) (map[string]*TrustSnapshot, error) {
	sess := ts.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (s:MemoryNode:J17TrustState)
			RETURN s.session_id AS session_id, s.data AS data
		`, nil)
		if err != nil {
			return nil, err
		}

		snapshots := make(map[string]*TrustSnapshot)
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

			var snap TrustSnapshot
			if err := json.Unmarshal([]byte(ds), &snap); err != nil {
				slog.Warn("j17: failed to unmarshal trust snapshot", "session_id", sid, "error", err)
				continue
			}
			snapshots[sid] = &snap
		}
		return snapshots, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]*TrustSnapshot), nil
}

// GetStatus returns persistence info for health endpoints.
func (ts *TrustStore) GetStatus() map[string]any {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	status := map[string]any{
		"enabled":      true,
		"dirty_keys":   len(ts.dirty),
		"flush_errors": ts.flushErrors,
	}

	if !ts.lastFlush.IsZero() {
		status["last_flush"] = ts.lastFlush.UTC().Format(time.RFC3339)
	}

	return status
}

// writeSnapshot persists a single trust snapshot to Neo4j.
func (ts *TrustStore) writeSnapshot(ctx context.Context, snap TrustSnapshot) error {
	jsonBytes, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	nodeID := "j17-trust-" + snap.SessionID

	sess := ts.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
			MERGE (s:MemoryNode:J17TrustState {node_id: $nodeId})
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
