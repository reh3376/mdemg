# JIMINY-RULES-UI-001 — Sprint Post

**Date:** 2026-08-13 → 2026-08-14
**Branch:** `reh3376_dev01`
**Sprint plan:** [sprint_plan.md](./sprint_plan.md)
**Feature doc:** [../../features/jiminy-rules-ui.md](../../features/jiminy-rules-ui.md)

## Summary

Shipped a dedicated `/ui/rules` tab that gives operators a first-class surface to view + add + edit + tombstone the Jiminy rule corpus. Backend is 4 new endpoints under `/v1/jiminy/rules/*`; WRITE endpoints (create + tombstone) are flag-gated behind `JIMINY_RULES_UI_WRITE_ENABLED` (default false through the JIMINY-CEILING-BREAK-2 arc window; operator flips 2026-08-19). READ endpoints unconditional + arc-safe. No new schema — reuses shipped `MemoryNode role_type='constraint'|'correction'` + `is_archived` + `is_informational` + `constraint_code`.

## Epics (7, executed sequentially per `sequential-epics` rule)

- **E1: Docs + design** — sprint plan + feature doc; locked 7 design decisions via 2-round operator discussion
- **E2: Backend READ endpoints** — `GET /v1/jiminy/rules` (filterable) + `GET /v1/jiminy/rules/{code}` (detail + recent outcomes). Commit `013d6974`.
- **E3: Backend WRITE endpoints (flag-gated)** — `POST /v1/jiminy/rules` (create + dedup) + `POST /v1/jiminy/rules/{code}/tombstone`. Commit `9b4e20ce`.
- **E4: UI Rules tab** — `internal/api/ui/tabs/rules.js` + tab registration + api.js helpers. Commit `3aa4b714`.
- **E5: Live Tier-3 + Playwright + UX-review revisions** — 3 rounds of operator hand-inspection revealed real bugs (below); this commit.
- **E6: Route inventory** — completed inside E2's commit (adjudicated at that time).
- **E7: Sprint post + CHANGELOG + CLAUDE.md pin** — this document + associated meta-docs.

## UX-review iterations (Epic 5)

Per the just-shipped MANDATORY UI-COMPLETION RULING (recorded during Epic 5 as a durable constraint), no UI work is complete without both (a) agent-side automated tests AND (b) operator hand-inspection. Operator surfaced 3 real UX/data bugs across 3 review rounds; all fixed with re-run Playwright + re-surface.

**Round 1 — 503 error message unhelpful:**
- Bug: Save failed on 503 showed generic `Save failed: 503 Service Unavailable`
- Root cause: shared `post()` helper dropped the response body; only `res.status + res.statusText` reached the throw
- Fix: `rulesTombstone` refactored to direct-fetch + `.payload` propagation (matches `rulesCreate` shape); Save/Tombstone handlers use `err.status === 503` branch to surface `Save disabled: ...` with the shipped rich body

**Round 2 — contrast unreadable:**
- Bug: expanded row content was invisible (white bg × light-blue font)
- Root cause: hardcoded `#f8f8fa` / `white` / `#ddd` light-theme fallbacks in a Catppuccin Mocha dark theme
- Fix: all inline styles use theme vars (`var(--surface0)`, `var(--mantle)`, `var(--text)`, `var(--surface1)`)

**Round 3 — expansion cascades on duplicates:**
- Bug: clicking one rule expanded a near-duplicate below it too
- Root cause: expansion keyed on `constraint_code` (duplicates share codes → both matched)
- Fix: keyed on `node_id` (CUIDv2, guaranteed unique per rule); metadata rendered from the LIST item directly (no DETAIL round-trip conflating duplicates); outcomes still fetched by code (semantically per-code)
- **Bonus finding**: the reported duplicates surfaced a real corpus issue → operator handled via same-session dispositions + spawned JIMINY-CORPUS-AUDIT-004 for the broader corpus review

## Live Tier-3 (mdemg-dev, 2026-08-13 → 2026-08-14)

- **READ** live-verified across LIST (with 5 filter axes + include_archived), DETAIL (with 7d outcomes), 404 on unknown code
- **WRITE** end-to-end smoke on a scratch space (with flag temporarily flipped): create → dedup-warn 409 with similar_rules (sim=0.9931) → override → tombstone → include_archived shows both; revert to 503
- **Playwright**: 9/9 PASS across the 3 UX-review iterations
- **Real corpus manipulation via the shipped Save flow**: JIMINY-CORPUS-AUDIT-004's 7 content rewrites all executed via the UI-Save 2-call pattern (tombstone + create with same code), validating the round-1 immutable-tombstone lock in production

## New arch rules pinned (CLAUDE.md)

1. **UI-completion contract** (recorded during Epic 5 as constraint `faeofh6bixlc…`): no UI work is complete without both agent-side automated review (Playwright e2e or equivalent) AND operator user-side hand-inspection. Applies to every future UI-touching sprint.
2. **Contrast contract for dashboard tabs**: all inline styles MUST use the shipped Catppuccin theme variables (`var(--surface0)`, `var(--mantle)`, `var(--text)`, `var(--surface1)`) — never hardcoded light-theme colors. The dashboard is dark-theme by default; light-theme fallbacks are illegible.
3. **Row-identity keying for accordion/select UI**: when an operator UI shows a list where duplicates by any human-mnemonic code CAN exist, keying selection/expansion state on the mnemonic will cascade. ALWAYS key on the truly-unique identifier (CUIDv2 `node_id` here). Client-side detail metadata should render from the LIST item directly, not a re-fetched detail endpoint that may collapse duplicates.
4. **Rich error surface for flag-gated endpoints**: helper wrappers around fetch that only preserve `${status} ${statusText}` on error swallow the operator-actionable message. When calling a flag-gated endpoint, use a direct-fetch pattern that attaches `.status + .payload` to the thrown error so the UI can surface the flag-flip instructions verbatim.

## Follow-ups disclosed (filed as tasks)

- **#116 JIMINY-CLASSIFIER-META-SCOPE-001** — classifier over-triggers on "editing content that MENTIONS the ruling" (false-positive from Epic 2 route-inventory edit hit `never-classify-policy-docs-as-constraint`). Deferred post-arc.
- **JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001** — the constraint_detector regex mints multiple L1 nodes per L0 observation when multiple pattern-variants match (e.g. same content matching both `must` and `must_not`). 2 instances found in the audit today. Fix belongs at the promotion layer; sibling of CREATE-CORRECTION-DEDUP-001 which operates at the vector-similarity layer.
- **CLI companion** (`mdemg jiminy rules list|create|tombstone`) — deferred; UI covers the operator surface today.
- **UI edit-mode notes field** — REVIEW-GRADE-NOTES-FIELD-001 shipped `notes` for grades; a similar operator-reason field on rules edits (beyond the archive_reason) would help audit trails. Deferred.

## Documents Accessed

- `docs/development/jiminy-rules-ui-001/sprint_plan.md` (this sprint's own plan)
- `docs/features/jiminy-rules-ui.md` (feature doc)
- `docs/development/jiminy-corpus-audit-004/batch_record.md` — the corpus audit that used this sprint's shipped Save flow to execute 7 content rewrites live
- CLAUDE.md pins consumed: HITL-CURATION-002 (auto-grader invariant), JIMINY-CORPUS-001/002/003 (tombstone-safety), JIMINY-INFORMATIONAL-CATEGORY-001 (is_informational + CLI shape), JIMINY-ARCHIVED-CODE-FILTER-001 (reader-side archive filter), JIMINY-MODE-001 (tab pattern), ENFORCE-UI-OVERRIDES (table + timeline UI pattern), LEVER-C-TIGHTEN-002 (9 scope families), REVIEW-GRADE-NOTES-FIELD-001 (readJSON improvement), DORMANT-CENSUS-001 (route inventory), PRE-COMMIT-INVENTORY-GATE-001 (this session's inventory forcing function)
- `internal/api/handlers_jiminy_rules.go`, `handlers_jiminy_rules_test.go` — Epic 2/3 backend
- `internal/api/ui/tabs/rules.js`, `api.js`, `main.js`, `index.html` — Epic 4 + UX-review revisions
- `tests/e2e/browser-ui/test_rules_tab.py` — Epic 5 Playwright
- Live: 3 rounds of operator hand-inspection; 9/9 Playwright green; scratch-space WRITE smoke; 7 real-corpus rewrites via the shipped Save flow
