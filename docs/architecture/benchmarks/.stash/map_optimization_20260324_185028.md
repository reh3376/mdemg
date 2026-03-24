# Architecture Map Optimization Report

**Date**: 2026-03-24T22:50:28.270943Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 8.67 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| uxts_frameworks | UXTS | 253 | 83% | 8.7 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated functions without missing any critical details. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly provided the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 9 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner's demotion due to GAP-04 and its impact on CI status but slightly misrepresents the reason phrasing and omits the false passes detail. |
| 4 | factual | 8 PASS | Which framework has no runner at all? | The agent correctly identifies the framework and the gap but adds extra details about runner status and test runner that are not in the ground truth. |
| 5 | factual | 8 PASS | What is the UETS framework for? | The agent correctly identifies the evaluation focus on LLM emergence and concept-naming quality but adds the UETS framework detail not present in the ground truth. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all four frameworks using soft-fail CI gating with accurate descriptions. |
