# HOOKWIRE-001 — Sprint Close

**Date:** 2026-06-10 · **Branch:** `reh3376_dev01` · **Roadmap:** Q3 Phase 1, rank #1

## What shipped

The per-prompt delivery channel — dead since inception on a one-field stdin
contract mismatch — is reconnected. Three fixes across both hook copies
(live + installer templates), each with a backward-compatible fallback:

| Epic | Fix | Commit |
|---|---|---|
| 0 | Audit of all 6 hooks vs the real Claude Code stdin schemas + plan | `04cb2c0` |
| 1 | `prompt-context.sh`: `.prompt // .user_prompt`; guidance/warm/reinforcement decoupled from `RESULT_COUNT=0` | `403a0e8` |
| 2 | `post-tool-observe.py`: `tool_response` (normalized string\|object\|list); success requires non-empty clean output | `77ee3d3` |
| 3 | `pre-compact.sh`: transcript extraction reads the real line shape | `cd7df2f` |
| 4–5 | Tier 3 in the real session + docs | (this) |

Hooks verified already-correct and untouched: `session-start.sh`,
`pre-bash-check.py`, `pre-write-check.py`.

## Live evidence (the headline)

- **First guidance delivery in the channel's history**: the fixed hook
  emitted the J17 T1 bootstrap + constraint dictionary (5,363 guidance
  bytes; the synergy footer had recorded 0 on every prompt forever).
- **First real error observation**: a real failing `go build` through the
  real PostToolUse hook landed an `error` observation in CMS carrying the
  actual compiler output — under the per-conversation session id.
- The blind-"Build/test succeeded" path is gone: empty output records
  nothing.

## Findings logged for adjacent sprints

- **PostToolUse fires only on successful tool completion** — non-zero-exit
  commands are unobserved unless output surfaces in a successful completion
  (compound commands / `2>&1`, the common case). Documented in CLAUDE.md;
  candidate consideration for HOOKSYNC-001's absence-detection design.
- **Template↔live drift confirmed bidirectional** (template has an alert
  block the live hook lacks — the 50-unread-alerts mechanism). HOOKSYNC-001
  scope; both copies were kept in sync for the contract fixes here.
- **Live corroboration of the CoactivateSession clique-semantics rider**
  (NEGFEED-001+COOLER-001, Phase 2): 2,662 `coactivate_session` rows in 30
  minutes from this single session.

## Verification

Tier 1: full payload matrix (real shape, legacy, short, empty, malformed)
against the live server + real CMS. Tier 2: hooks tests green, copies
byte-identical modulo `{{SPACE_ID}}`, py_compile/bash -n clean. Tier 3: real
session, real hook fires, real CMS rows (see `verification.md`). Final
confirmation of prompt-context arrives with the operator's next real prompt
(UserPromptSubmit cannot be self-triggered by the agent).

## Documents Accessed

- `.claude/hooks/*` (all 6) + `internal/cli/hook_templates/*`
- `internal/cli/hooks.go` / `hooks_test.go` (embed + tests)
- `docs/development/roadmap/ROADMAP_2026Q3.md` (scope)
- This session's real transcript (`46583515-…jsonl`) for shape verification
- Live CMS (Neo4j) + `reinforcement_events` (TSDB)
