# Feature Spec: CMS Agent Enforcement

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: Phase 3A
**Status**: Implemented
**Author**: reh3376 & Claude (gMEM-dev)
**Date**: 2026-02-04

---

## Overview

Transform CMS from opt-in to enforced by tracking per-session usage, exposing session health, and warning when agents skip the resume step. This ensures agents consistently use the Conversation Memory System, preventing context loss across compaction boundaries.

## Requirements

### Functional Requirements

1. FR-1: Track whether each agent session has called `/v1/conversation/resume`
2. FR-2: Track observation counts per session since last resume
3. FR-3: Expose a session health endpoint with a computed health score (0.0 - 1.0)
4. FR-4: Add a warning header when agents call `/v1/memory/retrieve` or `/v1/conversation/recall` without prior resume
5. FR-5: Auto-expire stale session tracking data via TTL-based cleanup

### Non-Functional Requirements

1. NFR-1: Performance — session tracking uses in-memory `sync.Map`, O(1) lookups
2. NFR-2: Non-breaking — warning middleware never blocks requests, only adds headers
3. NFR-3: Concurrency — all session state operations are goroutine-safe
4. NFR-4: Memory — TTL cleanup prevents unbounded growth (default 2-hour TTL)

## API Contract

### Endpoints

#### Session Health

```
GET /v1/conversation/session/health?session_id=X
```

**Response (tracked session):**

```json
{
  "session_id": "string",
  "space_id": "string",
  "resumed": true,
  "observations_since_resume": 5,
  "health_score": 0.9,
  "tracked": true,
  "last_resume_at": "2026-02-04T12:00:00Z",
  "last_observe_at": "2026-02-04T12:05:00Z",
  "last_activity_at": "2026-02-04T12:05:00Z"
}
```

**Response (untracked session):**

```json
{
  "session_id": "string",
  "resumed": false,
  "observations_since_resume": 0,
  "health_score": 0.0,
  "tracked": false
}
```

#### Warning Header

When `/v1/memory/retrieve` or `/v1/conversation/recall` is called by a session that has not called `/resume`:

```
X-MDEMG-Warning: session-not-resumed
```

### Error Codes

| Code | Meaning |
|------|---------|
| 400  | Missing `session_id` query parameter |
| 405  | Method not allowed (non-GET for health endpoint) |
| 503  | Session tracker not available |

## Data Model

### Go Types

```go
// SessionState tracks per-session CMS usage.
type SessionState struct {
    SessionID               string    `json:"session_id"`
    SpaceID                 string    `json:"space_id,omitempty"`
    Resumed                 bool      `json:"resumed"`
    LastResumeAt            time.Time `json:"last_resume_at,omitempty"`
    ObservationsSinceResume int       `json:"observations_since_resume"`
    LastObserveAt           time.Time `json:"last_observe_at,omitempty"`
    LastActivityAt          time.Time `json:"last_activity_at"`
    CreatedAt               time.Time `json:"created_at"`
}

// SessionTracker uses sync.Map with TTL-based cleanup goroutine.
type SessionTracker struct {
    sessions sync.Map
    ttl      time.Duration
    stopCh   chan struct{}
}
```

### Health Score Formula

- **Resumed** (0.4): +0.4 if session has called `/resume`
- **Observations** (0.0 - 0.4): +0.1 for 1+, +0.2 for 3+, +0.3 for 5+, +0.4 for 10+
- **Recency** (0.2): +0.2 if last activity within 10 minutes
- **Max**: capped at 1.0

## Test Plan

### Unit Tests

- [x] TestSessionTracker_RecordResume: verify state created with Resumed=true
- [x] TestSessionTracker_RecordObserve: verify observation count increments
- [x] TestSessionTracker_IsResumed: verify false for unknown/observe-only, true after resume
- [x] TestSessionState_HealthScore: table-driven tests for 5 scenarios (empty, resumed-only, resumed+observations, fully healthy, not-resumed+observing)
- [x] TestSessionTracker_Cleanup: verify TTL expiration removes stale sessions
- [x] TestSessionTracker_GetState_Unknown: verify nil for unknown sessions

### Integration Tests

- [ ] End-to-end: POST /resume -> POST /observe -> GET /health -> verify score
- [ ] Warning header: POST /retrieve without resume -> verify X-MDEMG-Warning header
- [ ] Warning absent: POST /resume then POST /retrieve -> verify no warning header

## Acceptance Criteria

- [x] AC-1: SessionTracker tracks resume and observe events per session_id
- [x] AC-2: HealthScore returns 0.0-1.0 based on resume, observations, recency
- [x] AC-3: GET /v1/conversation/session/health returns session health data
- [x] AC-4: X-MDEMG-Warning header added for non-resumed sessions calling retrieve/recall
- [x] AC-5: All existing tests pass (`go test ./...`)
- [x] AC-6: `go vet ./...` reports no issues
- [x] AC-7: SHA256 hash added to `docs/specs/manifest.sha256`

## Dependencies

- Depends on: Phase 2 (self-ingest, MCP updates)
- Blocks: Phase 3B (quality scoring), Phase 3C (multi-agent)

## Files Changed

### New Files

- `internal/conversation/session_tracker.go` — SessionState, SessionTracker with sync.Map, TTL cleanup
- `internal/conversation/session_tracker_test.go` — 6 test functions covering all tracker behavior

### Modified Files

- `internal/api/server.go` — added sessionTracker field, initialization, shutdown, health route
- `internal/api/handlers_conversation.go` — added tracking calls in handleObserve/handleResume, added handleSessionHealth handler
- `internal/api/middleware.go` — added SessionResumeWarningMiddleware function
