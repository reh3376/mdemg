package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestIsHealthEndpoint tests the isHealthEndpoint helper function
func TestIsHealthEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"healthz endpoint", "/healthz", true},
		{"readyz endpoint", "/readyz", true},
		{"retrieve endpoint", "/v1/memory/retrieve", false},
		{"ingest endpoint", "/v1/memory/ingest", false},
		{"metrics endpoint", "/v1/metrics", false},
		{"root path", "/", false},
		{"empty path", "", false},
		{"healthz with trailing slash", "/healthz/", false},
		{"readyz with trailing slash", "/readyz/", false},
		{"healthz with query params path only", "/healthz", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isHealthEndpoint(tt.path)
			if result != tt.expected {
				t.Errorf("isHealthEndpoint(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

// TestGenerateRequestID tests the generateRequestID function
func TestGenerateRequestID(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"generates non-empty ID"},
		{"generates different IDs"},
		{"generates valid hex string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.name {
			case "generates non-empty ID":
				id := generateRequestID()
				if id == "" {
					t.Error("generateRequestID() returned empty string")
				}

			case "generates different IDs":
				id1 := generateRequestID()
				id2 := generateRequestID()
				if id1 == id2 {
					t.Errorf("generateRequestID() returned same ID twice: %s", id1)
				}

			case "generates valid hex string":
				id := generateRequestID()
				// Should be 16 characters (8 bytes hex encoded)
				if len(id) != 16 {
					t.Errorf("generateRequestID() returned ID with length %d, want 16", len(id))
				}
				// Should be valid hex
				_, err := hex.DecodeString(id)
				if err != nil {
					t.Errorf("generateRequestID() returned invalid hex: %s, error: %v", id, err)
				}
			}
		})
	}
}

// TestResponseWriter_WriteHeader tests the responseWriter WriteHeader method
func TestResponseWriter_WriteHeader(t *testing.T) {
	tests := []struct {
		name           string
		statusCodes    []int
		expectedStatus int
		expectedWrote  bool
	}{
		{
			name:           "single write captures status",
			statusCodes:    []int{http.StatusOK},
			expectedStatus: http.StatusOK,
			expectedWrote:  true,
		},
		{
			name:           "captures 404 status",
			statusCodes:    []int{http.StatusNotFound},
			expectedStatus: http.StatusNotFound,
			expectedWrote:  true,
		},
		{
			name:           "captures 500 status",
			statusCodes:    []int{http.StatusInternalServerError},
			expectedStatus: http.StatusInternalServerError,
			expectedWrote:  true,
		},
		{
			name:           "multiple writes use first status",
			statusCodes:    []int{http.StatusCreated, http.StatusOK},
			expectedStatus: http.StatusCreated,
			expectedWrote:  true,
		},
		{
			name:           "captures 201 status",
			statusCodes:    []int{http.StatusCreated},
			expectedStatus: http.StatusCreated,
			expectedWrote:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := &responseWriter{
				ResponseWriter: w,
				status:         http.StatusOK, // default
			}

			for _, code := range tt.statusCodes {
				rw.WriteHeader(code)
			}

			if rw.status != tt.expectedStatus {
				t.Errorf("responseWriter.status = %d, want %d", rw.status, tt.expectedStatus)
			}

			if rw.wroteHeader != tt.expectedWrote {
				t.Errorf("responseWriter.wroteHeader = %v, want %v", rw.wroteHeader, tt.expectedWrote)
			}
		})
	}
}

// TestResponseWriter_Write tests the responseWriter Write method
func TestResponseWriter_Write(t *testing.T) {
	tests := []struct {
		name                string
		writeHeaderFirst    bool
		initialStatus       int
		body                string
		expectedStatus      int
		expectedWroteHeader bool
	}{
		{
			name:                "write without header sets implicit 200",
			writeHeaderFirst:    false,
			initialStatus:       http.StatusOK,
			body:                "test body",
			expectedStatus:      http.StatusOK,
			expectedWroteHeader: true,
		},
		{
			name:                "write after header preserves status",
			writeHeaderFirst:    true,
			initialStatus:       http.StatusCreated,
			body:                "created resource",
			expectedStatus:      http.StatusCreated,
			expectedWroteHeader: true,
		},
		{
			name:                "empty write still triggers header",
			writeHeaderFirst:    false,
			initialStatus:       http.StatusOK,
			body:                "",
			expectedStatus:      http.StatusOK,
			expectedWroteHeader: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := &responseWriter{
				ResponseWriter: w,
				status:         tt.initialStatus,
			}

			if tt.writeHeaderFirst {
				rw.WriteHeader(tt.initialStatus)
			}

			n, err := rw.Write([]byte(tt.body))
			if err != nil {
				t.Errorf("responseWriter.Write() error = %v", err)
			}

			if n != len(tt.body) {
				t.Errorf("responseWriter.Write() returned %d bytes, want %d", n, len(tt.body))
			}

			if rw.status != tt.expectedStatus {
				t.Errorf("responseWriter.status = %d, want %d", rw.status, tt.expectedStatus)
			}

			if rw.wroteHeader != tt.expectedWroteHeader {
				t.Errorf("responseWriter.wroteHeader = %v, want %v", rw.wroteHeader, tt.expectedWroteHeader)
			}
		})
	}
}

// TestLoggingMiddleware tests the LoggingMiddleware function
func TestLoggingMiddleware(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		path            string
		config          LogConfig
		handlerStatus   int
		expectRequestID bool
	}{
		{
			name:            "basic GET request with text format",
			method:          http.MethodGet,
			path:            "/v1/memory/retrieve",
			config:          LogConfig{Format: "text", SkipHealth: false},
			handlerStatus:   http.StatusOK,
			expectRequestID: true,
		},
		{
			name:            "POST request with json format",
			method:          http.MethodPost,
			path:            "/v1/memory/ingest",
			config:          LogConfig{Format: "json", SkipHealth: false},
			handlerStatus:   http.StatusCreated,
			expectRequestID: true,
		},
		{
			name:            "error status is captured",
			method:          http.MethodGet,
			path:            "/v1/memory/retrieve",
			config:          LogConfig{Format: "text", SkipHealth: false},
			handlerStatus:   http.StatusInternalServerError,
			expectRequestID: true,
		},
		{
			name:            "health endpoint with skip disabled",
			method:          http.MethodGet,
			path:            "/healthz",
			config:          LogConfig{Format: "text", SkipHealth: false},
			handlerStatus:   http.StatusOK,
			expectRequestID: true,
		},
		{
			name:            "health endpoint with skip enabled",
			method:          http.MethodGet,
			path:            "/healthz",
			config:          LogConfig{Format: "text", SkipHealth: true},
			handlerStatus:   http.StatusOK,
			expectRequestID: true,
		},
		{
			name:            "readyz endpoint with skip enabled",
			method:          http.MethodGet,
			path:            "/readyz",
			config:          LogConfig{Format: "text", SkipHealth: true},
			handlerStatus:   http.StatusOK,
			expectRequestID: true,
		},
		{
			name:            "default format when empty",
			method:          http.MethodGet,
			path:            "/test",
			config:          LogConfig{Format: "", SkipHealth: false},
			handlerStatus:   http.StatusOK,
			expectRequestID: true,
		},
		{
			name:            "JSON format uppercase",
			method:          http.MethodGet,
			path:            "/test",
			config:          LogConfig{Format: "JSON", SkipHealth: false},
			handlerStatus:   http.StatusOK,
			expectRequestID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler that returns the specified status
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.handlerStatus)
			})

			// Wrap with middleware
			wrapped := LoggingMiddleware(handler, tt.config)

			// Create test request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			// Execute
			wrapped.ServeHTTP(w, req)

			// Verify status code is captured correctly
			if w.Code != tt.handlerStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.handlerStatus)
			}

			// Verify X-Request-ID header is set
			if tt.expectRequestID {
				requestID := w.Header().Get("X-Request-ID")
				if requestID == "" {
					t.Error("X-Request-ID header not set")
				}
				// Verify it's a valid hex string
				_, err := hex.DecodeString(requestID)
				if err != nil {
					t.Errorf("X-Request-ID is not valid hex: %s", requestID)
				}
			}
		})
	}
}

// TestLoggingMiddleware_RequestIDUniqueness tests that each request gets a unique ID
func TestLoggingMiddleware_RequestIDUniqueness(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := LoggingMiddleware(handler, LogConfig{Format: "text", SkipHealth: false})

	// Make multiple requests and collect request IDs
	ids := make(map[string]bool)
	numRequests := 10

	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)

		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Fatalf("Request %d: X-Request-ID header not set", i)
		}

		if ids[requestID] {
			t.Errorf("Request %d: duplicate request ID: %s", i, requestID)
		}
		ids[requestID] = true
	}

	if len(ids) != numRequests {
		t.Errorf("Expected %d unique IDs, got %d", numRequests, len(ids))
	}
}

// TestLoggingMiddleware_PassesRequestThrough tests that the middleware properly passes
// the request to the next handler
func TestLoggingMiddleware_PassesRequestThrough(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		path         string
		responseBody string
	}{
		{
			name:         "GET request passes through",
			method:       http.MethodGet,
			path:         "/v1/memory/retrieve",
			responseBody: `{"result": "ok"}`,
		},
		{
			name:         "POST request passes through",
			method:       http.MethodPost,
			path:         "/v1/memory/ingest",
			responseBody: `{"id": "123"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request properties are preserved
				if r.Method != tt.method {
					t.Errorf("handler received method = %s, want %s", r.Method, tt.method)
				}
				if r.URL.Path != tt.path {
					t.Errorf("handler received path = %s, want %s", r.URL.Path, tt.path)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.responseBody))
			})

			wrapped := LoggingMiddleware(handler, LogConfig{Format: "text"})

			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			// Verify response body is preserved
			if w.Body.String() != tt.responseBody {
				t.Errorf("response body = %q, want %q", w.Body.String(), tt.responseBody)
			}

			// Verify content-type header is preserved
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json")
			}
		})
	}
}

// TestLogConfig tests the LogConfig struct
func TestLogConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     LogConfig
		wantFormat string
		wantSkip   bool
	}{
		{
			name:       "default values",
			config:     LogConfig{},
			wantFormat: "",
			wantSkip:   false,
		},
		{
			name:       "text format",
			config:     LogConfig{Format: "text", SkipHealth: false},
			wantFormat: "text",
			wantSkip:   false,
		},
		{
			name:       "json format with skip",
			config:     LogConfig{Format: "json", SkipHealth: true},
			wantFormat: "json",
			wantSkip:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.Format != tt.wantFormat {
				t.Errorf("LogConfig.Format = %q, want %q", tt.config.Format, tt.wantFormat)
			}
			if tt.config.SkipHealth != tt.wantSkip {
				t.Errorf("LogConfig.SkipHealth = %v, want %v", tt.config.SkipHealth, tt.wantSkip)
			}
		})
	}
}

// TestLogEntry tests the logEntry struct JSON marshaling
func TestLogEntry(t *testing.T) {
	tests := []struct {
		name     string
		entry    logEntry
		wantJSON map[string]any
	}{
		{
			name: "basic entry",
			entry: logEntry{
				Timestamp: "2024-01-01T00:00:00Z",
				Method:    "GET",
				Path:      "/test",
				Status:    200,
				Duration:  100,
				RequestID: "abc123",
			},
			wantJSON: map[string]any{
				"timestamp":   "2024-01-01T00:00:00Z",
				"method":      "GET",
				"path":        "/test",
				"status":      float64(200), // JSON numbers are float64
				"duration_ms": float64(100),
				"request_id":  "abc123",
			},
		},
		{
			name: "error status entry",
			entry: logEntry{
				Timestamp: "2024-01-01T00:00:00Z",
				Method:    "POST",
				Path:      "/v1/memory/ingest",
				Status:    500,
				Duration:  50,
				RequestID: "def456",
			},
			wantJSON: map[string]any{
				"timestamp":   "2024-01-01T00:00:00Z",
				"method":      "POST",
				"path":        "/v1/memory/ingest",
				"status":      float64(500),
				"duration_ms": float64(50),
				"request_id":  "def456",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.entry)
			if err != nil {
				t.Fatalf("failed to marshal logEntry: %v", err)
			}

			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("failed to unmarshal JSON: %v", err)
			}

			for k, v := range tt.wantJSON {
				if got[k] != v {
					t.Errorf("JSON[%q] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}
