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
	Sidecar                  *SidecarMetrics    `json:"sidecar_metrics,omitempty"` // NS-07

	// Per-tier comprehension tracking (NLI feedback loop)
	TierComprehension     [3]float64                `json:"tier_comprehension"`
	TierCodeComprehension map[int]map[string]float64 `json:"tier_code_comprehension,omitempty"`
	TierOutcomeCount      [3]int64                   `json:"tier_outcome_count"`

	// NLI fallback tracking (degraded-state awareness)
	NLIFallbackCount int64              `json:"nli_fallback_count,omitempty"`
	NLIFallbackRate  float64            `json:"nli_fallback_rate,omitempty"`
	CodeOutcomeCount map[string]int64   `json:"code_outcome_count,omitempty"`

	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	TotalEvents int64     `json:"total_events"`
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

	// Code coverage tracking (Gap 1)
	constraintTotal    int64
	constraintWithCode int64

	// NS-07: Sidecar telemetry
	sidecarRequests   int64
	sidecarErrors     int64
	sidecarTimeouts   int64
	sidecarAgreements int64
	sidecarOverrides  int64
	sidecarLatencySum int64 // nanoseconds

	// Per-tier comprehension tracking (NLI feedback loop)
	tierCompScoreSum [3]float64            // running sum of comprehension per tier
	tierCompCount    [3]int64              // count of outcomes per tier
	tierCodeScores   [3]map[string][]float64 // per-tier, per-code scores

	// NLI fallback tracking (degraded-state awareness)
	nliFallbackCount int64

	// Window tracking
	windowStart time.Time
}

// NewProtocolMetricsCollector creates a new metrics collector.
func NewProtocolMetricsCollector() *ProtocolMetricsCollector {
	return &ProtocolMetricsCollector{
		codeFollowed: make(map[string]int),
		codeTotal:    make(map[string]int),
		t2Frequency:  make(map[string]int),
		tierCodeScores: [3]map[string][]float64{
			make(map[string][]float64),
			make(map[string][]float64),
			make(map[string][]float64),
		},
		windowStart: time.Now(),
	}
}

// SidecarMetrics is an immutable snapshot of sidecar telemetry (NS-07).
type SidecarMetrics struct {
	Requests      int64   `json:"requests"`
	Errors        int64   `json:"errors"`
	Timeouts      int64   `json:"timeouts"`
	AgreementRate float64 `json:"agreement_rate"`
	OverrideRate  float64 `json:"override_rate"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}

// RecordSidecarCall records a sidecar prediction call for telemetry (NS-07).
// isTimeout should be true when the error was caused by a context deadline or timeout.
func (c *ProtocolMetricsCollector) RecordSidecarCall(latencyMs float64, err error, isTimeout bool, mlTier int, ruleTier int, overridden bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sidecarRequests++
	c.sidecarLatencySum += int64(latencyMs * 1e6) // convert ms to ns

	if err != nil {
		c.sidecarErrors++
		if isTimeout {
			c.sidecarTimeouts++
		}
		return
	}

	if mlTier == ruleTier {
		c.sidecarAgreements++
	}
	if overridden {
		c.sidecarOverrides++
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

// RecordOutcomeWithTier records a constraint outcome with per-tier tracking.
// tier must be 1, 2, or 3; out-of-range tiers are silently ignored for tier-specific
// tracking but the outcome is still recorded via RecordOutcome.
func (c *ProtocolMetricsCollector) RecordOutcomeWithTier(code string, tier int, comprehensionScore float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Record into existing aggregate tracking (same as RecordOutcome, inlined to avoid double-lock)
	if code != "" {
		c.codeTotal[code]++
		if comprehensionScore >= 0.7 {
			c.codeFollowed[code]++
		}
	}

	// Per-tier tracking
	if tier >= 1 && tier <= 3 {
		idx := tier - 1
		c.tierCompScoreSum[idx] += comprehensionScore
		c.tierCompCount[idx]++
		if code != "" {
			c.tierCodeScores[idx][code] = append(c.tierCodeScores[idx][code], comprehensionScore)
		}
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

// RecordConstraintCoverage records the total number of constraint items and how many have codes.
func (c *ProtocolMetricsCollector) RecordConstraintCoverage(total, withCode int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.constraintTotal += int64(total)
	c.constraintWithCode += int64(withCode)
}

// RecordNLIFallback increments the NLI fallback counter (sidecar unavailable).
func (c *ProtocolMetricsCollector) RecordNLIFallback() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nliFallbackCount++
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

	// Per-code comprehension (weighted by sample count)
	codeComp := make(map[string]float64, len(c.codeTotal))
	var totalFollowed, totalSamples int
	for code, total := range c.codeTotal {
		if total > 0 {
			rate := float64(c.codeFollowed[code]) / float64(total)
			codeComp[code] = rate
			totalFollowed += c.codeFollowed[code]
			totalSamples += total
		}
	}
	var avgComp float64
	if totalSamples > 0 {
		avgComp = float64(totalFollowed) / float64(totalSamples)
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

	// Code coverage: fraction of constraint items that have a code assigned
	// When no constraints were surfaced, coverage = 1.0 (nothing to cover)
	var codeCoverage float64
	if c.constraintTotal > 0 {
		codeCoverage = float64(c.constraintWithCode) / float64(c.constraintTotal)
	} else {
		codeCoverage = 1.0
	}

	// T2 frequency by constraint (copy)
	t2Freq := make(map[string]int, len(c.t2Frequency))
	for k, v := range c.t2Frequency {
		t2Freq[k] = v
	}

	// Per-tier comprehension averages
	var tierComp [3]float64
	var tierOutcomeCount [3]int64
	for i := range 3 {
		tierOutcomeCount[i] = c.tierCompCount[i]
		if c.tierCompCount[i] > 0 {
			tierComp[i] = c.tierCompScoreSum[i] / float64(c.tierCompCount[i])
		}
	}

	// Per-tier, per-code comprehension averages
	var tierCodeComp map[int]map[string]float64
	for i := range 3 {
		if len(c.tierCodeScores[i]) > 0 {
			if tierCodeComp == nil {
				tierCodeComp = make(map[int]map[string]float64)
			}
			codeMap := make(map[string]float64, len(c.tierCodeScores[i]))
			for code, scores := range c.tierCodeScores[i] {
				if len(scores) > 0 {
					var sum float64
					for _, s := range scores {
						sum += s
					}
					codeMap[code] = sum / float64(len(scores))
				}
			}
			tierCodeComp[i+1] = codeMap // tier 1-indexed in the map
		}
	}

	// NS-07: Compute sidecar metrics
	var sidecar *SidecarMetrics
	if c.sidecarRequests > 0 {
		var agreementRate, overrideRate, avgLatencyMs float64
		agreementRate = float64(c.sidecarAgreements) / float64(c.sidecarRequests)
		overrideRate = float64(c.sidecarOverrides) / float64(c.sidecarRequests)
		avgLatencyMs = float64(c.sidecarLatencySum) / float64(c.sidecarRequests) / 1e6 // ns to ms
		sidecar = &SidecarMetrics{
			Requests:      c.sidecarRequests,
			Errors:        c.sidecarErrors,
			Timeouts:      c.sidecarTimeouts,
			AgreementRate: agreementRate,
			OverrideRate:  overrideRate,
			AvgLatencyMs:  avgLatencyMs,
		}
	}

	// NLI fallback rate
	var nliFallbackRate float64
	if c.totalEvents > 0 {
		nliFallbackRate = float64(c.nliFallbackCount) / float64(c.totalEvents)
	}

	// Per-code outcome count (for minimum sample guards)
	codeOutcomeCount := make(map[string]int64, len(c.codeTotal))
	for code, total := range c.codeTotal {
		codeOutcomeCount[code] = int64(total)
	}

	return &ProtocolMetrics{
		TierDistribution:         tierDist,
		AvgTokensPerGuidance:     avgTokens,
		CompressionRatio:         compressionRatio,
		CodeComprehension:        codeComp,
		AvgComprehension:         avgComp,
		ReplayFrequencyPerHour:   replayFreq,
		TicketRestoreSuccessRate: ticketRate,
		CodeCoverage:             codeCoverage,
		T2FrequencyByConstraint:  t2Freq,
		Sidecar:                  sidecar,
		TierComprehension:        tierComp,
		TierCodeComprehension:    tierCodeComp,
		TierOutcomeCount:         tierOutcomeCount,
		NLIFallbackCount:         c.nliFallbackCount,
		NLIFallbackRate:          nliFallbackRate,
		CodeOutcomeCount:         codeOutcomeCount,
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
	c.constraintTotal = 0
	c.constraintWithCode = 0
	c.t2Frequency = make(map[string]int)
	c.sidecarRequests = 0
	c.sidecarErrors = 0
	c.sidecarTimeouts = 0
	c.sidecarAgreements = 0
	c.sidecarOverrides = 0
	c.sidecarLatencySum = 0
	c.nliFallbackCount = 0
	c.tierCompScoreSum = [3]float64{}
	c.tierCompCount = [3]int64{}
	c.tierCodeScores = [3]map[string][]float64{
		make(map[string][]float64),
		make(map[string][]float64),
		make(map[string][]float64),
	}
	c.windowStart = time.Now()
}
