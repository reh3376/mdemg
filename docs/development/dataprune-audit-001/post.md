# DATAPRUNE-AUDIT-001 — Sprint Post

**Date:** 2026-06-13 · branch `reh3376_dev01` · training-integrity remediation
(operator-chosen audit-first prune phase, `feedback_prune_nonconforming_data`).

## Outcome
Pruned **1,898 genuinely-corrupt invalid-JSON rows** from `llm_interactions`
(operator-approved Category A), backup-first and small-batch-verified. The live
stack stayed healthy; zero corrupt rows remain; the APE-PROMPT-BUDGET-001
post-fix invariant holds (recent ape.reflect 14/14 valid).

## What was done
- **Epic 1 (audit, read-only)** — enumerated all non-conforming TSDB/file data.
  Targets + the explicit NOT-targets (schema/reward mismatches) in
  `sprint_plan_dataprune_audit_001.md`.
- **Epic 2 (Category A prune, operator-gated)** — deleted the corrupt JSON rows.

## The refinement that small-batch verify caught (why this mattered)
The audit's raw `NOT pg_input_is_valid(response,'jsonb')` flagged **2,111** rows.
Spot-checking before deletion revealed three classes:
- genuinely truncated / malformed (ape.reflect mid-array cutoff) — corrupt;
- **markdown-fenced** valid JSON (` ```json [...] ``` `, rerank_cross 184);
- **think-wrapped** valid JSON (`<think></think>{...}`, query_classify 18).

The production parser (`llmclient.SanitizeResponse` = StripThinkBlock +
StripCodeFence + Trim) recovers the latter two. Validating **all 2,111
candidates through a faithful replica of that exact parser**, **213 were
recoverable good data** (rerank_cross 184, query_classify 18, ape.reflect 11)
and were **spared**; **1,898 were unparseable even after sanitization** —
the true delete set (ape.reflect 1,879, jiminy.evaluate_llm 18,
consulting.classify 1). Deleting the raw 2,111 would have destroyed 213 valid
training rows. **Audit pitfall recorded:** a corrupt-data predicate must mirror
the production sanitizer, not raw JSON validity.

## Execution (backup-first, reversible)
1. **Backup** — full row data for all 1,898 (by unique `trace_id`) →
   `.mdemg-backup-20260613_195431/dataprune/dataprune_backup.csv` (38M) +
   `corrupt_trace_ids.txt`. Staged-id count == live-match count == 1,898.
2. **Delete** — `DELETE … USING doomed(trace_id)` in one verified session:
   total_before 102,415 → total_after 100,517 (exactly −1,898);
   `remaining_corrupt = 0`.
3. **Verify** — live `/healthz` ok; 0 corrupt across ALL object/array tasks
   (incl. the spared rerank_cross/query_classify — untouched); recent
   ape.reflect 14/14 valid.

## Carried forward (operator-declined this round / out of scope)
- **Category B** (21,135 error/empty rows, `mdemg data clean`) and **Category C**
  (rerank archive 6,894 events/21M, leaked `valid_golden` 108, ~14 stale April
  baselines) — not pruned this round; remain enumerated for a future gated pass.
- **Schema/reward mismatches** (NOT corrupt; fix the definition): hidden.summarize
  (72, prose vs object schema), jiminy.evaluate explanation_quality — the
  REWARD-CORRECTNESS follow-ups.
- **Noted, unrelated to this prune:** the recurring `alert_embedding_regression
  — empty call_sites` and a `Significant Node Count Drop` (Neo4j, not TSDB) —
  worth a triage pass; not caused by this TSDB row deletion.

## Documents Accessed
`feedback_prune_nonconforming_data`; the three correctness-fix sprints; live
TSDB `llm_interactions`; `internal/llmclient/sanitize.go` (the production parser
the delete predicate was matched to); ULTS specs (output_schema types).
