# JIMINY-PROCESS-OBSERVER-03 — Sprint Post

**Task**: #162
**Status**: **SHIPPED** — matcher + hook capture landed; live Tier-3 smoke PASSED end-to-end
**Sprint plan**: `docs/development/jiminy-process-observer-03/sprint_plan.md`
**Feature doc**: `docs/features/process-observation.md` §query-cms-first matcher (third observer)
**Predecessor**: JIMINY-PROCESS-OBSERVER-02 (#161/#667) — reused platform, zero schema change

## What shipped

Third observer in the JIMINY-PROCESS-OBSERVER-{01..06} arc per design §Phase E prioritization row 3. Grades the shipped `query-mdemg-cms-file-paths` Jiminy rule (correction — highest-volume ignored per JIMINY-CORPUS-AUDIT-004) via same-session ordering of two new event types.

### Epics (E1–E7)

| Epic | Delivery |
|---|---|
| E1 — Hook | Two new emit branches: `retrieval_call` (MCP retrieve tools OR Bash curl to /v1/memory/retrieve) + `filesystem_search` (Glob / Grep tools OR Bash search binaries word-boundary-anchored via module regex). HOOKSYNC-001 mirror. |
| E2 — Server | `filesystem_search` added to `allowedProcessEventTypes` (retrieval_call already allowlisted). |
| E3 — Config | `PROCESS_MATCHER_QUERY_CMS_FIRST_ENABLED` (default false; HEBB-ETA-001) + `PROCESS_MATCHER_QUERY_CMS_WINDOW_SEC` (default 300, floor 30). |
| E4 — Matcher | `internal/grader/process/matchers/query_cms_first.go` + 6 pin tests (prior→followed, no-prior→missed, empty-session→skip, default-window-fallback, reason-names-window, metadata). |
| E5 — Wiring | Registered in `server.go::StartSupervisedBackground` under new flag; boot log shows `matcher_count=3`. |
| E6 — Live Tier-3 smoke | 2 sessions; happy→followed with correct evidence_event_id, bad→missed with correct reason. Gauge = 0.833 (5/6 arithmetic). |
| E7 — Docs | Feature doc extended; CLAUDE.md pin; CHANGELOG; this sprint post. |

## Live Tier-3 smoke (2026-09-19)

Enabled `PROCESS_MATCHER_QUERY_CMS_FIRST_ENABLED=true` in `.env`, rebuilt binary, `launchctl kickstart -k`. Boot verified `matcher_count=3`. Two synthetic sessions emitted via POST /v1/process/event:

### Verdicts

```
smoke-cms-happy-*  →  retrieval_call → filesystem_search
                      MATCHER: process_followed
                      evidence: <retrieval event_id>
                      reason: "retrieval_call within 300s preceded filesystem_search"

smoke-cms-bad-*    →  filesystem_search (no prior retrieval)
                      MATCHER: process_missed
                      reason: "no retrieval_call on this session within 300s before the filesystem_search"
```

### Metric wire

Post-assessment tick:
```
mdemg_jiminy_follow_rate_process_verifiable{space_id="mdemg-dev"} = 0.833
```

Arithmetic check: **5** process_followed (4 lint_before_commit from natural hook traffic during this session + 1 query_cms_first happy) + **1** process_missed (query_cms_first bad) = 5 × 1.0 + 1 × 0.0 = 5 / 6 = **0.833** ✓

The natural-traffic contribution is a side benefit of live smoking after OBSERVER-01+02 ships: the operator's own Claude Code session has been emitting real `file_write` / `lint_run` / `git_commit` events, producing real `process_followed` verdicts — the observer is grading MY OWN COMMITS AS I WORK. Dogfooding validated.

### Cleanup

```
DELETE FROM process_events   WHERE session_id LIKE 'smoke-cms-%';  -- 3 rows
DELETE FROM process_outcomes WHERE session_id LIKE 'smoke-cms-%';  -- 2 rows
```

## Verification checklist

- [x] `go build ./...` clean
- [x] `golangci-lint run ./internal/grader/... ./internal/config/ ./internal/api/` → 0 issues
- [x] `go test ./internal/grader/process/matchers/... -count=1` → 23/23 pass (17 prior + 6 new)
- [x] All 5 drift checks clean (metrics/routes/config/tsdb/doc-envs)
- [x] Live Tier-3: happy=followed with correct evidence ✓ bad=missed with correct reason ✓
- [x] Post-smoke cleanup ✓
- [x] Natural hook traffic verified feeding the pipeline

## New arch rules pinned

1. **Event-source decision framework for new observers**: When designing the terminal + anchor events for a new matcher, prefer **hook-side emit** if (a) the rule targets AGENT behavior specifically (not internal-server behavior), and (b) session_id correlation is required. Cross-table read from existing hypertables (Option A) only if the source table has session_id AND the rule targets any-caller behavior. Server-side emit (Option C) adds hot-path cost — reserve for cases where hook-scoping actually misses meaningful events.
2. **MCP-tool detection in the hook uses substring match on the tool_name prefix `mcp__*`**: PostToolUse for MCP tools fires with names like `mcp__mdemg__memory_recall`. Substring-match on `memory_recall` / `memory_retrieve` / `__recall` catches any MCP server's retrieve-shape tool.
3. **Bash-side search-binary detection MUST be word-boundary-anchored**: `\b(rg|ripgrep|ag)\b|\bgrep\s+-\w*r|\bfind\s` — `pygrep`, `mygrep`, function names named `find_*` don't false-match. Module-level regex compilation avoids per-invocation cost.

## Follow-ups

- JIMINY-PROCESS-OBSERVER-04 (`unit-integration-e2e-docs`, hybrid) — sprint plan CONTENT parseable via file-tree walk + test-tier events from CI
- JIMINY-PROCESS-OBSERVER-05 (`must-use-uxts-frameworks-consistently`, hybrid) — file-path checker for new JSON schemas
- JIMINY-PROCESS-OBSERVER-06 (`never-haiku-for-planning`) — lowest priority per design
- Search-shape heuristic follow-up: if `query_cms_first` FP rate is unacceptable at T+7d, add a matcher-side "narrow search skip" (path scope + single-quoted literal → skip verdict for exact-token / CMS-miss-fallback cases)

## Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§Phase B + §Phase E row 3)
- `docs/development/jiminy-process-observer-{01,02}/{sprint_plan,sprint_post}.md`
- `docs/features/process-observation.md`
- `internal/grader/process/{grader,adapter}.go` + `matchers/{lint_before_commit,sequential_epics}.go`
- `internal/api/handlers_process.go` (allowlist)
- `internal/tsdb/migrations/*.sql` (retrieval_events schema check — no session_id column ruled out cross-table Option A)
- `.claude/hooks/post-tool-observe.py` + `internal/cli/hook_templates/post-tool-observe.py`
- CLAUDE.md pins: HEBB-ETA-001, HOOKSYNC-001, SUPERVISOR-002, JIMINY-PROCESS-OBSERVER-01/02, `query-mdemg-cms-file-paths` rule
