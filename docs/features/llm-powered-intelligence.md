# LLM-Powered Intelligence

**Phase AR-3** — LLM reflection, constraint classification, and query classification.

## Overview

Three rule-based systems in MDEMG — RSIC reflection, constraint detection, and query type classification — now have LLM-powered alternatives that complement or replace their hardcoded logic. All three follow the same architecture pattern (EmergenceNamer), are opt-in via config, and fail-open to existing behavior when the LLM is unavailable.

This is not about replacing working systems with AI for its own sake. Each LLM path addresses a specific limitation of the rule-based approach:

| System | Rule-Based Limitation | LLM Advantage |
|--------|----------------------|---------------|
| **RSIC Reflection** | Pattern matching on static thresholds; can't detect cross-cycle trends | Sees full assessment + last 5 cycle outcomes; detects recurring failures and metric correlations |
| **Constraint Detection** | Keyword matching ("must", "should"); misses paraphrased constraints | Understands intent; classifies "always validate input" as `must` without requiring the keyword |
| **Query Classification** | Regex patterns; single-label only | Few-shot classification; multi-label support (a query can be both "code" and "architecture") |

## Architecture Pattern

All three LLM classifiers follow the **EmergenceNamer pattern** established in Phase 103:

```
┌─────────────────────────────────────────┐
│          LLM Classifier                 │
│                                         │
│  ┌───────────┐     ┌───────────┐       │
│  │  OpenAI   │ OR  │  Ollama   │       │
│  │ /chat/    │     │ /api/     │       │
│  │completions│     │generate   │       │
│  └─────┬─────┘     └─────┬─────┘       │
│        │                  │             │
│  ┌─────┴──────────────────┴──────┐      │
│  │    Circuit Breaker            │      │
│  │    (5 failures → open 60s)    │      │
│  └──────────────┬────────────────┘      │
│                 │                       │
│  ┌──────────────▼────────────────┐      │
│  │    JSON Grammar Constraint    │      │
│  │    (Ollama: format schema)    │      │
│  │    (OpenAI: prompt-enforced)  │      │
│  └──────────────┬────────────────┘      │
│                 │                       │
│  ┌──────────────▼────────────────┐      │
│  │    Strict Validation          │      │
│  │    (enum checks, skip bad)    │      │
│  └──────────────┬────────────────┘      │
│                 │                       │
│  ┌──────────────▼────────────────┐      │
│  │    LRU Cache                  │      │
│  │    (avoid repeated LLM calls) │      │
│  └───────────────────────────────┘      │
└─────────────────────────────────────────┘
```

**Shared characteristics:**
- **Dual provider**: OpenAI (chat completions) and Ollama (generate) with JSON grammar schemas
- **Circuit breaker**: Per-classifier breakers (e.g., `openai-rsic-reflect`, `ollama-constraint-classify`)
- **Strict output validation**: Invalid severities, actions, types are silently skipped (not errors)
- **Code fence stripping**: Handles `\`\`\`json ... \`\`\`` wrapper that LLMs sometimes add
- **Fail-open**: If the LLM fails, the existing rule-based path runs instead
- **Config defaults from EMERGENCE_***: Provider/model default from the emergence system config

## R3: LLM RSIC Reflection

### What It Does

Alongside the rule-based reflector (threshold checks), an LLM receives the current `SelfAssessmentReport`, the last 5 `CycleOutcome` entries, and calibration confidence per action type. It returns pattern-based insights that the rule engine might miss.

### How It Works

1. `Reflector.Reflect()` runs both the rule-based and LLM reflectors
2. LLM insights are prefixed with `llm:` in their `PatternID`
3. Results are merged via `deduplicateInsights()` — if both produce the same `recommended_action`, the rule-based one wins
4. If the LLM call fails (timeout, circuit open, bad JSON), only rule-based results are returned

### LLM Input

The user prompt includes:
- Current assessment (JSON): health, confidence, edge count, orphan ratio, etc.
- Last 5 cycles: actions executed, success/fail counts, criteria met, metrics before/after
- Calibration confidence: per-action success rate

### LLM Output

```json
[
  {
    "pattern_id": "recurring_prune_failure",
    "severity": "high",
    "description": "Edge pruning has failed in 3 consecutive cycles",
    "recommended_action": "prune_excess_edges",
    "reasoning": "Pattern detected across recent history"
  }
]
```

Valid severities: `low`, `medium`, `high`, `critical`.
Valid actions: `prune_decayed_edges`, `prune_excess_edges`, `tombstone_stale`, `graduate_volatile`, `trigger_consolidation`, `refresh_stale_edges`.

### Configuration

| Parameter | Default | Env Var | Description |
|-----------|---------|---------|-------------|
| RSICLLMReflectEnabled | `false` | `RSIC_LLM_REFLECT_ENABLED` | Enable LLM reflection |
| RSICLLMReflectProvider | (from EMERGENCE_PROVIDER) | `RSIC_LLM_REFLECT_PROVIDER` | `openai` or `ollama` |
| RSICLLMReflectModel | (from EMERGENCE_MODEL) | `RSIC_LLM_REFLECT_MODEL` | LLM model name |

## J3: LLM Constraint Classification

### What It Does

Replaces keyword-based constraint detection (`findApplicableConstraints`) with LLM classification. Given a text snippet from a knowledge node, the LLM determines if it expresses a requirement, prohibition, or recommendation.

### How It Works

1. For each retrieved node, the LLM classifies its text
2. Results are cached by `node_id` in an LRU cache (512 entries) — constraints don't change frequently
3. If the LLM is unavailable, falls back to the keyword matcher (now improved to correctly check "must not" before "must")

### LLM Output

```json
{"type": "must_not", "summary": "Never commit directly to main branch"}
```

Valid types: `must`, `must_not`, `should`, `should_not`, `none`.

### Keyword Fallback

The improved keyword matcher checks in this order (longest match first):
1. `must not`, `never`, `forbidden`, `prohibited` → `must_not`
2. `should not`, `avoid`, `discouraged` → `should_not`
3. `must`, `required`, `always`, `mandatory` → `must`
4. `should`, `recommended`, `prefer` → `should`

Additionally, nodes must have `Score >= 0.5` to be considered constraint candidates.

### Configuration

| Parameter | Default | Env Var | Description |
|-----------|---------|---------|-------------|
| ConsultingLLMConstraintsEnabled | `false` | `CONSULTING_LLM_CONSTRAINTS_ENABLED` | Enable LLM constraint classification |
| ConsultingLLMConstraintsProvider | (from EMERGENCE_PROVIDER) | `CONSULTING_LLM_CONSTRAINTS_PROVIDER` | `openai` or `ollama` |
| ConsultingLLMConstraintsModel | (from EMERGENCE_MODEL) | `CONSULTING_LLM_CONSTRAINTS_MODEL` | LLM model name |

## C1: LLM Query Classification

### What It Does

Replaces regex-based query type detection in `ComputeRetrievalHints()` with LLM few-shot classification. Supports multi-label queries (e.g., "how does auth middleware handle JWT?" is both `code` and `architecture`).

### How It Works

1. The query text is hashed (SHA-256, first 16 bytes) and checked against the LRU cache (256 entries)
2. On cache miss, the LLM classifies the query using few-shot examples in the system prompt
3. Multi-label results use the most permissive retrieval hints (highest SeedN, deepest HopDepth)
4. If the LLM is unavailable, falls back to regex matchers

### LLM Output

```json
{
  "types": ["code", "architecture"],
  "temporal": "recent"
}
```

Valid types: `code`, `architecture`, `relationship`, `data_flow`, `symbol_lookup`, `generic`.
Valid temporal: `recent`, `historical`, `none`.

Invalid types are filtered; if all are invalid, falls back to `["generic"]`.

### Multi-Label Hint Selection

When a query has multiple types, `ComputeRetrievalHintsWithLLM()` computes hints for each type and takes the maximum:

```go
// For each type, compute base hints
// Take: max(SeedN), max(HopDepth), union of EdgeTypeHints
```

This ensures that a "code + architecture" query gets the deeper traversal of architecture queries combined with the code-specific edge type hints.

### Configuration

| Parameter | Default | Env Var | Description |
|-----------|---------|---------|-------------|
| RetrievalLLMClassifyEnabled | `false` | `RETRIEVAL_LLM_CLASSIFY_ENABLED` | Enable LLM query classification |
| RetrievalLLMClassifyProvider | (from EMERGENCE_PROVIDER) | `RETRIEVAL_LLM_CLASSIFY_PROVIDER` | `openai` or `ollama` |
| RetrievalLLMClassifyModel | (from EMERGENCE_MODEL) | `RETRIEVAL_LLM_CLASSIFY_MODEL` | LLM model name |

## Enabling All Three

To enable all LLM intelligence features:

```bash
# In .env or environment
RSIC_LLM_REFLECT_ENABLED=true
CONSULTING_LLM_CONSTRAINTS_ENABLED=true
RETRIEVAL_LLM_CLASSIFY_ENABLED=true

# Provider defaults from emergence config (or override per-feature)
EMERGENCE_PROVIDER=openai
EMERGENCE_MODEL=gpt-4o-mini
OPENAI_API_KEY=sk-...
```

Or with Ollama:

```bash
EMERGENCE_PROVIDER=ollama
EMERGENCE_MODEL=llama3.2:3b
OLLAMA_URL=http://localhost:11434
```

## Key Files

| File | Description |
|------|-------------|
| **R3: LLM Reflector** | |
| `internal/ape/llm_reflector.go` | LLM reflector — prompt, OpenAI/Ollama clients, parse |
| `internal/ape/self_reflect.go` | Merge logic — `SetLLMReflector()`, `deduplicateInsights()` |
| `internal/ape/llm_reflector_test.go` | 8 unit tests |
| **J3: Constraint Classifier** | |
| `internal/consulting/llm_classifier.go` | LLM constraint classifier — prompt, cache, clients, parse |
| `internal/consulting/service.go` | Integration — `SetConstraintClassifier()`, `keywordClassifyConstraint()` |
| `internal/consulting/llm_classifier_test.go` | 9 unit tests + 7 keyword subtests |
| **C1: Query Classifier** | |
| `internal/retrieval/query_classifier.go` | LLM query classifier — prompt, cache, clients, parse |
| `internal/retrieval/scoring.go` | Integration — `ComputeRetrievalHintsWithLLM()` |
| `internal/retrieval/query_classifier_test.go` | 10 unit tests |
| **Shared** | |
| `internal/config/config.go` | 9 `*_LLM_*` config fields |
| `internal/api/server.go` | Wiring — creates classifiers, calls setters |
| `internal/circuitbreaker/` | Per-classifier circuit breaker instances |

## Response Sanitization (SanitizeResponse)

All 16 LLM consumers (not just the 3 AR-3 classifiers) now use `SanitizeResponse()` before JSON parsing. This unified pipeline strips `<think>...</think>` blocks (Qwen3 think mode), removes code fences, and trims whitespace. Without this, switching to any local model with think mode would break all JSON-parsing consumers.

See [LLM Response Sanitization](llm-response-sanitization.md) for the full call site table and implementation details.

## Dependencies

- **EmergenceNamer (Phase 103)** — established the OpenAI/Ollama dual-provider, circuit breaker, JSON grammar pattern
- **Circuit Breaker** (`internal/circuitbreaker/`) — per-classifier protection
- **RSIC Core (Phase 60b)** — reflector, calibrator, assessment reports
- **Consulting Service** — constraint detection pipeline
- **Retrieval Pipeline** — query hint computation
- **LLM Provider** — OpenAI API key or Ollama running locally
