package metrics

import (
	"testing"

	"mdemg/internal/ratelimit"
)

// TSDB-CONSUME-001: synthetic p95/p99 must be computed over the bucket delta
// between flushes, not lifetime-cumulative counts. The lifetime variant pegs
// permanently after one slow call (live symptom: a constant 9.95 top-bucket
// clamp firing a perpetual latency CRITICAL).

func TestEstimatePercentileFromCumulative_Interpolation(t *testing.T) {
	// 100 observations: 50 in (0, 0.1], 40 in (0.1, 0.5], 10 in (0.5, 1.0]
	buckets := map[float64]int64{0.1: 50, 0.5: 90, 1.0: 100}

	// p95 → target 95 lands in the (0.5, 1.0] bucket: 0.5 + (95-90)/10 × 0.5 = 0.75
	if got := estimatePercentileFromCumulative(buckets, 100, 0.95); got < 0.7499 || got > 0.7501 {
		t.Errorf("p95 = %v, want ≈0.75", got)
	}
	// p50 → target 50 is satisfied by the first bucket boundary
	if got := estimatePercentileFromCumulative(buckets, 100, 0.50); got != 0.1 {
		t.Errorf("p50 = %v, want 0.1", got)
	}
}

func TestEstimatePercentileFromCumulative_Empty(t *testing.T) {
	if got := estimatePercentileFromCumulative(nil, 0, 0.95); got != 0 {
		t.Errorf("empty buckets → %v, want 0", got)
	}
	if got := estimatePercentileFromCumulative(map[float64]int64{1: 5}, 0, 0.95); got != 0 {
		t.Errorf("zero count → %v, want 0", got)
	}
}

// The regression scenario: one historic slow call must NOT peg the windowed
// percentile. Lifetime snapshot A contains the slow call; the next window's
// delta is all-fast and its p99 must reflect only the window.
func TestWindowedPercentile_SlowCallDoesNotPeg(t *testing.T) {
	// Snapshot A (lifetime): 95 fast calls ≤1.0s, 5 slow in (1.0, 10.0]
	// (>1% slow — enough to peg a lifetime p99)
	prev := histSnapshot{buckets: map[float64]int64{1.0: 95, 10.0: 100}, count: 100}
	// Snapshot B (lifetime): +100 fast calls since A
	cur := map[float64]int64{1.0: 195, 10.0: 200}
	curCount := int64(200)

	// The delta computation mirrors FlushToTSDB.
	deltaCount := curCount - prev.count
	delta := make(map[float64]int64, len(cur))
	for le, cnt := range cur {
		delta[le] = cnt - prev.buckets[le]
	}

	p99 := estimatePercentileFromCumulative(delta, deltaCount, 0.99)
	if p99 > 1.0 {
		t.Errorf("windowed p99 = %v — the historic slow call leaked into the window (lifetime-cumulative bug)", p99)
	}
	// Lifetime p99 over the same data WOULD be pegged in the slow bucket;
	// prove the distinction the fix depends on.
	lifetime := estimatePercentileFromCumulative(cur, curCount, 0.99)
	if lifetime <= 1.0 {
		t.Errorf("lifetime p99 = %v — test setup no longer demonstrates the pegging scenario", lifetime)
	}
}

// First flush has no prior snapshot: the delta equals the full cumulative
// snapshot (nil-map reads return 0), so synthetics still emit.
func TestWindowedPercentile_FirstFlush(t *testing.T) {
	var prev histSnapshot // zero value: nil buckets, count 0
	cur := map[float64]int64{0.5: 90, 1.0: 100}
	deltaCount := int64(100) - prev.count
	delta := make(map[float64]int64, len(cur))
	for le, cnt := range cur {
		delta[le] = cnt - prev.buckets[le]
	}
	if deltaCount != 100 {
		t.Fatalf("first-flush deltaCount = %d, want 100", deltaCount)
	}
	if got := estimatePercentileFromCumulative(delta, deltaCount, 0.95); got <= 0.5 || got > 1.0 {
		t.Errorf("first-flush p95 = %v, want in (0.5, 1.0]", got)
	}
}

// TSDB-CONSUME-001: CollectRateLimitMetrics must add the DELTA of the
// cumulative rejection total, not the running total itself (the old code
// inflated the counter quadratically once anything was rejected) — and it
// previously had zero callers, so the rate_limiting_active rule could never
// fire.
func TestCollectRateLimitMetrics_Delta(t *testing.T) {
	m := NewStandardMetrics(NewRegistry(DefaultConfig()))

	m.CollectRateLimitMetrics() // sync baseline against the package-global total
	base := m.RateLimitRejected.Value()

	m.CollectRateLimitMetrics()
	if got := m.RateLimitRejected.Value(); got != base {
		t.Errorf("idle re-collection moved counter %d → %d (cumulative re-add bug)", base, got)
	}

	ratelimit.AddRejectedForTest(5)
	m.CollectRateLimitMetrics()
	if got := m.RateLimitRejected.Value(); got != base+5 {
		t.Errorf("after 5 rejections counter = %d, want %d", got, base+5)
	}

	m.CollectRateLimitMetrics()
	if got := m.RateLimitRejected.Value(); got != base+5 {
		t.Errorf("re-collection after delta moved counter to %d (cumulative re-add bug)", got)
	}
}
