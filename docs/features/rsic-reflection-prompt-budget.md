# RSIC Reflection Prompt Budget (ape.reflect)

**Sprint:** APE-PROMPT-BUDGET-001 (2026-06-13) · training-integrity remediation.

## Why
ape.reflect — the RSIC LLM reflection call, and the **largest LLM training
target** (54k production rows) — was emitting **~87% truncated, invalid-JSON
responses**. Live root-cause (REWARD-CORRECTNESS-001 `live_findings.md` +
APE-PROMPT-BUDGET-001 recon):

- The assembled user prompt had grown to **7489 tokens** (measured on real
  stored rows): Current Assessment JSON ~3895 tok (dominated by verbose TSDB
  dataset fields), 5-cycle history ~2693 tok, calibration ~274, system ~628.
- `llama-server` bounds each slot's KV cache to `--ctx-size / --parallel =
  32768 / 4 = 8192 tokens`. A 7489-token prompt leaves only **~700 tokens**
  for output.
- Real `tokens_out`: 191/200 invalid responses clustered at **490–520 tokens**,
  cut off mid-JSON exactly at the KV ceiling. The few valid rows had shorter
  prompts (down to 5719 tok) with room to finish.
- It was **not** a `max_tokens` cap (`MaxTokens` floors at 2000; valid rows
  exceeded 500) and **not** fixable by compression (already on by default).
  The prompt was **structurally unbounded** — it grows with task count,
  history depth, and dataset richness, so any fixed slot is eventually
  re-crossed.

This corrupted the corpus: ~87% of ape.reflect rows are unusable for training
(the distill gate's `json_valid` correctly rejects them, scoring 0.133), which
is a large part of why retrains over this data underperformed.

## How it works
`internal/ape/llm_reflector.go::buildUserPrompt` now assembles the prompt under
a **token budget** so output always has guaranteed headroom:

1. **Dataset gating** — the verbose TSDB dataset fields (`LLMPerformance`,
   `RetrievalDataset`, `EmbeddingDataset`, `TrainingReadiness`) are excluded
   unless `RSIC_LLM_REFLECT_INCLUDE_DATASETS=true`. They dominated the prompt
   (~3895 tok) but are rarely referenced by pattern detection. The scalar
   health/edge/orphan metrics the detectors actually use are **always kept**.
2. **History cap** — recent-cycle count is bounded by
   `RSIC_LLM_REFLECT_HISTORY_CYCLES` (default 3; was hardcoded 5).
3. **Budget guard** — if the assembled prompt still exceeds
   `RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS` (default 3500; `0` disables), it
   drops history **oldest-first**, then trims the assessment tail as a last
   resort, **logging loudly** what was dropped (never silent). `estimateTokens`
   is calibrated to the measured ~2.3 chars/token ratio (slightly conservative
   so the guard stays under the KV ceiling).

With the default 3500-token budget, output headroom is ~4000 tokens
(8192 − 3500 − ~628 system) — 5× the failing ~700, ample for a multi-pattern
insight array.

**Serving slot (Lever B) — considered, not applied.** Raising the per-slot KV
bound (`--parallel 4→2` or `--ctx-size` up) was evaluated as a complementary
margin but is unnecessary: the prompt budget alone restores ample headroom, and
the slot change would cost concurrency or KV RAM. It remains the documented
fallback if the prompt budget ever needs to be raised past what one slot allows.

## How to use / tune
All defaults are sane; no action needed. To adjust:
- `RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS` (default 3500, range 0 or
  [1000,7000]) — lower for more output headroom, `0` to disable the guard.
- `RSIC_LLM_REFLECT_HISTORY_CYCLES` (default 3, range [0,10]).
- `RSIC_LLM_REFLECT_INCLUDE_DATASETS` (default false) — set true to feed the
  TSDB dataset fields back into the prompt (raises prompt size; the guard still
  trims to budget).

Roll back to prior behavior: `INCLUDE_DATASETS=true`, `HISTORY_CYCLES=5`,
`PROMPT_BUDGET_TOKENS=0`.

## Validation
Tier 1: 6 unit tests (dataset gating, history cap, drop-history-under-budget,
trim-assessment-under-budget, under-budget-unchanged, estimator).
Tier 3 (live, 2026-06-13): after restart with the budget on, the 3 fresh
post-restart ape.reflect rows measured on the real stack were **3/3 valid JSON
(100%, up from ~13%)**, with `tokens_in` down to **2549–2596** (from ~7489 —
dataset gating + history cap alone cleared the 3500 budget, so the hard guard
never fired) and `tokens_out` 607–981 (complete arrays finishing naturally vs
the pre-fix ~507 KV ceiling).
