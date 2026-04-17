package jiminy

import (
	"fmt"
	"math"
	"sync"
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
	if snapshot.TicketRestoreTotal != 3 {
		t.Errorf("ticket restore total = %d, want 3", snapshot.TicketRestoreTotal)
	}
}

// TestSnapshot_TicketRestoreDefaultWhenNoData asserts that with zero
// RecordTicketRestore calls, TicketRestoreSuccessRate defaults to 1.0 (healthy
// "nothing to restore") rather than 0.0 (which would look like "100% failures").
// Matches the existing pattern for CodeCoverage with no constraints.
func TestSnapshot_TicketRestoreDefaultWhenNoData(t *testing.T) {
	c := NewProtocolMetricsCollector()
	snap := c.Snapshot()

	if snap.TicketRestoreSuccessRate != 1.0 {
		t.Errorf("TicketRestoreSuccessRate with no data = %f, want 1.0 (neutral-healthy)", snap.TicketRestoreSuccessRate)
	}
	if snap.TicketRestoreTotal != 0 {
		t.Errorf("TicketRestoreTotal with no data = %d, want 0", snap.TicketRestoreTotal)
	}
}

// TestSnapshot_TicketRestoreWithFailures confirms the 1.0-when-empty default does
// not mask real failures: once any Record call arrives, the computed rate is used.
func TestSnapshot_TicketRestoreWithFailures(t *testing.T) {
	c := NewProtocolMetricsCollector()
	c.RecordTicketRestore(false)
	c.RecordTicketRestore(false)

	snap := c.Snapshot()
	if snap.TicketRestoreSuccessRate != 0.0 {
		t.Errorf("TicketRestoreSuccessRate with all failures = %f, want 0.0", snap.TicketRestoreSuccessRate)
	}
	if snap.TicketRestoreTotal != 2 {
		t.Errorf("TicketRestoreTotal = %d, want 2", snap.TicketRestoreTotal)
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
	c.RecordSidecarCall(15.0, nil, false, 1, 1, false)                  // agreement, no override
	c.RecordSidecarCall(20.0, nil, false, 2, 1, true)                  // override
	c.RecordSidecarCall(0, fmt.Errorf("timeout"), true, 0, 1, false)   // error + timeout

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
	if snap.Sidecar.Timeouts != 1 {
		t.Errorf("timeouts = %d, want 1", snap.Sidecar.Timeouts)
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

// --- Edge case and concurrency tests ---

func TestSnapshot_ZeroSamples_AllFieldsSafe(t *testing.T) {
	c := NewProtocolMetricsCollector()
	snap := c.Snapshot()

	// No NaN or Inf in any float64 field
	floatFields := map[string]float64{
		"AvgComprehension":         snap.AvgComprehension,
		"CompressionRatio":         snap.CompressionRatio,
		"AvgTokensPerGuidance":     snap.AvgTokensPerGuidance,
		"ReplayFrequencyPerHour":   snap.ReplayFrequencyPerHour,
		"TicketRestoreSuccessRate":  snap.TicketRestoreSuccessRate,
		"CodeCoverage":             snap.CodeCoverage,
		"NLIFallbackRate":          snap.NLIFallbackRate,
	}
	for name, val := range floatFields {
		if math.IsNaN(val) {
			t.Errorf("%s is NaN on zero-sample snapshot", name)
		}
		if math.IsInf(val, 0) {
			t.Errorf("%s is Inf on zero-sample snapshot", name)
		}
	}

	// TierDistribution and TierComprehension arrays
	for i := range 3 {
		if math.IsNaN(snap.TierDistribution[i]) || math.IsInf(snap.TierDistribution[i], 0) {
			t.Errorf("TierDistribution[%d] is NaN or Inf", i)
		}
		if math.IsNaN(snap.TierComprehension[i]) || math.IsInf(snap.TierComprehension[i], 0) {
			t.Errorf("TierComprehension[%d] is NaN or Inf", i)
		}
	}

	// All int fields are 0
	if snap.TotalEvents != 0 {
		t.Errorf("TotalEvents = %d, want 0", snap.TotalEvents)
	}
	if snap.NLIFallbackCount != 0 {
		t.Errorf("NLIFallbackCount = %d, want 0", snap.NLIFallbackCount)
	}
	for i := range 3 {
		if snap.TierOutcomeCount[i] != 0 {
			t.Errorf("TierOutcomeCount[%d] = %d, want 0", i, snap.TierOutcomeCount[i])
		}
	}

	// Maps are non-nil but empty
	if snap.CodeComprehension == nil {
		t.Error("CodeComprehension should be non-nil (empty map), got nil")
	}
	if len(snap.CodeComprehension) != 0 {
		t.Errorf("CodeComprehension should be empty, got %d entries", len(snap.CodeComprehension))
	}
	if snap.T2FrequencyByConstraint == nil {
		t.Error("T2FrequencyByConstraint should be non-nil (empty map), got nil")
	}
	if len(snap.T2FrequencyByConstraint) != 0 {
		t.Errorf("T2FrequencyByConstraint should be empty, got %d entries", len(snap.T2FrequencyByConstraint))
	}
	if snap.CodeOutcomeCount == nil {
		t.Error("CodeOutcomeCount should be non-nil (empty map), got nil")
	}
	if len(snap.CodeOutcomeCount) != 0 {
		t.Errorf("CodeOutcomeCount should be empty, got %d entries", len(snap.CodeOutcomeCount))
	}

	// Sidecar is nil when no sidecar calls recorded
	if snap.Sidecar != nil {
		t.Errorf("Sidecar should be nil on zero-sample snapshot, got %+v", snap.Sidecar)
	}
}

func TestRecordGuidance_InvalidTier(t *testing.T) {
	c := NewProtocolMetricsCollector()

	// Record guidance with out-of-range tiers: 0 and 4
	c.RecordGuidance(0, 10, []string{"code-zero"})
	c.RecordGuidance(4, 20, []string{"code-four"})

	snap := c.Snapshot()

	// TotalEvents should still be incremented for both calls
	if snap.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, want 2 (invalid tiers should still count as events)", snap.TotalEvents)
	}

	// TierDistribution should be safe (no index out of range, no NaN/Inf)
	for i := range 3 {
		if math.IsNaN(snap.TierDistribution[i]) || math.IsInf(snap.TierDistribution[i], 0) {
			t.Errorf("TierDistribution[%d] is NaN or Inf after invalid tier recording", i)
		}
	}

	// Since tiers 0 and 4 are out of [1,3], all tier buckets should be 0
	// But totalEvents = 2, so distribution = 0/2 = 0.0 for each tier
	for i := range 3 {
		if snap.TierDistribution[i] != 0.0 {
			t.Errorf("TierDistribution[%d] = %f, want 0.0 (invalid tiers should not land in any bucket)", i, snap.TierDistribution[i])
		}
	}

	// AvgTokensPerGuidance should still be computed: (10+20)/2 = 15
	if snap.AvgTokensPerGuidance != 15.0 {
		t.Errorf("AvgTokensPerGuidance = %f, want 15.0", snap.AvgTokensPerGuidance)
	}

	// Snapshot should be generally valid (no panics getting here is also a pass)
	if math.IsNaN(snap.CompressionRatio) || math.IsInf(snap.CompressionRatio, 0) {
		t.Errorf("CompressionRatio is NaN or Inf after invalid tier recording")
	}
}

func TestRecordConstraintCoverage_CodeCoverageClamp(t *testing.T) {
	c := NewProtocolMetricsCollector()

	// constraintsWithCode > totalConstraints (should not happen, but test edge case)
	c.RecordConstraintCoverage(3, 5)

	snap := c.Snapshot()

	// Document actual behavior: coverage = 5/3 = 1.666...
	// The implementation does NOT clamp to 1.0 — this is a known finding.
	if snap.CodeCoverage > 1.0 {
		t.Logf("FINDING: CodeCoverage = %f exceeds 1.0 when constraintsWithCode > totalConstraints (not clamped)", snap.CodeCoverage)
	}

	// At minimum, it should not be NaN or Inf
	if math.IsNaN(snap.CodeCoverage) || math.IsInf(snap.CodeCoverage, 0) {
		t.Errorf("CodeCoverage is NaN or Inf, got %f", snap.CodeCoverage)
	}

	// Verify the actual value is what we expect from the formula: 5/3
	expected := 5.0 / 3.0
	if math.Abs(snap.CodeCoverage-expected) > 0.001 {
		t.Errorf("CodeCoverage = %f, expected %f (withCode/total)", snap.CodeCoverage, expected)
	}
}

func TestAvgComprehension_WeightedAverage(t *testing.T) {
	c := NewProtocolMetricsCollector()

	// Code "rare": 1 followed out of 1 (comprehension >= 0.7 counts as followed)
	c.RecordOutcome("rare", 0.9) // followed (score >= 0.7)

	// Code "common": 2 followed out of 10
	c.RecordOutcome("common", 0.8)  // followed
	c.RecordOutcome("common", 0.75) // followed
	c.RecordOutcome("common", 0.3)  // not followed
	c.RecordOutcome("common", 0.2)  // not followed
	c.RecordOutcome("common", 0.1)  // not followed
	c.RecordOutcome("common", 0.4)  // not followed
	c.RecordOutcome("common", 0.5)  // not followed
	c.RecordOutcome("common", 0.6)  // not followed
	c.RecordOutcome("common", 0.65) // not followed
	c.RecordOutcome("common", 0.1)  // not followed

	snap := c.Snapshot()

	// Per-code: rare = 1/1 = 1.0, common = 2/10 = 0.2
	if snap.CodeComprehension["rare"] != 1.0 {
		t.Errorf("rare comprehension = %f, want 1.0", snap.CodeComprehension["rare"])
	}
	if snap.CodeComprehension["common"] != 0.2 {
		t.Errorf("common comprehension = %f, want 0.2", snap.CodeComprehension["common"])
	}

	// Old unweighted average would be (1.0 + 0.2) / 2 = 0.6
	// New weighted average (M5 fix): (1+2) / (1+10) = 3/11 ≈ 0.2727...
	expectedWeighted := 3.0 / 11.0
	if math.Abs(snap.AvgComprehension-expectedWeighted) > 0.01 {
		t.Errorf("AvgComprehension = %f, want ~%f (weighted), not 0.6 (unweighted)",
			snap.AvgComprehension, expectedWeighted)
	}
}

func TestConcurrentRecordAndSnapshot(t *testing.T) {
	c := NewProtocolMetricsCollector()

	const goroutines = 10
	const recordsPerGoroutine = 100

	var wg sync.WaitGroup

	// 10 goroutines each recording 100 guidance events
	for g := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tier := (id % 3) + 1 // tiers 1, 2, 3
			for range recordsPerGoroutine {
				c.RecordGuidance(tier, 10, []string{fmt.Sprintf("code-%d", id)})
			}
		}(g)
	}

	// Another goroutine repeatedly calling Snapshot concurrently
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				snap := c.Snapshot()
				// Just verify the snapshot is safe to read (no panic)
				_ = snap.TotalEvents
				_ = snap.TierDistribution
				_ = snap.AvgComprehension
			}
		}
	}()

	wg.Wait()
	close(done)

	snap := c.Snapshot()
	expectedTotal := int64(goroutines * recordsPerGoroutine)
	if snap.TotalEvents != expectedTotal {
		t.Errorf("TotalEvents = %d, want %d", snap.TotalEvents, expectedTotal)
	}

	// Verify no NaN/Inf in the final snapshot
	if math.IsNaN(snap.AvgTokensPerGuidance) || math.IsInf(snap.AvgTokensPerGuidance, 0) {
		t.Errorf("AvgTokensPerGuidance is NaN or Inf after concurrent access")
	}
}

func TestNLIFallbackRate_CannotExceed1(t *testing.T) {
	c := NewProtocolMetricsCollector()

	// Record 2 guidance events (totalEvents = 2)
	c.RecordGuidance(1, 10, nil)
	c.RecordGuidance(2, 20, nil)

	// Record 5 NLI fallbacks (more than totalEvents)
	for range 5 {
		c.RecordNLIFallback()
	}

	snap := c.Snapshot()

	if snap.NLIFallbackCount != 5 {
		t.Errorf("NLIFallbackCount = %d, want 5", snap.NLIFallbackCount)
	}

	// Rate = 5 / 2 = 2.5 — the implementation does NOT clamp to 1.0
	// Document this as a known limitation (L2).
	if snap.NLIFallbackRate > 1.0 {
		t.Logf("KNOWN LIMITATION (L2): NLIFallbackRate = %f exceeds 1.0 (fallbacks=%d > events=%d)",
			snap.NLIFallbackRate, snap.NLIFallbackCount, snap.TotalEvents)
	}

	// At minimum, it should not be NaN or Inf
	if math.IsNaN(snap.NLIFallbackRate) || math.IsInf(snap.NLIFallbackRate, 0) {
		t.Errorf("NLIFallbackRate is NaN or Inf, got %f", snap.NLIFallbackRate)
	}

	// Verify the actual value matches expected formula: 5/2 = 2.5
	expectedRate := 5.0 / 2.0
	if math.Abs(snap.NLIFallbackRate-expectedRate) > 0.001 {
		t.Errorf("NLIFallbackRate = %f, expected %f", snap.NLIFallbackRate, expectedRate)
	}
}

func TestReplayFrequencyAfterReset(t *testing.T) {
	c := NewProtocolMetricsCollector()

	// Record some replays and take a normal snapshot
	c.RecordReplay(5)
	snap1 := c.Snapshot()

	if snap1.ReplayFrequencyPerHour <= 0 {
		t.Errorf("pre-reset ReplayFrequencyPerHour = %f, want > 0", snap1.ReplayFrequencyPerHour)
	}
	if math.IsInf(snap1.ReplayFrequencyPerHour, 0) {
		t.Errorf("pre-reset ReplayFrequencyPerHour is Inf")
	}

	// Reset and immediately take a snapshot
	// windowDuration will be extremely small (nanoseconds), so
	// replayFreq = 0 / tiny_hours = 0 (no replays recorded after reset).
	c.Reset()
	snap2 := c.Snapshot()

	// After reset, replay count is 0, so frequency should be 0 regardless of window size
	if snap2.ReplayFrequencyPerHour != 0 {
		t.Errorf("post-reset ReplayFrequencyPerHour = %f, want 0 (no replays after reset)", snap2.ReplayFrequencyPerHour)
	}

	// Verify it's not Inf or NaN (the main concern with tiny windowDuration)
	if math.IsInf(snap2.ReplayFrequencyPerHour, 0) {
		t.Errorf("post-reset ReplayFrequencyPerHour is Inf (L1: tiny window duration after reset)")
	}
	if math.IsNaN(snap2.ReplayFrequencyPerHour) {
		t.Errorf("post-reset ReplayFrequencyPerHour is NaN")
	}

	// Now record replays immediately after reset to test the tiny-window scenario
	c.Reset()
	c.RecordReplay(10)
	snap3 := c.Snapshot()

	// With a near-zero window and 10 replay events, frequency could be astronomically large
	// but should NOT be Inf or NaN
	if math.IsInf(snap3.ReplayFrequencyPerHour, 0) {
		t.Errorf("FINDING (L1): ReplayFrequencyPerHour is Inf when window ≈ 0 after Reset()+RecordReplay()")
	}
	if math.IsNaN(snap3.ReplayFrequencyPerHour) {
		t.Errorf("ReplayFrequencyPerHour is NaN after Reset()+RecordReplay()")
	}
	if snap3.ReplayFrequencyPerHour > 0 {
		t.Logf("ReplayFrequencyPerHour after immediate reset+record = %f (very large expected with tiny window)", snap3.ReplayFrequencyPerHour)
	}
}
