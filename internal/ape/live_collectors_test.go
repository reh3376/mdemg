package ape

import (
	"context"
	"testing"
	"time"
)

// ─── Mock providers ───

type mockJiminyProvider struct {
	stats JiminyStatsResult
	err   error
	calls int
}

func (m *mockJiminyProvider) GetGuidanceStats(_ context.Context, _ string) (JiminyStatsResult, error) {
	m.calls++
	return m.stats, m.err
}

type mockProtocolProvider struct {
	stats ProtocolStatsResult
	err   error
	calls int
}

func (m *mockProtocolProvider) GetProtocolStats(_ context.Context, _ string) (ProtocolStatsResult, error) {
	m.calls++
	return m.stats, m.err
}

// ─── NewLiveCollectors ───

func TestLiveCollectors_NewLiveCollectors(t *testing.T) {
	tests := []struct {
		name     string
		jiminy   JiminyStatsProvider
		protocol ProtocolStatsProvider
	}{
		{
			name:     "nil providers",
			jiminy:   nil,
			protocol: nil,
		},
		{
			name:     "non-nil providers",
			jiminy:   &mockJiminyProvider{},
			protocol: &mockProtocolProvider{},
		},
		{
			name:     "mixed nil and non-nil",
			jiminy:   &mockJiminyProvider{},
			protocol: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := NewLiveCollectors(tt.jiminy, tt.protocol, nil, "test-space", 60*time.Second)
			if lc == nil {
				t.Errorf("got nil, want non-nil LiveCollectors")
			}
			if lc.spaceID != "test-space" {
				t.Errorf("got spaceID %q, want %q", lc.spaceID, "test-space")
			}
		})
	}
}

// ─── StoreReport ───

func TestLiveCollectors_StoreReport(t *testing.T) {
	tests := []struct {
		name       string
		report     *SelfAssessmentReport
		wantStored bool
	}{
		{
			name:       "non-nil report is stored",
			report:     &SelfAssessmentReport{SpaceID: "test", OverallHealth: 0.85},
			wantStored: true,
		},
		{
			name:       "nil report is ignored",
			report:     nil,
			wantStored: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := NewLiveCollectors(nil, nil, nil, "test-space", 60*time.Second)
			lc.StoreReport(tt.report)

			loaded := lc.cachedReport.Load()
			if tt.wantStored {
				if loaded == nil {
					t.Errorf("got nil cached report, want non-nil")
				} else if loaded.OverallHealth != tt.report.OverallHealth {
					t.Errorf("got OverallHealth %f, want %f", loaded.OverallHealth, tt.report.OverallHealth)
				}
			} else {
				if loaded != nil {
					t.Errorf("got non-nil cached report, want nil")
				}
			}
		})
	}
}

// ─── CollectProtocolMetrics ───

func TestLiveCollectors_CollectProtocolMetrics_NilProvider(t *testing.T) {
	lc := NewLiveCollectors(nil, nil, nil, "test-space", 60*time.Second)
	// Must not panic
	lc.CollectProtocolMetrics()
}

func TestLiveCollectors_CollectProtocolMetrics_Success(t *testing.T) {
	mock := &mockProtocolProvider{
		stats: ProtocolStatsResult{
			TierDistribution:   [3]float64{0.5, 0.3, 0.2},
			CompressionRatio:   0.75,
			AvgComprehension:   0.9,
			AvgTokensPerGuidance: 120.0,
			TotalEvents:        42,
		},
	}
	lc := NewLiveCollectors(nil, mock, nil, "test-space", 60*time.Second)

	// Collect once — this calls the mock and caches
	lc.CollectProtocolMetrics()

	if mock.calls != 1 {
		t.Errorf("got %d mock calls, want 1", mock.calls)
	}

	gauges := lc.LastGaugeValues()
	if got, want := gauges["j17_compression_ratio"], 0.75; got != want {
		t.Errorf("j17_compression_ratio: got %f, want %f", got, want)
	}
	if got, want := gauges["j17_avg_comprehension"], 0.9; got != want {
		t.Errorf("j17_avg_comprehension: got %f, want %f", got, want)
	}
	if got, want := gauges["j17_total_events"], float64(42); got != want {
		t.Errorf("j17_total_events: got %f, want %f", got, want)
	}
}

// ─── CollectGuidanceMetrics ───

func TestLiveCollectors_CollectGuidanceMetrics_NilProvider(t *testing.T) {
	lc := NewLiveCollectors(nil, nil, nil, "test-space", 60*time.Second)
	// Must not panic
	lc.CollectGuidanceMetrics()
}

func TestLiveCollectors_CollectGuidanceMetrics_CacheHit(t *testing.T) {
	mock := &mockJiminyProvider{
		stats: JiminyStatsResult{
			TotalGuidanceIssued: 100,
			FollowRate:          0.80,
			SourceDiversity:     0.65,
		},
	}
	// Use a very long cache interval so second call definitely hits cache
	lc := NewLiveCollectors(mock, nil, nil, "test-space", 10*time.Minute)

	// First call — fetches from provider
	lc.CollectGuidanceMetrics()
	if mock.calls != 1 {
		t.Errorf("after first call: got %d mock calls, want 1", mock.calls)
	}

	// Second call — should hit cache, not call provider again
	lc.CollectGuidanceMetrics()
	if mock.calls != 1 {
		t.Errorf("after second call: got %d mock calls, want 1 (cache hit)", mock.calls)
	}

	gauges := lc.LastGaugeValues()
	if got, want := gauges["jiminy_follow_rate"], 0.80; got != want {
		t.Errorf("jiminy_follow_rate: got %f, want %f", got, want)
	}
}

// ─── CollectHealthMetrics ───

func TestLiveCollectors_CollectHealthMetrics_NilReport(t *testing.T) {
	lc := NewLiveCollectors(nil, nil, nil, "test-space", 60*time.Second)
	// Must not panic when no report cached
	lc.CollectHealthMetrics()
}

// ─── LastGaugeValues ───

func TestLiveCollectors_LastGaugeValues_Empty(t *testing.T) {
	lc := NewLiveCollectors(nil, nil, nil, "test-space", 60*time.Second)
	gauges := lc.LastGaugeValues()
	if len(gauges) != 0 {
		t.Errorf("got %d gauge entries, want 0", len(gauges))
	}
}

func TestLiveCollectors_LastGaugeValues_Full(t *testing.T) {
	mock := &mockProtocolProvider{
		stats: ProtocolStatsResult{
			TierDistribution:         [3]float64{0.5, 0.3, 0.2},
			CompressionRatio:         0.75,
			AvgComprehension:         0.9,
			ReplayFrequencyPerHour:   1.5,
			TicketRestoreSuccessRate: 0.95,
			CodeCoverage:             0.88,
			TotalEvents:              42,
			NLIFallbackCount:         3,
			NLIFallbackRate:          0.07,
			NLIMeanBias:              0.01,
			AvgTokensPerGuidance:     120.0,
		},
	}
	mockJ := &mockJiminyProvider{
		stats: JiminyStatsResult{
			TotalGuidanceIssued: 100,
			TotalFollowed:       80,
			TotalIgnored:        15,
			TotalContradicted:   5,
			FollowRate:          0.80,
			ConstraintEffRate:    0.75,
			ConstraintEffRate30d: 0.80,
			ConstraintDataAvail:  true,
			SourceDiversity:      0.65,
		},
	}
	lc := NewLiveCollectors(mockJ, mock, nil, "test-space", 60*time.Second)

	// Populate caches
	lc.CollectProtocolMetrics()
	lc.CollectGuidanceMetrics()
	lc.StoreReport(&SelfAssessmentReport{
		OverallHealth:    0.85,
		RetrievalQuality: 0.9,
		MemoryHealth:     0.88,
		EdgeHealth:       0.82,
		TaskPerformance:  0.79,
		GuidanceHealth:   0.77,
		ProtocolHealth:   0.91,
		SynergyHealth:    0.86,
		Confidence:       0.93,
	})

	gauges := lc.LastGaugeValues()

	// j17_ keys (13 expected)
	j17Keys := []string{
		"j17_tier1_distribution", "j17_tier2_distribution", "j17_tier3_distribution",
		"j17_avg_tokens_per_guidance", "j17_compression_ratio", "j17_avg_comprehension",
		"j17_replay_frequency_per_hour", "j17_ticket_restore_success_rate", "j17_code_coverage",
		"j17_total_events", "j17_nli_fallback_count", "j17_nli_fallback_rate", "j17_nli_mean_bias",
	}
	for _, k := range j17Keys {
		if _, ok := gauges[k]; !ok {
			t.Errorf("missing expected j17 gauge key %q", k)
		}
	}

	// jiminy_ keys (8 expected)
	jiminyKeys := []string{
		"jiminy_follow_rate", "jiminy_constraint_effectiveness", "jiminy_constraint_effectiveness_30d",
		"jiminy_source_diversity",
		"jiminy_total_issued", "jiminy_total_followed", "jiminy_total_ignored", "jiminy_total_contradicted",
	}
	for _, k := range jiminyKeys {
		if _, ok := gauges[k]; !ok {
			t.Errorf("missing expected jiminy gauge key %q", k)
		}
	}

	// rsic_ keys (9 expected)
	rsicKeys := []string{
		"rsic_health_overall", "rsic_health_retrieval", "rsic_health_memory",
		"rsic_health_edge", "rsic_health_task", "rsic_health_guidance",
		"rsic_health_protocol", "rsic_health_synergy", "rsic_health_confidence",
	}
	for _, k := range rsicKeys {
		if _, ok := gauges[k]; !ok {
			t.Errorf("missing expected rsic gauge key %q", k)
		}
	}

	// Verify a few specific values
	if got, want := gauges["j17_compression_ratio"], 0.75; got != want {
		t.Errorf("j17_compression_ratio: got %f, want %f", got, want)
	}
	if got, want := gauges["jiminy_follow_rate"], 0.80; got != want {
		t.Errorf("jiminy_follow_rate: got %f, want %f", got, want)
	}
	if got, want := gauges["rsic_health_overall"], 0.85; got != want {
		t.Errorf("rsic_health_overall: got %f, want %f", got, want)
	}
}

func TestLiveCollectors_LastGaugeValues_RSICSubDimensions(t *testing.T) {
	lc := NewLiveCollectors(nil, nil, nil, "test-space", 60*time.Second)
	lc.StoreReport(&SelfAssessmentReport{
		OverallHealth:    0.85,
		RetrievalQuality: 0.90,
		MemoryHealth:     0.88,
		EdgeHealth:       0.82,
		TaskPerformance:  0.79,
		GuidanceHealth:   0.77,
		ProtocolHealth:   0.91,
		SynergyHealth:    0.86,
		Confidence:       0.93,
	})

	gauges := lc.LastGaugeValues()

	// All 9 RSIC dimensions must be present
	wantKeys := map[string]float64{
		"rsic_health_overall":    0.85,
		"rsic_health_retrieval":  0.90,
		"rsic_health_memory":     0.88,
		"rsic_health_edge":       0.82,
		"rsic_health_task":       0.79,
		"rsic_health_guidance":   0.77,
		"rsic_health_protocol":   0.91,
		"rsic_health_synergy":    0.86,
		"rsic_health_confidence": 0.93,
	}

	for k, want := range wantKeys {
		got, ok := gauges[k]
		if !ok {
			t.Errorf("missing RSIC dimension key %q", k)
			continue
		}
		if got != want {
			t.Errorf("%s: got %f, want %f", k, got, want)
		}
	}

	if len(wantKeys) != 9 {
		t.Errorf("expected 9 RSIC dimensions, defined %d", len(wantKeys))
	}
}
