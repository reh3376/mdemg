# REVIEW-CANDIDATES-DEDUP-409-001 + REVIEW-CANDIDATES-EXCLUDE-ERROR-ROWS-001 — Sprint Post

**Date:** 2026-08-13
**Branch:** `reh3376_dev01`
**Trigger:** Both defects surfaced by Fable HITL bulk-grade sessions (#106, #110); both are `FetchCandidates` WHERE-clause additions on the same code path, so shipping as one commit is efficient.

## Problem

Two coupled defects on the HITL candidate-selection surface:

**#109 (contradicted_drafts dedup)**: Fable pass 1 hit 3 already-graded contradicted_drafts items surfacing in `GET /v1/review/candidates`; the write path 409'd on the HITL-CURATION-002 idempotency contract but only after Fable had already spent per-item LLM reasoning cost. Root cause: `FetchPendingBySpace` filtered on the draft's OWN `status='pending'` column, not on `review_grades`. When an auto-grader like Fable submits a grade on a `contradicted_drafts` item whose sink refuses (`HITL-AUTO-DISMISS-001` `NonReinforcingApplier` returning `handled=false` on `reinforce=false + reinforceable-verdict`), the grade lands in `review_grades` BUT the draft's `status` stays 'pending' (the substrate-mutation path never fired). So the queue re-serves the same drafts forever and every attempt on them 409s.

**#112 (llm dataset error/empty exclusion)**: Fable pass 2 hit 5 ungradeable rows across 3 llm:* datasets: `jiminy.evaluate ×1, jiminy.synthesize ×2, retrieval.intent_translate ×2` — all with LLM caller-cancellation artifacts (`error LIKE 'caller_canceled:%'` per LLM-HEALTH-INVESTIGATION-001) or empty response text. Same class as #109: wasted per-item grader reasoning cost on things there's no way to grade. The `alert.LLMErrorRateRule` ALREADY filters `caller_canceled:%` out of the error-rate signal per LLM-HEALTH-INVESTIGATION-001; the HITL corpus consumer should apply the SAME filter (contract: if the retrain pipeline wouldn't consume the row, don't offer it for grading).

## Shipped

Both fixes as one commit — both are additive WHERE-clause changes on the candidate-selector code path. Sibling datasets (guidance, llm) already had the `LEFT JOIN review_grades ... AND r.item_id IS NULL` dedup pattern — this sprint brings contradicted_drafts to parity and layers the error-row filter on llm:*.

**`internal/tsdb/contradicted_drafts_writer.go::FetchPendingBySpace`** (#109):
- Extended query with `LEFT JOIN review_grades r ON r.dataset_id='contradicted_drafts' AND r.item_id=d.id AND r.reversed=FALSE AND r.rubric_version='gr-v1'` + WHERE clause `AND r.item_id IS NULL`.
- Rubric version pinned to `gr-v1` (current); reversed grades stay eligible for re-grading (reversed=FALSE predicate — mirrors llm/guidance sibling patterns).
- Aliased draft columns as `d.*` (was unqualified; needed for JOIN clarity).

**`internal/api/llm_dataset.go::FetchCandidates`** (#112):
- Extended WHERE clause with `AND (i.error IS NULL OR i.error = '' OR i.error NOT LIKE 'caller_canceled:%') AND i.response IS NOT NULL AND LENGTH(i.response) > 0`.
- Handles all 3 nil/empty/tagged forms of "not gradeable" that the LLM interaction recorder writes.

**Pin tests**:
- `internal/tsdb/contradicted_drafts_dedup_test.go` (new) — `TestFetchPendingBySpace_SQLIncludesGradeDedup`: source-string inspection asserting the required predicates are present in the FetchPendingBySpace body. Cheap pin catching the "somebody removed the JOIN" regression class. (Full runtime LEFT JOIN correctness needs a real Postgres; source-string pin is sufficient for the removal-regression class.)
- `internal/api/llm_dataset_filter_test.go` (new) — `TestLLMDataset_FetchCandidates_ExcludesErrorAndEmptyRows`: same source-string inspection pattern, asserts the 4 required predicates (`caller_canceled:`, `NOT LIKE`, `i.response IS NOT NULL`, `LENGTH(i.response) > 0`) are present.

Both pins live under `readSourceFile` helper (in tsdb) or inline `os.ReadFile` (in api). Simple, self-documenting.

## Live Tier-3 (mdemg-dev, 2026-08-13)

- Rebuilt binary + `launchctl kickstart -k gui/501/com.mdemg.server` → server up at 11:33 EDT, `/healthz` all subsystems ok
- **contradicted_drafts dedup live**: pending count dropped **7 → 2** (5 previously-graded drafts correctly excluded from the queue). Neither surviving item's grade_ids appear in review_grades — genuinely never-graded.
- **llm error/empty exclusion live**: 3 sampled `llm:jiminy.evaluate_llm` items all have non-empty responses (`resp_len` 253-360) + empty `error` prefix. Raw TSDB query confirms **175 caller_canceled/empty rows** exist in the 7d window that the OLD query would have surfaced — now correctly excluded.
- Full test suite green: `go test ./internal/api/ ./internal/tsdb/ ./internal/config/ ./internal/llmclient/` PASS
- Lint clean: `golangci-lint run ./internal/api/ ./internal/tsdb/` — 0 issues

## Two arch rules pinned (CLAUDE.md)

1. **HITL candidate-selectors MUST use the LEFT JOIN review_grades + IS NULL pattern to exclude already-graded items at the current rubric_version.** The pre-fix `WHERE status='pending'` filter on the source table catches operator-approve/dismiss transitions (which mutate status) but MISSES auto-grader writes with sink-refuses (per HITL-AUTO-DISMISS-001's `NonReinforcingApplier` returning `handled=false`) — those grade rows land but the source table's `status` column stays `'pending'`. Result: queue re-serves items forever + write endpoint 409s. Mirrors the shipped guidance + llm dataset filter pattern; every new HITL dataset MUST implement this JOIN before its first grader interaction.

2. **HITL candidate-selectors on LLM-derived datasets MUST filter out `caller_canceled:*`-tagged rows and empty responses.** These are ungradeable by construction — the LLM interaction recorder tags client-cancellations with `caller_canceled:` prefix per LLM-HEALTH-INVESTIGATION-001, and empty responses have no content to grade. The `alert.LLMErrorRateRule` ALREADY excludes these from the error-rate signal — the HITL corpus filter must apply the SAME exclusion. Contract: if the retrain pipeline wouldn't consume the row, don't offer it for grading.

## Follow-ups disclosed

- **Extend the caller_canceled exclusion pattern** — if the recorder ever adds a new noise-class prefix (e.g. `caller_timeout:`, `provider_500:` for structured retry-eligible errors), extend the WHERE clause + update `LLMErrorRateRule`'s exclusion in the same commit. Same rule as EXPORT-SCRUB-INTAKE-001's "intake and export scan must share one predicate" — divergence between filter sites is silent-drift risk.
- **Consider a shared `hitl_candidate_filter` SQL fragment** so future datasets can `WHERE <sharedFilter>` — currently each dataset has its own WHERE and the caller_canceled/empty class is now duplicated across llm:* + will need mirror on any future LLM-derived dataset. Deferred; ship if a 4th LLM dataset lands.
- **Post-fix Fable pass 3 would give a cleaner baseline** — pass 2's 3 known 409s + 5 ungradeables would be zero on a fresh run. Not required to ship; documented for the next HITL run.

## Documents Accessed

- `internal/tsdb/contradicted_drafts_writer.go` — `FetchPendingBySpace` (edit site)
- `internal/api/llm_dataset.go` — `FetchCandidates` (edit site)
- `internal/api/guidance_dataset.go` — sibling LEFT JOIN pattern reference (already correct)
- `internal/api/contradicted_drafts_dataset.go` — the caller wrapping FetchPendingBySpace
- CLAUDE.md pins: HITL-CURATION-002 (auto-grader invariant), HITL-AUTO-DISMISS-001 (NonReinforcingApplier), LLM-HEALTH-INVESTIGATION-001 (caller_canceled tagging), LLM-HEALTH-CANCELLATION-ALERT-001 (alerter filter)
- Task #106 (HITL-BULK-GRADE-SESSION-001) + #110 (HITL-BULK-GRADE-SESSION-002) sprint posts — the Fable runs that surfaced both defects
- Live: `mdemg data export-auto` clean run, TSDB filter verification via raw SQL, `/v1/review/datasets` + `/v1/review/candidates` sampling post-restart
