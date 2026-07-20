package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// HTTPMiddleware returns middleware that collects HTTP metrics.
func HTTPMiddleware(registry *Registry) func(http.Handler) http.Handler {
	if registry == nil {
		registry = globalRegistry
	}

	m := NewStandardMetrics(registry)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start)
			status := strconv.Itoa(wrapped.status)
			method := r.Method
			path := r.URL.Path

			m.HTTPRequestsTotal(method, path, status).Inc()
			m.HTTPRequestDuration(method, path).Observe(duration.Seconds())
		})
	}
}

// statusResponseWriter wraps http.ResponseWriter to capture the status code.
type statusResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.status = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
