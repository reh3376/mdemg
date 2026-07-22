---
created: 2026-03-30
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "AR-2"
---

# Jiminy Guidance Effectiveness Tracking

## Summary

**Feature**: Jiminy Effectiveness Tracking
**Summary**: Tracks whether Jiminy guidance was followed, ignored, or contradicted, feeding effectiveness metrics into RSIC for self-calibrating guidance.


**Phase AR-2** — Track whether Jiminy guidance was followed, ignored, or contradicted.

## Overview

Jiminy surfaces constraints, corrections, contradictions, and patterns before the agent acts. But without feedback, there's no way to know whether the guidance was useful. Did the agent follow the constraint? Ignore the correction? Contradict the pattern?

Effectiveness tracking closes this loop. Every `Guide()` call now returns a `guidance_id` (CUID2 unique identifier). After the agent acts, the caller sends a feedback request with the `guidance_id` and a summary of what the agent actually did. The system classifies each guidance item's outcome and returns the results.

## How It Works

### Guide → Track → Feedback Flow

```
┌──────────────────┐       ┌──────────────────┐       ┌──────────────────┐
│  1. Guide()      │──────▶│  2. Agent Acts   │──────▶│  3. Feedback     │
│                  │       │                  │       │                  │
│ Returns items +  │       │ Takes action     │       │ POST /feedback   │
│ guidance_id      │       │ based on prompt  │       │ with guidance_id │
│                  │       │                  │       │ + action_summary │
└──────────────────┘       └──────────────────┘       └────────┬─────────┘
                                                               │
                                                               ▼
                                                    ┌──────────────────┐
                                                    │ 4. Classify      │
                                                    │                  │
                                                    │ For each item:   │
                                                    │ followed?        │
                                                    │ ignored?         │
                                                    │ contradicted?    │
                                                    └──────────────────┘
```

### Outcome Classification

For each tracked guidance item, the system classifies the outcome using a 3-tier system based on embedding cosine similarity, LLM judgment, and negation detection:

| Outcome | Meaning | Classification Path |
|---------|---------|-----------|
| `followed` | Agent acted consistently with guidance | Tier 1: similarity >= 0.55 and no negation; or Tier 2: LLM judgment |
| `partial_compliance` | Agent partially addressed guidance | Tier 2: LLM judgment; or Tier 3: heuristic fallback for uncertain range |
| `ignored` | Agent's action shows no evidence of considering guidance | Tier 1: similarity in `[0.10, 0.20)` (relevant domain, JIMINY-CORPUS-001 E4); or Tier 2: LLM semantic judgment |
| `contradicted` | Agent's action directly opposes guidance | Tier 2: LLM judgment (with negation context); or Tier 3: heuristic when negation detected and no LLM |
| `not_applicable` | Guidance topic unrelated to action | Tier 1: similarity < `JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY` (0.10) |
| `unknown` | No action_summary provided or classification failed | Empty input or LLM/parse error |

Negation patterns ("instead of", "did not", "skipped", etc.) are detected but never short-circuit classification — they are passed as context to the LLM Tier 2 for semantic evaluation. The LLM prompt explains that action summaries using `replaced 'OLD' with 'NEW'` format mean OLD was removed, so negation words in OLD text are from deleted code.

### EffectivenessTracker

The tracker is an in-memory LRU cache with TTL expiry:

- **Capacity**: 1000 entries (configurable)
- **TTL**: 2 hours by default (guidance older than this is expired)
- **Thread-safe**: `sync.Mutex` protects all operations
- **LRU eviction**: When at capacity, oldest entries are evicted first
- **Automatic cleanup**: `CleanupExpired()` removes all expired entries
- **Re-registration on cache hits**: When a cached guidance response is served (cache hit in `Guide()`), the tracker re-registers the `guidance_id` so that feedback can still correlate even though no new tracker entry was created
- **Re-registration on `/v1/jiminy/latest` reads**: When the warm store serves guidance via the latest endpoint, the tracker re-registers the `guidance_id` to extend its TTL window
- **Warning log on expired feedback**: When `RecordOutcome()` receives a `guidance_id` that has already expired or was never tracked, a warning-level log is emitted (e.g., `"jiminy: feedback dropped, guidance_id expired"`) before returning `applied: false`

## API Endpoints

### POST /v1/jiminy/guide (updated)

Now returns `guidance_id` in the response:

```bash
curl -s -X POST http://localhost:9999/v1/jiminy/guide \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","context":"Refactoring auth middleware"}' \
  | jq '.data.guidance_id'
# "cm5x7k2j10000jn08h1g2i3j4"
```

### POST /v1/jiminy/feedback (new)

Send feedback after the agent acts:

```bash
curl -s -X POST http://localhost:9999/v1/jiminy/feedback \
  -H "Content-Type: application/json" \
  -d '{
    "guidance_id": "cm5x7k2j10000jn08h1g2i3j4",
    "action_summary": "I validated the input before processing it",
    "space_id": "mdemg-dev"
  }'
```

**Request fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `guidance_id` | string | Yes | The guidance_id from the guide response |
| `action_summary` | string | No | What the agent actually did |
| `space_id` | string | No | Memory space (for context) |

**Response (200):**

```json
{
  "data": {
    "guidance_id": "cm5x7k2j10000jn08h1g2i3j4",
    "applied": true,
    "results": [
      {
        "type": "constraint",
        "content": "[must] Always validate input before processing",
        "outcome": "followed",
        "similarity": 0.42
      },
      {
        "type": "correction",
        "content": "Prior correction: use text-embedding-3-large",
        "outcome": "ignored",
        "similarity": 0.02
      }
    ]
  }
}
```

If the `guidance_id` is unknown (expired or never tracked), the response returns `"applied": false` with an empty `results` array.

**Error responses:** `400` (empty guidance_id), `405` (wrong HTTP method), `503` (Jiminy not enabled).

### Hook Integration (Implemented)

The feedback loop is fully automated via two Claude Code hooks:

**1. Capture (`prompt-context.sh`)** — On each user prompt, the hook calls `GET /v1/jiminy/latest` to retrieve warm guidance. It extracts `.data.guidance_id` and writes a state file:

```bash
# State file: ~/.mdemg/.jiminy-guidance-state
{"guidance_id":"o9vwef0lmljw13f7zgl0qa96","space_id":"mdemg-dev","session_id":"claude-core","ts":1774618498}
```

The guidance response is written to a temp file and parsed with `jq` directly (not via shell variable) because responses contain control characters that corrupt shell variable expansion. A `perl` filter strips U+0000–U+001F before writing.

**2. Feedback (`post-tool-observe.py`)** — After each Write, Edit, or Bash tool execution, the hook:

1. Reads `~/.mdemg/.jiminy-guidance-state`
2. Validates the state is fresh (< 2 hours / `FEEDBACK_STATE_MAX_AGE = 7200`, matching EffectivenessTracker TTL)
3. Checks cooldown (30 seconds between submissions to avoid flooding)
4. Builds a concise action summary from the tool's input/output
5. Fires `POST /v1/jiminy/feedback` with the `guidance_id` and action summary (fire-and-forget via `subprocess.Popen`)

**Action summary format** by tool type:

| Tool | Summary |
|------|---------|
| Write | `"Wrote file: {path}"` |
| Edit | `"Edited {path}: replaced '{old[:80]}' with '{new[:80]}'"` |
| Bash | `"Ran: {cmd[:200]}\nOutput: {out[:200]}"` |

This closes the loop documented in the flow diagram above — `Guide()` → agent acts → `Feedback` → outcome classification → trust update.

See `docs/features/j17-feedback-loop-closure.md` for the full implementation story.

## Configuration

| Parameter | Default | Env Var | Description |
|-----------|---------|---------|-------------|
| JiminyEffectivenessTTLSec | `86400` | `JIMINY_EFFECTIVENESS_TTL_SEC` | TTL for tracked guidance (seconds; raised 7200→86400 by DD-P1P2 — must exceed realistic feedback delay) |

There is no separate enable flag — tracking is unconditional while Jiminy is
enabled (an earlier version of this doc described a `JIMINY_EFFECTIVENESS_ENABLED`
variable that never existed).

## Key Files

| File | Description |
|------|-------------|
| `internal/jiminy/effectiveness.go` | `EffectivenessTracker` — LRU cache with TTL |
| `internal/jiminy/service.go` | `Guide()` — CUID2 ID generation, item tracking; `RecordOutcome()` — feedback processing; `classifyOutcome()` — text overlap scoring |
| `internal/jiminy/types.go` | `GuidanceOutcome`, `GuidanceFeedbackRequest`, `GuidanceFeedbackResponse`, `GuidanceItemFeedback` |
| `internal/api/handlers_jiminy.go` | `handleJiminyFeedback()` — HTTP handler |
| `internal/api/server.go` | Route registration for `/v1/jiminy/feedback` |
| `internal/jiminy/effectiveness_test.go` | 9 unit tests (track/lookup, expiry, eviction, cleanup, classification) |
| `internal/config/config.go` | 2 `JIMINY_EFFECTIVENESS_*` config fields |
| `docs/api/api-spec/uats/specs/jiminy_feedback.uats.json` | 4 UATS contract test variants |
| `tests/integration/autoresearch_test.go` | 4 integration tests (guidance_id, roundtrip, validation, unknown ID) |

## Dependencies

- **Jiminy Service (Phase Jiminy)** — `Guide()` method provides the items to track
- **Consulting Service** — Source of constraint and pattern items
- **Embedding Provider** — Source of correction and frontier items (for guide context)

## Re-baseline + verdict provenance (JIMINY-OUTCOME-002, 2026-06-11)

**History before 2026-06-11T19:00Z is heuristic-dominated and not comparable
to later data.** Until SUPERVISOR-002's feedback-detach fix, 94.9% of Tier-2
outcome-classification LLM calls were context-cancelled by the hook's 5s curl
and silently fell back to the heuristic, which half-credits the uncertain
band as `partial_compliance` — manufacturing a flat ~0.45–0.50 effectiveness.
Post-fix LLM verdicts are honest and the metric dropped to its true range.

Two corrections shipped:
1. **Tier-2 can now say `not_applicable`** (prompts + grammar schema in
   `internal/jiminy/outcome_classifier.go`). Previously the LLM enum had no
   way to say "this guidance doesn't apply to this action," so irrelevant
   guidance (hooks deliver up to 10 items per action) was scored `ignored`,
   structurally depressing effectiveness. `not_applicable` is excluded from
   every sink (escalation, protocol metrics, Neo4j edges, TSDB rows), so
   denominators self-correct for new data. Live-verified: unrelated actions
   now yield `not_applicable` with coherent reasoning; genuinely-applicable
   guidance still classifies `ignored`/`followed`.
2. **`constraint_outcomes.classifier_source`** (V0026): `tier1` | `llm` |
   `heuristic` | `explicit` — fallback-derived rows (the artifact class) are
   forever distinguishable. Historical rows carry `''`.

Effectiveness queries needing the artifact class excluded can filter
`classifier_source <> 'heuristic'` (post-V0026 data only).
