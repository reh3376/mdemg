package jiminy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// JIMINY-ENFORCE-003 (2026-08-03): operator escape-hatch for the enforcement arc.
//
// When a WARNED+ escalation causes /strict to deny an action the operator
// judges as a false-positive (the substrate's classifier was wrong about
// this specific action), the operator can install a time-boxed override:
//
//   mdemg jiminy override apply --constraint <code> --reason <text> --duration 15m
//
// The classifier consults the OverrideManager on each Classify() call. If the
// violated constraint_code has an active (non-expired) override for the
// session, the deny verdict is downgraded to pass with the override reason
// recorded in DenialReason (so the audit trail still names WHY the block
// would have fired).
//
// Overrides are:
//   - Session-scoped: an override for session S doesn't affect session T
//   - Time-boxed: every override has an expires_at; expired entries are lazily
//     purged on next Get()/List() call
//   - JSONL-audited: every apply + revoke + auto-expire writes a line to
//     ~/.mdemg/jiminy-overrides.jsonl for forensic + retrospective use
//   - Reversible via `mdemg jiminy override revoke <code>` before expiry
//
// The TSDB audit table is DEFERRED to JIMINY-ENFORCE-004 (RSIC enforcement-
// learning) which will consume overrides as `blocked_false_positive` outcome
// signal. Until then the JSONL trail is the durable record.

// OverrideEntry represents a single active override.
type OverrideEntry struct {
	SessionID      string    `json:"session_id"`
	ConstraintCode string    `json:"constraint_code"`
	Reason         string    `json:"reason"`
	AppliedAt      time.Time `json:"applied_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// IsExpired reports whether the override has aged past expiry.
func (o *OverrideEntry) IsExpired() bool {
	return time.Now().After(o.ExpiresAt)
}

// overrideKey is the composite key: (session, constraint code).
type overrideKey struct {
	SessionID      string
	ConstraintCode string
}

// OverrideManager holds per-session per-constraint overrides. Safe for
// concurrent use. Optional JSONL audit path — empty disables audit.
type OverrideManager struct {
	mu        sync.RWMutex
	entries   map[overrideKey]*OverrideEntry
	auditPath string
}

// NewOverrideManager constructs a manager. auditPath="" disables audit
// (tests + memory-only fallback). Non-empty path — the enclosing directory
// is MkdirAll'd; any write failure is WARN-logged and the operation still
// succeeds (audit is best-effort, not load-bearing for the override itself).
func NewOverrideManager(auditPath string) *OverrideManager {
	return &OverrideManager{
		entries:   make(map[overrideKey]*OverrideEntry),
		auditPath: auditPath,
	}
}

// Apply installs an override. Returns the resulting entry (with applied_at
// + expires_at stamped) or an error on validation failure. duration<=0 is
// rejected — every override MUST have a bounded expiry so a forgotten
// override can't silently disable enforcement forever.
func (m *OverrideManager) Apply(sessionID, constraintCode, reason string, duration time.Duration) (*OverrideEntry, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("override: session_id required")
	}
	if constraintCode == "" {
		return nil, fmt.Errorf("override: constraint_code required")
	}
	if reason == "" {
		return nil, fmt.Errorf("override: reason required (audit trail depends on it)")
	}
	if duration <= 0 {
		return nil, fmt.Errorf("override: duration must be positive (received %s) — forgotten overrides silently disable enforcement forever if unbounded", duration)
	}
	now := time.Now()
	entry := &OverrideEntry{
		SessionID:      sessionID,
		ConstraintCode: constraintCode,
		Reason:         reason,
		AppliedAt:      now,
		ExpiresAt:      now.Add(duration),
	}
	m.mu.Lock()
	m.entries[overrideKey{sessionID, constraintCode}] = entry
	m.mu.Unlock()
	m.audit("apply", entry)
	return entry, nil
}

// Get returns the active override for (session, code) or nil if none exists
// or the entry is expired. Expired entries are lazily removed.
func (m *OverrideManager) Get(sessionID, constraintCode string) *OverrideEntry {
	m.mu.RLock()
	entry, ok := m.entries[overrideKey{sessionID, constraintCode}]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	if entry.IsExpired() {
		m.mu.Lock()
		// Re-check under write lock (another goroutine may have already removed)
		if e2, ok := m.entries[overrideKey{sessionID, constraintCode}]; ok && e2.IsExpired() {
			delete(m.entries, overrideKey{sessionID, constraintCode})
			m.audit("expire", e2)
		}
		m.mu.Unlock()
		return nil
	}
	return entry
}

// List returns all active (non-expired) overrides. Expired entries are
// lazily removed. Filters to sessionID when non-empty.
func (m *OverrideManager) List(sessionID string) []*OverrideEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*OverrideEntry
	for k, entry := range m.entries {
		if entry.IsExpired() {
			delete(m.entries, k)
			m.audit("expire", entry)
			continue
		}
		if sessionID != "" && entry.SessionID != sessionID {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// Revoke removes an override before its scheduled expiry. Returns the
// revoked entry or nil if no active entry exists.
func (m *OverrideManager) Revoke(sessionID, constraintCode string) *OverrideEntry {
	m.mu.Lock()
	key := overrideKey{sessionID, constraintCode}
	entry, ok := m.entries[key]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.entries, key)
	m.mu.Unlock()
	m.audit("revoke", entry)
	return entry
}

// audit writes a single JSONL line to the audit file. Best-effort; a write
// failure is WARN-logged but does not fail the caller (audit is durable
// forensic record, not load-bearing for the override itself).
func (m *OverrideManager) audit(op string, entry *OverrideEntry) {
	if m.auditPath == "" || entry == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.auditPath), 0o755); err != nil {
		slog.Warn("override audit: mkdir failed", "path", m.auditPath, "error", err)
		return
	}
	record := map[string]any{
		"op":              op,
		"session_id":      entry.SessionID,
		"constraint_code": entry.ConstraintCode,
		"reason":          entry.Reason,
		"applied_at":      entry.AppliedAt.UTC().Format(time.RFC3339),
		"expires_at":      entry.ExpiresAt.UTC().Format(time.RFC3339),
		"logged_at":       time.Now().UTC().Format(time.RFC3339),
	}
	line, err := json.Marshal(record)
	if err != nil {
		slog.Warn("override audit: marshal failed", "error", err)
		return
	}
	f, err := os.OpenFile(m.auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		slog.Warn("override audit: open failed", "path", m.auditPath, "error", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		slog.Warn("override audit: write failed", "path", m.auditPath, "error", err)
	}
}
