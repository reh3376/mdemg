# REVIEW-GRADE-VALIDATE-ITEM-ID-001 — Sprint Post

**Date:** 2026-08-13
**Branch:** `reh3376_dev01`
**Trigger:** Fable HITL bulk-grade session #110 (pass 2) discovered that `POST /v1/review/grade` accepted arbitrary `item_id` values and recorded the grade, creating orphan rows in `review_grades` that can never be joined back to any real item. Fable caught its own mistake and reversed the orphan via `/v1/review/reverse`, but the class defect stayed open.

## Problem

`handleReviewGrade` at `handlers_review.go:265` called `d.FetchItem(...)` and captured `item` but discarded the `found` return with `_`. Result: any arbitrary `item_id` passed the (implicit non-)validation and landed as a corpus row. Downstream effects: retrain pipeline reads rows it can't join back; grade counts falsely inflated; latest-grade lookups may miss real grades if operators hand-type an id.

## Shipped

**`internal/api/handlers_review.go::handleReviewGrade`**:
- Captured all 3 FetchItem return values (`item, found, fetchErr`) instead of discarding.
- On `fetchErr != nil`: `writeInternalError(w, fetchErr, "review fetch item for validation")` — the read may fail for infra reasons; a 500 with named context is safer than silently proceeding to a write.
- On `found == false`: `writeJSON(w, StatusNotFound, {error, dataset_id, item_id})` — explicit "unknown item_id in dataset X" message + the offending ids echoed back for one-shot debug. No row written.
- Only when `found == true` proceeds to construct the grade + write.

The comment cites this sprint + names the class of defect for future readers.

**Pin tests** (`internal/api/handlers_review_validate_item_test.go` — new):
- `TestReview_Grade_RejectsNonexistentItemID` — asserts empty `item_id` returns 400 (the shipped required-field validator catches this first).
- `TestReview_Grade_UnknownItemIDReturns404` — uses a new `alwaysNotFoundDataset` fixture (implements `ReviewableDataset` with FetchItem always returning `found=false`) via a helper `reviewTestServerWithDataset(t, ds)`. Submits a grade with a non-empty but non-existent item_id; asserts 404 + body contains "unknown item_id in dataset", asserts no row written (CopyFrom rows = 0).

## Live Tier-3 (mdemg-dev, 2026-08-13)

- Rebuilt binary + `launchctl kickstart -k gui/501/com.mdemg.server` → server up at 11:38 EDT, `/healthz` all subsystems ok
- **Orphan-write attempt live**:
  - `POST /v1/review/grade` with `{"dataset_id":"llm:hidden.summarize","item_id":"NONEXISTENT-ITEM-ID-TEST",...}` → response body: `{"dataset_id":"llm:hidden.summarize","error":"unknown item_id in dataset llm:hidden.summarize","item_id":"NONEXISTENT-ITEM-ID-TEST"}` (HTTP 404 implicit via writeJSON status)
  - Immediate TSDB query: `SELECT COUNT(*) FROM review_grades WHERE grader_id='orphan-test'` → **0** (confirmed no row landed)
- Full test suite green: `go test ./internal/api/` PASS (12.9s)
- Lint clean: `golangci-lint run ./internal/api/` — 0 issues

## One arch rule pinned (CLAUDE.md)

**Review-grade handlers MUST validate item_id exists in the target dataset BEFORE writing.** `d.FetchItem(ctx, space, item_id)` returns `(item, found, err)` — capture all three, 404 on `!found`, 5xx on `err != nil`. The pre-fix pattern of discarding `found` with `_` was a stealth-orphan-write class: any arbitrary or hand-typed item_id landed as a corpus row that would never join back. The extra FetchItem round-trip is worth the write-safety invariant. When adding a NEW `POST /v1/review/*` endpoint that writes to `review_grades` (or any corpus-append table with an FK-like relationship), replicate this validate-first pattern.

## Follow-ups disclosed

- **Batch-validate endpoint if performance-sensitive** — the per-item FetchItem round-trip costs ~1-2ms in the local case (fake) or 20-50ms via TSDB. For high-volume auto-graders that submit N grades at once, a `POST /v1/review/grades/batch` endpoint could pre-validate all N item_ids in a single JOIN then bulk-write. Not needed today; auto-graders submit sequentially and the per-item cost is dwarfed by the LLM reasoning cost.
- **Extend the same pattern to `/v1/review/reverse`** — reverse takes a `grade_id`, not an `item_id`, but the same class of stealth-write exists if a bogus grade_id gets silently persisted as a reversal row. Audit + pin in a follow-up.

## Documents Accessed

- `internal/api/handlers_review.go::handleReviewGrade` — the edit site
- `internal/api/handlers_review_test.go` — fakeReviewPool + fakeReviewDataset patterns for the pin test helper
- `internal/review/dataset.go::ReviewableDataset` interface — for the alwaysNotFoundDataset shim
- `internal/review/rubric.go::RubricDimension` — `Anchors` is `[5]string`, not `[]string` (caught during test build)
- Task #110 (HITL-BULK-GRADE-SESSION-002) sprint post — the Fable-run report that surfaced the defect
- Live: orphan-write POST against real llm:hidden.summarize dataset + TSDB row-count verify
- CLAUDE.md pins: REVIEW-GRADE-NOTES-FIELD-001 (same-sprint-adjacent), REVIEW-CANDIDATES-DEDUP-409-001 + REVIEW-CANDIDATES-EXCLUDE-ERROR-ROWS-001 (sibling filter sprints)
