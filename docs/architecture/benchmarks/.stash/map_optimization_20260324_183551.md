# Architecture Map Optimization Report

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-03-24T22:35:51.983805Z  
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
| flow_jiminy_guide | FLOW | 191 | -4% | 9.0 |

## Question-Level Detail (Final Iteration)


### flow_jiminy_guide

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 8 PASS | What triggers the Jiminy guide flow? | The agent correctly identifies the script and its trigger timing but adds an unexplained term "Jiminy guide flow" not present in the ground truth. |
| 2 | factual | 9 PASS | What is the timeout for the guide orchestration? | The agent correctly states the timeout duration and context but adds unnecessary detail not present in the ground truth. |
| 3 | factual | 10 PASS | What are the three J17 encoding tiers and their approximate token sizes? | The agent's answer accurately captures all key facts about the three J17 encoding tiers, including token counts and traffic percentages. |
| 4 | relational | 10 PASS | What parallel sources does the guide query? | The agent's answer correctly lists all key components from the ground truth with accurate details and no omissions. |
| 5 | relational | 8 PASS | What does jim.Effectiveness track? | The agent correctly identifies the use of guidance_id and its outcomes but adds extra detail about edges and outcomes not explicitly stated in the ground truth. |
