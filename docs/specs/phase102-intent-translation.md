<!-- markdownlint-disable MD013 MD022 MD032 MD040 -->
# Feature Spec: Intent Translation

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: 102
**Status**: Complete
**Author**: Agent
**Date**: 2026-02-23

---

## Overview

The Intent Translation feature bridges the gap between conversational AI queries (e.g., "Why do we use Redis?") and the factual, declarative text stored within MDEMG's vector index (e.g., "Architecture Decision: Redis selected for session state due to...").

By introducing an LLM-driven "Query Rewriting" step before vector embedding, we significantly improve recall. This phase builds upon the existing Skill Registry infrastructure to create a dedicated reasoning module that intercepts, expands, and aligns user queries with the underlying graph structure.

## Requirements

### Functional Requirements
1. **FR-1**: Intercept incoming queries to the `/v1/memory/retrieve`, `/v1/memory/consult`, and `/v1/memory/suggest` endpoints.
2. **FR-2**: Provide an optional `translate_intent` boolean flag on the request payloads. When true, perform intent translation.
3. **FR-3**: Use an LLM reasoning module to rewrite the conversational query into a dense, keyword-rich search string optimized for vector similarity against declarative code documentation and architectural decisions.
4. **FR-4**: Use the translated intent string for vector embedding generation instead of the raw user query.
5. **FR-5**: Return the `translated_intent` string in the API response to provide visibility into the query rewriting process.
6. **FR-6**: Fail open—if the LLM translation fails or times out, log the error and fall back to embedding the original user query.

### Non-Functional Requirements
1. **NFR-1**: Latency — The intent translation step should add no more than 2 seconds (P95) to the retrieval pipeline.
2. **NFR-2**: Transparency — The user/agent must be able to see the translated query in the response payload.

## API Contract

### Endpoints

```
POST /v1/memory/retrieve
POST /v1/memory/consult
POST /v1/memory/suggest
```

**Request (Extended):**

```json
{
  "space_id": "mdemg-dev",
  "query": "How do I handle database migrations here?",
  "translate_intent": true
}
```

**Response (Extended):**

```json
{
  "data": {
    "results": [ ... ],
    "translated_intent": "database migrations schema version cypher scripts execution runner cli"
  }
}
```

*(Note: The exact location of `translated_intent` in the response depends on the specific endpoint's data envelope, but it must be exposed.)*

## Data Model

### Go Types

```go
// In internal/models/models.go

// Extend RetrieveRequest, ConsultRequest, SuggestRequest
type RetrieveRequest struct {
    // ... existing fields ...
    TranslateIntent bool `json:"translate_intent,omitempty"`
}

type ConsultRequest struct {
    // ... existing fields ...
    TranslateIntent bool `json:"translate_intent,omitempty"`
}

type SuggestRequest struct {
    // ... existing fields ...
    TranslateIntent bool `json:"translate_intent,omitempty"`
}

// Extend Response models to include the translated string for debugging/transparency
type RetrieveResponse struct {
    // ... existing fields ...
    TranslatedIntent string `json:"translated_intent,omitempty"`
}

type ConsultResponse struct {
    // ... existing fields ...
    TranslatedIntent string `json:"translated_intent,omitempty"`
}

type SuggestResponse struct {
    // ... existing fields ...
    TranslatedIntent string `json:"translated_intent,omitempty"`
}
```

## Internal Implementation Plan

1. **Modify Request/Response Models**: Update `internal/models/models.go` to include `TranslateIntent` boolean flags and `TranslatedIntent` response strings across the retrieval/consulting endpoints.
2. **Create Translator Component**: Create `internal/retrieval/intent_translator.go` with an interface `IntentTranslator` that takes a raw query and returns a rewritten query using the LLM client.
3. **Integrate Translator into Pipeline**:
    * In `internal/retrieval/service.go` (`Retrieve()` method), check the `TranslateIntent` flag.
    * If true, call the `IntentTranslator`. Use the result as the text input for the `Embedder`.
    * Attach the translated string to the response object.
    * Repeat integration for `internal/consulting/service.go` (`Consult()` and `Suggest()`).
4. **Fallback Logic**: Ensure the `IntentTranslator` handles timeouts or errors gracefully by returning the original string, preventing the entire request from failing.

## Test Plan

### Unit Tests
* [ ] `internal/retrieval/intent_translator_test.go`: Test query expansion with a mock LLM.
* [ ] `internal/retrieval/intent_translator_test.go`: Test fallback behavior on mock LLM error/timeout.
* [ ] `internal/retrieval/service_test.go`: Verify the `TranslateIntent` flag correctly routes the text to the embedder.

### Integration / UATS Tests
* [ ] `docs/api/api-spec/uats/drafts/retrieve_intent_translation.phase102.uats.json`: Verify `/v1/memory/retrieve` with `translate_intent=true`.
* [ ] `docs/api/api-spec/uats/drafts/consult_intent_translation.phase102.uats.json`: Verify `/v1/memory/consult` with `translate_intent=true`.

## Acceptance Criteria

* [ ] AC-1: The retrieval, consult, and suggest endpoints accept the `translate_intent` flag.
* [ ] AC-2: When true, the API response includes the `translated_intent` string.
* [ ] AC-3: The system falls back to the original query if intent translation fails.
* [ ] AC-4: Draft UATS acceptance tests are written and pass schema validation.
* [ ] AC-5: SHA256 hash added to `docs/specs/manifest.sha256`.

## Dependencies

* Depends on: Phase 11.2 (LLM Semantic Summary Service) for the underlying LLM client.
* Blocks: None directly, though it significantly improves the context quality for Phase 101 (SME Synthesis).

## Files Changed

### Modified Files
* `internal/models/models.go` — Request/response type extensions.
* `internal/retrieval/service.go` — Pipeline integration.
* `internal/consulting/service.go` — Pipeline integration.
* `internal/api/server.go` — Wiring the translator component.

### New Files
* `internal/retrieval/intent_translator.go` — Core translation logic.
* `docs/api/api-spec/uats/drafts/retrieve_intent_translation.phase102.uats.json` — Draft UATS specs.
* `docs/api/api-spec/uats/drafts/consult_intent_translation.phase102.uats.json` — Draft UATS specs.
