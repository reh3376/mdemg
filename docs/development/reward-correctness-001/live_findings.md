# REWARD-CORRECTNESS-001 — Live Tier 3 Findings

**Date:** 2026-06-13 · live run against the real stack (server :9999 healthy,
TSDB :5433 `mdemg_metrics`, llama-server :8102, real OpenAI for x9).

These are the results of scoring **real production responses** (not fixtures)
with the committed reward functions, per ULTS reward array, at the 0.8 distill
inclusion gate. They both validate Epic 1 and surface three larger
pipeline-correctness issues that the length-bias fix alone does **not** close.

## Method
For 5 generative/structured tasks, pulled the 60–100 most recent
`llm_interactions` rows with a non-empty response, scored each with the OLD
length-biased reward implementations and the NEW (Epic 1) length-neutral ones,
using each task's actual `reward_functions` array + `output_schema`. Reported
kept-rate @0.8 and how many real rows flip dropped→kept.

## Result table (old vs new kept @0.8)

| task | n | reward_functions | old kept | new kept | flipped→kept |
|------|---|------------------|---------:|---------:|-------------:|
| ape.reflect | 100 | json_valid, insight_count, actionability_score | 8 | 8 | 0 |
| jiminy.synthesize | 100 | follow_rate, coherence_score, specificity_score | 3 | 3 | 0 |
| **hidden.summarize** | 72 | coherence_score, coverage_score | **3** | **72** | **69** |
| hidden.name_emergence | 100 | json_valid, naming_quality_score | 100 | 100 | 0 |
| jiminy.evaluate | 100 | json_valid, evaluation_accuracy, explanation_quality | 0 | 0 | 0 |

## What Epic 1 fixed (validated)
**`hidden.summarize`: 69 of 72 real production rows recovered.** Its reward
array is exactly the two functions Epic 1 made length-neutral
(`coherence_score`, `coverage_score`). The old length ladder was dropping 96%
of real, valid, concise summaries below the gate — a direct instance of the
corpus-skew mechanism the audit named. Now kept. This is the length-bias fix
working on the real wire.

## What Epic 1 does NOT fix — three larger correctness issues the live run exposed

Per-function means on real rows isolate the actual suppressors:

### 1. ape.reflect (54k rows — the LARGEST training target): responses are TRUNCATED
`json_valid` mean = **0.133**. 23/30 sampled recent responses fail to parse —
"Unterminated string" / "Expecting ',' delimiter" at char ~2900–3300, ending
mid-string. The outputs are **cut off mid-generation**, producing invalid JSON.
Likely cause (per CLAUDE.md Local-LLM-Runtime note): ape.reflect production
prompts are ~5800 tokens and the llama-server KV slot is 32768/4 = **8192**
tokens; a multi-pattern array reflection needs ~3000 output tokens, so
prompt + output overflows the slot → truncation.

- **This is a production serving/capture defect, NOT a reward bug.** The gate
  *correctly* rejects truncated JSON; insight_count (Epic 1, =0.900) and
  actionability (0.611) are healthy. The 8/100 kept are the non-truncated rows.
- **Impact on the FT line:** the largest corpus is dominated by truncated,
  invalid responses. EVAL-INTEGRITY-001 "recovered" 71k ape.reflect rows by
  fixing hash-drift exclusion — but if ~87% of recent rows are truncated, most
  of that corpus is unusable for training and would poison a retrain.
- **Recommended follow-up (own sprint):** raise ape.reflect's effective output
  budget — increase `--parallel`-derived per-slot ctx (fewer slots → bigger
  slot), or chunk/cap the reflection so prompt+output < slot, or raise ctx-size.
  Then re-capture. Until then, ape.reflect rows must be json_valid-gated before
  any training use (the distill gate already does this).

### 2. jiminy.evaluate: `explanation_quality` is the WRONG reward for the schema
`json_valid` = 1.000, `evaluation_accuracy` = 1.000, but `explanation_quality`
= **0.000** → mean 0.667, below the gate, for *correct* responses. Cause: the
real schema is `{"violations":[...], "warnings":[...]}` — there is no top-level
`explanation`/`reasoning` key (explanation lives *inside* each violation
object), so `explanation_quality` (which reads top-level `explanation`/
`reasoning`) returns 0.0 for every response, including the correct empty
`{"violations":[],"warnings":[]}`.

- **This is a reward-array correctness bug** — a function that does not match
  the task's output shape, systematically penalizing correct answers. Same
  class as the length bias (reward not measuring correctness), different
  mechanism.
- jiminy.evaluate is NOT a current distill-capture target (only
  consulting.classify + retrieval.rerank_cross are), so this affects the
  benchmark/eval grade, not today's distill gate.
- **Recommended fix (scoped, needs operator sign-off — it changes a ULTS
  reward array + re-grades):** drop `explanation_quality` from
  jiminy.evaluate's `reward_functions` (keep `json_valid` +
  `evaluation_accuracy`), OR make `explanation_quality` schema-aware (credit
  per-violation reasoning when the schema nests it).

### 3. jiminy.synthesize: keyword-bag functions sit just below the gate
`coherence_score` = 0.900 (Epic 1, fixed), but `follow_rate` = 0.620 and
`specificity_score` = 0.637 → mean 0.719, just under 0.8. These are the
keyword-bag functions deliberately deferred from Epic 1 (`specificity_score`,
`actionability_score`, and `follow_rate` = their mean). They reward presence of
"specific/action" keywords and penalize "generic" ones — a brittle proxy that
drops valid concise guidance lacking the magic words.

- **Recommended follow-up:** the keyword-bag continuation named in the Epic 1
  commit — make specificity/actionability credit a substantive valid response
  at a correctness floor, with keyword presence as a small bounded *bonus*, not
  a gate. OR use the per-task threshold map (Epic 2) to gate synthesize at a
  calibrated bar reflecting its reward array's natural ceiling.

## Epic 2 validation (live)
`mdemg`'s x9 distill capture ran live against real OpenAI + TSDB with
`--reward-threshold-map '{"consulting.classify": 0.6}'`: 3/3 real prompts
captured, gate `0.6` applied (run header `gate=0.6`; manifest records
`reward_threshold_map` + per-task `reward_threshold: 0.6`). The per-task
override wires through end-to-end.

## Bottom line for the data-correctness campaign
Epic 1 was real and necessary (hidden.summarize: 69 rows recovered), but the
live run proves the **dominant** corpus-correctness problems for the big tasks
are not length bias — they are (1) **truncated production responses** on the
largest target (a serving-config defect) and (2) **reward functions that don't
match the task's output schema**. Both are squarely "data must be correct"
issues and are the highest-value next work. The prune phase (operator
directive) must treat truncated/invalid-JSON rows as non-conforming —
especially the ape.reflect corpus.
