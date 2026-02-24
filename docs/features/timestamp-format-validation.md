# Timestamp Format Validation

The `timestamp_format` enum on ingest endpoints accepts multiple timestamp formats (RFC3339, Unix seconds, Unix milliseconds, date-only) and normalizes all to RFC3339 before Neo4j storage. This replaces the previous behavior of delegating format validation to Neo4j's `datetime()` function, which produced cryptic driver errors on invalid input.

## How It Works

### Format Enum

The optional `timestamp_format` field on `IngestRequest` and `BatchIngestItem` declares the format of the `timestamp` (and `canonical_time`) fields:

| Format | Example Input | Parsed As |
|--------|---------------|-----------|
| `rfc3339` (default) | `"2026-02-09T10:30:00Z"` | `time.RFC3339` |
| `unix` | `"1739054400"` | `time.Unix(n, 0)` |
| `unix_ms` | `"1739054400000"` | `time.UnixMilli(n)` |
| `date_only` | `"2026-02-09"` | `"2006-01-02"` layout, midnight UTC |

When omitted, `timestamp_format` defaults to `rfc3339` for full backward compatibility.

### Normalization

All formats are parsed Go-side, then normalized to RFC3339 UTC before the value reaches Neo4j. This means:

- Neo4j `datetime()` always receives a consistent RFC3339 string
- Invalid timestamps produce clear 400 errors with format hints (e.g., `"timestamp '...' is not valid unix format. Expected: integer seconds since epoch"`)
- Batch ingest errors include the item index (e.g., `"observations[3]: timestamp '...' is not valid rfc3339 format"`)

### Validation Flow

```
Client sends timestamp + timestamp_format
    |
    v
Struct validation (oneof enum check)
    |
    v
NormalizeTimestamp(value, format) → RFC3339 string or 400 error
    |
    v
Replace request field with normalized value
    |
    v
Pass to retriever (Neo4j always sees RFC3339)
```

## Usage

```bash
# Unix seconds
curl -s -X POST http://localhost:9999/v1/memory/ingest \
  -H "Content-Type: application/json" \
  -d '{"space_id":"my-space","timestamp":"1739054400","timestamp_format":"unix","source":"cli","content":"Test"}'

# Date only (midnight UTC)
curl -s -X POST http://localhost:9999/v1/memory/ingest \
  -H "Content-Type: application/json" \
  -d '{"space_id":"my-space","timestamp":"2026-02-09","timestamp_format":"date_only","source":"cli","content":"Test"}'

# Default (no format field) — RFC3339 assumed
curl -s -X POST http://localhost:9999/v1/memory/ingest \
  -H "Content-Type: application/json" \
  -d '{"space_id":"my-space","timestamp":"2026-02-09T10:30:00Z","source":"cli","content":"Test"}'
```

## Related Files

| File | Description |
|------|-------------|
| `internal/models/models.go` | `TimestampFormat` field on `IngestRequest` and `BatchIngestItem` |
| `internal/models/timestamp.go` | `ParseTimestamp` and `NormalizeTimestamp` functions |
| `internal/models/timestamp_test.go` | Unit tests (25 cases across all formats) |
| `internal/api/handlers.go` | Normalization calls in `handleIngest` and `handleBatchIngest` |
| `docs/api/api-spec/uats/specs/ingest.uats.json` | UATS contract tests (3 new variants) |

## Backward Compatibility

- `timestamp_format` is optional — omitting it defaults to `rfc3339`
- Existing clients sending RFC3339 timestamps without the field continue working unchanged
- The only behavioral change: malformed timestamps that previously passed through to Neo4j are now rejected with clear 400 errors
