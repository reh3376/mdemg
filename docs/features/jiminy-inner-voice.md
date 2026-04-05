---
created: 2026-03-30
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "Jiminy"
---

# Jiminy Inner Voice

## Summary

**Feature**: Jiminy Inner Voice
**Summary**: 5-source parallel fan-out guidance system (J7) with LLM synthesis (J8), constraint tracking, trust scoring, and feedback loop closure for AI-agent behavioral guidance.


**Phase Jiminy** — Proactive guidance service for AI coding agents.

## Overview

Jiminy is the inner-voice guidance service that transforms MDEMG from a passive retrieval system into an active cognitive participant. Named after Jiminy Cricket (Pinocchio's conscience), it proactively surfaces constraints, prior corrections, contradictions, and frontier exploration opportunities from the knowledge graph — injected into every prompt before the agent sees it.

In the internal dialogue analogy: if MDEMG is the ANN equivalent of a human's inner dialogue, Jiminy is the **conscience** — the part that says "wait, we tried this before and it failed" or "be careful, there's a constraint here you might violate."

## How It Works

### 5-Source Parallel Fan-Out Architecture (J7)

Jiminy orchestrates five independent knowledge sources in parallel, each running as a goroutine with a shared timeout context:

```
User Prompt
    │
    ▼
┌─────────────────────────────────────────────┐
│           Jiminy Guide() — 6s timeout        │
│                                              │
│  ┌──────────┐ ┌──────────┐ ┌────────────┐  │
│  │    A:     │ │    B:    │ │     C:     │  │
│  │ Consult  │ │ Correct  │ │ Contradict │  │
│  │Suggest() │ │ Vector   │ │   Edge     │  │
│  │          │ │ Search   │ │  Lookup    │  │
│  └────┬─────┘ └────┬─────┘ └─────┬──────┘  │
│       │             │              │         │
│  ┌────┴─────────────┴──────────────┴──────┐  │
│  │     Lock-Protected Merge + Dedup       │  │
│  └────────────────┬───────────────────────┘  │
│                   │                          │
│  ┌────────────────▼───────────────────────┐  │
│  │  Filter → Sort → Truncate → Format    │  │
│  └────────────────────────────────────────┘  │
│                                              │
│  ┌──────────┐                                │
│  │    D:    │  (runs in parallel with A-C)   │
│  │ Frontier │                                │
│  │ Surface  │                                │
│  └──────────┘                                │
└─────────────────────────────────────────────┘
    │
    ▼
Prompt Augmentation Block (injected before agent sees prompt)
```

| Source | Method | What It Finds | Priority |
|--------|--------|---------------|----------|
| **A: Consulting Suggestions** | `consulting.Suggest()` | Constraints, conflicts, patterns, concepts | High |
| **B: Correction Vector Search** | `findRelevantCorrections()` | Past correction observations via semantic similarity | High |
| **C: Contradiction Checking** | `findContradictions()` | `CONTRADICTS` edges between nodes (conflicting knowledge) | High |
| **D: Frontier Surfacing** | `findRelevantFrontiers()` | Frontier nodes (`is_frontier=true`) — thin knowledge areas | Low |
| **E: Full Retrieval Pipeline** (J7) | `retriever.RetrieveForJiminy()` | All 8 obs_types, L0-L5 concepts, Hebbian edges, 14-component scoring | Varies by layer |

**Processing pipeline:** Generate embedding → fan out to 5 sources → lock-protected merge → filter by min confidence → deduplicate by content → sort by priority then confidence → truncate to max items → apply escalation effects (J12) → LLM synthesis (J8, if enabled) → format prompt augmentation.

**Graceful degradation:** If embeddings fail, vector-based sources (B, C, D) are skipped; consulting (A) and retrieval (E) still work. Late arrivals past the timeout are dropped silently. LLM synthesis (J8) falls back to static list formatting on error.

## What Jiminy Surfaces

| Type | Example | When |
|------|---------|------|
| **Constraints** | `[must_not] Never use deprecated auth middleware` | Constraint nodes match current context |
| **Corrections** | `Prior correction: "Default model is text-embedding-3-large, NOT small"` | Past corrections are semantically similar |
| **Contradictions** | `Result A contradicts known pattern B (evidence: 3 rejections)` | CONTRADICTS edges exist between relevant nodes |
| **Patterns** | `API handlers follow: method check → parse → validate → service call → writeJSON` | Consulting service finds related patterns |
| **Frontiers** | `Scraper orchestration: limited knowledge, proceed carefully` | Frontier nodes match context (thin knowledge areas) |
| **Decisions** (J7) | `Past decision: Use PostgreSQL for transactional data` | Decision observations retrieved via full pipeline |
| **Learnings** (J7) | `Learning: DBSCAN eps=0.3 works best for concept clustering` | Learning observations from retrieval pipeline |
| **Preferences** (J7) | `Preference: Use conventional commits for all messages` | User/team preference observations |
| **Concepts** (J7) | `Concept: Auth middleware pattern (L3, high-level)` | L2-L5 concept nodes from knowledge hierarchy |

## Injection Format

When guidance exists, Jiminy injects a block into the prompt context:

```
═══ JIMINY GUIDANCE ═══
CONSTRAINTS:
  • [must_not] Never use deprecated auth middleware (conf: 0.92)
  • [should] Use structured logging pattern from internal/log (conf: 0.78)
CORRECTIONS:
  • Prior correction: "Default embedding model is text-embedding-3-large, NOT text-embedding-3-small" (conf: 0.85)
PATTERNS:
  • API handlers follow: method check → parse → validate → service call → writeJSON (conf: 0.71)
CONFLICTS:
  • Result A contradicts known pattern B (evidence: 3 rejections) (conf: 0.65)
FRONTIERS:
  • Scraper orchestration: limited knowledge, proceed carefully (conf: 0.58)
[confidence: 0.82 | sources: 2 constraints, 1 correction, 1 pattern, 1 conflict, 1 frontier]
═══ END JIMINY GUIDANCE ═══
```

**Formatting rules:**
- Empty guidance → no block injected (silent when irrelevant)
- Items grouped by type (constraints, corrections, patterns, conflicts, frontiers)
- Each item shows content + confidence score (2 decimal places)
- Summary line lists source counts
- Wrapped with `═══ JIMINY GUIDANCE ═══` / `═══ END JIMINY GUIDANCE ═══`

## Configuration

| Parameter | Default | Env Var | Description |
|-----------|---------|---------|-------------|
| JiminyEnabled | `true` | `JIMINY_ENABLED` | Master toggle for Jiminy guidance service |
| JiminyTimeoutMs | `6000` | `JIMINY_TIMEOUT_MS` | Overall timeout for Guide() in milliseconds |
| JiminyMaxItems | `10` | `JIMINY_MAX_ITEMS` | Maximum guidance items returned |
| JiminyMinConfidence | `0.3` | `JIMINY_MIN_CONFIDENCE` | Minimum confidence threshold to include an item |
| JiminyIncludeFrontiers | `true` | `JIMINY_INCLUDE_FRONTIERS` | Enable/disable frontier node suggestions |
| JiminyFrontierMinSim | `0.5` | `JIMINY_FRONTIER_MIN_SIM` | Minimum cosine similarity for frontier nodes |

Additional CMS config: `CMS_JIMINY_BASE_CONFIDENCE` (default: `0.5`) — base confidence for Jiminy rationale.

### J7-J12 Configuration

| Parameter | Default | Env Var | Description |
|-----------|---------|---------|-------------|
| JiminyRetrievalEnabled | `true` | `JIMINY_RETRIEVAL_ENABLED` | Enable Source E (full retrieval pipeline) |
| JiminyRetrievalTopK | `10` | `JIMINY_RETRIEVAL_TOP_K` | Max results from retrieval pipeline |
| JiminyRetrievalHopDepth | `2` | `JIMINY_RETRIEVAL_HOP_DEPTH` | Graph hop depth for retrieval |
| JiminySynthesisEnabled | `true` | `JIMINY_SYNTHESIS_ENABLED` | Enable LLM-powered guidance synthesis (J8) |
| JiminySynthesisProvider | (inherits) | `JIMINY_SYNTHESIS_PROVIDER` | LLM provider for synthesis |
| JiminySynthesisModel | (inherits) | `JIMINY_SYNTHESIS_MODEL` | LLM model for synthesis |
| JiminySynthesisMaxTokens | `2000` | `JIMINY_SYNTHESIS_MAX_TOKENS` | Max tokens for synthesis |
| JiminySynthesisTimeoutMs | `10000` | `JIMINY_SYNTHESIS_TIMEOUT_MS` | Synthesis timeout (ms) |
| JiminyEvaluateEnabled | `true` | `JIMINY_EVALUATE_ENABLED` | Enable agent output evaluation (J9) |
| JiminyEvaluateTimeoutMs | `3000` | `JIMINY_EVALUATE_TIMEOUT_MS` | Evaluation timeout (ms) |
| JiminyEvaluateMaxConstraints | `10` | `JIMINY_EVALUATE_MAX_CONSTRAINTS` | Max constraints to check per evaluation |
| JiminyOutcomeClassifierEnabled | `true` | `JIMINY_OUTCOME_CLASSIFIER_ENABLED` | Enable semantic outcome classification (J11) |
| JiminyOutcomeLLMEnabled | `false` | `JIMINY_OUTCOME_LLM_ENABLED` | Enable LLM tier for uncertain classifications |
| JiminyOutcomeSimilarityHigh | `0.7` | `JIMINY_OUTCOME_SIMILARITY_HIGH` | Cosine similarity threshold for "followed" |
| JiminyOutcomeSimilarityLow | `0.3` | `JIMINY_OUTCOME_SIMILARITY_LOW` | Cosine similarity threshold for "uncertain" |
| JiminyEscalationEnabled | `true` | `JIMINY_ESCALATION_ENABLED` | Enable session-aware escalation (J12) |
| JiminyEscalationWarnAfter | `2` | `JIMINY_ESCALATION_WARN_AFTER` | Ignores before WARNED |
| JiminyEscalationEscalateAfter | `4` | `JIMINY_ESCALATION_ESCALATE_AFTER` | Ignores before ESCALATED |
| JiminyEscalationBlockAfter | `6` | `JIMINY_ESCALATION_BLOCK_AFTER` | Ignores before BLOCKED |
| JiminyEscalationBlockEnabled | `false` | `JIMINY_ESCALATION_BLOCK_ENABLED` | Enable hard blocking at max escalation |
| JiminyEscalationDecayMinutes | `60` | `JIMINY_ESCALATION_DECAY_MINUTES` | Escalation state decay period |

### J13-J15 Configuration

| Parameter | Default | Env Var | Description |
|-----------|---------|---------|-------------|
| JiminyEvaluateLLMEnabled | `false` | `JIMINY_EVALUATE_LLM_ENABLED` | Enable LLM Tier 2 evaluation reasoning (J13) |
| JiminyEvaluateLLMProvider | (inherits) | `JIMINY_EVALUATE_LLM_PROVIDER` | LLM provider for evaluation |
| JiminyEvaluateLLMModel | (inherits) | `JIMINY_EVALUATE_LLM_MODEL` | LLM model for evaluation |
| JiminyEvaluateLLMTimeoutMs | `5000` | `JIMINY_EVALUATE_LLM_TIMEOUT_MS` | LLM request timeout (ms) |
| JiminyEvaluateLLMMaxTokens | `2000` | `JIMINY_EVALUATE_LLM_MAX_TOKENS` | Max tokens for LLM evaluation |
| JiminyOutcomeLLMMaxTokens | `100` | `JIMINY_OUTCOME_LLM_MAX_TOKENS` | Max tokens for outcome classification (J14) |
| JiminyOutcomeCacheSize | `256` | `JIMINY_OUTCOME_CACHE_SIZE` | LRU cache capacity for classifications |
| JiminySynthesisTemperature | (API default) | `JIMINY_SYNTHESIS_TEMPERATURE` | Optional temperature override (J15) |

### Docker Deployment

When running via Docker Compose, Jiminy configuration is propagated from `.env` to the container. Running `mdemg init --defaults` (or answering "yes" to the Jiminy prompt) writes the following to `.env`:

```bash
JIMINY_ENABLED=true
JIMINY_SYNTHESIS_MODEL=gpt-5.4-nano      # or your chosen model
JIMINY_EVALUATE_LLM_MODEL=gpt-5.4-nano
JIMINY_SYNTHESIS_PROVIDER=openai          # inherits from LLM_PROVIDER
JIMINY_EVALUATE_LLM_PROVIDER=openai
```

The `docker-compose.yml` template passes these through with empty defaults — the server's `FromEnv()` falls back to `LLM_PROVIDER`/`LLM_MODEL` when sub-settings are unset. If `.env` does not contain `JIMINY_ENABLED`, Docker Compose defaults to `false`.

## Hook Integration

Jiminy guidance is injected via Claude Code hooks that run on every user prompt submission.

### prompt-context.sh (Bash — macOS/Linux)

The `prompt-context.sh` hook calls `/v1/jiminy/guide` after CMS recall completes. The user's prompt text is sent as `context`, and the returned `prompt_augmentation` block is appended to the system reminder.

- **Trigger**: User prompt > 15 characters
- **Timeout**: 6s max (3s connect timeout)
- **Failure mode**: Silent — `|| true` ensures hook never blocks the prompt
- **Server is single source of truth**: No `JIMINY_ENABLED` check in the hook; server returns 503 if disabled

### prompt-context.ps1 (PowerShell — Windows)

Windows equivalent using `Invoke-RestMethod` with the same endpoint and timeout behavior.

### session-start.sh / session-start.ps1

Session start hooks call `/v1/conversation/resume` to restore CMS context. This ensures Jiminy has observations and corrections available to search against.

## CLI Commands

```bash
# Install Claude Code hooks (includes prompt-context with Jiminy injection)
mdemg hooks install --type claude

# Uninstall hooks
mdemg hooks uninstall --type claude

# List installed hooks
mdemg hooks list
```

The `mdemg hooks install` command installs both `session-start` and `prompt-context` hooks, which together provide CMS resume + Jiminy guidance injection.

## API Endpoint

### `POST /v1/jiminy/guide`

**Request:**

```bash
curl -s -X POST http://localhost:9999/v1/jiminy/guide \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "mdemg-dev",
    "context": "Refactoring authentication middleware to use new JWT library",
    "file_path": "internal/auth/middleware.go",
    "session_id": "claude-core",
    "max_items": 10
  }'
```

| Field | Required | Description |
|-------|----------|-------------|
| `space_id` | Yes | Knowledge space to search |
| `context` | Yes | What the agent is currently doing |
| `file_path` | No | Current file being edited |
| `agent_output` | No | Proposed code or action for review |
| `query` | No | User's original query |
| `session_id` | No | For correction lookup scoping |
| `max_items` | No | Override default max items |

**Response (200 OK):**

```json
{
  "data": {
    "guidance": [
      {
        "type": "constraint",
        "priority": "high",
        "content": "[must_not] Never use deprecated auth middleware",
        "confidence": 0.92,
        "source_nodes": ["node-uuid-1"]
      }
    ],
    "prompt_augmentation": "═══ JIMINY GUIDANCE ═══\n...\n═══ END JIMINY GUIDANCE ═══",
    "confidence": 0.82,
    "rationale": "Found 2 guidance items: 1 constraints, 1 corrections",
    "warnings": [],
    "source_counts": {
      "constraints": 1,
      "corrections": 1,
      "patterns": 0,
      "conflicts": 0,
      "frontiers": 0
    }
  }
}
```

**Error responses:** 400 (missing required fields), 405 (wrong HTTP method), 503 (Jiminy disabled).

**Note:** The response now includes a `guidance_id` (CUID2 unique identifier) in the `data` object for effectiveness tracking. See [Jiminy Effectiveness Tracking](jiminy-effectiveness-tracking.md) for the feedback loop.

### `POST /v1/jiminy/evaluate` (J9)

Evaluate agent output (code, actions) against stored constraints and corrections.

```bash
curl -s -X POST http://localhost:9999/v1/jiminy/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "mdemg-dev",
    "agent_output": "const API_KEY = \"sk-abc123\"",
    "file_path": "internal/auth/config.go",
    "tool_name": "Write",
    "session_id": "claude-core"
  }'
```

| Field | Required | Description |
|-------|----------|-------------|
| `space_id` | Yes | Knowledge space to search |
| `agent_output` | Yes | Code or action to evaluate |
| `file_path` | No | File being modified |
| `tool_name` | No | Tool that produced the output (Write, Edit) |
| `session_id` | No | Session identifier |

**Response (200 OK):**
```json
{
  "data": {
    "evaluation_id": "uuid",
    "status": "warning",
    "items": [
      {"type": "constraint", "content": "[must_not] No hardcoded secrets (sim: 0.87)", "severity": "high", "source_node": "node-id"}
    ],
    "summary": "Found 1 potential concern(s) in agent output"
  }
}
```

**Status values:** `pass` (no concerns), `warning` (medium severity matches), `concern` (high severity matches).

**Hook integration:** The `post-tool-observe.py` hook automatically calls this endpoint after Write/Edit completions and injects warnings as system-reminders.

### `POST /v1/jiminy/feedback`

Record whether guidance was followed, ignored, or contradicted. See [Jiminy Effectiveness Tracking](jiminy-effectiveness-tracking.md) for full documentation.

```bash
curl -s -X POST http://localhost:9999/v1/jiminy/feedback \
  -H "Content-Type: application/json" \
  -d '{"guidance_id":"<id>","action_summary":"I validated the input","space_id":"mdemg-dev"}'
```

## MCP Tool

**Tool name:** `jiminy_guide`

Available via `mdemg serve --mcp` or `mdemg mcp`. Registered with parameters:

| Parameter | Required | Description |
|-----------|----------|-------------|
| `context` | Yes | What you're currently working on |
| `file_path` | No | Path of the file being edited |
| `agent_output` | No | Your proposed code or action |
| `space_id` | No | Space ID (default: `ide-agent`) |

Returns the `prompt_augmentation` text directly for IDE injection.

## Related Files

| File | Description |
|------|-------------|
| `internal/jiminy/service.go` | Core orchestration — Guide() with 5-source fan-out; RecordOutcome() with semantic classification |
| `internal/jiminy/types.go` | All types: GuidanceRequest/Response, EvaluateRequest/Response, EscalationLevel, RetrievalProvider |
| `internal/jiminy/effectiveness.go` | EffectivenessTracker — LRU cache with TTL for guidance tracking |
| `internal/jiminy/corrections.go` | Vector search for correction observations |
| `internal/jiminy/contradictions.go` | CONTRADICTS edge queries |
| `internal/jiminy/frontiers.go` | Vector search for frontier nodes |
| `internal/jiminy/formatter.go` | FormatPromptAugmentation() — injectable text renderer (includes J7 types) |
| `internal/jiminy/retrieval_source.go` | J7: Maps RetrievalResult → GuidanceItem with obs_type/layer classification |
| `internal/jiminy/synthesizer.go` | J8: LLM-powered guidance synthesis with circuit breaker |
| `internal/jiminy/guidance_prompt.go` | J8: System/user prompt templates for LLM synthesis |
| `internal/jiminy/evaluator.go` | J9: Agent output evaluation against constraints/corrections |
| `internal/jiminy/stats.go` | J10: StatsCollector for RSIC integration (follow rate, effectiveness, diversity) |
| `internal/jiminy/outcome_classifier.go` | J11: Two-tier semantic outcome classification (embedding + optional LLM) |
| `internal/jiminy/escalation.go` | J12: Session-aware escalation state machine |
| `internal/jiminy/service_test.go` | 20 original unit tests |
| `internal/jiminy/j7_j12_test.go` | 22 unit tests for J7-J12 features |
| `internal/api/handlers_jiminy.go` | Handlers for guide, feedback, evaluate endpoints |
| `internal/api/rsic_adapters.go` | Adapter wiring: jiminyRetrievalAdapter, rsicJiminyAdapter |
| `internal/retrieval/jiminy.go` | JiminyExplanation transparency layer for retrieval pipeline |
| `internal/ape/self_assess.go` | J10: GuidanceHealth dimension in RSIC assessment |
| `internal/ape/self_reflect.go` | J10: Jiminy-specific reflection patterns |
| `internal/config/config.go` | ~30 JIMINY_* and RSIC_JIMINY_* config fields |
| `internal/cli/mcp.go` | jiminy_guide MCP tool registration |
| `.claude/hooks/prompt-context.sh` | Hook that injects Jiminy guidance |
| `docs/specs/phase-jiminy-guidance.md` | Complete phase specification |
| `docs/api/api-spec/uats/specs/jiminy_guide.uats.json` | 4 functional contract test variants |
| `docs/api/api-spec/uats/specs/jiminy_guide_validation.uats.json` | 5 validation contract test variants |
| `docs/api/api-spec/uats/specs/jiminy_feedback.uats.json` | 4 feedback contract test variants |
| `docs/features/jiminy-effectiveness-tracking.md` | Effectiveness tracking feature doc |
| `docs/features/j17-ai2ai-protocol.md` | J17 AI-to-AI communication protocol (3-tier encoding, trust, codegen, ML tier prediction) |
| `internal/jiminy/codegen.go` | J17-2: LLM-powered constraint code generator |
| `internal/jiminy/encoder.go` | J17-1: Three-tier protocol encoder (T1/T2/T3) |
| `internal/jiminy/trust.go` | J17-3: Per-session trust scoring |
| `internal/jiminy/ticket.go` | J17-3: Signed session tickets for state persistence |
| `internal/jiminy/protocol.go` | J17-1: Protocol types and constants |
| `internal/jiminy/sequence.go` | J17-1: Sequence number tracking |
| `internal/jiminy/protocol_metrics.go` | J17-4: Protocol performance metrics collection |
| `internal/jiminy/protocol_data_collector.go` | J17-4: JSONL training data collector for ML pipeline |
| `internal/jiminy/protocol_evolution.go` | J17-4: RSIC-driven protocol evolution engine |
| `internal/jiminy/nli_comprehension.go` | J17-4: NLI comprehension testing |
| `internal/jiminy/extensions.go` | J17-4: Extension negotiation for protocol capabilities |
| `internal/jiminy/tier_predictor.go` | J17-5: Go-side tier predictor (calls neural sidecar) |
| `internal/api/handlers_j17.go` | J17 HTTP handlers (bootstrap, feedback, learn, metrics) |
| `neural/neural_sidecar/tier_model.py` | J17-5: ML tier prediction model (CrossEncoder) |
| `neural/neural_sidecar/train_protocol.py` | J17-5: Training pipeline for tier prediction model |

## Training Data: RAFT Context Capture

When Jiminy synthesizes guidance (J8), the retrieval context — which constraint nodes were retrieved and their relevance scores — is now logged alongside the LLM interaction in the `llm_interactions` TSDB table. This enables RAFT (Retrieval Augmented Fine-Tuning): the fine-tuned model trains with the same retrieval context it will see during inference, avoiding the open-book/closed-book quality gap identified in UC Berkeley's RAFT research. See [RAFT Retrieval Context](raft-retrieval-context.md).

## Dependencies

- **Consulting Service** (`internal/consulting/`) — Source A uses `Suggest()` for constraints and patterns
- **Retrieval Service** (`internal/retrieval/`) — Source E uses full 14-component scoring pipeline (J7)
- **Embedding Provider** — Sources B, C, D require embeddings for vector similarity search; J11 classifier uses embeddings for semantic comparison
- **LLM Client** (`internal/llmclient/`) — J8 synthesis and J11 Tier 2 classification (optional)
- **Neo4j** — All 5 sources query the graph database; J9 evaluator searches constraints/corrections
- **RSIC** (`internal/ape/`) — J10 bidirectional feedback (GuidanceHealth in assessment, Jiminy-specific reflection patterns)
- **Consolidation Pipeline** — Frontier nodes and constraint nodes must exist (created during consolidation)
- **CMS** — Correction observations must be stored via `POST /v1/conversation/correct`
