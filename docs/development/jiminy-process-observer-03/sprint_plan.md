# JIMINY-PROCESS-OBSERVER-03 — Sprint Plan

**Task**: #162 (third in the JIMINY-PROCESS-OBSERVER-{01..06} arc)
**Predecessor**: JIMINY-PROCESS-OBSERVER-02 (#161/#667) — reuses shipped platform, adds two new event types
**Wall-clock estimate**: ~3h
**Type**: shipping — one new matcher + hook extension (2 new event types) + config + tests + docs

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | JIMINY-PROCESS-OBSERVER-03 |
| Task # | #162 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | 0 — reuses V0036 process_events/process_outcomes |
| Design source | `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase E row 3 |
| Grades | `query-mdemg-cms-file-paths` rule |

## 2. Problem Statement

`query-mdemg-cms-file-paths` (correction — highest-volume ignored rule per JIMINY-CORPUS-AUDIT-004): "When discovering unfamiliar code structure, query MDEMG CMS retrieval FIRST; glob/grep only for exact-token or CMS-miss fallback." Classifier cannot verify from action-text alone (a Glob call doesn't say whether the agent tried retrieval first); process observation can via same-session tool-invocation ordering.

## 3. Scope & Constraints

**In scope (hook + matcher):**
- Hook extension `.claude/hooks/post-tool-observe.py` + template mirror (HOOKSYNC-001):
  - **New emit** `retrieval_call` on:
    - `tool_name` matches `mcp__*memory_recall*` or `mcp__*memory_retrieve*` (agent-side MCP retrieve)
    - `Bash` command matches `/v1/memory/retrieve` (agent-side curl)
  - **New emit** `filesystem_search` on:
    - `tool_name` == `Glob` (subtype=`glob`)
    - `tool_name` == `Grep` (subtype=`grep`)
    - `Bash` command matching search binaries (`\brg\b`, `\bripgrep\b`, `\bgrep\s+-r`, `\bfind\s`, `\bag\b`) — subtype=`bash-search`
- Server-side allowlist: add `filesystem_search` to `allowedProcessEventTypes` in `internal/api/handlers_process.go` (`retrieval_call` already allowed).
- New matcher `internal/grader/process/matchers/query_cms_first.go`:
  - Terminal event: `filesystem_search`
  - Query same-session prior `retrieval_call` events within `RETRIEVAL_LOOKBACK_SEC` window
  - ≥1 prior retrieval → `process_followed`
  - 0 prior retrievals → `process_missed`
- New config: `PROCESS_MATCHER_QUERY_CMS_FIRST_ENABLED` (default false) + `PROCESS_MATCHER_QUERY_CMS_WINDOW_SEC` (default 300, floor 30)
- Wire in `server.go::StartSupervisedBackground` under the new flag

**Out of scope:**
- Distinguishing "exact-token search" (CMS-miss fallback territory — legitimate exception) from "broad-discovery search" — MVP grades both alike as `process_missed` when no retrieval preceded. Refinement is a follow-up sprint if false-positive rate is unacceptable.
- Recording internal-server retrievals (hook path is agent-scoped by design)
- Observers 04-06

**Constraints honored:** `plan-mode-before-change`, `unit-integration-e2e-docs`, `live-testing-tier-required`, `never-hardcode-config`, `mandatory-feature-docs`, `sequential-epics`, HOOKSYNC-001, HEBB-ETA-001.

## 4. Dependencies

- OBSERVER-02 shipped 2026-09-19 ✅
- V0036 schema at version 36 ✅
- `PROCESS_EVENTS_ENABLED=true` + `PROCESS_GRADER_ENABLED=true` in `.env` ✅
- `session_id` naturally flows from hook stdin (SESSION_ID resolved same as sibling observers)

## 5. Implementation Plan (sequential epics)

- **E1** — Hook: two new emission branches (retrieval_call + filesystem_search) + template mirror.
- **E2** — Server allowlist: add `filesystem_search` to `allowedProcessEventTypes`.
- **E3** — Config: 2 new knobs (enable flag + window seconds).
- **E4** — Matcher: `query_cms_first.go` + 6 pin tests (no-priors → missed, prior-retrieval → followed, out-of-window → missed, cross-session isolation, multiple prior retrievals → followed with newest evidence, metadata pins).
- **E5** — Wire in server.go under the new flag.
- **E6** — Live Tier-3 smoke: 3 sessions (happy retrieval→search=followed; bad search-alone=missed; noise = search without any prior events = missed too since no retrievals — different scenario from OBSERVER-02).
- **E7** — Docs: extend `process-observation.md`, CLAUDE.md pin, CHANGELOG, sprint post.

## 6. Testing Plan (3 tiers)

- **Tier 1** — Unit: 6 matcher tests + config default pin.
- **Tier 2** — Grader loop w/ capturingPool for synthetic session.
- **Tier 3** — Live e2e (mandatory): 3 synthetic sessions via POST /v1/process/event; wait 45s; verify TSDB rows + gauge.

## 7. Commit Strategy

Two commits:
1. `feat(process): query-cms-first matcher + hook retrieval_call/filesystem_search events (JIMINY-PROCESS-OBSERVER-03 E1-E5)`
2. `docs(process-observation): query-cms-first observer + smoke result (JIMINY-PROCESS-OBSERVER-03 E6+E7)`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./...` 0 issues
- [ ] Unit tests green
- [ ] All 5 drift checks clean
- [ ] Live Tier-3 smoke: happy=followed, bad=missed
- [ ] Post-smoke cleanup
- [ ] Gauge sample rate reflects added observations

## 9. Documentation Update

- `docs/features/process-observation.md` extended
- `CLAUDE.md` Architecture Note
- `CHANGELOG.md` new entry
- `docs/development/jiminy-process-observer-03/sprint_post.md`

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Rule over-triggers (exact-token search classed as missed) | Ship default-OFF; operator flip after passive observation. If FP rate unacceptable, add a search-shape heuristic (path-scope + single-quoted literal) as a follow-up sprint. |
| MCP tool name detection misses variants | Hook regex is broad (`memory_recall`\|`memory_retrieve`\|`recall`); false-negatives (event unemitted) fail-safe under matcher's `process_missed` verdict; false-positives (event over-emitted) unlikely given the specific name pattern. |
| Time-window tuning | Config knob `RETRIEVAL_LOOKBACK_SEC` (300 default = 5 min) matches typical operator context switch. Floor 30s. |

## 11. Rollback

Flag flip: `PROCESS_MATCHER_QUERY_CMS_FIRST_ENABLED=false` + kickstart. No substrate mutation.

## 12. Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§B + §E row 3)
- `docs/development/jiminy-process-observer-{01,02}/{sprint_plan,sprint_post}.md`
- `docs/features/process-observation.md`
- `internal/grader/process/{grader,adapter}.go` + `matchers/{lint_before_commit,sequential_epics}.go`
- `internal/api/handlers_process.go` (allowlist)
- `.claude/hooks/post-tool-observe.py` + `internal/cli/hook_templates/post-tool-observe.py`
- CLAUDE.md: HEBB-ETA-001, HOOKSYNC-001, JIMINY-PROCESS-OBSERVER-01/02, `query-mdemg-cms-file-paths` rule
