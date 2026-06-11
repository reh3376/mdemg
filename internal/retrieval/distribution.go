package retrieval

import (
	"math"
	"sort"
	"sync"
	"time"
)

// ScoreDistribution tracks score statistics for a single retrieval query
type ScoreDistribution struct {
	Mean      float64   `json:"mean"`
	StdDev    float64   `json:"std_dev"`
	Min       float64   `json:"min"`
	Max       float64   `json:"max"`
	P10       float64   `json:"p10"`   // 10th percentile
	P25       float64   `json:"p25"`   // 25th percentile
	P50       float64   `json:"p50"`   // Median
	P75       float64   `json:"p75"`   // 75th percentile
	P90       float64   `json:"p90"`   // 90th percentile
	Range     float64   `json:"range"` // Max - Min
	Count     int       `json:"count"` // Number of results
	Timestamp time.Time `json:"timestamp"`
}

// LearningPhase represents the current state of learning edge accumulation
type LearningPhase string

const (
	PhaseCold      LearningPhase = "cold"      // 0 edges - pure semantic retrieval
	PhaseLearning  LearningPhase = "learning"  // 0-10k edges - actively learning
	PhaseWarm      LearningPhase = "warm"      // 10k-50k edges - rich associative network
	PhaseSaturated LearningPhase = "saturated" // 50k+ edges - consider pruning
)

// PhaseThresholds defines the edge count boundaries for learning phases
var PhaseThresholds = struct {
	Learning  int64 // Transition from cold to learning
	Warm      int64 // Transition from learning to warm
	Saturated int64 // Transition from warm to saturated
}{
	Learning:  1,
	Warm:      10000,
	Saturated: 50000,
}

// AlertCondition represents a detected anomaly in score distribution
type AlertCondition struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Timestamp   time.Time `json:"timestamp"`
}

// DistributionMonitor tracks score distributions over time and detects anomalies
type DistributionMonitor struct {
	mu sync.RWMutex

	// Rolling window of recent distributions (per space)
	history map[string][]ScoreDistribution

	// Current learning phase (per space)
	phases map[string]LearningPhase

	// Edge counts (per space)
	edgeCounts map[string]int64

	// Active alerts (per space)
	alerts map[string][]AlertCondition

	// Configuration
	historySize int     // Max distributions to keep per space
	stdDevAlert float64 // Alert if std dev falls below this
	meanAlert   float64 // Alert if mean falls below this
}

// NewDistributionMonitor creates a new monitor with default settings
func NewDistributionMonitor() *DistributionMonitor {
	return &DistributionMonitor{
		history:     make(map[string][]ScoreDistribution),
		phases:      make(map[string]LearningPhase),
		edgeCounts:  make(map[string]int64),
		alerts:      make(map[string][]AlertCondition),
		historySize: 100, // Keep last 100 distributions per space
		stdDevAlert: 0.05,
		meanAlert:   0.4,
	}
}

// ComputeDistribution calculates score distribution from a set of scores
func ComputeDistribution(scores []float64) ScoreDistribution {
	n := len(scores)
	if n == 0 {
		return ScoreDistribution{Timestamp: time.Now()}
	}

	// Sort for percentile calculation
	sorted := make([]float64, n)
	copy(sorted, scores)
	sort.Float64s(sorted)

	// Calculate mean
	sum := 0.0
	for _, s := range scores {
		sum += s
	}
	mean := sum / float64(n)

	// Calculate std dev
	sumSq := 0.0
	for _, s := range scores {
		diff := s - mean
		sumSq += diff * diff
	}
	stdDev := math.Sqrt(sumSq / float64(n))

	// Percentiles
	p10 := percentile(sorted, 0.10)
	p25 := percentile(sorted, 0.25)
	p50 := percentile(sorted, 0.50)
	p75 := percentile(sorted, 0.75)
	p90 := percentile(sorted, 0.90)

	return ScoreDistribution{
		Mean:      mean,
		StdDev:    stdDev,
		Min:       sorted[0],
		Max:       sorted[n-1],
		P10:       p10,
		P25:       p25,
		P50:       p50,
		P75:       p75,
		P90:       p90,
		Range:     sorted[n-1] - sorted[0],
		Count:     n,
		Timestamp: time.Now(),
	}
}

// percentile calculates the p-th percentile from a sorted slice
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	// Linear interpolation
	index := p * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sorted[lower]
	}

	// Interpolate between lower and upper
	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// RecordDistribution adds a new distribution to the history and checks for alerts
func (m *DistributionMonitor) RecordDistribution(spaceID string, dist ScoreDistribution) []AlertCondition {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Add to history
	if m.history[spaceID] == nil {
		m.history[spaceID] = make([]ScoreDistribution, 0, m.historySize)
	}
	m.history[spaceID] = append(m.history[spaceID], dist)

	// Trim to max size
	if len(m.history[spaceID]) > m.historySize {
		m.history[spaceID] = m.history[spaceID][len(m.history[spaceID])-m.historySize:]
	}

	// Check for alert conditions
	var newAlerts []AlertCondition

	// Alert: Score distribution too compressed
	if dist.StdDev < m.stdDevAlert && dist.Count > 5 {
		newAlerts = append(newAlerts, AlertCondition{
			Type:        "distribution_compressed",
			Description: "Score distribution is highly compressed - learning edges may be saturated",
			Value:       dist.StdDev,
			Threshold:   m.stdDevAlert,
			Timestamp:   time.Now(),
		})
	}

	// Alert: Mean score too low
	if dist.Mean < m.meanAlert && dist.Count > 5 {
		newAlerts = append(newAlerts, AlertCondition{
			Type:        "mean_score_low",
			Description: "Mean retrieval score is low - check query quality or embeddings",
			Value:       dist.Mean,
			Threshold:   m.meanAlert,
			Timestamp:   time.Now(),
		})
	}

	// Alert: Very narrow range
	if dist.Range < 0.1 && dist.Count > 5 {
		newAlerts = append(newAlerts, AlertCondition{
			Type:        "narrow_range",
			Description: "Score range is very narrow - results may lack differentiation",
			Value:       dist.Range,
			Threshold:   0.1,
			Timestamp:   time.Now(),
		})
	}

	// Store alerts
	if len(newAlerts) > 0 {
		if m.alerts[spaceID] == nil {
			m.alerts[spaceID] = make([]AlertCondition, 0)
		}
		m.alerts[spaceID] = append(m.alerts[spaceID], newAlerts...)
		// Keep only last 50 alerts
		if len(m.alerts[spaceID]) > 50 {
			m.alerts[spaceID] = m.alerts[spaceID][len(m.alerts[spaceID])-50:]
		}
	}

	return newAlerts
}

// UpdateEdgeCount updates the edge count and recalculates learning phase
func (m *DistributionMonitor) UpdateEdgeCount(spaceID string, count int64) LearningPhase {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.edgeCounts[spaceID] = count

	// Determine phase based on edge count
	var phase LearningPhase
	switch {
	case count < PhaseThresholds.Learning:
		phase = PhaseCold
	case count < PhaseThresholds.Warm:
		phase = PhaseLearning
	case count < PhaseThresholds.Saturated:
		phase = PhaseWarm
	default:
		phase = PhaseSaturated
	}

	oldPhase := m.phases[spaceID]
	m.phases[spaceID] = phase

	// Alert on phase transitions to saturated
	if phase == PhaseSaturated && oldPhase != PhaseSaturated {
		if m.alerts[spaceID] == nil {
			m.alerts[spaceID] = make([]AlertCondition, 0)
		}
		m.alerts[spaceID] = append(m.alerts[spaceID], AlertCondition{
			Type:        "phase_saturated",
			Description: "Learning edge count exceeded 50,000 - consider pruning low-evidence edges",
			Value:       float64(count),
			Threshold:   float64(PhaseThresholds.Saturated),
			Timestamp:   time.Now(),
		})
	}

	return phase
}

// GetStats returns comprehensive statistics for a space
func (m *DistributionMonitor) GetStats(spaceID string) DistributionStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := DistributionStats{
		SpaceID:    spaceID,
		EdgeCount:  m.edgeCounts[spaceID],
		Phase:      m.phases[spaceID],
		Alerts:     m.alerts[spaceID],
		QueryCount: len(m.history[spaceID]),
	}

	// If no phase set, default to cold
	if stats.Phase == "" {
		stats.Phase = PhaseCold
	}

	// Calculate aggregate stats from history
	if len(m.history[spaceID]) > 0 {
		history := m.history[spaceID]

		// Latest distribution
		stats.Latest = &history[len(history)-1]

		// Aggregate means and std devs
		var sumMean, sumStdDev float64
		var minMean, maxMean float64 = 1.0, 0.0
		var minStdDev, maxStdDev float64 = 1.0, 0.0

		for _, d := range history {
			sumMean += d.Mean
			sumStdDev += d.StdDev
			if d.Mean < minMean {
				minMean = d.Mean
			}
			if d.Mean > maxMean {
				maxMean = d.Mean
			}
			if d.StdDev < minStdDev {
				minStdDev = d.StdDev
			}
			if d.StdDev > maxStdDev {
				maxStdDev = d.StdDev
			}
		}

		n := float64(len(history))
		stats.AggregatedStats = &AggregatedDistributionStats{
			AvgMean:     sumMean / n,
			AvgStdDev:   sumStdDev / n,
			MinMean:     minMean,
			MaxMean:     maxMean,
			MinStdDev:   minStdDev,
			MaxStdDev:   maxStdDev,
			SampleCount: len(history),
			WindowStart: history[0].Timestamp,
			WindowEnd:   history[len(history)-1].Timestamp,
		}

		// Calculate trend (comparing recent vs older)
		if len(history) >= 10 {
			recent := history[len(history)-5:]
			older := history[:5]

			var recentMean, olderMean float64
			for _, d := range recent {
				recentMean += d.Mean
			}
			for _, d := range older {
				olderMean += d.Mean
			}
			recentMean /= 5
			olderMean /= 5

			stats.Trend = &DistributionTrend{
				MeanDelta:   recentMean - olderMean,
				Direction:   trendDirection(recentMean - olderMean),
				Description: trendDescription(recentMean - olderMean),
			}
		}
	}

	return stats
}

// GetHistory returns the distribution history for a space
func (m *DistributionMonitor) GetHistory(spaceID string, limit int) []ScoreDistribution {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := m.history[spaceID]
	if history == nil {
		return nil
	}

	if limit <= 0 || limit > len(history) {
		limit = len(history)
	}

	// Return most recent
	result := make([]ScoreDistribution, limit)
	copy(result, history[len(history)-limit:])
	return result
}

// ClearAlerts clears alerts for a space
func (m *DistributionMonitor) ClearAlerts(spaceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts[spaceID] = nil
}

// DistributionStats contains comprehensive monitoring statistics
type DistributionStats struct {
	SpaceID         string                       `json:"space_id"`
	EdgeCount       int64                        `json:"edge_count"`
	Phase           LearningPhase                `json:"phase"`
	PhaseThresholds map[string]int64             `json:"phase_thresholds,omitempty"`
	QueryCount      int                          `json:"query_count"`
	Latest          *ScoreDistribution           `json:"latest,omitempty"`
	AggregatedStats *AggregatedDistributionStats `json:"aggregated,omitempty"`
	Trend           *DistributionTrend           `json:"trend,omitempty"`
	Alerts          []AlertCondition             `json:"alerts,omitempty"`
}

// AggregatedDistributionStats contains aggregate statistics across multiple queries
type AggregatedDistributionStats struct {
	AvgMean     float64   `json:"avg_mean"`
	AvgStdDev   float64   `json:"avg_std_dev"`
	MinMean     float64   `json:"min_mean"`
	MaxMean     float64   `json:"max_mean"`
	MinStdDev   float64   `json:"min_std_dev"`
	MaxStdDev   float64   `json:"max_std_dev"`
	SampleCount int       `json:"sample_count"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

// DistributionTrend indicates whether score distributions are improving or degrading
type DistributionTrend struct {
	MeanDelta   float64 `json:"mean_delta"`
	Direction   string  `json:"direction"` // "improving", "stable", "degrading"
	Description string  `json:"description"`
}

func trendDirection(delta float64) string {
	switch {
	case delta > 0.05:
		return "improving"
	case delta < -0.05:
		return "degrading"
	default:
		return "stable"
	}
}

func trendDescription(delta float64) string {
	switch {
	case delta > 0.1:
		return "Scores are significantly improving"
	case delta > 0.05:
		return "Scores are slightly improving"
	case delta < -0.1:
		return "Scores are significantly degrading - investigate retrieval quality"
	case delta < -0.05:
		return "Scores are slightly degrading"
	default:
		return "Scores are stable"
	}
}

// Global monitor instance
var globalDistributionMonitor = NewDistributionMonitor()

// GetDistributionMonitor returns the global distribution monitor
func GetDistributionMonitor() *DistributionMonitor {
	return globalDistributionMonitor
}
