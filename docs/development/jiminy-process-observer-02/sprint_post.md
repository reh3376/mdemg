# JIMINY-PROCESS-OBSERVER-02 — Sprint Post

**Task**: #161
**Status**: **SHIPPED** — matcher + hook capture landed; live Tier-3 smoke PASSED end-to-end
**Sprint plan**: `docs/development/jiminy-process-observer-02/sprint_plan.md`
**Feature doc**: `docs/features/process-observation.md` §sequential-epics matcher (second observer)
**Predecessor**: JIMINY-PROCESS-OBSERVER-01 (#160/#657) — reused platform, zero schema change

## What shipped

Second observer in the JIMINY-PROCESS-OBSERVER-{01..06} arc per design §Phase E prioritization. Grades the shipped `sequential-epics` Jiminy rule via git-commit ordering + commit-message parse.

### Epics (E1–E6 complete)

| Epic | Delivery |
|---|---|
| E1 — Hook | `git log -1 --format=%B` capture into `metadata.commit_message` (bounded 500 chars, 2s timeout, fail-open). Mirror in `internal/cli/hook_templates/post-tool-observe.py` (HOOKSYNC-001 parity). |
| E2 — Config | `PROCESS_MATCHER_SEQUENTIAL_EPICS_ENABLED` default false in code (HEBB-ETA-001) |
| E3 — Matcher | `internal/grader/process/matchers/sequential_epics.go` + 17 pin tests (10-case extractEpicNumber table + 6 Grade scenarios + metadata pins) |
| E4 — Wiring | Registered in `server.go::StartSupervisedBackground` under the new flag; boot log names `matcher_count=2` |
| E5 — Live Tier-3 smoke | 3 synthetic sessions; verdicts + gauge exactly as predicted (see below) |
| E6 — Docs | Feature doc extended; CLAUDE.md pin; CHANGELOG; this sprint post |

## Live Tier-3 smoke (2026-09-19)

Emitted 3 sessions via `POST /v1/process/event` against the running server (pid updated after `launchctl kickstart -k` picked up the rebuilt binary + `.env` flag flip). Grader tick @30s + writer flush @30s. TSDB verified.

### Verdicts

```
smoke-seq-happy-*  →  process_followed  Epic 1
                      process_followed  Epic 2
                      process_followed  Epic 3
smoke-seq-bad-*    →  process_followed  Epic 1
                      process_followed  Epic 3
                      process_incomplete   "prior commit on session had Epic 3 but this commit has Epic 2"
smoke-seq-noise-*  →  (no outcome row — fail-open skip on missing Epic marker)
```

### Metric wire

```
mdemg_jiminy_follow_rate_process_verifiable{space_id="mdemg-dev"} = 0.916
```

Arithmetic check: 5 × `process_followed` (1.0) + 1 × `process_incomplete` (0.5) = 5.5 / 6 = **0.916** ✓

### Cleanup

Smoke rows removed:
```
DELETE FROM process_events   WHERE session_id LIKE 'smoke-seq-%';   -- 7 rows
DELETE FROM process_outcomes WHERE session_id LIKE 'smoke-seq-%';   -- 6 rows
```
Post-cleanup counts on both tables = 0.

## Verification checklist

- [x] `go build ./...` clean
- [x] `golangci-lint run ./internal/grader/... ./internal/config/ ./internal/api/` → 0 issues
- [x] `go test ./internal/grader/process/matchers/... -count=1` → 17/17 pass
- [x] All 5 drift checks clean (metrics/routes/config/tsdb/doc-envs)
- [x] Live Tier-3: happy=3× followed ✓ bad=1× incomplete on 3rd commit ✓ noise=0 rows ✓
- [x] `mdemg_jiminy_follow_rate_process_verifiable` gauge reflects added samples ✓
- [x] Post-smoke cleanup ✓
- [x] PR comment with smoke evidence (part of merge flow)

## New arch rules pinned

1. **Process-observer matchers MAY read arbitrary metadata columns via same-session prior-event queries** — `sequential_epics` reads `metadata->>'commit_message'` from the current terminal event AND queries prior same-session `git_commit` events for their metadata. The `PoolIface`/`Rows` narrow interface supports arbitrary SQL; matchers own their per-rule query shape. Fail-open on any query error (never break the grader loop for one matcher's bug).
2. **Fail-open skip is the correct verdict when required metadata is absent** — a `git_commit` with no `commit_message` field (e.g. `git log` shell-out failed) MUST NOT be graded as "missed"; skip silently. Same class as OBSERVER-01's "docs-only commit with no prior file_write" fail-open. When adding a new observer, decide up-front which absent-evidence classes are `skip` vs `process_missed`.
3. **Session-scope for sequential-epics is intentional** — the rule targets within-sprint parallel execution, and sessions in Claude Code map 1:1 to sprints in operator practice. Cross-session correlation is a different rule (would need PR-branch correlation via git tooling).

## Follow-ups

- JIMINY-PROCESS-OBSERVER-03 (`query-mdemg-cms-file-paths`) — memory-recall correlation with glob/grep on same session. Requires a new event type (`retrieval_call`).
- JIMINY-PROCESS-OBSERVER-04..-06 per design §Phase E prioritization
- Passive re-measure of `follow_rate_process_verifiable` @T+168h to see the gauge under natural traffic (the smoke rows were cleaned up; only real hook-emitted events feed the gauge going forward)

## Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§Phase B + §Phase E row 2)
- `docs/development/jiminy-process-observer-01/{sprint_plan,sprint_post}.md` (template + shipped smoke pattern)
- `docs/features/process-observation.md` (extension target)
- `internal/grader/process/{grader,adapter}.go` + `matchers/lint_before_commit.go` (structural mirror)
- `.claude/hooks/post-tool-observe.py` + `internal/cli/hook_templates/post-tool-observe.py`
- CLAUDE.md pins: HEBB-ETA-001, HOOKSYNC-001, SUPERVISOR-002, JIMINY-PROCESS-OBSERVER-01, `sequential-epics` rule
