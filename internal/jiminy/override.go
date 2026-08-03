package jiminy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
// concurrent use. Two independent audit sinks:
//   - JSONL file (auditPath) — forensic + portable (works without TSDB)
//   - TSDB constraint_overrides table (tsdbPool) — queryable by RSIC + UI
// Both are best-effort; write failures WARN-log and the override itself
// still succeeds.
type OverrideManager struct {
	mu        sync.RWMutex
	entries   map[overrideKey]*OverrideEntry
	auditPath string
	tsdbPool  *pgxpool.Pool // ENFORCE-OVERRIDES-TSDB (nil-safe when TSDB disabled)
	spaceID   string        // default space stamped on TSDB rows when caller doesn't override
}

// NewOverrideManager constructs a memory + JSONL-only manager.
// auditPath="" disables JSONL audit (tests + memory-only fallback).
func NewOverrideManager(auditPath string) *OverrideManager {
	return &OverrideManager{
		entries:   make(map[overrideKey]*OverrideEntry),
		auditPath: auditPath,
	}
}

// SetTSDB wires the pgxpool + default space for the constraint_overrides
// hypertable audit (ENFORCE-OVERRIDES-TSDB). Called from the server init
// after the pool is up. Nil pool is a no-op (JSONL-only fallback).
func (m *OverrideManager) SetTSDB(pool *pgxpool.Pool, defaultSpaceID string) {
	m.mu.Lock()
	m.tsdbPool = pool
	m.spaceID = defaultSpaceID
	m.mu.Unlock()
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

// audit writes to both sinks (JSONL + TSDB constraint_overrides). Both are
// best-effort — write failures WARN-log but do not fail the caller (audit
// is durable forensic record, not load-bearing for the override itself).
func (m *OverrideManager) audit(op string, entry *OverrideEntry) {
	if entry == nil {
		return
	}
	m.auditTSDB(op, entry)
	if m.auditPath == "" {
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

// auditTSDB inserts one row into constraint_overrides. Silent no-op when
// tsdbPool is nil (test / JSONL-only fallback). Uses a short timeout so the
// hot Apply/Revoke path isn't blocked by a slow database. Best-effort —
// failures WARN-log but caller succeeds.
//
// NO MUTEX: audit() is called from BOTH lock-held paths (Get's lazy purge
// under mu.Lock) and lock-released paths (Apply after mu.Unlock). Taking the
// mutex here would deadlock the lock-held caller. Instead we accept an
// unsynchronised read of tsdbPool + spaceID — SetTSDB is called once at
// startup before any Apply/Get/Revoke, so a data race here is impossible
// in the shipped call order. (If SetTSDB grows a runtime-toggle use case,
// switch to atomic.Value.)
func (m *OverrideManager) auditTSDB(op string, entry *OverrideEntry) {
	pool := m.tsdbPool
	spaceID := m.spaceID
	if pool == nil || entry == nil {
		return
	}
	if spaceID == "" {
		spaceID = "mdemg-dev"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
		INSERT INTO constraint_overrides
		    (time, space_id, session_id, constraint_code, reason, op, applied_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		time.Now(), spaceID, entry.SessionID, entry.ConstraintCode,
		entry.Reason, op, entry.AppliedAt, entry.ExpiresAt)
	if err != nil {
		slog.Warn("override audit: TSDB insert failed", "op", op, "code", entry.ConstraintCode, "error", err)
	}
}
