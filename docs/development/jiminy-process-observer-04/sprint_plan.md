# JIMINY-PROCESS-OBSERVER-04 — Sprint Plan

**Task**: #163 (fourth in the JIMINY-PROCESS-OBSERVER-{01..06} arc)
**Predecessor**: JIMINY-PROCESS-OBSERVER-03 (#162 / merged #667) — reuses shipped platform, adds one new matcher + filesystem-read helper
**Wall-clock estimate**: ~2-3h
**Type**: shipping — one new matcher (with a new capability class: filesystem read) + config + tests + docs

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | JIMINY-PROCESS-OBSERVER-04 |
| Task # | #163 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | 0 — reuses V0036 process_events/process_outcomes; no schema change |
| Design source | `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase E row 4 (hybrid class) |
| Grades | `unit-integration-e2e-docs` rule |

## 2. Problem Statement

`unit-integration-e2e-docs` is a hybrid-class rule per the design taxonomy: "All development plans MUST include three testing tiers: unit tests, integration tests, e2e tests, plus documentation updates." The **classifier** can partially grade the shape of a sprint plan (does the doc mention the tiers?); the **process** side needs to verify the actual document at commit time meets the shape requirement. This sprint ships the process-side grader.

## 3. Scope & Constraints

**In scope:**
- New matcher `internal/grader/process/matchers/uied.go` (unit-integration-e2e-docs):
  - Terminal event: `git_commit`
  - Extract sprint code from `metadata.commit_message` via regex `\b([A-Z][A-Z0-9]+(?:-[A-Z0-9]+)+-\d+)\b`
  - Fail-open skip if no sprint code (chore / merge / non-sprint commit)
  - Kebab-lower the sprint code → look up `<SPRINT_DOCS_ROOT>/<kebab-lower>/sprint_plan.md` on disk
  - **File missing → `process_missed`** (sprint code referenced without a plan on disk)
  - **File present**: check for tier keywords in a case-insensitive way — `unit`, `integration`, and `e2e` (or `end-to-end`) all present anywhere in the file:
    - All three present → `process_followed`
    - Missing 1+ → `process_incomplete` (reason names the missing tier)
- New config knobs:
  - `PROCESS_MATCHER_UIED_ENABLED` (default false; HEBB-ETA-001)
  - `SPRINT_DOCS_ROOT` (default `docs/development` relative to CWD; ⚠️ operator-tunable per `never-hardcode-config`)
  - `PROCESS_MATCHER_UIED_MAX_FILE_BYTES` (default 200_000; safety cap on file read; skip if exceeded)
- Wire in `server.go::StartSupervisedBackground` under the new flag

**Out of scope:**
- CI test-tier events (the design spec's other half of the hybrid — separate follow-up if a CI-signal matcher becomes worthwhile)
- Parsing sprint plans that use non-standard filenames (`plan.md`, `spec.md`, etc.) — MVP is strict on `sprint_plan.md`
- Cross-repo / multi-branch correlation
- Documentation-tier check (rule says "plus documentation updates" — deferring; the 3 tiers alone are the primary signal)

**Constraints honored:** `plan-mode-before-change`, `unit-integration-e2e-docs` (this sprint IS the observer for it — dogfooding), `live-testing-tier-required`, `never-hardcode-config`, `mandatory-feature-docs`, `sequential-epics`, HEBB-ETA-001.

## 4. Dependencies

- OBSERVER-02+03 shipped 2026-09-19 ✅
- Hook already captures `metadata.commit_message` (OBSERVER-02 E1) ✅
- Matcher reads filesystem — the mdemg server process runs from the working-directory root and can read files under it

## 5. Implementation Plan (sequential epics)

- **E1** — Config: 3 new knobs.
- **E2** — Matcher `uied.go`:
  - Extract sprint code via regex
  - Skip on missing code
  - Read file (with `os.ReadFile` bounded by max-bytes)
  - Case-fold check for 3 tier keywords
  - Return one of {followed, incomplete, missed, skip}
  - 8 pin tests using `t.TempDir()` synthetic sprint plans (all 3 tiers → followed; missing tier → incomplete; file absent → missed; no sprint code → skip; over-cap file → skip; case-insensitive matching; e2e vs end-to-end alt; regex-anchor coverage)
- **E3** — Wire matcher under new flag in server.go.
- **E4** — Live Tier-3 smoke: seed a `git_commit` event on a synthetic session referencing a real sprint (this one). Verify `process_followed` verdict lands. Also seed a negative case with a bogus sprint code → `process_missed`.
- **E5** — Docs: extend `process-observation.md`, CLAUDE.md pin, CHANGELOG, sprint post.

## 6. Testing Plan (3 tiers — the rule this sprint grades, dogfooded)

- **Tier 1 — Unit tests**: 8 pin tests as above.
- **Tier 2 — Integration**: real filesystem read from tempdir with synthetic sprint plan content.
- **Tier 3 — Live e2e (mandatory per `live-testing-tier-required`)**: real mdemg server, synthetic git_commit event, verify TSDB `process_outcomes` row + gauge.

## 7. Commit Strategy

Two commits:
1. `feat(process): unit-integration-e2e-docs matcher + config + wire (JIMINY-PROCESS-OBSERVER-04 E1-E3)`
2. `docs(process-observation): uied observer + live smoke result (JIMINY-PROCESS-OBSERVER-04 E4-E5)`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./...` 0 issues
- [ ] Unit + integration tests green
- [ ] All 5 drift checks clean
- [ ] Live Tier-3 smoke: real sprint plan → followed; bogus code → missed
- [ ] Post-smoke cleanup
- [ ] Sprint post + docs

## 9. Documentation Update

- `docs/features/process-observation.md` extended (feature-doc rule)
- `CLAUDE.md` Architecture Note
- `CHANGELOG.md` new Added entry
- `docs/development/jiminy-process-observer-04/sprint_post.md`

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Matcher accesses filesystem — new capability class | Bounded via `MaxFileBytes` cap; read-only; path is under a config-driven root; single-path lookup (no recursive walk); errors return skip verdict (fail-open). |
| Sprint plan file uses different naming convention | Strict `sprint_plan.md` only; if the operator uses another convention, matcher grades `process_missed` — this is honest (the shipped convention IS `sprint_plan.md` per `project-planning-docs-in-repo-only` + `must-follow-12-section-format` rules). |
| Sprint code regex over-matches (e.g. an issue number in commit body) | Anchor on word-boundary + require at least one hyphen (differentiating from ISSUE-123 style single-word codes); allowlist expected shape `PREFIX-WORD-...-###`. |
| Historical sprints without sprint plans | Ship default-OFF; the operator flips the flag when appropriate; historical commits without plans won't grade unless the flag is on AND grader reprocesses them (which it won't — grader only anchors on FRESH terminal events per its startup-lookback config). |

## 11. Rollback

Flag flip: `PROCESS_MATCHER_UIED_ENABLED=false` + kickstart. No substrate mutation to undo.

## 12. Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§B + §E row 4)
- `docs/development/jiminy-process-observer-{01,02,03}/{sprint_plan,sprint_post}.md`
- `docs/features/process-observation.md`
- `internal/grader/process/{grader,adapter}.go` + `matchers/{lint_before_commit,sequential_epics,query_cms_first}.go`
- `internal/api/server.go::StartSupervisedBackground` (wiring)
- CLAUDE.md: HEBB-ETA-001, JIMINY-PROCESS-OBSERVER-{01,02,03}, `unit-integration-e2e-docs` rule, `must-follow-12-section-format`, `project-planning-docs-in-repo-only`
