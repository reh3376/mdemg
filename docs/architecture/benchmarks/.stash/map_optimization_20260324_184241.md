# Architecture Map Optimization Report

**Date**: 2026-03-24T22:42:41.320414Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 7.80 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| dep_pkg_graph | DEP | 150 | -3% | 7.8 |

## Question-Level Detail (Final Iteration)


### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no dependencies exactly as in the ground truth. |
| 2 | factual | 9 PASS | What does the api package depend on? | The agent's answer is accurate and complete except for the missing "lrn" package in the list. |
| 3 | relational | 4 **WEAK** | Why does ret depend on plg? | The agent mentions a dependency on "plg" for fallback functionality but misses directly addressing the core functionality of the MatchIngestionModule as stated in the ground truth. |
| 4 | relational | 8 PASS | Which package does hid depend on for emergence naming? | The agent correctly identifies the dependency of the package "hid" on "llm" for emergence naming but adds extra context not present in the ground truth. |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies the dependency on embedding-based constraint retrieval but adds unnecessary detail and slightly rephrases the concept. |
