# Architecture Map Optimization Report

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-03-24T23:04:41.667862Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 9.00 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| uxts_frameworks | UXTS | 261 | 83% | 9.0 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated purposes, matching the ground truth completely. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly identified the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 9 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner demotion due to missing inline grading and the CI status, but slightly misses the implication of false passes. |
| 4 | factual | 10 PASS | Which framework has no runner at all? | The agent's answer correctly states that UAMS has no runner implementation and references GAP-21, fully matching the ground truth. |
| 5 | factual | 8 PASS | What is the UETS framework for? | The agent correctly identifies the UETS framework's purpose related to emergence evaluation of LLM concept-naming quality but does not explicitly mention "Evaluating LLM emergence concept-naming quality" as the core concept. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all frameworks using soft-fail CI gating with accurate descriptions and no omissions. |
