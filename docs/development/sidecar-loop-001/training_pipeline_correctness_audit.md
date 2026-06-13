# Training-Pipeline Data-Correctness Audit (2026-06-13)

**Trigger**: operator reported the last 3 LLM-adapter retrains were
discarded as worse than baseline and suspected data-pipeline
correctness. Three read-only audit lanes + orchestrator code
verification. **All claims below verified at file:line on HEAD.**

## Verdict
The retrain loop is corrupted by ONE class of defect — a label/score
decoupled from the data it describes — in THREE independent places. Any
one invalidates a retrain verdict; together they fully explain three
"worse than baseline" outcomes (which are, themselves, not credible
model judgments).

## Defect 1 — Corpus filter skews toward verbose output (PRIMARY for SFT/distill)
- Inclusion gate: `reward_score = mean(reward_vector) >= 0.8`, hardcoded
  global (`x9_distill_capture_v2.py:247,361,365`). Threshold inconsistent
  across capture scripts (x9=0.8, x10=0.7).
- Length/keyword-biased rewards under-credit correct-but-terse answers:
  `coverage_score` (reward_functions.py:271) caps concise at 0.7;
  `coherence_score` (:246) needs ≥2 sentences; `insight_count` (:372)
  rewards bullet count; `jiminy.synthesize`/`consulting.synthesis`
  (:307,:435 compositions) have NO correctness term.
- Result: `summarize.generate` and **`ape.reflect` (largest target, 500
  rows)** drop their terse-correct majority below 0.8. The
  FT-CLASSIFY-002 `summary_quality` fix special-cased ONLY
  `consulting.classify {type:none}` — the same mechanism is live and
  unfixed across ≥6 generative tasks.
- `balanced_sampler.py` upsamples SURVIVORS after the filter — amplifies
  the skew, cannot restore a dropped class.

## Defect 2 — Promotion eval is untrustworthy (mis-judges good vs bad)
- Gate evals on `valid_golden.jsonl` — the **99%-leaked** set
  (`benchmark_phase10.yaml:96`, `mdemg.ubench.json:40`). `valid_clean.jsonl`
  (0% leak) exists but is wired into no gate. `scripts/audit_eval_leakage.py`
  has ZERO automated callers (verified: grep across yml/go/Makefile empty).
- Baseline is a **frozen constant `0.8338`** (`rl_phase11.yaml:158`,
  `regression.py:4,42`), computed under the OLD biased reward + MLX serving
  form; never recomputed. Candidate compared apples-to-oranges.
- Serving-form mismatch: gate serves MLX (`regression.py:343`); production
  promotion form is GGUF Q5_K_M via llama-server (Phase 13.5). MLX
  side-server OOM-crashed mid-sweep (FT-CLASSIFY-002).
- No hard-fail on zero successful calls — 4 false-0.0 reports in one
  sprint (port rot `rl_phase11.yaml:39` :8101; model-name 404s).
- The `summary_quality` fix "invalidates ALL stored classify baselines"
  (run_record.md) — yet the gate still compares against them. Under the
  corrected reward, `consulting.classify` baseline was 0.9228, not the
  stored 0.668 — a "good adapter looks worse" mechanism was live.

## Defect 3 — DPO chosen/rejected positionally assigned (corpus-corruptor for DPO)
- `dpo_builder.py:150-157`: `chosen=interactions[0].response`,
  `rejected=interactions[1].response` by list ORDER; the
  `followed`/`contradicted` outcome that defines the preference is
  computed (`:116-117`) but used only for metadata, never to select the
  text. chosen↔rejected can be swapped → trains preference for the
  contradicted response. Metadata `chosen_similarity` describes a
  different pairing than the text. SFT/RAFT path verified CLEAN
  (name-keyed, same-row — `format_converter.py:93-114`).

## Recommended remediation (do NOT retrain until done)
1. **Reward correctness** — every generative reward must score a
   spec-correct answer ≥ threshold regardless of length; per-task
   inclusion thresholds (not one global 0.8); add a correctness/grounding
   term to synthesis rewards. Forcing function: a test that a known-correct
   terse golden answer for each task clears its gate.
2. **Eval integrity** — wire `valid_clean.jsonl` into the gate; run
   `audit_eval_leakage.py` IN the gate (fail on overlap); recompute the
   baseline under the fixed reward + GGUF form; serve candidates in GGUF;
   hard-fail on zero successful calls.
3. **DPO pairing** — select chosen/rejected by per-interaction outcome
   identity (needs an interaction_id↔outcome link), not list position;
   until then, do not train on DPO data.
4. Re-establish honest baselines, THEN consider a retrain.

This is multi-sprint (FT-RECURSIVE / training-integrity territory).
Scope/priority is an operator call.
