package api

import (
	"net/http"
	"strings"
)

// CORSConfig holds CORS middleware configuration.
type CORSConfig struct {
	// Enabled controls whether CORS headers are added
	Enabled bool

	// AllowedOrigins is a list of allowed origins (or "*" for all)
	AllowedOrigins []string

	// AllowedMethods is a list of allowed HTTP methods
	AllowedMethods []string

	// AllowedHeaders is a list of allowed request headers
	AllowedHeaders []string

	// ExposedHeaders is a list of headers clients can access
	ExposedHeaders []string

	// AllowCredentials allows cookies and auth headers
	AllowCredentials bool

	// MaxAge is the preflight cache duration in seconds
	MaxAge int
}

// DefaultCORSConfig returns sensible defaults for CORS.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		Enabled:          false, // Disabled by default
		AllowedOrigins:   []string{},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining"},
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
	}
}

// isLocalhostOrigin returns true if the origin is http://localhost with any port.
func isLocalhostOrigin(origin string) bool {
	return strings.HasPrefix(origin, "http://localhost:") || origin == "http://localhost"
}

// CORSMiddleware adds CORS headers to responses and handles preflight requests.
// When CORS is disabled, it still allows localhost origins for the browser dashboard
// to communicate with other MDEMG instances on different ports.
func CORSMiddleware(cfg CORSConfig) func(http.Handler) http.Handler {
	if !cfg.Enabled {
		// Even when CORS is disabled, allow localhost cross-port requests
		// so the dashboard can talk to other local MDEMG instances.
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				origin := r.Header.Get("Origin")
				if origin != "" && isLocalhostOrigin(origin) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-API-Key, X-Request-ID")
					if r.Method == "OPTIONS" {
						w.Header().Set("Access-Control-Max-Age", "86400")
						w.WriteHeader(http.StatusNoContent)
						return
					}
				}
				next.ServeHTTP(w, r)
			})
		}
	}

	// Build allowed origins map for O(1) lookup
	allowedOriginsMap := make(map[string]bool)
	allowAllOrigins := false
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			allowAllOrigins = true
			break
		}
		allowedOriginsMap[o] = true
	}

	// Precompute header values
	methodsHeader := strings.Join(cfg.AllowedMethods, ", ")
	headersHeader := strings.Join(cfg.AllowedHeaders, ", ")
	exposedHeader := strings.Join(cfg.ExposedHeaders, ", ")
	maxAgeHeader := ""
	if cfg.MaxAge > 0 {
		maxAgeHeader = itoa(cfg.MaxAge)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// No origin header = same-origin request, skip CORS
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check if origin is allowed
			allowed := allowAllOrigins || allowedOriginsMap[origin]
			if !allowed {
				// Origin not allowed, continue without CORS headers
				next.ServeHTTP(w, r)
				return
			}

			// Set CORS headers
			if allowAllOrigins {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}

			if cfg.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if exposedHeader != "" {
				w.Header().Set("Access-Control-Expose-Headers", exposedHeader)
			}

			// Handle preflight request
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", methodsHeader)
				w.Header().Set("Access-Control-Allow-Headers", headersHeader)
				if maxAgeHeader != "" {
					w.Header().Set("Access-Control-Max-Age", maxAgeHeader)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// itoa converts an int to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return sign + result
}
