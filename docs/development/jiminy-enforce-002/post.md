# JIMINY-ENFORCE-002 — Sprint Post

**Date:** 2026-08-02 | **Branch:** `reh3376_dev01`
**Arc:** JIMINY-ENFORCE sprint 2 of 5
**Trigger:** JIMINY-ENFORCE-001 shipped Write/Edit coverage; Bash was explicitly disclosed as sprint 2. Both upstream signal-loss prerequisites (ESCALATION-ACCUMULATE-001, JIMINY-TRACKER-TTL-001) are now landed, so escalation → deny → enforcement can fire on natural traffic.

## Verdict

**Shipped.** Bash tool now flows through `/v1/jiminy/classify` via an extension to the existing `pre-bash-check.py` hook. Live-verified all 4 scenarios: whitelisted read-only skips classify (no LLM cost); non-whitelisted with strict-on hits classify (pass when no escalation exists); server-unreachable falls open with stderr WARN + marker file; destructive-pattern class still fails-closed.

## What shipped

### Hook extension (`internal/cli/hook_templates/pre-bash-check.py`)
Two-check pipeline (order preserved):
1. **Destructive-pattern guard** (fail-CLOSED, shipped) — unchanged: blocks `rm -rf`, `DROP TABLE`, `git push --force`, etc.
2. **JIMINY-ENFORCE-002 classify** (fail-OPEN, new) — when `~/.mdemg/.jiminy-strict-mode` state file present AND command isn't in the read-only whitelist, POSTs to `/v1/jiminy/classify` with `tool_name="Bash"`, `agent_output=<command truncated to 2000 chars>`, `file_path=""`. Verdict `deny` → block with `[/strict] <reason>`. Fail-open branches (URLError/timeout/OSError/JSONDecodeError) emit stderr WARN + write `~/.mdemg/.jiminy-server-unreachable` marker (parity with pre-write-check.py). Successful classify clears the marker.

### Read-only whitelist
5 default regexes exempt commands from classify (skips the LLM round-trip for zero-enforcement-value calls):
- Basic Unix: `ls|cat|head|tail|wc|grep|find|file|stat|which|whereis|pwd|whoami|hostname|date|echo|printf|env|id|uname|uptime|df|du|ps|top|htop|free|lsof|netstat|dig|nslookup|ping`
- Git reads: `git (status|log|diff|show|branch|remote|config --get|rev-parse|blame|ls-files|describe|reflog)`
- Docker/Kubectl reads: `docker (ps|inspect|logs|images|version|info|exec <c> (ls|cat|ps))` / `kubectl (get|describe|logs|config view|version)`
- Version prints: `(go|python|node|npm|yarn|cargo) (version|--version|-V)`

Operator can extend via `MDEMG_BASH_WHITELIST_REGEX` (comma-separated regex list, appended to defaults).

### Server-side alert-message shape (`internal/api/handlers_jiminy.go`)
`emitJiminyBlockAlert` now branches on `req.ToolName`:
- Write/Edit (has `FilePath`): `"<reason> (file: <path>, tool: <name>)"` (unchanged)
- Bash (empty FilePath, non-empty AgentOutput): `"<reason> (tool: Bash, command: <first 200 chars>…)"` (new — truncated command preview)
- Other tools: `"<reason> (tool: <name>)"`

### Tests
2 new pins in `internal/api/handlers_jiminy_test.go`:
- `TestEmitJiminyBlockAlert_BashIncludesCommandPreview` — Bash message must include `tool: Bash`, command preview, and reason
- `TestEmitJiminyBlockAlert_BashTruncatesLongCommands` — long commands truncated with `…` marker, total message bounded

Total emit-alert coverage: 6 tests (4 existing + 2 new); all pass.

## Live Tier-3 (mdemg-dev, 2026-08-02)

Four scenarios verified via subprocess-invoked hook:

| # | Scenario | Command | Expected | Observed |
|---|---|---|---|---|
| 1 | Whitelisted | `ls -la /tmp` | exit 0, no stderr | exit 0, no stderr ✓ |
| 2 | Non-whitelisted, no escalation | `gh pr list --json number` | exit 0, no output (classify passed) | exit 0, no output ✓ |
| 3 | Fail-open | `gh pr view 42` with `MDEMG_URL=http://127.0.0.1:1` | stderr WARN + marker file created | `⚠️  JIMINY ENFORCEMENT SUSPENDED (Bash, ... URLError)`, marker file created ✓ |
| 4 | Destructive (unchanged) | `rm -rf /tmp/foo` | deny JSON with matched pattern | `DESTRUCTIVE COMMAND BLOCKED. Matched pattern: \brm\s+(-…)` ✓ |

## Rules pinned

⚠️ **Bash hook enforcement layers stack: destructive-guard (fail-closed) first, Jiminy classify (fail-open) second.** The two have different risk profiles and must retain different failure modes:
- Destructive-guard defends the operator from irreversible data loss — a broken hook must NOT allow through
- Jiminy classify defends against durable-rule violations — a broken server must NOT wedge Bash (fail-open with persistent warning, per operator directive 2026-08-01)

Never reorder them or merge their failure modes.

⚠️ **Read-only command whitelist exists to keep LLM cost/latency bounded — extending it is safe when the ADDED command is definitively read-only.** A wrong-side entry (adding a write command to the whitelist) creates a stealth bypass of enforcement. Rule: whitelist entries MUST be commands that CANNOT violate a durable constraint (no file writes, no network POSTs, no state mutations). `git status` yes; `git commit` no.

## Not shipped (arc scope, disclosed)

- **JIMINY-ENFORCE-003** — Override CLI + audit trail (`mdemg jiminy override --constraint <code> --reason <text> --duration <window>` for the escape-hatch flow)
- **JIMINY-ENFORCE-004** — RSIC enforcement-learning outcome types (`blocked_true_positive`, `blocked_false_positive`, `missed_violation`)
- **JIMINY-ENFORCE-005** — Post-hoc missed-violation detector

Deny path live-triggerability: with ESCALATION-ACCUMULATE-001 shipped, natural WARNED escalations are now producible via real ignored outcomes (proven in the ACCUMULATE-001 drill — 8 WARNED states created from 2 seeded ignores). Bash deny on natural traffic is now genuinely reachable — a Bash command that matches a WARNED constraint will get blocked, alert dispatched, marker never created (successful classify).

## Rollback

Single-commit revert. The pre-bash-check.py hook reverts to destructive-only (its original shape). Marker files and state files are harmless leftovers.

## Documents Accessed

- ESCALATION-ACCUMULATE-001 + JIMINY-TRACKER-TTL-001 sprint posts (prerequisites)
- JIMINY-ENFORCE-001 sprint post (pattern reference)
- `internal/cli/hook_templates/pre-bash-check.py` (extended)
- `internal/cli/hook_templates/pre-write-check.py` (mirror source for fail-open shape)
- `internal/api/handlers_jiminy.go` (message-shape branch)
- `internal/api/handlers_jiminy_test.go` (2 new tests)
- Live server (all 4 subprocess scenarios)
