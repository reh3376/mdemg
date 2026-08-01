package hidden

import "math"

// HEBB-ETA-001 — precision-weighted Hebbian η.
//
// A node's ActivationConfidence is a unit-interval precision proxy for its
// reliability as a source of co-activation evidence. Higher = more reliable
// signal for Hebbian updates. Consumed by the Cypher update rule when
// PRECISION_WEIGHTED_ETA_ENABLED=true to compute:
//
//	eta_effective = eta * confidence_a * confidence_b (multiplied AFTER existing etaMult)
//
// Sigmoid formulation keeps the output in [0.05, 1.0] — never zero so that
// new nodes still participate in learning even when they have no history.

// ConfidenceConfig holds the tunable parameters of ComputeActivationConfidence.
// Wired from `internal/config/config.go` (CONFIDENCE_ALPHA/BETA/GAMMA/HALF_LIFE_SEC).
type ConfidenceConfig struct {
	Alpha       float64 // reinforcement weight
	Beta        float64 // recency weight
	Gamma       float64 // surprise-variance penalty
	HalfLifeSec float64 // recency decay half-life
}

// DefaultConfidenceConfig returns the shipped defaults calibrated in HEBB-ETA-001.
// The recency half-life of 1 week matches the JIMINY-SIGNAL-001 default follow-rate
// window; alpha=1 saturates around ~10 reinforcements; gamma=0.3 penalizes unstable
// surprise histories without dominating.
func DefaultConfidenceConfig() ConfidenceConfig {
	return ConfidenceConfig{Alpha: 1.0, Beta: 0.5, Gamma: 0.3, HalfLifeSec: 604800}
}

// ComputeActivationConfidence returns the per-node confidence in [0.05, 1.0].
//
// Semantics:
//   - reinforceCount: total number of times this node has been reinforced
//     (co-activated with any peer). Uses log(1+n) to reward accumulation
//     without unbounded growth.
//   - lastActivatedSecondsAgo: recency signal. Exponential decay with the
//     configured half-life. Nodes that were reinforced within the half-life
//     get near-full recency credit.
//   - surpriseHistory: the node's recent surprise scores (unit-interval).
//     High variance = unstable signal = confidence penalty. Empty or
//     single-sample histories carry no penalty.
//
// Output is clamped to [0.05, 1.0]. The lower bound is deliberate: a floor of
// 0 would multiplicatively zero out η for edges touching an un-reinforced node
// and stall learning on those pairs.
func ComputeActivationConfidence(
	reinforceCount int64,
	lastActivatedSecondsAgo float64,
	surpriseHistory []float64,
	cfg ConfidenceConfig,
) float64 {
	nTerm := cfg.Alpha * math.Log1p(float64(reinforceCount))

	var recencyTerm float64
	if cfg.HalfLifeSec > 0 && lastActivatedSecondsAgo >= 0 {
		// exp(-t/tau) — halfLife=604800s means age of 604800s gives ~0.37
		recencyTerm = cfg.Beta * math.Exp(-lastActivatedSecondsAgo/cfg.HalfLifeSec)
	}

	var varianceTerm float64
	if len(surpriseHistory) > 1 {
		varianceTerm = cfg.Gamma * variance(surpriseHistory)
	}

	raw := sigmoid(nTerm + recencyTerm - varianceTerm)
	return clamp(raw, 0.05, 1.0)
}

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func variance(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var ssq float64
	for _, x := range xs {
		d := x - mean
		ssq += d * d
	}
	return ssq / float64(len(xs)-1)
}
