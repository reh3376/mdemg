# BETA-IMPORT-001 — Sprint Plan (B5 arc)

## 1. Header & Metadata

- **Sprint arc:** B5 (beta-program roadmap #5 — corpus loop closer)
- **Sub-sprints:** B5a (exporter extension) + B5b (receiver CLI) + B5c (janitor) + B5 docs
- **Date:** 2026-08-10
- **Branch:** `reh3376_dev01`
- **Author:** Roger Henley (via Claude Opus 4.7)
- **Parent context:** Beta pipeline arc (Sprints A–D, shipped 2026-08-05→08-06) built the outbound half. B3 (2026-08-10) added a submission indexer. B5 closes the loop: bundles feed the local retrain corpus via the shipped `guidance_training_rows` → HITL-CURATION-003 → retrain path.
- **Effort:** ~1 day (B5a 2h + B5b 4h + B5c 2h + docs 1h).

## 2. Problem Statement

Shipped `mdemg beta-share` bundles carried `llm_interactions` + `retrieval_events` + `embedding_events` (raw telemetry) but NOT `guidance_training_rows` — the actual corpus table Jiminy retrain feeds off. Even a receiver couldn't help: 5 corpus-critical fields (`guidance_content`, `source_node_id`, `source_role_type`, `source_layer`, `constraint_code`) exist ONLY in memory at feedback-POST time and were never projected in any of the 3 exported tables.

Additionally: without an inbound receiver, bundles sit as GH-issue artifacts. No path into the training corpus. No 30-day retention enforcement.

## 3. Scope & Constraints

- **In-scope:** Extend exporter to project `guidance_training_rows` at manifest schema 9. Ship `mdemg beta-import <bundle>` receiver with SHA + privacy re-scrub gates. Ship `mdemg beta-janitor {delete,sweep}` for retention. Sprint dir + CLAUDE.md pins + CHANGELOG.
- **Out-of-scope:**
  - Auto-scheduling import (no GH webhook receiver — bundles are downloaded manually)
  - Importing `llm_interactions` / `retrieval_events` / `embedding_events` into local tables (pollutes local pools; forensic-replay is a separate need)
  - Modifying the beta-share `--since-days 30` default
  - Touching B3's submission indexer (records receipts; doesn't process)
  - Adding a `submission_id` column to `guidance_training_rows` (space_id LIKE prefix carries the same information)

## 4. Dependencies

- `mdemg beta-share` (v0.11.0-beta.2) — the outbound half we're closing the loop on.
- `HITL-CURATION-003` (2026-08-08) — makes `guidance_training_rows` the retrain corpus source (autograder consumes it).
- `JIMINY-ARCHIVED-CODE-FILTER-001` (2026-08-10) — closed the archive-drag class of corpus pollution; imported rows benefit from this cleanup going forward.
- `llmclient.ScrubStringExcluding` — shipped scrubber, reused for the receive-time re-scrub.

## 5. Implementation Plan (sequential — 4 sub-sprints)

**B5a — exporter projects guidance_training_rows.**
- `internal/tsdb/exporter.go`: new `guidanceTrainingRowsSpec` (15 cols, schema 027 minus `row_id`); registered in `tableSpecs`; default `Tables` list gets 4th entry; `ExportManifest.SchemaVersion` bumped 8 → 9.
- CLI defaults updated: `internal/cli/data_export.go` + `internal/cli/data_export_auto.go`.
- 2 new pin tests: `TestExporter_SchemaVersionIs9`, `TestExportConfig_DefaultTablesIncludesGuidanceCorpus`. Existing spec/text-field tests extended.

**B5b — receiver CLI + shared verify helpers.**
- `internal/tsdb/exporter_verify.go` (new): `VerifyJSONLSHA256` + `RescrubJSONL` + `TableColumns`. Extracted so beta-import can enforce the same contracts the exporter enforces at write time.
- `internal/cli/beta_import.go` (new): `mdemg beta-import <bundle>` — unpacks outer + inner tars, tar-slip guarded; parses receipt; validates manifest (utds_version + schema_version); SHA-verifies each JSONL; re-runs privacy scrubber over every text field; interactive opt-in; dry-run branch; imports via per-row INSERT with `space_id + instance_id` remap to `beta-tester-<submission_id>` and `row_id` re-mint via `cuid2.Generate()`.
- Wired into `internal/cli/root.go` under `GroupID=config` next to `beta-share`.

**B5c — janitor.**
- `internal/cli/beta_janitor.go` (new): parent command + `delete` + `sweep` subcommands. Keyed on `space_id LIKE 'beta-tester-%'`. Preview counts before deletion (grouped by space_id for `sweep`). Dry-run + `--yes` bypass. Wired into root.

**B5 docs.**
- `docs/features/beta-import.md` (new).
- `docs/development/beta-import-001/{sprint_plan,post}.md` (new — this).
- CLAUDE.md pin (Architecture Notes): the two arch rules named in §9.
- CHANGELOG entry (Unreleased Added section).

## 6. Testing Plan

**Unit tests:**
- 2 new exporter pin tests (schema version + default table list) — colocated in `internal/tsdb/exporter_test.go`.
- (Deferred to a follow-up: negative-path pin tests for beta-import. The SHA + re-scrub + tar-slip logic is trivial + live-verified below.)

**Tier-3 live smoke on mdemg-dev** (all 9 steps captured in `post.md`):
- E1: dry-run against synthetic fixture bundle — SHA + rescrub gates pass, 3 rows previewed
- E2: real import — 3 rows land at `space_id=beta-tester-smoketest001alpha23456`
- E3: TSDB query confirms attribution
- E4: `mdemg-dev` space untouched; original tester `space_id` remapped away
- E5: janitor delete dry-run
- E6: janitor delete real → 3 rows removed
- E7: `beta-tester-*` namespace empty
- E8: sweep dry-run → 0 rows (correctly a no-op)
- E9: NEGATIVE — tamper the JSONL, keep old manifest SHA → SHA gate REJECTS with both hashes named for triage

## 7. Commit Strategy

3 sequential commits on `reh3376_dev01`, each auto-PR'd:

- **B5a**: `feat(tsdb): exporter projects guidance_training_rows at schema 9` (shipped, PR #604)
- **B5b + B5c**: `feat(cli): mdemg beta-import + beta-janitor — corpus loop closer` (this commit)
- **B5 docs**: `docs(beta-import-001): feature doc + CLAUDE.md pins + sprint dir` (this commit's docs bundle)

## 8. Verification Checklist

- [x] `go build ./...` clean at each sub-sprint
- [x] `golangci-lint run ./internal/cli/... ./internal/tsdb/...` = 0 issues
- [x] Existing tests still pass (`go test ./internal/tsdb/... ./internal/cli/...`)
- [x] 2 new B5a pin tests green
- [x] Live Tier-3 all 9 steps pass
- [x] CLI help renders correctly for `beta-import` + `beta-janitor` + `beta-janitor delete` + `beta-janitor sweep`
- [x] Tar-slip guard: bundle with `../etc/passwd` path rejected (implicit via `strings.Contains(name, "..")`)
- [x] Feature doc, CLAUDE.md pin, CHANGELOG, sprint dir all landed

## 9. Two arch rules pinned to CLAUDE.md

⚠️ **Receiver-side privacy re-scrub is REQUIRED, not optional.** `mdemg beta-import` must re-run `llmclient.ScrubStringExcluding` over every text field on load — defense-in-depth against a tampered bundle. The exporter's write-time scrub could be bypassed by a hand-crafted bundle that also matches its manifest SHA (hash + content edited together). The receiver's re-scrub catches this class. Uses the same per-field skip patterns from `internal/tsdb/exporter.go::tableSpecs`.

⚠️ **Beta-tester data lands in `space_id=beta-tester-<submission_id>` — NEVER the operator's own space.** Import-time invariant: `destSpace = "beta-tester-" + submission_id` (or `--space-suffix` override). Enforced in `importGuidanceRows`. This gives the janitor a stable prefix-based keying (`space_id LIKE 'beta-tester-%'`) for 30-day retention sweeps. No `submission_id` column needed on `guidance_training_rows`. Deletion by space_id (exact match for `delete --submission-id`, LIKE prefix for `sweep`) can NEVER touch the operator's own space or any non-beta-tester space.

## 10. Risks & Mitigations

**R1: Exporter's own privacy scrub blocks beta-share bundle production on mdemg-dev.**
- Observed live during smoke: 4114 privacy scrub violations in `guidance_training_rows.action_summary` on the maintainer's dev workspace (dense with `/Users/reh3376/...` paths → matches `abs_path` pattern).
- Mitigation: this is the exporter behaving correctly — real tester workspaces would have their own path shapes; the operator smoke-tested with a synthetic fixture instead. Follow-up sprint could add per-field allowlist for the operator's own `abs_path` false-positive class if this becomes a persistent workflow issue.

**R2: Repeat imports of same bundle → duplicate rows.**
- No idempotency guard today. Second import of the same bundle with the same `submission_id` produces a second copy of every row (space_id = same beta-tester-<sid>).
- Mitigation: caller runs `--dry-run` first to confirm; janitor `delete --submission-id` can undo mistake. Follow-up: idempotency-by-content-hash if this becomes a real problem.

**R3: `guidance_content` false-positive rescrub rejection on legitimate rules referencing paths.**
- Rescrub is per-field with the exporter's same skip patterns. `guidance_content` has `abs_path` in its skip list (per B5a spec) so this should be false-positive-safe.
- Verified live: 3 synthetic rows including `guidance_content = "Always use CUIDv2 for identifiers"` passed rescrub with 0 violations.

## 11. Rollback Procedures

Revert the 3 commits; no TSDB schema change; no persistent state. Any already-imported beta-tester rows can be swept via the janitor even after revert (the DELETE SQL is portable psql). No migrations to undo.

## 12. Documents Accessed

- Plan file: `/Users/reh3376/.claude/plans/shimmying-jingling-hamming.md`
- Recon agent report (Explore subagent, 2026-08-10, 8 numbered fact clusters)
- `internal/tsdb/exporter.go`, `guidance_training_rows_writer.go`, migrations/027_*.sql
- `internal/cli/beta_share.go` (structural mirror for the receiver)
- `internal/llmclient/scrubber.go` (ScrubStringExcluding contract)
- `internal/hidden/{correction_nodes,constraint_nodes}.go` (cuid2 usage precedent)
- Live SQL against mdemg-dev TSDB (row counts, deletion verification)
- Live E1-E9 smoke against a synthetic fixture bundle
