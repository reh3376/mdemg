---
created: 2026-03-30
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "J17-FL"
---

# J17 Feedback Loop Closure

## Summary

**Feature**: J17 Feedback Loop Closure
**Summary**: Fixes 3 independent failures in the J17 protocol feedback loop — Jiminy evaluate, TSDB recording, and trust score persistence — restoring the complete guidance-feedback-learning cycle.


**Phase**: J17 Feedback Loop Fix
**Status**: Complete
**Date**: 2026-03-27

---

## 1. Overview

The J17 protocol's tier graduation mechanism was fully implemented server-side but **never activated** because the feedback loop between hooks and the server was completely broken at the triggering layer. Guidance was generated, delivered to the agent, but no feedback ever flowed back — so trust never changed, comprehension was never measured, and the protocol remained stuck at T3 (full natural language) indefinitely.

This fix closes the loop by:

1. **Capturing `guidance_id`** from `GET /v1/jiminy/latest` in `prompt-context.sh` and persisting it to a state file
2. **Sending feedback** from `post-tool-observe.py` after tool execution, correlating agent actions with the last guidance
3. **Sanitizing control characters** in guidance responses that broke JSON parsing in the hooks
4. **Bootstrapping codification** on session start when code coverage is 0% but events exist

### The Problem

Three independent failures prevented tier graduation:

| Failure | Impact |
|---------|--------|
| No automated feedback delivery | `POST /v1/jiminy/feedback` handler exists but nothing ever calls it. `prompt-context.sh` extracted guidance text but discarded `guidance_id`. `post-tool-observe.py` captured tool observations but never correlated them with prior guidance. |
| Trust stuck at 0.5 | T1 requires trust > 0.8. Trust only updates via `RecordOutcome()` (called by the feedback endpoint). Since feedback never fires, trust never changes. 6 positive feedbacks would reach 0.8. |
| No constraint codes | T1 requires `ConstraintCode`. RSIC cold-start insight exists but never triggered because `TotalEvents` was 0 (fixed in prior commit via cache hit metrics recording). Even after that fix, RSIC needs to run a codification cycle. |

### What Was NOT Broken

The entire server-side pipeline was already correct:

- `RecordOutcome()` (`service.go`) — processes feedback, updates trust, records protocol metrics
- `TrustScorer` (`trust.go`) — initial=0.65, boost +0.05/follow, decay -0.02/ignore, -0.04/contradict (current defaults)
- `selectTier()` (`encoder.go`) — trust-based tier selection with code/annotation requirements
- `CodifyConstraint()` (`protocol_evolution.go`) — LLM + hash fallback code generation
- RSIC `j17_cold_start_codification` insight (`self_reflect.go`) — detects codification opportunity

---

## 2. Architecture

### Data Flow

```
prompt-context.sh                              post-tool-observe.py
GET /v1/jiminy/latest                          (tool completes)
  → perl sanitize control chars                → read ~/.mdemg/.jiminy-guidance-state
  → write to temp file                         → validate age < 2 hours
  → jq extract .data.guidance_id               → check cooldown (30s)
  → write state file                           → build action_summary
                                               → POST /v1/jiminy/feedback (fire-and-forget)
                                                       ↓
                                               RecordOutcome() [service.go]
                                                       ↓
                                               TrustScorer.RecordOutcome() → trust += 0.05
                                                       ↓
                                               Next Guide() → selectTier(trust=0.55)
                                                       ↓
                                               ... 6 rounds later: trust=0.8 → T1 eligible
```

### State File

**Path**: `~/.mdemg/.jiminy-guidance-state`

```json
{
  "guidance_id": "o9vwef0lmljw13f7zgl0qa96",
  "space_id": "mdemg-dev",
  "session_id": "claude-core",
  "ts": 1774618498
}
```

Written by `prompt-context.sh` whenever a valid `guidance_id` is present (the previous `warm=true` gate was removed — any non-empty guidance_id is now captured). When no guidance_id is present, stale state files are cleared to prevent feedback from correlating with expired guidance. Read by `post-tool-observe.py` before sending feedback. The `ts` field enables age validation — state older than 2 hours (matching `EffectivenessTracker` TTL) is discarded.

### Cooldown File

**Path**: `~/.mdemg/.jiminy-last-feedback`

Touched (mtime updated) after each feedback submission. The 30-second cooldown prevents flooding the feedback endpoint during rapid tool sequences — one representative action summary per cooldown window is sufficient for trust scoring.

---

## 3. Control Character Sanitization

### The Bug

Guidance responses from `/v1/jiminy/latest` contain LLM-generated text with JSON control characters (U+0000–U+001F) embedded in string values. These are valid in Go's `json.Marshal` output but invalid in strict JSON parsers like `jq`.

The original `prompt-context.sh` stored the response in a shell variable (`GUIDANCE=$(curl ...)`), then piped it through `echo "$GUIDANCE" | jq`. This double-failed:

1. `jq` rejects control characters in JSON strings
2. Shell variable expansion + `echo` corrupts multi-byte UTF-8 sequences containing control-char-adjacent bytes

### The Fix

Write directly to a temp file with inline `perl` sanitization, then parse from the file:

```bash
GUIDANCE_TMP=$(mktemp /tmp/jiminy-guidance-XXXXXX.json)
curl -sf "${MDEMG_URL}/v1/jiminy/latest?space_id=${SPACE_ID}" \
  --connect-timeout 1 --max-time 2 | \
  perl -pe 's/[\x00-\x08\x0b\x0c\x0e-\x1f]//g' > "$GUIDANCE_TMP"

# Parse directly from file — no shell variable corruption
GUIDANCE_ID=$(jq -r '.data.guidance_id // empty' "$GUIDANCE_TMP")
```

The `perl` regex strips all control characters except tab (0x09), newline (0x0a), and carriage return (0x0d).

---

## 4. Bootstrap Codification

### The Problem

T1 encoding requires `ConstraintCode` values on constraint nodes. These codes are generated by RSIC's `CodifyConstraint()` action, which is triggered by the `j17_cold_start_codification` reflection insight. But this insight only fires when `TotalEvents > 0` and `code_coverage == 0` — a condition that was never met because cache hit metrics weren't being recorded (fixed separately).

Even after that fix, RSIC needs to actually run a cycle to detect the codification opportunity and execute it.

### The Fix

`session-start.sh` now checks protocol metrics on startup and triggers a codification cycle when:

- J17 is enabled (detected by querying `/v1/jiminy/ready` for `features.j17 == true`)
- `code_coverage == 0` (no constraint codes exist)
- `total_events > 0` (the protocol has recorded events)

```bash
J17_ON=$(curl -sf "${MDEMG_URL}/v1/jiminy/ready" --connect-timeout 1 --max-time 2 \
  | jq -r '.features.j17 // false')
if [ "$J17_ON" = "true" ]; then
  PROTO_METRICS=$(curl -sf "${MDEMG_URL}/v1/jiminy/protocol/metrics" ...)
  CODE_COV=$(echo "$PROTO_METRICS" | jq -r '.data.code_coverage // -1')
  TOTAL_EVT=$(echo "$PROTO_METRICS" | jq -r '.data.total_events // 0')
  if [ "$CODE_COV" = "0" ] && [ "$TOTAL_EVT" -gt "0" ]; then
    curl -sf -X POST "${MDEMG_URL}/v1/self-improve/cycle" \
      -d '{"space_id":"...","tier":"meso","trigger_source":"j17-cold-start-bootstrap"}' &
  fi
fi
```

> **Note:** Shell hooks (`session-start.sh`, `pre-compact.sh`) previously gated J17 logic on `${J17_ENABLED:-false}`, but this env var was never set because hooks don't source `.env`. The `/v1/jiminy/ready` endpoint provides a reliable runtime check.

This is **self-disabling**: once codes are generated, `code_coverage > 0` and the condition no longer matches.

---

## 5. Trust Graduation Path

With the feedback loop closed, tier graduation follows this progression:

| Session | Trust Score | Tier | Encoding |
|---------|-------------|------|----------|
| 1 (start) | 0.65 | T3 (T2 if codes exist) | Full natural language (~80 tokens/item) |
| 1 (after 1 feedback) | 0.70 | T2 (with codes) | Telegraphic (~50 tokens/item) |
| 1 (after 2 feedbacks) | 0.75 | T1 eligible | Coded (~15 tokens/item) if codes exist |
| 1+ (codes generated) | 0.75+ | T1 | Full compression active |

**Math**: `(0.75 - 0.65) / 0.05 = 2` consecutive positive feedbacks to reach the T1 trust threshold (current defaults: initial 0.65, boost +0.05, `J17_TRUST_HIGH_THRESHOLD` 0.75; this is the legacy ratchet arithmetic — under the default EMA trust mode the exact count varies).

Trust is per-session and in-memory (4-hour TTL), so graduation happens within a session, not across sessions. The signed session ticket mechanism (`ticket.go`) persists trust across context compactions within a session. Additionally, trust now persists durably to Neo4j via `TrustStore`, so the ticket-based checkpoint/resume is supplemented by durable persistence — trust can survive full session restarts, not just compactions.

---

## 6. Configuration

No new configuration variables. The fix uses existing config:

| Parameter | Value | Source |
|-----------|-------|--------|
| `J17_TRUST_INITIAL` | 0.65 | Trust starting point |
| `J17_TRUST_BOOST_PER_FOLLOW` | 0.05 | Per-feedback trust increase |
| `J17_TRUST_HIGH_THRESHOLD` | 0.75 | T1 eligibility threshold |
| `JIMINY_EFFECTIVENESS_TTL_SEC` | 7200 | State file max age (2 hours) |
| `GET /v1/jiminy/ready` | `features.j17` | Bootstrap codification gate (replaces `J17_ENABLED` env var) |

New constants in `post-tool-observe.py`:

| Constant | Value | Purpose |
|----------|-------|---------|
| `FEEDBACK_COOLDOWN_SEC` | 30 | Minimum seconds between feedback submissions |
| `FEEDBACK_STATE_MAX_AGE` | 7200 | Maximum age of state file before discard (2 hours) |

---

## 7. Key Files

| File | Action | Description |
|------|--------|-------------|
| `.claude/hooks/prompt-context.sh` | Modified | File-based JSON parsing, perl sanitization, guidance_id capture, state file write |
| `.claude/hooks/post-tool-observe.py` | Modified | 4 new functions: `_feedback_cooled_down()`, `_mark_feedback_sent()`, `_build_action_summary()`, `send_jiminy_feedback()` |
| `.claude/hooks/session-start.sh` | Modified | Bootstrap codification trigger when code_coverage=0 |
| `internal/jiminy/service.go` | Modified | `recordCacheHitMetrics()` — records J17 protocol metrics on cache hits (prerequisite fix) |
| `internal/api/server.go` | Modified | Bootstrap RSIC assessment goroutine (prerequisite fix) |
| `internal/jiminy/service_cache_metrics_test.go` | New | 10 unit tests for cache hit metrics |
| `tests/integration/j17_feedback_loop_test.go` | New | 3 integration tests: feedback updates metrics, endpoint returns OK, `/v1/metrics/snapshot` has J17 metrics (originally written against the pre-2026-03-28 Prometheus endpoint; migrated to the JSON snapshot) |
| `tests/integration/j17_metrics_test.go` | Modified | Fixed metric name assertion (`j17_events_total` not `j17_total_events`) |

## 8. Dependencies

- **Jiminy Service (Phase Jiminy)** — `Guide()` provides guidance items; `RecordOutcome()` processes feedback
- **EffectivenessTracker (Phase AR-2)** — LRU cache correlating guidance_id to items (2-hour TTL)
- **TrustScorer (Phase J17-3)** — Per-session trust scoring updated by feedback outcomes
- **TrustStore (Neo4j)** — Durable trust persistence supplementing ticket-based checkpoint/resume
- **RSIC (Phase 60b)** — Codification cycle triggered by bootstrap logic
- **Protocol Metrics (Phase J17-4)** — `RecordGuidance()` and `RecordConstraintCoverage()` for protocol health scoring

## Documents Accessed

- `.claude/hooks/prompt-context.sh` (modified)
- `.claude/hooks/post-tool-observe.py` (modified)
- `.claude/hooks/session-start.sh` (modified)
- `internal/jiminy/service.go` (modified)
- `internal/jiminy/encoder.go` (read)
- `internal/jiminy/trust.go` (read)
- `internal/jiminy/protocol_evolution.go` (read)
- `internal/jiminy/protocol_metrics.go` (read)
- `internal/api/server.go` (modified)
- `internal/api/handlers_jiminy.go` (read)
- `internal/ape/self_reflect.go` (read)
- `internal/ape/self_assess.go` (read)
- `tests/integration/j17_feedback_loop_test.go` (new)
- `tests/integration/j17_metrics_test.go` (modified)
- `internal/jiminy/service_cache_metrics_test.go` (new)
