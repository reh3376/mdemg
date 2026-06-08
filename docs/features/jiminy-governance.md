# Jiminy Governance (J17) — How It Works + File Inventory

This document describes, in detail, the **agent-governance system**: how an AI coding agent (Claude Code) is steered by MDEMG's Jiminy subsystem over the J17 protocol, the moving parts, and every file relevant to its success. It is the operational/architecture companion to the skill spec in `docs/development/jiminy-governance-skill/`.

## 1. What it is, in one paragraph

When Claude Code works in this repo, it should pull project context + constraints **from Jiminy** (the authoritative source, backed by the MDEMG Neo4j graph) rather than from training-data priors or stale `.md` files. Two independent mechanisms make that happen: **(a) a Claude Code Agent Skill** that tells the agent *who* to ask and *how* (the J17 handshake), and **(b) deterministic PreToolUse hooks** that enforce constraints at the moment each tool call is made — regardless of whether the agent "remembered" to consult Jiminy. The skill is the polite front door; the hooks are the backstop. Real rules live in the graph so they can be measured, escalated, and persisted; the skill and this doc are routing/handshake shims, never rulebooks.

## 2. The J17 handshake (what the agent does)

The skill instructs the agent to run a 5-step loop at session start and before governed actions:

| Step | Purpose | Concrete call (this instance) |
|---|---|---|
| **1. Identify + arm** | Establish identity; turn on deterministic enforcement for the session | `POST /v1/jiminy/strict {"session_id":"<SessionID>","enabled":true}`; optional `GET /v1/jiminy/bootstrap?space_id=<space>` for the constraint-code glossary |
| **2. Request** | Get the constraints/context applicable to the task at hand | MCP tool `jiminy_guide`, **or** `POST /v1/jiminy/guide {space_id, context, file_path?, agent_output?, session_id}` → returns `guidance_id` + guidance items |
| **3. Comprehend** | Record that the agent understood the constraints (J17 measures comprehension) | restate them; when tested, `POST /v1/jiminy/protocol/feedback {trials:[{constraint_code, tier, score 0-10, interpretation, sender_intent}]}` |
| **4. Act under enforcement** | Do the work; the PreToolUse hooks gate each tool call deterministically | (no agent call — the hooks fire automatically) |
| **5. Report outcome** | Close the loop so effectiveness is recorded | `POST /v1/jiminy/feedback {guidance_id, action_summary, space_id, session_id, outcome}` → writes a Neo4j `GUIDANCE_OUTCOME` edge + a TSDB `constraint_outcomes` row |

If Jiminy is unreachable, the skill says: stop the governed action and surface it — do not proceed on assumed constraints.

## 3. SessionID — per-conversation identity

Trust, escalation, and observations are keyed to a **SessionID**. It is resolved identically by every hook and by the skill, with this precedence:

1. `MDEMG_SESSION_ID` env — explicit override; pin it to a stable value to share escalation state across conversations (the old behavior).
2. **Claude Code stdin `session_id`** — the per-conversation id Claude Code passes to every hook (the default; realizes J17's per-`(session, constraint)` isolation).
3. `~/.mdemg/.claude-session` — published by the SessionStart + UserPromptSubmit hooks so the agent (skill) and stdin-less contexts read the same id the hooks use.
4. `claude-core` — final fallback.

Each hook reads its own stdin id (race-free). The published file is the side-channel that lets the agent's *direct* calls (steps 1/2/5) use the same id the hooks key to. Under truly concurrent same-project conversations only that file is shared; the hooks themselves are always correct.

## 4. Enforcement model (the PreToolUse hooks)

Enforcement does not depend on the skill triggering. Two hooks gate tool calls:

- **Bash** → `pre-bash-check.py`: blocks destructive shell commands (DB drops, `rm -rf`, force-push, Cypher `DETACH DELETE`, …) by local pattern match. **Fail-closed** (no server needed; if it can't decide, it blocks).
- **Write / Edit** → `pre-write-check.py`: when `/strict` is active for the session, calls `POST /v1/jiminy/classify` and denies the edit on `verdict == "deny"` (an *escalated* constraint violation). **Fail-open** if the MDEMG server is unreachable (a hard server dependency on every edit is too brittle); `/strict` is armed by handshake step 1.

If a tool call is blocked, the protocol is working as designed: **comply, surface it to the user, do not engineer a workaround.**

### Escalation (why constraints "stick")
Per `(session, constraint)`, an ignored constraint climbs `SURFACED → WARNED → ESCALATED → BLOCKED` (`JIMINY_ESCALATION_PERSIST_ENABLED`, default true, persisted to Neo4j). Only at `ESCALATED`+ does the `/classify` gate deny. **Only Jiminy retires a constraint** — retirement is internal (the RSIC/APE protocol-evolution path); there is no agent-facing "retire" call, and the agent must never treat a constraint as expired on its own judgment.

## 5. The two delivery surfaces

- **MCP server `mdemg`** (registered in `.mcp.json`, stdio): exposes `jiminy_guide`, `validate_changes`, `memory_recall`, … Lets the agent **pull** guidance as a tool call. Claude Code auto-discovers `.mcp.json` at the repo root (first use prompts an approval).
- **Hooks** push context every prompt (`prompt-context.sh` reads `/v1/jiminy/latest`) and enforce every tool call. The hooks run regardless of the skill or MCP.

So context reaches the agent two ways — pulled (MCP/HTTP via the skill) and pushed (hooks) — and enforcement is independent of both.

## 6. File inventory (everything relevant to its success)

### The skill
| File | Role |
|---|---|
| `.claude/skills/jiminy-governance/SKILL.md` | The installed Agent Skill (local-only; `.claude/skills/` is gitignored). Frontmatter `name`/`description` drives auto-invocation. |
| `docs/development/jiminy-governance-skill/jiminy-governance.skill.md` | Tracked, install-ready copy (the reproducible source — `cp` it into `.claude/skills/jiminy-governance/SKILL.md`). |
| `docs/development/jiminy-governance-skill/SKILL.md` | Design spec + resolved wire-up + integration-gap analysis. |
| `docs/development/jiminy-governance-skill/README.md` | What's here + the one-line install. |
| `docs/development/jiminy-governance-skill/verification.md`, `session-id-verification.md` | Live Tier-3 verification records. |

### MCP registration
| File | Role |
|---|---|
| `.mcp.json` (repo root, tracked) | Registers the `mdemg` MCP server (`mdemg mcp`, stdio). |
| `internal/cli/mcp.go` | The MCP server implementation (`mdemg mcp`): 20 tools incl. `jiminy_guide`, `validate_changes`, `memory_*`. |
| `cmd/mcp-server/main.go` | MCP server entrypoint. |

### Hooks — tracked source (installed by `mdemg hooks install`)
| File | Hook event / matcher | Role |
|---|---|---|
| `internal/cli/hook_templates/session-start.sh` (+`.ps1`) | SessionStart | Restore CMS memory; resolve + **publish** the SessionID file; warm guidance. |
| `internal/cli/hook_templates/prompt-context.sh` (+`.ps1`) | UserPromptSubmit | Recall CMS context + Jiminy guidance per prompt; republish SessionID. |
| `internal/cli/hook_templates/post-tool-observe.py` | PostToolUse (Bash\|Write\|Edit) | Auto-capture decisions/errors/progress as observations, keyed to the SessionID. |
| `internal/cli/hook_templates/pre-compact.sh` | PreCompact | Snapshot state to CMS before compaction; J17 ticket. |
| `internal/cli/hook_templates/pre-bash-check.py` | PreToolUse (Bash) | **Fail-closed** destructive-command block. |
| `internal/cli/hook_templates/pre-write-check.py` | PreToolUse (Write\|Edit) | **Fail-open** `/strict` J17 `/classify` gate. |
| `internal/cli/hook_templates/embed.go` | — | `//go:embed *.sh *.ps1 *.py` — embeds the templates into the binary. |
| `internal/cli/hooks.go` | — | `claudeHookFiles()` (the hook registry: name, event, timeout, matcher) + `InstallClaudeHooks` (substitutes `{{SPACE_ID}}`, writes `.claude/hooks/`, registers in `.claude/settings.local.json`). |
| `internal/cli/hooks_test.go` | — | Asserts the hook registry + that templates carry no `{{MDEMG_URL}}` placeholder (URL is discovered at runtime). |

### Hooks — installed copies (this machine)
`.claude/hooks/{session-start.sh, prompt-context.sh, post-tool-observe.py, pre-compact.sh, pre-bash-check.py}` are **tracked**; `pre-write-check.py` is **local-only** until the installer reinstalls it. `.claude/settings.local.json` registers each hook with its event/matcher/timeout.

### Server-side (the J17 protocol)
| File | Role |
|---|---|
| `internal/api/handlers_jiminy.go` | `/v1/jiminy/{guide,warm,latest,feedback,evaluate,classify,reformulate,strict}` handlers. |
| `internal/api/handlers_j17.go` | `/v1/jiminy/{bootstrap,protocol/*,checkpoint,resume-protocol,extension}` (the J17 protocol surface). |
| `internal/jiminy/service.go` | Core guidance synthesis + constraint-code matching. |
| `internal/jiminy/escalation.go`, `escalation_store.go` | Per-`(session,constraint)` escalation state machine + Neo4j persistence. |
| `internal/jiminy/strict_mode.go` | `/strict` toggle + `~/.mdemg/.jiminy-strict-mode` state file (hooks read it without HTTP). |
| `internal/jiminy/persistence.go` | `PersistGuidanceOutcome` → Neo4j `GUIDANCE_OUTCOME` edges; `GetConstraintEffectiveness`. |
| `internal/jiminy/protocol_evolution.go` | `RetireCode` (Jiminy-internal constraint retirement). |
| `internal/jiminy/code_comprehension_tracker.go` | Comprehension decay → triggers code regen/retire. |

### Runtime state files (under `~/.mdemg/`)
| File | Written by | Read by |
|---|---|---|
| `~/.mdemg/.claude-session` | session-start / prompt-context hooks | every hook + the agent (skill) — the SessionID side-channel |
| `~/.mdemg/.jiminy-strict-mode` | `POST /v1/jiminy/strict` | `pre-write-check.py` (is `/strict` on? + session_id) |
| `~/.mdemg/.jiminy-guidance-state` | prompt-context hook | feedback loop (the `guidance_id` to report against) |
| `~/.mdemg/alerts/current.json` | alert dispatcher | session-start / prompt-context hooks (surface alerts) |

## 7. Configuration knobs

| Concern | Env / mechanism | Default |
|---|---|---|
| SessionID override (stable identity) | `MDEMG_SESSION_ID` | unset → per-conversation |
| Server URL discovery (hooks) | `MDEMG_URL` → `.mdemg.port` → `.env MDEMG_PORT` | `http://localhost:9999` |
| Space | `MDEMG_SPACE_ID` / `.mdemg/config.yaml` | install-substituted `{{SPACE_ID}}` |
| Strict mode | `POST /v1/jiminy/strict` + `~/.mdemg/.jiminy-strict-mode` | off (skill arms it) |
| Escalation persistence | `JIMINY_ESCALATION_PERSIST_ENABLED` | true |
| Constraint-code match threshold | `JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD` | 0.55 |

## 8. How to install / verify on a fresh machine

```bash
# 1. Hooks (installs the 6 hooks + registers them in .claude/settings.local.json)
mdemg hooks install

# 2. Skill (.claude/ is gitignored — copy the tracked, install-ready skill in)
mkdir -p .claude/skills/jiminy-governance
cp docs/development/jiminy-governance-skill/jiminy-governance.skill.md \
   .claude/skills/jiminy-governance/SKILL.md

# 3. MCP server is already registered via the tracked repo-root .mcp.json
#    (Claude Code prompts to approve it on first use)

# 4. Verify the loop end-to-end
curl -s localhost:9999/healthz
curl -s -X POST localhost:9999/v1/jiminy/strict -d '{"session_id":"claude-core","enabled":true}'
# ... then guide → protocol/feedback → feedback (see the handshake table)
```

## 9. Known limitations / follow-ups
- **Concurrent same-project conversations** share `~/.mdemg/.claude-session`; the hooks are unaffected (own stdin id), but the agent's direct calls could read the other conversation's id.
- **Write/Edit gate is fail-open** by design; a config-gated fail-closed-and-surface mode is a possible future option for environments that want a hard guarantee.
- Per-Claude-session SessionID uses the `claude-core` fallback only when no stdin/file/env id is available.
