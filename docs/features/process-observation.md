# Process Observation

**Sprint**: JIMINY-PROCESS-OBSERVER-01 (task #160)
**Predecessor**: JIMINY-METRIC-PARTITION-001 (#158) shipped the follow-rate class partition; this sprint feeds the `process` bucket.
**Design source**: `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase B
**Feature status**: Platform + first observer (`lint-before-commit`) shipped default-OFF. Subsequent observers land in sibling sprints without further schema changes.

## Why

The Jiminy classifier grades follow-rate against **action-text**. Some rules are structurally invisible in that text — they govern the *process* the agent ran (was lint invoked before commit? did retrieval precede glob/grep?). The design-verdict sprint concluded these rules must be graded via a **new signal channel** (process-observation events) rather than by tuning the classifier. Otherwise the aggregate follow-rate ceiling is bounded by the class mix, not by classifier quality.

The `mdemg_jiminy_follow_rate_process_verifiable{space_id}` gauge shipped in JIMINY-METRIC-PARTITION-001 (#158) is the reporting side of the mechanism. This sprint ships the writing side.

## Choices

- **Two hypertables, not an enum extension**. `process_events` captures raw observations; `process_outcomes` captures graded verdicts. Keeps concerns separated + lets the aggregator `UNION` the outcome tables uniformly.
- **Fire-and-forget client-side emitter**. The `.claude/hooks/post-tool-observe.py` hook POSTs events with a 1s connect / 3s max-time budget and never blocks the tool result. Same pattern as GUARDRAIL-PRODUCER-001.
- **Server-side buffered writer**. Mirrors the shipped `ReinforcementEventsWriter` (V0022) shape — CopyFrom flush, FIFO eviction, `registerWriterStats` for the shipped `tsdb_writer_flush_failures` alert family.
- **Periodic grader loop**. Reads *new* terminal events (`git_commit` for the lint matcher) since the last graded time, runs registered matchers, writes verdicts to `process_outcomes`. Supervised via SUPERVISOR-002 (nil-return contract for graceful shutdown).
- **Per-matcher config gate**. Each matcher checks its own `PROCESS_MATCHER_<RULE>_ENABLED` flag. New rules ship default-OFF and are enabled per-operator.
- **Fail-open matcher contract**. Missing evidence ≠ violation. Only present-event-with-failure-outcome produces `process_incomplete`; no event at all produces `process_missed`. Mirrors JIMINY-CLASSIFIER-CONTEXT-002's mechanism-scope gate.

## How it works

```
Claude Code                                          MDEMG server
──────────────                                       ──────────────

Bash (golangci-lint …) ─┐
Bash (git commit …)    ─┼─► POST /v1/process/event ─► ProcessEventsWriter (buffered)
Write / Edit           ─┘         (fire-and-forget)         │
                                                            ▼
                                                    process_events (V0036, TSDB)
                                                            │
                                       ┌────────────────────┘
                                       ▼
                                Process Grader loop (60s tick)
                                 - For each matcher:
                                     fetch fresh terminal events
                                     Grade(event) → Verdict
                                     ProcessOutcomesWriter.Record
                                            │
                                            ▼
                                    process_outcomes (V0036, TSDB)
                                            │
              ┌─────────────────────────────┘
              ▼
   DatasetBuilder.GuidanceEffectivenessByClass
     UNION (constraint_outcomes → classifier/hybrid/human class)
        with (process_outcomes → process class)
              │
              ▼
   applyHonestFollowRate → per-class gauge emit
   mdemg_jiminy_follow_rate_process_verifiable{space_id}
```

### `lint-before-commit` matcher (first observer)

Semantics per design spec §B4:
1. Given a `git_commit` event on session S at time T,
2. Find the most-recent `file_write` event on session S before T,
3. Find any `lint_run` event on session S in that window,
4. If `lint_run.outcome == success` → `process_followed` (1.0 credit),
5. If `lint_run.outcome == failure` → `process_incomplete` (0.5 credit),
6. Else → `process_missed` (0.0 credit).
7. If no prior `file_write` → fail-open skip (docs-only commit; not a violation).

### `uxts-frameworks` matcher (fifth observer — JIMINY-PROCESS-OBSERVER-05)

Grades the shipped `must-use-uxts-frameworks-consistently` Jiminy rule (hybrid class): "When creating a JSON schema, contract, or test spec that will be used repeatedly, use the UxTS framework family." Design classifier side is "new JSON schema files outside `docs/tests/u*ts/`"; this observer ships the process side. **No cross-table query, no filesystem read** — purely a metadata regex check on `file_write` events.

Semantics:
1. Terminal event: `file_write` (Write or Edit tool) with `metadata.file_path`
2. Filename regex `\.u[a-z]+\.json$` — matches all 11 shipped UxTS families (uaits, uams, ubench, ubts, uets, uits, ults, uobs, usts, utds, uvts) + any future u-prefix framework
3. No match → fail-open skip (not a UxTS-shape JSON file)
4. Path contains `docs/tests/u` → `process_followed`
5. Path doesn't contain → `process_incomplete` (reason names the violating path)

### `unit-integration-e2e-docs` (UIED) matcher (fourth observer — JIMINY-PROCESS-OBSERVER-04)

Grades the shipped `unit-integration-e2e-docs` Jiminy rule ("All development plans MUST include three testing tiers: unit tests, integration tests, e2e tests, plus documentation updates"). Hybrid class — the classifier grades doc-shape presence in general prose; this observer grades the specific sprint plan for the sprint code named in the commit message.

⚠️ **New capability class: filesystem read**. Prior matchers only queried `process_events`. UIED reads a single file (`<SprintDocsRoot>/<kebab-code>/sprint_plan.md`) with a `MaxFileBytes` safety cap and fail-open on any error.

Semantics:
1. Terminal event: `git_commit` on session S with `metadata.commit_message` populated (via OBSERVER-02 hook capture)
2. Extract sprint code via `\b([A-Z][A-Z0-9]+(?:-[A-Z0-9]+)+-\d+)\b` — requires word-boundary + at least one hyphen
3. If no marker → fail-open skip (chore/merge/non-sprint commit)
4. Kebab-lower the code, look up the sprint plan file
5. File absent → `process_missed` (sprint referenced without a plan)
6. File present, all 3 tier keywords found (case-fold `unit` + `integration` + [`e2e` OR `end-to-end`]) → `process_followed`
7. Missing ≥1 tier → `process_incomplete` (reason names the missing tier)
8. File over cap → fail-open skip (safety)

### `query-cms-first` matcher (third observer — JIMINY-PROCESS-OBSERVER-03)

Grades the `query-mdemg-cms-file-paths` Jiminy rule ("When discovering unfamiliar code structure, query MDEMG CMS retrieval FIRST; glob/grep only for exact-token or CMS-miss fallback"). Introduces two new event types:
- `retrieval_call` — emitted by the hook when the agent invokes an MCP retrieve tool (`mcp__*memory_recall*` / `memory_retrieve` / `__recall`) OR runs a Bash `curl` to `/v1/memory/retrieve`
- `filesystem_search` — emitted on `Glob` / `Grep` tool completion, OR Bash containing `rg` / `ripgrep` / `ag` / `grep -r*` / `find <path>` (word-boundary-anchored)

Semantics:
1. Terminal event: `filesystem_search` on session S at time T
2. Query same-session prior `retrieval_call` events within `PROCESS_MATCHER_QUERY_CMS_WINDOW_SEC` (default 300s / 5min) before T
3. ≥1 prior retrieval → `process_followed` (evidence = the most recent retrieval event_id)
4. 0 prior retrievals → `process_missed`
5. Empty session_id → fail-open skip (can't correlate)

⚠️ **MVP limitation**: the "exact-token / CMS-miss fallback" exception (a narrow specific-string search is a legitimate case even without prior retrieval) is NOT distinguished — all filesystem_search events without a prior retrieval are graded `process_missed`. Ship default-OFF; operator flip after passive observation. If FP rate is unacceptable, a follow-up sprint can add a search-shape heuristic (path scope + single-quoted literal).

### `sequential-epics` matcher (second observer — JIMINY-PROCESS-OBSERVER-02)

Grades the `sequential-epics` Jiminy rule ("Execute sprint epics SEQUENTIALLY — Epic N MUST complete fully before Epic N+1 begins"). The hook captures the actual commit message via `git log -1 --format=%B` (bounded 500 chars, fail-open on error) into `metadata.commit_message`; the matcher parses it.

Semantics:
1. Given a `git_commit` event on session S at time T with a commit message,
2. Extract the `Epic N` marker via `(?i)\bepic\s+(\d+)\b` (word-boundary),
3. If no marker → fail-open skip (chore/docs/refactor commits are NOT graded),
4. Query same-session prior `git_commit` events; find max prior Epic N,
5. If max prior ≤ current → `process_followed` (monotonic sequence maintained; equal N is fine — same epic, multiple commits),
6. If max prior > current → `process_incomplete` (out-of-order — exactly what the rule prohibits).

## How to use

### Enable end-to-end (default-off in code, opt-in via `.env`)

```bash
# In your .env
PROCESS_EVENTS_ENABLED=true
PROCESS_GRADER_ENABLED=true
PROCESS_MATCHER_LINT_BEFORE_COMMIT_ENABLED=true
PROCESS_MATCHER_SEQUENTIAL_EPICS_ENABLED=true    # JIMINY-PROCESS-OBSERVER-02
PROCESS_MATCHER_QUERY_CMS_FIRST_ENABLED=true     # JIMINY-PROCESS-OBSERVER-03
PROCESS_MATCHER_UIED_ENABLED=true                # JIMINY-PROCESS-OBSERVER-04
PROCESS_MATCHER_UXTS_FRAMEWORKS_ENABLED=true     # JIMINY-PROCESS-OBSERVER-05
```

Then `mdemg service restart` (or `docker compose up -d`).

The hook auto-emits events from any Claude Code session because it reads `MDEMG_URL`/`.mdemg.port` at each PostToolUse invocation — no hook re-install needed.

### Verify events are flowing

```bash
# Recent events
psql -c "SELECT time, event_type, outcome, event_subtype
         FROM process_events
         WHERE space_id='mdemg-dev'
         ORDER BY time DESC LIMIT 10"

# Recent verdicts
psql -c "SELECT time, matcher_name, outcome_type, reason
         FROM process_outcomes
         WHERE space_id='mdemg-dev'
         ORDER BY time DESC LIMIT 10"

# Gauge (after a matcher run)
curl -s http://localhost:9999/v1/metrics/snapshot \
  | jq '.gauges.mdemg_jiminy_follow_rate_process_verifiable'
```

### Turn off

Flip any of the 3 env vars to `false` and restart. The V0036 tables persist (data-loss-safe) — flipping back on resumes with the retained events. Full uninstall = drop the tables (see rollback in the migration file).

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `PROCESS_EVENTS_ENABLED` | `false` | Master enable for the writers + HTTP endpoint |
| `PROCESS_EVENT_WRITER_FLUSH_INTERVAL_SEC` | `30` | Buffered writer flush cadence (floor 5) |
| `PROCESS_EVENT_WRITER_BUFFER_SIZE` | `500` | Max rows before FIFO eviction |
| `PROCESS_GRADER_ENABLED` | `false` | Enable the periodic matcher loop |
| `PROCESS_GRADER_INTERVAL_SEC` | `60` | Grader loop cadence (floor 15) |
| `PROCESS_MATCHER_LINT_BEFORE_COMMIT_ENABLED` | `false` | Enable the lint-before-commit matcher |
| `PROCESS_MATCHER_SEQUENTIAL_EPICS_ENABLED` | `false` | Enable the sequential-epics matcher (JIMINY-PROCESS-OBSERVER-02) |
| `PROCESS_MATCHER_QUERY_CMS_FIRST_ENABLED` | `false` | Enable the query-cms-first matcher (JIMINY-PROCESS-OBSERVER-03) |
| `PROCESS_MATCHER_QUERY_CMS_WINDOW_SEC` | `300` | Lookback window (sec) for prior retrieval_call before a filesystem_search; floor 30 |
| `PROCESS_MATCHER_UIED_ENABLED` | `false` | Enable the unit-integration-e2e-docs matcher (JIMINY-PROCESS-OBSERVER-04) |
| `SPRINT_DOCS_ROOT` | `docs/development` | Filesystem root the UIED matcher joins the kebab-lower sprint code under (relative to cwd) |
| `PROCESS_MATCHER_UIED_MAX_FILE_BYTES` | `200000` | Safety cap on sprint_plan.md read size; over-cap → skip; floor 4096 |
| `PROCESS_MATCHER_UXTS_FRAMEWORKS_ENABLED` | `false` | Enable the uxts-frameworks matcher (JIMINY-PROCESS-OBSERVER-05) |

## Adding a new observer (sibling sprints)

Each subsequent observer (`sequential-epics`, `query-mdemg-cms-file-paths`, `unit-integration-e2e-docs`, `must-use-uxts-frameworks-consistently`, `never-haiku-for-planning`) reuses the platform. Per-observer sprint scope (2-3h):

1. **Add the matcher**: new file under `internal/grader/process/matchers/` implementing `process.Matcher`
2. **Add a flag**: `PROCESS_MATCHER_<RULE>_ENABLED` in `internal/config/config.go`
3. **Wire the matcher**: `g.Register(procmatchers.NewYourMatcher())` guarded by the flag in `server.go::StartSupervisedBackground`
4. **Extend the hook (if a new event_type is needed)**: emit from `_process_events_for_tool` in `.claude/hooks/post-tool-observe.py` + template mirror
5. **Tests + docs + CHANGELOG**

The platform absorbs the addition without schema or endpoint changes.
