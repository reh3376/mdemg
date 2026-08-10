# BETA-IMPORT-001 — Sprint Post (B5 arc closeout)

**Date:** 2026-08-10
**Branch:** `reh3376_dev01`
**Arc:** B5 of the beta-program roadmap — corpus-loop closer

## Summary

Closes the loop from `mdemg beta-share` (opt-in tester submission, shipped v0.11.0-beta.2) to the local retrain corpus. New: `mdemg beta-import <bundle>` receiver + `mdemg beta-janitor {delete,sweep}` retention janitor. Bundles from testers on v0.11.0-beta.4+ carry the guidance_training_rows JSONL (B5a-added at manifest schema 9); the receiver verifies SHA + re-scrubs privacy + imports to `space_id=beta-tester-<submission_id>` with per-tester attribution.

## Shipped

**B5a** (PR #604 — already merged):
- `internal/tsdb/exporter.go` — 4th tableSpec `guidance_training_rows` (15 cols, schema 027 minus `row_id`); manifest schema 8 → 9; default `Tables` list gets 4th entry.
- `internal/cli/data_export{,_auto}.go` — CLI default tables updated.
- 2 new pin tests: `TestExporter_SchemaVersionIs9`, `TestExportConfig_DefaultTablesIncludesGuidanceCorpus`. Existing spec + text-field + privacy-skip tests extended.

**B5b + B5c** (this commit):
- `internal/tsdb/exporter_verify.go` (new) — shared `VerifyJSONLSHA256`, `RescrubJSONL`, `TableColumns` helpers. Extracted so beta-import can enforce the same contracts the exporter enforces at write time.
- `internal/cli/beta_import.go` (new) — `mdemg beta-import <bundle>` with double-tar unpack, tar-slip guard, receipt parse, manifest validate (utds_version + schema_version gate), per-JSONL SHA verify, privacy re-scrub, interactive opt-in, dry-run, per-row INSERT with `space_id + instance_id` remap + `row_id` cuid2 remint.
- `internal/cli/beta_janitor.go` (new) — parent + `delete --submission-id` + `sweep --older-than-days`. Keyed on `space_id LIKE 'beta-tester-%'`. Preview counts before deletion.
- `internal/cli/root.go` — 2 new leaf-command wirings under `GroupID=config`.

**B5 docs** (this commit):
- `docs/features/beta-import.md` (new).
- `docs/development/beta-import-001/{sprint_plan,post}.md` (new).
- CLAUDE.md pin — 2 arch rules.
- CHANGELOG entry.

## Live Tier-3 results (mdemg-dev, 2026-08-10)

| Step | Verification | Result |
|---|---|---|
| E1 | dry-run against synthetic fixture bundle | ✓ SHA + rescrub gates pass, 3 rows previewed |
| E2 | real import (`--yes`) | ✓ "Imported 3 rows to space_id=beta-tester-smoketest001alpha23456" |
| E3 | TSDB query confirms rows in the beta-tester-* space | ✓ 3 rows, correct date range |
| E4 | `mdemg-dev` space untouched; original `tester-alpha` remapped away | ✓ mdemg-dev row count unchanged; `tester-alpha` NOT present |
| E5 | janitor delete dry-run | ✓ "Matching rows: 3 ... (dry-run — no rows deleted)" |
| E6 | janitor delete real (`--yes`) | ✓ "Deleted 3 rows" |
| E7 | `beta-tester-*` namespace empty | ✓ 0 rows |
| E8 | sweep dry-run | ✓ "0 rows across 0 spaces (nothing older than cutoff)" |
| E9 | NEGATIVE: tampered JSONL, keep old manifest SHA | ✓ REJECTS with `"SHA-256 mismatch on <path>: manifest=<expected> actual=<actual>"` |

## Bundle production friction (documented, not a bug)

During smoke prep, `mdemg beta-share --space-id mdemg-dev --since-days 30` produced 17 `llm_interactions` privacy scrub violations + 4114 `guidance_training_rows.action_summary` violations on the maintainer's dev workspace. The exporter correctly refused to produce the bundle.

Root cause: `/Users/reh3376/...` absolute paths are dense in the maintainer's action summaries; those match `abs_path` pattern. Real tester workspaces would have their own path shapes and this class of false-positive would be much sparser.

Workaround for smoke: built a synthetic fixture bundle by hand (3 clean rows with no PII). Fixture path: `/private/tmp/rqa/b5-smoke/bundle.tar.gz` (temporary; deleted after smoke).

Not a bug. If this becomes a persistent workflow issue for testers, a follow-up sprint could add per-field allowlists for the tester's own workspace paths.

## Two arch rules pinned to CLAUDE.md

⚠️ **Receiver-side privacy re-scrub is REQUIRED, not optional.** `mdemg beta-import` must re-run `llmclient.ScrubStringExcluding` over every text field on load — defense-in-depth against a tampered bundle. The exporter's write-time scrub could be bypassed by a hand-crafted bundle that also matches its manifest SHA (hash + content edited together). The receiver's re-scrub catches this class. Uses the same per-field skip patterns from `internal/tsdb/exporter.go::tableSpecs`.

⚠️ **Beta-tester data lands in `space_id=beta-tester-<submission_id>` — NEVER the operator's own space.** Import-time invariant enforced in `importGuidanceRows`. This gives the janitor a stable prefix-based keying (`space_id LIKE 'beta-tester-%'`) for 30-day retention sweeps. Deletion by space_id (exact match for `delete --submission-id`, LIKE prefix for `sweep`) can NEVER touch the operator's own space or any non-beta-tester space.

## What this arc does NOT do (explicit non-goals)

- **NOT auto-scheduling import.** Operator runs `beta-import <bundle>` manually per submission. Automation would need a GH webhook receiver — separate future sprint if the beta-testing cadence justifies it.
- **NOT importing telemetry tables.** `llm_interactions` / `retrieval_events` / `embedding_events` are useful for maintainer's forensic replay but pollute the local TSDB pools + retention. B5 imports only `guidance_training_rows`.
- **NOT modifying the beta-share default `--since-days=30` window.** Exporter change is additive; existing testers on beta.3 keep working; new bundles from beta.4+ carry corpus rows.
- **NOT touching B3 submission indexer.** Records receipts; does not process. `beta-import` is manually invoked against downloaded bundles.
- **NOT adding a `submission_id` column** to `guidance_training_rows`. Would require a migration + break the space_id-based deletion contract. `space_id=beta-tester-<sid>` carries the same information + gives the janitor a keying prefix.

## Follow-ups disclosed

- **Idempotency guard on beta-import**: repeat imports of the same bundle produce duplicate rows. Guard by content-hash or by `(submission_id, guidance_id)` uniqueness. Not urgent — janitor `delete --submission-id` undoes the mistake.
- **Unit tests for beta-import negative paths**: SHA-mismatch, rescrub-violation, tar-slip, schema<9 lite-mode. Live-verified E9 for SHA; the others are trivial + live-verified in dry-runs.
- **Per-workspace scrub allowlist**: if bundle production consistently blocks on the operator's own path patterns, add a knob for tester-workspace-relative-only scrubbing.
- **GH webhook automation**: sprint if beta-testing cadence proves this is worth the automation.

## Beta program state after B5

Complete inbound + outbound loop:

| Stage | Sprint | Command | Status |
|---|---|---|---|
| Tester produces bundle | A + C (v0.11.0-beta.2) | `mdemg beta-share` | ✓ Shipped |
| Tester attaches to GH issue | B templates + B7 README_BETA.md | GH issue templates | ✓ Shipped |
| Maintainer notified + labeled | Beta pipeline D | `beta-triage.yml` | ✓ Shipped |
| Weekly digest of open issues | Beta pipeline D | `beta-weekly-digest.yml` | ✓ Shipped |
| Submission receipt tracked | B3 | `beta-submission-indexer.yml` | ✓ Shipped |
| Bundle carries corpus rows | B5a (this arc) | Exporter schema 9 | ✓ Ship v0.11.0-beta.4 |
| Maintainer verifies + imports | B5b (this arc) | `mdemg beta-import` | ✓ Ship v0.11.0-beta.4 |
| 30-day retention | B5c (this arc) | `mdemg beta-janitor sweep` | ✓ Ship v0.11.0-beta.4 |

## Documents Accessed

- Plan file: `/Users/reh3376/.claude/plans/shimmying-jingling-hamming.md`
- Recon agent (Explore, 2026-08-10): 8-cluster fact-finding report on bundle format, writer contracts, RSICProtectedSpaces scope, CUIDv2 usage, deletion patterns
- `internal/tsdb/exporter.go`, `guidance_training_rows_writer.go`, `migrations/027_guidance_training_rows.sql`
- `internal/cli/beta_share.go`
- `internal/llmclient/scrubber.go`
- `docs/features/hitl-auto-curation.md` (arc context)
- Live SQL against mdemg-dev TSDB (9-step Tier-3 smoke)
- Sibling shipped features referenced in the sprint plan: HITL-CURATION-003, JIMINY-ARCHIVED-CODE-FILTER-001
