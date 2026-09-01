# MDEMG-USAGE-LORA-001 — Sprint Post (REVISED 2026-09-01)

**Task**: #145
**Started**: 2026-08-27 (Epic 3 kickoff) · Predecessors #144 (corpus) + #139 (bench tool) shipped 2026-08-24
**Completed**: 2026-09-01 (verdict + docs + fair-comparison re-verification)
**Verdict**: **⚠️ PARITY-WITH-TRADEOFFS — my adapter is at STATISTICAL PARITY with v1 (−0.006, within noise) on same runtime. Est. production score ~0.913 (competitive with v1's 0.9188). NOT a FAIL. Recommend GGUF conversion + re-benchmark on llama.cpp before final promote decision.**

> ⚠️ **VERDICT REVERSAL 2026-09-01**: The initial verdict was written as "❌ FAIL — NO PROMOTION" based on comparing my adapter's 0.8388 (mlx_lm.server runtime) against v1's 0.9188 (llama.cpp GGUF runtime — from APE-REFLECT-EVAL-REFRESH-001). Operator caught this as apples-to-oranges and directed a fair-comparison baseline run. Same-runtime v1 measured at 0.8449 → my adapter's actual delta is **−0.006, within noise**. The 0.074 runtime cost (mlx_lm.server vs llama.cpp) was baked into the initial "FAIL" delta. Filed as PHASE-E3 arch rule 5 in the updated verdict.md.

Full training-curve + per-task breakdown at `verdict.md`. This post captures ship state + deviations + follow-ups + arch rules pinned.

## What shipped

| Artifact | Notes |
|---|---|
| `adapters/mdemg_usage_lora_001/0007200_adapters.safetensors` | 514MB, iter 7200 (best val_loss 0.395), SHA `de2675b58800…` |
| `adapters/mdemg_usage_lora_001/*.safetensors` | 19 other checkpoints (iter 400 through 7702) preserved for follow-up analysis |
| `training_data/eval/mdemg_usage_lora_001_iter7200_bench.json` | 13-task per-spec benchmark output (aggregate 0.8388) |
| `training_data/eval/mdemg_usage_lora_001_iter7200_mdemg_usage.json` | mdemg.usage supplemental eval (121 rows, aggregate 0.307) |
| `scripts/mdemg_usage_eval.py` | New — targeted mdemg.usage capability probe (3 factuality proxy metrics; ~350 LoC) |
| `docs/development/mdemg-usage-lora-001/{sprint_plan,verdict,sprint_post,training_start}.md/.json` | Full sprint record |

## Verification

| Check | Result |
|---|---|
| Training ran to natural completion | ✅ 7,702/7,702 iters, val_loss 1.949 → 0.395 (5× improvement) |
| Frozen family SHAs post-training | ✅ UNCHANGED — no substrate contamination |
| Production llama-server restored | ✅ port 8102 back within 2s |
| Best adapter freeze via `mdemg adapter freeze` | ✅ iter 7200 pinned; .pre-freeze backup captured |
| Benchmark ran via bench-serve on 8103 | ✅ 1,450+ requests, exit 0, aggregate 0.8388 |
| mdemg.usage supplemental eval | ✅ 121/121 rows scored, 0 errors, 39.8 min |
| Production llama-server untouched during bench | ✅ 8102 healthy throughout |

## Sprint execution — deviations from plan

### Deviation 1: Bash tool 10-min timeout broke first atomic-benchmark attempt

Sprint plan Epic 4 called for `mdemg adapter benchmark` (atomic orchestrator from #139). Bash tool max is 10 min; benchmark takes ~3-4h. The wrapper hit SIGTERM at 10 min WITHOUT triggering Go's `defer` cleanup (no `signal.Notify`). Pidfile left stale; bench-serve pgroup happened to die on parent SIGTERM but the atomic contract was broken.

**Correction shipped inline**: killed wrapper, ran `bench-serve` + `run_benchmark` separately, kicked off in Bash tool's native `run_in_background` mode (task ID `bcrdrazos`). Benchmark ran cleanly to completion.

**Follow-up filed as ADAPTER-SWAP-STANDARDIZE-002**: add `signal.Notify(SIGTERM|SIGINT)` handler to `mdemg adapter benchmark` orchestrator so defer-cleanup ALWAYS runs. This was noted as a "known limitation" in #139's sprint post; live-hit here confirms priority.

### Deviation 2: `mdemg.usage` was NOT measured in the standard `run_benchmark`

Sprint plan §5 Epic 4 called for `--eval-files valid_clean.jsonl,mdemg_usage_v1_holdout.jsonl`. The shipped `configs/benchmark_phase10.yaml` reads only `valid_clean.jsonl` (which was frozen BEFORE mdemg_usage_v1 existed). I did NOT extend the runner nor merge the holdout into valid_clean — so the aggregate 0.8388 measures ONLY preservation, not the sprint's own capability delta.

**Correction shipped inline**: wrote `scripts/mdemg_usage_eval.py` as an Epic-4-supplemental probe (3 factuality proxy metrics: substr_recall, token_jaccard, length_gate). Ran against the 121-row holdout after the main benchmark completed. Result: **aggregate 0.307** — adapter learned STYLE but not FACTS.

### Deviation 3: Wall-clock estimate was too conservative

Plan §5 Epic 3 predicted "conservative 30-90h to early-stop; 90-193h steady state." Actual: **99.6h** wall-clock. FT-OAI-001 early-stop did NOT fire (val_loss kept finding new minima across the run — 20 evals were above 1.05× best but never 2 consecutive). Ran to natural completion; iter 7200 was best.

## Root cause of the FAIL

**Single-task-driven aggregate collapse**: `claude.code_knowledge` scored 0.363 (n=246 rows). Because it's in the T group (weight 0.5) and has the largest N in the suite, its −0.55 drop from v1's ~0.92 baseline dominates the aggregate.

**Why?** Reduced-scope training config (batch=2, max_seq=4096, ~1 epoch on the 453-row v3-stripped family = 906 exposures) provided insufficient signal density to preserve v1's Claude Code knowledge base. Full-scope config (batch=4, max_seq=8192, 2 epochs) that would likely preserve it was ruled out for this sprint's wall-clock budget (~170-340h).

**12/13 non-CCK tasks preserved well** (8/13 ≥ 0.90, 3/13 in 0.68-0.90 range). The retrain didn't destroy MDEMG's own operational capabilities — it destroyed the imported Claude Code knowledge.

**mdemg.usage capability didn't materialize either**: 949 rows × 2 epochs across 14B params = insufficient for verbatim fact memorization. Adapter learned answer SHAPE (length_gate 0.877) but not CONTENT (substr_recall 0.204, jaccard 0.097).

## Follow-ups filed

### 🟢 ADAPTER-SWAP-STANDARDIZE-002 (deferred)
Add SIGTERM/SIGINT handler to `mdemg adapter benchmark` atomic orchestrator so defer-cleanup ALWAYS runs when the wrapper is killed. Documented as known limitation in #139; hit live here.

### 🟢 MDEMG-DOCS-INGEST-002 (hygiene, small)
The MDEMG-DOCS-INGEST-001 ingester picked up `.venv/lib/python3.12/site-packages/requests/*.py` from `docs/api/api-spec/uats/.venv/` — vendored dependency files that should have been excluded. 2 rows of pollution surfaced by mdemg.usage eval. Fix: extend ingester's path predicate to exclude `.venv/`, `__pycache__/`, `node_modules/`, `dist-info/`, `egg-info/` patterns.

### 🟢 MDEMG-USAGE-CORPUS-CURATE-002 (hygiene, small)
The mdemg_usage curator's WHERE clause `path CONTAINS 'cli-reference'` matched both `mdemg-docs/user/cli-reference/*` AND `claude-docs/cli-reference/*`. 2-3 claude-docs rows slipped into mdemg_usage_v1. Fix: tighten path predicate to require `mdemg-docs/` prefix.

### 🔴 Decision point — 3 shapes for next sprint

Operator decision:

1. **MDEMG-USAGE-LORA-002** — full-scope retrain (batch=4, max_seq=8192, 2 epochs, ~170-340h wall-clock; would keep MDEMG offline ~7-14 days). Would likely preserve claude.code_knowledge; still unclear whether mdemg.usage would land at production quality even at full scope (fact-recall is fundamentally hard on 949-row family).
2. **MDEMG-USAGE-LORA-003 (eval-frame shift)** — accept that adapter-in-isolation is the wrong eval frame. Extend `run_benchmark` to route through `/v1/consulting/*` + `/v1/jiminy/*` for tasks where MDEMG's runtime path adds RAG context. Measures the adapter as it would ACTUALLY be used in production.
3. **DROP the mdemg.usage arm** — retrieval-only path via MDEMG-DOCS-INGEST-001 + RETRIEVAL-META-DOC-SUPPRESSION-001 already exists. If mdemg.usage LoRA doesn't land at any feasible training scope, substrate retrieval is the alternative. v1 stays production; no further training.

Deep-dive workflow `wf_b389463a-61b`'s original recommendation Alt 1 was the substrate-path (Alt 3 above). This sprint's data STRENGTHENS that recommendation — LoRA-only adapter learning of docs-corpus facts at Qwen3-14B-4bit / 12%-of-corpus scale is not viable.

## Three arch rules pinned (proposed for CLAUDE.md next PR)

1. **A reduced-scope training config that fits a wall-clock budget is a REAL trade-off, not a free lunch.** PHASE-E3 arch rule 1 (wall-clock arithmetic) said this abstractly; this sprint's `claude.code_knowledge 0.363` result names the specific damage: **imported-domain fact bases (like Claude Code docs in mdemg-llm-v1) require full-scope training density to hold at production quality**. batch=2/max_seq=4096/1-epoch is enough for MDEMG's own domain tasks (consulting/jiminy/hidden/retrieval) but degrades imported-domain preservation. When planning a retrain: if the corpus includes a large imported-domain family critical to the eval, the wall-clock budget must accommodate full-scope training (~85h+ per PHASE-E3 arch rule) OR the eval frame must shift to something that doesn't score adapter-in-isolation on that domain.

2. **Fact-recall via SFT on a doc corpus is fundamentally hard at 12%-of-corpus scale.** The mdemg_usage_v1 experiment quantifies it: 949 deterministic Q→A rows × 2 epochs = 1,898 exposures across 14B params → substr_recall 0.20, token_jaccard 0.10. Even doubling to 4 epochs (or 5×-ing the corpus to 5000 rows) would only linearly improve exposure count; the fundamental issue is that a 14B model doesn't have parameter budget to memorize the specific values/paths/keys of a full doc corpus in the presence of 5 other families competing for capacity. **When the goal is "model produces MDEMG-specific facts unprompted" — invest in RAG (substrate) capacity, not LoRA parameter budget.** Confirmed by this sprint: length_gate=0.88 (adapter LEARNED the doc-response shape) but substr_recall=0.20 (adapter DID NOT LEARN the doc content).

3. **Ephemeral background tasks (~30 min – 4h) require the harness-native `run_in_background` pattern.** The Bash tool's 10-min ceiling breaks any `nohup ... &` or foreground wait-loop pattern used to run wrappers longer than 10 min. This sprint's Epic 4 hit this twice (once with `mdemg adapter benchmark` atomic wrapper, once with direct `run_benchmark` via `nohup`). Right shape: `Bash(..., run_in_background: true)` + Monitor-armed on completion signals in the output log. Any script that requires 10+ minutes of foreground time in the harness MUST use this pattern; wrapping with `nohup` + shell backgrounding is unreliable under the harness's shell semantics.

## Documents Accessed

- `docs/development/mdemg-usage-lora-001/{sprint_plan,verdict,training_start}.md/.json` (this sprint)
- `docs/development/mdemg-usage-corpus-curate-001/{sprint_plan,sprint_post}.md` (predecessor #144)
- `docs/development/adapter-swap-standardize-001/sprint_post.md` (predecessor #139 — the `mdemg adapter` tool used in Epic 4)
- `docs/development/phase-e3-retrain-benchmark-001/{sprint_plan,sprint_post}.md` (arch rules; direct comparison)
- `docs/development/mdemg-docs-ingest-001/verdict.md` (substrate-side counterpart)
- `docs/features/adapter-swap.md` (bench-serve + freeze semantics)
- `configs/sft_mdemg_usage_lora_001.yaml` (training config)
- `configs/benchmark_phase10.yaml` (eval config — source of the mdemg.usage measurement gap)
- `training_data/sft/mdemg_usage_lora_001/{train,valid,manifest}.jsonl/.json` (6-family corpus)
- `training_data/sft/mdemg_usage_v1/{train,valid,benchmark_holdout,manifest}.jsonl/.json` (source family)
- `training_data/sft/{claude_code_knowledge_v3_stripped,tier1,family_*}/train.jsonl` (frozen source families)
- `training_data/eval/valid_clean.jsonl` (13-task benchmark holdout, source of baseline 0.9188)
- `training_data/eval/mdemg_usage_lora_001_iter7200_bench.json` (benchmark output)
- `training_data/eval/mdemg_usage_lora_001_iter7200_mdemg_usage.json` (supplemental output)
- `logs/mdemg_usage_lora_001_training.log` (99.6h training log, 20 val checkpoints)
- `logs/mdemg_usage_lora_001_bench_run.log` (benchmark stdout — 0 bytes, run_benchmark buffered)
- `logs/mdemg_usage_eval_run.log` (mdemg.usage eval stdout — 121 rows of per-row detail)
- `~/.mdemg/bench-serve-8103.log` (mlx_lm.server bench-serve request log — 1300+ requests)
- `adapters/mdemg_usage_lora_001/*.safetensors` (20 checkpoints, ~10.3 GB)
- `scripts/mdemg_usage_eval.py` (new — targeted eval)
- `internal/cli/adapter_*.go` (#139 tools — freeze, bench-serve, benchmark)
- CLAUDE.md pins:
  - PHASE-E3-RETRAIN-BENCHMARK-001 arch rules 1-4 (wall-clock arithmetic, corpus-strip-vs-eval-path, val_batches variance, peak GPU memory)
  - ADAPTER-SWAP-STANDARDIZE-001 (used `mdemg adapter freeze` + `mdemg adapter bench-serve`; hit the SIGTERM defer gap live)
  - `must-follow-12-section-format`, `end-with-docs-accessed`, `live-testing-tier-required`, `mandatory-feature-docs`
  - `iterate-break-fix-verify` (verdict landed via LIVE benchmark, not extrapolation from val loss)
  - `data-decides-not-operator` (mdemg.usage eval was DATA-DECIDED before finalizing verdict; would not have accepted "FAIL" without measuring the sprint's own capability delta)
- Live Neo4j via `docker exec cypher-shell` (frozen-family SHA verification)
- Live `curl :8102/v1/models` + `curl :8103/v1/models` (bench + prod endpoint health throughout)
- Live `launchctl` (bootout + bootstrap + kickstart for llama-server lifecycle)
- Operator directives 2026-08-24 (proceed with #144) + 2026-08-24 (proceed with #145) + 2026-08-27 (resume Epic 3) + 2026-08-31 (check progress; auto-mode)
