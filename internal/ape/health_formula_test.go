package ape

import (
	"math"
	"testing"
)

// floatClose reports whether |a-b| <= eps.
func floatClose(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// TestDefaultHealthWeights_SumToOne documents the default-weight convention
// (sum == 1.0). The formula does not require this — it normalises — but the
// convention keeps the default interpretable as "share of contribution".
func TestDefaultHealthWeights_SumToOne(t *testing.T) {
	w := DefaultHealthWeights()
	sum := w.Retrieval + w.Memory + w.Edge + w.Task + w.Guidance + w.Protocol + w.Synergy
	if !floatClose(sum, 1.0, 0.001) {
		t.Errorf("DefaultHealthWeights must sum to ~1.0, got %f", sum)
	}
}

// TestComputeOverallHealth_AllDimensions: every dimension present with
// confidence=1 yields the straight weighted sum of scores.
func TestComputeOverallHealth_AllDimensions(t *testing.T) {
	r := &SelfAssessmentReport{
		RetrievalQuality: 0.8, RetrievalConfidence: 1,
		MemoryHealth: 0.9, MemoryConfidence: 1,
		EdgeHealth: 0.7, EdgeConfidence: 1,
		TaskPerformance: 0.6, TaskConfidence: 1,
		GuidanceHealth: 0.5, GuidanceConfidence: 1,
		ProtocolHealth: 0.85, ProtocolConfidence: 1,
		SynergyHealth: 0.4, SynergyConfidence: 1,
	}
	w := DefaultHealthWeights()
	want := w.Retrieval*0.8 + w.Memory*0.9 + w.Edge*0.7 + w.Task*0.6 +
		w.Guidance*0.5 + w.Protocol*0.85 + w.Synergy*0.4
	got := ComputeOverallHealth(r)
	if !floatClose(got, want, 0.0001) {
		t.Errorf("7-dim: got %f, want %f", got, want)
	}
}

// TestComputeOverallHealth_ZeroConfidenceExcluded: a dimension with c=0
// contributes nothing to either numerator or denominator, so the result is
// the weighted average over the remaining dimensions only.
func TestComputeOverallHealth_ZeroConfidenceExcluded(t *testing.T) {
	r := &SelfAssessmentReport{
		RetrievalQuality: 1.0, RetrievalConfidence: 1,
		MemoryHealth: 1.0, MemoryConfidence: 1,
		EdgeHealth: 1.0, EdgeConfidence: 1,
		TaskPerformance: 1.0, TaskConfidence: 1,
		// Guidance excluded (c=0) despite score being populated
		GuidanceHealth: 0.0, GuidanceConfidence: 0,
		ProtocolHealth: 1.0, ProtocolConfidence: 1,
		SynergyHealth: 1.0, SynergyConfidence: 1,
	}
	got := ComputeOverallHealth(r)
	if !floatClose(got, 1.0, 0.0001) {
		t.Errorf("excluded dim (c=0) with score=0 should not drag result; got %f want 1.0", got)
	}
}

// TestComputeOverallHealth_AllZeroReturnsZero: when no dimension is usable
// (all weights zero or all confidences zero), the result is 0 by contract.
func TestComputeOverallHealth_AllZeroReturnsZero(t *testing.T) {
	r := &SelfAssessmentReport{} // every field zero
	got := ComputeOverallHealth(r)
	if got != 0 {
		t.Errorf("all-zero report must yield 0, got %f", got)
	}
}

// TestComputeOverallHealth_FullyHealthy: all scores=1, all confidences=1 →
// exactly 1.0, independent of the weight vector.
func TestComputeOverallHealth_FullyHealthy(t *testing.T) {
	r := &SelfAssessmentReport{
		RetrievalQuality: 1, RetrievalConfidence: 1,
		MemoryHealth: 1, MemoryConfidence: 1,
		EdgeHealth: 1, EdgeConfidence: 1,
		TaskPerformance: 1, TaskConfidence: 1,
		GuidanceHealth: 1, GuidanceConfidence: 1,
		ProtocolHealth: 1, ProtocolConfidence: 1,
		SynergyHealth: 1, SynergyConfidence: 1,
	}
	got := ComputeOverallHealth(r)
	if !floatClose(got, 1.0, 0.0001) {
		t.Errorf("fully-healthy must yield 1.0, got %f", got)
	}
	// Same under a skewed weight vector.
	skewed := HealthWeights{Retrieval: 10, Memory: 0.1, Edge: 0.1, Task: 0.1, Guidance: 0.1, Protocol: 0.1, Synergy: 0.1}
	got2 := ComputeOverallHealthWith(r, skewed)
	if !floatClose(got2, 1.0, 0.0001) {
		t.Errorf("fully-healthy under skewed weights must yield 1.0, got %f", got2)
	}
}

// TestComputeOverallHealth_PreservesThresholds: a degraded well-instrumented
// system must land below 0.4; a healthy one must land at or above 0.7. This
// locks the dashboard red/yellow/green thresholds against formula changes.
func TestComputeOverallHealth_PreservesThresholds(t *testing.T) {
	// Degraded — all scores at 0.3, all confidences at 1.
	degraded := &SelfAssessmentReport{
		RetrievalQuality: 0.3, RetrievalConfidence: 1,
		MemoryHealth: 0.3, MemoryConfidence: 1,
		EdgeHealth: 0.3, EdgeConfidence: 1,
		TaskPerformance: 0.3, TaskConfidence: 1,
		GuidanceHealth: 0.3, GuidanceConfidence: 1,
		ProtocolHealth: 0.3, ProtocolConfidence: 1,
		SynergyHealth: 0.3, SynergyConfidence: 1,
	}
	if got := ComputeOverallHealth(degraded); got >= 0.4 {
		t.Errorf("degraded system must land <0.4, got %f", got)
	}
	// Healthy — all scores at 0.8.
	healthy := &SelfAssessmentReport{
		RetrievalQuality: 0.8, RetrievalConfidence: 1,
		MemoryHealth: 0.8, MemoryConfidence: 1,
		EdgeHealth: 0.8, EdgeConfidence: 1,
		TaskPerformance: 0.8, TaskConfidence: 1,
		GuidanceHealth: 0.8, GuidanceConfidence: 1,
		ProtocolHealth: 0.8, ProtocolConfidence: 1,
		SynergyHealth: 0.8, SynergyConfidence: 1,
	}
	if got := ComputeOverallHealth(healthy); got < 0.7 {
		t.Errorf("healthy system must land >=0.7, got %f", got)
	}
}

// TestComputeOverallHealth_HybridReliabilityPriors: verifies that under a
// specific known score vector, the default weights produce an aggregate that
// reflects the reliability priors (high-reliability dims dominate).
func TestComputeOverallHealth_HybridReliabilityPriors(t *testing.T) {
	// Retrieval (low-reliability) scores perfectly; everything else is 0.
	// Under default weights, Retrieval=0.08 → overall should be ~0.08.
	retrievalOnly := &SelfAssessmentReport{
		RetrievalQuality: 1.0, RetrievalConfidence: 1,
		MemoryConfidence: 1, EdgeConfidence: 1, TaskConfidence: 1,
		GuidanceConfidence: 1, ProtocolConfidence: 1, SynergyConfidence: 1,
	}
	got := ComputeOverallHealth(retrievalOnly)
	if !floatClose(got, 0.08, 0.001) {
		t.Errorf("retrieval-only=1 should yield Retrieval weight (~0.08), got %f", got)
	}
	// Protocol (high-reliability) scores perfectly; everything else is 0.
	// Default Protocol weight = 0.20 → overall should be ~0.20.
	protocolOnly := &SelfAssessmentReport{
		RetrievalConfidence: 1, MemoryConfidence: 1, EdgeConfidence: 1, TaskConfidence: 1,
		GuidanceConfidence: 1, SynergyConfidence: 1,
		ProtocolHealth: 1.0, ProtocolConfidence: 1,
	}
	got2 := ComputeOverallHealth(protocolOnly)
	if !floatClose(got2, 0.20, 0.001) {
		t.Errorf("protocol-only=1 should yield Protocol weight (~0.20), got %f", got2)
	}
	// Confirms Protocol > Retrieval under the new priors (was 0.13 vs 0.18 before).
	if got2 <= got {
		t.Errorf("hybrid priors must weight Protocol (%f) above Retrieval (%f)", got2, got)
	}
}

// TestComputeOverallHealth_PartialInstrumentation: simulates a fresh system
// where only Memory and Edge have meaningful data (others c=0). Result should
// be the weighted average over just those two — no penalty for absence.
func TestComputeOverallHealth_PartialInstrumentation(t *testing.T) {
	r := &SelfAssessmentReport{
		MemoryHealth: 0.9, MemoryConfidence: 1,
		EdgeHealth: 0.8, EdgeConfidence: 1,
		// Everything else has c=0 → excluded.
	}
	w := DefaultHealthWeights()
	want := (w.Memory*0.9 + w.Edge*0.8) / (w.Memory + w.Edge)
	got := ComputeOverallHealth(r)
	if !floatClose(got, want, 0.0001) {
		t.Errorf("partial instrumentation: got %f, want %f", got, want)
	}
}

// TestComputeOverallHealthWith_DisabledDimension: zero weight removes a
// dimension from both numerator and denominator — operator can tune the
// formula to ignore a dimension entirely.
func TestComputeOverallHealthWith_DisabledDimension(t *testing.T) {
	r := &SelfAssessmentReport{
		RetrievalQuality: 1.0, RetrievalConfidence: 1,
		MemoryHealth: 0.5, MemoryConfidence: 1,
	}
	// Disable Retrieval by zeroing its weight.
	w := HealthWeights{Retrieval: 0, Memory: 1}
	got := ComputeOverallHealthWith(r, w)
	if !floatClose(got, 0.5, 0.0001) {
		t.Errorf("disabled dim should be ignored; got %f want 0.5", got)
	}
}

// TestComputeOverallHealthWith_PartialConfidence: a dimension with 0<c<1
// contributes proportionally — used to validate the smooth data-sufficiency
// ramp (vs the old hard branch-table cutoff).
func TestComputeOverallHealthWith_PartialConfidence(t *testing.T) {
	r := &SelfAssessmentReport{
		MemoryHealth: 1.0, MemoryConfidence: 1.0,
		EdgeHealth: 0.0, EdgeConfidence: 0.5, // half-confidence
	}
	w := HealthWeights{Memory: 1, Edge: 1}
	// num = 1*1*1 + 1*0.5*0 = 1; denom = 1*1 + 1*0.5 = 1.5 → 0.6667
	got := ComputeOverallHealthWith(r, w)
	want := 1.0 / 1.5
	if !floatClose(got, want, 0.0001) {
		t.Errorf("partial confidence: got %f, want %f", got, want)
	}
}
