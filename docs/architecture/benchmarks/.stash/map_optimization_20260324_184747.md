# Architecture Map Optimization Report

**Date**: 2026-03-24T22:47:47.191058Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 8.80 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| flow_jiminy_guide | FLOW | 204 | -11% | 8.8 |

## Question-Level Detail (Final Iteration)


### flow_jiminy_guide

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What triggers the Jiminy guide flow? | The agent correctly identified the hook name, script path, and that it runs on every prompt, but added an unnecessary detail about the "Jiminy guide flow" which was not in the ground truth. |
| 2 | factual | 9 PASS | What is the timeout for the guide orchestration? | The agent correctly states the timeout is 6 seconds and specifies the context, but the answer is slightly more detailed than the ground truth. |
| 3 | factual | 10 PASS | What are the three J17 encoding tiers and their approximate token sizes? | The agent correctly identified all three tiers with accurate token counts, traffic percentages, and descriptions matching the ground truth. |
| 4 | relational | 9 PASS | What parallel sources does the guide query? | The agent correctly identifies all main sources and concepts with minor differences in phrasing but no critical omissions. |
| 5 | relational | 7 PASS | What does jim.Effectiveness track? | The agent correctly identifies the role of guidance_id in tracking outcomes but adds unnecessary detail and misses the concise focus on feedback correlation. |
