# Architecture Map Optimization Report

**Date**: 2026-03-24T22:59:24.636479Z  
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
| uxts_frameworks | UXTS | 263 | 82% | 8.8 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated functions without missing any critical details. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly provided the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 9 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner demotion due to GAP-04 and the need for inline grading before CI activation, but slightly rephrases and omits the detail about producing false passes. |
| 4 | factual | 9 PASS | Which framework has no runner at all? | The agent correctly states that UAMS has no runner and its status is unbuilt, matching the ground truth, but slightly rephrases without mentioning the tracking issue (GAP-21). |
| 5 | factual | 8 PASS | What is the UETS framework for? | The agent correctly identifies the evaluation focus on LLM emergence concept-naming quality but adds the UETS framework detail, which is not in the ground truth, making it mostly accurate with minor extraneous information. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all four frameworks using soft-fail CI gating with accurate descriptions and no omissions. |
