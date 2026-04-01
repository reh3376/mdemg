package jiminy

import (
	"sync"
	"time"
)

// TrustScorer manages per-session trust scores (0.0 - 1.0).
// Trust modulates encoding tier: high trust → denser encoding, low trust → more explanation.
type TrustScorer struct {
	mu     sync.RWMutex
	scores map[string]*trustEntry

	initial           float64
	boostPerFollow    float64
	decayPerIgnore    float64
	decayPerContradict float64
	highThreshold     float64
	lowThreshold      float64
	ttl               time.Duration

	onDirty func(string) // called after score changes with sessionID
}

type trustEntry struct {
	Score      float64
	LastUpdate time.Time
}

// TrustConfig holds trust scorer configuration.
type TrustConfig struct {
	Initial           float64       // starting trust score (default: 0.65)
	BoostPerFollow    float64       // trust increase per followed constraint (default: 0.05)
	DecayPerIgnore    float64       // trust decrease per ignored constraint (default: 0.02)
	DecayPerContradict float64      // trust decrease per contradicted constraint (default: 0.04)
	HighThreshold     float64       // above this → agent has earned dense encoding (default: 0.75)
	LowThreshold      float64       // below this → agent needs more explanation (default: 0.35)
	TTL               time.Duration // trust entry expiry (default: 4h)
}

// NewTrustScorer creates a new trust scorer with the given config.
func NewTrustScorer(cfg TrustConfig) *TrustScorer {
	if cfg.Initial <= 0 {
		cfg.Initial = 0.65
	}
	if cfg.BoostPerFollow <= 0 {
		cfg.BoostPerFollow = 0.05
	}
	if cfg.DecayPerIgnore <= 0 {
		cfg.DecayPerIgnore = 0.02
	}
	if cfg.DecayPerContradict <= 0 {
		cfg.DecayPerContradict = 0.04
	}
	if cfg.HighThreshold <= 0 {
		cfg.HighThreshold = 0.75
	}
	if cfg.LowThreshold <= 0 {
		cfg.LowThreshold = 0.35
	}

	ttl := 168 * time.Hour // 7 days — trust persisted to Neo4j, TTL is cleanup threshold
	if cfg.TTL > 0 {
		ttl = cfg.TTL
	}

	return &TrustScorer{
		scores:             make(map[string]*trustEntry),
		initial:            cfg.Initial,
		boostPerFollow:     cfg.BoostPerFollow,
		decayPerIgnore:     cfg.DecayPerIgnore,
		decayPerContradict: cfg.DecayPerContradict,
		highThreshold:      cfg.HighThreshold,
		lowThreshold:       cfg.LowThreshold,
		ttl:                ttl,
	}
}

// SetOnDirty sets a callback invoked after any trust score change.
// The callback receives the sessionID whose score was updated.
func (ts *TrustScorer) SetOnDirty(fn func(string)) {
	ts.onDirty = fn
}

// GetAllEntries returns a snapshot of all non-expired trust entries for persistence.
func (ts *TrustScorer) GetAllEntries() map[string]trustEntry {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	result := make(map[string]trustEntry, len(ts.scores))
	for id, entry := range ts.scores {
		if time.Since(entry.LastUpdate) <= ts.ttl {
			result[id] = *entry
		}
	}
	return result
}

// GetScore returns the current trust score for a session.
// Returns the initial trust score if no entry exists.
func (ts *TrustScorer) GetScore(sessionID string) float64 {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	entry, ok := ts.scores[sessionID]
	if !ok || time.Since(entry.LastUpdate) > ts.ttl {
		return ts.initial
	}
	return entry.Score
}

// RecordOutcome updates the trust score based on an outcome.
func (ts *TrustScorer) RecordOutcome(sessionID string, outcome GuidanceOutcome) float64 {
	ts.mu.Lock()

	entry, ok := ts.scores[sessionID]
	if !ok || time.Since(entry.LastUpdate) > ts.ttl {
		entry = &trustEntry{Score: ts.initial, LastUpdate: time.Now()}
		ts.scores[sessionID] = entry
	}

	switch outcome {
	case OutcomeFollowed:
		entry.Score += ts.boostPerFollow
	case OutcomeIgnored:
		entry.Score -= ts.decayPerIgnore
	case OutcomeContradicted:
		entry.Score -= ts.decayPerContradict
	case OutcomePartialCompliance:
		entry.Score += ts.boostPerFollow * 0.5
	}

	// Clamp to [0.0, 1.0]
	if entry.Score > 1.0 {
		entry.Score = 1.0
	}
	if entry.Score < 0.0 {
		entry.Score = 0.0
	}
	entry.LastUpdate = time.Now()

	score := entry.Score
	ts.mu.Unlock()

	if ts.onDirty != nil {
		ts.onDirty(sessionID)
	}

	return score
}

// SetScore sets the trust score for a session (used for ticket restore and hydration).
func (ts *TrustScorer) SetScore(sessionID string, score float64) {
	ts.mu.Lock()

	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	ts.scores[sessionID] = &trustEntry{
		Score:      score,
		LastUpdate: time.Now(),
	}
	ts.mu.Unlock()

	if ts.onDirty != nil {
		ts.onDirty(sessionID)
	}
}

// HighThreshold returns the configured high trust threshold.
func (ts *TrustScorer) HighThreshold() float64 {
	return ts.highThreshold
}

// LowThreshold returns the configured low trust threshold.
func (ts *TrustScorer) LowThreshold() float64 {
	return ts.lowThreshold
}

// SetThresholds updates the high and low trust thresholds.
// Validates: high in (0, 1], low in [0, 1), low < high.
func (ts *TrustScorer) SetThresholds(high, low float64) {
	if high <= 0 || high > 1.0 {
		return
	}
	if low < 0 || low >= 1.0 {
		return
	}
	if low >= high {
		return
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.highThreshold = high
	ts.lowThreshold = low
}

// Aggregates returns trust score statistics across all active (non-expired) sessions.
// Returns avg, min, max scores and session count. If no sessions exist, returns
// the initial trust score for avg/min/max with count=0.
func (ts *TrustScorer) Aggregates() (avg, min, max float64, count int) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	now := time.Now()
	min = 1.0
	var sum float64
	for _, entry := range ts.scores {
		if now.Sub(entry.LastUpdate) > ts.ttl {
			continue
		}
		count++
		sum += entry.Score
		if entry.Score < min {
			min = entry.Score
		}
		if entry.Score > max {
			max = entry.Score
		}
	}
	if count == 0 {
		return ts.initial, ts.initial, ts.initial, 0
	}
	avg = sum / float64(count)
	return avg, min, max, count
}

// CleanupExpired removes entries older than TTL.
func (ts *TrustScorer) CleanupExpired() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	removed := 0
	now := time.Now()
	for id, entry := range ts.scores {
		if now.Sub(entry.LastUpdate) > ts.ttl {
			delete(ts.scores, id)
			removed++
		}
	}
	return removed
}
