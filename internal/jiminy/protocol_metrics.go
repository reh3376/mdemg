package jiminy

import (
	"sync"
	"time"
)

// ProtocolMetrics is an immutable snapshot of protocol performance over a window.
type ProtocolMetrics struct {
	TierDistribution         [3]float64         `json:"tier_distribution"`
	AvgTokensPerGuidance     float64            `json:"avg_tokens_per_guidance"`
	CompressionRatio         float64            `json:"compression_ratio"`
	CodeComprehension        map[string]float64 `json:"code_comprehension,omitempty"`
	AvgComprehension         float64            `json:"avg_comprehension"`
	ReplayFrequencyPerHour   float64            `json:"replay_frequency_per_hour"`
	TicketRestoreSuccessRate float64            `json:"ticket_restore_success_rate"`
	CodeCoverage             float64            `json:"code_coverage"`
	T2FrequencyByConstraint  map[string]int     `json:"t2_frequency_by_constraint,omitempty"`
	WindowStart              time.Time          `json:"window_start"`
	WindowEnd                time.Time          `json:"window_end"`
	TotalEvents              int64              `json:"total_events"`
}

// ProtocolMetricsCollector is a thread-safe collector for J17 protocol metrics.
type ProtocolMetricsCollector struct {
	mu sync.RWMutex

	// Tier distribution counters
	tierCounts  [3]int64
	totalEvents int64

	// Token tracking
	totalTokens   int64
	guidanceCount int64

	// NL equivalent tokens (for compression ratio)
	totalNLEquivTokens int64

	// Per-code comprehension tracking
	codeFollowed map[string]int // code → times followed
	codeTotal    map[string]int // code → total occurrences

	// Replay & ticket tracking
	replayEvents      int64
	ticketRestoreOK   int64
	ticketRestoreTotal int64

	// T2 frequency per constraint
	t2Frequency map[string]int

	// Window tracking
	windowStart time.Time
}

// NewProtocolMetricsCollector creates a new metrics collector.
func NewProtocolMetricsCollector() *ProtocolMetricsCollector {
	return &ProtocolMetricsCollector{
		codeFollowed: make(map[string]int),
		codeTotal:    make(map[string]int),
		t2Frequency:  make(map[string]int),
		windowStart:  time.Now(),
	}
}

// RecordGuidance records a guidance event with tier and token information.
func (c *ProtocolMetricsCollector) RecordGuidance(tier int, tokenCount int, codes []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if tier >= 1 && tier <= 3 {
		c.tierCounts[tier-1]++
	}
	c.totalEvents++
	c.totalTokens += int64(tokenCount)
	c.guidanceCount++

	// Estimate NL equivalent (T3 baseline: ~80 tokens per item)
	nlEquiv := tokenCount
	if tier == 1 {
		nlEquiv = tokenCount * 5 // T1 is ~5x compressed
	} else if tier == 2 {
		nlEquiv = tokenCount * 2 // T2 is ~2x compressed
	}
	c.totalNLEquivTokens += int64(nlEquiv)

	// Track T2 frequency per code
	if tier == 2 {
		for _, code := range codes {
			c.t2Frequency[code]++
		}
	}
}

// RecordOutcome records a constraint outcome for comprehension tracking.
func (c *ProtocolMetricsCollector) RecordOutcome(code string, comprehensionScore float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if code == "" {
		return
	}

	c.codeTotal[code]++
	if comprehensionScore >= 0.7 {
		c.codeFollowed[code]++
	}
}

// RecordReplay records event replay events.
func (c *ProtocolMetricsCollector) RecordReplay(eventCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replayEvents += int64(eventCount)
}

// RecordTicketRestore records a ticket restore attempt.
func (c *ProtocolMetricsCollector) RecordTicketRestore(success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ticketRestoreTotal++
	if success {
		c.ticketRestoreOK++
	}
}

// Snapshot returns an immutable copy of the current metrics.
func (c *ProtocolMetricsCollector) Snapshot() *ProtocolMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	windowDuration := now.Sub(c.windowStart)

	// Tier distribution
	var tierDist [3]float64
	if c.totalEvents > 0 {
		for i := range 3 {
			tierDist[i] = float64(c.tierCounts[i]) / float64(c.totalEvents)
		}
	}

	// Average tokens per guidance
	var avgTokens float64
	if c.guidanceCount > 0 {
		avgTokens = float64(c.totalTokens) / float64(c.guidanceCount)
	}

	// Compression ratio
	var compressionRatio float64
	if c.totalTokens > 0 {
		compressionRatio = float64(c.totalNLEquivTokens) / float64(c.totalTokens)
	}

	// Per-code comprehension
	codeComp := make(map[string]float64, len(c.codeTotal))
	var totalComp float64
	var compCount int
	for code, total := range c.codeTotal {
		if total > 0 {
			rate := float64(c.codeFollowed[code]) / float64(total)
			codeComp[code] = rate
			totalComp += rate
			compCount++
		}
	}
	var avgComp float64
	if compCount > 0 {
		avgComp = totalComp / float64(compCount)
	}

	// Replay frequency per hour
	var replayFreq float64
	if windowDuration.Hours() > 0 {
		replayFreq = float64(c.replayEvents) / windowDuration.Hours()
	}

	// Ticket restore success rate
	var ticketRate float64
	if c.ticketRestoreTotal > 0 {
		ticketRate = float64(c.ticketRestoreOK) / float64(c.ticketRestoreTotal)
	}

	// T2 frequency by constraint (copy)
	t2Freq := make(map[string]int, len(c.t2Frequency))
	for k, v := range c.t2Frequency {
		t2Freq[k] = v
	}

	return &ProtocolMetrics{
		TierDistribution:         tierDist,
		AvgTokensPerGuidance:     avgTokens,
		CompressionRatio:         compressionRatio,
		CodeComprehension:        codeComp,
		AvgComprehension:         avgComp,
		ReplayFrequencyPerHour:   replayFreq,
		TicketRestoreSuccessRate: ticketRate,
		T2FrequencyByConstraint:  t2Freq,
		WindowStart:              c.windowStart,
		WindowEnd:                now,
		TotalEvents:              c.totalEvents,
	}
}

// Reset clears the metrics window (called at meso/macro cycle boundary).
func (c *ProtocolMetricsCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tierCounts = [3]int64{}
	c.totalEvents = 0
	c.totalTokens = 0
	c.guidanceCount = 0
	c.totalNLEquivTokens = 0
	c.codeFollowed = make(map[string]int)
	c.codeTotal = make(map[string]int)
	c.replayEvents = 0
	c.ticketRestoreOK = 0
	c.ticketRestoreTotal = 0
	c.t2Frequency = make(map[string]int)
	c.windowStart = time.Now()
}
