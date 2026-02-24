<!-- markdownlint-disable MD013 MD022 MD032 MD040 -->
# Feature Spec: Active MCP Guardrails

**Phase**: 104
**Status**: Draft
**Author**: Agent
**Date**: 2026-02-23

---

## Overview

Active MCP Guardrails transition MDEMG from a passive observer (waiting for `/suggest` calls) to an active participant in the agent's workflow. This phase implements a new pre-commit or pre-action validation hook within the MCP (Model Context Protocol) Server.

When the agent proposes a code change, the MCP server will proactively cross-reference the proposed diff against active `ConstraintNodes` (identified in Phase 45.5) in the memory graph. If a violation is detected (e.g., using a deprecated API or violating an architectural "must"), the MCP server returns a warning or error, enforcing organizational memory in real-time.

This addresses Gap 4 in the Cognitive Intelligence Gap Analysis.

## Requirements

### Functional Requirements
1. **FR-1**: Extend the MCP Server with a new tool: `validate_changes` (or intercept existing write operations if supported by the MCP client protocol).
2. **FR-2**: Provide an internal API endpoint `POST /v1/memory/guardrail/validate` that accepts a proposed diff or code snippet.
3. **FR-3**: The validation endpoint must query the graph for all nodes with `role_type: 'constraint'` relevant to the files or symbols being modified.
4. **FR-4**: Use the LLM Semantic Summary Service to evaluate if the proposed changes violate the retrieved constraints.
5. **FR-5**: Return a structured response indicating `Pass`, `Warning`, or `Block`, along with the specific constraint node IDs and rationale for the decision.
6. **FR-6**: The MCP server surfaces this response to the agent, potentially blocking the action depending on severity.

### Non-Functional Requirements
1. **NFR-1**: Latency — Pre-commit validation must be fast. The system should prioritize vector retrieval of constraints and parallelize the LLM evaluation. Target < 3 seconds.
2. **NFR-2**: False Positives — The prompt used for validation must lean towards `Pass` unless a constraint is explicitly and unambiguously violated, to avoid frustrating the agent.

## API Contract

### Endpoints

```
POST /v1/memory/guardrail/validate
```

**Request:**

```json
{
  "space_id": "mdemg-dev",
  "files_changed": ["src/api/auth.go"],
  "diff": "@@ -15,7 +15,7 @@\n func Login(c *gin.Context) {\n-    token := jwt.New()\n+    token := legacy_auth.GenerateToken()\n }"
}
```

**Response:**

```json
{
  "data": {
    "status": "Block",
    "violations": [
      {
        "constraint_node_id": "uuid-1234",
        "description": "Must not use legacy_auth package for new endpoints.",
        "rationale": "The proposed diff introduces 'legacy_auth.GenerateToken()' which explicitly violates the deprecation constraint."
      }
    ],
    "warnings": []
  }
}
```

## Data Model

### Go Types

```go
// In internal/models/models.go

type GuardrailValidateRequest struct {
    SpaceID      string   `json:"space_id"`
    FilesChanged []string `json:"files_changed"`
    Diff         string   `json:"diff"`
}

type GuardrailViolation struct {
    ConstraintNodeID string `json:"constraint_node_id"`
    Description      string `json:"description"`
    Rationale        string `json:"rationale"`
}

type GuardrailValidateResponse struct {
    Status     string               `json:"status"` // "Pass", "Warning", "Block"
    Violations []GuardrailViolation `json:"violations,omitempty"`
    Warnings   []GuardrailViolation `json:"warnings,omitempty"`
}
```

## Internal Implementation Plan

1. **New Handler**: Create `internal/api/handlers_guardrail.go` to expose the new endpoint.
2. **Validation Service**: Create `internal/guardrail/service.go`.
    * Step 1: Extract symbols/keywords from the provided `Diff` and `FilesChanged`.
    * Step 2: Vector query the graph specifically filtering for `(:MemoryNode {role_type: 'constraint'})`.
    * Step 3: If constraints are found, construct a prompt containing the constraints and the diff. Ask the LLM to act as a strict compliance checker.
    * Step 4: Parse the LLM response into the `GuardrailValidateResponse` struct.
3. **MCP Server Integration**: Modify `cmd/mcp-server/main.go` (or wherever MCP tools are registered) to expose a `validate_proposed_changes` tool that calls the new HTTP endpoint.

## Test Plan (UxTS Framework)

### UATS (Universal API Test Specification)
* [ ] `docs/api/api-spec/uats/drafts/guardrail_validate.phase104.uats.json`: Verify the API contract handles diffs and returns structured violation arrays.

### USTS (Universal Security Test Specification)
<!-- (Applying USTS as this involves enforcing policy/blocking actions) -->
* [ ] `docs/tests/usts/specs/guardrail_enforcement.phase104.usts.json`: Ensure the endpoint accurately identifies a known bad pattern (mocked LLM) and returns a `Block` status, preventing unauthorized architectural drift.

### Unit Tests
* [ ] `internal/guardrail/service_test.go`: Test symbol extraction from diffs.
* [ ] `internal/guardrail/service_test.go`: Mock LLM returning a violation and assert response parsing.

## Acceptance Criteria

* [ ] AC-1: The `/v1/memory/guardrail/validate` endpoint is implemented and accessible.
* [ ] AC-2: The endpoint successfully filters vector searches to `ConstraintNodes`.
* [ ] AC-3: The MCP server registers a tool for validation that calls this endpoint.
* [ ] AC-4: UATS and USTS draft specs are written and hashes generated.
* [ ] AC-5: SHA256 hash added to `docs/specs/manifest.sha256`.

## Dependencies

* Depends on: Phase 45.5 (Constraint Nodes), Phase 11.2 (LLM Semantic Summary), Phase 96 (IDE + Repo Integration / MCP).

## Files Changed

### Modified Files
* `internal/models/models.go`
* `internal/api/server.go`
* `cmd/mcp-server/main.go` (or related MCP registration files)

### New Files
* `internal/api/handlers_guardrail.go`
* `internal/guardrail/service.go`
* `docs/api/api-spec/uats/drafts/guardrail_validate.phase104.uats.json`
* `docs/tests/usts/specs/guardrail_enforcement.phase104.usts.json`
