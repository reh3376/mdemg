# Feature Spec: Linear Integration — Full CRUD + Workflows

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: Phase 4
**Status**: Approved
**Author**: reh3376 & Claude (gMEM-dev)
**Date**: 2026-02-04

---

## Overview

Add full Create/Read/Update/Delete operations and a config-driven workflow engine to the existing Linear plugin. The CRUDModule protobuf service is designed generically for reuse by future integrations (Obsidian in Phase 5).

**Data flow:**

```
MCP Tool / REST Client
  → REST API (handlers_linear.go)
    → Plugin Manager → gRPC (Unix socket)
      → CRUDModule (linear-module)
        → Linear GraphQL API
```

## Requirements

### Functional Requirements

1. FR-1: Generic CRUDModule protobuf service with entity_type dispatch and map<string,string> fields
2. FR-2: Linear plugin implements Create/Read/Update/Delete/List for issues, projects, and comments
3. FR-3: REST API exposes full CRUD routes under /v1/linear/
4. FR-4: MCP tools for create, read, update, list, comment, and search operations
5. FR-5: Config-driven workflow engine with triggers, conditions, and actions
6. FR-6: Plugin manager supports additional_services for backward-compatible multi-service modules

### Non-Functional Requirements

1. NFR-1: Performance — 100ms delay between workflow actions to respect Linear API rate limits
2. NFR-2: Backward Compatibility — Linear stays type INGESTION, adds CRUD via additional_services
3. NFR-3: Extensibility — CRUDModule design is generic for reuse by future plugins

## API Contract

### REST Endpoints

```
POST   /v1/linear/issues          → Create issue
GET    /v1/linear/issues          → List issues (query: team, state, assignee, limit, cursor)
GET    /v1/linear/issues/{id}     → Read issue
PUT    /v1/linear/issues/{id}     → Update issue
DELETE /v1/linear/issues/{id}     → Delete issue
POST   /v1/linear/projects        → Create project
GET    /v1/linear/projects        → List projects
GET    /v1/linear/projects/{id}   → Read project
PUT    /v1/linear/projects/{id}   → Update project
POST   /v1/linear/comments        → Create comment
```

### Create Issue Request

```json
{
  "title": "string — required",
  "team_id": "string — required",
  "description": "string — optional",
  "priority": "int — optional (1=urgent, 4=low)",
  "assignee_id": "string — optional",
  "state_id": "string — optional",
  "project_id": "string — optional",
  "label_ids": ["string — optional"]
}
```

### CRUD Response

```json
{
  "entity": {
    "id": "string",
    "entity_type": "string",
    "fields": { "key": "value" }
  }
}
```

### Error Codes

| Code | Meaning |
|------|---------|
| 400  | Invalid request (missing required fields, unknown entity type) |
| 404  | Entity not found |
| 405  | Method not allowed |
| 503  | CRUD module not available |

## Data Model

### Protobuf Changes

```protobuf
enum ModuleType {
  MODULE_TYPE_CRUD = 4;
}

service CRUDModule {
  rpc Create(CRUDCreateRequest) returns (CRUDCreateResponse);
  rpc Read(CRUDReadRequest) returns (CRUDReadResponse);
  rpc Update(CRUDUpdateRequest) returns (CRUDUpdateResponse);
  rpc Delete(CRUDDeleteRequest) returns (CRUDDeleteResponse);
  rpc List(CRUDListRequest) returns (CRUDListResponse);
}
```

### Go Types

```go
// Manifest additions
type Manifest struct {
    AdditionalServices []string `json:"additional_services,omitempty"`
}

type Capabilities struct {
    CRUDEntityTypes []string `json:"crud_entity_types,omitempty"`
}

// ModuleInfo addition
type ModuleInfo struct {
    CRUDClient pb.CRUDModuleClient
}
```

## Test Plan

### Unit Tests

- [ ] Mutation builder tests — verify GraphQL strings, field mapping, optional field omission
- [ ] CRUD dispatch tests — entity_type routing, error responses for unknown types
- [ ] Workflow condition tests — eq/neq/contains/changed_to matching logic
- [ ] Workflow action tests — action type dispatch, template interpolation

### Integration Tests

- [ ] REST handler tests — validation, method checks, service unavailable responses
- [ ] End-to-end: create issue → read → update → add comment → list → delete

## Acceptance Criteria

- [ ] AC-1: `go build ./...` compiles clean
- [ ] AC-2: `go test ./...` all tests pass
- [ ] AC-3: `go vet ./...` no issues
- [ ] AC-4: Proto regen matches source
- [ ] AC-5: Plugin builds: `cd plugins/linear-module && go build .`
- [ ] AC-6: SHA256 hash added to `docs/specs/manifest.sha256`

## Dependencies

- Depends on: Phase 3 (CMS), existing Linear ingestion module
- Blocks: Phase 5 (Obsidian plugin reuses CRUDModule)

## Files Changed

### New Files

- `plugins/linear-module/mutations.go` — GraphQL mutation builders
- `plugins/linear-module/workflow.go` — Workflow engine implementation
- `plugins/linear-module/workflows.yaml` — Default workflow configuration
- `internal/api/handlers_linear.go` — REST CRUD handlers
- `plugins/linear-module/mutations_test.go` — Mutation builder tests
- `plugins/linear-module/workflow_test.go` — Workflow engine tests
- `internal/api/handlers_linear_test.go` — REST handler tests

### Modified Files

- `api/proto/mdemg-module.proto` — Add CRUDModule service
- `api/modulepb/*.pb.go` — Regenerated protobuf code
- `internal/plugins/types.go` — CRUDClient, AdditionalServices
- `internal/plugins/manager.go` — Wire CRUD clients, GetCRUDModules()
- `plugins/linear-module/main.go` — Register CRUDModuleServer, implement dispatch
- `plugins/linear-module/manifest.json` — Add CRUD capabilities
- `internal/api/server.go` — Register Linear routes
- `cmd/mcp-server/main.go` — Register Linear MCP tools
- `internal/config/config.go` — LinearTeamID, LinearWorkspaceID
