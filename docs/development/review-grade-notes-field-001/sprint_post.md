# REVIEW-GRADE-NOTES-FIELD-001 — Sprint Post

**Date:** 2026-08-13
**Branch:** `reh3376_dev01`
**Trigger:** Fable HITL bulk-grade session #106 hit two coupled defects on `POST /v1/review/grade`: (1) client sending `{"notes":"..."}` got an opaque `{"error":"invalid request body"}` 400 with no hint at what was wrong; (2) even after debug via server logs, there was no `notes` column to persist grader reasoning into. Blocked auto-graders from recording "why this score" alongside dimension values.

## Problem

Two coupled DX defects across the HITL surface:

1. **`reviewGradeRequest` had no `notes` field**. It had `SuggestedGuidance` (SME corrective example — "what would have been better guidance") but grader-reasoning-per-grade had no home. Auto-graders (Fable, future retrain workers, etc.) want to record their 1-line "why this score" alongside dimension values for provenance + downstream retrain analysts to see the WHY alongside the WHAT.

2. **`readJSON` returned an opaque error on `DisallowUnknownFields` violations**. `writeJSON(w, StatusBadRequest, map[string]any{"error": "invalid request body"})` — swallowed the actual decode error message. Real error text was only in server logs. Two-round-trip debug for what should be one-shot: submit → 400 → tail logs → discover offending field → retry. **Affects EVERY handler using `readJSON`** (not just review/grade); this is the higher-leverage fix.

## Shipped

**`internal/llmclient/scrubber.go`** — no changes (SCRUB-IDEMPOTENT-001 sprint's territory; unrelated).

**`internal/api/server.go::readJSON`**:
- On decode error, return `{"error": err.Error()}` instead of `{"error": "invalid request body"}`. Decode errors from `encoding/json` carry only structural information (field name, line/col, expected type) — no request body content leaks, so exposing the message is safe. Comment cites this sprint for future readers.

**`internal/tsdb/migrations/034_review_grades_notes.sql`** (new):
- `ALTER TABLE review_grades ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT ''`
- `UPDATE tsdb_schema_meta SET value = '34'`
- Mirrors 029's `suggested_guidance` addition shape (forward-only ALTER, rollback documented in header).

**`internal/config/config.go`**:
- Bumped `TSDBRequiredSchemaVersion` default 33 → 34, comment updated to name this sprint.

**`internal/tsdb/review_grades_writer.go`**:
- Added `Notes string` field to `ReviewGradeRow` (comment cites this sprint + NOT NULL DEFAULT '' contract).
- Extended `Flush` row values + CopyFrom column list to include `notes` (17 columns total, up from 16).

**`internal/api/handlers_review.go`**:
- Added `Notes string \`json:"notes,omitempty"\`` to `reviewGradeRequest`.
- Extended `reviewGradeRow(...)` signature to accept `notes` param + populate the field.
- Both call sites updated (`handleReviewGrade` passes `req.Notes`; `handleReviewReverse` passes `""` — reversal rows don't carry grader notes).

**Pin tests** (`internal/api/handlers_review_notes_test.go` — new):
- `TestReview_Grade_NotesRoundTrip` — capturing pool asserts `notes` column present + value round-trips from request body → CopyFrom row.
- `TestReadJSON_UnknownFieldSurfacesName` — submit `{"totally_unknown_key":42}`, assert 400 + response body contains `totally_unknown_key`, assert body is NOT the pre-fix opaque form. This test uses the review-grade endpoint but the assertion pattern applies to ALL `readJSON` callers.

**Pre-existing pin update** (`internal/tsdb/review_grades_writer_test.go:96`):
- `TestReviewGradesWriter_RecordThenFlush` column-count assertion bumped 16 → 17 with comment naming this sprint.

## Live Tier-3 (mdemg-dev, 2026-08-13)

- V0034 migration applied (`schema_version: 33 → 34`)
- Binary rebuilt + `launchctl kickstart -k gui/501/com.mdemg.server` → server up at 11:24 EDT, `/healthz` all subsystems ok
- **Notes round-trip live**: submitted grade on `llm:hidden.summarize / z19ilbhcgpoucz6ybteoikva` with `"notes":"REVIEW-GRADE-NOTES-FIELD-001 live-smoke verification"` → response `{grade_id, gold_score:0.75, grade_recorded:true, reinforcement_applied:false}` → TSDB query confirms `notes` column populated with the exact string. Cleaned up smoke row post-verify.
- **Unknown-field error surface live**: `POST /v1/review/grade -d '{"totally_unknown_key":42, ...}'` → `{"error":"json: unknown field \"totally_unknown_key\""}` (was opaque `{"error":"invalid request body"}`). Working across ALL readJSON-using handlers.
- Full test suite green: `go test ./internal/api/ ./internal/tsdb/ ./internal/config/ ./internal/llmclient/` PASS
- Lint clean: `golangci-lint run ./internal/api/ ./internal/tsdb/ ./internal/config/` — 0 issues

## Two arch rules pinned (CLAUDE.md)

1. **`readJSON` decode errors MUST surface the offending field name in the response body** — the pre-fix opaque `"invalid request body"` forced every debug into a server-log tail (client-visible error was uninformative; real hint was only in slog output). `encoding/json` decode errors carry only structural information (field name, line/col, expected type), NOT request body content, so exposing `err.Error()` is safe. The `DisallowUnknownFields` contract is a valid safety measure; the opaque error message wasn't. Applies to every future handler using `readJSON`.

2. **Additive schema columns for grader-authored metadata follow the migration-029 pattern**: `ALTER TABLE ... ADD COLUMN IF NOT EXISTS <name> TEXT NOT NULL DEFAULT ''` in a new migration file, bump `tsdb_schema_meta.value`, bump `TSDBRequiredSchemaVersion` default in `config.go`, extend the writer struct + CopyFrom cols, extend the request struct + thread through the row-builder helper. Column-count pin test in the writer package MUST be updated in the same commit (else CI FAILs cleanly — which is how we caught the bump this sprint). Free-text grader/SME metadata columns are additive-only; NEVER migrate them into structured columns without a data-migration plan.

## Follow-ups disclosed

- **Extend `notes` to the `/v1/review/reverse` request path** — currently reversal rows carry `notes=""`. A "why I reversed this" note would help downstream corpus audit. Not shipped — reversal is operator-driven and rare.
- **UI `/ui/review` doesn't yet expose a notes textbox** — grader-agents can submit notes via API but human operators grading via the tab have no field for it. Small follow-up to `internal/api/ui/tabs/review.js`.
- **Consider a `notes` column pipeline consumer** — the Phase 4b retrain currently reads dimensions but not grader reasoning. When retrain harness reads corpus rows, `notes` could feed a rationale channel. Design call for the retrain sprint author.

## Documents Accessed

- `internal/api/handlers_review.go` — `reviewGradeRequest`, `handleReviewGrade`, `handleReviewReverse`, `reviewGradeRow` (edit sites)
- `internal/api/server.go::readJSON` — the cross-handler improvement site
- `internal/tsdb/migrations/029_review_grades_suggested.sql` — the ALTER-precedent this sprint mirrors
- `internal/tsdb/review_grades_writer.go` — `ReviewGradeRow` + `Flush` (edit sites)
- `internal/tsdb/review_grades_writer_test.go` — column-count pin (updated)
- `internal/config/config.go` — `TSDBRequiredSchemaVersion` default + field comment
- Task #106 (HITL-BULK-GRADE-SESSION-001) sprint post — the Fable-run report that surfaced both defects
- CLAUDE.md pins: EXPORT-SCRUB-INTAKE-001 (adjacent scrub-writer pattern), HITL-CURATION-002 (auto-grader invariant), HITL-AUTOGRADE-PREVIEW-001 (sibling review-endpoint sprint)
- Live: `POST /v1/review/grade` smoke + TSDB row verification, `launchctl kickstart` + `/healthz` post-restart probe
