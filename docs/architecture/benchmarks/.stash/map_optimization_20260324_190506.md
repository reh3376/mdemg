# Architecture Map Optimization Report

**Date**: 2026-03-24T23:05:06.788810Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 8.83 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| uxts_frameworks | UXTS | 261 | 83% | 8.8 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all the merge-blocking frameworks with accurate descriptions, matching the ground truth completely. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly stated the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 9 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner demotion due to missing inline grading and the GAP-04 reason, but slightly misrepresents the impact by not explicitly stating it would produce false passes. |
| 4 | factual | 10 PASS | Which framework has no runner at all? | The agent's answer accurately states that UAMS has no runner implementation and references GAP-21, matching the ground truth completely. |
| 5 | factual | 7 PASS | What is the UETS framework for? | The agent correctly identifies the UETS framework's purpose related to emergence evaluation of LLM concept-naming quality but does not explicitly state the evaluation of LLM emergence concept-naming quality itself. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all frameworks using soft-fail CI gating with accurate descriptions and no omissions. |
