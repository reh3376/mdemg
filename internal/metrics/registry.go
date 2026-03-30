package metrics

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config holds metrics configuration.
type Config struct {
	// Enabled controls whether metrics collection is active
	Enabled bool

	// Namespace is the prefix for all metrics (default: "mdemg")
	Namespace string

	// HistogramBuckets are the bucket boundaries for latency histograms
	HistogramBuckets []float64
}

// DefaultConfig returns sensible defaults for metrics.
func DefaultConfig() Config {
	return Config{
		Enabled:   true,
		Namespace: "mdemg",
		// Default latency buckets: 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
		HistogramBuckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
	}
}

// Registry holds all registered metrics.
type Registry struct {
	mu       sync.RWMutex
	cfg      Config
	counters map[string]*Counter
	gauges   map[string]*Gauge
	hists    map[string]*Histogram
}

// NewRegistry creates a new metrics registry.
func NewRegistry(cfg Config) *Registry {
	return &Registry{
		cfg:      cfg,
		counters: make(map[string]*Counter),
		gauges:   make(map[string]*Gauge),
		hists:    make(map[string]*Histogram),
	}
}

// global registry instance
var globalRegistry = NewRegistry(DefaultConfig())

// SetGlobalRegistry replaces the global registry.
func SetGlobalRegistry(r *Registry) {
	globalRegistry = r
}

// Global returns the global registry.
func Global() *Registry {
	return globalRegistry
}

// Counter is a monotonically increasing counter.
type Counter struct {
	name   string
	help   string
	labels map[string]string
	value  int64
}

// NewCounter creates and registers a new counter.
func (r *Registry) NewCounter(name, help string, labels map[string]string) *Counter {
	key := makeKey(name, labels)
	r.mu.Lock()
	defer r.mu.Unlock()

	if c, exists := r.counters[key]; exists {
		return c
	}

	c := &Counter{
		name:   r.cfg.Namespace + "_" + name,
		help:   help,
		labels: labels,
	}
	r.counters[key] = c
	return c
}

// Inc increments the counter by 1.
func (c *Counter) Inc() {
	atomic.AddInt64(&c.value, 1)
}

// Add adds the given value to the counter.
func (c *Counter) Add(v int64) {
	atomic.AddInt64(&c.value, v)
}

// Value returns the current counter value.
func (c *Counter) Value() int64 {
	return atomic.LoadInt64(&c.value)
}

// Gauge is a metric that can go up or down.
type Gauge struct {
	name   string
	help   string
	labels map[string]string
	value  int64 // Using int64 for atomic ops, convert to float as needed
	dirty  int32 // 1 if Set/Inc/Dec called since last flush reset
}

// NewGauge creates and registers a new gauge.
func (r *Registry) NewGauge(name, help string, labels map[string]string) *Gauge {
	key := makeKey(name, labels)
	r.mu.Lock()
	defer r.mu.Unlock()

	if g, exists := r.gauges[key]; exists {
		return g
	}

	g := &Gauge{
		name:   r.cfg.Namespace + "_" + name,
		help:   help,
		labels: labels,
	}
	r.gauges[key] = g
	return g
}

// RemoveGaugesByPrefix removes all gauges whose key starts with the given prefix.
// Used to purge stale per-label gauges (e.g., deleted spaces).
func (r *Registry) RemoveGaugesByPrefix(prefix string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.gauges {
		if strings.HasPrefix(key, prefix) {
			delete(r.gauges, key)
		}
	}
}

// Set sets the gauge to the given value.
func (g *Gauge) Set(v float64) {
	atomic.StoreInt64(&g.value, int64(v*1000)) // Store as milli-units
	atomic.StoreInt32(&g.dirty, 1)
}

// Inc increments the gauge by 1.
func (g *Gauge) Inc() {
	atomic.AddInt64(&g.value, 1000)
	atomic.StoreInt32(&g.dirty, 1)
}

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() {
	atomic.AddInt64(&g.value, -1000)
	atomic.StoreInt32(&g.dirty, 1)
}

// Value returns the current gauge value.
func (g *Gauge) Value() float64 {
	return float64(atomic.LoadInt64(&g.value)) / 1000
}

// IsDirty returns true if the gauge has been set since the last ResetDirty call.
func (g *Gauge) IsDirty() bool {
	return atomic.LoadInt32(&g.dirty) != 0
}

// ResetDirty clears the dirty flag after a flush cycle.
func (g *Gauge) ResetDirty() {
	atomic.StoreInt32(&g.dirty, 0)
}

// Histogram tracks the distribution of values.
type Histogram struct {
	name    string
	help    string
	labels  map[string]string
	buckets []float64

	mu          sync.Mutex
	bucketCounts []int64
	sum         float64
	count       int64
}

// NewHistogram creates and registers a new histogram.
func (r *Registry) NewHistogram(name, help string, labels map[string]string) *Histogram {
	key := makeKey(name, labels)
	r.mu.Lock()
	defer r.mu.Unlock()

	if h, exists := r.hists[key]; exists {
		return h
	}

	h := &Histogram{
		name:         r.cfg.Namespace + "_" + name,
		help:         help,
		labels:       labels,
		buckets:      r.cfg.HistogramBuckets,
		bucketCounts: make([]int64, len(r.cfg.HistogramBuckets)),
	}
	r.hists[key] = h
	return h
}

// Observe records a value in the histogram.
// Increments the first bucket where v <= bound.
// Snapshot() accumulates to produce proper cumulative counts.
func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += v
	h.count++

	// Find the first bucket that can contain this value
	for i, bound := range h.buckets {
		if v <= bound {
			h.bucketCounts[i]++
			return // Only increment one bucket; Snapshot accumulates
		}
	}
	// Value exceeds all defined buckets; it will only appear in +Inf
}

// ObserveDuration observes a duration since the given start time.
func (h *Histogram) ObserveDuration(start time.Time) {
	h.Observe(time.Since(start).Seconds())
}

// Snapshot returns a copy of the histogram data with cumulative bucket counts.
func (h *Histogram) Snapshot() (buckets map[float64]int64, sum float64, count int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	buckets = make(map[float64]int64, len(h.buckets))
	cumulative := int64(0)
	for i, bound := range h.buckets {
		cumulative += h.bucketCounts[i]
		buckets[bound] = cumulative
	}
	return buckets, h.sum, h.count
}

// makeKey creates a unique key for a metric with labels.
func makeKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(name)
	for _, k := range keys {
		sb.WriteString("|")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(labels[k])
	}
	return sb.String()
}
