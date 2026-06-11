# Sprint Plan — UATS-GAP-001: Contract Floor Under the Revived Channels

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | UATS-GAP-001 |
| Sprint line | `docs/development/uats-gap-001/` |
| Date opened | 2026-06-11 |
| Branch | `reh3376_dev01` |
| Roadmap slot | Q3 Phase 2 (committed member, post BACKUP-RESTORE-VERIFY-001) |
| Estimated effort | 3 dev-days (roadmap); spec-authoring heavy |
| OpenAI spend | $0 |
| Risk level | Low (additive test specs; no production code expected — surprises become fix commits) |

## 2. Problem Statement

Zero UATS contract specs cover the /strict surface (`/v1/jiminy/strict`,
`/reformulate`, `/classify`), the warm-store channel (`/v1/jiminy/warm`,
`/latest`), or the DH-004 operator escape hatch (`/v1/admin/breakers`
± `/reset`). Two of these endpoints already produced live incidents
(Follow-up C on `/latest`, GUIDANCE-SYNTH-001 on `/warm`), and
HOOKWIRE-001 made them load-bearing per prompt. Likewise, zero specs
reference the `?sparse=` / `?context=auto` retrieval URL params shipped
default-on in Phases 14.x. An uncontracted load-bearing surface regresses
silently — the dominant historical bug class.

## 3. Scope & Constraints

**In scope**: 8 new `.uats.json` specs (the 7 endpoints + retrieve URL-param
response-shape contract), hash-stamped, passing live against the dev stack;
disabled-path (503) contracts documented as skip-variants where the live
deployment has the feature enabled (running them would require a config
flip + restart per variant — out of proportion for contract tests).

**Out of scope (disclosed)**: UXTS-CI-001 (wiring tagged specs into CI —
its own roadmap sprint); MCP tool contracts (MCP-REVIVE-001); new
endpoint behavior changes.

**Constraints**: specs must mirror the existing UATS 1.0.0 conventions
(deep-merged variants, `config.sha256` hash verification, tag filtering);
live Tier 3 = the full new-spec set passing against the running server;
live-smoke surprises get their own fix commits.

## 4. Dependencies

- UATS runner v1.2.0 (`docs/api/api-spec/uats/runners/uats_runner.py`),
  hash tooling (`add-hashes` / `verify-hashes`).
- Handler ground truth (recon 2026-06-11): `handlers_jiminy.go`
  (strict 587-645, reformulate 548-585, classify 505-546, warm 265-356,
  latest 358-417), `handlers_breakers.go` (40-108), `handlers.go`
  retrieve URL params (448-504) + sparse debug fields (948-968).
- Live stack with `JIMINY_ENABLED=true`, warm store on, breakers
  registered, retrieval default-on.

## 5. Implementation Plan (sequential epics)

- **Epic 0** — plan + recon committed.
- **Epic 1** — Jiminy strict-surface specs: `jiminy_strict`,
  `jiminy_reformulate`, `jiminy_classify` (happy path, required-field
  400s, method guards, fail-open classify contract, disabled-503 skip
  variants).
- **Epic 2** — Warm-channel specs: `jiminy_warm` (202 warming|debounced
  union), `jiminy_latest` (warm=true shape ∪ no_guidance shape — the
  Follow-up C strict-JSON contract).
- **Epic 3** — Admin breaker specs: `admin_breakers_list`,
  `admin_breakers_reset` (incl. 404-with-available-list contract and
  reset round-trip against a real registered breaker).
- **Epic 4** — `memory_retrieve_sparse_context` (URL-param overrides,
  sparse debug-field presence contract, percentile validation,
  `?context=auto` derivation path).
- **Epic 5** — Tier 3: hash-stamp all specs; full suite run against the
  live server (new specs 100% pass; existing suite stays green).
- **Epic 6** — Documentation (final epic — never cut): CHANGELOG,
  CLAUDE.md note, sprint post; no feature doc (test specs, not an
  operator feature — the UATS README already documents the framework).

## 6. Testing Plan

Tier 1 = spec lint (runner parse + hash verify). Tier 2 = n/a (specs ARE
the integration contract). Tier 3 = full live run against the dev stack
(`validate-all`), including the pre-existing spec corpus to prove no
regression in the shared harness.

## 7. Commit Strategy

One commit per epic; surprises in live runs get their own fix commits;
single push (auto-PR).

## 8. Verification Checklist

- [ ] 8 new specs parse, hash-verify, and pass live (100% assertions)
- [ ] Existing UATS corpus still passes (no harness regression)
- [ ] Each spec covers: happy-path shape, required-field 400, method guard
- [ ] Disabled-path 503 contracts documented (skip variants with reason)
- [ ] `make test-api`-style invocation documented in sprint post
- [ ] CHANGELOG + CLAUDE.md + post.md

## 9. Documentation Update — Epic 6 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Live spec run exposes real endpoint defects (the sprint's purpose) | Medium | Positive | Own fix commits, disclosed |
| Warm/latest specs flaky on debounce timing | Medium | Low | Assert the 202 union (warming\|debounced) not a single state |
| classify spec triggers a real LLM call (latency) | Medium | Low | classify is Tier-1-embedding first; 5s handler timeout; spec timeout 15s |
| Breaker reset spec mutates a real breaker | Low | Low | Reset forces StateClosed — the healthy state; round-trip asserts it |

## 11. Documents Accessed

Recon report (agent, 2026-06-11) over: `uats_runner.py`, 4 representative
specs, `handlers_jiminy.go`, `handlers_breakers.go`, `handlers.go`,
`auth/middleware.go`, `config.go`; roadmap entry (UATS-GAP-001, line 52).

## 12. Rollback Procedures

Additive spec files only — revert commits. No data or config changes.
