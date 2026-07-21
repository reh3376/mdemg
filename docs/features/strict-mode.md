---
created: 2026-04-12
updated: 2026-04-12
version: v0.8.1
author: reh3376
status: active
phase: "STRICT-P0P1"
---

# /strict Mode — Deterministic Agent Governance

## Summary

**Feature**: /strict Mode
**Summary**: Deterministic enforcement layer for Jiminy guidance that replaces advisory recommendations with imperative directives and blocks Write/Edit operations when escalated constraints are violated.

## Vision & Goals

Jiminy's default advisory mode delivers guidance as contextual suggestions — the agent is free to follow or ignore them. Production data shows a ~20% ignore rate with T1/T2 comprehension at 6.8%/9.0%, meaning the agent fails to even parse most compressed guidance. Training alone caps follow rates at ~75-85%, which is insufficient for safety-critical constraints.

/strict mode exists to close this gap. When active, it transforms guidance delivery from advisory to imperative and enforces compliance through a PreToolUse hook that blocks Write/Edit operations when escalated constraints are violated. The design principle is **graduated enforcement**: new constraints remain advisory (SURFACED level), and only constraints that have been repeatedly ignored (WARNED or higher) trigger blocking. This prevents false-positive blocking while ensuring persistent violations are caught.

### Design Constraints

- **Context-neutral**: /strict reformulated output must be <= current ~430 tokens/prompt (achieved: ~200-350 tokens)
- **Classification latency < 5s**: fits within PreToolUse hook timeout budget
- **No LLM in enforcement path**: reformulation and classification are deterministic (cosine similarity only)
- **Fail-open**: if MDEMG server is unreachable, all operations are allowed — hooks must never block if the server is down

## Current State

### Architecture

/strict mode operates across three layers:

```
                  +--------------------------+
                  |   StrictModeManager      |
                  |   (per-session toggle)   |
                  +------+-------+-----------+
                         |       |
               +---------+       +----------+
               v                            v
    +-------------------+        +-------------------+
    | StrictReformulator |        | StrictClassifier  |
    | (prompt delivery)  |        | (action blocking) |
    +--------+----------+        +--------+----------+
             |                            |
             v                            v
    prompt-context.sh             pre-write-check.py
    (UserPromptSubmit)            (PreToolUse)
```

**StrictModeManager** — In-memory per-session boolean backed by a state file (`~/.mdemg/.jiminy-strict-mode`). The file's existence is the signal hooks check — no HTTP call needed for the fast path.

**StrictReformulator** — Deterministic (no LLM) transformation of guidance items into imperative directives. Filters to items with confidence >= 0.5 or escalation level >= WARNED, sorts by escalation severity, and emits imperative text within a 350-word (~500-token) budget.

**StrictClassifier** — Server-side response classification using Tier 1 evaluation only (vector similarity, no LLM). Implements graduated enforcement: SURFACED constraints always pass, WARNED+ constraints deny on high-severity findings.

**EscalationStore** — Write-behind persistence of escalation state to Neo4j (label `:MemoryNode:J12EscalationState`). Ensures escalation state survives server restarts.

### Workflow

#### Enabling /strict Mode

```
User/Agent → POST /v1/jiminy/strict {"session_id":"claude-core","enabled":true}
                    │
                    ├─ Sets in-memory flag: sessions["claude-core"] = true
                    └─ Writes state file: ~/.mdemg/.jiminy-strict-mode
                       {"session_id":"claude-core","enabled":true,"ts":1744300000}
```

#### Prompt Delivery (Every User Prompt)

```
prompt-context.sh fires (UserPromptSubmit hook)
    │
    ├─ Checks: does ~/.mdemg/.jiminy-strict-mode exist?
    │
    ├─ YES (strict) ──────────────────────────────────────────┐
    │   POST /v1/jiminy/reformulate                           │
    │       │                                                 │
    │       ├─ Calls Guide() internally                       │
    │       ├─ Filters items: confidence >= 0.5 OR level >= WARNED
    │       ├─ Sorts by escalation severity (BLOCKED first)   │
    │       ├─ BLOCKED items → "STOP. Constraint [CODE]..."   │
    │       ├─ Non-blocked → "DIRECTIVE: Comply with..."      │
    │       └─ Caps output at 350 words (~500 tokens)         │
    │                                                         │
    │   Prints directive to stdout → agent context             │
    │                                                         │
    └─ NO (advisory) ────────────────────────────────────────┐
        GET /v1/jiminy/latest (cached, <100ms)               │
        Prints advisory guidance to stdout                    │
```

#### Action Enforcement (Every Write/Edit)

```
pre-write-check.py fires (PreToolUse hook)
    │
    ├─ Checks: does ~/.mdemg/.jiminy-strict-mode exist?
    │   NO → exit 0 (allow, zero latency)
    │
    ├─ Extracts agent_output from tool_input (truncated to 2000 chars)
    │
    ├─ POST /v1/jiminy/classify (5s timeout)
    │       │
    │       ├─ Evaluator.Evaluate() — Tier 1 only (cosine similarity)
    │       ├─ For each high-severity finding:
    │       │   ├─ Look up escalation level for that constraint
    │       │   ├─ SURFACED → pass (first-time, advisory only)
    │       │   └─ WARNED / ESCALATED / BLOCKED → deny
    │       │
    │       └─ Returns: {verdict: "pass"|"deny", denial_reason, violated_codes}
    │
    ├─ verdict == "pass" → exit 0 (allow)
    ├─ verdict == "deny" → emit permissionDecision: "deny" + reason
    └─ ANY error (timeout, connection refused, etc.) → exit 0 (fail-open)
```

### Escalation State Machine

Constraints progress through escalation levels based on agent behavior:

```
INACTIVE ──(surfaced)──→ SURFACED ──(ignored x2)──→ WARNED ──(ignored x4)──→ ESCALATED ──(ignored x6)──→ BLOCKED
    ↑                        │                         │                        │                         │
    │                        │                         │                        │                         │
    └────────────(followed at any level)───────────────┴────────────────────────┴─────────→ RESOLVED ─────┘
                                                                                              │
                                                                                   (IgnoreCount reset to 0)
```

| Level | Ignore Count | /strict Behavior | Directive Prefix |
|-------|-------------|------------------|-----------------|
| SURFACED | 0 | Advisory (pass) | `- ` |
| WARNED | >= 2 | **Blocking** | `REQUIRED: ` |
| ESCALATED | >= 4 | **Blocking** | `MUST: ` |
| BLOCKED | >= 6 | **Blocking** | `STOP. Constraint [CODE]...` |
| RESOLVED | (reset) | Pass | N/A |

Escalation decays after 60 minutes of inactivity (configurable via `JIMINY_ESCALATION_DECAY_MINUTES`). State is persisted to Neo4j and survives server restarts.

### Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `JIMINY_ESCALATION_PERSIST_ENABLED` | `true` | Persist escalation state to Neo4j |
| `JIMINY_STRICT_STATE_PATH` | `~/.mdemg/.jiminy-strict-mode` | Path to strict mode state file |
| `J17_T1_COMPREHENSION_GATE` | `0.5` | Minimum T1 follow rate before downgrading to T2 |
| `J17_TIER_GATE_MODE` | `trust` | Tier-gate axis: `trust` (legacy compliance-EMA) or `comprehension` (T1 promotion keyed on measured comprehension) |
| `JIMINY_ESCALATION_WARN_AFTER` | `2` | Ignore count to trigger WARNED |
| `JIMINY_ESCALATION_ESCALATE_AFTER` | `4` | Ignore count to trigger ESCALATED |
| `JIMINY_ESCALATION_BLOCK_AFTER` | `6` | Ignore count to trigger BLOCKED |
| `JIMINY_ESCALATION_DECAY_MINUTES` | `60` | TTL before escalation state resets |
| `JIMINY_ESCALATION_BLOCK_ENABLED` | `false` | Whether BLOCKED level is reachable |

## Usage

### Enabling /strict Mode

Via API:
```bash
curl -X POST http://localhost:9999/v1/jiminy/strict \
  -H "Content-Type: application/json" \
  -d '{"session_id":"claude-core","enabled":true}'
```

Response:
```json
{"data":{"session_id":"claude-core","strict":true,"message":"strict mode enabled"}}
```

### Disabling /strict Mode

```bash
curl -X POST http://localhost:9999/v1/jiminy/strict \
  -H "Content-Type: application/json" \
  -d '{"session_id":"claude-core","enabled":false}'
```

### Checking Status

The state file at `~/.mdemg/.jiminy-strict-mode` is the canonical indicator. Its presence means strict mode is active. Hooks check this file directly without HTTP.

### What the Agent Sees

**Advisory mode (default):**
```
═══ JIMINY GUIDANCE ═══
Consider using environment variables instead of hardcoded connection strings.
Severity: medium | Source: correction-2026-04-01
═══ END ═══
```

**Strict mode:**
```
DIRECTIVE: Comply with the following before proceeding:
REQUIRED: [CFG001] Use environment variables for all connection parameters — hardcoded values caused OOM in production.
- Prefer configuration structs over inline defaults for pool sizing.
```

**Strict mode with BLOCKED constraint:**
```
STOP. Constraint [SEC003]: Never commit API keys or credentials to source files. Do not proceed until resolved.
```

### What Happens on Denial

When the PreToolUse hook denies a Write/Edit:

1. The agent receives a denial message: `[/strict] ESCALATED — constraint [CODE] violated: <reason>`
2. The Write/Edit operation is cancelled — no file changes occur
3. The agent must address the constraint before retrying
4. The constraint's escalation level may increase if the agent retries without addressing it

## Notes

### Known Limitations

- **Single-session file persistence**: Only the most recently enabled session's state is written to the state file. Multiple concurrent strict-mode sessions are tracked in memory only.
- **Tier 1 classification only**: The classifier uses cosine similarity, not LLM evaluation. This keeps latency under 5s but may miss nuanced violations that require semantic understanding.
- **Agent output truncation**: The PreToolUse hook truncates agent output to 2000 characters for classification. Very large Write operations may have relevant content beyond the truncation point.
- **No PostToolUse enforcement**: /strict only gates Write/Edit via PreToolUse. It does not inspect tool results or Bash output after execution.

### Risks & Gaps

| Risk | Mitigation |
|------|------------|
| False-positive blocking on new constraints | Graduated enforcement: SURFACED always passes. Requires 2+ ignores before blocking. |
| Classification latency exceeding hook timeout | Tier 1 only (no LLM). 5s context deadline on classify endpoint. |
| Stale strict-mode file after server crash | Hooks fail-open when server unreachable. Server startup can clean orphans. |
| Agent circumventing /strict via Bash | Only Write/Edit are gated. Bash `echo >` or `sed` are not blocked (pre-bash-check.py handles destructive ops separately). |

### Future Improvements

- **Phase 3**: LLM-based reformulation — use training data from /strict to build an LLM reformulator that produces higher-quality directives
- **Phase 4**: DPO training on compliance data — use /strict deny/pass outcomes as preference pairs for model fine-tuning
- **PostToolUse classification**: Inspect tool results for constraint violations after execution
- **Multi-session file persistence**: Support concurrent strict-mode sessions in the state file
- **Bash enforcement**: Extend PreToolUse hook to gate Bash commands that write files (e.g., `echo >`, `cat <<EOF >`)

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/jiminy/strict` | Toggle strict mode on/off for a session |
| POST | `/v1/jiminy/reformulate` | Generate imperative directives from guidance items |
| POST | `/v1/jiminy/classify` | Classify agent output as pass/deny against active constraints |

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `JIMINY_ESCALATION_PERSIST_ENABLED` | `true` | Persist escalation state to Neo4j (write-behind) |
| `JIMINY_STRICT_STATE_PATH` | `~/.mdemg/.jiminy-strict-mode` | Strict mode state file location |
| `J17_T1_COMPREHENSION_GATE` | `0.5` | T1 follow rate threshold — below this, T1 downgrades to T2 |
| `JIMINY_ESCALATION_WARN_AFTER` | `2` | Ignores before WARNED level |
| `JIMINY_ESCALATION_ESCALATE_AFTER` | `4` | Ignores before ESCALATED level |
| `JIMINY_ESCALATION_BLOCK_AFTER` | `6` | Ignores before BLOCKED level |
| `JIMINY_ESCALATION_DECAY_MINUTES` | `60` | Escalation state TTL (minutes) |
| `JIMINY_ESCALATION_BLOCK_ENABLED` | `false` | Enable BLOCKED level (if false, max is ESCALATED) |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Jiminy Inner Voice | requires — /strict extends Jiminy's guidance pipeline |
| Escalation Tracking | requires — graduated enforcement depends on escalation state machine |
| J17 Protocol | enhances — T1/T2 comprehension gate improves compressed guidance delivery |
| Trust Scoring | feeds-into — trust levels influence tier selection and escalation thresholds |
| PreToolUse Hooks | requires — Write/Edit blocking relies on Claude Code hook infrastructure |
| Neo4j | requires — escalation persistence uses J12EscalationState nodes |
| TSDB | feeds-into — constraint_outcomes provide training signal for future DPO |

## Context Window Impact

| Metric | Advisory Mode | /strict Mode | Delta |
|--------|--------------|-------------|-------|
| Per-prompt overhead | ~430 tokens | ~200-350 tokens | **-80 to -230 tokens** |
| Classification cost | 0 | 0 (server-side) | **neutral** |
| Denial context (rare) | 0 | ~75 tokens | **+75 tokens** |
| Compaction survival | ~20% | ~65% (state persisted) | **+45%** |

## Related Files

- `internal/jiminy/strict_mode.go` — StrictModeManager (toggle, state file I/O)
- `internal/jiminy/reformulator.go` — StrictReformulator (deterministic directive generation)
- `internal/jiminy/strict_classifier.go` — StrictClassifier (graduated enforcement logic)
- `internal/jiminy/escalation.go` — EscalationTracker (state machine, decay, apply)
- `internal/jiminy/escalation_store.go` — EscalationStore (Neo4j write-behind persistence)
- `internal/jiminy/types.go` — Type definitions (ClassifyRequest/Response, EscalationLevel, etc.)
- `internal/api/handlers_jiminy.go` — HTTP handlers for /strict, /reformulate, /classify
- `.claude/hooks/pre-write-check.py` — PreToolUse enforcement hook
- `.claude/hooks/prompt-context.sh` — Guidance delivery hook (strict vs advisory branch)
- `.claude/settings.local.json` — Hook registration for Write/Edit matchers
