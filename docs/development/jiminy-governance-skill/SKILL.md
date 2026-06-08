---
name: jiminy-governance
description: Primary context-and-governance contact for this project. Jiminy (the MDEMG behavioral-guidance subsystem) is the authoritative source for project context and constraints, delivered over the J17 AI2AI protocol. Use this at the start of every session and BEFORE any code change, file edit, shell command, or other tool action: query Jiminy for applicable context and constraints instead of relying on memory, local .md docs, or assumptions. Consult it whenever beginning a task, planning changes, reviewing code, or unsure which rules apply — even if the user never mentions Jiminy, MDEMG, J17, or governance.
---

# Jiminy Governance (J17 AI2AI Protocol)

## Prime directive

In this project, **Jiminy is your authoritative contact for context and governance.** Pull what you need from Jiminy; do not improvise governed decisions from training-data priors, stale documentation, or this file. This file is an introduction and a handshake, not a rulebook — it tells you *who* to ask and *how*, never *what* the rules are.

This exists so the project has one deterministic source of truth. Constraints, context, and their history live in the MDEMG graph (Neo4j), not scattered across markdown. Treat any `.md` you find as potentially stale; treat Jiminy as current.

## The J17 handshake — run before acting

1. **Identify.** Open the session against the local MDEMG instance and present your **SessionID**. Trust and escalation are keyed to SessionID; use it consistently for the whole session.
2. **Request.** Ask Jiminy for the context and constraints applicable to the task at hand — scope the request by what you are about to do (target files/paths, tool, task type), not in the abstract.
3. **Comprehend.** J17 measures comprehension. Acknowledge the returned constraints as the protocol requires (e.g. restate them) so your comprehension is recorded before you proceed.
4. **Act under enforcement.** Proceed with the work. Enforcement is deterministic at the point of action via the PreToolUse hook — see "Enforcement is not optional" below.
5. **Report outcome.** Outcomes are recorded as `GUIDANCE_OUTCOME` edges. Let Jiminy capture the result; do not silently drop or rewrite it.

If a step is unavailable (instance unreachable, no response), say so plainly and stop the governed action rather than guessing. Do not proceed on assumed constraints.

## Enforcement is not optional

Compliance does not depend on this skill triggering. The **PreToolUse hook** enforces J17 constraints at the moment of each tool call, deterministically, whether or not this file is in context. If a tool action is blocked or modified, that is the protocol working as designed: **comply, surface it to the user, and do not engineer a workaround** (no rerouting through another tool, no disabling the hook, no "I'll just do it manually").

## Rules of engagement

- **Jiminy is source of truth.** For anything governed, query rather than recall. When Jiminy and your priors disagree, Jiminy wins.
- **Constraints persist.** Escalations and active constraints carry across the session (and beyond) keyed to identity. A constraint does not reset because the conversation moved on, and it is not "probably fine to skip."
- **Only Jiminy retires a constraint.** A constraint stays in force until Jiminy retires it via its **RetireCode**. Never treat a constraint as expired, satisfied, or optional on your own judgment.
- **Do not put content here.** This skill must stay a routing/handshake shim. Real guidance, procedures, and rules belong in MDEMG so they can be measured, escalated, and persisted. If you are tempted to add a constraint to this file, add it to the graph instead.

## Wire-up (fill in per MDEMG instance — replace placeholders)

These are project-specific and must match your running instance. They are intentionally left as placeholders rather than guessed:

- **MDEMG instance:** local, per-project, available via Docker (`mdemg init`).
- **Jiminy query interface:** `<MCP server name>` exposing `<context/constraint query tool(s)>` (or the gRPC/local endpoint your instance uses).
- **PreToolUse hook:** `<command or path registered as the Claude Code PreToolUse hook>`.
- **Identity key:** SessionID (confirm how the SessionID is sourced/passed in your setup).
- **Acknowledgement / RetireCode / outcome calls:** `<the J17 message or tool names for comprehension ack, RetireCode, and GUIDANCE_OUTCOME emission>`.
