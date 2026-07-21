# Sprint APE-REFLECT-EVAL-REFRESH-001 — Regenerate ape.reflect eval rows from post-budget-fix production prompts

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | APE-REFLECT-EVAL-REFRESH-001 |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | ~0.5 dev-day + 12 min benchmark wall |
| Parent | Operator FT-dashboard investigation (2026-07-21): ape.reflect mean_score 0.623 |

## 2. Problem Statement

The fresh FT-BENCH-REFRESH-001 benchmark scored `ape.reflect` at 0.623 — **an eval artifact, not a model regression**. All 5 rows: `json_valid=0.0` with `finish_reason=length`; the responses are valid JSON arrays truncated mid-array. Root cause: `valid_clean.jsonl`'s 20 ape.reflect rows carry **pre-APE-PROMPT-BUDGET-001 prompts (~4,000 tokens user prompt)**; under llama-server's 8,192-token per-slot KV bound (prompt+think+output share it, `enable_thinking: true`), generation hits the ceiling ~700-1,000 output tokens in. The April baseline "passed" only because mlx_lm.server had unbounded KV — the exact pathology Phase 13.5 migrated away from. Production has been fixed since 2026-06-13 (prompt bounded to ~3,500 tokens; live rows avg ~1,370 est. tokens); the eval must match production reality.

## 3. Scope & Constraints

**In scope**: regenerate ONLY the 20 ape.reflect rows from post-2026-06-14 clean production `llm_interactions` (5,487 clean rows available, avg ~1,370 est. tokens); identical row schema (`meta` keys preserved incl `tsdb_trace_id`/`system_prompt_hash`); response-validity filter (assistant target parses as JSON array); leak-audit the refreshed file with the same `--against` sources as the original audit; **[AMD-2] re-pin**: update `valid_clean_manifest.json` + `docs/development/ft-recursive-001/augmented_eval_manifest.json` `eval_file.sha256` with an amendment note; archive the old file (`.pre_ape_refresh_bak`); re-run the benchmark → corrected panel + aggregate.
**Out of scope**: other tasks' rows (byte-identical); reward functions; benchmark runner; the model.
**Constraints**: regeneration via a checked-in reproducible script (no hand-edited rows); backup before splice; leak-audit must PASS before the manifest re-pin; the [AMD-2] amendment documents provenance (this sprint, date, reason).

## 4. Dependencies

✅ 5,487 clean post-fix production rows; ✅ `audit_eval_leakage.py` + original audit's source list (`valid_clean_leakage_audit.json`); ✅ benchmark runner validated by FT-BENCH-REFRESH-001 (incl. sidecar-apply step); ⚠️ [AMD-2] manifest is promotion-gate-pinned — the SHA re-pin is the governance step, executed with an explicit amendment entry.

## 5. Implementation Plan (sequential)

- **E0** plan (this doc). **E1** recon (done — findings in §2/§3).
- **E2** `scripts/refresh_ape_reflect_eval_rows.py`: sample 20 clean rows (dedup by trace_id, chronological spread, response parses as JSON array), emit identical-schema rows; splice into `valid_clean.jsonl` (backup first).
- **E3** leak-audit refreshed file with original `--against` sources → PASS gate.
- **E4** manifest re-pin: `valid_clean_manifest.json` + [AMD-2] `augmented_eval_manifest.json` (new sha256, rows_total, amendment note).
- **E5** re-run benchmark (same FT-BENCH-REFRESH-001 invocation, `--rows-per-spec 5 --n-runs 1`) + apply SQL sidecar → fresh `benchmark_runs` row.
- **E6** verify: ape.reflect `json_valid=1.0`, `finish_reason != length`, corrected aggregate; dashboard panel shows honest score.
- **E7** docs: CHANGELOG, CLAUDE.md, sprint post.

## 6. Testing Plan

Tier 1: regen script row-schema assertions (roles, meta keys, array-parse). Tier 2: leak-audit PASS + `wc -l` = 240 + non-ape rows byte-identical (diff check). Tier 3: live benchmark re-run against real llama-server; TSDB row + panel verification.

## 7. Commit Strategy

`docs(E0)` → `feat(E2+E3: script + refreshed eval + audit)` → `docs(E4: manifest re-pin)` → `docs(E5+E6: benchmark evidence)` → `docs(E7)`.

## 8. Verification Checklist

script reproducible · backup exists · leak-audit PASS · SHAs re-pinned with amendment · benchmark ape.reflect json_valid=1.0 · aggregate corrected · docs complete · pushed.

## 9. Documentation Update

CHANGELOG Fixed; CLAUDE.md eval-governance note ("eval rows must track production prompt-budget reality; KV-slot truncation in eval = artifact"); sprint post.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| New rows leak vs training data | Med | audit_eval_leakage gate before re-pin; same sources as original |
| [AMD-2] re-pin breaks promotion-gate reproducibility | Med | Amendment entry with old+new SHA, date, reason; backup file retained |
| Sampled rows unrepresentative (all from one day) | Low | Chronological spread sampling across the 5-week window |
| Benchmark busy-server flakiness | Low | Same timeouts as FT-BENCH-REFRESH-001; re-runnable |

## 11. Rollback

Restore `valid_clean.jsonl.pre_ape_refresh_bak` + revert manifest commits. No substrate changes.

## 12. Documents Accessed

`training_data/eval/valid_clean.jsonl` + manifests + leakage audits; `docs/development/ft-recursive-001/augmented_eval_manifest.json` ([AMD-2]); `scripts/{build_clean_eval,audit_eval_leakage}.py`; FT-BENCH-REFRESH-001 live_verification (sidecar-apply learning); benchmark report JSON (finish_reason evidence); `llm_interactions` schema; CLAUDE.md §APE-PROMPT-BUDGET-001, §Phase 13.5.
