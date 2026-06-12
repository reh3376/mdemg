# Architecture Map Optimization Report

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-03-24T22:56:47.351252Z  
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
| uxts_frameworks | UXTS | 260 | 83% | 7.7 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated functions without missing any critical details. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly stated the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 9 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner demotion due to GAP-04 and the need for inline grading, but slightly rephrases and omits the explicit mention of "functional but inline grading missing" and "would produce false passes." |
| 4 | factual | 2 **WEAK** | Which framework has no runner at all? | The agent's answer is largely irrelevant and incorrect, failing to identify UAMS (GAP-21) as the correct answer. |
| 5 | factual | 8 PASS | What is the UETS framework for? | The agent correctly identifies the UETS framework's purpose related to LLM emergence and concept-naming quality but uses less precise wording than the ground truth. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all frameworks using soft-fail CI gating with accurate descriptions and no omissions. |
