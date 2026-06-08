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

## Wire-up — RESOLVED against this instance (2026-06-08)

Resolved from the running MDEMG instance (`reh3376/mdemg`, local Docker, server at `http://localhost:9999`). Endpoints below were verified live unless marked. **Port discovery** (used by all hooks): `MDEMG_URL` env → `.mdemg.port` file → `.env` `MDEMG_PORT=` → `http://localhost:9999`.

- **MDEMG instance:** local, per-project, Docker via `mdemg init`. Server `http://localhost:9999`; `mdemg healthz` to confirm up.

- **Jiminy query interface — two surfaces:**
  - **MCP server (preferred for an agent):** `mdemg mcp` (stdio JSON-RPC; `internal/cli/mcp.go`). Register in `.mcp.json`:
    ```json
    { "mcpServers": { "mdemg": { "command": "mdemg", "args": ["mcp"], "env": { "MDEMG_ENDPOINT": "http://localhost:9999" } } } }
    ```
    Relevant tools: **`jiminy_guide`** `{context, file_path?, agent_output?, space_id?}` → `prompt_augmentation` (proactive guidance incl. applicable constraints); **`validate_changes`** `{diff, files_changed, space_id?}` → pass/warning/block + violations; plus `memory_recall` / `memory_store`. MCP default `space_id` is `ide-agent`. **There is no separate "raw constraint list" MCP tool** — constraints arrive embedded in `jiminy_guide` output.
  - **HTTP (canonical / fallback):** `POST /v1/jiminy/guide` `{space_id, context, file_path?, agent_output?, session_id?, max_items?}` → `{guidance_id, guidance[], prompt_augmentation, confidence, source_counts, session_escalation, …}` (the Request step; returns a real `guidance_id` — verified). Session calibration handshake: `GET /v1/jiminy/bootstrap?space_id=` → J17 spec + the constraint-code **glossary** (DICT: code→definition), `version: j17v1`, `first_session` (verified). Fast path: `GET /v1/jiminy/latest?space_id=` (instant pre-computed guidance, no LLM) + `POST /v1/jiminy/warm` to pre-compute.

- **PreToolUse hook (enforcement):** registered in `.claude/settings.local.json`:
  - **Bash** → `.claude/hooks/pre-bash-check.py` — blocks destructive shell patterns (DB drops, `rm -rf`, `git push --force`, Cypher `DETACH DELETE`, …); **fail-closed**; no server call.
  - **Write/Edit** → `.claude/hooks/pre-write-check.py` — the J17 content gate: when `/strict` is active it calls `POST /v1/jiminy/classify` `{space_id, session_id, agent_output, tool_name, file_path}` → `{verdict: "pass"|"deny", denial_reason?, violated_codes?}` and denies on `verdict=="deny"`; **fail-open** when the server is unreachable; **no-op when not in `/strict` mode**.

- **Identity key (SessionID):** convention is the hardcoded **`claude-core`** (used by every hook). `pre-write-check.py` reads `session_id` from `~/.mdemg/.jiminy-strict-mode` (JSON), falling back to `claude-core`. There is **no per-Claude-Code-session derivation today** — use `claude-core` consistently for the whole session unless the build-out introduces a real per-session ID.

- **Acknowledgement / RetireCode / outcome calls:**
  - **Comprehend (ack):** `POST /v1/jiminy/protocol/feedback` `{trials:[{constraint_code, tier, score 0-10, interpretation, sender_intent}]}` → `{ingested, weak_codes, …}` — the explicit "restate + score" comprehension test (verified: ingested 1). Comprehension is also captured implicitly via NLI in the outcome call below.
  - **RetireCode:** **internal-only, by design — NO agent-facing call.** Only Jiminy retires a code, via the RSIC/APE protocol-evolution path (`internal/jiminy/protocol_evolution.go::RetireCode`, triggered by `CodeComprehensionTracker` when comprehension decays; operator regen via `POST /v1/jiminy/protocol/learn`). The agent must never treat a constraint as retired/expired on its own judgment.
  - **Report outcome (`GUIDANCE_OUTCOME`):** `POST /v1/jiminy/feedback` `{guidance_id, action_summary, space_id, session_id?, outcome?}` (outcome ∈ followed | partial_compliance | ignored | contradicted | not_applicable) → writes the Neo4j `GUIDANCE_OUTCOME` edge on the matched constraint node + a TSDB `constraint_outcomes` row. `guidance_id` comes from the prior `/guide` (or `jiminy_guide`) response.
  - **Enforcement toggle + persistence:** `POST /v1/jiminy/strict {session_id, enabled}` toggles strict mode (state file `~/.mdemg/.jiminy-strict-mode`). Escalation persists per `(session, constraint)` SURFACED→WARNED→ESCALATED→BLOCKED (`JIMINY_ESCALATION_PERSIST_ENABLED`, default true). Session carry-across: `POST /v1/jiminy/checkpoint` / `POST /v1/jiminy/resume-protocol`.

## Integration gaps the build-out must close (found while resolving)

The protocol pieces all exist and respond, but two things are NOT yet wired as the skill assumes — these are the real build-out work, not the prose:

1. **The MDEMG MCP server is not registered in this repo** (no `.mcp.json` / `.claude/mcp.json` present). Today an agent has **no MCP tool to call** — context is pushed by the `prompt-context.sh` hook (`/v1/jiminy/latest` + `reformulate`), not pulled by the agent. The build-out must either register the MCP server (so `jiminy_guide` is callable) or have the skill route the handshake over HTTP.

2. **PreToolUse enforcement is `/strict`-mode-gated and fail-open**, so it is **not deterministic-by-default**. The skill's claim that "enforcement is deterministic at the point of action" only holds when `/strict` is enabled (`POST /v1/jiminy/strict … enabled:true`) AND the server is reachable. The build-out must decide: enable `/strict` as part of the handshake, and whether the Write/Edit gate should stay fail-open (current) or move to fail-closed-and-surface (the skill's "comply, surface, no workaround" intent leans fail-closed). The Bash gate is already fail-closed; the Write/Edit (J17) gate is not.
