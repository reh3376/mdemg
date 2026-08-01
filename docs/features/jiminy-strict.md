# Jiminy /strict Mode — Enforcement of Hard Constraints

**Purpose:** Force the stateless + probabilistic nature of an LLM into a stateful + deterministic set of behaviors by BLOCKING Write/Edit tool calls that violate escalated constraints AND emitting user-visible alerts.

**Ships DEFAULT-ON** as of JIMINY-ENFORCE-001 (2026-08-01) per operator directive: Jiminy is an ENFORCER, not merely an advisor.

## Why

The stateless nature of an LLM means every response is a probabilistic sample from a distribution — the model has no memory of its own prior commitments, corrections, or hard rules across turns. MDEMG's substrate (CMS + Jiminy + RSIC) exists to make that stateless behavior effectively stateful and deterministic where it matters most. `/strict` is the enforcement seam:

- **Advisory alone is insufficient.** Even with 100% surfacing of a "never commit to main" constraint, an advisory hook doesn't prevent the violation — it only records the outcome after the fact.
- **Enforcement without alerts is opaque.** A silent block would leave the operator unaware that a real violation was prevented (or that the enforcement gate is misfiring).
- **Fail-open without warning is silent-failure.** If MDEMG is unreachable, the tool must not deadlock — but the operator MUST know that the enforcement guarantee is temporarily off.

## How it works

### Enforcement path (PreToolUse:Write / PreToolUse:Edit)

```
Tool call → Claude Code invokes .claude/hooks/pre-write-check.py
         → if state file ~/.mdemg/.jiminy-strict-mode absent → allow (strict mode off)
         → POST /v1/jiminy/classify {agent_output, file_path, tool_name, session_id}
           → server evaluates via StrictClassifier:
             1. Evaluator finds high-severity constraint matches
             2. Escalation manager checks per-session state
             3. WARNED+ → deny; else pass
           → HIGH-severity alert dispatched on deny (server-side)
         → hook exits deny → tool call blocked
```

### Fail-open path (MDEMG server unreachable)

```
Tool call → hook → POST /v1/jiminy/classify → URLError / Timeout / etc
         → stderr WARN: "⚠️  JIMINY ENFORCEMENT SUSPENDED (…): action allowed…"
         → write ~/.mdemg/.jiminy-server-unreachable {reason, url, ts}
         → hook exits 0 → tool call ALLOWED
         → next prompt-context hook fire displays the persistent warning
         → next successful classify call clears the marker
```

### Default-on flow

```
mdemg start
  → StrictModeManager.LoadFromFile() (restores prior state)
  → if JIMINY_STRICT_DEFAULT_ENABLED=true AND state absent for the default session:
    → StrictModeManager.Enable("claude-core") → writes state file
    → INFO log: jiminy: strict mode auto-enabled ...
```

## Configuration

| Env var | Default | Meaning |
|---|---|---|
| `JIMINY_STRICT_DEFAULT_ENABLED` | `false` (code); `true` (mdemg-dev `.env`) | Auto-enable strict mode at boot |
| `JIMINY_STRICT_DEFAULT_SESSION_ID` | `claude-core` | Session key auto-enabled at boot |
| `JIMINY_STRICT_STATE_PATH` | `~/.mdemg/.jiminy-strict-mode` | Path to strict-mode state file |
| `JIMINY_ESCALATION_WARN_AFTER` | (see config) | Ignoreds before a constraint escalates to WARNED (the level that triggers deny) |

## API surface

- `POST /v1/jiminy/strict` `{"session_id": "…", "enabled": true|false}` — toggle strict mode per session
- `POST /v1/jiminy/classify` `{"space_id", "session_id", "agent_output", "tool_name", "file_path"}` — the classify endpoint the hook calls. Returns `{"verdict": "pass"|"deny", "denial_reason", "violated_codes", "escalation_level"}`
- Server dispatches HIGH-severity alert on `verdict=deny` — no client-side action required

## Fail-open behavior

Fail-open is intentional (operator-confirmed policy 2026-08-01):
- If the MDEMG server is unreachable, tool calls MUST proceed — the alternative deadlocks the operator with no path to recover.
- BUT every fail-open leaves a visible trail: stderr WARN on the tool call + persistent marker file + prompt-context surfacing until cleared.
- The marker clears automatically on the next successful classify call.

## Coverage

Currently: `Write` + `Edit` PreToolUse hook only.
Extending to `Bash` is JIMINY-ENFORCE-002 (sequential arc; design decisions surface after JIMINY-ENFORCE-001's live verification).

## Observability

- Startup log: `jiminy: strict mode auto-enabled (JIMINY_STRICT_DEFAULT_ENABLED) session_id=claude-core`
- Block alert: `[HIGH] jiminy-block: Jiminy blocked action — <reason> (file: <path>, tool: <tool>)`
- Fail-open marker: `~/.mdemg/.jiminy-server-unreachable` (JSON with reason + url + ts)
- Prompt-context warning: `⚠️  JIMINY ENFORCEMENT DEGRADED — server was unreachable at <time>: <reason>. Marker clears on next successful pre-write-check.`

## Related sprints + arc

The JIMINY-ENFORCE-* arc (5 sprints, sequential):

1. **JIMINY-ENFORCE-001** — this sprint (foundation: default-on + alert-on-block + fail-open-with-warning)
2. **JIMINY-ENFORCE-002** — Bash coverage
3. **JIMINY-ENFORCE-003** — Override CLI + audit trail
4. **JIMINY-ENFORCE-004** — RSIC enforcement-learning (new outcome types + patterns)
5. **JIMINY-ENFORCE-005** — Post-hoc missed-violation detector

Prior/related:
- JIMINY-CORPUS-001/-002 (corpus quality — constraint content Jiminy enforces)
- JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 (classifier semantics)
- LEVER-C-TIGHTEN-001 (surface composition tuning)
