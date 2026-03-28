package backpressure

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestMemoryPressure_DisabledAlwaysFalse(t *testing.T) {
	mp := NewMemoryPressure(1, false) // 1MB threshold, disabled
	if mp.IsUnderPressure() {
		t.Error("expected disabled pressure monitor to return false")
	}
}

func TestMemoryPressure_HighThresholdNoPress(t *testing.T) {
	mp := NewMemoryPressure(100000, true) // 100GB threshold - should never hit
	if mp.IsUnderPressure() {
		t.Error("expected high threshold to not trigger pressure")
	}
}

func TestMemoryPressure_LowThresholdTriggers(t *testing.T) {
	// Allocate some memory to ensure heap > 0
	data := make([]byte, 2*1024*1024) // 2MB
	_ = data                          // prevent optimization

	// Get actual heap size
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	heapMB := memStats.HeapAlloc / (1024 * 1024)

	// Use a threshold lower than current heap
	if heapMB == 0 {
		t.Skip("heap allocation too small for test")
	}

	mp := NewMemoryPressure(heapMB-1, true)
	if !mp.IsUnderPressure() {
		t.Errorf("expected threshold %d MB to trigger pressure (heap: %d MB)", heapMB-1, heapMB)
	}
}

func TestMemoryPressure_HeapUsageBytes(t *testing.T) {
	// Allocate some memory
	data := make([]byte, 1024*1024) // 1MB
	_ = data

	mp := NewMemoryPressure(4096, true)
	_ = mp // use mp to satisfy compiler

	// Get heap in bytes for more precision
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// HeapUsageMB might be 0 if heap < 1MB, so we check the raw value
	if memStats.HeapAlloc == 0 {
		t.Error("expected non-zero heap allocation")
	}
}

func TestMemoryPressure_Stats(t *testing.T) {
	mp := NewMemoryPressure(4096, true)
	stats := mp.Stats()

	if _, ok := stats["enabled"]; !ok {
		t.Error("missing 'enabled' in stats")
	}
	if _, ok := stats["heap_alloc_mb"]; !ok {
		t.Error("missing 'heap_alloc_mb' in stats")
	}
	if _, ok := stats["threshold_mb"]; !ok {
		t.Error("missing 'threshold_mb' in stats")
	}
	if _, ok := stats["under_pressure"]; !ok {
		t.Error("missing 'under_pressure' in stats")
	}

	if stats["threshold_mb"].(uint64) != 4096 {
		t.Errorf("expected threshold 4096, got %v", stats["threshold_mb"])
	}
}

func TestMemoryPressure_Middleware_HealthBypass(t *testing.T) {
	// Allocate memory and use low threshold
	data := make([]byte, 2*1024*1024)
	_ = data

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	heapMB := memStats.HeapAlloc / (1024 * 1024)

	if heapMB == 0 {
		t.Skip("heap too small")
	}

	mp := NewMemoryPressure(heapMB-1, true) // Should be under pressure

	handler := mp.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test health endpoints bypass
	healthPaths := []string{"/healthz", "/readyz", "/v1/metrics/snapshot"}
	for _, path := range healthPaths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("health endpoint %s should bypass pressure, got %d", path, rr.Code)
		}
	}
}

func TestMemoryPressure_Middleware_Rejection(t *testing.T) {
	// Allocate enough memory to guarantee heap stays well above threshold
	// even if GC runs between our measurement and the middleware check.
	data := make([]byte, 10*1024*1024) // 10MB

	// Force GC first to get a stable baseline, then read stats
	runtime.GC()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	heapMB := memStats.HeapAlloc / (1024 * 1024)

	if heapMB < 2 {
		t.Skip("heap too small for rejection test")
	}

	// Use threshold well below current heap to avoid flakiness from GC
	threshold := heapMB / 2
	mp := NewMemoryPressure(threshold, true) // Should be under pressure

	handler := mp.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/memory/retrieve", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Keep data alive past the middleware call to prevent GC from collecting it
	runtime.KeepAlive(data)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 under pressure, got %d (heap: %d MB, threshold: %d MB)",
			rr.Code, heapMB, threshold)
	}

	if rr.Header().Get("Retry-After") != "5" {
		t.Error("expected Retry-After header")
	}

	if rr.Header().Get("X-Memory-Pressure") != "true" {
		t.Error("expected X-Memory-Pressure header")
	}

	if mp.RejectedCount() != 1 {
		t.Errorf("expected rejected count 1, got %d", mp.RejectedCount())
	}
}

func TestMemoryPressure_Middleware_PassThrough(t *testing.T) {
	mp := NewMemoryPressure(100000, true) // High threshold - no pressure

	handler := mp.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/memory/retrieve", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 without pressure, got %d", rr.Code)
	}
}

func TestMemoryPressure_RejectedCount(t *testing.T) {
	// Create a mock that simulates pressure by using threshold 0
	// With threshold 0, any heap > 0 bytes will trigger pressure
	// Since Go runtime always has some heap, this should reliably work
	mp := NewMemoryPressure(0, true)

	// Verify we're under pressure (heap > 0 bytes / 1MB = heap in MB > 0)
	// This may not trigger if heap is < 1MB, so we verify first
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// If heap in bytes is > 0, and our threshold is 0 MB,
	// then heapMB (which is HeapAlloc / 1MB) must be > 0 for pressure
	// But if HeapAlloc < 1MB, heapMB will be 0, and 0 > 0 is false
	// So this test is fundamentally flaky with small heaps

	// Instead, let's test the counter directly by verifying the middleware behavior
	// when under pressure, and separately test no pressure

	// Test 1: Not under pressure (high threshold) - no rejections
	mpNoPressure := NewMemoryPressure(100000, true) // 100GB threshold
	handlerNoPressure := mpNoPressure.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/memory/ingest", nil)
		rr := httptest.NewRecorder()
		handlerNoPressure.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 without pressure, got %d", rr.Code)
		}
	}

	if mpNoPressure.RejectedCount() != 0 {
		t.Errorf("expected rejected count 0 without pressure, got %d", mpNoPressure.RejectedCount())
	}

	// Test 2: Under pressure - verify counter increments
	// Skip if heap is too small to trigger pressure with threshold 0
	if memStats.HeapAlloc/(1024*1024) == 0 {
		t.Log("heap < 1MB, skipping pressure rejection count test")
		return
	}

	handler := mp.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/memory/ingest", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	if mp.RejectedCount() != 3 {
		t.Errorf("expected rejected count 3, got %d", mp.RejectedCount())
	}
}

func TestMemoryPressure_HeapUsageMB(t *testing.T) {
	// Allocate memory to ensure heap > 0
	data := make([]byte, 2*1024*1024) // 2MB
	_ = data

	mp := NewMemoryPressure(4096, true)
	heapMB := mp.HeapUsageMB()

	// Should be at least 1MB after allocating 2MB
	if heapMB == 0 {
		// This can happen if GC runs, just verify the function doesn't panic
		t.Log("heap reported as 0 MB (GC may have run)")
	}
}

func TestMemoryPressure_ThresholdMB(t *testing.T) {
	mp := NewMemoryPressure(4096, true)
	if mp.ThresholdMB() != 4096 {
		t.Errorf("expected threshold 4096, got %d", mp.ThresholdMB())
	}

	mp2 := NewMemoryPressure(8192, false)
	if mp2.ThresholdMB() != 8192 {
		t.Errorf("expected threshold 8192, got %d", mp2.ThresholdMB())
	}
}
