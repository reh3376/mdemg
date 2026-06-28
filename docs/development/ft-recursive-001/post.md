# FT-RECURSIVE-001 (Phase 6a) — Post

**Status: COMPLETE** · 2026-06-28 · branch `reh3376_dev01`

Phase 6a of the recursive-retraining loop: the manual retrain path is now fully
observable + the silent-failure seams on it are closed. **No actuator, no
autonomy, no serving mutation shipped** — the scope boundary held.

## Shipped (7 epics)
1. **Epic 1 — SF-3:** RSIC diagnostic alerts now use `rsic-<action>` services (were all `("rsic",Medium)` → mutual suppression). Live: `rsic-trigger_training_pipeline`, `rsic-alert_llm_health` distinct in the alert file.
2. **Epic 2 — SF-7:** `evaluateReadinessGates` (pure, unit-tested) surfaces per-gate not-ready reasons in `mdemg data status`. Live: `jiminy.evaluate insufficient_rows: 310 < 500`, `guardrail.evaluate: 3 < 500`, a `missing_system_prompt` case.
3. **Epic 3 — SF-1:** heartbeat gauge `mdemg_rsic_readiness_assessed` set on each successful assessment + `training_readiness_stale` evaluator rule (idle-safe, distinct `ft-readiness` service, `FT_READINESS_STALENESS_MIN`=30). Live: gauge lands; rule registered (25 rules).
4. **Epic 4 — stage instrumentation (core):** `mdemg ft-loop report-stage` → `scheduled_job_events` (`job_name=ft-loop:<stage>`) via the NOSILENT-001 CLI-job pattern; failure → high alert under distinct `ft-loop` service (`jobhealth.ReportWithService`). Live: capture-success + train-failure rows landed; failure dispatched `service=ft-loop severity=high`.
5. **Epic 5 — eval pin `[AMD-2]`:** committed `augmented_eval_manifest.json` (recipe + SHA f215a34a… verified + 17-task spec_hashes + leak verdict CLEAN 0/240 + gating policy). Closes the "gate references an ad-hoc rebuild of a gitignored set" gap.
6. **Epic 6 — docstrings + SF-6:** `evaluate_ft.py` `:8101`→`:8102`; `pruneOldExports` (`MDEMG_EXPORT_RETENTION_HOURS`=168) ends the unbounded `mdemg-exports` growth.
7. **Epic 7 — docs:** feature doc `docs/features/ft-recursive-loop.md`; this post; CHANGELOG; CLAUDE.md note; **stale-FT-doc correction** (see below); `00_README_v2.md` STATUS.

## Stale-doc correction (a finding of this sprint)
The request that opened this work ("start FT-CLASSIFY-002") was based on stale
canonical docs: **FT-CLASSIFY-002 was already COMPLETE** (PR #446, 2026-06-12,
NO-PROMOTE), and **FT-RECURSIVE-000** is complete (doc-only). CLAUDE.md "Open FT
work", ROADMAP §Phase 4, and OUTSTANDING_BACKLOG listed FT-CLASSIFY-002 as open.
Corrected here so the FT build queue (6a→6b→7→9 + GUARDRAIL-PRODUCER-001) is the
documented next work.

## Testing
- **Tier 1:** SF-3 distinct-service, SF-7 6-case gate reasons, SF-1 rule SQL contract, ft-loop stage validation, SF-6 prune — all green; `verify_config_consumers` 731/731; lint 0; ruff clean.
- **Tier 3 (live):** table above (all six items verified against the live binary + TSDB + alert stream). Test rows cleaned (`DELETE … cycle_id='ftc-test-6a'`, operator-confirmed).

## Follow-ups (next phases, not this sprint)
- FT-RECURSIVE-002 (actuator), -003 (RSIC integration/canary), -004 (drift/issue-filer).
- GUARDRAIL-PRODUCER-001 (the `guardrail.evaluate` producer — 3 rows, can't retrain without it).
- The ft-loop stages have no staleness rule yet (correct for a *manual* path; the automated loop adds `ft_loop_never_ran` in 6b).

## Documents Accessed
- `SPEC_recursive_retraining_loop.md`, `ft-classify-002/{post,run_record}.md`
- `internal/ape/{task_dispatch,self_assess}.go`, `internal/tsdb/dataset_builder.go`, `internal/jobhealth/jobhealth.go`, `internal/alert/rules.go`, `internal/metrics/collectors.go`, `internal/api/handlers_training_data.go`, `internal/cli/{ft_loop,data,job_report,root}.go`
- `scripts/{build_clean_eval,audit_eval_leakage}.py`, `neural/training/evaluate_ft.py`, `configs/benchmark_phase10.yaml`
- Live `mdemg data status`, `scheduled_job_events`, `metric_samples`, alert stream
