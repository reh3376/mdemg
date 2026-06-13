# SIDECAR-LOOP-001 Recon (2026-06-13, HEAD) — verdict + root cause

Operator decision: **Fix-but-defer** — make the line coherent + trainable,
keep collecting until the (clean) dataset is large enough to train, do NOT
train/promote now (rerank is default-off). Keep only valuable data
(storage is finite).

## The data (live-counted)
- Sink: JSONL at `.mdemg/neural/training-data/training_*.jsonl` (NOT TSDB).
  `NEURAL_DATA_COLLECTION=true` in .env (config default false); still
  writing today.
- **6,814 events / 204,437 candidate rows.** Every event carries a
  top-level `rerank_scores[]` (0.0–1.0 LLM reranker output — teacher
  labels; 6,126 non-zero). So the data is conceptually a valid
  distillation set — NOT unlabeled (an earlier orchestrator pass wrongly
  concluded "0 labeled" by inspecting only `candidates[].score`).

## Root cause — labels are mis-correspondent in 100% of records
`internal/retrieval/rerank.go`: `scored` is built from the input
candidates, **sorted by FinalScore** (rerank.go:183), then `results` and
`rerankScores` are appended together from the sorted slice (:191-197) —
those two ARE positionally aligned and length-equal. **But the collector
is called with `req.Candidates[:topN]` (unsorted input) + `rerankScores`
(sorted output)** (rerank.go:208). Consequences:
- 84% of events: `len(candidates) != len(rerank_scores)` (topN ≠ returnK).
- The "aligned" 16% (len-equal): still positionally WRONG — input order
  vs rerank-sorted order. So candidate[i] is NOT the candidate that got
  rerank_scores[i].
⇒ **All pre-fix collected data is mislabeled and unusable for training**,
including the 16%.

## Trainer
`neural/neural_sidecar/train.py` fine-tunes `cross-encoder/ms-marco-
MiniLM-L-6-v2` (CrossEncoder, MSE, Spearman eval). Reads FLAT keys
`query`/`candidate`/`score`; collector writes NESTED `candidates[]`/
`rerank_scores[]` → 0 records ever parsed (min-samples 100 → hard fail).
Schema mismatch is the second half of the dormancy.

## Current rerank path
`rerank.go::Rerank` dispatches on `RERANK_PROVIDER` (default `openai` →
gpt-5.4-mini). `RERANK_ENABLED` default **false**; `NEURAL_RERANK_ENABLED`
default false. A trained sidecar reranker would distill the LLM
`rerank_cross` into a local cross-encoder — but the whole rerank path is
off by default, which is why training is deferred, not done now.

## Fix-but-defer deliverables
1. **Collector alignment fix** (the real bug): log `result.Results` +
   `result.RerankScores` (built-together, aligned) instead of
   `req.Candidates[:topN]` — every future record cleanly labeled.
2. **Collector guard**: drop any record where len(candidates) !=
   len(rerank_scores) (defense-in-depth; dataset clean by construction).
3. **Trainer schema reconciliation**: train.py reads nested records and
   explodes each event → (query, candidate_text, rerank_scores[i]) pairs.
4. **Existing-data disposition**: pre-fix data is 100% mislabeled →
   archive it out of the active training dir (reversible move, not hard
   delete; serves the operator's "no value-less data / finite storage"
   intent without destroying it outright). Fresh clean collection builds
   from the fix forward.
5. **Flag honesty**: NEURAL_DATA_COLLECTION documented as explicit opt-in
   (stays on here so the clean dataset builds).
6. **Readiness signal**: a clean-record count (gauge or doc'd threshold)
   so "large enough to train" is observable. Training itself deferred.
