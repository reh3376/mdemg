// Package mathutil provides standardized mathematical utility functions
// for score normalization, clamping, and range mapping across the MDEMG codebase.
//
// All confidence, relevance, and health scores exposed via API or used in
// cross-component comparisons MUST be in [0, 1] range. Use these functions
// to enforce that invariant consistently.
package mathutil

import "math"

// Clamp restricts value to [lo, hi].
func Clamp(value, lo, hi float64) float64 {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}

// Clamp01 restricts value to [0, 1].
func Clamp01(value float64) float64 {
	return Clamp(value, 0, 1)
}

// Sigmoid maps any real number to (0, 1) using the logistic function.
// The midpoint parameter controls where the output equals 0.5.
// The steepness parameter controls how quickly the curve transitions.
//
//	Sigmoid(x, midpoint=1.0, steepness=2.0):
//	  x=0.0 → 0.12,  x=0.5 → 0.27,  x=1.0 → 0.50,  x=2.0 → 0.88,  x=3.0 → 0.98
func Sigmoid(x, midpoint, steepness float64) float64 {
	return 1.0 / (1.0 + math.Exp(-steepness*(x-midpoint)))
}

// NormalizeScore maps an unbounded positive score to [0, 1] using a shifted
// sigmoid. Designed for composite retrieval scores where:
//   - 0.0 maps to ~0.0
//   - Scores around midpoint map to ~0.5
//   - High scores asymptotically approach 1.0 but never reach it
//
// This is the canonical normalization for converting multi-component retrieval
// scores (vector + BM25 + activation + boosts) into confidence values
// comparable with cosine-similarity-based confidence from other sources.
func NormalizeScore(score, midpoint, steepness float64) float64 {
	if score <= 0 {
		return 0
	}
	return Sigmoid(score, midpoint, steepness)
}
