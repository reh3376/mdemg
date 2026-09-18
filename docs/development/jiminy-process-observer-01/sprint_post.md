# JIMINY-PROCESS-OBSERVER-01 — Sprint Post

**Task**: #160
**Status**: **SHIPPED** — platform + first observer (`lint-before-commit`), all default-OFF; live smoke pending operator opt-in
**PR**: #657 (auto-PR on `reh3376_dev01`)
**Sprint plan**: `docs/development/jiminy-process-observer-01/sprint_plan.md`
**Feature doc**: `docs/features/process-observation.md`

## What shipped

Path 2 of JIMINY-METRIC-DENOMINATOR-DESIGN-001: the process-observation platform + first observer. The `mdemg_jiminy_follow_rate_process_verifiable` gauge shipped in JIMINY-METRIC-PARTITION-001 (#158) can now be moved by real signal (previously dormant at 0).

### Epics shipped (all 8)

| Epic | Delivery | Commit |
|---|---|---|
| E1 — Config | 6 new env vars (default-OFF); TSDBRequiredSchemaVersion 35→36 | `94b8c91b` |
| E2 — Migration | V0036 `process_events` + `process_outcomes` hypertables | `94b8c91b` |
| E3 — Writer + HTTP | Buffered writers + `POST /v1/process/event` | `6635b34f` |
| E4 — Grader + matcher | Periodic loop + `lint-before-commit` matcher | `6635b34f` |
| E5 — Hook emitter | `.claude/hooks/post-tool-observe.py` + template mirror | `ea5d374d` |
| E6 — Metric wire | `GuidanceEffectivenessByClass` UNION over both outcome tables | `ea5d374d` |
| E7 — Route inventory | `/v1/process/event` adjudicated IN_USE (inline with E3) | `6635b34f` |
| E8 — Docs + pin | Feature doc + CLAUDE.md pin + this sprint post | (this commit) |

### Non-substrate-mutation surface

Ships default-OFF in code AND `.env`. Turning it on is a two-step decision: flip `PROCESS_EVENTS_ENABLED` (writers construct) and `PROCESS_GRADER_ENABLED` + `PROCESS_MATCHER_LINT_BEFORE_COMMIT_ENABLED` (grader loop runs). Zero side effects on shipped Jiminy classifier, retrieval, or metric partition.

## Verification (this sprint)

| Check | Result |
|---|---|
| `go build ./...` | clean |
| `go test ./internal/config/... ./internal/tsdb/... ./internal/api/... ./internal/grader/...` | ok |
| `golangci-lint run ./...` | 0 issues |
| `python3 scripts/verify_metrics_consumers.py` | OK — no drift |
| `python3 scripts/verify_doc_env_vars.py --strict` | 111 docs scanned, no unknown tokens |
| `python3 scripts/verify_config_consumers.py` | 922/922 consumed, 0 dead |
| `python3 scripts/verify_tsdb_consumers.py` | 29 declared, 29 inventoried |
| `python3 scripts/verify_route_consumers.py` | 194 live, 198 inventoried; OK |
| V0036 migration applied against real TimescaleDB 2.25.1 | idempotent + clean |
| Both hook copies parse (with `{{SPACE_ID}}` substitution) | OK |
| HOOKSYNC-001 parity (diff = 1 line — the space_id template marker) | OK |

## Live Tier-3 smoke (operator-executed, mandatory per `live-testing-tier-required`)

The sprint plan's success criterion is: **`mdemg_jiminy_follow_rate_process_verifiable` gauge moves from 0.000 to a real fraction after a genuine edit → lint → commit sequence.**

Since the platform ships default-OFF, live smoke is an operator decision:

```bash
# 1. Enable in .env, then restart the server
grep -E '^PROCESS_' .env    # verify 3 flags = true
mdemg service restart       # or: docker compose up -d

# 2. Confirm boot log shows the writer + grader wiring
tail -F ~/.mdemg/logs/mdemg.log | grep -i process

# 3. Real edit → lint → commit sequence in a session
#    (this hook fires from any Claude Code session interacting with mdemg;
#     the emitter reads the port file at each PostToolUse)
#    a. Edit a Go file  (Write / Edit tool)
#    b. Run: golangci-lint run ./internal/…
#    c. Run: git commit -m "…"

# 4. Wait ≥ 60s (PROCESS_GRADER_INTERVAL_SEC), then verify
psql -c "SELECT time, event_type, outcome, event_subtype
         FROM process_events WHERE space_id='mdemg-dev'
         ORDER BY time DESC LIMIT 10"

psql -c "SELECT time, matcher_name, outcome_type, reason
         FROM process_outcomes WHERE space_id='mdemg-dev'
         ORDER BY time DESC LIMIT 5"

curl -s http://localhost:9999/v1/metrics/snapshot \
  | jq '.gauges.mdemg_jiminy_follow_rate_process_verifiable'
```

**Success**: `process_events` has ≥3 rows (file_write, lint_run success, git_commit); `process_outcomes` has ≥1 row for `lint-before-commit` with `outcome_type='process_followed'`; the gauge returns a non-zero fraction (initially 1.0 with just the one row).

**Negative case (skip the lint step)**: `process_outcomes` records `outcome_type='process_missed'` — proves the fail-open contract routes correctly.

## Follow-ups

- **JIMINY-PROCESS-OBSERVER-02..-06** — remaining Path 2 observers per design §Phase E prioritization:
  02: `sequential-epics` (commit-message parse, no new hook needed)
  03: `query-mdemg-cms-file-paths` (memory_recall → glob/grep correlation)
  04: `unit-integration-e2e-docs` (hybrid: sprint-plan content + CI test-tier events)
  05: `must-use-uxts-frameworks-consistently` (hybrid: file-path checker for new JSON schemas)
  06: `never-haiku-for-planning` (lowest priority; single-event lookup)
- **UATS contract for `/v1/process/event`** — deferred; ships with observer #2 alongside the second event-type shape
- **Unit tests for `_process_events_for_tool`** — deferred; behavior verified live in Tier 3, but a pytest-style hook unit test would harden the regex/tool-name gates for future observers
- **Grafana panel for the process class** — after the gauge shows real data (T+168h of natural traffic), a `Follow Rate — Process Verifiable` timeseries panel on `mdemg-jiminy.json` would surface trend
- **Retention/compression policy tuning** — 90d/7d matches the shipped telemetry family default; revisit after 30d of natural volume observation

## Arch rules pinned (added to CLAUDE.md in this sprint)

1. **Process-observer emitters MUST be fire-and-forget async with a short timeout** (1s connect / 3s max-time budget for the hook; mirrors GUARDRAIL-PRODUCER-001).
2. **Process matchers MUST be fail-open**: missing evidence ≠ violation; only present-event-with-failure produces `process_incomplete`. Same class as JIMINY-CLASSIFIER-CONTEXT-002 mechanism-scope gate.
3. **Hook + template mirror move in the same commit** (HOOKSYNC-001 parity — CI-gated).
4. **Each new observer registers its own `PROCESS_MATCHER_<RULE>_ENABLED` flag** in `internal/config/config.go` (never-hardcode-config); default-OFF; opt-in via `.env` per HEBB-ETA-001.

## Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§Phase B design; §Phase E prioritization)
- `docs/development/phase-4b-gate-decision-001/verdict.md` (this sprint's kickoff signal)
- `docs/development/jiminy-metric-partition-001/sprint_plan.md` (12-section format template)
- `internal/tsdb/migrations/022_reinforcement_events.sql` (hypertable + retention pattern to mirror)
- `internal/tsdb/migrations/025_retention_compression.sql` (columnstore + compression policy pattern — the migration-fix cause on `add_compression_policy`)
- `internal/tsdb/reinforcement_writer.go` (writer shape to mirror)
- `internal/tsdb/dataset_builder.go::GuidanceEffectivenessByClass` (metric emit path)
- `internal/api/eventgraph_handler.go` (HTTP handler pattern)
- `internal/api/server.go::StartSupervisedBackground` (supervisor wiring)
- `.claude/hooks/post-tool-observe.py` (client-side emitter to extend)
- `internal/cli/hook_templates/post-tool-observe.py` (HOOKSYNC-001 template mirror)
- `docs/api/tsdb_consumer_inventory.json`, `docs/api/route_consumer_inventory.json` (DORMANT-CENSUS forcing functions)
- CLAUDE.md — HEBB-ETA-001, HOOKSYNC-001, SUPERVISOR-002, TSDB-CONSUME-001, DORMANT-CENSUS-{001,002}, JIMINY-METRIC-PARTITION-001, JIMINY-CLASSIFIER-CONTEXT-002, GUARDRAIL-PRODUCER-001
