# JIMINY-PROCESS-OBSERVER-01 — Sprint Plan

**Task**: #160
**Predecessor**: JIMINY-METRIC-DENOMINATOR-DESIGN-001 (#157) — this sprint implements Path 2 §Phase B, first observer per §Phase E prioritization
**Wall-clock estimate**: ~6-8h (this sprint ships the platform + first observer; subsequent observers are 2-3h each per design spec §B5)
**Type**: shipping — code + schema + hook + tests + docs

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | JIMINY-PROCESS-OBSERVER-01 |
| Task # | #160 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | 2 new TSDB hypertables (additive; V0036 migration) + hook extension (client-side only, no substrate touch) |
| Design source | `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase B + Phase E prioritization item 1 |
| Unblocks | JIMINY-PROCESS-OBSERVER-02 (`sequential-epics`), remaining -03..-06 observers, honest movement on `mdemg_jiminy_follow_rate_process_verifiable` gauge (currently dormant at 0) |

## 2. Problem Statement

`mdemg_jiminy_follow_rate_process_verifiable` gauge shipped in JIMINY-METRIC-PARTITION-001 (#158) but is DORMANT — nothing writes to it because there is no process-observation platform yet. Per the Phase 4b gate decision (#159), Path 2 first observer is the highest-value next step to add real signal to the process class (currently 50 rows / 22% follow, single-digit organic per day). The `lint-before-commit` rule is the highest-priority observer per Path 2 prioritization: real quality signal, trivial observer (shell hook already captures Bash), every commit becomes a graded event.

## 3. Scope & Constraints

**In scope (this sprint = platform + first observer):**

*Platform:*
- 2 new TSDB hypertables (V0036 migration):
  - `process_events` — one row per observed process event (lint runs, model calls, mcp calls, retrieval calls, bash commands, git commits, plan-mode entries)
  - `process_outcomes` — one row per process-graded outcome (parallel to `constraint_outcomes` shape)
- New Go package `internal/grader/process/` with `ProcessGrader` interface + rule-matcher registry
- New HTTP endpoint `POST /v1/process/event` (batched; fire-and-forget contract)
- New buffered `ProcessEventWriter` (mirrors V0022 reinforcement_events buffered pattern)
- New periodic grader loop (supervised via SUPERVISOR-002) that runs each registered matcher on freshly-committed events and writes `process_outcomes`
- Metric wire — extend the follow-rate gauge emitter in `internal/ape/self_assess.go::applyHonestFollowRate` to include `process_outcomes` rows in the `process_verifiable` denominator

*First observer:*
- `internal/grader/process/matchers/lint_before_commit.go` — Cypher-free SQL grader; queries `process_events` for `event_type='lint_run' AND outcome='success'` between last `event_type='file_write'` (or `write_file` semantic marker) and next `event_type='git_commit'` on the same session
- Hook extension: `.claude/hooks/post-tool-observe.py` (+ template mirror in `internal/cli/hook_templates/`) emits `POST /v1/process/event` on the following tool completions:
  - `Bash` with `golangci-lint` or `ruff` in command → `event_type=lint_run` with exit_code + subtype
  - `Bash` with `git commit` → `event_type=git_commit` (already partially captured via PostToolUse; expand to POST)
  - `Write`/`Edit` completion → `event_type=file_write` with the target path
- Grader runs after each git_commit event lands, evaluates the matcher, writes a `process_outcomes` row with the correct `outcome_type`

*Config (all default-off per HEBB-ETA-001 rule):*
- `PROCESS_EVENTS_ENABLED` (default false)
- `PROCESS_EVENT_WRITER_FLUSH_INTERVAL_SEC` (default 30)
- `PROCESS_EVENT_WRITER_BUFFER_SIZE` (default 500)
- `PROCESS_GRADER_ENABLED` (default false)
- `PROCESS_GRADER_INTERVAL_SEC` (default 60)
- `PROCESS_MATCHER_LINT_BEFORE_COMMIT_ENABLED` (default false)

**Out of scope (deferred to sibling sprints):**
- Observers 2-6 (`sequential-epics`, `query-mdemg-cms-file-paths`, `unit-integration-e2e-docs`, `must-use-uxts-frameworks-consistently`, `never-haiku-for-planning`) → each is its own JIMINY-PROCESS-OBSERVER-NN sprint
- HITL integration for human-class rules → JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001
- Retrain gate re-evaluation (needs 168h+ of real process data first) → post-observer-arc sprint
- Grafana panel for process_outcomes trend → after gauge shows real data
- Retention/compression on the 2 new hypertables → follows the V0022 shape (30d chunks, 90d retention)

**Constraints honored:**
- `plan-mode-before-change` — this doc IS the plan; drafted before any code
- `unit-integration-e2e-docs` — 3 tiers below
- `live-testing-tier-required` — Tier-3 on real MDEMG server + real hook subprocess + real git commit + real lint invocation
- `never-hardcode-config` — 6 new env vars (all in §3)
- `never-direct-alter-schema` — V0036 migration file lands with schema; `TSDBRequiredSchemaVersion` bumps 35→36
- `must-use-cuid2` — `event_id` + `outcome_id` use `cuid2.Generate()`
- `mandatory-feature-docs` — `docs/features/process-observation.md` ships with the code (per PR 657's DOC-CURRENCY-002 lesson: env vars named in docs MUST be wired)
- `sequential-epics` — 8 sequential epics below; NEVER parallelized
- HOOKSYNC-001 — hook + template mirror in `internal/cli/hook_templates/` update together

## 4. Dependencies

- V0035 in place (current schema version 35 after JIMINY-METRIC-PARTITION-001)
- `mdemg_jiminy_follow_rate_process_verifiable` gauge declared (#158)
- SUPERVISOR-002 supervisor available for the new grader loop
- `.claude/hooks/post-tool-observe.py` already exists; extend it
- `internal/api/server.go` — one new handler registration
- No new Go modules

## 5. Implementation Plan (sequential epics)

### Epic 1 — Type definitions + config
- `internal/process/types.go` (new package): `Event`, `EventType` (enum), `Outcome` (enum), `ProcessVerdict`
- Config additions in `internal/config/config.go`:
  - `ProcessEventsEnabled` bool
  - `ProcessEventWriterFlushIntervalSec` int
  - `ProcessEventWriterBufferSize` int
  - `ProcessGraderEnabled` bool
  - `ProcessGraderIntervalSec` int
  - `ProcessMatcherLintBeforeCommitEnabled` bool
- Pin test in `internal/config/config_test.go` for defaults

### Epic 2 — V0036 migration + schema version bump
- New file `internal/tsdb/migrations/036_process_events_and_outcomes.sql`:
  - `process_events` table (all fields from design spec §B1) as hypertable
  - `process_outcomes` table (mirrors `constraint_outcomes` shape) as hypertable
  - Indexes: `(session_id, time DESC)` and `(event_type, time DESC)` on `process_events`; `(constraint_code, time DESC)` on `process_outcomes`
  - 30d chunk interval, 90d retention (matches sibling telemetry tables per TSDB-CONSUME-001)
  - Update `tsdb_schema_meta` version to 36
- `TSDBRequiredSchemaVersion` default bumped 35→36 in `internal/config/config.go`

### Epic 3 — ProcessEventWriter + HTTP endpoint
- `internal/tsdb/process_event_writer.go` — buffered writer mirroring `retrieval_event_writer.go` shape (registerWriterStats, CopyFrom bulk flush, drop-on-overflow counter)
- Register writer in `internal/cli/serve.go` inside the "early writer block" WITH SUPERVISOR-002 wiring
- `internal/api/handlers_process.go` new `handleProcessEvent`:
  - `POST /v1/process/event` accepts `{events: [...]}` batches
  - Validates required fields (space_id, session_id, event_type)
  - Sanitizes text fields via `llmclient.ScrubString` (privacy — DOC-CURRENCY-002 arc lesson)
  - Enqueues to `ProcessEventWriter.Record` (nil-guard fail-open — hot-path contract)
  - Returns 202 immediately
- Route registered in `internal/api/server.go`; UOTS-friendly (add to route inventory in same PR per DORMANT-CENSUS-001 forcing function)
- Pin tests: happy path + validation + writer overflow + nil-writer safety

### Epic 4 — Grader + first matcher
- `internal/grader/process/grader.go` — `ProcessGrader` interface + `Registry` + `Run` method
- `internal/grader/process/matchers/lint_before_commit.go` — implements the semantic:
  1. Given a `git_commit` event on session S at time T
  2. Find the most-recent `file_write` event on session S before T
  3. Find any `lint_run` event on session S between those two moments
  4. If `lint_run` exists with `outcome='success'` → `process_followed`
  5. If `lint_run` exists with `outcome='failure'` → `process_incomplete`
  6. If NO `lint_run` in that window → `process_missed`
  7. Fail-open: if `file_write` event absent (defensive default), return no verdict (grader skips row)
- `internal/grader/process/loop.go` — periodic loop reads `git_commit` events since `last_graded_time`, runs each enabled matcher, writes `process_outcomes` rows via a new `ProcessOutcomeWriter`
- Loop supervised via `SetSupervise` (SUPERVISOR-002)
- Grader wired in `internal/cli/serve.go` under `PROCESS_GRADER_ENABLED`

### Epic 5 — Hook emitter (client-side)
- Extend `.claude/hooks/post-tool-observe.py`:
  - On `Bash` tool complete AND command contains `golangci-lint` OR `ruff check` → POST `/v1/process/event` with `event_type=lint_run`, `event_subtype=<tool>`, `exit_code`, `outcome`
  - On `Bash` tool complete AND command contains `git commit` (not `commit --amend` or `commit --dry-run`) AND exit_code=0 → POST `event_type=git_commit`
  - On `Write` / `Edit` tool complete → POST `event_type=file_write` with `metadata.file_path`
  - Fire-and-forget, 500ms timeout, drop on network error (mirrors JIMINY-ENFORCE-002's fail-open)
- Mirror in `internal/cli/hook_templates/post-tool-observe.py` (HOOKSYNC-001 parity — CI-blocking)
- Space_id resolved via `MDEMG_SPACE_ID` env or `.mdemg.port` neighbor file (existing hook pattern)

### Epic 6 — Metric wire (make gauge move)
- Extend `DatasetProvider.GuidanceEffectivenessByClass` in `internal/tsdb/dataset_builder.go`:
  - Current implementation aggregates `constraint_outcomes` GROUP BY `verifiability_class`
  - Extend to UNION over `process_outcomes` rows and treat as `verifiability_class='process'` in the aggregate
- Update `internal/ape/self_assess.go::applyHonestFollowRate` to consume the extended shape (no signature change; the map already includes 'process' — this just makes it non-empty)
- Live-verify `mdemg_jiminy_follow_rate_process_verifiable` gauge emits from real process_outcomes rows after Epic 5's smoke

### Epic 7 — Route inventory + metric inventory
- `python3 scripts/verify_route_consumers.py --generate` → adjudicate `/v1/process/event` (IN_USE, consumers: post-tool-observe.py hook)
- Metrics inventory already covers `follow_rate_process_verifiable` (added in #158)
- `python3 scripts/verify_doc_env_vars.py --strict` (for the 6 new env vars — they're wired, so they'll be in code corpus automatically)

### Epic 8 — Documentation + CLAUDE.md pin
- `docs/features/process-observation.md` — Why / Choices / How-it-works / How-to-use for the whole platform
- `CLAUDE.md` — new "Architecture Notes" entry summarizing:
  - Rule pinned: process-observer emitters MUST be fire-and-forget async with a short timeout (mirrors constraint_outcomes writer pattern per JIMINY-OUTCOME-001)
  - Rule pinned: process matchers MUST be fail-open (missing evidence ≠ violation) — same class as JIMINY-CLASSIFIER-CONTEXT-002 mechanism-scope gate
  - Rule pinned: hook + template mirror must move in same commit (HOOKSYNC-001 parity)
- `CHANGELOG.md` entry
- Sprint post-mortem `docs/development/jiminy-process-observer-01/sprint_post.md`

## 6. Testing Plan (3 tiers — mandatory per `unit-integration-e2e-docs`)

### Tier 1 — Unit tests
- `internal/process/types_test.go` — enum stability
- `internal/tsdb/process_event_writer_test.go` — buffer/flush/overflow/nil-safety
- `internal/api/handlers_process_test.go` — validation, sanitization, 202-on-success, 4xx-on-bad-input
- `internal/grader/process/matchers/lint_before_commit_test.go` — 5 subtests: happy path, no lint in window, lint failed, no prior file_write, session isolation
- `internal/config/config_test.go` — 6 new env-var default assertions

### Tier 2 — Integration tests
- With real TSDB pool (dockertest): apply V0036, insert events, run grader, assert `process_outcomes` rows appear with correct `outcome_type`
- Buffered writer under contention (parallel Record calls) — no drops when under buffer, dropped_total increments when over

### Tier 3 — Live e2e (mandatory per `live-testing-tier-required`)
- Real `bin/mdemg serve` on 9999 with `PROCESS_EVENTS_ENABLED=true PROCESS_GRADER_ENABLED=true PROCESS_MATCHER_LINT_BEFORE_COMMIT_ENABLED=true`
- Real git commit sequence:
  1. Real `Write` on a Go file → hook posts `file_write` event
  2. Real `golangci-lint run ./internal/process/...` → hook posts `lint_run` event with `outcome=success`
  3. Real `git commit` → hook posts `git_commit` event
  4. Wait ≥ `PROCESS_GRADER_INTERVAL_SEC`
  5. Assert `process_outcomes` has a row for `lint-before-commit` with `outcome_type='process_followed'`
  6. Assert `mdemg_jiminy_follow_rate_process_verifiable` gauge shows a non-zero denominator
- Negative case: skip the lint step, commit anyway → `outcome_type='process_missed'` row lands
- **Success criterion**: gauge moves from `0.000` to a real fraction reflecting the two events (2/2 followed = 1.0 initially, then more mixed data as natural traffic flows)

## 7. Commit Strategy

One commit per epic ⇒ 8 commits on `reh3376_dev01`:
1. `feat(process): type definitions + 6 config knobs (JIMINY-PROCESS-OBSERVER-01 E1)`
2. `feat(tsdb): V0036 process_events + process_outcomes hypertables (E2)`
3. `feat(process): buffered writer + POST /v1/process/event endpoint (E3)`
4. `feat(process): grader loop + lint-before-commit matcher (E4)`
5. `feat(hooks): post-tool-observe emits process events (E5, HOOKSYNC-001 parity)`
6. `feat(metrics): wire process_outcomes into follow_rate_process_verifiable gauge (E6)`
7. `chore(inventory): adjudicate /v1/process/event route (E7)`
8. `docs(process-observation): feature doc + CLAUDE.md pin + sprint post (E8)`

Auto-PR updates the existing PR (or opens a new one) via the shipped workflow.

## 8. Verification Checklist

- [ ] `go build ./...` clean after each epic
- [ ] `golangci-lint run ./...` clean
- [ ] Tier 1 tests green
- [ ] Tier 2 integration tests green
- [ ] Tier 3 live smoke: `mdemg_jiminy_follow_rate_process_verifiable` gauge moves from 0 to a non-zero fraction (**live-testing-tier-required**)
- [ ] Live smoke: `curl http://localhost:9999/v1/metrics/snapshot | jq '.gauges.mdemg_jiminy_follow_rate_process_verifiable'` returns non-zero
- [ ] Live smoke: `SELECT COUNT(*) FROM process_events WHERE space_id='mdemg-dev'` returns > 0
- [ ] Live smoke: `SELECT COUNT(*) FROM process_outcomes WHERE space_id='mdemg-dev' AND constraint_code='lint-before-commit'` returns > 0
- [ ] `python3 scripts/verify_doc_env_vars.py --strict` — no drift on the 6 new env vars
- [ ] `python3 scripts/verify_route_consumers.py` — `/v1/process/event` adjudicated
- [ ] `python3 scripts/verify_metrics_consumers.py` — no drift
- [ ] `make verify-grafana-embed` — no drift (this sprint doesn't touch grafana; forcing function anyway)
- [ ] `make sync-grafana-embed` — no-op if truly untouched (belt-and-suspenders)
- [ ] Docs Accessed section at bottom of sprint_post.md

## 9. Documentation Update (never cut per format rule)

- New feature doc: `docs/features/process-observation.md`
- CLAUDE.md — new sprint entry under Architecture Notes
- CHANGELOG.md — new entry under Unreleased
- Sprint post-mortem: `docs/development/jiminy-process-observer-01/sprint_post.md`
- PR summary comment (per `must-comment-sprint-summary-on-pr` rule)

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Hook adds latency to every tool completion | Fire-and-forget POST with 500ms timeout; drop on error; async subprocess; no blocking of tool result |
| Process events pollute TSDB with noise | Bounded via `PROCESS_EVENT_WRITER_BUFFER_SIZE` + 90d retention; default-OFF flag; scrub-at-intake |
| Grader misses events due to session_id mismatch | Both hook + server read `MDEMG_SPACE_ID` + `MDEMG_SESSION_ID` env vars; document the session-id contract in the feature doc |
| Matcher false-follows on a lint run that failed silently | The matcher requires `outcome='success'` which comes from `exit_code=0`; failed runs produce `process_incomplete` (distinct from `process_followed`) |
| Metric gauge stays flat post-ship because no natural traffic yet | Live smoke synthesizes traffic; passive re-check at T+24h shows organic pattern |
| Retrain gate not immediately unblocked (still needs classifier signal) | Not this sprint's job; Phase 4b decision already deferred per #159 |

## 11. Rollback Procedures

- Substrate-mutation surface = 2 additive hypertables. Rollback = `DROP TABLE process_events, process_outcomes;` (SQL); no data loss to existing systems.
- Code-side rollback = flip `PROCESS_EVENTS_ENABLED=false` + `PROCESS_GRADER_ENABLED=false` + `PROCESS_MATCHER_LINT_BEFORE_COMMIT_ENABLED=false`; hook is fail-open on network error (server-off = no-op).
- V0036 migration is IF NOT EXISTS-guarded; re-applying is idempotent.

## 12. Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§Phase B design; §Phase E prioritization)
- `docs/development/phase-4b-gate-decision-001/verdict.md` (this sprint's kickoff signal)
- `docs/development/jiminy-metric-partition-001/sprint_plan.md` (sibling sprint format template)
- `internal/jiminy/service.go::RecordOutcome` (metric emit point)
- `internal/ape/self_assess.go::applyHonestFollowRate` (follow-rate gauge emitter)
- `internal/tsdb/reinforcement_events_writer.go` (buffered writer shape to mirror)
- `internal/tsdb/migrations/022_reinforcement_events.sql` (hypertable + retention pattern)
- `internal/cli/serve.go` (writer registration + supervisor wiring)
- `.claude/hooks/post-tool-observe.py` (hook to extend)
- `internal/cli/hook_templates/post-tool-observe.py` (HOOKSYNC-001 template mirror)
- `docs/api/route_consumer_inventory.json` (Epic 7 adjudication target)
- `CLAUDE.md` — HEBB-ETA-001 (default-off pattern), HOOKSYNC-001 (template mirror rule), SUPERVISOR-002 (supervisor contract), TSDB-CONSUME-001 (retention/compression contract), DORMANT-CENSUS-001 (route inventory forcing function), JIMINY-METRIC-PARTITION-001 (predecessor metric partition)
