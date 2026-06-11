package ratelimit

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

// rejectedTotal tracks the number of rate-limited requests atomically.
var rejectedTotal int64

// RejectedTotal returns the total number of rate-limited requests.
func RejectedTotal() int64 {
	return atomic.LoadInt64(&rejectedTotal)
}

// Middleware returns HTTP middleware that applies rate limiting.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	var globalLimiter *Limiter
	var store *LimiterStore

	if cfg.ByIP {
		store = NewLimiterStore(cfg.RequestsPerSecond, cfg.BurstSize)
	} else {
		globalLimiter = NewLimiter(cfg.RequestsPerSecond, cfg.BurstSize)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip rate limiting for configured endpoints
			if cfg.SkipEndpoints != nil && cfg.SkipEndpoints[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			var limiter *Limiter
			if cfg.ByIP {
				ip := extractIP(r, cfg.TrustedProxies)
				limiter = store.GetLimiter(ip)
			} else {
				limiter = globalLimiter
			}

			if !limiter.Allow() {
				atomic.AddInt64(&rejectedTotal, 1)
				w.Header().Set("Retry-After", "1")
				w.Header().Set("X-RateLimit-Limit", formatFloat(limiter.Rate()))
				w.Header().Set("X-RateLimit-Remaining", "0")
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			// Add rate limit headers
			w.Header().Set("X-RateLimit-Limit", formatFloat(limiter.Rate()))
			w.Header().Set("X-RateLimit-Remaining", formatFloat(limiter.Tokens()))

			next.ServeHTTP(w, r)
		})
	}
}

// extractIP gets the client IP, respecting X-Forwarded-For from trusted proxies.
func extractIP(r *http.Request, trustedProxies []string) string {
	// Check X-Forwarded-For if we have trusted proxies
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" && len(trustedProxies) > 0 {
		remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if isTrustedProxy(remoteIP, trustedProxies) {
			// Take the first (leftmost) IP in the chain
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// isTrustedProxy checks if the IP is in the trusted proxies list.
func isTrustedProxy(ip string, trusted []string) bool {
	for _, t := range trusted {
		if t == ip {
			return true
		}
		// Check CIDR notation
		if strings.Contains(t, "/") {
			_, cidr, err := net.ParseCIDR(t)
			if err == nil && cidr.Contains(net.ParseIP(ip)) {
				return true
			}
		}
	}
	return false
}

// formatFloat formats a float64 for headers.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// AddRejectedForTest increments the rejection counter directly. Test-only:
// lets metrics tests exercise CollectRateLimitMetrics delta tracking without
// standing up the middleware.
func AddRejectedForTest(n int64) {
	atomic.AddInt64(&rejectedTotal, n)
}
