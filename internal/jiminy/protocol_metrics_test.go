package jiminy

import (
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
