package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"mdemg/internal/tsdb"
)

// MetricsRecorder bridges the in-memory metrics Registry with TSDB persistence.
// It periodically flushes all registered metrics to TimescaleDB and provides
// JSON-serializable snapshots for the /v1/metrics/snapshot endpoint.
//
// Counter semantics:
// Counters represent event rates, not cumulative totals. On restart, counters reset to 0.
// FlushToTSDB writes counter DELTAS (change since last flush).
// TSDB rows = "events in the last flush interval". Grafana: SUM(value) for totals.
// Differs from Prometheus monotonically-increasing counters. TSDB model is simpler.
type MetricsRecorder struct {
	registry   *Registry
	writer     *tsdb.MetricWriter
	spaceID    string
	instanceID string // e.g. "localhost:9999" — injected into every sample's labels

	// Counter delta tracking — stores last-flushed value per counter key
	lastCounterValues sync.Map // map[string]int64

	// Histogram window tracking — last-flushed cumulative bucket snapshot per
	// histogram key (TSDB-CONSUME-001). Synthetic p95/p99 are computed over
	// the delta between flushes; lifetime-cumulative percentiles peg
	// permanently once one slow observation lands (live symptom: a constant
	// 9.95 top-bucket clamp firing a perpetual latency CRITICAL).
	// Touched only inside FlushToTSDB (single flush goroutine).
	lastHistSnapshots map[string]histSnapshot

	// Bounded retry buffer for TSDB write resilience
	mu           sync.Mutex
	retryBuffer  []tsdb.MetricSample
	maxBufferLen int

	// Health tracking
	lastSuccessfulWrite time.Time
	consecutiveFailures int

	// Pre-flush hook — called before each FlushToTSDB to refresh gauge values
	preFlushHook func()

	// Lifecycle
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewMetricsRecorder creates a new recorder. writer may be nil if TSDB is not configured.
func NewMetricsRecorder(registry *Registry, writer *tsdb.MetricWriter, spaceID string) *MetricsRecorder {
	return &MetricsRecorder{
		registry:          registry,
		writer:            writer,
		spaceID:           spaceID,
		maxBufferLen:      10000,
		lastHistSnapshots: make(map[string]histSnapshot),
	}
}

// SetInstanceID sets the instance identifier (e.g. "localhost:9999") that gets
// injected into every metric sample's labels for multi-instance filtering.
func (mr *MetricsRecorder) SetInstanceID(id string) {
	mr.instanceID = id
}

// SetPreFlushHook registers a function called before each FlushToTSDB cycle.
// Used to refresh live gauge values (e.g., protocol metrics, sidecar stats)
// so they're current when flushed to TSDB.
func (mr *MetricsRecorder) SetPreFlushHook(fn func()) {
	mr.preFlushHook = fn
}

// SnapshotAll returns a JSON-serializable snapshot of every metric in the registry.
func (mr *MetricsRecorder) SnapshotAll() MetricSnapshot {
	mr.registry.mu.RLock()
	defer mr.registry.mu.RUnlock()

	snap := MetricSnapshot{
		Timestamp:  time.Now(),
		Counters:   make(map[string]int64, len(mr.registry.counters)),
		Gauges:     make(map[string]float64, len(mr.registry.gauges)),
		Histograms: make(map[string]HistogramSnapshot, len(mr.registry.hists)),
	}

	// Counters
	for _, c := range mr.registry.counters {
		snap.Counters[c.name+formatLabels(c.labels)] = c.Value()
	}

	// Gauges
	for _, g := range mr.registry.gauges {
		snap.Gauges[g.name+formatLabels(g.labels)] = g.Value()
	}

	// Histograms
	for _, h := range mr.registry.hists {
		buckets, sum, count := h.Snapshot()
		bucketStr := make(map[string]int64, len(buckets))
		for le, cnt := range buckets {
			bucketStr[fmt.Sprintf("%.3f", le)] = cnt
		}
		hs := HistogramSnapshot{
			Count:   count,
			Sum:     sum,
			Buckets: bucketStr,
		}
		// Compute P95 and P99 from cumulative buckets
		hs.P95 = estimatePercentile(h, 0.95)
		hs.P99 = estimatePercentile(h, 0.99)
		snap.Histograms[h.name+formatLabels(h.labels)] = hs
	}

	// Writer health
	mr.mu.Lock()
	snap.Writer = WriterHealthSnapshot{
		ConsecutiveFailures: mr.consecutiveFailures,
		BufferSize:          len(mr.retryBuffer),
	}
	if !mr.lastSuccessfulWrite.IsZero() {
		snap.Writer.LastSuccessfulWrite = mr.lastSuccessfulWrite.Format(time.RFC3339)
	}
	mr.mu.Unlock()

	return snap
}

// FlushToTSDB writes all current metric values to TimescaleDB.
// Counter deltas are computed since the last flush; gauges are written as absolute values;
// histograms are decomposed into bucket, sum, count, and synthetic P95/P99 samples.
func (mr *MetricsRecorder) FlushToTSDB() {
	if mr.writer == nil {
		return
	}

	// Refresh live gauge values before reading them for TSDB write
	if mr.preFlushHook != nil {
		mr.preFlushHook()
	}

	now := time.Now()
	var samples []tsdb.MetricSample
	instanceID := mr.instanceID

	mr.registry.mu.RLock()

	// --- Counters (delta) ---
	for key, c := range mr.registry.counters {
		current := c.Value()
		var delta int64
		if prev, ok := mr.lastCounterValues.Load(key); ok {
			delta = current - prev.(int64)
		} else {
			delta = current
		}
		mr.lastCounterValues.Store(key, current)

		if delta == 0 {
			continue // Don't write zero-delta rows
		}

		samples = append(samples, tsdb.MetricSample{
			Time:       now,
			SpaceID:    mr.spaceID,
			MetricName: c.name,
			Value:      float64(delta),
			Source:     "recorder",
			QualityTag: "nominal",
			MetricType: "counter",
			Labels:     copyLabels(c.labels, instanceID),
		})
	}

	// --- Gauges (absolute, dirty-filtered) ---
	var gaugeFlushed, gaugeSkipped int
	for _, g := range mr.registry.gauges {
		if !g.IsDirty() {
			gaugeSkipped++
			continue
		}
		val := g.Value()
		g.ResetDirty()
		samples = append(samples, tsdb.MetricSample{
			Time:       now,
			SpaceID:    mr.spaceID,
			MetricName: g.name,
			Value:      val,
			Source:     "recorder",
			QualityTag: "nominal",
			MetricType: "gauge",
			Labels:     copyLabels(g.labels, instanceID),
		})
		gaugeFlushed++
	}
	if gaugeSkipped > 0 {
		slog.Debug("TSDB flush gauge stats", "flushed", gaugeFlushed, "skipped_clean", gaugeSkipped)
	}

	// --- Histograms (decomposed) ---
	for key, h := range mr.registry.hists {
		buckets, sum, count := h.Snapshot()

		// Bucket samples
		for le, cnt := range buckets {
			lbls := copyLabels(h.labels, instanceID)
			lbls["le"] = fmt.Sprintf("%.3f", le)
			samples = append(samples, tsdb.MetricSample{
				Time:       now,
				SpaceID:    mr.spaceID,
				MetricName: h.name + "_bucket",
				Value:      float64(cnt),
				Source:     "recorder",
				QualityTag: "nominal",
				MetricType: "histogram_bucket",
				Labels:     lbls,
			})
		}

		// Sum
		samples = append(samples, tsdb.MetricSample{
			Time:       now,
			SpaceID:    mr.spaceID,
			MetricName: h.name + "_sum",
			Value:      sum,
			Source:     "recorder",
			QualityTag: "nominal",
			MetricType: "histogram_sum",
			Labels:     copyLabels(h.labels, instanceID),
		})

		// Count
		samples = append(samples, tsdb.MetricSample{
			Time:       now,
			SpaceID:    mr.spaceID,
			MetricName: h.name + "_count",
			Value:      float64(count),
			Source:     "recorder",
			QualityTag: "nominal",
			MetricType: "histogram_count",
			Labels:     copyLabels(h.labels, instanceID),
		})

		// Synthetic P95 / P99 gauges — WINDOWED (TSDB-CONSUME-001): computed
		// over the bucket delta since the last flush, emitted only when the
		// window saw observations. Lifetime-cumulative percentiles never
		// recover after one slow call; idle windows emit nothing (gaps are
		// honest — alert rules must aggregate + COALESCE, not LIMIT 1).
		prev := mr.lastHistSnapshots[key]
		deltaCount := count - prev.count
		deltaBuckets := make(map[float64]int64, len(buckets))
		for le, cnt := range buckets {
			deltaBuckets[le] = cnt - prev.buckets[le]
		}
		mr.lastHistSnapshots[key] = histSnapshot{buckets: buckets, count: count}
		if deltaCount > 0 {
			p95 := estimatePercentileFromCumulative(deltaBuckets, deltaCount, 0.95)
			p99 := estimatePercentileFromCumulative(deltaBuckets, deltaCount, 0.99)
			samples = append(samples, tsdb.MetricSample{
				Time:       now,
				SpaceID:    mr.spaceID,
				MetricName: h.name + "_p95",
				Value:      p95,
				Source:     "recorder",
				QualityTag: "nominal",
				MetricType: "gauge",
				Labels:     copyLabels(h.labels, instanceID),
			})
			samples = append(samples, tsdb.MetricSample{
				Time:       now,
				SpaceID:    mr.spaceID,
				MetricName: h.name + "_p99",
				Value:      p99,
				Source:     "recorder",
				QualityTag: "nominal",
				MetricType: "gauge",
				Labels:     copyLabels(h.labels, instanceID),
			})
		}
	}

	mr.registry.mu.RUnlock()

	if len(samples) == 0 {
		return
	}

	// Try to drain the retry buffer first, then write current batch
	mr.mu.Lock()
	pending := mr.retryBuffer
	mr.retryBuffer = nil
	mr.mu.Unlock()

	if len(pending) > 0 {
		mr.writer.Record(pending...)
	}
	mr.writer.Record(samples...)

	// Attempt a synchronous flush so we can detect failures
	if err := mr.writer.Flush(context.Background()); err != nil {
		slog.Warn("metrics_recorder: TSDB flush failed, buffering samples",
			"error", err, "sample_count", len(samples))
		mr.mu.Lock()
		mr.consecutiveFailures++
		// Re-buffer: pending that failed to flush + current samples
		rebuffer := append(pending, samples...)
		if len(mr.retryBuffer)+len(rebuffer) > mr.maxBufferLen {
			// Drop oldest to stay within bounds
			overflow := len(mr.retryBuffer) + len(rebuffer) - mr.maxBufferLen
			if overflow >= len(mr.retryBuffer) {
				mr.retryBuffer = rebuffer
				if len(mr.retryBuffer) > mr.maxBufferLen {
					mr.retryBuffer = mr.retryBuffer[len(mr.retryBuffer)-mr.maxBufferLen:]
				}
			} else {
				mr.retryBuffer = append(mr.retryBuffer[overflow:], rebuffer...)
			}
		} else {
			mr.retryBuffer = append(mr.retryBuffer, rebuffer...)
		}
		mr.mu.Unlock()
		return
	}

	mr.mu.Lock()
	mr.lastSuccessfulWrite = time.Now()
	mr.consecutiveFailures = 0
	mr.mu.Unlock()
}

// Start begins periodic TSDB flushing at the given interval.
func (mr *MetricsRecorder) Start(interval time.Duration) {
	mr.stopCh = make(chan struct{})
	mr.doneCh = make(chan struct{})

	go func() {
		defer close(mr.doneCh)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				mr.FlushToTSDB()
			case <-mr.stopCh:
				// Final flush on shutdown
				mr.FlushToTSDB()
				return
			}
		}
	}()
}

// Stop halts the periodic flushing. It blocks until the final flush completes.
func (mr *MetricsRecorder) Stop() {
	if mr.stopCh == nil {
		return
	}
	close(mr.stopCh)
	<-mr.doneCh
}

// SetWriter attaches or replaces the TSDB writer. This is called after
// SetTSDBClient when the writer becomes available post-construction.
func (mr *MetricsRecorder) SetWriter(w *tsdb.MetricWriter) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.writer = w
}

// WriterHealth returns the current TSDB writer health state.
func (mr *MetricsRecorder) WriterHealth() (lastWrite time.Time, failures int, bufferSize int) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	return mr.lastSuccessfulWrite, mr.consecutiveFailures, len(mr.retryBuffer)
}

// ── Helpers ──

// formatLabels formats labels as a sorted key=value string for metric keys.
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, k, labels[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// copyLabels creates a copy of a labels map, optionally injecting an instance label.
func copyLabels(labels map[string]string, instanceID string) map[string]string {
	result := make(map[string]string, len(labels)+2)
	maps.Copy(result, labels)
	if instanceID != "" {
		result["instance"] = instanceID
	}
	return result
}

// histSnapshot stores a histogram's cumulative bucket counts at the last
// flush, so FlushToTSDB can compute windowed (per-flush-interval) synthetic
// percentiles instead of lifetime-cumulative ones (TSDB-CONSUME-001).
type histSnapshot struct {
	buckets map[float64]int64 // bound → cumulative count (nil on first flush; reads return 0)
	count   int64
}

// estimatePercentileFromCumulative computes an estimated percentile from a
// map of bucket bound → cumulative count (the Histogram.Snapshot format, or
// a delta of two such snapshots) using the same linear interpolation as
// estimatePercentile.
func estimatePercentileFromCumulative(buckets map[float64]int64, count int64, q float64) float64 {
	if count <= 0 || len(buckets) == 0 {
		return 0
	}
	bounds := make([]float64, 0, len(buckets))
	for b := range buckets {
		bounds = append(bounds, b)
	}
	sort.Float64s(bounds)

	target := q * float64(count)
	var prevCum int64
	var prevBound float64
	for _, bound := range bounds {
		cum := buckets[bound]
		if float64(cum) >= target {
			bucketCount := cum - prevCum
			if bucketCount <= 0 {
				return prevBound
			}
			fraction := (target - float64(prevCum)) / float64(bucketCount)
			return prevBound + fraction*(bound-prevBound)
		}
		prevCum = cum
		prevBound = bound
	}
	return bounds[len(bounds)-1]
}

// estimatePercentile computes an estimated percentile from histogram buckets
// using linear interpolation within the target bucket (same as Prometheus histogram_quantile).
// Lifetime-cumulative — used by SnapshotAll (the /v1/metrics/snapshot debug
// endpoint); FlushToTSDB uses the windowed variant above.
func estimatePercentile(h *Histogram, q float64) float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.count == 0 {
		return 0
	}

	target := q * float64(h.count)

	// Build cumulative counts
	cumulative := make([]int64, len(h.buckets))
	var cum int64
	for i := range h.buckets {
		cum += h.bucketCounts[i]
		cumulative[i] = cum
	}

	// Find the bucket that contains the target count
	for i, bound := range h.buckets {
		if float64(cumulative[i]) >= target {
			// Linear interpolation within this bucket
			var prevCum int64
			var prevBound float64
			if i > 0 {
				prevCum = cumulative[i-1]
				prevBound = h.buckets[i-1]
			}
			bucketCount := cumulative[i] - prevCum
			if bucketCount == 0 {
				return prevBound
			}
			fraction := (target - float64(prevCum)) / float64(bucketCount)
			return prevBound + fraction*(bound-prevBound)
		}
	}

	// Beyond all buckets — return the last bucket boundary
	if len(h.buckets) > 0 {
		return h.buckets[len(h.buckets)-1]
	}
	return 0
}

