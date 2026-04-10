package ape

// ComputeOverallHealth calculates the weighted overall health score from a
// SelfAssessmentReport, using dimension-adaptive weights. This is the single
// source of truth — both self_assess.go and live_collectors.go must call this
// instead of inlining the formula.
func ComputeOverallHealth(r *SelfAssessmentReport) float64 {
	switch {
	case r.SynergyHealth > 0 && r.ProtocolHealth > 0 && r.GuidanceHealth > 0:
		// All 7 dimensions
		return 0.18*r.RetrievalQuality +
			0.18*r.MemoryHealth +
			0.13*r.EdgeHealth +
			0.13*r.TaskPerformance +
			0.13*r.GuidanceHealth +
			0.13*r.ProtocolHealth +
			0.12*r.SynergyHealth
	case r.ProtocolHealth > 0 && r.GuidanceHealth > 0:
		// 6 dimensions (no synergy)
		return 0.20*r.RetrievalQuality +
			0.20*r.MemoryHealth +
			0.15*r.EdgeHealth +
			0.15*r.TaskPerformance +
			0.15*r.GuidanceHealth +
			0.15*r.ProtocolHealth
	case r.GuidanceHealth > 0:
		// 5 dimensions (no protocol)
		return 0.25*r.RetrievalQuality +
			0.25*r.MemoryHealth +
			0.20*r.EdgeHealth +
			0.15*r.TaskPerformance +
			0.15*r.GuidanceHealth
	default:
		// 4 dimensions (no guidance)
		return 0.30*r.RetrievalQuality +
			0.25*r.MemoryHealth +
			0.25*r.EdgeHealth +
			0.20*r.TaskPerformance
	}
}
