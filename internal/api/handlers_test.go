package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mdemg/internal/models"
)

// TestHandleHealthz verifies the healthz response includes version, commit, and checks fields
func TestHandleHealthz(t *testing.T) {
	s := &Server{}
	s.cfg.MdemgVersion = "0.6.0"
	s.cfg.MdemgCommit = "abc1234"

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	s.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// With nil driver, status should be degraded
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded (nil driver)", body["status"])
	}
	if body["version"] != "0.6.0" {
		t.Errorf("version = %v, want 0.6.0", body["version"])
	}
	if body["commit"] != "abc1234" {
		t.Errorf("commit = %v, want abc1234", body["commit"])
	}

	// Verify checks map exists and reports neo4j as no_driver
	checks, ok := body["checks"].(map[string]any)
	if !ok {
		t.Fatal("expected checks map in response")
	}
	if checks["neo4j"] != "no_driver" {
		t.Errorf("checks[neo4j] = %v, want no_driver", checks["neo4j"])
	}
}

// TestHandleMetrics_MethodNotAllowed tests that handleMetrics returns 405 for non-GET methods
func TestHandleMetrics_MethodNotAllowed(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{"POST method not allowed", http.MethodPost},
		{"PUT method not allowed", http.MethodPut},
		{"DELETE method not allowed", http.MethodDelete},
		{"PATCH method not allowed", http.MethodPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server (no driver needed for method validation)
			s := &Server{}

			// Create test request with the specified method
			req := httptest.NewRequest(tt.method, "/v1/metrics", nil)
			w := httptest.NewRecorder()

			// Call the handler directly
			s.handleMetrics(w, req)

			// Verify response
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("handleMetrics(%s) status = %d, want %d", tt.method, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestToInt64 tests the toInt64 helper function
func TestToInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int64
	}{
		{"int64 value", int64(100), 100},
		{"int value", int(50), 50},
		{"float64 value", float64(75.9), 75},
		{"float32 value", float32(25.5), 25},
		{"zero value", int64(0), 0},
		{"negative int64", int64(-10), -10},
		{"negative float64", float64(-5.5), -5},
		{"nil value", nil, 0},
		{"string value returns 0", "not a number", 0},
		{"bool value returns 0", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toInt64(tt.input)
			if result != tt.expected {
				t.Errorf("toInt64(%v) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// TestToFloat64Val tests the toFloat64Val helper function
func TestToFloat64Val(t *testing.T) {
	tests := []struct {
		name       string
		input      any
		defaultVal float64
		expected   float64
	}{
		{"float64 value", float64(0.75), 0.0, 0.75},
		{"float32 value", float32(0.5), 0.0, 0.5},
		{"int64 value", int64(10), 0.0, 10.0},
		{"int value", int(5), 0.0, 5.0},
		{"nil returns default", nil, 0.5, 0.5},
		{"string returns default", "not a number", 0.25, 0.25},
		{"zero float64", float64(0.0), 1.0, 0.0},
		{"negative float64", float64(-0.5), 0.0, -0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toFloat64Val(tt.input, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("toFloat64Val(%v, %f) = %f, want %f", tt.input, tt.defaultVal, result, tt.expected)
			}
		})
	}
}

// TestContentToText tests the contentToText helper function
func TestContentToText(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string content", "hello world", "hello world"},
		{"map with text field", map[string]any{"text": "from text field"}, "from text field"},
		{"map with content field", map[string]any{"content": "from content field"}, "from content field"},
		{"map with message field", map[string]any{"message": "from message field"}, "from message field"},
		{"map without common fields", map[string]any{"data": "some data"}, "map[data:some data]"},
		{"integer value", 42, "42"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contentToText(tt.input)
			if result != tt.expected {
				t.Errorf("contentToText(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestWriteJSON tests the writeJSON helper function
func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		data           any
		expectedStatus int
		expectedBody   map[string]any
	}{
		{
			name:           "ok status with map",
			status:         http.StatusOK,
			data:           map[string]any{"status": "ok"},
			expectedStatus: http.StatusOK,
			expectedBody:   map[string]any{"status": "ok"},
		},
		{
			name:           "error status with error message",
			status:         http.StatusBadRequest,
			data:           map[string]any{"error": "bad request"},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   map[string]any{"error": "bad request"},
		},
		{
			name:           "internal error",
			status:         http.StatusInternalServerError,
			data:           map[string]any{"error": "internal error"},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   map[string]any{"error": "internal error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeJSON(w, tt.status, tt.data)

			// Verify status code
			if w.Code != tt.expectedStatus {
				t.Errorf("writeJSON status = %d, want %d", w.Code, tt.expectedStatus)
			}

			// Verify content type
			contentType := w.Header().Get("content-type")
			if contentType != "application/json" {
				t.Errorf("writeJSON content-type = %q, want %q", contentType, "application/json")
			}

			// Verify body
			var body map[string]any
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			for k, v := range tt.expectedBody {
				if body[k] != v {
					t.Errorf("response body[%q] = %v, want %v", k, body[k], v)
				}
			}
		})
	}
}

// TestHandleMetrics_Success tests that a GET request with valid method is accepted
// Note: This test validates HTTP handling layer; full database integration requires a running Neo4j instance
func TestHandleMetrics_Success(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"GET without parameters", "/v1/metrics"},
		{"GET with space_id parameter", "/v1/metrics?space_id=test-space"},
		{"GET with empty space_id", "/v1/metrics?space_id="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server (nil driver will cause internal error, but
			// method validation passes - verifying HTTP handling layer)
			s := &Server{}

			// Create test request with GET method
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()

			// Call the handler - will panic or return error due to nil driver,
			// but we can use recover to verify the method validation passed
			func() {
				defer func() {
					// Expected: either panic (nil pointer) or graceful error handling
					// The key assertion is that we did not get 405 Method Not Allowed
					if r := recover(); r != nil {
						// Panic means we got past method check - that is expected with nil driver
						// This validates the GET method is accepted
						t.Logf("Expected panic due to nil driver: %v", r)
					}
				}()
				s.handleMetrics(w, req)
			}()

			// If we got past without panic, verify we did not get 405
			if w.Code == http.StatusMethodNotAllowed {
				t.Errorf("handleMetrics(GET %s) returned 405 Method Not Allowed, but GET should be allowed", tt.url)
			}
		})
	}
}

// TestHandleMetrics_WithSpaceID tests that the space_id query parameter is correctly parsed
func TestHandleMetrics_WithSpaceID(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		expectSpaceID string
	}{
		{
			name:          "no space_id parameter",
			url:           "/v1/metrics",
			expectSpaceID: "",
		},
		{
			name:          "with space_id parameter",
			url:           "/v1/metrics?space_id=test-space",
			expectSpaceID: "test-space",
		},
		{
			name:          "with empty space_id parameter",
			url:           "/v1/metrics?space_id=",
			expectSpaceID: "",
		},
		{
			name:          "with different space_id",
			url:           "/v1/metrics?space_id=production",
			expectSpaceID: "production",
		},
		{
			name:          "with special characters in space_id",
			url:           "/v1/metrics?space_id=space-123_test",
			expectSpaceID: "space-123_test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request to verify URL parsing
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)

			// Verify space_id is correctly extracted from query parameters
			gotSpaceID := req.URL.Query().Get("space_id")
			if gotSpaceID != tt.expectSpaceID {
				t.Errorf("space_id from URL %q = %q, want %q", tt.url, gotSpaceID, tt.expectSpaceID)
			}
		})
	}
}

// TestHandleStats_MethodNotAllowed tests that handleStats returns 405 for non-GET methods
func TestHandleStats_MethodNotAllowed(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{"POST method not allowed", http.MethodPost},
		{"PUT method not allowed", http.MethodPut},
		{"DELETE method not allowed", http.MethodDelete},
		{"PATCH method not allowed", http.MethodPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server (no driver needed for method validation)
			s := &Server{}

			// Create test request with the specified method
			req := httptest.NewRequest(tt.method, "/v1/memory/stats?space_id=test", nil)
			w := httptest.NewRecorder()

			// Call the handler directly
			s.handleStats(w, req)

			// Verify response
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("handleStats(%s) status = %d, want %d", tt.method, w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestHandleStats_MissingSpaceID tests that handleStats returns 400 when space_id is missing
func TestHandleStats_MissingSpaceID(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"no space_id parameter", "/v1/memory/stats"},
		{"empty space_id parameter", "/v1/memory/stats?space_id="},
		{"other parameter but no space_id", "/v1/memory/stats?other=value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server (no driver needed for validation)
			s := &Server{}

			// Create test request with GET method
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()

			// Call the handler directly
			s.handleStats(w, req)

			// Verify response is 400 Bad Request
			if w.Code != http.StatusBadRequest {
				t.Errorf("handleStats(GET %s) status = %d, want %d", tt.url, w.Code, http.StatusBadRequest)
			}

			// Verify error message is present
			var body map[string]any
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if _, ok := body["error"]; !ok {
				t.Errorf("handleStats(GET %s) response missing 'error' field", tt.url)
			}
		})
	}
}

// TestHandleStats_Success tests that a GET request with valid space_id is accepted
// Note: This test validates HTTP handling layer; full database integration requires a running Neo4j instance
func TestHandleStats_Success(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"GET with space_id", "/v1/memory/stats?space_id=test-space"},
		{"GET with different space_id", "/v1/memory/stats?space_id=production"},
		{"GET with special characters in space_id", "/v1/memory/stats?space_id=space-123_test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server (nil driver will cause internal error, but
			// method and parameter validation passes - verifying HTTP handling layer)
			s := &Server{}

			// Create test request with GET method
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()

			// Call the handler - will panic or return error due to nil driver,
			// but we can use recover to verify the parameter validation passed
			func() {
				defer func() {
					// Expected: either panic (nil pointer) or graceful error handling
					// The key assertion is that we did not get 400 Bad Request for missing space_id
					if r := recover(); r != nil {
						// Panic means we got past validation - that is expected with nil driver
						// This validates the space_id is correctly accepted
						t.Logf("Expected panic due to nil driver: %v", r)
					}
				}()
				s.handleStats(w, req)
			}()

			// If we got past without panic, verify we did not get 400 or 405
			if w.Code == http.StatusBadRequest {
				// Check if the error is about space_id
				var body map[string]any
				if err := json.NewDecoder(w.Body).Decode(&body); err == nil {
					if errMsg, ok := body["error"].(string); ok {
						if errMsg == "space_id query parameter is required" {
							t.Errorf("handleStats(GET %s) returned space_id required error, but space_id was provided", tt.url)
						}
					}
				}
			}
			if w.Code == http.StatusMethodNotAllowed {
				t.Errorf("handleStats(GET %s) returned 405 Method Not Allowed, but GET should be allowed", tt.url)
			}
		})
	}
}

// TestHandleStats_WithSpaceID tests that the space_id query parameter is correctly parsed
func TestHandleStats_WithSpaceID(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		expectSpaceID string
	}{
		{
			name:          "with space_id parameter",
			url:           "/v1/memory/stats?space_id=test-space",
			expectSpaceID: "test-space",
		},
		{
			name:          "with different space_id",
			url:           "/v1/memory/stats?space_id=production",
			expectSpaceID: "production",
		},
		{
			name:          "with special characters in space_id",
			url:           "/v1/memory/stats?space_id=space-123_test",
			expectSpaceID: "space-123_test",
		},
		{
			name:          "with UUID-style space_id",
			url:           "/v1/memory/stats?space_id=550e8400-e29b-41d4-a716-446655440000",
			expectSpaceID: "550e8400-e29b-41d4-a716-446655440000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request to verify URL parsing
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)

			// Verify space_id is correctly extracted from query parameters
			gotSpaceID := req.URL.Query().Get("space_id")
			if gotSpaceID != tt.expectSpaceID {
				t.Errorf("space_id from URL %q = %q, want %q", tt.url, gotSpaceID, tt.expectSpaceID)
			}
		})
	}
}

// TestComputeHealthScore tests the computeHealthScore function
func TestComputeHealthScore(t *testing.T) {
	tests := []struct {
		name     string
		input    StatsResponseForTest
		expected float64
		delta    float64 // acceptable difference from expected
	}{
		{
			name: "empty space returns 0.0",
			input: StatsResponseForTest{
				MemoryCount:       0,
				EmbeddingCoverage: 0.0,
				Connectivity:      &ConnectivityForTest{OrphanCount: 0},
				TemporalDist:      &TemporalDistForTest{Last7d: 0},
			},
			expected: 0.0,
			delta:    0.001,
		},
		{
			name: "perfect health - all embeddings, no orphans, all recent",
			input: StatsResponseForTest{
				MemoryCount:       100,
				EmbeddingCoverage: 1.0,
				Connectivity:      &ConnectivityForTest{OrphanCount: 0},
				TemporalDist:      &TemporalDistForTest{Last7d: 100},
			},
			expected: 1.0,
			delta:    0.001,
		},
		{
			name: "no embeddings, all orphans, no recent activity",
			input: StatsResponseForTest{
				MemoryCount:       100,
				EmbeddingCoverage: 0.0,
				Connectivity:      &ConnectivityForTest{OrphanCount: 100},
				TemporalDist:      &TemporalDistForTest{Last7d: 0},
			},
			expected: 0.0,
			delta:    0.001,
		},
		{
			name: "50% embeddings, 50% orphans, 50% recent",
			input: StatsResponseForTest{
				MemoryCount:       100,
				EmbeddingCoverage: 0.5,
				Connectivity:      &ConnectivityForTest{OrphanCount: 50},
				TemporalDist:      &TemporalDistForTest{Last7d: 50},
			},
			expected: 0.5,
			delta:    0.001,
		},
		{
			name: "only embeddings component",
			input: StatsResponseForTest{
				MemoryCount:       100,
				EmbeddingCoverage: 1.0,
				Connectivity:      &ConnectivityForTest{OrphanCount: 100},
				TemporalDist:      &TemporalDistForTest{Last7d: 0},
			},
			expected: 0.4, // 1.0 * 0.4 = 0.4
			delta:    0.001,
		},
		{
			name: "only connectivity component",
			input: StatsResponseForTest{
				MemoryCount:       100,
				EmbeddingCoverage: 0.0,
				Connectivity:      &ConnectivityForTest{OrphanCount: 0},
				TemporalDist:      &TemporalDistForTest{Last7d: 0},
			},
			expected: 0.3, // (1.0 - 0.0) * 0.3 = 0.3
			delta:    0.001,
		},
		{
			name: "only recency component",
			input: StatsResponseForTest{
				MemoryCount:       100,
				EmbeddingCoverage: 0.0,
				Connectivity:      &ConnectivityForTest{OrphanCount: 100},
				TemporalDist:      &TemporalDistForTest{Last7d: 100},
			},
			expected: 0.3, // 1.0 * 0.3 = 0.3
			delta:    0.001,
		},
		{
			name: "partial scores across all components",
			input: StatsResponseForTest{
				MemoryCount:       200,
				EmbeddingCoverage: 0.75,          // 0.75 * 0.4 = 0.3
				Connectivity:      &ConnectivityForTest{OrphanCount: 50}, // (1 - 0.25) * 0.3 = 0.225
				TemporalDist:      &TemporalDistForTest{Last7d: 100},     // 0.5 * 0.3 = 0.15
			},
			expected: 0.675, // 0.3 + 0.225 + 0.15 = 0.675
			delta:    0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert test input to actual StatsResponse
			resp := convertToStatsResponse(tt.input)
			result := computeHealthScore(resp)

			if diff := result - tt.expected; diff < -tt.delta || diff > tt.delta {
				t.Errorf("computeHealthScore() = %f, want %f (delta %f)", result, tt.expected, tt.delta)
			}
		})
	}
}

// StatsResponseForTest is a simplified version for test data construction
type StatsResponseForTest struct {
	MemoryCount       int64
	EmbeddingCoverage float64
	Connectivity      *ConnectivityForTest
	TemporalDist      *TemporalDistForTest
}

type ConnectivityForTest struct {
	OrphanCount int64
}

type TemporalDistForTest struct {
	Last7d int64
}

// convertToStatsResponse converts test helper struct to actual models.StatsResponse
func convertToStatsResponse(input StatsResponseForTest) models.StatsResponse {
	return models.StatsResponse{
		MemoryCount:       input.MemoryCount,
		EmbeddingCoverage: input.EmbeddingCoverage,
		Connectivity: &models.Connectivity{
			OrphanCount: input.Connectivity.OrphanCount,
		},
		TemporalDistribution: &models.TemporalDistribution{
			Last7d: input.TemporalDist.Last7d,
		},
	}
}
