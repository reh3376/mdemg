# PHASE-E3-RETRAIN-BENCHMARK-001 — Sprint Plan

**Task**: #138
**Arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase E3 (LoRA reframe: retrain execution)
**Owner**: `reh3376`
**Scope**: LARGE (compute-heavy) — corpus assembly + LoRA retrain + benchmark + verdict. Multi-hour wall-clock.

---

## 1. Header & Metadata

- **Sprint name**: PHASE-E3-RETRAIN-BENCHMARK-001
- **Version**: 1.0
- **Author**: Claude (opus 4.7) + operator `reh3376`
- **Date**: 2026-08-21
- **Est. duration**: 1 focused session for planning + corpus assembly + kickoff (~1h); then multi-hour training in background (~2-6h wall-clock on M5 Max); then benchmark + verdict (~30-45 min).
- **Category**: dev / model training + benchmark

## 2. Problem Statement

The `mdemg-llm-v1` production LoRA was trained on 9,988 rows across 5 families (tier1 + family_* + claude_code_knowledge v1 + v2). Phase E1 audit proved 81.4% of the two claude_code_knowledge families (2,203 of 2,706 rows) are now retrievable from the mdemg-dev substrate — they trained the model to recall facts the substrate can serve directly. Phase E2 built the v3-stripped corpus (503 fact-recall rows the substrate CAN'T serve, byte-verbatim from v2).

**Gap**: no retrain has been run against the stripped corpus. The current production model still carries the redundant fact-recall training weight. Retraining with the leaner corpus should:
- Preserve (ideally slightly lift) the aggregate 0.9188 baseline by concentrating LoRA capacity on TASK-behavior + honest fact-recall gaps (not substrate-covered ones)
- Free the substrate + retrieval path to be the primary fact-recall channel (architectural cleanliness — right tool for right job)

**Impact of not retraining**: v1 remains the production adapter indefinitely; the E2-shipped stripped corpus provides no runtime benefit; PHASE-E4-GATE-PROMOTE-001 has nothing to promote.

## 3. Scope & Constraints

**In scope**:
- Assemble E3 training corpus (concatenate 5 family train.jsonl files; hold out valid rows per family; SHA-verify sources pre + post; leak-audit against `valid_clean.jsonl`)
- Author training config `configs/sft_phase_e3_v1_base_v3.yaml` mirroring the shipped Phase 5 / Run 7 shape (rank=32, 7 modules, batch=4, seed=0, grad_checkpoint, max_seq_length=8192)
- Compute E3 LoRA via `mlx_lm.lora` against `mlx-community/Qwen3-14B-4bit` (v1 base, unchanged — the same base v1 was trained on; apples-to-apples benchmark)
- Benchmark E3 adapter on `training_data/eval/valid_clean.jsonl` (290 rows, 9 tasks); persist result to `benchmark_runs` + `benchmark_results` via `run_benchmark --apply-tsdb`
- Verdict document: PASS (aggregate ≥ 0.9188 AND no per-task regression > 0.02) → hand off to Phase E4 with adapter path; FAIL → sprint post with diagnosis + follow-up options

**Out of scope** (each disclosed in §11):
- v2 base (Qwen3.8-27B) — separate sprint per operator scope decision 2026-08-21 (this session)
- Promote to production — that's PHASE-E4-GATE-PROMOTE-001 (blocked on this sprint completing PASS)
- GGUF conversion — Phase E4 concern (fuse → dequantize → GGUF pipeline is shipped in MODEL-DIST-002)
- Adapter distribution — Phase E4 concern (Ollama publish is shipped)
- Any UBENCH re-run — apples-to-apples benchmark against 0.9188 baseline (which was measured on `valid_clean.jsonl` per APE-REFLECT-EVAL-REFRESH-001) is sufficient for the E3→E4 gate
- Multi-adapter A/B or ensemble

**Constraints**:
- `mdemg-llm-v1` remains the running production model throughout — this sprint produces a CANDIDATE adapter; Phase E4 handles the swap
- FT-OAI-001 policy: epoch cap 3; early-stop `val_loss > best × 1.05` for 2 consecutive evals; `n_epochs=auto` disallowed (explicit iter count)
- Preserve source corpus dirs byte-verbatim (E2 rule)
- Leak audit against `valid_clean.jsonl` is a hard gate (E2 rule; not the pre-anti-leakage `valid.jsonl`)
- No hardcoded values (all knobs in yaml + CLI overrides)
- CUIDv2 for `run_id`, `adapter_id`
- max_completion_tokens ≥ 3000 (memory rule); latency budgets ≥ 15000ms
- Compute lease: this sprint's training will saturate the M5 Max — the shipped FT-RECURSIVE-002 controller pattern (compute lease + Quiesce) applies; retraining while `llama-server` serves live inference degrades both. Operator should quiesce llama-server manually OR run training in a low-traffic window.

## 4. Dependencies

- **PHASE-E1-CORPUS-AUDIT-001 SHIPPED** — provides the strip-list ✅
- **PHASE-E2-CORPUS-CURATION-001 SHIPPED** — provides v3-stripped corpus ✅
- **HOMEBREW-INSTALLER-QWEN-UPDATE-001+002 SHIPPED** — E4 has a distribution channel for whatever E3 produces ✅
- **`mlx-community/Qwen3-14B-4bit` LOCAL** — verified at `.local-models/qwen3-14b-4bit-base/` (2 shards, arch `Qwen3ForCausalLM`, 40 layers) ✅
- **`valid_clean.jsonl` LOCAL** — 290 rows, 9 tasks per Phase 11.5c ✅
- **`neural/.venv/`** — has `mlx_lm`, `run_benchmark`, `psycopg` (pinned by FT-BENCH-REFRESH-001) ✅
- **TSDB running** — `benchmark_runs` writer needs `docker compose up -d timescaledb` ✅ (verify at kickoff)
- **Free disk headroom**: adapter dir ~500 MB, checkpoint saves ~500 MB × N eval points; keep 5-10 GB free
- **Compute headroom**: M5 Max 128 GB RAM; grad_checkpoint keeps peak RSS < ~40 GB per Phase 5 experience

## 5. Implementation Plan (sequential epics + gates)

### Epic 1 — Corpus assembly

New script `scripts/phase_e3_assemble_corpus.py`:
- Reads 5 family train.jsonl files: tier1 (3150), family_reasoning_think (1530), family_classify_notink (1080), family_structured_notink (540), claude_code_knowledge_v3_stripped (503) — **total 6,803 train rows**
- Reads 4 family valid.jsonl files (v3-stripped has no valid.jsonl — hold out 50 rows from its 503 train to serve as valid, per row_id deterministic sort): tier1 (350), family_reasoning_think (170), family_classify_notink (120), family_structured_notink (60), + v3-stripped-holdout (50) — **total 750 valid rows**
  - Actually: to preserve v3-stripped byte-verbatim, hold out FROM the tail (last 50 rows by file order) as valid; the 453 remaining become E3's v3-stripped train contribution. Total train adjusts to 6,753.
- SHA-verify each source file pre-read (fail-hard on mid-run mutation)
- Emit `training_data/sft/e3_v1_base_v3/{train.jsonl,valid.jsonl,manifest.json}` byte-verbatim (raw line copy, no JSON round-trip — same rule as E2)
- Manifest fields: sprint, base_dataset_ver="e3_v1_base_v3", source SHAs per family, output SHAs, per-family row counts, deterministic ordering seed
- Leak audit: run shipped `scripts/audit_eval_leakage.py --eval training_data/eval/valid_clean.jsonl --against training_data/sft/e3_v1_base_v3/train.jsonl` — MUST exit 0 (hard gate)

**Deliverable**: `training_data/sft/e3_v1_base_v3/{train.jsonl,valid.jsonl,manifest.json}` + assembly script.

**Gate**: leak audit clean (0 overlap); SHAs post-verify identical to pre-verify on all 5 sources; total train + valid counts match expected (6,753 train + 750 valid).

### Epic 2 — Training config

New `configs/sft_phase_e3_v1_base_v3.yaml`:
- `model: "mlx-community/Qwen3-14B-4bit"` (points at HF cache; will pull if not present; local `.local-models/qwen3-14b-4bit-base/` also OK if it's the same digest)
- `data: "training_data/sft/e3_v1_base_v3"` (Epic 1's output)
- `fine_tune_type: lora`, `num_layers: 40`
- `lora_parameters.rank: 32`, `.scale: 2.0`, `.dropout: 0.05` — matches Phase 5 / Run 7
- `lora_parameters.keys`: 7 modules (self_attn q/k/v/o + mlp gate/up/down) — matches Phase 5
- `batch_size: 4`, `learning_rate: 1.0e-5`, `seed: 0`, `grad_checkpoint: true`
- `max_seq_length: 8192` (rerank_cross observed 5899 tokens per Phase 11.5d)
- `iters`: **derived** — 6,753 rows / batch=4 = 1,689 iters/epoch. Cap 3 epochs = 5,067 iters. Ship **2 epochs = 3,378 iters** as the initial run (early-stop can end sooner; 3-epoch cap is the hard ceiling per FT-OAI-001).
- `steps_per_eval: 100` (~34 eval checkpoints across 2 epochs — early-stop visibility)
- `val_batches: 20` (holds ~80 valid rows per eval)
- `save_every: 200`
- `adapter_path: "adapters/phase_e3_v1_base_v3"`
- Optimizer: adamw

**Deliverable**: yaml file + short config-choice rationale note in sprint dir.

**Gate**: yaml parses; `mlx_lm.lora --config <path> --dry-run` (if flag exists — else spot-check with `python -c "import yaml; yaml.safe_load(...)"`) validates schema.

### Epic 3 — Training run

Run `mlx_lm.lora --config configs/sft_phase_e3_v1_base_v3.yaml` in background with log redirect to `docs/development/phase-e3-retrain-benchmark-001/train.log`.

**Early-stop watch** (per FT-OAI-001):
- Tail the log periodically; when 2 consecutive evals show `val_loss > best_val_loss × 1.05`, kill the run and use the last checkpoint before the regression.
- Automate: `neural/training/early_stop.py` if it exists; else manual observation with a small watcher script.

**Deliverable**: `adapters/phase_e3_v1_base_v3/` (LoRA safetensors + optimizer state + adapter_config.json).

**Gate**: training terminates cleanly (either full 3,378 iters OR early-stopped); adapter safetensors exists + parses.

### Epic 4 — Benchmark

Run `neural/benchmarks/run_benchmark.py --config configs/benchmark_phase10.yaml --adapter adapters/phase_e3_v1_base_v3 --eval training_data/eval/valid_clean.jsonl --rows-per-spec 0 --n-runs 2 --mlx-timeout-s 300 --out training_data/eval/e3_benchmark.json --apply-tsdb`.

- `--rows-per-spec 0` iterates all matched rows (Phase 11.5d Epic 4 fix)
- `--n-runs 2` for reward variance
- `--apply-tsdb` applies the SQL sidecar directly (BENCH-SIDECAR-APPLY-001 shipped path) — requires `neural/.venv/bin/python` for `psycopg`
- Adapter served via `mlx_lm.server` OR loaded inline by run_benchmark (whatever the shipped path is — verify at kickoff)

**Deliverable**: `training_data/eval/e3_benchmark.json` + one row in `benchmark_runs` + N rows in `benchmark_results` (one per task).

**Gate**: benchmark completes 0 errors; aggregate score is real-valued; per-task scores present for all 9 tasks in valid_clean.jsonl.

### Epic 5 — Verdict

Compare E3 aggregate vs 0.9188 v1 baseline. Verdict logic:

| Aggregate delta | Per-task max regression | Verdict |
|---|---|---|
| ≥ 0.9188 (equal or better) | ≤ 0.02 | **PROMOTE** — hand off to Phase E4 |
| ≥ 0.9188 | > 0.02 on any task | **CONDITIONAL** — investigate the regression; may still promote if the regressing task is a known-low-priority family |
| 0.9188 - 0.010 ≤ agg < 0.9188 | ≤ 0.02 | **HOLD** — E3 recovers most of the corpus reduction; run a second training with different hyperparams before deciding |
| agg < 0.9188 - 0.010 OR per-task > 0.05 on TASK-behavior families | any | **FAIL** — do not promote; write sprint post with diagnosis + follow-up (larger corpus, different LR, different rank) |

**Deliverable**: `docs/development/phase-e3-retrain-benchmark-001/verdict.md` with per-task table + aggregate delta + verdict + rationale.

**Gate**: verdict is one of the 4 above; not "unclear" — operator can move to the next action deterministically.

### Epic 6 — Sprint post + CLAUDE.md pin + hand-off to Phase E4 (or FAIL follow-up)

- `sprint_post.md` with the shipped state, verification table, decisions, arc rules pinned (new: e.g. "when retraining LoRA against a corpus-audit-cleaned dataset, benchmark on the honest eval and compare against the pre-clean baseline — a clean lift proves the corpus reduction didn't lose signal")
- `CHANGELOG.md` [Unreleased] entry
- If PASS: task PHASE-E4-GATE-PROMOTE-001 filed with the adapter path + benchmark row_id as inputs
- If FAIL: follow-up sprint filed (options: expand corpus, change LR, change rank, different base) — operator picks the next attempt

**Deliverable**: 3 docs + task update.

**Gate**: verify_doc_env_vars.py clean; end-with-docs-accessed satisfied; PR comment posted.

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit tests

- `test_phase_e3_assemble_corpus_row_count` — assemble against fixture directories; assert output counts match sums
- `test_phase_e3_assemble_corpus_byte_verbatim` — output train.jsonl lines are byte-identical (SHA equality) to concatenated source lines
- `test_phase_e3_assemble_corpus_sha_verify_fires_on_mutation` — mid-run mutation of a source file causes fail-hard

### Tier 2 — Integration

- **Leak audit against `valid_clean.jsonl`** — shipped `scripts/audit_eval_leakage.py` MUST exit 0
- **Config parse** — `yaml.safe_load(configs/sft_phase_e3_v1_base_v3.yaml)` returns dict with all required keys
- **Dry-run training kickoff** — spawn mlx_lm.lora with a fixture 10-row corpus + iters=2 to verify config-to-runner integration works before spending real compute (~30s)

### Tier 3 — Live e2e (the actual sprint)

- Full corpus assembly on real data
- Full training run (2 epochs, 3,378 iters, early-stop-armed)
- Full benchmark on valid_clean.jsonl (290 rows × 2 runs)
- Verdict document with per-task numbers

## 7. Commit Strategy

Two commits (compute-heavy sprint benefits from a natural split):
1. **Setup commit** — corpus assembly script + assembled corpus files + training config + Tier 1/2 tests. Reviewable BEFORE any compute is spent.
2. **Results commit** — adapter (Git LFS if size warrants; otherwise checksum-only reference) + benchmark JSON + verdict.md + sprint_post.md + CLAUDE.md pin + CHANGELOG entry.

## 8. Verification Checklist

- [ ] `scripts/phase_e3_assemble_corpus.py` exists + Tier 1 tests green
- [ ] `training_data/sft/e3_v1_base_v3/{train,valid,manifest}.{jsonl,json}` exist; manifest SHAs match live files
- [ ] Leak audit clean against `valid_clean.jsonl`
- [ ] `configs/sft_phase_e3_v1_base_v3.yaml` parses; matches Phase 5 shape
- [ ] Dry-run training kickoff succeeds (2 iters on fixture)
- [ ] Full training terminates cleanly (natural end or early-stop)
- [ ] `adapters/phase_e3_v1_base_v3/` exists + parses as LoRA
- [ ] Benchmark completes 0 errors; TSDB row lands in `benchmark_runs`
- [ ] `verdict.md` written with one of 4 verdicts
- [ ] Sprint post + CHANGELOG + PR comment + task update

## 9. Documentation Update (final epic — never cut)

Epic 6 IS the doc update. Additionally:
- If PASS: update `docs/features/fine-tune-loop.md` (if exists) with E3's improvement over v1
- CLAUDE.md pin the "corpus-audit-driven retrain" rule surfaced by this sprint

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Training saturates M5 Max, degrades running `llama-server` | HIGH (multi-hour run on the same box that serves inference) | MEDIUM (user experience during training) | Kick off in low-traffic window OR quiesce llama-server for the training duration (documented in Epic 3 preflight) |
| Overfitting on 6,753 rows before 2 epochs | MEDIUM | LOW (early-stop catches it; save last-good checkpoint) | Early-stop armed at `val_loss > best × 1.05` for 2 consecutive evals per FT-OAI-001 |
| Corpus assembly bug (silent row loss / dupe) | LOW | HIGH (poisoned training run) | SHA-verify sources pre+post; row-count pin test; byte-verbatim rule (no JSON round-trip) |
| Leak audit false-negative | LOW | HIGH (measured lift is inflated) | Shipped `scripts/audit_eval_leakage.py` is the E2 rule's tool; if it exits 0, treat as clean; hand-inspect a random sample if paranoid |
| E3 aggregate < 0.9188 (corpus reduction lost signal) | MEDIUM | HIGH (blocks promote; sprint FAIL) | Verdict framework §Epic 5 handles all 4 outcomes; FAIL branch has documented follow-ups |
| `mlx_lm.lora` version drift / arch surprise | LOW | HIGH (training fails to start) | Dry-run kickoff at Tier 2 catches this before real compute; base + corpus already known-compatible from Phase 5 |
| Benchmark hits `--mlx-timeout-s` on longest prompts | MEDIUM | LOW (per-task NaN, easy to re-run) | Set `--mlx-timeout-s 300` (matches Phase 11.5c/d guidance); re-run affected tasks if any timeout |

## 11. Non-Goals (explicit — deferred to future sprints)

- **v2 base (Qwen3.8-27B) retrain** — separate sprint per operator scope decision 2026-08-21; needs mlx_lm.lora compat check for qwen3_5 arch first
- **Promote to production** — Phase E4 concern (PHASE-E4-GATE-PROMOTE-001, filed on PASS verdict)
- **UBENCH re-run** — apples-to-apples benchmark against 0.9188 (which was measured on valid_clean.jsonl) is sufficient for the E3→E4 gate; UBENCH is a separate contract
- **Ollama publish** — Phase E4 concern (MODEL-DIST-001/002 shipped path)
- **Adapter A/B or ensemble** — one-adapter-at-a-time per shipped LoRA production model
- **Change the training corpus family mix** — this sprint uses exactly the 5 families the E2 plan named; changing the mix is a separate hypothesis worth its own sprint

## 12. Documents Accessed

- `training_data/sft/*/` (5 families — counted, schema-verified, not modified)
- `training_data/sft/claude_code_knowledge_v3_stripped/manifest.json` (E2 manifest)
- `training_data/eval/valid_clean.jsonl` (290 rows, honest eval)
- `.local-models/qwen3-14b-4bit-base/config.json` (v1 base arch verify)
- `.local-models/qwen3.8-27b-mlx-4bit/config.json` (verified qwen3_5 arch — reason for scope-clarifying operator question)
- `configs/sft_phase11_5d_distill.yaml` (Phase 5-shape reference — lora params + optimizer + max_seq_length)
- `configs/sft_ft_classify_002.yaml` (FT-CLASSIFY-002 reference — most recent shipped LoRA config)
- `configs/benchmark_phase10.yaml` (benchmark spec — task list, weights, stagnation rules)
- `neural/benchmarks/run_benchmark.py` (referenced; actual invocation deferred to Epic 4)
- `neural/training/early_stop.py` (referenced; verified exists in `ls neural/training/`)
- CLAUDE.md pins: MDEMG Fine-Tuning shipped state (Phase 5, 0.9188 baseline, APE-REFLECT-EVAL-REFRESH-001, FT-OAI-001 policy), PHASE-E1-CORPUS-AUDIT-001, PHASE-E2-CORPUS-CURATION-001
- Task #91 MODEL-SWAP-QWEN27B-EVAL bake-off (referenced; v2 base is a separate concern)
- E2 sprint post + manifest (source-of-truth for corpus provenance + strip audit)
