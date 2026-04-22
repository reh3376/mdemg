package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestLimiter_Allow(t *testing.T) {
	// Create a limiter with 10 tokens/sec, burst of 5
	l := NewLimiter(10, 5)

	// Should allow burst of 5
	for i := 0; i < 5; i++ {
		if !l.Allow() {
			t.Errorf("request %d should have been allowed", i+1)
		}
	}

	// 6th request should be denied
	if l.Allow() {
		t.Error("6th request should have been denied")
	}
}

func TestLimiter_Refill(t *testing.T) {
	// Create a limiter with 100 tokens/sec, burst of 10
	l := NewLimiter(100, 10)

	// Drain all tokens
	for i := 0; i < 10; i++ {
		l.Allow()
	}

	// Should be empty
	if l.Allow() {
		t.Error("should be empty after draining")
	}

	// Wait for refill (100 tokens/sec = 1 token per 10ms)
	time.Sleep(50 * time.Millisecond)

	// Should have refilled at least some tokens
	if !l.Allow() {
		t.Error("should have refilled after 50ms")
	}
}

func TestLimiter_Concurrent(t *testing.T) {
	l := NewLimiter(1000, 100)

	var wg sync.WaitGroup
	allowed := int64(0)
	var mu sync.Mutex

	// Use a barrier to synchronize start
	start := make(chan struct{})

	// Launch 200 concurrent requests
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if l.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}

	// Release all at once
	close(start)
	wg.Wait()

	// Should have allowed at most burst size (100), possibly more due to refill
	// during execution window. At 1000 tokens/sec, each millisecond of dispatch
	// spread adds 1 token; under -race on a contended CI runner, 15ms+ of spread
	// is realistic. Upper bound 120 catches genuine limit breakage while
	// tolerating timing variance. (Observed CI flake at 107 with prior bound 105.)
	if allowed < 100 || allowed > 120 {
		t.Errorf("expected 100..120 allowed (burst + refill during dispatch), got %d", allowed)
	}
}

func TestLimiterStore_GetLimiter(t *testing.T) {
	store := NewLimiterStore(100, 10)

	// Same key should return same limiter
	l1 := store.GetLimiter("192.168.1.1")
	l2 := store.GetLimiter("192.168.1.1")

	if l1 != l2 {
		t.Error("same key should return same limiter")
	}

	// Different key should return different limiter
	l3 := store.GetLimiter("192.168.1.2")
	if l1 == l3 {
		t.Error("different keys should return different limiters")
	}

	// Store should have 2 entries
	if store.Size() != 2 {
		t.Errorf("expected 2 limiters, got %d", store.Size())
	}
}

func TestMiddleware_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// All requests should pass through
	for i := 0; i < 1000; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d should have been allowed when disabled", i+1)
		}
	}
}

func TestMiddleware_RateLimits(t *testing.T) {
	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 10,
		BurstSize:         5,
		ByIP:              false, // Global limiter
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 5 should pass (burst)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d should have been allowed", i+1)
		}
	}

	// 6th should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}

	// Should have Retry-After header
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestMiddleware_SkipEndpoints(t *testing.T) {
	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		BurstSize:         1,
		ByIP:              false,
		SkipEndpoints: map[string]bool{
			"/healthz": true,
		},
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust the limiter
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Health check should still pass
	req = httptest.NewRequest("GET", "/healthz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("healthz should bypass rate limiting, got %d", rec.Code)
	}
}

func TestMiddleware_ByIP(t *testing.T) {
	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 10,
		BurstSize:         2,
		ByIP:              true,
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP 1 - exhaust its burst
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if i < 2 && rec.Code != http.StatusOK {
			t.Errorf("IP1 request %d should have been allowed", i+1)
		}
		if i >= 2 && rec.Code != http.StatusTooManyRequests {
			t.Errorf("IP1 request %d should have been rate limited", i+1)
		}
	}

	// IP 2 should still have its own burst allowance
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.2:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("IP2 request %d should have been allowed", i+1)
		}
	}
}

func TestExtractIP_DirectConnection(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:12345"

	ip := extractIP(r, nil)
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

func TestExtractIP_TrustedProxy(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:12345"
	r.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.2")

	// Without trusted proxy, should use RemoteAddr
	ip := extractIP(r, nil)
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", ip)
	}

	// With trusted proxy, should use X-Forwarded-For
	ip = extractIP(r, []string{"10.0.0.1"})
	if ip != "203.0.113.1" {
		t.Errorf("expected 203.0.113.1, got %s", ip)
	}
}

func TestExtractIP_TrustedProxyCIDR(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.50:12345"
	r.Header.Set("X-Forwarded-For", "203.0.113.1")

	// Trust entire 10.0.0.0/24 subnet
	ip := extractIP(r, []string{"10.0.0.0/24"})
	if ip != "203.0.113.1" {
		t.Errorf("expected 203.0.113.1, got %s", ip)
	}
}

func TestExtractIP_InvalidRemoteAddr(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "invalid-no-port" // No port, will fail SplitHostPort

	ip := extractIP(r, nil)
	if ip != "invalid-no-port" {
		t.Errorf("expected raw address fallback, got %s", ip)
	}
}

func TestExtractIP_EmptyXFF(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:12345"
	r.Header.Set("X-Forwarded-For", "") // Empty XFF

	ip := extractIP(r, []string{"10.0.0.1"})
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", ip)
	}
}

func TestIsTrustedProxy_InvalidCIDR(t *testing.T) {
	// Invalid CIDR should not crash, just return false
	trusted := isTrustedProxy("192.168.1.1", []string{"invalid/cidr/format"})
	if trusted {
		t.Error("invalid CIDR should not match")
	}
}

func TestIsTrustedProxy_NoMatch(t *testing.T) {
	trusted := isTrustedProxy("192.168.1.1", []string{"10.0.0.0/8"})
	if trusted {
		t.Error("IP outside CIDR should not match")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("default should be enabled")
	}
	if cfg.RequestsPerSecond != 100 {
		t.Errorf("expected RPS 100, got %f", cfg.RequestsPerSecond)
	}
	if cfg.BurstSize != 200 {
		t.Errorf("expected burst 200, got %d", cfg.BurstSize)
	}
	if !cfg.ByIP {
		t.Error("default should be per-IP")
	}
	if !cfg.SkipEndpoints["/healthz"] {
		t.Error("default should skip /healthz")
	}
	if !cfg.SkipEndpoints["/readyz"] {
		t.Error("default should skip /readyz")
	}
}

func TestLimiter_Burst(t *testing.T) {
	l := NewLimiter(10, 5)

	if l.Burst() != 5 {
		t.Errorf("expected burst 5, got %d", l.Burst())
	}
}

func TestLimiter_Rate(t *testing.T) {
	l := NewLimiter(42.5, 10)

	if l.Rate() != 42.5 {
		t.Errorf("expected rate 42.5, got %f", l.Rate())
	}
}

func TestLimiter_Tokens(t *testing.T) {
	l := NewLimiter(10, 5)

	// Should start with full burst
	tokens := l.Tokens()
	if tokens != 5 {
		t.Errorf("expected 5 tokens, got %f", tokens)
	}

	// Consume one
	l.Allow()

	// Should have 4 (or slightly more due to refill)
	tokens = l.Tokens()
	if tokens < 3.9 || tokens > 4.1 {
		t.Errorf("expected ~4 tokens, got %f", tokens)
	}
}

func TestLimiter_AllowN(t *testing.T) {
	l := NewLimiter(10, 10)

	// Should allow consuming 5 at once
	if !l.AllowN(5) {
		t.Error("should allow 5 tokens")
	}

	// Should have 5 left
	if !l.AllowN(5) {
		t.Error("should allow another 5 tokens")
	}

	// Should not allow 1 more
	if l.AllowN(1) {
		t.Error("should not allow more tokens")
	}
}

func TestRejectedTotal(t *testing.T) {
	// Get initial value
	initial := RejectedTotal()

	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 10,
		BurstSize:         1,
		ByIP:              false,
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request should pass
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Second request should be rejected
	req = httptest.NewRequest("GET", "/test", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// RejectedTotal should have increased
	if RejectedTotal() <= initial {
		t.Error("RejectedTotal should have increased after rejection")
	}
}

func TestLimiterStore_Cleanup(t *testing.T) {
	store := &LimiterStore{
		limiters:        make(map[string]*Limiter),
		rate:            10,
		burst:           5,
		cleanupInterval: 1 * time.Millisecond,
		maxAge:          1 * time.Millisecond,
	}

	// Add a limiter
	_ = store.GetLimiter("test-ip")
	if store.Size() != 1 {
		t.Error("should have 1 limiter")
	}

	// Wait for it to become stale
	time.Sleep(10 * time.Millisecond)

	// Manually trigger cleanup
	store.cleanup()

	// Should be cleaned up
	if store.Size() != 0 {
		t.Errorf("expected 0 limiters after cleanup, got %d", store.Size())
	}
}

func TestLimiterStore_CleanupKeepsActive(t *testing.T) {
	store := &LimiterStore{
		limiters:        make(map[string]*Limiter),
		rate:            10,
		burst:           5,
		cleanupInterval: 1 * time.Minute,
		maxAge:          1 * time.Minute,
	}

	// Add a limiter
	limiter := store.GetLimiter("test-ip")
	limiter.Allow() // Use it

	// Immediately cleanup
	store.cleanup()

	// Should still be there (not stale yet)
	if store.Size() != 1 {
		t.Error("active limiter should not be cleaned up")
	}
}

func TestMiddleware_HeadersOnSuccess(t *testing.T) {
	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 100,
		BurstSize:         10,
		ByIP:              false,
	}
	middleware := Middleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should have rate limit headers
	if rec.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("expected X-RateLimit-Limit header")
	}
	if rec.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("expected X-RateLimit-Remaining header")
	}
}

func TestLimiterStore_GetLimiter_RaceCondition(t *testing.T) {
	store := NewLimiterStore(100, 10)

	var wg sync.WaitGroup
	const key = "race-test-ip"
	limiters := make([]*Limiter, 100)

	// Use a channel as a barrier to force all goroutines to start simultaneously
	start := make(chan struct{})

	// Launch 100 concurrent goroutines all requesting the same key
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // Wait for signal
			limiters[idx] = store.GetLimiter(key)
		}(i)
	}

	// Release all goroutines at once
	close(start)
	wg.Wait()

	// All should get the exact same limiter instance
	first := limiters[0]
	for i := 1; i < 100; i++ {
		if limiters[i] != first {
			t.Errorf("goroutine %d got different limiter instance", i)
		}
	}

	// Store should have exactly 1 entry
	if store.Size() != 1 {
		t.Errorf("expected 1 limiter, got %d", store.Size())
	}
}

func TestLimiterStore_PeriodicCleanup(t *testing.T) {
	// Create store with very short cleanup interval - must set fields before accessing
	store := &LimiterStore{
		limiters:        make(map[string]*Limiter),
		rate:            10,
		burst:           5,
		cleanupInterval: 5 * time.Millisecond,
		maxAge:          1 * time.Millisecond,
	}

	// Add a limiter
	_ = store.GetLimiter("periodic-test")
	if store.Size() != 1 {
		t.Error("should have 1 limiter")
	}

	// Start periodic cleanup in background
	go store.periodicCleanup()

	// Wait for periodic cleanup to run at least once
	time.Sleep(20 * time.Millisecond)

	// Should be cleaned up
	if store.Size() != 0 {
		t.Errorf("expected 0 limiters after periodic cleanup, got %d", store.Size())
	}
}
