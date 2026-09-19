# JIMINY-PROCESS-OBSERVER-02 — Sprint Plan

**Task**: #161 (next in the JIMINY-PROCESS-OBSERVER-{01..06} arc)
**Predecessor**: JIMINY-PROCESS-OBSERVER-01 (#160/#657) — reuses the shipped platform (V0036 hypertables, HTTP endpoint, grader loop, hook emitter, metric wire) without further schema changes.
**Wall-clock estimate**: ~2-3h (per design spec §B5)
**Type**: shipping — one new matcher + one config flag + one small hook extension + tests + docs

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | JIMINY-PROCESS-OBSERVER-02 |
| Task # | #161 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | 0 — reuses V0036 `process_events`/`process_outcomes`; no migration; no schema bump |
| Design source | `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase E row 2 |
| Grades | `sequential-epics` rule |

## 2. Problem Statement

`sequential-epics` is the second-priority process-verifiable rule per the design spec: "Execute sprint epics SEQUENTIALLY — Epic N MUST complete fully before Epic N+1 begins." The classifier structurally cannot verify this from action-text; the process-observation platform can, via git-commit ordering + commit-message parsing. Currently the `process_verifiable` gauge is fed only by `lint-before-commit`; adding `sequential-epics` broadens the sample.

## 3. Scope & Constraints

**In scope:**
- Hook extension in `.claude/hooks/post-tool-observe.py` (+ template mirror): on `git_commit`, capture the commit message via `git log -1 --format=%B` (2s timeout, fail-open) into `metadata.commit_message` (bounded to 500 chars).
- New matcher `internal/grader/process/matchers/sequential_epics.go`:
  - Terminal event: `git_commit`
  - Extract `Epic N` marker via `\b[Ee]pic\s+(\d+)\b`
  - Fail-open skip when no marker
  - Query same-session prior `git_commit` events; extract max prior Epic N
  - Verdicts: prior ≤ current → `process_followed`; prior > current → `process_incomplete` (out-of-order)
- New flag `PROCESS_MATCHER_SEQUENTIAL_EPICS_ENABLED` (default false; `.env` opt-in after live smoke)
- Wired via `g.Register(procmatchers.NewSequentialEpics())` in `server.go::StartSupervisedBackground`

**Out of scope:** parsing actual sprint-plan documents; multi-repo / cross-session correlation; amended-commit / rebase-rewrite handling; observers 03-06.

**Constraints honored:** `sequential-epics` (dogfooding — E1→E6), `plan-mode-before-change`, `unit-integration-e2e-docs`, `live-testing-tier-required`, `never-hardcode-config`, `mandatory-feature-docs`, HOOKSYNC-001, HEBB-ETA-001.

## 4. Dependencies

- OBSERVER-01 platform live-verified 2026-09-19 ✅
- V0036 schema at version 36 ✅
- `PROCESS_EVENTS_ENABLED=true` + `PROCESS_GRADER_ENABLED=true` in `.env` ✅

## 5. Implementation Plan (sequential epics)

- **E1** — Hook: capture `commit_message` via `git log -1 --format=%B`; truncate 500 chars; add to `git_commit` event `metadata`. Mirror in template (HOOKSYNC-001).
- **E2** — Config knob `PROCESS_MATCHER_SEQUENTIAL_EPICS_ENABLED` in `internal/config/config.go`; default-off pin test.
- **E3** — Matcher `internal/grader/process/matchers/sequential_epics.go` + 6 pin tests (no-marker skip, monotonic, equal-N, out-of-order, no-priors, malformed-marker skip).
- **E4** — Wire in `server.go::StartSupervisedBackground` under the new flag; boot log names enabled matchers.
- **E5** — Live Tier-3 smoke: 3 synthetic sessions (happy monotonic 1→2→3; bad 1→3→2 with expected `process_incomplete` on 3rd; noise no-marker with expected skip).
- **E6** — Docs: extend `docs/features/process-observation.md` §Observer catalog; CLAUDE.md Architecture Note; CHANGELOG entry; sprint post.

## 6. Testing Plan (3 tiers)

- **Tier 1** — Unit: 6 matcher tests + default-off config pin.
- **Tier 2** — Integration: grader with `capturingPool` scanning a synthetic 3-commit sequence.
- **Tier 3** — Live e2e (mandatory): real mdemg server + 3 sessions via `POST /v1/process/event`, 45s wait, verify `process_outcomes` rows + `mdemg_jiminy_follow_rate_process_verifiable` gauge.

## 7. Commit Strategy

Two commits:
1. `feat(process): sequential-epics matcher + hook commit_message capture (JIMINY-PROCESS-OBSERVER-02 E1-E4)`
2. `docs(process-observation): sequential-epics observer + smoke result (JIMINY-PROCESS-OBSERVER-02 E5+E6)`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./...` 0 issues
- [ ] Unit + integration tests green
- [ ] All 5 drift checks clean (metrics/routes/config/tsdb/doc-envs)
- [ ] Live Tier-3 smoke: happy=3× followed, bad=1× incomplete on 3rd commit, noise=0 rows
- [ ] `mdemg_jiminy_follow_rate_process_verifiable` gauge reflects the added samples
- [ ] Post-smoke cleanup
- [ ] PR comment with smoke evidence

## 9. Documentation Update

- `docs/features/process-observation.md` extended (feature-doc rule)
- `CLAUDE.md` Architecture Note (references OBSERVER-01 for the platform)
- `CHANGELOG.md` new Added entry
- `docs/development/jiminy-process-observer-02/sprint_post.md`

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| `git log -1` subprocess cost on every commit | Bounded 2s timeout, fail-open |
| Session-scope mis-attributes cross-sprint commits | Same scope OBSERVER-01 already uses; inherit its semantics |
| Regex over-match on `Epic N` in a paste | Word-boundary anchors; if dogfooding surfaces false-matches, tighten to header-line only |

## 11. Rollback

Flag flip: `PROCESS_MATCHER_SEQUENTIAL_EPICS_ENABLED=false` + kickstart. No substrate mutation to undo.

## 12. Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§B + §E row 2)
- `docs/development/jiminy-process-observer-01/{sprint_plan,sprint_post}.md` (template + smoke pattern)
- `docs/features/process-observation.md` (extension target)
- `internal/grader/process/grader.go` + `matchers/lint_before_commit.go` (shape to mirror)
- `.claude/hooks/post-tool-observe.py` + `internal/cli/hook_templates/post-tool-observe.py`
- CLAUDE.md: HEBB-ETA-001, HOOKSYNC-001, SUPERVISOR-002, JIMINY-PROCESS-OBSERVER-01, `sequential-epics` rule
