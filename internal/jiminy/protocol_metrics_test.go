package jiminy

import (
	"fmt"
	"testing"
)

func TestProtocolMetricsCollector_RecordAndSnapshot(t *testing.T) {
	c := NewProtocolMetricsCollector()

	// Record T1 guidance
	c.RecordGuidance(1, 15, []string{"no-force-push"})
	c.RecordGuidance(1, 12, []string{"test-first"})

	// Record T2 guidance
	c.RecordGuidance(2, 50, []string{"auth-compliance"})

	// Record T3 guidance
	c.RecordGuidance(3, 100, nil)

	snapshot := c.Snapshot()

	if snapshot.TotalEvents != 4 {
		t.Errorf("total events = %d, want 4", snapshot.TotalEvents)
	}

	// Tier distribution: 2 T1, 1 T2, 1 T3
	if snapshot.TierDistribution[0] != 0.5 {
		t.Errorf("T1 distribution = %f, want 0.5", snapshot.TierDistribution[0])
	}
	if snapshot.TierDistribution[1] != 0.25 {
		t.Errorf("T2 distribution = %f, want 0.25", snapshot.TierDistribution[1])
	}
	if snapshot.TierDistribution[2] != 0.25 {
		t.Errorf("T3 distribution = %f, want 0.25", snapshot.TierDistribution[2])
	}

	// Average tokens: (15+12+50+100)/4 = 44.25
	if snapshot.AvgTokensPerGuidance != 44.25 {
		t.Errorf("avg tokens = %f, want 44.25", snapshot.AvgTokensPerGuidance)
	}

	// Compression ratio should be > 1.0 (we have T1 events)
	if snapshot.CompressionRatio <= 1.0 {
		t.Errorf("compression ratio = %f, should be > 1.0", snapshot.CompressionRatio)
	}

	// T2 frequency
	if snapshot.T2FrequencyByConstraint["auth-compliance"] != 1 {
		t.Errorf("T2 frequency for auth-compliance = %d, want 1",
			snapshot.T2FrequencyByConstraint["auth-compliance"])
	}
}

func TestProtocolMetricsCollector_Comprehension(t *testing.T) {
	c := NewProtocolMetricsCollector()

	// Record outcomes for a code
	c.RecordOutcome("no-force-push", 0.9) // followed
	c.RecordOutcome("no-force-push", 0.8) // followed
	c.RecordOutcome("no-force-push", 0.3) // not followed

	snapshot := c.Snapshot()

	rate, ok := snapshot.CodeComprehension["no-force-push"]
	if !ok {
		t.Fatal("code comprehension missing for no-force-push")
	}
	// 2 followed out of 3 total → 0.666...
	if rate < 0.6 || rate > 0.7 {
		t.Errorf("comprehension rate = %f, want ~0.667", rate)
	}
}

func TestProtocolMetricsCollector_TicketRestore(t *testing.T) {
	c := NewProtocolMetricsCollector()

	c.RecordTicketRestore(true)
	c.RecordTicketRestore(true)
	c.RecordTicketRestore(false)

	snapshot := c.Snapshot()

	// 2 success out of 3
	if snapshot.TicketRestoreSuccessRate < 0.6 || snapshot.TicketRestoreSuccessRate > 0.7 {
		t.Errorf("ticket restore rate = %f, want ~0.667", snapshot.TicketRestoreSuccessRate)
	}
}

func TestProtocolMetricsCollector_Reset(t *testing.T) {
	c := NewProtocolMetricsCollector()

	c.RecordGuidance(1, 15, nil)
	c.RecordGuidance(2, 50, nil)

	c.Reset()

	snapshot := c.Snapshot()
	if snapshot.TotalEvents != 0 {
		t.Errorf("total events after reset = %d, want 0", snapshot.TotalEvents)
	}
}

func TestProtocolMetricsCollector_EmptySnapshot(t *testing.T) {
	c := NewProtocolMetricsCollector()
	snapshot := c.Snapshot()

	if snapshot.TotalEvents != 0 {
		t.Errorf("empty total events = %d, want 0", snapshot.TotalEvents)
	}
	if snapshot.CompressionRatio != 0 {
		t.Errorf("empty compression ratio = %f, want 0", snapshot.CompressionRatio)
	}
}

// Gap 1: CodeCoverage tests

func TestProtocolMetrics_CodeCoverage_AllCoded(t *testing.T) {
	c := NewProtocolMetricsCollector()
	c.RecordConstraintCoverage(5, 5) // 5 constraints, all have codes
	snapshot := c.Snapshot()
	if snapshot.CodeCoverage != 1.0 {
		t.Errorf("CodeCoverage = %f, want 1.0 (all coded)", snapshot.CodeCoverage)
	}
}

func TestProtocolMetrics_CodeCoverage_NoCoded(t *testing.T) {
	c := NewProtocolMetricsCollector()
	c.RecordConstraintCoverage(5, 0) // 5 constraints, none have codes
	snapshot := c.Snapshot()
	if snapshot.CodeCoverage != 0.0 {
		t.Errorf("CodeCoverage = %f, want 0.0 (no coded)", snapshot.CodeCoverage)
	}
}

func TestProtocolMetrics_CodeCoverage_Partial(t *testing.T) {
	c := NewProtocolMetricsCollector()
	c.RecordConstraintCoverage(4, 2) // 4 constraints, 2 have codes
	snapshot := c.Snapshot()
	if snapshot.CodeCoverage != 0.5 {
		t.Errorf("CodeCoverage = %f, want 0.5 (partial)", snapshot.CodeCoverage)
	}
}

func TestProtocolMetrics_CodeCoverage_NoConstraints(t *testing.T) {
	c := NewProtocolMetricsCollector()
	c.RecordConstraintCoverage(0, 0) // no constraints surfaced
	snapshot := c.Snapshot()
	// When no constraints were surfaced, coverage should be 1.0 (nothing to cover)
	if snapshot.CodeCoverage != 1.0 {
		t.Errorf("CodeCoverage = %f, want 1.0 (no constraints = nothing to cover)", snapshot.CodeCoverage)
	}
}

// Gap 2: RecordReplay test

func TestProtocolMetrics_RecordReplay(t *testing.T) {
	c := NewProtocolMetricsCollector()
	c.RecordReplay(10)
	snapshot := c.Snapshot()
	if snapshot.ReplayFrequencyPerHour <= 0 {
		t.Errorf("ReplayFrequencyPerHour = %f, want > 0 after recording events", snapshot.ReplayFrequencyPerHour)
	}
}

// Gap 5: T2 frequency for uncoded constraints

func TestProtocolMetrics_T2FrequencyTracksUncodedByNodeID(t *testing.T) {
	c := NewProtocolMetricsCollector()
	// Simulate an uncoded constraint tracked by node ID
	c.RecordGuidance(2, 50, []string{"node-abc-123"})
	c.RecordGuidance(2, 50, []string{"node-abc-123"})
	snapshot := c.Snapshot()
	if snapshot.T2FrequencyByConstraint["node-abc-123"] != 2 {
		t.Errorf("T2 frequency for uncoded node = %d, want 2",
			snapshot.T2FrequencyByConstraint["node-abc-123"])
	}
}

// NS-07: Sidecar metrics tests

func TestProtocolMetrics_SidecarMetrics(t *testing.T) {
	c := NewProtocolMetricsCollector()

	// Record some sidecar calls
	c.RecordSidecarCall(15.0, nil, 1, 1, false)            // agreement, no override
	c.RecordSidecarCall(20.0, nil, 2, 1, true)             // override
	c.RecordSidecarCall(0, fmt.Errorf("timeout"), 0, 1, false) // error

	snap := c.Snapshot()
	if snap.Sidecar == nil {
		t.Fatal("expected sidecar metrics in snapshot")
	}
	if snap.Sidecar.Requests != 3 {
		t.Errorf("requests = %d, want 3", snap.Sidecar.Requests)
	}
	if snap.Sidecar.Errors != 1 {
		t.Errorf("errors = %d, want 1", snap.Sidecar.Errors)
	}
	// Agreement: 1 out of 3 (error doesn't set agreement, mismatched tiers don't either)
	expectedAgreementRate := 1.0 / 3.0
	if snap.Sidecar.AgreementRate < expectedAgreementRate-0.01 || snap.Sidecar.AgreementRate > expectedAgreementRate+0.01 {
		t.Errorf("agreement rate = %f, want ~%f", snap.Sidecar.AgreementRate, expectedAgreementRate)
	}
	// Override: 1 out of 3
	expectedOverrideRate := 1.0 / 3.0
	if snap.Sidecar.OverrideRate < expectedOverrideRate-0.01 || snap.Sidecar.OverrideRate > expectedOverrideRate+0.01 {
		t.Errorf("override rate = %f, want ~%f", snap.Sidecar.OverrideRate, expectedOverrideRate)
	}
	// Avg latency: (15+20+0)/3 ≈ 11.67 ms
	if snap.Sidecar.AvgLatencyMs < 11.0 || snap.Sidecar.AvgLatencyMs > 12.0 {
		t.Errorf("avg latency = %f ms, want ~11.67", snap.Sidecar.AvgLatencyMs)
	}
}

func TestProtocolMetrics_SidecarMetricsNilWhenNoCalls(t *testing.T) {
	c := NewProtocolMetricsCollector()
	snap := c.Snapshot()
	if snap.Sidecar != nil {
		t.Errorf("expected sidecar metrics to be nil when no calls recorded, got %+v", snap.Sidecar)
	}
}

// NLI Feedback Loop: Per-tier comprehension tests

func TestRecordOutcomeWithTier_UpdatesTierMaps(t *testing.T) {
	c := NewProtocolMetricsCollector()
	c.RecordOutcomeWithTier("no-force-push", 1, 0.9)
	c.RecordOutcomeWithTier("no-force-push", 1, 0.7)
	c.RecordOutcomeWithTier("test-first", 2, 0.5)

	snap := c.Snapshot()
	if snap.TierOutcomeCount[0] != 2 {
		t.Errorf("tier 1 outcome count = %d, want 2", snap.TierOutcomeCount[0])
	}
	if snap.TierOutcomeCount[1] != 1 {
		t.Errorf("tier 2 outcome count = %d, want 1", snap.TierOutcomeCount[1])
	}
	if snap.TierOutcomeCount[2] != 0 {
		t.Errorf("tier 3 outcome count = %d, want 0", snap.TierOutcomeCount[2])
	}
}

func TestSnapshot_TierComprehension(t *testing.T) {
	c := NewProtocolMetricsCollector()
	c.RecordOutcomeWithTier("code-a", 1, 0.8)
	c.RecordOutcomeWithTier("code-a", 1, 0.6)
	c.RecordOutcomeWithTier("code-b", 2, 1.0)

	snap := c.Snapshot()
	// T1 avg = (0.8+0.6)/2 = 0.7
	if snap.TierComprehension[0] < 0.69 || snap.TierComprehension[0] > 0.71 {
		t.Errorf("T1 comprehension = %f, want ~0.7", snap.TierComprehension[0])
	}
	// T2 avg = 1.0
	if snap.TierComprehension[1] != 1.0 {
		t.Errorf("T2 comprehension = %f, want 1.0", snap.TierComprehension[1])
	}
	// T3 avg = 0 (no data)
	if snap.TierComprehension[2] != 0 {
		t.Errorf("T3 comprehension = %f, want 0", snap.TierComprehension[2])
	}
}

func TestSnapshot_TierCodeComprehension(t *testing.T) {
	c := NewProtocolMetricsCollector()
	c.RecordOutcomeWithTier("code-a", 1, 0.9)
	c.RecordOutcomeWithTier("code-a", 1, 0.7)
	c.RecordOutcomeWithTier("code-a", 2, 0.5)

	snap := c.Snapshot()
	if snap.TierCodeComprehension == nil {
		t.Fatal("expected non-nil tier code comprehension")
	}
	// Tier 1 code-a avg = (0.9+0.7)/2 = 0.8
	t1Map, ok := snap.TierCodeComprehension[1]
	if !ok {
		t.Fatal("expected tier 1 in TierCodeComprehension")
	}
	if t1Map["code-a"] < 0.79 || t1Map["code-a"] > 0.81 {
		t.Errorf("T1 code-a comprehension = %f, want ~0.8", t1Map["code-a"])
	}
	// Tier 2 code-a avg = 0.5
	t2Map, ok := snap.TierCodeComprehension[2]
	if !ok {
		t.Fatal("expected tier 2 in TierCodeComprehension")
	}
	if t2Map["code-a"] != 0.5 {
		t.Errorf("T2 code-a comprehension = %f, want 0.5", t2Map["code-a"])
	}
}

func TestReset_ClearsTierFields(t *testing.T) {
	c := NewProtocolMetricsCollector()
	c.RecordOutcomeWithTier("code-a", 1, 0.9)
	c.RecordOutcomeWithTier("code-b", 2, 0.5)

	c.Reset()

	snap := c.Snapshot()
	for i := range 3 {
		if snap.TierComprehension[i] != 0 {
			t.Errorf("tier %d comprehension = %f after reset, want 0", i+1, snap.TierComprehension[i])
		}
		if snap.TierOutcomeCount[i] != 0 {
			t.Errorf("tier %d outcome count = %d after reset, want 0", i+1, snap.TierOutcomeCount[i])
		}
	}
	if snap.TierCodeComprehension != nil {
		t.Error("expected nil TierCodeComprehension after reset")
	}
}

func TestRecordOutcomeWithTier_InvalidTier(t *testing.T) {
	c := NewProtocolMetricsCollector()
	// Should not panic on invalid tiers
	c.RecordOutcomeWithTier("code-a", 0, 0.9)
	c.RecordOutcomeWithTier("code-a", 4, 0.5)
	c.RecordOutcomeWithTier("code-a", -1, 0.3)

	snap := c.Snapshot()
	// All tier counters should be 0 (invalid tiers ignored)
	for i := range 3 {
		if snap.TierOutcomeCount[i] != 0 {
			t.Errorf("tier %d count = %d, want 0 (invalid tiers should be ignored)", i+1, snap.TierOutcomeCount[i])
		}
	}
	// But code-level tracking still works (3 records via inlined RecordOutcome logic)
	if snap.CodeComprehension["code-a"] == 0 {
		t.Error("code comprehension should still be tracked for invalid tier values")
	}
}

func TestProtocolMetrics_NLIFallbackCount(t *testing.T) {
	c := NewProtocolMetricsCollector()

	c.RecordNLIFallback()
	c.RecordNLIFallback()
	c.RecordNLIFallback()

	// Also record some genuine events for rate calculation
	c.RecordGuidance(1, 15, nil)
	c.RecordGuidance(2, 50, nil)

	snap := c.Snapshot()
	if snap.NLIFallbackCount != 3 {
		t.Errorf("NLIFallbackCount = %d, want 3", snap.NLIFallbackCount)
	}
	// Rate = 3 fallbacks / 2 total events = 1.5 (more fallbacks than events is valid)
	if snap.NLIFallbackRate == 0 {
		t.Error("NLIFallbackRate should be > 0")
	}
}

func TestProtocolMetrics_NLIFallback_Reset(t *testing.T) {
	c := NewProtocolMetricsCollector()

	c.RecordNLIFallback()
	c.RecordNLIFallback()

	snap := c.Snapshot()
	if snap.NLIFallbackCount != 2 {
		t.Errorf("before reset: NLIFallbackCount = %d, want 2", snap.NLIFallbackCount)
	}

	c.Reset()

	snap = c.Snapshot()
	if snap.NLIFallbackCount != 0 {
		t.Errorf("after reset: NLIFallbackCount = %d, want 0", snap.NLIFallbackCount)
	}
}

func TestProtocolMetrics_CodeOutcomeCount(t *testing.T) {
	c := NewProtocolMetricsCollector()

	c.RecordOutcomeWithTier("code-a", 1, 0.85)
	c.RecordOutcomeWithTier("code-a", 1, 0.90)
	c.RecordOutcomeWithTier("code-b", 2, 0.50)

	snap := c.Snapshot()
	if snap.CodeOutcomeCount["code-a"] != 2 {
		t.Errorf("code-a outcome count = %d, want 2", snap.CodeOutcomeCount["code-a"])
	}
	if snap.CodeOutcomeCount["code-b"] != 1 {
		t.Errorf("code-b outcome count = %d, want 1", snap.CodeOutcomeCount["code-b"])
	}
}
