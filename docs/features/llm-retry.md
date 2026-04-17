# LLM Retry with Exponential Backoff (SR-001)

Automatic retry logic for transient LLM API failures, integrated into `llmclient.Client`.

## How It Works

All LLM completions (OpenAI and Ollama) are wrapped in a retry loop. On transient failure, the client waits with exponential backoff before retrying, up to `MaxAttempts` retries.

```
Request → Fail (429/503) → Wait (backoff) → Retry → Fail → Wait (longer) → Retry → Success
```

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `LLM_RETRY_ENABLED` | `true` | Enable/disable retry logic |
| `LLM_RETRY_MAX_ATTEMPTS` | `3` | Maximum retry attempts (0 = no retries) |
| `LLM_RETRY_BASE_DELAY_MS` | `500` | Base delay before first retry |
| `LLM_RETRY_MAX_DELAY_MS` | `10000` | Maximum backoff delay cap |
| `LLM_RETRY_DEADLINE_ENABLED` | `true` | Retry once on `context.DeadlineExceeded` when remaining context budget > 2× base delay (DH-004) |

Set `LLM_RETRY_ENABLED=false` to restore single-attempt behaviour.

## Retryable vs Non-Retryable Errors

| Error | Retryable | Reason |
|-------|-----------|--------|
| HTTP 429 (Too Many Requests) | Yes | Rate limit — transient |
| HTTP 503 (Service Unavailable) | Yes | Temporary outage |
| Network timeout / connection refused | Yes | Transient infrastructure |
| HTTP 400 (Bad Request) | No | Client error — won't change on retry |
| HTTP 401 (Unauthorized) | No | Auth failure |
| HTTP 403 (Forbidden) | No | Permission denied |
| HTTP 404 (Not Found) | No | Wrong endpoint |
| HTTP 422 (Unprocessable Entity) | No | Invalid payload |
| HTTP 500 (Internal Server Error) | No | Server bug — conservative |
| `context.Canceled` | No | Caller cancelled |
| `context.DeadlineExceeded` | Conditional | When `LLM_RETRY_DEADLINE_ENABLED=true` AND remaining context budget > 2× base delay, retried once. Otherwise not retried (would certainly re-fail). Prevents single slow OpenAI responses from tripping circuit breakers while avoiding doubled spend under sustained slowness (DH-004). |

## Retry-After Header

On HTTP 429, if the response includes a `Retry-After` header, the client uses that value instead of the calculated backoff. The header is parsed as either:
- Integer seconds (e.g., `Retry-After: 5`)
- HTTP-date (e.g., `Retry-After: Mon, 07 Apr 2026 10:05:00 GMT`)

The `Retry-After` duration is capped at `LLM_RETRY_MAX_DELAY_MS`.

## Backoff Formula

```
delay = BaseDelay * Multiplier^attempt * (1 + Jitter * (rand - 0.5))
delay = min(delay, MaxDelay)
```

Default values: Multiplier=2.0, Jitter=0.2.

## Wiring

Retry config is set via `llmclient.SetDefaultRetryConfig()` at server startup, automatically applying to all 18+ LLM client construction sites without per-site modification.
