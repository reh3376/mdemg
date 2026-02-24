# UAMS - Universal Auth Method Specification

UAMS is a declarative framework for defining authentication method contracts in MDEMG. It provides:

1. **Declarative auth method definitions** via JSON specs
2. **Pluggable auth method registration** via Go interface + registry
3. **Automated test generation** from UAMS specs
4. **Standardized extension pattern** for new auth methods

## Relationship to Other Frameworks

| Framework | Purpose |
|-----------|---------|
| UPTS | Parser contracts - parsers implement them |
| **UAMS** | Auth method contracts - authenticators implement them |
| USTS | Security tests - consumes UAMS specs for test generation |
| UBTS | Benchmark tests |
| UOBS | Observability specs |

## Directory Structure

```
docs/tests/uams/
├── schema/
│   └── uams.schema.json      # JSON Schema for method specs
├── specs/
│   ├── apikey.uams.json      # API key method spec
│   ├── jwt.uams.json         # JWT Bearer method spec
│   └── none.uams.json        # No-auth method spec
├── fixtures/
│   ├── valid_apikey.txt      # Valid API key fixture
│   ├── valid_jwt.txt         # Valid JWT fixture
│   └── invalid_*.txt         # Invalid credential fixtures
├── runners/
│   └── uams_runner.go        # Go test harness (future)
└── README.md
```

## Spec Structure

Each UAMS spec defines:

### Method Metadata

```json
{
  "method": {
    "name": "apikey",
    "type": "apikey",
    "description": "Static API key authentication",
    "status": "stable"
  }
}
```

### Credential Extraction

Defines where and how credentials are extracted from requests:

```json
{
  "credentials": {
    "extraction": [
      { "source": "header", "name": "X-API-Key", "priority": 1 },
      { "source": "header", "name": "Authorization", "prefix": "Bearer ", "priority": 2 }
    ],
    "format": {
      "type": "opaque",
      "min_length": 16
    }
  }
}
```

### Validation Rules

```json
{
  "validation": {
    "algorithm": "sha256-hmac-compare",
    "timing_safe": true,
    "checks": ["signature", "expiry"],
    "config_required": ["keys"]
  }
}
```

### Principal Construction

```json
{
  "principal": {
    "id_source": "key_hash",
    "metadata_fields": ["scopes"]
  }
}
```

### Error Responses

```json
{
  "errors": [
    {
      "code": "invalid_api_key",
      "status": 401,
      "message": "Invalid API key",
      "www_authenticate": "ApiKey realm=\"mdemg\""
    }
  ]
}
```

## Go Implementation

### Authenticator Interface

```go
// Authenticator validates credentials and returns a Principal.
type Authenticator interface {
    Name() string
    Authenticate(r *http.Request) (*Principal, error)
}
```

### Registry Pattern

```go
// Register a new auth method
auth.DefaultRegistry().MustRegister("oauth2", newOAuth2Authenticator)

// Build an authenticator
authenticator, err := auth.DefaultRegistry().Build("apikey", config)
```

### Adding a New Auth Method

1. Create UAMS spec: `docs/tests/uams/specs/mymethod.uams.json`
2. Implement `Authenticator` interface
3. Register with `DefaultRegistry().MustRegister("mymethod", factory)`
4. Run UAMS conformance tests

## Validation

Validate specs against the schema:

```bash
# Using ajv-cli
npx ajv validate -s schema/uams.schema.json -d "specs/*.uams.json"

# Using Go tests
go test ./internal/auth/... -run TestUAMS -v
```

## Current Auth Methods

| Method | Type | Status | Description |
|--------|------|--------|-------------|
| `none` | none | stable | Pass-through (development) |
| `apikey` | apikey | stable | Static API key with timing protection |
| `jwt` | bearer | stable | JWT Bearer tokens (HS256) |
| `saml` | bearer | stable | Microsoft Entra ID SAML 2.0 assertions |

## Security Features

- **Timing-safe comparison**: All credential validation uses constant-time operations
- **Hashed storage**: API keys are stored as SHA-256 hashes
- **Structured errors**: Consistent error codes and WWW-Authenticate headers
- **Metadata propagation**: Principal metadata available via context
