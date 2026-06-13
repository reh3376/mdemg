# Rerank Training-Data Collection

**Sprint**: SIDECAR-LOOP-001 (2026-06-13) · **Status**: collector fixed;
reranker training deferred (see below)

## Why
The neural reranker (a local cross-encoder serving as a cheaper
alternative to the LLM `rerank_cross`) is trained by distilling the LLM
reranker's relevance scores. `NEURAL_DATA_COLLECTION=true` passively
captures `(query, candidates, rerank_scores)` from live rerank calls to
`.mdemg/neural/training-data/*.jsonl`.

## The correctness fix
The collector logged `req.Candidates[:topN]` (unsorted input order)
against `rerankScores` (built in rerank-SORTED output order), so the
candidate↔score correspondence was broken in 100% of records — 84%
length-mismatched, the rest positionally wrong. It now logs
`result.Results` + `result.RerankScores`, which are appended together
from the same sorted slice (aligned 1:1). A `Collect` guard drops any
record where `len(candidates) != len(rerank_scores)`, or where the score
array is empty / all-zero — the corpus is clean by construction.

The pre-fix data (6,814 mislabeled events) was moved to
`.mdemg/neural/training-data-prefix-archive/` (recoverable; not deleted)
so clean collection rebuilds from the fix forward. Per the operator's
data-quality directive, only correctly-labeled data accumulates.

## Deferred: training the reranker
Reranker training + the shadow-A/B vs `rerank_cross` are **deferred**.
Two reasons: (1) rerank is default-off (`RERANK_ENABLED=false`), so it's
not on the critical path; (2) the broader training-pipeline correctness
audit (`docs/development/sidecar-loop-001/training_pipeline_correctness_audit.md`)
found the LLM-adapter eval + reward + DPO pipelines carry the same
mislabeling class — those are being remediated first (eval-integrity
leads). The reranker trainer's flat-vs-nested schema reconcile is part of
that deferred work. Collection continues so a clean dataset builds to a
trainable size in the meantime.

## Config
- `NEURAL_DATA_COLLECTION` (default false; opt-in) — enable passive capture
- `NEURAL_DATA_DIR` (default `.mdemg/neural/training-data`)
