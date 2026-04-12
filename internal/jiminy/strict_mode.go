package jiminy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// StrictModeManager manages per-session strict mode state.
// State is persisted to a file so hooks can check strict mode without HTTP calls.
type StrictModeManager struct {
	mu            sync.RWMutex
	sessions      map[string]bool
	stateFilePath string
}

// strictModeFileEntry is the JSON format of the state file.
type strictModeFileEntry struct {
	SessionID string `json:"session_id"`
	Enabled   bool   `json:"enabled"`
	Timestamp int64  `json:"ts"`
}

// NewStrictModeManager creates a new strict mode manager.
func NewStrictModeManager(stateFilePath string) *StrictModeManager {
	return &StrictModeManager{
		sessions:      make(map[string]bool),
		stateFilePath: stateFilePath,
	}
}

// Enable activates strict mode for a session.
func (m *StrictModeManager) Enable(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[sessionID] = true
	return m.writeFile(sessionID, true)
}

// Disable deactivates strict mode for a session.
func (m *StrictModeManager) Disable(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
	// Remove the state file so hooks see strict mode as off
	if err := os.Remove(m.stateFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("strict mode: remove state file: %w", err)
	}
	return nil
}

// IsStrict returns whether strict mode is active for a session.
func (m *StrictModeManager) IsStrict(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionID]
}

// LoadFromFile reads the state file on startup to restore strict mode.
func (m *StrictModeManager) LoadFromFile() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no state file — strict mode off
		}
		return fmt.Errorf("strict mode: read state file: %w", err)
	}

	var entry strictModeFileEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		slog.Warn("strict mode: invalid state file, ignoring", "error", err)
		return nil
	}

	if entry.Enabled && entry.SessionID != "" {
		m.sessions[entry.SessionID] = true
		slog.Info("strict mode: restored from file", "session_id", entry.SessionID)
	}
	return nil
}

// writeFile persists strict mode state to disk for hook consumption.
func (m *StrictModeManager) writeFile(sessionID string, enabled bool) error {
	entry := strictModeFileEntry{
		SessionID: sessionID,
		Enabled:   enabled,
		Timestamp: time.Now().Unix(),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.stateFilePath, data, 0600); err != nil {
		return fmt.Errorf("strict mode: write state file: %w", err)
	}
	return nil
}
