package ape

// HealthWeights carries the per-dimension base priors used to combine the 7
// RSIC health sub-scores into an overall health score. Values do not need to
// sum to 1.0 — the formula normalises. Zero or negative values disable a
// dimension entirely (it contributes nothing to the numerator or denominator).
//
// Introduced by DH-005: hybrid reliability × user-impact priors replacing the
// near-uniform 0.12–0.18 weights that were inversely correlated with dimension
// reliability.
type HealthWeights struct {
	Retrieval float64
	Memory    float64
	Edge      float64
	Task      float64
	Guidance  float64
	Protocol  float64
	Synergy   float64
}

// DefaultHealthWeights returns the hybrid reliability × user-impact priors.
// See docs/features/rsic-feedback-loop.md "Health Weighting & Confidence" for
// the derivation table.
//
//	Retrieval  0.08  LOW reliability  (static LearningPhase lookup), modest impact
//	Memory     0.15  MODERATE         (orphan/correction/consolidation), HIGH impact
//	Edge       0.15  HIGH             (entropy + below-threshold), MEDIUM impact
//	Task       0.20  MOD-HIGH (post-DH-004 graduation fix), HIGH impact
//	Guidance   0.17  MODERATE         (follow rate + effectiveness), HIGH impact
//	Protocol   0.20  HIGH             (5-component J17 composite), MED-HIGH impact
//	Synergy    0.05  LOW              (CLAUDE.md/MEMORY.md file-size proxy), LOW impact
//
// Sum = 1.00 (convenience; formula normalises regardless).
func DefaultHealthWeights() HealthWeights {
	return HealthWeights{
		Retrieval: 0.08,
		Memory:    0.15,
		Edge:      0.15,
		Task:      0.20,
		Guidance:  0.17,
		Protocol:  0.20,
		Synergy:   0.05,
	}
}

// ComputeOverallHealth calculates the overall health score from a
// SelfAssessmentReport using DefaultHealthWeights. It is the single source of
// truth — both self_assess.go and live_collectors.go must call this instead of
// inlining the formula.
//
// Formula: overall = Σ(w_i · c_i · s_i) / Σ(w_i · c_i)
//
// where w_i = base weight, c_i = per-dimension data-sufficiency confidence ∈
// [0,1], s_i = per-dimension score ∈ [0,1]. Dimensions with w_i ≤ 0 or c_i ≤ 0
// contribute nothing to either the numerator or the denominator — they are
// automatically excluded, not penalised. This replaces the 4/5/6/7-dimension
// branch table of the prior implementation with a single normalised weighted
// average.
//
// Because the result is a weighted average of values in [0,1], it always lands
// in [0,1] — so dashboard thresholds (red <0.4, green ≥0.7) remain meaningful
// by construction regardless of which dimensions are present.
func ComputeOverallHealth(r *SelfAssessmentReport) float64 {
	return ComputeOverallHealthWith(r, DefaultHealthWeights())
}

// ComputeOverallHealthWith is ComputeOverallHealth with operator-tunable
// weights. Used by the live wiring to thread RSIC_HEALTH_WEIGHT_<DIM> env
// overrides through without requiring a rebuild.
func ComputeOverallHealthWith(r *SelfAssessmentReport, w HealthWeights) float64 {
	type dim struct{ weight, conf, score float64 }
	dims := [...]dim{
		{w.Retrieval, r.RetrievalConfidence, r.RetrievalQuality},
		{w.Memory, r.MemoryConfidence, r.MemoryHealth},
		{w.Edge, r.EdgeConfidence, r.EdgeHealth},
		{w.Task, r.TaskConfidence, r.TaskPerformance},
		{w.Guidance, r.GuidanceConfidence, r.GuidanceHealth},
		{w.Protocol, r.ProtocolConfidence, r.ProtocolHealth},
		{w.Synergy, r.SynergyConfidence, r.SynergyHealth},
	}
	var num, denom float64
	for _, d := range dims {
		if d.weight <= 0 || d.conf <= 0 {
			continue
		}
		num += d.weight * d.conf * d.score
		denom += d.weight * d.conf
	}
	if denom <= 0 {
		return 0
	}
	return num / denom
}
