# Architecture Map Optimization Report

**Date**: 2026-03-24T22:47:14.131870Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 7.67 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| uxts_frameworks | UXTS | 216 | 86% | 7.7 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated functions without missing any critical details. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly stated the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 9 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner's demotion due to GAP-04 and the lack of CI gating, but slightly misrepresents the reason as "inline-grading-missing" rather than "functional but inline grading missing," which is a minor detail. |
| 4 | factual | 3 **WEAK** | Which framework has no runner at all? | The agent's answer is mostly irrelevant and incorrect, failing to identify UAMS (GAP-21) as the correct answer. |
| 5 | factual | 7 PASS | What is the UETS framework for? | The agent correctly identifies the UETS framework's purpose related to emergence evaluation of LLM concept-naming quality but does not explicitly state the evaluation of the emergence concept itself as in the ground truth. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all frameworks using soft-fail CI gating with accurate descriptions and no missing information. |
