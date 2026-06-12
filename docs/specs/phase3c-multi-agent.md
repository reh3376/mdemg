# Feature Spec: Multi-Agent CMS Support

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: Phase 3C
**Status**: Implemented
**Author**: reh3376 & Claude (gMEM-dev)
**Date**: 2026-02-04

---

## Overview

Extend the Conversation Memory System to support persistent agent identities that survive across ephemeral sessions. Agents are long-lived principals (e.g., "agent-claude", "agent-cursor") while sessions are short-lived contexts that start and end with each compaction cycle. This enables cross-session learning, agent isolation, and team-level knowledge sharing.

## Requirements

### Functional Requirements

1. FR-1: Support `agent_id` field on all CMS operations (observe, correct, resume, recall)
2. FR-2: Store `agent_id` on MemoryNode in Neo4j with appropriate indexes
3. FR-3: Enable cross-session resume — when `agent_id` is set, retrieve observations across all sessions for that agent
4. FR-4: Agent isolation — private observations from agent A are not visible to agent B
5. FR-5: Team visibility — observations with `visibility=team` are shared across all agents in the same space
6. FR-6: Backward compatibility — requests without `agent_id` work exactly as before

### Non-Functional Requirements

1. NFR-1: Zero breaking changes — all existing API contracts preserved
2. NFR-2: Indexed queries — `agent_id` filtering uses Neo4j composite indexes
3. NFR-3: Agent + user coexistence — `agent_id` and `user_id` can both be set on the same observation

## API Contract

### Agent Identity on Observe

```
POST /v1/conversation/observe
```

**Request (new field):**

```json
{
  "space_id": "mdemg-dev",
  "session_id": "session-123",
  "content": "...",
  "agent_id": "agent-claude",
  "user_id": "user-123",
  "visibility": "team"
}
```

### Cross-Session Resume

```
POST /v1/conversation/resume
```

**Request (new field):**

```json
{
  "space_id": "mdemg-dev",
  "agent_id": "agent-claude",
  "max_observations": 20
}
```

When `agent_id` is set, `session_id` filter is skipped — observations from all sessions are returned.

### Agent-Filtered Recall

```
POST /v1/conversation/recall
```

**Request (new field):**

```json
{
  "space_id": "mdemg-dev",
  "query": "what decisions were made?",
  "agent_id": "agent-claude"
}
```

### Agent Identity on Correct

```
POST /v1/conversation/correct
```

**Request (new field):**

```json
{
  "agent_id": "agent-claude"
}
```

## Data Model

### Neo4j Schema Changes (V0011)

```cypher
// Index for filtering observations by agent_id
CREATE INDEX memorynode_agent_id_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.agent_id);

// Composite index for agent + visibility
CREATE INDEX memorynode_agent_visibility_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.agent_id, n.visibility);

// Composite index for agent + session
CREATE INDEX memorynode_agent_session_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.agent_id, n.session_id);
```

### Go Types (Modified)

```go
// Observation — added AgentID field
type Observation struct {
    // ... existing fields ...
    AgentID string // Persistent agent identity (survives across sessions)
}

// ObserveRequest — added AgentID field
// CorrectRequest — added AgentID field
// ResumeRequest — added AgentID field
// RecallRequest — added AgentID field
```

### Visibility Model

```
Private:  visible to owning agent only   (agent_id = requestor OR visibility != 'private')
Team:     visible within space           (visibility = 'team' OR 'global')
Global:   visible to all agents          (visibility = 'global')
```

When `agent_id` is set on the request:

- Private observations: filtered by `agent_id` match (not `user_id`)
- Team/global observations: visible to all agents in the space

When `agent_id` is not set:

- Falls back to `user_id`-based filtering (backward compatible)

## Test Plan

### Unit Tests

- [x] TestObservation_AgentID: field exists and stores correctly
- [x] TestObserveRequest_AgentID: request type has AgentID field
- [x] TestCorrectRequest_AgentID: correction request has AgentID field
- [x] TestResumeRequest_CrossSessionAgent: AgentID without SessionID
- [x] TestResumeRequest_SessionAndAgent: both fields set
- [x] TestRecallRequest_AgentFilter: recall with agent filtering
- [x] TestAgentVisibilityModel: 5-case table-driven visibility test
- [x] TestAgentIsolation_DifferentAgentsSameSpace: agents can't see each other's private obs
- [x] TestTeamVisibility_SharedAcrossAgents: team obs visible to all agents
- [x] TestCrossSessionResume_AgentIdentity: multi-session agent identity
- [x] TestBackwardCompatibility_NoAgentID: no AgentID = legacy behavior

### Integration Tests

- [ ] End-to-end: observe with agent_id -> resume with agent_id -> verify cross-session
- [ ] Agent isolation: two agents observe privately -> verify mutual invisibility
- [ ] Team sharing: observe with team visibility -> verify both agents can recall

## Acceptance Criteria

- [x] AC-1: `agent_id` field added to all CMS request/response types
- [x] AC-2: `agent_id` stored on MemoryNode in Neo4j
- [x] AC-3: Neo4j migration V0011 creates agent_id indexes
- [x] AC-4: Cross-session resume works when agent_id is set (session filter skipped)
- [x] AC-5: Private observations filtered by agent_id ownership
- [x] AC-6: Team observations shared across agents in same space
- [x] AC-7: Backward compatible — requests without agent_id work as before
- [x] AC-8: All existing tests pass (`go test ./...`)
- [x] AC-9: `go vet ./...` reports no issues
- [x] AC-10: SHA256 hash added to `docs/specs/manifest.sha256`

## Dependencies

- Depends on: Phase 3A (session tracking), Phase 3B (quality scoring)
- Blocks: Phase 4 (Linear integration), Phase 5 (Obsidian integration)

## Files Changed

### New Files

- `migrations/V0011__agent_identity.cypher` — Neo4j indexes for agent_id
- `internal/conversation/multi_agent_test.go` — 11 multi-agent test functions

### Modified Files

- `internal/conversation/types.go` — Added `AgentID` to Observation struct
- `internal/conversation/service.go` — Added `AgentID` to ObserveRequest, CorrectRequest, ResumeRequest, RecallRequest; agent filtering in fetchRecentObservations and findSimilarObservations; agent_id stored in Neo4j node
- `internal/models/models.go` — Added `AgentID` to API request types (ObserveRequest, CorrectRequest, ResumeRequest, RecallRequest)
- `internal/api/handlers_conversation.go` — Wire AgentID through all handler conversions
