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

	initial            float64
	boostPerFollow     float64
	decayPerIgnore     float64
	decayPerContradict float64
	highThreshold      float64
	lowThreshold       float64
	ttl                time.Duration

	// JIMINY-EFFECTIVENESS-001: trust update rule.
	mode     string  // "ema" (default, recoverable) | "ratchet" (legacy monotonic)
	emaAlpha float64 // EMA smoothing factor in (0,1]

	onDirty func(string) // called after score changes with sessionID
}

// JIMINY-EFFECTIVENESS-001: per-outcome effectiveness anchors for the EMA trust
// mode. These define the metric (like reward-function anchors), not tunable
// thresholds: trust converges toward the recent regime's anchor. A steady
// all-Ignored regime → ~0.2 (well below the 0.75 T1 threshold); a following
// regime → toward 1.0 (crosses 0.75). The α knob controls how fast.
const (
	trustTargetFollowed     = 1.0
	trustTargetPartial      = 0.6
	trustTargetIgnored      = 0.2
	trustTargetContradicted = 0.0
)

// trustTargetFor maps an outcome to its EMA effectiveness anchor. ok=false for
// outcomes that carry no effectiveness signal (Unknown / NotApplicable —
// topically-unrelated or unclassifiable), which must not move trust.
func trustTargetFor(outcome GuidanceOutcome) (float64, bool) {
	switch outcome {
	case OutcomeFollowed:
		return trustTargetFollowed, true
	case OutcomePartialCompliance:
		return trustTargetPartial, true
	case OutcomeIgnored:
		return trustTargetIgnored, true
	case OutcomeContradicted:
		return trustTargetContradicted, true
	default:
		return 0, false
	}
}

type trustEntry struct {
	Score      float64
	LastUpdate time.Time
}

// TrustConfig holds trust scorer configuration.
type TrustConfig struct {
	Initial            float64       // starting trust score (default: 0.65)
	BoostPerFollow     float64       // trust increase per followed constraint (default: 0.05)
	DecayPerIgnore     float64       // trust decrease per ignored constraint (default: 0.02)
	DecayPerContradict float64       // trust decrease per contradicted constraint (default: 0.04)
	HighThreshold      float64       // above this → agent has earned dense encoding (default: 0.75)
	LowThreshold       float64       // below this → agent needs more explanation (default: 0.35)
	TTL                time.Duration // trust entry expiry (default: 4h)
	// JIMINY-EFFECTIVENESS-001
	Mode     string  // "ema" (default) | "ratchet" (legacy monotonic boost/decay)
	EMAAlpha float64 // EMA smoothing factor, (0,1] (default: 0.1)
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

	// JIMINY-EFFECTIVENESS-001 defaults.
	if cfg.Mode != "ratchet" {
		cfg.Mode = "ema" // recoverable trust is the default
	}
	if cfg.EMAAlpha <= 0 || cfg.EMAAlpha > 1 {
		cfg.EMAAlpha = 0.1
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
		mode:               cfg.Mode,
		emaAlpha:           cfg.EMAAlpha,
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

	if ts.mode == "ema" {
		// JIMINY-EFFECTIVENESS-001: EMA toward the outcome's effectiveness anchor.
		// trust ← trust + α·(target − trust). Unlike the legacy ratchet, this
		// RECOVERS: a session floored by past ignores climbs back toward 1.0 once
		// guidance is followed, so trust tracks RECENT effectiveness (the J17 T1
		// unblocker). A steady-ignored regime converges to ~0.2 (< 0.75), so a
		// genuinely-ineffective session is still correctly kept out of T1.
		if target, ok := trustTargetFor(outcome); ok {
			entry.Score += ts.emaAlpha * (target - entry.Score)
		}
	} else {
		// Legacy monotonic ratchet (JIMINY_TRUST_MODE=ratchet).
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

// SetScore sets the trust score for a session (used for ticket restore and
// live operator overrides — real activity, so stamping now() is correct).
// Do NOT use for startup hydration: it destroys timestamp provenance (the TTL
// key) — use RestoreEntry there (DASHBOARD-TRUTH-001 Epic 4).
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

// RestoreEntry rehydrates a persisted trust entry PRESERVING its timestamp
// provenance (DASHBOARD-TRUTH-001 Epic 4). Unlike SetScore it does NOT stamp
// time.Now() — stamping boot-time on load made every stale session look
// freshly active, so the TTL cleanup (keyed on LastUpdate) could never expire
// it. It also does NOT fire onDirty: hydration is a read-back, not a change —
// marking hydrated sessions dirty caused the next flush to rewrite the
// persisted last_update with now(), rotting the stored provenance every boot.
// A zero lastUpdate (no provenance at all) is preserved as zero, which makes
// the entry immediately TTL-expired — never fall back to now().
func (ts *TrustScorer) RestoreEntry(sessionID string, score float64, lastUpdate time.Time) {
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.scores[sessionID] = &trustEntry{
		Score:      score,
		LastUpdate: lastUpdate,
	}
}

// GetEntry returns the raw score + LastUpdate for a session, without TTL
// filtering or initial-score fallback. Used by the persistence flush so the
// stored last_update reflects the entry's real provenance, not flush time.
func (ts *TrustScorer) GetEntry(sessionID string) (score float64, lastUpdate time.Time, ok bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	entry, ok := ts.scores[sessionID]
	if !ok {
		return 0, time.Time{}, false
	}
	return entry.Score, entry.LastUpdate, true
}

// TTL returns the configured trust-entry expiry window.
func (ts *TrustScorer) TTL() time.Duration {
	return ts.ttl
}

// Initial returns the initial trust score assigned to new sessions.
func (ts *TrustScorer) Initial() float64 {
	return ts.initial
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
	return ts.AggregatesFiltered(nil)
}

// AggregatesFiltered returns trust aggregates over SIGNIFICANT LIVE sessions
// only (DASHBOARD-TRUTH-001 Epic 4): sessions within TTL of their last trust
// update (recency — always applied) AND accepted by the optional significant
// filter (typically "feedback_count ≥ J17_TRUST_MIN_FEEDBACK_COUNT"). A nil
// filter applies recency only. This keeps one-shot / stale test sessions from
// polluting the min/avg/max/count gauges.
func (ts *TrustScorer) AggregatesFiltered(significant func(sessionID string) bool) (avg, min, max float64, count int) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	now := time.Now()
	min = 1.0
	var sum float64
	for id, entry := range ts.scores {
		if now.Sub(entry.LastUpdate) > ts.ttl {
			continue
		}
		if significant != nil && !significant(id) {
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
