# Linear Integration Module

The Linear module syncs teams, projects, and issues from Linear into MDEMG's memory graph.

## Quick Start

1. **Get a Linear API key:**
   - Go to Linear → Settings → Account → Security & Access
   - Create a new API key with "Read" scope (for sync only) or "Read & Write" scope (for CRUD operations)

2. **Add to .env:**

   ```
   LINEAR_API_KEY=lin_api_xxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```

3. **Sync data:**

   ```bash
   # Sync everything (teams, projects, issues)
   curl -X POST http://localhost:9999/v1/modules/linear-module/sync \
     -H "Content-Type: application/json" \
     -d '{"source_id": "linear://all", "ingest": true, "space_id": "linear"}'
   ```

## Sync Options

### Source Types

| Source ID | Description |
|-----------|-------------|
| `linear://all` | Sync teams, projects, and all issues |
| `linear://teams` | Sync only teams |
| `linear://projects` | Sync only projects |
| `linear://issues` | Sync all issues |
| `linear://issues?team=TEC` | Sync issues from specific team |

### Request Parameters

```json
{
  "source_id": "linear://all",    // What to sync
  "ingest": true,                  // Store in MDEMG (false = dry run)
  "space_id": "linear",           // Memory space to store in
  "cursor": ""                     // Resume from cursor (for incremental sync)
}
```

### Response

```json
{
  "data": {
    "count": 1948,
    "cursor": "6c460bae-7530-...",
    "ingested": 1948,
    "ingest_errors": 0,
    "stats": {
      "items_processed": 1806,
      "items_created": 1806,
      "items_updated": 0,
      "items_skipped": 0
    }
  }
}
```

## Data Model

### Teams → Observations

```
NodeId:      linear-team-{uuid}
Path:        linear://teams/{key}
ContentType: application/vnd.linear.team
Tags:        [linear, team, {key}]
Metadata:    linear_id, team_key, issue_count, private, created_at, updated_at
```

### Projects → Observations

```
NodeId:      linear-project-{uuid}
Path:        linear://projects/{uuid}
ContentType: application/vnd.linear.project
Tags:        [linear, project, {state}, {team_keys...}]
Metadata:    linear_id, state, progress, lead, created_at, updated_at
```

### Issues → Observations

```
NodeId:      linear-issue-{uuid}
Path:        linear://issues/{identifier}
ContentType: application/vnd.linear.issue
Tags:        [linear, issue, {team_key}, {state_type}, {labels...}]
Metadata:    linear_id, identifier, team_key, state, state_type, priority,
             project_id, project_name, assignee, created_at, updated_at
```

## CRUD Operations

The Linear module supports full Create/Read/Update/Delete operations via the REST API. These operations call Linear's GraphQL API through the CRUDModule gRPC service.

### Creating Issues

```bash
curl -X POST http://localhost:9999/v1/linear/issues \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Fix login bug",
    "team_id": "team-uuid",
    "description": "Users cannot log in on mobile",
    "priority": 2
  }'
```

Required fields: `title`, `team_id`. Optional: `description`, `priority` (1=urgent, 4=low), `assignee_id`, `state_id`, `project_id`, `label_ids` (comma-separated).

### Listing Issues

```bash
# List all issues (default limit: 50)
curl "http://localhost:9999/v1/linear/issues"

# Filter by team
curl "http://localhost:9999/v1/linear/issues?team=ENG&limit=10"

# Filter by state
curl "http://localhost:9999/v1/linear/issues?state=In%20Progress"

# Paginate
curl "http://localhost:9999/v1/linear/issues?cursor=abc123&limit=25"
```

### Reading a Single Issue

```bash
curl "http://localhost:9999/v1/linear/issues/issue-uuid"
```

### Updating Issues

```bash
curl -X PUT http://localhost:9999/v1/linear/issues/issue-uuid \
  -H "Content-Type: application/json" \
  -d '{"title": "Updated title", "priority": 1}'
```

Only provided fields are updated. Supports: `title`, `description`, `priority`, `state_id`, `assignee_id`, `project_id`, `label_ids`.

### Deleting (Archiving) Issues

```bash
curl -X DELETE "http://localhost:9999/v1/linear/issues/issue-uuid"
```

Issues are archived (soft-deleted) in Linear, not permanently removed.

### Projects

```bash
# Create
curl -X POST http://localhost:9999/v1/linear/projects \
  -H "Content-Type: application/json" \
  -d '{"name": "Q1 Sprint", "description": "First quarter work"}'

# List
curl "http://localhost:9999/v1/linear/projects"

# Read
curl "http://localhost:9999/v1/linear/projects/project-uuid"

# Update
curl -X PUT http://localhost:9999/v1/linear/projects/project-uuid \
  -H "Content-Type: application/json" \
  -d '{"name": "Q1 Sprint - Updated"}'
```

### Comments

```bash
# Add a comment to an issue
curl -X POST http://localhost:9999/v1/linear/comments \
  -H "Content-Type: application/json" \
  -d '{"issue_id": "issue-uuid", "body": "This needs attention."}'
```

## MCP Tools

The following MCP tools are available for IDE/agent integration:

| Tool | Description | Required Params |
|------|-------------|-----------------|
| `linear_create_issue` | Create a new issue | `title`, `team_id` |
| `linear_list_issues` | List issues with filters | — |
| `linear_read_issue` | Read a single issue | `issue_id` |
| `linear_update_issue` | Update an issue | `issue_id` |
| `linear_add_comment` | Add a comment | `issue_id`, `body` |
| `linear_search` | Full-text search issues | `query` |

## Workflow Engine

The workflow engine provides config-driven automation triggered by CRUD events.

### Configuration

Workflows are defined in `plugins/linear-module/workflows.yaml`:

```yaml
workflows:
  - name: "auto-triage-urgent"
    trigger:
      event: "on-create"        # on-create, on-update, on-delete
      entity_type: "issue"      # issue, project, comment
      conditions:
        - field: "priority"
          operator: "eq"        # eq, neq, contains, changed_to, exists
          value: "1"
    actions:
      - type: "add-comment"     # add-comment, auto-assign, auto-label, auto-transition, set-field
        params:
          body: "Urgent issue auto-triaged by workflow engine."
```

### Supported Conditions

| Operator | Description | Requires Previous State |
|----------|-------------|------------------------|
| `eq` | Field equals value | No |
| `neq` | Field does not equal value | No |
| `contains` | Field contains substring | No |
| `changed_to` | Field changed to value (from different value) | Yes |
| `exists` | Field is present and non-empty | No |

### Supported Actions

| Action | Description | Params |
|--------|-------------|--------|
| `add-comment` | Add a comment to the entity | `body` (supports `{{field}}` templates) |
| `auto-assign` | Assign to a user | `assignee_id` |
| `auto-label` | Add labels | `label_ids` (comma-separated) |
| `auto-transition` | Change state | `state_id` |
| `set-field` | Set any field value | `field`, `value` |

### Template Interpolation

Action params support `{{field_name}}` placeholders:

```yaml
- type: "add-comment"
  params:
    body: "Issue {{identifier}} ({{title}}) has been completed."
```

## Querying Linear Data

```bash
# Search for issues related to documentation
curl -X POST http://localhost:9999/v1/memory/retrieve \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "linear",
    "query_text": "documentation migration",
    "top_k": 10
  }'
```

## Module Health

```bash
# Check module status
curl http://localhost:9999/v1/modules | jq '.data.modules[] | select(.id == "linear-module")'
```

Response includes:

- `api_configured`: Whether LINEAR_API_KEY is set
- `last_sync`: Timestamp of last sync
- `requests_handled`: Number of API calls made
- `uptime`: Module uptime

## Environment Variables

| Variable | Description |
|----------|-------------|
| `LINEAR_API_KEY` | Linear API key (required) |
| `LINEAR_TEAM_ID` | Default team key for issue creation (optional) |
| `LINEAR_WORKSPACE_ID` | Workspace identifier (optional) |

## Incremental Sync

The module supports incremental sync using cursors:

```bash
# First sync - full
curl -X POST .../sync -d '{"source_id": "linear://issues", "ingest": true, "space_id": "linear"}'
# Response: {"cursor": "abc123..."}

# Later - incremental from cursor
curl -X POST .../sync -d '{"source_id": "linear://issues", "cursor": "abc123...", "ingest": true, "space_id": "linear"}'
```

## Rate Limits

The module respects Linear's API rate limits:

- 1,500 requests/hour for API key auth
- 100ms delay between paginated requests

## Error Handling

The Linear module returns appropriate HTTP status codes:

| Status | Condition |
|--------|-----------|
| 200 | Successful operation |
| 400 | Validation error (missing required fields like `team_id`) |
| 404 | Resource not found (issue/project/comment doesn't exist) |
| 405 | Method not allowed |
| 503 | Linear module unavailable — API key not configured, plugin not running, or gRPC service unimplemented |

**Key behavior**: When the Linear plugin is running but doesn't implement the `CRUDModule` gRPC service (e.g., during development), all CRUD endpoints return **503** with `{"error": "..."}`. The handler detects gRPC `Unimplemented` and `unknown service` errors and maps them to 503 (Service Unavailable) rather than 500 (Internal Server Error).

## Troubleshooting

### "LINEAR_API_KEY not configured"

Add `LINEAR_API_KEY=lin_api_xxx` to your `.env` file and restart the server. Without this, all Linear endpoints return 503.

### "GraphQL errors"

Check that your API key has "Read" scope and hasn't expired.

### All endpoints returning 503

This means the Linear module is either:
1. Not running — check `GET /v1/plugins` to see if the plugin is loaded
2. Running but not implementing the CRUDModule gRPC service — the plugin binary exists but doesn't expose CRUD operations
3. API key not configured — add `LINEAR_API_KEY` to `.env`

Plugins are discovered only at server startup — if you added the plugin binary after starting the server, restart it.

### Vector dimension mismatch

If you see errors about vector dimensions, ensure your vector index matches your embedding provider:

- OpenAI `text-embedding-3-large`: 3072 dimensions
- Ollama: varies by model

Recreate the index if needed:

```cypher
DROP INDEX memNodeEmbedding IF EXISTS;
CREATE VECTOR INDEX memNodeEmbedding FOR (n:MemoryNode) ON (n.embedding)
OPTIONS {indexConfig: {`vector.dimensions`: 3072, `vector.similarity_function`: 'COSINE'}};
```
