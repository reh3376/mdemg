package ape

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ───────────── Persistence Snapshots ─────────────

// CalibrationSnapshot holds serializable calibration state.
type CalibrationSnapshot struct {
	ActionHistory map[string][]ActionOutcome `json:"action_history"`
	CycleHistory  []CycleOutcome             `json:"cycle_history"`
}

// OrchestrationSnapshot holds serializable orchestration policy state.
type OrchestrationSnapshot struct {
	LastTriggers    []TriggerRecord            `json:"last_triggers"`
	SessionCounters map[string]*SessionCounter `json:"session_counters"`
}

// ───────────── RSICStore ─────────────

// RSICStore provides write-behind persistence for RSIC state using Neo4j.
// State changes are cached in memory and flushed to Neo4j periodically.
type RSICStore struct {
	driver neo4j.DriverWithContext
	mu     sync.Mutex
	dirty  map[string]bool

	cancel    context.CancelFunc
	supervise func(name string, fn func(ctx context.Context) error) // SUPERVISOR-002

	// In-memory cached state
	calibrationData   map[string]*CalibrationSnapshot // key: spaceID
	watchdogData      map[string]*WatchdogState       // key: spaceID
	orchestrationData *OrchestrationSnapshot

	lastFlush   time.Time
	flushErrors int64
}

// NewRSICStore creates a new persistence store.
func NewRSICStore(driver neo4j.DriverWithContext) *RSICStore {
	return &RSICStore{
		driver:          driver,
		dirty:           make(map[string]bool),
		calibrationData: make(map[string]*CalibrationSnapshot),
		watchdogData:    make(map[string]*WatchdogState),
	}
}

// SetSupervise injects a supervised-goroutine launcher (SUPERVISOR-002).
// Must be called before Start; nil keeps legacy bare-goroutine behavior.
func (s *RSICStore) SetSupervise(fn func(name string, fn func(ctx context.Context) error)) {
	s.supervise = fn
}

// Start begins the background flush goroutine (every 30 seconds).
func (s *RSICStore) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	run := func(runCtx context.Context) error {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return nil
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if err := s.Flush(context.Background()); err != nil { //nolint:gosec // G118: flush uses Background() intentionally — must succeed independently of parent ctx
					slog.Warn("RSIC persistence flush failed", "error", err)
				}
			}
		}
	}
	if s.supervise != nil {
		s.supervise("rsic-store-flush", run)
		return
	}
	go func() { //nolint:gosec // G118: legacy fallback runs for process lifetime; stop is via s.cancel
		_ = run(context.Background())
	}()
}

// Stop stops the flush goroutine and performs a final flush.
func (s *RSICStore) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	// Final flush
	if err := s.Flush(context.Background()); err != nil {
		slog.Warn("RSIC persistence final flush failed", "error", err)
	}
}

// MarkDirty marks a state key for the next flush cycle.
func (s *RSICStore) MarkDirty(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty[key] = true
}

// ───────────── Save (in-memory cache update) ─────────────

// SaveCalibration updates the in-memory calibration snapshot.
func (s *RSICStore) SaveCalibration(spaceID string, history []CycleOutcome, actions map[string][]ActionOutcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calibrationData[spaceID] = &CalibrationSnapshot{
		ActionHistory: actions,
		CycleHistory:  history,
	}
	s.dirty["calibration:"+spaceID] = true
}

// SaveWatchdogState updates the in-memory watchdog state.
func (s *RSICStore) SaveWatchdogState(spaceID string, state WatchdogState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchdogData[spaceID] = &state
	s.dirty["watchdog:"+spaceID] = true
}

// SaveOrchestrationState updates the in-memory orchestration snapshot.
func (s *RSICStore) SaveOrchestrationState(triggers []TriggerRecord, counters map[string]*SessionCounter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orchestrationData = &OrchestrationSnapshot{
		LastTriggers:    triggers,
		SessionCounters: counters,
	}
	s.dirty["orchestration"] = true
}

// ───────────── Load (from Neo4j) ─────────────

// LoadCalibration loads persisted calibration state from Neo4j.
func (s *RSICStore) LoadCalibration(spaceID string) ([]CycleOutcome, map[string][]ActionOutcome, error) {
	data, err := s.loadState(spaceID, "calibration")
	if err != nil || data == "" {
		return nil, nil, err
	}

	var snap CalibrationSnapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return nil, nil, err
	}

	// Cache in memory
	s.mu.Lock()
	s.calibrationData[spaceID] = &snap
	s.mu.Unlock()

	return snap.CycleHistory, snap.ActionHistory, nil
}

// LoadWatchdogState loads persisted watchdog state from Neo4j.
func (s *RSICStore) LoadWatchdogState(spaceID string) (*WatchdogState, error) {
	data, err := s.loadState(spaceID, "watchdog")
	if err != nil || data == "" {
		return nil, err
	}

	var state WatchdogState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return nil, err
	}

	// Cache in memory
	s.mu.Lock()
	s.watchdogData[spaceID] = &state
	s.mu.Unlock()

	return &state, nil
}

// LoadOrchestrationState loads persisted orchestration state from Neo4j.
func (s *RSICStore) LoadOrchestrationState() ([]TriggerRecord, map[string]*SessionCounter, error) {
	data, err := s.loadState("global", "orchestration")
	if err != nil || data == "" {
		return nil, nil, err
	}

	var snap OrchestrationSnapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return nil, nil, err
	}

	// Cache in memory
	s.mu.Lock()
	s.orchestrationData = &snap
	s.mu.Unlock()

	return snap.LastTriggers, snap.SessionCounters, nil
}

// ───────────── Flush ─────────────

// Flush writes all dirty state to Neo4j.
func (s *RSICStore) Flush(ctx context.Context) error {
	s.mu.Lock()
	dirtyKeys := make(map[string]bool, len(s.dirty))
	for k, v := range s.dirty {
		dirtyKeys[k] = v
	}
	// Clear dirty set immediately
	s.dirty = make(map[string]bool)
	s.mu.Unlock()

	if len(dirtyKeys) == 0 {
		return nil
	}

	var lastErr error

	for key := range dirtyKeys {
		var err error
		switch {
		case len(key) > 12 && key[:12] == "calibration:":
			spaceID := key[12:]
			s.mu.Lock()
			snap := s.calibrationData[spaceID]
			s.mu.Unlock()
			if snap != nil {
				err = s.writeState(ctx, spaceID, "calibration", "rsic-state-calibration-"+spaceID, snap)
			}

		case len(key) > 9 && key[:9] == "watchdog:":
			spaceID := key[9:]
			s.mu.Lock()
			state := s.watchdogData[spaceID]
			s.mu.Unlock()
			if state != nil {
				err = s.writeState(ctx, spaceID, "watchdog", "rsic-state-watchdog-"+spaceID, state)
			}

		case key == "orchestration":
			s.mu.Lock()
			snap := s.orchestrationData
			s.mu.Unlock()
			if snap != nil {
				err = s.writeState(ctx, "global", "orchestration", "rsic-state-orchestration-global", snap)
			}
		}

		if err != nil {
			slog.Warn("RSIC persistence flush failed for key", "key", key, "error", err)
			s.mu.Lock()
			s.flushErrors++
			// Re-mark as dirty for retry
			s.dirty[key] = true
			s.mu.Unlock()
			lastErr = err
		}
	}

	s.mu.Lock()
	s.lastFlush = time.Now()
	s.mu.Unlock()

	return lastErr
}

// ───────────── Cleanup ─────────────

// CleanupExpired removes RSICState nodes older than 30 days.
func (s *RSICStore) CleanupExpired(ctx context.Context) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (s:MemoryNode:RSICState)
			WHERE s.updated_at < datetime() - duration({days: 30})
			DETACH DELETE s
			RETURN count(s) AS removed
		`, nil)
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("removed"); ok {
				if n, ok := v.(int64); ok {
					return int(n), nil
				}
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// ───────────── Status ─────────────

// GetStatus returns persistence info for the health endpoint.
func (s *RSICStore) GetStatus() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := map[string]any{
		"enabled":      true,
		"dirty_keys":   len(s.dirty),
		"flush_errors": s.flushErrors,
	}

	if !s.lastFlush.IsZero() {
		status["last_flush"] = s.lastFlush.UTC().Format(time.RFC3339)
	}

	// Count state nodes from cache
	stateNodes := len(s.calibrationData) + len(s.watchdogData)
	if s.orchestrationData != nil {
		stateNodes++
	}
	status["state_nodes"] = stateNodes

	return status
}

// ───────────── Internal Neo4j helpers ─────────────

func (s *RSICStore) writeState(ctx context.Context, spaceID, rsicType, nodeID string, data any) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
			MERGE (s:MemoryNode:RSICState {node_id: $nodeId, space_id: $spaceId})
			SET s.rsic_type = $rsicType,
			    s.data = $data,
			    s.updated_at = datetime()
			RETURN s.node_id
		`, map[string]any{
			"nodeId":   nodeID,
			"spaceId":  spaceID,
			"rsicType": rsicType,
			"data":     string(jsonBytes),
		})
		return nil, err
	})
	return err
}

func (s *RSICStore) loadState(spaceID, rsicType string) (string, error) {
	ctx := context.Background()
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (s:MemoryNode:RSICState {space_id: $spaceId, rsic_type: $rsicType})
			RETURN s.data AS data, s.updated_at AS updated_at
			ORDER BY s.updated_at DESC
			LIMIT 1
		`, map[string]any{
			"spaceId":  spaceID,
			"rsicType": rsicType,
		})
		if err != nil {
			return "", err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("data"); ok && v != nil {
				if str, ok := v.(string); ok {
					return str, nil
				}
			}
		}
		return "", res.Err()
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}
