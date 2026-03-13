# Jiminy Inner Voice

**Phase Jiminy** — Proactive guidance service for AI coding agents.

## Overview

Jiminy is the inner-voice guidance service that transforms MDEMG from a passive retrieval system into an active cognitive participant. Named after Jiminy Cricket (Pinocchio's conscience), it proactively surfaces constraints, prior corrections, contradictions, and frontier exploration opportunities from the knowledge graph — injected into every prompt before the agent sees it.

In the internal dialogue analogy: if MDEMG is the ANN equivalent of a human's inner dialogue, Jiminy is the **conscience** — the part that says "wait, we tried this before and it failed" or "be careful, there's a constraint here you might violate."

## How It Works

### 4-Source Parallel Fan-Out Architecture

Jiminy orchestrates four independent knowledge sources in parallel, each running as a goroutine with a shared timeout context:

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

**Processing pipeline:** Generate embedding → fan out to 4 sources → lock-protected merge → filter by min confidence → deduplicate by content → sort by priority then confidence → truncate to max items → format prompt augmentation.

**Graceful degradation:** If embeddings fail, vector-based sources (B, C, D) are skipped; consulting (A) still works. Late arrivals past the timeout are dropped silently.

## What Jiminy Surfaces

| Type | Example | When |
|------|---------|------|
| **Constraints** | `[must_not] Never use deprecated auth middleware` | Constraint nodes match current context |
| **Corrections** | `Prior correction: "Default model is text-embedding-3-large, NOT small"` | Past corrections are semantically similar |
| **Contradictions** | `Result A contradicts known pattern B (evidence: 3 rejections)` | CONTRADICTS edges exist between relevant nodes |
| **Patterns** | `API handlers follow: method check → parse → validate → service call → writeJSON` | Consulting service finds related patterns |
| **Frontiers** | `Scraper orchestration: limited knowledge, proceed carefully` | Frontier nodes match context (thin knowledge areas) |

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
| `internal/jiminy/service.go` | Core orchestration — Guide() method with 4-source fan-out |
| `internal/jiminy/types.go` | GuidanceRequest, GuidanceResponse, GuidanceItem, SourceCounts |
| `internal/jiminy/corrections.go` | Vector search for correction observations |
| `internal/jiminy/contradictions.go` | CONTRADICTS edge queries |
| `internal/jiminy/frontiers.go` | Vector search for frontier nodes |
| `internal/jiminy/formatter.go` | FormatPromptAugmentation() — injectable text renderer |
| `internal/jiminy/service_test.go` | 11 unit tests |
| `internal/api/handlers_jiminy.go` | POST /v1/jiminy/guide handler |
| `internal/retrieval/jiminy.go` | JiminyExplanation transparency layer for retrieval pipeline |
| `internal/config/config.go` | 6 JIMINY_* config fields |
| `internal/cli/mcp.go` | jiminy_guide MCP tool registration |
| `.claude/hooks/prompt-context.sh` | Hook that injects Jiminy guidance |
| `docs/specs/phase-jiminy-guidance.md` | Complete phase specification |
| `docs/api/api-spec/uats/specs/jiminy_guide.uats.json` | 4 functional contract test variants |
| `docs/api/api-spec/uats/specs/jiminy_guide_validation.uats.json` | 5 validation contract test variants |

## Dependencies

- **Consulting Service** (`internal/consulting/`) — Source A uses `Suggest()` for constraints and patterns
- **Embedding Provider** — Sources B, C, D require embeddings for vector similarity search
- **Neo4j** — All 4 sources query the graph database
- **Consolidation Pipeline** — Frontier nodes and constraint nodes must exist (created during consolidation)
- **CMS** — Correction observations must be stored via `POST /v1/conversation/correct`
