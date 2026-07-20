package metrics

import (
	"mdemg/internal/circuitbreaker"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCounter(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	c := r.NewCounter("test_counter", "Test counter", nil)

	if c.Value() != 0 {
		t.Errorf("initial value = %d, want 0", c.Value())
	}

	c.Inc()
	if c.Value() != 1 {
		t.Errorf("after Inc() value = %d, want 1", c.Value())
	}

	c.Add(10)
	if c.Value() != 11 {
		t.Errorf("after Add(10) value = %d, want 11", c.Value())
	}
}

func TestCounterWithLabels(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	c1 := r.NewCounter("http_requests", "HTTP requests", map[string]string{"method": "GET", "path": "/"})
	c2 := r.NewCounter("http_requests", "HTTP requests", map[string]string{"method": "POST", "path": "/"})
	c3 := r.NewCounter("http_requests", "HTTP requests", map[string]string{"method": "GET", "path": "/"})

	c1.Inc()
	c2.Add(5)

	if c1.Value() != 1 {
		t.Errorf("c1 value = %d, want 1", c1.Value())
	}
	if c2.Value() != 5 {
		t.Errorf("c2 value = %d, want 5", c2.Value())
	}
	if c3.Value() != 1 {
		t.Errorf("c3 should be same as c1, value = %d, want 1", c3.Value())
	}
}

func TestGauge(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	g := r.NewGauge("test_gauge", "Test gauge", nil)

	if g.Value() != 0 {
		t.Errorf("initial value = %.3f, want 0", g.Value())
	}

	g.Set(42.5)
	if g.Value() != 42.5 {
		t.Errorf("after Set(42.5) value = %.3f, want 42.5", g.Value())
	}

	g.Inc()
	if g.Value() != 43.5 {
		t.Errorf("after Inc() value = %.3f, want 43.5", g.Value())
	}

	g.Dec()
	if g.Value() != 42.5 {
		t.Errorf("after Dec() value = %.3f, want 42.5", g.Value())
	}
}

func TestHistogram(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	h := r.NewHistogram("test_latency", "Test latency", nil)

	h.Observe(0.005) // 5ms - falls in first bucket
	h.Observe(0.015) // 15ms - falls in 25ms bucket
	h.Observe(0.1)   // 100ms
	h.Observe(1.0)   // 1s

	buckets, sum, count := h.Snapshot()

	if count != 4 {
		t.Errorf("count = %d, want 4", count)
	}

	expectedSum := 0.005 + 0.015 + 0.1 + 1.0
	if sum != expectedSum {
		t.Errorf("sum = %.3f, want %.3f", sum, expectedSum)
	}

	// Check bucket counts (cumulative)
	if buckets[0.005] != 1 { // 5ms bucket
		t.Errorf("bucket[0.005] = %d, want 1", buckets[0.005])
	}
	if buckets[0.1] != 3 { // 100ms bucket (cumulative: 5ms + 15ms + 100ms)
		t.Errorf("bucket[0.1] = %d, want 3", buckets[0.1])
	}
}

func TestHistogram_ObserveDuration(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	h := r.NewHistogram("test_duration", "Test duration", nil)

	start := time.Now()
	time.Sleep(10 * time.Millisecond)
	h.ObserveDuration(start)

	_, sum, count := h.Snapshot()

	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if sum < 0.01 || sum > 0.1 {
		t.Errorf("sum = %.3f, expected ~0.01", sum)
	}
}

func TestRegistry_SnapshotAll(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	r.NewCounter("http_requests_total", "Total HTTP requests", map[string]string{"method": "GET"}).Add(100)
	r.NewGauge("active_connections", "Active connections", nil).Set(42)
	r.NewHistogram("request_latency_seconds", "Request latency", nil).Observe(0.1)

	// Use MetricsRecorder.SnapshotAll to verify metrics are registered
	mr := NewMetricsRecorder(r, nil, "test")
	snap := mr.SnapshotAll()

	// Check counter
	found := false
	for k, v := range snap.Counters {
		if strings.Contains(k, "http_requests_total") && v == 100 {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing counter value in snapshot")
	}

	// Check gauge
	found = false
	for k, v := range snap.Gauges {
		if strings.Contains(k, "active_connections") && v == 42 {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing gauge value in snapshot")
	}

	// Check histogram
	found = false
	for k, h := range snap.Histograms {
		if strings.Contains(k, "request_latency_seconds") && h.Count == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing histogram in snapshot")
	}
}

func TestHTTPMiddleware(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	handler := HTTPMiddleware(r)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	// Check that metrics were recorded via snapshot
	mr := NewMetricsRecorder(r, nil, "test")
	snap := mr.SnapshotAll()

	foundCounter := false
	for k := range snap.Counters {
		if strings.Contains(k, "http_requests_total") {
			foundCounter = true
			break
		}
	}
	if !foundCounter {
		t.Error("HTTP request counter not recorded")
	}

	foundHist := false
	for k := range snap.Histograms {
		if strings.Contains(k, "http_request_duration_seconds") {
			foundHist = true
			break
		}
	}
	if !foundHist {
		t.Error("HTTP request duration not recorded")
	}
}

func TestStandardMetrics(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	m := NewStandardMetrics(r)

	// Test HTTP metrics factory functions
	c1 := m.HTTPRequestsTotal("GET", "/test", "200")
	c2 := m.HTTPRequestsTotal("GET", "/test", "200")

	c1.Inc()
	if c2.Value() != 1 {
		t.Error("same labels should return same counter")
	}

	// Test histogram factory
	h := m.HTTPRequestDuration("GET", "/test")
	h.Observe(0.1)
	_, _, count := h.Snapshot()
	if count != 1 {
		t.Errorf("histogram count = %d, want 1", count)
	}

	// Test fixed metrics
	m.RetrievalLatency.Observe(0.05)
	m.RetrievalCacheHits.Inc()
	m.RateLimitRejected.Inc()

	// Test gauge factory
	g := m.CircuitBreakerState("openai")
	g.Set(1)
	if g.Value() != 1 {
		t.Errorf("circuit breaker state = %.1f, want 1", g.Value())
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/v1/memory", "/v1/memory"},
		{"/short", "/short"},
		// Long path should be truncated to 50 chars + "..."
		{"/v1/memory/nodes/very-long-uuid-here-that-exceeds-fifty-characters-total", "/v1/memory/nodes/very-long-uuid-here-that-exceeds-..."},
	}

	for _, tt := range tests {
		got := normalizePath(tt.path)
		if got != tt.expected {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.path, got, tt.expected)
		}
	}
}

func TestCollectRateLimitMetrics(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	m := NewStandardMetrics(r)

	// Initial value
	initial := m.RateLimitRejected.Value()

	// Collect rate limit metrics (this calls ratelimit.RejectedTotal())
	m.CollectRateLimitMetrics()

	// Value should not decrease
	if m.RateLimitRejected.Value() < initial {
		t.Error("RateLimitRejected should not decrease")
	}
}

func TestCollectCircuitBreakerMetrics(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	m := NewStandardMetrics(r)

	// Test with nil registry (should not panic)
	m.CollectCircuitBreakerMetrics(nil)

	// Create a circuit breaker registry and collect metrics
	cbCfg := circuitbreaker.DefaultConfig()
	cbCfg.FailureThreshold = 1
	cbRegistry := circuitbreaker.NewRegistry(cbCfg)

	// Create breakers in different states
	closedBreaker := cbRegistry.Get("closed-service")
	openBreaker := cbRegistry.Get("open-service")
	openBreaker.RecordFailure() // This will open it

	// Collect metrics
	m.CollectCircuitBreakerMetrics(cbRegistry)

	// Verify gauges were set
	closedGauge := m.CircuitBreakerState("closed-service")
	openGauge := m.CircuitBreakerState("open-service")

	_ = closedBreaker // Verify we created it

	if closedGauge.Value() != 0 {
		t.Errorf("closed-service gauge = %.1f, want 0", closedGauge.Value())
	}
	if openGauge.Value() != 1 {
		t.Errorf("open-service gauge = %.1f, want 1", openGauge.Value())
	}
}

func TestGlobalMetrics(t *testing.T) {
	// Reset global state for testing
	globalMetrics = nil

	// First call to Metrics() should initialize
	m1 := Metrics()
	if m1 == nil {
		t.Fatal("Metrics() returned nil")
	}

	// Second call should return the same instance
	m2 := Metrics()
	if m1 != m2 {
		t.Error("Metrics() should return same instance")
	}

	// InitStandardMetrics should replace
	m3 := InitStandardMetrics()
	if m3 == nil {
		t.Fatal("InitStandardMetrics() returned nil")
	}
}

func TestGlobalRegistry(t *testing.T) {
	// Test SetGlobalRegistry and Global
	r := NewRegistry(DefaultConfig())

	SetGlobalRegistry(r)

	if Global() != r {
		t.Error("Global() should return the set registry")
	}
}

func TestHTTPMiddleware_Write(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	handler := HTTPMiddleware(r)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Write without calling WriteHeader first
		// This should implicitly set status 200
		w.Write([]byte("hello world"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if body != "hello world" {
		t.Errorf("body = %q, want 'hello world'", body)
	}
}

func TestHTTPMiddleware_NoWriteHeader(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	handler := HTTPMiddleware(r)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Handler that writes body without explicitly setting status
		w.Write([]byte("response"))
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should get 200 OK (implicit)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestNewGauge_WithExistingLabels(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	// Create gauge with labels
	g1 := r.NewGauge("test_gauge", "Test gauge", map[string]string{"env": "prod"})
	g2 := r.NewGauge("test_gauge", "Test gauge", map[string]string{"env": "prod"})

	g1.Set(42)

	// Same labels should return same gauge
	if g2.Value() != 42 {
		t.Errorf("same labels should return same gauge, got %.1f want 42", g2.Value())
	}
}

func TestNewHistogram_WithExistingLabels(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	// Create histogram with labels
	h1 := r.NewHistogram("test_hist", "Test histogram", map[string]string{"env": "prod"})
	h2 := r.NewHistogram("test_hist", "Test histogram", map[string]string{"env": "prod"})

	h1.Observe(0.5)

	// Same labels should return same histogram
	_, _, count := h2.Snapshot()
	if count != 1 {
		t.Errorf("same labels should return same histogram, got count %d want 1", count)
	}
}

func TestCacheHitRatio(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	m := NewStandardMetrics(r)

	// Test the CacheHitRatio factory function
	g := m.CacheHitRatio("query")
	g.Set(0.85)

	if g.Value() != 0.85 {
		t.Errorf("CacheHitRatio = %.2f, want 0.85", g.Value())
	}
}

func TestCollectCircuitBreakerMetrics_HalfOpen(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	m := NewStandardMetrics(r)

	cbCfg := circuitbreaker.DefaultConfig()
	cbCfg.FailureThreshold = 1
	cbCfg.Timeout = 10 * time.Millisecond
	cbRegistry := circuitbreaker.NewRegistry(cbCfg)

	// Create a breaker and put it in half-open state
	breaker := cbRegistry.Get("halfopen-service")
	breaker.RecordFailure() // Opens it

	// Wait for half-open transition
	time.Sleep(20 * time.Millisecond)
	_ = breaker.State() // Force state check

	// Collect metrics
	m.CollectCircuitBreakerMetrics(cbRegistry)

	gauge := m.CircuitBreakerState("halfopen-service")
	if gauge.Value() != 2 {
		t.Errorf("halfopen-service gauge = %.1f, want 2 (half-open)", gauge.Value())
	}
}

func TestHTTPMiddleware_NilRegistry(t *testing.T) {
	// HTTPMiddleware with nil should use global registry
	handler := HTTPMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestStatusResponseWriter_WriteHeaderTwice(t *testing.T) {
	r := NewRegistry(DefaultConfig())

	handler := HTTPMiddleware(r)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Write header twice - second call should be ignored
		w.WriteHeader(http.StatusCreated)
		w.WriteHeader(http.StatusOK) // This should be ignored
		w.Write([]byte("created"))
	}))

	req := httptest.NewRequest("POST", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should be 201 (first call), not 200
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
}
