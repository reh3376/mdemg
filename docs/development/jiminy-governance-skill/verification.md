# jiminy-governance Skill — Build-out Verification (Tier 3)

**Date:** 2026-06-08
**Stack:** native `mdemg serve` (launchd) + Docker (Neo4j `mdemg-neo4j-1` + TimescaleDB) + llama-server :8102. Space `mdemg-dev`.

Build-out of roadmap Workstream C Action 7. The wire-up was resolved in the prior step; this verifies the two integration gaps are closed and the full J17 handshake the skill instructs works against the real instance.

## Deliverables
- `.mcp.json` (repo root) — registers the MDEMG MCP server (closes gap 1).
- `.claude/skills/jiminy-governance/SKILL.md` — the operational skill (canonical `.claude/skills/<name>/SKILL.md` layout, YAML frontmatter).
- Enforcement policy (gap 2): Write/Edit gate kept **fail-open** (a hard server dependency on every edit is too brittle); the skill's handshake **auto-enables `/strict`** so the gate is active for the session. No hook code change.

## Acceptance bar: the registration is valid and the handshake works live end-to-end.

### Registration (gap 1)
- `.mcp.json` is valid JSON: `mcpServers.mdemg = {type: stdio, command: "mdemg", args: ["mcp"], env: {MDEMG_ENDPOINT}}`. `mdemg` resolves on PATH (`/opt/homebrew/bin/mdemg`).
- **MCP server live-probed:** `mdemg mcp` over stdio (initialize → tools/list) returns **20 tools incl. `jiminy_guide` + `validate_changes`** — so an agent can now *pull* guidance, not only receive the hook *push*. Claude Code auto-discovers `.mcp.json` at the repo root on next start (first use prompts an approval).
- Skill frontmatter parses; `name: jiminy-governance` (→ `/jiminy-governance`); description 607 chars (under the 1536 cap).

### Handshake (live, end-to-end)
| Step | Call | Result |
|---|---|---|
| 1 — Identify + arm | `POST /v1/jiminy/strict {session_id:"claude-core", enabled:true}` | `strict:true`; state file `~/.mdemg/.jiminy-strict-mode` written |
| 2 — Request | `POST /v1/jiminy/guide {space_id, context, session_id, max_items:5}` | `guidance_id: j75lhzqhtq37v6uy4ccf43sd`, **5 items (3 coded)** |
| 3 — Comprehend | `POST /v1/jiminy/protocol/feedback {trials:[…score 10…]}` | `ingested: 1` |
| 4 — Act/enforce | `POST /v1/jiminy/classify {…, tool_name:"Edit"}` | `verdict: pass` (nothing escalated — correct) |
| 5 — Report | `POST /v1/jiminy/feedback {guidance_id, action_summary, outcome:"followed"}` | `applied:true`, 5 results → **`GUIDANCE_OUTCOME` edges 906 → 909** (+3, one per coded constraint) |

The full loop closes against the real graph — the skill's instructions map 1:1 to working calls, and the outcome sink writes real edges.

### Enforcement, observed live (gap 2)
The Bash gate's fail-closed enforcement was demonstrated *unintentionally but perfectly*: a test `curl` whose JSON payload contained the literal `git push --force` was **blocked by `pre-bash-check.py`** ("DESTRUCTIVE COMMAND BLOCKED. Matched pattern…") — proving the PreToolUse Bash gate enforces deterministically and fail-closed, exactly as the skill states. The Write/Edit gate (`/classify`) returns a verdict when `/strict` is on (step 4) and is fail-open when the server is unreachable.

## Notes / honesty
- `/strict` was enabled only to exercise the mechanics, then **restored to disabled** (state cleared) so the environment isn't silently changed — the skill re-arms `/strict` per session when invoked.
- Full Claude-Code-side skill invocation (the model auto-loading `/jiminy-governance` and the MCP approval dialog) can't be self-simulated in one shell session; the skill *did* register (it appeared in this session's available-skills list), and every call the skill instructs is verified live above.

## Acceptance criteria — met
1. ✅ MCP server registered + live-verified (`jiminy_guide`/`validate_changes` reachable) — gap 1 closed.
2. ✅ Skill authored at the canonical path; frontmatter valid; wire-up concrete.
3. ✅ Enforcement policy set (fail-open + `/strict` armed in handshake); Bash gate fail-closed demonstrated live — gap 2 addressed.
4. ✅ Full handshake (identify → request → comprehend → act → report) runs live; `GUIDANCE_OUTCOME` edges written (906→909).

## Follow-ups
- Per-Claude-session SessionID (today everything uses the `claude-core` convention; a real per-session ID would isolate trust/escalation per conversation).
- Optional: move the Write/Edit J17 gate to fail-closed-and-surface behind a config flag for environments that want a hard guarantee (the skill's intent leans that way; kept fail-open by default for resilience).
