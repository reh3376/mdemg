package ape

import (
	"math"
	"testing"

	"mdemg/internal/config"
)

// assertClose checks that got is within epsilon of want.
func assertClose(t *testing.T, got, want, epsilon float64, msg string) {
	t.Helper()
	if math.Abs(got-want) > epsilon {
		t.Errorf("%s: got %f, want %f (epsilon %f)", msg, got, want, epsilon)
	}
}

// newTestAssessor returns an Assessor with synergy target config for testing.
func newTestAssessor() *Assessor {
	return &Assessor{cfg: config.Config{
		SynergyTargetClaudeLines: 100,
		SynergyTargetMemoryLines: 80,
	}}
}

// ─── Health weight sum tests ───

func TestHealthWeights_7Dim_SumToOne(t *testing.T) {
	sum := 0.18 + 0.18 + 0.13 + 0.13 + 0.13 + 0.13 + 0.12
	assertClose(t, sum, 1.0, 0.001, "7-dim weights must sum to 1.0")
}

func TestHealthWeights_6Dim_SumToOne(t *testing.T) {
	sum := 0.20 + 0.20 + 0.15 + 0.15 + 0.15 + 0.15
	assertClose(t, sum, 1.0, 0.001, "6-dim weights must sum to 1.0")
}

func TestHealthWeights_5Dim_SumToOne(t *testing.T) {
	sum := 0.25 + 0.25 + 0.20 + 0.15 + 0.15
	assertClose(t, sum, 1.0, 0.001, "5-dim weights must sum to 1.0")
}

func TestHealthWeights_4Dim_SumToOne(t *testing.T) {
	sum := 0.30 + 0.25 + 0.25 + 0.20
	assertClose(t, sum, 1.0, 0.001, "4-dim weights must sum to 1.0")
}

// ─── scoreRetrieval tests ───

func TestScoreRetrieval_AllPhases(t *testing.T) {
	a := newTestAssessor()
	tests := []struct {
		phase string
		want  float64
	}{
		{"cold", 0.3},
		{"learning", 0.6},
		{"warm", 0.9},
		{"saturated", 0.7},
		{"", 0.5},
		{"unknown", 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			r := &SelfAssessmentReport{LearningPhase: tt.phase}
			got, _ := a.scoreRetrieval(r)
			assertClose(t, got, tt.want, 0.001, "scoreRetrieval("+tt.phase+")")
		})
	}
}

// ─── scoreMemory tests ───

func TestScoreMemory_Perfect(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		OrphanRatio:         0.0,
		CorrectionRate:      0.0,
		ConsolidationAgeSec: 0,
	}
	got, _ := a.scoreMemory(r)
	assertClose(t, got, 1.0, 0.001, "scoreMemory perfect")
}

func TestScoreMemory_HighOrphans(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		OrphanRatio:         0.25,
		CorrectionRate:      0.0,
		ConsolidationAgeSec: 0,
	}
	got, _ := a.scoreMemory(r)
	// OrphanRatio > 0.2 → -0.3, no other penalties → 0.7
	assertClose(t, got, 0.7, 0.001, "scoreMemory high orphans")
}

func TestScoreMemory_MediumOrphans(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		OrphanRatio:         0.15,
		CorrectionRate:      0.0,
		ConsolidationAgeSec: 0,
	}
	got, _ := a.scoreMemory(r)
	// OrphanRatio > 0.1 → -0.1 → 0.9
	assertClose(t, got, 0.9, 0.001, "scoreMemory medium orphans")
}

func TestScoreMemory_AllPenalties(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		OrphanRatio:         0.15,  // > 0.1 → -0.1
		CorrectionRate:      0.20,  // > 0.15 → -0.2
		ConsolidationAgeSec: 90000, // > 86400 → -0.2
	}
	got, _ := a.scoreMemory(r)
	// 1.0 - 0.1 - 0.2 - 0.2 = 0.5
	assertClose(t, got, 0.5, 0.001, "scoreMemory all penalties (medium orphans)")
}

func TestScoreMemory_AllPenaltiesMax(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		OrphanRatio:         0.25,  // > 0.2 → -0.3
		CorrectionRate:      0.20,  // > 0.15 → -0.2
		ConsolidationAgeSec: 90000, // > 86400 → -0.2
	}
	got, _ := a.scoreMemory(r)
	// 1.0 - 0.3 - 0.2 - 0.2 = 0.3
	assertClose(t, got, 0.3, 0.001, "scoreMemory all penalties max")
}

func TestScoreMemory_ClampToZero(t *testing.T) {
	a := newTestAssessor()
	// This scenario can't actually go below 0 with the defined penalties
	// (max total penalty = 0.3 + 0.2 + 0.2 = 0.7, so minimum is 0.3).
	// But verify the clamp logic still holds — a score of 0.3 is the floor.
	r := &SelfAssessmentReport{
		OrphanRatio:         0.99,   // > 0.2 → -0.3
		CorrectionRate:      0.99,   // > 0.15 → -0.2
		ConsolidationAgeSec: 999999, // > 86400 → -0.2
	}
	got, _ := a.scoreMemory(r)
	// 1.0 - 0.3 - 0.2 - 0.2 = 0.3 (not negative, clamp not needed)
	if got < 0 {
		t.Errorf("scoreMemory returned negative: %f", got)
	}
	assertClose(t, got, 0.3, 0.001, "scoreMemory extreme penalties")
}

// ─── scoreEdge tests ───

func TestScoreEdge_NoEdges(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		EdgeCount:           0,
		EdgesBelowThreshold: 0,
		EdgeWeightEntropy:   0.0, // < 0.5 → -0.2
	}
	got, _ := a.scoreEdge(r)
	// No edges: below-ratio branch skipped. Entropy < 0.5 → -0.2. Result: 0.8
	assertClose(t, got, 0.8, 0.001, "scoreEdge no edges")
}

func TestScoreEdge_HealthyEdges(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		EdgeCount:           100,
		EdgesBelowThreshold: 10,  // 10% < 30%, no penalty
		EdgeWeightEntropy:   0.8, // >= 0.5, no penalty
	}
	got, _ := a.scoreEdge(r)
	assertClose(t, got, 1.0, 0.001, "scoreEdge healthy edges")
}

func TestScoreEdge_HighBelowRatio(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		EdgeCount:           100,
		EdgesBelowThreshold: 40,  // 40% > 30% → -0.3
		EdgeWeightEntropy:   0.8, // >= 0.5, no penalty
	}
	got, _ := a.scoreEdge(r)
	assertClose(t, got, 0.7, 0.001, "scoreEdge high below ratio")
}

func TestScoreEdge_LowEntropy(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		EdgeCount:           100,
		EdgesBelowThreshold: 10,  // 10% < 30%, no penalty
		EdgeWeightEntropy:   0.3, // < 0.5 → -0.2
	}
	got, _ := a.scoreEdge(r)
	assertClose(t, got, 0.8, 0.001, "scoreEdge low entropy")
}

func TestScoreEdge_BothPenalties(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		EdgeCount:           100,
		EdgesBelowThreshold: 50,  // 50% > 30% → -0.3
		EdgeWeightEntropy:   0.2, // < 0.5 → -0.2
	}
	got, _ := a.scoreEdge(r)
	// 1.0 - 0.3 - 0.2 = 0.5
	assertClose(t, got, 0.5, 0.001, "scoreEdge both penalties")
}

func TestScoreEdge_ClampToZero(t *testing.T) {
	a := newTestAssessor()
	// Max penalty is 0.3 + 0.2 = 0.5, so minimum is 0.5; can't go negative.
	// Verify it never goes below 0.
	r := &SelfAssessmentReport{
		EdgeCount:           10,
		EdgesBelowThreshold: 10,  // 100% > 30% → -0.3
		EdgeWeightEntropy:   0.0, // < 0.5 → -0.2
	}
	got, _ := a.scoreEdge(r)
	if got < 0 {
		t.Errorf("scoreEdge returned negative: %f", got)
	}
	assertClose(t, got, 0.5, 0.001, "scoreEdge max penalty clamp")
}

// ─── scoreTask tests ───

func TestScoreTask_NoTasks(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		VolatileCount:  0,
		PermanentCount: 0,
	}
	got, _ := a.scoreTask(r)
	assertClose(t, got, 0.5, 0.001, "scoreTask no tasks")
}

func TestScoreTask_AllPermanent(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		VolatileCount:  0,
		PermanentCount: 50,
	}
	got, _ := a.scoreTask(r)
	assertClose(t, got, 1.0, 0.001, "scoreTask all permanent")
}

func TestScoreTask_AllVolatile(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		VolatileCount:  100,
		PermanentCount: 0,
	}
	got, _ := a.scoreTask(r)
	assertClose(t, got, 0.0, 0.001, "scoreTask all volatile")
}

func TestScoreTask_Mixed(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		VolatileCount:  30,
		PermanentCount: 70,
	}
	got, _ := a.scoreTask(r)
	// 70/100 = 0.7
	assertClose(t, got, 0.7, 0.001, "scoreTask mixed 70/30")
}

func TestScoreTask_HalfAndHalf(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		VolatileCount:  50,
		PermanentCount: 50,
	}
	got, _ := a.scoreTask(r)
	assertClose(t, got, 0.5, 0.001, "scoreTask 50/50")
}

// ─── scoreGuidance tests ───

func TestScoreGuidance_Perfect(t *testing.T) {
	a := newTestAssessor()
	stats := JiminyStatsResult{
		FollowRate:        1.0,
		ConstraintEffRate: 1.0,
		SourceDiversity:   1.0,
	}
	got, _ := a.scoreGuidance(stats)
	// 0.5*1.0 + 0.3*1.0 + 0.2*1.0 = 1.0
	assertClose(t, got, 1.0, 0.001, "scoreGuidance perfect")
}

func TestScoreGuidance_Zero(t *testing.T) {
	a := newTestAssessor()
	stats := JiminyStatsResult{
		FollowRate:        0.0,
		ConstraintEffRate: 0.0,
		SourceDiversity:   0.0,
	}
	got, _ := a.scoreGuidance(stats)
	assertClose(t, got, 0.0, 0.001, "scoreGuidance zero")
}

func TestScoreGuidance_Mixed(t *testing.T) {
	a := newTestAssessor()
	stats := JiminyStatsResult{
		FollowRate:        0.8,
		ConstraintEffRate: 0.6,
		SourceDiversity:   0.4,
	}
	got, _ := a.scoreGuidance(stats)
	// 0.5*0.8 + 0.3*0.6 + 0.2*0.4 = 0.40 + 0.18 + 0.08 = 0.66
	assertClose(t, got, 0.66, 0.001, "scoreGuidance mixed")
}

func TestScoreGuidance_ClampInputs(t *testing.T) {
	a := newTestAssessor()
	stats := JiminyStatsResult{
		FollowRate:        2.0,  // clamped to 1.0
		ConstraintEffRate: -0.5, // clamped to 0.0
		SourceDiversity:   1.5,  // clamped to 1.0
	}
	got, _ := a.scoreGuidance(stats)
	// 0.5*1.0 + 0.3*0.0 + 0.2*1.0 = 0.50 + 0.00 + 0.20 = 0.70
	assertClose(t, got, 0.70, 0.001, "scoreGuidance clamped inputs")
}

// ─── scoreSynergy tests ───

func TestScoreSynergy_NotHealthy(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy: false,
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 0.0, 0.001, "scoreSynergy not healthy")
}

func TestScoreSynergy_Perfect(t *testing.T) {
	a := newTestAssessor()
	// cfg.SynergyTargetClaudeLines=100, SynergyTargetMemoryLines=80
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  90,  // < 100, no penalty
		SynergyLinesMemory:  70,  // < 80, no penalty
		SynergyOverflowRate: 2.0, // < 5, no penalty
		SynergyOverlapScore: 0.3, // < 0.5, no penalty
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 1.0, 0.001, "scoreSynergy perfect")
}

func TestScoreSynergy_SlightlyBloatedClaude(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  120, // > 100 but <= 150 → -0.1
		SynergyLinesMemory:  70,
		SynergyOverflowRate: 2.0,
		SynergyOverlapScore: 0.3,
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 0.9, 0.001, "scoreSynergy slightly bloated claude")
}

func TestScoreSynergy_VeryBloatedClaude(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  200, // > 100+50=150 → -0.3
		SynergyLinesMemory:  70,
		SynergyOverflowRate: 2.0,
		SynergyOverlapScore: 0.3,
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 0.7, 0.001, "scoreSynergy very bloated claude")
}

func TestScoreSynergy_BloatedMemoryV2(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  90,
		SynergyLinesMemory:  200, // > 80+60=140 → -0.3
		SynergyOverflowRate: 2.0,
		SynergyOverlapScore: 0.3,
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 0.7, 0.001, "scoreSynergy bloated memory")
}

func TestScoreSynergy_HighOverflowV2(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  90,
		SynergyLinesMemory:  70,
		SynergyOverflowRate: 15.0, // > 10 → -0.3
		SynergyOverlapScore: 0.3,
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 0.7, 0.001, "scoreSynergy high overflow")
}

func TestScoreSynergy_MediumOverflow(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  90,
		SynergyLinesMemory:  70,
		SynergyOverflowRate: 7.0, // > 5 but <= 10 → -0.1
		SynergyOverlapScore: 0.3,
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 0.9, 0.001, "scoreSynergy medium overflow")
}

func TestScoreSynergy_HighOverlapV2(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  90,
		SynergyLinesMemory:  70,
		SynergyOverflowRate: 2.0,
		SynergyOverlapScore: 0.6, // > 0.5 → -0.2
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 0.8, 0.001, "scoreSynergy high overlap")
}

func TestScoreSynergy_AllPenalties(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  200,  // > 150 → -0.3
		SynergyLinesMemory:  200,  // > 140 → -0.3
		SynergyOverflowRate: 15.0, // > 10 → -0.3
		SynergyOverlapScore: 0.8,  // > 0.5 → -0.2
	}
	got, _ := a.scoreSynergy(r)
	// 1.0 - 0.3 - 0.3 - 0.3 - 0.2 = -0.1 → clamped to 0.0
	assertClose(t, got, 0.0, 0.001, "scoreSynergy all penalties clamp to zero")
}

// ─── scoreProtocol tests ───

func TestScoreProtocol_Perfect(t *testing.T) {
	a := newTestAssessor()
	stats := ProtocolStatsResult{
		AvgComprehension:         1.0,
		NLIBiasAlert:             false, // calibration = 1.0
		CompressionRatio:         5.0,   // (5.0-1.0)/4.0 = 1.0
		CodeCoverage:             1.0,
		TicketRestoreSuccessRate: 1.0,
		ReplayFrequencyPerHour:   0.0, // penalty = 0, stability = 0.5*1.0 + 0.5*1.0 = 1.0
	}
	got, _ := a.scoreProtocol(stats)
	// 0.35*1.0 + 0.05*1.0 + 0.25*1.0 + 0.20*1.0 + 0.15*1.0 = 1.0
	assertClose(t, got, 1.0, 0.001, "scoreProtocol perfect")
}

func TestScoreProtocol_Zero(t *testing.T) {
	a := newTestAssessor()
	stats := ProtocolStatsResult{
		AvgComprehension:         0.0,
		NLIBiasAlert:             false, // calibration = 1.0 (not zero)
		CompressionRatio:         1.0,   // (1.0-1.0)/4.0 = 0.0
		CodeCoverage:             0.0,
		TicketRestoreSuccessRate: 0.0,
		ReplayFrequencyPerHour:   0.0, // stability = 0.5*0.0 + 0.5*1.0 = 0.5
	}
	got, _ := a.scoreProtocol(stats)
	// 0.35*0.0 + 0.05*1.0 + 0.25*0.0 + 0.20*0.0 + 0.15*0.5 = 0.05 + 0.075 = 0.125
	assertClose(t, got, 0.125, 0.001, "scoreProtocol mostly zero")
}

func TestScoreProtocol_NLIBiasAlert(t *testing.T) {
	a := newTestAssessor()
	stats := ProtocolStatsResult{
		AvgComprehension:         1.0,
		NLIBiasAlert:             true, // calibration = 0.3
		CompressionRatio:         5.0,  // compression = 1.0
		CodeCoverage:             1.0,
		TicketRestoreSuccessRate: 1.0,
		ReplayFrequencyPerHour:   0.0, // stability = 1.0
	}
	got, _ := a.scoreProtocol(stats)
	// 0.35*1.0 + 0.05*0.3 + 0.25*1.0 + 0.20*1.0 + 0.15*1.0 = 0.35+0.015+0.25+0.20+0.15 = 0.965
	assertClose(t, got, 0.965, 0.001, "scoreProtocol NLI bias alert")
}

func TestScoreProtocol_CompressionBelow1(t *testing.T) {
	a := newTestAssessor()
	stats := ProtocolStatsResult{
		AvgComprehension:         0.8,
		NLIBiasAlert:             false,
		CompressionRatio:         0.5, // (0.5-1.0)/4.0 = -0.125 → clamped to 0
		CodeCoverage:             0.8,
		TicketRestoreSuccessRate: 0.9,
		ReplayFrequencyPerHour:   2.0, // penalty = 2.0/10.0 = 0.2, stability = 0.5*0.9 + 0.5*0.8 = 0.85
	}
	got, _ := a.scoreProtocol(stats)
	// 0.35*0.8 + 0.05*1.0 + 0.25*0.0 + 0.20*0.8 + 0.15*0.85
	// = 0.28 + 0.05 + 0.0 + 0.16 + 0.1275 = 0.6175
	assertClose(t, got, 0.6175, 0.001, "scoreProtocol compression below 1")
}

func TestScoreProtocol_HighReplayFrequency(t *testing.T) {
	a := newTestAssessor()
	stats := ProtocolStatsResult{
		AvgComprehension:         1.0,
		NLIBiasAlert:             false,
		CompressionRatio:         3.0, // (3.0-1.0)/4.0 = 0.5
		CodeCoverage:             1.0,
		TicketRestoreSuccessRate: 1.0,
		ReplayFrequencyPerHour:   20.0, // penalty = 20/10 = 2.0 → clamped to 1.0, stability = 0.5*1.0 + 0.5*0.0 = 0.5
	}
	got, _ := a.scoreProtocol(stats)
	// 0.35*1.0 + 0.05*1.0 + 0.25*0.5 + 0.20*1.0 + 0.15*0.5
	// = 0.35 + 0.05 + 0.125 + 0.20 + 0.075 = 0.8
	assertClose(t, got, 0.8, 0.001, "scoreProtocol high replay frequency")
}

func TestScoreProtocol_WeightsSumToOne(t *testing.T) {
	// Verify the protocol sub-weights sum to 1.0
	sum := 0.35 + 0.05 + 0.25 + 0.20 + 0.15
	assertClose(t, sum, 1.0, 0.001, "protocol sub-weights")
}

// TestScoreProtocol_NoTicketRestoreData verifies that a healthy system with
// zero ticket-restore events doesn't get penalized on the stability component.
// Once the collector defaults TicketRestoreSuccessRate to 1.0 on no data, and
// ReplayFrequencyPerHour is 0 (no replays), the 15% stability weight should be
// full — not the 0.075 the old 0.0 default produced.
// Scenario: healthy comprehension + coverage + compression, zero ticket data.
func TestScoreProtocol_NoTicketRestoreData(t *testing.T) {
	a := newTestAssessor()
	stats := ProtocolStatsResult{
		AvgComprehension:         0.9,
		NLIBiasAlert:             false, // calibration = 1.0
		CompressionRatio:         3.0,   // (3.0-1.0)/4.0 = 0.5
		CodeCoverage:             1.0,
		TicketRestoreSuccessRate: 1.0, // null-tolerant default (set by collector when total==0)
		TicketRestoreTotal:       0,
		ReplayFrequencyPerHour:   0.0, // no replays — stability = 0.5*1.0 + 0.5*1.0 = 1.0
	}
	got, _ := a.scoreProtocol(stats)
	// 0.35*0.9 + 0.05*1.0 + 0.25*0.5 + 0.20*1.0 + 0.15*1.0
	// = 0.315 + 0.05 + 0.125 + 0.20 + 0.15 = 0.84
	assertClose(t, got, 0.84, 0.001, "scoreProtocol with no ticket restore data")
	if got < 0.75 {
		t.Errorf("protocolScore = %f, want >= 0.75 (null-tolerant stability lift)", got)
	}
}

// TestScoreProtocol_RegressionFromBugReport reproduces the Grafana dashboard
// scenario where Protocol Health read 0.432. With the null-tolerance fix
// (TicketRestoreSuccessRate=1.0 on zero data), the same healthy inputs should
// now yield >= 0.7, matching the verification gate in the DH-004 sprint plan.
func TestScoreProtocol_RegressionFromBugReport(t *testing.T) {
	a := newTestAssessor()
	stats := ProtocolStatsResult{
		AvgComprehension:         0.8,
		NLIBiasAlert:             false,
		CompressionRatio:         2.5, // modest compression
		CodeCoverage:             0.9,
		TicketRestoreSuccessRate: 1.0, // post-fix default
		TicketRestoreTotal:       0,
		ReplayFrequencyPerHour:   0.0,
	}
	got, _ := a.scoreProtocol(stats)
	if got < 0.7 {
		t.Errorf("Protocol Health = %f, want >= 0.7 (post-fix target)", got)
	}
}

// ─── computeConfidence tests ───

func TestComputeConfidence_AllProviders(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		EdgeCount:           10,
		TotalNodes:          50,
		VolatileCount:       5,
		PermanentCount:      20,
		ConsolidationAgeSec: 3600,
	}
	got := a.computeConfidence(r)
	// 4 data points / 4.0 = 1.0
	assertClose(t, got, 1.0, 0.001, "computeConfidence all providers")
}

func TestComputeConfidence_NoData(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		EdgeCount:           0,
		TotalNodes:          0,
		VolatileCount:       0,
		PermanentCount:      0,
		ConsolidationAgeSec: 0,
	}
	got := a.computeConfidence(r)
	// 0 data points / 4.0 = 0.0 → clamped to 0.1
	assertClose(t, got, 0.1, 0.001, "computeConfidence no data (minimum clamp)")
}

func TestComputeConfidence_Partial_OneProvider(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		EdgeCount:           5,
		TotalNodes:          0,
		VolatileCount:       0,
		PermanentCount:      0,
		ConsolidationAgeSec: 0,
	}
	got := a.computeConfidence(r)
	// 1 data point / 4.0 = 0.25
	assertClose(t, got, 0.25, 0.001, "computeConfidence one provider")
}

func TestComputeConfidence_Partial_TwoProviders(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		EdgeCount:           5,
		TotalNodes:          10,
		VolatileCount:       0,
		PermanentCount:      0,
		ConsolidationAgeSec: 0,
	}
	got := a.computeConfidence(r)
	// 2 data points / 4.0 = 0.5
	assertClose(t, got, 0.5, 0.001, "computeConfidence two providers")
}

func TestComputeConfidence_Partial_ThreeProviders(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{
		EdgeCount:           5,
		TotalNodes:          10,
		VolatileCount:       3,
		PermanentCount:      7,
		ConsolidationAgeSec: 0,
	}
	got := a.computeConfidence(r)
	// 3 data points / 4.0 = 0.75
	assertClose(t, got, 0.75, 0.001, "computeConfidence three providers")
}

func TestComputeConfidence_VolatileOnly(t *testing.T) {
	a := newTestAssessor()
	// VolatileCount + PermanentCount > 0 counts as one data point
	r := &SelfAssessmentReport{
		EdgeCount:           0,
		TotalNodes:          0,
		VolatileCount:       10,
		PermanentCount:      0,
		ConsolidationAgeSec: 0,
	}
	got := a.computeConfidence(r)
	// 1 data point / 4.0 = 0.25
	assertClose(t, got, 0.25, 0.001, "computeConfidence volatile only")
}

// ─── Overall health weight application tests ───

func TestOverallHealth_7Dim_AllOnes(t *testing.T) {
	// Directly compute the 7-dim weighted sum with all sub-scores = 1.0
	retrieval := 1.0
	memory := 1.0
	edge := 1.0
	task := 1.0
	guidance := 1.0
	protocol := 1.0
	synergy := 1.0

	overall := 0.18*retrieval + 0.18*memory + 0.13*edge + 0.13*task +
		0.13*guidance + 0.13*protocol + 0.12*synergy
	assertClose(t, overall, 1.0, 0.001, "7-dim overall health all ones")
}

func TestOverallHealth_6Dim_AllOnes(t *testing.T) {
	retrieval := 1.0
	memory := 1.0
	edge := 1.0
	task := 1.0
	guidance := 1.0
	protocol := 1.0

	overall := 0.20*retrieval + 0.20*memory + 0.15*edge + 0.15*task +
		0.15*guidance + 0.15*protocol
	assertClose(t, overall, 1.0, 0.001, "6-dim overall health all ones")
}

func TestOverallHealth_5Dim_AllOnes(t *testing.T) {
	retrieval := 1.0
	memory := 1.0
	edge := 1.0
	task := 1.0
	guidance := 1.0

	overall := 0.25*retrieval + 0.25*memory + 0.20*edge + 0.15*task +
		0.15*guidance
	assertClose(t, overall, 1.0, 0.001, "5-dim overall health all ones")
}

func TestOverallHealth_4Dim_AllOnes(t *testing.T) {
	retrieval := 1.0
	memory := 1.0
	edge := 1.0
	task := 1.0

	overall := 0.30*retrieval + 0.25*memory + 0.25*edge + 0.20*task
	assertClose(t, overall, 1.0, 0.001, "4-dim overall health all ones")
}

// ─── Edge cases and boundary tests ───

func TestScoreRetrieval_EmptyPhase(t *testing.T) {
	a := newTestAssessor()
	r := &SelfAssessmentReport{LearningPhase: ""}
	got, _ := a.scoreRetrieval(r)
	assertClose(t, got, 0.5, 0.001, "scoreRetrieval empty phase defaults to 0.5")
}

func TestScoreMemory_BoundaryOrphanRatio(t *testing.T) {
	a := newTestAssessor()
	// Exactly at boundary: 0.1 should not trigger penalty (> 0.1 required)
	r := &SelfAssessmentReport{
		OrphanRatio:         0.1,
		CorrectionRate:      0.0,
		ConsolidationAgeSec: 0,
	}
	got, _ := a.scoreMemory(r)
	assertClose(t, got, 1.0, 0.001, "scoreMemory orphan ratio at boundary 0.1")
}

func TestScoreMemory_BoundaryCorrectionRate(t *testing.T) {
	a := newTestAssessor()
	// Exactly at boundary: 0.15 should not trigger penalty (> 0.15 required)
	r := &SelfAssessmentReport{
		OrphanRatio:         0.0,
		CorrectionRate:      0.15,
		ConsolidationAgeSec: 0,
	}
	got, _ := a.scoreMemory(r)
	assertClose(t, got, 1.0, 0.001, "scoreMemory correction rate at boundary 0.15")
}

func TestScoreMemory_BoundaryConsolidationAge(t *testing.T) {
	a := newTestAssessor()
	// Exactly at boundary: 86400 should not trigger penalty (> 86400 required)
	r := &SelfAssessmentReport{
		OrphanRatio:         0.0,
		CorrectionRate:      0.0,
		ConsolidationAgeSec: 86400,
	}
	got, _ := a.scoreMemory(r)
	assertClose(t, got, 1.0, 0.001, "scoreMemory consolidation age at boundary 86400")
}

func TestScoreEdge_BoundaryBelowRatio(t *testing.T) {
	a := newTestAssessor()
	// Exactly 30% should not trigger penalty (> 0.3 required)
	r := &SelfAssessmentReport{
		EdgeCount:           100,
		EdgesBelowThreshold: 30,
		EdgeWeightEntropy:   0.8,
	}
	got, _ := a.scoreEdge(r)
	assertClose(t, got, 1.0, 0.001, "scoreEdge below ratio at boundary 0.3")
}

func TestScoreEdge_BoundaryEntropy(t *testing.T) {
	a := newTestAssessor()
	// Exactly 0.5 should not trigger penalty (< 0.5 required)
	r := &SelfAssessmentReport{
		EdgeCount:           100,
		EdgesBelowThreshold: 10,
		EdgeWeightEntropy:   0.5,
	}
	got, _ := a.scoreEdge(r)
	assertClose(t, got, 1.0, 0.001, "scoreEdge entropy at boundary 0.5")
}

func TestScoreSynergy_BoundaryClaudeLines(t *testing.T) {
	a := newTestAssessor()
	// Exactly at target (100) should not trigger penalty
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  100,
		SynergyLinesMemory:  70,
		SynergyOverflowRate: 2.0,
		SynergyOverlapScore: 0.3,
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 1.0, 0.001, "scoreSynergy claude lines at boundary")
}

func TestScoreSynergy_BoundaryOverflowRate(t *testing.T) {
	a := newTestAssessor()
	// Exactly 5.0 should not trigger penalty (> 5 required)
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  90,
		SynergyLinesMemory:  70,
		SynergyOverflowRate: 5.0,
		SynergyOverlapScore: 0.3,
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 1.0, 0.001, "scoreSynergy overflow rate at boundary 5.0")
}

func TestScoreSynergy_BoundaryOverlapScore(t *testing.T) {
	a := newTestAssessor()
	// Exactly 0.5 should not trigger penalty (> 0.5 required)
	r := &SelfAssessmentReport{
		JiminyHealthy:       true,
		SynergyLinesClaude:  90,
		SynergyLinesMemory:  70,
		SynergyOverflowRate: 2.0,
		SynergyOverlapScore: 0.5,
	}
	got, _ := a.scoreSynergy(r)
	assertClose(t, got, 1.0, 0.001, "scoreSynergy overlap score at boundary 0.5")
}

func TestScoreProtocol_StabilityWeighting(t *testing.T) {
	a := newTestAssessor()
	// Test stability sub-component in isolation: restoreRate=0.6, replay=5/hr
	stats := ProtocolStatsResult{
		AvgComprehension:         0.0,
		NLIBiasAlert:             true, // calibration = 0.3
		CompressionRatio:         1.0,  // compression = 0.0
		CodeCoverage:             0.0,
		TicketRestoreSuccessRate: 0.6,
		ReplayFrequencyPerHour:   5.0, // penalty = 0.5, stability = 0.5*0.6 + 0.5*0.5 = 0.55
	}
	got, _ := a.scoreProtocol(stats)
	// 0.35*0.0 + 0.05*0.3 + 0.25*0.0 + 0.20*0.0 + 0.15*0.55
	// = 0.0 + 0.015 + 0.0 + 0.0 + 0.0825 = 0.0975
	assertClose(t, got, 0.0975, 0.001, "scoreProtocol stability weighting")
}
