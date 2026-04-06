package ape

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"mdemg/internal/metrics"
)

// LiveCollectors provides fresh metric values on every Prometheus scrape.
// It bridges the gap between RSIC assessment cycles (hours/weekly) and
// the 15-second Prometheus scrape interval.
type LiveCollectors struct {
	jiminyProvider   JiminyStatsProvider
	protocolProvider ProtocolStatsProvider
	assessor         *Assessor
	spaceID          string

	// Cached assessment report (updated by Assess() via StoreReport)
	cachedReport atomic.Pointer[SelfAssessmentReport]

	// Guidance cache (Neo4j queries are 50-200ms, cache at 60s)
	guidanceCacheInterval time.Duration
	mu                    sync.Mutex
	cachedJiminyStats     *JiminyStatsResult
	cachedProtoStats      *ProtocolStatsResult
	lastGuidanceRefresh   time.Time
	lastProtoRefresh      time.Time
}

// NewLiveCollectors creates a LiveCollectors wired to the given subsystem providers.
func NewLiveCollectors(
	jiminy JiminyStatsProvider,
	protocol ProtocolStatsProvider,
	assessor *Assessor,
	spaceID string,
	guidanceCacheInterval time.Duration,
) *LiveCollectors {
	return &LiveCollectors{
		jiminyProvider:        jiminy,
		protocolProvider:      protocol,
		assessor:              assessor,
		spaceID:               spaceID,
		guidanceCacheInterval: guidanceCacheInterval,
	}
}

// StoreReport caches the latest SelfAssessmentReport from Assess().
func (lc *LiveCollectors) StoreReport(report *SelfAssessmentReport) {
	if report != nil {
		lc.cachedReport.Store(report)
	}
}

// CollectProtocolMetrics fetches fresh J17 protocol stats and publishes
// all protocol gauges. Protocol stats come from in-memory Snapshot()
// (microseconds), so no cache is needed. On error, cached values are used.
func (lc *LiveCollectors) CollectProtocolMetrics() {
	if lc.protocolProvider == nil {
		return
	}

	ctx := context.Background()
	stats, err := lc.protocolProvider.GetProtocolStats(ctx, lc.spaceID)
	if err != nil {
		slog.Warn("live_collectors: protocol stats fetch failed, using cache", "error", err)
		lc.mu.Lock()
		cached := lc.cachedProtoStats
		lc.mu.Unlock()
		if cached != nil {
			lc.publishProtocolGauges(*cached)
		}
		return
	}

	lc.mu.Lock()
	lc.cachedProtoStats = &stats
	lc.lastProtoRefresh = time.Now()
	lc.mu.Unlock()

	lc.publishProtocolGauges(stats)
}

// CollectGuidanceMetrics fetches Jiminy guidance stats, respecting the
// cache interval to avoid hammering Neo4j. If the interval has not elapsed,
// cached stats are republished. On error, cached values are used.
func (lc *LiveCollectors) CollectGuidanceMetrics() {
	if lc.jiminyProvider == nil {
		return
	}

	lc.mu.Lock()
	elapsed := time.Since(lc.lastGuidanceRefresh) >= lc.guidanceCacheInterval
	cached := lc.cachedJiminyStats
	lc.mu.Unlock()

	if !elapsed && cached != nil {
		lc.publishGuidanceGauges(*cached)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := lc.jiminyProvider.GetGuidanceStats(ctx, lc.spaceID)
	if err != nil {
		slog.Warn("live_collectors: guidance stats fetch failed, using cache", "error", err)
		if cached != nil {
			lc.publishGuidanceGauges(*cached)
		}
		return
	}

	lc.mu.Lock()
	lc.cachedJiminyStats = &stats
	lc.lastGuidanceRefresh = time.Now()
	lc.mu.Unlock()

	lc.publishGuidanceGauges(stats)
}

// CollectHealthMetrics uses the cached SelfAssessmentReport as a baseline,
// then recomputes protocol and guidance sub-scores from fresh stats if
// available. The overall health is recomputed using the same weight formula
// from self_assess.go.
func (lc *LiveCollectors) CollectHealthMetrics() {
	report := lc.cachedReport.Load()
	if report == nil {
		return
	}

	// Start from the cached report values
	r := *report

	// Recompute protocol and guidance sub-scores from fresh stats if available
	lc.mu.Lock()
	freshProto := lc.cachedProtoStats
	freshGuidance := lc.cachedJiminyStats
	lc.mu.Unlock()

	if freshProto != nil && lc.assessor != nil {
		r.ProtocolHealth = lc.assessor.scoreProtocol(*freshProto)
	}
	if freshGuidance != nil && freshGuidance.TotalGuidanceIssued > 0 && lc.assessor != nil {
		r.GuidanceHealth = lc.assessor.scoreGuidance(*freshGuidance)
	}

	// Recompute OverallHealth using the EXACT same weight formula from self_assess.go
	if r.SynergyHealth > 0 && r.ProtocolHealth > 0 && r.GuidanceHealth > 0 {
		// All 7 dimensions
		r.OverallHealth = 0.18*r.RetrievalQuality +
			0.18*r.MemoryHealth +
			0.13*r.EdgeHealth +
			0.13*r.TaskPerformance +
			0.13*r.GuidanceHealth +
			0.13*r.ProtocolHealth +
			0.12*r.SynergyHealth
	} else if r.ProtocolHealth > 0 && r.GuidanceHealth > 0 {
		// 6 dimensions (no synergy)
		r.OverallHealth = 0.20*r.RetrievalQuality +
			0.20*r.MemoryHealth +
			0.15*r.EdgeHealth +
			0.15*r.TaskPerformance +
			0.15*r.GuidanceHealth +
			0.15*r.ProtocolHealth
	} else if r.GuidanceHealth > 0 {
		// 5 dimensions (no protocol)
		r.OverallHealth = 0.25*r.RetrievalQuality +
			0.25*r.MemoryHealth +
			0.20*r.EdgeHealth +
			0.15*r.TaskPerformance +
			0.15*r.GuidanceHealth
	} else {
		// 4 dimensions (no guidance)
		r.OverallHealth = 0.30*r.RetrievalQuality +
			0.25*r.MemoryHealth +
			0.25*r.EdgeHealth +
			0.20*r.TaskPerformance
	}

	// Publish all 9 health gauges
	m := metrics.Metrics()
	sid := lc.spaceID
	m.RSICHealthOverall(sid).Set(r.OverallHealth)
	m.RSICHealthRetrieval(sid).Set(r.RetrievalQuality)
	m.RSICHealthMemory(sid).Set(r.MemoryHealth)
	m.RSICHealthEdge(sid).Set(r.EdgeHealth)
	m.RSICHealthTask(sid).Set(r.TaskPerformance)
	m.RSICHealthGuidance(sid).Set(r.GuidanceHealth)
	m.RSICHealthProtocol(sid).Set(r.ProtocolHealth)
	m.RSICHealthSynergy(sid).Set(r.SynergyHealth)
	m.RSICHealthConfidence(sid).Set(r.Confidence)
}

// ─── Private helpers ───

// publishProtocolGauges sets all J17 protocol Prometheus gauges from stats.
func (lc *LiveCollectors) publishProtocolGauges(stats ProtocolStatsResult) {
	m := metrics.Metrics()
	sid := lc.spaceID

	// Tier distribution
	m.J17TierT1Fraction(sid).Set(stats.TierDistribution[0])
	m.J17TierT2Fraction(sid).Set(stats.TierDistribution[1])
	m.J17TierT3Fraction(sid).Set(stats.TierDistribution[2])

	// Core metrics
	m.J17CompressionRatio(sid).Set(stats.CompressionRatio)
	m.J17AvgComprehension(sid).Set(stats.AvgComprehension)
	m.J17AvgTokensPerGuidance(sid).Set(stats.AvgTokensPerGuidance)
	m.J17ReplayFrequency(sid).Set(stats.ReplayFrequencyPerHour)
	m.J17TicketRestoreRate(sid).Set(stats.TicketRestoreSuccessRate)
	m.J17CodeCoverage(sid).Set(stats.CodeCoverage)
	m.J17EventsTotal(sid).Set(float64(stats.TotalEvents))

	// Per-tier comprehension
	m.J17TierT1Comprehension(sid).Set(stats.TierComprehension[0])
	m.J17TierT2Comprehension(sid).Set(stats.TierComprehension[1])
	m.J17TierT3Comprehension(sid).Set(stats.TierComprehension[2])

	// Per-tier outcome counts
	m.J17TierT1OutcomeCount(sid).Set(float64(stats.TierOutcomeCount[0]))
	m.J17TierT2OutcomeCount(sid).Set(float64(stats.TierOutcomeCount[1]))
	m.J17TierT3OutcomeCount(sid).Set(float64(stats.TierOutcomeCount[2]))

	// Trust score aggregates
	m.J17AvgTrustScore(sid).Set(stats.AvgTrustScore)
	m.J17MinTrustScore(sid).Set(stats.MinTrustScore)
	m.J17MaxTrustScore(sid).Set(stats.MaxTrustScore)
	m.J17TrustSessionCount(sid).Set(float64(stats.TrustSessionCount))

	// NLI calibration
	m.J17NLIMeanBias(sid).Set(stats.NLIMeanBias)
	biasAlertVal := 0.0
	if stats.NLIBiasAlert {
		biasAlertVal = 1.0
	}
	m.J17NLIBiasAlert(sid).Set(biasAlertVal)

	// NLI fallback tracking
	m.J17NLIFallbackTotal(sid).Set(float64(stats.NLIFallbackCount))

	// Sidecar metrics
	if stats.Sidecar != nil {
		m.J17SidecarRequests(sid).Set(float64(stats.Sidecar.Requests))
		m.J17SidecarErrors(sid).Set(float64(stats.Sidecar.Errors))
		m.J17SidecarTimeouts(sid).Set(float64(stats.Sidecar.Timeouts))
		m.J17SidecarAgreementRate(sid).Set(stats.Sidecar.AgreementRate)
		m.J17SidecarOverrideRate(sid).Set(stats.Sidecar.OverrideRate)
		m.J17SidecarLatency(sid).Set(stats.Sidecar.AvgLatencyMs)
	} else {
		m.J17SidecarRequests(sid).Set(0)
		m.J17SidecarErrors(sid).Set(0)
		m.J17SidecarTimeouts(sid).Set(0)
		m.J17SidecarAgreementRate(sid).Set(0)
		m.J17SidecarOverrideRate(sid).Set(0)
		m.J17SidecarLatency(sid).Set(0)
	}
}

// LastGaugeValues returns the most recently collected gauge values as a map.
// This is used by the TSDB writer to record historical snapshots without
// creating a dependency on the tsdb package.
func (lc *LiveCollectors) LastGaugeValues() map[string]float64 {
	lc.mu.Lock()
	proto := lc.cachedProtoStats
	jiminy := lc.cachedJiminyStats
	lc.mu.Unlock()

	gauges := make(map[string]float64, 40)

	if proto != nil {
		gauges["j17_tier1_distribution"] = proto.TierDistribution[0]
		gauges["j17_tier2_distribution"] = proto.TierDistribution[1]
		gauges["j17_tier3_distribution"] = proto.TierDistribution[2]
		gauges["j17_avg_tokens_per_guidance"] = proto.AvgTokensPerGuidance
		gauges["j17_compression_ratio"] = proto.CompressionRatio
		gauges["j17_avg_comprehension"] = proto.AvgComprehension
		gauges["j17_replay_frequency_per_hour"] = proto.ReplayFrequencyPerHour
		gauges["j17_ticket_restore_success_rate"] = proto.TicketRestoreSuccessRate
		gauges["j17_code_coverage"] = proto.CodeCoverage
		gauges["j17_total_events"] = float64(proto.TotalEvents)
		gauges["j17_nli_fallback_count"] = float64(proto.NLIFallbackCount)
		gauges["j17_nli_fallback_rate"] = proto.NLIFallbackRate
		gauges["j17_nli_mean_bias"] = proto.NLIMeanBias
	}

	if jiminy != nil {
		gauges["jiminy_follow_rate"] = jiminy.FollowRate
		gauges["jiminy_constraint_effectiveness"] = jiminy.ConstraintEffRate
		gauges["jiminy_source_diversity"] = jiminy.SourceDiversity
		gauges["jiminy_total_issued"] = float64(jiminy.TotalGuidanceIssued)
		gauges["jiminy_total_followed"] = float64(jiminy.TotalFollowed)
		gauges["jiminy_total_partial_compliance"] = float64(jiminy.TotalPartialCompliance)
		gauges["jiminy_total_ignored"] = float64(jiminy.TotalIgnored)
		gauges["jiminy_total_contradicted"] = float64(jiminy.TotalContradicted)
	}

	report := lc.cachedReport.Load()
	if report != nil {
		gauges["rsic_health_overall"] = report.OverallHealth
		gauges["rsic_health_retrieval"] = report.RetrievalQuality
		gauges["rsic_health_memory"] = report.MemoryHealth
		gauges["rsic_health_edge"] = report.EdgeHealth
		gauges["rsic_health_task"] = report.TaskPerformance
		gauges["rsic_health_guidance"] = report.GuidanceHealth
		gauges["rsic_health_protocol"] = report.ProtocolHealth
		gauges["rsic_health_synergy"] = report.SynergyHealth
		gauges["rsic_health_confidence"] = report.Confidence
	}

	return gauges
}

// publishGuidanceGauges sets all Jiminy guidance Prometheus gauges from stats.
func (lc *LiveCollectors) publishGuidanceGauges(stats JiminyStatsResult) {
	m := metrics.Metrics()
	sid := lc.spaceID

	m.JiminyFollowRate(sid).Set(stats.FollowRate)
	m.JiminyConstraintEffectiveness(sid).Set(stats.ConstraintEffRate)
	m.JiminySourceDiversity(sid).Set(stats.SourceDiversity)
	m.JiminyTotalIssued(sid).Set(float64(stats.TotalGuidanceIssued))
	m.JiminyTotalFollowed(sid).Set(float64(stats.TotalFollowed))
	m.JiminyTotalPartialCompliance(sid).Set(float64(stats.TotalPartialCompliance))
	m.JiminyTotalIgnored(sid).Set(float64(stats.TotalIgnored))
	m.JiminyTotalContradicted(sid).Set(float64(stats.TotalContradicted))
}
