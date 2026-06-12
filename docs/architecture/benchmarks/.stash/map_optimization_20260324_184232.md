# Architecture Map Optimization Report

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-03-24T22:42:32.108170Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 8.33 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| uxts_frameworks | UXTS | 200 | 87% | 8.3 |

## Question-Level Detail (Final Iteration)


### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated merge-blocking criteria with accurate details. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly provided the number of specs and variants but omitted the critical fact about the number of test cases. |
| 3 | relational | 8 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner as demoted (GAP-04) and its CI status as NONE, but misses the critical detail about inline grading missing and potential false passes. |
| 4 | factual | 7 PASS | Which framework has no runner at all? | The agent correctly identifies UAMS and the absence of a runner, but adds unclear or extraneous details not present in the ground truth. |
| 5 | factual | 8 PASS | What is the UETS framework for? | The agent correctly identifies the evaluation focus on LLM emergence concept-naming quality but adds the UETS framework without explanation, which is a minor detail not in the ground truth. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all four frameworks using soft-fail CI gating with accurate descriptions and no missing information. |
