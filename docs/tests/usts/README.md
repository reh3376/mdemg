# USTS - Universal Security Test Specification

A standardized framework for defining and running security tests against MDEMG.

## Overview

USTS provides:

- **Declarative Specs**: JSON-based security test definitions
- **OWASP Mapping**: Tests mapped to OWASP Top 10 categories
- **Injection Payloads**: Pre-built payload libraries
- **Severity Levels**: Critical, High, Medium, Low classifications

## Directory Structure

```
usts/
├── schema/
│   └── usts.schema.json     # JSON Schema for validation
├── specs/
│   ├── control_char_injection.usts.json
│   ├── input_injection.usts.json
│   ├── metrics_snapshot_security.usts.json
│   ├── rate_limit_enforcement.usts.json
│   └── sensitive_data_exposure.usts.json
├── drafts/
│   ├── auth_required.usts.json
│   └── guardrail_enforcement.phase104.usts.json
├── payloads/
│   ├── cypher_injection.txt
│   └── sql_injection.txt
├── runners/
│   └── usts_runner.py       # Security test runner
└── README.md
```

## Quick Start

### 1. Install Dependencies

```bash
pip install requests
```

### 2. Run Injection Tests

```bash
cd docs/tests/usts
python runners/usts_runner.py \
  --spec specs/input_injection.usts.json \
  --base-url http://localhost:9999
```

### 3. Run With API Key (for authorized tests)

```bash
python runners/usts_runner.py \
  --spec specs/input_injection.usts.json \
  --api-key "your-valid-api-key" \
  --output results/
```

### 4. Run All Security Tests

```bash
python runners/usts_runner.py \
  --spec "specs/*.usts.json" \
  --api-key "your-api-key" \
  --output results/
```

## Test Categories

| Category | Description |
|----------|-------------|
| `authentication` | Verify auth requirements are enforced |
| `authorization` | Verify access control is correct |
| `injection` | Test for Cypher/SQL/command injection |
| `rate_limiting` | Verify rate limits are enforced |
| `data_exposure` | Check for sensitive data leaks |
| `headers` | Verify security headers are set |

## Severity Levels

| Level | Description | CI Behavior |
|-------|-------------|-------------|
| `critical` | Security breach possible | Exit code 2 |
| `high` | Significant vulnerability | Exit code 2 |
| `medium` | Moderate concern | Exit code 1 |
| `low` | Minor issue | Exit code 1 |

## Spec Format

```json
{
  "usts_version": "1.0.0",
  "test": {
    "name": "auth_required",
    "category": "authentication",
    "severity": "critical",
    "endpoint": "/v1/memory/retrieve"
  },
  "requests": [
    {
      "name": "no_auth",
      "payload": {"space_id": "test"},
      "expected_status": 401
    }
  ],
  "assertions": {
    "body_not_contains": ["results", "nodes"],
    "headers_present": ["WWW-Authenticate"]
  }
}
```

## Assertions

| Assertion | Description |
|-----------|-------------|
| `status_code` | Exact status code match |
| `status_in` | Status code in list |
| `body_contains` | Strings that must be in body |
| `body_not_contains` | Strings that must NOT be in body |
| `headers_present` | Headers that must exist |
| `headers_not_present` | Headers that must NOT exist |
| `response_time_ms_max` | Maximum response time |

## OWASP Top 10 Mapping

| Test | OWASP Categories |
|------|------------------|
| auth_required | A07: Authentication Failures |
| rate_limit_enforcement | A04: Insecure Design |
| input_injection | A03: Injection |
| sensitive_data_exposure | A01: Broken Access Control, A02: Crypto Failures |

## CI/CD Integration

```yaml
security-test:
  script:
    - python docs/tests/usts/runners/usts_runner.py \
        --spec "docs/tests/usts/specs/*.usts.json" \
        --api-key "$MDEMG_API_KEY"
  allow_failure: false  # Critical/High failures must block
```

## Adding Custom Payloads

Create payload files in `payloads/`:

```
# my_payloads.txt
' OR 1=1 --
<script>alert(1)</script>
{{constructor.prototype}}
```

Reference in specs:

```json
{
  "requests": [
    {
      "name": "custom_test",
      "payload_file": "payloads/my_payloads.txt"
    }
  ]
}
```
