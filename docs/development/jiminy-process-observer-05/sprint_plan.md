# JIMINY-PROCESS-OBSERVER-05 — Sprint Plan

**Task**: #164 (fifth in the JIMINY-PROCESS-OBSERVER-{01..06} arc)
**Predecessor**: JIMINY-PROCESS-OBSERVER-04 (#163 / merged #668) — reuses shipped platform, no schema change, no filesystem read
**Wall-clock estimate**: ~2h
**Type**: shipping — one new matcher + config + tests + docs

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | JIMINY-PROCESS-OBSERVER-05 |
| Task # | #164 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` |
| Substrate touch? | 0 |
| Design source | `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase E row 5 (hybrid class) |
| Grades | `must-use-uxts-frameworks-consistently` rule |

## 2. Problem Statement

`must-use-uxts-frameworks-consistently` (hybrid): "When creating a JSON schema, contract, or test spec that will be used repeatedly, use the UxTS framework family." The design spec's classifier side is "new JSON schema files outside `docs/tests/u*ts/`"; this observer ships the process side — a filename-shape + path-prefix check on `file_write` events.

## 3. Scope & Constraints

**In scope:**
- New matcher `internal/grader/process/matchers/uxts_frameworks.go`:
  - Terminal event: `file_write` (emitted by OBSERVER-01 hook for Write/Edit tools)
  - Extract `metadata.file_path`
  - **Filename regex** `\.u[a-z]+\.json$` (u-prefix + letters + `.json`) — matches the 11 shipped UxTS framework variants (uaits, uams, ubench, ubts, uets, uits, ults, uobs, usts, utds, uvts) + any future u-prefix framework
  - **Skip** if filename doesn't match (not a UxTS-shape JSON)
  - **Path check**: does path start with (or contain) `docs/tests/u`?
    - Path OK → `process_followed` (reason: filename matched under UxTS framework dir)
    - Path violation → `process_incomplete` (reason: names the violating path)
- New config: `PROCESS_MATCHER_UXTS_FRAMEWORKS_ENABLED` (default false; HEBB-ETA-001)
- Wire in `server.go::StartSupervisedBackground`

**Out of scope:**
- Cross-file "consistency" audit (design spec mentions this — matcher would need to see ALL new UxTS files together and check they use the same schema shape / conventions). MVP is single-file-write per-terminal-event; cross-file audit is a future refinement.
- Non-JSON UxTS artifacts (Python, YAML, etc.)
- Observers 06

**Constraints honored:** `plan-mode-before-change`, `unit-integration-e2e-docs`, `live-testing-tier-required`, `never-hardcode-config`, `mandatory-feature-docs`, `sequential-epics`, HEBB-ETA-001, `must-use-uxts-frameworks-consistently` (dogfooding — this sprint's plan lives in the shipped UxTS-framework family dir).

## 4. Dependencies

- OBSERVER-01+02+03+04 shipped 2026-09-19 ✅
- `file_write` events emitted by shipped hook (OBSERVER-01) ✅
- Matcher works on `metadata.file_path` — no filesystem read; no cross-table query.

## 5. Implementation Plan (sequential epics)

- **E1** — Config: 1 new knob.
- **E2** — Matcher `uxts_frameworks.go` + 6 pin tests (filename match variants, non-match skip, path-violation, path-OK, edge cases like nested `docs/tests/u...` paths, metadata pins).
- **E3** — Wire under new flag in server.go.
- **E4** — Live Tier-3 smoke: 3 sessions (happy `docs/tests/uats/specs/x.uats.json`; bad `internal/foo/schema.uats.json`; noise `internal/foo/config.json`).
- **E5** — Docs.

## 6. Testing Plan (3 tiers)

- **Tier 1 — Unit tests**: 6 pin tests.
- **Tier 2 — Integration**: grader with capturingPool on synthetic file_write events.
- **Tier 3 — Live e2e (mandatory)**: real POST /v1/process/event, 45s wait, verify outcomes + gauge.

## 7. Commit Strategy

Two commits:
1. `feat(process): uxts-frameworks matcher + config + wire (JIMINY-PROCESS-OBSERVER-05 E1-E3)`
2. `docs(process-observation): uxts-frameworks observer + smoke result (JIMINY-PROCESS-OBSERVER-05 E4-E5)`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./...` 0 issues
- [ ] Unit tests green
- [ ] All 5 drift checks clean
- [ ] Live Tier-3 smoke: happy=followed, bad=incomplete, noise=skip
- [ ] Post-smoke cleanup
- [ ] PR comment with smoke evidence

## 9. Documentation Update

- `docs/features/process-observation.md` extended
- `CLAUDE.md` Architecture Note
- `CHANGELOG.md` new Added entry
- `docs/development/jiminy-process-observer-05/sprint_post.md`

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Regex over-matches `.uxxx.json` files that aren't UxTS specs | Path check catches these — a legitimate config.json in `docs/tests/uats/` still requires the filename shape; a coincidentally-named `.uapi.json` in `internal/` is graded incomplete (rare + honest signal). If a real FP class emerges, tighten with a schema-content check follow-up. |
| Sprint code false-positive class not applicable here | No sprint code parsing this observer |
| Edit-only-updates to existing schemas | Both Write and Edit fire `file_write`; both are graded — a schema file MOVED out of `docs/tests/u.../` via Edit path would grade `process_incomplete` (honest). |

## 11. Rollback

Flag flip: `PROCESS_MATCHER_UXTS_FRAMEWORKS_ENABLED=false` + kickstart.

## 12. Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§B + §E row 5)
- `docs/development/jiminy-process-observer-{01,02,03,04}/{sprint_plan,sprint_post}.md`
- `docs/features/process-observation.md`
- `docs/tests/` directory listing (11 UxTS framework families confirmed)
- `internal/grader/process/matchers/{lint_before_commit,sequential_epics,query_cms_first,uied}.go`
- CLAUDE.md: `must-use-uxts-frameworks-consistently` rule, JIMINY-PROCESS-OBSERVER-{01,02,03,04}
