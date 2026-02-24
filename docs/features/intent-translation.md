# Intent Translation (Phase 102)

Phase 102 adds LLM-driven query rewriting before vector embedding, bridging the vocabulary mismatch between conversational agent queries and the declarative text stored in MDEMG's knowledge graph.

## How It Works

### Query Rewriting Pipeline

When `translate_intent: true` is set on a retrieval request:

1. **Intercept**: The raw conversational query is captured before embedding generation
2. **Translate**: An LLM rewrites the query into a dense, keyword-rich search string optimized for vector similarity against declarative graph text
3. **Embed**: The translated string (not the original) is used for vector embedding generation
4. **Return**: The `translated_intent` string is included in the API response for transparency

**Example**:
- Input: `"Why do we use Redis?"`
- Translated: `"Redis session state architecture decision caching layer database optimization"`
- The translated version embeds much closer to graph nodes like "Architecture Decision: Redis selected for session state due to..."

### System Prompt (The Cognitive Core)

The LLM is constrained by a carefully designed system prompt that:
- Describes what the knowledge graph contains (architecture decisions, code patterns, constraints, etc.)
- Instructs output of ONLY the rewritten query — no explanation, no preamble, no quotes
- Requires domain-specific terms, file names, function names, and technical jargon
- Expands abbreviations and adds synonyms
- Removes conversational filler
- Keeps output under 100 words
- Returns already keyword-dense queries unchanged

Temperature is set to `0.0` for deterministic rewrites — same query always produces the same translation.

### Fail-Open Design

Three independent fallback paths ensure the system never breaks:

1. **`translate_intent: false`** — Translation skipped entirely (default)
2. **`INTENT_ENABLED=false`** — Translator not initialized (nil check)
3. **LLM error or timeout** — Original query used, error logged, request continues

### Endpoint Integration

Translation is available on all three retrieval-adjacent endpoints:

| Endpoint | Request Field | Response Field |
|----------|--------------|----------------|
| `POST /v1/memory/retrieve` | `translate_intent: true` | `translated_intent` |
| `POST /v1/memory/consult` | `translate_intent: true` | `translated_intent` |
| `POST /v1/memory/suggest` | `translate_intent: true` | `translated_intent` |

**Critical**: In the consult endpoint, only the embedding input (`queryText`) is translated. The original `req.Question` is preserved for Phase 101 synthesis, preventing the synthesis engine from receiving keyword soup instead of the user's actual question.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `INTENT_ENABLED` | `false` | Enable intent translation |
| `INTENT_PROVIDER` | `openai` | LLM provider: `openai` or `ollama` |
| `INTENT_MODEL` | `gpt-4o-mini` | Model for intent translation |
| `INTENT_MAX_TOKENS` | `150` | Max tokens for rewritten query (range: 10-500) |
| `INTENT_TIMEOUT_MS` | `2000` | Timeout in ms (min: 200, NFR-1: ≤2s P95) |

Reuses shared `OPENAI_API_KEY`, `OPENAI_ENDPOINT`, `OLLAMA_ENDPOINT` from existing config.

## Key Files

| File | Purpose |
|------|---------|
| `internal/retrieval/intent_translator.go` | IntentTranslator interface, LLMIntentTranslator, system prompt, OpenAI/Ollama calls |
| `internal/retrieval/intent_translator_test.go` | 7 unit tests (disabled, empty, OpenAI success/failure, Ollama, timeout, prompt content) |
| `internal/consulting/service.go` | Consult() and Suggest() integration with local IntentTranslator interface |
| `internal/api/handlers.go` | handleRetrieve integration |
| `internal/api/server.go` | IntentTranslator initialization and wiring |
| `internal/config/config.go` | 5 INTENT_* config fields |
| `internal/models/models.go` | TranslateIntent/TranslatedIntent on 6 structs |
