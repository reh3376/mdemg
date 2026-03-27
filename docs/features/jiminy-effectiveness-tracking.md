# Jiminy Guidance Effectiveness Tracking

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

For each tracked guidance item, the system compares the agent's `action_summary` against the item's content using text overlap scoring with negation detection:

| Outcome | Meaning | Detection |
|---------|---------|-----------|
| `followed` | Agent acted consistently with guidance | Overlap > 0.3 AND no negation detected |
| `contradicted` | Agent did the opposite of guidance | Overlap > 0.15 AND negation words present ("not", "don't", "never", "without", "removed", "deleted") |
| `ignored` | Agent's action is unrelated | Overlap ≤ 0.15 |
| `unknown` | No action_summary provided | Empty or missing action_summary |

The overlap score is calculated from significant words (4+ characters) shared between the guidance content and the action summary, normalized by total content words.

### EffectivenessTracker

The tracker is an in-memory LRU cache with TTL expiry:

- **Capacity**: 1000 entries (configurable)
- **TTL**: 30 minutes by default (guidance older than this is expired)
- **Thread-safe**: `sync.Mutex` protects all operations
- **LRU eviction**: When at capacity, oldest entries are evicted first
- **Automatic cleanup**: `CleanupExpired()` removes all expired entries

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
2. Validates the state is fresh (< 30 minutes, matching EffectivenessTracker TTL)
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
| JiminyEffectivenessEnabled | `true` | `JIMINY_EFFECTIVENESS_ENABLED` | Enable/disable effectiveness tracking |
| JiminyEffectivenessTTLSec | `1800` | `JIMINY_EFFECTIVENESS_TTL_SEC` | TTL for tracked guidance (seconds) |

When disabled, `Guide()` still works normally but doesn't generate `guidance_id` or track items.

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
