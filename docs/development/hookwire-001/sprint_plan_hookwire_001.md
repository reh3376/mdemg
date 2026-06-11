# Sprint Plan HOOKWIRE-001 — Fix the Hook Stdin Contract (Reconnect the Per-Prompt Channel)

## 1. Header & Metadata

- **Sprint ID:** HOOKWIRE-001 (Roadmap Q3 Phase 1, rank #1)
- **Sprint line:** `docs/development/hookwire-001/`
- **Date opened:** 2026-06-10
- **Branch:** `reh3376_dev01`
- **Target version:** v0.10.x
- **Estimated effort:** ~2 dev-days
- **OpenAI spend:** $0
- **Risk level:** Low (shell/python edits; the channel is already dead, so regressions cannot make it worse)

## 2. Problem Statement

The per-prompt delivery channel — the mechanism that makes MDEMG the
assistant's *internal dialogue* rather than a write-only database — silently
no-ops on every prompt, and the post-tool observer records false successes.
Live-verified (2026-06-10, this repo, running hooks):

1. **`prompt-context.sh:46`** reads `jq -r '.user_prompt // empty'`; Claude
   Code's UserPromptSubmit stdin sends the field **`prompt`**. `USER_PROMPT`
   is always empty → `exit 0` at line 48 → **no per-prompt CMS recall, no
   Jiminy guidance, no /strict reformulation, ever.**
2. **`post-tool-observe.py:574`** reads `input_data.get("tool_output")`;
   Claude Code's PostToolUse stdin sends **`tool_response`** (string or
   object). `output_str` is always empty → error indicators never match →
   every `go build` / `go test` / `pytest` Bash call records
   **"Build/test succeeded"** regardless of the actual result (confirmed:
   CMS is full of these), and real errors are never observed.
3. **`prompt-context.sh:72-87`** exits when recall returns 0 results —
   *before* the Jiminy guidance section. Guidance delivery is wrongly
   coupled to recall having results.
4. (Minor) **`pre-compact.sh:62`** extracts transcript context via
   `jq -r '.content'` on transcript lines whose shape is
   `{type, message:{content:[…]}}` — yields empty.

Hooks verified CORRECT (no change): `session-start.sh` (`session_id` ✓,
works — evidenced every session), `pre-bash-check.py` (`tool_input.command`
✓, works — evidenced blocks), `pre-write-check.py` (`tool_input` fields ✓).

## 3. Scope & Constraints

**In scope:**
- Fix the two stdin contract breaks (with backward-compatible fallbacks:
  `.prompt // .user_prompt`, `tool_response or tool_output`) in BOTH copies:
  `.claude/hooks/` (live) and `internal/cli/hook_templates/` (installer).
- `tool_response` shape handling: string OR object (`stdout`+`stderr` join).
- Decouple Jiminy guidance from `RESULT_COUNT=0` (guidance runs regardless;
  empty-recall message stays).
- Fix pre-compact transcript extraction (minor).
- Tier 1 simulated-stdin runs for every changed hook; Tier 3 = a **real**
  Claude Code session: next real prompt produces CMS RECALL output, a real
  failing build records an `error` observation (not "succeeded").
- Hooks tests (`internal/cli/hooks_test.go`) still green; template embed
  byte-sync.

**Out of scope (→ HOOKSYNC-001, next sprint):** CI parity gate between the
two hook copies, `hook_events` absence-detection rule, alert delivery
restoration, `mdemg hooks doctor`, AUTH riders.

**Out of scope (disclosed, no action):** purging the historical false
"Build/test succeeded" observations from CMS — forward-only precedent (same
as the CoactivateSession orphans); they are the record of the bug.

**Constraints:** sequential epics; live Tier 3 required; no hardcoded
values; fail-open behavior of hooks preserved (a broken server must never
block the user's prompt).

## 4. Dependencies

- Claude Code hook stdin schemas (UserPromptSubmit: `session_id`,
  `transcript_path`, `cwd`, `hook_event_name`, `prompt`; PostToolUse: adds
  `tool_name`, `tool_input`, `tool_response`).
- Running MDEMG server (live Tier 3).
- `internal/cli/hooks.go` embed of `hook_templates/` (both copies must move
  together).

## 5. Implementation Plan

**Epic 0 — Audit + plan (done):** every stdin field read across all 6 hooks
mapped against the actual contract; this plan.

**Epic 1 — prompt-context.sh (~0.5d):** `.prompt // .user_prompt // empty`;
move the Jiminy guidance block out of the RESULT_COUNT gate (guidance and
recall are independent deliveries); keep the empty-recall notice + health
ribbon. Mirror to template. Simulated-stdin runs: real-shape payload
(.prompt) → recall + guidance emitted; legacy payload (.user_prompt) →
still works; empty → silent exit.

**Epic 2 — post-tool-observe.py (~0.5d):** read
`tool_response` (fallback `tool_output`); normalize string|object (join
`stdout` + `stderr` when dict); success classification only when output is
NON-EMPTY and clean — empty output records nothing (no more blind
"succeeded"). Mirror to template. Simulated-stdin runs: failing-build
payload → `error` observation; passing-build payload → `progress`; empty
output → no success claim.

**Epic 3 — pre-compact.sh transcript fix (~0.1d):** robust jq for the
transcript line shape. Mirror to template.

**Epic 4 — Tier 3 live verification (~0.4d):** real Claude Code session
evidence: (a) a real prompt in this session produces the `═══ CMS RECALL ═══`
block + Jiminy guidance bytes; (b) a deliberately failing `go build` records
an `error` observation in CMS (and no "succeeded"); (c) a passing build
records `progress`. Verify via `/v1/conversation/recall` + TSDB/Neo4j reads.
`verification.md`.

**Epic 5 — Documentation (final, never cut):** CHANGELOG; CLAUDE.md
Enforced-Protocols note (contract pinned); roadmap checkbox; post.md.

## 6. Testing Plan (3 tiers)

- **Tier 1:** simulated-stdin invocations of each changed hook with the real
  payload shapes (current contract + legacy fallback + empty/malformed);
  `python3 -m py_compile` on templates; `go test ./internal/cli/ -run Hook`.
- **Tier 2:** `mdemg hooks install` into a temp dir → installed copies
  byte-match templates; existing hooks tests green.
- **Tier 3 (live):** the real running Claude Code session: real prompt →
  visible recall+guidance; real failing/passing builds → correct
  observations in CMS, queried back through the real API.
- **UVTS/UBENCH/UATS:** N/A (no server code change; the server contract is
  untouched — this is the client side of the channel).

## 7. Commit Strategy

One commit per epic on `reh3376_dev01`; live-smoke surprise bugs get their
own fix-commit; push → auto-PR; sprint summary comment on PR.

## 8. Verification Checklist

- [ ] Simulated UserPromptSubmit (`.prompt`) → recall block + guidance emitted
- [ ] Legacy `.user_prompt` payload still honored (fallback)
- [ ] Guidance emitted even when recall returns 0 results
- [ ] Simulated PostToolUse failing build → `error` observation; passing →
      `progress`; empty output → NO success claim
- [ ] `tool_response` object shape (stdout/stderr) handled
- [ ] Live smoke: real session prompt shows CMS RECALL; real failed build
      lands an `error` observation in CMS (confirmed via API query)
- [ ] Both hook copies identical; hooks tests green; templates py_compile
- [ ] CHANGELOG, CLAUDE.md, roadmap, post.md updated

## 9. Documentation Update — Epic 5 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Claude Code payload schema differs from assumed (e.g. tool_response shape varies by tool) | Medium | Medium | Tier 3 against the REAL session is the acceptance bar, not simulated payloads; normalize string|object defensively |
| Newly-live recall floods every prompt with low-value context | Medium | Low | Existing top_k=5 + 15-char minimum prompt gate retained; observe in Tier 3 and tune only if needed |
| Guidance decoupling doubles hook latency on empty-recall prompts | Low | Low | Existing curl timeouts (2s connect / 8s max) bound it; warm-path is async server-side |
| Template/live drift recurs | High | Medium | Out-of-scope here but next sprint (HOOKSYNC-001) adds the CI parity gate; this sprint keeps both copies in one commit |

## 11. Documents Accessed

- `.claude/hooks/prompt-context.sh`, `post-tool-observe.py`, `pre-compact.sh`,
  `session-start.sh`, `pre-bash-check.py`, `pre-write-check.py`
- `internal/cli/hook_templates/*` + `internal/cli/hooks.go` (embed)
- `~/.claude/settings.json` + `.claude/settings.local.json` (registration)
- `docs/development/roadmap/ROADMAP_2026Q3.md` (HOOKWIRE-001 scope)
- Claude Code hooks reference (UserPromptSubmit / PostToolUse stdin schemas)

## 12. Rollback Procedures

Revert the epic commits — hooks return to the (dead) prior state; no data
migration involved. Observations recorded during the sprint remain valid.
