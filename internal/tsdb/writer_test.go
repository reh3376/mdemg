package tsdb

import (
	"testing"
	"time"
)

// newTestWriter creates a MetricWriter without starting the flush goroutine.
func newTestWriter(spaceID string) *MetricWriter {
	return &MetricWriter{
		buffer:  make([]MetricSample, 0, 64),
		spaceID: spaceID,
		done:    make(chan struct{}),
	}
}

// ─── Record ───

func TestMetricWriter_Record(t *testing.T) {
	w := newTestWriter("test-space")

	s1 := MetricSample{MetricName: "m1", Value: 1.0}
	s2 := MetricSample{MetricName: "m2", Value: 2.0}
	w.Record(s1, s2)

	if got := len(w.buffer); got != 2 {
		t.Errorf("buffer length: got %d, want 2", got)
	}
	if w.buffer[0].MetricName != "m1" {
		t.Errorf("buffer[0].MetricName: got %q, want %q", w.buffer[0].MetricName, "m1")
	}
	if w.buffer[1].MetricName != "m2" {
		t.Errorf("buffer[1].MetricName: got %q, want %q", w.buffer[1].MetricName, "m2")
	}
}

// ─── RecordGaugeSnapshot ───

func TestMetricWriter_RecordGaugeSnapshot(t *testing.T) {
	w := newTestWriter("test-space")

	gauges := map[string]float64{
		"metric_a": 1.5,
		"metric_b": 2.5,
	}
	before := time.Now()
	w.RecordGaugeSnapshot(gauges, "live", "nominal")
	after := time.Now()

	if got := len(w.buffer); got != 2 {
		t.Errorf("buffer length: got %d, want 2", got)
	}

	// Build a lookup for easy verification (map iteration order is random)
	byName := make(map[string]MetricSample)
	for _, s := range w.buffer {
		byName[s.MetricName] = s
	}

	for name, wantVal := range gauges {
		s, ok := byName[name]
		if !ok {
			t.Errorf("missing sample for metric %q", name)
			continue
		}
		if s.Value != wantVal {
			t.Errorf("%s value: got %f, want %f", name, s.Value, wantVal)
		}
		if s.SpaceID != "test-space" {
			t.Errorf("%s spaceID: got %q, want %q", name, s.SpaceID, "test-space")
		}
		if s.Source != "live" {
			t.Errorf("%s source: got %q, want %q", name, s.Source, "live")
		}
		if s.QualityTag != "nominal" {
			t.Errorf("%s qualityTag: got %q, want %q", name, s.QualityTag, "nominal")
		}
		if s.Time.Before(before) || s.Time.After(after) {
			t.Errorf("%s time %v not in expected range [%v, %v]", name, s.Time, before, after)
		}
	}
}

func TestMetricWriter_RecordGaugeSnapshot_Empty(t *testing.T) {
	w := newTestWriter("test-space")
	w.RecordGaugeSnapshot(map[string]float64{}, "live", "nominal")

	if got := len(w.buffer); got != 0 {
		t.Errorf("buffer length: got %d, want 0", got)
	}
}
