# Architecture Map Optimization Report

**Date**: 2026-03-24T22:29:00.651881Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 7.50 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| uxts_frameworks | UXTS | 193 | 87% | 7.5 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated merge-blocking criteria with accurate details. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly provided the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 7 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner as demoted (GAP-04) but misses the critical detail about inline grading being missing and the risk of false passes. |
| 4 | factual | 7 PASS | Which framework has no runner at all? | The agent correctly identifies UAMS and the absence of a runner, but adds unclear or extraneous details not present in the ground truth. |
| 5 | factual | 4 **WEAK** | What is the UETS framework for? | The agent mentions the UETS framework and some details but misses the core concept of evaluating LLM emergence concept-naming quality. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all frameworks using soft-fail CI gating with accurate descriptions and no missing information. |
