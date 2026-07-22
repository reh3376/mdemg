---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "102"
---

# Intent Translation

## Summary

**Feature**: Intent Translation
**Summary**: LLM-driven query rewriting before vector embedding, bridging the vocabulary mismatch between conversational agent queries and the declarative text stored in MDEMG's knowledge graph.

## Vision & Goals

Users and agents ask questions in conversational language ("Why do we use Redis?") while the knowledge graph stores information in declarative form ("Architecture Decision: Redis selected for session state"). Intent translation bridges this vocabulary gap by rewriting queries into keyword-rich search strings optimized for vector similarity, dramatically improving retrieval recall for conversational inputs.

## Current State

### Architecture

When `translate_intent: true` is set on a retrieval request:

1. **Intercept**: The raw conversational query is captured before embedding generation
2. **Translate**: An LLM rewrites the query into a dense, keyword-rich search string optimized for vector similarity against declarative graph text
3. **Embed**: The translated string (not the original) is used for vector embedding generation
4. **Return**: The `translated_intent` string is included in the API response for transparency

**Example**:
- Input: `"Why do we use Redis?"`
- Translated: `"Redis session state architecture decision caching layer database optimization"`

The LLM is constrained by a system prompt that instructs output of ONLY the rewritten query, requires domain-specific terms, expands abbreviations, removes conversational filler, and keeps output under 100 words. Temperature is set to 0.0 for deterministic rewrites.

### Workflow

**Endpoint Integration** — Translation is available on all three retrieval-adjacent endpoints:

| Endpoint | Request Field | Response Field |
|----------|--------------|----------------|
| `POST /v1/memory/retrieve` | `translate_intent: true` | `translated_intent` |
| `POST /v1/memory/consult` | `translate_intent: true` | `translated_intent` |
| `POST /v1/memory/suggest` | `translate_intent: true` | `translated_intent` |

**Critical**: In the consult endpoint, only the embedding input (`queryText`) is translated. The original `req.Question` is preserved for Phase 101 synthesis, preventing the synthesis engine from receiving keyword soup instead of the user's actual question.

**Fail-Open Design** — Three independent fallback paths:

1. `translate_intent: false` — Translation skipped entirely (default)
2. `INTENT_ENABLED=false` — Translator not initialized (nil check)
3. LLM error or timeout — Original query used, error logged, request continues

Per-call URL override: `?intent=true|false` on `/v1/memory/retrieve` (mirrors `?sparse=`) forces translation on/off for one call — the INTENT-DISABLE-001 re-verification A/B tool.

### Configuration

See Configuration Reference table below.

## Notes

### Known Limitations

- Disabled by default (`INTENT_ENABLED=false`) — evidence-backed (INTENT-DISABLE-001 120q UVTS A/B: intent OFF 0.4170 vs ON 0.4070, net −0.010); re-enable only after a fresh `?intent=true` 120q A/B shows a real lift
- Adds avg ~3.8s (up to 15s) latency per query (LLM round-trip, live-measured in INTENT-DISABLE-001)
- Quality depends on LLM model capability

### Risks & Gaps

- No caching of translated queries (same input re-translated each time)

### Future Improvements

- Translation cache with TTL
- Offline translation model (eliminate LLM dependency)

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| POST | `/v1/memory/retrieve` | Semantic retrieval with optional `translate_intent` | `specs/retrieve.uats.json` |
| POST | `/v1/memory/consult` | SME consultation with optional `translate_intent` | `specs/consult.uats.json` |
| POST | `/v1/memory/suggest` | Suggestion with optional `translate_intent` | `specs/suggest.uats.json` |

## CLI Commands

None — intent translation is API-only (triggered via request field).

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `INTENT_ENABLED` | `false` | Enable intent translation |
| `INTENT_PROVIDER` | `openai` | LLM provider: `openai` or `ollama` |
| `INTENT_MODEL` | `gpt-4o-mini` | Model for intent translation |
| `INTENT_MAX_TOKENS` | `150` | Max tokens for rewritten query (10-500) |
| `INTENT_TIMEOUT_MS` | `15000` | Timeout in ms (min: 200; raised from 2000 in INTENT-DISABLE-001 — 2000 was below avg local-model latency ~4400ms) |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Retrieval Pipeline | Requires — translation runs before embedding generation |
| LLM Client Infrastructure | Requires — OpenAI or Ollama provider |
| Phase 101 SME Synthesis | Enhances — translated embedding improves consult retrieval |
| TSDB Recording | Feeds into — translation events recorded for training data |

## Related Files

- `internal/retrieval/intent_translator.go` - IntentTranslator interface, LLM calls, system prompt
- `internal/retrieval/intent_translator_test.go` - 7 unit tests
- `internal/consulting/service.go` - Consult() and Suggest() integration
- `internal/api/handlers.go` - handleRetrieve integration
- `internal/api/server.go` - IntentTranslator initialization and wiring
- `internal/config/config.go` - 5 `INTENT_*` config fields
