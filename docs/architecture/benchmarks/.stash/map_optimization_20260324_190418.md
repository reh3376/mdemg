# Architecture Map Optimization Report

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-03-24T23:04:18.723018Z  
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
| dep_pkg_graph | DEP | 301 | -99% | 8.0 |

## Question-Level Detail (Final Iteration)


### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no internal dependencies exactly as in the ground truth. |
| 2 | factual | 7 PASS | What does the api package depend on? | The agent correctly listed most dependencies but missed the package "lrn" from the ground truth. |
| 3 | relational | 7 PASS | Why does ret depend on plg? | The agent correctly identifies the delegation to plugin modules when core parsers fail, but does not explicitly mention the MatchIngestionModule functionality. |
| 4 | relational | 8 PASS | Which package does hid depend on for emergence naming? | The agent correctly identifies the dependency of the package "hid" on "llm" for emergence naming but adds unnecessary detail about the package "hid" that was not in the ground truth. |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies constraint retrieval via embedding similarity but adds unnecessary detail about "grd" and "emb" without fully matching the concise ground truth. |
