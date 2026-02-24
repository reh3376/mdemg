<!-- markdownlint-disable MD013 MD022 MD032 MD040 -->
# Feature Spec: SME Synthesis Engine

**Phase**: 101
**Status**: Draft
**Author**: Agent
**Date**: 2026-02-23

---

## Overview

The SME Synthesis Engine transforms the Agent Consulting Service (`/v1/memory/consult`) from a simple graph retriever with keyword-based classification into a true Subject Matter Expert (SME). By integrating an LLM Reasoning Module, the engine will perform multi-hop synthesis over retrieved graph nodes, explicitly explaining *why* the retrieved patterns, constraints, and historical decisions matter to the agent's current task.

This addresses Gap 1 in the Cognitive Intelligence Gap Analysis, moving MDEMG closer to its vision of providing an "Internal Dialog" for AI agents.

## Requirements

### Functional Requirements

1. **FR-1**: Extend the `POST /v1/memory/consult` endpoint to support an `llm_synthesis` flag.
2. **FR-2**: Integrate the LLM Semantic Summary Service (Phase 11.2) into the consulting pipeline to synthesize raw `RetrieveResult` and `RelatedConcept` nodes into a coherent, markdown-formatted SME response.
3. **FR-3**: Provide context-aware synthesis that directly addresses the user's `question` using the provided `context` and retrieved graph evidence.
4. **FR-4**: Fall back to the existing keyword-based classification and summary concatenation if `llm_synthesis` is false, the LLM service is unavailable, or the LLM request fails.
5. **FR-5**: Retain existing structured data (e.g., `Confidence`, `RelatedConcepts`, `Suggestions` array with raw nodes) in the response payload alongside the new synthesized narrative.

### Non-Functional Requirements

1. **NFR-1**: Performance — The LLM synthesis step should run concurrently or with strict timeouts to prevent the `/consult` endpoint from hanging indefinitely (max 30s timeout).
2. **NFR-2**: Transparency — The synthesized response must include citations or references to the source node IDs used to generate the advice, maintaining the "explainable retrieval" invariant.

## API Contract

### Endpoints

```
POST /v1/memory/consult
```

**Request (Extended):**

```json
{
  "space_id": "mdemg-dev",
  "context": "I am trying to implement a new caching layer for the user service.",
  "question": "Are there any existing patterns or constraints I should follow?",
  "max_suggestions": 5,
  "include_evidence": true,
  "llm_synthesis": true
}
```

**Response (Extended):**

```json
{
  "space_id": "mdemg-dev",
  "confidence": 0.85,
  "rationale": "Found 3 relevant patterns and 1 constraint related to caching.",
  "synthesis": "Based on the organization's history, you must use the `RedisCacheService` (Node: uuid-1) instead of in-memory caching to avoid the out-of-memory errors experienced in 2024. The typical pattern involves wrapping the service call with the `@Cacheable` decorator (Node: uuid-2).",
  "suggestions": [
    {
      "type": "constraint",
      "content": "Must use Redis for distributed caching.",
      "confidence": 0.9,
      "source_nodes": ["uuid-1"]
    }
  ],
  "related_concepts": []
}
```

## Data Model

### Go Types

```go
// In internal/models/models.go

// ConsultRequest is extended with LlmSynthesis
type ConsultRequest struct {
 SpaceID          string `json:"space_id"`
 Context          string `json:"context"`
 Question         string `json:"question"`
 MaxSuggestions   int    `json:"max_suggestions,omitempty"`
 IncludeEvidence  bool   `json:"include_evidence,omitempty"`
 LlmSynthesis     bool   `json:"llm_synthesis,omitempty"` // New field
}

// ConsultResponse is extended with Synthesis
type ConsultResponse struct {
 SpaceID         string           `json:"space_id"`
 Confidence      float64          `json:"confidence"`
 Rationale       string           `json:"rationale"`
 Synthesis       string           `json:"synthesis,omitempty"` // New field
 Suggestions     []Suggestion     `json:"suggestions"`
 RelatedConcepts []RelatedConcept `json:"related_concepts,omitempty"`
 Debug           map[string]any   `json:"debug,omitempty"`
}
```

## Internal Implementation Plan

1. **Modify `internal/models/models.go`**: Add `LlmSynthesis` to `ConsultRequest` and `Synthesis` to `ConsultResponse`.
2. **Modify `internal/consulting/service.go`**:
    * Inject the LLM Semantic Summary Service (or a generic LLM client interface) into the `consulting.Service` struct.
    * In the `Consult()` method, after Step 3 (generate suggestions) and Step 4 (fetch concepts), check if `req.LlmSynthesis` is true.
    * If true, build a prompt containing the user's `context`, `question`, and a JSON representation of the retrieved `suggestions` and `concepts`.
    * Call the LLM service to generate the SME synthesis.
    * Populate `resp.Synthesis` with the result.
3. **Update Configuration**: Ensure the existing `LLM_SUMMARY_*` configuration variables (Phase 11.2) are accessible to the consulting service for LLM client instantiation.

## Test Plan

### Unit Tests
* [ ] `internal/consulting/service_test.go`: Test `Consult()` with `LlmSynthesis=true` (mocking the LLM client to return a canned response).
* [ ] `internal/consulting/service_test.go`: Test `Consult()` fallback behavior when the LLM client returns an error.

### Integration / UATS Tests
* [ ] `docs/api/api-spec/uats/drafts/consult_synthesis.phase101.uats.json`: Verify `/v1/memory/consult` returns the `synthesis` field when requested.
* [ ] Update existing `consult.uats.json` variants to ensure backward compatibility when `llm_synthesis` is omitted.

## Acceptance Criteria

* [ ] AC-1: The `/v1/memory/consult` endpoint accepts the `llm_synthesis` boolean flag.
* [ ] AC-2: When requested, the response includes a markdown-formatted `synthesis` string generated from the retrieved context.
* [ ] AC-3: If `llm_synthesis` is false or fails, the endpoint degrades gracefully to the existing keyword-based behavior.
* [ ] AC-4: Draft UATS acceptance tests are written and pass schema validation.
* [ ] AC-5: SHA256 hash added to `docs/specs/manifest.sha256`.

## Dependencies

* Depends on: Phase 11.2 (LLM Semantic Summary Service) for the underlying LLM client infrastructure.
* Blocks: Phase 102 (Intent Translation) which will build on this advanced reasoning capability.

## Files Changed

### Modified Files
* `internal/models/models.go` — Add fields to request/response.
* `internal/consulting/service.go` — Integrate LLM synthesis logic.
* `internal/api/server.go` — Wire LLM client into the consulting service constructor.

### New Files
* `docs/api/api-spec/uats/drafts/consult_synthesis.phase101.uats.json` — Draft UATS specs.
