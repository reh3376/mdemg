# JIMINY-ENFORCE-001 — Sprint Post

**Date:** 2026-08-01 | **Branch:** `reh3376_dev01`
**Arc:** JIMINY-ENFORCE (sprint 1 of 5; operator-directed 2026-08-01)
**Trigger:** Operator architectural directive: Jiminy's purpose is to ENFORCE hard constraints on the main LLM's behavior, not merely advise. When the main LLM violates a Jiminy constraint the user MUST be alerted. Jiminy's overarching purpose is to force the stateless + probabilistic nature of an LLM into a stateful + deterministic set of behaviors. The current default-advisory + opt-in `/strict` architecture was MISALIGNED with this intent.

## Verdict

**Shipped.** Strict mode is now enabled by default at boot; every deny emits a HIGH-severity alert; fail-open policy stays (operator-confirmed) but now surfaces a persistent warning until the server is reachable again. Live Tier-3 verified end-to-end.

## What shipped

### E1 — Strict-mode default-on
- `internal/config/config.go`: 2 new knobs
  - `JIMINY_STRICT_DEFAULT_ENABLED` (bool, default false in code — behavior-changing flag pattern per HEBB-ETA-001 rule)
  - `JIMINY_STRICT_DEFAULT_SESSION_ID` (string, default "claude-core")
- `.env`: `JIMINY_STRICT_DEFAULT_ENABLED=true` + `JIMINY_STRICT_DEFAULT_SESSION_ID=claude-core` (live on `mdemg-dev`)
- `internal/jiminy/service.go`: after `StrictModeManager.LoadFromFile()`, auto-enable if flag on AND session not already active (idempotent). Startup log: `jiminy: strict mode auto-enabled (JIMINY_STRICT_DEFAULT_ENABLED) session_id=claude-core`.

### E2 — Alert-on-block (server-side)
- `internal/api/handlers_jiminy.go`: extracted alert emission into narrow helper `emitJiminyBlockAlert(ctx, dispatcher, req, resp)` for testability. Called from `handleJiminyClassify` after the LLM verdict. On `resp.Verdict == "deny"`:
  - dispatches HIGH-severity alert via `alertDispatcher.Send()`
  - Service label: `jiminy-block` (distinct per NOSILENT-001 cooldown-key rule)
  - Title: `"Jiminy blocked action"`
  - Message: `"<denial_reason> (file: <path>, tool: <tool>)"`
- Auto-clears via the shipped `POST /v1/alerts/clear` flow when the next hook fires.
- New `alertSender` interface (`Send(ctx, Alert)`) keeps handler-side dispatcher decoupled from `alert.Dispatcher` concrete for testability.

### E3 — Fail-open-with-persistent-warning
- `internal/cli/hook_templates/pre-write-check.py`:
  - Every fail-open branch (URLError / TimeoutError / OSError / JSONDecodeError) now:
    - Writes a stderr WARN naming the URL + reason
    - Writes/updates `~/.mdemg/.jiminy-server-unreachable` (JSON with reason + url + timestamp)
  - Successful classify call clears the marker
- `internal/cli/hook_templates/prompt-context.sh`: after the `/healthz` check, if marker exists, display a prominent warning line naming the marker's timestamp + reason. Persistent — appears every prompt until cleared.

### Tests
- 4 new unit tests in `internal/api/handlers_jiminy_test.go`:
  - `TestEmitJiminyBlockAlert_DenyFiresHighAlert` — pins the enforcement contract (deny → HIGH alert with correct service, severity, message)
  - `TestEmitJiminyBlockAlert_PassIsNoOp` — allowed action never emits an alert
  - `TestEmitJiminyBlockAlert_NilDispatcherIsSafe` — no panic when dispatcher is nil (defensive)
  - `TestEmitJiminyBlockAlert_EmptyReasonUsesFallback` — deny with no reason still gets a meaningful message

## Live Tier-3 (mdemg-dev)

### E1 — startup log
```
level=INFO msg="jiminy: strict mode auto-enabled (JIMINY_STRICT_DEFAULT_ENABLED)" session_id=claude-core
```
State file confirmed present + valid: `{"session_id":"claude-core","enabled":true,"ts":1785608016}`

### E2 — unit-tested (4/4 pass)
- No naturally-occurring WARNED escalation exists on `mdemg-dev` today for any constraint (`MATCH (e:J12EscalationState) WHERE e.level IN ['warned','escalated','blocked']` returns 0 rows). A deep-dive into why the shipped J13/J15 escalation machinery hasn't produced any WARNED states in practice is a candidate for the JIMINY-ENFORCE-004 (RSIC-enforcement-learning) sprint.
- Wire is unit-test-proven: 4 tests exercise the emit function against a `mockAlertSender`, verifying the alert has the correct service, severity, message shape, and no-op for pass/nil-dispatcher.
- The `Send()` path itself is proven by many other alerts flowing through the same dispatcher (`current.json` shows MEDIUM error-rate, LOW cache-hit-ratio, HIGH health-probe transitions all landing correctly).

### E3 — fail-open + marker + clear (all live)
```
# 1. Server unreachable → fail-open + marker created + stderr WARN
$ MDEMG_URL=http://127.0.0.1:1 hook <<< '{"tool_name":"Write",...}'
STDERR: ⚠️  JIMINY ENFORCEMENT SUSPENDED (MDEMG server unreachable at http://127.0.0.1:1/v1/jiminy/classify): URLError: <urlopen error [Errno 61] Connection refused>. Action allowed; strict-mode guarantee is temporarily OFF.
marker after: True
EXIT: 0

# 2. Server reachable → next hook fire clears the marker
$ hook <<< '{"tool_name":"Write",...}'  (real MDEMG_URL)
STDERR: (empty)
marker after clear: False
```

## Rules pinned

⚠️ **Jiminy is an ENFORCER, not merely an advisor** (operator directive 2026-08-01). Prior architectural framing that treated `/strict` as opt-in enrichment is superseded. Default enforcement is now shipped; the "advisory" positioning was misaligned with MDEMG's purpose of forcing stateful+deterministic behavior on the stateless+probabilistic LLM substrate.

⚠️ **Fail-open on enforcement gates MUST leave a persistent warning trail** (not silently fall open). The `~/.mdemg/.jiminy-server-unreachable` marker + stderr WARN on every fail-open + prompt-context surfacing until cleared is the contract. Silent fail-open would create a stealth window where the enforcement guarantee is off but nobody knows.

⚠️ **Alert dispatch on enforcement decisions is server-side, not hook-side.** The hook returns block to the tool; the SERVER (which owns the classify decision) also dispatches the alert. Same authority as the decision. Hooks calling a new `/v1/alerts/emit` endpoint would fragment the dispatch surface.

## Not shipped (arc scope, disclosed)

Sequential arc — each sprint ships, verifies, informs the next:
- **JIMINY-ENFORCE-002** — Bash coverage (extend enforcement to Bash tool via new PreToolUse hook consulting `/v1/jiminy/classify`). Design decisions surface AFTER this sprint's live verification: what "context" gets classified for a Bash command; whitelist shape for infrastructure commands.
- **JIMINY-ENFORCE-003** — Override CLI + audit trail (`mdemg jiminy override --constraint <code> --reason <text> --duration <window>` logging to constraint_outcomes as `blocked_false_positive` — a RSIC learning signal).
- **JIMINY-ENFORCE-004** — RSIC enforcement-learning outcome types (`blocked_true_positive`, `blocked_false_positive`, `missed_violation`) + RSIC patterns to consume. THIS is the sprint that will produce naturally-occurring WARNED escalations, closing the E2 live-verify gap.
- **JIMINY-ENFORCE-005** — Post-hoc missed-violation detector (scan for actions that violated a constraint but were not blocked; feed as `missed_violation` outcomes to E4's RSIC loop).

## Rollback

Single-commit revert. The state file `~/.mdemg/.jiminy-strict-mode` will persist (harmless — the strict-mode manager just consults it). Cleanup optional: `rm ~/.mdemg/.jiminy-strict-mode`. The `.env` line stays until operator removes it.

## Documents Accessed

- `internal/api/handlers_jiminy.go` (classify handler, alert emission)
- `internal/api/handlers_jiminy_test.go` (4 new tests)
- `internal/jiminy/strict_mode.go` (StrictModeManager)
- `internal/jiminy/strict_classifier.go` (verdict logic)
- `internal/jiminy/service.go` (strict mode init site)
- `internal/config/config.go` (2 new knobs)
- `internal/cli/hook_templates/pre-write-check.py` (fail-open marker + warning)
- `internal/cli/hook_templates/prompt-context.sh` (persistent marker surfacing)
- `.env` (mdemg-dev flag flip)
- `internal/alert/dispatcher.go` (Send signature)
- Live Neo4j: `J12EscalationState` inspection (0 WARNED+ states → E2 live-trigger deferred to JIMINY-ENFORCE-004)
- Live hook run under subprocess (E3 verification)
