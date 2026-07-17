# Sprint JIMINY-ROLETYPE-ADAPTER-001 — Propagate role_type through the retrieval→jiminy adapter

## 1. Header & Metadata

- **Sprint ID:** JIMINY-ROLETYPE-ADAPTER-001
- **Sprint line:** `docs/development/jiminy-roletype-adapter-001/`
- **Date opened:** 2026-07-13
- **Target version:** v0.11.1 (patch — additive fields + a widened classifier, no breaking changes)
- **Estimated effort:** ~0.5 dev-day (5 sequential epics)
- **OpenAI spend:** $0 (all-local; no LLM call additions)
- **Risk level:** Low — additive struct fields, single Cypher `RETURN` extension, one classifier `switch` widening; no schema migration, no default flip

## 2. Problem Statement

`RetrieveForJiminy` mis-types every retrieval-sourced guidance item as `GuidanceLearning`. Trace:

1. `internal/models/models.go::RetrieveResult` has no `RoleType` / `ObsType` fields.
2. `internal/retrieval/service.go::Candidate` has no `RoleType` / `ObsType` fields.
3. `vectorRecall` Cypher (`service.go:1247`) does not `RETURN node.role_type` / `node.obs_type`.
4. Both scorer paths — `scoring.go:874` (linear) and `scoring_rrf.go:143` (RRF; default-on since Phase 13.1) — build `RetrieveResult` from `c`, but `c` never had the fields to copy.
5. `internal/api/rsic_adapters.go:384::jiminyRetrievalAdapter.RetrieveForJiminy` copies only 5 fields into `jiminy.RetrievalResult` (which does have `ObsType`) — nothing upstream to copy.
6. `internal/jiminy/retrieval_source.go::classifyRetrievalItem` reads empty `ObsType` → falls through the switch → `default: return GuidanceLearning`.

**Live-verified state (2026-07-13):**
- 140 `role_type='constraint'` MemoryNodes at Layer 1 (pre-purge; branch tip reflects the JIMINY-CORPUS-001 purge to 61 once PR #499 lands).
- Zero `role_type='correction'` nodes anywhere in `mdemg-dev`.
- `constraint_outcomes.guidance_type` has never carried `'correction'` for a retrieval-sourced item.

Closes the disclosed follow-up from JIMINY-CORPUS-001 (`docs/features/jiminy-actionability.md` §Follow-up: "`RetrieveForJiminy` role_type adapter gap").

## 3. Scope & Constraints

### In scope
- Add `RoleType string` + `ObsType string` to `retrieval.Candidate` and `models.RetrieveResult`.
- Add `RoleType string` to `jiminy.RetrievalResult` (`ObsType` already exists).
- Extend `vectorRecall` Cypher `RETURN` with both columns; populate the Candidate; carry through to `RetrieveResult`.
- Extend both scorers to copy the two new fields.
- Extend `jiminyRetrievalAdapter.RetrieveForJiminy` to copy both.
- Widen `classifyRetrievalItem`: prefer `RoleType == "constraint"` / `"correction"` before the Layer≥2 short-circuit and the `ObsType` switch. Default fallback preserved.
- Unit + adapter tests.
- Live Tier-3 smoke on `mdemg-dev`.
- Canonical docs (CLAUDE.md, CHANGELOG, feature doc flip, post.md).

### Out of scope
- Producing `role_type='correction'` nodes.
- Retuning JIMINY_SURFACE_ACTIONABLE_WEIGHT or Lever A quotas.
- Schema / migration / compose changes.
- Non-vector-recall candidate fetches (verify in E1 that BM25/graph/structural columns produce seed views, not sinks).

### Constraints
- Sequential epics.
- Live Tier-3 required.
- No hardcoded config values.
- RRF-SCALE-001-safe (no new score gate).
- No CUIDv2 mint sites.
- Protected `mdemg-dev` — read-only during smoke.

## 4. Dependencies

- **PR #499 (JIMINY-CORPUS-001)** should merge first (baseline cleanliness); not a hard blocker.
- Existing `role_type` / `obs_type` MemoryNode properties.
- No new env vars, migrations, or MCP tools.

## 5. Implementation Plan

### Epic 0 — Sprint plan committed
This document, on `reh3376_dev01`.

### Epic 1 — Propagate role_type + obs_type through retrieval

Files touched:
- `internal/models/models.go` — `RoleType string ` + `ObsType string ` on `RetrieveResult` (both with `omitempty` json tags).
- `internal/retrieval/service.go` — same fields on `Candidate`; extend `vectorRecall` `RETURN` with `coalesce(node.role_type,'') AS role_type, coalesce(node.obs_type,'') AS obs_type`; populate in the record-loop.
- `internal/retrieval/scoring.go` — copy at line 875.
- `internal/retrieval/scoring_rrf.go` — copy at line 143.
- `internal/jiminy/types.go` — `RoleType string` on `RetrievalResult` (omitempty).
- `internal/api/rsic_adapters.go` — extend `RetrieveForJiminy` mapping.

Behavior-neutral (empty strings fall through the classifier default).

### Epic 2 — role_type-preferring classifier

`internal/jiminy/retrieval_source.go::classifyRetrievalItem` gains a prefix switch on `r.RoleType`: `"constraint"` → `GuidanceConstraint`, `"correction"` → `GuidanceCorrection`. Everything else unchanged.

### Epic 3 — Tier-1 unit + Tier-2 integration tests

Truth table (9 rows) in `retrieval_source_test.go`; adapter copy pin; full-suite green.

### Epic 4 — Live Tier-3 smoke

1. `go build -o bin/mdemg ./cmd/mdemg`
2. Replace running pid.
3. `POST /v1/memory/retrieve` for a constraint-relevant query; jq for `role_type` on results.
4. `POST /v1/jiminy/feedback`; query `constraint_outcomes` for `guidance_type='constraint'` rows.
5. Evidence to `live_verification.md`.

### Epic 5 — Canonical docs (never cut)

- CLAUDE.md architecture note.
- CHANGELOG `[Unreleased]` **Fixed** entry.
- `docs/features/jiminy-actionability.md` follow-up flipped from deferred to closed.
- `post.md` sprint close.

## 6. Testing Plan

- **Tier 1** — 9-row classifier truth table + adapter round-trip pin.
- **Tier 2** — no new; keep existing suite compiling with new field lists.
- **Tier 3** — before/after `guidance_type` distribution in `constraint_outcomes`.

## 7. Commit Strategy

One commit per epic on `reh3376_dev01`:
1. `docs(jiminy-roletype-adapter-001): E0 — sprint plan`
2. `feat(jiminy-roletype-adapter-001): E1 — propagate role_type + obs_type through retrieval`
3. `feat(jiminy-roletype-adapter-001): E2 — role_type-preferring classifier`
4. `test(jiminy-roletype-adapter-001): E3 — unit + adapter tests`
5. `docs(jiminy-roletype-adapter-001): E4 — live Tier-3 verification`
6. `docs(jiminy-roletype-adapter-001): E5 — CLAUDE.md/CHANGELOG/feature/post`

Auto-PR fires; sprint summary comment attached after E5.

## 8. Verification Checklist

- [ ] E0 committed
- [ ] `models.RetrieveResult` carries `role_type` + `obs_type`
- [ ] `vectorRecall` Cypher returns both columns
- [ ] Both scorers copy both fields
- [ ] Adapter copies both fields
- [ ] Classifier truth table matches E3
- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./...` clean
- [ ] `go test ./...` full-suite green
- [ ] Live retrieve shows non-empty `role_type` on constraint nodes
- [ ] Live `constraint_outcomes` shows `guidance_type='constraint'` post-fix
- [ ] CLAUDE.md architecture note appended
- [ ] CHANGELOG entry added
- [ ] Feature doc follow-up flipped to closed
- [ ] `post.md` written

## 9. Documentation Update — Epic 5 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Another candidate-fetch path mis-types a subset | Low | Low | Scope-check `grep -n "Candidate{" internal/retrieval/*.go` in E1; batched hydrate if any sink lacks metadata. |
| Cypher latency delta | Very Low | Low | Two string properties; no new match/index. Re-check latency histogram post-restart. |
| RetrieveResult json shape consumer breakage | Very Low | Low | `omitempty` tags. Grep UATS fixtures. |
| Dashboards / metrics relying on the bug | Very Low | Medium | Desired end state — call out in CHANGELOG. |
| `jiminy.RetrievalResult` mock break | Low | Low | Mock is an interface impl, not a struct literal. |

## 11. Documents Accessed

- `internal/models/models.go` (~988)
- `internal/retrieval/service.go` (Candidate 1196, vectorRecall Cypher 1247, record-loop 1275)
- `internal/retrieval/scoring.go` (874)
- `internal/retrieval/scoring_rrf.go` (143)
- `internal/jiminy/types.go` (194)
- `internal/jiminy/retrieval_source.go` (classifyRetrievalItem)
- `internal/api/rsic_adapters.go` (380)
- `docs/features/jiminy-actionability.md` §Follow-up
- CLAUDE.md JIMINY-CORPUS-001 note
- Live Neo4j role_type census + live TSDB `constraint_outcomes` distribution (2026-07-13).

## 12. Rollback Procedures

No data mutations, no schema/compose changes. Rollback = revert the six commits (or `git revert` the merged squash-commit). Additive Cypher `RETURN` is safe on a rolled-back consumer.

## Acceptance Criteria

1. `POST /v1/memory/retrieve` responses carry `role_type` / `obs_type` on constraint-role hits.
2. `classifyRetrievalItem` emits `GuidanceConstraint` for retrieval-sourced constraint nodes.
3. Live `constraint_outcomes` gains at least one `guidance_type='constraint'` row post-fix.
4. Full test suite green; lint clean.
5. Canonical docs updated per §5 Epic 5.
