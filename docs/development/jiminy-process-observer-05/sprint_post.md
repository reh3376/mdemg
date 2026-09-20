# JIMINY-PROCESS-OBSERVER-05 — Sprint Post

**Task**: #164
**Status**: **SHIPPED** — matcher landed; live Tier-3 smoke PASSED
**Sprint plan**: `docs/development/jiminy-process-observer-05/sprint_plan.md`
**Feature doc**: `docs/features/process-observation.md` §uxts-frameworks matcher
**Predecessor**: JIMINY-PROCESS-OBSERVER-04 (#163 / merged #668)

## What shipped

Fifth observer. Grades `must-use-uxts-frameworks-consistently` via a filename-regex + path-substring check on `file_write` events. **No cross-table query, no filesystem read** — the leanest observer yet.

### Epics (E1–E5)

| Epic | Delivery |
|---|---|
| E1 — Config | `PROCESS_MATCHER_UXTS_FRAMEWORKS_ENABLED` (default false; HEBB-ETA-001) |
| E2 — Matcher | `internal/grader/process/matchers/uxts_frameworks.go` — regex `\.u[a-z]+\.json$` (11 UxTS families + any u-prefix future family) + substring check `docs/tests/u`. 6 pin tests (9-case regex table + path-OK / path-violation / skip / empty-path / nested-docs / metadata pins). |
| E3 — Wiring | Registered under new flag; boot log shows `matcher_count=5`. |
| E4 — Live smoke | 3 synthetic sessions; happy → followed, bad → incomplete, noise → skip (all as predicted). |
| E5 — Docs | Feature doc + CLAUDE.md pin + CHANGELOG + this sprint post. |

## Live Tier-3 smoke (mdemg-dev, 2026-09-19)

```
smoke-uxts-happy-*  →  file_path=docs/tests/uats/specs/smoke.uats.json
                       Verdict: process_followed
                       Reason: "UxTS-shape JSON ... under docs/tests/u* framework family dir"

smoke-uxts-bad-*    →  file_path=internal/schemas/rogue.uats.json
                       Verdict: process_incomplete
                       Reason: "UxTS-shape JSON ... outside docs/tests/u* framework family (should live under docs/tests/u<name>/)"

smoke-uxts-noise-*  →  file_path=internal/api/config.json
                       (no outcome row emitted — filename doesn't match `\.u[a-z]+\.json$`, skipped per fail-open)
```

### Cleanup

```
DELETE FROM process_events   WHERE session_id LIKE 'smoke-uxts-%';  -- 3 rows
DELETE FROM process_outcomes WHERE session_id LIKE 'smoke-uxts-%';  -- 2 rows
```

## Verification checklist

- [x] `go build ./...` clean
- [x] `go test ./internal/grader/process/matchers/... -count=1` → 38/38 pass (32 prior + 6 new)
- [x] `golangci-lint run` → 0 issues
- [x] All 5 drift checks clean
- [x] Live Tier-3: happy=followed ✓ bad=incomplete ✓ noise=skip ✓
- [x] Post-smoke cleanup ✓

## New arch rules pinned

1. **Leanest-observer pattern**: matchers can grade purely from `metadata.file_path` without cross-table queries or filesystem reads. `uxts_frameworks` is the shipped exemplar — just a regex + substring check. When a rule reduces to a filename/path-shape predicate, prefer this shape over cross-table joins or filesystem reads (both add attack surface + failure modes).
2. **UxTS framework family detection uses u-prefix + letters + `.json`** — `\.u[a-z]+\.json$` matches all 11 shipped families AND any future u-prefix framework. Path-side check uses substring `docs/tests/u` (not a strict directory match) so nested `docs/tests/u<name>/subdir/foo.uats.json` still grades followed. This is deliberately permissive — the spec pattern is `docs/tests/u*ts/`, and any nested path under a UxTS framework directory is compliant.
3. **Non-UxTS-shape JSON files are skipped, not missed** — `docs/tests/uats/specs/config.json` is a legitimate non-schema config file inside a UxTS dir; it doesn't grade `process_missed` (that would be a false positive on legitimate config files). Only files that LOOK like UxTS schemas get graded.

## Follow-ups

- JIMINY-PROCESS-OBSERVER-06 (`never-haiku-for-planning`) — lowest priority per design; single-event lookup on model_call event
- **Cross-file consistency audit** (design spec's "cross-file audit" side of the hybrid) — MVP is single-file; if a real consistency FP class emerges, a follow-up sprint could add cross-file schema-shape validation
- Optional: JSON-body content check for future-proofing (schema shape verification) — deferred; MVP shape-check is sufficient

## Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§B + §E row 5)
- `docs/development/jiminy-process-observer-{01,02,03,04}/{sprint_plan,sprint_post}.md`
- `docs/features/process-observation.md`
- `docs/tests/` (11 UxTS framework families enumerated live)
- `internal/grader/process/matchers/*.go` (structural mirror of sibling matchers)
- CLAUDE.md: `must-use-uxts-frameworks-consistently` rule, JIMINY-PROCESS-OBSERVER-{01..04}
