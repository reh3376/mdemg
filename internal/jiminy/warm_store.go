package jiminy

import (
	"sync"
	"time"
)

// WarmStore is a per-space in-memory store for pre-computed guidance.
// The warm/latest pattern decouples Guide() computation from the prompt hook:
// Guide() runs asynchronously between prompts, and the hook reads pre-computed results instantly.
type WarmStore struct {
	mu      sync.RWMutex
	entries map[string]*WarmEntry
}

// WarmEntry holds the pre-computed guidance for a single space.
type WarmEntry struct {
	Response    GuidanceResponse
	ContextHint string
	ComputedAt  time.Time
	ComputeMs   int64
}

// NewWarmStore creates a new WarmStore.
func NewWarmStore() *WarmStore {
	return &WarmStore{
		entries: make(map[string]*WarmEntry),
	}
}

// Put stores a pre-computed guidance response for a space.
func (ws *WarmStore) Put(spaceID string, resp GuidanceResponse, contextHint string, computeMs int64) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.entries[spaceID] = &WarmEntry{
		Response:    resp,
		ContextHint: contextHint,
		ComputedAt:  time.Now(),
		ComputeMs:   computeMs,
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
	defer ws.mu.Unlock()
	delete(ws.entries, spaceID)
}
