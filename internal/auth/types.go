package auth

import (
	"context"
	"errors"
)

// AuthMode specifies the authentication method.
type AuthMode string

const (
	// ModeNone disables authentication.
	ModeNone AuthMode = "none"
	// ModeAPIKey requires an API key in X-API-Key header or Authorization: ApiKey <key>.
	ModeAPIKey AuthMode = "apikey"
	// ModeBearer requires a Bearer token in Authorization header.
	ModeBearer AuthMode = "bearer"
	// ModeSAML requires a SAML assertion from Microsoft Entra ID.
	ModeSAML AuthMode = "saml"
)

// Authorization scopes (GAP-16). These define the vocabulary for scope-based access control.
const (
	ScopeReadMemory   = "read:memory"
	ScopeWriteMemory  = "write:memory"
	ScopeDeleteMemory = "delete:memory"
	ScopeReadSpaces   = "read:spaces"
	ScopeWriteSpaces  = "write:spaces"
	ScopeAdminSpaces  = "admin:spaces"
	ScopeAll          = "*" // Superuser — matches all scopes.
)

// Config holds authentication configuration.
type Config struct {
	// Enabled controls whether authentication is required
	Enabled bool

	// Mode specifies the authentication method
	Mode AuthMode

	// APIKeys is the list of valid API keys (for ModeAPIKey)
	APIKeys []string

	// JWTSecret is the secret for validating JWT tokens (for ModeBearer)
	JWTSecret string

	// JWTIssuer is the expected issuer for JWT tokens
	JWTIssuer string

	// SAMLEntityID is the Service Provider entity ID (for ModeSAML)
	SAMLEntityID string

	// SAMLCertificate is the IdP certificate for signature validation (PEM format)
	SAMLCertificate string

	// SAMLIssuer is the expected IdP issuer (Microsoft Entra ID tenant URL)
	SAMLIssuer string

	// SkipEndpoints are paths that bypass authentication
	SkipEndpoints map[string]bool
}

// DefaultConfig returns sensible defaults for authentication.
func DefaultConfig() Config {
	return Config{
		Enabled: false, // Disabled by default for development
		Mode:    ModeNone,
		SkipEndpoints: map[string]bool{
			"/healthz": true,
			"/readyz":  true,
		},
	}
}

// Principal represents an authenticated identity.
type Principal struct {
	// ID is the unique identifier (API key hash, user ID, etc.)
	ID string

	// Type indicates the authentication method used
	Type AuthMode

	// Scopes is the set of granted authorization scopes (GAP-16).
	// Populated from JWT claims or API key configuration.
	Scopes []string

	// Metadata contains additional claims or info
	Metadata map[string]any
}

// HasScope returns true if the principal has the given scope.
func (p *Principal) HasScope(scope string) bool {
	if p == nil {
		return false
	}
	for _, s := range p.Scopes {
		if s == scope || s == "*" {
			return true
		}
	}
	return false
}

// HasAnyScope returns true if the principal has at least one of the given scopes.
func (p *Principal) HasAnyScope(scopes ...string) bool {
	if p == nil {
		return false
	}
	for _, scope := range scopes {
		if p.HasScope(scope) {
			return true
		}
	}
	return false
}

// contextKey is a private type for context keys.
type contextKey int

const (
	principalKey contextKey = iota
)

// WithPrincipal adds the principal to the context.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// GetPrincipal retrieves the principal from context, or nil if not authenticated.
func GetPrincipal(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}

// IsAuthenticated returns true if the context has a principal.
func IsAuthenticated(ctx context.Context) bool {
	return GetPrincipal(ctx) != nil
}

// AuthMethodConfig is implemented by method-specific configurations.
type AuthMethodConfig interface {
	// Validate checks if the configuration is valid.
	Validate() error

	// MethodName returns the method this config is for.
	MethodName() string
}

// APIKeyConfig configures API key authentication.
type APIKeyConfig struct {
	Keys []string
}

// Validate checks if the API key configuration is valid.
func (c *APIKeyConfig) Validate() error {
	if len(c.Keys) == 0 {
		return errors.New("apikey: at least one key required")
	}
	return nil
}

// MethodName returns the method name for API key auth.
func (c *APIKeyConfig) MethodName() string { return "apikey" }

// JWTConfig configures JWT Bearer authentication.
type JWTConfig struct {
	Secret string
	Issuer string
}

// Validate checks if the JWT configuration is valid.
func (c *JWTConfig) Validate() error {
	if c.Secret == "" {
		return errors.New("jwt: secret required")
	}
	return nil
}

// MethodName returns the method name for JWT auth.
func (c *JWTConfig) MethodName() string { return "jwt" }

// NoneConfig configures no-auth pass-through mode.
type NoneConfig struct{}

// Validate always returns nil for no-auth mode.
func (c *NoneConfig) Validate() error { return nil }

// MethodName returns the method name for no-auth mode.
func (c *NoneConfig) MethodName() string { return "none" }

// SAMLConfig configures Microsoft SAML authentication.
type SAMLConfig struct {
	// EntityID is the Service Provider entity ID (audience)
	EntityID string

	// Certificate is the IdP certificate for signature validation (PEM format)
	Certificate string

	// Issuer is the expected IdP issuer URL (e.g., https://sts.windows.net/{tenant-id}/)
	Issuer string

	// AllowClockSkew allows clock drift in seconds for time validation
	AllowClockSkew int
}

// Validate checks if the SAML configuration is valid.
func (c *SAMLConfig) Validate() error {
	if c.EntityID == "" {
		return errors.New("saml: entity_id required")
	}
	if c.Certificate == "" {
		return errors.New("saml: certificate required")
	}
	if c.Issuer == "" {
		return errors.New("saml: issuer required")
	}
	return nil
}

// MethodName returns the method name for SAML auth.
func (c *SAMLConfig) MethodName() string { return "saml" }

// GetMethodConfig returns the appropriate AuthMethodConfig for the given mode.
func (c *Config) GetMethodConfig() AuthMethodConfig {
	switch c.Mode {
	case ModeAPIKey:
		return &APIKeyConfig{Keys: c.APIKeys}
	case ModeBearer:
		return &JWTConfig{Secret: c.JWTSecret, Issuer: c.JWTIssuer}
	case ModeSAML:
		return &SAMLConfig{
			EntityID:    c.SAMLEntityID,
			Certificate: c.SAMLCertificate,
			Issuer:      c.SAMLIssuer,
		}
	default:
		return &NoneConfig{}
	}
}
