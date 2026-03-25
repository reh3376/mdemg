package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
)

// Middleware returns HTTP middleware that enforces authentication.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	var apiKeyValidator *APIKeyValidator
	var jwtValidator *JWTValidator

	switch cfg.Mode {
	case ModeAPIKey:
		apiKeyValidator = NewAPIKeyValidator(cfg.APIKeys)
	case ModeBearer:
		jwtValidator = NewJWTValidator(cfg.JWTSecret, cfg.JWTIssuer)
	case ModeNone:
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip authentication for configured endpoints
			if cfg.SkipEndpoints != nil && cfg.SkipEndpoints[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			var principal *Principal
			var err error

			switch cfg.Mode {
			case ModeAPIKey:
				principal, err = authenticateAPIKey(r, apiKeyValidator)
			case ModeBearer:
				principal, err = authenticateBearer(r, jwtValidator)
			}

			if err != nil {
				slog.Warn("auth failed", "error", err, "path", r.URL.Path, "remote", r.RemoteAddr)
				writeAuthError(w, err)
				return
			}

			if principal == nil {
				slog.Warn("auth missing", "path", r.URL.Path, "remote", r.RemoteAddr)
				writeAuthError(w, errMissingCredentials)
				return
			}

			// Add principal to context and continue
			ctx := WithPrincipal(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

var errMissingCredentials = &authError{
	status:  http.StatusUnauthorized,
	code:    "missing_credentials",
	message: "authentication required",
}

type authError struct {
	status  int
	code    string
	message string
}

func (e *authError) Error() string {
	return e.message
}

// authenticateAPIKey extracts and validates an API key.
func authenticateAPIKey(r *http.Request, validator *APIKeyValidator) (*Principal, error) {
	headers := make(map[string]string)
	headers["X-API-Key"] = r.Header.Get("X-API-Key")
	headers["Authorization"] = r.Header.Get("Authorization")

	query := make(map[string]string)
	query["api_key"] = r.URL.Query().Get("api_key")

	key := ExtractAPIKey(headers, query)
	if key == "" {
		return nil, nil
	}

	id, valid := validator.Validate(key)
	if !valid {
		return nil, &authError{
			status:  http.StatusUnauthorized,
			code:    "invalid_api_key",
			message: "invalid API key",
		}
	}

	return &Principal{
		ID:   id,
		Type: ModeAPIKey,
	}, nil
}

// authenticateBearer extracts and validates a Bearer token.
func authenticateBearer(r *http.Request, validator *JWTValidator) (*Principal, error) {
	token := ExtractBearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return nil, nil
	}

	claims, err := validator.Validate(token)
	if err != nil {
		return nil, &authError{
			status:  http.StatusUnauthorized,
			code:    "invalid_token",
			message: err.Error(),
		}
	}

	return &Principal{
		ID:     claims.Subject,
		Type:   ModeBearer,
		Scopes: claims.Scopes,
		Metadata: map[string]any{
			"issuer":  claims.Issuer,
			"expires": claims.ExpiresAt,
		},
	}, nil
}

// writeAuthError writes an authentication error response.
func writeAuthError(w http.ResponseWriter, err error) {
	status := http.StatusUnauthorized
	code := "unauthorized"
	message := "authentication failed"

	if ae, ok := err.(*authError); ok {
		status = ae.status
		code = ae.code
		message = ae.message
	}

	if ae, ok := err.(*AuthError); ok {
		status = ae.Status
		code = ae.Code
		message = ae.Message
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `ApiKey realm="mdemg"`)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error":   code,
		"message": message,
	})
}

// RequireScope returns per-route middleware that enforces a specific authorization scope (GAP-16).
// If the principal is nil (unauthenticated), returns 401. If the principal lacks the required
// scope, returns 403. When auth is disabled (ModeNone), the middleware is a no-op pass-through.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := GetPrincipal(r.Context())

			// If no principal in context, auth middleware hasn't run or auth is disabled.
			// In that case, pass through — the auth middleware handles 401 already.
			if p == nil {
				next.ServeHTTP(w, r)
				return
			}

			// API key principals with no scopes get full access (backward compat).
			if p.Type == ModeAPIKey && len(p.Scopes) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			if !p.HasScope(scope) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]any{
					"error":          "forbidden",
					"message":        "insufficient scope",
					"required_scope": scope,
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// MiddlewareWithRegistry creates middleware using the registry pattern.
// This enables pluggable authentication methods.
func MiddlewareWithRegistry(cfg Config, registry *Registry) func(http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	if cfg.Mode == ModeNone {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	// Build authenticator from registry
	methodCfg := cfg.GetMethodConfig()
	auth, err := registry.Build(string(cfg.Mode), methodCfg)
	if err != nil {
		slog.Error("failed to build authenticator", "error", err)
		os.Exit(1)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip authentication for configured endpoints
			if cfg.SkipEndpoints != nil && cfg.SkipEndpoints[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			principal, err := auth.Authenticate(r)
			if err != nil {
				slog.Warn("auth failed", "error", err, "path", r.URL.Path, "remote", r.RemoteAddr)
				writeAuthError(w, err)
				return
			}

			if principal == nil {
				slog.Warn("auth missing", "path", r.URL.Path, "remote", r.RemoteAddr)
				writeAuthError(w, errMissingCredentials)
				return
			}

			// Add principal to context and continue
			ctx := WithPrincipal(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
