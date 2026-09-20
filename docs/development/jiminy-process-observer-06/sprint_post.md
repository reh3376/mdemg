# JIMINY-PROCESS-OBSERVER-06 — Sprint Post

**Task**: #165
**Ship date**: 2026-09-19
**Wall-clock**: ~1.5h
**Status**: SHIPPED default-off in code, enabled in `.env` after live smoke
**Arc**: JIMINY-PROCESS-OBSERVER-{01..06} — **FINAL / CLOSED**

## Goal delivered

Sixth and final observer in the process-observation arc. Grades the operator-authored `never-haiku-for-planning` Jiminy rule ("When planning complex features or fixes, ALWAYS use the most advanced coding model (Opus). Haiku only for simple mechanical tasks.") via the leanest possible matcher shape: **one row of `process_events.metadata`** — no cross-table query, no filesystem read, no same-session lookup.

## What shipped

| Component | File | Change |
|---|---|---|
| Hook emitter (both copies, HOOKSYNC-001 parity) | `.claude/hooks/post-tool-observe.py` + `internal/cli/hook_templates/post-tool-observe.py` | Emit `model_call` event with `event_subtype='planning'` + `metadata.model = os.environ['ANTHROPIC_MODEL']` on Write of `sprint_plan.md` OR `plan.md` |
| Matcher | `internal/grader/process/matchers/never_haiku_planning.go` | ~90 LOC; scans one row of `metadata->>'model'`; case-insensitive `contains 'haiku'` → `process_incomplete`; else → `process_followed`; empty model / non-planning subtype → fail-open skip |
| Tests | `internal/grader/process/matchers/never_haiku_planning_test.go` | 6 pins: opus followed, haiku incomplete, sonnet followed, non-planning subtype skip, empty-model skip, metadata contract |
| Config | `internal/config/config.go` | 1 new flag `PROCESS_MATCHER_NEVER_HAIKU_ENABLED` (default false; HEBB-ETA-001) |
| Wire | `internal/api/server.go::StartSupervisedBackground` | Register matcher under the new flag; boot log now shows `matcher_count=6` |
| Env | `.env` | `PROCESS_MATCHER_NEVER_HAIKU_ENABLED=true` |
| Docs | `docs/features/process-observation.md` | §"Observer 06" + §"JIMINY-PROCESS-OBSERVER arc closeout" |
| Docs | `CLAUDE.md` | Pinned arch rule (metadata-only matcher shape) + arc-closeout summary |
| Docs | `CHANGELOG.md` | Unreleased/Added entry for #165 |

## Live smoke (mdemg-dev)

Boot log confirmed all six matchers wired:
```
matcher_count=6 …
```

Two synthetic sessions emitted via `POST /v1/process/event`:

| session_id | metadata.model | expected verdict | observed verdict | reason |
|---|---|---|---|---|
| `smoke-haiku-happy-<ts>` | `claude-opus-4-7` | `process_followed` | ✓ `process_followed` | "planning model was claude-opus-4-7 (non-haiku)" |
| `smoke-haiku-bad-<ts>` | `claude-haiku-4-5` | `process_incomplete` | ✓ `process_incomplete` | "planning model was claude-haiku-4-5 (contains 'haiku'; rule requires Opus / Sonnet for planning)" |

Smoke rows cleaned via `DELETE FROM process_outcomes WHERE session_id LIKE 'smoke-haiku-%'` + same on `process_events`.

## Verification

- `go build ./...` clean
- `go test ./internal/grader/process/matchers/... -count=1 -run NeverHaiku` — 6/6 pass
- `golangci-lint run ./internal/grader/... ./internal/config/ ./internal/api/` — 0 issues
- All 5 drift checks clean (metrics / routes / config / tsdb / doc-envs)
- Production `llama-server` on port 8102 UNTOUCHED throughout

## Arc closeout — JIMINY-PROCESS-OBSERVER-{01..06}

**Arc completion date**: 2026-09-19
**Wall-clock**: 4 days (2026-09-18 → 2026-09-19; OBSERVER-01 shipped 2026-09-18, OBSERVER-02..-06 shipped 2026-09-19)

### Observer scoreboard

| # | Sprint | Rule graded | Terminal event | Capability class | New event type |
|---|--------|-------------|----------------|------------------|-----------------|
| 01 | JIMINY-PROCESS-OBSERVER-01 | `lint-before-commit` | `git_commit` | same-session join | `lint_run`, `git_commit`, `file_write` (platform primitives) |
| 02 | JIMINY-PROCESS-OBSERVER-02 | `sequential-epics` | `git_commit` | same-session + metadata parse | none (reused git_commit + metadata.commit_message) |
| 03 | JIMINY-PROCESS-OBSERVER-03 | `query-mdemg-cms-file-paths` | `filesystem_search` | same-session join (timed window) | `retrieval_call`, `filesystem_search` |
| 04 | JIMINY-PROCESS-OBSERVER-04 | `unit-integration-e2e-docs` | `git_commit` | filesystem read (bounded) | none |
| 05 | JIMINY-PROCESS-OBSERVER-05 | `must-use-uxts-frameworks-consistently` | `file_write` | metadata regex + substring | none (reused file_write + metadata.file_path) |
| 06 | JIMINY-PROCESS-OBSERVER-06 | `never-haiku-for-planning` | `model_call` | metadata-only | `model_call` (with event_subtype='planning') |

### Platform totals

- **Hypertables**: 2 (`process_events`, `process_outcomes`) — V0036 migration, no schema change since OBSERVER-01
- **Endpoints**: 1 (`POST /v1/process/event`) — added in OBSERVER-01, untouched since
- **Grader loop**: 1 supervised worker (SUPERVISOR-002 nil-return contract; per-matcher lastGraded tracking)
- **Emitter event types actually used**: 5 (`git_commit`, `filesystem_search`, `retrieval_call`, `file_write`, `model_call`)
- **Config knobs**: 13 (all default-off in code AND `.env` — HEBB-ETA-001)
- **Matcher files**: 6 at ~90 LOC each = ~540 LOC across all matchers
- **Downstream wiring changed**: 1 (OBSERVER-01 UNIONed `process_outcomes` into `DatasetBuilder.GuidanceEffectivenessByClass` — every subsequent observer inherits gauge emission for free)

### Marginal-cost proof

OBSERVER-02..-06 average sprint cost: **~2h wall-clock each**, containing:
- 1 new matcher file (~90 LOC)
- 1 new config flag
- 1-6 pin tests
- Optional 1 new event type in the hook
- Feature doc extension + CLAUDE.md pin + CHANGELOG entry
- Live Tier-3 smoke

Zero platform-shape changes across observers 02-06. The design goal of "each subsequent observer is a lightweight sprint reusing the OBSERVER-01 platform" was validated.

### Downstream impact

`mdemg_jiminy_follow_rate_process_verifiable` now has 6 live producers. Every rule the design spec (JIMINY-METRIC-DENOMINATOR-DESIGN-001 #157) classified as process-verifiable or hybrid has a live grader. Gauge values will move organically as natural hook traffic accumulates.

### What's NOT in the arc

- HITL grading path for `human`-class rules (design spec §Phase D — separate arc)
- Grafana panels + alert rules per verifiability class (deferred to a JSON-diff sprint)
- Full retrain compute decision (JIMINY-CEILING-BREAK-2 Phase 4b — blocked awaiting >30 days of organic classifier-class rows per #159's DEFER verdict)

## Arch rules pinned across the arc

1. Process-observer emitters MUST be fire-and-forget async with short timeout (mirrors GUARDRAIL-PRODUCER-001)
2. Matchers MUST be fail-open — missing evidence ≠ violation
3. Hook + template mirror move in the same commit (HOOKSYNC-001)
4. Each new observer registers its own `PROCESS_MATCHER_<RULE>_ENABLED` flag default-OFF in code AND `.env` (HEBB-ETA-001)
5. Matchers MAY read arbitrary `metadata` JSONB via same-session prior-event queries
6. Fail-open skip on absent required metadata
7. Session-scope is intentional — targets within-sprint behavior
8. Event-source decision framework: hook-side emit (preferred) → same-session cross-table join → server-side detection
9. MCP-tool detection via `mcp__` prefix + retrieve-shape substring
10. Bash-side search-binary detection MUST use word-boundary-anchored module regex
11. Filesystem-read is valid matcher capability class when bounded (config-driven root + MaxFileBytes cap + single-path lookup + fail-open)
12. Sprint code regex requires ≥1 hyphen (differentiates from ISSUE-123 style)
13. Tier detection MUST use case-fold + alt spellings
14. Leanest-observer pattern — matchers can grade purely from `metadata` without cross-table / filesystem access
15. UxTS detection uses u-prefix regex + path substring
16. Non-shape-matching files skip, not miss (a config.json inside a UxTS dir is a legitimate non-schema file)
17. **NEW (#165)**: Metadata-only matchers are the leanest observer shape — prefer them whenever the graded rule reduces to a single-row metadata predicate captured at emit-time. Complexity grows monotonically with capability class; each step adds latency + failure surface.

## Documents Accessed

- `docs/development/jiminy-process-observer-06/sprint_plan.md`
- `docs/features/process-observation.md`
- `docs/development/jiminy-metric-denominator-design-001/sprint_post.md`
- `docs/development/jiminy-process-observer-{01..05}/sprint_post.md`
- `internal/grader/process/matcher.go` (interface contract)
- `internal/grader/process/matchers/uxts_frameworks.go` (leanest shipped comparator)
- `internal/config/config.go`
- `internal/api/server.go::StartSupervisedBackground`
- `.claude/hooks/post-tool-observe.py` + `internal/cli/hook_templates/post-tool-observe.py`
- `CLAUDE.md`
- `CHANGELOG.md`
