# Sprint FT-RECURSIVE-001 (Phase 6a) — Harden + instrument the manual retrain path

## 1. Header & Metadata
- **Sprint ID**: FT-RECURSIVE-001 (recursive-loop Phase 6a)
- **Sprint line**: `docs/development/ft-recursive-001/` (alongside `SPEC_recursive_retraining_loop.md` — the authoritative design)
- **Date opened**: 2026-06-26
- **Target version**: v0.11.x
- **Estimated effort**: ~3 dev-days
- **OpenAI spend**: $0 (no training/teacher calls — this sprint is observability + hardening only; it does NOT run a retrain)
- **Risk level**: Low–Medium (additive instrumentation + alert-hygiene + one config field; no actuator, no serving mutation, no autonomous operation)

## 2. Problem Statement
The recursive-retraining loop's **manual vertical slice** (FT-CLASSIFY-002) was walked by hand but is **not observable**: its stages leave no trace in `scheduled_job_events`, a failed stage fires no alert, and several silent-failure seams (SPEC §2 inventory) sit on the path. Per the SPEC's phased plan (§4), Phase 6a hardens + instruments that manual path so a future end-to-end run is **fully observable** — the prerequisite before 6b builds the autonomous actuator. **No actuator, no autonomy, no serving mutation ships here** — that is explicitly 6b/7.

Exit criteria (SPEC §4): *a manual end-to-end run is fully observable — every stage lands in `scheduled_job_events`, failures alert.*

## 3. Scope & Constraints
**In scope** — the 7 Phase-6a items from SPEC §4 + the SF inventory:
1. **SF-3 — distinct RSIC alert Services.** `task_dispatch.go:1152` `executeAlertLog` hardcodes Service `"rsic"` for all 6 diagnostic executors → they share the `(Service,Severity)` dispatcher cooldown key and mutually suppress (the exact NOSILENT-001 class). Give each diagnostic action a distinct Service.
2. **SF-7 — per-gate readiness reasons.** `dataset_builder.go:473` computes `Ready` as a 3-gate boolean (`TotalRows >= threshold && errorRate < 0.05 && HasSystemPrompt == TotalCalls`) but surfaces only the bool — `ape.reflect` (70,880 rows) is Ready=NO with no visible reason. Expose which gate(s) failed.
3. **SF-1 — readiness-staleness evaluator rule.** `self_assess.go:237` skips on a readiness-query error (`dErr == nil` guard) → insight #29 silently never fires, loop goes dormant with no alert. Add an evaluator rule that alerts when the readiness assessment goes stale.
4. **ft-loop stage jobhealth instrumentation** (the core deliverable). A reporting surface so each manual stage (capture/curate/train/benchmark+gate/promote) lands a row in `scheduled_job_events` (`job_name='ft-loop:<stage>'`) via `jobhealth.Report`, and a failed stage fires a high alert — reusing the NOSILENT-001 CLI-job pattern (short-lived pool + file-backed dispatcher).
5. **16-task augmented eval pinned as a versioned artifact** `[AMD-2]`. The promotion-gate eval (16-task augmented, leak-audited, + `valid_clean.jsonl`) is today a recipe (`build_clean_eval.py --target-per-task 20` + `audit_eval_leakage.py`), not a committed file. Pin its construction + a manifest with SHAs so the gate is reproducible and leak-audited.
6. **Docstring/endpoint fixes.** `evaluate_ft.py` docstrings reference the decommissioned `:8101` (production is llama-server `:8102`; `--base-url` default is `None`). Fix docstrings; verify `benchmark_phase10.yaml` endpoint (the run_record caught a `:8101` rot — confirm it stays fixed).
7. **SF-6 — export hygiene.** `handlers_training_data.go:80` writes exports to `os.TempDir()/mdemg-exports/` forever. Add retention/cleanup.

**Out of scope (explicitly — these are 6b/7/9):** the actuator/controller, the `ft_training_cycles` ledger + writer, trigger conditions / single-flight, compute lease / RSIC quiesce, the new mutating-action class, canary/auto-rollback, drift monitoring, the issue filer, any autonomous retrain. SF-2 (insight #29 re-fire gate) is fixed by 6b's ledger — NOT here. SF-4/SF-5 collectors stay out (not on the loop-critical path until 6b enables them).

**Constraints:** no-hardcoding (new knobs are env/config with defaults); sequential epics; 3 testing tiers + live Tier-3; CUIDv2 for any new id; docs final epic; new Python (if any) passes neural pytest+ruff + the UxTS drift checker.

## 4. Dependencies
- `internal/jobhealth.Report` (`jobhealth.go:27`), `scheduled_job_events` (V0024), `internal/alert` dispatcher + evaluator-rule pattern, `internal/tsdb/dataset_builder.go` readiness, `internal/ape/{task_dispatch,self_assess}.go`, the NOSILENT-001 `reportScheduledJob` CLI-job pattern (serve.go / the CLI jobs), `scripts/{build_clean_eval,audit_eval_leakage}.py`.

## 5. Implementation Plan (sequential epics + gates)

**Epic 0 — Sprint plan** (this doc).

**Epic 1 — SF-3 distinct RSIC alert Services.** Thread a per-action Service into `executeAlertLog` (derive from `spec.ActionType`, e.g. `rsic-<action>`), so the 6 diagnostic alerts no longer share one cooldown key. Preserve title/severity. **Gate:** unit test asserts distinct Services; `trigger_training_pipeline` gets its own Service.

**Epic 2 — SF-7 per-gate readiness reasons.** Add a `NotReadyReasons []string` (or equivalent) to `TaskReadiness`, populated when a gate fails (`rows<threshold` / `error_rate>=0.05` / `system_prompt_missing=N`). Surface it in `mdemg data status` output. **Gate:** unit test over the 3 gate-fail cases; live `mdemg data status` shows `ape.reflect`'s real reason.

**Epic 3 — SF-1 readiness-staleness rule.** Emit a heartbeat/timestamp when the RSIC readiness assessment runs successfully (a gauge or a `scheduled_job_events` row), and add an evaluator rule (`training_readiness_stale`, distinct Service, idle-safe COALESCE per the TSDB-CONSUME-001 SQL contract, config floor) that alerts when no successful assessment in `FT_READINESS_STALENESS_MIN`. **Gate:** rule SQL returns idle-safe; unit test.

**Epic 4 — ft-loop stage jobhealth instrumentation (core).** Add `mdemg ft-loop report-stage --stage <capture|curate|train|benchmark|gate|promote> --status <success|failure> [--detail …] [--cycle-id …]` that writes `scheduled_job_events` (`job_name='ft-loop:<stage>'`) via the NOSILENT-001 short-lived-pool + file-dispatcher pattern; failure → high alert (distinct Service `ft-loop`). This is the operator/script hook that makes a manual run observable without the actuator. **Gate:** running each stage-report lands a row; a failure report fires an alert (live Tier-3).

**Epic 5 — Pin the 16-task augmented eval artifact** `[AMD-2]`. Commit a build manifest (`docs/development/ft-recursive-001/augmented_eval_manifest.json`) capturing the `build_clean_eval.py` recipe params + per-source SHAs + the `audit_eval_leakage.py` verdict, so the promotion gate references a SHA-pinned set (not an ad-hoc rebuild). Document that `valid_golden.jsonl` is barred from gating. **Gate:** manifest reproduces; leak audit clean.

**Epic 6 — Docstring/endpoint fixes + SF-6 export hygiene.** Fix `evaluate_ft.py` `:8101`→`:8102` docstrings; verify `benchmark_phase10.yaml`. Add retention to the export dir (`MDEMG_EXPORT_RETENTION_*`, prune on write). **Gate:** grep shows no stale `:8101` in non-rollback contexts; export prune unit test.

**Epic 7 — Documentation (final, never cut).** Feature doc note + CLAUDE.md FT note (Phase 6a shipped; the manual path is observable), CHANGELOG, advance `00_README_v2.md` STATUS for Phase 6a, `post.md`. Correct the **stale FT docs** (CLAUDE.md:142 "Open FT work", ROADMAP Phase 4, OUTSTANDING_BACKLOG) to reflect FT-CLASSIFY-002/RECURSIVE-000 done + the real build queue.

## 6. Testing Plan (3 tiers)
- **Tier 1 (unit):** SF-3 distinct services; SF-7 per-gate reasons (3 cases); SF-1 staleness rule SQL idle-safe; Epic-4 stage-report row construction; SF-6 export prune; config-consumer guard for new env fields.
- **Tier 2 (integration):** `mdemg ft-loop report-stage` writes to `scheduled_job_events` against the live TSDB; readiness staleness rule executes; `mdemg data status` renders per-gate reasons.
- **Tier 3 (live, required):** drive the real binary — report each of the 6 stages (success + one forced failure) → confirm rows in `scheduled_job_events` and a high alert on the failure; confirm `ape.reflect` shows its real not-ready reason; confirm the readiness-staleness rule is registered and idle-safe; confirm the 6 RSIC diagnostic alerts now carry distinct Services (no mutual suppression).

## 7. Commit Strategy
Sequential commits per epic on `reh3376_dev01`. Push → auto-PR. Each epic builds + lints + config-guards green before the next.

## 8. Verification Checklist
- [ ] SF-3: 6 diagnostic alerts carry distinct Services
- [ ] SF-7: per-gate not-ready reasons in `mdemg data status` (ape.reflect reason visible live)
- [ ] SF-1: `training_readiness_stale` rule registered, idle-safe, config-floored
- [ ] Epic 4: each `ft-loop:<stage>` lands in `scheduled_job_events`; failure alerts (live)
- [ ] Epic 5: augmented-eval manifest committed + leak-audit clean
- [ ] Epic 6: no stale `:8101` docstrings; export retention works
- [ ] build + lint + config-guard + neural pytest/ruff green
- [ ] stale FT docs corrected; CHANGELOG + CLAUDE.md + 00_README_v2 STATUS + post.md
- [ ] NO actuator/autonomy/serving-mutation shipped (scope boundary held)

## 9. Documentation Update — Epic 7 above

## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| Scope creep into the actuator (6b) | Med | Med | Hard scope boundary in §3; this sprint ships observability only — no ledger, no trigger, no controller |
| SF-3 service rename breaks an existing alert consumer/cooldown expectation | Low | Low | Services are dispatcher-internal cooldown keys; distinct keys is strictly more-correct (NOSILENT-001 precedent) |
| Stage-report CLI is a no-op nobody calls (a new dead seam) | Low | Med | Tier-3 exercises all 6 stages live; the doc shows the operator/wrapper invocation; 6b's controller will be its first automated caller |
| Pinned eval artifact drifts from the recipe | Low | Med | Manifest stores recipe params + SHAs + leak verdict; rebuild check in Tier-2 |

## 11. Documents Accessed
- `docs/development/ft-recursive-001/SPEC_recursive_retraining_loop.md` (authoritative design)
- `docs/development/ft-classify-002/{post,run_record}.md` (the manual-slice evidence base)
- `internal/ape/{task_dispatch,self_assess}.go`, `internal/tsdb/dataset_builder.go`, `internal/jobhealth/jobhealth.go`, `internal/api/handlers_training_data.go`, `internal/alert/rules.go`
- `scripts/{build_clean_eval,audit_eval_leakage}.py`, `configs/benchmark_phase10.yaml`, `neural/training/evaluate_ft.py`
- Live `mdemg data status`, `scheduled_job_events` TSDB

## 12. Rollback Procedures
Pure-additive sprint (instrumentation + alert-hygiene + one config field). Revert the commit(s); no migrations beyond (if any) an additive one for the readiness heartbeat — reversible. No serving or data mutation to undo.
