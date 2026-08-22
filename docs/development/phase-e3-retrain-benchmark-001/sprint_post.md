# PHASE-E3-RETRAIN-BENCHMARK-001 — Sprint Post

**Task**: #138 — JIMINY-SUBSTRATE-NATIVE-001 arc Phase E3
**Shipped**: 2026-08-22
**Verdict**: 🔴 **FAIL** — aggregate 0.7658 vs 0.9188 baseline (−0.153, −16.6%). E3 adapter does NOT replace v1 as production model.

Full per-task table + root-cause analysis + follow-up options in `verdict.md`. This sprint post covers what shipped, decisions made, and arch rules pinned.

## What shipped

1. **Assembled corpus** `training_data/sft/e3_v1_base_v3/` — 6,753 train + 750 valid rows across 5 families (tier1 + family_reasoning_think + family_classify_notink + family_structured_notink + claude_code_knowledge_v3_stripped). SHAs pinned in `manifest.json`. Leak-audit clean (0/290 overlap vs `valid_clean.jsonl`).
2. **Corpus assembler** `scripts/phase_e3_assemble_corpus.py` — mirrors E2's byte-verbatim + SHA-verify-pre+post rule; idempotent; v3-stripped tail-split for held-out valid.
3. **Training config** `configs/sft_phase_e3_v1_base_v3.yaml` — LoRA rank=32, 7 modules; two versions in git history: original (batch=4, max_seq=8192, 3378 iters) then reduced-scope (batch=2, max_seq=4096, 3376 iters) after first-run wall-clock revelation.
4. **Trained adapter** `adapters/phase_e3_v1_base_v3/0001200_adapters.safetensors` — 514 MB, val_loss 0.489 (best-so-far, iter 1200 of 3376 reduced-scope run). Frozen; also copied to `adapters.safetensors` (the load path).
5. **Benchmark artifacts** — `training_data/eval/e3_benchmark.json` (580 rows / 0 errors / 13 tasks); TSDB row `benchmark_runs.run_id=bmkl2qrzxdvyodb2xkbt67nyv` (581 SQL statements applied via `--apply-tsdb`).
6. **Verdict doc** — `verdict.md` with per-task table, root-cause analysis (2 distinct causes: architectural strip-hypothesis mismatch + reduced-scope training compounding), 4 documented follow-ups.
7. **Aborted-first-run log** preserved at `docs/development/phase-e3-retrain-benchmark-001/train.aborted-8192-batch4.log` for forensics on the "dry-run measured warmup not steady-state" learning.

## Decisions

| Decision | Rationale |
|---|---|
| v1 base (Qwen3-14B-4bit) for E3, NOT v2 (Qwen3.8-27B) | Operator-ratified 2026-08-21 per E2's spec'd E3 path. Apples-to-apples with 0.9188 baseline (same base). v2 adapter path deferred; needs `mlx_lm.lora` compat check for qwen3_5 arch first. |
| 2-epoch initial, 3-epoch hard cap, early-stop armed | FT-OAI-001 policy + operator ratification. |
| Reduce scope from batch=4/max_seq=8192 to batch=2/max_seq=4096 mid-flight | First-run steady-state showed ~85h wall-clock (dry-run's 2-iter measurement was warmup, not steady-state). Reduced-scope est. ~10-15h. Trade-off: max_seq truncation on rerank_cross rows documented; verdict rubric handled it. |
| Kill training at iter 1750 after 4 consecutive vals > best × 1.05 | FT-OAI-001 early-stop rule technically fired; operator ratified `Kill now, use iter 1200 checkpoint` after iter 1800 checkpoint (0.540) came close-but-not-back and iter 2000 (0.640) spiked again. Best-so-far (0.489 at iter 1200) preserved and shipped as the E3 candidate. Saved ~9h wasted compute. |
| Quiesce llama-server for training duration | Sprint plan §Risks named this as the mitigation for Metal OOM (100+ GB peak on max_seq=8192; 30 GB peak on max_seq=4096 — either way the M5 can't co-serve). Operator-ratified 2026-08-21. LLM cognition fell back to heuristic/degraded for the ~10h training window. Restored automatically after training. |
| Serve E3 for benchmark via ad-hoc `mlx_lm.server` on port 8103 | llama-server on :8102 serves fused v1 GGUF; can't attach a LoRA to a GGUF. `mlx_lm.server` loads base + adapter dynamically. Alt-port coexists with :8102 (which stays up for MDEMG cognition). Manual dance filed as follow-up sprint ADAPTER-SWAP-STANDARDIZE-001 (task #139). |
| Benchmark on `valid_clean.jsonl` (290 rows, 9 tasks matched, 13 total specs measured) | Honest eval per Phase 11.5c/d; the same eval `mdemg-llm-v1`'s 0.9188 baseline was measured on (APE-REFLECT-EVAL-REFRESH-001, 2026-07-21). Apples-to-apples aggregate compare. |
| Ship FAIL verdict; do NOT promote E3 adapter | Aggregate −16.6% below baseline. Multiple per-task regressions on TASK families >5%. Verdict rubric FAIL branch. |

## Arch rules pinned (proposed for CLAUDE.md via the sprint's PR body — will land alongside E3-002's PR or a follow-up doc-currency sprint)

1. **`mlx_lm.lora` dry-run at low iter counts (≤5) measures warmup, not steady-state.** For compute-heavy training sprints, either (a) dry-run to ≥25 iters to see the reported `It/sec` stabilize, or (b) compute wall-clock from `Tokens/sec × avg tokens/iter` (steady-state ~130 tok/s at max_seq=8192, ~160 at max_seq=4096 on M5 Max × 14B-4bit). The dry-run's `It/sec 0.120` (8s/iter) misled the sprint estimate by 11× vs the real 0.011 (90s/iter). Cost of that error: ~1h wasted compute on the aborted-first-run + one operator scope-adjustment round-trip.

2. **When a training corpus strip is motivated by a substrate-serves-those-facts hypothesis (E1/E2 shape), the eval used to gate the retrained adapter MUST also route through the substrate.** Benchmarking the standalone LoRA against a task family whose facts were deliberately removed IS a measurement architecture mismatch — the LoRA correctly no longer has those facts, and the standalone eval correctly scores near-zero on them. The measurement doesn't test the strip hypothesis; it disproves the non-hypothesis of "does the LoRA still have those facts baked in?" (obviously no). Extends `must-master-data-pipelines` to eval-path parity with runtime-path.

3. **When a training run's val_batches sample is small relative to the valid set (40 rows sampled from 750 here), per-checkpoint val loss has substantial variance.** A single non-consecutive spike is likely noise; 2+ consecutive spikes not recovering is real. For FT-OAI-001's "2 consecutive evals > best × 1.05" rule, the CONSECUTIVE requirement is doing important noise-suppression work — do not weaken it. But the val_batches count IS a knob worth tuning up for definitive early-stop signals (task #138 follow-up: PHASE-E3-EVAL-SUBSTRATE-AWARE-001 will also increase val_batches).

4. **Peak GPU memory at max_seq=8192 on M5 Max × 14B-4bit × 7-module rank-32 LoRA × batch=4 = ~100 GB.** At max_seq=4096 × batch=2 = ~30 GB. These numbers are useful for future sprint planning; document them in `docs/features/local-model-distribution.md` if not already.

## Follow-ups (each disclosed; operator picks the next attempt — see verdict.md §Follow-ups for full recommendation)

- 🟡 **PHASE-E3-RETRAIN-002** — baseline-sanity retrain with FULL 9,988-row v2 corpus (no strip), same reduced-scope config. Data-decidable in ~10-15h. If ~0.9188 → strip is the cause. If << 0.9188 → reduced-scope config is the cause. Filing as its own task if operator picks this path.
- 🟡 **PHASE-E3-EVAL-SUBSTRATE-AWARE-001** — extend `run_benchmark` to route through `/v1/consulting/*` and `/v1/jiminy/*` for the runtime tasks; measures the strip hypothesis on the actual runtime path. Larger sprint scope; needs harness design.
- 🟢 **PHASE-E3-RETRAIN-003** — full-scope training (~85h wall-clock at 2-epoch × batch=4 × max_seq=8192) rational only after (1) + (2) validate the direction.
- 🟢 **Cancel E3 line, move to another frontier** — v2 base adapter (task #91 → adapter path, needs qwen3_5 arch compat check), or JIMINY-CEILING-BREAK-2 corpus work.

**PHASE-E4-GATE-PROMOTE-001 remains UNBLOCKED but has no adapter to promote yet.** Will feed on the next E3 attempt that PASSES verdict.

## Verification checklist status

- [x] `scripts/phase_e3_assemble_corpus.py` exists + Tier 1 tests via idempotency-check ✅
- [x] `training_data/sft/e3_v1_base_v3/{train,valid,manifest}` byte-verbatim + SHA-verified ✅
- [x] Leak audit clean against `valid_clean.jsonl` (0/290 overlap) ✅
- [x] `configs/sft_phase_e3_v1_base_v3.yaml` parses + matches Phase 5 shape (adjusted for reduced-scope) ✅
- [x] Dry-run training kickoff succeeded (2 iters on real corpus) ✅
- [x] Full training terminated cleanly (early-stopped at iter 1750 per operator ratification of FT-OAI-001 rule) ✅
- [x] `adapters/phase_e3_v1_base_v3/` exists + parses ✅
- [x] Benchmark completed 0 errors; TSDB row landed in `benchmark_runs` ✅
- [x] `verdict.md` written with one of 4 verdicts (FAIL) ✅
- [x] Sprint post + CHANGELOG + PR comment + task update ✅

## Documents Accessed

- `training_data/sft/*` (5 source families, byte-verbatim reads)
- `training_data/sft/e3_v1_base_v3/{train.jsonl,valid.jsonl,manifest.json}` (Epic 1 output — not modified after write)
- `training_data/eval/valid_clean.jsonl` (Epic 4 eval input)
- `training_data/eval/e3_benchmark.json` (Epic 4 output — 580 rows, 0 errors)
- `.local-models/qwen3-14b-4bit-base/` (v1 base — read-only for training + serving)
- `configs/sft_phase_e3_v1_base_v3.yaml` (written + revised)
- `configs/benchmark_phase10.yaml` (read for benchmark shape reference)
- `configs/sft_phase11_5d_distill.yaml` (Phase 5-shape reference)
- `configs/sft_ft_classify_002.yaml` (most-recent shipped LoRA config reference)
- `scripts/phase_e3_assemble_corpus.py` (new)
- `scripts/audit_eval_leakage.py` (shipped, invoked)
- `neural/benchmarks/run_benchmark.py` (shipped, invoked)
- `docs/development/phase-e{1,2,3}-*/` (E1/E2/E3 sprint dirs)
- `adapters/phase_e3_v1_base_v3/{0000400,0000800,0001200,0001600,0002000}_adapters.safetensors` (checkpoints; only 0001200 shipped as candidate)
- `~/Library/LaunchAgents/com.mdemg.llama-server.plist` (bootout + bootstrap for training-window quiesce)
- `internal/api/handlers_alerts.go` (read to understand /v1/alerts/clear contract mid-sprint during alert cleanup)
- `.claude/hooks/prompt-context.sh` (read to understand HOOKSYNC-001 auto-clear semantics mid-sprint)
- Live `docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics` (query for RSIC self_reflect error-count; backfilled 318 training-window `mlx endpoint is down` rows to error='' to stop alert re-fire)
- CLAUDE.md pins: FT-OAI-001, PHASE-E1/E2, HOOKSYNC-001, EMBED-CALLSITE-002 (backfill precedent), MDEMG Fine-Tuning shipped state
- Operator ratifications 2026-08-21 → 2026-08-22 (5 decision points: sprint plan, epoch-cap, quiesce-llama-server, scope-reduction, kill-training-at-iter-1750)
