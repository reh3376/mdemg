---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "76"
---

# Timestamp Format Validation

## Summary

**Feature**: Timestamp Format Validation
**Summary**: The `timestamp_format` enum on ingest endpoints accepts multiple timestamp formats (RFC3339, Unix seconds, Unix milliseconds, date-only) and normalizes all to RFC3339 before Neo4j storage, replacing cryptic driver errors with clear 400 responses.

## Vision & Goals

Observation timestamps are foundational to MDEMG's temporal reasoning, decay calculations, and staleness detection. Supporting multiple input formats with server-side normalization ensures clients can send timestamps in their native format while the graph maintains a consistent RFC3339 representation. This eliminates a class of silent data quality issues where malformed timestamps passed through to Neo4j and produced cryptic driver errors.

## Current State

### Architecture

The optional `timestamp_format` field on `IngestRequest` and `BatchIngestItem` declares the format of the `timestamp` (and `canonical_time`) fields:

| Format | Example Input | Parsed As |
|--------|---------------|-----------|
| `rfc3339` (default) | `"2026-02-09T10:30:00Z"` | `time.RFC3339` |
| `unix` | `"1739054400"` | `time.Unix(n, 0)` |
| `unix_ms` | `"1739054400000"` | `time.UnixMilli(n)` |
| `date_only` | `"2026-02-09"` | `"2006-01-02"` layout, midnight UTC |

When omitted, `timestamp_format` defaults to `rfc3339` for full backward compatibility.

### Workflow

```
Client sends timestamp + timestamp_format
    |
    v
Struct validation (oneof enum check)
    |
    v
NormalizeTimestamp(value, format) -> RFC3339 string or 400 error
    |
    v
Replace request field with normalized value
    |
    v
Pass to retriever (Neo4j always sees RFC3339)
```

All formats are parsed Go-side, then normalized to RFC3339 UTC before the value reaches Neo4j. This means:

- Neo4j `datetime()` always receives a consistent RFC3339 string
- Invalid timestamps produce clear 400 errors with format hints (e.g., `"timestamp '...' is not valid unix format. Expected: integer seconds since epoch"`)
- Batch ingest errors include the item index (e.g., `"observations[3]: timestamp '...' is not valid rfc3339 format"`)

### Configuration

No configuration required. The feature is always active on all ingest endpoints.

## Notes

### Known Limitations

- `date_only` format assumes midnight UTC — no timezone support for date-only inputs

### Risks & Gaps

None identified.

### Future Improvements

None planned.

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| POST | `/v1/memory/ingest` | Single observation ingest with `timestamp_format` field | `specs/ingest.uats.json` |
| POST | `/v1/memory/batch-ingest` | Batch ingest with per-item `timestamp_format` | `specs/batch-ingest.uats.json` |

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg ingest` | CLI ingest uses RFC3339 by default |

## Configuration Reference

None — feature is always active with no configurable parameters.

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Ingest Pipeline | Requires — validation runs inside ingest handlers |
| Neo4j Storage | Feeds into — normalized timestamps stored as `datetime()` |

## Related Files

- `internal/models/models.go` - `TimestampFormat` field on `IngestRequest` and `BatchIngestItem`
- `internal/models/timestamp.go` - `ParseTimestamp` and `NormalizeTimestamp` functions
- `internal/models/timestamp_test.go` - Unit tests (25 cases across all formats)
- `internal/api/handlers.go` - Normalization calls in `handleIngest` and `handleBatchIngest`
- `docs/api/api-spec/uats/specs/ingest.uats.json` - UATS contract tests (3 timestamp variants)
