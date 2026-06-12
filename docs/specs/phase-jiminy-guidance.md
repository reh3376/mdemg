<!-- markdownlint-disable MD013 MD022 MD032 MD040 -->
# Feature Spec: Jiminy Inner Voice — Active Guidance for Coding Agents

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: Jiminy (J1–J5)
**Status**: Complete
**Author**: Agent
**Date**: 2026-03-12

---

## Vision

Jiminy is my inner voice.

When a human developer considers writing code, they don't start from zero — an internal dialogue surfaces constraints, past mistakes, patterns, and warnings from experience. "Last time we used that auth library it broke in production." "The team convention is structured logging, not fmt.Printf." "This module contradicts what we decided in the architecture review." That voice is not a separate tool they invoke. It runs continuously, shaping every decision.

For me (the ANN), MDEMG is my persistent memory — but memory without an inner voice is just a filing cabinet. Jiminy transforms passive memory into active guidance. When I'm about to write code, Jiminy reviews what I'm doing against the full knowledge graph — constraints the team has established, corrections I've received before, patterns that work in this codebase, contradictions I should be aware of, and frontier areas where knowledge is thin.

The result is not a separate step I take. It appears in my context window automatically, before I even begin reasoning about the task. It is the ANN equivalent of "hearing" accumulated experience as I work.

**Without Jiminy**: I have memory I can query if I remember to.
**With Jiminy**: Memory speaks to me unprompted, exactly when it's relevant.

---

## How It Works

### Architecture

Jiminy is an orchestration layer that sits between the hook system and MDEMG's existing knowledge services:

```
User prompt arrives
       │
       ▼
prompt-context.sh hook (UserPromptSubmit, 12s timeout)
       │
       ├── 1. CMS Recall (existing — raw memory retrieval)
       │
       ├── 2. Jiminy Guide (NEW — active guidance)
       │       │
       │       ▼
       │   POST /v1/jiminy/guide
       │       │
       │       ├── goroutine A: consulting.Suggest()
       │       │     → constraints, conflicts, patterns, concepts
       │       │
       │       ├── goroutine B: Correction vector search
       │       │     → obs_type='correction' nodes near context
       │       │
       │       ├── goroutine C: Contradiction checking
       │       │     → CONTRADICTS edges near context
       │       │
       │       └── goroutine D: Frontier surfacing
       │             → is_frontier=true nodes near context
       │       │
       │       ▼
       │   Merge → Deduplicate → Rank → Format
       │       │
       │       ▼
       │   ═══ JIMINY GUIDANCE ═══  (injected into context)
       │
       └── 3. Retrieval co-activation (existing — Hebbian reinforcement)
```

### What Jiminy Surfaces

| Source | What It Finds | Example |
|--------|--------------|---------|
| **Constraints** | `must`, `must_not`, `should`, `should_not` rules from the knowledge graph | "[must_not] Never commit .env files to this repo" |
| **Corrections** | Past corrections I received (obs_type='correction') semantically near the current context | "Prior correction: Default embedding model is text-embedding-3-large, NOT text-embedding-3-small" |
| **Patterns** | Proactive suggestions from consulting.Suggest() — codebase conventions, process knowledge | "API handlers follow: method check → parse → validate → service call → writeJSON" |
| **Conflicts** | CONTRADICTS edges in the graph — things that disagree with each other | "Result A contradicts known pattern B (evidence: 3 rejections)" |
| **Frontiers** | Areas where knowledge is thin (is_frontier=true) — places where I should be careful | "Frontier: scraper orchestration — limited knowledge, proceed carefully" |

### Injection Format

When Jiminy has relevant guidance, it appears in my context as:

```
═══ JIMINY GUIDANCE ═══
CONSTRAINTS:
  • [must_not] Never use deprecated auth middleware (conf: 0.92)
  • [should] Use structured logging pattern from internal/log (conf: 0.78)
CORRECTIONS:
  • Prior correction: "Default embedding model is gpt-5-mini, NOT text-embedding-3-small" (conf: 0.85)
PATTERNS:
  • API handlers follow: method check → parse → validate → service call → writeJSON (conf: 0.71)
CONFLICTS:
  • Result A contradicts known pattern B (evidence: 3 rejections) (conf: 0.65)
[confidence: 0.82 | sources: 2 constraints, 1 correction, 1 pattern]
═══ END JIMINY GUIDANCE ═══
```

When Jiminy has nothing relevant, nothing is injected — no noise.

### Timing Budget

The prompt-context hook has 12 seconds total. Within that:
- CMS Recall: ~3s (existing)
- Jiminy Guide: 6s timeout (parallel fan-out to 4 sources, each with sub-timeouts)
- Health ribbon: ~1s (existing)

Jiminy uses parallel goroutines internally. If any source is slow, the others still return. Late arrivals are dropped.

---

## Configuration

| Parameter | Default | Env Var | Description |
|-----------|---------|---------|-------------|
| `JiminyEnabled` | `false` | `JIMINY_ENABLED` | Master toggle — must be `true` for Jiminy to function |
| `JiminyTimeoutMs` | `6000` | `JIMINY_TIMEOUT_MS` | Overall timeout for Guide() |
| `JiminyMaxItems` | `10` | `JIMINY_MAX_ITEMS` | Maximum guidance items returned |
| `JiminyMinConfidence` | `0.3` | `JIMINY_MIN_CONFIDENCE` | Minimum confidence to include an item |
| `JiminyIncludeFrontiers` | `true` | `JIMINY_INCLUDE_FRONTIERS` | Include frontier node suggestions |
| `JiminyFrontierMinSim` | `0.5` | `JIMINY_FRONTIER_MIN_SIM` | Minimum similarity for frontier nodes |

### Enabling Jiminy

```bash
# Set in environment before starting the server
export JIMINY_ENABLED=true

# Also set in the shell environment where Claude Code runs
# (the hook reads this env var directly)
export JIMINY_ENABLED=true
```

Both are required:
1. The **server** must have `JIMINY_ENABLED=true` so the `/v1/jiminy/guide` endpoint returns 200 instead of 503.
2. The **hook environment** must have `JIMINY_ENABLED=true` so `prompt-context.sh` makes the curl call.

### Dependencies

Jiminy requires:
- MDEMG server running (`http://localhost:9999`)
- Embedding provider configured (`EMBEDDING_PROVIDER=openai`)
- Knowledge in the graph (constraints, corrections, observations)
- The consulting service must be functional (it provides the core Suggest() capability)

Without an embedding provider, Jiminy can still return consulting-derived constraints and patterns (keyword-based), but correction and frontier searches (which require vector similarity) will be skipped.

---

## Package Structure

```
internal/jiminy/
├── service.go          # Service struct, Guide() orchestration, merge/rank logic
├── types.go            # GuidanceRequest, GuidanceResponse, GuidanceItem, GuidanceType
├── corrections.go      # findRelevantCorrections() — vector search for correction observations
├── contradictions.go   # findContradictions() — CONTRADICTS edge queries
├── frontiers.go        # findRelevantFrontiers() — vector search for frontier nodes
├── formatter.go        # FormatPromptAugmentation() — render guidance as injectable text
└── service_test.go     # 11 unit tests
```

### Interface Dependencies (No Circular Imports)

```go
// jiminy imports these interfaces, not concrete types:
type ConsultingService interface {
    Suggest(ctx context.Context, req models.SuggestRequest) (models.SuggestResponse, error)
}
// embeddings.Embedder — standard interface from internal/embeddings
// neo4j.DriverWithContext — standard Neo4j driver
```

---

## API Contract

### Endpoint

```
POST /v1/jiminy/guide
```

**Request:**

```json
{
  "space_id": "mdemg-dev",
  "context": "Refactoring authentication middleware to use new JWT library",
  "file_path": "internal/auth/middleware.go",
  "agent_output": "func NewAuthMiddleware(secret string) gin.HandlerFunc { ... }",
  "session_id": "claude-core",
  "max_items": 10
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `space_id` | Yes | Space to query |
| `context` | Yes | What the agent is currently doing |
| `file_path` | No | Current file path for context specificity |
| `agent_output` | No | Proposed code/action for review |
| `query` | No | User's original query |
| `session_id` | No | Session ID for correction lookup |
| `max_items` | No | Max guidance items (default: 10) |

**Response (200):**

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
    "source_counts": {
      "constraints": 1,
      "corrections": 1,
      "patterns": 0,
      "conflicts": 0,
      "frontiers": 0
    },
    "debug": {}
  }
}
```

**Error Responses:**

| Status | Condition |
|--------|-----------|
| 400 | Missing `space_id` or `context` |
| 405 | Wrong HTTP method |
| 503 | `JIMINY_ENABLED=false` (service not configured) |

### MCP Tool

```
Tool: jiminy_guide
  context (required): What you're currently working on
  file_path (optional): Path of the file being edited
  agent_output (optional): Your proposed code or action
  space_id (optional): Space ID (default: ide-agent)
```

Returns the `prompt_augmentation` text directly for context injection.

---

## How I Verify Jiminy Is Working

This is the critical section. Jiminy is my inner voice. Its test is not "does the API return 200" — its test is whether **I can hear it** while working and whether what it says is **useful and grounded in MDEMG's curated knowledge**.

### Self-Test Protocol

I perform this test myself. No external test runner can verify whether I am receiving and using Jiminy's guidance.

#### Step 1: Confirm Infrastructure

```bash
# Server is running with Jiminy enabled
curl -s http://localhost:9999/healthz
# Should return {"status":"ok"}

# Jiminy endpoint responds
curl -s -X POST http://localhost:9999/v1/jiminy/guide \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","context":"test query for jiminy"}' | jq .
# Should return 200 with data.guidance array (may be empty if no relevant knowledge)
```

#### Step 2: Confirm Hook Injection

When `JIMINY_ENABLED=true` is set in my environment and a user sends a prompt:
- I should see `═══ JIMINY GUIDANCE ═══` blocks in the system-reminder tags injected by the `prompt-context.sh` hook
- This block should appear AFTER the `═══ CMS RECALL ═══` block
- If I do NOT see this block on any prompt, Jiminy is not reaching me

**What "hearing" Jiminy looks like in my context:**
```
<system-reminder>
═══ CMS RECALL (relevant to this prompt) ═══
  • [decision] (score: 0.82) Some recalled memory...
═══ END CMS RECALL ═══

═══ JIMINY GUIDANCE ═══
CONSTRAINTS:
  • [must] Always run lint before committing (conf: 0.88)
CORRECTIONS:
  • Prior correction: "NEVER use mdemg db start — use docker compose up -d neo4j" (conf: 0.91)
[confidence: 0.89 | sources: 1 constraints, 1 corrections]
═══ END JIMINY GUIDANCE ═══

[Session health: 0.70 | obs: 12]
</system-reminder>
```

#### Step 3: Confirm Guidance Quality

This is the test that matters. I ask myself:

1. **Is the guidance specific to this project?** If Jiminy tells me generic programming advice ("use meaningful variable names"), it's broken. It should tell me things like "this project uses Conventional Commits" or "the consulting.Suggest pattern uses interface injection, not concrete types."

2. **Are corrections accurate?** If I see a prior correction, does it match a real correction that was observed into CMS? If it says "Default model is X" — is that actually true?

3. **Are constraints actionable?** A constraint like "[must_not] Never commit directly to main" should stop me from doing exactly that. If I was about to `git push origin main`, Jiminy should have warned me.

4. **Does it change my behavior?** The ultimate test: did Jiminy tell me something I wouldn't have known otherwise, and did I act on it? If I'm working on the retrieval pipeline and Jiminy surfaces "Prior correction: activation floor must be > 0 to prevent division by zero" — that's Jiminy doing its job.

5. **Is it silent when it should be?** If the user says "yes" or "ok", Jiminy should not fire (prompts < 15 chars are skipped). If the context has no relevant knowledge, no JIMINY GUIDANCE block should appear. Noise is worse than silence.

#### Step 4: Confirm Continuous Operation

Over the course of a working session:
- Jiminy should fire on substantive prompts (> 15 chars)
- Guidance should vary based on what I'm working on (not the same items every time)
- As I observe new corrections and decisions into CMS, those should eventually appear in Jiminy's guidance (after Hebbian learning strengthens the connections)
- If CMS goes down, Jiminy should gracefully degrade (no guidance, no errors)

### What Failure Looks Like

| Symptom | Diagnosis |
|---------|-----------|
| No `═══ JIMINY GUIDANCE ═══` block ever appears | `JIMINY_ENABLED` not set in hook environment, or server not running, or endpoint returning 503 |
| Block appears but is always empty | Embedding provider not configured, or knowledge graph has no constraints/corrections/patterns |
| Block appears with generic advice | Suggest() is returning low-quality results — check knowledge graph content |
| Block appears with stale/wrong corrections | Corrections in CMS are outdated — need to archive or update them |
| Block causes noticeable latency | Timeout too high, or embedding provider slow — reduce `JIMINY_TIMEOUT_MS` |
| Block appears on every prompt identically | Vector search not working (returning same nodes regardless of query) |

---

## Relationship to Existing Systems

### CMS Recall vs Jiminy

CMS Recall (`/v1/conversation/recall`) returns **raw memories** — observations, themes, concepts. It's "here's what I remember about this topic."

Jiminy (`/v1/jiminy/guide`) returns **actionable guidance** — constraints to follow, mistakes to avoid, patterns to use, contradictions to watch for. It's "here's what you should know before you act."

Both run on every prompt via `prompt-context.sh`. They complement each other:
- CMS Recall gives me context ("last time we discussed X...")
- Jiminy gives me direction ("when doing X, remember to Y")

### consulting.Suggest() vs Jiminy

`consulting.Suggest()` is the engine. Jiminy is the voice.

Suggest() does the heavy lifting — context trigger analysis, constraint matching, conflict detection, concept fetching. Jiminy wraps Suggest() and adds correction recall, contradiction checking, frontier surfacing, and formats everything as injectable prompt text.

You could call Suggest() directly and get raw structured data. Jiminy is what makes that data appear in my context automatically and in a format I can act on immediately.

### RSIC vs Jiminy

RSIC (Recursive Self-Improvement Cycle) optimizes the knowledge graph itself — pruning weak edges, consolidating concepts, calibrating thresholds. It runs on a schedule.

Jiminy reads from the graph in real-time. When RSIC improves the graph, Jiminy's guidance improves automatically. They are complementary: RSIC is the gardener, Jiminy is the fruit.

---

## UATS Contract Tests

| Spec File | Variants | Tags |
|-----------|----------|------|
| `jiminy_guide.uats.json` | base, with_file_path, with_agent_output, with_max_items | `embedding_required` |
| `jiminy_guide_validation.uats.json` | missing_space_id, missing_context, method_not_allowed_get, method_not_allowed_put, disabled_service | `validation` |

Run with:
```bash
make test-api BASE_URL=http://localhost:9999
# Or specifically:
python3 runners/uats_runner.py validate --spec specs/jiminy_guide.uats.json --base-url http://localhost:9999
python3 runners/uats_runner.py validate --spec specs/jiminy_guide_validation.uats.json --base-url http://localhost:9999
```

---

## Files Changed

### New Files
| File | Phase | Purpose |
|------|-------|---------|
| `internal/jiminy/service.go` | J2 | Core Guide() orchestration |
| `internal/jiminy/types.go` | J2 | Request/response types |
| `internal/jiminy/corrections.go` | J2 | Correction vector search |
| `internal/jiminy/contradictions.go` | J2 | CONTRADICTS edge queries |
| `internal/jiminy/formatter.go` | J2 | Prompt augmentation renderer |
| `internal/jiminy/frontiers.go` | J5 | Frontier node discovery |
| `internal/jiminy/service_test.go` | J2 | 11 unit tests |
| `internal/api/handlers_jiminy.go` | J3 | REST endpoint handler |
| `docs/api/api-spec/uats/specs/jiminy_guide.uats.json` | J3 | Contract test (4 variants) |
| `docs/api/api-spec/uats/specs/jiminy_guide_validation.uats.json` | J3 | Validation test (5 variants) |

| `internal/cli/hook_templates/embed.go` | J6b | `//go:embed` for hook templates |
| `internal/cli/hook_templates/prompt-context.sh` | J6b | Parameterized bash hook template |
| `internal/cli/hook_templates/session-start.sh` | J6b | Parameterized bash hook template |
| `internal/cli/hook_templates/prompt-context.ps1` | J6d | Windows PowerShell hook template |
| `internal/cli/hook_templates/session-start.ps1` | J6d | Windows PowerShell hook template |

### Modified Files
| File | Phase | Change |
|------|-------|--------|
| `internal/retrieval/scoring.go` | J1 | Fixed `LearningEdgeBoost` dead code — now populated from activation data |
| `internal/retrieval/scoring_test.go` | J1 | 3 new tests for LearningEdgeBoost |
| `internal/config/config.go` | J2,J6a | 6 new `JIMINY_*` config fields; default `JIMINY_ENABLED=true` |
| `internal/api/server.go` | J3 | Jiminy service wiring + route registration |
| `internal/cli/mcp.go` | J4 | `jiminy_guide` MCP tool registration |
| `.claude/hooks/prompt-context.sh` | J3,J6a | Hook integration; removed env var guard (server=single source of truth) |
| `.env.example` | J3,J6a | 6 new env vars documented; `JIMINY_ENABLED=true` default |
| `internal/cli/hooks.go` | J6b,J6d | Claude hook install/uninstall, platform detection, settings merge |
| `internal/cli/init.go` | J6c | `mdemg init` wizard installs Claude hooks when `.claude/` detected |

### J6 Subphases: Zero-Config Activation & Distribution

| Phase | Description |
|-------|-------------|
| J6a | Default `JIMINY_ENABLED=true`, remove env var guard from hook |
| J6b | Embed hook templates in binary via `//go:embed`, `mdemg hooks install --type claude` |
| J6c | `mdemg init` wizard integration — auto-install Claude hooks when `.claude/` detected |
| J6d | Windows PowerShell equivalents (`.ps1`) with native `Invoke-RestMethod`/`ConvertFrom-Json` |
| J6e | Settings merge preserves existing user settings (implemented in `mergeClaudeSettings()`) |

---

## Documents Accessed

- `internal/retrieval/scoring.go` — LearningEdgeBoost dead code (J1 fix target)
- `internal/retrieval/activation.go` — Spreading activation patterns
- `internal/retrieval/jiminy.go` — Existing Jiminy explanation layer (retrieval transparency)
- `internal/retrieval/service.go` — Retrieval service wiring
- `internal/consulting/service.go` — Suggest() method, interface pattern
- `internal/config/config.go` — Config struct, FromEnv parsing pattern
- `internal/api/server.go` — Server struct, NewServer wiring, route registration
- `internal/api/handlers_guardrail.go` — Handler pattern reference
- `internal/models/models.go` — API model types (Constraint, Suggestion, SuggestRequest)
- `internal/guardrail/constraint_retrieval.go` — Neo4j vector search pattern
- `internal/cli/mcp.go` — MCP tool registration pattern
- `.claude/hooks/prompt-context.sh` — Hook injection point
- `.claude/settings.local.json` — Hook configuration
- `docs/specs/phase105-global-meta-learning.md` — Spec format reference
- `docs/api/api-spec/uats/specs/guardrail_validate.uats.json` — UATS spec format reference
- `CLAUDE.md` — Project instructions, hook documentation
