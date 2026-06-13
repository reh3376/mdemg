# Sprint Plan — DATAPRUNE-AUDIT-001: Audit Non-Conforming Training Data (Prune Phase, Audit-First)

## 1. Header & Metadata
2026-06-13 · branch `reh3376_dev01` · training-integrity remediation (the
operator's standing data-cleanup directive — `feedback_prune_nonconforming_data`)
· effort ~1d audit + gated prune · risk **medium** (the audit is read-only; the
prune is destructive and operator-gated, executed small-batch with backup).

## 2. Problem Statement
After three correctness fixes (EVAL-INTEGRITY-001, REWARD-CORRECTNESS-001,
APE-PROMPT-BUDGET-001) "conforming" is now concretely defined. The standing
directive: prune every dataset that fails correctness/reliability — storage is
finite and bad data poisons retrains. **Audit-first** (operator-chosen): a
non-destructive enumeration with exact counts + a backup/small-batch/verify
plan BEFORE any deletion, so nothing is pruned against an undefined bar or
without a recovery path.

## 3. Scope & Constraints
**In (this sprint — non-destructive):** the audit (Epic 1, complete below) +
the gated prune *plan*. **Gated (separate operator approval per category):** the
actual deletions. **Out:** the schema/reward-mismatch fixes (NOT corrupt data —
own follow-ups). **Constraints:** backup before any destructive op; small-batch
(LIMIT 5) verify then full; reversible-move before hard-delete; never touch the
`mdemg-dev` deletion guards; the protected space's nodes are CMS memory, NOT a
prune target — only TSDB telemetry rows + file artifacts are.

## 4. Dependencies
TSDB `llm_interactions` (PG16 `pg_input_is_valid`); `mdemg data clean`
(error/silent-failure remover, dry-run default); the rerank archive dir;
`training_data/eval/`; the three correctness-fix sprints (define "conforming").

## 5. AUDIT FINDINGS (Epic 1 — COMPLETE, read-only)

### Genuinely corrupt / stale — PRUNE TARGETS
| # | Target | Count | Notes |
|---|--------|-------|-------|
| A | invalid-JSON rows in object/array tasks (`llm_interactions`) | **2,111** | ape.reflect 1890, rerank_cross 184, evaluate_llm 18, query_classify 18, classify 1 |
| B | error / empty-response rows (`llm_interactions`) | **21,135** | existing `mdemg data clean` target (error records + silent failures) |
| C1 | rerank mislabeled pre-fix archive | **6,894 events / 202 files / 21M** | `.mdemg/neural/training-data-prefix-archive/` — SIDECAR-LOOP-001 pre-fix collector (100% mislabeled candidate↔score) |
| C2 | leaked eval `valid_golden.jsonl` | **108 rows** | 99% leaked with training data; superseded by `valid_clean` (EVAL-INTEGRITY-001) |
| C3 | stale baselines | ~14 `baseline_*.json` | computed under OLD reward + MLX form (mostly April); the frozen 0.8338 in `benchmark_qwen3_14b_v1_baseline.json` |

**Total TSDB rows to prune: ~23,246** (2,111 corrupt + 21,135 error) of 102,392
(~22.7%). ape.reflect's 1,890 corrupt rows are concentrated **2026-06-11→06-13**
(1355 on 06-11) — the APE-PROMPT-BUDGET-001 truncation window; post-fix rows are
valid (bleeding stopped).

### NOT prune targets — schema/reward mismatch (data is fine; fix the DEFINITION)
- **hidden.summarize (72)** — emits a bare prose summary; its ULTS schema
  declares `{object, required:["summary"]}`. The summaries are correct, just
  not wrapped. → REWARD-CORRECTNESS follow-up (fix the schema/reward, keep data).
- **string-type tasks** the `jsonb` validity check false-flags —
  `retrieval.intent_translate` (4,260), `jiminy.codegen` (823),
  `jiminy.synthesize` (486): their ULTS `output_schema.type` is `string`; they
  emit valid bare strings. **NOT corrupt.** (Audit pitfall: never run a
  jsonb-validity prune predicate against string-schema tasks.)
- **jiminy.evaluate** explanation_quality mismatch (REWARD-CORRECTNESS finding).

## 5b. Gated Prune Plan (Epic 2+ — each category separately operator-approved)
1. **Backup first** — `mdemg data export` (TSDB archive) + `pg_dump
   llm_interactions` to `.mdemg-backup-<ts>/` before ANY delete. The file
   targets are `git mv`'d to a dated `_pruned-archive/` (reversible) before hard
   delete.
2. **Category B (21,135 error rows)** — `mdemg data clean --space-id mdemg-dev`
   (dry-run first, confirm count, then `--dry-run=false --force`). Existing tool.
3. **Category A (2,111 corrupt JSON)** — small-batch (LIMIT 5) verify the
   `NOT pg_input_is_valid(response,'jsonb') AND type-is-object/array` predicate
   hits only truncated/garbage rows, then delete by id. May need a new
   `mdemg data clean --invalid-json` predicate (string-schema tasks EXCLUDED —
   audit pitfall above) rather than ad-hoc SQL.
4. **Category C** — `git mv` archive + valid_golden + stale baselines to a dated
   pruned-archive dir; retain `valid_clean`, the fixed-reward June baseline, and
   the tables the FT recursive loop needs.
5. **Verify after each** — row counts match the audit; spot-check no
   conforming data removed; the live stack still healthy.

## 6. Testing Plan (3 tiers)
T1: the prune predicate as a unit/SQL test on a fixture (invalid-JSON object row
deleted; valid row kept; string-schema row NEVER matched). T2: dry-run on the
real table prints the exact audit counts (2,111 / 21,135) — a mismatch aborts.
T3 (LIVE): after a gated prune, re-query the audit counts → zero non-conforming
remain in the pruned category; `mdemg data export` backup exists + restores;
live stack healthy; a fresh ape.reflect row is valid (post-fix invariant holds).

## 7. Commit Strategy
Epic 0 (plan) + Epic 1 (this audit doc) commit now, non-destructive. Each prune
category is its own operator-gated commit AFTER approval, with the backup
artifact path recorded in the commit body.

## 8. Verification Checklist
- [ ] Audit counts reproduced by a dry-run (2,111 corrupt + 21,135 error)
- [ ] Backup taken + restore-verified before any delete
- [ ] String-schema tasks EXCLUDED from any jsonb-validity predicate
- [ ] Each category small-batch (LIMIT 5) verified before full delete
- [ ] Schema/reward-mismatch data (hidden.summarize etc.) NOT pruned
- [ ] Post-prune audit counts → 0 in pruned category; live stack healthy

## 9. Documentation Update — final epic (never cut): CHANGELOG + post + feature note.

## 10. Risks & Mitigations
Pruning a schema/reward mismatch as if corrupt → the audit's explicit NOT-target
list + string-schema exclusion. Over-broad SQL delete → small-batch + backup +
reversible-move. Deleting `mdemg-dev` CMS nodes → out of scope; only TSDB
telemetry + files. ape.reflect corrupt rows are real training data the recursive
loop might want → they are TRUNCATED/invalid (json_valid-rejected by the gate
anyway); confirmed unusable, safe to drop.

## 11. Documents Accessed
`feedback_prune_nonconforming_data` memory; the three correctness-fix sprints;
live TSDB `llm_interactions` (counts, pg_input_is_valid, per-task invalid, time
distribution); the rerank archive dir; `training_data/eval/` listing; ULTS
specs (output_schema types).

## 12. Rollback Procedures
The audit is read-only (nothing to roll back). The gated prune: restore from the
`mdemg data export` archive / `pg_dump` + `git mv` the pruned-archive files back.
No CMS-node or schema changes.
