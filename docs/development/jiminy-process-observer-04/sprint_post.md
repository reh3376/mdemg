# JIMINY-PROCESS-OBSERVER-04 — Sprint Post

**Task**: #163
**Status**: **SHIPPED** — matcher landed; live Tier-3 smoke PASSED
**Sprint plan**: `docs/development/jiminy-process-observer-04/sprint_plan.md`
**Feature doc**: `docs/features/process-observation.md` §unit-integration-e2e-docs (UIED) matcher
**Predecessor**: JIMINY-PROCESS-OBSERVER-03 (#162 / merged #667) — reused platform, no schema change

## What shipped

Fourth observer in the arc. Grades the shipped `unit-integration-e2e-docs` Jiminy rule (hybrid class) by reading the sprint plan file for the sprint code named in the commit message.

### Epics (E1–E5)

| Epic | Delivery |
|---|---|
| E1 — Config | 3 new knobs (`PROCESS_MATCHER_UIED_ENABLED`, `SPRINT_DOCS_ROOT`, `PROCESS_MATCHER_UIED_MAX_FILE_BYTES` with 4096 floor) |
| E2 — Matcher | `internal/grader/process/matchers/uied.go` — sprint code extraction, filesystem read (bounded), case-fold tier detection with e2e/end-to-end alt. 9 pin tests via `t.TempDir()`. |
| E3 — Wiring | Registered under new flag; boot log confirms `matcher_count=4`. |
| E4 — Live smoke | 2 synthetic sessions on real filesystem via POST /v1/process/event. Real plan → followed; bogus code → missed. |
| E5 — Docs | Feature doc extended; CLAUDE.md pin; CHANGELOG; this sprint post. |

## Live Tier-3 smoke (2026-09-19)

```
smoke-uied-happy-*  →  commit_message: "feat(process): live smoke JIMINY-PROCESS-OBSERVER-04 all tiers present"
                       Matcher extracted sprint code: JIMINY-PROCESS-OBSERVER-04
                       Kebab-lower: jiminy-process-observer-04
                       Read: docs/development/jiminy-process-observer-04/sprint_plan.md (real file, this sprint's plan)
                       Verdict: process_followed
                       Reason: "sprint plan has all 3 testing tiers: unit, integration, e2e"

smoke-uied-bad-*    →  commit_message: "feat: BOGUS-SPRINT-NEVER-EXISTS-999 nothing here"
                       Matcher extracted sprint code: BOGUS-SPRINT-NEVER-EXISTS-999
                       Kebab-lower: bogus-sprint-never-exists-999
                       File lookup: docs/development/bogus-sprint-never-exists-999/sprint_plan.md (absent)
                       Verdict: process_missed
                       Reason: "sprint code BOGUS-SPRINT-NEVER-EXISTS-999 referenced but no sprint_plan.md at ..."
```

Post-tick gauge `mdemg_jiminy_follow_rate_process_verifiable = 0.769` — moved with added samples.

⚠️ **Dogfooding**: the matcher successfully read the sprint plan this same sprint just wrote, parsed the §Testing Plan section, and graded it followed. The observer that grades sprint plans just graded ITS OWN sprint plan.

### Cleanup

```
DELETE FROM process_events   WHERE session_id LIKE 'smoke-uied-%';  -- 2 rows
DELETE FROM process_outcomes WHERE session_id LIKE 'smoke-uied-%';  -- 2 rows
```

## Verification checklist

- [x] `go build ./...` clean
- [x] `golangci-lint run ./internal/grader/... ./internal/config/ ./internal/api/` → 0 issues
- [x] `go test ./internal/grader/process/matchers/... -count=1` → 32/32 pass (23 prior + 9 new)
- [x] All 5 drift checks clean
- [x] Live Tier-3: happy=followed reading real plan ✓ bad=missed with correct reason ✓
- [x] Post-smoke cleanup ✓

## New arch rules pinned

1. **Filesystem-read is a valid matcher capability class when bounded by a config-driven root + max-file-bytes cap** — mdemg-server process filesystem access exists (the working directory is trusted); the risk is unbounded reads or path traversal. UIED constrains both: path is fully constructed from regex-validated sprint code (only `[A-Z][A-Z0-9-]+-\d+` chars) joined under a config-driven root (`SprintDocsRoot`); no directory walks; single-path lookup; over-cap files skip. `//nolint:gosec` on the ReadFile call documents this safe shape.
2. **Sprint code regex requires ≥1 hyphen** to differentiate multi-word sprint codes (`JIMINY-PROCESS-OBSERVER-04`) from issue-number style (`ISSUE-123`). The CVE-style (`CVE-2026-XYZ`) matches but fail-open at file-lookup — documented false-positive class, doesn't produce false verdicts.
3. **`e2e` / `end-to-end` / `end to end` all satisfy the same tier check** — tier-marker detection MUST use case-fold + alt spellings; sprint plans in the wild use all three shapes.

## Follow-ups

- JIMINY-PROCESS-OBSERVER-05 (`must-use-uxts-frameworks-consistently`, hybrid) — file-path checker for new JSON schemas outside `docs/tests/u*ts/`
- JIMINY-PROCESS-OBSERVER-06 (`never-haiku-for-planning`) — lowest priority per design; single-event lookup on model_call event
- Documentation tier check (deferred from scope) — rule says "plus documentation updates". Could add a 4th tier check: sprint plan mentions `docs/features/` OR `docs/user/` OR CHANGELOG.md updates. Ship default-OFF as an add-on if a real signal need emerges.

## Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§B + §E row 4)
- `docs/development/jiminy-process-observer-{01,02,03}/{sprint_plan,sprint_post}.md`
- `docs/features/process-observation.md`
- `internal/grader/process/{grader,adapter}.go` + `matchers/{lint_before_commit,sequential_epics,query_cms_first}.go`
- `internal/api/server.go::StartSupervisedBackground`
- CLAUDE.md: HEBB-ETA-001, JIMINY-PROCESS-OBSERVER-{01,02,03}, `unit-integration-e2e-docs` rule, `must-follow-12-section-format`, `project-planning-docs-in-repo-only`
