# Architecture Map Optimization Report

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-03-24T22:35:41.285214Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 8.00 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| dep_pkg_graph | DEP | 150 | -3% | 8.0 |

## Question-Level Detail (Final Iteration)


### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no dependencies exactly as in the ground truth. |
| 2 | factual | 9 PASS | What does the api package depend on? | The agent's answer is accurate and complete except for missing the package "lrn" from the list. |
| 3 | relational | 4 **WEAK** | Why does ret depend on plg? | The agent mentions a dependency related to "MatchIngestionModule-fallback" but does not directly address the core functionality of the match-module (MatchIngestionModule) as stated in the ground truth. |
| 4 | relational | 9 PASS | Which package does hid depend on for emergence naming? | The agent correctly identifies the dependency of "hid" on "llm" for emergence naming but adds unnecessary detail about the package "hid." |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies the dependency on embedding-based constraint retrieval but adds unnecessary detail and slightly rephrases the concept. |
