# HOOKWIRE-001 — Verification (Tiers 1–3)

**Date:** 2026-06-10 · **Stack:** the real running Claude Code session (hooks
live from `.claude/hooks/`) + native `mdemg serve` + Docker Neo4j/TSDB.

## Tier 1 — simulated stdin, real server (every payload shape)

`prompt-context.sh`:
- Real payload (`.prompt`) → recall ran; empty-recall notice printed without
  exiting; **Jiminy guidance delivered for the first time in the channel's
  history** (J17 T1 bootstrap + constraint DICT; `guidance_tokens=5363` vs 0
  forever); health ribbon + synergy footer emitted.
- Legacy payload (`.user_prompt`) → fallback honored (recall hit rendered:
  RRF scorer observation, score 0.80).
- Short (`"ok thanks"`), empty (`{}`), malformed (`not json`) → silent
  `exit 0` (fail-open preserved).

`post-tool-observe.py` (verified against the real CMS, session `hookwire-t1`):
- Failing build, `tool_response` **object** (stderr) → `error` observation
  with real stderr content.
- Passing build, `tool_response` **string** → `progress` "Build/test
  succeeded".
- Empty output → **nothing recorded** (the blind-success path is gone).

`pre-compact.sh`: new jq extracts real activity from this session's actual
transcript (`Bash`, `Bash`); the old `.content` read provably yields nothing.

## Tier 2

`go test ./internal/cli/ -run Hook` green (template embed); both template
copies byte-identical to live modulo `{{SPACE_ID}}` placeholders (verified by
diff); `py_compile` + `bash -n` clean on all changed files.

## Tier 3 — the real session

- **Real failing `go build`** (exit-0 wrapped) executed via the real Bash
  tool → real PostToolUse fire → **`error` observation landed in CMS** with
  the actual compiler output, under the per-conversation session id
  (`46583515-…`). First real error observation through this path ever.
- **Real passing `go build`** → `progress` observation landed (same session
  id).
- **PostToolUse limitation discovered (documented, not fixable hook-side):**
  the hook does NOT fire when the tool result itself is an error (non-zero
  Bash exit) — Claude Code runs PostToolUse on successful tool completion.
  Errors are observed whenever output is surfaced in a successful completion
  (the common case in practice: compound commands, piped output, `2>&1`).
- **prompt-context final confirmation** is the operator's next real prompt
  in this session (UserPromptSubmit cannot be self-triggered by the agent);
  the simulated runs used the exact real payload shape.

## Live corroboration of a roadmap item (out of scope here)

During verification, `reinforcement_events` showed **2,662
`coactivate_session` rows in 30 minutes** from this single session — the
full-clique re-strengthening on every observe that the roadmap's
NEGFEED-001+COOLER-001 rider targets (C(n,2) per observe, `evidence_count`
≈ session length). Phase 2 scope; recorded here as live evidence.

## Acceptance criteria — met

1. ✅ `.prompt` read (legacy fallback kept); channel delivers recall +
   guidance + warm + reinforcement legs.
2. ✅ Guidance decoupled from RESULT_COUNT (delivered on empty recall).
3. ✅ `tool_response` read (string|object|list normalized); success requires
   non-empty clean output; real error/progress observations land in CMS.
4. ✅ Pre-compact transcript extraction works on the real shape.
5. ✅ Both copies in sync (modulo placeholder); tests green; fail-open
   behavior preserved on all degenerate payloads.
