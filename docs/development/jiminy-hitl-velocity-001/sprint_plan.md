# JIMINY-HITL-VELOCITY-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint:** JIMINY-HITL-VELOCITY-001
- **Date:** 2026-08-12
- **Branch:** `reh3376_dev01`
- **Phase 4a of:** `docs/development/jiminy-ceiling-break-2/README.md`
- **Effort:** ~1 hour (tight-scope MVP)
- **Prior phases:** JIMINY-CORPUS-003 (Phase 1) + LEVER-C-TIGHTEN-002 (Phase 2) + JIMINY-CLASSIFIER-CONTEXT-002 (Phase 3) — all SHIPPED 2026-08-11/12

## 2. Problem Statement

The HITL curation platform is shipped end-to-end (HITL-REVIEW-001 + HITL-CURATION-002/003 + HITL-AUTO-DISMISS-001 + AUTOGRADE-SCHEDULE-001 + HITL-ANALYTICS-TILE-001) but corpus growth is bottlenecked on operator throughput: the JIMINY-CEILING-BREAK-2 arc analysis reported "4 pending grades in 7d" — the operator is grading at ~0.5 items/day, far short of the N=500+ golds the classifier retrain (Phase 4b) needs to move the follow-rate ceiling meaningfully.

Root cause: the current Review tab UI (`internal/api/ui/tabs/review.js`) is mouse-driven, single-item-per-page, requires per-dimension radio-click + Submit-button-click + Next-button-click for every grade. Even a fast operator takes ~30-60s per item; grading 40+ per session is grinding. Reading time is unavoidable; UI ceremony is not.

## 3. Scope & Constraints

**In-scope (MVP — 1 hour):**
- Keyboard shortcuts on the Review tab: number keys `0-4` populate ALL rubric dimension radios simultaneously (operator can differ per-dimension via mouse click); `enter`/`space` submits; `n` loads next; `u` undo/reverse; `?` shows a help overlay.
- Auto-load next item after successful submit (no manual click).
- Prominent session-graded counter at top of the item card (currently in the muted sub-line — visibility upgrade).
- Focus-management: on tab open + after item load, focus the review area so keyboard shortcuts work without clicking first.

**Explicitly out-of-scope for MVP:**
- Autograde-preview endpoint + pre-fill (would require new `/v1/review/autograde-preview` handler + LLM call per item). Defer to follow-up sprint if velocity data shows single-key grading still too slow.
- Per-dimension keyboard mapping (e.g. `1234` = dim1, `qwer` = dim2). Complex; premature. MVP shortcut sets ALL dims to the same number.
- Corpus-lift Grafana panel extension — HITL-ANALYTICS-TILE-001 already covers cadence.

## 4. Dependencies

- HITL-REVIEW-001 — the shipped Review UI + `/v1/review/*` endpoints. This sprint modifies existing UI file only; no new endpoints.
- HITL-ANALYTICS-TILE-001 — the `mdemg-hitl` Grafana dashboard reads `sessionGraded` growth via `grade_cadence` panel (server-side count from `review_grades` table). Nothing to change in dashboard.

## 5. Implementation Plan (sequential)

**E1 — Keyboard shortcuts + auto-advance in `review.js`:**
- `document.addEventListener('keydown', handleReviewKey)` inside review tab's render/init; teardown on unmount if the tab has one (check main.js for tab-switch pattern).
- Key bindings (only active when Review tab is visible and no text-field is focused):
  - `0-4`: set every rubric dimension radio to that value + focus updates
  - `space` / `enter`: click the "Submit grade" button
  - `n`: click "Next item →" if visible (post-submit state)
  - `u`: click "Reverse last grade" if visible (post-submit state)
  - `?`: toggle a help overlay listing shortcuts
- Auto-advance: `renderAfter` currently shows "Next item →" button; add `setTimeout(() => loadNext(), 400)` for a brief pause then auto-load. Operator can still cancel via `u` for reverse before advance.

**E2 — Focus + visibility polish:**
- Bump session-graded counter to prominent header row (was muted sub-text).
- Focus the review card container after each item load so keyboard shortcuts work immediately.

**E3 — Live smoke:**
- Open `:9999/ui/#review` in browser (or via curl to verify JS is served).
- Open dev-tools to confirm no JS errors on tab open.
- Grade 5 items on `guidance` dataset using ONLY keyboard: `4` → `space` → auto-advance → `4` → `space` → ...
- Verify session counter increments visibly + no clicks required.

**E4 — Docs:**
- Sprint post + CLAUDE.md pin + CHANGELOG.

## 6. Testing Plan

**Unit (T1):** N/A — pure UI change; test coverage on JS is not part of this repo's existing surface. Server-side handlers unchanged.

**Integration (T2):** Existing UATS review spec continues green (no API changes).

**Live (T3):**
- Manual keyboard smoke — 5 real items graded via keyboard-only flow. If any grade fails to submit or advance, fix before ship.

## 7. Commit Strategy

Single commit: `feat(ui): keyboard-driven bulk review for HITL grading (JIMINY-HITL-VELOCITY-001)`.

## 8. Verification Checklist

- [ ] `go build ./...` clean (no server-side changes; sanity check)
- [ ] `make verify-grafana-embed` clean (no changes; sanity check)
- [ ] Manual smoke: 5+ items graded keyboard-only
- [ ] No JS errors in dev-tools console
- [ ] Help overlay renders on `?` press
- [ ] Auto-advance fires after submit
- [ ] `u` undo works in the post-submit interval before auto-advance
- [ ] CHANGELOG entry
- [ ] CLAUDE.md pin

## 9. Risks & Mitigations

**R1: Keyboard shortcuts conflict with browser defaults (space scrolls, ? triggers browser help).**
- `event.preventDefault()` on handled keys.
- Only bind when Review tab is active AND focus isn't in a text field (check `document.activeElement.tagName`).

**R2: Auto-advance loses the "Reverse last grade" option if operator wanted to undo.**
- 400ms pause before advance gives operator a beat; `u` intercepts and reverses; `n` cancels advance and manually loads next.
- If a grade was wrong and auto-advanced past the button, operator can re-fetch the grade via `/v1/review/candidates` and reverse it there. Acceptable trade-off for velocity.

**R3: Fast-mode single-value-for-all-dimensions produces low-quality grades.**
- Operator can override any dimension with mouse click before pressing space.
- The rubric's dimensions are usually correlated (a "correct" verdict usually means all dimensions are correct); differ-mode edge cases are less common.
- If data shows fast-mode grades are systematically worse than manual grades, add per-dimension keyboard mapping as a follow-up.

**R4: Session counter isn't authoritative — reloading the tab resets it.**
- Client-side counter; HITL-ANALYTICS-TILE-001's server-side count from `review_grades` is authoritative. UI counter is UX-only.

## 10. Rollback Procedures

Single-file revert of `internal/api/ui/tabs/review.js`. Everything else unchanged.

## 11. Documents Accessed

- `docs/development/jiminy-ceiling-break-2/README.md` — Phase 4a spec + ceiling analysis
- `docs/development/jiminy-classifier-context-002/sprint_post.md` — Phase 3 precedent
- `internal/api/ui/tabs/review.js` — the file to modify
- `internal/api/handlers_review.go` — server-side (unchanged; understand API contract)
- `docs/features/hitl-auto-curation.md` — HITL platform overview
- CLAUDE.md pins: HITL-REVIEW-001, HITL-CURATION-002, HITL-CURATION-003, HITL-ANALYTICS-TILE-001, JIMINY-CEILING-BREAK-2
