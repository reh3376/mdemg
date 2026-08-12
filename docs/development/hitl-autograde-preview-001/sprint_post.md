# HITL-AUTOGRADE-PREVIEW-001 — Sprint Post

**Date:** 2026-08-12
**Branch:** `reh3376_dev01`
**Deferred from:** JIMINY-HITL-VELOCITY-001 MVP scope

## Problem

JIMINY-HITL-VELOCITY-001 shipped keyboard-driven bulk review + auto-advance. But operator still had to READ each item + judge each rubric dimension from scratch. For items the autograder gets right (majority per HITL-CURATION-002/003 evidence), the operator's judgment matches the autograder's — the operator is doing work the autograder already did.

Design: pre-fill radios on item load from an autograder proposal (LLM call, ~1-3s). Operator hits `space` to accept-as-is OR `0-4` to override per-dimension.

## Arc-safety

**100% safe for JIMINY-CEILING-BREAK-2 measurement window** (through 2026-08-19):
- Endpoint is READ-ONLY on the substrate. Does NOT write to `review_grades` or `constraint_outcomes`.
- Adds rows to `llm_interactions` (one per preview fetch) but that table doesn't feed the follow-rate signal.
- Zero touch on constraint/correction corpus, Jiminy classifier, retrieval Lever C, or writer gates.

## Shipped

**Server** (`internal/api/handlers_review_autograde.go` — new):
- `POST /v1/review/autograde-preview` with body `{dataset_id, item_id, space_id?}`
- Response: `{data: {dimensions: {<key>: 0..4}, confidence, rationale, available, skipped_reason?}}`
- Lazily-built singleton autograder on the `Server` struct (mirrors CLI `buildAutograder` shape) — first request builds, subsequent reuse. Reads `LLM_ENDPOINT` + `LLM_MODEL` from server config; falls back to local llama-server on `:8102/v1`.
- MinConfidence=0 (preview always returns whatever the autograder proposes; operator confirms per-item — the ≥0.80 CLI auto-write gate isn't applicable here since there's no write).
- Dataset-specific `AutogradePromptHinter` honored via type assertion (mirrors CLI path).
- Non-fatal on LLM error: returns `Available:true, SkippedReason:"autograde error: ..."` so UI shows blank radios + warning instead of crashing the whole preview.
- `Available:false` when no autograder built (no LLM endpoint) → UI falls back to blank radios (safe default).

**Server wiring** (`internal/api/server.go`):
- 2 new fields on `Server` struct: `reviewAutograder *review.Autograder` + `reviewAutograderMu sync.Mutex`
- Route registered at `/v1/review/autograde-preview` under `ScopeAdminSpaces` (same auth scope as other `/v1/review/*` endpoints)

**UI** (`internal/api/ui/tabs/review.js` + `api.js`):
- New `api.reviewAutogradePreview(datasetId, itemId, spaceId)` helper
- On `renderItem`: shows `Autograde preview: fetching…` status → async POST → on response, pre-fills radios matching the returned dimensions, updates `currentDimensionInputs` module state (so keyboard-`space` submit sees the autograded values), updates status to `pre-filled N/M dims (conf X.XX)`
- Guard: item might have changed while LLM was thinking (operator hit `n` before response) — check `currentItem === it` before touching the DOM
- Non-fatal on error: status shows `autograde error: <msg>` in orange; operator can still grade normally
- Rationale exposed as `title` attribute on the status element (hover-tooltip; keeps main UI dense)

**Pin tests** (`internal/api/handlers_review_autograde_test.go`):
- `TestAutogradePreview_WrongMethodReturns405` — GET/PUT/DELETE/PATCH all rejected
- `TestAutogradePreview_ResponseShape` — JSON key + value-type contract (dimensions as JSON NUMBERS not strings; available as JSON BOOL) — if UI breaks silently on shape drift, this catches it
- `TestAutogradePreview_ResponseShape_WhenUnavailable` — omitempty for rationale + skipped_reason
- `TestAutogradePreview_BadJSONDoesNotCrash` — malformed body → 4xx/5xx, no panic

## Live Tier-3 (mdemg-dev, 2026-08-12)

- `go build ./...` clean; `golangci-lint run ./internal/api/...` = 0 issues
- 4 new pin tests green
- Binary rebuilt + `launchctl kickstart`
- Served JS confirmed to contain `HITL-AUTOGRADE-PREVIEW-001` marker (2 instances)
- **Endpoint smoke**: `POST /v1/review/autograde-preview` on real guidance item `bcucnwq1eykcuhdn6w639648` returned in ~10s:
  ```json
  {
    "data": {
      "dimensions": {"relevance": 3, "actionability": 2, "outcome_label_correctness": 0},
      "confidence": 0.80,
      "rationale": "The guidance is about a conflict between context using class and codebase function patterns, which is directly relevant to the task. However, the agent's action does not clearly address the conflict, so the outcome label is likely incorrect.",
      "available": true
    }
  }
  ```
- Autograder's proposal vs my earlier manual grade for the same item (`relevance=0, actionability=0, outcome_label_correctness=1`): partial agreement on `outcome_label_correctness` direction (both say "wrong"); disagreement on `relevance` (autograder says 3="on-topic", I said 0="off-topic"). That disagreement resolution is exactly what the operator-confirms-or-overrides flow was designed for.
- No writes to `review_grades` or `constraint_outcomes` from the preview call (confirmed via post-call SQL check — preview didn't create any grade rows).

## Two arch rules pinned (CLAUDE.md)

1. **Preview endpoints for expensive-to-compute artifacts (LLM proposals, ML predictions, etc) MUST return `Available:bool` alongside the result** so the UI can fall back gracefully when the compute path is not wired. Silent 500s force the operator into fallback-mode after a wait; explicit `Available:false` renders "unavailable" immediately + preserves the manual flow.

2. **UI async fetches that pre-fill form fields MUST guard on the item ID they're pre-filling for.** In HITL bulk-review, the operator advances items with `n` faster than the ~2s LLM preview response. Without the guard, an in-flight preview from item A can populate radios AFTER item B has loaded, silently corrupting the operator's next grade. The `currentItem === it` closure check is the load-bearing invariant — any future async form-pre-fill MUST replicate it.

## Follow-ups disclosed

- **Autograder singleton reset on config change** — currently the singleton is built once and reused. If the operator changes `LLM_ENDPOINT` at runtime, they'd need a restart. Deferred; no operational need today.
- **Preview LLM cost tracking** — one preview per item load; operator grading 50 items = 50 LLM calls. Cheap on local llama-server, non-negligible if pointed at OpenAI. Add a `mdemg_review_autograde_preview_total` counter if operator adopts this pattern heavily.
- **Confidence-gated pre-fill** — currently pre-fills at any confidence. If the autograder returns confidence < 0.5, the operator might prefer BLANK radios (forces conscious judgment on uncertain items). Ship if UX complaint arises.

## Documents Accessed

- `docs/development/jiminy-hitl-velocity-001/sprint_post.md` — deferred follow-up context
- `internal/cli/review.go::buildAutograder` + `llmGraderAdapter` — CLI-side pattern to mirror server-side
- `internal/review/autograder.go` (`Grade`, `GradeWithHint`, `AutograderConfig`, `LLMGrader`)
- `internal/api/handlers_review.go` (existing `/v1/review/*` endpoints for pattern-matching)
- `internal/api/server.go` (route registration site + Server struct field pattern)
- `internal/api/ui/{api.js, tabs/review.js}` (UI wiring)
- Live: `POST /v1/review/autograde-preview` smoke against real guidance item
- CLAUDE.md pins: JIMINY-HITL-VELOCITY-001, HITL-CURATION-002, HITL-CURATION-003
