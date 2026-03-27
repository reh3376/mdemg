package jiminy

import (
	"testing"

	"mdemg/internal/config"
)

// newCacheMetricsService constructs a minimal Service with J17 components set up
// for testing recordCacheHitMetrics. All other fields are left nil.
func newCacheMetricsService() *Service {
	return &Service{
		cfg: config.Config{
			J17Enabled: true,
		},
		encoder:         NewProtocolEncoder(TierCoded),
		protocolMetrics: NewProtocolMetricsCollector(),
	}
}

// TestRecordCacheHitMetrics_NilProtocolMetrics verifies that calling
// recordCacheHitMetrics with a nil protocolMetrics field is a no-op and
// does not panic.
func TestRecordCacheHitMetrics_NilProtocolMetrics(t *testing.T) {
	s := &Service{
		cfg: config.Config{
			J17Enabled: true,
		},
		encoder:         NewProtocolEncoder(TierCoded),
		protocolMetrics: nil,
	}

	// Must not panic.
	s.recordCacheHitMetrics(GuidanceResponse{
		Guidance: []GuidanceItem{
			{Type: GuidanceConstraint, Tier: TierCoded, ConstraintCode: "no-force-push"},
		},
	})
}

// TestRecordCacheHitMetrics_J17Disabled verifies that when J17Enabled is false
// no metrics are recorded, even when protocolMetrics and encoder are both set.
func TestRecordCacheHitMetrics_J17Disabled(t *testing.T) {
	s := &Service{
		cfg: config.Config{
			J17Enabled: false,
		},
		encoder:         NewProtocolEncoder(TierCoded),
		protocolMetrics: NewProtocolMetricsCollector(),
	}

	s.recordCacheHitMetrics(GuidanceResponse{
		Guidance: []GuidanceItem{
			{Type: GuidanceConstraint, Tier: TierCoded, ConstraintCode: "no-force-push"},
		},
	})

	snap := s.protocolMetrics.Snapshot()
	if snap.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0 when J17Enabled=false", snap.TotalEvents)
	}
}

// TestRecordCacheHitMetrics_NilEncoder verifies that when encoder is nil
// (but protocolMetrics is set and J17Enabled is true) the method is a no-op.
func TestRecordCacheHitMetrics_NilEncoder(t *testing.T) {
	s := &Service{
		cfg: config.Config{
			J17Enabled: true,
		},
		encoder:         nil,
		protocolMetrics: NewProtocolMetricsCollector(),
	}

	s.recordCacheHitMetrics(GuidanceResponse{
		Guidance: []GuidanceItem{
			{Type: GuidanceConstraint, Tier: TierCoded, ConstraintCode: "no-force-push"},
		},
	})

	snap := s.protocolMetrics.Snapshot()
	if snap.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0 when encoder=nil", snap.TotalEvents)
	}
}

// TestRecordCacheHitMetrics_EmptyGuidance verifies that when resp.Guidance is
// empty, RecordGuidance is never called (TotalEvents stays 0) but
// RecordConstraintCoverage is still called with (0, 0), leaving CodeCoverage at
// its default of 1.0.
func TestRecordCacheHitMetrics_EmptyGuidance(t *testing.T) {
	s := newCacheMetricsService()

	s.recordCacheHitMetrics(GuidanceResponse{Guidance: nil})

	snap := s.protocolMetrics.Snapshot()
	if snap.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0 for empty guidance", snap.TotalEvents)
	}
	// RecordConstraintCoverage(0, 0) → no constraints → coverage defaults to 1.0.
	if snap.CodeCoverage != 1.0 {
		t.Errorf("CodeCoverage = %f, want 1.0 for empty guidance", snap.CodeCoverage)
	}
}

// TestRecordCacheHitMetrics_RecordsAllTiers verifies that items with different
// tiers produce the correct per-tier token estimates:
//
//	TierCoded (1)       → tokenEst 15
//	TierTelegraphic (2) → tokenEst 50
//	default tier (3)    → tokenEst 80 (T3 baseline)
func TestRecordCacheHitMetrics_RecordsAllTiers(t *testing.T) {
	s := newCacheMetricsService()

	resp := GuidanceResponse{
		Guidance: []GuidanceItem{
			{Type: GuidancePattern, Tier: TierCoded, ConstraintCode: "code-t1"},
			{Type: GuidancePattern, Tier: TierTelegraphic, ConstraintCode: "code-t2"},
			{Type: GuidancePattern, Tier: TierFullNL}, // default / T3 baseline
		},
	}

	s.recordCacheHitMetrics(resp)

	snap := s.protocolMetrics.Snapshot()

	if snap.TotalEvents != 3 {
		t.Errorf("TotalEvents = %d, want 3", snap.TotalEvents)
	}

	// Tier distribution: 1 per tier → each 1/3.
	const want = 1.0 / 3.0
	const epsilon = 0.001
	for i, got := range snap.TierDistribution {
		if got < want-epsilon || got > want+epsilon {
			t.Errorf("TierDistribution[%d] = %f, want ~%f", i, got, want)
		}
	}

	// Average tokens: (15 + 50 + 80) / 3 = 48.333...
	wantAvg := (15.0 + 50.0 + 80.0) / 3.0
	if snap.AvgTokensPerGuidance < wantAvg-epsilon || snap.AvgTokensPerGuidance > wantAvg+epsilon {
		t.Errorf("AvgTokensPerGuidance = %f, want %f", snap.AvgTokensPerGuidance, wantAvg)
	}
}

// TestRecordCacheHitMetrics_ConstraintCoverage verifies that constraint items
// are counted correctly and RecordConstraintCoverage reflects the right totals.
//
// Test scenario:
//   - 3 constraint items total
//   - 2 have a ConstraintCode (coded)
//   - 1 has no code → coverage = 2/3
func TestRecordCacheHitMetrics_ConstraintCoverage(t *testing.T) {
	s := newCacheMetricsService()

	resp := GuidanceResponse{
		Guidance: []GuidanceItem{
			// Constraint with code.
			{Type: GuidanceConstraint, Tier: TierCoded, ConstraintCode: "no-force-push"},
			// Constraint with code.
			{Type: GuidanceConstraint, Tier: TierTelegraphic, ConstraintCode: "test-first"},
			// Constraint without code (no ConstraintCode, no SourceNodes).
			{Type: GuidanceConstraint, Tier: TierFullNL},
			// Non-constraint item: should not affect constraint counters.
			{Type: GuidancePattern, Tier: TierCoded, ConstraintCode: "some-pattern"},
		},
	}

	s.recordCacheHitMetrics(resp)

	snap := s.protocolMetrics.Snapshot()

	// 4 guidance items → 4 RecordGuidance calls.
	if snap.TotalEvents != 4 {
		t.Errorf("TotalEvents = %d, want 4", snap.TotalEvents)
	}

	// 3 constraint items, 2 with codes → coverage = 2/3 ≈ 0.667.
	wantCoverage := 2.0 / 3.0
	const epsilon = 0.001
	if snap.CodeCoverage < wantCoverage-epsilon || snap.CodeCoverage > wantCoverage+epsilon {
		t.Errorf("CodeCoverage = %f, want ~%f (2 coded out of 3 constraints)", snap.CodeCoverage, wantCoverage)
	}
}

// TestRecordCacheHitMetrics_UncodedConstraintTrackedByNodeID verifies the Gap 5
// behaviour: uncoded constraint items that have SourceNodes pass the first node
// ID as the code to RecordGuidance when tier is T2, making it visible in
// T2FrequencyByConstraint.
func TestRecordCacheHitMetrics_UncodedConstraintTrackedByNodeID(t *testing.T) {
	s := newCacheMetricsService()

	resp := GuidanceResponse{
		Guidance: []GuidanceItem{
			{
				Type:        GuidanceConstraint,
				Tier:        TierTelegraphic, // T2 → tracked in T2FrequencyByConstraint
				SourceNodes: []string{"node-abc-123"},
				// No ConstraintCode → falls into the Gap 5 branch.
			},
		},
	}

	s.recordCacheHitMetrics(resp)

	snap := s.protocolMetrics.Snapshot()

	if snap.T2FrequencyByConstraint["node-abc-123"] != 1 {
		t.Errorf("T2FrequencyByConstraint[node-abc-123] = %d, want 1",
			snap.T2FrequencyByConstraint["node-abc-123"])
	}
}

// TestRecordCacheHitMetrics_ConstraintCoverage_AllCoded verifies that when
// every constraint item has a ConstraintCode, CodeCoverage is 1.0.
func TestRecordCacheHitMetrics_ConstraintCoverage_AllCoded(t *testing.T) {
	s := newCacheMetricsService()

	resp := GuidanceResponse{
		Guidance: []GuidanceItem{
			{Type: GuidanceConstraint, Tier: TierCoded, ConstraintCode: "rule-a"},
			{Type: GuidanceConstraint, Tier: TierCoded, ConstraintCode: "rule-b"},
		},
	}

	s.recordCacheHitMetrics(resp)

	snap := s.protocolMetrics.Snapshot()
	if snap.CodeCoverage != 1.0 {
		t.Errorf("CodeCoverage = %f, want 1.0 (all constraints coded)", snap.CodeCoverage)
	}
}

// TestRecordCacheHitMetrics_ConstraintCoverage_NoneCoded verifies that when no
// constraint item has a ConstraintCode, CodeCoverage is 0.0.
func TestRecordCacheHitMetrics_ConstraintCoverage_NoneCoded(t *testing.T) {
	s := newCacheMetricsService()

	resp := GuidanceResponse{
		Guidance: []GuidanceItem{
			{Type: GuidanceConstraint, Tier: TierFullNL},
			{Type: GuidanceConstraint, Tier: TierFullNL},
		},
	}

	s.recordCacheHitMetrics(resp)

	snap := s.protocolMetrics.Snapshot()
	if snap.CodeCoverage != 0.0 {
		t.Errorf("CodeCoverage = %f, want 0.0 (no constraints coded)", snap.CodeCoverage)
	}
}

// TestRecordCacheHitMetrics_OnlyNonConstraints verifies that non-constraint
// guidance items do not contribute to constraint coverage counts.  With no
// constraint items, RecordConstraintCoverage(0, 0) is called and coverage
// defaults to 1.0.
func TestRecordCacheHitMetrics_OnlyNonConstraints(t *testing.T) {
	s := newCacheMetricsService()

	resp := GuidanceResponse{
		Guidance: []GuidanceItem{
			{Type: GuidancePattern, Tier: TierCoded, ConstraintCode: "pat-1"},
			{Type: GuidanceCorrection, Tier: TierTelegraphic},
		},
	}

	s.recordCacheHitMetrics(resp)

	snap := s.protocolMetrics.Snapshot()

	if snap.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, want 2", snap.TotalEvents)
	}
	// No constraint items → coverage defaults to 1.0.
	if snap.CodeCoverage != 1.0 {
		t.Errorf("CodeCoverage = %f, want 1.0 (no constraint items)", snap.CodeCoverage)
	}
}
