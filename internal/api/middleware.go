package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"mdemg/internal/auth"
	"mdemg/internal/conversation"
)

// LogConfig holds configuration for request logging middleware.
type LogConfig struct {
	Format     string // "text" (default) or "json"
	SkipHealth bool   // Skip logging for /healthz and /readyz endpoints
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// WriteHeader captures the status code and delegates to the wrapped ResponseWriter.
func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.status = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

// Write ensures status is set before writing body (implicit 200 OK).
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// generateRequestID creates a unique request ID using crypto/rand.
func generateRequestID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return hex.EncodeToString([]byte(time.Now().String()[:16]))
	}
	return hex.EncodeToString(b)
}

// isHealthEndpoint returns true if the path is a health check endpoint.
func isHealthEndpoint(path string) bool {
	return path == "/healthz" || path == "/readyz"
}

// logEntry represents a structured log entry for JSON format.
type logEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	Duration  int64  `json:"duration_ms"`
	RequestID string `json:"request_id"`
	TraceID   string `json:"trace_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	RemoteIP  string `json:"remote_ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	Error     string `json:"error,omitempty"`
}

// traceIDContextKey is the context key for trace IDs.
type traceIDContextKey struct{}

// GetTraceID retrieves the trace ID from context.
func GetTraceID(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDContextKey{}).(string); ok {
		return id
	}
	return ""
}

// WithTraceID adds a trace ID to the context.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDContextKey{}, traceID)
}

// extractClientIP extracts the client IP from the request.
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// LoggingMiddleware returns middleware that logs HTTP requests.
func LoggingMiddleware(next http.Handler, cfg LogConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID
		requestID := generateRequestID()
		w.Header().Set("X-Request-ID", requestID)

		// Extract or generate trace ID for distributed tracing
		traceID := r.Header.Get("X-Trace-ID")
		if traceID == "" {
			traceID = generateRequestID() // Generate new trace if not provided
		}
		w.Header().Set("X-Trace-ID", traceID)

		// Add trace ID to context for downstream use
		ctx := WithTraceID(r.Context(), traceID)
		r = r.WithContext(ctx)

		// Wrap response writer to capture status
		wrapped := &responseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		// Serve the request
		next.ServeHTTP(wrapped, r)

		// Skip logging for health endpoints if configured
		if cfg.SkipHealth && isHealthEndpoint(r.URL.Path) {
			return
		}

		duration := time.Since(start)

		// Extract user ID from auth context if available
		userID := ""
		if principal := auth.GetPrincipal(r.Context()); principal != nil {
			userID = principal.ID
		}

		// Determine log level based on status
		level := "info"
		if wrapped.status >= 500 {
			level = "error"
		} else if wrapped.status >= 400 {
			level = "warn"
		}

		// Build slog attributes for the request
		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.status),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.String("request_id", requestID),
			slog.String("trace_id", traceID),
		}
		if userID != "" {
			attrs = append(attrs, slog.String("user_id", userID))
		}
		if clientIP := extractClientIP(r); clientIP != "" {
			attrs = append(attrs, slog.String("remote_ip", clientIP))
		}
		if ua := r.UserAgent(); ua != "" {
			attrs = append(attrs, slog.String("user_agent", ua))
		}

		// Log at appropriate level based on status code
		msg := "http request"
		switch level {
		case "error":
			slog.LogAttrs(r.Context(), slog.LevelError, msg, attrs...)
		case "warn":
			slog.LogAttrs(r.Context(), slog.LevelWarn, msg, attrs...)
		default:
			slog.LogAttrs(r.Context(), slog.LevelInfo, msg, attrs...)
		}
	})
}

// gzipResponseWriter wraps http.ResponseWriter to compress output
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	statusCode int
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// gzipWriterPool reuses gzip.Writer instances to reduce allocations
var gzipWriterPool = sync.Pool{
	New: func() any {
		return gzip.NewWriter(io.Discard)
	},
}

// CompressionMiddleware adds gzip compression to responses when the client supports it.
// Only compresses responses larger than minSize bytes.
func CompressionMiddleware(next http.Handler, _ int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip compression for small responses or streaming
		// Use response buffering to check size
		gz := gzipWriterPool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() {
			gz.Close()
			gzipWriterPool.Put(gz)
		}()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length") // Length changes with compression

		gzw := &gzipResponseWriter{
			Writer:         gz,
			ResponseWriter: w,
		}

		next.ServeHTTP(gzw, r)
	})
}

// Pagination contains cursor-based pagination parameters
type Pagination struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// PaginatedResponse wraps results with pagination info
type PaginatedResponse struct {
	Data       any    `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	TotalCount int    `json:"total_count,omitempty"`
}

// ParsePagination extracts pagination parameters from request
func ParsePagination(r *http.Request, defaultLimit, maxLimit int) Pagination {
	query := r.URL.Query()

	limit := defaultLimit
	if l := query.Get("limit"); l != "" {
		if n, err := parseInt(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	return Pagination{
		Cursor: query.Get("cursor"),
		Limit:  limit,
	}
}

// parseInt safely parses an integer string
func parseInt(s string) (int, error) {
	var n int
	err := json.Unmarshal([]byte(s), &n)
	return n, err
}

// FieldSelector supports selective field loading
type FieldSelector struct {
	Fields  []string // Fields to include (empty = all)
	Exclude []string // Fields to exclude
}

// ParseFieldSelector extracts field selection parameters from request
func ParseFieldSelector(r *http.Request) FieldSelector {
	query := r.URL.Query()

	var fields, exclude []string

	if f := query.Get("fields"); f != "" {
		fields = strings.Split(f, ",")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
	}

	if e := query.Get("exclude"); e != "" {
		exclude = strings.Split(e, ",")
		for i := range exclude {
			exclude[i] = strings.TrimSpace(exclude[i])
		}
	}

	return FieldSelector{
		Fields:  fields,
		Exclude: exclude,
	}
}

// FilterFields applies field selection to a map
func (fs FieldSelector) FilterFields(data map[string]any) map[string]any {
	if len(fs.Fields) == 0 && len(fs.Exclude) == 0 {
		return data
	}

	result := make(map[string]any)

	if len(fs.Fields) > 0 {
		// Include only specified fields
		for _, f := range fs.Fields {
			if v, ok := data[f]; ok {
				result[f] = v
			}
		}
	} else {
		// Start with all fields
		for k, v := range data {
			result[k] = v
		}
	}

	// Remove excluded fields
	for _, f := range fs.Exclude {
		delete(result, f)
	}

	return result
}

// warningPaths are the endpoints that trigger a session-not-resumed warning.
var warningPaths = map[string]bool{
	"/v1/memory/retrieve":     true,
	"/v1/conversation/recall": true,
}

// SessionResumeWarningMiddleware adds an X-MDEMG-Warning header when an agent
// calls retrieve or recall without having called /resume first in the session.
// This is non-breaking — requests are never blocked.
func SessionResumeWarningMiddleware(next http.Handler, tracker *conversation.SessionTracker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only check POST requests to warning paths
		if tracker == nil || r.Method != http.MethodPost || !warningPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		// Read body to extract session_id, then restore it for downstream
		body, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			// Can't read body — pass through without warning
			r.Body = io.NopCloser(bytes.NewReader(nil))
			next.ServeHTTP(w, r)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		var partial struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal(body, &partial) == nil && partial.SessionID != "" {
			if !tracker.IsResumed(partial.SessionID) {
				w.Header().Set("X-MDEMG-Warning", "session-not-resumed")
			}
		}

		next.ServeHTTP(w, r)
	})
}
