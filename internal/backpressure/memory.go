// Package backpressure provides memory pressure monitoring and backpressure middleware.
package backpressure

import (
	"net/http"
	"runtime"
	"sync/atomic"
)

// MemoryPressure monitors heap memory usage and provides backpressure middleware.
type MemoryPressure struct {
	heapThresholdMB uint64
	enabled         bool
	rejectedCount   atomic.Int64
}

// NewMemoryPressure creates a new memory pressure monitor.
// thresholdMB is the heap threshold in megabytes above which requests are rejected.
func NewMemoryPressure(thresholdMB uint64, enabled bool) *MemoryPressure {
	return &MemoryPressure{
		heapThresholdMB: thresholdMB,
		enabled:         enabled,
	}
}

// IsUnderPressure returns true if heap usage exceeds the threshold.
func (m *MemoryPressure) IsUnderPressure() bool {
	if !m.enabled {
		return false
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	heapMB := memStats.HeapAlloc / (1024 * 1024)
	return heapMB > m.heapThresholdMB
}

// HeapUsageMB returns the current heap usage in megabytes.
func (m *MemoryPressure) HeapUsageMB() uint64 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return memStats.HeapAlloc / (1024 * 1024)
}

// ThresholdMB returns the configured threshold in megabytes.
func (m *MemoryPressure) ThresholdMB() uint64 {
	return m.heapThresholdMB
}

// RejectedCount returns the total number of requests rejected due to memory pressure.
func (m *MemoryPressure) RejectedCount() int64 {
	return m.rejectedCount.Load()
}

// isHealthEndpoint returns true for health check endpoints that bypass backpressure.
func isHealthEndpoint(path string) bool {
	return path == "/healthz" || path == "/readyz" || path == "/v1/metrics/snapshot"
}

// Middleware returns an HTTP middleware that rejects requests when under memory pressure.
// Health endpoints (/healthz, /readyz, /v1/metrics/snapshot) are never rejected.
func (m *MemoryPressure) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip health endpoints
		if isHealthEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if m.IsUnderPressure() {
			m.rejectedCount.Add(1)
			w.Header().Set("Retry-After", "5")
			w.Header().Set("X-Memory-Pressure", "true")
			http.Error(w, "Service temporarily unavailable due to memory pressure", http.StatusServiceUnavailable)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Stats returns current memory pressure statistics.
func (m *MemoryPressure) Stats() map[string]any {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]any{
		"enabled":           m.enabled,
		"heap_alloc_mb":     memStats.HeapAlloc / (1024 * 1024),
		"heap_sys_mb":       memStats.HeapSys / (1024 * 1024),
		"threshold_mb":      m.heapThresholdMB,
		"under_pressure":    m.IsUnderPressure(),
		"rejected_requests": m.rejectedCount.Load(),
	}
}
