# EXPORT-SCRUB-INTAKE-001 — Sprint Post

**Date:** 2026-08-11
**Branch:** `reh3376_dev01`

## Summary

Closes the chronic HIGH `scheduled-job export-auto` alert (BETA-IMPORT-001 disclosed follow-up). Root cause: `guidance_training_rows` and `retrieval_events` writers wrote content raw at intake (no scrub) — the exporter's block-on-scrub-diff gate then flagged the raw workspace paths + literal env-secrets in every daily export attempt.

## What was broken

- `GuidanceTrainingRowsWriter.Record` (`internal/tsdb/guidance_training_rows_writer.go`) buffered `ActionSummary` + `GuidanceContent` raw, no scrub.
- `RetrievalEventWriter.Record` (`internal/tsdb/retrieval_writer.go`) buffered `QueryText` raw. A prior test `TestRetrievalWriter_NoPrivacyScrub` explicitly pinned this behavior — that pin was wrong and got updated.
- The exporter (`internal/tsdb/exporter.go`) SCANS these fields with `llmclient.ScrubStringExcluding` at export time and BLOCKS when the scrubbed output differs from the raw stored value. On mdemg-dev (the maintainer's own workstation) this created a chronic false-positive class: 511 violations at the last fire, blocking every 24h export-auto run and dispatching a HIGH alert.

## What's fixed

**Intake scrub (forward-only):**
- `GuidanceTrainingRowsWriter.Record` — now scrubs `ActionSummary` with the full pattern set + `GuidanceContent` with `abs_path` skipped. Skip list matches `guidanceTrainingRowsSpec.textFields` in the exporter EXACTLY (must stay in sync — see comment).
- `RetrievalEventWriter.Record` — now scrubs `QueryText` with `abs_path` skipped. Skip list matches `retrievalEventsSpec.textFields` in the exporter.

**Backfill CLI (historical rows):**
- `mdemg data scrub-export-tables --space-id X [--since 720h] [--dry-run|--yes] [--limit N]` — UPDATE-in-place scrubs matching historical rows in both `guidance_training_rows` (action_summary + guidance_content) and `retrieval_events` (query_text). Dry-run default. Bounded by space_id + optional --since window (default 30d matches exporter default lookback). Prints matched-row count + a 3-row sample before applying.

**Test contract update:**
- `TestRetrievalWriter_NoPrivacyScrub` REPLACED by `TestRetrievalWriter_IntakeScrub` — asserts api_key redacted, abs_path preserved. Old pin was wrong (predated the "shipped bundle must be clean" requirement).
- 3 new pins in `guidance_training_rows_writer_test.go`:
  - `TestGuidanceTrainingWriter_Record_ScrubsAbsPathInActionSummary` — full scrub on action_summary
  - `TestGuidanceTrainingWriter_Record_PreservesAbsPathInGuidanceContent` — abs_path skip respected on guidance_content
  - `TestGuidanceTrainingWriter_Record_StillScrubsApiKeyInGuidanceContent` — api_key still redacted (skip list is targeted, not blanket-off)

## Live Tier-3 (mdemg-dev, 2026-08-11)

| Step | Result |
|---|---|
| Baseline: `mdemg data export --since 1d` | ✗ EXPORT BLOCKED: 30 violations (all `action_summary`) |
| Backfill dry-run: `mdemg data scrub-export-tables --space-id mdemg-dev --since 720h` | ✓ 6489 dirty rows previewed, no writes |
| Backfill apply: `... --yes` | ✓ 6489/6489 scrubbed in guidance_training_rows |
| Export retry | ✗ 2 violations remain (in `retrieval_events.query_text` — `PGPASSWORD=mdemg_metrics` env_secret) |
| **Extended sprint scope to retrieval_events** — added intake scrub + backfill loop | |
| Backfill both tables: `mdemg data scrub-export-tables --space-id mdemg-dev --since 720h --yes` | ✓ 8/8 scrubbed in retrieval_events; 2/2 in guidance rows this round |
| Final export | **✓ 11660 rows exported, 0 violations** |
| Restart mdemg (fresh binary) | ✓ Healthy, all new intake writes will be clean-by-construction |

## Two arch rules pinned (CLAUDE.md)

1. **Every text-field writer that feeds the export scan-gate MUST scrub at intake.** Skip lists in the intake scrub MUST match `exporter.go::tableSpecs.textFields` EXACTLY for the same table+column. The intake scrub is the single mechanism that keeps stored content clean-by-construction; the exporter's block-on-diff gate is the correctness pin. If a new text field lands in a hypertable, the writer MUST scrub before buffer, or the exporter WILL chronically block on it once natural content flows through.

2. **When the exporter's scan gate blocks on a class, the intake path is what to fix — not the gate.** Weakening the gate (e.g. skipping more patterns at export time OR toleration allowlists) exchanges bundle safety for operator convenience. The right shape: scrub at intake so the row is clean when stored, keep the export gate strong as a correctness contract, run a backfill CLI once to clean pre-fix historical rows.

## Follow-ups disclosed

- **Cross-writer parity audit**: `llm_interactions` uses `llmclient.Scrub()` at intake since day one; guidance + retrieval now match. If new hypertable writers land (event graph, HITL, ape.reflect), pin them into this same intake-scrub contract at creation.
- **Extract shared skip-list constants**: the skip-list literals in `Record()` (writers) + `textFields` (exporter) are duplicated. If they drift, the intake stores content the export can't accept OR vice versa. Follow-up sprint: extract to package-level consts in `exporter.go` and reference from writers. Deferred — 2 writers × 2 columns is a small surface today.
- **Cron the scrub-export-tables CLI**: currently one-off. If new writers ever leak, the operator has to remember to run it. Consider wiring into `maintenance` (weekly) so pre-existing leaks self-remedy without ceremony.

## Documents Accessed

- `docs/development/beta-import-001/post.md` — disclosed this follow-up
- `docs/development/scrub-env-ref-001/` — sibling class precedent (env-var reference misclassification)
- `internal/tsdb/{guidance_training_rows_writer,retrieval_writer,exporter}.go`
- `internal/llmclient/scrubber.go`
- `internal/cli/data.go` (parent command wiring)
- Live: `mdemg data export` dry-run + scan output, `scheduled_job_events` history
- CLAUDE.md pins: BETA-IMPORT-001, SCRUB-ENV-REF-001, JIMINY-RELEVANCE-001
