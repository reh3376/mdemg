# MODEL-SWAP-MUSE-GLIMMER-EVAL-001 — Sprint Plan (QUEUED)

> **STATUS: QUEUED — do NOT execute until the pick-up gate below is satisfied.** This is a research-only evaluation sprint. It produces a verdict document + benchmarks, not a production model swap. A separate `MODEL-SWAP-MUSE-GLIMMER-DEPLOY-001` sprint handles migration if the verdict is positive.

## 1. Header & Metadata

- **Sprint:** MODEL-SWAP-MUSE-GLIMMER-EVAL-001
- **Date queued:** 2026-08-11
- **Earliest pick-up:** 2026-08-17 (7 days post-beta-pipeline landing 2026-08-10)
- **Branch (when executed):** `reh3376_dev01`
- **Effort:** ~1 day (P1 pull + serve, P2 UBENCH, P3 UVTS 120q A/B, P4 verdict)
- **Provenance:** `docs/development/muse-glimmer-30b-investigation/RESEARCH_MEMO.md` (2026-08-11); Lever C of `docs/development/jiminy-follow-rate-decline-2026-08-10/`; operator hint 2026-08-11 ("Muse Glimmer 30B looks promising").

## 2. Problem Statement

Meta Superintelligence Labs released Muse Glimmer 30B on 2026-08-10 under Apache 2.0 — a dense, MLX+GGUF-available model with 131K context that fits MDEMG's architectural envelope. The research memo classifies it as PROMISING BUT NOT A DROP-IN SWAP because:

1. It was TRAINED for agentic tool-calling loops (Meta's whole pitch) — direct conflict with MDEMG's 9-pattern grep-audited no-tool-calling policy.
2. Its published benchmarks are all AGENTIC (MCP Atlas, DeepSearch QA, SWE-Bench Pro) — none of which correspond to MDEMG's 16 single-shot structured-output / reasoning call sites.
3. No published comparison to Qwen3-14B (the incumbent).

The eval sprint answers ONE question: **on MDEMG's specific task set (UBENCH + UVTS 120q against a live retrieval flow), does Muse Glimmer 30B match or beat the shipped `mdemg-llm-v1` Qwen3-14B LoRA baseline?**

## 3. Scope & Constraints

**In-scope:**
- Pull `muse-glimmer:30b-mlx` (or `:30b-q4_K_M`) via `mdemg model pull` (or direct `ollama pull` if MODEL-DIST-001 doesn't cover it).
- Serve on a NON-PRODUCTION port (llama-server on `:8103` or MLX server on a different port) so the shipped `mdemg-llm-v1` at `:8102` stays serving production traffic during the A/B.
- Run UBENCH aggregate against candidate + baseline with identical config.
- Run UVTS 120q A/B via `?llm_endpoint=` URL override on retrieval calls.
- Ship a verdict document with numbers, chat-template shape observations, and go/no-go recommendation.

**Out-of-scope:**
- Any production traffic mutation (default flip, `LLM_MODEL` env change, adapter rebuild).
- LoRA fine-tuning on Muse Glimmer (that's the P5 follow-up, only fires if base loses).
- Migration docs / MODEL-DIST-001 default update (that's the DEPLOY sprint).
- Cost / license compliance — Apache 2.0 already cleared in the research memo.

## 4. Dependencies

- Beta pipeline stability (pick-up gate — see §12).
- UBENCH framework (`docs/features/ubench-framework.md`, `docs/tests/ubench/specs/mdemg.ubench.json`).
- UVTS 120q corpus (`docs/tests/uvts/specs/lnl_demo_validation.uvts.json`) + `uvts_ab_compare.py` harness.
- llama-server binary (already installed for `mdemg-llm-v1` production serve).
- Optionally: `ollama pull muse-glimmer:30b-mlx` if we go MLX path; requires ollama 0.32.7+.

## 5. Implementation Plan (sequential — 4 phases)

**P1 — pull + serve (~1 hour):**
- Pull the model: `mdemg model pull --backend ollama --name muse-glimmer --quant 30b-mlx` (if MODEL-DIST-001 supports it out of the box) OR `ollama pull muse-glimmer:30b-q4_K_M` followed by manual GGUF symlink.
- Identify the GGUF blob under `<OLLAMA_MODELS>/blobs/sha256-<digest>`.
- Serve via a side-port `llama-server --model <blob> --port 8103 --ctx-size 32768 --parallel 4 --cont-batching --metrics --jinja` (mirror production shape).
- Probe: `curl -s http://127.0.0.1:8103/v1/models` returns the model card. `curl -s -X POST http://127.0.0.1:8103/v1/chat/completions -H 'Content-Type: application/json' -d '{"model":"muse-glimmer","messages":[{"role":"user","content":"reply with the word ok"}]}'` returns "ok" (validates chat template shape).
- Capture the chat template Jinja from the model card; note whether it requires a `tools` field.

**P2 — UBENCH aggregate (~30 minutes):**
- Run baseline (shipped `mdemg-llm-v1` on `:8102`): `python -m neural.benchmarks.run_benchmark --config configs/benchmark_phase10.yaml --out training_data/eval/benchmark_baseline_$(date +%Y%m%d).json --persist-tsdb`.
- Run candidate (Muse Glimmer on `:8103`): same command with `LLM_ENDPOINT=http://127.0.0.1:8103/v1 LLM_MODEL=muse-glimmer:30b` env override + `--out training_data/eval/benchmark_muse_glimmer_$(date +%Y%m%d).json`.
- Compute per-task delta. Look for regressions on the 3 highest-value tasks: `retrieval.rerank_cross`, `consulting.classify`, `jiminy.synthesize` (JIMINY-CEILING-INVESTIGATION-001 shortlist).
- Explicitly test single-shot NO-TOOL-CALLING contract: if any candidate row contains `tool_call`, `function_call`, `tool_use`, or `{"tools":` in the response, that's a red flag — Muse Glimmer's agentic training may be leaking. Document quantitatively (`spurious_tool_call_rate` = rows with tool-call artifacts / total rows).

**P3 — UVTS 120q A/B (~4-6 hours):**
- Baseline UVTS run — already in TSDB from routine ops; pull the most recent `mdemg-llm-v1` full-profile run: `mdemg data inspect --table uvts_results --since 7d`.
- Candidate UVTS run against `:8103`: `python3 docs/tests/uvts/runners/uvts_runner.py --spec docs/tests/uvts/specs/lnl_demo_validation.uvts.json --base-url http://localhost:9999 --profile full --persist-tsdb --env-override LLM_ENDPOINT=http://127.0.0.1:8103/v1 --env-override LLM_MODEL=muse-glimmer:30b` (needs `--env-override` support in the runner — if absent, wrap with launchctl setenv + kickstart to point production at the eval endpoint temporarily; note the production disruption + minimize the window).
- Compare with `uvts_ab_compare.py`: `python3 docs/tests/uvts/runners/uvts_ab_compare.py --baseline baseline.grades.json --candidate muse-glimmer.grades.json --spec lnl_demo_validation.uvts.json --out verdict.json`.
- Apply Note 02 strict merge gate: candidate mean ≥ baseline mean AND no per-question regression > 0.10.

**P4 — verdict + write-up (~1 hour):**
- Verdict document `docs/development/model-swap-muse-glimmer-eval-001/verdict.md`:
  - Per-task UBENCH deltas
  - UVTS aggregate + per-question worst-case
  - Spurious-tool-call-rate + observed chat-template shape
  - Latency comparison (median + p95 for the 3 high-value tasks)
  - Go/no-go recommendation
- If go: propose `MODEL-SWAP-MUSE-GLIMMER-DEPLOY-001` sprint.
- If no-go: close, document what failed, list next candidate models to evaluate (or defer entirely).

**P5 (conditional — only if P4 verdict is "close but not enough" AND we want to invest) — LoRA retrain on MDEMG corpus (~1-2 days):**
- Follow FT recursive-loop shipped path (`docs/features/ft-recursive-loop.md`) — export corpus → curate → train MLX LoRA → convert to GGUF → run gate benchmark → promote.
- Not automatic — requires explicit operator authorization to fire.

## 6. Testing Plan

**Unit (T1):** N/A — evaluation-only sprint; no production code changes.

**Integration (T2):** N/A — same reason.

**Live (T3):**
- P1: side-port serve + one probe response
- P2: full UBENCH run against both models
- P3: full UVTS 120q A/B
- Spurious-tool-call check across ALL P2 + P3 outputs

## 7. Commit Strategy

Single commit at end: `docs: MODEL-SWAP-MUSE-GLIMMER-EVAL-001 verdict + benchmark artifacts` (attaches verdict.md + benchmark JSONs + optional UVTS ab_verdict.json). No code changes — pure research artifact.

## 8. Verification Checklist

- [ ] Muse Glimmer serves cleanly on `:8103` (probe returns model card + can answer "ok")
- [ ] Chat template shape documented (does it require a `tools` field?)
- [ ] UBENCH aggregate captured for both baseline + candidate
- [ ] Per-task delta table produced
- [ ] Spurious-tool-call rate measured across all outputs
- [ ] UVTS 120q A/B run + `uvts_ab_compare.py` verdict captured
- [ ] Latency delta (median + p95) captured for `retrieval.rerank_cross`, `consulting.classify`, `jiminy.synthesize`
- [ ] `verdict.md` with go/no-go recommendation committed
- [ ] Deploy sprint proposed (if go) OR next-candidate list documented (if no-go)

## 9. Risks & Mitigations

**R1: Serving Muse Glimmer on a side-port interferes with production llama-server on `:8102`.**
- Side-port isolation is standard; both processes bind different ports and use different GGUF blobs. Memory pressure on M5 Max 128 GB is fine (Muse Glimmer Q5_K_M 19 GB + Qwen3-14B Q5_K_M 10 GB = 29 GB well under limit).

**R2: UVTS runner doesn't support `--env-override` for LLM_ENDPOINT / LLM_MODEL.**
- Fallback: temporarily point production `LLM_ENDPOINT` at `:8103` via `launchctl setenv MDEMG_LLM_ENDPOINT http://127.0.0.1:8103/v1` + kickstart mdemg, run UVTS, then restore. Production traffic during the ~4-6h A/B window will use Muse Glimmer instead of the incumbent — LIMITED-BLAST-RADIUS incident risk. Alternative: run UVTS during a low-activity window (e.g. overnight) or extend runner to accept the env override in a P0 mini-sprint before this one.

**R3: Muse Glimmer's tool-calling training produces spurious `{"tool_call": ...}` output on structured-output prompts even when no tools are advertised.**
- If `spurious_tool_call_rate > 5%` in P2, this is a hard NO-GO regardless of quality. Document as a shipping blocker.

**R4: 30B latency (2.1× params of Qwen3-14B) makes the retrieval-hot-path unacceptable.**
- Even with DFlash speculative decoding (M5 Max 50.2 tok/s claim), the retrieval-rerank p95 latency is already at 11.1s on the incumbent (LLM-HEALTH-INVESTIGATION-001). If Muse Glimmer's p95 rerank_cross > 15s, that's a NO-GO regardless of quality lift — user-visible latency regression is worse than a modest quality regression on classification tasks.

**R5: Meta withdraws or relicenses the model between queue and pick-up.**
- Apache 2.0 is irrevocable for weights already published; low probability. If it happens, close the sprint and re-evaluate.

## 10. Rollback Procedures

N/A — no production changes shipped. If P1 side-port serve breaks anything, kill the `:8103` llama-server process; production `:8102` unaffected. Any environment overrides applied during P3 (per R2 mitigation) revert via `launchctl unsetenv` + kickstart.

## 11. Pick-up Gate (READ BEFORE EXECUTING)

Do NOT start this sprint until ALL of the following are true:

1. **Beta pipeline has been live and stable for 7 days.** Landed 2026-08-10; earliest pick-up 2026-08-17. Check: `mdemg data inspect --table scheduled_job_events --since 7d` shows no HIGH `beta-*` failures.
2. **No unresolved CRITICAL / HIGH alerts on `~/.mdemg/alerts/current.json`.** The chronic `scheduled-job export-auto` alert was closed by EXPORT-SCRUB-INTAKE-001 on 2026-08-11; confirm no new chronic HIGH class emerged.
3. **No beta-arc follow-ups remain open** as blockers. Check `docs/development/beta-import-001/post.md` §Follow-ups + `docs/development/hitl-curation-003/` state.
4. **JIMINY-HEURISTIC-DEFAULT-001 passive re-check has landed** (24-48h post-2026-08-10 ship; verify gauge deflated toward ~11-12%). If gauge stabilized at unexpected level, sequence that follow-up FIRST.
5. **Operator authorization to start.** The queue-vs-execute decision is the operator's, not automatic.

If ANY of the above is false, DEFER. Update this doc with the pick-up-attempt observation and re-queue.

## 12. Documents Accessed

- `docs/development/muse-glimmer-30b-investigation/RESEARCH_MEMO.md` — the research foundation
- `docs/development/jiminy-follow-rate-decline-2026-08-10/INVESTIGATION.md` — Lever C hint
- `docs/development/jiminy-heuristic-default-001/post.md` — Lever A precedent (passive re-check pattern)
- `docs/features/local-model-distribution.md` — MODEL-DIST-001 pipeline
- `docs/features/ubench-framework.md` — UBENCH shape
- `docs/tests/uvts/runners/uvts_ab_compare.py` — A/B harness
- `docs/features/ft-recursive-loop.md` — P5 retrain path (conditional)
- CLAUDE.md `Standing policies` §1 (no-tool-calling policy — the caveat R3 tests)
- CLAUDE.md `Testing — Live System Testing Is Required` (informs the "no mocks" P2+P3 approach)
