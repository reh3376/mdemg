# jiminy-governance skill

The Claude Code Agent Skill that makes Jiminy (the MDEMG behavioral-guidance subsystem) the authoritative source of project context + governance over the J17 protocol. Roadmap: `SPRINT_ROADMAP_POST_FT_LORA.md` Workstream C Action 7.

## Files here

| File | Role |
|---|---|
| `jiminy-governance.skill.md` | **Install-ready** operational skill (concrete wire-up + the 5-step handshake). |
| `SKILL.md` | Design spec: the original handshake spec + the resolved wire-up + the integration-gaps analysis (history). |
| `verification.md` | Tier-3 live verification of the build-out. |

## Why the installed skill isn't committed directly

`.claude/` is gitignored (`.gitignore` `.claude/*`) — it holds per-developer local config (hooks, settings, skills). So the **installed** skill at `.claude/skills/jiminy-governance/SKILL.md` is local-only by convention (like the other project skills). This directory holds the **tracked, reproducible** copy.

## Install (per developer / machine)

```bash
mkdir -p .claude/skills/jiminy-governance
cp docs/development/jiminy-governance-skill/jiminy-governance.skill.md \
   .claude/skills/jiminy-governance/SKILL.md
```

The MCP server it uses (`mdemg mcp`) is registered in the repo-root `.mcp.json` (tracked) — Claude Code auto-discovers it on next start (first use prompts an approval).

## What it does (one-liner)

At session start + before governed actions: query Jiminy (`jiminy_guide` MCP tool or `POST /v1/jiminy/guide`) for applicable constraints, arm `/strict` enforcement, restate constraints (comprehension ack), act under the PreToolUse hooks, and report the outcome (`POST /v1/jiminy/feedback` → `GUIDANCE_OUTCOME` edge). It is a routing/handshake shim — the rules live in the MDEMG graph, not in the skill.
