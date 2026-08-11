# EXPORT-SCRUB-INTAKE-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint:** EXPORT-SCRUB-INTAKE-001
- **Date:** 2026-08-11
- **Branch:** `reh3376_dev01`
- **Effort:** ~1.5 hours
- **Provenance:** BETA-IMPORT-001 disclosed follow-up ("per-workspace scrub allowlist if bundle production consistently blocks"). Triggered live by the chronic HIGH `scheduled-job export-auto` alert firing daily on mdemg-dev (511 privacy scrub violations at last fire).

## 2. Problem Statement

`internal/tsdb/guidance_training_rows_writer.go::Record` writes rows raw (no intake-side scrubbing) — unlike `llm_interactions` where `internal/llmclient/scrubber.go::Scrub()` runs at intake and cleans SystemPrompt/UserPrompt/Response/ThinkContent before the row lands. When the exporter runs its integrity scan at export time, it finds unscrubbed abs_paths in `guidance_training_rows.action_summary` (captured from bash-tool observations that legitimately contain `/Users/reh3376/...`), counts them as violations, and BLOCKS the export.

Result: `export-auto` fails daily on mdemg-dev, `scheduled-job` HIGH alert re-fires, operator has no clean bundle to test the beta pipeline with.

## 3. Scope & Constraints

**In-scope:**
- Add intake-side scrubbing to `GuidanceTrainingRowsWriter.Record` — mirroring `llmclient.Scrub()`'s pattern for `llm_interactions`.
- Scrub `ActionSummary` with the full pattern set (no skips).
- Scrub `GuidanceContent` with `abs_path` skipped (matches the existing export spec's `guidanceTrainingRowsSpec.textFields[6] = {"abs_path"}` — operator-authored rules legitimately reference paths).
- Add `mdemg data scrub-guidance-rows --space-id X [--dry-run] [--since DUR]` one-off backfill CLI so operator can clean historical rows (the exporter looks back 30d by default → without backfill, the fix takes 30 days to become visible).
- Pin test asserting the writer scrubs at intake.
- Live smoke: run backfill on mdemg-dev, run export-auto, verify clean.

**Out-of-scope:**
- Changing the export-scan gate semantics (block-on-scrub-diff stays as the strong contract).
- Adding a per-workspace allowlist (unnecessary now — proper intake scrubbing solves the class without needing operator-specific config).
- Retrofitting other buffered writers (llm_interactions already scrubs; retrieval_events + embedding_events don't carry the same PII risk shape).

## 4. Dependencies

- BETA-IMPORT-001 — the sprint that first surfaced this class as friction.
- SCRUB-ENV-REF-001 — the previous export-auto scrub fix (env-var reference misclassification). Same failure MODE (export blocked on scrub-scan diff), different root cause; this sprint closes the last remaining chronic class.
- JIMINY-RELEVANCE-001 Epic 1 — the sprint that introduced the guidance_training_rows writer without intake scrubbing.

## 5. Implementation Plan (sequential)

**E1 — intake scrub in writer.**
- `internal/tsdb/guidance_training_rows_writer.go::Record` — call `llmclient.ScrubStringExcluding(row.ActionSummary, nil)` and `llmclient.ScrubStringExcluding(row.GuidanceContent, []string{"abs_path"})` before buffering. Doc comment names the sprint + why.
- Skip lists match `guidanceTrainingRowsSpec.textFields` exactly so intake + export contracts stay in sync. Extract the skip lists to constants shared by both files? Deferred — the mismatch surface is 2 lines; a shared const adds package boundary considerations.

**E2 — one-off backfill CLI.**
- `internal/cli/data_scrub_guidance.go` (new): `mdemg data scrub-guidance-rows --space-id X [--dry-run] [--since 30d]` — UPDATE-in-place SQL rewrite of matching rows. Dry-run default. Prints matched-row count + preview before applying. Backs up nothing (rows are training telemetry; scrubbing is what the operator wanted).

**E3 — pin test.**
- `internal/tsdb/guidance_training_rows_writer_test.go` (extend or new pin file): assert Record scrubs `/Users/whoever/foo/bar.go` → `/[PATH]/foo/bar.go` in ActionSummary, and does NOT scrub the same in GuidanceContent (skip-list respected).

**E4 — live Tier-3 smoke.**
- Rebuild + reinstall binary.
- Run `mdemg data scrub-guidance-rows --space-id mdemg-dev --since 30d` — dry-run then real.
- Run `mdemg data export --space-id mdemg-dev --since <1d>` — must succeed with 0 violations.
- Restart mdemg + observe next post-restart guidance feedback row has scrubbed action_summary.

**E5 — docs.**
- Sprint dir: sprint_plan + post.md.
- CLAUDE.md pin.
- CHANGELOG entry.

## 6. Testing Plan

**Unit (T1):**
- New pin: `TestGuidanceTrainingRowsWriter_Record_ScrubsAbsPathInActionSummary` — writes a row with `/Users/tester/foo/bar` in ActionSummary, flushes, reads back, asserts scrubbed shape.
- New pin: `TestGuidanceTrainingRowsWriter_Record_PreservesAbsPathInGuidanceContent` — same but on GuidanceContent, asserts abs_path stays raw (matches export spec's abs_path skip).

**Integration (T2):**
- Existing writer suite continues green.

**Live (T3):**
- E4 above: backfill CLI on mdemg-dev, then `mdemg data export` succeeds cleanly.

## 7. Commit Strategy

Single commit: `fix(tsdb): intake-scrub guidance_training_rows action_summary (parity with llm_interactions) (EXPORT-SCRUB-INTAKE-001)`.

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./internal/tsdb/... ./internal/cli/...` = 0 issues
- [ ] `go test ./internal/tsdb/...` green including new pins
- [ ] Backfill CLI runs successfully on mdemg-dev
- [ ] `mdemg data export --space-id mdemg-dev --since 1d` = 0 violations
- [ ] Next natural export-auto run does not re-fire the HIGH alert
- [ ] CHANGELOG entry
- [ ] CLAUDE.md pin

## 9. Risks & Mitigations

**R1: Scrubbing at intake changes what's stored — affects RSIC / dashboard consumers.**
- The `abs_path` scrub replaces `/Users/reh3376/mdemg/foo/bar.go` with `/[PATH]/foo/bar.go`. Downstream consumers (RSIC introspection, HITL curation, HTML dashboards) read this same content. Substitution loses username + workspace-parent-dir, keeps final 2 path segments — enough for context, safe by design.
- The `llm_interactions` writer has been doing this since day one; matches an already-shipped pattern.

**R2: Backfill CLI on production rows without operator consent.**
- Dry-run default. Only applies on `--yes` or interactive prompt confirm. Rows are training-corpus telemetry — the operator is opting to scrub the ones they're producing on their own workstation.
- Not reversible (scrub is one-way). Operator can `rm -rf` the guidance_training_rows hypertable if they want a fresh start; this is the standard mdemg-dev reset path.

**R3: Skip lists in intake writer diverge from export spec.**
- Sprint plan defers a shared-const extraction. If future drift becomes a concern, extract `GuidanceContentSkipPatterns` + `ActionSummarySkipPatterns` to a shared const in `exporter.go` and reference from both files.

## 10. Rollback Procedures

Revert the writer change (Record() will go back to writing raw). Backfill CLI revert doesn't undo already-scrubbed rows. No schema/migration.

## 11. Documents Accessed

- `docs/development/beta-import-001/post.md` — disclosed this follow-up
- `internal/tsdb/guidance_training_rows_writer.go` (the fix site)
- `internal/tsdb/exporter.go` — export scan gate + skip-list spec
- `internal/llmclient/scrubber.go` — ScrubString + patterns
- `docs/development/scrub-env-ref-001/` — sibling class fix precedent
- Live: `mdemg data export` dry-run + scan output on mdemg-dev
- Live: `scheduled_job_events` table for export-auto failure history
- CLAUDE.md pins: SCRUB-ENV-REF-001, BETA-IMPORT-001, JIMINY-RELEVANCE-001
