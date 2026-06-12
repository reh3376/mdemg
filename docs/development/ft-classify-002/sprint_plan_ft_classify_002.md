# Sprint Plan — FT-CLASSIFY-002: Distribution-Matched consulting.classify Distillation

## 1. Header & Metadata
Sprint: FT-CLASSIFY-002 (FT Phase 11.5f) · 2026-06-11 · branch
`reh3376_dev01` · Roadmap Q3 Phase 4 · effort ~4d wall (incl. ~3-5h
training run) · OpenAI spend ~$3 (teacher distillation; well under cap)
· risk medium (training run; promotion is operator-gated). **This is the
manual vertical slice the FT-RECURSIVE-001 spec names** — one task walked
through capture → curate → train → gate by hand; its run record feeds 6a.

## 2. Problem Statement
`consulting.classify` is a purpose-target connection-layer task scoring
**0.668 vs gpt-5.4-mini's 0.778** (full-sweep valid_clean, +11pp gap).
The 11.5d distillation attempt produced +2pp but was built on a
class-mismatched corpus: {must:22, must_not:10, should:3, should_not:1,
**none:0**} vs the production distribution measured live today —
**none 81.9%, must 12.2%, should 4.0%, must_not 1.6%, should_not 0.2%**
(4,028 parseable rows). The model never saw the dominant class.
Secondary roadmap item: `guardrail.evaluate` has 3 production rows ever
(all leaked; `GUARDRAIL_ENABLED=false` default) — accumulation must
start before any retrain is possible.

## 3. Scope & Constraints
**In**: class-stratified distill capture (extend
`x9_distill_capture_v2.py` with per-class stratification matching the
live distribution; teacher gpt-5.4-mini; reward ≥0.8 filter; 10-source
leak audit); ~200-pair corpus (target ±5pp per-class match incl. none);
LoRA continue-from-Phase-5-adapter training (`mlx_lm.lora`, rank 32
α=64 scale 2.0, batch 4, lr 1e-5, seq 8192, **explicit iters ≈2 epochs,
cap 3, early-stop val_loss>best×1.05×2**); full-sweep A/B gate vs the
Phase 5 baseline 0.8553 (16-task valid_clean v2 + synthetic — the
FT-RECURSIVE `[AMD-2]` rule); guardrail.evaluate accumulation switch-on
+ follow-up recorded; stage-by-stage run record (timings, jobhealth-class
observations) as 6a input. **Operator gate before promotion**: gate
results presented; fuse → GGUF → symlink swap only on explicit approval.
**Out**: full 6a jobhealth instrumentation (FT-RECURSIVE-001's scope —
disclosed); RL/DPO; guardrail retrain (data-starved — accumulation only);
recursive-loop actuator code.

## 4. Dependencies
`scripts/x9_distill_capture_v2.py`; `configs/sft_phase11_5d_distill.yaml`
(template); `adapters/tier1/` (Phase 5 adapter — resume base; NOT the
rolled-back Run-7 sandbox); `mlx-community/Qwen3-14B-4bit` (SHA pinned);
`training_data/eval/valid_clean.jsonl` + synthetic + baselines;
`neural/benchmarks/run_benchmark.py`; OpenAI API (teacher); llama-server
:8102 must stay up (always-on policy) — training competes for RAM
(~36 GB peak at Phase-5 scale; 11.5d's 50-iter run: 84.7 GB peak, 109 min).

## 5. Implementation Plan
Epic 0 plan · **Epic 1** stratified capture (class-classify candidate
prompts from their production responses; sample to the live distribution;
teacher calls; reward+leak filters; manifest with class-dist proof) ·
**Epic 2** train (config from 11.5d template, resume `adapters/tier1`,
explicit iters; background run with progress monitoring) · **Epic 3**
full-sweep A/B gate (candidate vs Phase 5 vs gpt-mini; aggregate ≥
baseline, consulting.classify materially up, no task regression >2pp) ·
**Epic 4** guardrail accumulation switch-on + follow-up · **Epic 5**
operator gate → (on approval) fuse → GGUF Q5_K_M → promote + rollback
note · **Epic 6** docs (00_README_v2 STATUS, CHANGELOG, post, run
record), push.

## 6. Testing Plan
Tier 1: stratification unit check (corpus class-dist within ±5pp of
target per class); leak audit 0-overlap; manifest SHAs. Tier 2: training
telemetry (val_loss trajectory, early-stop armed); benchmark harness
parity run. Tier 3 (live): full-sweep A/B on the real binary + llama
runtime against valid_clean v2 (the gate IS the live test); if promoted —
production smoke on :8102 + UBENCH contract.

## 7. Commit Strategy
Per-epic commits (capture script changes + manifest · config + run
record · gate results · docs) · artifacts >100 MB stay untracked (model
outputs; gitignored per training_data conventions) · push at gate-result
time (auto-PR) so the operator reviews gates with the PR open.

## 8. Verification Checklist
- [ ] Corpus class-dist matches production ±5pp/class (incl. none ≈82%)
- [ ] 0-leak audit vs all 10 sources; manifest committed
- [ ] Training: explicit iters, epoch cap, early-stop log present
- [ ] Full-sweep A/B: aggregate ≥0.8553; classify ↑; no task >2pp down
- [ ] guardrail.evaluate accumulation enabled + follow-up recorded
- [ ] Operator gate honored before any promotion artifact
- [ ] Run record (stage timings/failures) usable as 6a input
- [ ] 00_README_v2 STATUS + CHANGELOG + post

## 9. Documentation Update — Epic 6 (never cut).

## 10. Risks & Mitigations
None-heavy corpus teaches lazy-none → reward filter keeps only correct
teacher outputs AND the gate requires classify to RISE; if it ships
lazy-none the per-class eval slice exposes it (gate fails, normal
outcome). Training starves llama-server → run when idle, monitor; 11.5d
precedent: coexisted. Teacher disagreement with production labels →
reward ≥0.8 filter; disagreements dropped, spend bounded. Context-window
loss mid-sprint → plan + run record carry state (this file is the
handoff).

## 11. Documents Accessed
phase_11_5d_post.md (failure forensics); phase_5_sft_post.md (recipe);
00_README_v2.md; recon agent report (2026-06-11); live TSDB class
distribution (4,028 rows); x9_distill_capture_v2.py; valid_clean
manifest + baselines; consulting_classify.ults.json; ROADMAP §Phase 4.

## 12. Rollback Procedures
Pre-promotion: nothing to roll back (artifacts are files). Post-promotion
(only after operator approval): restore prior GGUF symlink target +
`launchctl kickstart -k com.mdemg.llama-server` (documented Stage-E
rollback); adapter + eval artifacts archived per 11.5e precedent.
