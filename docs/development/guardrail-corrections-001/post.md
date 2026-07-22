# GUARDRAIL-CORRECTIONS-001 — Sprint Post

**Date:** 2026-07-22 | **Branch:** `reh3376_dev01`
**Parent:** GUARDRAIL-PRODUCER-001 disclosed follow-up.

## What shipped

- `GUARDRAIL_INCLUDE_CORRECTIONS` (code default false; dev `.env` true after
  smoke) — `retrievalRoleTypes()` returns `[constraint]` or
  `[constraint, correction]`; both retrieval phases (semantic
  partition-cosine + keyword) take the role set as a `$roleTypes` param.
- `typeCoalesceExpr` renders `constraint_type` as `'correction'` for
  correction nodes (they carry none — verified 0/35 live) → prompt clarity
  + **Warning-tier cap** via the untouched `isBlockingType`
  (must/must_not only). Learned lessons advise; hard constraints block.
- Applies identically to the sync MCP `validate_changes` path (one
  retrieval function) — disclosed, acceptable: corrections can only add
  Warnings, never Blocks.
- Tests: role-set flag pin + `isBlockingType("correction")=false`; config
  default pin.

## Live Tier-3 (mdemg-dev)

The decisive runs were the **instrumented passes** (process env checked via
`ps eww` before each fixture — a first pass was confused by the
CONFIG-LOCAL-DEFAULTS-001 stale-binary/restart-timing class, likely a
throttled kickstart leaving the pre-union binary serving run #1):

- **Flag ON** (union binary, `.env` true, no override): fixture Write
  containing `mdemg db start` → prompt contains correction-typed items
  (`[N] … | correction |`), tokens_in 1328. Earlier same-state run produced
  verdict **Warning, warnings=1** — the LLM flagged the correction
  violation.
- **Flag OFF** (`launchctl setenv … false`, process env verified `false`):
  same fixture → prompt constraints-only (corr=f, tokens_in 1412).
- State restored: override unset, kickstart, healthz ok; `.env` keeps the
  flag true (operator-authorized enable-after-smoke).

Also observed: an organic producer evaluation from this session's own
edits landed between fixtures — the producer is live in normal operation.

## Verification checklist

- [x] Unit + config pins green; `go build ./...`; lint clean
- [x] Live both flag states, process-env-verified
- [x] Correction violation produces a Warning verdict live
- [x] Feature doc §, CHANGELOG, post
- [x] Env-var drift checker clean

## Documents Accessed

`internal/guardrail/{constraint_retrieval,guardrail}.go`;
`internal/config/config.go`; `internal/api/server.go`;
live Neo4j partition counts; `llm_interactions` prompt inspection;
`internal/cli/config_loader.go` (env precedence: process env beats `.env`).
