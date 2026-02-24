# Feature Spec: CMS Advanced Functionality II

**Phase**: Phase 60
**Status**: Complete
**Author**: reh3376 & Claude (gMEM-dev)
**Date Created**: 2026-02-06
**Date Completed**: 2026-02-07
**Priority**: Very High

---

## Overview

Enhance the Conversation Memory System (CMS) with structured observations, intelligent resume functionality, and context window optimization. The goal is to ensure agents have necessary information for task continuity on context reset without flooding the context window with irrelevant information.

**Core Objectives:**

1. **Structured Observations** — Templates for consistent, parseable observation capture
2. **Task Context Snapshots** — Auto-capture task state before compaction
3. **Smart Resume** — Relevance-scored, token-budgeted context restoration
4. **Org-Level Curation** — Promote high-value observations with user review

## Requirements

### Functional Requirements — High Priority

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-1 | Observation Templates: Predefined schemas stored in Neo4j sub-space | P0 |
| FR-2 | Task Context Snapshots: Auto-capture structured task state | P0 |
| FR-3 | Resume Relevance Scoring: Score by recency, importance, task-relevance | P0 |
| FR-4 | Smart Truncation: Summarize older obs, preserve detail for recent | P0 |
| FR-5 | Org-Level Flagging: Alert user for review before org-level ingestion | P0 |

### Functional Requirements — Medium Priority

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-6 | Observation Relationships: `supersedes`, `relates_to`, `contradicts` | P1 |
| FR-7 | Observation Consolidation: Periodically merge related observations | P1 |
| FR-8 | Importance Decay: Time-based decay with reinforcement on access | P1 |
| FR-9 | Resume Strategy Selection: `task_focused`, `comprehensive`, `minimal` | P1 |
| FR-10 | CMS Usage Metrics: Track observation rate, quality, resume frequency | P1 |

### Functional Requirements — Lower Priority

| ID | Requirement | Priority |
|----|-------------|----------|
| FR-11 | Session Continuity Score: Measure context preservation quality | P2 |
| FR-12 | Mandatory Observation Points: Hooks for critical moments | P2 |
| FR-13 | Delta Resume: Return only new/changed observations after first resume | P2 |

### Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| NFR-1 | Resume latency: < 500ms for standard resume |
| NFR-2 | Token budget: Configurable max tokens for resume (default: 4000) |
| NFR-3 | Template storage: Neo4j sub-space `{space_id}:templates` |
| NFR-4 | Backward compatibility: Existing observations remain valid |

## API Contract

### Observation Templates

#### Template CRUD

```
POST   /v1/conversation/templates           → Create template
GET    /v1/conversation/templates           → List templates
GET    /v1/conversation/templates/{id}      → Get template
PUT    /v1/conversation/templates/{id}      → Update template
DELETE /v1/conversation/templates/{id}      → Delete template
```

#### Create Template Request

```json
{
  "space_id": "mdemg-dev",
  "template_id": "task_handoff",
  "name": "Task Handoff",
  "description": "Capture task state for session continuity",
  "obs_type": "context",
  "schema": {
    "type": "object",
    "required": ["task_name", "status"],
    "properties": {
      "task_name": {"type": "string", "description": "Current task name"},
      "status": {"type": "string", "enum": ["in_progress", "blocked", "completed"]},
      "active_files": {"type": "array", "items": {"type": "string"}},
      "current_goal": {"type": "string"},
      "blockers": {"type": "array", "items": {"type": "string"}},
      "next_steps": {"type": "array", "items": {"type": "string"}},
      "recent_decisions": {"type": "array", "items": {"type": "string"}}
    }
  },
  "auto_capture": {
    "on_session_end": true,
    "on_compaction": true,
    "on_error": false
  }
}
```

#### Observe with Template

```json
{
  "space_id": "mdemg-dev",
  "session_id": "session-123",
  "template_id": "task_handoff",
  "data": {
    "task_name": "Implement Phase 60",
    "status": "in_progress",
    "active_files": ["internal/conversation/service.go"],
    "current_goal": "Add observation templates",
    "blockers": [],
    "next_steps": ["Create template schema", "Add API endpoints"]
  }
}
```

### Enhanced Resume

#### Resume Request (Extended)

```json
{
  "space_id": "mdemg-dev",
  "session_id": "session-123",
  "strategy": "task_focused",
  "options": {
    "max_tokens": 4000,
    "include_templates": ["task_handoff", "decision"],
    "relevance_weights": {
      "recency": 0.3,
      "importance": 0.4,
      "task_relevance": 0.3
    },
    "query_context": "Continuing Phase 60 implementation",
    "tiered": true
  }
}
```

#### Resume Response (Extended)

```json
{
  "session_id": "session-123",
  "observations": [
    {
      "obs_id": "...",
      "tier": "critical",
      "obs_type": "context",
      "template_id": "task_handoff",
      "content": "...",
      "structured_data": { "task_name": "...", "status": "..." },
      "relevance_score": 0.95,
      "created_at": "...",
      "truncated": false
    }
  ],
  "summary_observations": [
    {
      "obs_id": "summary-1",
      "tier": "background",
      "content": "3 decisions made regarding...",
      "summarizes": ["obs-1", "obs-2", "obs-3"],
      "truncated": true
    }
  ],
  "token_count": 3847,
  "token_budget": 4000,
  "omitted_count": 5,
  "continuity_context": {
    "last_session_id": "session-122",
    "last_activity": "2026-02-06T22:00:00Z",
    "active_task": "Phase 60 implementation"
  }
}
```

### Org-Level Observations

#### Flag for Org Review

```
POST /v1/conversation/observations/{obs_id}/flag-org
```

```json
{
  "reason": "Valuable architectural decision for team reference",
  "suggested_visibility": "team"
}
```

#### Response

```json
{
  "obs_id": "...",
  "flagged_for_review": true,
  "review_status": "pending",
  "flagged_at": "...",
  "flagged_by": "agent-claude"
}
```

#### List Pending Org Reviews

```
GET /v1/conversation/org-reviews?space_id=mdemg-dev&status=pending
```

#### Approve/Reject Org Review

```
POST /v1/conversation/org-reviews/{obs_id}/decision
```

```json
{
  "decision": "approve|reject",
  "visibility": "team|global",
  "notes": "Optional reviewer notes"
}
```

### Task Context Snapshots

#### Trigger Snapshot

```
POST /v1/conversation/snapshot
```

```json
{
  "space_id": "mdemg-dev",
  "session_id": "session-123",
  "trigger": "manual|compaction|session_end|error",
  "context": {
    "task_name": "Phase 60 implementation",
    "active_files": ["service.go", "types.go"],
    "current_goal": "Add templates",
    "recent_tool_calls": ["Read", "Edit", "Bash"],
    "pending_items": ["Create tests", "Update docs"]
  }
}
```

### Observation Relationships

#### Link Observations

```
POST /v1/conversation/observations/{obs_id}/link
```

```json
{
  "target_obs_id": "other-obs-id",
  "relationship": "supersedes|relates_to|contradicts|follows_from"
}
```

## Data Model

### Neo4j Schema Additions

```cypher
// Template storage (in sub-space)
CREATE (t:ObservationTemplate {
  template_id: "string",
  space_id: "string",
  name: "string",
  description: "string",
  obs_type: "string",
  schema: "json string",
  auto_capture: "json string",
  created_at: datetime(),
  updated_at: datetime()
})

// Index for templates
CREATE INDEX template_space_idx FOR (t:ObservationTemplate) ON (t.space_id, t.template_id)

// Extended Observation properties
// Add to existing Observation nodes:
//   template_id: "string" — reference to template used
//   structured_data: "json string" — template-validated data
//   importance_score: float — calculated importance (0-1)
//   last_accessed_at: datetime — for decay/reinforcement
//   tier: "critical|important|background" — resume tier
//   org_review_status: "none|pending|approved|rejected"
//   org_flagged_at: datetime
//   org_flagged_by: "string"

// Observation relationships
CREATE (a:Observation)-[:SUPERSEDES]->(b:Observation)
CREATE (a:Observation)-[:RELATES_TO]->(b:Observation)
CREATE (a:Observation)-[:CONTRADICTS]->(b:Observation)
CREATE (a:Observation)-[:FOLLOWS_FROM]->(b:Observation)

// Summary observations
CREATE (s:Observation {is_summary: true})-[:SUMMARIZES]->(o:Observation)

// Task context snapshots
CREATE (snap:TaskSnapshot {
  snapshot_id: "uuid",
  space_id: "string",
  session_id: "string",
  trigger: "string",
  context: "json string",
  created_at: datetime()
})
```

### Go Types

```go
// ObservationTemplate defines a structured observation schema
type ObservationTemplate struct {
    TemplateID   string                 `json:"template_id"`
    SpaceID      string                 `json:"space_id"`
    Name         string                 `json:"name"`
    Description  string                 `json:"description,omitempty"`
    ObsType      ObservationType        `json:"obs_type"`
    Schema       map[string]interface{} `json:"schema"` // JSON Schema
    AutoCapture  *AutoCaptureConfig     `json:"auto_capture,omitempty"`
    CreatedAt    time.Time              `json:"created_at"`
    UpdatedAt    time.Time              `json:"updated_at"`
}

type AutoCaptureConfig struct {
    OnSessionEnd bool `json:"on_session_end"`
    OnCompaction bool `json:"on_compaction"`
    OnError      bool `json:"on_error"`
}

// ResumeStrategy defines how resume should select observations
type ResumeStrategy string

const (
    ResumeTaskFocused   ResumeStrategy = "task_focused"
    ResumeComprehensive ResumeStrategy = "comprehensive"
    ResumeMinimal       ResumeStrategy = "minimal"
)

// EnhancedResumeRequest extends the resume request
type EnhancedResumeRequest struct {
    SpaceID   string         `json:"space_id"`
    SessionID string         `json:"session_id"`
    Strategy  ResumeStrategy `json:"strategy,omitempty"`
    Options   *ResumeOptions `json:"options,omitempty"`
}

type ResumeOptions struct {
    MaxTokens        int                `json:"max_tokens,omitempty"`
    IncludeTemplates []string           `json:"include_templates,omitempty"`
    RelevanceWeights *RelevanceWeights  `json:"relevance_weights,omitempty"`
    QueryContext     string             `json:"query_context,omitempty"`
    Tiered           bool               `json:"tiered,omitempty"`
}

type RelevanceWeights struct {
    Recency       float64 `json:"recency"`
    Importance    float64 `json:"importance"`
    TaskRelevance float64 `json:"task_relevance"`
}

// EnhancedResumeResponse extends the resume response
type EnhancedResumeResponse struct {
    SessionID           string                `json:"session_id"`
    Observations        []EnhancedObservation `json:"observations"`
    SummaryObservations []EnhancedObservation `json:"summary_observations,omitempty"`
    TokenCount          int                   `json:"token_count"`
    TokenBudget         int                   `json:"token_budget"`
    OmittedCount        int                   `json:"omitted_count"`
    ContinuityContext   *ContinuityContext    `json:"continuity_context,omitempty"`
}

type EnhancedObservation struct {
    ObsID          string                 `json:"obs_id"`
    Tier           string                 `json:"tier"` // critical, important, background
    ObsType        string                 `json:"obs_type"`
    TemplateID     string                 `json:"template_id,omitempty"`
    Content        string                 `json:"content"`
    StructuredData map[string]interface{} `json:"structured_data,omitempty"`
    RelevanceScore float64                `json:"relevance_score"`
    CreatedAt      time.Time              `json:"created_at"`
    Truncated      bool                   `json:"truncated"`
    Summarizes     []string               `json:"summarizes,omitempty"`
}

type ContinuityContext struct {
    LastSessionID string    `json:"last_session_id"`
    LastActivity  time.Time `json:"last_activity"`
    ActiveTask    string    `json:"active_task,omitempty"`
}

// TaskSnapshot captures task state for continuity
type TaskSnapshot struct {
    SnapshotID string                 `json:"snapshot_id"`
    SpaceID    string                 `json:"space_id"`
    SessionID  string                 `json:"session_id"`
    Trigger    string                 `json:"trigger"`
    Context    map[string]interface{} `json:"context"`
    CreatedAt  time.Time              `json:"created_at"`
}

// OrgReviewStatus tracks org-level review state
type OrgReviewStatus struct {
    ObsID        string    `json:"obs_id"`
    Status       string    `json:"status"` // pending, approved, rejected
    FlaggedAt    time.Time `json:"flagged_at"`
    FlaggedBy    string    `json:"flagged_by"`
    ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
    ReviewedBy   string    `json:"reviewed_by,omitempty"`
    Decision     string    `json:"decision,omitempty"`
    NewVisibility string   `json:"new_visibility,omitempty"`
}
```

## Implementation Plan

### Phase 60.1: Observation Templates (P0)

**Files:**

- `internal/conversation/templates.go` — Template CRUD operations
- `internal/conversation/templates_test.go` — Tests
- `internal/api/handlers_templates.go` — REST handlers
- `migrations/V0012__observation_templates.cypher` — Schema

**Tasks:**

1. Create template storage schema in Neo4j
2. Implement template CRUD service methods
3. Add template validation against JSON Schema
4. Extend observe endpoint to accept `template_id` + `data`
5. Store templates in `{space_id}:templates` sub-space

### Phase 60.2: Task Context Snapshots (P0)

**Files:**

- `internal/conversation/snapshot.go` — Snapshot capture
- `internal/conversation/snapshot_test.go` — Tests
- `internal/api/handlers_snapshot.go` — REST handlers

**Tasks:**

1. Define TaskSnapshot schema and storage
2. Implement manual snapshot trigger
3. Add auto-snapshot on session end (configurable)
4. Integrate with pre-compaction hook
5. Include snapshot in resume response

### Phase 60.3: Resume Relevance Scoring (P0)

**Files:**

- `internal/conversation/relevance.go` — Scoring algorithms
- `internal/conversation/relevance_test.go` — Tests

**Tasks:**

1. Implement recency scoring (exponential decay)
2. Implement importance scoring (based on obs_type, reinforcement)
3. Implement task-relevance scoring (embedding similarity to query_context)
4. Combine scores with configurable weights
5. Add relevance_score to observation output

### Phase 60.4: Smart Truncation (P0)

**Files:**

- `internal/conversation/truncation.go` — Truncation logic
- `internal/conversation/truncation_test.go` — Tests

**Tasks:**

1. Implement token counting for observations
2. Implement tiered resume (critical/important/background)
3. Implement observation summarization for background tier
4. Implement token budget enforcement
5. Track truncation in response metadata

### Phase 60.5: Org-Level Flagging (P0)

**Files:**

- `internal/conversation/org_review.go` — Review workflow
- `internal/conversation/org_review_test.go` — Tests
- `internal/api/handlers_org_review.go` — REST handlers

**Tasks:**

1. Add org_review fields to Observation
2. Implement flag-for-review endpoint
3. Implement list pending reviews endpoint
4. Implement approve/reject decision endpoint
5. Update visibility on approval

### Modified Files

| File | Changes |
|------|---------|
| `internal/conversation/service.go` | Integrate templates, snapshots, enhanced resume |
| `internal/conversation/types.go` | Add new types |
| `internal/api/server.go` | Register new routes |
| `internal/config/config.go` | Add CMS config vars |
| `.env.example` | Document new vars |

### Configuration

```bash
# CMS Advanced II Configuration
CMS_TEMPLATES_ENABLED=true
CMS_TEMPLATE_SUBSPACE_SUFFIX=:templates
CMS_SNAPSHOT_ON_SESSION_END=true
CMS_SNAPSHOT_ON_COMPACTION=true
CMS_RESUME_MAX_TOKENS=4000
CMS_RESUME_DEFAULT_STRATEGY=task_focused
CMS_RELEVANCE_WEIGHT_RECENCY=0.3
CMS_RELEVANCE_WEIGHT_IMPORTANCE=0.4
CMS_RELEVANCE_WEIGHT_TASK_RELEVANCE=0.3
CMS_IMPORTANCE_DECAY_RATE=0.1
CMS_ORG_REVIEW_REQUIRED=true
```

## Default Templates

### Task Handoff Template

```json
{
  "template_id": "task_handoff",
  "name": "Task Handoff",
  "obs_type": "context",
  "schema": {
    "type": "object",
    "required": ["task_name", "status"],
    "properties": {
      "task_name": {"type": "string"},
      "status": {"type": "string", "enum": ["in_progress", "blocked", "completed", "paused"]},
      "active_files": {"type": "array", "items": {"type": "string"}},
      "current_goal": {"type": "string"},
      "blockers": {"type": "array", "items": {"type": "string"}},
      "next_steps": {"type": "array", "items": {"type": "string"}},
      "recent_decisions": {"type": "array", "items": {"type": "string"}},
      "context_notes": {"type": "string"}
    }
  },
  "auto_capture": {"on_session_end": true, "on_compaction": true}
}
```

### Decision Template

```json
{
  "template_id": "decision",
  "name": "Decision Record",
  "obs_type": "decision",
  "schema": {
    "type": "object",
    "required": ["decision", "rationale"],
    "properties": {
      "decision": {"type": "string"},
      "rationale": {"type": "string"},
      "alternatives_considered": {"type": "array", "items": {"type": "string"}},
      "impact": {"type": "string", "enum": ["low", "medium", "high"]},
      "reversible": {"type": "boolean"}
    }
  }
}
```

### Error Template

```json
{
  "template_id": "error",
  "name": "Error Record",
  "obs_type": "error",
  "schema": {
    "type": "object",
    "required": ["error_type", "description"],
    "properties": {
      "error_type": {"type": "string"},
      "description": {"type": "string"},
      "file_path": {"type": "string"},
      "resolution": {"type": "string"},
      "root_cause": {"type": "string"},
      "prevention": {"type": "string"}
    }
  },
  "auto_capture": {"on_error": true}
}
```

### Learning Template

```json
{
  "template_id": "learning",
  "name": "Learning Record",
  "obs_type": "learning",
  "schema": {
    "type": "object",
    "required": ["topic", "insight"],
    "properties": {
      "topic": {"type": "string"},
      "insight": {"type": "string"},
      "source": {"type": "string"},
      "confidence": {"type": "string", "enum": ["low", "medium", "high"]},
      "applicable_to": {"type": "array", "items": {"type": "string"}}
    }
  }
}
```

## Test Plan

### Unit Tests

- [x] Template CRUD operations — `internal/conversation/templates_test.go`
- [x] Template validation against schema — `internal/conversation/templates_test.go`
- [x] Relevance scoring algorithms — `internal/conversation/relevance_test.go`
- [x] Token counting accuracy — `internal/conversation/truncation_test.go`
- [x] Truncation and summarization — `internal/conversation/truncation_test.go`
- [x] Org review workflow — `internal/conversation/org_review_test.go`

### Integration Tests

- [x] Full template lifecycle: create → use → validate → store
- [x] Enhanced resume with tiering and budget
- [x] Snapshot capture on session end
- [x] Org review approve/reject flow

### UATS Specs (15 total, 100% pass rate)

- [x] `cms_templates_list.uats.json`
- [x] `cms_templates_create.uats.json`
- [x] `cms_templates_get.uats.json`
- [x] `cms_templates_update.uats.json`
- [x] `cms_templates_delete.uats.json`
- [x] `cms_snapshot_list.uats.json`
- [x] `cms_snapshot_create.uats.json`
- [x] `cms_snapshot_get.uats.json`
- [x] `cms_snapshot_delete.uats.json`
- [x] `cms_snapshot_latest.uats.json`
- [x] `cms_snapshot_cleanup.uats.json`
- [x] `cms_org_reviews_list.uats.json`
- [x] `cms_org_reviews_stats.uats.json`
- [x] `cms_org_flag.uats.json`
- [x] `cms_org_decision.uats.json`

## Acceptance Criteria

- [x] AC-1: Templates stored in Neo4j sub-space
- [x] AC-2: Observe with template validates against schema
- [x] AC-3: Resume returns relevance-scored observations
- [x] AC-4: Resume respects token budget
- [x] AC-5: Older observations are summarized, not omitted
- [x] AC-6: Task snapshots captured on session end
- [x] AC-7: Org-level observations require user review
- [x] AC-8: All P0 features have unit tests
- [x] AC-9: `go build ./...` and `go test ./...` pass
- [x] AC-10: All 15 UATS specs pass (100% conformance)

## Dependencies

- **Depends on**: Phase 43A-C (CMS core), Phase 49 (LLM summaries)
- **Blocks**: None

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Token counting accuracy | Use tiktoken or similar; test against OpenAI tokenizer |
| Summarization quality | Use LLM with fallback to truncation |
| Resume latency | Cache relevance scores; precompute tiers |
| Org review backlog | Configurable auto-approve after N days |

---

*Created: 2026-02-06*
