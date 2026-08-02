package jiminy

import (
	"sync"
	"time"

	"mdemg/internal/config"
)

// escalationKey uniquely identifies a constraint within a session.
type escalationKey struct {
	SessionID string
	NodeID    string
}

// escalationState tracks the escalation state for a single constraint in a session.
type escalationState struct {
	Level        EscalationLevel
	IgnoreCount  int
	LastSurfaced time.Time
}

// EscalationTracker tracks per-session, per-constraint escalation state.
// Thread-safe in-memory map with TTL-based decay.
type EscalationTracker struct {
	mu            sync.Mutex
	states        map[escalationKey]*escalationState
	warnAfter     int                    // ignores before WARNED
	escalateAfter int                    // ignores before ESCALATED
	blockAfter    int                    // ignores before BLOCKED
	blockEnabled  bool                   // whether BLOCKED state is allowed
	decayDuration time.Duration          // reset if not surfaced within this duration
	onDirty       func(sessionID string) // callback when state changes (for persistence)
}

// NewEscalationTracker creates a new EscalationTracker from config.
func NewEscalationTracker(cfg config.Config) *EscalationTracker {
	warnAfter := cfg.JiminyEscalationWarnAfter
	if warnAfter <= 0 {
		warnAfter = 2
	}
	escalateAfter := cfg.JiminyEscalationEscalateAfter
	if escalateAfter <= 0 {
		escalateAfter = 4
	}
	blockAfter := cfg.JiminyEscalationBlockAfter
	if blockAfter <= 0 {
		blockAfter = 6
	}
	decayMinutes := cfg.JiminyEscalationDecayMinutes
	if decayMinutes <= 0 {
		decayMinutes = 60
	}

	return &EscalationTracker{
		states:        make(map[escalationKey]*escalationState),
		warnAfter:     warnAfter,
		escalateAfter: escalateAfter,
		blockAfter:    blockAfter,
		blockEnabled:  cfg.JiminyEscalationBlockEnabled,
		decayDuration: time.Duration(decayMinutes) * time.Minute,
	}
}

// SetOnDirty sets a callback invoked when escalation state changes for a session.
// Used by EscalationStore to mark sessions for persistence flushing.
func (et *EscalationTracker) SetOnDirty(fn func(sessionID string)) {
	et.mu.Lock()
	defer et.mu.Unlock()
	et.onDirty = fn
}

// RecordSurface records that a constraint was surfaced to the agent.
// Returns the current escalation level after recording.
func (et *EscalationTracker) RecordSurface(sessionID, nodeID string) EscalationLevel {
	et.mu.Lock()
	defer et.mu.Unlock()

	key := escalationKey{SessionID: sessionID, NodeID: nodeID}
	state, ok := et.states[key]
	if !ok {
		state = &escalationState{
			Level:        EscalationSurfaced,
			LastSurfaced: time.Now(),
		}
		et.states[key] = state
		if et.onDirty != nil {
			et.onDirty(sessionID)
		}
		return EscalationSurfaced
	}

	// ESCALATION-ACCUMULATE-001 Defect 3 fix: decay resets LEVEL to Surfaced but
	// PRESERVES IgnoreCount. Accumulation across normal activity gaps is the whole
	// point of the counter; wiping it on decay made WARNED unreachable in low-
	// cadence sessions (100% of J12EscalationState rows sat at ignore_count=0
	// pre-fix). Operator can still see the "constraint went quiet" signal via the
	// level demoting; the accumulator survives.
	if time.Since(state.LastSurfaced) > et.decayDuration {
		state.Level = EscalationSurfaced
	}

	state.LastSurfaced = time.Now()
	// ESCALATION-ACCUMULATE-001 Defect 4 fix: mark dirty on EVERY surface (not
	// just new-key or decay-reset). Persistence liveness matters more than the
	// tiny per-surface cost — pre-fix the persisted state lagged in-memory by
	// hours whenever the same nodes kept re-surfacing.
	if et.onDirty != nil {
		et.onDirty(sessionID)
	}
	return state.Level
}

// RecordOutcome advances the escalation state machine based on guidance outcome.
//
// ESCALATION-ACCUMULATE-001 (Defect 1 fix): if the (session, node) key has no
// prior state, CREATE one on-demand instead of returning EscalationInactive.
// The outcome is authoritative evidence that the constraint was live for the
// classifier — requiring a preceding RecordSurface introduces a temporal-order
// fragility (surface-before-outcome, no restart between) that the shipped
// architecture can't honor. Pre-fix behavior silently dropped every outcome
// on a not-in-map key, causing 100% of J12EscalationState rows on mdemg-dev
// to sit at level=surfaced ignore_count=0 despite 2,177 outcomes/7d.
func (et *EscalationTracker) RecordOutcome(sessionID, nodeID string, outcome GuidanceOutcome) EscalationLevel {
	et.mu.Lock()
	defer et.mu.Unlock()

	key := escalationKey{SessionID: sessionID, NodeID: nodeID}
	state, ok := et.states[key]
	if !ok {
		state = &escalationState{
			Level:        EscalationSurfaced,
			LastSurfaced: time.Now(),
		}
		et.states[key] = state
		if et.onDirty != nil {
			et.onDirty(sessionID)
		}
	}

	prevLevel := state.Level

	switch outcome {
	case OutcomeFollowed:
		state.Level = EscalationResolved
		state.IgnoreCount = 0
		if prevLevel != EscalationResolved && et.onDirty != nil {
			et.onDirty(sessionID)
		}
		return EscalationResolved

	case OutcomeIgnored, OutcomeContradicted:
		state.IgnoreCount++

		if et.blockEnabled && state.IgnoreCount >= et.blockAfter {
			state.Level = EscalationBlocked
		} else if state.IgnoreCount >= et.escalateAfter {
			state.Level = EscalationEscalated
		} else if state.IgnoreCount >= et.warnAfter {
			state.Level = EscalationWarned
		}
		if state.Level != prevLevel && et.onDirty != nil {
			et.onDirty(sessionID)
		}
		return state.Level
	}

	return state.Level
}

// GetState returns the current escalation state for a constraint in a session.
func (et *EscalationTracker) GetState(sessionID, nodeID string) EscalationLevel {
	et.mu.Lock()
	defer et.mu.Unlock()

	key := escalationKey{SessionID: sessionID, NodeID: nodeID}
	state, ok := et.states[key]
	if !ok {
		return EscalationInactive
	}

	// Check decay
	if time.Since(state.LastSurfaced) > et.decayDuration {
		return EscalationInactive
	}

	return state.Level
}

// GetSessionEscalation builds a SessionEscalation summary for the response.
func (et *EscalationTracker) GetSessionEscalation(sessionID string) *SessionEscalation {
	et.mu.Lock()
	defer et.mu.Unlock()

	summary := &SessionEscalation{}
	now := time.Now()

	for key, state := range et.states {
		if key.SessionID != sessionID {
			continue
		}
		// Skip decayed entries
		if now.Sub(state.LastSurfaced) > et.decayDuration {
			continue
		}

		switch state.Level {
		case EscalationWarned:
			summary.WarnedCount++
			summary.ActiveEscalations++
		case EscalationEscalated:
			summary.EscalatedCount++
			summary.ActiveEscalations++
		case EscalationBlocked:
			summary.BlockedCount++
			summary.ActiveEscalations++
		case EscalationSurfaced:
			summary.ActiveEscalations++
		}

		summary.Details = append(summary.Details, EscalationEntry{
			NodeID:       key.NodeID,
			Level:        state.Level,
			IgnoreCount:  state.IgnoreCount,
			LastSurfaced: state.LastSurfaced,
		})
	}

	if summary.ActiveEscalations == 0 {
		return nil
	}
	return summary
}

// ApplyEscalation modifies a guidance item based on its escalation level.
func ApplyEscalation(item *GuidanceItem, level EscalationLevel) {
	item.EscalationLevel = level

	switch level {
	case EscalationWarned:
		item.Priority = "high"
		item.Content = "REPEATED: " + item.Content
	case EscalationEscalated:
		item.Priority = "high"
		item.Confidence = 0.95
		item.Content = "ESCALATED — MUST COMPLY: " + item.Content
	case EscalationBlocked:
		item.Priority = "high"
		item.Confidence = 1.0
		item.Content = "BLOCKED — VIOLATION DETECTED: " + item.Content
	}
}

// CleanupExpired removes entries that have decayed past the TTL.
func (et *EscalationTracker) CleanupExpired() int {
	et.mu.Lock()
	defer et.mu.Unlock()

	removed := 0
	now := time.Now()
	for key, state := range et.states {
		if now.Sub(state.LastSurfaced) > et.decayDuration {
			delete(et.states, key)
			removed++
		}
	}
	return removed
}

// Size returns the number of tracked escalation entries.
func (et *EscalationTracker) Size() int {
	et.mu.Lock()
	defer et.mu.Unlock()
	return len(et.states)
}

// ExportState exports escalation state for a session as a map of node_id → EscalationEntry.
// Used by J17 TicketManager to serialize state into a session ticket.
func (et *EscalationTracker) ExportState(sessionID string) map[string]EscalationEntry {
	et.mu.Lock()
	defer et.mu.Unlock()

	result := make(map[string]EscalationEntry)
	now := time.Now()

	for key, state := range et.states {
		if key.SessionID != sessionID {
			continue
		}
		// Skip decayed entries
		if now.Sub(state.LastSurfaced) > et.decayDuration {
			continue
		}
		result[key.NodeID] = EscalationEntry{
			NodeID:       key.NodeID,
			Level:        state.Level,
			IgnoreCount:  state.IgnoreCount,
			LastSurfaced: state.LastSurfaced,
		}
	}

	return result
}

// ImportState restores escalation state for a session from a ticket snapshot.
// Used by J17 TicketManager to restore state after a context reset.
func (et *EscalationTracker) ImportState(sessionID string, states map[string]EscalationEntry) {
	et.mu.Lock()
	defer et.mu.Unlock()

	for nodeID, entry := range states {
		key := escalationKey{SessionID: sessionID, NodeID: nodeID}
		et.states[key] = &escalationState{
			Level:        entry.Level,
			IgnoreCount:  entry.IgnoreCount,
			LastSurfaced: entry.LastSurfaced,
		}
	}
}
