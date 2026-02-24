# MDEMG Conversation Memory - Phase 60: CMS Advanced Functionality II

## Overview

Phase 60 implements advanced CMS features for structured observations, intelligent resume, and context window optimization. This allows MDEMG to provide agents with precise task continuity after context compaction without flooding the context window with irrelevant information.

## Implementation Status

✅ **COMPLETE** - Phase 60 (CMS Advanced Functionality II)

### What's Implemented

1. **Observation Templates** - Predefined schemas with JSON Schema validation:
   - Template CRUD operations (Create, Read, Update, Delete, List)
   - Schema validation for structured observation data
   - Default templates: `task_handoff`, `decision`, `error`, `learning`

2. **Task Context Snapshots** - Auto-capture task state:
   - Manual and automatic triggers (session end, compaction, error)
   - Structured context storage (task name, active files, goals, blockers)
   - Latest snapshot retrieval for quick resume
   - Cleanup of old snapshots

3. **Resume Relevance Scoring** - Score observations for prioritization:
   - Recency scoring (exponential decay with configurable half-life)
   - Importance scoring (based on obs_type weights)
   - Task-relevance scoring (embedding similarity to query context)
   - Configurable weights (default: recency=0.3, importance=0.4, task_relevance=0.3)

4. **Smart Truncation** - Token-budgeted context restoration:
   - Tiered resume (critical/important/background)
   - Token counting and budget enforcement
   - Automatic summarization for background tier

5. **Org-Level Review** - Workflow for promoting observations:
   - Flag observations for org-level review
   - Pending review listing with statistics
   - Approve/reject decision workflow
   - Visibility promotion (private → team → global)

## API Endpoints

### Observation Templates

```bash
# List all templates
curl "http://localhost:9999/v1/conversation/templates?space_id=mdemg-dev"

# Create template
curl -X POST http://localhost:9999/v1/conversation/templates \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "mdemg-dev",
    "template_id": "task_handoff",
    "name": "Task Handoff",
    "description": "Capture task state for session continuity",
    "obs_type": "context",
    "schema": {
      "type": "object",
      "required": ["task_name", "status"],
      "properties": {
        "task_name": {"type": "string"},
        "status": {"type": "string", "enum": ["in_progress", "blocked", "completed"]}
      }
    }
  }'

# Get template
curl "http://localhost:9999/v1/conversation/templates/task_handoff?space_id=mdemg-dev"

# Update template
curl -X PUT http://localhost:9999/v1/conversation/templates/task_handoff \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "mdemg-dev",
    "name": "Updated Task Handoff",
    "description": "Updated description"
  }'

# Delete template
curl -X DELETE "http://localhost:9999/v1/conversation/templates/task_handoff?space_id=mdemg-dev"
```

### Task Context Snapshots

```bash
# List snapshots
curl "http://localhost:9999/v1/conversation/snapshots?space_id=mdemg-dev&session_id=session-123"

# Create snapshot
curl -X POST http://localhost:9999/v1/conversation/snapshots \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "mdemg-dev",
    "session_id": "session-123",
    "trigger": "manual",
    "context": {
      "task_name": "Implement Phase 60",
      "active_files": ["service.go", "types.go"],
      "current_goal": "Add templates",
      "pending_items": ["Create tests", "Update docs"]
    }
  }'

# Get latest snapshot for session
curl "http://localhost:9999/v1/conversation/snapshots/latest?space_id=mdemg-dev&session_id=session-123"

# Get specific snapshot
curl "http://localhost:9999/v1/conversation/snapshots/snap-abc123?space_id=mdemg-dev"

# Delete snapshot
curl -X DELETE "http://localhost:9999/v1/conversation/snapshots/snap-abc123?space_id=mdemg-dev"

# Cleanup old snapshots
curl -X POST http://localhost:9999/v1/conversation/snapshots/cleanup \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "mdemg-dev",
    "keep_count": 10,
    "older_than_days": 7
  }'
```

### Org-Level Review

```bash
# List pending reviews
curl "http://localhost:9999/v1/conversation/org-reviews?space_id=mdemg-dev"

# Get review statistics
curl "http://localhost:9999/v1/conversation/org-reviews/stats?space_id=mdemg-dev"

# Flag observation for review
curl -X POST http://localhost:9999/v1/conversation/org-reviews/flag \
  -H "Content-Type: application/json" \
  -d '{
    "obs_id": "obs-abc123",
    "space_id": "mdemg-dev",
    "reason": "Valuable architectural decision for team reference",
    "suggested_visibility": "team",
    "flagged_by": "agent-claude"
  }'

# Approve/reject decision
curl -X POST http://localhost:9999/v1/conversation/org-reviews/decision \
  -H "Content-Type: application/json" \
  -d '{
    "obs_id": "obs-abc123",
    "space_id": "mdemg-dev",
    "decision": "approve",
    "visibility": "team",
    "reviewed_by": "user@example.com",
    "notes": "Good addition to team knowledge"
  }'
```

## Architecture

### File Structure

```
internal/conversation/
├── templates.go              # Template CRUD operations
├── templates_test.go         # Template unit tests
├── snapshot.go               # Task context snapshot service
├── snapshot_test.go          # Snapshot unit tests
├── relevance.go              # Relevance scoring algorithms
├── relevance_test.go         # Relevance scoring tests
├── truncation.go             # Smart truncation with token budget
├── truncation_test.go        # Truncation tests
├── org_review.go             # Org-level review workflow
├── org_review_test.go        # Org review tests
└── ... (existing files)

internal/api/
├── server.go                 # Route registration for Phase 60 endpoints
└── ... (existing files)

docs/api/api-spec/uats/specs/
├── cms_templates_list.uats.json      # Template list contract
├── cms_templates_create.uats.json    # Template create contract
├── cms_templates_get.uats.json       # Template get contract
├── cms_templates_update.uats.json    # Template update contract
├── cms_templates_delete.uats.json    # Template delete contract
├── cms_snapshot_list.uats.json       # Snapshot list contract
├── cms_snapshot_create.uats.json     # Snapshot create contract
├── cms_snapshot_get.uats.json        # Snapshot get contract
├── cms_snapshot_delete.uats.json     # Snapshot delete contract
├── cms_snapshot_latest.uats.json     # Latest snapshot contract
├── cms_snapshot_cleanup.uats.json    # Snapshot cleanup contract
├── cms_org_reviews_list.uats.json    # Pending reviews contract
├── cms_org_reviews_stats.uats.json   # Review stats contract
├── cms_org_flag.uats.json            # Flag for review contract
└── cms_org_decision.uats.json        # Approve/reject contract
```

## Relevance Scoring Algorithm

The relevance score combines three factors:

```go
score = (recencyWeight × recencyScore) +
        (importanceWeight × importanceScore) +
        (taskRelevanceWeight × taskRelevanceScore)
```

### Recency Scoring

Uses exponential decay with configurable half-life:

```go
recencyScore = exp(-log(2) × ageHours / halfLifeHours)
```

Default half-life: 24 hours

### Importance Scoring

Based on observation type weights:

| obs_type | Base Weight | With Decay |
|----------|-------------|------------|
| correction | 0.9 | Decay applied |
| error | 0.8 | Decay applied |
| decision | 0.7 | Decay applied |
| task | 0.6 | Decay applied |
| learning | 0.5 | Decay applied |
| preference | 0.4 | Decay applied |
| other | 0.3 | Decay applied |

### Task Relevance Scoring

Embedding similarity between observation and query context (if provided):

```go
taskRelevance = cosineSimilarity(obsEmbedding, queryEmbedding)
```

## Smart Truncation

### Token Budget Allocation

| Tier | Budget % | Criteria |
|------|----------|----------|
| **Critical** | 40% | Corrections, errors, recent decisions |
| **Important** | 35% | Task context, active learnings |
| **Background** | 25% | Older observations (summarized) |

### Tier Assignment

```go
func AssignTier(obs *Observation) Tier {
    // Critical tier
    if obs.ObsType == "correction" || obs.ObsType == "error" {
        return TierCritical
    }
    if obs.ObsType == "decision" && obs.Age < 24*time.Hour {
        return TierCritical
    }

    // Important tier
    if obs.ObsType == "task" || obs.RelevanceScore > 0.7 {
        return TierImportant
    }

    // Background tier
    return TierBackground
}
```

### Summarization

Background tier observations are summarized to save tokens:

```go
type TruncationResult struct {
    Critical    []Observation    // Full content
    Important   []Observation    // Full content
    Background  []SummarizedObs  // Summarized
    TotalTokens int
    TokenBudget int
    OmittedCount int
}
```

## Org-Level Review Workflow

### Review States

```
none → pending → approved/rejected
```

### State Transitions

1. **Flag for Review**: Sets status to `pending`, records flagger and reason
2. **Approve**: Sets status to `approved`, updates visibility
3. **Reject**: Sets status to `rejected`, keeps visibility as `private`

### Visibility Levels

| Level | Access |
|-------|--------|
| `private` | Owner only |
| `team` | Space members |
| `global` | Everyone |

## Default Templates

### Task Handoff

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
      "recent_decisions": {"type": "array", "items": {"type": "string"}}
    }
  }
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
  }
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

## Testing

### Unit Tests

```bash
go test -v ./internal/conversation/... -run "Template|Snapshot|Relevance|Truncation|OrgReview"
```

Tests cover:

- Template CRUD operations (create, read, update, delete, list)
- Schema validation (valid/invalid schemas)
- Snapshot capture and retrieval (manual, auto-trigger)
- Relevance scoring (recency decay, importance weights, task relevance)
- Truncation (tier assignment, token budget, summarization)
- Org review workflow (flag, approve, reject, stats)

### UATS Contract Tests

```bash
# Run all Phase 60 UATS specs
cd docs/api/api-spec/uats
source .venv/bin/activate
python runners/uats_runner.py --spec "specs/cms_*.uats.json" --base-url http://localhost:9999
```

All 15 UATS specs pass with 100% conformance.

## Configuration

Phase 60 uses existing CMS configuration. No new environment variables required for P0 features.

Future configuration (P1):

```bash
CMS_RESUME_MAX_TOKENS=4000
CMS_RESUME_DEFAULT_STRATEGY=task_focused
CMS_RELEVANCE_WEIGHT_RECENCY=0.3
CMS_RELEVANCE_WEIGHT_IMPORTANCE=0.4
CMS_RELEVANCE_WEIGHT_TASK_RELEVANCE=0.3
CMS_ORG_REVIEW_AUTO_APPROVE_DAYS=7
```

## Development Workflow

### UATS-First Development

**Important:** All Phase 60 APIs were built using UATS-first workflow:

1. Write UATS spec defining the API contract
2. Run UATS test (expect failure - endpoint not implemented)
3. Implement the endpoint
4. Run UATS test (expect success)
5. Commit with spec + implementation

This ensures API conformance and prevents drift between spec and implementation.

### UATS Spec Structure

```json
{
  "uats_version": "1.0.0",
  "api": {
    "name": "Endpoint Name",
    "base_url": "${MDEMG_BASE_URL}",
    "version": "v1",
    "service": "mdemg",
    "tags": ["conversation", "phase60"]
  },
  "metadata": {
    "author": "claude",
    "created": "2026-02-07",
    "description": "What the endpoint does",
    "test_type": "contract",
    "priority": "high"
  },
  "request": {
    "method": "GET|POST|PUT|DELETE",
    "path": "/v1/conversation/...",
    "query": {...},
    "body": {...},
    "headers": {"Content-Type": "application/json"}
  },
  "expected": {
    "status": [200, 201, 404],
    "body_assertions": [...]
  }
}
```

## Next Steps

### P1 Features (Phase 60.5)

- Observation Relationships: `supersedes`, `relates_to`, `contradicts`
- Observation Consolidation: Periodically merge related observations
- Importance Decay: Time-based decay with reinforcement on access
- Resume Strategy Selection: `task_focused`, `comprehensive`, `minimal`
- CMS Usage Metrics: Track observation rate, quality, resume frequency

### P2 Features (Phase 60.6)

- Session Continuity Score: Measure context preservation quality
- Mandatory Observation Points: Hooks for critical moments
- Delta Resume: Return only new/changed observations after first resume

## Performance

- **Template CRUD**: ~10-30ms (Neo4j read/write)
- **Snapshot capture**: ~20-50ms
- **Relevance scoring**: ~5-10ms per observation
- **Truncation**: ~10-20ms (depends on observation count)
- **Overall resume**: < 500ms target (NFR-1)

## References

- Phase 60 Spec: `docs/specs/phase60-cms-advanced-ii.md`
- Phase 1 (Observation Capture): `docs/architecture/conversation_memory_phase1.md`
- Phase 2 (Hebbian Learning): `docs/architecture/conversation_learning_phase2.md`
- API Reference: `docs/development/API_REFERENCE.md`
- UATS Specs: `docs/api/api-spec/uats/specs/cms_*.uats.json`

---

*Created: 2026-02-07*
