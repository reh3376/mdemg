# Feature Spec: [TITLE]

**Phase**: [Phase N]
**Status**: Draft | In Review | Approved | Implemented
**Author**: [name]
**Date**: YYYY-MM-DD

---

## Overview

Brief description of the feature and its purpose within MDEMG.

## Requirements

### Functional Requirements

1. FR-1: ...
2. FR-2: ...

### Non-Functional Requirements

1. NFR-1: Performance — ...
2. NFR-2: Security — ...

## API Contract

### Endpoints

```
METHOD /v1/path
```

**Request:**

```json
{
  "field": "type — description"
}
```

**Response:**

```json
{
  "field": "type — description"
}
```

### Error Codes

| Code | Meaning |
|------|---------|
| 400  | ...     |
| 404  | ...     |

## Data Model

### Neo4j Schema Changes

```cypher
// New nodes, constraints, indexes
```

### Go Types

```go
// New or modified structs
```

## Test Plan

### Unit Tests

- [ ] Test case 1: ...
- [ ] Test case 2: ...

### Integration Tests

- [ ] Test case 1: ...

### Benchmark Tests

- [ ] Benchmark case 1: ...

## Acceptance Criteria

- [ ] AC-1: ...
- [ ] AC-2: ...
- [ ] AC-3: All existing tests pass (`go test ./...`)
- [ ] AC-4: `go vet ./...` reports no issues
- [ ] AC-5: SHA256 hash added to `docs/specs/manifest.sha256`

## Dependencies

- Depends on: [list any prerequisite specs/features]
- Blocks: [list specs/features that depend on this]

## Files Changed

### New Files

- `path/to/new/file.go` — description

### Modified Files

- `path/to/existing/file.go` — what changes
