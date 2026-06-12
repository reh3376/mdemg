# Architecture Map Optimization Report

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-03-24T23:06:59.800796Z  
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
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated functions, matching the ground truth completely. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly identified the number of specs and variants but omitted the number of test cases, a critical fact. |
| 3 | relational | 9 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner is demoted due to missing inline grading and its impact on CI gating, but slightly over-explains with map details not in the ground truth. |
| 4 | factual | 10 PASS | Which framework has no runner at all? | The agent's answer accurately states that UAMS has no runner implementation and references GAP-21, matching the ground truth completely. |
| 5 | factual | 8 PASS | What is the UETS framework for? | The agent correctly identifies the UETS framework's purpose related to emergence evaluation of LLM concept-naming quality but does not explicitly mention "Evaluating LLM emergence concept-naming quality" as the core task. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all four frameworks using soft-fail CI gating with accurate descriptions and no omissions. |
