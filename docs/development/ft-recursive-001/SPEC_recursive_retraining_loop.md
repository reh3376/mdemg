# FT Recursive-Retraining Loop — As-Built Audit + Buildable Spec

**Produced by sprint FT-RECURSIVE-000** (2026-06-11, doc-only — NO production
code ships with this spec). Every `file:line` below was verified on HEAD
**`17c283a`** (post-PR-#441). This repo merges several PRs/day: **builders
must re-verify each anchor before relying on it**; divergence is
authoritative over this document.

**Governance:** J17/Jiminy handshake performed against the live instance
(reachable). Disclosed: the `jiminy-governance` skill has no pinned
observations (`/v1/skills/jiminy-governance/recall` → 0 results) and scoped
guidance returned 0 items; executed under standing hook enforcement.

**Aligns-with / supersedes:**
- `docs/development/ft-lora/00_README_v2.md` (canonical FT plan): Phases
  6 (Recursive Cycle Automation) / 7 (RSIC Integration) / 9 (Monitoring)
  are NOT STARTED. This spec is their design document; it advances no
  STATUS phase.
- `ROADMAP_2026Q3.md` §4: the recursive loop stays deferred behind
  **FT-CLASSIFY-002** (Phase 4) — "the proving run for the
  production-row → retrain → regression-gate path." This spec honors that
  trigger (§6); it front-runs design only, never the build.
- Supersedes nothing. The operator prompt
  (`PROMPT_ft_recursive_loop_deep_dive_v2.md`) and its sub-agent review
  (2026-06-11; 7 binding amendments, all incorporated and marked `[AMD-n]`)
  are inputs, not ledger documents.

---

## 1. The central fact: the skeleton exists and is firing into a void, live

| Seam | Anchor (HEAD `17c283a`) | State |
|---|---|---|
| Readiness computed | `internal/tsdb/dataset_builder.go:442` (`Ready = TotalRows >= threshold && errorRate < 0.05 && HasSystemPrompt == TotalRows`); `AvgDailyRate :450`, `ProjectedDaysToReady :453` | **LIVE** |
| Readiness threshold | `dataset_builder.go:99` — `const DefaultReadinessThreshold = 500`, **global, hardcoded, zero env override** `[AMD-7]` (a no-hardcoding violation; fixed in phase 6b) | LIVE, misconfigured-by-design |
| RSIC senses it | `internal/ape/self_assess.go:235-236` populates `report.TrainingReadiness` | **LIVE** |
| Insight emitted | `internal/ape/self_reflect.go:540` (`PatternID: "training_data_ready"`), `:543` (`RecommendedAction: "trigger_training_pipeline"`); fires every cycle while `readyCount > 0` — no has-training-started gate | **LIVE, ungated** |
| Actuator | `internal/ape/task_dispatch.go:369-370` → `executeAlertLog` — logs, counts, alerts (`task_dispatch.go:1154`: Service `"rsic"`, SeverityMedium), returns. **No curation, training, gating, or promotion.** | **NO-OP — exhibit A of the dead-seam inventory** |
| Action class | `internal/ape/types_rsic.go:59` — `trigger_training_pipeline` is in the **DiagnosticActions** allowlist ("only observe/alert without mutating state") `[AMD-6]` | classification must change before the actuator becomes real |

**Live evidence (2026-06-11, mdemg-dev):** `mdemg data status` shows
**3 tasks Ready=YES** — `retrieval.rerank_cross` (6,834 rows, 94.6/day),
`hidden.name_emergence` (2,072), `hidden.reclassify` (705) — so
`readyCount=3` re-fires insight #29 on every assessment cycle, and ≥9
"RSIC: trigger_training_pipeline — Training data threshold reached" alerts
landed in one working session (20:52–22:57Z). Total corpus: 98,006
guidance-correlated interactions across 13 tasks. Notable: `ape.reflect`
has 70,880 rows yet Ready=NO — it fails a non-row gate (error rate or the
strict `HasSystemPrompt == TotalRows` condition); the build sprint must
surface per-gate failure reasons, not just the boolean.

**Therefore:** do NOT build a parallel readiness/trigger mechanism. The
sensing half exists and works. The job is to make the actuator real, under
the no-silent substrate, with the mutual-exclusion and gating design below.

---

## 2. As-built map (five stages + the RSIC spine)

### Stage A — Capture

| Component | Writes | Trigger | Live? | Notes |
|---|---|---|---|---|
| `internal/llmclient/recorder.go` (InteractionRecorder) | TSDB `llm_interactions` (V0002 + V0005 guidance_id + V0007 RAFT `retrieval_node_ids`/`retrieval_scores`/`system_prompt_hash` + V0008 instance_id) | every LLM call, all 16 call sites | **LIVE** — `SetDefaultRecorder` at `internal/cli/serve.go:255` BEFORE `api.NewServer` (init-order critical) | buffer overflow = FIFO-evict + one-shot alert; flush stats now visible via `mdemg_tsdb_writer_*{writer="llm_interactions"}` (TSDB-CONSUME-001) |
| `internal/retrieval/rerank_collector.go` `[AMD-4: lives in retrieval/, not llmclient/]` | local JSONL `.mdemg/neural/training-data/` | `Rerank()` success path | **GATED OFF** — `NEURAL_DATA_COLLECTION` default false (`config.go:3878`) | disk-only, no TSDB sink, no metrics; the SIDECAR-LOOP-001 5,517-sample corpus is historical |
| `internal/jiminy/protocol_data_collector.go` | local JSONL `.mdemg/neural/protocol-data/` | constraint evaluation (2 sites) | **GATED OFF** — `J17_PROTOCOL_DATA_COLLECTION` default false | disk-only; errors slog-and-drop |
| `tier_effectiveness_dataset.go` (`CollectDataset`) | aggregated JSON | — | **ORPHAN** — no production caller (tests only) | dead seam |
| `internal/embeddings/recorder.go` | TSDB `embedding_events` (V0006) | every embed via `CachedEmbedder` | **LIVE** (default-on) | if `EMBEDDING_CACHE_ENABLED=false` the cache layer is absent → recorder unwired with only a warn (EMBED-WIRE-001 class) |
| `internal/retrieval/retrieval_recorder.go` | TSDB `retrieval_events` (V0006) | every retrieve | **LIVE** (default-on) | `downstream_quality` + `oracle_node_id` columns have **no producer** — the RAFT oracle-feedback loop is schema-only |

### Stage B — Readiness + curation

- Readiness: §1. Callers of `TrainingDataReadiness`: `internal/cli/data.go:107`
  (`mdemg data status`), `internal/cli/data_check.go:223` (`--pre-campaign`),
  `internal/ape/self_assess.go:235` (RSIC).
- Python pipeline (`neural/training/`): `paradigm_router.py` is the single
  entry point (routes SFT/DPO/RAFT/curriculum through
  `quality_filter.py` → format conversion → `dataset_versioner.py`;
  `recurate.py` for the SHA-gated re-curation path). **Exactly one Go
  caller exists in the entire training pipeline:**
  `internal/cli/data_curate.go:45` subprocess-invokes `paradigm_router` —
  operator-initiated only. `quality_filter.py` enforces 8 gates including
  a privacy hard-reject; `dataset_versioner.py` enforces temporal
  (non-shuffled) splits, dedup, task balance, and the exogenous-ratio
  check.

### Stage C — Train

- `neural/training/train_ft.py` wraps `mlx_lm.lora --train` (the proven
  Phase 11.5d dense recipe). **Overfitting prevention `[AMD-1]`:**
  `n_epochs_type()` at `train_ft.py:106` hard-REJECTS `auto` (the
  FT-OAI-001 forcing function — verified); early-stop monitoring wraps the
  run (`val_loss > best × 1.05`, SFT mode). Both are enforced, but the
  thresholds are **code-baked, not env-tunable** — phase 6b parameterizes
  them (`LORA_N_EPOCHS_CAP`, `EARLY_STOP_VAL_LOSS_FACTOR`) without
  weakening the `auto` rejection.
- Conversion path (all runnable scripts, operator-invoked):
  `mlx_lm.fuse --dequantize` → `convert_hf_to_gguf.py` → `llama-quantize`
  (fused), or `scripts/mlx_adapter_to_peft.py` →
  `scripts/vendor/llama_cpp/convert_lora_to_gguf.py` (adapter,
  MODEL-DIST-002).
- RL/DPO: `neural/training/rl/` is code-complete with an injectable
  trainer (`mlx_adapter.py` implements the MLX side) but **invoked only by
  its test suite**; DPO has `pair_generator.py` and **no trainer**. Neither
  is on the loop's critical path (§7 fork 5 recommends SFT-only first).
- **Resource model `[AMD-3]`** (Phase 5 actuals, `phase_5_sft_post.md:81-82`):
  **9 h 7 m wall-clock, 36.03 GB peak RAM, ~85 GB transient disk**
  (28 GB bf16 dequant + 29.5 GB f16 GGUF + 14.5 GB checkpoints + 10.5 GB
  Q5_K_M). The compute lease must model RAM contention (~36 GB) and disk
  headroom (~85 GB), not a mythical 85 GB RAM.

### Stage D — Benchmark + gate

- `neural/benchmarks/run_benchmark.py` (Phase 10 harness; V0012
  persistence; CUIDv2 run ids; judge sampling isolated) + `llm_judge.py`,
  `variance.py`, `preflight.py`.
- Dual regression gate — both exist and are runnable today:
  `neural/training/regression_gate.py` (Phase 10 single-gate: no task
  regresses >5%, ≥2 tasks improve ≥2%, JSON validity ≥95%, aggregate ≥
  baseline; exit 0/1/2) and `neural/training/rl/regression.py` (Phase 11
  dual-gate: 5a vs Phase 5 baseline 0.8338, 5b vs fresh-merge ≤0.005).
- **Promotion-gate eval coverage `[AMD-2]` — binding:** the gate runs the
  **16-task augmented eval (leak-audited via
  `scripts/audit_eval_leakage.py`) PLUS `valid_clean.jsonl`** — never
  valid_clean alone. Precedent: 11.5d's "Stage-1 at gpt-mini parity" was a
  9-task-subset artifact; the 16-task eval flipped the verdict. The
  augmented set is a recipe (`scripts/build_clean_eval.py
  --target-per-task 20` + leak audit), not a committed file — phase 6a
  pins its construction + SHA as a versioned artifact. `valid_golden.jsonl`
  (~99% leaked) is barred from gating, permanently.
- UBENCH wraps the benchmark as a contract
  (`docs/tests/ubench/specs/mdemg.ubench.json`:
  `min_aggregate_weighted_score: 0.80`, `max_truncated_rows: 0`;
  `model_under_test.path/base_sha/endpoint` is how a candidate is pointed
  at it).
- Stale-doc note (corrected from the audit): `evaluate_ft.py`'s 8101
  references are **docstring examples only** — `--base-url` defaults to
  `None` (`evaluate_ft.py:952`); production is llama-server :8102. Phase 6a
  fixes the docstrings.

### Stage E — Promote / serve / rollback

- Serving: llama-server :8102 via
  `~/Library/LaunchAgents/com.mdemg.llama-server.plist`
  (`KeepAlive.SuccessfulExit=false`, `ThrottleInterval=30`), model path
  `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf`.
- Promotion = atomic symlink/rename swap of the GGUF target + llama-server
  restart (`launchctl kickstart -k`). Rollback = restore prior target +
  restart. Both are operator-manual today; no `mdemg model rollback`
  command exists.
- `mdemg model {pull,list,verify,remove,where}` is **pull-only from Ollama
  Library** — it cannot publish a locally-trained model. The loop's
  promotion is local-file + symlink, NOT distribution; publishing a new
  model to Ollama remains an operator release act (MODEL-DIST pattern).

### The RSIC spine — dispatch + validation mechanics

- Trace: `self_assess.go:235` → `self_reflect.go:530-549` (insight #29) →
  orchestrator → `task_dispatch.go:369` → `executeAlertLog` (`:1154`).
- **Action-class problem `[AMD-6]`:** `trigger_training_pipeline` sits in
  `DiagnosticActions` (`types_rsic.go:55-62`) — diagnostics bypass
  calibration tracking and the mutating-action safety machinery
  (RSIC-VALIDATE-001 fail-closed validation + snapshot/rollback). A real
  actuator is a long-running, state-mutating, exclusive-resource action —
  a class that does not currently exist. §7 fork 7.
- Admission: `orchestration_policy.go` reserves at admission
  (RSIC-STORM-001); cooldown key `(source, spaceID)`, default 300 s. The
  retrain trigger CANNOT reuse this scale — a retrain is ~9 h; it needs
  single-flight + a persistent did-we-already-train ledger, not a
  seconds-scale cooldown (§3 design).
- Alert behavior of the current no-op: Service `"rsic"` + SeverityMedium is
  shared by SIX diagnostic executors → dispatcher cooldown key collisions
  (`(Service, Severity)`) mean RSIC diagnostics suppress each other —
  the exact class CLAUDE.md's alert-system section warns about.

### Silent-failure inventory

| # | Failure | Anchor | Class (per §4 taxonomy) |
|---|---|---|---|
| SF-1 | `TrainingDataReadiness` query failure → insight #29 silently never fires (WARN log only); loop goes dormant with no alert | `self_assess.go:235-239` | 4 → needs an evaluator rule (readiness-assessment staleness) |
| SF-2 | Insight #29 has no training-in-progress / already-trained gate → permanent re-fire while ready | `self_reflect.go:531-548` | design defect, fixed by 6b ledger |
| SF-3 | RSIC diagnostic alerts share `("rsic", Medium)` cooldown key → mutual suppression | `task_dispatch.go:1154` | 6a fix: distinct Service per action |
| SF-4 | `retrieval_events.downstream_quality` / `oracle_node_id` have no producer — RAFT oracle loop is schema-only | V0006 + `api/server.go` adapter | recorded; out of loop-critical path |
| SF-5 | Disk-only collectors (rerank, protocol) error-and-drop with no metrics/jobhealth | `retrieval/rerank_collector.go`, `jiminy/protocol_data_collector.go` | only relevant if 6b enables them |
| SF-6 | Export artifacts accumulate in `os.TempDir()/mdemg-exports/` forever | `handlers_training_data.go` | hygiene, 6a |
| SF-7 | `ape.reflect` 70,880 rows but Ready=NO with no per-gate reason surfaced | `dataset_builder.go:442` | 6a: expose per-gate failure reasons |

### Dead-seam inventory

| # | Seam | Anchor |
|---|---|---|
| DS-1 (exhibit A) | `trigger_training_pipeline` → `executeAlertLog` no-op | `task_dispatch.go:369-370` |
| DS-2 | `CollectDataset` (tier-effectiveness aggregation) — zero production callers | `jiminy/tier_effectiveness_dataset.go` |
| DS-3 | RL trainer complete, invoked only by tests; DPO has pairs, no trainer | `neural/training/rl/`, `neural/training/dpo/` |
| DS-4 | `NEURAL_DATA_COLLECTION` / `J17_PROTOCOL_DATA_COLLECTION` collectors built, default-off, no consumer | `config.go:3878`, `:2506` |
| DS-5 | No Go invocation of any training/benchmark Python except `data_curate.go:45` | repo-wide grep |

---

## 3. Target design — the loop as a state machine

```
                            ┌────────────────────────────────────────────┐
                            ▼                                            │
 ACCUMULATE ──ready+fresh──► TRIGGER ──lease──► CURATE ──► TRAIN ──► CONVERT
   (existing                 (single-           (paradigm   (mlx_lm   (MLX→GGUF)
    dataset_builder)          flight +           router)     .lora)       │
        ▲                     ledger)                                     ▼
        │                                                            BENCHMARK+GATE
        │                                                            (16-task augmented
        │                                              FAIL──┐        + valid_clean +
        │                                             (normal│        dual 5a/5b)
        │                                              outcome)            │PASS
        │                                                    ▼            ▼
        └────────────── MONITOR ◄── promote ── CANARY ◄── PROMOTE-PENDING
                        (Phase 9)   or rollback             (operator gate,
                                                             first N cycles)
```

Every state transition reports `jobhealth.Report`
(`internal/jobhealth/jobhealth.go:27`, job_name=`ft-loop:<stage>`) and the
loop registers a staleness rule from day one (the BACKUP-RESTORE-VERIFY
"inverted NOSILENT" lesson: a default-on automated pipeline with zero
jobhealth coverage is forbidden).

**Trigger (replaces the no-op):**
- Conditions (ALL env-tunable, no-hardcoding): task `Ready` (existing
  logic) **AND minimum-fresh-row fraction**
  (`FT_LOOP_MIN_FRESH_FRACTION`, default 0.30 — rows newer than the last
  trained dataset's max(time); retrain on new signal, never the same
  corpus) **AND no cycle in the persistent ledger within
  `FT_LOOP_MIN_RETRAIN_INTERVAL_HOURS` (default 168)**.
- **Single-flight:** a persistent training-cycle ledger (new TSDB table
  `ft_training_cycles` — it already exists from V0002 with zero writers;
  the loop becomes its writer, which also revives the ft_* dashboard
  panels removed in TSDB-CONSUME-001). States:
  `triggered|curating|training|gating|promote_pending|promoted|failed|rolled_back`.
  An open cycle absolutely bars a new trigger — across restarts (DB-backed,
  not in-memory; the RSIC 300 s cooldown is the wrong tool at 9 h scale).
- **Compute lease + RSIC quiesce:** before TRAIN, acquire an exclusive
  lease (`FT_LOOP_COMPUTE_LEASE` — a DB row + local lockfile) that
  (a) pauses RSIC LLM fan-out (orchestration policy gains a `Quiesce(due)`
  switch — admission rejects all non-critical cycles while held),
  (b) requires ≥`FT_LOOP_MIN_FREE_DISK_GB` (default 100) free, and
  (c) is lease-expiring (`FT_LOOP_LEASE_MAX_HOURS`, default 14 ≈ 1.5× the
  9 h actual) so a crashed trainer cannot wedge RSIC forever — expiry =
  class-4 alert-and-halt.

**Gate (binding parameters):**
- Eval: 16-task augmented (leak-audited, SHA-pinned) + `valid_clean.jsonl`;
  dual 5a (vs current production aggregate, not the frozen 0.8338 —
  baseline is "the model being replaced") / 5b (vs fresh-merge).
- Overfitting `[AMD-1]`: epoch cap 3 (`LORA_N_EPOCHS_CAP`), early-stop
  `val_loss > best × 1.05` 2-consecutive, explicit integer epochs (the
  `auto` rejection stays a hard error). These are gate PARAMETERS the
  controller passes — machine-enforced, not operator lore.
- FAIL is a normal outcome: archive candidate + verdict row in the ledger,
  no promotion, no alert-storm (one medium alert, distinct Service).

**Promote / canary / rollback (single-host serving):**
- Default policy (§7 fork 3): operator-confirm for the first N=3 cycles
  (`FT_LOOP_AUTO_PROMOTE_AFTER`, 0 = never auto), then auto-promote with
  canary.
- Canary on a single host = **held-call replay**: before swap, replay a
  pinned probe set (`FT_LOOP_CANARY_PROBES`, default the UBENCH
  min_rows_per_task slice) against the candidate on a side port (:18102),
  compare to production outputs; structural divergence = no promote. After
  swap, the watchdog's existing state machine plus a
  `FT_LOOP_CANARY_WINDOW_MIN` (default 60) elevated-error tripwire
  auto-rolls-back (restore symlink + kickstart) — class 3.
- Promotion artifacts: ledger row + `ft_model_versions` row (second
  zero-writer V0002 table revived) with SHAs, gate scores, dataset id.

**Monitor (Phase 9):** post-promotion drift = production task-score trend
(from `llm_interactions` quality signals + UBENCH re-runs on schedule) vs
gate score; gauge + evaluator rule (`ft_production_drift`), plus the
staleness rule (`ft_loop_never_ran`) from day one.

**Auto-remediation taxonomy (every failure mode maps to exactly one):**
1. **Auto-retry/fail-open** — transient TSDB/llama-server errors during
   curation/benchmark (bounded retries, then escalate to 4).
2. **Auto-restart under budget** — the controller loop itself registers
   with `internal/supervisor` (panic/restart budget; nil-return on
   intentional completion).
3. **Auto-rollback** — canary regression or post-promotion drift tripwire.
4. **Alert-and-halt** — corrupt dataset, SHA-pin drift, leak-audit failure,
   gate-harness failure, lease expiry, disk floor: cycle → `failed`,
   high-severity alert (distinct Service `ft-loop`), ledger records cause.
5. **File an issue** — repeated class-4 on the same fingerprint, or any
   failure the taxonomy cannot classify (§3a).

### 3a. Auto-issue escape hatch (net-new — verified: no GitHub-issue automation exists)

- **Route through `internal/gaps`** (recommendation, §7 fork 6): a class-5
  failure records a `CapabilityGap` (the existing detector/store), and a
  new thin `gaps→issue` escalator files it — keeping one escalation path
  for "the system knows something is wrong it cannot fix" rather than a
  bespoke filer per subsystem.
- Mechanism: `gh issue create` via a scoped fine-grained PAT
  (`FT_LOOP_ISSUE_TOKEN_PATH`, issues:write on reh3376/mdemg only;
  GitHub Issues chosen by the operator over Linear for repo-local
  semantics).
- Payload (deterministic): failing stage, taxonomy class, captured error,
  dataset/adapter/run ids, TSDB row ids, cycle id, repro command.
  **Idempotent:** fingerprint = SHA(stage|class|error-signature); existing
  open issue with the fingerprint label → comment, not a new issue.
- No-silent guarantee: filing itself reports through `jobhealth.Report`
  (`job_name='ft-issue-filer'`); a failure to file fires a high alert.

---

## 4. Phased build plan (exit criteria; sequencing honors the FT-CLASSIFY-002 trigger)

**FT-CLASSIFY-002 (Roadmap Phase 4) executes FIRST as the manual vertical
slice** — one task (`consulting.classify`, distribution-matched) walked
through capture → curate → retrain → gate by hand, instrumented per 6a.
The loop generalizes what that run proves. No autonomous operation before
the manual path is proven; any reordering is an operator decision.

| Phase | Sprint | Scope | Exit criteria | Effort |
|---|---|---|---|---|
| **6a** | FT-RECURSIVE-001 | Harden+instrument the manual path: jobhealth on every stage of a manual run; distinct RSIC alert Services (SF-3); per-gate readiness reasons (SF-7); readiness-staleness rule (SF-1); 16-task augmented eval pinned as a versioned artifact; docstring/endpoint fixes; export hygiene (SF-6) | a manual end-to-end run (FT-CLASSIFY-002's) is fully observable: every stage lands in `scheduled_job_events`, failures alert | ~3d |
| **6b** | FT-RECURSIVE-002 | The actuator: training-cycle ledger (`ft_training_cycles` writer), trigger conditions (fresh-fraction + interval + single-flight), compute lease + RSIC quiesce, controller orchestrating the Python pipeline as supervised subprocesses, `TRAINING_READINESS_THRESHOLD` env (+ per-task overrides) `[AMD-7]`, epoch-cap/early-stop env parameters `[AMD-1]`, insight #29 gated on the ledger (SF-2) | RSIC trigger launches a real gated cycle end-to-end with operator-confirm promotion; FAIL path verified live; zero alert spam | ~4d |
| **7** | FT-RECURSIVE-003 | RSIC integration proper: new mutating-long-running action class (fail-closed per RSIC-VALIDATE-001, snapshot semantics defined for "promotion") `[AMD-6]`; canary + auto-rollback; auto-promote-after-N policy | a bad candidate auto-rolls-back in a live drill; action class validated fail-closed | ~4d |
| **9** | FT-RECURSIVE-004 | Monitoring + drift: `ft_production_drift` + `ft_loop_never_ran` rules, `ft_model_versions` writer, ft_* dashboard panels restored (reader-writer pairs complete), issue filer (§3a) | drift rule fires in a seeded drill; issue filer dedupes on repeat | ~2–3d |

Each build sprint carries the standard documentation epic (CHANGELOG,
`00_README_v2.md` STATUS advancement for its phase, feature doc
`docs/features/ft-recursive-loop.md` from 6b, cli-reference if a CLI
surface lands). New Python passes the neural pytest+ruff CI and the
UxTS drift checker (UXTS-CI-001).

---

## 5. Operator decision forks (recommended, NOT resolved)

| # | Fork | Recommendation | Rationale |
|---|---|---|---|
| 1 | Spec now vs front-run FT-CLASSIFY-002 | **Spec now (this document); build after FT-CLASSIFY-002** | the trigger is the roadmap's own gate; design has no dependency on it |
| 2 | Actuator architecture: extend the in-Go RSIC handler into a controller orchestrating Python subprocesses vs a dedicated training-controller service RSIC merely signals | **In-Go controller, supervised, subprocess-orchestrated** | reuses supervisor/jobhealth/config substrate; a second service adds an ops surface with no isolation benefit on a single host; revisit only if the loop ever leaves this machine |
| 3 | Promotion autonomy default + N | **Operator-confirm first 3 cycles (`FT_LOOP_AUTO_PROMOTE_AFTER=3`), then auto-promote w/ canary; `0` = never-auto stays available** | matches the prompt's own default; 3 cycles ≈ 3 weeks of evidence at the expected cadence |
| 4 | Canary design on single-host | **Held-call replay on a side port pre-swap + 60-min post-swap error tripwire** | true shadow traffic needs dual serving capacity the M5 Max doesn't have during/after a 36 GB retrain |
| 5 | RL/DPO in the loop now | **SFT-only first**; RL/DPO joins after the SFT loop survives ≥2 full autonomous cycles | RL trainer is test-harness-only today (DS-3); adding it multiplies failure modes in an unproven loop |
| 6 | Issue filer: via `internal/gaps` vs standalone; token model | **Escalate through `internal/gaps`; fine-grained PAT, issues:write, repo-scoped, path-configured** | one escalation path; the gap subsystem already models "known deficiency"; standalone filer duplicates it |
| 7 | `[AMD-6]` Action re-classification: keep `trigger_training_pipeline` diagnostic (controller listens out-of-band) vs new mutating-long-running RSIC action class | **New action class in phase 7; until then (6b) the action stays diagnostic and the controller consumes the insight via the ledger** | fail-closed validation (RSIC-VALIDATE-001) must wrap anything that mutates serving state; bolting a 9-h mutation onto the diagnostic path would bypass every safety invariant |

---

## 6. Live-evidence appendix (Tier 3, 2026-06-11)

- `mdemg data status` (live binary, live TSDB): 98,006 interactions /
  13 tasks; Ready=YES: `retrieval.rerank_cross` (6,834 @94.6/day),
  `hidden.name_emergence` (2,072 @195.7/day), `hidden.reclassify`
  (705 @9.7/day); `guardrail.evaluate` projected ready in 1 day;
  `ape.reflect` 70,880 rows Ready=NO (per-gate reason not surfaced — SF-7).
- Alert stream (one session, 20:52–22:57Z): ≥9
  `RSIC: trigger_training_pipeline — Training data threshold reached`
  firings — the no-op actuator's only output, demonstrating SF-2/SF-3 and
  DS-1 live.
- Anchors re-verified on HEAD `17c283a` (drift from the prompt's #439
  pins: `task_dispatch.go:307→369`; all others stable; full list §1/§2).
- Audit corrections made during verification: `evaluate_ft.py` does NOT
  hardcode :8101 (docstring example only; `--base-url` default None,
  `:952`) — one sub-agent claim corrected; `rerank_collector.go` location
  corrected per `[AMD-4]`.

## 7. Documents accessed

`PROMPT_ft_recursive_loop_deep_dive_v2.md` (full); sub-agent review report
(2026-06-11); `internal/tsdb/dataset_builder.go`;
`internal/ape/{self_assess,self_reflect,task_dispatch,types_rsic,
orchestration_policy}.go`; `internal/alert/dispatcher.go`;
`internal/jobhealth/jobhealth.go`; `internal/llmclient/recorder.go`;
`internal/retrieval/{rerank_collector,retrieval_recorder}.go`;
`internal/jiminy/{protocol_data_collector,tier_effectiveness_dataset}.go`;
`internal/embeddings/{recorder,ollama}.go`; `internal/cli/{serve,data,
data_check,data_curate,data_validate,model}.go`;
`internal/api/handlers_training_data.go`; `neural/training/{recurate,
dataset_versioner,quality_filter,paradigm_router,train_ft,evaluate_ft,
regression_gate}.py`; `neural/training/rl/{trainer,mlx_adapter,
regression}.py`; `neural/benchmarks/{run_benchmark,llm_judge,variance,
preflight}.py`; `scripts/{audit_eval_leakage,mlx_adapter_to_peft}.py`;
`docs/tests/ubench/specs/mdemg.ubench.json`;
`~/Library/LaunchAgents/com.mdemg.llama-server.plist`;
`docs/development/ft-lora/00_README_v2.md`; `ROADMAP_2026Q3.md`;
`docs/development/ft-lora/phase_5_sft_post.md:81-82`; live system
(`mdemg data status`, alert stream, TSDB).
