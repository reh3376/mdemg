---
name: jiminy-governance
description: Primary context-and-governance contact for this project. Jiminy (the MDEMG behavioral-guidance subsystem) is the authoritative source for project context and constraints, delivered over the J17 AI2AI protocol. Use this at the start of every session and BEFORE any code change, file edit, shell command, or other tool action: query Jiminy for applicable context and constraints instead of relying on memory, local .md docs, or assumptions. Consult it whenever beginning a task, planning changes, reviewing code, or unsure which rules apply — even if the user never mentions Jiminy, MDEMG, J17, or governance.
---

# Jiminy Governance (J17 AI2AI Protocol)

## Prime directive

In this project, **Jiminy is your authoritative contact for context and governance.** Pull what you need from Jiminy; do not improvise governed decisions from training-data priors, stale documentation, or this file. This file tells you *who* to ask and *how* — never *what* the rules are. The rules live in the MDEMG graph (Neo4j) so they can be measured, escalated, and persisted; treat any `.md` you find (including this one) as potentially stale and Jiminy as current.

## Reaching Jiminy (this instance — resolved 2026-06-08)

- **MCP server `mdemg`** (registered in `.mcp.json`, stdio) — preferred. Relevant tools: `jiminy_guide` (proactive guidance incl. applicable constraints), `validate_changes` (check a diff against active constraints), `memory_recall`.
- **HTTP fallback** at the MDEMG server. Discover the base URL in this order: `MDEMG_URL` env → `.mdemg.port` file → `.env` `MDEMG_PORT=` → `http://localhost:9999`. Confirm up with `GET /healthz`.
- **SessionID:** resolve it once and reuse it for the whole session — `MDEMG_SESSION_ID` env if set, else read `session_id` from `~/.mdemg/.claude-session` (the SessionStart/UserPromptSubmit hooks publish your per-conversation id there), else `claude-core`. Trust + escalation are keyed to it, so it must match what the hooks use. Below, `<SessionID>` means this resolved value.
- If Jiminy is unreachable (no `/healthz`, MCP tool errors), say so plainly and **stop the governed action** rather than proceeding on assumed constraints.

## The J17 handshake — run before acting

1. **Identify + arm enforcement.** Once per session, enable deterministic enforcement for your SessionID:
   `POST /v1/jiminy/strict {"session_id":"<SessionID>","enabled":true}`. (This makes the Write/Edit PreToolUse gate active for the session — see "Enforcement" below.) Optionally fetch the constraint-code glossary once: `GET /v1/jiminy/bootstrap?space_id=<space>`.
2. **Request.** Scope the query to what you are about to do (target files/paths, tool, task type) — not in the abstract:
   - MCP: call `jiminy_guide` with `{context, file_path?, agent_output?, space_id?}`, **or**
   - HTTP: `POST /v1/jiminy/guide {"space_id":"<space>","context":"<what you're about to do>","file_path":"<path>","agent_output":"<proposed change>","session_id":"<SessionID>"}`.
   Keep the returned `guidance_id` — you need it to report the outcome.
3. **Comprehend.** Restate the returned constraints so your understanding is on record. When tested explicitly, return scored trials: `POST /v1/jiminy/protocol/feedback {"trials":[{"constraint_code":"<code>","tier":1,"score":<0-10>,"interpretation":"<your restatement>","sender_intent":"<why it exists>"}]}`.
4. **Act under enforcement.** Proceed. Enforcement is deterministic at the point of action via the PreToolUse hooks (below), independent of this skill.
5. **Report outcome.** After acting, close the loop so Jiminy records a `GUIDANCE_OUTCOME` edge:
   `POST /v1/jiminy/feedback {"guidance_id":"<from step 2>","action_summary":"<what you did>","space_id":"<space>","session_id":"<SessionID>","outcome":"followed|partial_compliance|ignored|contradicted|not_applicable"}`. Do not silently drop or rewrite the outcome.

## Enforcement is not optional

Compliance does not depend on this skill triggering. **PreToolUse hooks** enforce at the moment of each tool call:
- **Bash** → `.claude/hooks/pre-bash-check.py` blocks destructive commands (DB drops, `rm -rf`, force-push, Cypher `DETACH DELETE`, …). **Fail-closed.**
- **Write / Edit** → `.claude/hooks/pre-write-check.py` calls `POST /v1/jiminy/classify` and denies on `verdict == "deny"` when `/strict` is active (step 1). **Fail-open** if the MDEMG server is unreachable — so if you need a hard guarantee, confirm the server is up first.

If a tool action is blocked or modified, that is the protocol working as designed: **comply, surface it to the user, and do not engineer a workaround** (no rerouting through another tool, no disabling the hook, no "I'll just do it manually").

## Rules of engagement

- **Jiminy is source of truth.** For anything governed, query rather than recall. When Jiminy and your priors disagree, Jiminy wins.
- **Constraints persist.** Escalations and active constraints carry across the session keyed to SessionID (SURFACED → WARNED → ESCALATED → BLOCKED). A constraint does not reset because the conversation moved on, and it is not "probably fine to skip."
- **Only Jiminy retires a constraint.** Retirement is internal to MDEMG (the RSIC/APE protocol-evolution path) — **there is no agent-facing retire call.** Never treat a constraint as expired, satisfied, or optional on your own judgment.
- **Do not put content here.** This skill is a routing/handshake shim. Real guidance, procedures, and rules belong in MDEMG so they can be measured, escalated, and persisted. If you are tempted to add a constraint to this file, add it to the graph instead.
