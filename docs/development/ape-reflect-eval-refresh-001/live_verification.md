# APE-REFLECT-EVAL-REFRESH-001 — E5/E6 Live Verification

**Date:** 2026-07-21 | benchmark run `ed062ea8…` (sidecar applied; 61 statements)

## Before / after (identical scope: --rows-per-spec 5 --n-runs 1, same GGUF endpoint)

| Metric | jc81749c (old rows) | ed062ea8 (refreshed rows) |
|---|---|---|
| ape.reflect finish_reason | `length` ×5 (clipped mid-array) | **`stop` ×5** |
| ape.reflect json_valid | 0.00 | **1.00** |
| ape.reflect task mean | 0.623 | **~0.94** (insight 0.9, actionability 0.85-1.0) |
| aggregate_weighted_score | 0.8544 | **0.9188** |

TSDB `benchmark_runs` now: ed062ea8 0.9188 (2026-07-21) > jc81749c 0.8544 (2026-07-21) > q283a23b 0.8338 (2026-04-24). The FT dashboard "Latest Run" panel reads ed062ea8; "Latest Benchmark Age" resets to ~0d.

## Chain of custody

- Refresh script: `scripts/refresh_ape_reflect_eval_rows.py` (deterministic chronological-spread sampling; array-parse filter)
- New rows: 20, prompts ~1,400 est. tokens max (was ~4,000); other 220 rows byte-identical (verified 20/240 diff vs backup)
- Leak-audit: CLEAN 0/240 vs the original 9 sources (report regenerated in place)
- [AMD-2] re-pin: `f215a34a…` → `79fd8e24…` in both manifests with a full amendment entry
- Backup: `training_data/eval/valid_clean.jsonl.pre_ape_refresh_bak`

Conclusion: the 0.623 was an eval artifact (pre-budget-fix prompts × 8K KV slot), exactly as diagnosed. The honest model score on production-realistic ape.reflect prompts is ~0.94.
