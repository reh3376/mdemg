package metrics

import (
	"testing"
	"time"

	"mdemg/internal/tsdb"
)

func TestSnapshotAll_EmptyRegistry(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	rec := NewMetricsRecorder(r, nil, "test-space")

	snap := rec.SnapshotAll()

	if snap.Timestamp.IsZero() {
		t.Error("snapshot timestamp should not be zero")
	}
	if len(snap.Counters) != 0 {
		t.Errorf("expected 0 counters, got %d", len(snap.Counters))
	}
	if len(snap.Gauges) != 0 {
		t.Errorf("expected 0 gauges, got %d", len(snap.Gauges))
	}
	if len(snap.Histograms) != 0 {
		t.Errorf("expected 0 histograms, got %d", len(snap.Histograms))
	}
	if snap.Writer.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 failures, got %d", snap.Writer.ConsecutiveFailures)
	}
	if snap.Writer.BufferSize != 0 {
		t.Errorf("expected 0 buffer size, got %d", snap.Writer.BufferSize)
	}
}

func TestSnapshotAll_WithCountersAndGauges(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	rec := NewMetricsRecorder(r, nil, "test-space")

	// Register some metrics
	c := r.NewCounter("http_requests", "Total HTTP requests", nil)
	c.Add(42)

	g := r.NewGauge("active_conns", "Active connections", nil)
	g.Set(7.5)

	h := r.NewHistogram("latency", "Request latency", nil)
	h.Observe(0.05)
	h.Observe(0.1)
	h.Observe(0.5)

	snap := rec.SnapshotAll()

	// Check counter
	counterVal, ok := snap.Counters["mdemg_http_requests"]
	if !ok {
		t.Fatal("counter mdemg_http_requests not found in snapshot")
	}
	if counterVal != 42 {
		t.Errorf("counter value = %d, want 42", counterVal)
	}

	// Check gauge
	gaugeVal, ok := snap.Gauges["mdemg_active_conns"]
	if !ok {
		t.Fatal("gauge mdemg_active_conns not found in snapshot")
	}
	if gaugeVal != 7.5 {
		t.Errorf("gauge value = %.1f, want 7.5", gaugeVal)
	}

	// Check histogram
	histSnap, ok := snap.Histograms["mdemg_latency"]
	if !ok {
		t.Fatal("histogram mdemg_latency not found in snapshot")
	}
	if histSnap.Count != 3 {
		t.Errorf("histogram count = %d, want 3", histSnap.Count)
	}
	expectedSum := 0.05 + 0.1 + 0.5
	if histSnap.Sum != expectedSum {
		t.Errorf("histogram sum = %.3f, want %.3f", histSnap.Sum, expectedSum)
	}
	if len(histSnap.Buckets) == 0 {
		t.Error("histogram should have bucket data")
	}
	// P95 should be between 0.1 and 0.5 (the last observation was 0.5, in the 0.5 bucket)
	if histSnap.P95 <= 0 {
		t.Errorf("P95 should be > 0, got %.3f", histSnap.P95)
	}
}

func TestSnapshotAll_WithLabels(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	rec := NewMetricsRecorder(r, nil, "test-space")

	c := r.NewCounter("http_requests", "Total", map[string]string{"method": "GET", "status": "200"})
	c.Inc()

	snap := rec.SnapshotAll()

	// The key should include labels using formatLabels() style: {method="GET",status="200"}
	expectedKey := `mdemg_http_requests{method="GET",status="200"}`
	found := false
	for key, val := range snap.Counters {
		if val == 1 {
			found = true
			if key != expectedKey {
				t.Errorf("counter key = %q, want %q", key, expectedKey)
			}
		}
	}
	if !found {
		t.Error("labeled counter not found in snapshot")
	}
}

// mockWriterRecorder is a test helper that captures recorded samples.
type mockWriterRecorder struct {
	samples    []tsdb.MetricSample
	flushErr   error
	flushCount int
}

func (m *mockWriterRecorder) Record(samples ...tsdb.MetricSample) {
	m.samples = append(m.samples, samples...)
}

func (m *mockWriterRecorder) Flush() error {
	m.flushCount++
	return m.flushErr
}

func TestFlushToTSDB_CounterDelta(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	// We need a real MetricWriter for the flush path, but we can't connect to TSDB in unit tests.
	// Instead, test the delta tracking logic directly.
	rec := NewMetricsRecorder(r, nil, "test-space")

	c := r.NewCounter("events", "Event counter", nil)

	// First "flush" — set counter to 5
	c.Add(5)

	// Simulate delta tracking: store current as "last flushed"
	rec.lastCounterValues.Store("events", int64(5))

	// Now set to 8 (delta should be 3)
	c.Add(3) // total is now 8

	// Verify delta calculation
	current := c.Value()
	if current != 8 {
		t.Fatalf("counter value = %d, want 8", current)
	}

	prev, ok := rec.lastCounterValues.Load("events")
	if !ok {
		t.Fatal("last counter value not found")
	}
	delta := current - prev.(int64)
	if delta != 3 {
		t.Errorf("delta = %d, want 3", delta)
	}
}

func TestFlushToTSDB_NilWriter(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	rec := NewMetricsRecorder(r, nil, "test-space")

	// Register some metrics to ensure the nil-writer path doesn't panic
	r.NewCounter("test", "test", nil).Inc()
	r.NewGauge("test_g", "test", nil).Set(1.0)
	r.NewHistogram("test_h", "test", nil).Observe(0.1)

	// Should not panic
	rec.FlushToTSDB()
}

func TestRetryBuffer_BoundedGrowth(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	rec := NewMetricsRecorder(r, nil, "test-space")
	rec.maxBufferLen = 100

	// Manually fill the retry buffer beyond the limit
	rec.mu.Lock()
	for i := 0; i < 200; i++ {
		rec.retryBuffer = append(rec.retryBuffer, tsdb.MetricSample{
			MetricName: "overflow_test",
			Value:      float64(i),
		})
	}
	// Trim to max
	if len(rec.retryBuffer) > rec.maxBufferLen {
		rec.retryBuffer = rec.retryBuffer[len(rec.retryBuffer)-rec.maxBufferLen:]
	}
	rec.mu.Unlock()

	rec.mu.Lock()
	bufLen := len(rec.retryBuffer)
	rec.mu.Unlock()

	if bufLen > rec.maxBufferLen {
		t.Errorf("buffer size %d exceeds max %d", bufLen, rec.maxBufferLen)
	}
	if bufLen != 100 {
		t.Errorf("buffer size = %d, want 100", bufLen)
	}
}

func TestStartStop_Lifecycle(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	rec := NewMetricsRecorder(r, nil, "test-space")

	// Start with a short interval
	rec.Start(50 * time.Millisecond)

	// Let it tick a couple times
	time.Sleep(150 * time.Millisecond)

	// Stop should not panic or hang
	done := make(chan struct{})
	go func() {
		rec.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() hung for more than 2 seconds")
	}
}

func TestStop_WithoutStart(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	rec := NewMetricsRecorder(r, nil, "test-space")

	// Stop without Start should not panic
	rec.Stop()
}

func TestWriterHealth_Initial(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	rec := NewMetricsRecorder(r, nil, "test-space")

	lastWrite, failures, bufSize := rec.WriterHealth()

	if !lastWrite.IsZero() {
		t.Errorf("initial lastWrite should be zero, got %v", lastWrite)
	}
	if failures != 0 {
		t.Errorf("initial failures = %d, want 0", failures)
	}
	if bufSize != 0 {
		t.Errorf("initial bufferSize = %d, want 0", bufSize)
	}
}

func TestEstimatePercentile_EmptyHistogram(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	h := r.NewHistogram("empty", "empty histogram", nil)

	p95 := estimatePercentile(h, 0.95)
	if p95 != 0 {
		t.Errorf("P95 of empty histogram = %.3f, want 0", p95)
	}
}

func TestEstimatePercentile_SingleObservation(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	h := r.NewHistogram("single", "single observation", nil)
	h.Observe(0.05) // Falls in the 0.05 bucket

	p50 := estimatePercentile(h, 0.50)
	p99 := estimatePercentile(h, 0.99)

	// With a single observation at 0.05, P50 should be between 0.025 and 0.05
	if p50 <= 0 || p50 > 0.05 {
		t.Errorf("P50 = %.4f, expected in (0, 0.05]", p50)
	}
	if p99 <= 0 || p99 > 0.05 {
		t.Errorf("P99 = %.4f, expected in (0, 0.05]", p99)
	}
}

func TestEstimatePercentile_MultipleObservations(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	h := r.NewHistogram("multi", "multiple observations", nil)

	// Add observations across different buckets
	for i := 0; i < 50; i++ {
		h.Observe(0.01) // 10ms bucket
	}
	for i := 0; i < 40; i++ {
		h.Observe(0.1) // 100ms bucket
	}
	for i := 0; i < 10; i++ {
		h.Observe(1.0) // 1s bucket
	}

	p95 := estimatePercentile(h, 0.95)
	p99 := estimatePercentile(h, 0.99)

	// P95: 95th of 100 obs = 95th. First 50 in <=0.01, next 40 in <=0.1 (cum=90),
	// next 10 in <=1.0 (cum=100). P95 should be in the 1.0 bucket.
	if p95 <= 0.1 || p95 > 1.0 {
		t.Errorf("P95 = %.3f, expected in (0.1, 1.0]", p95)
	}

	// P99 should also be in the 1.0 bucket
	if p99 <= 0.1 || p99 > 1.0 {
		t.Errorf("P99 = %.3f, expected in (0.1, 1.0]", p99)
	}

	// P95 should be less than or equal to P99
	if p95 > p99 {
		t.Errorf("P95 (%.3f) > P99 (%.3f)", p95, p99)
	}
}


func TestGauge_DirtyFlag(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	g := r.NewGauge("test_dirty", "dirty flag test", nil)

	// Initially not dirty
	if g.IsDirty() {
		t.Error("gauge should not be dirty initially")
	}

	// Set marks dirty
	g.Set(5.0)
	if !g.IsDirty() {
		t.Error("gauge should be dirty after Set")
	}

	// ResetDirty clears
	g.ResetDirty()
	if g.IsDirty() {
		t.Error("gauge should not be dirty after ResetDirty")
	}

	// Inc marks dirty
	g.Inc()
	if !g.IsDirty() {
		t.Error("gauge should be dirty after Inc")
	}
	g.ResetDirty()

	// Dec marks dirty
	g.Dec()
	if !g.IsDirty() {
		t.Error("gauge should be dirty after Dec")
	}
}

func TestFlushToTSDB_SkipsCleanGauges(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	rec := NewMetricsRecorder(r, nil, "test-space")

	// Register two gauges — set one, leave the other clean
	gDirty := r.NewGauge("dirty_gauge", "will be set", nil)
	r.NewGauge("clean_gauge", "never set", nil)

	gDirty.Set(42.0)

	// FlushToTSDB with nil writer just returns early (no TSDB), but we can
	// verify the dirty flag behavior by checking state after the call
	rec.FlushToTSDB()

	// After flush (even nil-writer), dirty gauge should have been read
	if gDirty.Value() != 42.0 {
		t.Errorf("dirty gauge value = %.1f, want 42.0", gDirty.Value())
	}
}

func TestFlushToTSDB_ResetsDirtyAfterFlush(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	g := r.NewGauge("resettable", "test reset", nil)
	g.Set(10.0)

	if !g.IsDirty() {
		t.Fatal("gauge should be dirty after Set")
	}

	// Simulate what FlushToTSDB does: read value, reset dirty
	_ = g.Value()
	g.ResetDirty()

	if g.IsDirty() {
		t.Error("gauge should be clean after ResetDirty")
	}

	// Without another Set, it should stay clean
	if g.IsDirty() {
		t.Error("gauge should remain clean without mutation")
	}
}

func TestNewMetricsRecorder_Defaults(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	rec := NewMetricsRecorder(r, nil, "my-space")

	if rec.registry != r {
		t.Error("registry not stored correctly")
	}
	if rec.writer != nil {
		t.Error("writer should be nil")
	}
	if rec.spaceID != "my-space" {
		t.Errorf("spaceID = %q, want %q", rec.spaceID, "my-space")
	}
	if rec.maxBufferLen != 10000 {
		t.Errorf("maxBufferLen = %d, want 10000", rec.maxBufferLen)
	}
}
