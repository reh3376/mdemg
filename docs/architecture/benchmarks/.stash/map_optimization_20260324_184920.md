# Architecture Map Optimization Report

**Date**: 2026-03-24T22:49:20.595526Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 7.83 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| uxts_frameworks | UXTS | 253 | 83% | 7.8 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated purposes, matching the ground truth completely. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly stated the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 9 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner demotion due to GAP-04 and the need for inline grading before CI activation, but slightly rephrases and omits the detail about producing false passes. |
| 4 | factual | 3 **WEAK** | Which framework has no runner at all? | The agent's answer is fundamentally incorrect and does not provide the correct framework name UAMS (GAP-21). |
| 5 | factual | 8 PASS | What is the UETS framework for? | The agent correctly identifies the UETS framework's purpose related to LLM emergence concept-naming quality but uses less clear phrasing and omits the exact evaluation context. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all frameworks using soft-fail CI gating with accurate descriptions and no omissions. |
