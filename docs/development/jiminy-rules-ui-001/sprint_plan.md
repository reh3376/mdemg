# Sprint Plan — JIMINY-RULES-UI-001

## 1. Header & Metadata

- **Sprint ID:** JIMINY-RULES-UI-001
- **Format Version:** v1.0 (12-section)
- **Date:** 2026-08-13
- **Author:** Claude (per operator directive 2026-08-13, "users need a UI to add rules for Jiminy to enforce; …view/edit/remove")
- **Branch:** `reh3376_dev01`
- **Parent design task:** #105 (now completed via 2-round design discussion, this session)
- **Filed as task:** #114
- **Owner:** operator (reh3376) — writes gated behind flag flip

## 2. Problem Statement

Jiminy enforces a corpus of constraint + correction nodes (JIMINY-CORRECTION-PRODUCER-001), but users have no UI to add/view/edit/remove rules. Today the pathways are:

1. Rules land as CMS observations via `POST /v1/conversation/observe` (operator or a hook triggers it), then the auto-classifier tags them, then the consolidation cycle promotes them via `CreateConstraintNodes`/`CreateCorrectionNodes` to `role_type='constraint'|'correction'` nodes. Multi-hop, indirect, invisible to users.
2. Operator uses `mdemg jiminy constraint mark --code X --informational=true|false` (JIMINY-INFORMATIONAL-CATEGORY-001) — CLI-only.
3. Fable HITL sessions surface junk/dup rules; operator tombstones via SQL or CLI (JIMINY-CORPUS-001/002/003 and JIMINY-CORRECTION-CORPUS-001 pattern).

The gap: no user-facing "here are all the rules Jiminy enforces + add + edit + remove" surface. And the JIMINY-CORRECTION-CORPUS-001 finding showed 24 of 35 tombstoned corrections were duplicates — an active corpus-bloat class that a "warn on similarity ≥ 0.75 at create-time" gate would prevent recurring.

**Operator directive 2026-08-13**: "users need a UI to add rules for Jiminy to enforce (rules take various forms, need categorization) + view/edit/remove existing rules."

## 3. Scope & Constraints

### In scope
- **Backend endpoints** (4 new routes under `/v1/jiminy/rules/*`):
  - `GET /v1/jiminy/rules` — list with filters (?type, ?severity, ?category, ?scope, ?include_archived, ?limit)
  - `GET /v1/jiminy/rules/{code}` — rule detail + recent enforcement outcomes (last 7d from `constraint_outcomes`)
  - `POST /v1/jiminy/rules` — create (with dedup-warn shape)
  - `POST /v1/jiminy/rules/{code}/tombstone` — soft-delete (is_archived=true + archive_reason='ui_tombstone_<timestamp>')
- **UI Rules tab** in `/ui/`:
  - List view (filterable table + pagination + sort)
  - Detail side-panel (per-rule surfacing/outcomes history from `constraint_outcomes`)
  - "Add rule" modal (form with dropdowns + dedup-warn inline)
  - Tombstone-confirm dialog (with reversible-via-CLI reminder)
- **Arc-safety flag** `JIMINY_RULES_UI_WRITE_ENABLED` — code default FALSE; operator flips to TRUE in `.env` on 2026-08-19 or when arc closes early. READ endpoints unconditional.

### Locked decisions (from 2-round design discussion)
- **Authorship**: operator only (same trust model as `mdemg jiminy constraint mark`)
- **Taxonomy**: reuse existing 3-axis (type × severity × category) + optional scope; scope dropdown populates from the 9 shipped LEVER-C-TIGHTEN-002 scope families (git / file_mutation / bash / schema / identifier / testing / process_docs / llm_config / cms)
- **Lifecycle**: immutable + tombstone-and-recreate (no in-place edit; matches JIMINY-CORPUS-001 pattern; NO schema migration)
- **UI placement**: new dedicated "Rules" tab in `/ui/` (not merged into Jiminy tab)
- **Default state**: active + actionable on publish (publish = live enforcement immediately)
- **Dedup**: warn ≥ 0.75 similarity + operator override (mirrors CREATE-CORRECTION-DEDUP-001 threshold)

### Constraints
- **NO new schema** — reuses `MemoryNode` with `role_type='constraint'|'correction'` + `is_archived` + `archive_reason` + `is_informational` + `constraint_code`
- **CUIDv2 for node_id + constraint_code** — per `never-use-uuid-v4` + `must-use-cuid2` shipped rules
- **Space-scoped** — all writes bounded to `space_id` from the request; NEVER cross-space
- **RSICProtectedSpaces** — the shipped `is_archived` tombstone (never DELETE) is the only mutation form; no destructive delete
- **Route inventory adjudication** — 4 new routes MUST be inventoried in `docs/api/route_consumer_inventory.json` in the same PR (DORMANT-CENSUS-001 forcing function)
- **DisallowUnknownFields** — leverage REVIEW-GRADE-NOTES-FIELD-001's readJSON improvement; new endpoints surface offending fields in errors

### Out of scope (explicit)
- ❌ Multi-user authorship (operator-only)
- ❌ Edit-in-place (immutable-tombstone)
- ❌ Draft workflow (active-on-publish)
- ❌ Version history schema (no schema change)
- ❌ Rule import/export (deferred follow-up)
- ❌ Auto-detection of rules from RSIC (deferred follow-up)
- ❌ HITL corrections-review tab (separate future sprint)
- ❌ Cross-space rule copy/paste

## 4. Dependencies

### Shipped infrastructure this sprint composes over
- **Neo4j schema**: `MemoryNode` with `role_type`, `is_archived`, `archive_reason`, `is_informational`, `constraint_code`, `constraint_type`, `content`, `embedding` (already present)
- **Vector index**: `memNodeEmbedding` (used by CREATE-CORRECTION-DEDUP-001 for dedup query — same shape here)
- **Constraint code minting**: `BootstrapCodes` (from CORRECTION-CODE-GEN-001) for auto-code generation on manually-authored rules
- **Neo4j write path**: `internal/hidden/service.go` execution shape (transactional Cypher via `session.ExecuteWrite`)
- **UI tab pattern**: JIMINY-MODE-001's Jiminy tab (`internal/api/ui/tabs/jiminy.js` + `internal/api/ui/api.js`) — mirror shape for consistency
- **TSDB `constraint_outcomes`**: rule-detail view reads recent outcomes for enforcement history
- **readJSON improvement**: REVIEW-GRADE-NOTES-FIELD-001 — new endpoints benefit automatically from named-field errors
- **Playwright infra**: existing `tests/e2e/browser-ui/` conventions for UI e2e coverage

### Ruling-based dependencies (must be honored)
- **`never-classify-policy-docs-as-constraint`** (operator ruling 2026-08-13, shipped this session as a constraint node) — the UI's "Add rule" form MUST NOT allow creating a rule whose content is descriptive of policy (e.g., "The SECURITY.md file describes X"); the dedup gate + LLM classifier should also refuse promotion
- **`must-enforce-jiminy-constraints`** — active + actionable on publish is a direct implementation of this directive
- **`must-follow-12-section-format`** — this sprint plan follows it
- **`mandatory-feature-docs`** — feature doc lands in Epic 1
- **`end-with-docs-accessed`** — mandatory Documents Accessed list at end of this plan + sprint post

## 5. Implementation Plan (sequential epics + gates)

Per `auto-c0a62b1da979`: Epic N MUST complete fully before Epic N+1 begins.

### Epic 1: Documentation + design (this file + feature doc)
**Deliverables:**
- `docs/development/jiminy-rules-ui-001/sprint_plan.md` (this file)
- `docs/features/jiminy-rules-ui.md` (feature doc — Why / Choices / How it works / How to use / Follow-ups)

**Gate:** both docs exist; operator has reviewed sprint plan; taxonomy + endpoint shape locked before code moves.

### Epic 2: Backend endpoints (READ-only)
**Deliverables:**
- New file `internal/api/handlers_jiminy_rules.go`:
  - `handleRulesList` (GET) — Cypher query filtered by `role_type IN ['constraint','correction']` + optional filters + pagination; returns `{data: {items: [...], total, next_cursor}}`
  - `handleRulesDetail` (GET `/v1/jiminy/rules/{code}`) — single-rule detail + recent outcomes from `constraint_outcomes` (last 7d)
- Route registration in `internal/api/server.go` under `ScopeAdminSpaces`
- 4-6 pin tests (`internal/api/handlers_jiminy_rules_test.go` — new): list happy path, filter combinations, pagination, detail happy path, detail 404, method-not-allowed shapes

**Gate:** `go test ./internal/api/` PASS; `golangci-lint run ./internal/api/` clean; live smoke on mdemg-dev returns expected shape.

### Epic 3: Backend endpoints (WRITE, flag-gated)
**Deliverables:**
- Extend `handlers_jiminy_rules.go`:
  - `handleRulesCreate` (POST) — validate 3-axis fields; run dedup vector query (same as CREATE-CORRECTION-DEDUP-001 shape); if `?override_dedup=false` (default) and hits ≥ 0.75 similarity → return 409 with `{data: {similar_rules: [...]}}`; if `?override_dedup=true` OR no hits → mint CUIDv2 constraint_code + node_id, write MemoryNode via `session.ExecuteWrite`, return `{data: {node_id, constraint_code, similar_count}}`
  - `handleRulesTombstone` (POST `/v1/jiminy/rules/{code}/tombstone`) — set `is_archived=true` + `archive_reason='ui_tombstone_<timestamp>'` + `archived_at=datetime()`; return `{data: {node_id, code, previous_state}}`
- Flag `JIMINY_RULES_UI_WRITE_ENABLED` in `internal/config/config.go` (default FALSE); both WRITE handlers return 503 `"rule mutation is currently disabled (JIMINY_RULES_UI_WRITE_ENABLED=false)"` when off
- Pin tests: happy-path create, dedup-warn shape, override-dedup path, tombstone happy path, flag-off returns 503, item_id validation

**Gate:** `go test ./internal/api/` PASS; `golangci-lint run ./internal/api/` clean; flag-off smoke returns 503; flag-on smoke on a scratch space creates + tombstones cleanly.

### Epic 4: UI Rules tab
**Deliverables:**
- New file `internal/api/ui/tabs/rules.js` (mirrors `internal/api/ui/tabs/jiminy.js` structure)
- New file `internal/api/ui/tabs/rules.css` (mirrors jiminy.css)
- Tab registration in `internal/api/ui/index.html` + `internal/api/ui/nav.js`
- Extend `internal/api/ui/api.js` with `rulesList(...)`, `rulesDetail(code)`, `rulesCreate(...)`, `rulesTombstone(code, reason)` helpers
- List view: filterable table (type/severity/category/scope/archived); column sort; page-through UI; row-click opens detail
- Detail side-panel: rule content + metadata + recent surfacing/outcomes (line chart or table); "Tombstone" button (opens confirm dialog); "Show similar rules" (uses dedup query for exploration)
- "Add rule" modal: form with dropdowns (type × severity × category × optional scope); content textarea; on save → POST /v1/jiminy/rules with `override_dedup=false`; on 409 dedup warn → inline "Similar rules exist: [tags with code + similarity%]; [Confirm anyway] [Cancel]"; on 503 flag-off → clean "Rule creation is disabled; contact operator to enable JIMINY_RULES_UI_WRITE_ENABLED"

**Gate:** UI renders; list + detail READ working live; ADD form renders but "Confirm" button either succeeds (flag on) or shows a friendly disabled state (flag off).

### Epic 5: Live Tier-3 + Playwright e2e
**Deliverables:**
- Live smoke on mdemg-dev:
  - READ: GET /v1/jiminy/rules returns real corpus (from JIMINY-CORPUS-003 post-purge state); GET detail on a specific code returns outcome history
  - WRITE (temporarily flip flag ON for smoke, revert after): scratch-space create + dedup-warn trip + tombstone flow
- Playwright e2e (`tests/e2e/browser-ui/test_rules_tab.py`):
  - Tab loads
  - Filters work
  - Detail opens
  - Add-form validates required fields
  - Dedup-warn shows when similar rules exist
  - Tombstone-confirm dialog appears

**Gate:** All smoke steps pass; Playwright suite green; server logs clean of errors.

### Epic 6: CI + route inventory
**Deliverables:**
- Route inventory adjudication (DORMANT-CENSUS-001 forcing function): add 4 entries to `docs/api/route_consumer_inventory.json` with disposition ACTIVE + consumer `ui:api.js` (for the tab-consuming ones) or `cli:mdemg jiminy rules` (if we add a CLI companion; see follow-ups)
- Run `python3 scripts/verify_route_consumers.py` — MUST return `OK`
- `mdemg config validate` regression check with the new env var

**Gate:** verify_route_consumers.py PASS; CI Lint checks green (route consumer guard); no drift.

### Epic 7: CLAUDE.md pin + CHANGELOG + sprint post
**Deliverables:**
- Sprint post at `docs/development/jiminy-rules-ui-001/sprint_post.md` (per `mandatory-feature-docs` and `end-with-docs-accessed`)
- CHANGELOG entry under `## [Unreleased]` → `### Added`
- CLAUDE.md pin naming the 4 new endpoints + the arc-safety flag + 2 new arch rules (see §10)

**Gate:** all docs written + committed; pushed to `reh3376_dev01`; auto-PR fires; CI green.

## 6. Testing Plan (3 tiers)

### Tier 1: Unit tests
- `handlers_jiminy_rules_test.go`:
  - List: happy path, filter-combos, pagination, empty result, method-not-allowed, wrong-scope
  - Detail: happy path, unknown-code 404, method-not-allowed
  - Create: happy path (flag on), dedup-warn shape (409 with similar_rules), override-dedup path, missing-required-field 400, wrong-role rejection, flag-off 503
  - Tombstone: happy path, already-archived idempotent, unknown-code 404, flag-off 503
- Every handler asserts response shape (data envelope + expected fields) and status code
- `readJSON` unknown-field passthrough test (already covered by REVIEW-GRADE-NOTES-FIELD-001 pin but we spot-check on this handler)

### Tier 2: Integration
- Real Neo4j smoke via docker-compose test: create → list → detail → tombstone → list-with-include-archived
- Real vector-index dedup query: seed 2 near-duplicate rules, verify create-attempt on 3rd hits dedup-warn
- Real `constraint_outcomes` join: create rule, seed a fake outcome row, verify detail includes it

### Tier 3: E2E (live system) — REQUIRED per `live-testing-tier-required` + `must-e2e-live-data-verify`
- Run mdemg live binary against real Neo4j + TSDB on mdemg-dev (read-only during arc)
- Playwright browser-ui suite: tab loads + filters work + detail opens + add-form UI renders + dedup-warn UI renders when similar-rules exists + tombstone-confirm dialog appears
- Observe actual side-effects: after operator flips WRITE flag on 2026-08-19, real create lands in Neo4j (verify via direct Cypher); real tombstone flips is_archived=true (verify)

## 7. Commit Strategy

Three commits (one per implementation cluster + one for docs):
- **Commit 1 (Epic 2)**: `feat(jiminy): JIMINY-RULES-UI-001 Epic 2 — READ endpoints for rules corpus` (backend + tests, no UI)
- **Commit 2 (Epics 3+4)**: `feat(jiminy,ui): JIMINY-RULES-UI-001 Epics 3+4 — WRITE endpoints (flag-gated) + Rules tab` (backend WRITE + full UI + tests + Playwright)
- **Commit 3 (Epics 5+6+7)**: `docs(jiminy): JIMINY-RULES-UI-001 Epics 5+6+7 — live Tier-3 verify + route inventory + docs + pin` (verify + inventory + sprint post + CHANGELOG + CLAUDE.md pin)

All 3 commits sign-off via HEREDOC + `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`.

Each commit pushed to `reh3376_dev01`; auto-PR fires; operator merges; CI green.

## 8. Verification Checklist

- [ ] Epic 1: sprint plan + feature doc written (this epic)
- [ ] Epic 2: READ endpoints ship + unit tests green + live smoke returns real corpus
- [ ] Epic 3: WRITE endpoints ship + flag-off returns 503 + unit tests green
- [ ] Epic 4: Rules tab renders + filters work + Add modal renders + dedup-warn UI renders
- [ ] Epic 5: **LIVE SMOKE — operator opens /ui/rules, sees actual JIMINY-CORPUS-003 rules (~33 constraints + 3 corrections); filters work; detail shows real outcome history; Playwright e2e green**
- [ ] Epic 6: `verify_route_consumers.py` OK; `mdemg config validate` OK; CI Lint green
- [ ] Epic 7: sprint post + CHANGELOG + CLAUDE.md pin all committed
- [ ] Final: all 3 commits pushed; auto-PR merged; task #114 marked completed

## 9. Documentation Update (final epic — never cut)

- **Feature doc** `docs/features/jiminy-rules-ui.md` — lands in Epic 1 per `mandatory-feature-docs`
- **Sprint post** `docs/development/jiminy-rules-ui-001/sprint_post.md` — lands in Epic 7 per `end-with-docs-accessed`
- **CHANGELOG entry** — lands in Epic 7 under `## [Unreleased] → ### Added`
- **CLAUDE.md pin** — lands in Epic 7; names the 4 new endpoints + `JIMINY_RULES_UI_WRITE_ENABLED` flag + 2 arch rules
- **Route inventory** — `docs/api/route_consumer_inventory.json` extended in Epic 6

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Arc measurement pollution if write flag flipped early | Low | Medium | Flag DEFAULTS off in code; `.env` stays off through 2026-08-19; operator gate |
| Dedup false negatives (near-duplicate slips past 0.75 threshold) | Medium | Low | Threshold configurable via `JIMINY_RULES_DEDUP_SIM_THRESHOLD` env; operator can tighten if bloat class recurs |
| Neo4j write contention with consolidation cycle | Low | Low | Writes are single-node MERGE via ExecuteWrite; consolidation is on a separate transaction; no lock contention observed in JIMINY-INFORMATIONAL-CATEGORY-001 |
| Playwright e2e flakes on tab load timing | Medium | Low | Use existing `waitForSelector` patterns from `test_jiminy_tab.py`; retries built into browser-ui infra |
| Operator creates a rule that violates the `never-classify-policy-docs-as-constraint` ruling | Medium | Medium | UI has a soft-warn on the content field if it looks doc-shaped (starts with "The X.md file", "This document describes"…); doesn't hard-block (operator override) |
| Route inventory miss (repeat of PR 614 CI failure class) | Low | High (CI-blocking) | Epic 6 GATE explicitly runs `verify_route_consumers.py`; commit 3 won't push until it's green |

## 11. Rollback Procedures

Because lifecycle is immutable-tombstone-and-recreate, rollback is a first-class supported operation:

- **Rollback a UI-created rule** (post-2026-08-19): call `POST /v1/jiminy/rules/{code}/tombstone` → `is_archived=true`. The node stays in Neo4j; queries + Jiminy surfacing exclude it (existing `NOT coalesce(is_archived, false)` filter in JIMINY-ARCHIVED-CODE-FILTER-001).
- **Un-tombstone (reverse the rollback)**: direct Cypher only — no UI endpoint (deliberate; keeps the reverse path operator-gated):
  ```cypher
  MATCH (n:MemoryNode {constraint_code: $code})
  SET n.is_archived = false, n.archive_reason = null, n.archived_at = null
  RETURN n.node_id
  ```
- **Full sprint rollback** (worst-case, if a critical bug ships): revert the 3 commits via `git revert`, kickstart server. No schema change means no migration to reverse.
- **Substrate cleanup after live smoke** (Epic 5 fake data): tombstone all `constraint_code STARTS WITH 'jiminy-rules-ui-001-smoke'` nodes via direct Cypher; documented in Epic 5 exit checklist.

## 12. Documents Accessed

- `CLAUDE.md` — the 6 shipped rules cited in Dependencies + Constraints + Risks (must-follow-12-section-format, mandatory-feature-docs, end-with-docs-accessed, sequential-epics, live-testing-tier-required live-testing-required, must-e2e-live-data-verify, never-classify-policy-docs-as-constraint, must-enforce-jiminy-constraints, must-use-cuid2)
- `docs/development/jiminy-informational-category-001/sprint_post.md` — the CLI shape this UI mirrors (mark/unmark, dry-run, list)
- `docs/development/jiminy-corpus-001/sprint_post.md` + `jiminy-corpus-002/` + `jiminy-corpus-003/` — tombstone-safety pattern
- `docs/development/jiminy-correction-corpus-001/sprint_post.md` — dedup-class evidence (24/35 duplicates)
- `docs/development/create-correction-dedup-001/sprint_post.md` — the promotion-time dedup pattern this UI's create endpoint mirrors
- `docs/development/jiminy-mode-001/sprint_post.md` — the UI tab pattern (Jiminy tab structure)
- `docs/development/enforce-ui-overrides/sprint_post.md` — the /ui/ table + timeline pattern
- `docs/development/lever-c-tighten-002/sprint_post.md` — the 9 scope families the "scope" dropdown reuses
- `docs/development/review-grade-notes-field-001/sprint_post.md` — readJSON improvement that new endpoints benefit from
- `docs/development/hitl-review-001/sprint_post.md` — the dataset-agnostic UI CRUD pattern (Rules tab is a specialization of this shape)
- `docs/development/dormant-census-001/` — the route inventory forcing function
- `internal/hidden/service.go::CreateConstraintNodes` + `CreateCorrectionNodes` — the promotion sites that today mint the corpus this UI will manage
- `internal/hidden/correction_gate.go` + `constraint_gate.go` — the promotion-time backstops the UI's create endpoint mirrors
- `internal/api/handlers_review.go` — the CRUD-endpoint shape reference
- `internal/api/ui/tabs/jiminy.js` — the tab template
- Live: `POST /v1/conversation/observe` for the operator taxonomy ruling recording; TSDB `constraint_outcomes` schema inspection; Neo4j MemoryNode schema (via existing Cypher in CreateCorrectionNodes)
