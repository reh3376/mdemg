# Architecture Map Optimization Report

**Date**: 2026-03-24T22:33:12.883509Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 8.17 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| uxts_frameworks | UXTS | 200 | 87% | 8.2 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated purposes without missing any critical information. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly stated the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 8 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner as demoted due to GAP-04 and its exclusion from CI gating, but misses the detail about functional inline grading and the risk of false passes. |
| 4 | factual | 7 PASS | Which framework has no runner at all? | The agent correctly identifies UAMS and the runner as NONE but misses the specific designation "GAP-21" from the ground truth. |
| 5 | factual | 7 PASS | What is the UETS framework for? | The agent correctly identifies the UETS framework's purpose related to emergence evaluation of LLM concept-naming quality but does not explicitly mention "Evaluating LLM emergence concept-naming quality" as the core task. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all four frameworks using soft-fail CI gating with accurate descriptions and no missing information. |
