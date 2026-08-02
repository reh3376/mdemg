package jiminy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WarmStore is a per-space store for pre-computed guidance.
// The warm/latest pattern decouples Guide() computation from the prompt hook:
// Guide() runs asynchronously between prompts, and the hook reads pre-computed results instantly.
//
// JIMINY-TRACKER-TTL-001 (2026-08-02): the store now DISK-PERSISTS entries so
// a server restart doesn't wipe the guidance state that hook-side .jiminy-
// guidance-state files are still pointing at. Post-restart, the FIRST
// /v1/jiminy/latest call reloads the entry into the EffectivenessTracker via
// RefreshTrackedGuidance (already wired at handler line ~421). Without this,
// every restart caused hooks to POST feedbacks for guidance_ids the tracker
// no longer knew, silently dropping the outcome signal (129 fires / 12
// distinct guidance_ids / 1 log-window on mdemg-dev at ship time).
type WarmStore struct {
	mu      sync.RWMutex
	entries map[string]*WarmEntry

	// persistDir is where per-space JSON files live. Empty string disables
	// disk persistence (in-memory-only fallback for tests + spaces that
	// can't allocate a home dir).
	persistDir string
}

// WarmEntry holds the pre-computed guidance for a single space.
type WarmEntry struct {
	Response    GuidanceResponse `json:"response"`
	ContextHint string           `json:"context_hint"`
	ComputedAt  time.Time        `json:"computed_at"`
	ComputeMs   int64            `json:"compute_ms"`
}

// NewWarmStore creates a new in-memory-only WarmStore. Prefer NewWarmStoreWithPersistence
// for production; this constructor is retained for tests.
func NewWarmStore() *WarmStore {
	return &WarmStore{
		entries: make(map[string]*WarmEntry),
	}
}

// NewWarmStoreWithPersistence creates a WarmStore that persists per-space entries
// as JSON files under persistDir. On construction, the store hydrates from disk —
// so a fresh process picks up the last-persisted guidance for every space.
// If persistDir is empty or unwritable, the store falls back to in-memory-only
// (WARN logged) and returns nil error so startup never fails on this path.
func NewWarmStoreWithPersistence(persistDir string) *WarmStore {
	ws := &WarmStore{
		entries:    make(map[string]*WarmEntry),
		persistDir: persistDir,
	}
	if persistDir == "" {
		return ws
	}
	if err := os.MkdirAll(persistDir, 0o755); err != nil {
		slog.Warn("warm store: persist dir unwritable, falling back to memory-only", "dir", persistDir, "error", err)
		ws.persistDir = ""
		return ws
	}
	ws.hydrateFromDisk()
	return ws
}

// Put stores a pre-computed guidance response for a space AND (if persistence
// enabled) writes it atomically to disk.
func (ws *WarmStore) Put(spaceID string, resp GuidanceResponse, contextHint string, computeMs int64) {
	entry := &WarmEntry{
		Response:    resp,
		ContextHint: contextHint,
		ComputedAt:  time.Now(),
		ComputeMs:   computeMs,
	}
	ws.mu.Lock()
	ws.entries[spaceID] = entry
	persistDir := ws.persistDir
	ws.mu.Unlock()

	if persistDir != "" {
		if err := writeWarmEntry(persistDir, spaceID, entry); err != nil {
			slog.Warn("warm store: persist failed (in-memory OK)", "space_id", spaceID, "error", err)
		}
	}
}

// Get returns the pre-computed guidance for a space, or nil if none exists.
func (ws *WarmStore) Get(spaceID string) *WarmEntry {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.entries[spaceID]
}

// Age returns the age of the stored guidance in milliseconds, or -1 if no entry exists.
func (ws *WarmStore) Age(spaceID string) int64 {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	entry := ws.entries[spaceID]
	if entry == nil {
		return -1
	}
	return time.Since(entry.ComputedAt).Milliseconds()
}

// Invalidate removes the cached guidance for a space, forcing a fresh computation
// on the next /latest read. Used when trust crosses a tier threshold.
func (ws *WarmStore) Invalidate(spaceID string) {
	ws.mu.Lock()
	delete(ws.entries, spaceID)
	persistDir := ws.persistDir
	ws.mu.Unlock()
	if persistDir != "" {
		_ = os.Remove(warmEntryPath(persistDir, spaceID))
	}
}

// hydrateFromDisk loads every persisted warm entry under persistDir into the
// in-memory map. Silently skips malformed files (WARN log). Called once from
// NewWarmStoreWithPersistence.
func (ws *WarmStore) hydrateFromDisk() {
	pattern := filepath.Join(ws.persistDir, "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		slog.Warn("warm store: glob failed on hydrate", "error", err)
		return
	}
	loaded := 0
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("warm store: hydrate read failed", "path", path, "error", err)
			continue
		}
		var wrapper struct {
			SpaceID string     `json:"space_id"`
			Entry   *WarmEntry `json:"entry"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			slog.Warn("warm store: hydrate parse failed", "path", path, "error", err)
			continue
		}
		if wrapper.SpaceID == "" || wrapper.Entry == nil {
			continue
		}
		ws.entries[wrapper.SpaceID] = wrapper.Entry
		loaded++
	}
	if loaded > 0 {
		slog.Info("warm store: hydrated from disk", "entries", loaded, "dir", ws.persistDir)
	}
}

// writeWarmEntry writes the entry atomically (tmp + rename) so a crashed
// half-write can't corrupt a subsequent hydrate.
func writeWarmEntry(dir, spaceID string, entry *WarmEntry) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	wrapper := struct {
		SpaceID string     `json:"space_id"`
		Entry   *WarmEntry `json:"entry"`
	}{SpaceID: spaceID, Entry: entry}
	data, err := json.Marshal(wrapper)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	finalPath := warmEntryPath(dir, spaceID)
	tmpPath := finalPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		// best-effort cleanup
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// warmEntryPath returns the disk path for a space's warm entry.
// space_ids are opaque tokens that can contain shell-hostile characters —
// we sanitize by replacing filesystem-hostile bytes rather than trusting them.
func warmEntryPath(dir, spaceID string) string {
	safe := make([]byte, 0, len(spaceID))
	for i := 0; i < len(spaceID); i++ {
		b := spaceID[i]
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b == '-', b == '_', b == '.':
			safe = append(safe, b)
		default:
			safe = append(safe, '_')
		}
	}
	return filepath.Join(dir, string(safe)+".json")
}

// FileExists is a small helper exported for tests.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return err == nil
}
