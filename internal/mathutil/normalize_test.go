package mathutil

import (
	"math"
	"testing"
)

func almostEqual(a, b, eps float64) bool {
	return math.Abs(a-b) < eps
}

func TestClamp(t *testing.T) {
	tests := []struct {
		value, lo, hi, want float64
	}{
		{0.5, 0, 1, 0.5},
		{-1, 0, 1, 0},
		{2, 0, 1, 1},
		{0, 0, 1, 0},
		{1, 0, 1, 1},
		{50, 10, 100, 50},
	}
	for _, tt := range tests {
		got := Clamp(tt.value, tt.lo, tt.hi)
		if got != tt.want {
			t.Errorf("Clamp(%f, %f, %f) = %f, want %f", tt.value, tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestClamp01(t *testing.T) {
	tests := []struct {
		value, want float64
	}{
		{0.5, 0.5},
		{-0.3, 0},
		{1.5, 1},
		{0, 0},
		{1, 1},
	}
	for _, tt := range tests {
		got := Clamp01(tt.value)
		if got != tt.want {
			t.Errorf("Clamp01(%f) = %f, want %f", tt.value, got, tt.want)
		}
	}
}

func TestSigmoid(t *testing.T) {
	// Sigmoid(midpoint, midpoint, steepness) should always be 0.5
	got := Sigmoid(1.0, 1.0, 2.0)
	if !almostEqual(got, 0.5, 1e-9) {
		t.Errorf("Sigmoid(1, 1, 2) = %f, want 0.5", got)
	}

	// Below midpoint should be < 0.5
	got = Sigmoid(0.0, 1.0, 2.0)
	if got >= 0.5 {
		t.Errorf("Sigmoid(0, 1, 2) = %f, want < 0.5", got)
	}

	// Above midpoint should be > 0.5
	got = Sigmoid(2.0, 1.0, 2.0)
	if got <= 0.5 {
		t.Errorf("Sigmoid(2, 1, 2) = %f, want > 0.5", got)
	}

	// Should be monotonically increasing
	prev := 0.0
	for x := -5.0; x <= 5.0; x += 0.5 {
		cur := Sigmoid(x, 1.0, 2.0)
		if cur < prev {
			t.Errorf("Sigmoid not monotonic: Sigmoid(%f)=%f < Sigmoid(%f)=%f", x, cur, x-0.5, prev)
		}
		prev = cur
	}

	// Should always be in [0, 1] (extreme values may hit float64 precision limits)
	for _, x := range []float64{-10, -5, -1, 0, 1, 5, 10} {
		v := Sigmoid(x, 1.0, 2.0)
		if v < 0 || v > 1 {
			t.Errorf("Sigmoid(%f) = %f, want in [0, 1]", x, v)
		}
	}
}

func TestNormalizeScore(t *testing.T) {
	midpoint := 2.0
	steepness := 1.5

	// Zero and negative scores should return 0
	if NormalizeScore(0, midpoint, steepness) != 0 {
		t.Error("NormalizeScore(0) should be 0")
	}
	if NormalizeScore(-1, midpoint, steepness) != 0 {
		t.Error("NormalizeScore(-1) should be 0")
	}

	// Midpoint should give ~0.5
	got := NormalizeScore(midpoint, midpoint, steepness)
	if !almostEqual(got, 0.5, 1e-9) {
		t.Errorf("NormalizeScore(midpoint) = %f, want 0.5", got)
	}

	// Low scores should be < 0.5
	got = NormalizeScore(0.5, midpoint, steepness)
	if got >= 0.5 {
		t.Errorf("NormalizeScore(0.5) = %f, want < 0.5", got)
	}

	// High scores should be > 0.5 but < 1.0
	got = NormalizeScore(5.0, midpoint, steepness)
	if got <= 0.5 || got >= 1.0 {
		t.Errorf("NormalizeScore(5.0) = %f, want in (0.5, 1.0)", got)
	}

	// Very high scores should approach 1.0 (may equal 1.0 at float64 precision)
	got = NormalizeScore(10.0, midpoint, steepness)
	if got < 0.99 {
		t.Errorf("NormalizeScore(10) = %f, want > 0.99", got)
	}

	// Typical retrieval score ranges (0.5-5.0) should produce reasonable spread
	low := NormalizeScore(0.5, midpoint, steepness)
	mid := NormalizeScore(2.0, midpoint, steepness)
	high := NormalizeScore(4.5, midpoint, steepness)
	if !(low < mid && mid < high) {
		t.Errorf("Expected monotonic: low=%f < mid=%f < high=%f", low, mid, high)
	}
	if low < 0.05 || high > 0.99 {
		t.Errorf("Expected reasonable spread: low=%f (want >0.05), high=%f (want <0.99)", low, high)
	}
}
