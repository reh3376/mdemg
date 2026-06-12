# Architecture Map Optimization Report

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-03-24T23:06:03.775004Z  
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
| dep_pkg_graph | DEP | 302 | -99% | 8.8 |

## Question-Level Detail (Final Iteration)


### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no dependencies exactly as in the ground truth. |
| 2 | factual | 9 PASS | What does the api package depend on? | The agent's answer is accurate and complete except for the missing "lrn" package in the list. |
| 3 | relational | 8 PASS | Why does ret depend on plg? | The answer correctly identifies that MatchIngestionModule is used as a fallback for unrecognized file types, but it lacks explicit mention of its core functionality as a match-module. |
| 4 | relational | 9 PASS | Which package does hid depend on for emergence naming? | The agent correctly identifies the dependency of "hid" on "llm" for emergence naming but adds extra context not present in the ground truth. |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies embedding-based similarity for constraint retrieval but uses unclear abbreviations and lacks clarity compared to the ground truth. |
