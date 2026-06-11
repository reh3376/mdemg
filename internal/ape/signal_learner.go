package ape

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// SignalEffectiveness tracks the effectiveness of a meta-cognitive signal.
type SignalEffectiveness struct {
	Code         string  `json:"code"`
	Emissions    int     `json:"emissions"`
	Responses    int     `json:"responses"`
	Strength     float64 `json:"strength"`      // Hebbian strength (0.1-1.0)
	ResponseRate float64 `json:"response_rate"` // responses/emissions
}

// SignalLearner tracks signal emission/response patterns using Hebbian learning.
// Persists state to Neo4j via HydrateSignals/FlushSignals for cross-restart continuity.
type SignalLearner struct {
	mu        sync.RWMutex
	signals   map[string]*signalState
	dirty     map[string]bool // signal codes needing flush
	decayRate float64
	boostRate float64

	driver    neo4j.DriverWithContext // nil = in-memory only (tests)
	cancel    context.CancelFunc
	supervise func(name string, fn func(ctx context.Context) error) // SUPERVISOR-002
}

type signalState struct {
	Emissions int     `json:"emissions"`
	Responses int     `json:"responses"`
	Strength  float64 `json:"strength"`
}

// NewSignalLearner creates a learner with configurable decay/boost rates.
func NewSignalLearner(decayRate, boostRate float64) *SignalLearner {
	if decayRate <= 0 {
		decayRate = 0.05
	}
	if boostRate <= 0 {
		boostRate = 0.1
	}
	return &SignalLearner{
		signals:   make(map[string]*signalState),
		dirty:     make(map[string]bool),
		decayRate: decayRate,
		boostRate: boostRate,
	}
}

// SetDriver sets the Neo4j driver for persistence. Call before HydrateSignals.
func (sl *SignalLearner) SetDriver(driver neo4j.DriverWithContext) {
	sl.driver = driver
}

// RecordEmission records that a signal was emitted (shown to agent).
// If no response follows, strength decays.
func (sl *SignalLearner) RecordEmission(code string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	s, ok := sl.signals[code]
	if !ok {
		s = &signalState{Strength: 0.5}
		sl.signals[code] = s
	}
	s.Emissions++

	// Decay: emission without response reduces strength
	s.Strength -= sl.decayRate
	if s.Strength < 0.1 {
		s.Strength = 0.1
	}
	sl.dirty[code] = true
}

// RecordResponse records that the agent acted on a signal.
// This boosts the signal's strength via Hebbian reinforcement.
func (sl *SignalLearner) RecordResponse(code string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	s, ok := sl.signals[code]
	if !ok {
		s = &signalState{Strength: 0.5}
		sl.signals[code] = s
	}
	s.Responses++

	// Boost: response increases strength (undo decay from emission)
	s.Strength += sl.boostRate + sl.decayRate
	if s.Strength > 1.0 {
		s.Strength = 1.0
	}
	sl.dirty[code] = true
}

// GetStrength returns the current Hebbian strength for a signal code.
func (sl *SignalLearner) GetStrength(code string) float64 {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	s, ok := sl.signals[code]
	if !ok {
		return 0.5 // Default
	}
	return s.Strength
}

// GetAllEffectiveness returns effectiveness stats for all tracked signals.
func (sl *SignalLearner) GetAllEffectiveness() []SignalEffectiveness {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	result := make([]SignalEffectiveness, 0, len(sl.signals))
	for code, s := range sl.signals {
		rate := 0.0
		if s.Emissions > 0 {
			rate = float64(s.Responses) / float64(s.Emissions)
		}
		result = append(result, SignalEffectiveness{
			Code:         code,
			Emissions:    s.Emissions,
			Responses:    s.Responses,
			Strength:     s.Strength,
			ResponseRate: rate,
		})
	}
	return result
}

// HydrateSignals loads persisted signal state from Neo4j.
func (sl *SignalLearner) HydrateSignals(ctx context.Context) error {
	if sl.driver == nil {
		return nil
	}

	sess := sl.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (s:SignalState)
			RETURN s.node_id AS node_id, s.code AS code, s.data AS data
		`, nil)
		if err != nil {
			return nil, err
		}

		states := make(map[string]*signalState)
		codes := make(map[string]string) // node_id -> code
		for res.Next(ctx) {
			record := res.Record()
			codeVal, _ := record.Get("code")
			dataVal, _ := record.Get("data")

			code, ok := codeVal.(string)
			if !ok || code == "" {
				continue
			}
			dataStr, ok := dataVal.(string)
			if !ok || dataStr == "" {
				continue
			}

			var s signalState
			if err := json.Unmarshal([]byte(dataStr), &s); err != nil {
				nodeID, _ := record.Get("node_id")
				slog.Warn("signal learner: failed to unmarshal state", "node_id", nodeID, "error", err)
				continue
			}
			states[code] = &s
			nid, _ := record.Get("node_id")
			if nidStr, ok := nid.(string); ok {
				codes[nidStr] = code
			}
		}
		return states, res.Err()
	})
	if err != nil {
		return fmt.Errorf("signal learner hydrate: %w", err)
	}

	states := result.(map[string]*signalState)
	if len(states) == 0 {
		return nil
	}

	sl.mu.Lock()
	defer sl.mu.Unlock()
	maps.Copy(sl.signals, states)
	slog.Info("signal learner: hydrated from Neo4j", "signals", len(states))
	return nil
}

// FlushSignals writes dirty signal state to Neo4j.
func (sl *SignalLearner) FlushSignals(ctx context.Context) error {
	if sl.driver == nil {
		return nil
	}

	sl.mu.Lock()
	dirtyCodes := make([]string, 0, len(sl.dirty))
	for code := range sl.dirty {
		dirtyCodes = append(dirtyCodes, code)
	}
	// Clear dirty set; re-add on failure
	sl.dirty = make(map[string]bool)

	// Snapshot state while holding lock
	snapshots := make(map[string]*signalState, len(dirtyCodes))
	for _, code := range dirtyCodes {
		if s, ok := sl.signals[code]; ok {
			cp := *s
			snapshots[code] = &cp
		}
	}
	sl.mu.Unlock()

	if len(snapshots) == 0 {
		return nil
	}

	sess := sl.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	var failedCodes []string
	for code, s := range snapshots {
		data, err := json.Marshal(s)
		if err != nil {
			slog.Warn("signal learner: marshal failed", "code", code, "error", err)
			failedCodes = append(failedCodes, code)
			continue
		}

		nodeID := "signal-" + code
		_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, `
				MERGE (s:SignalState {node_id: $nodeId})
				SET s.code = $code,
				    s.data = $data,
				    s.updated_at = datetime()
				RETURN s.node_id
			`, map[string]any{
				"nodeId": nodeID,
				"code":   code,
				"data":   string(data),
			})
			return nil, err
		})
		if err != nil {
			slog.Warn("signal learner: flush failed", "code", code, "error", err)
			failedCodes = append(failedCodes, code)
		}
	}

	// Re-mark failed codes as dirty for next cycle
	if len(failedCodes) > 0 {
		sl.mu.Lock()
		for _, code := range failedCodes {
			sl.dirty[code] = true
		}
		sl.mu.Unlock()
		return fmt.Errorf("signal learner: %d/%d signals failed to flush", len(failedCodes), len(snapshots))
	}

	slog.Debug("signal learner: flushed to Neo4j", "signals", len(snapshots))
	return nil
}

// SetSupervise injects a supervised-goroutine launcher (SUPERVISOR-002).
// Must be called before StartPersistence; nil keeps legacy behavior.
func (sl *SignalLearner) SetSupervise(fn func(name string, fn func(ctx context.Context) error)) {
	sl.supervise = fn
}

// StartPersistence begins a background flush loop (30s interval).
func (sl *SignalLearner) StartPersistence(ctx context.Context) {
	if sl.driver == nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	sl.cancel = cancel

	run := func(runCtx context.Context) error {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return nil
			case <-ctx.Done():
				// Final flush on shutdown
				if err := sl.FlushSignals(context.Background()); err != nil { //nolint:gosec // flush uses Background() intentionally to survive parent cancel
					slog.Warn("signal learner: final flush failed", "error", err)
				}
				return nil
			case <-ticker.C:
				if err := sl.FlushSignals(context.Background()); err != nil {
					slog.Warn("signal learner: periodic flush failed", "error", err)
				}
			}
		}
	}
	if sl.supervise != nil {
		sl.supervise("signal-learner-flush", run)
		return
	}
	go func() { //nolint:gosec // G118: legacy fallback runs for process lifetime; stop is via sl.cancel
		_ = run(context.Background())
	}()
}

// StopPersistence stops the background flush loop.
func (sl *SignalLearner) StopPersistence() {
	if sl.cancel != nil {
		sl.cancel()
	}
}
