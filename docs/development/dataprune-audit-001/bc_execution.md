# DATAPRUNE-AUDIT-001 — Category B + C Execution

**Date:** 2026-06-14 · operator-approved continuation after Category A (#463).
Backup-first, reference-checked.

## Category B — error / silent-failure rows (DELETED)
Via `mdemg data clean` (predicate `internal/tsdb/cleaner.go`: Gate2 `error IS
NOT NULL AND error != ''` + Gate1 `(error IS NULL/empty) AND length(response) <
10`). Backed up all matching rows first → `.mdemg-backup-20260613_195431/
dataprune_BC/B_errorrows.csv` (267M, 21,254 rows).

| Space | Deleted | Before → After |
|-------|---------|----------------|
| mdemg-dev | 20,856 (20,754 err + 102 silent) | 94,982 → 74,126 |
| lnl-demo-whk | 271 | 1,105 → 834 |
| whk-wms | 126 | 4,567 → 4,441 |
| demo | 1 | 38 → 37 |
| **Total** | **21,254** | |

Post: error/empty rows remaining across ALL spaces = **0**; live `/healthz` ok.

### ⚠️ Verification catch (the dry-run survivor-table misread)
The `mdemg data clean` dry-run prints a per-task table that is the count of
**surviving good rows** (`fillPerTaskCounts`: `non-error AND length ≥
min_response_len`), which totalled 74,126 — easy to misread as the delete set.
The real delete count is `errors_found + silent_fails_found` (20,856 for
mdemg-dev), cross-checked against direct SQL before running `--force`. **Always
read the JSON `errors_found/silent_fails_found`, not the per-task table, as the
delete count.**

## Category C — file artifacts (REFERENCE-CHECKED; only 1 of 3 pruned)
A reference scan changed the C scope substantially — most "stale" artifacts are
still load-bearing:

| Target | Refs | Action |
|--------|------|--------|
| rerank mislabeled archive (`.mdemg/neural/training-data-prefix-archive/`, 6,894 events / 21M) | **none** | **MOVED** → `.mdemg-backup-…/dataprune_BC/rerank-prefix-archive` (reversible) |
| `valid_golden.jsonl` | **9** (rl_phase11.yaml, benchmark_phase10.yaml, ubench spec, clean-eval builders as a **leak SOURCE**, rl trainer/wiring) | **RETAINED** — removing it breaks leak-filtering + config paths |
| ~14 `baseline_*.json` (incl. frozen 0.8338 `benchmark_qwen3_14b_v1_baseline.json`) | referenced by `regression.py`, `run_benchmark.py`, configs | **RETAINED** — removing breaks the regression harness |

**Why retained ≠ ignored:** valid_golden + the baselines are *stale* but
*referenced*. The correct retirement is the **baseline-recompute sprint's** job:
after `regression.py` is re-pointed to a fresh post-fix baseline and
`build_clean_eval` no longer needs valid_golden as a leak source, they become
unreferenced and prunable. A blind file move now would break the regression
gate and leak detection. Documented so they're retired deliberately, not lost.

## Final state
`llm_interactions`: **79,461 rows, 0 non-conforming** (0 error/empty, 0
invalid-JSON after Category A). All deletions backed up + reversible.

## Carried forward
- Retire valid_golden + stale baselines during the baseline-recompute sprint
  (once their referencing configs/harness move off them).
- Schema/reward-mismatch fixes (hidden.summarize, jiminy.evaluate) — separate.
