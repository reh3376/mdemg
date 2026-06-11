package retrieval

import (
	"encoding/json"
	"log/slog"
	"time"
)

// ConflictType represents the type of conflict detected during operations.
type ConflictType string

const (
	// ConflictVersionMismatch indicates a version conflict where expected != actual.
	ConflictVersionMismatch ConflictType = "version_mismatch"

	// ConflictStaleUpdate indicates an update to data that hasn't been touched recently.
	ConflictStaleUpdate ConflictType = "stale_update"

	// ConflictConcurrentWrite indicates a concurrent write was detected.
	ConflictConcurrentWrite ConflictType = "concurrent_write"

	// ConflictOrphanedNode indicates a node was found orphaned from its parent.
	ConflictOrphanedNode ConflictType = "orphaned_node"
)

// ConflictResolution describes how a conflict was resolved.
type ConflictResolution string

const (
	// ResolutionRetrySucceeded indicates the operation succeeded after retry.
	ResolutionRetrySucceeded ConflictResolution = "retry_succeeded"

	// ResolutionLatestWins indicates the latest write was kept.
	ResolutionLatestWins ConflictResolution = "latest_wins"

	// ResolutionManualRequired indicates manual intervention is needed.
	ResolutionManualRequired ConflictResolution = "manual_required"

	// ResolutionSkipped indicates the operation was skipped.
	ResolutionSkipped ConflictResolution = "skipped"

	// ResolutionUnresolved indicates the conflict was not resolved.
	ResolutionUnresolved ConflictResolution = "unresolved"
)

// ConflictEvent represents a detected conflict during ingest or update operations.
type ConflictEvent struct {
	Type        ConflictType       `json:"type"`
	SpaceID     string             `json:"space_id"`
	NodeID      string             `json:"node_id,omitempty"`
	Path        string             `json:"path,omitempty"`
	ExpectedVer int64              `json:"expected_version,omitempty"`
	ActualVer   int64              `json:"actual_version,omitempty"`
	Operation   string             `json:"operation"`
	Resolved    bool               `json:"resolved"`
	Resolution  ConflictResolution `json:"resolution,omitempty"`
	Details     string             `json:"details,omitempty"`
	Timestamp   time.Time          `json:"timestamp"`
}

// conflictLogEnabled controls whether conflict logging is active.
// This can be toggled via configuration.
var conflictLogEnabled = true

// SetConflictLogEnabled enables or disables conflict logging.
func SetConflictLogEnabled(enabled bool) {
	conflictLogEnabled = enabled
}

// LogConflict logs a conflict event with structured logging.
// The event is logged as JSON for easy parsing by log aggregation tools.
func LogConflict(evt ConflictEvent) {
	if !conflictLogEnabled {
		return
	}

	// Ensure timestamp is set
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}

	// Marshal to JSON for structured logging
	data, err := json.Marshal(evt)
	if err != nil {
		slog.Error("conflict: error marshaling event", "error", err)
		return
	}

	slog.Info("conflict: event", "data", string(data))
}

// LogVersionMismatch is a convenience function for version mismatch conflicts.
func LogVersionMismatch(spaceID, nodeID, operation string, expected, actual int64) {
	LogConflict(ConflictEvent{
		Type:        ConflictVersionMismatch,
		SpaceID:     spaceID,
		NodeID:      nodeID,
		ExpectedVer: expected,
		ActualVer:   actual,
		Operation:   operation,
		Resolved:    false,
		Resolution:  ResolutionUnresolved,
	})
}

// LogStaleUpdate is a convenience function for stale update conflicts.
func LogStaleUpdate(spaceID, nodeID, path, operation string) {
	LogConflict(ConflictEvent{
		Type:       ConflictStaleUpdate,
		SpaceID:    spaceID,
		NodeID:     nodeID,
		Path:       path,
		Operation:  operation,
		Resolved:   false,
		Resolution: ResolutionUnresolved,
	})
}

// LogConcurrentWrite is a convenience function for concurrent write conflicts.
func LogConcurrentWrite(spaceID, nodeID, operation string, details string) {
	LogConflict(ConflictEvent{
		Type:       ConflictConcurrentWrite,
		SpaceID:    spaceID,
		NodeID:     nodeID,
		Operation:  operation,
		Details:    details,
		Resolved:   false,
		Resolution: ResolutionUnresolved,
	})
}

// LogConflictResolved logs a conflict that has been resolved.
func LogConflictResolved(evt ConflictEvent, resolution ConflictResolution) {
	evt.Resolved = true
	evt.Resolution = resolution
	LogConflict(evt)
}

// ConflictStats tracks conflict statistics for monitoring.
type ConflictStats struct {
	TotalConflicts      int64            `json:"total_conflicts"`
	ConflictsByType     map[string]int64 `json:"conflicts_by_type"`
	ResolvedConflicts   int64            `json:"resolved_conflicts"`
	UnresolvedConflicts int64            `json:"unresolved_conflicts"`
	LastConflictTime    time.Time        `json:"last_conflict_time,omitempty"`
}

// ConflictTracker provides in-memory tracking of conflict statistics.
// For production use, consider persisting these to a database.
type ConflictTracker struct {
	stats ConflictStats
}

// NewConflictTracker creates a new conflict tracker.
func NewConflictTracker() *ConflictTracker {
	return &ConflictTracker{
		stats: ConflictStats{
			ConflictsByType: make(map[string]int64),
		},
	}
}

// Track records a conflict event in the tracker.
func (ct *ConflictTracker) Track(evt ConflictEvent) {
	ct.stats.TotalConflicts++
	ct.stats.ConflictsByType[string(evt.Type)]++
	ct.stats.LastConflictTime = evt.Timestamp

	if evt.Resolved {
		ct.stats.ResolvedConflicts++
	} else {
		ct.stats.UnresolvedConflicts++
	}
}

// GetStats returns the current conflict statistics.
func (ct *ConflictTracker) GetStats() ConflictStats {
	return ct.stats
}

// Reset clears all tracked statistics.
func (ct *ConflictTracker) Reset() {
	ct.stats = ConflictStats{
		ConflictsByType: make(map[string]int64),
	}
}
