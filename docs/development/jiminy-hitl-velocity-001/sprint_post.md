# JIMINY-HITL-VELOCITY-001 — Sprint Post

**Date:** 2026-08-12
**Branch:** `reh3376_dev01`
**Phase 4a of:** `docs/development/jiminy-ceiling-break-2/README.md`

## Shipped

Keyboard-driven bulk-review UI in the shipped HITL Review tab (`internal/api/ui/tabs/review.js`). MVP scope per sprint_plan.md — smallest addition that unlocks the operator to grade 40+ items per session.

Key bindings (only fire when Review tab is visible + no text field focused):
- **`0`..`4`** — set every rubric dimension to that value (mouse-click still available per-dimension for differ-mode)
- **`Space` / `Enter`** — submit the current grade
- **`n`** — load next item (also works to advance manually before auto-advance timer fires)
- **`u`** — reverse (undo) the last grade (post-submit)
- **`?`** — toggle help overlay
- **`Esc`** — close help overlay OR cancel pending auto-advance

Additional UX polish:
- **Auto-advance** — after a successful submit, load next item in 400ms. Status line tells operator "auto-advance in 400ms — press u to reverse, n to advance now, esc to cancel." Operator has real time to intercept without breaking flow.
- **Prominent session counter** — bright green banner at top of item card shows `Session graded: N   ·   Shortcuts: 0-4 grade · space submit · n next · u undo · ? help`. Was buried in muted sub-text.
- **`currentDimensionInputs` module state** — exposes the per-item radio input state to the keyboard handler so `0-4` can set all dimensions in one keystroke.
- **Idempotent `bindKeyboardHandler`** — safe to re-bind on every tab render; global `keydown` listener persists across tab switches but is gated by `isReviewTabActive()`.

Server-side: no changes. Uses existing `/v1/review/*` endpoints.

## Live Tier-3 (mdemg-dev, 2026-08-12)

- `go build ./... clean`
- Binary rebuilt + `launchctl kickstart` — served JS confirmed via `curl` shows the new markers (`JIMINY-HITL-VELOCITY-001`, `bindKeyboardHandler`, `handleReviewKey`).
- `/v1/review/datasets?space_id=mdemg-dev` returns 18 datasets, 16 have pending items — `guidance` alone shows 200-capped candidates (real backlog ≥200; operator has been at ~0.5 items/day pre-sprint per JIMINY-CEILING-BREAK-2 analysis).
- Browser smoke deferred to the operator (Playwright/Chrome-in-headless not fired here); MVP-scope tolerance per sprint_plan §6.

## Two arch rules pinned (CLAUDE.md)

1. **UI corpus-growth features MUST unlock a keyboard-only flow.** The bottleneck for HITL grading is operator throughput, not classifier throughput. Mouse-driven per-item flows cap at ~10 items/session before fatigue. Keyboard bindings (single-key grade + auto-advance + single-key undo) are what let an operator grind through 40+ items per sitting. This applies to any future HITL surface (correction curation, contradicted-draft review, ULTS-golden grading, etc).

2. **Auto-advance MUST have a visible cancellation window with keyboard cancel.** Silent auto-advance breaks the ability to reverse a mis-graded item (the class the "Reverse last grade" button was designed to catch). A 400ms delay with the status line telling the operator "auto-advance in 400ms — press u to reverse, n to advance now, esc to cancel" preserves undo while keeping the velocity gain. Never ship silent-auto-advance-with-no-escape.

## Follow-ups disclosed

- **Autograde-preview endpoint + pre-fill** — deferred from MVP per sprint plan §3. If passive re-check data shows single-key grading still leaves operator throughput short of the retrain-corpus target, add `/v1/review/autograde-preview?dataset_id=X&item_id=Y` that returns the autograder's proposed per-dimension grades so the UI pre-fills radios to the autograder's proposal. Operator hits `space` to confirm-as-is OR `0..4` to override.
- **Per-dimension keyboard mapping** — e.g. `1234` = dim1, `qwer` = dim2. Deferred; premature. Only ship if multi-dim rubrics prove tedious under the unified single-value pattern.
- **Browser Playwright e2e test** — deferred; UI test coverage isn't part of this repo's existing surface. Add if a UI regression class emerges.

## Documents Accessed

- `docs/development/jiminy-ceiling-break-2/README.md` — Phase 4a spec
- `docs/development/jiminy-classifier-context-002/sprint_post.md` — Phase 3 precedent
- `docs/development/jiminy-hitl-velocity-001/sprint_plan.md` — this sprint's plan
- `internal/api/ui/tabs/review.js` — the file modified
- `internal/api/handlers_review.go` — server endpoints (unchanged; contract-reference)
- `internal/api/ui_embed.go` — confirmed `//go:embed ui/*` (required binary rebuild for the UI change to take effect)
- Live: `/v1/review/datasets?space_id=mdemg-dev` — pending backlog check
- Live: `curl http://localhost:9999/ui/tabs/review.js` — marker verification
- CLAUDE.md pins: HITL-REVIEW-001, HITL-CURATION-002, HITL-CURATION-003, HITL-ANALYTICS-TILE-001, JIMINY-CEILING-BREAK-2
