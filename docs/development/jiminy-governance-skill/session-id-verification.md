# Per-conversation SessionID — Verification

**Date:** 2026-06-08. Follow-up to the jiminy-governance build-out (roadmap Workstream C Action 7).

## Problem

Every hook and the skill hardcoded `session_id = "claude-core"`, so trust/escalation/observations from **all** Claude Code conversations collapsed into one shared MDEMG session. J17's design is per-`(session, constraint)` — the implementation just never used the per-conversation id Claude Code already passes on stdin.

## Change

A consistent SessionID resolver, precedence:
1. `MDEMG_SESSION_ID` env — explicit stable-identity escape hatch (set it to pin one id across conversations, restoring the old behavior).
2. **Claude Code stdin `session_id`** — the per-conversation id (new default; every hook gets it on its own stdin → race-free).
3. `~/.mdemg/.claude-session` — published by the SessionStart + UserPromptSubmit hooks so the agent (skill) and any stdin-less context read the same id.
4. `claude-core` — final fallback (preserves prior behavior if nothing else resolves).

### Files
- **Tracked templates** (`internal/cli/hook_templates/`, embedded + installed by `mdemg hooks install`): `session-start.sh`, `prompt-context.sh`, `post-tool-observe.py`, `pre-compact.sh`, + Windows `session-start.ps1` / `prompt-context.ps1`. All hardcoded `claude-core` in MDEMG calls replaced with the resolved `$SESSION_ID`; `session-start`/`prompt-context` publish the session file.
- **Live `.claude/hooks/`** (gitignored, per-developer): same edits applied so this machine benefits now, incl. the local-only `pre-write-check.py` (the `/strict` `/classify` gate) which now prefers its stdin `session_id`.
- **Skill** (`.claude/skills/jiminy-governance/SKILL.md` + tracked `docs/.../jiminy-governance.skill.md`): SessionID instruction changed from hardcoded `claude-core` to "resolve from `MDEMG_SESSION_ID` → `~/.mdemg/.claude-session` → `claude-core`"; handshake steps use `<SessionID>`.

## Live verification (Tier 3)

**Resolution + publish** — ran `prompt-context.sh` with a Claude Code `session_id` on stdin:
```
stdin: {"session_id":"test-perconv-1780949944","user_prompt":"…long test prompt…"}
→ ~/.mdemg/.claude-session = {"session_id":"test-perconv-1780949944","ts":…}   ✓ matches
```

**Flows to an actual MDEMG write** — ran `post-tool-observe.py` (Write→CLAUDE.md path) with a test session, then queried Neo4j:
```
stdin: {"tool_name":"Write","session_id":"test-perconv-obs-1780949964",...}
Neo4j: MATCH (n) WHERE n.session_id='test-perconv-obs-1780949964' →
       MemoryNode | test-perconv-obs-1780949964 | "Modified CLAUDE.md: /tmp/CLAUDE.md"   ✓ keyed per-conversation, NOT claude-core
```

**Static checks:** `bash -n` clean on all `.sh`; `py_compile` clean on all `.py`; every jq `session_id: $sess` has a matching `--arg sess`; no functional `claude-core` literals remain (only doc comments + the documented fallback); `go build ./internal/cli` + the hooks test pass with the modified embedded templates.

## Notes / follow-ups
- **Concurrent same-project conversations:** each hook is always correct (its own stdin id); only the agent-read `~/.mdemg/.claude-session` file is shared, so under truly concurrent conversations the agent's *direct* calls could read the other conversation's id. The hooks (the enforcement path) are unaffected. `prompt-context.sh` rewrites the file every prompt, so it reflects the most-recently-active conversation.
- **`pre-write-check.py` has no tracked template** (it's a local-only `/strict` hook) — its fix is local-only and won't propagate via `mdemg hooks install`. Adding it (and `pre-bash-check.py`'s sibling) to the tracked installer is a clean follow-up.
- Test created ~2 clearly-tagged `test-perconv-*` MemoryNodes in `mdemg-dev` (didn't delete — the destructive-op guard correctly blocks Cypher `DETACH DELETE`, and I won't circumvent it); they're harmless and prune as orphans, or can be removed with a user-confirmed cleanup.
